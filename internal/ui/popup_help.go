package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/mouse"
)

var popupHelpKeys = struct {
	Close key.Binding
}{
	Close: NewKeyBinding("enter", "esc", "f1"),
}

type PopupHelpCloseMsg struct{}

type PopupHelp struct {
	title    string
	viewport viewport.Model
	width    int
	height   int
}

func NewPopupHelp(title, content string, termWidth, termHeight int) PopupHelp {
	popupWidth := min(termWidth-4, 60)
	boxStyle := popupHelpBoxStyle()

	// Measure the chrome height by rendering with a placeholder viewport
	innerWidth := popupWidth - boxStyle.GetHorizontalFrameSize()
	chrome := popupHelpChrome(title, "placeholder", true)
	chromeHeight := lipgloss.Height(boxStyle.Render(chrome)) - 1 // minus the placeholder line

	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(content)
	wrappedLines := lipgloss.Height(wrapped)
	vpHeight := min(wrappedLines, max(termHeight-chromeHeight-2, 3)) // -2 for outer margin

	vp := viewport.New()
	vp.SetWidth(innerWidth)
	vp.SetHeight(vpHeight)
	vp.SetContent(wrapped)
	vp.MouseWheelEnabled = true

	return PopupHelp{
		title:    title,
		viewport: vp,
		width:    popupWidth,
		height:   vpHeight,
	}
}

func (m PopupHelp) Update(msg tea.Msg) (PopupHelp, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, popupHelpKeys.Close) {
			return m, func() tea.Msg { return PopupHelpCloseMsg{} }
		}

		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case MouseEvent:
		if msg.IsClick && msg.Target == "ok" {
			return m, func() tea.Msg { return PopupHelpCloseMsg{} }
		}
	}

	return m, nil
}

func (m PopupHelp) View() string {
	scrollable := m.viewport.TotalLineCount() > m.viewport.Height()
	content := popupHelpChrome(m.title, m.viewport.View(), scrollable)
	return popupHelpBoxStyle().Render(content)
}

// Private

func popupHelpChrome(title, body string, scrollable bool) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary)

	buttonStyle := Styles.ButtonPrimary.BorderForeground(Colors.Focused)
	okButton := mouse.Mark("ok", buttonStyle.Render("OK"))

	parts := []string{titleStyle.Render(title), "", body}
	if scrollable {
		hint := lipgloss.NewStyle().Foreground(Colors.Muted).Render("↑↓ to scroll")
		parts = append(parts, hint)
	}
	parts = append(parts, "", okButton)

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

func popupHelpBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 3)
}
