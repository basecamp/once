package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
)

const PanelHeight = 7
const StoppedPanelHeight = 2

type DashboardPanel struct {
	app           docker.Application
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	cpuChart      Chart
	memoryChart   Chart
	requestChart  Chart
	errorChart    Chart
}

func NewDashboardPanel(app *docker.Application, scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper) DashboardPanel {
	return DashboardPanel{
		app:           *app,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		cpuChart:      NewChart("CPU", UnitPercent),
		memoryChart:   NewChart("Memory", UnitBytes),
		requestChart:  NewChart("Req/min", UnitCount),
		errorChart:    NewChart("Err/min", UnitCount),
	}
}

func (p DashboardPanel) View(selected bool, toggling bool, width int) string {
	innerWidth := max(width-3, 0) // 1 indicator + 1 left pad + 1 right pad

	var left string
	if url := p.app.Settings.URL(); url != "" {
		left = Styles.Title.Hyperlink(url).Render(p.app.Settings.Host)
	} else {
		left = Styles.Title.Render(p.app.Settings.Name)
	}
	right := renderStateInfo(&p.app, toggling)
	gap := max(innerWidth-1-lipgloss.Width(left)-lipgloss.Width(right), 1)
	titleLine := " " + left + strings.Repeat(" ", gap) + right

	var lines []string
	lines = append(lines, titleLine)

	// Show charts when the app is running and there's enough width
	chartHeight := 6
	minChartWidth := 10
	if p.app.Running && innerWidth >= minChartWidth*4+3 {
		baseWidth := (innerWidth - 3) / 4 // 3 single-char gaps between 4 charts
		remainder := (innerWidth - 3) % 4
		chartW := func(i int) int {
			if i < remainder {
				return baseWidth + 1
			}
			return baseWidth
		}

		cpuChart := p.cpuChart.View(p.fetchCPUData(), chartW(0), chartHeight)
		memChart := p.memoryChart.View(p.fetchMemoryData(), chartW(1), chartHeight)
		reqChart := p.requestChart.View(p.fetchRequestData(), chartW(2), chartHeight)
		errChart := p.errorChart.View(p.fetchErrorData(), chartW(3), chartHeight)

		chartsRow := lipgloss.JoinHorizontal(lipgloss.Top, cpuChart, " ", memChart, " ", reqChart, " ", errChart)
		lines = append(lines, chartsRow)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	height := PanelHeight
	if !p.app.Running {
		height = StoppedPanelHeight
	}

	bodyStyle := lipgloss.NewStyle().
		Width(width-1).
		Padding(0, 1).
		Height(height)

	var body string
	if selected {
		body = bodyStyle.Background(Colors.PanelBg).Render(content)
		body = WithBackground(Colors.PanelBg, body)
	} else {
		body = bodyStyle.Render(content)
	}

	indicator := p.renderIndicator(selected)
	topTrans := p.renderTopTransition(selected, width)
	bottomTrans := p.renderBottomTransition(selected, width)

	return topTrans + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, indicator, body) + "\n" + bottomTrans
}

// Private

func (p DashboardPanel) Height(selected bool, width int) int {
	return lipgloss.Height(p.View(selected, false, width))
}

func (p DashboardPanel) renderTopTransition(selected bool, width int) string {
	if !selected {
		return strings.Repeat(" ", width)
	}
	indicatorChar := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▗")
	bodyChars := lipgloss.NewStyle().Foreground(Colors.PanelBg).Render(strings.Repeat("▄", width-1))
	return indicatorChar + bodyChars
}

func (p DashboardPanel) renderBottomTransition(selected bool, width int) string {
	if !selected {
		return strings.Repeat(" ", width)
	}
	indicatorChar := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▝")
	bodyChars := lipgloss.NewStyle().Foreground(Colors.PanelBg).Render(strings.Repeat("▀", width-1))
	return indicatorChar + bodyChars
}

func (p DashboardPanel) renderIndicator(selected bool) string {
	height := PanelHeight
	if !p.app.Running {
		height = StoppedPanelHeight
	}
	rows := make([]string, height)
	if selected {
		line := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▐")
		for i := range rows {
			rows[i] = line
		}
	} else {
		for i := range rows {
			rows[i] = " "
		}
	}
	return strings.Join(rows, "\n")
}

func (p DashboardPanel) fetchCPUData() []float64 {
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = s.CPUPercent
	}
	slices.Reverse(data)
	return data
}

func (p DashboardPanel) fetchMemoryData() []float64 {
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.MemoryBytes)
	}
	slices.Reverse(data)
	return data
}

func (p DashboardPanel) fetchRequestData() []float64 {
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.Success + s.ClientErrors + s.ServerErrors)
	}
	slices.Reverse(data)
	return SlidingSum(data, ChartSlidingWindow)
}

func (p DashboardPanel) fetchErrorData() []float64 {
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	data := make([]float64, len(samples))
	for i, s := range samples {
		data[i] = float64(s.ServerErrors)
	}
	slices.Reverse(data)
	return SlidingSum(data, ChartSlidingWindow)
}

// Helpers

func renderStateInfo(app *docker.Application, toggling bool) string {
	var status string
	var statusColor color.Color
	if toggling && app.Running {
		status = "stopping..."
		statusColor = Colors.Border
	} else if toggling {
		status = "starting..."
		statusColor = Colors.Border
	} else if app.Running {
		status = "running"
		statusColor = chartGradientBottom
	} else {
		status = "stopped"
		statusColor = chartGradientTop
	}

	stateStyle := lipgloss.NewStyle().Foreground(statusColor)
	stateDisplay := stateStyle.Render(status)

	if app.Running && !app.RunningSince.IsZero() {
		stateDisplay += fmt.Sprintf(" (up %s)", formatDuration(time.Since(app.RunningSince)))
	}

	return stateDisplay
}

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
