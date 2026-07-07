package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/pkg/docker"
)

func TestSettings_InitialStateIsForm(t *testing.T) {
	s := testSettings()
	assert.Equal(t, settingsStateForm, s.state)
}

func TestSettings_EscNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	s, _ = updateSettings(s, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := updateSettings(s, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Equal(t, "test-app", navMsg.AppName)
}

func TestSettings_CancelNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	_, cmd := updateSettings(s, SettingsSectionCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_SubmitUnchangedNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	_, cmd := updateSettings(s, SettingsSectionSubmitMsg{Settings: s.app.Settings})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_SubmitChangedStartsDeploy(t *testing.T) {
	s := testSettings()
	s, _ = updateSettings(s, tea.WindowSizeMsg{Width: 80, Height: 24})

	changed := s.app.Settings
	changed.Host = "new.example.com"

	s, _ = updateSettings(s, SettingsSectionSubmitMsg{Settings: changed})
	assert.Equal(t, settingsStateDeploying, s.state)
}

func TestSettings_DeployFinishedNavigatesToApp(t *testing.T) {
	s := testSettings()
	s.state = settingsStateDeploying

	_, cmd := updateSettings(s, settingsDeployFinishedMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToAppMsg)
	assert.True(t, ok)
}

func TestSettings_DeployFinishedWithErrorStillNavigates(t *testing.T) {
	// Current behavior: deploy errors are not surfaced to the user;
	// the handler always navigates to the app screen.
	s := testSettings()
	s.state = settingsStateDeploying

	_, cmd := updateSettings(s, settingsDeployFinishedMsg{err: assert.AnError})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToAppMsg)
	assert.True(t, ok)
}

func TestSettings_ActionFinishedWithError(t *testing.T) {
	s := testSettings()
	s.state = settingsStateRunningAction

	s, _ = updateSettings(s, settingsActionFinishedMsg{err: assert.AnError})
	assert.Equal(t, settingsStateForm, s.state)
	assert.Error(t, s.err)
}

func TestSettings_ActionFinishedWithMessage(t *testing.T) {
	s := testSettings()
	s.state = settingsStateRunningAction

	s, _ = updateSettings(s, settingsActionFinishedMsg{message: "Success!"})
	assert.Equal(t, settingsStateActionComplete, s.state)
	assert.Equal(t, "Success!", s.actionSuccessMessage)
}

func TestSettings_ActionCompleteEnterNavigates(t *testing.T) {
	s := testSettings()
	s.state = settingsStateActionComplete

	_, cmd := updateSettings(s, keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_ActionCompleteDoneClick(t *testing.T) {
	s := testSettings()
	s.state = settingsStateActionComplete

	_, cmd := updateSettings(s, MouseEvent{IsClick: true, Target: "done"})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_ErrorClearsOnKeypress(t *testing.T) {
	s := testSettings()
	s.state = settingsStateForm
	s.err = assert.AnError

	s, _ = updateSettings(s, keyPressMsg("a"))
	assert.Nil(t, s.err)
}

func TestSettings_ViewShowsError(t *testing.T) {
	s := testSettings()
	s.state = settingsStateForm
	s.err = assert.AnError
	s.width = 80
	s.height = 24

	view := s.View()
	assert.Contains(t, view, assert.AnError.Error())
}

func TestSettings_ViewShowsProgressWhileDeploying(t *testing.T) {
	s := testSettings()
	s.state = settingsStateDeploying
	s.width = 80
	s.height = 24

	// Should not panic
	view := s.View()
	assert.NotEmpty(t, view)
}

// Helpers

func testSettings() Settings {
	ns, _ := docker.NewNamespace("test")
	app := &docker.Application{
		Running: true,
		Settings: docker.ApplicationSettings{
			Name:  "test-app",
			Host:  "app.example.com",
			Image: "ghcr.io/basecamp/once-campfire:latest",
		},
	}

	return NewSettings(ns, app, SettingsSectionApplication)
}

func updateSettings(s Settings, msg tea.Msg) (Settings, tea.Cmd) {
	comp, cmd := s.Update(msg)
	return comp.(Settings), cmd
}
