package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/components"
	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

type SettingsSection interface {
	Init() tui.Cmd
	Update(tui.Msg) tui.Cmd
	Render() string
	Title() string
	StatusLine() string
}

type SettingsSectionSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsSectionCancelMsg struct{}

var settingsKeys = struct {
	Back KeyBinding
}{
	Back: NewKeyBinding(Key(tui.KeyEscape)).WithHelp("esc", "back"),
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
	progress             *components.ProgressBusy
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
	state, _ := ns.LoadState(context.Background())
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

func (m *Settings) Init() tui.Cmd {
	return m.section.Init()
}

func (m *Settings) Update(msg tui.Msg) tui.Cmd {
	var cmds []tui.Cmd

	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = components.NewProgressBusy(m.width, Colors.Border)
		if m.state == settingsStateForm {
			m.section.Update(msg)
		}
		if m.state == settingsStateDeploying || m.state == settingsStateRunningAction {
			cmds = append(cmds, m.progress.Init())
		}

	case tui.MouseMsg:
		if m.state == settingsStateActionComplete {
			if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft && msg.Target == "done" {
				return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }
			}
			return nil
		}
		if m.state == settingsStateForm {
			if cmd := m.help.Update(msg); cmd != nil {
				return cmd
			}
		}

	case tui.KeyMsg:
		if m.state == settingsStateActionComplete {
			if msg.Type == tui.KeyEnter {
				return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }
			}
			return nil
		}
		if m.state == settingsStateForm {
			if m.err != nil {
				m.err = nil
			}
			if settingsKeys.Back.Matches(msg) {
				return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }
			}
		}

	case SettingsSectionCancelMsg:
		return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }

	case SettingsSectionSubmitMsg:
		if msg.Settings.Equal(m.app.Settings) {
			return func() tui.Msg { return navigateToDashboardMsg{appName: m.app.Settings.Name} }
		}
		m.state = settingsStateDeploying
		m.app.Settings = msg.Settings
		m.progress = components.NewProgressBusy(m.width, Colors.Border)
		return tui.Batch(m.progress.Init(), m.runDeploy())

	case settingsRunActionMsg:
		m.state = settingsStateRunningAction
		m.progress = components.NewProgressBusy(m.width, Colors.Border)
		return tui.Batch(m.progress.Init(), func() tui.Msg {
			message, err := msg.action()
			return settingsActionFinishedMsg{err: err, message: message}
		})

	case settingsDeployFinishedMsg:
		return func() tui.Msg { return navigateToAppMsg{app: m.app} }

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
		return func() tui.Msg { return navigateToAppMsg{app: m.app} }

	case components.ProgressBusyTickMsg:
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

	return tui.Batch(cmds...)
}

func (m *Settings) Render() string {
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
		contentView = lipgloss.JoinVertical(lipgloss.Center, statusLine, "", m.section.Render())
	case settingsStateActionComplete:
		contentView = m.renderActionComplete()
	default:
		if m.progress != nil {
			contentView = m.progress.Render()
		}
	}

	var helpLine string
	if m.state == settingsStateForm {
		helpView := m.help.Render([]KeyBinding{settingsKeys.Back})
		helpLine = Styles.HelpLine(m.width, helpView)
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
	button := tui.WithTarget("done", buttonStyle.Render("Done"))
	buttonView := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		MarginTop(1).
		Render(button)

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, buttonView)
}

func (m *Settings) runDeploy() tui.Cmd {
	return func() tui.Msg {
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
