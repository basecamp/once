package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelp_RenderBasic(t *testing.T) {
	bindings := []KeyBinding{
		NewKeyBinding(RuneKey('s')).WithHelp("s", "settings"),
		NewKeyBinding(RuneKey('g')).WithHelp("g", "logs"),
	}

	h := NewHelp()
	h.SetWidth(80)
	result := h.Render(bindings)

	assert.Contains(t, result, "s")
	assert.Contains(t, result, "settings")
	assert.Contains(t, result, "g")
	assert.Contains(t, result, "logs")
}

func TestHelp_RenderEmpty(t *testing.T) {
	h := NewHelp()
	h.SetWidth(80)

	result := h.Render(nil)
	assert.Empty(t, result)
}

func TestHelp_RenderSkipsEmptyHelp(t *testing.T) {
	bindings := []KeyBinding{
		NewKeyBinding(RuneKey('s')).WithHelp("s", "settings"),
		NewKeyBinding(RuneKey('x')), // no help text
		NewKeyBinding(RuneKey('g')).WithHelp("g", "logs"),
	}

	h := NewHelp()
	h.SetWidth(80)
	result := h.Render(bindings)

	assert.Contains(t, result, "settings")
	assert.Contains(t, result, "logs")
	// The binding without help should not appear
	assert.NotContains(t, result, "x ")
}

func TestHelp_RenderWraps(t *testing.T) {
	bindings := []KeyBinding{
		NewKeyBinding(RuneKey('a')).WithHelp("a", "aaaaaaaaa"),
		NewKeyBinding(RuneKey('b')).WithHelp("b", "bbbbbbbbb"),
		NewKeyBinding(RuneKey('c')).WithHelp("c", "ccccccccc"),
	}

	h := NewHelp()
	h.SetWidth(30) // narrow enough to force wrapping
	result := h.Render(bindings)

	lines := strings.Split(result, "\n")
	assert.Greater(t, len(lines), 1, "should wrap to multiple lines")
}
