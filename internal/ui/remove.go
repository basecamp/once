package ui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/pkg/docker"
)

var removeKeys = struct {
	Back key.Binding
}{
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type removeFinishedMsg struct {
	err error
}

type Remove struct {
	namespace     *docker.Namespace
	app           *docker.Application
	confirmation  Confirmation
	width, height int
	help          Help
	removing      bool
	progress      Progress
	err           error
}

func NewRemove(ns *docker.Namespace, app *docker.Application) Remove {
	h := NewHelp()
	h.SetBindings([]key.Binding{removeKeys.Back})
	return Remove{
		namespace:    ns,
		app:          app,
		confirmation: NewConfirmation("Remove application and data?", "Remove"),
		help:         h,
		progress:     NewProgress(0, Colors.Border),
	}
}

func (m Remove) Init() tea.Cmd {
	return nil
}

func (m Remove) Update(msg tea.Msg) (Component, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.progress = m.progress.SetWidth(m.width)
		if m.removing {
			cmds = append(cmds, m.progress.Init())
		}

	case MouseEvent:
		if !m.removing {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			if cmd != nil {
				return m, cmd
			}
			m.confirmation, cmd = m.confirmation.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		if !m.removing {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, removeKeys.Back) {
				return m, func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
			}
			var cmd tea.Cmd
			m.confirmation, cmd = m.confirmation.Update(msg)
			return m, cmd
		}

	case ConfirmationConfirmMsg:
		m.removing = true
		m.progress = NewProgress(m.width, Colors.Border)
		return m, tea.Batch(m.progress.Init(), m.runRemove())

	case ConfirmationCancelMsg:
		return m, func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }

	case removeFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.removing = false
			return m, nil
		}
		return m, func() tea.Msg { return NavigateToDashboardMsg{AllowEmpty: true} }

	case ProgressTickMsg:
		if m.removing {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Remove) View() string {
	titleLine := Styles.TitleRule(m.width, m.app.Settings.Host, "remove")

	var contentView string
	if m.removing {
		contentView = m.progress.View()
	} else {
		var errorLine string
		if m.err != nil {
			errorLine = lipgloss.NewStyle().Foreground(Colors.Error).Width(m.width).Align(lipgloss.Center).Render(docker.ErrorMessage(m.err))
		}
		contentView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", m.confirmation.View())
	}

	var helpLine string
	if !m.removing {
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

func (m Remove) runRemove() tea.Cmd {
	return func() tea.Msg {
		err := m.app.Remove(context.Background(), true)
		return removeFinishedMsg{err: err}
	}
}
