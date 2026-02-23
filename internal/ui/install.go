package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/tui"
	"github.com/charmbracelet/x/ansi"

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
	starfield     *Starfield
	err           error
}

func NewInstall(ns *docker.Namespace, imageRef string) *Install {
	return &Install{
		namespace: ns,
		help:      NewHelp(),
		state:     installStateForm,
		form:      NewInstallForm(imageRef),
		starfield: NewStarfield(),
	}
}

func (m *Install) Init() tui.Cmd {
	return tui.Batch(m.form.Init(), m.starfield.Init())
}

func (m *Install) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.starfield.Update(tui.WindowSizeMsg{Width: m.width, Height: m.middleHeight()})
		if m.state == installStateForm {
			m.form.Update(msg)
		} else {
			m.activity.Update(msg)
		}

	case starfieldTickMsg:
		return m.starfield.Update(msg)

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

	middle := m.renderMiddle(contentView, m.middleHeight())

	return titleLine + "\n\n" + middle + helpLine
}

// Private

func (m *Install) middleHeight() int {
	titleHeight := 2 // title + blank line
	helpHeight := 1  // help line when in form state
	return max(m.height-titleHeight-helpHeight, 0)
}

func (m *Install) cancelFromScreen() tui.Cmd {
	if m.form.ImageRef() != "" {
		return func() tui.Msg { return quitMsg{} }
	}
	return func() tui.Msg { return navigateToDashboardMsg{} }
}

// renderMiddle composites the content view over the starfield background.
// It builds the output row by row, writing starfield cells directly on the
// edges and inserting the content lines in the center. This avoids using
// OverlayCenter and its ANSI-aware string slicing, which can leak escape
// sequences from the background into the composed output and confuse the
// renderer's incremental diff.
func (m *Install) renderMiddle(contentView string, middleHeight int) string {
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
			sb.WriteString(starReset)

			fgLine := fgLines[fgRow]
			if w := ansi.StringWidth(fgLine); w < fgWidth {
				fgLine += strings.Repeat(" ", fgWidth-w)
			}
			sb.WriteString(fgLine)

			sb.WriteString(starReset)
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
