package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

var logsKeys = struct {
	Filter key.Binding
	Back   key.Binding
}{
	Filter: WithHelp(NewKeyBinding("/"), "/", "filter"),
	Back:   WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type Logs struct {
	namespace     *docker.Namespace
	app           *docker.Application
	streamer      *docker.LogStreamer
	viewport      viewport.Model
	filterInput   textinput.Model
	filterActive  bool
	filterText    string
	filterEnabled bool
	width, height int
	help          Help

	lastVersion    uint64
	lastFilterText string
	wasAtBottom    bool
}

type logsTickMsg struct{}

func NewLogs(ns *docker.Namespace, app *docker.Application) Logs {
	streamer := docker.NewLogStreamer(ns, docker.LogStreamerSettings{})

	filterInput := textinput.New()
	filterInput.Prompt = ""
	filterInput.Placeholder = "Filter logs"
	filterInput.CharLimit = 256

	vp := viewport.New()
	vp.MouseWheelEnabled = false
	vp.KeyMap = viewport.KeyMap{}
	vp.SoftWrap = true

	h := NewHelp()
	h.SetBindings(logsHelpBindings(true))
	return Logs{
		namespace:     ns,
		app:           app,
		streamer:      streamer,
		viewport:      vp,
		filterInput:   filterInput,
		filterEnabled: true,
		help:          h,
		wasAtBottom:   true,
	}
}

func (m Logs) Init() tea.Cmd {
	containerName, err := m.app.ContainerName(context.Background())
	if err == nil {
		m.streamer.Start(context.Background(), containerName)
	}
	return m.scheduleNextLogsTick()
}

func (m Logs) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.updateViewportSize()
		m.rebuildContent()

	case MouseEvent:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		if cmd != nil {
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.filterActive {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)

	case logsTickMsg:
		m.checkForUpdates()
		cmds = append(cmds, m.scheduleNextLogsTick())

	case namespaceChangedMsg:
		containerName, err := m.app.ContainerName(context.Background())
		if err == nil {
			m.streamer.Start(context.Background(), containerName)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Logs) View() string {
	titleLine := Styles.TitleRule(m.width, m.app.Settings.Host, "logs")

	helpLine := Styles.CenteredLine(m.width, m.help.View())

	header := titleLine + "\n"
	if m.filterActive || m.filterText != "" {
		filterWidth := m.width - 2
		m.filterInput.SetWidth(filterWidth)
		header += " " + m.filterInput.View() + "\n"
	} else {
		header += "\n"
	}

	headerHeight := lipgloss.Height(header)
	helpHeight := lipgloss.Height(helpLine)
	viewportHeight := m.height - headerHeight - helpHeight

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}

	return header + m.viewport.View() + "\n" + helpLine
}

// Private

func (m Logs) scheduleNextLogsTick() tea.Cmd {
	return tea.Every(100*time.Millisecond, func(time.Time) tea.Msg { return logsTickMsg{} })
}

func (m Logs) handleFilterKey(msg tea.KeyPressMsg) (Component, tea.Cmd) {
	if key.Matches(msg, logsKeys.Back) {
		m.filterActive = false
		m.filterEnabled = true
		m.filterText = ""
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.help.SetBindings(logsHelpBindings(m.filterEnabled))
		m.rebuildContent()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.rebuildContent()
	return m, cmd
}

func (m Logs) handleNormalKey(msg tea.KeyPressMsg) (Component, tea.Cmd) {
	switch {
	case key.Matches(msg, logsKeys.Back):
		m.streamer.Stop()
		return m, func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }

	case key.Matches(msg, logsKeys.Filter) && m.filterEnabled:
		m.filterActive = true
		m.filterEnabled = false
		m.help.SetBindings(logsHelpBindings(m.filterEnabled))
		return m, m.filterInput.Focus()
	}

	m.wasAtBottom = m.viewport.AtBottom()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Logs) checkForUpdates() {
	version := m.streamer.Version()
	if version != m.lastVersion || m.filterText != m.lastFilterText {
		m.rebuildContent()
	}
}

func (m *Logs) rebuildContent() {
	m.wasAtBottom = m.viewport.AtBottom() || m.lastVersion == 0

	lines := m.streamer.Fetch(docker.DefaultLogBufferSize)
	m.lastVersion = m.streamer.Version()
	m.lastFilterText = m.filterText

	if len(lines) == 0 {
		if !m.streamer.Ready() {
			return
		}
		if m.filterText != "" {
			m.viewport.SetContent(m.centeredMessage("No logs match the filter"))
		} else {
			m.viewport.SetContent(m.centeredMessage("No logs yet..."))
		}
		return
	}

	var filtered []string
	filterLower := strings.ToLower(m.filterText)
	for _, line := range lines {
		if filterLower == "" || strings.Contains(strings.ToLower(line.Content), filterLower) {
			filtered = append(filtered, line.Content)
		}
	}

	if len(filtered) == 0 {
		m.viewport.SetContent(m.centeredMessage("No logs match the filter"))
		return
	}

	m.viewport.SetContent(strings.Join(filtered, "\n"))

	if m.wasAtBottom {
		m.viewport.GotoBottom()
	}
}

func (m Logs) centeredMessage(msg string) string {
	return lipgloss.Place(m.viewport.Width(), m.viewport.Height(), lipgloss.Center, lipgloss.Center, msg)
}

func logsHelpBindings(filterEnabled bool) []key.Binding {
	bindings := []key.Binding{logsKeys.Back}
	if filterEnabled {
		bindings = append([]key.Binding{logsKeys.Filter}, bindings...)
	}
	return bindings
}

func (m *Logs) updateViewportSize() {
	headerHeight := 2 // title + blank/filter line
	helpHeight := 1
	viewportHeight := m.height - headerHeight - helpHeight

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}
	m.viewport.SetWidth(m.width)
}
