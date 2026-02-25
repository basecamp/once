package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func newTestLogs() *Logs {
	vp := viewport.New()
	vp.MouseWheelEnabled = false
	vp.KeyMap = viewport.KeyMap{}
	vp.SoftWrap = true

	fi := textinput.New()
	fi.Placeholder = "Filter logs"
	fi.CharLimit = 256

	return &Logs{
		app: &docker.Application{
			Settings: docker.ApplicationSettings{Name: "testapp"},
		},
		streamer:      newTestLogStreamerForUI(),
		viewport:      vp,
		filterInput:   fi,
		filterEnabled: true,
		help:          NewHelp(),
	}
}

func newTestLogStreamerForUI() *docker.LogStreamer {
	return docker.NewLogStreamer(nil, docker.LogStreamerSettings{})
}

func TestLogsFilterActivation(t *testing.T) {
	m := newTestLogs()
	m.filterActive = false

	m.Update(runeKeyMsg('/'))

	assert.True(t, m.filterActive)
}

func TestLogsFilterAppliesOnKeypress(t *testing.T) {
	m := newTestLogs()
	m.filterActive = true
	m.filterInput.SetValue("err")

	m.Update(runeKeyMsg('o'))

	assert.Equal(t, m.filterInput.Value(), m.filterText)
}

func TestLogsFilterClearedOnEscape(t *testing.T) {
	m := newTestLogs()
	m.filterActive = true
	m.filterText = "error"
	m.filterInput.SetValue("error")

	m.Update(keyPressMsg("esc"))

	assert.False(t, m.filterActive)
	assert.Equal(t, "", m.filterText)
	assert.Equal(t, "", m.filterInput.Value())
}

func TestLogsBackNavigation(t *testing.T) {
	m := newTestLogs()
	m.filterActive = false

	cmd := m.Update(keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestLogsFilterMatchesCaseInsensitive(t *testing.T) {
	m := newTestLogs()
	m.filterText = "ERROR"

	lines := []docker.LogLine{
		{Content: "This is an error message"},
		{Content: "This is INFO"},
		{Content: "Another ERROR here"},
	}

	var filtered []string
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line.Content), strings.ToLower(m.filterText)) {
			filtered = append(filtered, line.Content)
		}
	}

	assert.Len(t, filtered, 2)
	assert.Equal(t, "This is an error message", filtered[0])
	assert.Equal(t, "Another ERROR here", filtered[1])
}
