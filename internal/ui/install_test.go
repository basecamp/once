package ui

import (
	"errors"
	"testing"

	"github.com/basecamp/gliff/tui"
	"github.com/basecamp/once/internal/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_SubmitTriggersActivity(t *testing.T) {
	m := newTestInstall()
	assert.Equal(t, installStateForm, m.state)

	m.Update(tui.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_SuccessNavigatesToApp(t *testing.T) {
	m := newTestInstall()
	app := &docker.Application{}

	cmd := m.Update(InstallActivityDoneMsg{App: app})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(navigateToAppMsg)
	require.True(t, ok, "expected navigateToAppMsg, got %T", msg)
	assert.Equal(t, app, navMsg.app)
}

func TestInstall_FailureReturnsToFormWithError(t *testing.T) {
	m := newTestInstall()

	// Fill the form fields before submitting
	fillInstallForm(m.form, "nginx:latest", "app.example.com")

	// Submit to enter activity state
	m.Update(tui.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(InstallFormSubmitMsg{ImageRef: "nginx:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	// Simulate failure
	installErr := errors.New("connection refused")
	cmd := m.Update(InstallActivityFailedMsg{Err: installErr})

	assert.Nil(t, cmd)
	assert.Equal(t, installStateForm, m.state)
	assert.Equal(t, installErr, m.err)
	assert.Contains(t, m.Render(), "Error: connection refused")

	// Form field values are preserved
	assert.Equal(t, "nginx:latest", m.form.ImageRef())
	assert.Equal(t, "app.example.com", m.form.Hostname())
}

func TestInstall_ErrorClearsOnKeypress(t *testing.T) {
	m := newTestInstall()
	m.err = errors.New("some error")

	m.Update(runeMsg('a'))
	assert.Nil(t, m.err)
}

func TestInstall_EscNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	cmd := m.Update(keyMsg(tui.KeyEscape, 0))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(navigateToDashboardMsg)
	assert.True(t, ok, "expected navigateToDashboardMsg, got %T", msg)
}

func TestInstall_CancelNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	cmd := m.Update(InstallFormCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(navigateToDashboardMsg)
	assert.True(t, ok, "expected navigateToDashboardMsg, got %T", msg)
}

func TestInstall_EscQuitsWhenImageRefSet(t *testing.T) {
	m := NewInstall(nil, "nginx:latest")

	cmd := m.Update(keyMsg(tui.KeyEscape, 0))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(quitMsg)
	assert.True(t, ok, "expected quitMsg, got %T", msg)
}

func TestInstall_CancelQuitsWhenImageRefSet(t *testing.T) {
	m := NewInstall(nil, "nginx:latest")

	cmd := m.Update(InstallFormCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(quitMsg)
	assert.True(t, ok, "expected quitMsg, got %T", msg)
}

// Helpers

func newTestInstall() *Install {
	return NewInstall(nil, "")
}

func fillInstallForm(form *InstallForm, imageRef, hostname string) {
	installTypeText(form, imageRef)
	installPressEnter(form)
	installTypeText(form, hostname)
}
