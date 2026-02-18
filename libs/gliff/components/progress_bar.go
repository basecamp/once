package components

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/basecamp/gliff/tui"
)

var blocks = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

const pulseWidth = 48 // 6 chars

func TerminalBackgroundColor() color.Color {
	return color.Black
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

type ProgressBarTickMsg struct{}

func NewProgressBar(width int, clr color.Color) *ProgressBar {
	return &ProgressBar{
		Width:    width,
		Color:    clr,
		pulseDir: 1,
	}
}

func (p *ProgressBar) Init() tui.Cmd {
	if p.Pulsing {
		return p.tick()
	}
	return nil
}

func (p *ProgressBar) Update(msg tui.Msg) tui.Cmd {
	if !p.Pulsing {
		return nil
	}

	switch msg.(type) {
	case ProgressBarTickMsg:
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

		return p.tick()
	}
	return nil
}

func (p *ProgressBar) Render() string {
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

func (p *ProgressBar) tick() tui.Cmd {
	return tui.After(20*time.Millisecond, func(t time.Time) tui.Msg {
		return ProgressBarTickMsg{}
	})
}

func (p *ProgressBar) pulseExtent() (int, int) {
	totalUnits := p.Width * 8

	// The blob's left edge position, which can be negative (entering from left)
	leftEdge := p.pulsePos - (pulseWidth - 1)
	rightEdge := leftEdge + pulseWidth

	// Clamp to visible area
	startUnits := max(0, leftEdge)
	endUnits := min(totalUnits, rightEdge)

	return startUnits, endUnits
}

func (p *ProgressBar) renderBar(startUnits, endUnits int) string {
	var result strings.Builder

	fgCode := colorToANSI(p.Color, false)
	fgBgCode := colorToANSI(p.Color, true) + colorToANSI(p.Background, false)

	for i := range p.Width {
		charStart := i * 8
		charEnd := (i + 1) * 8

		if charEnd <= startUnits || charStart >= endUnits {
			result.WriteRune(' ')
		} else if charStart >= startUnits && charEnd <= endUnits {
			result.WriteString(fgCode + string(blocks[8]) + "\x1b[0m")
		} else if charStart < startUnits {
			filled := charEnd - startUnits
			result.WriteString(fgBgCode + string(blocks[8-filled]) + "\x1b[0m")
		} else {
			filled := endUnits - charStart
			result.WriteString(fgCode + string(blocks[filled]) + "\x1b[0m")
		}
	}

	return result.String()
}

// Helpers

func colorToANSI(c color.Color, background bool) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	r8, g8, b8 := r>>8, g>>8, b>>8
	if background {
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r8, g8, b8)
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r8, g8, b8)
}
