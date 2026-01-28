package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/amar/internal/docker"
)

func TestSettingsForm_InitialState_NonLocalhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "app.example.com",
		DisableTLS: false,
	}
	form := NewSettingsForm(settings)

	assert.Equal(t, settingsFieldImage, form.focused)
	assert.Equal(t, "nginx:latest", form.imageInput.Value())
	assert.Equal(t, "app.example.com", form.hostnameInput.Value())
	assert.True(t, form.tlsEnabled)
}

func TestSettingsForm_InitialState_Localhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "chat.localhost",
		DisableTLS: false,
	}
	form := NewSettingsForm(settings)

	assert.Equal(t, "chat.localhost", form.hostnameInput.Value())
	assert.False(t, form.tlsEnabled, "TLS should be disabled for localhost even when DisableTLS is false")
}

func TestSettingsForm_TabNavigation(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, settingsFieldImage, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldHostname, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSaveButton, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldCancelButton, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldImage, form.focused)
}

func TestSettingsForm_ShiftTabNavigation(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})

	form = settingsPressShiftTab(form)
	assert.Equal(t, settingsFieldCancelButton, form.focused)

	form = settingsPressShiftTab(form)
	assert.Equal(t, settingsFieldSaveButton, form.focused)
}

func TestSettingsForm_SpaceTogglesTLS(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.tlsEnabled)

	// Tab twice to get to TLS field (Image -> Hostname -> TLS)
	form = settingsPressTab(form)
	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressSpace(form)
	assert.False(t, form.tlsEnabled)

	form = settingsPressSpace(form)
	assert.True(t, form.tlsEnabled)
}

func TestSettingsForm_SpaceDoesNotToggleTLSForLocalhost(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "chat.localhost"})
	assert.False(t, form.tlsEnabled)

	// Tab twice to get to TLS field
	form = settingsPressTab(form)
	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressSpace(form)
	assert.False(t, form.tlsEnabled, "TLS should remain disabled for localhost")
}

func TestSettingsForm_ChangingHostnameToLocalhostDisablesTLS(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.tlsEnabled)

	// Tab to hostname field
	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldHostname, form.focused)

	form = settingsTypeText(form, ".localhost")
	assert.False(t, form.tlsEnabled, "TLS should be disabled after hostname becomes localhost")
}

func TestSettingsForm_Submit(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{
		Image: "nginx:latest",
		Host:  "app.example.com",
	})

	form.focused = settingsFieldSaveButton
	_, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	submitMsg, ok := msg.(SettingsFormSubmitMsg)
	require.True(t, ok, "expected SettingsFormSubmitMsg, got %T", msg)
	assert.Equal(t, "nginx:latest", submitMsg.Image)
	assert.Equal(t, "app.example.com", submitMsg.Hostname)
	assert.True(t, submitMsg.TLSEnabled)
}

func TestSettingsForm_Cancel(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})

	form.focused = settingsFieldCancelButton
	_, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(SettingsFormCancelMsg)
	assert.True(t, ok, "expected SettingsFormCancelMsg, got %T", msg)
}

// Helpers

func settingsTypeText(form SettingsForm, text string) SettingsForm {
	for _, r := range text {
		form, _ = form.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return form
}

func settingsPressTab(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return form
}

func settingsPressShiftTab(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	return form
}

func settingsPressSpace(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	return form
}
