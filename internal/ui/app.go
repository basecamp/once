package ui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/basecamp/gliff/tui"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
	"github.com/basecamp/once/internal/version"
)

var appKeys = struct {
	Quit KeyBinding
}{
	Quit: NewKeyBinding(Key(tui.KeyCtrlC)).WithHelp("ctrl+c", "quit"),
}

type (
	namespaceChangedMsg          struct{}
	scrapeTickMsg                struct{}
	scrapeDoneMsg                struct{}
	navigateToInstallMsg         struct{}
	navigateToDashboardMsg       struct{ appName string }
	navigateToAppMsg             struct{ app *docker.Application }
	navigateToSettingsSectionMsg struct {
		app     *docker.Application
		section SettingsSectionType
	}
)

type (
	navigateToLogsMsg struct{ app *docker.Application }
	quitMsg           struct{}
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
	currentScreen   tui.Component
	lastSize        tui.WindowSizeMsg
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

	var screen tui.Component
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

func (m *App) Init() tui.Cmd {
	return tui.Batch(
		m.currentScreen.Init(),
		m.watchForChanges(),
		m.runScrape(),
		m.scheduleNextScrapeTick(),
	)
}

func (m *App) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		m.lastSize = msg
	case tui.KeyMsg:
		if appKeys.Quit.Matches(msg) {
			m.shutdown()
			return func() tui.Msg { return tui.QuitMsg{} }
		}
	case namespaceChangedMsg:
		_ = m.namespace.Refresh(m.watchCtx)
		m.currentScreen.Update(msg)
		return m.watchForChanges()
	case scrapeTickMsg:
		return tui.Batch(
			m.runScrape(),
			m.scheduleNextScrapeTick(),
		)
	case scrapeDoneMsg:
		m.currentScreen.Update(msg)
	case navigateToInstallMsg:
		m.currentScreen = NewInstall(m.namespace, "")
		m.currentScreen.Update(m.lastSize)
		return m.currentScreen.Init()
	case navigateToAppMsg:
		_ = m.namespace.Refresh(m.watchCtx)
		apps := m.namespace.Applications()
		targetIndex := 0
		for i, app := range apps {
			if app.Settings.Name == msg.app.Settings.Name {
				targetIndex = i
				break
			}
		}
		m.currentScreen = NewDashboard(m.namespace, apps, targetIndex, m.scraper, m.dockerScraper)
		m.currentScreen.Update(m.lastSize)
		return m.currentScreen.Init()
	case navigateToDashboardMsg:
		_ = m.namespace.Refresh(m.watchCtx)
		apps := m.namespace.Applications()
		if len(apps) > 0 {
			selectedIndex := 0
			for i, app := range apps {
				if app.Settings.Name == msg.appName {
					selectedIndex = i
					break
				}
			}
			m.currentScreen = NewDashboard(m.namespace, apps, selectedIndex, m.scraper, m.dockerScraper)
			m.currentScreen.Update(m.lastSize)
			return m.currentScreen.Init()
		}
		m.shutdown()
		return func() tui.Msg { return tui.QuitMsg{} }
	case navigateToSettingsSectionMsg:
		m.currentScreen = NewSettings(m.namespace, msg.app, msg.section)
		m.currentScreen.Update(m.lastSize)
		return m.currentScreen.Init()
	case navigateToLogsMsg:
		m.currentScreen = NewLogs(m.namespace, msg.app)
		m.currentScreen.Update(m.lastSize)
		return m.currentScreen.Init()
	case quitMsg:
		m.shutdown()
		return func() tui.Msg { return tui.QuitMsg{} }
	}

	return m.currentScreen.Update(msg)
}

func (m *App) Render() string {
	return m.currentScreen.Render()
}

func Run(ns *docker.Namespace, installImageRef string) error {
	slog.Info("Starting ONCE UI", "version", version.Version)
	defer func() { slog.Info("Stopping ONCE UI") }()

	app := NewApp(ns, installImageRef)
	return tui.NewApplication(app).Run()
}

// Private

func (m *App) scheduleNextScrapeTick() tui.Cmd {
	return tui.Every(ChartUpdateInterval, func(time.Time) tui.Msg { return scrapeTickMsg{} })
}

func (m *App) shutdown() {
	m.watchCancel()
}

func (m *App) runScrape() tui.Cmd {
	return func() tui.Msg {
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

func (m *App) watchForChanges() tui.Cmd {
	return func() tui.Msg {
		_, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return namespaceChangedMsg{}
	}
}
