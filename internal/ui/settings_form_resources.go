package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/pkg/docker"
)

const (
	resourcesCPUField = iota
	resourcesMemoryField
)

type SettingsFormResources struct {
	settingsFormBase
}

func NewSettingsFormResources(settings docker.ApplicationSettings) SettingsFormResources {
	cpuField := NewTextField("e.g. 2")
	cpuField.SetCharLimit(10)
	cpuField.SetDigitsOnly(true)
	if settings.Resources.CPUs != 0 {
		cpuField.SetValue(strconv.Itoa(settings.Resources.CPUs))
	}

	memoryField := NewTextField("e.g. 512")
	memoryField.SetCharLimit(10)
	memoryField.SetDigitsOnly(true)
	if settings.Resources.MemoryMB != 0 {
		memoryField.SetValue(strconv.Itoa(settings.Resources.MemoryMB))
	}

	m := SettingsFormResources{
		settingsFormBase: settingsFormBase{
			title: "Resources",
			form: NewForm("Done",
				FormItem{Label: "CPU Limit", Field: cpuField},
				FormItem{Label: "Memory Limit (MB)", Field: memoryField},
			),
		},
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Resources.CPUs, _ = strconv.Atoi(f.TextField(resourcesCPUField).Value())
		s.Resources.MemoryMB, _ = strconv.Atoi(f.TextField(resourcesMemoryField).Value())
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormResources) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
