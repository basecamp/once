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
	"github.com/basecamp/once/internal/userstats"
)

const PanelHeight = 7
const StoppedPanelHeight = 2

type DashboardPanel struct {
	app           docker.Application
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	userStats     *userstats.Reader
	cpuChart      Chart
	memoryChart   Chart
	requestChart  Chart
	errorChart    Chart
}

func NewDashboardPanel(app *docker.Application, scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper, userStats *userstats.Reader) DashboardPanel {
	return DashboardPanel{
		app:           *app,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		userStats:     userStats,
		cpuChart:      NewChart("CPU", UnitPercent),
		memoryChart:   NewChart("Memory", UnitBytes),
		requestChart:  NewChart("Req/min", UnitCount),
		errorChart:    NewChart("Err/min", UnitCount),
	}
}

func (p DashboardPanel) DataMaxes() (cpu, memory, requests, errors float64) {
	if !p.app.Running {
		return
	}

	cpuData, memData := p.fetchDockerData()
	reqData, errData := p.fetchMetricsData()

	return maxValue(cpuData), maxValue(memData), maxValue(reqData), maxValue(errData)
}

func (p DashboardPanel) View(selected bool, toggling bool, width int, scales DashboardScales) string {
	innerWidth := max(width-3, 0) // 1 indicator + 1 left pad + 1 right pad

	url := Styles.Title.Hyperlink(p.app.URL()).Render(p.app.Settings.Host)
	name := lipgloss.NewStyle().Foreground(Colors.Border).Render("(" + docker.NameFromImageRef(p.app.Settings.Image) + ")")
	left := url + " " + name
	if label := p.userStatsLabel(); label != "" {
		left += " " + lipgloss.NewStyle().Foreground(Colors.Border).Render(label)
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

		cpuData, memData := p.fetchDockerData()
		reqData, errData := p.fetchMetricsData()

		cpuChart := p.cpuChart.View(cpuData, chartW(0), chartHeight, scales.CPU)
		memChart := p.memoryChart.View(memData, chartW(1), chartHeight, scales.Memory)
		reqChart := p.requestChart.View(reqData, chartW(2), chartHeight, scales.Requests)
		errChart := p.errorChart.View(errData, chartW(3), chartHeight, scales.Errors)

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

func (p DashboardPanel) Height() int {
	bodyHeight := PanelHeight
	if !p.app.Running {
		bodyHeight = StoppedPanelHeight
	}
	return bodyHeight + 2 // top + bottom transition lines
}

// Private

func (p DashboardPanel) userStatsLabel() string {
	if p.userStats == nil {
		return ""
	}
	stats := p.userStats.Fetch(p.app.Settings.Name)
	if stats == nil || (stats.UniqueUsers24h == 0 && stats.UniqueUsers7d == 0) {
		return ""
	}
	return fmt.Sprintf("%d today · %d this week", stats.UniqueUsers24h, stats.UniqueUsers7d)
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

func (p DashboardPanel) fetchDockerData() (cpu, memory []float64) {
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	cpu = make([]float64, len(samples))
	memory = make([]float64, len(samples))
	for i, s := range samples {
		cpu[i] = s.CPUPercent
		memory[i] = float64(s.MemoryBytes)
	}
	slices.Reverse(cpu)
	slices.Reverse(memory)
	return
}

func (p DashboardPanel) fetchMetricsData() (requests, errors []float64) {
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartHistoryLength)
	requests = make([]float64, len(samples))
	errors = make([]float64, len(samples))
	for i, s := range samples {
		requests[i] = float64(s.Success + s.ClientErrors + s.ServerErrors)
		errors[i] = float64(s.ServerErrors)
	}
	slices.Reverse(requests)
	slices.Reverse(errors)
	return SlidingSum(requests, ChartSlidingWindow), SlidingSum(errors, ChartSlidingWindow)
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
