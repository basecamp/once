package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/command"
	"github.com/basecamp/once/internal/docker"
)

const integrationAppImageRef = "ghcr.io/basecamp/once-integration-app:latest"

// Pre-pull shared images so parallel tests don't all block on the same pull
// inside their own timeouts.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	err := pullImages(ctx, integrationAppImageRef, "registry:2", docker.ProxyImage)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to pull test images:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestDockerDeployment(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "campfire",
		Image: integrationAppImageRef,
		Host:  "campfire.localhost",
	})

	// After deploy + refresh, the namespace should know about the app
	require.NotNil(t, app)
	assert.Equal(t, "campfire", app.Settings.Name)
	assert.Len(t, ns.Applications(), 1)
	assert.True(t, ns.HostInUse("campfire.localhost"))
}

func TestRestoreState(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1 := newTestNamespace(t, "once-restore-test")

	require.NoError(t, ns1.EnsureNetwork(ctx))

	proxySettings := getProxyPorts(t)
	require.NoError(t, ns1.Proxy().Boot(ctx, proxySettings))

	app := deployApp(t, ctx, ns1, docker.ApplicationSettings{
		Name:  "testapp",
		Image: integrationAppImageRef,
		Host:  "testapp.localhost",
	})

	ns2, err := docker.RestoreNamespace(ctx, ns1.Name())
	require.NoError(t, err)

	require.NotNil(t, ns2.Proxy().Settings)
	assert.Equal(t, proxySettings.HTTPPort, ns2.Proxy().Settings.HTTPPort)
	assert.Equal(t, proxySettings.HTTPSPort, ns2.Proxy().Settings.HTTPSPort)

	restoredApp := ns2.Application("testapp")
	require.NotNil(t, restoredApp)
	assert.Equal(t, app.Settings.Image, restoredApp.Settings.Image)
}

func TestRestoreAdoptsLegacyVolumeKeys(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-legacy-keys-test")

	legacyKeys := docker.Keys{
		SecretKeyBase:   "legacy-secret",
		VAPIDPublicKey:  "legacy-pub",
		VAPIDPrivateKey: "legacy-priv",
	}
	_, err := docker.CreateVolume(ctx, ns, "legacyapp", docker.ApplicationLegacyVolumeSettings{Keys: legacyKeys})
	require.NoError(t, err)

	// A legacy install's app container has settings without keys on its label
	settings := docker.ApplicationSettings{
		Name:  "legacyapp",
		Image: integrationAppImageRef,
		Host:  "legacyapp.localhost",
	}

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	reader, err := c.ImagePull(ctx, settings.Image, image.PullOptions{})
	require.NoError(t, err)
	io.Copy(io.Discard, reader)
	reader.Close()

	_, err = c.ContainerCreate(ctx,
		&container.Config{
			Image:  settings.Image,
			Labels: map[string]string{"once": settings.Marshal()},
		},
		nil, nil, nil,
		ns.Name()+"-app-legacyapp-abc123",
	)
	require.NoError(t, err)

	// Adopt the container so teardown removes it
	require.NoError(t, ns.Refresh(ctx))

	restored, err := docker.RestoreNamespace(ctx, ns.Name())
	require.NoError(t, err)

	app := restored.Application("legacyapp")
	require.NotNil(t, app)
	assert.Equal(t, legacyKeys, app.Settings.Keys)
}

func TestApplicationVolume(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-volume-label-test")

	vol1, err := docker.CreateVolume(ctx, ns, "testapp", docker.ApplicationLegacyVolumeSettings{Keys: docker.Keys{SecretKeyBase: "test-secret"}})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", vol1.Settings.SecretKeyBase)

	vol2, err := docker.FindVolume(ctx, ns, "testapp")
	require.NoError(t, err)
	assert.Equal(t, vol1.Settings.SecretKeyBase, vol2.Settings.SecretKeyBase)

	require.NoError(t, vol1.Destroy(ctx))
}

