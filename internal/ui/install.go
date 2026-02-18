package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

var installKeys = struct {
	Back KeyBinding
}{
	Back: NewKeyBinding(Key(tui.KeyEscape)).WithHelp("esc", "back"),
}

type installState int

const (
	installStateForm installState = iota
	installStateActivity
)

type Install struct {
	namespace     *docker.Namespace
	width, height int
	help          Help
	state         installState
	form          *InstallForm
	activity      *InstallActivity
	err           error
}

func NewInstall(ns *docker.Namespace, imageRef string) *Install {
	return &Install{
		namespace: ns,
		help:      NewHelp(),
		state:     installStateForm,
		form:      NewInstallForm(imageRef),
	}
}

func (m *Install) Init() tui.Cmd {
	return m.form.Init()
}

func (m *Install) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		if m.state == installStateForm {
			m.form.Update(msg)
		} else {
			m.activity.Update(msg)
		}

	case tui.MouseMsg:
		if m.state == installStateForm {
			if cmd := m.help.Update(msg); cmd != nil {
				return cmd
			}
		}

	case tui.KeyMsg:
		if m.state == installStateForm {
			if m.err != nil {
				m.err = nil
			}
			if installKeys.Back.Matches(msg) {
				return m.cancelFromScreen()
			}
		}

	case InstallFormCancelMsg:
		return m.cancelFromScreen()

	case InstallFormSubmitMsg:
		m.state = installStateActivity
		m.activity = NewInstallActivity(m.namespace, msg.ImageRef, msg.Hostname)
		m.activity.Update(tui.WindowSizeMsg{Width: m.width, Height: m.height})
		return m.activity.Init()

	case InstallActivityFailedMsg:
		m.state = installStateForm
		m.err = msg.Err
		return nil

	case InstallActivityDoneMsg:
		return func() tui.Msg { return navigateToAppMsg{app: msg.App} }
	}

	var cmd tui.Cmd
	if m.state == installStateForm {
		cmd = m.form.Update(msg)
	} else {
		cmd = m.activity.Update(msg)
	}
	return cmd
}

func (m *Install) cancelFromScreen() tui.Cmd {
	if m.form.ImageRef() != "" {
		return func() tui.Msg { return quitMsg{} }
	}
	return func() tui.Msg { return navigateToDashboardMsg{} }
}

func (m *Install) Render() string {
	titleLine := Styles.TitleRule(m.width, "install")

	var contentView string
	if m.state == installStateForm {
		if m.err != nil {
			errorLine := lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
			contentView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", m.form.Render())
		} else {
			contentView = m.form.Render()
		}
	} else {
		contentView = m.activity.Render()
	}

	var helpLine string
	if m.state == installStateForm {
		helpView := m.help.Render([]KeyBinding{installKeys.Back})
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
