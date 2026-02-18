package components

import (
	"image/color"
	"math/rand/v2"
	"time"

	"github.com/basecamp/gliff/tui"
)

type ProgressBusy struct {
	Width int
	Color color.Color

	pattern []rune
}

type ProgressBusyTickMsg struct{}

func NewProgressBusy(width int, clr color.Color) *ProgressBusy {
	return &ProgressBusy{
		Width:   width,
		Color:   clr,
		pattern: generateBraillePattern(width),
	}
}

func (p *ProgressBusy) Init() tui.Cmd {
	if p == nil {
		return nil
	}
	return p.tick()
}

func (p *ProgressBusy) Update(msg tui.Msg) tui.Cmd {
	switch msg.(type) {
	case ProgressBusyTickMsg:
		p.pattern = generateBraillePattern(p.Width)
		return p.tick()
	}
	return nil
}

func (p *ProgressBusy) Render() string {
	if p == nil || p.Width <= 0 {
		return ""
	}

	return colorToANSI(p.Color, false) + string(p.pattern) + "\x1b[0m"
}

// Private

func (p *ProgressBusy) tick() tui.Cmd {
	return tui.After(50*time.Millisecond, func(t time.Time) tui.Msg {
		return ProgressBusyTickMsg{}
	})
}

func generateBraillePattern(width int) []rune {
	pattern := make([]rune, width)
	for i := range pattern {
		// Braille patterns: U+2800 to U+28FF (256 patterns)
		pattern[i] = rune(0x2800 + rand.IntN(256))
	}
	return pattern
}
