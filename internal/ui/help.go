package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

type Help struct {
	model  help.Model
	prefix string
}

func NewHelp() Help {
	return Help{
		model:  help.New(),
		prefix: zone.NewPrefix(),
	}
}

func (h *Help) SetWidth(w int) {
	h.model.SetWidth(w)
}

func (h Help) View(k help.KeyMap) string {
	bindings := k.ShortHelp()
	if len(bindings) == 0 {
		return ""
	}

	separator := h.model.Styles.ShortSeparator.Inline(true).Render(h.model.ShortSeparator)
	sepWidth := lipgloss.Width(separator)

	type helpItem struct {
		str   string
		width int
	}
	var items []helpItem
	for i, kb := range bindings {
		if !kb.Enabled() {
			continue
		}
		rendered := h.model.Styles.ShortKey.Inline(true).Render(kb.Help().Key) + " " +
			h.model.Styles.ShortDesc.Inline(true).Render(kb.Help().Desc)
		str := zone.Mark(h.zoneID(i), rendered)
		items = append(items, helpItem{str: str, width: lipgloss.Width(str)})
	}

	maxWidth := h.model.Width()
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
		line.WriteString(it.str)
		lineWidth += it.width
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

func (h Help) Update(msg tea.MouseClickMsg, k help.KeyMap) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}

	for i, kb := range k.ShortHelp() {
		if !kb.Enabled() {
			continue
		}
		if zi := zone.Get(h.zoneID(i)); zi != nil && zi.InBounds(msg) {
			return func() tea.Msg { return keyPressFromBinding(kb) }
		}
	}

	return nil
}

// Private

func (h Help) zoneID(i int) string {
	return fmt.Sprintf("%shelp_%d", h.prefix, i)
}

// Helpers

func keyPressFromBinding(b key.Binding) tea.KeyPressMsg {
	keys := b.Keys()
	if len(keys) == 0 {
		return tea.KeyPressMsg{}
	}

	k := keys[0]

	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}

	if len(k) == 1 {
		r := rune(k[0])
		return tea.KeyPressMsg{Code: r, Text: k}
	}

	return tea.KeyPressMsg{}
}
