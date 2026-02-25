package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
)

var dashboardKeys = struct {
	Up       key.Binding
	Down     key.Binding
	Settings key.Binding
	Actions  key.Binding
	NewApp   key.Binding
	Logs     key.Binding
	Quit     key.Binding
}{
	Up:       WithHelp(NewKeyBinding("up", "k"), "↑/k", "up"),
	Down:     WithHelp(NewKeyBinding("down", "j"), "↓/j", "down"),
	Settings: WithHelp(NewKeyBinding("s"), "s", "settings"),
	Actions:  WithHelp(NewKeyBinding("a"), "a", "actions"),
	NewApp:   WithHelp(NewKeyBinding("n"), "n", "new app"),
	Logs:     WithHelp(NewKeyBinding("g"), "g", "logs"),
	Quit:     WithHelp(NewKeyBinding("esc"), "esc", "quit"),
}

type Dashboard struct {
	namespace     *docker.Namespace
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	apps          []*docker.Application
	panels        []DashboardPanel
	selectedIndex int
	width, height int
	viewport      viewport.Model
	toggling      bool
	togglingApp   string
	progress      *ProgressBusy
	help          Help
	overlay       Component
}

type dashboardTickMsg struct{}

type startStopFinishedMsg struct {
	err error
}

func NewDashboard(ns *docker.Namespace, apps []*docker.Application, selectedIndex int,
	scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper) *Dashboard {

	vp := viewport.New()
	vp.MouseWheelEnabled = false
	vp.KeyMap = viewport.KeyMap{} // disable default keys, we handle navigation ourselves

	d := &Dashboard{
		namespace:     ns,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		apps:          apps,
		selectedIndex: selectedIndex,
		viewport:      vp,
		help:          NewHelp(),
	}
	d.buildPanels()
	return d
}

func (m *Dashboard) Init() tea.Cmd {
	return m.scheduleNextDashboardTick()
}

func (m *Dashboard) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress = NewProgressBusy(m.width, Colors.Border)
		m.help.SetWidth(m.width)
		m.updateViewportSize()
		m.rebuildViewportContent()

		if m.overlay != nil {
			cmds = append(cmds, m.overlay.Update(msg))
		}

	case MouseEvent:
		if m.overlay != nil {
			return m.overlay.Update(msg)
		}
		if msg.IsClick {
			if i, ok := m.panelIndexAtY(msg.Y); ok {
				m.selectedIndex = i
				m.rebuildViewportContent()
				m.scrollToSelection()
				return nil
			}
			return m.help.Update(msg)
		}

	case tea.KeyPressMsg:
		if m.overlay != nil {
			return m.overlay.Update(msg)
		}

		if key.Matches(msg, dashboardKeys.Quit) {
			return func() tea.Msg { return QuitMsg{} }
		}
		if key.Matches(msg, dashboardKeys.Up) {
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.rebuildViewportContent()
				m.scrollToSelection()
			}
			return nil
		}
		if key.Matches(msg, dashboardKeys.Down) {
			if m.selectedIndex < len(m.apps)-1 {
				m.selectedIndex++
				m.rebuildViewportContent()
				m.scrollToSelection()
			}
			return nil
		}
		if key.Matches(msg, dashboardKeys.NewApp) {
			return func() tea.Msg { return NavigateToInstallMsg{} }
		}
		if key.Matches(msg, dashboardKeys.Settings) && len(m.apps) > 0 {
			app := m.apps[m.selectedIndex]
			menu := NewSettingsMenu(app)
			m.overlay = &menu
			cmd := m.overlay.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return cmd
		}
		if key.Matches(msg, dashboardKeys.Actions) && len(m.apps) > 0 && !m.toggling {
			app := m.apps[m.selectedIndex]
			menu := NewActionsMenu(app)
			m.overlay = &menu
			cmd := m.overlay.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return cmd
		}
		if key.Matches(msg, dashboardKeys.Logs) && len(m.apps) > 0 {
			return func() tea.Msg { return NavigateToLogsMsg{App: m.apps[m.selectedIndex]} }
		}

	case SettingsMenuCloseMsg:
		m.overlay = nil

	case SettingsMenuSelectMsg:
		m.overlay = nil
		return func() tea.Msg {
			return NavigateToSettingsSectionMsg{App: msg.app, Section: msg.section}
		}

	case ActionsMenuCloseMsg:
		m.overlay = nil

	case ActionsMenuSelectMsg:
		m.overlay = nil
		switch msg.action {
		case ActionsMenuStartStop:
			app := msg.app
			m.toggling = true
			m.togglingApp = app.Settings.Name
			m.progress = NewProgressBusy(m.width, Colors.Border)
			m.updateViewportSize()
			m.rebuildViewportContent()
			return tea.Batch(m.progress.Init(), m.runStartStop(app))
		case ActionsMenuRemove:
			return func() tea.Msg { return NavigateToRemoveMsg{App: msg.app} }
		}

	case startStopFinishedMsg:
		m.toggling = false
		m.togglingApp = ""
		m.updateViewportSize()
		m.rebuildViewportContent()

	case scrapeDoneMsg:
		m.rebuildViewportContent()

	case dashboardTickMsg:
		m.rebuildViewportContent()
		cmds = append(cmds, m.scheduleNextDashboardTick())

	case ProgressBusyTickMsg:
		if m.toggling && m.progress != nil {
			cmds = append(cmds, m.progress.Update(msg))
		}

	case namespaceChangedMsg:
		previousName := ""
		if m.selectedIndex < len(m.apps) {
			previousName = m.apps[m.selectedIndex].Settings.Name
		}
		m.apps = m.namespace.Applications()
		m.buildPanels()
		m.selectedIndex = 0
		for i, app := range m.apps {
			if app.Settings.Name == previousName {
				m.selectedIndex = i
				break
			}
		}
		if m.selectedIndex >= len(m.apps) && len(m.apps) > 0 {
			m.selectedIndex = len(m.apps) - 1
		}
		m.rebuildViewportContent()
		m.scrollToSelection()
	}

	if m.overlay != nil {
		cmds = append(cmds, m.overlay.Update(msg))
	}

	return tea.Batch(cmds...)
}

