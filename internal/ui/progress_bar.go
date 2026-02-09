package ui

import (
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var blocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

const pulseWidth = 48 // 6 chars

func TerminalBackgroundColor() color.Color {
	bg, err := lipgloss.BackgroundColor(os.Stdin, os.Stdout)
	if err != nil {
		// Fall back to black if detection fails
		return Colors.Background
	}
	return bg
}

type ProgressBar struct {
	Width      int
	Total      float64
	Current    float64
	Color      color.Color
	Background color.Color
	Pulsing    bool

	pulsePos int
	pulseDir int
}

type progressBarTickMsg struct{}

func NewProgressBar(width int, clr color.Color) ProgressBar {
	return ProgressBar{
		Width:    width,
		Color:    clr,
		pulseDir: 1,
	}
}

func (p ProgressBar) Init() tea.Cmd {
	if p.Pulsing {
		return p.tick()
	}
	return nil
}

func (p ProgressBar) Update(msg tea.Msg) (ProgressBar, tea.Cmd) {
	if !p.Pulsing {
		return p, nil
	}

	switch msg.(type) {
	case progressBarTickMsg:
		totalUnits := p.Width * 8
		maxPos := totalUnits + pulseWidth - 2

		p.pulsePos += p.pulseDir * 2

		if p.pulsePos >= maxPos {
			p.pulsePos = maxPos
			p.pulseDir = -1
		} else if p.pulsePos <= 0 {
			p.pulsePos = 0
			p.pulseDir = 1
		}

		return p, p.tick()
	}
	return p, nil
}

func (p ProgressBar) View() string {
	if p.Width <= 0 {
		return ""
	}

	var startUnits, endUnits int

	if p.Pulsing {
		startUnits, endUnits = p.pulseExtent()
	} else {
		percent := 0.0
		if p.Total > 0 {
			percent = max(0, min(1, p.Current/p.Total))
		}
		totalUnits := p.Width * 8
		startUnits = 0
		endUnits = int(percent * float64(totalUnits))
	}

	return p.renderBar(startUnits, endUnits)
}

// Private

func (p ProgressBar) tick() tea.Cmd {
	return tea.Tick(20*time.Millisecond, func(t time.Time) tea.Msg {
		return progressBarTickMsg{}
	})
}

func (p ProgressBar) pulseExtent() (int, int) {
	totalUnits := p.Width * 8

	// The blob's left edge position, which can be negative (entering from left)
	leftEdge := p.pulsePos - (pulseWidth - 1)
	rightEdge := leftEdge + pulseWidth

	// Clamp to visible area
	startUnits := max(0, leftEdge)
	endUnits := min(totalUnits, rightEdge)

	return startUnits, endUnits
}

func (p ProgressBar) renderBar(startUnits, endUnits int) string {
	var result strings.Builder

	normalStyle := lipgloss.NewStyle().Foreground(p.Color)
	invertedStyle := lipgloss.NewStyle().Background(p.Color).Foreground(p.Background)

	for i := range p.Width {
		charStart := i * 8
		charEnd := (i + 1) * 8

		if charEnd <= startUnits || charStart >= endUnits {
			result.WriteRune(' ')
		} else if charStart >= startUnits && charEnd <= endUnits {
			result.WriteString(normalStyle.Render(string(blocks[8])))
		} else if charStart < startUnits {
			// Left edge of filled region: simulate right-side block with inverted colors
			filled := charEnd - startUnits
			// Use complement block (8-filled) with background color instead
			result.WriteString(invertedStyle.Render(string(blocks[8-filled])))
		} else {
			// Right edge of filled region: use left-side blocks normally
			filled := endUnits - charStart
			result.WriteString(normalStyle.Render(string(blocks[filled])))
		}
	}

	return result.String()
}
