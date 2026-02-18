package ui

import (
	"context"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

const updatesAutoUpdateField = 0

type SettingsFormUpdates struct {
	app        *docker.Application
	settings   docker.ApplicationSettings
	form       *Form
	lastResult *docker.OperationResult
}

func NewSettingsFormUpdates(app *docker.Application, lastResult *docker.OperationResult) *SettingsFormUpdates {
	autoUpdateField := NewCheckboxField("Automatically apply updates", app.Settings.AutoUpdate)

	m := &SettingsFormUpdates{
		app:      app,
		settings: app.Settings,
		form: NewForm("Done",
			FormItem{Label: "Updates", Field: autoUpdateField},
		),
		lastResult: lastResult,
	}

	m.form.SetActionButton("Check for updates", func() tui.Msg {
		return settingsRunActionMsg{action: func() (string, error) {
			changed, err := app.Update(context.Background(), nil)
			if err != nil {
				return "", err
			}
			if !changed {
				return "Already running the latest version", nil
			}
			return "Update complete", nil
		}}
	})
	m.form.OnSubmit(func() tui.Cmd {
		m.settings.AutoUpdate = m.form.CheckboxField(updatesAutoUpdateField).Checked()
		return func() tui.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m *SettingsFormUpdates) Title() string {
	return "Updates"
}

func (m *SettingsFormUpdates) Init() tui.Cmd {
	return m.form.Init()
}

func (m *SettingsFormUpdates) Update(msg tui.Msg) tui.Cmd {
	return m.form.Update(msg)
}

func (m *SettingsFormUpdates) Render() string {
	return m.form.Render()
}

func (m *SettingsFormUpdates) StatusLine() string {
	return formatOperationStatus("checked", m.lastResult)
}
