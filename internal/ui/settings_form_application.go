package ui

import (
	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

const (
	appImageField = iota
	appHostnameField
	appTLSField
)

type SettingsFormApplication struct {
	settings docker.ApplicationSettings
	form     *Form
}

func NewSettingsFormApplication(settings docker.ApplicationSettings) *SettingsFormApplication {
	imageField := NewTextField("user/repo:tag")
	imageField.SetValue(settings.Image)

	hostnameField := NewTextField("app.example.com")
	hostnameField.SetValue(settings.Host)

	tlsField := NewCheckboxField("Enabled", !settings.DisableTLS)
	tlsField.SetDisabledWhen(func() (bool, string) {
		if docker.IsLocalhost(hostnameField.Value()) {
			return true, "Not available for localhost"
		}
		return false, ""
	})

	m := &SettingsFormApplication{
		settings: settings,
		form: NewForm("Done",
			FormItem{Label: "Image", Field: imageField},
			FormItem{Label: "Hostname", Field: hostnameField},
			FormItem{Label: "TLS", Field: tlsField},
		),
	}

	m.form.OnSubmit(func() tui.Cmd {
		m.settings.Image = m.form.TextField(appImageField).Value()
		m.settings.Host = m.form.TextField(appHostnameField).Value()
		m.settings.DisableTLS = !m.form.CheckboxField(appTLSField).Checked()
		return func() tui.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m *SettingsFormApplication) Title() string {
	return "Application"
}

func (m *SettingsFormApplication) Init() tui.Cmd {
	return m.form.Init()
}

func (m *SettingsFormApplication) Update(msg tui.Msg) tui.Cmd {
	return m.form.Update(msg)
}

func (m *SettingsFormApplication) StatusLine() string { return "" }

func (m *SettingsFormApplication) Render() string {
	return m.form.Render()
}
