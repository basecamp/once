package docker

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	ProxyImage = "basecamp/kamal-proxy:v0.10.3@sha256:0a66b4a1e719abd497c3bdcb15d9210882059688b67c4b7e76691201a785462e"
	labelKey   = "once"
)

const (
	stateFileDir  = "/home/kamal-proxy/.config/kamal-proxy"
	stateFileName = "once-state.json"
	stateFilePath = stateFileDir + "/" + stateFileName

	proxyStateFilePath = stateFileDir + "/kamal-proxy.state"
)

const (
	DefaultHTTPPort    = 80
	DefaultHTTPSPort   = 443
	DefaultMetricsPort = 1318
	deployTimeout      = "120s"
	proxyReadyTimeout  = 30 * time.Second

	proxyRecoveryTimeout = 2 * time.Minute
)

var ErrProxyRoutesNotRestored = errors.New("new proxy did not restore the saved routes")

type ProxySettings struct {
	HTTPPort    int `json:"httpPort"`
	HTTPSPort   int `json:"httpsPort"`
	MetricsPort int `json:"metricsPort"`
}

func UnmarshalProxySettings(s string) (ProxySettings, error) {
	var settings ProxySettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ProxySettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

type DeployOptions struct {
	AppName string
	Target  string
	Host    string
	TLS     bool
}

type Proxy struct {
	namespace *Namespace
	image     string
	Settings  *ProxySettings
}

func NewProxy(ns *Namespace) *Proxy {
	return &Proxy{namespace: ns, image: ProxyImage}
}

func (p *Proxy) Boot(ctx context.Context, settings ProxySettings) error {
	settings = withDefaultPorts(settings)

	unlock, err := lockNamespace(ctx, p.namespace.name)
	if err != nil {
		return err
	}
	defer unlock()

	if err := p.recoverInterruptedReplace(ctx); err != nil {
		return err
	}

	info, err := p.namespace.client.ContainerInspect(ctx, p.containerName(), client.ContainerInspectOptions{})
	if err == nil {
		if p.isCurrent(info.Container) {
			return p.ensureRunning(ctx, info.Container)
		}
		return p.replace(ctx, info.Container)
	}
	if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting proxy container: %w", err)
	}

	if err := p.pullImage(ctx); err != nil {
		return err
	}

	id, err := p.createContainer(ctx, p.containerName(), p.image, settings)
	if err != nil {
		return err
	}

	if err := p.startContainer(ctx, id); err != nil {
		return err
	}

	p.Settings = &settings
	return nil
}

// Upgrade replaces the proxy container when it was created from an image
// other than the one this version of Once pins, and finishes or undoes a
// replacement that was interrupted. It does nothing when there is no proxy
// container.
func (p *Proxy) Upgrade(ctx context.Context) error {
	unlock, err := lockNamespace(ctx, p.namespace.name)
	if err != nil {
		return err
	}
	defer unlock()

	if err := p.recoverInterruptedReplace(ctx); err != nil {
		return err
	}

	info, err := p.namespace.client.ContainerInspect(ctx, p.containerName(), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting proxy container: %w", err)
	}

	if p.isCurrent(info.Container) {
		return nil
	}
	return p.replace(ctx, info.Container)
}

func (p *Proxy) Destroy(ctx context.Context) error {
	unlock, err := lockNamespace(ctx, p.namespace.name)
	if err != nil {
		return err
	}
	defer unlock()

	containerName := p.containerName()

	if _, err := p.namespace.client.ContainerRemove(ctx, p.nextContainerName(), client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("removing proxy: %w", err)
	}

	if _, err := p.namespace.client.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{Force: true}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing proxy: %w", err)
		}
	}

	if _, err := p.namespace.client.VolumeRemove(ctx, containerName, client.VolumeRemoveOptions{Force: true}); err != nil {
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing proxy volume: %w", err)
		}
	}

	p.Settings = nil
	return nil
}

func (p *Proxy) Exec(ctx context.Context, cmd []string) error {
	output, err := p.ExecOutput(ctx, cmd)
	if err != nil && output != "" {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(output))
	}
	return err
}

func (p *Proxy) Remove(ctx context.Context, appName string) error {
	return p.lockedExec(ctx, []string{"kamal-proxy", "remove", appName})
}

