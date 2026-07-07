package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/pkg/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionsMenu_ShowsStopWhenRunning(t *testing.T) {
	app := &docker.Application{Running: true}
	m := NewActionsMenu(app)

	assert.Contains(t, m.View(), "Stop")
}

func TestActionsMenu_ShowsStartWhenStopped(t *testing.T) {
	app := &docker.Application{Running: false}
	m := NewActionsMenu(app)

	assert.Contains(t, m.View(), "Start")
}

func TestActionsMenu_SelectStartStop(t *testing.T) {
	app := &docker.Application{Running: true}
	m := NewActionsMenu(app)

	// Shortcut key goes to menu, which returns MenuSelectMsg
	m, cmd := updateActionsMenu(m, keyPressMsg("s"))
	require.NotNil(t, cmd)
	msg := cmd()

	// Feed MenuSelectMsg back to get ActionsMenuSelectMsg
	_, cmd = updateActionsMenu(m, msg)
	require.NotNil(t, cmd)
	msg = cmd()

	selectMsg, ok := msg.(ActionsMenuSelectMsg)
	require.True(t, ok, "expected ActionsMenuSelectMsg, got %T", msg)
	assert.Equal(t, ActionsMenuStartStop, selectMsg.action)
	assert.Equal(t, app, selectMsg.app)
}

func TestActionsMenu_SelectRemove(t *testing.T) {
	app := &docker.Application{}
	m := NewActionsMenu(app)

	m, cmd := updateActionsMenu(m, keyPressMsg("r"))
	require.NotNil(t, cmd)
	msg := cmd()

	_, cmd = updateActionsMenu(m, msg)
	require.NotNil(t, cmd)
	msg = cmd()

	selectMsg, ok := msg.(ActionsMenuSelectMsg)
	require.True(t, ok, "expected ActionsMenuSelectMsg, got %T", msg)
	assert.Equal(t, ActionsMenuRemove, selectMsg.action)
}

func TestActionsMenu_EscCloses(t *testing.T) {
	app := &docker.Application{}
	m := NewActionsMenu(app)

	_, cmd := updateActionsMenu(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ActionsMenuCloseMsg)
	assert.True(t, ok, "expected ActionsMenuCloseMsg, got %T", msg)
}

func TestActionsMenu_KeyboardNavigation(t *testing.T) {
	app := &docker.Application{Running: true}
	m := NewActionsMenu(app)

	// Navigate down to Remove
	m, _ = updateActionsMenu(m, keyPressMsg("j"))
	assert.Equal(t, 1, m.menu.selected)

	// Navigate back up to Start/Stop
	m, _ = updateActionsMenu(m, keyPressMsg("k"))
	assert.Equal(t, 0, m.menu.selected)

	// Navigate down and select with enter
	m, _ = updateActionsMenu(m, keyPressMsg("j"))
	m, cmd := updateActionsMenu(m, keyPressMsg("enter"))
	require.NotNil(t, cmd)
	msg := cmd()

	_, cmd = updateActionsMenu(m, msg)
	require.NotNil(t, cmd)
	msg = cmd()

	selectMsg, ok := msg.(ActionsMenuSelectMsg)
	require.True(t, ok, "expected ActionsMenuSelectMsg, got %T", msg)
	assert.Equal(t, ActionsMenuRemove, selectMsg.action)
}

func TestActionsMenu_TitleCentered(t *testing.T) {
	app := &docker.Application{Running: true}
	m := NewActionsMenu(app)
	view := ansi.Strip(m.View())

	lines := strings.Split(view, "\n")

	titleOffset := -1
	menuOffset := -1
	for _, line := range lines {
		if i := strings.Index(line, "Actions"); i >= 0 {
			titleOffset = i
		}
		if i := strings.Index(line, "Stop"); i >= 0 && !strings.Contains(line, "Actions") {
			menuOffset = i
		}
	}

	require.GreaterOrEqual(t, titleOffset, 0)
	require.GreaterOrEqual(t, menuOffset, 0)
	assert.Greater(t, titleOffset, menuOffset)
}

// Helpers

func updateActionsMenu(m ActionsMenu, msg tea.Msg) (ActionsMenu, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(ActionsMenu), cmd
}
