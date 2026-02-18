package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"
)

var menuKeys = struct {
	Up     KeyBinding
	Down   KeyBinding
	Select KeyBinding
}{
	Up:     NewKeyBinding(Key(tui.KeyUp), RuneKey('k')),
	Down:   NewKeyBinding(Key(tui.KeyDown), RuneKey('j')),
	Select: NewKeyBinding(Key(tui.KeyEnter)),
}

type MenuItem struct {
	Label    string
	Key      int
	Shortcut KeyBinding
}

type MenuSelectMsg struct{ Key int }

type Menu struct {
	items    []MenuItem
	selected int
	padWidth int
}

func NewMenu(items ...MenuItem) Menu {
	m := Menu{
		items: items,
	}
	m.measureItems()
	return m
}

func (m *Menu) Update(msg tui.Msg) tui.Cmd {
	count := len(m.items)
	if count == 0 {
		return nil
	}

	switch msg := msg.(type) {
	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			for i, item := range m.items {
				if msg.Target == menuItemTarget(i) {
					m.selected = i
					return m.selectItem(item.Key)
				}
			}
		}

	case tui.KeyMsg:
		switch {
		case menuKeys.Up.Matches(msg):
			m.selected = (m.selected - 1 + count) % count
		case menuKeys.Down.Matches(msg):
			m.selected = (m.selected + 1) % count
		case menuKeys.Select.Matches(msg):
			return m.selectItem(m.items[m.selected].Key)
		default:
			for i, item := range m.items {
				if item.Shortcut.Matches(msg) {
					m.selected = i
					return m.selectItem(item.Key)
				}
			}
		}
	}

	return nil
}

func (m *Menu) Render() string {
	itemStyle := lipgloss.NewStyle()
	selectedStyle := lipgloss.NewStyle().Reverse(true)
	keyStyle := lipgloss.NewStyle().Foreground(Colors.Border)

	lines := make([]string, len(m.items))
	for i, item := range m.items {
		padding := strings.Repeat(" ", m.padWidth-len(item.Label))
		shortcutStr := item.Shortcut.Help.Key
		styledKey := keyStyle.Render(shortcutStr)

		var line string
		if m.selected == i {
			line = selectedStyle.Render(item.Label) + padding + styledKey
		} else {
			line = itemStyle.Render(item.Label) + padding + styledKey
		}
		lines[i] = tui.WithTarget(menuItemTarget(i), line)
	}

	return strings.Join(lines, "\n")
}

// Private

func (m *Menu) measureItems() {
	maxLen := 0
	for _, item := range m.items {
		if len(item.Label) > maxLen {
			maxLen = len(item.Label)
		}
	}
	m.padWidth = maxLen + 2
}

func (m *Menu) selectItem(key int) tui.Cmd {
	return func() tui.Msg { return MenuSelectMsg{Key: key} }
}

// Helpers

func menuItemTarget(i int) string {
	return "menu-item:" + strconv.Itoa(i)
}