// Deploy holds the namespace lock until kamal-proxy has finished the deploy,
// so the proxy can't be replaced while it's in progress. kamal-proxy saves the
// new target before the deploy returns, so interrupting it would leave the
// restarted proxy routing to a container the failed deploy then removes.
func (p *Proxy) Deploy(ctx context.Context, opts DeployOptions) error {
	return p.lockedExec(ctx, p.deployArgs(opts))
}

func (p *Proxy) containerName() string {
	return p.namespace.name + "-proxy"
}

func (p *Proxy) nextContainerName() string {
	return p.containerName() + "-next"
}

// Private

func (p *Proxy) isCurrent(info container.InspectResponse) bool {
	return info.Config.Image == p.image
}

func (p *Proxy) lockedExec(ctx context.Context, cmd []string) error {
	unlock, err := lockNamespace(ctx, p.namespace.name)
	if err != nil {
		return err
	}
	defer unlock()

	return p.Exec(ctx, cmd)
}

// replace swaps the existing proxy container for one created from the pinned
// image. The new container is created alongside the old one under a temporary
// name, and the old one is only stopped, so it can be started again if the new
// one fails. kamal-proxy's routes and certificates, and Once's own state file,
// live on the proxy volume, which the new container mounts too; the new proxy
// is accepted only once it has restored every route the old one had saved.
func (p *Proxy) replace(ctx context.Context, old container.InspectResponse) error {
	settings, err := settingsFromConfig(old.Config)
	if err != nil {
		return err
	}

	routes, err := p.savedRoutes(ctx, old.ID)
	if err != nil {
		return err
	}

	slog.Info("Replacing proxy container", "from", old.Config.Image, "to", p.image)

	if err := p.pullImage(ctx); err != nil {
		return err
	}

	nextID, err := p.createContainer(ctx, p.nextContainerName(), p.image, settings)
	if err != nil {
		return err
	}

	err = p.stopContainer(ctx, old.ID)
	if err == nil {
		err = p.startContainer(ctx, nextID)
	}
	if err == nil {
		err = p.waitForRoutes(ctx, p.nextContainerName(), routes)
	}
	if err != nil {
		slog.Error("New proxy container failed; restarting the previous one", "error", err)
		return errors.Join(err, p.rollBack(ctx, nextID, old.ID))
	}

	if err := p.promote(context.WithoutCancel(ctx), nextID, old.ID); err != nil {
		return err
	}

	p.Settings = &settings
	return nil
}

// rollBack removes the new container and starts the old one. It runs even when
// ctx is canceled, so a shutdown mid-replace doesn't leave the host without a
// proxy.
func (p *Proxy) rollBack(ctx context.Context, nextID, oldID string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), proxyRecoveryTimeout)
	defer cancel()

	if _, err := p.namespace.client.ContainerRemove(ctx, nextID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("removing new proxy container: %w", err)
	}
	if err := p.startContainer(ctx, oldID); err != nil {
		return fmt.Errorf("restarting previous proxy container: %w", err)
	}
	return nil
}

func (p *Proxy) promote(ctx context.Context, nextID, oldID string) error {
	ctx, cancel := context.WithTimeout(ctx, proxyRecoveryTimeout)
	defer cancel()

	if _, err := p.namespace.client.ContainerRemove(ctx, oldID, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("removing previous proxy container: %w", err)
	}
	if _, err := p.namespace.client.ContainerRename(ctx, nextID, client.ContainerRenameOptions{NewName: p.containerName()}); err != nil {
		return fmt.Errorf("renaming new proxy container: %w", err)
	}
	return nil
}

