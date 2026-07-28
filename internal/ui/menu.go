package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/pkg/mouse"
)

var menuKeys = struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
}{
	Up:     NewKeyBinding("up", "k"),
	Down:   NewKeyBinding("down", "j"),
	Select: NewKeyBinding("enter"),
}

type MenuItem struct {
	Label    string
	Key      int
	Shortcut key.Binding
}

type MenuSelectMsg struct{ Key int }

type Menu struct {
	items        []MenuItem
	selected     int
	width        int
	minWidth     int
	hasShortcuts bool
}

func NewMenu(items ...MenuItem) Menu {
	maxLabel := 0
	maxKey := 0
	hasShortcuts := false
	for _, item := range items {
		if w := lipgloss.Width(item.Label); w > maxLabel {
			maxLabel = w
		}
		if len(item.Shortcut.Keys()) > 0 {
			hasShortcuts = true
			if w := len(item.Shortcut.Help().Key); w > maxKey {
				maxKey = w
			}
		}
	}

	minWidth := maxLabel
	if hasShortcuts {
		minWidth = maxLabel + 2 + maxKey
	}

	return Menu{
		items:        items,
		minWidth:     minWidth,
		hasShortcuts: hasShortcuts,
	}
}

func (m *Menu) SetWidth(w int) {
	m.width = w
}

func (m Menu) Update(msg tea.Msg) (Menu, tea.Cmd) {
	count := len(m.items)
	if count == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case MouseEvent:
		if msg.IsClick {
			for i, item := range m.items {
				if msg.Target == menuItemTarget(i) {
					m.selected = i
					return m, m.selectItem(item.Key)
				}
			}
		}

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, menuKeys.Up):
			m.selected = (m.selected - 1 + count) % count
		case key.Matches(msg, menuKeys.Down):
			m.selected = (m.selected + 1) % count
		case key.Matches(msg, menuKeys.Select):
			return m, m.selectItem(m.items[m.selected].Key)
		default:
			for i, item := range m.items {
				if key.Matches(msg, item.Shortcut) {
					m.selected = i
					return m, m.selectItem(item.Key)
				}
			}
		}
	}

	return m, nil
}

func (m Menu) View() string {
	itemStyle := lipgloss.NewStyle()
	selectedStyle := lipgloss.NewStyle().Reverse(true)
	keyStyle := lipgloss.NewStyle().Foreground(Colors.Border)

	width := m.effectiveWidth()

	lines := make([]string, len(m.items))
	for i, item := range m.items {
		var line string
		if m.selected == i {
			line = selectedStyle.Render(item.Label)
		} else {
			line = itemStyle.Render(item.Label)
		}

		if width == 0 {
			lines[i] = mouse.Mark(menuItemTarget(i), line)
			continue
		}

		if m.hasShortcuts {
			keyText := keyStyle.Render(item.Shortcut.Help().Key)
			keyWidth := lipgloss.Width(keyText)
			line = lipgloss.NewStyle().Width(width-keyWidth).Render(line) + keyText
		} else {
			line = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(line)
		}

		lines[i] = mouse.Mark(menuItemTarget(i), line)
	}

	return strings.Join(lines, "\n")
}

// Private

func (m Menu) effectiveWidth() int {
	if m.width == 0 {
		return 0
	}
	return max(m.width, m.minWidth)
}

func (m Menu) selectItem(key int) tea.Cmd {
	return func() tea.Msg { return MenuSelectMsg{Key: key} }
}

// Helpers

func menuItemTarget(i int) string {
	return "menu-item:" + strconv.Itoa(i)
}