func TestGaplessDeployment(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-gapless-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "gapless",
		Image: integrationAppImageRef,
		Host:  "gapless.localhost",
	})

	firstSecretKeyBase := app.Settings.Keys.SecretKeyBase
	require.NotEmpty(t, firstSecretKeyBase)

	firstName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	containerPrefix := ns.Name() + "-app-gapless-"
	countBefore := countContainers(t, ctx, containerPrefix)

	require.NoError(t, app.Deploy(ctx, nil), "second deploy")

	countAfter := countContainers(t, ctx, containerPrefix)
	assert.Equal(t, countBefore, countAfter, "container count should not change")

	secondName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstName, secondName, "container name should change between deploys")

	require.NoError(t, ns.Refresh(ctx))
	assert.Len(t, ns.Applications(), 1, "should have exactly one application after redeploy and refresh")
	assert.Equal(t, firstSecretKeyBase, ns.Application("gapless").Settings.Keys.SecretKeyBase, "SecretKeyBase should persist across deploys")
}

func TestUpdateDetectsLocalImageChange(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalRegistry(t, ctx)
	imageTag := registryURL + "/update-test:latest"

	buildAndPushImage(t, ctx, imageTag, "v1")

	ns := newTestNamespace(t, "once-update-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "updateapp",
		Image: imageTag,
		Host:  "updateapp.localhost",
	})

	firstContainer, err := app.ContainerName(ctx)
	require.NoError(t, err)

	// Push v2 with the same tag. The registry now holds a newer image than
	// what the running container uses.
	buildAndPushImage(t, ctx, imageTag, "v2")

	changed, err := app.Update(ctx, nil)
	require.NoError(t, err)
	assert.True(t, changed, "Update should detect the newer local image")

	secondContainer, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstContainer, secondContainer, "container should change after update")
}

func TestDeployDetectsMissingRegistryCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalAuthRegistry(t, ctx)
	imageTag := registryURL + "/private-test:latest"
	auth := remote.WithAuth(&authn.Basic{Username: "testuser", Password: "testpass"})
	buildAndPushImage(t, ctx, imageTag, "v1", auth)

	t.Setenv("DOCKER_CONFIG", t.TempDir())

	ns := newTestNamespace(t, "once-auth-test")

	err := docker.NewApplication(ns, docker.ApplicationSettings{
		Name:  "authapp",
		Image: imageTag,
		Host:  "authapp.localhost",
	}).Deploy(ctx, nil)

	var authErr *docker.RegistryAuthError
	require.ErrorAs(t, err, &authErr)
	assert.Equal(t, registryURL, authErr.Registry)
	assert.ErrorIs(t, err, docker.ErrPullFailed)
	assert.Equal(t, "Log in to "+registryURL+" first. This image can't be downloaded without registry credentials.", docker.ErrorMessage(err))
}

func TestDeployWithRegistryCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalAuthRegistry(t, ctx)
	imageTag := registryURL + "/private-deploy-test:latest"
	auth := remote.WithAuth(&authn.Basic{Username: "testuser", Password: "testpass"})
	buildAndPushImage(t, ctx, imageTag, "v1", auth)

	// The Docker credential store holds wrong credentials for this registry,
	// to prove that the credentials in the app settings take precedence.
	configDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", configDir)
	wrongAuth := base64.StdEncoding.EncodeToString([]byte("wronguser:wrongpass"))
	config := fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, registryURL, wrongAuth)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(config), 0600))

	ns := newTestNamespace(t, "once-auth-deploy-test")
	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	settings := docker.ApplicationSettings{
		Name:     "authapp",
		Image:    imageTag,
		Host:     "authapp.localhost",
		Registry: docker.NewRegistrySettings(imageTag, "testuser", "testpass"),
	}
	app := deployApp(t, ctx, ns, settings)
	assert.Equal(t, settings.Registry, app.Settings.Registry, "registry credentials should persist in the app label")
}

func TestLargeLabelData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	largeValue := strings.Repeat("x", 64*1024) // 64KB

	ns := newTestNamespace(t, "once-large-label-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "largelabel",
		Image: integrationAppImageRef,
		Host:  "largelabel.localhost",
		EnvVars: map[string]string{
			"LARGE_VALUE": largeValue,
		},
	})

	ns2, err := docker.RestoreNamespace(ctx, ns.Name())
	require.NoError(t, err)

	restoredApp := ns2.Application("largelabel")
	require.NotNil(t, restoredApp)
	assert.Equal(t, largeValue, restoredApp.Settings.EnvVars["LARGE_VALUE"])
}

