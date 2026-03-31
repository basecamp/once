package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

var (
	ErrApplicationExists  = errors.New("application already exists")
	ErrHostnameInUse      = errors.New("hostname already in use")
	ErrHostRequired       = errors.New("host is required")
	ErrInvalidBackup      = errors.New("invalid backup archive")
	ErrBackupPathRelative    = errors.New("backup path must be absolute")
	ErrAutoBackupWithoutPath = errors.New("auto-backup requires a backup path")
	ErrSetupFailed        = errors.New("setup failed")
	ErrPullFailed         = &describedError{
		msg:         "pull failed",
		description: "Failed to download the application image. Check that the image name is correct and try again.",
	}
	ErrDeployFailed       = errors.New("deploy failed")
	ErrVerificationFailed = &describedError{
		msg:         "verification failed",
		description: "The application couldn't be verified. Please check that you have a valid DNS record set up.",
	}
	ErrUnpauseFailed = errors.New("failed to unpause container after backup")
)

const (
	AutomaticTaskInterval = 24 * time.Hour
	HealthCheckPath       = "/up"
	httpVerifyTimeout     = 30 * time.Second
)

// AppVolumeMountTargets defines the paths where the app data volume is mounted
// inside the container. The first entry is the primary path used for backups.
var AppVolumeMountTargets = []string{"/storage", "/rails/storage"}

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
	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if a.namespace.containerAppName(name) == a.Settings.Name {
			return name, nil
		}
	}

	return "", fmt.Errorf("no container found for app %s", a.Settings.Name)
}

func (a *Application) Volume(ctx context.Context) (*ApplicationVolume, error) {
	vol, err := FindVolume(ctx, a.namespace, a.Settings.Name)
	if err == nil {
		return vol, nil
	}
	if !errors.Is(err, ErrVolumeNotFound) {
		return nil, err
	}

	skb, err := generateSecretKeyBase()
	if err != nil {
		return nil, fmt.Errorf("generating secret key base: %w", err)
	}
	vapidPub, vapidPriv, err := generateVAPIDKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generating VAPID key pair: %w", err)
	}
	return CreateVolume(ctx, a.namespace, a.Settings.Name, ApplicationVolumeSettings{
		SecretKeyBase:   skb,
		VAPIDPublicKey:  vapidPub,
		VAPIDPrivateKey: vapidPriv,
	})
}

func (a *Application) URL() string {
	if a.Settings.Host == "" {
		return ""
	}

	scheme := "http"
	defaultPort := 80
	if a.Settings.TLSEnabled() {
		scheme = "https"
		defaultPort = 443
	}

	base := scheme + "://" + a.Settings.Host

	if a.namespace == nil {
		return base
	}

	proxy := a.namespace.Proxy()
	if proxy.Settings == nil {
		return base
	}

	port := proxy.Settings.HTTPPort
	if a.Settings.TLSEnabled() {
		port = proxy.Settings.HTTPSPort
	}

	if port != 0 && port != defaultPort {
		return base + ":" + strconv.Itoa(port)
	}
	return base
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

func (a *Application) Update(ctx context.Context, progress DeployProgressCallback) (bool, error) {
	changed, err := a.pullImage(ctx, progress)
	if err != nil {
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
		return false, err
	}

	if !changed {
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, nil) })
		return false, nil
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		err = fmt.Errorf("getting volume: %w", err)
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
		return false, err
	}

	err = a.deployWithVolume(ctx, vol, progress)
	a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
	return true, err
}

