package ui

import (
	"context"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	registry      docker.RegistrySettings
	width, height int
	stage         installStage
	percentage    int
	progress      Progress
	progressChan  chan installProgressMsg
	doneChan      chan installDoneMsg
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewInstallActivity(ns *docker.Namespace, imageRef, hostname string, registry docker.RegistrySettings) *InstallActivity {
	ctx, cancel := context.WithCancel(context.Background())
	return &InstallActivity{
		namespace:    ns,
		imageRef:     imageRef,
		hostname:     hostname,
		registry:     registry,
		stage:        stagePreparing,
		progress:     NewProgress(0, Colors.Primary),
		progressChan: make(chan installProgressMsg, 10),
		doneChan:     make(chan installDoneMsg, 1),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (m *InstallActivity) Init() tea.Cmd {
	return tea.Batch(m.progress.Init(), m.startInstall(), m.waitForProgress())
}

func (m *InstallActivity) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress = m.progress.SetWidth(min(m.width-4, 60))

	case installProgressMsg:
		m.stage = msg.stage
		m.percentage = msg.percentage
		switch msg.stage {
		case stageDownloading:
			m.progress = m.progress.SetPercent(msg.percentage)
		default:
			m.progress = m.progress.SetPercent(-1)
		}
		return m.waitForProgress()

	case installDoneMsg:
		if msg.err != nil {
			return func() tea.Msg { return InstallActivityFailedMsg{Err: msg.err} }
		}
		return func() tea.Msg { return InstallActivityDoneMsg{App: msg.app} }

	case ProgressTickMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return cmd
	}

	return nil
}

func (m *InstallActivity) View() string {
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

	progressView := Styles.CenteredLine(m.width, m.progress.View())

	return lipgloss.JoinVertical(lipgloss.Left, statusLine, progressView)
}

func (m *InstallActivity) Cancel() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Private

func (m *InstallActivity) startInstall() tea.Cmd {
	return func() tea.Msg {
		go m.runInstall(m.ctx)
		return nil
	}
}

func (m *InstallActivity) waitForProgress() tea.Cmd {
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

func (m *InstallActivity) runInstall(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			m.doneChan <- installDoneMsg{err: fmt.Errorf("install panicked: %v", r)}
		}
	}()

	m.progressChan <- installProgressMsg{stage: stagePreparing}

	if err := m.namespace.Setup(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageDownloading, percentage: 0}

	appName, err := m.namespace.UniqueName(docker.NameFromImageRef(m.imageRef))
	if err != nil {
		m.doneChan <- installDoneMsg{err: fmt.Errorf("generating app name: %w", err)}
		return
	}
	hostname := m.hostname

	app := docker.NewApplication(m.namespace, docker.ApplicationSettings{
		Name:       appName,
		Image:      m.imageRef,
		Registry:   m.registry,
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
		if cleanupErr := app.Destroy(context.Background(), true); cleanupErr != nil {
			slog.Error("Failed to clean up after deploy failure", "app", appName, "error", cleanupErr)
		}
		m.doneChan <- installDoneMsg{err: fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)}
		return
	}

	m.progressChan <- installProgressMsg{stage: stageVerifying}

	if err := app.VerifyHTTPOrRemove(ctx); err != nil {
		m.doneChan <- installDoneMsg{err: err}
		return
	}

	m.doneChan <- installDoneMsg{app: app}
}