func TestStartStop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-startstop-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "startstop",
		Image: integrationAppImageRef,
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

func TestExec(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-exec-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "exec",
		Image: integrationAppImageRef,
		Host:  "exec.localhost",
	})

	require.NoError(t, app.Exec(ctx, []string{"sh", "-c", "exit 0"}))

	err := app.Exec(ctx, []string{"sh", "-c", "exit 7"})
	require.Error(t, err)
	code, ok := command.ExitCode(err)
	assert.True(t, ok)
	assert.Equal(t, 7, code)
}

func TestExecFailsWhenNotRunning(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-exec-stopped-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "exec-stopped",
		Image: integrationAppImageRef,
		Host:  "exec-stopped.localhost",
	})

	require.NoError(t, app.Stop(ctx))
	require.NoError(t, ns.Refresh(ctx))

	err := ns.Application("exec-stopped").Exec(ctx, []string{"sh", "-c", "exit 0"})
	assert.ErrorIs(t, err, docker.ErrApplicationNotRunning)
}

func TestLongAppName(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Container names can be very long since we use container IDs for proxy targeting.
	// This test verifies that long app names work correctly.
	longName := strings.Repeat("x", 200)

	ns := newTestNamespace(t, "once-long-name-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  longName,
		Image: integrationAppImageRef,
		Host:  "longname.localhost",
	})

	ns2, err := docker.RestoreNamespace(ctx, ns.Name())
	require.NoError(t, err)

	restoredApp := ns2.Application(longName)
	require.NotNil(t, restoredApp)
	assert.Equal(t, longName, restoredApp.Settings.Name)
}

func TestContainerLogConfig(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-logconfig-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "logtest",
		Image: integrationAppImageRef,
		Host:  "logtest.localhost",
	})

	assertContainerLogConfig(t, ctx, ns.Name()+"-proxy")

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assertContainerLogConfig(t, ctx, containerName)
}

func TestBackup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-backup-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := integrationAppImageRef
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
	assert.NotEmpty(t, appSettings.Keys.SecretKeyBase)

	assert.Contains(t, entries, "once.volume.json")

	assert.Contains(t, entries, "data/testfile.txt")
	assert.Equal(t, "test content\n", string(entries["data/testfile.txt"]))
}

func TestRestore(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create and backup an app
	ns1 := newTestNamespace(t, "once-restore-src")

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := integrationAppImageRef
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

	originalSecretKeyBase := app.Settings.Keys.SecretKeyBase
	require.NotEmpty(t, originalSecretKeyBase)

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	// Restore to a different namespace
	ns2 := newTestNamespace(t, "once-restore-dst")

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	restoredApp, err := ns2.Restore(ctx, backupFile)
	require.NoError(t, err)

	// Verify the restored app gets a fresh unique name based on the image
	assert.True(t, strings.HasPrefix(restoredApp.Settings.Name, docker.NameFromImageRef(integrationAppImageRef)+"."), "restored name should start with image base name")
	assert.NotEqual(t, "restoreapp", restoredApp.Settings.Name)
	assert.Equal(t, imageName, restoredApp.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredApp.Settings.Host)

	// Verify the namespace is refreshed — the app should be visible immediately
	assert.NotNil(t, ns2.Application(restoredApp.Settings.Name), "app should be in namespace immediately after Restore")
	assert.True(t, ns2.HostInUse("restore.localhost"), "hostname should be in use after Restore")

	// Verify the keys (SecretKeyBase) were preserved
	assert.Equal(t, originalSecretKeyBase, restoredApp.Settings.Keys.SecretKeyBase)

	// Verify data was restored
	restoredContainerName, err := restoredApp.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, restoredContainerName, []string{
		"test", "-f", "/rails/storage/restore-test.txt",
	})

	// Verify that the app and volume are properly labelled by restoring the namespace
	restoredName := restoredApp.Settings.Name
	ns3, err := docker.RestoreNamespace(ctx, ns2.Name())
	require.NoError(t, err)

	restoredAppFromState := ns3.Application(restoredName)
	require.NotNil(t, restoredAppFromState, "app should be discoverable after restore")
	assert.Equal(t, imageName, restoredAppFromState.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredAppFromState.Settings.Host)

	assert.Equal(t, originalSecretKeyBase, restoredAppFromState.Settings.Keys.SecretKeyBase, "SecretKeyBase should be preserved")
}

