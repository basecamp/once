package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestParseEnvVars(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		f := &settingsFlags{}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("valid pairs", func(t *testing.T) {
		f := &settingsFlags{env: []string{"FOO=bar", "BAZ=qux"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"FOO": "bar", "BAZ": "qux"}, result)
	})

	t.Run("value containing equals", func(t *testing.T) {
		f := &settingsFlags{env: []string{"DSN=postgres://host?opt=val"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "postgres://host?opt=val", result["DSN"])
	})

	t.Run("missing equals", func(t *testing.T) {
		f := &settingsFlags{env: []string{"INVALID"}}
		_, err := f.parseEnvVars()
		assert.ErrorContains(t, err, "must be in KEY=VALUE format")
	})

	t.Run("empty key", func(t *testing.T) {
		f := &settingsFlags{env: []string{"=value"}}
		_, err := f.parseEnvVars()
		assert.ErrorContains(t, err, "key must not be empty")
	})

	t.Run("empty value is valid", func(t *testing.T) {
		f := &settingsFlags{env: []string{"KEY="}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "", result["KEY"])
	})

	t.Run("duplicate keys last wins", func(t *testing.T) {
		f := &settingsFlags{env: []string{"KEY=first", "KEY=second"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "second", result["KEY"])
	})
}

func TestBuildSettingsImageRequired(t *testing.T) {
	f := &settingsFlags{}
	_, err := f.buildSettings(&cobra.Command{}, "", "app.example.com")
	assert.ErrorIs(t, err, docker.ErrImageRequired)
}

func TestBuildSettingsRegistryCredentials(t *testing.T) {
	f := &settingsFlags{registryUsername: "user", registryPassword: "pass"}
	s, err := f.buildSettings(&cobra.Command{}, "image:latest", "app.example.com")
	require.NoError(t, err)
	assert.Equal(t, docker.RegistrySettings{Username: "user", Password: "pass"}, s.Registry)
}

func TestBuildSettingsRegistryPasswordFromStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("stdin-pass\n"))

	f := &settingsFlags{registryUsername: "user", registryPasswordStdin: true}
	s, err := f.buildSettings(cmd, "image:latest", "app.example.com")
	require.NoError(t, err)
	assert.Equal(t, docker.RegistrySettings{Username: "user", Password: "stdin-pass"}, s.Registry)
}

func TestRegistryPasswordFlagsAreMutuallyExclusive(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)
	require.NoError(t, cmd.Flags().Set("registry-password", "pass"))
	require.NoError(t, cmd.Flags().Set("registry-password-stdin", "true"))

	assert.Error(t, cmd.ValidateFlagGroups())
}

func TestBuildSettingsAutoBackupRequiresPath(t *testing.T) {
	t.Run("auto-backup without path", func(t *testing.T) {
		f := &settingsFlags{autoBackup: true}
		_, err := f.buildSettings(&cobra.Command{}, "image:latest", "app.example.com")
		assert.ErrorIs(t, err, docker.ErrAutoBackupWithoutPath)
	})

	t.Run("auto-backup with path", func(t *testing.T) {
		f := &settingsFlags{autoBackup: true, backupPath: "/backups"}
		_, err := f.buildSettings(&cobra.Command{}, "image:latest", "app.example.com")
		require.NoError(t, err)
	})
}
