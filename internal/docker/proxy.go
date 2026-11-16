package docker

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
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
)

const (
	DefaultHTTPPort    = 80
	DefaultHTTPSPort   = 443
	DefaultMetricsPort = 1318
	deployTimeout      = "120s"
	proxyReadyTimeout  = 30 * time.Second
)

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

	id, err := p.createContainer(ctx, p.image, settings)
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
// other than the one this version of Once pins. It does nothing when there is
// no proxy container.
func (p *Proxy) Upgrade(ctx context.Context) error {
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
	containerName := p.containerName()

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
	return p.Exec(ctx, []string{"kamal-proxy", "remove", appName})
}

func (p *Proxy) Deploy(ctx context.Context, opts DeployOptions) error {
	return p.Exec(ctx, p.deployArgs(opts))
}

func (p *Proxy) containerName() string {
	return p.namespace.name + "-proxy"
}

// Private

func (p *Proxy) isCurrent(info container.InspectResponse) bool {
	return info.Config.Image == p.image
}

// replace swaps the existing proxy container for one created from the pinned
// image. kamal-proxy's routes and certificates, and Once's own state file, live
// on the proxy volume, which the new container mounts again, so kamal-proxy
// restores them when it starts. If the new container can't be started, the old
// one is recreated from its image so the apps stay reachable.
func (p *Proxy) replace(ctx context.Context, old container.InspectResponse) error {
	settings, err := settingsFromConfig(old.Config)
	if err != nil {
		return err
	}

	slog.Info("Replacing proxy container", "from", old.Config.Image, "to", p.image)

	if err := p.pullImage(ctx); err != nil {
		return err
	}

	if err := p.removeContainer(ctx, old.ID); err != nil {
		return err
	}

	id, err := p.createContainer(ctx, p.image, settings)
	if err == nil {
		err = p.startContainer(ctx, id)
	}
	if err == nil {
		err = p.waitUntilReady(ctx)
	}
	if err != nil {
		slog.Error("Failed to start new proxy container; restoring the previous one", "error", err)
		if restoreErr := p.restore(ctx, id, old.Image, settings); restoreErr != nil {
			slog.Error("Failed to restore previous proxy container", "error", restoreErr)
		}
		return err
	}

	p.Settings = &settings
	return nil
}

func (p *Proxy) restore(ctx context.Context, failedID, image string, settings ProxySettings) error {
	if failedID != "" {
		if _, err := p.namespace.client.ContainerRemove(ctx, failedID, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing failed proxy container: %w", err)
		}
	}

	id, err := p.createContainer(ctx, image, settings)
	if err != nil {
		return err
	}
	return p.startContainer(ctx, id)
}

func (p *Proxy) pullImage(ctx context.Context) error {
	reader, err := p.namespace.client.ImagePull(ctx, p.image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}
	defer reader.Close()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}
	return nil
}

func (p *Proxy) createContainer(ctx context.Context, image string, settings ProxySettings) (string, error) {
	name := p.containerName()
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
					Source: name,
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

// waitUntilReady waits for kamal-proxy to answer on its control socket, which
// it opens only once it has restored its saved state. A container can start
// and then exit straight away, so a successful start alone doesn't show that.
func (p *Proxy) waitUntilReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, proxyReadyTimeout)
	defer cancel()

	for {
		_, err := p.ExecOutput(ctx, []string{"kamal-proxy", "list"})
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for proxy to start: %w", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (p *Proxy) removeContainer(ctx context.Context, id string) error {
	if _, err := p.namespace.client.ContainerStop(ctx, id, client.ContainerStopOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("stopping proxy container: %w", err)
	}
	if _, err := p.namespace.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("removing proxy container: %w", err)
	}
	return nil
}

func (p *Proxy) ensureRunning(ctx context.Context, info container.InspectResponse) error {
	if !info.State.Running {
		if err := p.startContainer(ctx, info.ID); err != nil {
			return err
		}
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
