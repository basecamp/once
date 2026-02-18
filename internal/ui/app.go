package ui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
	"github.com/basecamp/once/internal/version"
)

type KeyMap struct {
	Accept key.Binding
	Quit   key.Binding
}

var DefaultKeyMap = KeyMap{
	Accept: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "accept")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}

type Component interface {
	Init() tea.Cmd
	Update(tea.Msg) (Component, tea.Cmd)
	View() string
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
	currentScreen   Component
	lastSize        tea.WindowSizeMsg
	eventChan       <-chan struct{}
	watchCtx        context.Context
	watchCancel     context.CancelFunc
	installImageRef string
}

func NewApp(ns *docker.Namespace, installImageRef string) App {
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

	return App{
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

func (m App) Init() tea.Cmd {
	return tea.Batch(
		m.currentScreen.Init(),
		m.watchForChanges(),
		m.runScrape(),
		m.scheduleNextScrapeTick(),
	)
}

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.lastSize = msg
	case tea.KeyMsg:
		if key.Matches(msg, DefaultKeyMap.Quit) {
			m.shutdown()
			return m, tea.Quit
		}
	case namespaceChangedMsg:
		_ = m.namespace.Refresh(m.watchCtx)
		m.currentScreen, _ = m.currentScreen.Update(msg)
		return m, m.watchForChanges()
	case scrapeTickMsg:
		return m, tea.Batch(
			m.runScrape(),
			m.scheduleNextScrapeTick(),
		)
	case scrapeDoneMsg:
		m.currentScreen, _ = m.currentScreen.Update(msg)
	case navigateToInstallMsg:
		m.currentScreen = NewInstall(m.namespace, "")
		m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
		return m, m.currentScreen.Init()
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
		m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
		return m, m.currentScreen.Init()
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
			m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
			return m, m.currentScreen.Init()
		}
		m.shutdown()
		return m, tea.Quit
	case navigateToSettingsSectionMsg:
		m.currentScreen = NewSettings(m.namespace, msg.app, msg.section)
		m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
		return m, m.currentScreen.Init()
	case navigateToLogsMsg:
		m.currentScreen = NewLogs(m.namespace, msg.app)
		m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
		return m, m.currentScreen.Init()
	case quitMsg:
		m.shutdown()
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.currentScreen, cmd = m.currentScreen.Update(msg)
	return m, cmd
}

func (m App) View() tea.View {
	view := tea.View{
		AltScreen: true,
		MouseMode: tea.MouseModeCellMotion,
	}
	view.SetContent(zone.Scan(m.currentScreen.View()))
	return view
}

func Run(ns *docker.Namespace, installImageRef string) error {
	slog.Info("Starting ONCE UI", "version", version.Version)
	defer func() { slog.Info("Stopping ONCE UI") }()

	zone.NewGlobal()
	app := NewApp(ns, installImageRef)
	_, err := tea.NewProgram(app).Run()
	return err
}

// Private

func (m App) scheduleNextScrapeTick() tea.Cmd {
	return tea.Every(ChartUpdateInterval, func(time.Time) tea.Msg { return scrapeTickMsg{} })
}

func (m App) shutdown() {
	m.watchCancel()
}

func (m App) runScrape() tea.Cmd {
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

func (m App) watchForChanges() tea.Cmd {
	return func() tea.Msg {
		_, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return namespaceChangedMsg{}
	}
}
