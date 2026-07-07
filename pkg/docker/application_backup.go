package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"

	"github.com/basecamp/once/internal/fsutil"
)

const (
	BackupDataDir   = "data"
	BackupRetention = 30 * 24 * time.Hour

	backupAppSettingsEntry = "once.application.json"
	backupVolSettingsEntry = "once.volume.json"
	backupTimeFormat       = "20060102-150405"
)

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
	if dir == "" {
		return fmt.Errorf("backup location is required")
	}
	if !filepath.IsAbs(dir) {
		return ErrBackupPathRelative
	}

	filePath := filepath.Join(dir, name)
	file, err := fsutil.CreateFile(filePath)
	if err != nil {
		slog.Error("Failed to create backup file", "app", a.Settings.Name, "filename", filePath, "error", err)
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer file.Close()

	err = a.backupToWriter(ctx, file)
	a.saveOperationResult(ctx, func(s *State) { s.RecordBackup(a.Settings.Name, err) })

	if err != nil {
		if !errors.Is(err, ErrUnpauseFailed) {
			os.Remove(filePath)
		}
		slog.Error("Backup failed", "app", a.Settings.Name, "filename", filePath, "error", err)
		return err
	}

	slog.Info("Created backup file", "app", a.Settings.Name, "filename", filePath)

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
				slog.Error("Failed to remove expired backup file", "app", a.Settings.Name, "filename", filename, "error", err)
				errs = append(errs, err)
			} else {
				slog.Info("Removed expired backup file", "app", a.Settings.Name, "filename", filename)
			}
		}
	}

	return errors.Join(errs...)
}

func (a *Application) Restore(ctx context.Context, volSettings ApplicationVolumeSettings, volumeData []byte) (returnErr error) {
	slog.Info("Restoring application", "app", a.Settings.Name)

	defer func() {
		if returnErr != nil {
			slog.Error("Restore failed", "app", a.Settings.Name, "error", returnErr)
		} else {
			slog.Info("Restored application", "app", a.Settings.Name)
		}
	}()

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

	if err := a.runRestoreHook(ctx, vol); err != nil {
		vol.Destroy(ctx)
		return fmt.Errorf("running post-restore hook: %w", err)
	}

	if err := a.deployWithVolume(ctx, vol, nil); err != nil {
		vol.Destroy(ctx)
		return err
	}

	return nil
}

// Private

func (a *Application) backupToWriter(ctx context.Context, w io.Writer) error {
	containerName, err := a.ContainerName(ctx)
	if err != nil {
		return fmt.Errorf("getting container name: %w", err)
	}

	found, hookErr := a.tryHookScript(ctx, containerName, "pre-backup")
	if !found && hookErr != nil {
		return fmt.Errorf("checking for pre-backup hook: %w", hookErr)
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

	needsPause := !found || hookErr != nil
	return a.copyVolumeData(ctx, containerName, tw, needsPause)
}

func (a *Application) copyVolumeData(ctx context.Context, containerName string, tw *tar.Writer, pause bool) (returnErr error) {
	if pause {
		info, err := a.namespace.client.ContainerInspect(ctx, containerName)
		if err != nil {
			return fmt.Errorf("inspecting container: %w", err)
		}
		pause = info.State.Running
	}

	if pause {
		slog.Info("Pausing container to create backup", "app", a.Settings.Name)

		if err := a.namespace.client.ContainerPause(ctx, containerName); err != nil {
			return fmt.Errorf("pausing container: %w", err)
		}
		defer func() {
			unpauseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := a.namespace.client.ContainerUnpause(unpauseCtx, containerName); err != nil {
				slog.Error("Failed to unpause container after backup", "app", a.Settings.Name, "container", containerName, "error", err)
				if returnErr == nil {
					returnErr = fmt.Errorf("%w: %w", ErrUnpauseFailed, err)
				}
			}
			slog.Info("Resumed container after creating backup", "app", a.Settings.Name)
		}()
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

func (a *Application) populateVolume(ctx context.Context, vol *ApplicationVolume, data []byte) error {
	containerName := fmt.Sprintf("%s-restore-temp", a.namespace.name)

	resp, err := a.namespace.client.ContainerCreate(ctx,
		&container.Config{
			Image: a.Settings.Image,
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

	defer func() {
		removeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		a.namespace.client.ContainerRemove(removeCtx, resp.ID, container.RemoveOptions{Force: true})
	}()

	if len(data) > 0 {
		if err := a.namespace.client.CopyToContainer(ctx, resp.ID, "/", bytes.NewReader(data), container.CopyToContainerOptions{}); err != nil {
			return fmt.Errorf("copying data to volume: %w", err)
		}
	}

	return nil
}

func (a *Application) runRestoreHook(ctx context.Context, vol *ApplicationVolume) error {
	containerName := fmt.Sprintf("%s-restore-hook-temp", a.namespace.name)

	resp, err := a.namespace.client.ContainerCreate(ctx,
		&container.Config{
			Image:      a.Settings.Image,
			Entrypoint: []string{},
			Cmd:        []string{"sleep", "infinity"},
			Env:        a.Settings.BuildEnv(vol.Settings),
		},
		&container.HostConfig{Mounts: a.volumeMounts(vol)},
		nil, nil, containerName,
	)
	if err != nil {
		return fmt.Errorf("creating restore hook container: %w", err)
	}
	defer func() {
		removeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := a.namespace.client.ContainerRemove(removeCtx, resp.ID, container.RemoveOptions{Force: true}); err != nil {
			slog.Error("Failed to remove restore hook container", "app", a.Settings.Name, "container", containerName, "error", err)
		}
	}()

	if err := a.namespace.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting restore hook container: %w", err)
	}

	return a.runHookScript(ctx, containerName, "post-restore")
}

func (a *Application) runHookScript(ctx context.Context, containerName, name string) error {
	_, err := a.tryHookScript(ctx, containerName, name)
	return err
}

func (a *Application) tryHookScript(ctx context.Context, containerName, name string) (bool, error) {
	path := "/hooks/" + name

	_, err := a.namespace.client.ContainerStatPath(ctx, containerName, path)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking for hook script %q: %w", name, err)
	}

	result, err := execInContainer(ctx, a.namespace.client, containerName, []string{path})
	if err != nil {
		return true, err
	}
	if result.ExitCode != 0 {
		return true, fmt.Errorf("hook script %q failed with exit code %d: %s", name, result.ExitCode, result.Stderr)
	}
	return true, nil
}

// Helpers

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
