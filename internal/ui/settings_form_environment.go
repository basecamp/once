package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type environmentFormField int

const (
	environmentFieldDoneButton environmentFormField = iota
	environmentFieldCancelButton
	environmentFieldCount
)

type SettingsFormEnvironment struct {
	width, height int
	focused       environmentFormField
	settings      docker.ApplicationSettings
}

func NewSettingsFormEnvironment(settings docker.ApplicationSettings) SettingsFormEnvironment {
	return SettingsFormEnvironment{
		focused:  environmentFieldDoneButton,
		settings: settings,
	}
}

func (m SettingsFormEnvironment) Title() string {
	return "Environment"
}

func (m SettingsFormEnvironment) Init() tea.Cmd {
	return nil
}

func (m SettingsFormEnvironment) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			return m.focusNext()
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			return m.focusPrev()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m.handleEnter()
		}
	}

	return m, nil
}

func (m SettingsFormEnvironment) View() string {
	placeholder := lipgloss.NewStyle().
		Foreground(Colors.Border).
		Italic(true).
		Render("(Environment variable editing coming soon)")

	doneButton := Styles.Focus(Styles.ButtonPrimary, m.focused == environmentFieldDoneButton).
		Render("Done")
	cancelButton := Styles.Focus(Styles.Button, m.focused == environmentFieldCancelButton).
		Render("Cancel")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, doneButton, cancelButton)

	form := lipgloss.JoinVertical(lipgloss.Left,
		placeholder,
		"",
		buttons,
	)

	return form
}

// Private

func (m SettingsFormEnvironment) focusNext() (SettingsSection, tea.Cmd) {
	m.focused = (m.focused + 1) % environmentFieldCount
	return m, nil
}

func (m SettingsFormEnvironment) focusPrev() (SettingsSection, tea.Cmd) {
	m.focused = (m.focused - 1 + environmentFieldCount) % environmentFieldCount
	return m, nil
}

func (m SettingsFormEnvironment) handleEnter() (SettingsSection, tea.Cmd) {
	switch m.focused {
	case environmentFieldDoneButton:
		return m, func() tea.Msg { return SettingsSectionCancelMsg{} }
	case environmentFieldCancelButton:
		return m, func() tea.Msg { return SettingsSectionCancelMsg{} }
	}
	return m, nil
}
