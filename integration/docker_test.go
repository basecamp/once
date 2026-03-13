package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestDockerDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "campfire",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "campfire.localhost",
	})

	// After deploy + refresh, the namespace should know about the app
	require.NotNil(t, app)
	assert.Equal(t, "campfire", app.Settings.Name)
	assert.Len(t, ns.Applications(), 1)
	assert.True(t, ns.HostInUse("campfire.localhost"))
}

func TestRestoreState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1, err := docker.NewNamespace("once-restore-test")
	require.NoError(t, err)
	defer ns1.Teardown(ctx, true)

	require.NoError(t, ns1.EnsureNetwork(ctx))

	proxySettings := getProxyPorts(t)
	require.NoError(t, ns1.Proxy().Boot(ctx, proxySettings))

	app := deployApp(t, ctx, ns1, docker.ApplicationSettings{
		Name:  "testapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "testapp.localhost",
	})

	ns2, err := docker.RestoreNamespace(ctx, "once-restore-test")
	require.NoError(t, err)

	require.NotNil(t, ns2.Proxy().Settings)
	assert.Equal(t, proxySettings.HTTPPort, ns2.Proxy().Settings.HTTPPort)
	assert.Equal(t, proxySettings.HTTPSPort, ns2.Proxy().Settings.HTTPSPort)

	restoredApp := ns2.Application("testapp")
	require.NotNil(t, restoredApp)
	assert.Equal(t, app.Settings.Image, restoredApp.Settings.Image)
}

func TestVolumePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1, err := docker.NewNamespace("once-volume-test")
	require.NoError(t, err)

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	testFile := "/home/kamal-proxy/.config/kamal-proxy/test-persistence.txt"
	require.NoError(t, ns1.Proxy().Exec(ctx, []string{"sh", "-c", "echo 'hello' > " + testFile}))
	require.NoError(t, ns1.Teardown(ctx, false))

	ns2, err := docker.NewNamespace("once-volume-test")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))
	require.NoError(t, ns2.Proxy().Exec(ctx, []string{"test", "-f", testFile}), "test file should exist after reboot")
}

func TestApplicationVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-volume-label-test")
	require.NoError(t, err)

	vol1, err := docker.CreateVolume(ctx, ns, "testapp", docker.ApplicationVolumeSettings{SecretKeyBase: "test-secret"})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", vol1.SecretKeyBase())

	vol2, err := docker.FindVolume(ctx, ns, "testapp")
	require.NoError(t, err)
	assert.Equal(t, vol1.SecretKeyBase(), vol2.SecretKeyBase())

	require.NoError(t, vol1.Destroy(ctx))
}

func TestGaplessDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-gapless-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "gapless",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "gapless.localhost",
	})

	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	firstSecretKeyBase := vol.SecretKeyBase()

	firstName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	containerPrefix := "once-gapless-test-app-gapless-"
	countBefore := countContainers(t, ctx, containerPrefix)

	require.NoError(t, app.Deploy(ctx, nil), "second deploy")

	countAfter := countContainers(t, ctx, containerPrefix)
	assert.Equal(t, countBefore, countAfter, "container count should not change")

	vol2, err := app.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, firstSecretKeyBase, vol2.SecretKeyBase(), "SecretKeyBase should persist across deploys")

	secondName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstName, secondName, "container name should change between deploys")

	require.NoError(t, ns.Refresh(ctx))
	assert.Len(t, ns.Applications(), 1, "should have exactly one application after redeploy and refresh")
}

func TestLargeLabelData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	largeValue := strings.Repeat("x", 64*1024) // 64KB

	ns, err := docker.NewNamespace("once-large-label-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "largelabel",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "largelabel.localhost",
		EnvVars: map[string]string{
			"LARGE_VALUE": largeValue,
		},
	})

	ns2, err := docker.RestoreNamespace(ctx, "once-large-label-test")
	require.NoError(t, err)

	restoredApp := ns2.Application("largelabel")
	require.NotNil(t, restoredApp)
	assert.Equal(t, largeValue, restoredApp.Settings.EnvVars["LARGE_VALUE"])
}

func TestStartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-startstop-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "startstop",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "startstop.localhost",
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	assertContainerRunning(t, ctx, containerName, true)

	require.NoError(t, app.Stop(ctx))
	assertContainerRunning(t, ctx, containerName, false)

	require.NoError(t, app.Start(ctx))
	assertContainerRunning(t, ctx, containerName, true)
}

func TestLongAppName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Container names can be very long since we use container IDs for proxy targeting.
	// This test verifies that long app names work correctly.
	longName := strings.Repeat("x", 200)

	ns, err := docker.NewNamespace("once-long-name-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  longName,
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "longname.localhost",
	})

	ns2, err := docker.RestoreNamespace(ctx, "once-long-name-test")
	require.NoError(t, err)

	restoredApp := ns2.Application(longName)
	require.NotNil(t, restoredApp)
	assert.Equal(t, longName, restoredApp.Settings.Name)
}

func TestContainerLogConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-logconfig-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "logtest",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "logtest.localhost",
	})

	assertContainerLogConfig(t, ctx, "once-logconfig-test-proxy")

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assertContainerLogConfig(t, ctx, containerName)
}

func TestBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-backup-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "backupapp",
		Image: imageName,
		Host:  "backupapp.localhost",
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	// Create a test file in storage
	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'test content' > /rails/storage/testfile.txt",
	})

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	entries := extractTarGz(t, backupFile)

	assert.Contains(t, entries, "once.application.json")
	var appSettings docker.ApplicationSettings
	require.NoError(t, json.Unmarshal(entries["once.application.json"], &appSettings))
	assert.Equal(t, "backupapp", appSettings.Name)
	assert.Equal(t, imageName, appSettings.Image)

	assert.Contains(t, entries, "once.volume.json")
	var volSettings docker.ApplicationVolumeSettings
	require.NoError(t, json.Unmarshal(entries["once.volume.json"], &volSettings))
	assert.NotEmpty(t, volSettings.SecretKeyBase)

	assert.Contains(t, entries, "data/testfile.txt")
	assert.Equal(t, "test content\n", string(entries["data/testfile.txt"]))
}

func TestRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create and backup an app
	ns1, err := docker.NewNamespace("once-restore-src")
	require.NoError(t, err)
	defer ns1.Teardown(ctx, true)

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := deployApp(t, ctx, ns1, docker.ApplicationSettings{
		Name:  "restoreapp",
		Image: imageName,
		Host:  "restore.localhost",
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'restore test data' > /rails/storage/restore-test.txt",
	})

	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	originalSecretKeyBase := vol.SecretKeyBase()

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	// Restore to a different namespace
	ns2, err := docker.NewNamespace("once-restore-dst")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	restoredApp, err := ns2.Restore(ctx, backupFile)
	require.NoError(t, err)

	// Verify the restored app gets a fresh unique name based on the image
	assert.True(t, strings.HasPrefix(restoredApp.Settings.Name, "once-campfire."), "restored name should start with image base name")
	assert.NotEqual(t, "restoreapp", restoredApp.Settings.Name)
	assert.Equal(t, imageName, restoredApp.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredApp.Settings.Host)

	// Verify the namespace is refreshed — the app should be visible immediately
	assert.NotNil(t, ns2.Application(restoredApp.Settings.Name), "app should be in namespace immediately after Restore")
	assert.True(t, ns2.HostInUse("restore.localhost"), "hostname should be in use after Restore")

	// Verify volume settings (SecretKeyBase) were preserved
	restoredVol, err := restoredApp.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, restoredVol.SecretKeyBase())

	// Verify data was restored
	restoredContainerName, err := restoredApp.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, restoredContainerName, []string{
		"test", "-f", "/rails/storage/restore-test.txt",
	})

	// Verify that the app and volume are properly labelled by restoring the namespace
	restoredName := restoredApp.Settings.Name
	ns3, err := docker.RestoreNamespace(ctx, "once-restore-dst")
	require.NoError(t, err)

	restoredAppFromState := ns3.Application(restoredName)
	require.NotNil(t, restoredAppFromState, "app should be discoverable after restore")
	assert.Equal(t, imageName, restoredAppFromState.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredAppFromState.Settings.Host)

	volFromState, err := restoredAppFromState.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, volFromState.SecretKeyBase(), "volume SecretKeyBase should be preserved")
}

func TestRestoreHostnameConflictFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-restore-host-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "existingapp",
		Image: imageName,
		Host:  "existingapp.localhost",
	})

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	// Try to restore when another app already uses the same hostname
	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	_, err = ns.Restore(ctx, backupFile)
	assert.ErrorIs(t, err, docker.ErrHostnameInUse)
}

func TestBackupHookBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-backup-hook-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "hooktest",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "hooktest.localhost",
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	backupDir := t.TempDir()

	t.Run("WithoutHook", func(t *testing.T) {
		stop := collectPauseEvents(t, ctx, containerName)
		require.NoError(t, app.BackupToFile(ctx, backupDir, "no-hook.tar.gz"))
		actions := stop()
		assert.Contains(t, actions, "pause")
		assert.Contains(t, actions, "unpause")
	})

	t.Run("WithSuccessfulHook", func(t *testing.T) {
		copyHookToContainer(t, ctx, containerName, "pre-backup", []byte("#!/bin/sh\nexit 0"))

		stop := collectPauseEvents(t, ctx, containerName)
		require.NoError(t, app.BackupToFile(ctx, backupDir, "successful-hook.tar.gz"))
		actions := stop()
		assert.Empty(t, actions)
	})

	t.Run("WithFailedHook", func(t *testing.T) {
		copyHookToContainer(t, ctx, containerName, "pre-backup", []byte("#!/bin/sh\nexit 1"))

		stop := collectPauseEvents(t, ctx, containerName)
		require.NoError(t, app.BackupToFile(ctx, backupDir, "failed-hook.tar.gz"))
		actions := stop()
		assert.Contains(t, actions, "pause")
		assert.Contains(t, actions, "unpause")
	})
}

func TestBackupStoppedContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-backup-stopped-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "stoppedapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "stoppedapp.localhost",
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'stopped test content' > /rails/storage/testfile.txt",
	})

	require.NoError(t, app.Stop(ctx))

	stop := collectPauseEvents(t, ctx, containerName)

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	actions := stop()
	assert.Empty(t, actions)

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	entries := extractTarGz(t, backupFile)

	assert.Contains(t, entries, "once.application.json")
	assert.Contains(t, entries, "once.volume.json")
	assert.Contains(t, entries, "data/testfile.txt")
	assert.Equal(t, "stopped test content\n", string(entries["data/testfile.txt"]))
}

func TestRestoreWithPostRestoreHook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalRegistry(t, ctx)
	hookImage := buildHookImage(t, ctx, registryURL, "restore-hook-success", "#!/bin/sh\ncp /storage/hook-input /storage/hook-output")

	backup := buildTestBackup(t, hookImage)

	ns, err := docker.NewNamespace("once-restore-hook-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	restoredApp, err := ns.Restore(ctx, bytes.NewReader(backup))
	require.NoError(t, err)
	assert.NotEmpty(t, restoredApp.Settings.Name)

	containerName, err := restoredApp.ContainerName(ctx)
	require.NoError(t, err)
	execInContainer(t, ctx, containerName, []string{"test", "-f", "/storage/hook-output"})
}

func TestRestoreFailsWithFailedPostRestoreHook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalRegistry(t, ctx)
	hookImage := buildHookImage(t, ctx, registryURL, "restore-hook-fail", "#!/bin/sh\nexit 1")

	backup := buildTestBackup(t, hookImage)

	ns, err := docker.NewNamespace("once-restore-hook-fail-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	_, err = ns.Restore(ctx, bytes.NewReader(backup))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post-restore")
}

func TestRemoveApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-remove-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "removeapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "removeapp.localhost",
	})

	containerPrefix := "once-remove-test-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, false))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err = docker.FindVolume(ctx, ns, "removeapp")
	assert.NoError(t, err, "volume should still exist")
}

func TestRemoveApplicationWithData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-removedata-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "removeapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "removeapp.localhost",
	})

	containerPrefix := "once-removedata-test-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, true))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err = docker.FindVolume(ctx, ns, "removeapp")
	assert.ErrorIs(t, err, docker.ErrVolumeNotFound)
}

func TestDeployWithSettings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-settings-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	settings := docker.ApplicationSettings{
		Name:       "settingsapp",
		Image:      "ghcr.io/basecamp/once-campfire:main",
		Host:       "settingsapp.localhost",
		DisableTLS: true,
		EnvVars:    map[string]string{"CUSTOM_VAR": "custom_value", "ANOTHER": "thing"},
		SMTP: docker.SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user",
			Password: "pass",
			From:     "noreply@example.com",
		},
		Resources:  docker.ContainerResources{CPUs: 1, MemoryMB: 512},
		AutoUpdate: false,
		Backup:     docker.BackupSettings{Path: "/backups", AutoBackup: true},
	}

	app := deployApp(t, ctx, ns, settings)

	// Verify settings persisted via label restore
	ns2, err := docker.RestoreNamespace(ctx, "once-settings-test")
	require.NoError(t, err)

	restored := ns2.Application("settingsapp")
	require.NotNil(t, restored)
	assert.True(t, restored.Settings.DisableTLS)
	assert.Equal(t, "custom_value", restored.Settings.EnvVars["CUSTOM_VAR"])
	assert.Equal(t, "thing", restored.Settings.EnvVars["ANOTHER"])
	assert.Equal(t, "smtp.example.com", restored.Settings.SMTP.Server)
	assert.Equal(t, "587", restored.Settings.SMTP.Port)
	assert.False(t, restored.Settings.AutoUpdate)
	assert.Equal(t, "/backups", restored.Settings.Backup.Path)
	assert.True(t, restored.Settings.Backup.AutoBackup)

	// Verify container env vars
	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	envVars := inspectContainerEnv(t, ctx, containerName)
	assert.Contains(t, envVars, "CUSTOM_VAR=custom_value")
	assert.Contains(t, envVars, "ANOTHER=thing")
	assert.Contains(t, envVars, "SMTP_ADDRESS=smtp.example.com")

	// Verify container resources
	assertContainerResources(t, ctx, containerName, 1e9, 512*1024*1024)
}

func TestRedeployToExistingHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-redeploy-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	// Initial deploy with settings
	initialSettings := docker.ApplicationSettings{
		Name:    "redeployapp",
		Image:   "ghcr.io/basecamp/once-campfire:main",
		Host:    "redeploy.localhost",
		EnvVars: map[string]string{"OLD_VAR": "old_value"},
		SMTP: docker.SMTPSettings{
			Server: "smtp.old.com",
			Port:   "25",
		},
		Resources: docker.ContainerResources{CPUs: 2, MemoryMB: 1024},
	}

	app := deployApp(t, ctx, ns, initialSettings)
	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	originalSecretKeyBase := vol.SecretKeyBase()

	// Redeploy to the same host with different settings via DeployApplication
	newSettings := docker.ApplicationSettings{
		Image:   "ghcr.io/basecamp/once-campfire:main",
		Host:    "redeploy.localhost",
		EnvVars: map[string]string{"NEW_VAR": "new_value"},
	}

	redeployedApp, isNew, err := ns.DeployApplication(ctx, newSettings, nil)
	require.NoError(t, err)
	assert.False(t, isNew)
	require.NoError(t, ns.Refresh(ctx))

	// Should still be one app
	assert.Len(t, ns.Applications(), 1)

	// Name preserved
	assert.Equal(t, "redeployapp", redeployedApp.Settings.Name)

	// Volume preserved
	vol2, err := redeployedApp.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, vol2.SecretKeyBase())

	// New env vars present, old ones gone
	containerName, err := redeployedApp.ContainerName(ctx)
	require.NoError(t, err)
	envVars := inspectContainerEnv(t, ctx, containerName)
	assert.Contains(t, envVars, "NEW_VAR=new_value")
	assertEnvAbsent(t, envVars, "OLD_VAR")
	assertEnvAbsent(t, envVars, "SMTP_ADDRESS")

	// Resources cleared (zero values)
	assertContainerResources(t, ctx, containerName, 0, 0)
}

