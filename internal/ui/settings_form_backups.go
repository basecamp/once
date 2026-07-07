package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
)

const (
	backupsPathField = iota
	backupsAutoBackupField
)

type SettingsFormBackups struct {
	settingsFormBase
}

func NewSettingsFormBackups(app *docker.Application, lastResult *docker.OperationResult) SettingsFormBackups {
	pathField := NewTextField("/path/to/backups")
	pathField.SetValue(app.Settings.Backup.Path)

	autoBackupField := NewCheckboxField("Automatically create backups", app.Settings.Backup.AutoBackup)

	m := SettingsFormBackups{
		settingsFormBase: settingsFormBase{
			title: "Backups",
			form: NewForm("Done",
				FormItem{Label: "Backup location", Field: pathField},
				FormItem{Label: "Backups", Field: autoBackupField},
			),
		},
	}

	m.statusLine = func() string {
		return formatOperationStatus("backup", lastResult)
	}

	m.form.SetActionButton("Run backup now", func() tea.Msg {
		return settingsRunActionMsg{action: func() (string, error) {
			return "Backup complete", runBackup(app, pathField.Value())
		}}
	})
	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := app.Settings
		s.Backup.Path = f.TextField(backupsPathField).Value()
		s.Backup.AutoBackup = f.CheckboxField(backupsAutoBackupField).Checked()
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormBackups) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}

// Helpers

func runBackup(app *docker.Application, dir string) error {
	return app.BackupToFile(context.Background(), dir, app.BackupName())
}
