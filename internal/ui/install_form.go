package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
)

type installFormField int

const (
	fieldImageRef installFormField = iota
	fieldHostname
	fieldInstallButton
	fieldCancelButton
	fieldCount
)

type InstallFormSubmitMsg struct {
	ImageRef string
	Hostname string
}

type InstallFormCancelMsg struct{}

type InstallForm struct {
	width, height  int
	focused        installFormField
	imageRefInput  textinput.Model
	hostnameInput  textinput.Model
	lastAppName    string
}

func NewInstallForm() InstallForm {
	imageRef := textinput.New()
	imageRef.Placeholder = "user/repo:tag"
	imageRef.Prompt = ""
	imageRef.CharLimit = 256
	imageRef.Focus()

	hostname := textinput.New()
	hostname.Placeholder = "app.example.com"
	hostname.Prompt = ""
	hostname.CharLimit = 256

	return InstallForm{
		focused:       fieldImageRef,
		imageRefInput: imageRef,
		hostnameInput: hostname,
	}
}

func (m InstallForm) Init() tea.Cmd {
	return nil
}

func (m InstallForm) Update(msg tea.Msg) (InstallForm, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inputWidth := min(m.width-4, 60)
		m.imageRefInput.SetWidth(inputWidth)
		m.hostnameInput.SetWidth(inputWidth)

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

	// Update the focused input
	switch m.focused {
	case fieldImageRef:
		var cmd tea.Cmd
		m.imageRefInput, cmd = m.imageRefInput.Update(msg)
		cmds = append(cmds, cmd)
	case fieldHostname:
		var cmd tea.Cmd
		m.hostnameInput, cmd = m.hostnameInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m InstallForm) View() string {
	focusedColor := lipgloss.Color("#FFA500")

	labelStyle := lipgloss.NewStyle().Bold(true)
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6272a4")).
		Padding(0, 1).
		MarginBottom(1)

	focusedInputStyle := inputStyle.BorderForeground(focusedColor)

	// Image ref field
	imageRefLabel := labelStyle.Render("Image")
	imageRefStyle := inputStyle
	if m.focused == fieldImageRef {
		imageRefStyle = focusedInputStyle
	}
	imageRefField := imageRefStyle.Render(m.imageRefInput.View())

	// Hostname field
	hostnameLabel := labelStyle.Render("Hostname")
	hostnameStyle := inputStyle
	if m.focused == fieldHostname {
		hostnameStyle = focusedInputStyle
	}
	hostnameField := hostnameStyle.Render(m.hostnameInput.View())

	// Buttons
	buttonStyle := lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1).
		Border(lipgloss.RoundedBorder())

	primaryButtonStyle := buttonStyle.
		BorderForeground(Colors.Primary)

	secondaryButtonStyle := buttonStyle.
		BorderForeground(lipgloss.Color("#6272a4"))

	var installButton, cancelButton string
	if m.focused == fieldInstallButton {
		installButton = buttonStyle.BorderForeground(focusedColor).Render("Install")
	} else {
		installButton = primaryButtonStyle.Render("Install")
	}

	if m.focused == fieldCancelButton {
		cancelButton = buttonStyle.BorderForeground(focusedColor).Render("Cancel")
	} else {
		cancelButton = secondaryButtonStyle.Render("Cancel")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, installButton, cancelButton)

	form := lipgloss.JoinVertical(lipgloss.Left,
		imageRefLabel,
		imageRefField,
		hostnameLabel,
		hostnameField,
		"",
		buttons,
	)

	return form
}

func (m InstallForm) ImageRef() string {
	return m.imageRefInput.Value()
}

func (m InstallForm) Hostname() string {
	return m.hostnameInput.Value()
}

// Private

func (m InstallForm) focusNext() (InstallForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused + 1) % fieldCount
	return m.focusCurrent()
}

func (m InstallForm) focusPrev() (InstallForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused - 1 + fieldCount) % fieldCount
	return m.focusCurrent()
}

func (m *InstallForm) blurCurrent() {
	switch m.focused {
	case fieldImageRef:
		m.imageRefInput.Blur()
		m.updateHostnamePlaceholder()
	case fieldHostname:
		m.hostnameInput.Blur()
	}
}

func (m InstallForm) focusCurrent() (InstallForm, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focused {
	case fieldImageRef:
		cmd = m.imageRefInput.Focus()
	case fieldHostname:
		cmd = m.hostnameInput.Focus()
	}
	return m, cmd
}

func (m InstallForm) handleEnter() (InstallForm, tea.Cmd) {
	switch m.focused {
	case fieldImageRef, fieldHostname:
		return m.focusNext()
	case fieldInstallButton:
		return m, func() tea.Msg {
			return InstallFormSubmitMsg{
				ImageRef: m.imageRefInput.Value(),
				Hostname: m.hostnameInput.Value(),
			}
		}
	case fieldCancelButton:
		return m, func() tea.Msg { return InstallFormCancelMsg{} }
	}
	return m, nil
}

func (m *InstallForm) updateHostnamePlaceholder() {
	appName := docker.NameFromImageRef(m.imageRefInput.Value())
	if appName != m.lastAppName && appName != "" {
		m.hostnameInput.Placeholder = appName + ".example.com"
		m.lastAppName = appName
	}
}
