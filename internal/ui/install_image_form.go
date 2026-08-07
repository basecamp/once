package ui

import (
	tea "charm.land/bubbletea/v2"
)

type InstallImageSubmitMsg struct {
	ImageRef string
	Username string
	Password string
}
type InstallImageBackMsg struct{}

type InstallImageForm struct {
	form           Form
	hasCredentials bool
}

func NewInstallImageForm() InstallImageForm {
	return newInstallImageForm(NewForm("Next", imageFormItem("")), false)
}

// NewInstallImageFormWithCredentials builds the image form with username and
// password fields revealed, for retrying after a registry credentials error.
func NewInstallImageFormWithCredentials(imageRef, username string) InstallImageForm {
	usernameField := NewTextField("")
	usernameField.SetValue(username)
	passwordField := NewTextField("")
	passwordField.SetEchoPassword()

	form := NewForm("Next",
		imageFormItem(imageRef),
		FormItem{Label: "Username", Field: usernameField},
		FormItem{Label: "Password", Field: passwordField},
	)
	return newInstallImageForm(form, true)
}

func (m InstallImageForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallImageForm) Update(msg tea.Msg) (InstallImageForm, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m InstallImageForm) View() string {
	return m.form.View()
}

// Helpers

func newInstallImageForm(form Form, hasCredentials bool) InstallImageForm {
	m := InstallImageForm{form: form, hasCredentials: hasCredentials}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		ref := f.TextField(0).Value()
		if expanded, ok := expandAlias(ref); ok {
			ref = expanded
		}
		msg := InstallImageSubmitMsg{ImageRef: ref}
		if hasCredentials {
			msg.Username = f.TextField(1).Value()
			msg.Password = f.TextField(2).Value()
		}
		return func() tea.Msg { return msg }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallImageBackMsg{} }
	})

	return m
}

func imageFormItem(imageRef string) FormItem {
	field := NewTextField("user/repo:tag")
	field.SetValue(imageRef)
	return FormItem{Label: "Image", Field: field, Required: true}
}
