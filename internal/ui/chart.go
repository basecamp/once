package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const ChartHistoryLength = 200
const ChartUpdateInterval = 2 * time.Second
const ChartSlidingWindow = int(time.Minute / ChartUpdateInterval)

type UnitType int

const (
	UnitCount   UnitType = iota // 1K, 1M (requests, errors)
	UnitPercent                 // 50.0%
	UnitBytes                   // 128 MiB, 1.5 GiB
)

func (u UnitType) Format(value float64) string {
	switch u {
	case UnitPercent:
		return fmt.Sprintf("%.0f%%", value)
	case UnitBytes:
		const (
			KiB = 1024
			MiB = KiB * 1024
			GiB = MiB * 1024
		)
		switch {
		case value >= GiB:
			return fmt.Sprintf("%.1fG", value/GiB)
		case value >= MiB:
			return fmt.Sprintf("%.0fM", value/MiB)
		case value >= KiB:
			return fmt.Sprintf("%.0fK", value/KiB)
		default:
			return fmt.Sprintf("%.0fB", value)
		}
	default: // UnitCount
		if value >= 1_000_000 {
			return fmt.Sprintf("%.1fM", value/1_000_000)
		}
		if value >= 1_000 {
			return fmt.Sprintf("%.1fK", value/1_000)
		}
		return fmt.Sprintf("%.0f", value)
	}
}

type DataSource func() []float64

// Chart renders a histogram-style chart using braille characters.
// Each data point is one character wide, and the height scales dynamically
// so the maximum value fills the available height.
type Chart struct {
	title      string
	color      lipgloss.Style
	unit       UnitType
	dataSource DataSource
	width      int
	height     int
	data       []float64
}

// braille bit patterns for left and right columns
// Each column has 4 dots, allowing 2 data points per character.
// Left column dots (bottom to top): 7, 3, 2, 1
// Right column dots (bottom to top): 8, 6, 5, 4
var (
	leftDots  = [4]rune{0x40, 0x04, 0x02, 0x01} // dots 7, 3, 2, 1
	rightDots = [4]rune{0x80, 0x20, 0x10, 0x08} // dots 8, 6, 5, 4
)

func NewChart(title string, color lipgloss.Style, unit UnitType, dataSource DataSource) Chart {
	return Chart{
		title:      title,
		color:      color,
		unit:       unit,
		dataSource: dataSource,
	}
}

func (c Chart) Init() tea.Cmd {
	return nil
}

func (c Chart) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case ComponentSizeMsg:
		c.width, c.height = msg.Width, msg.Height
	case ChartRefreshMsg:
		c.refreshData()
	}
	return c, nil
}

// ChartRefreshMsg tells a Chart to refresh its data from its data source
type ChartRefreshMsg struct{}

func (c *Chart) refreshData() {
	c.data = c.dataSource()
}

func (c Chart) View() string {
	if c.width == 0 || c.height == 0 {
		return ""
	}

	// Account for border (2 chars width, 2 lines height) and title (1 line)
	innerWidth := c.width - 2
	chartRows := c.height - 2 - 1 // minus border, minus title
	if chartRows < 1 {
		chartRows = 1
	}

	// Ensure data fills the chart width (each chart char = 2 data points)
	dataPoints := c.width * 2
	data := make([]float64, dataPoints)
	srcStart := max(0, len(c.data)-dataPoints)
	dstStart := max(0, dataPoints-len(c.data))
	copy(data[dstStart:], c.data[srcStart:])

	maxVal := maxValue(data)
	displayMax := maxVal
	if maxVal == 0 {
		maxVal = 1
	}

	// Format labels and calculate label width
	maxLabel := c.unit.Format(displayMax)
	labelWidth := max(len(maxLabel), 1)
	chartWidth := innerWidth - labelWidth - 1 // -1 for space between label and chart

	if chartWidth <= 0 {
		return ""
	}

	// Each character row represents 4 vertical dots
	dotsHeight := chartRows * 4

	// Calculate the height in dots for each data point
	heights := make([]int, len(data))
	for i, v := range data {
		heights[i] = int((v / maxVal) * float64(dotsHeight))
		if v > 0 && heights[i] == 0 {
			heights[i] = 1 // ensure non-zero values show at least 1 dot
		}
	}

	var content []string

	// Title line (centered over inner width)
	if c.title != "" {
		titleLine := Styles.CenteredLine(innerWidth, c.title)
		content = append(content, titleLine)
	}

	// Build the chart row by row, from top to bottom
	// Use rightmost data points if we have more data than chart columns
	dataOffset := max(0, len(heights)-chartWidth*2)

	labelStyle := lipgloss.NewStyle().Width(labelWidth).Align(lipgloss.Left)
	for row := range chartRows {
		var sb strings.Builder
		rowBottomDot := (chartRows - 1 - row) * 4
		rowTopDot := rowBottomDot + 4

		for col := range chartWidth {
			dataIdxLeft := dataOffset + col*2
			dataIdxRight := dataOffset + col*2 + 1

			var char rune = 0x2800 // braille base character

			// Left column (first data point)
			if dataIdxLeft < len(heights) {
				char |= brailleColumn(heights[dataIdxLeft], rowBottomDot, rowTopDot, leftDots)
			}

			// Right column (second data point)
			if dataIdxRight < len(heights) {
				char |= brailleColumn(heights[dataIdxRight], rowBottomDot, rowTopDot, rightDots)
			}

			sb.WriteRune(char)
		}

		// Add label on first and last row
		var label string
		switch row {
		case 0:
			label = labelStyle.Render(maxLabel)
		case chartRows - 1:
			label = labelStyle.Render("0")
		default:
			label = labelStyle.Render("")
		}

		chartRow := c.color.Render(sb.String())
		content = append(content, label+" "+chartRow)
	}

	// Wrap in rounded border
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border)

	return borderStyle.Render(strings.Join(content, "\n"))
}

// brailleColumn returns the braille bits for a single column based on height
func brailleColumn(h, rowBottom, rowTop int, dots [4]rune) rune {
	if h <= rowBottom {
		return 0
	}

	var bits rune
	dotsToFill := min(h-rowBottom, 4)
	for i := range dotsToFill {
		bits |= dots[i]
	}
	return bits
}

// SlidingSum computes the sum of each point and the preceding window-1 points.
// Missing values before the start of data are treated as zero.
// Returns same length as input.
func SlidingSum(data []float64, window int) []float64 {
	if len(data) == 0 || window <= 0 {
		return data
	}

	result := make([]float64, len(data))
	for i := range data {
		var sum float64
		start := max(0, i-window+1)
		for j := start; j <= i; j++ {
			sum += data[j]
		}
		result[i] = sum
	}
	return result
}

// Helpers

func maxValue(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
