package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
)

type settingsFormField int

const (
	settingsFieldHostname settingsFormField = iota
	settingsFieldTLS
	settingsFieldSaveButton
	settingsFieldCancelButton
	settingsFieldCount
)

type SettingsFormSubmitMsg struct {
	Hostname   string
	TLSEnabled bool
}

type SettingsFormCancelMsg struct{}

type SettingsForm struct {
	width, height int
	focused       settingsFormField
	hostnameInput textinput.Model
	tlsEnabled    bool
}

func NewSettingsForm(settings docker.ApplicationSettings) SettingsForm {
	hostname := textinput.New()
	hostname.Placeholder = "app.example.com"
	hostname.Prompt = ""
	hostname.CharLimit = 256
	hostname.SetValue(settings.Host)
	hostname.Focus()

	return SettingsForm{
		focused:       settingsFieldHostname,
		hostnameInput: hostname,
		tlsEnabled:    !settings.DisableTLS,
	}
}

func (m SettingsForm) Init() tea.Cmd {
	return nil
}

func (m SettingsForm) Update(msg tea.Msg) (SettingsForm, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inputWidth := min(m.width-4, 60)
		m.hostnameInput.SetWidth(inputWidth)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			return m.focusNext()
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			return m.focusPrev()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m.handleEnter()
		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))) && m.focused == settingsFieldTLS:
			m.tlsEnabled = !m.tlsEnabled
			return m, nil
		}
	}

	if m.focused == settingsFieldHostname {
		var cmd tea.Cmd
		m.hostnameInput, cmd = m.hostnameInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m SettingsForm) View() string {
	hostnameLabel := Styles.Label.Render("Hostname")
	hostnameField := Styles.Focus(Styles.Input, m.focused == settingsFieldHostname).
		Render(m.hostnameInput.View())

	tlsLabel := Styles.Label.Render("TLS")
	tlsCheckbox := "[x]"
	if !m.tlsEnabled {
		tlsCheckbox = "[ ]"
	}
	tlsField := Styles.Focus(Styles.Input, m.focused == settingsFieldTLS).
		Render(tlsCheckbox + " Enabled")

	saveButton := Styles.Focus(Styles.ButtonPrimary, m.focused == settingsFieldSaveButton).
		Render("Save")
	cancelButton := Styles.Focus(Styles.Button, m.focused == settingsFieldCancelButton).
		Render("Cancel")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveButton, cancelButton)

	form := lipgloss.JoinVertical(lipgloss.Left,
		hostnameLabel,
		hostnameField,
		tlsLabel,
		tlsField,
		"",
		buttons,
	)

	return form
}

// Private

func (m SettingsForm) focusNext() (SettingsForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused + 1) % settingsFieldCount
	return m.focusCurrent()
}

func (m SettingsForm) focusPrev() (SettingsForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused - 1 + settingsFieldCount) % settingsFieldCount
	return m.focusCurrent()
}

func (m *SettingsForm) blurCurrent() {
	if m.focused == settingsFieldHostname {
		m.hostnameInput.Blur()
	}
}

func (m SettingsForm) focusCurrent() (SettingsForm, tea.Cmd) {
	var cmd tea.Cmd
	if m.focused == settingsFieldHostname {
		cmd = m.hostnameInput.Focus()
	}
	return m, cmd
}

func (m SettingsForm) handleEnter() (SettingsForm, tea.Cmd) {
	switch m.focused {
	case settingsFieldHostname, settingsFieldTLS:
		return m.focusNext()
	case settingsFieldSaveButton:
		return m, func() tea.Msg {
			return SettingsFormSubmitMsg{
				Hostname:   m.hostnameInput.Value(),
				TLSEnabled: m.tlsEnabled,
			}
		}
	case settingsFieldCancelButton:
		return m, func() tea.Msg { return SettingsFormCancelMsg{} }
	}
	return m, nil
}
