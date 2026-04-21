package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

var settingsMenuCloseKey = WithHelp(NewKeyBinding("esc"), "esc", "close")

type SettingsMenuCloseMsg struct{}

type SettingsMenuSelectMsg struct {
	app     *docker.Application
	section SettingsSectionType
}

type SettingsMenu struct {
	app  *docker.Application
	menu Menu
	help Help
}

func NewSettingsMenu(app *docker.Application) SettingsMenu {
	h := NewHelp()
	h.SetBindings([]key.Binding{settingsMenuCloseKey})
	return SettingsMenu{
		app: app,
		menu: NewMenu(
			MenuItem{Label: "Application", Key: int(SettingsSectionApplication), Shortcut: WithHelp(NewKeyBinding("a"), "a", "")},
			MenuItem{Label: "Email", Key: int(SettingsSectionEmail), Shortcut: WithHelp(NewKeyBinding("e"), "e", "")},
			MenuItem{Label: "Environment", Key: int(SettingsSectionEnvironment), Shortcut: WithHelp(NewKeyBinding("v"), "v", "")},
			MenuItem{Label: "Resources", Key: int(SettingsSectionResources), Shortcut: WithHelp(NewKeyBinding("r"), "r", "")},
			MenuItem{Label: "Updates", Key: int(SettingsSectionUpdates), Shortcut: WithHelp(NewKeyBinding("u"), "u", "")},
			MenuItem{Label: "Backups", Key: int(SettingsSectionBackups), Shortcut: WithHelp(NewKeyBinding("b"), "b", "")},
			MenuItem{Label: "Mounts", Key: int(SettingsSectionMounts), Shortcut: WithHelp(NewKeyBinding("m"), "m", "")},
		),
		help: h,
	}
}

func (m SettingsMenu) Init() tea.Cmd {
	return nil
}

func (m SettingsMenu) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case MouseEvent:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		if cmd != nil {
			return m, cmd
		}

	case tea.KeyPressMsg:
		if key.Matches(msg, settingsMenuCloseKey) {
			return m, func() tea.Msg { return SettingsMenuCloseMsg{} }
		}

	case MenuSelectMsg:
		return m, m.selectSection(SettingsSectionType(msg.Key))
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func (m SettingsMenu) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		MarginBottom(1)

	title := titleStyle.Render("Settings")

	m.menu.SetWidth(24)
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

func (m SettingsMenu) selectSection(section SettingsSectionType) tea.Cmd {
	return func() tea.Msg {
		return SettingsMenuSelectMsg{app: m.app, section: section}
	}
}
