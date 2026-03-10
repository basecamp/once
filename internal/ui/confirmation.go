package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/mouse"
)

var confirmationKeys = struct {
	Toggle key.Binding
	Enter  key.Binding
	Esc    key.Binding
}{
	Toggle: NewKeyBinding("tab", "shift+tab"),
	Enter:  NewKeyBinding("enter"),
	Esc:    NewKeyBinding("esc"),
}

type ConfirmationConfirmMsg struct{}
type ConfirmationCancelMsg struct{}

type Confirmation struct {
	message      string
	confirmLabel string
	focused      int // 0 = confirm, 1 = cancel
}

func NewConfirmation(message, confirmLabel string) Confirmation {
	return Confirmation{
		message:      message,
		confirmLabel: confirmLabel,
		focused:      1, // default to cancel for safety
	}
}

func (m Confirmation) Update(msg tea.Msg) (Confirmation, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, confirmationKeys.Toggle):
			m.focused = 1 - m.focused
		case key.Matches(msg, confirmationKeys.Enter):
			return m, m.activate()
		case key.Matches(msg, confirmationKeys.Esc):
			return m, func() tea.Msg { return ConfirmationCancelMsg{} }
		}

	case MouseEvent:
		if msg.IsClick {
			switch msg.Target {
			case "confirm":
				return m, func() tea.Msg { return ConfirmationConfirmMsg{} }
			case "cancel":
				return m, func() tea.Msg { return ConfirmationCancelMsg{} }
			}
		}
	}

	return m, nil
}

func (m Confirmation) View() string {
	messageView := lipgloss.NewStyle().Bold(true).Render(m.message)

	confirmStyle := Styles.Button
	if m.focused == 0 {
		confirmStyle = confirmStyle.BorderForeground(Colors.Error)
	}
	confirmButton := mouse.Mark("confirm", confirmStyle.Render(m.confirmLabel))

	cancelStyle := Styles.Button
	if m.focused == 1 {
		cancelStyle = cancelStyle.BorderForeground(Colors.Focused)
	}
	cancelButton := mouse.Mark("cancel", cancelStyle.Render("Cancel"))

	buttonRow := lipgloss.JoinHorizontal(lipgloss.Center, confirmButton, cancelButton)

	return lipgloss.JoinVertical(lipgloss.Center,
		messageView,
		"",
		buttonRow,
	)
}

// Private

func (m Confirmation) activate() tea.Cmd {
	if m.focused == 0 {
		return func() tea.Msg { return ConfirmationConfirmMsg{} }
	}
	return func() tea.Msg { return ConfirmationCancelMsg{} }
}
