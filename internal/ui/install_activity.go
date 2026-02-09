package ui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type installStage int

const (
	stagePreparing installStage = iota
	stageDownloading
	stageStarting
	stageFinished
	stageFailed
)

type installProgressMsg struct {
	stage      installStage
	percentage int
}

type installDoneMsg struct {
	app *docker.Application
	err error
}

type InstallActivityDoneMsg struct {
	App *docker.Application
}

type InstallActivity struct {
	namespace     *docker.Namespace
	imageRef      string
	hostname      string
	width, height int
	stage         installStage
	percentage    int
	progressBar   ProgressBar
	progressBusy  ProgressBusy
	err           error
	app           *docker.Application
	focused       bool
	progressChan  chan installProgressMsg
	doneChan      chan installDoneMsg
}

func NewInstallActivity(ns *docker.Namespace, imageRef, hostname string) InstallActivity {
	return InstallActivity{
		namespace:    ns,
		imageRef:     imageRef,
		hostname:     hostname,
		stage:        stagePreparing,
		focused:      false,
		progressChan: make(chan installProgressMsg, 10),
		doneChan:     make(chan installDoneMsg, 1),
	}
}

func (m InstallActivity) Init() tea.Cmd {
	return tea.Batch(m.progressBusy.Init(), m.startInstall(), m.waitForProgress())
}

func (m InstallActivity) Update(msg tea.Msg) (InstallActivity, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		progressWidth := min(m.width-4, 60)
		m.progressBar = NewProgressBar(progressWidth, Colors.Primary)
		m.progressBar.Total = 100
		m.progressBusy = NewProgressBusy(progressWidth, Colors.Primary)

	case tea.KeyMsg:
		if m.stage == stageFinished || m.stage == stageFailed {
			if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
				if m.stage == stageFinished {
					return m, func() tea.Msg { return InstallActivityDoneMsg{App: m.app} }
				}
				return m, func() tea.Msg { return navigateToDashboardMsg{} }
			}
		}

	case installProgressMsg:
		m.stage = msg.stage
		m.percentage = msg.percentage
		m.progressBar.Current = float64(msg.percentage)
		if msg.stage == stageStarting {
			return m, tea.Batch(m.progressBusy.Init(), m.waitForProgress())
		}
		return m, m.waitForProgress()

	case installDoneMsg:
		if msg.err != nil {
			m.stage = stageFailed
			m.err = msg.err
		} else {
			m.stage = stageFinished
			m.app = msg.app
		}
		m.focused = true

	case progressBusyTickMsg:
		var cmd tea.Cmd
		m.progressBusy, cmd = m.progressBusy.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m InstallActivity) View() string {
	var status string
	switch m.stage {
	case stagePreparing:
		status = "Preparing..."
	case stageDownloading:
		status = "Downloading..."
	case stageStarting:
		status = "Starting..."
	case stageFinished:
		status = "Installation complete!"
	case stageFailed:
		status = "Installation failed: " + m.err.Error()
	}

	statusLine := Styles.CenteredLine(m.width, status)

	var progressView string
	switch m.stage {
	case stagePreparing, stageStarting:
		progressView = Styles.CenteredLine(m.width, m.progressBusy.View())
	case stageDownloading:
		progressView = Styles.CenteredLine(m.width, m.progressBar.View())
	}

	var buttonView string
	if m.stage == stageFinished || m.stage == stageFailed {
		buttonStyle := Styles.Button

		var label string
		if m.stage == stageFinished {
			label = "Done"
		} else {
			label = "Back"
		}

		if m.focused {
			buttonStyle = buttonStyle.BorderForeground(Colors.Focused)
		} else {
			buttonStyle = buttonStyle.BorderForeground(Colors.Primary)
		}

		buttonView = lipgloss.NewStyle().
			Width(m.width).
			Align(lipgloss.Center).
			MarginTop(1).
			Render(buttonStyle.Render(label))
	}

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, progressView, buttonView)
}

// Private

func (m InstallActivity) startInstall() tea.Cmd {
	return func() tea.Msg {
		go m.runInstall()
		return nil
	}
}

func (m InstallActivity) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		select {
		case progress, ok := <-m.progressChan:
			if ok {
				return progress
			}
		case done := <-m.doneChan:
			return done
		}
		return nil
	}
}

func (m InstallActivity) runInstall() {
	ctx := context.Background()

	m.progressChan <- installProgressMsg{stage: stagePreparing}

	if err := m.namespace.Setup(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: err}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: 0}

	appName := docker.NameFromImageRef(m.imageRef)
	hostname := m.hostname
	if hostname == "" {
		hostname = appName + ".localhost"
	}

	app := m.namespace.AddApplication(docker.ApplicationSettings{
		Name:  appName,
		Image: m.imageRef,
		Host:  hostname,
	})

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: p.Percentage}
		case docker.DeployStageStarting:
			m.progressChan <- installProgressMsg{stage: stageStarting, percentage: 100}
		}
	}

	err := app.Deploy(ctx, progress)
	m.doneChan <- installDoneMsg{app: app, err: err}
}
