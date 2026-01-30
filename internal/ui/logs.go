package ui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
)

type logsKeyMap struct {
	Filter key.Binding
	Back   key.Binding
}

func (k logsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Back}
}

func (k logsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Filter, k.Back}}
}

var logsKeys = logsKeyMap{
	Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type Logs struct {
	namespace     *docker.Namespace
	app           *docker.Application
	streamer      *docker.LogStreamer
	viewport      viewport.Model
	filterInput   textinput.Model
	filterActive  bool
	filterText    string
	width, height int
	help          help.Model

	lastVersion    uint64
	lastFilterText string
	wasAtBottom    bool
}

type logsTickMsg struct{}

func NewLogs(ns *docker.Namespace, app *docker.Application) Logs {
	streamer := docker.NewLogStreamer(ns, docker.LogStreamerSettings{})

	filterInput := textinput.New()
	filterInput.Placeholder = "Filter logs"
	filterInput.Prompt = ""
	filterInput.CharLimit = 256

	vp := viewport.New()
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true

	return Logs{
		namespace:   ns,
		app:         app,
		streamer:    streamer,
		viewport:    vp,
		filterInput: filterInput,
		help:        help.New(),
		wasAtBottom: true,
	}
}

func (m Logs) Init() tea.Cmd {
	containerName, err := m.app.ContainerName(context.Background())
	if err == nil {
		m.streamer.Start(context.Background(), containerName)
	}
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return logsTickMsg{} })
}

func (m Logs) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.updateViewportSize()
		m.rebuildContent()

	case tea.KeyMsg:
		if m.filterActive {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)

	case logsTickMsg:
		m.checkForUpdates()
		cmds = append(cmds, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return logsTickMsg{} }))

	case NamespaceChangedMsg:
		containerName, err := m.app.ContainerName(context.Background())
		if err == nil {
			m.streamer.Start(context.Background(), containerName)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Logs) View() string {
	titleBox := m.renderTitleBox()

	helpView := m.help.View(logsKeys)
	helpLine := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(helpView)

	titleHeight := lipgloss.Height(titleBox)
	helpHeight := lipgloss.Height(helpLine)
	viewportHeight := m.height - titleHeight - helpHeight

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}

	topLayer := lipgloss.NewLayer(titleBox + "\n" + m.viewport.View())
	bottomLayer := lipgloss.NewLayer(helpLine).Y(m.height - helpHeight)

	return lipgloss.NewCanvas(topLayer, bottomLayer).Render()
}

// Private

func (m Logs) handleFilterKey(msg tea.KeyMsg) (Component, tea.Cmd) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
		m.filterActive = false
		m.filterText = ""
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.rebuildContent()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.rebuildContent()
	return m, cmd
}

func (m Logs) handleNormalKey(msg tea.KeyMsg) (Component, tea.Cmd) {
	switch {
	case key.Matches(msg, logsKeys.Back):
		m.streamer.Stop()
		return m, func() tea.Msg { return navigateToDashboardMsg{} }

	case key.Matches(msg, logsKeys.Filter):
		m.filterActive = true
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

func (m *Logs) centeredMessage(msg string) string {
	return lipgloss.Place(m.viewport.Width(), m.viewport.Height(), lipgloss.Center, lipgloss.Center, msg)
}

func (m *Logs) updateViewportSize() {
	titleBox := m.renderTitleBox()
	helpView := m.help.View(logsKeys)

	titleHeight := lipgloss.Height(titleBox)
	helpHeight := lipgloss.Height(helpView)
	viewportHeight := m.height - titleHeight - helpHeight - 1

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}
	m.viewport.SetWidth(m.width)
}

func (m *Logs) renderTitleBox() string {
	innerWidth := m.width - 6
	if innerWidth < 0 {
		innerWidth = 0
	}
	m.filterInput.SetWidth(innerWidth)

	var extraLines []string
	if m.filterActive || m.filterText != "" {
		extraLines = append(extraLines, " "+m.filterInput.View())
	}
	return Styles.TitleBox(m.width, m.app.Settings.Name+" - Logs", extraLines...)
}
