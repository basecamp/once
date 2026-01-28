package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

type ApplicationSettings struct {
	Name       string            `json:"n"`
	Image      string            `json:"i"`
	Host       string            `json:"h"`
	DisableTLS bool              `json:"dt"`
	EnvVars    map[string]string `json:"env"`
}

func UnmarshalApplicationSettings(s string) (ApplicationSettings, error) {
	var settings ApplicationSettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ApplicationSettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.Host)
}

func (s ApplicationSettings) URL() string {
	if s.Host == "" {
		return ""
	}
	if s.TLSEnabled() {
		return "https://" + s.Host
	}
	return "http://" + s.Host
}

func (s ApplicationSettings) BuildEnv(secretKeyBase string) []string {
	env := make([]string, 0, len(s.EnvVars)+2)
	env = append(env, "SECRET_KEY_BASE="+secretKeyBase)

	if !s.TLSEnabled() {
		env = append(env, "DISABLE_SSL=true")
	}

	for k, v := range s.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}

type Application struct {
	namespace    *Namespace
	Settings     ApplicationSettings
	Running      bool
	RunningSince time.Time
}

func NewApplication(ns *Namespace, settings ApplicationSettings) *Application {
	return &Application{
		namespace: ns,
		Settings:  settings,
	}
}

func (a *Application) ContainerName(ctx context.Context) (string, error) {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.HasPrefix(name, prefix) {
			return name, nil
		}
	}

	return "", fmt.Errorf("no container found for app %s", a.Settings.Name)
}

func (a *Application) Volume(ctx context.Context) (*ApplicationVolume, error) {
	return FindOrCreateVolume(ctx, a.namespace, a.Settings.Name)
}

func (a *Application) Stop(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	return a.namespace.client.ContainerStop(ctx, name, container.StopOptions{})
}

func (a *Application) Start(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	return a.namespace.client.ContainerStart(ctx, name, container.StartOptions{})
}

func (a *Application) Update(ctx context.Context, progress DeployProgressCallback) error {
	return a.Deploy(ctx, progress)
}

func (a *Application) Deploy(ctx context.Context, progress DeployProgressCallback) error {
	reader, err := a.namespace.client.ImagePull(ctx, a.Settings.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer reader.Close()

	if progress != nil {
		tracker := newPullProgressTracker(progress)
		if err := tracker.Track(reader); err != nil {
			return fmt.Errorf("tracking pull progress: %w", err)
		}
		progress(DeployProgress{Stage: DeployStageStarting})
	} else {
		_, _ = io.Copy(io.Discard, reader)
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	id, err := ContainerRandomID()
	if err != nil {
		return fmt.Errorf("generating container id: %w", err)
	}

	containerName := fmt.Sprintf("%s-app-%s-%s", a.namespace.name, a.Settings.Name, id)

	env := a.Settings.BuildEnv(vol.SecretKeyBase())

	resp, err := a.namespace.client.ContainerCreate(ctx,
		a.containerConfig(env),
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: vol.Name(),
					Target: "/rails/storage",
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				a.namespace.name: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	if err := a.namespace.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	shortContainerID := resp.ID[:12]

	if err := a.namespace.Proxy().Deploy(ctx, DeployOptions{
		AppName: a.Settings.Name,
		Target:  shortContainerID,
		Host:    a.Settings.Host,
		TLS:     a.Settings.TLSEnabled(),
	}); err != nil {
		a.namespace.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("registering with proxy: %w", err)
	}

	if err := a.removeContainersExcept(ctx, containerName); err != nil {
		return fmt.Errorf("removing old containers: %w", err)
	}

	if progress != nil {
		progress(DeployProgress{Stage: DeployStageFinished})
	}

	return nil
}

func (a *Application) Destroy(ctx context.Context, destroyVolumes bool) error {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if strings.HasPrefix(name, prefix) {
				if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
					return fmt.Errorf("removing container: %w", err)
				}
				break
			}
		}
	}

	if destroyVolumes {
		vol, err := a.Volume(ctx)
		if err != nil {
			return fmt.Errorf("getting volume: %w", err)
		}
		if err := vol.Destroy(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Private

func (a *Application) containerConfig(env []string) *container.Config {
	return &container.Config{
		Image: a.Settings.Image,
		Labels: map[string]string{
			"amar": a.Settings.Marshal(),
		},
		Env: env,
	}
}

func (a *Application) removeContainersExcept(ctx context.Context, keep string) error {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.HasPrefix(name, prefix) && name != keep {
			if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helpers

func IsLocalhost(host string) bool {
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func NameFromImageRef(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return imageRef
	}
	path := reference.Path(named)
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func ContainerRandomID() (string, error) {
	return randomID(6)
}

func randomID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}
