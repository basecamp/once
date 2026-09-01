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

const PanelHeight = 6
const StoppedPanelHeight = 2

const containerStatsBuffer = 10
const peakWindow = containerStatsBuffer

const (
	defaultWarningPct = 60
	defaultErrorPct   = 85
	trafficWarningPct = 3
	trafficErrorPct   = 5
)

type DashboardPanel struct {
	app           docker.Application
	scraper       *metrics.MetricsScraper
	dockerScraper *docker.Scraper
	userStats     *userstats.Reader
}

func NewDashboardPanel(app *docker.Application, scraper *metrics.MetricsScraper, dockerScraper *docker.Scraper, userStats *userstats.Reader) DashboardPanel {
	return DashboardPanel{
		app:           *app,
		scraper:       scraper,
		dockerScraper: dockerScraper,
		userStats:     userStats,
	}
}

func (p DashboardPanel) DataMaxes() (traffic float64) {
	if !p.app.Running {
		return
	}

	reqData, _ := p.fetchMetricsData()
	return maxValue(reqData)
}

func (p DashboardPanel) View(selected bool, toggling bool, showDetails bool, width int, scales DashboardScales) string {
	innerWidth := max(width-3, 0) // indicator(1) + left padding(1) + right padding(1)
	detailed := showDetails && p.app.Running

	var cards [3]MetricCard
	if p.app.Running {
		cards = p.buildMetricCards(scales)
	}

	url := Styles.Title.Hyperlink(p.app.URL()).Render(p.app.Settings.Host)
	name := lipgloss.NewStyle().Foreground(Colors.Border).Render("(" + docker.NameFromImageRef(p.app.Settings.Image) + ")")
	badge := p.renderHealthBadge(cards)
	left := badge + " " + url + " " + name
	right := renderStateInfo(&p.app, toggling)
	gap := max(innerWidth-2-lipgloss.Width(left)-lipgloss.Width(right), 1)
	titleLine := " " + left + strings.Repeat(" ", gap) + right + " "

	var lines []string
	lines = append(lines, titleLine)

	// Show cards when running, details on, and enough width for all 4 cards + 3 gaps
	const minCardWidth = 8
	const cardCount = 4
	const cardGaps = cardCount - 1
	if detailed && innerWidth >= minCardWidth*cardCount+cardGaps {
		cardViews := p.renderCards(innerWidth, cards)
		lines = append(lines, cardViews)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	height := PanelHeight
	if !detailed {
		height = StoppedPanelHeight
	}

	bodyStyle := lipgloss.NewStyle().
		Width(width-1).
		Padding(0, 1).
		Height(height)

	var body string
	if selected {
		body = bodyStyle.Background(Colors.BackgroundTint).Render(content)
		body = WithBackground(Colors.BackgroundTint, body)
	} else {
		body = bodyStyle.Render(content)
	}

	indicator := p.renderIndicator(selected, height)
	topTrans := p.renderTopTransition(selected, width)
	bottomTrans := p.renderBottomTransition(selected, width)

	return topTrans + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, indicator, body) + "\n" + bottomTrans
}

func (p DashboardPanel) Height(showDetails bool) int {
	bodyHeight := PanelHeight
	if !showDetails || !p.app.Running {
		bodyHeight = StoppedPanelHeight
	}
	return bodyHeight + 2 // top + bottom transition lines
}

// Private

func (p DashboardPanel) renderHealthBadge(cards [3]MetricCard) string {
	if !p.app.Running {
		return lipgloss.NewStyle().Foreground(Colors.Border).Render("●")
	}

	worst := healthNormal
	for _, c := range cards {
		worst = max(worst, c.Health())
	}

	return lipgloss.NewStyle().Foreground(worst.Color()).Render("●")
}

func (p DashboardPanel) renderCards(innerWidth int, cards [3]MetricCard) string {
	gaps := 3 // 3 single-char gaps between 4 cards
	visitsWidth := (innerWidth - gaps) / 3
	remaining := (innerWidth - gaps) - visitsWidth
	metricBase := remaining / 3
	metricRem := remaining % 3

	metricWidth := func(i int) int {
		if i < metricRem {
			return metricBase + 1
		}
		return metricBase
	}

	visitsCard := p.renderVisitsCard(visitsWidth)
	cpuCard := cards[0].View(metricWidth(0))
	memCard := cards[1].View(metricWidth(1))
	trafficCard := cards[2].View(metricWidth(2))

	return lipgloss.JoinHorizontal(lipgloss.Top,
		visitsCard, " ", cpuCard, " ", memCard, " ", trafficCard)
}

