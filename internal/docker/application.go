package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
)

var (
	ErrApplicationExists  = errors.New("application already exists")
	ErrHostnameInUse      = errors.New("hostname already in use")
	ErrHostRequired       = errors.New("host is required")
	ErrInvalidBackup      = errors.New("invalid backup archive")
	ErrBackupPathRelative = errors.New("backup path must be absolute")
	ErrSetupFailed        = errors.New("setup failed")
	ErrPullFailed         = errors.New("pull failed")
	ErrDeployFailed       = errors.New("deploy failed")
	ErrVerificationFailed = errors.New("verification failed")
)

const (
	BackupDataDir         = "data"
	BackupRetention       = 30 * 24 * time.Hour
	AutomaticTaskInterval = 24 * time.Hour

	backupAppSettingsEntry = "once.application.json"
	backupVolSettingsEntry = "once.volume.json"
	backupTimeFormat       = "20060102-150405"
	httpVerifyTimeout      = 30 * time.Second
)

// AppVolumeMountTargets defines the paths where the app data volume is mounted
// inside the container. The first entry is the primary path used for backups.
var AppVolumeMountTargets = []string{"/storage", "/rails/storage"}

type SMTPSettings struct {
	Server   string `json:"s,omitempty"`
	Port     string `json:"p,omitempty"`
	Username string `json:"u,omitempty"`
	Password string `json:"pw,omitempty"`
	From     string `json:"f,omitempty"`
}

func (s SMTPSettings) BuildEnv() []string {
	if s.Server == "" {
		return nil
	}
	return []string{
		"SMTP_ADDRESS=" + s.Server,
		"SMTP_PORT=" + s.Port,
		"SMTP_USERNAME=" + s.Username,
		"SMTP_PASSWORD=" + s.Password,
		"MAILER_FROM_ADDRESS=" + s.From,
	}
}

type ContainerResources struct {
	CPUs     int `json:"cpus,omitempty"`
	MemoryMB int `json:"mem,omitempty"`
}

type BackupSettings struct {
	Path     string `json:"p,omitempty"`
	AutoBack bool   `json:"a,omitempty"`
}

