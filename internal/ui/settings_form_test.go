package ui

import (
	"testing"

	"github.com/basecamp/gliff/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestSettingsFormApplication_InitialState_NonLocalhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "app.example.com",
		DisableTLS: false,
	}
	form := NewSettingsFormApplication(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "nginx:latest", form.form.TextField(appImageField).Value())
	assert.Equal(t, "app.example.com", form.form.TextField(appHostnameField).Value())
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())
}

func TestSettingsFormApplication_InitialState_Localhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "chat.localhost",
		DisableTLS: false,
	}
	form := NewSettingsFormApplication(settings)

	assert.Equal(t, "chat.localhost", form.form.TextField(appHostnameField).Value())
	assert.True(t, form.form.CheckboxField(appTLSField).Checked(), "checkbox is checked (DisableTLS=false)")
}

func TestSettingsFormApplication_TabNavigation(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, 0, form.form.Focused())

	applicationPressTab(form)
	assert.Equal(t, 1, form.form.Focused(), "hostname")

	applicationPressTab(form)
	assert.Equal(t, 2, form.form.Focused(), "tls")

	applicationPressTab(form)
	assert.Equal(t, 3, form.form.Focused(), "done button")

	applicationPressTab(form)
	assert.Equal(t, 4, form.form.Focused(), "cancel button")

	applicationPressTab(form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to image")
}

func TestSettingsFormApplication_ShiftTabNavigation(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})

	applicationPressShiftTab(form)
	assert.Equal(t, 4, form.form.Focused(), "cancel button")

	applicationPressShiftTab(form)
	assert.Equal(t, 3, form.form.Focused(), "done button")
}

func TestSettingsFormApplication_SpaceTogglesTLS(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())

	applicationPressTab(form)
	applicationPressTab(form)
	assert.Equal(t, 2, form.form.Focused())

	applicationPressSpace(form)
	assert.False(t, form.form.CheckboxField(appTLSField).Checked())

	applicationPressSpace(form)
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())
}

func TestSettingsFormApplication_SpaceDoesNotToggleTLSForLocalhost(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "chat.localhost"})

	applicationPressTab(form)
	applicationPressTab(form)
	assert.Equal(t, 2, form.form.Focused())

	applicationPressSpace(form)
	assert.True(t, form.form.CheckboxField(appTLSField).Checked(), "toggle ignored for localhost")
}

func TestSettingsFormApplication_TLSShowsDisabledForLocalhost(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, "[✓] Enabled", form.form.CheckboxField(appTLSField).Render())

	applicationPressTab(form)
	applicationTypeText(form, ".localhost")
	assert.Equal(t, "Not available for localhost", form.form.CheckboxField(appTLSField).Render())

	applicationClearAndType(form, "app.example.com")
	assert.Equal(t, "[✓] Enabled", form.form.CheckboxField(appTLSField).Render())
}

func TestSettingsFormApplication_Submit(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{
		Name:  "myapp",
		Image: "nginx:latest",
		Host:  "app.example.com",
	})

	for range 3 {
		applicationPressTab(form)
	}

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, "nginx:latest", submitMsg.Settings.Image)
	assert.Equal(t, "app.example.com", submitMsg.Settings.Host)
	assert.False(t, submitMsg.Settings.DisableTLS)
}

func TestSettingsFormApplication_Cancel(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})

	for range 4 {
		applicationPressTab(form)
	}

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormEmail_InitialState(t *testing.T) {
	settings := docker.ApplicationSettings{
		SMTP: docker.SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}
	form := NewSettingsFormEmail(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "smtp.example.com", form.form.TextField(emailServerField).Value())
	assert.Equal(t, "587", form.form.TextField(emailPortField).Value())
	assert.Equal(t, "user@example.com", form.form.TextField(emailUsernameField).Value())
	assert.Equal(t, "secret", form.form.TextField(emailPasswordField).Value())
	assert.Equal(t, "noreply@example.com", form.form.TextField(emailFromField).Value())
}

func TestSettingsFormEmail_TabNavigation(t *testing.T) {
	form := NewSettingsFormEmail(docker.ApplicationSettings{})
	assert.Equal(t, 0, form.form.Focused())

	emailPressTab(form)
	assert.Equal(t, 1, form.form.Focused(), "port")

	emailPressTab(form)
	assert.Equal(t, 2, form.form.Focused(), "username")

	emailPressTab(form)
	assert.Equal(t, 3, form.form.Focused(), "password")

	emailPressTab(form)
	assert.Equal(t, 4, form.form.Focused(), "from")

	emailPressTab(form)
	assert.Equal(t, 5, form.form.Focused(), "done button")

	emailPressTab(form)
	assert.Equal(t, 6, form.form.Focused(), "cancel button")

	emailPressTab(form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to server")
}

func TestSettingsFormEmail_Submit(t *testing.T) {
	settings := docker.ApplicationSettings{
		Name: "myapp",
		SMTP: docker.SMTPSettings{
			Server: "smtp.example.com",
			Port:   "587",
		},
	}
	form := NewSettingsFormEmail(settings)

	for range 5 {
		emailPressTab(form)
	}

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, "smtp.example.com", submitMsg.Settings.SMTP.Server)
	assert.Equal(t, "587", submitMsg.Settings.SMTP.Port)
}

