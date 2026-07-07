package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
)

const (
	appImageField = iota
	appHostnameField
	appTLSField
)

type SettingsFormApplication struct {
	settingsFormBase
}

func NewSettingsFormApplication(settings docker.ApplicationSettings) SettingsFormApplication {
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

	m := SettingsFormApplication{
		settingsFormBase: settingsFormBase{
			title: "Application",
			form: NewForm("Done",
				FormItem{Label: "Image", Field: imageField, Required: true},
				FormItem{Label: "Hostname", Field: hostnameField, Required: true},
				FormItem{Label: "TLS", Field: tlsField},
			),
		},
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Image = f.TextField(appImageField).Value()
		s.Host = f.TextField(appHostnameField).Value()
		s.DisableTLS = !f.CheckboxField(appTLSField).Checked()
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormApplication) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
