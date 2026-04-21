package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/mouse"
)

type SettingsFormMounts struct {
	settingsFormBase
	width    int
	height   int
	scroll   int
	settings docker.ApplicationSettings
}

func NewSettingsFormMounts(settings docker.ApplicationSettings) SettingsFormMounts {
	var items []FormItem

	for _, m := range settings.Mounts {
		items = append(items, newMountSourceItem(m.Source), newMountTargetItem(m.Target))
	}
	items = append(items, newMountSourceItem(""), newMountTargetItem(""))

	f := SettingsFormMounts{
		settingsFormBase: settingsFormBase{
			title: "Mounts",
			form:  NewForm("Done", items...),
		},
		settings: settings,
	}

	f.form.OnRebuild(func(form *Form) {
		lastSourceIdx := form.ItemCount() - 2
		if lastSourceIdx >= 0 && form.TextField(lastSourceIdx).Value() != "" {
			form.AppendItems(newMountSourceItem(""), newMountTargetItem(""))
		}
	})

	f.form.OnSubmit(func(form *Form) tea.Cmd {
		s := settings
		s.Mounts = nil
		for i := 0; i < form.ItemCount(); i += 2 {
			source := form.TextField(i).Value()
			if source == "" {
				continue
			}
			s.Mounts = append(s.Mounts, docker.MountSetting{
				Source: source,
				Target: form.TextField(i + 1).Value(),
			})
		}
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})

	f.form.OnCancel(func(form *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return f
}

func (m SettingsFormMounts) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
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

func (m SettingsFormMounts) View() string {
	return m.renderContent()
}

// Private

func (m SettingsFormMounts) rowCount() int {
	return m.form.ItemCount() / 2
}

func (m SettingsFormMounts) columnWidths() (int, int) {
	totalWidth := max(min(m.width, 64), 6)
	sourceWidth := totalWidth / 2
	targetWidth := totalWidth - sourceWidth - 1
	return sourceWidth, targetWidth
}

func (m SettingsFormMounts) setFieldWidths() {
	sourceWidth, targetWidth := m.columnWidths()
	for i := range m.form.ItemCount() {
		if i%2 == 0 {
			m.form.TextField(i).SetWidth(max(sourceWidth-4, 1))
		} else {
			m.form.TextField(i).SetWidth(max(targetWidth-4, 1))
		}
	}
}

func (m *SettingsFormMounts) adjustScroll() {
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

func (m SettingsFormMounts) focusedRow() int {
	focused := m.form.Focused()
	if focused < m.form.ItemCount() {
		return focused / 2
	}
	return -1
}

func (m SettingsFormMounts) maxVisibleRows() int {
	if m.height <= 0 {
		return m.rowCount()
	}
	available := m.height - 11
	rowHeight := 4
	visible := available / rowHeight
	return max(visible, 1)
}

func (m SettingsFormMounts) renderContent() string {
	sourceWidth, targetWidth := m.columnWidths()

	headerStyle := lipgloss.NewStyle().Bold(true)
	sourceHeader := headerStyle.Width(sourceWidth).Render("Source")
	targetHeader := headerStyle.Width(targetWidth).Render("Target")
	header := lipgloss.JoinHorizontal(lipgloss.Top, sourceHeader, " ", targetHeader)

	var parts []string
	parts = append(parts, header, "")

	maxVisible := m.maxVisibleRows()
	rows := m.rowCount()
	end := min(m.scroll+maxVisible, rows)

	if m.scroll > 0 {
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("\u2191 %d more above", m.scroll))
		parts = append(parts, indicator)
	}

	focused := m.form.Focused()
	for i := m.scroll; i < end; i++ {
		srcIdx := i * 2
		tgtIdx := i*2 + 1

		srcStyle := Styles.Focus(Styles.Input, focused == srcIdx).Width(sourceWidth)
		tgtStyle := Styles.Focus(Styles.Input, focused == tgtIdx).Width(targetWidth)

		srcView := mouse.Mark(fieldTarget(srcIdx), srcStyle.Render(m.form.TextField(srcIdx).View()))
		tgtView := mouse.Mark(fieldTarget(tgtIdx), tgtStyle.Render(m.form.TextField(tgtIdx).View()))

		rowView := lipgloss.JoinHorizontal(lipgloss.Top, srcView, " ", tgtView)
		parts = append(parts, rowView, "")
	}

	if end < rows {
		remaining := rows - end
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("\u2193 %d more below", remaining))
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

func newMountSourceItem(value string) FormItem {
	f := NewTextField("/host/path")
	f.SetValue(value)
	f.SetCharLimit(1024)
	return FormItem{Field: f}
}

func newMountTargetItem(value string) FormItem {
	f := NewTextField("/container/path")
	f.SetValue(value)
	f.SetCharLimit(1024)
	return FormItem{Field: f}
}
