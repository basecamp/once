package ui

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/gliff/components"
	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
)

type installStage int

const (
	stagePreparing installStage = iota
	stageDownloading
	stageStarting
	stageVerifying
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

type InstallActivityFailedMsg struct {
	Err error
}

type InstallActivity struct {
	namespace     *docker.Namespace
	imageRef      string
	hostname      string
	width, height int
	stage         installStage
	percentage    int
	progressBar   *components.ProgressBar
	progressBusy  *components.ProgressBusy
	progressChan  chan installProgressMsg
	doneChan      chan installDoneMsg
}

func NewInstallActivity(ns *docker.Namespace, imageRef, hostname string) *InstallActivity {
	return &InstallActivity{
		namespace:    ns,
		imageRef:     imageRef,
		hostname:     hostname,
		stage:        stagePreparing,
		progressChan: make(chan installProgressMsg, 10),
		doneChan:     make(chan installDoneMsg, 1),
	}
}

func (m *InstallActivity) Init() tui.Cmd {
	var busyInit tui.Cmd
	if m.progressBusy != nil {
		busyInit = m.progressBusy.Init()
	}
	return tui.Batch(busyInit, m.startInstall(), m.waitForProgress())
}

func (m *InstallActivity) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		progressWidth := min(m.width-4, 60)
		m.progressBar = components.NewProgressBar(progressWidth, Colors.Primary)
		m.progressBar.Total = 100
		m.progressBusy = components.NewProgressBusy(progressWidth, Colors.Primary)

	case installProgressMsg:
		m.stage = msg.stage
		m.percentage = msg.percentage
		if m.progressBar != nil {
			m.progressBar.Current = float64(msg.percentage)
		}
		if msg.stage == stageStarting || msg.stage == stageVerifying {
			var busyInit tui.Cmd
			if m.progressBusy != nil {
				busyInit = m.progressBusy.Init()
			}
			return tui.Batch(busyInit, m.waitForProgress())
		}
		return m.waitForProgress()

	case installDoneMsg:
		if msg.err != nil {
			return func() tui.Msg { return InstallActivityFailedMsg{Err: msg.err} }
		}
		return func() tui.Msg { return InstallActivityDoneMsg{App: msg.app} }

	case components.ProgressBusyTickMsg:
		if m.progressBusy != nil {
			return m.progressBusy.Update(msg)
		}
	}

	return nil
}

func (m *InstallActivity) Render() string {
	var status string
	switch m.stage {
	case stagePreparing:
		status = "Preparing..."
	case stageDownloading:
		status = "Downloading..."
	case stageStarting:
		status = "Starting..."
	case stageVerifying:
		status = "Verifying..."
	}

	statusLine := Styles.CenteredLine(m.width, status)

	var progressView string
	switch m.stage {
	case stagePreparing, stageStarting, stageVerifying:
		if m.progressBusy != nil {
			progressView = Styles.CenteredLine(m.width, m.progressBusy.Render())
		}
	case stageDownloading:
		if m.progressBar != nil {
			progressView = Styles.CenteredLine(m.width, m.progressBar.Render())
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, progressView)
}

// Private

func (m *InstallActivity) startInstall() tui.Cmd {
	return func() tui.Msg {
		go m.runInstall()
		return nil
	}
}

func (m *InstallActivity) waitForProgress() tui.Cmd {
	return func() tui.Msg {
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

func (m *InstallActivity) runInstall() {
	ctx := context.Background()

	m.progressChan <- installProgressMsg{stage: stagePreparing}

	if err := m.namespace.Setup(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: 0}

	appName := m.namespace.UniqueName(docker.NameFromImageRef(m.imageRef))
	hostname := m.hostname
	if hostname == "" {
		hostname = appName + ".localhost"
	}

	app := m.namespace.AddApplication(docker.ApplicationSettings{
		Name:       appName,
		Image:      m.imageRef,
		Host:       hostname,
		AutoUpdate: true,
	})

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: p.Percentage}
		case docker.DeployStageStarting:
			m.progressChan <- installProgressMsg{stage: stageStarting, percentage: 100}
		}
	}

	if err := app.Deploy(ctx, progress); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageVerifying}

	if err := app.VerifyHTTP(ctx); err != nil {
		app.Destroy(ctx, true)
		m.doneChan <- installDoneMsg{err: err}
		return
	}

	m.doneChan <- installDoneMsg{app: app}
}