func TestRestoreHostnameConflictFails(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-restore-host-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := integrationAppImageRef
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
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-backup-hook-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "hooktest",
		Image: integrationAppImageRef,
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
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-backup-stopped-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "stoppedapp",
		Image: integrationAppImageRef,
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
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalRegistry(t, ctx)
	hookImage := buildHookImage(t, ctx, registryURL, "restore-hook-success", "#!/bin/sh\ncp /storage/hook-input /storage/hook-output")

	backup := buildTestBackup(t, hookImage)

	ns := newTestNamespace(t, "once-restore-hook-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	restoredApp, err := ns.Restore(ctx, bytes.NewReader(backup))
	require.NoError(t, err)
	assert.NotEmpty(t, restoredApp.Settings.Name)
	assert.Equal(t, "test-secret-key", restoredApp.Settings.Keys.SecretKeyBase, "legacy key from backup volume settings should migrate to app settings")

	containerName, err := restoredApp.ContainerName(ctx)
	require.NoError(t, err)
	execInContainer(t, ctx, containerName, []string{"test", "-f", "/storage/hook-output"})
}

func TestRestoreFailsWithFailedPostRestoreHook(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	registryURL := startLocalRegistry(t, ctx)
	hookImage := buildHookImage(t, ctx, registryURL, "restore-hook-fail", "#!/bin/sh\nexit 1")

	backup := buildTestBackup(t, hookImage)

	ns := newTestNamespace(t, "once-restore-hook-fail-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	_, err := ns.Restore(ctx, bytes.NewReader(backup))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post-restore")
}

func TestRemoveApplication(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-remove-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "removeapp",
		Image: integrationAppImageRef,
		Host:  "removeapp.localhost",
	})

	containerPrefix := ns.Name() + "-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, false))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err := docker.FindVolume(ctx, ns, "removeapp")
	assert.NoError(t, err, "volume should still exist")
}

func TestVerifyHTTPOrRemoveAllowsRedeployWithSameHost(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-verify-redeploy-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "first",
		Image: integrationAppImageRef,
		Host:  "reuse.invalid",
	})

	err := app.VerifyHTTPOrRemove(ctx)
	require.ErrorIs(t, err, docker.ErrVerificationFailed)

	require.NoError(t, ns.Refresh(ctx))
	assert.False(t, ns.HostInUse("reuse.invalid"))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "second",
		Image: integrationAppImageRef,
		Host:  "reuse.invalid",
	})
}

func TestRemoveApplicationWithData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-removedata-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "removeapp",
		Image: integrationAppImageRef,
		Host:  "removeapp.localhost",
	})

	containerPrefix := ns.Name() + "-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, true))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err := docker.FindVolume(ctx, ns, "removeapp")
	assert.ErrorIs(t, err, docker.ErrVolumeNotFound)
}

