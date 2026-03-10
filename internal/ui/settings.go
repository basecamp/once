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
	Update(tea.Msg) (SettingsSection, tea.Cmd)
	View() string
	Title() string
	StatusLine() string
}

type SettingsSectionSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsSectionCancelMsg struct{}

var settingsKeys = struct {
	Back  key.Binding
	Enter key.Binding
}{
	Back:  WithHelp(NewKeyBinding("esc"), "esc", "back"),
	Enter: NewKeyBinding("enter"),
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
	progress             Progress
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

func NewSettings(ns *docker.Namespace, app *docker.Application, sectionType SettingsSectionType) Settings {
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

	h := NewHelp()
	h.SetBindings([]key.Binding{settingsKeys.Back})
	return Settings{
		namespace:   ns,
		app:         app,
		help:        h,
		state:       settingsStateForm,
		section:     section,
		sectionType: sectionType,
		progress:    NewProgress(0, Colors.Border),
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
		m.progress = m.progress.SetWidth(m.width)
		if m.state == settingsStateForm {
			m.section, _ = m.section.Update(msg)
		}
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			cmds = append(cmds, m.progress.Init())
		}

	case MouseEvent:
		if m.state == settingsStateActionComplete {
			if msg.IsClick && msg.Target == "done" {
				return m, m.navigateToDashboard()
			}
			return m, nil
		}
		if m.state == settingsStateForm {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			if cmd != nil {
				return m, cmd
			}
		}

	case tea.KeyPressMsg:
		if m.state == settingsStateActionComplete {
			if key.Matches(msg, settingsKeys.Enter) {
				return m, m.navigateToDashboard()
			}
			return m, nil
		}
		if m.state == settingsStateForm {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, settingsKeys.Back) {
				return m, m.navigateToDashboard()
			}
		}

	case SettingsSectionCancelMsg:
		return m, m.navigateToDashboard()

	case SettingsSectionSubmitMsg:
		return m.handleFormSubmit(msg)

	case settingsRunActionMsg:
		m.state = settingsStateRunningAction
		m.progress = NewProgress(m.width, Colors.Border)
		return m, tea.Batch(m.progress.Init(), func() tea.Msg {
			message, err := msg.action()
			return settingsActionFinishedMsg{err: err, message: message}
		})

	case settingsDeployFinishedMsg:
		return m, func() tea.Msg { return NavigateToAppMsg{App: m.app} }

	case settingsActionFinishedMsg:
		return m.handleActionResult(msg)

	case ProgressTickMsg:
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	if m.state == settingsStateForm {
		var cmd tea.Cmd
		m.section, cmd = m.section.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Settings) View() string {
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
		contentView = m.progress.View()
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpLine = Styles.CenteredLine(m.width, m.help.View())
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

func (m Settings) navigateToDashboard() tea.Cmd {
	return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
}

func (m Settings) handleFormSubmit(msg SettingsSectionSubmitMsg) (Component, tea.Cmd) {
	if msg.Settings.Equal(m.app.Settings) {
		return m, m.navigateToDashboard()
	}
	if m.namespace.HostInUseByAnother(msg.Settings.Host, m.app.Settings.Name) {
		m.err = docker.ErrHostnameInUse
		return m, nil
	}
	m.state = settingsStateDeploying
	m.app.Settings = msg.Settings
	m.progress = NewProgress(m.width, Colors.Border)
	return m, tea.Batch(m.progress.Init(), m.runDeploy())
}

func (m Settings) handleActionResult(msg settingsActionFinishedMsg) (Component, tea.Cmd) {
	if msg.err != nil {
		m.state = settingsStateForm
		m.err = msg.err
		return m, nil
	}
	if msg.message != "" {
		m.actionSuccessMessage = msg.message
		m.state = settingsStateActionComplete
		return m, nil
	}
	return m, func() tea.Msg { return NavigateToAppMsg{App: m.app} }
}

func (m Settings) renderActionComplete() string {
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

func (m Settings) runDeploy() tea.Cmd {
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
