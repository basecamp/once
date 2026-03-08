package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const DefaultNamespace = "once"

var ErrInvalidNamespace = errors.New("invalid namespace: must contain only lowercase letters, digits, and hyphens, and must not start with a hyphen")

type Namespace struct {
	name         string
	client       *client.Client
	proxy        *Proxy
	applications []*Application
}

func NewNamespace(name string) (*Namespace, error) {
	if name == "" {
		name = DefaultNamespace
	}

	if !validNamespace.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidNamespace, name)
	}

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	ns := &Namespace{
		name:   name,
		client: c,
	}
	ns.proxy = NewProxy(ns)
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

func (n *Namespace) AddApplication(settings ApplicationSettings) *Application {
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

func (n *Namespace) RemoveApplication(app *Application) {
	n.applications = slices.DeleteFunc(n.applications, func(a *Application) bool {
		return a == app
	})
}

func (n *Namespace) HostInUse(host string) bool {
	for _, app := range n.applications {
		if app.Settings.Host == host {
			return true
		}
	}
	return false
}

func (n *Namespace) HostInUseByAnother(host string, excludeApp string) bool {
	for _, app := range n.applications {
		if app.Settings.Host == host && app.Settings.Name != excludeApp {
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

	if n.proxy.Settings == nil {
		if err := n.proxy.Boot(ctx, ProxySettings{}); err != nil {
			return err
		}
	}

	return nil
}

func (n *Namespace) EnsureNetwork(ctx context.Context) error {
	networks, err := n.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return err
	}

	for _, net := range networks {
		if net.Name == n.name {
			return nil
		}
	}

	_, err = n.client.NetworkCreate(ctx, n.name, network.CreateOptions{
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

	if err := n.proxy.Destroy(ctx, destroyVolumes); err != nil {
		return err
	}

	return n.client.NetworkRemove(ctx, n.name)
}

func (n *Namespace) Refresh(ctx context.Context) error {
	n.applications = nil
	return n.restoreState(ctx)
}

func (n *Namespace) DockerRootDir(ctx context.Context) (string, error) {
	info, err := n.client.Info(ctx)
	if err != nil {
		return "", err
	}
	return info.DockerRootDir, nil
}

func (n *Namespace) EventWatcher() *EventWatcher {
	return NewEventWatcher(n.client, n.name)
}

func (n *Namespace) ApplicationExists(ctx context.Context, name string) (bool, error) {
	containers, err := n.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, c := range containers {
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

func (n *Namespace) Restore(ctx context.Context, r io.Reader) (*Application, error) {
	appSettings, volSettings, volumeData, err := n.parseBackup(r)
	if err != nil {
		return nil, fmt.Errorf("parsing backup: %w", err)
	}

	if n.HostInUse(appSettings.Host) {
		return nil, ErrHostnameInUse
	}

	name, err := n.UniqueName(NameFromImageRef(appSettings.Image))
	if err != nil {
		return nil, fmt.Errorf("generating app name: %w", err)
	}
	appSettings.Name = name

	app := n.AddApplication(appSettings)
	if err := app.Restore(ctx, volSettings, volumeData); err != nil {
		n.RemoveApplication(app)
		return nil, err
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
	containers, err := n.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	proxyPrefix := n.name + "-proxy"
	appPrefix := n.name + "-app-"

	// Use a map to deduplicate apps by name, preferring the most recently created container
	appsByName := make(map[string]appCandidate)

	for _, c := range containers {
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
						info, err := n.client.ContainerInspect(ctx, c.ID)
						if err == nil && info.State != nil {
							if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
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
		n.applications = append(n.applications, candidate.app)
	}

	n.sortApplications()
	return nil
}

func (n *Namespace) sortApplications() {
	slices.SortFunc(n.applications, func(a, b *Application) int {
		return strings.Compare(a.Settings.Host, b.Settings.Host)
	})
}

func (n *Namespace) parseBackup(r io.Reader) (ApplicationSettings, ApplicationVolumeSettings, []byte, error) {
	var appSettings ApplicationSettings
	var volSettings ApplicationVolumeSettings
	var volumeData bytes.Buffer

	gr, err := gzip.NewReader(r)
	if err != nil {
		return appSettings, volSettings, nil, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	tw := tar.NewWriter(&volumeData)
	defer tw.Close()

	foundApp := false
	foundVol := false

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return appSettings, volSettings, nil, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
		}

		switch header.Name {
		case backupAppSettingsEntry:
			data, err := io.ReadAll(tr)
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: reading application settings: %v", ErrInvalidBackup, err)
			}
			appSettings, err = UnmarshalApplicationSettings(string(data))
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: parsing application settings: %v", ErrInvalidBackup, err)
			}
			foundApp = true

		case backupVolSettingsEntry:
			data, err := io.ReadAll(tr)
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: reading volume settings: %v", ErrInvalidBackup, err)
			}
			volSettings, err = UnmarshalApplicationVolumeSettings(string(data))
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: parsing volume settings: %v", ErrInvalidBackup, err)
			}
			foundVol = true

		default:
			if header.Name == BackupDataDir || strings.HasPrefix(header.Name, BackupDataDir+"/") {
				newHeader := *header
				if header.Name == BackupDataDir {
					newHeader.Name = "data"
				} else {
					newHeader.Name = "data" + strings.TrimPrefix(header.Name, BackupDataDir)
				}
				if err := tw.WriteHeader(&newHeader); err != nil {
					return appSettings, volSettings, nil, err
				}
				if header.Size > 0 {
					if _, err := io.Copy(tw, tr); err != nil {
						return appSettings, volSettings, nil, err
					}
				}
			}
		}
	}

	if !foundApp || !foundVol {
		return appSettings, volSettings, nil, fmt.Errorf("%w: missing required metadata files", ErrInvalidBackup)
	}

	return appSettings, volSettings, volumeData.Bytes(), nil
}

// Helpers

var validNamespace = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// cutLast splits s around the last occurrence of sep, like strings.Cut but
// from the right.
func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}
