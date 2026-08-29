package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

const DefaultNamespace = "once"

var ErrInvalidNamespace = errors.New("invalid namespace: must contain only lowercase letters, digits, and hyphens, and must not start with a hyphen")

type Namespace struct {
	name         string
	client       *client.Client
	proxy        *Proxy
	applications []*Application
}

type NamespaceOption func(*Namespace)

func WithApplications(apps ...ApplicationSettings) NamespaceOption {
	return func(ns *Namespace) {
		for _, s := range apps {
			ns.addApplication(s)
		}
	}
}

func NewNamespace(name string, opts ...NamespaceOption) (*Namespace, error) {
	if name == "" {
		name = DefaultNamespace
	}

	if !validNamespace.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidNamespace, name)
	}

	c, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}

	ns := &Namespace{
		name:   name,
		client: c,
	}
	ns.proxy = NewProxy(ns)

	for _, opt := range opts {
		opt(ns)
	}

	return ns, nil
}

func RestoreNamespace(ctx context.Context, name string) (*Namespace, error) {
	ns, err := NewNamespace(name)
	if err != nil {
		return nil, err
	}

	if err := ns.restoreState(ctx); err != nil {
		return nil, err
	}

	return ns, nil
}

func (n *Namespace) Name() string {
	return n.name
}

func (n *Namespace) addApplication(settings ApplicationSettings) *Application {
	app := NewApplication(n, settings)
	n.applications = append(n.applications, app)
	n.sortApplications()
	return app
}

func (n *Namespace) Proxy() *Proxy {
	return n.proxy
}

func (n *Namespace) Application(name string) *Application {
	for _, app := range n.applications {
		if app.Settings.Name == name {
			return app
		}
	}
	return nil
}

func (n *Namespace) Applications() []*Application {
	return n.applications
}

func (n *Namespace) ApplicationByHost(host string) *Application {
	for _, app := range n.applications {
		if slices.Contains(app.Settings.Hosts(), host) {
			return app
		}
	}
	return nil
}

func (n *Namespace) HostInUse(host string) bool {
	return n.ApplicationByHost(host) != nil
}

func (n *Namespace) HostInUseByAnother(host string, excludeApp string) bool {
	for _, app := range n.applications {
		if slices.Contains(app.Settings.Hosts(), host) && app.Settings.Name != excludeApp {
			return true
		}
	}
	return false
}

func (n *Namespace) UniqueName(base string) (string, error) {
	for {
		id, err := randomID(6)
		if err != nil {
			return "", err
		}
		candidate := fmt.Sprintf("%s.%s", base, id)
		if n.Application(candidate) == nil {
			return candidate, nil
		}
	}
}

func (n *Namespace) Setup(ctx context.Context) error {
	if err := n.EnsureNetwork(ctx); err != nil {
		return err
	}

	return n.proxy.Boot(ctx, ProxySettings{})
}

func (n *Namespace) EnsureNetwork(ctx context.Context) error {
	networks, err := n.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return err
	}

	for _, net := range networks.Items {
		if net.Name == n.name {
			return nil
		}
	}

	_, err = n.client.NetworkCreate(ctx, n.name, client.NetworkCreateOptions{
		Driver: "bridge",
	})
	return err
}