func TestDeployWithSettings(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-settings-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	settings := docker.ApplicationSettings{
		Name:       "settingsapp",
		Image:      integrationAppImageRef,
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
	ns2, err := docker.RestoreNamespace(ctx, ns.Name())
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

func TestUpdatePreservesSettings(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-update-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	// Deploy with full settings
	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:    "updateapp",
		Image:   integrationAppImageRef,
		Host:    "update.localhost",
		EnvVars: map[string]string{"MY_VAR": "my_value"},
		SMTP: docker.SMTPSettings{
			Server: "smtp.example.com",
			Port:   "587",
		},
		Resources: docker.ContainerResources{CPUs: 2, MemoryMB: 1024},
	})

	originalSecretKeyBase := app.Settings.Keys.SecretKeyBase
	require.NotEmpty(t, originalSecretKeyBase)

	// Update only the env vars, leaving everything else as-is
	newSettings := app.Settings
	newSettings.EnvVars = map[string]string{"NEW_VAR": "new_value"}
	app.Settings = newSettings
	require.NoError(t, app.Deploy(ctx, nil))
	require.NoError(t, ns.Refresh(ctx))

	updatedApp := ns.ApplicationByHost("update.localhost")
	require.NotNil(t, updatedApp)

	// Name preserved
	assert.Equal(t, "updateapp", updatedApp.Settings.Name)

	// SMTP and resources preserved
	assert.Equal(t, "smtp.example.com", updatedApp.Settings.SMTP.Server)
	assert.Equal(t, "587", updatedApp.Settings.SMTP.Port)
	assert.Equal(t, 2, updatedApp.Settings.Resources.CPUs)
	assert.Equal(t, 1024, updatedApp.Settings.Resources.MemoryMB)

	// Env vars replaced
	containerName, err := updatedApp.ContainerName(ctx)
	require.NoError(t, err)
	envVars := inspectContainerEnv(t, ctx, containerName)
	assert.Contains(t, envVars, "NEW_VAR=new_value")
	assertEnvAbsent(t, envVars, "MY_VAR")

	// Keys preserved
	assert.Equal(t, originalSecretKeyBase, updatedApp.Settings.Keys.SecretKeyBase)
}

func TestUpdateSettings(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-update-settings-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "updatesettingsapp",
		Image: integrationAppImageRef,
		Host:  "updatesettings.localhost",
	})

	oldContainerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	newSettings := app.Settings
	newSettings.EnvVars = map[string]string{"UPDATED_VAR": "updated_value"}
	require.NoError(t, app.UpdateSettings(ctx, newSettings, nil))
	require.NoError(t, ns.Refresh(ctx))

	updatedApp := ns.ApplicationByHost("updatesettings.localhost")
	require.NotNil(t, updatedApp)
	assert.True(t, updatedApp.Running)
	assert.Equal(t, "updated_value", updatedApp.Settings.EnvVars["UPDATED_VAR"])

	containerName, err := updatedApp.ContainerName(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, oldContainerName, containerName)

	envVars := inspectContainerEnv(t, ctx, containerName)
	assert.Contains(t, envVars, "UPDATED_VAR=updated_value")
}

func TestUpdateChangeHost(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-update-host-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "hostchangeapp",
		Image: integrationAppImageRef,
		Host:  "old.localhost",
	})

	// Change the host
	app.Settings.Host = "new.localhost"
	require.NoError(t, app.Deploy(ctx, nil))
	require.NoError(t, ns.Refresh(ctx))

	assert.Nil(t, ns.ApplicationByHost("old.localhost"))
	assert.NotNil(t, ns.ApplicationByHost("new.localhost"))
}

func TestUpdateHostCollision(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-update-collision-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "app1",
		Image: integrationAppImageRef,
		Host:  "host1.localhost",
	})

	app2 := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:  "app2",
		Image: integrationAppImageRef,
		Host:  "host2.localhost",
	})

	// Attempting to change app2's host to app1's host should be detected
	assert.True(t, ns.HostInUseByAnother("host1.localhost", app2.Settings.Name))
}

// Helpers

