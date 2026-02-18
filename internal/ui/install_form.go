package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

const (
	installImageRefField = iota
	installHostnameField
)

type InstallFormSubmitMsg struct {
	ImageRef string
	Hostname string
}

type InstallFormCancelMsg struct{}

type InstallForm struct {
	form        *Form
	lastAppName string
	imageRef    string
}

func NewInstallForm(imageRef string) *InstallForm {
	var formItems []FormItem

	if imageRef != "" {
		if expanded, ok := imageAliases[imageRef]; ok {
			imageRef = expanded
		}

		styleFunc := func(s string) string {
			return lipgloss.NewStyle().
				Foreground(Colors.Muted).
				Padding(1, 0).
				Width(60).
				Align(lipgloss.Center).
				Render("Installing " + s)
		}

		formItems = append(formItems, FormItem{
			Label: "",
			Field: NewStaticField(imageRef, styleFunc),
		})
	} else {
		formItems = append(formItems, FormItem{
			Label: "Image",
			Field: NewTextField("user/repo:tag"),
		})
	}

	hostnameField := NewTextField("app.example.com")
	formItems = append(formItems, FormItem{
		Label: "Hostname",
		Field: hostnameField,
	})

	m := &InstallForm{
		form:     NewForm("Install", formItems...),
		imageRef: imageRef,
	}

	m.form.OnSubmit(func() tui.Cmd {
		return func() tui.Msg {
			return InstallFormSubmitMsg{
				ImageRef: m.ImageRef(),
				Hostname: m.form.TextField(installHostnameField).Value(),
			}
		}
	})
	m.form.OnCancel(func() tui.Cmd {
		return func() tui.Msg { return InstallFormCancelMsg{} }
	})

	if imageRef != "" {
		m.updateHostnamePlaceholder()
	}

	return m
}

func (m *InstallForm) Init() tui.Cmd {
	return m.form.Init()
}

func (m *InstallForm) Update(msg tui.Msg) tui.Cmd {
	prev := m.form.Focused()

	cmd := m.form.Update(msg)

	if prev == 0 && m.form.Focused() != 0 && m.imageRef == "" {
		m.expandImageAlias()
		m.updateHostnamePlaceholder()
	}

	return cmd
}

func (m *InstallForm) Render() string {
	return m.form.Render()
}

func (m *InstallForm) ImageRef() string {
	if m.imageRef != "" {
		return m.imageRef
	}
	return m.form.TextField(installImageRefField).Value()
}

func (m *InstallForm) Hostname() string {
	return m.form.TextField(installHostnameField).Value()
}

// Private

var imageAliases = map[string]string{
	"campfire": "ghcr.io/basecamp/once-campfire",
	"fizzy":    "ghcr.io/basecamp/fizzy:main",
}

func (m *InstallForm) expandImageAlias() {
	field := m.form.TextField(installImageRefField)
	if expanded, ok := imageAliases[field.Value()]; ok {
		field.SetValue(expanded)
	}
}

func (m *InstallForm) updateHostnamePlaceholder() {
	appName := docker.NameFromImageRef(m.ImageRef())
	if appName != m.lastAppName && appName != "" {
		m.form.TextField(installHostnameField).SetPlaceholder(appName + ".example.com")
		m.lastAppName = appName
	}
}
