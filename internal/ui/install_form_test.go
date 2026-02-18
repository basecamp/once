package ui

import (
	"testing"

	"github.com/basecamp/gliff/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallForm_FillAndSubmit(t *testing.T) {
	form := NewInstallForm("")

	installTypeText(form, "nginx:latest")
	assert.Equal(t, "nginx:latest", form.ImageRef())

	installPressEnter(form)
	assert.Equal(t, 1, form.form.Focused())

	installTypeText(form, "myapp.example.com")
	assert.Equal(t, "myapp.example.com", form.Hostname())

	installPressEnter(form)
	assert.Equal(t, 2, form.form.Focused(), "submit button")

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)

	msg := cmd()
	submitMsg, ok := msg.(InstallFormSubmitMsg)
	require.True(t, ok, "expected InstallFormSubmitMsg, got %T", msg)
	assert.Equal(t, "nginx:latest", submitMsg.ImageRef)
	assert.Equal(t, "myapp.example.com", submitMsg.Hostname)
}

func TestInstallForm_TabNavigation(t *testing.T) {
	form := NewInstallForm("")
	assert.Equal(t, 0, form.form.Focused())

	installPressTab(form)
	assert.Equal(t, 1, form.form.Focused())

	installPressTab(form)
	assert.Equal(t, 2, form.form.Focused(), "submit button")

	installPressTab(form)
	assert.Equal(t, 3, form.form.Focused(), "cancel button")

	installPressTab(form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to first field")
}

func TestInstallForm_ShiftTabNavigation(t *testing.T) {
	form := NewInstallForm("")

	installPressShiftTab(form)
	assert.Equal(t, 3, form.form.Focused(), "cancel button")

	installPressShiftTab(form)
	assert.Equal(t, 2, form.form.Focused(), "submit button")
}

func TestInstallForm_CancelButton(t *testing.T) {
	form := NewInstallForm("")

	for range 3 {
		installPressTab(form)
	}
	assert.Equal(t, 3, form.form.Focused(), "cancel button")

	cmd := form.Update(keyMsg(tui.KeyEnter, 0))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(InstallFormCancelMsg)
	assert.True(t, ok, "expected InstallFormCancelMsg, got %T", msg)
}

// Helpers

func installTypeText(form *InstallForm, text string) {
	for _, r := range text {
		form.Update(runeMsg(r))
	}
}

func installPressEnter(form *InstallForm) {
	form.Update(keyMsg(tui.KeyEnter, 0))
}

func installPressTab(form *InstallForm) {
	form.Update(keyMsg(tui.KeyTab, 0))
}

func installPressShiftTab(form *InstallForm) {
	form.Update(keyMsg(tui.KeyShiftTab, 0))
}
