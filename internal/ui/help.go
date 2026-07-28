package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/pkg/mouse"
)

type Help struct {
	width    int
	bindings []key.Binding
}

func NewHelp() Help {
	return Help{}
}

func (h *Help) SetWidth(w int) {
	h.width = w
}

func (h Help) Update(msg tea.Msg) (Help, tea.Cmd) {
	if msg, ok := msg.(MouseEvent); ok {
		if msg.IsClick {
			for i, kb := range h.bindings {
				keys := kb.Keys()
				if msg.Target == helpTarget(i) && len(keys) > 0 {
					first := keys[0]
					return h, func() tea.Msg {
						return tea.KeyPressMsg(tea.Key{Text: first})
					}
				}
			}
		}
	}
	return h, nil
}

func (h *Help) SetBindings(bindings []key.Binding) {
	h.bindings = bindings
}

func (h Help) Height() int {
	return strings.Count(h.View(), "\n") + 1
}

func (h Help) View() string {
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
	for i, kb := range h.bindings {
		help := kb.Help()
		if help.Key == "" {
			continue
		}
		rendered := keyStyle.Render(help.Key) + " " + descStyle.Render(help.Desc)
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
		line.WriteString(mouse.Mark(helpTarget(it.index), it.str))
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
