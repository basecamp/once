package ui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
	"github.com/basecamp/once/internal/mouse"
	"github.com/basecamp/once/internal/system"
	"github.com/basecamp/once/internal/userstats"
	"github.com/basecamp/once/internal/version"
)

var appKeys = struct {
	Quit key.Binding
}{
	Quit: WithHelp(NewKeyBinding("ctrl+c"), "ctrl+c", "quit"),
}

type (
	namespaceChangedMsg    struct{}
	scrapeTickMsg          struct{}
	scrapeUserStatsTickMsg struct{}
	scrapeDoneMsg          struct{}

	NavigateToInstallMsg   struct{}
	NavigateToDashboardMsg struct {
		AppName    string
		AllowEmpty bool
	}
	NavigateToAppMsg             struct{ App *docker.Application }
	NavigateToSettingsSectionMsg struct {
		App     *docker.Application
		Section SettingsSectionType
	}
	NavigateToLogsMsg   struct{ App *docker.Application }
	NavigateToRemoveMsg struct{ App *docker.Application }

	QuitMsg struct{}
)

type SettingsSectionType int

const (
	SettingsSectionApplication SettingsSectionType = iota
	SettingsSectionEmail
	SettingsSectionEnvironment
	SettingsSectionResources
	SettingsSectionUpdates
	SettingsSectionBackups
	SettingsSectionMounts
)

type App struct {
	namespace       *docker.Namespace
	scraper         *metrics.MetricsScraper
	dockerScraper   *docker.Scraper
	systemScraper   *system.Scraper
	userStats       *userstats.Reader
	currentScreen   Component
	sizeGuard       TerminalSizeGuard
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
		BufferSize: ChartSlidingWindow,
	})

	dockerScraper := docker.NewScraper(ns, docker.ScraperSettings{
		BufferSize: containerStatsBuffer,
	})

	diskPath, err := ns.DockerRootDir(ctx)
	if err != nil {
		slog.Warn("failed to get Docker root dir, using default", "err", err)
		diskPath = "/var/lib/docker"
	}

	systemScraper := system.NewScraper(system.ScraperSettings{
		BufferSize: ChartHistoryLength,
		DiskPath:   diskPath,
	})

	userStats := userstats.NewReader(ns.Name())

	var screen Component
	if len(apps) > 0 && installImageRef == "" {
		screen = NewDashboard(ns, apps, 0, scraper, dockerScraper, systemScraper, userStats)
	} else {
		screen = NewInstall(ns, installImageRef)
	}

	return &App{
		namespace:       ns,
		scraper:         scraper,
		dockerScraper:   dockerScraper,
		systemScraper:   systemScraper,
		userStats:       userStats,
		currentScreen:   screen,
		sizeGuard:       NewTerminalSizeGuard(80, 24),
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
		m.runUserStatsScrape(),
		m.scheduleNextUserStatsTick(),
	)
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.lastSize = msg
		m.sizeGuard = m.sizeGuard.Update(msg)

	case tea.MouseClickMsg:
		ms := msg.Mouse()
		target := mouse.Resolve(ms.X, ms.Y)
		var cmd tea.Cmd
		m.currentScreen, cmd = m.currentScreen.Update(MouseEvent{
			X:       ms.X,
			Y:       ms.Y,
			Button:  ms.Button,
			Target:  target,
			IsClick: true,
		})
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.currentScreen, cmd = m.currentScreen.Update(msg)
		return m, cmd

	case tea.MouseReleaseMsg, tea.MouseMotionMsg:
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
		var cmd tea.Cmd
		m.currentScreen, cmd = m.currentScreen.Update(msg)
		return m, tea.Batch(cmd, m.watchForChanges())

	case scrapeTickMsg:
		return m, tea.Batch(
			m.runScrape(),
			m.scheduleNextScrapeTick(),
		)

	case scrapeUserStatsTickMsg:
		return m, tea.Batch(
			m.runUserStatsScrape(),
			m.scheduleNextUserStatsTick(),
		)

	case scrapeDoneMsg:
		var cmd tea.Cmd
		m.currentScreen, cmd = m.currentScreen.Update(msg)
		return m, cmd

	case NavigateToInstallMsg:
		return m, m.navigateTo(NewInstall(m.namespace, ""))

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
		return m, m.navigateTo(NewDashboard(m.namespace, apps, targetIndex, m.scraper, m.dockerScraper, m.systemScraper, m.userStats))

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
		return m, m.navigateTo(NewDashboard(m.namespace, apps, selectedIndex, m.scraper, m.dockerScraper, m.systemScraper, m.userStats))

	case NavigateToSettingsSectionMsg:
		return m, m.navigateTo(NewSettings(m.namespace, msg.App, msg.Section))

	case NavigateToRemoveMsg:
		return m, m.navigateTo(NewRemove(m.namespace, msg.App))

	case NavigateToLogsMsg:
		return m, m.navigateTo(NewLogs(m.namespace, msg.App))

	case QuitMsg:
		m.shutdown()
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.currentScreen, cmd = m.currentScreen.Update(msg)
	return m, cmd
}

func (m *App) View() tea.View {
	var content string
	if m.sizeGuard.LargeEnough() {
		content = mouse.Sweep(m.currentScreen.View())
	} else {
		content = m.sizeGuard.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func Run(ns *docker.Namespace, installImageRef string) error {
	slog.Info("Starting ONCE UI", "version", version.Version)
	defer func() { slog.Info("Stopping ONCE UI") }()

	detected := DetectTerminalColors(100 * time.Millisecond)
	ApplyPalette(NewPalette(detected))

	app := NewApp(ns, installImageRef)

	var opts []tea.ProgramOption
	if detected.SupportsTrueColor() {
		opts = append(opts, tea.WithColorProfile(colorprofile.TrueColor))
	}

	_, err := tea.NewProgram(app, opts...).Run()
	return err
}

// Private

func (m *App) scheduleNextScrapeTick() tea.Cmd {
	return tea.Every(ChartUpdateInterval, func(time.Time) tea.Msg { return scrapeTickMsg{} })
}

func (m *App) scheduleNextUserStatsTick() tea.Cmd {
	return tea.Every(UserStatsUpdateInterval, func(time.Time) tea.Msg { return scrapeUserStatsTickMsg{} })
}

func (m *App) navigateTo(screen Component) tea.Cmd {
	m.currentScreen = screen
	var sizeCmd tea.Cmd
	m.currentScreen, sizeCmd = m.currentScreen.Update(m.lastSize)
	return tea.Batch(sizeCmd, m.currentScreen.Init())
}

func (m *App) shutdown() {
	m.watchCancel()
}

func (m *App) runScrape() tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		wg.Go(func() { m.scraper.Scrape(m.watchCtx) })
		wg.Go(func() { m.dockerScraper.Scrape(m.watchCtx) })
		wg.Go(func() { m.systemScraper.Scrape(m.watchCtx) })
		wg.Wait()
		return scrapeDoneMsg{}
	}
}

func (m *App) runUserStatsScrape() tea.Cmd {
	return func() tea.Msg {
		m.userStats.Scrape(m.watchCtx)
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
