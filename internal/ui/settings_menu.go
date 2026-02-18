package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

var settingsMenuCloseKey = NewKeyBinding(Key(tui.KeyEscape)).WithHelp("esc", "close")

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
	return SettingsMenu{
		app: app,
		menu: NewMenu(
			MenuItem{Label: "Application", Key: int(SettingsSectionApplication), Shortcut: NewKeyBinding(RuneKey('a')).WithHelp("a", "")},
			MenuItem{Label: "Email", Key: int(SettingsSectionEmail), Shortcut: NewKeyBinding(RuneKey('e')).WithHelp("e", "")},
			MenuItem{Label: "Environment", Key: int(SettingsSectionEnvironment), Shortcut: NewKeyBinding(RuneKey('v')).WithHelp("v", "")},
			MenuItem{Label: "Resources", Key: int(SettingsSectionResources), Shortcut: NewKeyBinding(RuneKey('r')).WithHelp("r", "")},
			MenuItem{Label: "Updates", Key: int(SettingsSectionUpdates), Shortcut: NewKeyBinding(RuneKey('u')).WithHelp("u", "")},
			MenuItem{Label: "Backups", Key: int(SettingsSectionBackups), Shortcut: NewKeyBinding(RuneKey('b')).WithHelp("b", "")},
		),
		help: NewHelp(),
	}
}

func (m *SettingsMenu) Init() tui.Cmd {
	return nil
}

func (m *SettingsMenu) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.MouseMsg:
		if cmd := m.help.Update(msg); cmd != nil {
			return cmd
		}

	case tui.KeyMsg:
		if settingsMenuCloseKey.Matches(msg) {
			return func() tui.Msg { return SettingsMenuCloseMsg{} }
		}

	case MenuSelectMsg:
		return m.selectSection(SettingsSectionType(msg.Key))
	}

	return m.menu.Update(msg)
}

func (m *SettingsMenu) Render() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		MarginBottom(1)

	title := titleStyle.Render("Settings")

	menuView := m.menu.Render()

	helpView := m.help.Render([]KeyBinding{settingsMenuCloseKey})
	menuWidth := lipgloss.Width(menuView)
	helpLine := lipgloss.NewStyle().MarginTop(1).Width(menuWidth).Align(lipgloss.Center).Render(helpView)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		menuView,
		helpLine,
	)

	return boxStyle.Render(content)
}

// Private

func (m *SettingsMenu) selectSection(section SettingsSectionType) tui.Cmd {
	return func() tui.Msg {
		return SettingsMenuSelectMsg{app: m.app, section: section}
	}
}
