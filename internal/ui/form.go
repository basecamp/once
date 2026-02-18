package ui

import (
	"strconv"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/components"
	"github.com/basecamp/gliff/tui"
)

type FormField interface {
	Update(tui.Msg) tui.Cmd
	Render() string
	Focus() tui.Cmd
	Blur()
	SetWidth(int)
	IsFocusable() bool
}

// TextField

type TextField struct {
	input      *components.TextField
	digitsOnly bool
}

func NewTextField(placeholder string) *TextField {
	input := components.NewTextField()
	input.SetPlaceholder(placeholder)
	input.SetCharLimit(256)
	return &TextField{input: input}
}

func (f *TextField) Value() string {
	return f.input.Value()
}

func (f *TextField) SetValue(v string) {
	f.input.SetValue(v)
}

func (f *TextField) SetPlaceholder(p string) {
	f.input.SetPlaceholder(p)
}

func (f *TextField) SetCharLimit(n int) {
	f.input.SetCharLimit(n)
}

func (f *TextField) SetDigitsOnly(v bool) {
	f.digitsOnly = v
}

func (f *TextField) SetEchoPassword() {
	f.input.SetEchoMode(components.EchoPassword)
}

func (f *TextField) Update(msg tui.Msg) tui.Cmd {
	if f.digitsOnly {
		if msg, ok := msg.(tui.KeyMsg); ok && msg.Type == tui.KeyRune {
			if msg.Rune < '0' || msg.Rune > '9' {
				return nil
			}
		}
	}

	return f.input.Update(msg)
}

func (f *TextField) Render() string {
	return f.input.Render()
}

func (f *TextField) Focus() tui.Cmd {
	return f.input.Focus()
}

func (f *TextField) Blur() {
	f.input.Blur()
}

func (f *TextField) SetWidth(w int) {
	f.input.SetWidth(w)
}

func (f *TextField) IsFocusable() bool { return true }

// CheckboxField

type CheckboxField struct {
	label      string
	checked    bool
	disabledFn func() (disabled bool, text string)
}

func NewCheckboxField(label string, checked bool) *CheckboxField {
	return &CheckboxField{label: label, checked: checked}
}

func (f *CheckboxField) Checked() bool {
	return f.checked
}

func (f *CheckboxField) SetDisabledWhen(fn func() (disabled bool, text string)) {
	f.disabledFn = fn
}

func (f *CheckboxField) Toggle() {
	if f.disabledFn != nil {
		if disabled, _ := f.disabledFn(); disabled {
			return
		}
	}
	f.checked = !f.checked
}

func (f *CheckboxField) Update(msg tui.Msg) tui.Cmd {
	if msg, ok := msg.(tui.KeyMsg); ok {
		if msg.Type == tui.KeyRune && msg.Rune == ' ' {
			f.Toggle()
		}
	}
	return nil
}

func (f *CheckboxField) Render() string {
	if f.disabledFn != nil {
		if disabled, text := f.disabledFn(); disabled {
			return text
		}
	}

	if f.checked {
		return "[✓] " + f.label
	}
	return "[ ] " + f.label
}

func (f *CheckboxField) Focus() tui.Cmd    { return nil }
func (f *CheckboxField) Blur()             {}
func (f *CheckboxField) SetWidth(int)      {}
func (f *CheckboxField) IsFocusable() bool { return true }

// StaticField

type StaticField struct {
	value   string
	styleFn func(string) string
}

func NewStaticField(value string, styleFn func(string) string) *StaticField {
	return &StaticField{value: value, styleFn: styleFn}
}

func (f *StaticField) Value() string {
	return f.value
}

func (f *StaticField) SetValue(v string) {
	f.value = v
}

func (f *StaticField) Update(tui.Msg) tui.Cmd { return nil }
func (f *StaticField) Render() string         { return f.styleFn(f.value) }
func (f *StaticField) Focus() tui.Cmd         { return nil }
func (f *StaticField) Blur()                  {}
func (f *StaticField) SetWidth(int)           {}
func (f *StaticField) IsFocusable() bool      { return false }

// FormActionButton

type FormActionButton struct {
	Label   string
	OnPress func() tui.Msg
}

// Form

type FormItem struct {
	Label string
	Field FormField
}

type Form struct {
	items        []FormItem
	submitLabel  string
	actionButton *FormActionButton
	focused      int
	width        int
	onSubmit     func() tui.Cmd
	onCancel     func() tui.Cmd
}

func NewForm(submitLabel string, items ...FormItem) *Form {
	f := &Form{
		items:       items,
		submitLabel: submitLabel,
	}

	for i, item := range items {
		if item.Field.IsFocusable() {
			f.focused = i
			item.Field.Focus()
			break
		}
	}

	return f
}

func (f *Form) Init() tui.Cmd {
	if f.focused < len(f.items) {
		return f.items[f.focused].Field.Focus()
	}
	return nil
}

func (f *Form) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		f.width = msg.Width
		inputWidth := min(f.width-4, 60)
		for _, item := range f.items {
			item.Field.SetWidth(inputWidth)
		}

	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			return f.handleClick(msg.Target)
		}

	case tui.KeyMsg:
		switch msg.Type {
		case tui.KeyTab:
			return f.focusNext()
		case tui.KeyShiftTab:
			return f.focusPrev()
		case tui.KeyEnter:
			return f.handleEnter()
		}
	}

	if f.focused < len(f.items) {
		return f.items[f.focused].Field.Update(msg)
	}

	return nil
}