func TestSettingsFormEmail_Cancel(t *testing.T) {
	form := NewSettingsFormEmail(docker.ApplicationSettings{})

	for range 6 {
		emailPressTab(form)
	}

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormResources_InitialState(t *testing.T) {
	settings := docker.ApplicationSettings{
		Resources: docker.ContainerResources{
			CPUs:     2,
			MemoryMB: 512,
		},
	}
	form := NewSettingsFormResources(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "2", form.form.TextField(resourcesCPUField).Value())
	assert.Equal(t, "512", form.form.TextField(resourcesMemoryField).Value())
}

func TestSettingsFormResources_InitialState_ZeroValues(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "", form.form.TextField(resourcesCPUField).Value())
	assert.Equal(t, "", form.form.TextField(resourcesMemoryField).Value())
}

func TestSettingsFormResources_TabNavigation(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})
	assert.Equal(t, 0, form.form.Focused())

	resourcesPressTab(form)
	assert.Equal(t, 1, form.form.Focused(), "memory")

	resourcesPressTab(form)
	assert.Equal(t, 2, form.form.Focused(), "done button")

	resourcesPressTab(form)
	assert.Equal(t, 3, form.form.Focused(), "cancel button")

	resourcesPressTab(form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to cpu")
}

func TestSettingsFormResources_Submit(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{Name: "myapp"})

	resourcesTypeText(form, "2")
	resourcesPressTab(form)
	resourcesTypeText(form, "1024")
	resourcesPressTab(form)

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, 2, submitMsg.Settings.Resources.CPUs)
	assert.Equal(t, 1024, submitMsg.Settings.Resources.MemoryMB)
}

func TestSettingsFormResources_SubmitBlank(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	resourcesPressTab(form)
	resourcesPressTab(form)

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, 0, submitMsg.Settings.Resources.CPUs)
	assert.Equal(t, 0, submitMsg.Settings.Resources.MemoryMB)
}

func TestSettingsFormResources_Cancel(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	for range 3 {
		resourcesPressTab(form)
	}

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormBackups_ActionReadsCurrentFieldValue(t *testing.T) {
	app := &docker.Application{
		Settings: docker.ApplicationSettings{
			Name:   "chat",
			Backup: docker.BackupSettings{Path: "/old/path"},
		},
	}
	form := NewSettingsFormBackups(app, nil)

	assert.Equal(t, "/old/path", form.form.TextField(backupsPathField).Value())

	// Type a new path into the field
	form.form.TextField(backupsPathField).SetValue("/new/path")

	// Tab to Done, then to the action button, then press enter
	backupsPressTab(form)
	backupsPressTab(form)
	backupsPressTab(form)
	assert.Equal(t, form.form.actionIndex(), form.form.Focused(), "action button focused")

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	actionMsg, ok := msg.(settingsRunActionMsg)
	require.True(t, ok, "expected settingsRunActionMsg, got %T", msg)

	// Run the action — it will fail (no Docker) but should use the new path
	_, err := actionMsg.action()
	require.Error(t, err)
	// The error should NOT be "backup location is required", proving it read "/new/path"
	assert.NotContains(t, err.Error(), "backup location is required")
}

func TestSettingsFormBackups_Submit(t *testing.T) {
	app := &docker.Application{
		Settings: docker.ApplicationSettings{Name: "chat"},
	}
	form := NewSettingsFormBackups(app, nil)

	// Type a path
	backupsTypeText(form, "/my/backups")
	backupsPressTab(form)

	// Toggle auto-backup
	backupsPressSpace(form)

	// Tab to Done
	backupsPressTab(form)

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "/my/backups", submitMsg.Settings.Backup.Path)
	assert.True(t, submitMsg.Settings.Backup.AutoBack)
}

// Helpers

func applicationTypeText(form *SettingsFormApplication, text string) {
	for _, r := range text {
		form.Update(runeMsg(r))
	}
}

func applicationClearAndType(form *SettingsFormApplication, text string) {
	form.form.TextField(appHostnameField).SetValue("")
	applicationTypeText(form, text)
}

func applicationPressTab(form *SettingsFormApplication) {
	form.Update(keyMsg(tui.KeyTab, 0))
}

func applicationPressShiftTab(form *SettingsFormApplication) {
	form.Update(keyMsg(tui.KeyShiftTab, 0))
}

func applicationPressSpace(form *SettingsFormApplication) {
	form.Update(runeMsg(' '))
}

func emailPressTab(form *SettingsFormEmail) {
	form.Update(keyMsg(tui.KeyTab, 0))
}

func resourcesPressTab(form *SettingsFormResources) {
	form.Update(keyMsg(tui.KeyTab, 0))
}

func resourcesTypeText(form *SettingsFormResources, text string) {
	for _, r := range text {
		form.Update(runeMsg(r))
	}
}

func backupsPressTab(form *SettingsFormBackups) {
	form.Update(keyMsg(tui.KeyTab, 0))
}

func backupsPressSpace(form *SettingsFormBackups) {
	form.Update(runeMsg(' '))
}

func backupsTypeText(form *SettingsFormBackups, text string) {
	for _, r := range text {
		form.Update(runeMsg(r))
	}
}