type ApplicationSettings struct {
	Name       string             `json:"n"`
	Image      string             `json:"i"`
	Host       string             `json:"h"`
	DisableTLS bool               `json:"dt"`
	EnvVars    map[string]string  `json:"env"`
	SMTP       SMTPSettings       `json:"sm"`
	Resources  ContainerResources `json:"res"`
	AutoUpdate bool               `json:"au"`
	Backup     BackupSettings     `json:"bk"`
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

func (s ApplicationSettings) Equal(other ApplicationSettings) bool {
	if s.Name != other.Name || s.Image != other.Image || s.Host != other.Host || s.DisableTLS != other.DisableTLS {
		return false
	}
	if s.Resources != other.Resources {
		return false
	}
	if s.SMTP != other.SMTP {
		return false
	}
	if s.AutoUpdate != other.AutoUpdate {
		return false
	}
	if s.Backup != other.Backup {
		return false
	}
	if len(s.EnvVars) != len(other.EnvVars) {
		return false
	}
	for k, v := range s.EnvVars {
		if other.EnvVars[k] != v {
			return false
		}
	}
	return true
}

func (s ApplicationSettings) BuildEnv(secretKeyBase string) []string {
	env := []string{
		"SECRET_KEY_BASE=" + secretKeyBase,
	}

	if !s.TLSEnabled() {
		env = append(env, "DISABLE_SSL=true")
	}

	env = append(env, s.SMTP.BuildEnv()...)

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
	return CreateVolume(ctx, a.namespace, a.Settings.Name, ApplicationVolumeSettings{SecretKeyBase: skb})
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

func (a *Application) Restore(ctx context.Context, volSettings ApplicationVolumeSettings, volumeData []byte) error {
	if _, err := a.pullImage(ctx, nil); err != nil {
		return err
	}

	vol, err := CreateVolume(ctx, a.namespace, a.Settings.Name, volSettings)
	if err != nil {
		return fmt.Errorf("creating volume: %w", err)
	}

	if err := a.populateVolume(ctx, vol, volumeData); err != nil {
		vol.Destroy(ctx)
		return fmt.Errorf("populating volume: %w", err)
	}

	if err := a.deployWithVolume(ctx, vol, nil); err != nil {
		vol.Destroy(ctx)
		return err
	}

	return nil
}

func (a *Application) Backup(ctx context.Context) error {
	if a.Settings.Backup.Path == "" {
		return fmt.Errorf("backup location is required")
	}

	return a.BackupToFile(ctx, a.Settings.Backup.Path, a.BackupName())
}

func (a *Application) BackupName() string {
	return fmt.Sprintf("%s-%s.tar.gz", a.Settings.Name, time.Now().Format(backupTimeFormat))
}

func (a *Application) BackupToFile(ctx context.Context, dir string, name string) error {
	uid, gid, err := prepareBackupDir(dir)
	if err != nil {
		slog.Error("Failed to create backup directory", "directory", dir, "error", err)
		return err
	}

	filePath := filepath.Join(dir, name)
	file, err := os.Create(filePath)
	if err != nil {
		slog.Error("Failed to create backup file", "filename", filePath, "error", err)
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer file.Close()

	if err := os.Chown(filePath, uid, gid); err != nil {
		slog.Error("Failed to set backup file ownership", "filename", filePath, "error", err)
		return fmt.Errorf("setting backup file ownership: %w", err)
	}

	err = a.backupToWriter(ctx, file)
	a.saveOperationResult(ctx, func(s *State) { s.RecordBackup(a.Settings.Name, err) })

	if err != nil {
		os.Remove(filePath)
		slog.Error("Failed to generate backup", "filename", filePath, "error", err)
		return err
	}

	slog.Info("Created backup file", "filename", filePath)

	return nil
}

func (a *Application) VerifyHTTP(ctx context.Context) error {
	url := a.URL()
	if url == "" {
		return nil
	}

	client := &http.Client{Timeout: httpVerifyTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

func (a *Application) TrimBackups() error {
	if a.Settings.Backup.Path == "" {
		return nil
	}

	entries, err := os.ReadDir(a.Settings.Backup.Path)
	if err != nil {
		return fmt.Errorf("reading backup directory: %w", err)
	}

	var errs []error
	cutoff := time.Now().Add(-BackupRetention)

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		t, ok := parseBackupTime(a.Settings.Name, entry.Name())
		if !ok {
			continue
		}

		if t.Before(cutoff) {
			filename := filepath.Join(a.Settings.Backup.Path, entry.Name())
			if err := os.Remove(filename); err != nil {
				slog.Error("Failed to remove expired backup file", "filename", filename, "error", err)
				errs = append(errs, err)
			} else {
				slog.Info("Removed expired backup file", "filename", filename)
			}
		}
	}

	return errors.Join(errs...)
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

func (a *Application) backupToWriter(ctx context.Context, w io.Writer) error {
	containerName, err := a.ContainerName(ctx)
	if err != nil {
		return fmt.Errorf("getting container name: %w", err)
	}

	if err := a.runHookScript(ctx, containerName, "pre-backup"); err != nil {
		return fmt.Errorf("running pre-backup script: %w", err)
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := writeTarEntry(tw, backupAppSettingsEntry, []byte(a.Settings.Marshal())); err != nil {
		return fmt.Errorf("writing application settings: %w", err)
	}

	if err := writeTarEntry(tw, backupVolSettingsEntry, []byte(vol.Settings.Marshal())); err != nil {
		return fmt.Errorf("writing volume settings: %w", err)
	}

	reader, _, err := a.namespace.client.CopyFromContainer(ctx, containerName, AppVolumeMountTargets[0])
	if err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}
	defer reader.Close()

	if err := copyTarEntriesWithPrefix(reader, tw, filepath.Base(AppVolumeMountTargets[0]), BackupDataDir); err != nil {
		return fmt.Errorf("copying volume contents: %w", err)
	}

	return nil
}

func (a *Application) pullImage(ctx context.Context, progress DeployProgressCallback) (bool, error) {
	beforeID := a.currentImageID(ctx)

	reader, err := a.namespace.client.ImagePull(ctx, a.Settings.Image, image.PullOptions{})
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
		_, _ = io.Copy(io.Discard, reader)
	}

	afterInspect, err := a.namespace.client.ImageInspect(ctx, a.Settings.Image)
	if err != nil {
		return false, fmt.Errorf("%w: inspecting image after pull: %w", ErrPullFailed, err)
	}

	return afterInspect.ID != beforeID, nil
}

func (a *Application) currentImageID(ctx context.Context) string {
	inspect, err := a.namespace.client.ImageInspect(ctx, a.Settings.Image)
	if err != nil {
		return ""
	}
	return inspect.ID
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

	env := a.Settings.BuildEnv(vol.SecretKeyBase())

	var mounts []mount.Mount
	for _, target := range AppVolumeMountTargets {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: vol.Name(),
			Target: target,
		})
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
		LogConfig:     ContainerLogConfig(),
		Mounts:        mounts,
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

func (a *Application) populateVolume(ctx context.Context, vol *ApplicationVolume, data []byte) error {
	containerName := fmt.Sprintf("%s-restore-temp", a.namespace.name)

	resp, err := a.namespace.client.ContainerCreate(ctx,
		&container.Config{
			Image:      a.Settings.Image,
			Entrypoint: []string{},
			Cmd:        []string{"sleep", "infinity"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: vol.Name(),
					Target: "/data",
				},
			},
		},
		nil,
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("creating temp container: %w", err)
	}

	defer a.namespace.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := a.namespace.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting temp container: %w", err)
	}

	if len(data) > 0 {
		if err := a.namespace.client.CopyToContainer(ctx, resp.ID, "/", bytes.NewReader(data), container.CopyToContainerOptions{}); err != nil {
			return fmt.Errorf("copying data to volume: %w", err)
		}
	}

	return nil
}

func (a *Application) runHookScript(ctx context.Context, containerName, name string) error {
	cmd := []string{"/scripts/" + name}

	execResp, err := a.namespace.client.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("creating exec: %w", err)
	}

	resp, err := a.namespace.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("attaching exec: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return fmt.Errorf("reading exec output: %w", err)
	}

	inspect, err := a.namespace.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("inspecting exec: %w", err)
	}

	// Exit codes 126 (not executable) and 127 (not found) mean the script doesn't exist
	if inspect.ExitCode == 126 || inspect.ExitCode == 127 {
		return nil
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("hook script %q failed with exit code %d: %s", name, inspect.ExitCode, stderr.String())
	}

	return nil
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

func prepareBackupDir(dir string) (int, int, error) {
	if dir == "" {
		return 0, 0, fmt.Errorf("backup location is required")
	}

	if !filepath.IsAbs(dir) {
		return 0, 0, ErrBackupPathRelative
	}

	uid, gid, err := findOwnership(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("determining backup directory ownership: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, 0, fmt.Errorf("creating backup directory: %w", err)
	}

	if err := chownNewDirs(dir, uid, gid); err != nil {
		return 0, 0, fmt.Errorf("setting backup directory ownership: %w", err)
	}

	return uid, gid, nil
}

func findOwnership(dir string) (int, int, error) {
	for path := dir; ; path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			return int(stat.Uid), int(stat.Gid), nil
		}
		if !os.IsNotExist(err) {
			return 0, 0, err
		}
		if path == "/" {
			return 0, 0, fmt.Errorf("no existing parent directory found for %s", dir)
		}
	}
}

