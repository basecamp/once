package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

type SettingsFormEnvironment struct {
	settings docker.ApplicationSettings
	form     *Form
}

func NewSettingsFormEnvironment(settings docker.ApplicationSettings) *SettingsFormEnvironment {
	m := &SettingsFormEnvironment{
		settings: settings,
		form:     NewForm("Done"),
	}

	m.form.OnSubmit(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m *SettingsFormEnvironment) Title() string {
	return "Environment"
}

func (m *SettingsFormEnvironment) Init() tui.Cmd {
	return m.form.Init()
}

func (m *SettingsFormEnvironment) Update(msg tui.Msg) tui.Cmd {
	return m.form.Update(msg)
}

func (m *SettingsFormEnvironment) StatusLine() string { return "" }

func (m *SettingsFormEnvironment) Render() string {
	placeholder := lipgloss.NewStyle().
		Foreground(Colors.Border).
		Italic(true).
		Render("(Environment variable editing coming soon)")

	return lipgloss.JoinVertical(lipgloss.Left,
		placeholder,
		"",
		m.form.Render(),
	)
}