// Helpers

func TestContainerResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-res-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:      "campfire",
		Image:     "ghcr.io/basecamp/once-campfire:main",
		Host:      "campfire.localhost",
		Resources: docker.ContainerResources{CPUs: 1, MemoryMB: 1024},
	})

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	assertContainerResources(t, ctx, containerName, 1e9, 1024*1024*1024)
}

func deployApp(t *testing.T, ctx context.Context, ns *docker.Namespace, settings docker.ApplicationSettings) *docker.Application {
	t.Helper()
	app := docker.NewApplication(ns, settings)
	require.NoError(t, app.Deploy(ctx, nil))
	require.NoError(t, ns.Refresh(ctx))
	return ns.Application(settings.Name)
}

func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func getProxyPorts(t *testing.T) docker.ProxySettings {
	t.Helper()
	return docker.ProxySettings{
		HTTPPort:    getFreePort(t),
		HTTPSPort:   getFreePort(t),
		MetricsPort: getFreePort(t),
	}
}

func assertContainerRunning(t *testing.T, ctx context.Context, name string, expectRunning bool) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	if expectRunning {
		assert.True(t, info.State.Running, "container should be running")
	} else {
		assert.False(t, info.State.Running, "container should be stopped")
	}
}

func assertContainerResources(t *testing.T, ctx context.Context, name string, expectedCPUs, expectedMemory int64) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	assert.Equal(t, expectedCPUs, info.HostConfig.NanoCPUs)
	assert.Equal(t, expectedMemory, info.HostConfig.Memory)
}

func assertContainerLogConfig(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	assert.Equal(t, "json-file", info.HostConfig.LogConfig.Type)
	assert.Equal(t, docker.ContainerLogMaxSize, info.HostConfig.LogConfig.Config["max-size"])
	assert.Equal(t, "1", info.HostConfig.LogConfig.Config["max-file"])
}

func countContainers(t *testing.T, ctx context.Context, prefix string) int {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
	require.NoError(t, err)

	count := 0
	for _, ctr := range containers {
		if len(ctr.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(ctr.Names[0], "/")
		if strings.HasPrefix(name, prefix) {
			count++
		}
	}
	return count
}

func execInContainer(t *testing.T, ctx context.Context, containerName string, cmd []string) {
	t.Helper()

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	execResp, err := c.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	require.NoError(t, err)

	resp, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	require.NoError(t, err)
	defer resp.Close()

	_, err = io.Copy(io.Discard, resp.Reader)
	require.NoError(t, err)

	inspect, err := c.ContainerExecInspect(ctx, execResp.ID)
	require.NoError(t, err)
	require.Equal(t, 0, inspect.ExitCode, "exec command failed")
}

func copyHookToContainer(t *testing.T, ctx context.Context, containerName, hookName string, script []byte) {
	t.Helper()

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "hooks/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "hooks/" + hookName,
		Mode: 0755,
		Size: int64(len(script)),
	}))
	_, err = tw.Write(script)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	require.NoError(t, c.CopyToContainer(ctx, containerName, "/", &buf, container.CopyToContainerOptions{}))
}

func collectPauseEvents(t *testing.T, ctx context.Context, containerName string) func() []string {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	eventCtx, eventCancel := context.WithCancel(ctx)
	eventCh, errCh := c.Events(eventCtx, events.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("container", containerName),
			filters.Arg("event", "pause"),
			filters.Arg("event", "unpause"),
		),
	})

	var actions []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case e, ok := <-eventCh:
				if !ok {
					return
				}
				actions = append(actions, string(e.Action))
			case <-errCh:
				return
			}
		}
	}()

	return func() []string {
		time.Sleep(100 * time.Millisecond)
		eventCancel()
		<-done
		c.Close()
		return actions
	}
}

