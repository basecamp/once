package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
)

const updatesAutoUpdateField = 0

type SettingsFormUpdates struct {
	settingsFormBase
}

func NewSettingsFormUpdates(app *docker.Application, lastResult *docker.OperationResult) SettingsFormUpdates {
	autoUpdateField := NewCheckboxField("Automatically apply updates", app.Settings.AutoUpdate)

	m := SettingsFormUpdates{
		settingsFormBase: settingsFormBase{
			title: "Updates",
			form: NewForm("Done",
				FormItem{Label: "Updates", Field: autoUpdateField},
			),
		},
	}

	m.statusLine = func() string {
		return formatOperationStatus("checked", lastResult)
	}

	m.form.SetActionButton("Check for updates", func() tea.Msg {
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
	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := app.Settings
		s.AutoUpdate = f.CheckboxField(updatesAutoUpdateField).Checked()
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormUpdates) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
