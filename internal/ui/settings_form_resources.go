package ui

import (
	"strconv"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

const (
	resourcesCPUField = iota
	resourcesMemoryField
)

type SettingsFormResources struct {
	settings docker.ApplicationSettings
	form     *Form
}

func NewSettingsFormResources(settings docker.ApplicationSettings) *SettingsFormResources {
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

	m := &SettingsFormResources{
		settings: settings,
		form: NewForm("Done",
			FormItem{Label: "CPU Limit", Field: cpuField},
			FormItem{Label: "Memory Limit (MB)", Field: memoryField},
		),
	}

	m.form.OnSubmit(func() tui.Cmd {
		m.settings.Resources.CPUs, _ = strconv.Atoi(m.form.TextField(resourcesCPUField).Value())
		m.settings.Resources.MemoryMB, _ = strconv.Atoi(m.form.TextField(resourcesMemoryField).Value())
		return func() tui.Msg { return SettingsSectionSubmitMsg{Settings: m.settings} }
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m *SettingsFormResources) Title() string {
	return "Resources"
}

func (m *SettingsFormResources) Init() tui.Cmd {
	return m.form.Init()
}

func (m *SettingsFormResources) Update(msg tui.Msg) tui.Cmd {
	return m.form.Update(msg)
}

func (m *SettingsFormResources) StatusLine() string { return "" }

func (m *SettingsFormResources) Render() string {
	return m.form.Render()
}
