package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/pkg/docker"
)

var actionsMenuCloseKey = WithHelp(NewKeyBinding("esc"), "esc", "close")

type ActionsMenuCloseMsg struct{}

type ActionsMenuSelectMsg struct {
	app    *docker.Application
	action ActionsMenuAction
}

type ActionsMenuAction int

const (
	ActionsMenuStartStop ActionsMenuAction = iota
	ActionsMenuRemove
)

type ActionsMenu struct {
	app  *docker.Application
	menu Menu
	help Help
}

func NewActionsMenu(app *docker.Application) ActionsMenu {
	startStopLabel := "Start"
	if app.Running {
		startStopLabel = "Stop"
	}

	h := NewHelp()
	h.SetBindings([]key.Binding{actionsMenuCloseKey})
	return ActionsMenu{
		app: app,
		menu: NewMenu(
			MenuItem{Label: startStopLabel, Key: int(ActionsMenuStartStop), Shortcut: WithHelp(NewKeyBinding("s"), "s", "")},
			MenuItem{Label: "Remove", Key: int(ActionsMenuRemove), Shortcut: WithHelp(NewKeyBinding("r"), "r", "")},
		),
		help: h,
	}
}

func (m ActionsMenu) Init() tea.Cmd {
	return nil
}

func (m ActionsMenu) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case MouseEvent:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		if cmd != nil {
			return m, cmd
		}

	case tea.KeyPressMsg:
		if key.Matches(msg, actionsMenuCloseKey) {
			return m, func() tea.Msg { return ActionsMenuCloseMsg{} }
		}

	case MenuSelectMsg:
		return m, m.selectAction(ActionsMenuAction(msg.Key))
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func (m ActionsMenu) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		MarginBottom(1)

	title := titleStyle.Render("Actions")

	m.menu.SetWidth(20)
	menuView := m.menu.View()

	helpView := m.help.View()
	menuWidth := lipgloss.Width(menuView)
	helpLine := lipgloss.NewStyle().MarginTop(1).Width(menuWidth).Align(lipgloss.Center).Render(helpView)

	content := lipgloss.JoinVertical(lipgloss.Center,
		title,
		menuView,
		helpLine,
	)

	return boxStyle.Render(content)
}

// Private

func (m ActionsMenu) selectAction(action ActionsMenuAction) tea.Cmd {
	return func() tea.Msg {
		return ActionsMenuSelectMsg{app: m.app, action: action}
	}
}
