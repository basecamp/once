package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
)

const (
	emailServerField = iota
	emailPortField
	emailUsernameField
	emailPasswordField
	emailFromField
)

type SettingsFormEmail struct {
	settingsFormBase
}

func NewSettingsFormEmail(settings docker.ApplicationSettings) SettingsFormEmail {
	serverField := NewTextField("smtp.example.com")
	serverField.SetValue(settings.SMTP.Server)

	portField := NewTextField("587")
	portField.SetCharLimit(5)
	portField.SetValue(settings.SMTP.Port)

	usernameField := NewTextField("user@example.com")
	usernameField.SetValue(settings.SMTP.Username)

	passwordField := NewTextField("password")
	passwordField.SetEchoPassword()
	passwordField.SetValue(settings.SMTP.Password)

	fromField := NewTextField("noreply@example.com")
	fromField.SetValue(settings.SMTP.From)

	m := SettingsFormEmail{
		settingsFormBase: settingsFormBase{
			title: "Email",
			form: NewForm("Done",
				FormItem{Label: "SMTP Server", Field: serverField},
				FormItem{Label: "SMTP Port", Field: portField},
				FormItem{Label: "SMTP Username", Field: usernameField},
				FormItem{Label: "SMTP Password", Field: passwordField},
				FormItem{Label: "SMTP From", Field: fromField},
			),
		},
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.SMTP.Server = f.TextField(emailServerField).Value()
		s.SMTP.Port = f.TextField(emailPortField).Value()
		s.SMTP.Username = f.TextField(emailUsernameField).Value()
		s.SMTP.Password = f.TextField(emailPasswordField).Value()
		s.SMTP.From = f.TextField(emailFromField).Value()
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormEmail) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