func (a *Application) Deploy(ctx context.Context, progress DeployProgressCallback) error {
	if a.Settings.Host == "" {
		return ErrHostRequired
	}

	if _, err := a.pullImage(ctx, progress); err != nil {
		return err
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	return a.deployWithVolume(ctx, vol, progress)
}

func (a *Application) VerifyHTTPOrRemove(ctx context.Context) error {
	if err := a.verifyHTTP(ctx); err != nil {
		if cleanupErr := a.Remove(context.Background(), true); cleanupErr != nil {
			slog.Error("Failed to clean up after verification failure", "app", a.Settings.Name, "error", cleanupErr)
		}
		return err
	}

	return nil
}

func (a *Application) Remove(ctx context.Context, removeData bool) error {
	if err := a.namespace.Proxy().Remove(ctx, a.Settings.Name); err != nil {
		return fmt.Errorf("removing from proxy: %w", err)
	}

	return a.Destroy(ctx, removeData)
}

func (a *Application) Destroy(ctx context.Context, destroyVolumes bool) error {
	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if a.namespace.containerAppName(name) == a.Settings.Name {
				if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
					return fmt.Errorf("removing container: %w", err)
				}
				break
			}
		}
	}

	if destroyVolumes {
		vol, err := FindVolume(ctx, a.namespace, a.Settings.Name)
		if err != nil && !errors.Is(err, ErrVolumeNotFound) {
			return fmt.Errorf("getting volume: %w", err)
		}
		if vol != nil {
			if err := vol.Destroy(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

// Private

func (a *Application) saveOperationResult(ctx context.Context, record func(*State)) {
	state, err := a.namespace.LoadState(ctx)
	if err != nil {
		return
	}
	record(state)
	a.namespace.SaveState(ctx, state)
}

func (a *Application) pullImage(ctx context.Context, progress DeployProgressCallback) (bool, error) {
	opts := image.PullOptions{RegistryAuth: registryAuthFor(a.Settings.Image)}
	reader, err := a.namespace.client.ImagePull(ctx, a.Settings.Image, opts)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrPullFailed, err)
	}
	defer reader.Close()

	if progress != nil {
		tracker := newPullProgressTracker(progress)
		if err := tracker.Track(reader); err != nil {
			return false, fmt.Errorf("%w: %w", ErrPullFailed, err)
		}
	} else {
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return false, fmt.Errorf("%w: %w", ErrPullFailed, err)
		}
	}

	pulledInspect, err := a.namespace.client.ImageInspect(ctx, a.Settings.Image)
	if err != nil {
		return false, fmt.Errorf("%w: inspecting image after pull: %w", ErrPullFailed, err)
	}

	return pulledInspect.ID != a.runningImageID(ctx), nil
}

func (a *Application) runningImageID(ctx context.Context) string {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return ""
	}
	info, err := a.namespace.client.ContainerInspect(ctx, name)
	if err != nil {
		return ""
	}
	return info.Image
}

func (a *Application) deployWithVolume(ctx context.Context, vol *ApplicationVolume, progress DeployProgressCallback) error {
	if progress != nil {
		progress(DeployProgress{Stage: DeployStageStarting})
	}

	id, err := ContainerRandomID()
	if err != nil {
		return fmt.Errorf("generating container id: %w", err)
	}

	containerName := fmt.Sprintf("%s-app-%s-%s", a.namespace.name, a.Settings.Name, id)

	env := a.Settings.BuildEnv(vol.Settings)

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
		LogConfig:     ContainerLogConfig(),
		Mounts:        a.volumeMounts(vol),
	}
	hostConfig.Resources = container.Resources{
		Memory:   int64(a.Settings.Resources.MemoryMB) * 1024 * 1024,
		NanoCPUs: int64(a.Settings.Resources.CPUs) * 1e9,
	}

	resp, err := a.namespace.client.ContainerCreate(ctx,
		a.containerConfig(env),
		hostConfig,
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
		a.namespace.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
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
		if strings.Contains(err.Error(), "target not healthy") || strings.Contains(err.Error(), "deploy timed out") {
			slog.Error("Application failed to start", "app", a.Settings.Name, "error", err)
			return ErrAppNotStarted
		}
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

func (a *Application) verifyHTTP(ctx context.Context) error {
	url := a.URL()
	if url == "" {
		return nil
	}

	client := &http.Client{Timeout: httpVerifyTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+HealthCheckPath, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: unexpected status %d from %s", ErrVerificationFailed, resp.StatusCode, url)
	}

	return nil
}

func (a *Application) removeContainersExcept(ctx context.Context, keep string) error {
	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if a.namespace.containerAppName(name) == a.Settings.Name && name != keep {
			if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Application) volumeMounts(vol *ApplicationVolume) []mount.Mount {
	var mounts []mount.Mount
	for _, target := range AppVolumeMountTargets {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: vol.Name(),
			Target: target,
		})
	}
	return mounts
}

func (a *Application) containerConfig(env []string) *container.Config {
	return &container.Config{
		Image: a.Settings.Image,
		Labels: map[string]string{
			labelKey: a.Settings.Marshal(),
		},
		Env: env,
	}
}