// recoverInterruptedReplace tidies up after a replace that didn't finish. If
// the old container was already removed, the new one had restored its routes,
// so it takes over the proxy name. Otherwise the new one is discarded and the
// old one is started again.
func (p *Proxy) recoverInterruptedReplace(ctx context.Context) error {
	next, err := p.namespace.client.ContainerInspect(ctx, p.nextContainerName(), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspecting proxy container: %w", err)
	}

	current, err := p.namespace.client.ContainerInspect(ctx, p.containerName(), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		slog.Info("Completing interrupted proxy replacement")
		if _, err := p.namespace.client.ContainerRename(ctx, next.Container.ID, client.ContainerRenameOptions{NewName: p.containerName()}); err != nil {
			return fmt.Errorf("renaming new proxy container: %w", err)
		}
		return p.ensureStarted(ctx, next.Container)
	}
	if err != nil {
		return fmt.Errorf("inspecting proxy container: %w", err)
	}

	slog.Info("Undoing interrupted proxy replacement")
	if _, err := p.namespace.client.ContainerRemove(ctx, next.Container.ID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("removing new proxy container: %w", err)
	}
	return p.ensureStarted(ctx, current.Container)
}

// savedRoutes returns the names of the services kamal-proxy has saved in its
// state file, which the replacement proxy must restore.
func (p *Proxy) savedRoutes(ctx context.Context, containerID string) ([]string, error) {
	res, err := p.namespace.client.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: proxyStateFilePath})
	if errdefs.IsNotFound(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading proxy state: %w", err)
	}
	defer res.Content.Close()

	tr := tar.NewReader(res.Content)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("reading proxy state: %w", err)
	}

	var services []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(tr).Decode(&services); err != nil {
		return nil, fmt.Errorf("decoding proxy state: %w", err)
	}

	names := make([]string, 0, len(services))
	for _, s := range services {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	return names, nil
}

// waitForRoutes waits for kamal-proxy in the named container to answer on its
// control socket, which it opens only after restoring its saved state, and
// checks that it restored the expected routes. kamal-proxy starts with no
// routes when it can't read its state file, so answering isn't enough.
func (p *Proxy) waitForRoutes(ctx context.Context, name string, expected []string) error {
	ctx, cancel := context.WithTimeout(ctx, proxyReadyTimeout)
	defer cancel()

	for {
		routes, err := p.listRoutes(ctx, name)
		if err == nil {
			if !slices.Equal(routes, expected) {
				return fmt.Errorf("%w: expected %v, got %v", ErrProxyRoutesNotRestored, expected, routes)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for proxy to start: %w", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (p *Proxy) listRoutes(ctx context.Context, name string) ([]string, error) {
	result, err := execInContainer(ctx, p.namespace.client, name, []string{"kamal-proxy", "list", "--json"})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("listing routes failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	var services map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Stdout), &services); err != nil {
		return nil, fmt.Errorf("decoding routes: %w", err)
	}

	names := slices.Sorted(maps.Keys(services))
	if names == nil {
		names = []string{}
	}
	return names, nil
}

// pullImage pulls the proxy image unless it's already present. The image is
// pinned by digest, so a local copy is always the right one.
func (p *Proxy) pullImage(ctx context.Context) error {
	if _, err := p.namespace.client.ImageInspect(ctx, p.image); err == nil {
		return nil
	}

	resp, err := p.namespace.client.ImagePull(ctx, p.image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}

	if err := resp.Wait(ctx); err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}
	return nil
}

func (p *Proxy) createContainer(ctx context.Context, name, image string, settings ProxySettings) (string, error) {
	httpPortTCP := network.MustParsePort("80/tcp")
	httpsPortTCP := network.MustParsePort("443/tcp")
	metricsPortTCP := network.MustParsePort(fmt.Sprintf("%d/tcp", settings.MetricsPort))

	resp, err := p.namespace.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: name,
		Config: &container.Config{
			Image: image,
			Cmd:   []string{"kamal-proxy", "run", "--metrics-port", fmt.Sprintf("%d", settings.MetricsPort)},
			Labels: map[string]string{
				labelKey: settings.Marshal(),
			},
			ExposedPorts: network.PortSet{
				httpPortTCP:    struct{}{},
				httpsPortTCP:   struct{}{},
				metricsPortTCP: struct{}{},
			},
		},
		HostConfig: &container.HostConfig{
			PortBindings: network.PortMap{
				httpPortTCP:    []network.PortBinding{{HostPort: fmt.Sprintf("%d", settings.HTTPPort)}},
				httpsPortTCP:   []network.PortBinding{{HostPort: fmt.Sprintf("%d", settings.HTTPSPort)}},
				metricsPortTCP: []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: fmt.Sprintf("%d", settings.MetricsPort)}},
			},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
			LogConfig:     ContainerLogConfig(),
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: p.containerName(),
					Target: stateFileDir,
				},
			},
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				p.namespace.name: {},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating proxy container: %w", err)
	}
	return resp.ID, nil
}

