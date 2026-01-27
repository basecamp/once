package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
	"github.com/basecamp/amar/internal/metrics"
)

var chartColors = struct {
	Green  lipgloss.Style
	Red    lipgloss.Style
	Blue   lipgloss.Style
	Purple lipgloss.Style
}{
	Green:  lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	Red:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")),
	Blue:   lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")),
	Purple: lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")),
}

type dashboardKeyMap struct {
	Settings key.Binding
	Upgrade  key.Binding
	NewApp   key.Binding
	PrevApp  key.Binding
	NextApp  key.Binding
	Quit     key.Binding
}

func (k dashboardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.PrevApp, k.NextApp, k.Settings, k.NewApp, k.Upgrade, k.Quit}
}

func (k dashboardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.PrevApp, k.NextApp, k.Settings, k.NewApp, k.Upgrade, k.Quit}}
}

var dashboardKeys = dashboardKeyMap{
	Settings: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "settings")),
	Upgrade:  key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upgrade")),
	NewApp:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new app")),
	PrevApp:  key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev app")),
	NextApp:  key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next app")),
	Quit:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
}

type Dashboard struct {
	namespace     *docker.Namespace
	app           *docker.Application
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	width, height int
	upgrading     bool
	progress      ProgressBusy
	help          help.Model
	allReqChart   Chart
	errorChart    Chart
	cpuChart      Chart
	memoryChart   Chart
}

type dashboardTickMsg struct{}

type upgradeFinishedMsg struct {
	err error
}

func NewDashboard(ns *docker.Namespace, app *docker.Application, scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper) Dashboard {
	service := app.Settings.Name

	allReqChart := NewChart("Requests/min", chartColors.Green, UnitCount, func() []float64 {
		samples := scraper.Fetch(service, ChartHistoryLength)
		data := make([]float64, len(samples))
		for i, s := range samples {
			data[i] = float64(s.Success + s.ClientErrors + s.ServerErrors)
		}
		slices.Reverse(data)
		return SlidingSum(data, 12)
	})

	errorChart := NewChart("Errors/min", chartColors.Red, UnitCount, func() []float64 {
		samples := scraper.Fetch(service, ChartHistoryLength)
		data := make([]float64, len(samples))
		for i, s := range samples {
			data[i] = float64(s.ServerErrors)
		}
		slices.Reverse(data)
		return SlidingSum(data, 12)
	})

	cpuChart := NewChart("CPU", chartColors.Blue, UnitPercent, func() []float64 {
		samples := dockerScraper.Fetch(service, ChartHistoryLength)
		data := make([]float64, len(samples))
		for i, s := range samples {
			data[i] = s.CPUPercent
		}
		slices.Reverse(data)
		return data
	})

	memoryChart := NewChart("Memory", chartColors.Purple, UnitBytes, func() []float64 {
		samples := dockerScraper.Fetch(service, ChartHistoryLength)
		data := make([]float64, len(samples))
		for i, s := range samples {
			data[i] = float64(s.MemoryBytes)
		}
		slices.Reverse(data)
		return data
	})

	allReqChart.Update()
	errorChart.Update()
	cpuChart.Update()
	memoryChart.Update()

	return Dashboard{
		namespace:     ns,
		app:           app,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		help:          help.New(),
		allReqChart:   allReqChart,
		errorChart:    errorChart,
		cpuChart:      cpuChart,
		memoryChart:   memoryChart,
	}
}

func (m Dashboard) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return dashboardTickMsg{} })
}

