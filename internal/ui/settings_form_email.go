package ui

import (
	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

const (
	emailServerField = iota
	emailPortField
	emailUsernameField
	emailPasswordField
	emailFromField
)

type SettingsFormEmail struct {
	settings docker.ApplicationSettings
	form     *Form
}

func NewSettingsFormEmail(settings docker.ApplicationSettings) *SettingsFormEmail {
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

	m := &SettingsFormEmail{
		settings: settings,
		form: NewForm("Done",
			FormItem{Label: "SMTP Server", Field: serverField},
			FormItem{Label: "SMTP Port", Field: portField},
			FormItem{Label: "SMTP Username", Field: usernameField},
			FormItem{Label: "SMTP Password", Field: passwordField},
			FormItem{Label: "SMTP From", Field: fromField},
		),
	}

	m.form.OnSubmit(func() tui.Cmd {
		m.settings.SMTP.Server = m.form.TextField(emailServerField).Value()
		m.settings.SMTP.Port = m.form.TextField(emailPortField).Value()
		m.settings.SMTP.Username = m.form.TextField(emailUsernameField).Value()
		m.settings.SMTP.Password = m.form.TextField(emailPasswordField).Value()
		m.settings.SMTP.From = m.form.TextField(emailFromField).Value()
		return func() tui.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m *SettingsFormEmail) Title() string {
	return "Email"
}

func (m *SettingsFormEmail) Init() tui.Cmd {
	return m.form.Init()
}

func (m *SettingsFormEmail) Update(msg tui.Msg) tui.Cmd {
	return m.form.Update(msg)
}

func (m *SettingsFormEmail) StatusLine() string { return "" }

func (m *SettingsFormEmail) Render() string {
	return m.form.Render()
}
