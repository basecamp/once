package ui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
	"github.com/basecamp/once/internal/mouse"
	"github.com/basecamp/once/internal/version"
)

var appKeys = struct {
	Quit key.Binding
}{
	Quit: WithHelp(NewKeyBinding("ctrl+c"), "ctrl+c", "quit"),
}

type (
	namespaceChangedMsg          struct{}
	scrapeTickMsg                struct{}
	scrapeDoneMsg                struct{}
	NavigateToInstallMsg struct{}
	NavigateToDashboardMsg struct {
		AppName    string
		AllowEmpty bool
	}
	NavigateToAppMsg             struct{ App *docker.Application }
	NavigateToSettingsSectionMsg struct {
		App     *docker.Application
		Section SettingsSectionType
	}
)

type (
	NavigateToLogsMsg   struct{ App *docker.Application }
	NavigateToRemoveMsg struct{ App *docker.Application }
	QuitMsg             struct{}
)

type SettingsSectionType int

const (
	SettingsSectionApplication SettingsSectionType = iota
	SettingsSectionEmail
	SettingsSectionEnvironment
	SettingsSectionResources
	SettingsSectionUpdates
	SettingsSectionBackups
)

type App struct {
	namespace       *docker.Namespace
	scraper         *metrics.MetricsScraper
	dockerScraper   *docker.Scraper
	currentScreen   Component
	lastSize        tea.WindowSizeMsg
	eventChan       <-chan struct{}
	watchCtx        context.Context
	watchCancel     context.CancelFunc
	installImageRef string
}

func NewApp(ns *docker.Namespace, installImageRef string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	eventChan := ns.EventWatcher().Watch(ctx)

	apps := ns.Applications()

	metricsPort := docker.DefaultMetricsPort
	if ns.Proxy().Settings != nil && ns.Proxy().Settings.MetricsPort != 0 {
		metricsPort = ns.Proxy().Settings.MetricsPort
	}

	scraper := metrics.NewMetricsScraper(metrics.ScraperSettings{
		Port:       metricsPort,
		BufferSize: ChartHistoryLength,
	})

	dockerScraper := docker.NewScraper(ns, docker.ScraperSettings{
		BufferSize: ChartHistoryLength,
	})

	var screen Component
	if len(apps) > 0 && installImageRef == "" {
		screen = NewDashboard(ns, apps, 0, scraper, dockerScraper)
	} else {
		screen = NewInstall(ns, installImageRef)
	}

	return &App{
		namespace:       ns,
		scraper:         scraper,
		dockerScraper:   dockerScraper,
		currentScreen:   screen,
		eventChan:       eventChan,
		watchCtx:        ctx,
		watchCancel:     cancel,
		installImageRef: installImageRef,
	}
}

func (m *App) Init() tea.Cmd {
	return tea.Batch(
		m.currentScreen.Init(),
		m.watchForChanges(),
		m.runScrape(),
		m.scheduleNextScrapeTick(),
	)
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.lastSize = msg

	case tea.MouseClickMsg:
		ms := msg.Mouse()
		target := mouse.Resolve(ms.X, ms.Y)
		cmd := m.currentScreen.Update(MouseEvent{
			X:       ms.X,
			Y:       ms.Y,
			Button:  ms.Button,
			Target:  target,
			IsClick: true,
		})
		return m, cmd

	case tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
		return m, nil

	case tea.KeyPressMsg:
		if key.Matches(msg, appKeys.Quit) {
			m.shutdown()
			return m, tea.Quit
		}

	case namespaceChangedMsg:
		if err := m.namespace.Refresh(m.watchCtx); err != nil {
			slog.Error("refreshing namespace", "err", err)
		}
		cmd := m.currentScreen.Update(msg)
		return m, tea.Batch(cmd, m.watchForChanges())

	case scrapeTickMsg:
		return m, tea.Batch(
			m.runScrape(),
			m.scheduleNextScrapeTick(),
		)

	case scrapeDoneMsg:
		cmd := m.currentScreen.Update(msg)
		return m, cmd

	case NavigateToInstallMsg:
		m.currentScreen = NewInstall(m.namespace, "")
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case NavigateToAppMsg:
		if err := m.namespace.Refresh(m.watchCtx); err != nil {
			slog.Error("refreshing namespace", "err", err)
		}
		apps := m.namespace.Applications()
		targetIndex := 0
		for i, app := range apps {
			if app.Settings.Name == msg.App.Settings.Name {
				targetIndex = i
				break
			}
		}
		m.currentScreen = NewDashboard(m.namespace, apps, targetIndex, m.scraper, m.dockerScraper)
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case NavigateToDashboardMsg:
		if err := m.namespace.Refresh(m.watchCtx); err != nil {
			slog.Error("refreshing namespace", "err", err)
		}
		apps := m.namespace.Applications()
		if len(apps) == 0 && !msg.AllowEmpty {
			m.shutdown()
			return m, tea.Quit
		}
		selectedIndex := 0
		for i, app := range apps {
			if app.Settings.Name == msg.AppName {
				selectedIndex = i
				break
			}
		}
		m.currentScreen = NewDashboard(m.namespace, apps, selectedIndex, m.scraper, m.dockerScraper)
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case NavigateToSettingsSectionMsg:
		m.currentScreen = NewSettings(m.namespace, msg.App, msg.Section)
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case NavigateToRemoveMsg:
		m.currentScreen = NewRemove(m.namespace, msg.App)
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case NavigateToLogsMsg:
		m.currentScreen = NewLogs(m.namespace, msg.App)
		sizeCmd := m.currentScreen.Update(m.lastSize)
		return m, tea.Batch(sizeCmd, m.currentScreen.Init())

	case QuitMsg:
		m.shutdown()
		return m, tea.Quit
	}

	cmd := m.currentScreen.Update(msg)
	return m, cmd
}

func (m *App) View() tea.View {
	content := m.currentScreen.View()
	cleaned := mouse.Sweep(content)

	v := tea.NewView(cleaned)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func Run(ns *docker.Namespace, installImageRef string) error {
	slog.Info("Starting ONCE UI", "version", version.Version)
	defer func() { slog.Info("Stopping ONCE UI") }()

	app := NewApp(ns, installImageRef)
	_, err := tea.NewProgram(app).Run()
	return err
}

// Private

func (m *App) scheduleNextScrapeTick() tea.Cmd {
	return tea.Every(ChartUpdateInterval, func(time.Time) tea.Msg { return scrapeTickMsg{} })
}

func (m *App) shutdown() {
	m.watchCancel()
}

func (m *App) runScrape() tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.scraper.Scrape(m.watchCtx)
		}()
		go func() {
			defer wg.Done()
			m.dockerScraper.Scrape(m.watchCtx)
		}()
		wg.Wait()
		return scrapeDoneMsg{}
	}
}

func (m *App) watchForChanges() tea.Cmd {
	return func() tea.Msg {
		_, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return namespaceChangedMsg{}
	}
}