func (p *Proxy) startContainer(ctx context.Context, id string) error {
	if _, err := p.namespace.client.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
		if isPortConflict(err) {
			slog.Error("Port conflict starting proxy", "error", err)
			return ErrProxyPortInUse
		}
		return fmt.Errorf("starting proxy container: %w", err)
	}
	return nil
}

func (p *Proxy) stopContainer(ctx context.Context, id string) error {
	if _, err := p.namespace.client.ContainerStop(ctx, id, client.ContainerStopOptions{}); err != nil {
		return fmt.Errorf("stopping proxy container: %w", err)
	}
	return nil
}

func (p *Proxy) ensureStarted(ctx context.Context, info container.InspectResponse) error {
	if info.State.Running {
		return nil
	}
	return p.startContainer(ctx, info.ID)
}

func (p *Proxy) ensureRunning(ctx context.Context, info container.InspectResponse) error {
	if err := p.ensureStarted(ctx, info); err != nil {
		return err
	}

	label := info.Config.Labels[labelKey]
	if label != "" {
		settings, err := UnmarshalProxySettings(label)
		if err != nil {
			return fmt.Errorf("unmarshalling proxy settings: %w", err)
		}
		p.Settings = &settings
	}

	return nil
}

func (p *Proxy) deployArgs(opts DeployOptions) []string {
	args := []string{"kamal-proxy", "deploy", opts.AppName, "--target", opts.Target, "--deploy-timeout", deployTimeout}

	if opts.Host != "" {
		args = append(args, "--host", opts.Host)
	}

	if opts.TLS {
		args = append(args, "--tls")
	}

	return args
}

func (p *Proxy) LoadState(ctx context.Context) (*State, error) {
	containerName := p.containerName()

	res, err := p.namespace.client.CopyFromContainer(ctx, containerName, client.CopyFromContainerOptions{SourcePath: stateFilePath})
	if err != nil {
		// Return empty state when the file doesn't exist yet (first boot)
		if errdefs.IsNotFound(err) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("copying state from container: %w", err)
	}
	defer res.Content.Close()

	tr := tar.NewReader(res.Content)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("reading state tar: %w", err)
	}

	var state State
	if err := json.NewDecoder(tr).Decode(&state); err != nil {
		return nil, fmt.Errorf("decoding state: %w", err)
	}

	return &state, nil
}

func (p *Proxy) SaveState(ctx context.Context, state *State) error {
	containerName := p.containerName()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	header := &tar.Header{
		Name: stateFileName,
		Mode: 0o644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("writing tar data: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar writer: %w", err)
	}

	if _, err := p.namespace.client.CopyToContainer(ctx, containerName, client.CopyToContainerOptions{DestinationPath: stateFileDir, Content: &buf}); err != nil {
		return fmt.Errorf("copying state to container: %w", err)
	}

	return nil
}

func (p *Proxy) ExecOutput(ctx context.Context, cmd []string) (string, error) {
	result, err := execInContainer(ctx, p.namespace.client, p.containerName(), cmd)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout + result.Stderr, fmt.Errorf("exec failed with exit code %d", result.ExitCode)
	}
	return result.Stdout, nil
}

// Helpers

func withDefaultPorts(settings ProxySettings) ProxySettings {
	settings.HTTPPort = cmp.Or(settings.HTTPPort, DefaultHTTPPort)
	settings.HTTPSPort = cmp.Or(settings.HTTPSPort, DefaultHTTPSPort)
	settings.MetricsPort = cmp.Or(settings.MetricsPort, DefaultMetricsPort)
	return settings
}

func settingsFromConfig(config *container.Config) (ProxySettings, error) {
	var settings ProxySettings
	if config.Labels[labelKey] != "" {
		var err error
		if settings, err = UnmarshalProxySettings(config.Labels[labelKey]); err != nil {
			return ProxySettings{}, fmt.Errorf("unmarshalling proxy settings: %w", err)
		}
	}
	return withDefaultPorts(settings), nil
}
