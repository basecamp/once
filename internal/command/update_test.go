package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestApplyChanges(t *testing.T) {
	existing := docker.ApplicationSettings{
		Name:       "myapp",
		Image:      "myimage:latest",
		Host:       "app.example.com",
		DisableTLS: false,
		EnvVars:    map[string]string{"KEY": "value"},
		SMTP: docker.SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user",
			Password: "pass",
			From:     "noreply@example.com",
		},
		Resources: docker.ContainerResources{
			CPUs:     2,
			MemoryMB: 1024,
		},
		AutoUpdate: true,
		Backup: docker.BackupSettings{
			Path:       "/backups",
			AutoBackup: true,
		},
	}

	newCmd := func(changed ...string) *cobra.Command {
		cmd := &cobra.Command{}
		f := &settingsFlags{}
		f.register(cmd)
		for _, name := range changed {
			require.NoError(t, cmd.Flags().Set(name, cmd.Flags().Lookup(name).DefValue))
		}
		return cmd
	}

	t.Run("no flags changed returns existing", func(t *testing.T) {
		f := &settingsFlags{}
		cmd := newCmd()
		result, err := f.applyChanges(cmd, existing)
		require.NoError(t, err)
		assert.True(t, existing.Equal(result))
	})

	t.Run("single flag changed", func(t *testing.T) {
		f := &settingsFlags{memory: 2048}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("memory", "2048"))

		result, err := f.applyChanges(cmd, existing)
		require.NoError(t, err)
		assert.Equal(t, 2048, result.Resources.MemoryMB)
		assert.Equal(t, existing.Resources.CPUs, result.Resources.CPUs)
		assert.Equal(t, existing.Host, result.Host)
		assert.Equal(t, existing.EnvVars, result.EnvVars)
	})

	t.Run("multiple flags changed", func(t *testing.T) {
		f := &settingsFlags{host: "new.example.com", cpus: 4, autoBackup: false}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("host", "new.example.com"))
		require.NoError(t, cmd.Flags().Set("cpus", "4"))
		require.NoError(t, cmd.Flags().Set("auto-backup", "false"))

		result, err := f.applyChanges(cmd, existing)
		require.NoError(t, err)
		assert.Equal(t, "new.example.com", result.Host)
		assert.Equal(t, 4, result.Resources.CPUs)
		assert.Equal(t, false, result.Backup.AutoBackup)
		// Unchanged fields preserved
		assert.Equal(t, existing.SMTP, result.SMTP)
		assert.Equal(t, existing.EnvVars, result.EnvVars)
		assert.Equal(t, existing.Resources.MemoryMB, result.Resources.MemoryMB)
	})

	t.Run("env replaces all vars", func(t *testing.T) {
		f := &settingsFlags{env: []string{"NEW=val"}}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("env", "NEW=val"))

		result, err := f.applyChanges(cmd, existing)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"NEW": "val"}, result.EnvVars)
	})

	t.Run("invalid env returns error", func(t *testing.T) {
		f := &settingsFlags{env: []string{"INVALID"}}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("env", "INVALID"))

		_, err := f.applyChanges(cmd, existing)
		assert.ErrorContains(t, err, "must be in KEY=VALUE format")
	})

	t.Run("enable auto-backup with existing path", func(t *testing.T) {
		noAutoBackup := existing
		noAutoBackup.Backup.AutoBackup = false

		f := &settingsFlags{autoBackup: true}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("auto-backup", "true"))

		result, err := f.applyChanges(cmd, noAutoBackup)
		require.NoError(t, err)
		assert.True(t, result.Backup.AutoBackup)
	})

	t.Run("enable auto-backup without path", func(t *testing.T) {
		noPath := existing
		noPath.Backup.Path = ""
		noPath.Backup.AutoBackup = false

		f := &settingsFlags{autoBackup: true}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("auto-backup", "true"))

		_, err := f.applyChanges(cmd, noPath)
		assert.ErrorIs(t, err, docker.ErrAutoBackupWithoutPath)
	})

	t.Run("clear backup path with auto-backup enabled", func(t *testing.T) {
		f := &settingsFlags{backupPath: ""}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("backup-path", ""))

		_, err := f.applyChanges(cmd, existing)
		assert.ErrorIs(t, err, docker.ErrAutoBackupWithoutPath)
	})

	t.Run("set both auto-backup and path", func(t *testing.T) {
		noBackup := existing
		noBackup.Backup = docker.BackupSettings{}

		f := &settingsFlags{autoBackup: true, backupPath: "/backups"}
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("auto-backup", "true"))
		require.NoError(t, cmd.Flags().Set("backup-path", "/backups"))

		result, err := f.applyChanges(cmd, noBackup)
		require.NoError(t, err)
		assert.True(t, result.Backup.AutoBackup)
		assert.Equal(t, "/backups", result.Backup.Path)
	})
}
