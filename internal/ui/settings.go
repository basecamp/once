package ui

import (
	"context"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type SettingsSection interface {
	Init() tea.Cmd
	Update(tea.Msg) (SettingsSection, tea.Cmd)
	View() string
	Title() string
}

type SettingsSectionSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsSectionCancelMsg struct{}

type settingsKeyMap struct {
	Back key.Binding
}

func (k settingsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Back}
}

func (k settingsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Back}}
}

var settingsKeys = settingsKeyMap{
	Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type settingsState int

const (
	settingsStateForm settingsState = iota
	settingsStateDeploying
)

type Settings struct {
	namespace     *docker.Namespace
	app           *docker.Application
	width, height int
	help          help.Model
	state         settingsState
	section       SettingsSection
	sectionType   SettingsSectionType
	progress      ProgressBusy
}

type settingsDeployFinishedMsg struct {
	err error
}

func NewSettings(ns *docker.Namespace, app *docker.Application, sectionType SettingsSectionType) Settings {
	var section SettingsSection
	switch sectionType {
	case SettingsSectionApplication:
		section = NewSettingsFormApplication(app.Settings)
	case SettingsSectionEmail:
		section = NewSettingsFormEmail(app.Settings)
	case SettingsSectionEnvironment:
		section = NewSettingsFormEnvironment(app.Settings)
	case SettingsSectionResources:
		section = NewSettingsFormResources(app.Settings)
	}

	return Settings{
		namespace:   ns,
		app:         app,
		help:        help.New(),
		state:       settingsStateForm,
		section:     section,
		sectionType: sectionType,
	}
}

func (m Settings) Init() tea.Cmd {
	return m.section.Init()
}

func (m Settings) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = NewProgressBusy(m.width, Colors.Border)
		if m.state == settingsStateForm {
			m.section, _ = m.section.Update(msg)
		}
		if m.state == settingsStateDeploying {
			cmds = append(cmds, m.progress.Init())
		}

	case tea.KeyMsg:
		if m.state == settingsStateForm && key.Matches(msg, settingsKeys.Back) {
			return m, func() tea.Msg { return navigateToDashboardMsg{} }
		}

	case SettingsSectionCancelMsg:
		return m, func() tea.Msg { return navigateToDashboardMsg{} }

	case SettingsSectionSubmitMsg:
		if msg.Settings.Equal(m.app.Settings) {
			return m, func() tea.Msg { return navigateToDashboardMsg{} }
		}
		m.state = settingsStateDeploying
		m.app.Settings = msg.Settings
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return m, tea.Batch(m.progress.Init(), m.runDeploy())

	case settingsDeployFinishedMsg:
		return m, func() tea.Msg { return navigateToAppMsg{app: m.app} }

	case progressBusyTickMsg:
		if m.state == settingsStateDeploying {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	var cmd tea.Cmd
	if m.state == settingsStateForm {
		m.section, cmd = m.section.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Settings) View() string {
	subtitle := Styles.SubTitle.Width(m.width).Align(lipgloss.Center).Render(m.section.Title() + " Settings")
	titleBox := Styles.TitleBox(m.width, m.app.Settings.URL(), subtitle)

	var contentView string
	if m.state == settingsStateForm {
		contentView = m.section.View()
	} else {
		contentView = m.progress.View()
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpView := m.help.View(settingsKeys)
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

// Private

func (m Settings) runDeploy() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Deploy(context.Background(), nil)
		return settingsDeployFinishedMsg{err: err}
	}
}
