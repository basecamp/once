package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/mouse"
)

type SettingsSection interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	View() string
	Title() string
	StatusLine() string
}

type SettingsSectionSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsSectionCancelMsg struct{}

var settingsKeys = struct {
	Back key.Binding
}{
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type settingsState int

const (
	settingsStateForm settingsState = iota
	settingsStateDeploying
	settingsStateRunningAction
	settingsStateActionComplete
)

type Settings struct {
	namespace            *docker.Namespace
	app                  *docker.Application
	width, height        int
	help                 Help
	state                settingsState
	section              SettingsSection
	sectionType          SettingsSectionType
	progress             *ProgressBusy
	err                  error
	actionSuccessMessage string
}

type settingsDeployFinishedMsg struct {
	err error
}

type settingsActionFinishedMsg struct {
	err     error
	message string
}

type settingsRunActionMsg struct {
	action func() (string, error)
}

func NewSettings(ns *docker.Namespace, app *docker.Application, sectionType SettingsSectionType) *Settings {
	state, err := ns.LoadState(context.Background())
	if err != nil {
		state = &docker.State{}
	}
	appState := state.AppState(app.Settings.Name)

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
	case SettingsSectionUpdates:
		section = NewSettingsFormUpdates(app, appState.LastUpdateResult())
	case SettingsSectionBackups:
		section = NewSettingsFormBackups(app, appState.LastBackupResult())
	}

	return &Settings{
		namespace:   ns,
		app:         app,
		help:        NewHelp(),
		state:       settingsStateForm,
		section:     section,
		sectionType: sectionType,
	}
}

func (m *Settings) Init() tea.Cmd {
	return m.section.Init()
}

func (m *Settings) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = NewProgressBusy(m.width, Colors.Border)
		if m.state == settingsStateForm {
			m.section.Update(msg)
		}
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			cmds = append(cmds, m.progress.Init())
		}

	case MouseEvent:
		if m.state == settingsStateActionComplete {
			if msg.IsClick && msg.Target == "done" {
				return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
			}
			return nil
		}
		if m.state == settingsStateForm {
			if cmd := m.help.Update(msg); cmd != nil {
				return cmd
			}
		}

	case tea.KeyPressMsg:
		if m.state == settingsStateActionComplete {
			if key.Matches(msg, NewKeyBinding("enter")) {
				return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
			}
			return nil
		}
		if m.state == settingsStateForm {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, settingsKeys.Back) {
				return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
			}
		}

	case SettingsSectionCancelMsg:
		return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }

	case SettingsSectionSubmitMsg:
		if msg.Settings.Equal(m.app.Settings) {
			return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
		}
		m.state = settingsStateDeploying
		m.app.Settings = msg.Settings
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return tea.Batch(m.progress.Init(), m.runDeploy())

	case settingsRunActionMsg:
		m.state = settingsStateRunningAction
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return tea.Batch(m.progress.Init(), func() tea.Msg {
			message, err := msg.action()
			return settingsActionFinishedMsg{err: err, message: message}
		})

	case settingsDeployFinishedMsg:
		return func() tea.Msg { return NavigateToAppMsg{App: m.app} }

	case settingsActionFinishedMsg:
		if msg.err != nil {
			m.state = settingsStateForm
			m.err = msg.err
			return nil
		}
		if msg.message != "" {
			m.actionSuccessMessage = msg.message
			m.state = settingsStateActionComplete
			return nil
		}
		return func() tea.Msg { return NavigateToAppMsg{App: m.app} }

	case ProgressBusyTickMsg:
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			if m.progress != nil {
				cmds = append(cmds, m.progress.Update(msg))
			}
		}
	}

	if m.state == settingsStateForm {
		cmd := m.section.Update(msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

func (m *Settings) View() string {
	titleLine := Styles.TitleRule(m.width, m.app.Settings.Host, strings.ToLower(m.section.Title()))

	var contentView string
	switch m.state {
	case settingsStateForm:
		var statusLine string
		if m.err != nil {
			statusLine = lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
		} else if line := m.section.StatusLine(); line != "" {
			statusLine = lipgloss.NewStyle().Foreground(Colors.Muted).Render(line)
		}
		contentView = lipgloss.JoinVertical(lipgloss.Center, statusLine, "", m.section.View())
	case settingsStateActionComplete:
		contentView = m.renderActionComplete()
	default:
		if m.progress != nil {
			contentView = m.progress.View()
		}
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpView := m.help.View([]key.Binding{settingsKeys.Back})
		helpLine = Styles.CenteredLine(m.width, helpView)
	}

	titleHeight := 2 // title + blank line
	helpHeight := lipgloss.Height(helpLine)
	middleHeight := m.height - titleHeight - helpHeight

	centeredContent := lipgloss.Place(
		m.width,
		middleHeight,
		lipgloss.Center,
		lipgloss.Center,
		contentView,
	)

	return titleLine + "\n\n" + centeredContent + helpLine
}

// Private

func (m *Settings) renderActionComplete() string {
	statusLine := Styles.CenteredLine(m.width, m.actionSuccessMessage)

	buttonStyle := Styles.Button.BorderForeground(Colors.Focused)
	button := mouse.Mark("done", buttonStyle.Render("Done"))
	buttonView := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		MarginTop(1).
		Render(button)

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, buttonView)
}

func (m *Settings) runDeploy() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Deploy(context.Background(), nil)
		return settingsDeployFinishedMsg{err: err}
	}
}

// Helpers

func formatOperationStatus(label string, result *docker.OperationResult) string {
	if result == nil {
		return ""
	}

	timeAgo := formatTimeAgo(time.Since(result.At))

	if result.Error != "" {
		return fmt.Sprintf("Last %s %s (failed: %s)", label, timeAgo, result.Error)
	}

	return fmt.Sprintf("Last %s %s", label, timeAgo)
}

func formatTimeAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
