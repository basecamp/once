package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallImageForm_SubmitWithAlias(t *testing.T) {
	form := NewInstallImageForm()

	imageFormTypeText(&form, "campfire")
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(InstallImageSubmitMsg)
	require.True(t, ok, "expected InstallImageSubmitMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", submit.ImageRef)
}

func TestInstallImageForm_SubmitWithCustomImage(t *testing.T) {
	form := NewInstallImageForm()

	imageFormTypeText(&form, "ghcr.io/basecamp/once-campfire:latest")
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(InstallImageSubmitMsg)
	require.True(t, ok, "expected InstallImageSubmitMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire:latest", submit.ImageRef)
}

func TestInstallImageForm_Cancel(t *testing.T) {
	form := NewInstallImageForm()

	// Tab to submit, tab to cancel
	imageFormPressTab(&form)
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(InstallImageBackMsg)
	assert.True(t, ok, "expected InstallImageBackMsg, got %T", msg)
}

func TestInstallImageForm_SubmitWithoutCredentialFields(t *testing.T) {
	form := NewInstallImageForm()

	imageFormTypeText(&form, "ghcr.io/acme/private")
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	submit := cmd().(InstallImageSubmitMsg)
	assert.Empty(t, submit.Username)
	assert.Empty(t, submit.Password)
}

func TestInstallImageForm_SubmitWithCredentials(t *testing.T) {
	form := NewInstallImageFormWithCredentials("ghcr.io/acme/private", "olduser")
	assert.Equal(t, "olduser", form.form.TextField(1).Value())

	form.form.TextField(1).SetValue("")
	imageFormPressTab(&form)
	imageFormTypeText(&form, "myuser")
	imageFormPressTab(&form)
	imageFormTypeText(&form, "mypass")
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(InstallImageSubmitMsg)
	require.True(t, ok, "expected InstallImageSubmitMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/acme/private", submit.ImageRef)
	assert.Equal(t, "myuser", submit.Username)
	assert.Equal(t, "mypass", submit.Password)
}

func TestInstallImageForm_CredentialsAreOptional(t *testing.T) {
	form := NewInstallImageFormWithCredentials("ghcr.io/acme/private", "")

	imageFormPressTab(&form)
	imageFormPressTab(&form)
	imageFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	submit, ok := cmd().(InstallImageSubmitMsg)
	require.True(t, ok, "expected InstallImageSubmitMsg")
	assert.Equal(t, "ghcr.io/acme/private", submit.ImageRef)
	assert.Empty(t, submit.Username)
	assert.Empty(t, submit.Password)
}

func TestInstallImageForm_RequiresImage(t *testing.T) {
	form := NewInstallImageForm()

	// Tab to submit button, then press enter with empty field
	imageFormPressTab(&form)
	form, _ = form.Update(keyPressMsg("enter"))
	assert.True(t, form.form.HasError())
}

// Helpers

func imageFormTypeText(form *InstallImageForm, text string) {
	for _, r := range text {
		*form, _ = form.Update(keyPressMsg(string(r)))
	}
}

func imageFormPressTab(form *InstallImageForm) {
	*form, _ = form.Update(keyPressMsg("tab"))
}