func (m *Dashboard) View() string {
	titleLine := Styles.TitleRule(m.width)

	var helpBindings []key.Binding
	if len(m.apps) > 0 {
		helpBindings = []key.Binding{
			dashboardKeys.Up, dashboardKeys.Down, dashboardKeys.Actions,
			dashboardKeys.Settings, dashboardKeys.Logs, dashboardKeys.NewApp, dashboardKeys.Quit,
		}
	} else {
		helpBindings = []key.Binding{dashboardKeys.NewApp, dashboardKeys.Quit}
	}
	helpView := m.help.View(helpBindings)
	helpLine := Styles.CenteredLine(m.width, helpView)

	if len(m.apps) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(Colors.Border).Render("There are no applications installed")
		middleHeight := m.height - 1 - 1 // title + help
		centeredContent := lipgloss.Place(m.width, middleHeight, lipgloss.Center, lipgloss.Center, emptyMsg)
		return titleLine + "\n" + centeredContent + "\n" + helpLine
	}

	var content string
	if m.toggling && m.progress != nil {
		content = titleLine + "\n" + m.viewport.View() + "\n" + m.progress.View() + "\n" + helpLine
	} else {
		content = titleLine + "\n" + m.viewport.View() + "\n" + helpLine
	}

	if m.overlay != nil {
		return OverlayCenter(content, m.overlay.View(), m.width, m.height)
	}

	return content
}

// Private

func (m *Dashboard) runStartStop(app *docker.Application) tea.Cmd {
	return func() tea.Msg {
		var err error
		if app.Running {
			err = app.Stop(context.Background())
		} else {
			err = app.Start(context.Background())
		}
		return startStopFinishedMsg{err: err}
	}
}

func (m *Dashboard) scheduleNextDashboardTick() tea.Cmd {
	return tea.Every(time.Second, func(time.Time) tea.Msg { return dashboardTickMsg{} })
}

func (m *Dashboard) updateViewportSize() {
	titleHeight := 1 // title line
	helpHeight := 1
	progressHeight := 0
	if m.toggling {
		progressHeight = 1
	}
	vpHeight := max(m.height-titleHeight-helpHeight-progressHeight, 0)
	m.viewport.SetHeight(vpHeight)
	m.viewport.SetWidth(m.width)
}

func (m *Dashboard) rebuildViewportContent() {
	var views []string
	for i := range m.panels {
		toggling := m.toggling && m.togglingApp == m.panels[i].app.Settings.Name
		views = append(views, m.panels[i].View(i == m.selectedIndex, toggling, m.width))
	}
	m.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, views...))
}

func (m *Dashboard) scrollToSelection() {
	panelTop := 0
	for i := range m.selectedIndex {
		panelTop += m.panels[i].Height()
	}
	panelBottom := panelTop + m.panels[m.selectedIndex].Height()
	if panelTop < m.viewport.YOffset() {
		m.viewport.SetYOffset(panelTop)
	} else if panelBottom > m.viewport.YOffset()+m.viewport.Height() {
		m.viewport.SetYOffset(panelBottom - m.viewport.Height())
	}
}

func (m *Dashboard) panelIndexAtY(y int) (int, bool) {
	titleHeight := 1
	vpRow := y - titleHeight
	if vpRow < 0 || vpRow >= m.viewport.Height() {
		return 0, false
	}

	contentRow := vpRow + m.viewport.YOffset()
	top := 0
	for i := range m.panels {
		h := m.panels[i].Height()
		if contentRow < top+h {
			return i, true
		}
		top += h
	}
	return 0, false
}

func (m *Dashboard) buildPanels() {
	m.panels = make([]DashboardPanel, len(m.apps))
	for i, app := range m.apps {
		m.panels[i] = NewDashboardPanel(app, m.scraper, m.dockerScraper)
	}
}
