package ui

import (
	"context"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
)

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
	form          SettingsForm
	progress      ProgressBusy
}

type settingsDeployFinishedMsg struct {
	err error
}

func NewSettings(ns *docker.Namespace, app *docker.Application) Settings {
	return Settings{
		namespace: ns,
		app:       app,
		help:      help.New(),
		state:     settingsStateForm,
		form:      NewSettingsForm(app.Settings),
	}
}

func (m Settings) Init() tea.Cmd {
	return m.form.Init()
}

func (m Settings) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
		if m.state == settingsStateForm {
			m.form, _ = m.form.Update(msg)
		}
		if m.state == settingsStateDeploying {
			cmds = append(cmds, m.progress.Init())
		}

	case tea.KeyMsg:
		if m.state == settingsStateForm && key.Matches(msg, settingsKeys.Back) {
			return m, func() tea.Msg { return navigateToDashboardMsg{} }
		}

	case SettingsFormCancelMsg:
		return m, func() tea.Msg { return navigateToDashboardMsg{} }

	case SettingsFormSubmitMsg:
		m.state = settingsStateDeploying
		m.app.Settings.Image = msg.Image
		m.app.Settings.Host = msg.Hostname
		m.app.Settings.DisableTLS = !msg.TLSEnabled
		m.progress = NewProgressBusy(m.width, lipgloss.Color("#6272a4"))
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
		m.form, cmd = m.form.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Settings) View() string {
	title := Styles.Title.Width(m.width).Align(lipgloss.Center).Render(m.app.Settings.Name)
	subtitle := Styles.SubTitle.Width(m.width).Align(lipgloss.Center).Render("Settings")

	var contentView string
	if m.state == settingsStateForm {
		contentView = m.form.View()
	} else {
		contentView = m.progress.View()
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpView := m.help.View(settingsKeys)
		helpLine = lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(helpView)
	}

	titleHeight := lipgloss.Height(title)
	subtitleHeight := lipgloss.Height(subtitle)
	helpHeight := lipgloss.Height(helpLine)
	middleHeight := m.height - titleHeight - subtitleHeight - helpHeight

	centeredContent := lipgloss.Place(
		m.width,
		middleHeight,
		lipgloss.Center,
		lipgloss.Center,
		contentView,
	)

	return title + subtitle + centeredContent + helpLine
}

// Private

func (m Settings) runDeploy() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Deploy(context.Background(), nil)
		return settingsDeployFinishedMsg{err: err}
	}
}