func chownNewDirs(dir string, uid, gid int) error {
	// Collect dirs from deepest to shallowest, stopping at the first
	// one that already has the correct ownership.
	var dirs []string
	for path := dir; ; path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err != nil {
			break
		}
		stat := info.Sys().(*syscall.Stat_t)
		if int(stat.Uid) == uid && int(stat.Gid) == gid {
			break
		}
		dirs = append(dirs, path)
		if path == "/" {
			break
		}
	}

	for _, d := range dirs {
		if err := os.Chown(d, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

func parseBackupTime(appName, filename string) (time.Time, bool) {
	prefix := appName + "-"
	suffix := ".tar.gz"

	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, suffix) {
		return time.Time{}, false
	}

	middle := strings.TrimPrefix(filename, prefix)
	middle = strings.TrimSuffix(middle, suffix)

	t, err := time.Parse(backupTimeFormat, middle)
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func copyTarEntriesWithPrefix(src io.Reader, dst *tar.Writer, oldPrefix, newPrefix string) error {
	tr := tar.NewReader(src)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		if oldPrefix != "" && newPrefix != "" {
			if header.Name == oldPrefix {
				header.Name = newPrefix
			} else if strings.HasPrefix(header.Name, oldPrefix+"/") {
				header.Name = newPrefix + strings.TrimPrefix(header.Name, oldPrefix)
			}
		}

		if err := dst.WriteHeader(header); err != nil {
			return err
		}

		if header.Size > 0 {
			if _, err := io.Copy(dst, tr); err != nil {
				return err
			}
		}
	}
}
