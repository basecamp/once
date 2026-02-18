package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/components"
	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

var logsKeys = struct {
	Filter KeyBinding
	Back   KeyBinding
}{
	Filter: NewKeyBinding(RuneKey('/')).WithHelp("/", "filter"),
	Back:   NewKeyBinding(Key(tui.KeyEscape)).WithHelp("esc", "back"),
}

type Logs struct {
	namespace     *docker.Namespace
	app           *docker.Application
	streamer      *docker.LogStreamer
	viewport      *components.Viewport
	filterInput   *components.TextField
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

func NewLogs(ns *docker.Namespace, app *docker.Application) *Logs {
	streamer := docker.NewLogStreamer(ns, docker.LogStreamerSettings{})

	filterInput := components.NewTextField()
	filterInput.SetPlaceholder("Filter logs")
	filterInput.SetCharLimit(256)

	vp := components.NewViewport()
	vp.SetSoftWrap(true)

	return &Logs{
		namespace:     ns,
		app:           app,
		streamer:      streamer,
		viewport:      vp,
		filterInput:   filterInput,
		filterEnabled: true,
		help:          NewHelp(),
		wasAtBottom:   true,
	}
}

func (m *Logs) Init() tui.Cmd {
	containerName, err := m.app.ContainerName(context.Background())
	if err == nil {
		m.streamer.Start(context.Background(), containerName)
	}
	return m.scheduleNextLogsTick()
}

func (m *Logs) Update(msg tui.Msg) tui.Cmd {
	var cmds []tui.Cmd

	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.updateViewportSize()
		m.rebuildContent()

	case tui.MouseMsg:
		if cmd := m.help.Update(msg); cmd != nil {
			return cmd
		}

	case tui.KeyMsg:
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

	return tui.Batch(cmds...)
}

func (m *Logs) Render() string {
	titleLine := Styles.TitleRule(m.width, m.app.Settings.Host, "logs")

	helpBindings := []KeyBinding{logsKeys.Back}
	if m.filterEnabled {
		helpBindings = append([]KeyBinding{logsKeys.Filter}, helpBindings...)
	}
	helpView := m.help.Render(helpBindings)
	helpLine := Styles.HelpLine(m.width, helpView)

	header := titleLine + "\n"
	if m.filterActive || m.filterText != "" {
		filterWidth := m.width - 2
		m.filterInput.SetWidth(filterWidth)
		header += " " + m.filterInput.Render() + "\n"
	} else {
		header += "\n"
	}

	headerHeight := lipgloss.Height(header)
	helpHeight := lipgloss.Height(helpLine)
	viewportHeight := m.height - headerHeight - helpHeight

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}

	return header + m.viewport.Render() + "\n" + helpLine
}

// Private

func (m *Logs) scheduleNextLogsTick() tui.Cmd {
	return tui.Every(100*time.Millisecond, func(time.Time) tui.Msg { return logsTickMsg{} })
}

func (m *Logs) handleFilterKey(msg tui.KeyMsg) tui.Cmd {
	if msg.Type == tui.KeyEscape {
		m.filterActive = false
		m.filterEnabled = true
		m.filterText = ""
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.rebuildContent()
		return nil
	}

	cmd := m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.rebuildContent()
	return cmd
}

func (m *Logs) handleNormalKey(msg tui.KeyMsg) tui.Cmd {
	switch {
	case logsKeys.Back.Matches(msg):
		m.streamer.Stop()
		return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }

	case logsKeys.Filter.Matches(msg) && m.filterEnabled:
		m.filterActive = true
		m.filterEnabled = false
		return m.filterInput.Focus()
	}

	m.wasAtBottom = m.viewport.AtBottom()
	m.viewport.Update(msg)
	return nil
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

func (m *Logs) centeredMessage(msg string) string {
	return lipgloss.Place(m.viewport.Width(), m.viewport.Height(), lipgloss.Center, lipgloss.Center, msg)
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
