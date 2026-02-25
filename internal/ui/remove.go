package ui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

var removeKeys = struct {
	Back key.Binding
}{
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type removeFinishedMsg struct {
	appName string
	err     error
}

type Remove struct {
	namespace     *docker.Namespace
	app           *docker.Application
	confirmation  *Confirmation
	width, height int
	help          Help
	removing      bool
	progress      *ProgressBusy
	err           error
}

func NewRemove(ns *docker.Namespace, app *docker.Application) *Remove {
	return &Remove{
		namespace:    ns,
		app:          app,
		confirmation: NewConfirmation("Remove application and data?", "Remove"),
		help:         NewHelp(),
	}
}

func (m *Remove) Init() tea.Cmd {
	return nil
}

func (m *Remove) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = NewProgressBusy(m.width, Colors.Border)
		if m.removing {
			cmds = append(cmds, m.progress.Init())
		}

	case MouseEvent:
		if !m.removing {
			if cmd := m.help.Update(msg); cmd != nil {
				return cmd
			}
			return m.confirmation.Update(msg)
		}

	case tea.KeyPressMsg:
		if !m.removing {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, removeKeys.Back) {
				return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
			}
			return m.confirmation.Update(msg)
		}

	case ConfirmationConfirmMsg:
		m.removing = true
		m.progress = NewProgressBusy(m.width, Colors.Border)
		return tea.Batch(m.progress.Init(), m.runRemove())

	case ConfirmationCancelMsg:
		return func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }

	case removeFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.removing = false
			return nil
		}
		return func() tea.Msg { return NavigateToDashboardMsg{AllowEmpty: true} }

	case ProgressBusyTickMsg:
		if m.removing && m.progress != nil {
			cmds = append(cmds, m.progress.Update(msg))
		}
	}

	return tea.Batch(cmds...)
}

func (m *Remove) View() string {
	titleLine := Styles.TitleRule(m.width, m.app.Settings.Host, "remove")

	var contentView string
	if m.removing {
		if m.progress != nil {
			contentView = m.progress.View()
		}
	} else {
		var errorLine string
		if m.err != nil {
			errorLine = lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
		}
		contentView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", m.confirmation.View())
	}

	var helpLine string
	if !m.removing {
		helpView := m.help.View([]key.Binding{removeKeys.Back})
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

func (m *Remove) runRemove() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Remove(context.Background(), true)
		return removeFinishedMsg{appName: m.app.Settings.Name, err: err}
	}
}
