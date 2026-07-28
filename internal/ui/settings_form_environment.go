package ui

import (
	"fmt"
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/pkg/docker"
	"github.com/basecamp/once/pkg/mouse"
)

type SettingsFormEnvironment struct {
	settingsFormBase
	width    int
	height   int
	scroll   int
	settings docker.ApplicationSettings
}

func NewSettingsFormEnvironment(settings docker.ApplicationSettings) SettingsFormEnvironment {
	var items []FormItem

	keys := slices.Sorted(maps.Keys(settings.EnvVars))
	for _, k := range keys {
		items = append(items, newEnvKeyItem(k), newEnvValueItem(settings.EnvVars[k]))
	}
	items = append(items, newEnvKeyItem(""), newEnvValueItem(""))

	m := SettingsFormEnvironment{
		settingsFormBase: settingsFormBase{
			title: "Environment",
			form:  NewForm("Done", items...),
		},
		settings: settings,
	}

	m.form.OnRebuild(func(f *Form) {
		lastKeyIdx := f.ItemCount() - 2
		if lastKeyIdx >= 0 && f.TextField(lastKeyIdx).Value() != "" {
			f.AppendItems(newEnvKeyItem(""), newEnvValueItem(""))
		}
	})

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.EnvVars = nil
		for i := 0; i < f.ItemCount(); i += 2 {
			k := f.TextField(i).Value()
			if k == "" {
				continue
			}
			if s.EnvVars == nil {
				s.EnvVars = make(map[string]string)
			}
			s.EnvVars[k] = f.TextField(i + 1).Value()
		}
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})

	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormEnvironment) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}

	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	m.setFieldWidths()
	m.adjustScroll()
	return m, cmd
}

func (m SettingsFormEnvironment) View() string {
	return m.renderContent()
}

// Private

func (m SettingsFormEnvironment) rowCount() int {
	return m.form.ItemCount() / 2
}

func (m SettingsFormEnvironment) columnWidths() (int, int) {
	totalWidth := max(min(m.width, 64), 6)
	keyWidth := totalWidth / 3
	valueWidth := totalWidth - keyWidth - 1
	return keyWidth, valueWidth
}

func (m SettingsFormEnvironment) setFieldWidths() {
	keyWidth, valueWidth := m.columnWidths()
	for i := range m.form.ItemCount() {
		if i%2 == 0 {
			m.form.TextField(i).SetWidth(max(keyWidth-4, 1))
		} else {
			m.form.TextField(i).SetWidth(max(valueWidth-4, 1))
		}
	}
}

func (m *SettingsFormEnvironment) adjustScroll() {
	maxVisible := m.maxVisibleRows()
	if maxVisible <= 0 {
		return
	}

	focusedRow := m.focusedRow()
	if focusedRow < 0 {
		focusedRow = m.rowCount() - 1
	}

	if focusedRow < m.scroll {
		m.scroll = focusedRow
	}
	if focusedRow >= m.scroll+maxVisible {
		m.scroll = focusedRow - maxVisible + 1
	}
}

func (m SettingsFormEnvironment) focusedRow() int {
	focused := m.form.Focused()
	if focused < m.form.ItemCount() {
		return focused / 2
	}
	return -1
}

func (m SettingsFormEnvironment) maxVisibleRows() int {
	if m.height <= 0 {
		return m.rowCount()
	}
	// Parent chrome: title (2) + help (1) + status+gap (2) = 5
	// Form chrome: headers+gap (2) + buttons (3) + button gap (1) = 6
	available := m.height - 11
	rowHeight := 4 // bordered input (3) + gap (1)
	visible := available / rowHeight
	return max(visible, 1)
}

func (m SettingsFormEnvironment) renderContent() string {
	keyWidth, valueWidth := m.columnWidths()

	headerStyle := lipgloss.NewStyle().Bold(true)
	keyHeader := headerStyle.Width(keyWidth).Render("Key")
	valueHeader := headerStyle.Width(valueWidth).Render("Value")
	header := lipgloss.JoinHorizontal(lipgloss.Top, keyHeader, " ", valueHeader)

	var parts []string
	parts = append(parts, header, "")

	maxVisible := m.maxVisibleRows()
	rows := m.rowCount()
	end := min(m.scroll+maxVisible, rows)

	if m.scroll > 0 {
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↑ %d more above", m.scroll))
		parts = append(parts, indicator)
	}

	focused := m.form.Focused()
	for i := m.scroll; i < end; i++ {
		keyIdx := i * 2
		valIdx := i*2 + 1

		keyStyle := Styles.Focus(Styles.Input, focused == keyIdx).Width(keyWidth)
		valueStyle := Styles.Focus(Styles.Input, focused == valIdx).Width(valueWidth)

		keyView := mouse.Mark(fieldTarget(keyIdx), keyStyle.Render(m.form.TextField(keyIdx).View()))
		valueView := mouse.Mark(fieldTarget(valIdx), valueStyle.Render(m.form.TextField(valIdx).View()))

		rowView := lipgloss.JoinHorizontal(lipgloss.Top, keyView, " ", valueView)
		parts = append(parts, rowView, "")
	}

	if end < rows {
		remaining := rows - end
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↓ %d more below", remaining))
		parts = append(parts, indicator)
	}

	submitIdx := m.form.ItemCount()
	cancelIdx := m.form.ItemCount() + 1
	submitButton := mouse.Mark("submit", Styles.Focus(Styles.ButtonPrimary, focused == submitIdx).
		Render("Done"))
	cancelButton := mouse.Mark("cancel", Styles.Focus(Styles.Button, focused == cancelIdx).
		Render("Cancel"))
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, submitButton, cancelButton)
	parts = append(parts, buttons)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// Helpers

func newEnvKeyItem(value string) FormItem {
	f := NewTextField("KEY")
	f.SetValue(value)
	f.SetCharLimit(256)
	return FormItem{Field: f}
}

func newEnvValueItem(value string) FormItem {
	f := NewTextField("value")
	f.SetValue(value)
	f.SetCharLimit(1024)
	return FormItem{Field: f}
}
