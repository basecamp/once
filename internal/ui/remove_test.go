package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemove_ViewShowsConfirmation(t *testing.T) {
	m := newTestRemove()
	m, _ = updateRemove(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	view := m.View()
	assert.Contains(t, view, "Remove application and data?")
	assert.Contains(t, view, "Remove")
	assert.Contains(t, view, "Cancel")
}

func TestRemove_CancelNavigatesToDashboard(t *testing.T) {
	m := newTestRemove()

	_, cmd := updateRemove(m, ConfirmationCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Equal(t, "test-app", navMsg.AppName)
}

func TestRemove_EscNavigatesToDashboard(t *testing.T) {
	m := newTestRemove()

	_, cmd := updateRemove(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Equal(t, "test-app", navMsg.AppName)
}

func TestRemove_ConfirmStartsRemoval(t *testing.T) {
	m := newTestRemove()
	m, _ = updateRemove(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = updateRemove(m, ConfirmationConfirmMsg{})
	assert.True(t, m.removing)
}

func TestRemove_SuccessNavigatesToDashboard(t *testing.T) {
	m := newTestRemove()

	_, cmd := updateRemove(m, removeFinishedMsg{err: nil})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Empty(t, navMsg.AppName)
	assert.True(t, navMsg.AllowEmpty)
}

func TestRemove_ErrorShowsErrorAndReturnsToConfirmation(t *testing.T) {
	m := newTestRemove()
	m, _ = updateRemove(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.removing = true

	m, _ = updateRemove(m, removeFinishedMsg{err: errors.New("permission denied")})

	assert.False(t, m.removing)
	assert.Error(t, m.err)
	assert.Contains(t, m.View(), "permission denied")
}

// Helpers

func newTestRemove() Remove {
	app := &docker.Application{
		Settings: docker.ApplicationSettings{
			Name: "test-app",
			Host: "test-app.example.com",
		},
	}
	return NewRemove(nil, app)
}

func updateRemove(m Remove, msg tea.Msg) (Remove, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(Remove), cmd
}
