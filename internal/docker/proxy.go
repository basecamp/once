package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	ProxyImage = "basecamp/kamal-proxy:once-01"
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
	AppName       string
	Target        string
	Hosts         []string
	CanonicalHost string
	TLS           bool
}

type Proxy struct {
	namespace *Namespace
	Settings  *ProxySettings
}

func NewProxy(ns *Namespace) *Proxy {
	return &Proxy{namespace: ns}
}

func (p *Proxy) Boot(ctx context.Context, settings ProxySettings) error {
	if settings.HTTPPort == 0 {
		settings.HTTPPort = DefaultHTTPPort
	}
	if settings.HTTPSPort == 0 {
		settings.HTTPSPort = DefaultHTTPSPort
	}
	if settings.MetricsPort == 0 {
		settings.MetricsPort = DefaultMetricsPort
	}

	info, err := p.namespace.client.ContainerInspect(ctx, p.containerName(), client.ContainerInspectOptions{})
	if err == nil {
		return p.ensureRunning(ctx, info.Container)
	}
	if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting proxy container: %w", err)
	}

	reader, err := p.namespace.client.ImagePull(ctx, ProxyImage, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling proxy image: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	name := p.containerName()
	httpPortTCP := network.MustParsePort("80/tcp")
	httpsPortTCP := network.MustParsePort("443/tcp")
	metricsPortTCP := network.MustParsePort(fmt.Sprintf("%d/tcp", settings.MetricsPort))

	resp, err := p.namespace.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: name,
		Config: &container.Config{
			Image: ProxyImage,
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
					Target: "/home/kamal-proxy/.config/kamal-proxy",
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
		return fmt.Errorf("creating proxy container: %w", err)
	}

	if _, err := p.namespace.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		if isPortConflict(err) {
			slog.Error("Port conflict starting proxy", "error", err)
			return ErrProxyPortInUse
		}
		return fmt.Errorf("starting proxy container: %w", err)
	}

	p.Settings = &settings
	return nil
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

func (p *Proxy) ensureRunning(ctx context.Context, info container.InspectResponse) error {
	if !info.State.Running {
		if _, err := p.namespace.client.ContainerStart(ctx, info.ID, client.ContainerStartOptions{}); err != nil {
			if isPortConflict(err) {
				slog.Error("Port conflict starting proxy", "error", err)
				return ErrProxyPortInUse
			}
			return fmt.Errorf("starting proxy container: %w", err)
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

	for _, host := range opts.Hosts {
		args = append(args, "--host", host)
	}

	if opts.CanonicalHost != "" {
		args = append(args, "--canonical-host", opts.CanonicalHost)
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