func startLocalRegistry(t *testing.T, ctx context.Context) string {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	reader, err := c.ImagePull(ctx, "registry:2", image.PullOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, reader)
	reader.Close()

	port := getFreePort(t)
	portStr := strconv.Itoa(port)

	resp, err := c.ContainerCreate(ctx,
		&container.Config{Image: "registry:2"},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				"5000/tcp": []nat.PortBinding{{HostPort: portStr}},
			},
		},
		nil, nil, fmt.Sprintf("test-registry-%s", portStr),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		c.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true})
	})

	require.NoError(t, c.ContainerStart(ctx, resp.ID, container.StartOptions{}))

	registryURL := fmt.Sprintf("localhost:%d", port)
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + registryURL + "/v2/")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 200*time.Millisecond, "registry did not become ready")

	return registryURL
}

func buildHookImage(t *testing.T, ctx context.Context, registryURL, name, hookScript string) string {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	dockerfile := `FROM ghcr.io/basecamp/once-campfire:main
COPY post-restore /hooks/post-restore
`

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	addTarEntry := func(name string, data []byte) {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0644}))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}
	addTarEntry("Dockerfile", []byte(dockerfile))

	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "post-restore", Size: int64(len(hookScript)), Mode: 0755}))
	_, err = tw.Write([]byte(hookScript))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	fullTag := registryURL + "/" + name + ":latest"

	buildResp, err := c.ImageBuild(ctx, &buf, build.ImageBuildOptions{
		Tags:       []string{fullTag},
		Dockerfile: "Dockerfile",
		Remove:     true,
	})
	require.NoError(t, err)
	io.Copy(io.Discard, buildResp.Body)
	buildResp.Body.Close()

	pushResp, err := c.ImagePush(ctx, fullTag, image.PushOptions{RegistryAuth: "e30="}) // base64 "{}"
	require.NoError(t, err)
	io.Copy(io.Discard, pushResp)
	pushResp.Close()

	return fullTag
}

func buildTestBackup(t *testing.T, imageName string) []byte {
	t.Helper()

	appSettings := docker.ApplicationSettings{
		Name:  "hookapp",
		Image: imageName,
		Host:  "hookapp.localhost",
	}
	volSettings := docker.ApplicationVolumeSettings{SecretKeyBase: "test-secret-key"}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	writeEntry := func(name string, data []byte) {
		header := &tar.Header{Name: name, Size: int64(len(data)), Mode: 0644}
		require.NoError(t, tw.WriteHeader(header))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}

	writeEntry("once.application.json", []byte(appSettings.Marshal()))
	writeEntry("once.volume.json", []byte(volSettings.Marshal()))

	// Add data directory with a marker file for hook testing.
	// Use UID/GID 1000 to match realistic backup ownership.
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "data/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
		Uid:      1000,
		Gid:      1000,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "data/hook-input",
		Mode: 0644,
		Size: int64(len("test data")),
		Uid:  1000,
		Gid:  1000,
	}))
	_, writeErr := tw.Write([]byte("test data"))
	require.NoError(t, writeErr)

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	return buf.Bytes()
}

func inspectContainerEnv(t *testing.T, ctx context.Context, name string) []string {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)
	return info.Config.Env
}

func assertEnvAbsent(t *testing.T, envVars []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, e := range envVars {
		if strings.HasPrefix(e, prefix) {
			t.Errorf("expected env var %s to be absent, but found %s", key, e)
		}
	}
}

func extractTarGz(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()

	gr, err := gzip.NewReader(r)
	require.NoError(t, err)
	defer gr.Close()

	tr := tar.NewReader(gr)
	entries := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			entries[header.Name] = data
		}
	}

	return entries
}
