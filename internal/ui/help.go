package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"
)

type Help struct {
	width    int
	bindings []KeyBinding
}

func NewHelp() Help {
	return Help{}
}

func (h *Help) SetWidth(w int) {
	h.width = w
}

func (h *Help) Update(msg tui.Msg) tui.Cmd {
	if msg, ok := msg.(tui.MouseMsg); ok {
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			for i, kb := range h.bindings {
				if msg.Target == helpTarget(i) && len(kb.Keys) > 0 {
					spec := kb.Keys[0]
					return func() tui.Msg {
						return tui.KeyMsg{Key: tui.Key{Type: spec.Type, Rune: spec.Rune}}
					}
				}
			}
		}
	}
	return nil
}

func (h *Help) Render(bindings []KeyBinding) string {
	h.bindings = bindings

	keyStyle := lipgloss.NewStyle().Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(Colors.Border)
	separator := descStyle.Render(" • ")
	sepWidth := lipgloss.Width(separator)

	type helpItem struct {
		str   string
		width int
		index int
	}

	var items []helpItem
	for i, kb := range bindings {
		if kb.Help.Key == "" {
			continue
		}
		rendered := keyStyle.Render(kb.Help.Key) + " " + descStyle.Render(kb.Help.Desc)
		items = append(items, helpItem{str: rendered, width: lipgloss.Width(rendered), index: i})
	}

	if len(items) == 0 {
		return ""
	}

	maxWidth := h.width
	var lines []string
	var line strings.Builder
	lineWidth := 0

	for _, it := range items {
		if lineWidth > 0 && maxWidth > 0 && lineWidth+sepWidth+it.width > maxWidth {
			lines = append(lines, line.String())
			line.Reset()
			lineWidth = 0
		}
		if lineWidth > 0 {
			line.WriteString(separator)
			lineWidth += sepWidth
		}
		line.WriteString(tui.WithTarget(helpTarget(it.index), it.str))
		lineWidth += it.width
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

// Helpers

func helpTarget(i int) string {
	return "help:" + strconv.Itoa(i)
}
