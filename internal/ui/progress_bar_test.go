package ui

import (
	"image/color"
	"strings"
	"testing"

	"github.com/basecamp/gliff/components"
)

func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func TestProgressBar(t *testing.T) {
	white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	assertOutput := func(p *components.ProgressBar, want string) {
		t.Helper()
		got := stripAnsi(p.Render())
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	t.Run("zero progress", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 100, Current: 0, Color: white}, "          ")
	})

	t.Run("full progress", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 100, Current: 100, Color: white}, "██████████")
	})

	t.Run("half progress", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 100, Current: 50, Color: white}, "█████     ")
	})

	t.Run("fractional progress", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 100, Current: 37.5, Color: white}, "███▊      ")
	})

	t.Run("clamp over 100%", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 100, Current: 150, Color: white}, "██████████")
	})

	t.Run("zero width", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 0, Total: 100, Current: 50, Color: white}, "")
	})

	t.Run("zero total", func(t *testing.T) {
		assertOutput(&components.ProgressBar{Width: 10, Total: 0, Current: 50, Color: white}, "          ")
	})
}