func (n *Namespace) Teardown(ctx context.Context, destroyVolumes bool) error {
	for _, app := range n.applications {
		if err := app.Destroy(ctx, destroyVolumes); err != nil {
			return err
		}
	}

	if err := n.proxy.Destroy(ctx); err != nil {
		return err
	}

	if _, err := n.client.NetworkRemove(ctx, n.name, client.NetworkRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (n *Namespace) Refresh(ctx context.Context) error {
	n.applications = nil
	return n.restoreState(ctx)
}

func (n *Namespace) DockerRootDir(ctx context.Context) (string, error) {
	info, err := n.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", err
	}
	return info.Info.DockerRootDir, nil
}

func (n *Namespace) EventWatcher() *EventWatcher {
	return NewEventWatcher(n.client, n.name)
}

func (n *Namespace) ApplicationExists(ctx context.Context, name string) (bool, error) {
	containers, err := n.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, c := range containers.Items {
		for _, cname := range c.Names {
			cname = strings.TrimPrefix(cname, "/")
			if n.containerAppName(cname) == name {
				return true, nil
			}
		}
	}

	return false, nil
}

func (n *Namespace) LoadState(ctx context.Context) (*State, error) {
	return n.proxy.LoadState(ctx)
}

func (n *Namespace) SaveState(ctx context.Context, state *State) error {
	return n.proxy.SaveState(ctx, state)
}

func (n *Namespace) Restore(ctx context.Context, r io.ReadSeeker) (*Application, error) {
	appSettings, volSettings, err := readBackupSettings(r)
	if err != nil {
		return nil, fmt.Errorf("parsing backup: %w", err)
	}

	for _, host := range appSettings.Hosts() {
		if n.HostInUse(host) {
			return nil, ErrHostnameInUse
		}
	}

	name, err := n.UniqueName(NameFromImageRef(appSettings.Image))
	if err != nil {
		return nil, fmt.Errorf("generating app name: %w", err)
	}
	appSettings.Name = name

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewinding backup: %w", err)
	}

	app := NewApplication(n, appSettings)
	if err := app.Restore(ctx, volSettings, r); err != nil {
		if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
			slog.Error("Failed to clean up after restore failure", "app", appSettings.Name, "error", cleanupErr)
		}
		return nil, err
	}

	if err := n.Refresh(ctx); err != nil {
		slog.Error("Failed to refresh namespace after restore", "app", appSettings.Name, "error", err)
	}

	if restored := n.Application(appSettings.Name); restored != nil {
		return restored, nil
	}
	return app, nil
}

// containerAppName extracts the application name from a container name
// matching the pattern {namespace}-app-{appName}-{id}. Returns "" if the
// container name doesn't match.
func (n *Namespace) containerAppName(containerName string) string {
	after, ok := strings.CutPrefix(containerName, n.name+"-app-")
	if !ok {
		return ""
	}
	appName, _, ok := cutLast(after, "-")
	if !ok {
		return ""
	}
	return appName
}

// Private

type appCandidate struct {
	app     *Application
	created int64
}

func (n *Namespace) restoreState(ctx context.Context) error {
	containers, err := n.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return connectionError(err)
	}

	proxyPrefix := n.name + "-proxy"
	appPrefix := n.name + "-app-"

	// Use a map to deduplicate apps by name, preferring the most recently created container
	appsByName := make(map[string]appCandidate)

	for _, c := range containers.Items {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")

			if name == proxyPrefix {
				label := c.Labels[labelKey]
				if label != "" {
					settings, err := UnmarshalProxySettings(label)
					if err != nil {
						return err
					}
					n.proxy.Settings = &settings
				}
				break
			}

			if strings.HasPrefix(name, appPrefix) {
				label := c.Labels[labelKey]
				if label != "" {
					settings, err := UnmarshalApplicationSettings(label)
					if err != nil {
						return err
					}
					app := NewApplication(n, settings)
					app.Running = c.State == "running"
					if app.Running {
						info, err := n.client.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
						if err == nil && info.Container.State != nil {
							if t, err := time.Parse(time.RFC3339Nano, info.Container.State.StartedAt); err == nil {
								app.RunningSince = t
							}
						}
					}

					existing, found := appsByName[settings.Name]
					if !found || c.Created > existing.created {
						appsByName[settings.Name] = appCandidate{app: app, created: c.Created}
					}
				}
				break
			}
		}
	}

	for _, candidate := range appsByName {
		app := candidate.app

		// Older installs may still have keys stored on the volume label
		if app.Settings.Keys.Empty() {
			if vol, err := FindVolume(ctx, n, app.Settings.Name); err == nil {
				app.Settings.Keys = vol.Settings.Keys
			}
		}

		n.applications = append(n.applications, app)
	}

	n.sortApplications()
	return nil
}

func (n *Namespace) sortApplications() {
	slices.SortFunc(n.applications, func(a, b *Application) int {
		return strings.Compare(a.Settings.Host, b.Settings.Host)
	})
}

// Helpers

var validNamespace = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