func TestContainerResources(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns := newTestNamespace(t, "once-res-test")

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := deployApp(t, ctx, ns, docker.ApplicationSettings{
		Name:      "campfire",
		Image:     integrationAppImageRef,
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

// Test ports are allocated from a range below the Linux ephemeral port range
// (32768-60999), so that concurrent tests never race each other or the kernel
// for the same port.
var nextPort atomic.Int32

func getFreePort(t *testing.T) int {
	t.Helper()
	for {
		port := 20000 + int(nextPort.Add(1))
		require.Less(t, port, 32768, "exhausted test port range")
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		listener.Close()
		return port
	}
}

func newTestNamespace(t *testing.T, base string) *docker.Namespace {
	t.Helper()
	suffix := make([]byte, 4)
	_, err := rand.Read(suffix)
	require.NoError(t, err)
	ns, err := docker.NewNamespace(fmt.Sprintf("%s-%x", base, suffix))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		assert.NoError(t, ns.Teardown(ctx, true), "namespace teardown failed")
	})
	return ns
}

func pullImages(ctx context.Context, images ...string) error {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer c.Close()

	for _, img := range images {
		reader, err := c.ImagePull(ctx, img, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("pulling %s: %w", img, err)
		}
		_, err = io.Copy(io.Discard, reader)
		reader.Close()
		if err != nil {
			return fmt.Errorf("pulling %s: %w", img, err)
		}
	}
	return nil
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
		Mode:     0o755,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "hooks/" + hookName,
		Mode: 0o755,
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
	return startRegistryContainer(t, ctx, nil, nil, http.StatusOK)
}

func startLocalAuthRegistry(t *testing.T, ctx context.Context) string {
	t.Helper()

	authDir := t.TempDir()
	htpasswd := "testuser:$2a$04$MpM8fR.Xgy8DLIwkC85y/eyL7GYvMxHPMcfv7JJCJElJuTjezW.Xa\n" // bcrypt of "testpass"
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "htpasswd"), []byte(htpasswd), 0644))

	env := []string{
		"REGISTRY_AUTH=htpasswd",
		"REGISTRY_AUTH_HTPASSWD_REALM=test",
		"REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd",
	}
	binds := []string{authDir + ":/auth:ro"}

	return startRegistryContainer(t, ctx, env, binds, http.StatusUnauthorized)
}

func startRegistryContainer(t *testing.T, ctx context.Context, env, binds []string, readyStatus int) string {
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
		&container.Config{Image: "registry:2", Env: env},
		&container.HostConfig{
			Binds: binds,
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
		return resp.StatusCode == readyStatus
	}, 10*time.Second, 200*time.Millisecond, "registry did not become ready")

	return registryURL
}

// Derived test images are assembled with go-containerregistry and written
// straight to the local registry, keeping the docker daemon out of the
// publishing path. Pushing through the daemon breaks when its containerd
// store holds a layer's content without the compressed blob a manifest
// references (moby/moby#49784).
func buildHookImage(t *testing.T, ctx context.Context, registryURL, imageName, hookScript string) string {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "hooks/", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "hooks/post-restore", Size: int64(len(hookScript)), Mode: 0o755}))
	_, err := tw.Write([]byte(hookScript))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	})
	require.NoError(t, err)

	img, err := mutate.AppendLayers(baseImage(t, ctx), layer)
	require.NoError(t, err)

	fullTag := registryURL + "/" + imageName + ":latest"
	pushToRegistry(t, ctx, fullTag, img)
	return fullTag
}

func baseImage(t *testing.T, ctx context.Context) v1.Image {
	t.Helper()

	ref, err := name.ParseReference(integrationAppImageRef)
	require.NoError(t, err)
	img, err := remote.Image(ref, remote.WithContext(ctx))
	require.NoError(t, err)
	return img
}

func pushToRegistry(t *testing.T, ctx context.Context, tag string, img v1.Image, opts ...remote.Option) {
	t.Helper()

	ref, err := name.ParseReference(tag)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img, append([]remote.Option{remote.WithContext(ctx)}, opts...)...))
}

func buildTestBackup(t *testing.T, imageName string) []byte {
	t.Helper()

	appSettings := docker.ApplicationSettings{
		Name:  "hookapp",
		Image: imageName,
		Host:  "hookapp.localhost",
	}
	volSettings := docker.ApplicationLegacyVolumeSettings{Keys: docker.Keys{SecretKeyBase: "test-secret-key"}}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	writeEntry := func(name string, data []byte) {
		header := &tar.Header{Name: name, Size: int64(len(data)), Mode: 0o644}
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
		Mode:     0o755,
		Uid:      1000,
		Gid:      1000,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "data/hook-input",
		Mode: 0o644,
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

func buildAndPushImage(t *testing.T, ctx context.Context, tag, version string, opts ...remote.Option) {
	t.Helper()

	base := baseImage(t, ctx)
	cfg, err := base.ConfigFile()
	require.NoError(t, err)

	cfg = cfg.DeepCopy()
	if cfg.Config.Labels == nil {
		cfg.Config.Labels = map[string]string{}
	}
	cfg.Config.Labels["version"] = version

	img, err := mutate.ConfigFile(base, cfg)
	require.NoError(t, err)

	pushToRegistry(t, ctx, tag, img, opts...)
}