func (f *Form) Render() string {
	var parts []string

	for i, item := range f.items {
		if _, isStatic := item.Field.(*StaticField); isStatic {
			parts = append(parts, item.Field.Render())
			continue
		}
		label := Styles.Label.Render(item.Label)
		inputStyle := Styles.Focus(Styles.Input, f.focused == i)
		field := tui.WithTarget(fieldTarget(i), inputStyle.Render(item.Field.Render()))
		parts = append(parts, label, field, "")
	}

	submitButton := tui.WithTarget("submit", Styles.Focus(Styles.ButtonPrimary, f.focused == f.submitIndex()).
		Render(f.submitLabel))

	buttonParts := []string{submitButton}

	if f.actionButton != nil {
		actionBtn := tui.WithTarget("action", Styles.Focus(Styles.Button, f.focused == f.actionIndex()).
			Render(f.actionButton.Label))
		buttonParts = append(buttonParts, actionBtn)
	}

	cancelButton := tui.WithTarget("cancel", Styles.Focus(Styles.Button, f.focused == f.cancelIndex()).
		Render("Cancel"))

	buttonParts = append(buttonParts, cancelButton)
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, buttonParts...)
	parts = append(parts, "", buttons)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (f *Form) SetActionButton(label string, onPress func() tui.Msg) {
	f.actionButton = &FormActionButton{Label: label, OnPress: onPress}
}

func (f *Form) OnSubmit(fn func() tui.Cmd) {
	f.onSubmit = fn
}

func (f *Form) OnCancel(fn func() tui.Cmd) {
	f.onCancel = fn
}

func (f *Form) Focused() int {
	return f.focused
}

func (f *Form) Field(i int) FormField {
	return f.items[i].Field
}

func (f *Form) TextField(i int) *TextField {
	return f.items[i].Field.(*TextField)
}

func (f *Form) CheckboxField(i int) *CheckboxField {
	return f.items[i].Field.(*CheckboxField)
}

// Private

func (f *Form) focusNext() tui.Cmd {
	f.blurCurrent()
	f.focused = (f.focused + 1) % f.totalCount()
	return f.focusToNextFocusable()
}

func (f *Form) focusPrev() tui.Cmd {
	f.blurCurrent()
	f.focused = (f.focused - 1 + f.totalCount()) % f.totalCount()
	return f.focusToNextFocusable()
}

func (f *Form) blurCurrent() {
	if f.focused < len(f.items) {
		f.items[f.focused].Field.Blur()
	}
}

func (f *Form) focusCurrent() tui.Cmd {
	if f.focused < len(f.items) {
		return f.items[f.focused].Field.Focus()
	}
	return nil
}

func (f *Form) focusToNextFocusable() tui.Cmd {
	start := f.focused
	for {
		if f.focused < len(f.items) {
			if f.items[f.focused].Field.IsFocusable() {
				return f.focusCurrent()
			}
			f.focused = (f.focused + 1) % f.totalCount()
		} else {
			return nil
		}

		if f.focused == start {
			return nil
		}
	}
}

func (f *Form) handleEnter() tui.Cmd {
	switch {
	case f.focused < len(f.items):
		return f.focusNext()
	case f.actionButton != nil && f.focused == f.actionIndex():
		return func() tui.Msg { return f.actionButton.OnPress() }
	case f.focused == f.submitIndex():
		if f.onSubmit != nil {
			return f.onSubmit()
		}
		return nil
	case f.focused == f.cancelIndex():
		if f.onCancel != nil {
			return f.onCancel()
		}
		return nil
	}
	return nil
}

func (f *Form) handleClick(target string) tui.Cmd {
	if target == "" {
		return nil
	}

	for i := range f.items {
		if target == fieldTarget(i) {
			if cb, ok := f.items[i].Field.(*CheckboxField); ok {
				cb.Toggle()
			}
			return f.focusIndex(i)
		}
	}

	switch target {
	case "submit":
		f.blurCurrent()
		f.focused = f.submitIndex()
		if f.onSubmit != nil {
			return f.onSubmit()
		}
	case "action":
		if f.actionButton != nil {
			f.blurCurrent()
			f.focused = f.actionIndex()
			return func() tui.Msg { return f.actionButton.OnPress() }
		}
	case "cancel":
		f.blurCurrent()
		f.focused = f.cancelIndex()
		if f.onCancel != nil {
			return f.onCancel()
		}
	}

	return nil
}

func (f *Form) focusIndex(i int) tui.Cmd {
	if i == f.focused {
		return nil
	}
	f.blurCurrent()
	f.focused = i
	return f.focusCurrent()
}

func (f *Form) submitIndex() int { return len(f.items) }

func (f *Form) actionIndex() int { return len(f.items) + 1 }

func (f *Form) cancelIndex() int {
	if f.actionButton != nil {
		return len(f.items) + 2
	}
	return len(f.items) + 1
}

func (f *Form) totalCount() int {
	return f.cancelIndex() + 1
}

// Helpers

func fieldTarget(i int) string {
	return "field:" + strconv.Itoa(i)
}