func (p DashboardPanel) buildMetricCards(scales DashboardScales) [3]MetricCard {
	cpuData, memData := p.fetchDockerData()
	reqData, errData := p.fetchMetricsData()

	cpuScale := scales.CPU
	cpuLimit := ""
	if c := p.app.Settings.Resources.CPUs; c > 0 {
		cpuScale = ChartScale{max: float64(c) * 100}
		cpuLimit = UnitPercent.Format(float64(c) * 100)
	}

	memScale := scales.Memory
	memLimit := ""
	if mb := p.app.Settings.Resources.MemoryMB; mb > 0 {
		memScale = ChartScale{max: float64(mb) * 1024 * 1024}
		memLimit = UnitBytes.Format(float64(mb) * 1024 * 1024)
	}

	currentReq := lastValue(reqData)
	currentErr := lastValue(errData)
	errPct := 0.0
	if currentReq > 0 {
		errPct = currentErr / currentReq * 100
	}

	return [3]MetricCard{
		NewMetricCard("CPU", cpuData, cpuScale, UnitPercent, cpuLimit, defaultWarningPct, defaultErrorPct),
		NewMetricCard("Memory", memData, memScale, UnitBytes, memLimit, defaultWarningPct, defaultErrorPct),
		NewTrafficCard(reqData, errData, scales.Traffic, errPct, trafficWarningPct, trafficErrorPct),
	}
}

func (p DashboardPanel) renderVisitsCard(width int) string {
	inner := width - 2 // left + right border

	left := boxSide()
	right := boxSide()

	var dayLine, weekLine string
	if p.userStats != nil {
		stats := p.userStats.Fetch(p.app.Settings.Name)
		if stats != nil && (stats.UniqueUsers24h > 0 || stats.UniqueUsers7d > 0) {
			noun := "visitors"
			if stats.UniqueUsers24h == 1 {
				noun = "visitor"
			}
			dayLine = fmt.Sprintf(" %d %s today", stats.UniqueUsers24h, noun)
			weekLine = fmt.Sprintf(" %d this week", stats.UniqueUsers7d)
		}
	}

	contentLines := make([]string, 3)
	contentLines[0] = left + strings.Repeat(" ", inner) + right
	contentLines[1] = left + padOrTruncate(dayLine, inner) + right
	contentLines[2] = left + padOrTruncate(weekLine, inner) + right

	return boxTop("Visits", inner) + "\n" + strings.Join(contentLines, "\n") + "\n" + boxBottom(inner)
}

func (p DashboardPanel) renderTopTransition(selected bool, width int) string {
	if !selected || Colors.BackgroundTint == nil {
		return strings.Repeat(" ", width)
	}
	indicatorChar := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▗")
	bodyChars := lipgloss.NewStyle().Foreground(Colors.BackgroundTint).Render(strings.Repeat("▄", width-1))
	return indicatorChar + bodyChars
}

func (p DashboardPanel) renderBottomTransition(selected bool, width int) string {
	if !selected || Colors.BackgroundTint == nil {
		return strings.Repeat(" ", width)
	}
	indicatorChar := lipgloss.NewStyle().Foreground(Colors.Focused).Render("▝")
	bodyChars := lipgloss.NewStyle().Foreground(Colors.BackgroundTint).Render(strings.Repeat("▀", width-1))
	return indicatorChar + bodyChars
}

func (p DashboardPanel) renderIndicator(selected bool, height int) string {
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
	samples := p.dockerScraper.Fetch(p.app.Settings.Name, containerStatsBuffer)
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
	samples := p.scraper.Fetch(p.app.Settings.Name, ChartSlidingWindow)
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
		statusColor = Colors.LightText
	} else if toggling {
		status = "starting..."
		statusColor = Colors.LightText
	} else if app.Running {
		status = "running"
		statusColor = Colors.Success
	} else {
		status = "stopped"
		statusColor = Colors.LightText
	}

	stateStyle := lipgloss.NewStyle().Foreground(statusColor)
	return stateStyle.Render(status)
}

func renderBar(current, peak, scaleMax float64, fillColor color.Color, width int) string {
	if width <= 0 {
		return ""
	}
	filled := int(current / scaleMax * float64(width))
	filled = min(filled, width)

	peakPos := int(peak / scaleMax * float64(width))
	peakPos = min(peakPos, width-1)

	filledStyle := lipgloss.NewStyle().Foreground(fillColor)
	emptyStyle := lipgloss.NewStyle().Foreground(Colors.Border)
	peakStyle := lipgloss.NewStyle().Foreground(Colors.FocusOrange)

	// Build per-character styles
	styles := make([]lipgloss.Style, width)
	if peak > 0 {
		peakPos = max(peakPos, filled-1)
		peakPos = min(peakPos, width-1)
		for i := range width {
			switch {
			case i == peakPos:
				styles[i] = peakStyle
			case i < filled:
				styles[i] = filledStyle
			default:
				styles[i] = emptyStyle
			}
		}
	} else {
		for i := range width {
			if i < filled {
				styles[i] = filledStyle
			} else {
				styles[i] = emptyStyle
			}
		}
	}

	// Render with rounded ends
	const barFull = "⣿"
	const barRoundLeft = "⢾"
	const barRoundRight = "⡷"

	var b strings.Builder
	for i, s := range styles {
		var ch string
		switch i {
		case 0:
			ch = barRoundLeft
		case width - 1:
			ch = barRoundRight
		default:
			ch = barFull
		}
		b.WriteString(s.Render(ch))
	}
	return b.String()
}

func lastValue(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	return data[len(data)-1]
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
