package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type installKeyMap struct {
	Back key.Binding
}

func (k installKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Back}
}

func (k installKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Back}}
}

var installKeys = installKeyMap{
	Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type installState int

const (
	installStateForm installState = iota
	installStateActivity
)

type Install struct {
	namespace     *docker.Namespace
	width, height int
	help          help.Model
	state         installState
	form          InstallForm
	activity      InstallActivity
}

func NewInstall(ns *docker.Namespace) Install {
	return Install{
		namespace: ns,
		help:      help.New(),
		state:     installStateForm,
		form:      NewInstallForm(),
	}
}

func (m Install) Init() tea.Cmd {
	return m.form.Init()
}

func (m Install) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		if m.state == installStateForm {
			m.form, _ = m.form.Update(msg)
		} else {
			m.activity, _ = m.activity.Update(msg)
		}

	case tea.KeyMsg:
		if m.state == installStateForm && key.Matches(msg, installKeys.Back) {
			return m, func() tea.Msg { return navigateToDashboardMsg{} }
		}

	case InstallFormCancelMsg:
		return m, func() tea.Msg { return navigateToDashboardMsg{} }

	case InstallFormSubmitMsg:
		m.state = installStateActivity
		m.activity = NewInstallActivity(m.namespace, msg.ImageRef, msg.Hostname)
		m.activity, _ = m.activity.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.activity.Init()

	case InstallActivityDoneMsg:
		return m, func() tea.Msg { return navigateToAppMsg{app: msg.App} }
	}

	var cmd tea.Cmd
	if m.state == installStateForm {
		m.form, cmd = m.form.Update(msg)
	} else {
		m.activity, cmd = m.activity.Update(msg)
	}
	return m, cmd
}

func (m Install) View() string {
	titleBox := Styles.TitleBox(m.width, "Install")

	var contentView string
	if m.state == installStateForm {
		contentView = m.form.View()
	} else {
		contentView = m.activity.View()
	}

	var helpLine string
	if m.state == installStateForm {
		helpView := m.help.View(installKeys)
		helpLine = Styles.HelpLine(m.width, helpView)
	}

	titleBoxHeight := lipgloss.Height(titleBox)
	helpHeight := lipgloss.Height(helpLine)
	middleHeight := m.height - titleBoxHeight - helpHeight

	centeredContent := lipgloss.Place(
		m.width,
		middleHeight,
		lipgloss.Center,
		lipgloss.Center,
		contentView,
	)

	return titleBox + centeredContent + helpLine
}
