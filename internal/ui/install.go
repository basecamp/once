package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/basecamp/once/internal/docker"
)

var installKeys = struct {
	Back key.Binding
}{
	Back: WithHelp(NewKeyBinding("esc"), "esc", "back"),
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
	starfield     *Starfield
	logo          *Logo
	err           error
	cliMode       bool
}

func NewInstall(ns *docker.Namespace, imageRef string) *Install {
	m := &Install{
		namespace: ns,
		help:      NewHelp(),
		state:     installStateForm,
		form:      NewInstallForm(imageRef),
		cliMode:   imageRef != "",
	}
	if m.showLogo() {
		m.starfield = NewStarfield()
		m.logo = NewLogo()
	}
	return m
}

func (m *Install) Init() tea.Cmd {
	cmds := []tea.Cmd{m.form.Init()}
	if m.showLogo() {
		cmds = append(cmds, m.starfield.Init(), m.logo.Init())
	}
	return tea.Batch(cmds...)
}

func (m *Install) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		var cmds []tea.Cmd
		if m.starfield != nil {
			cmds = append(cmds, m.starfield.Update(tea.WindowSizeMsg{Width: m.width, Height: m.middleHeight()}))
		}
		if m.state == installStateForm {
			cmds = append(cmds, m.form.Update(msg))
		} else {
			cmds = append(cmds, m.activity.Update(msg))
		}
		return tea.Batch(cmds...)

	case starfieldTickMsg:
		if m.starfield != nil {
			return m.starfield.Update(msg)
		}
		return nil

	case logoShineStartMsg, logoShineStepMsg:
		if m.showLogo() && m.state == installStateForm {
			return m.logo.Update(msg)
		}
		return nil

	case MouseEvent:
		if m.state == installStateForm {
			if cmd := m.help.Update(msg); cmd != nil {
				return cmd
			}
		}

	case tea.KeyPressMsg:
		if m.state == installStateForm {
			if m.err != nil {
				m.err = nil
			}
			if key.Matches(msg, installKeys.Back) {
				return m.cancelFromScreen()
			}
		}

	case InstallFormCancelMsg:
		return m.cancelFromScreen()

	case InstallFormSubmitMsg:
		m.state = installStateActivity
		m.activity = NewInstallActivity(m.namespace, msg.ImageRef, msg.Hostname)
		sizeCmd := m.activity.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return tea.Batch(sizeCmd, m.activity.Init())

	case InstallActivityFailedMsg:
		m.state = installStateForm
		m.activity = nil
		m.err = msg.Err
		if m.showLogo() {
			return m.logo.Init()
		}
		return nil

	case InstallActivityDoneMsg:
		return func() tea.Msg { return NavigateToAppMsg(msg) }
	}

	var cmd tea.Cmd
	if m.state == installStateForm {
		cmd = m.form.Update(msg)
	} else {
		cmd = m.activity.Update(msg)
	}
	return cmd
}

func (m *Install) View() string {
	var contentView string
	if m.state == installStateForm {
		formView := m.form.View()
		if m.err != nil {
			errorLine := lipgloss.NewStyle().Foreground(Colors.Error).Render("Error: " + m.err.Error())
			formView = lipgloss.JoinVertical(lipgloss.Center, errorLine, "", formView)
		}
		if m.showLogo() {
			contentView = lipgloss.JoinVertical(lipgloss.Center, m.logo.View(), "", formView)
		} else {
			contentView = formView
		}
	} else {
		contentView = m.activity.View()
	}

	var helpLine string
	if m.state == installStateForm {
		helpView := m.help.View([]key.Binding{installKeys.Back})
		helpLine = Styles.CenteredLine(m.width, helpView)
	}

	if m.starfield != nil {
		middle := m.renderMiddleWithStarfield(contentView, m.middleHeight())
		return middle + helpLine
	}

	middle := m.renderMiddleCentered(contentView, m.middleHeight())
	titleLine := Styles.TitleRule(m.width, "install")
	return titleLine + "\n\n" + middle + helpLine
}

// Private

func (m *Install) showLogo() bool {
	return m.namespace == nil || len(m.namespace.Applications()) == 0
}

func (m *Install) middleHeight() int {
	helpHeight := 1 // help line when in form state
	if m.starfield != nil {
		return max(m.height-helpHeight, 0)
	}
	titleHeight := 2 // title + blank line
	return max(m.height-titleHeight-helpHeight, 0)
}

func (m *Install) cancelFromScreen() tea.Cmd {
	if m.activity != nil {
		m.activity.Cancel()
	}
	if m.cliMode {
		return func() tea.Msg { return QuitMsg{} }
	}
	return func() tea.Msg { return NavigateToDashboardMsg{} }
}

func (m *Install) renderMiddleCentered(contentView string, middleHeight int) string {
	centered := lipgloss.NewStyle().
		Width(m.width).
		Height(middleHeight).
		Align(lipgloss.Center, lipgloss.Center).
		Render(contentView)
	return centered
}

// renderMiddleWithStarfield composites the content view over the starfield background.
func (m *Install) renderMiddleWithStarfield(contentView string, middleHeight int) string {
	m.starfield.ComputeGrid()

	fgLines := strings.Split(contentView, "\n")
	fgHeight := len(fgLines)
	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}

	topOffset := (middleHeight - fgHeight) / 2
	leftOffset := (m.width - fgWidth) / 2

	var sb strings.Builder
	for row := range middleHeight {
		fgRow := row - topOffset
		if fgRow >= 0 && fgRow < fgHeight {
			sb.WriteString(m.starfield.RenderRow(row, 0, leftOffset))

			fgLine := fgLines[fgRow]
			if w := ansi.StringWidth(fgLine); w < fgWidth {
				fgLine += strings.Repeat(" ", fgWidth-w)
			}
			sb.WriteString(fgLine)

			sb.WriteString(m.starfield.RenderRow(row, leftOffset+fgWidth, m.width))
		} else {
			sb.WriteString(m.starfield.RenderFullRow(row))
		}
		if row < middleHeight-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