func (m Dashboard) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
		m.help.SetWidth(m.width)

		chartWidth := m.width / 2
		chartHeight := 8
		m.allReqChart.SetSize(chartWidth, chartHeight)
		m.errorChart.SetSize(chartWidth, chartHeight)
		m.cpuChart.SetSize(chartWidth, chartHeight)
		m.memoryChart.SetSize(chartWidth, chartHeight)

		if m.upgrading {
			cmds = append(cmds, m.progress.Init())
		}
	case tea.KeyMsg:
		if key.Matches(msg, dashboardKeys.Quit) {
			return m, func() tea.Msg { return quitMsg{} }
		}
		if key.Matches(msg, dashboardKeys.PrevApp) {
			return m, func() tea.Msg { return switchAppMsg{delta: -1} }
		}
		if key.Matches(msg, dashboardKeys.NextApp) {
			return m, func() tea.Msg { return switchAppMsg{delta: 1} }
		}
		if key.Matches(msg, dashboardKeys.NewApp) {
			return m, func() tea.Msg { return navigateToInstallMsg{} }
		}
		if key.Matches(msg, dashboardKeys.Settings) {
			return m, func() tea.Msg { return navigateToSettingsMsg{app: m.app} }
		}
		if key.Matches(msg, dashboardKeys.Upgrade) && !m.upgrading {
			m.upgrading = true
			m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
			return m, tea.Batch(m.progress.Init(), m.runUpgrade())
		}
	case upgradeFinishedMsg:
		m.upgrading = false
	case dashboardTickMsg:
		m.allReqChart.Update()
		m.errorChart.Update()
		m.cpuChart.Update()
		m.memoryChart.Update()
		cmds = append(cmds, tea.Tick(time.Second, func(time.Time) tea.Msg { return dashboardTickMsg{} }))
	case progressBusyTickMsg:
		if m.upgrading {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	case NamespaceChangedMsg:
		_ = m.namespace.Refresh(context.Background())
		for _, app := range m.namespace.Applications() {
			if app.Settings.Name == m.app.Settings.Name {
				m.app = app
				break
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Dashboard) View() string {
	// Build info box content
	var status string
	var statusColor color.Color
	if m.upgrading {
		status = "upgrading..."
		statusColor = lipgloss.Color("#f1fa8c")
	} else if m.app.Running {
		status = "running"
		statusColor = lipgloss.Color("#50fa7b")
	} else {
		status = "stopped"
		statusColor = lipgloss.Color("#ff5555")
	}

	stateStyle := lipgloss.NewStyle().Foreground(statusColor)
	stateDisplay := fmt.Sprintf("State: %s", stateStyle.Render(status))

	if m.app.Running && !m.app.RunningSince.IsZero() && !m.upgrading {
		stateDisplay += fmt.Sprintf(" (up %s)", formatDuration(time.Since(m.app.RunningSince)))
	}

	// Inner width accounts for border (2) and padding (2)
	innerWidth := m.width - 4

	var infoLines []string
	titleLine := Styles.Title.Width(innerWidth).Align(lipgloss.Center).Render(m.app.Settings.Name)
	infoLines = append(infoLines, titleLine)
	infoLines = append(infoLines, stateDisplay)
	if url := m.app.Settings.URL(); url != "" {
		infoLines = append(infoLines, fmt.Sprintf("URL: %s", url))
	}

	infoContent := strings.Join(infoLines, "\n")
	infoBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6272a4")).
		Padding(0, 1).
		Width(m.width).
		Render(infoContent)

	// Charts in 2x2 grid
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, m.allReqChart.View(), m.errorChart.View())
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, m.cpuChart.View(), m.memoryChart.View())
	charts := lipgloss.JoinVertical(lipgloss.Left, row1, row2)

	// Help string (last line, centered)
	helpView := m.help.View(dashboardKeys)
	helpLine := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(helpView)

	// Progress bar (second-to-last line, only during upgrade)
	var bottomContent string
	if m.upgrading {
		bottomContent = m.progress.View() + "\n" + helpLine
	} else {
		bottomContent = helpLine
	}

	topContent := infoBox + "\n" + charts
	bottomHeight := lipgloss.Height(bottomContent)

	topLayer := lipgloss.NewLayer(topContent)
	bottomLayer := lipgloss.NewLayer(bottomContent).Y(m.height - bottomHeight)

	return lipgloss.NewCanvas(topLayer, bottomLayer).Render()
}

// Private

func (m Dashboard) runUpgrade() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Update(context.Background(), nil)
		return upgradeFinishedMsg{err: err}
	}
}

// Helpers

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}
