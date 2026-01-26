package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/amar/internal/docker"
	"github.com/basecamp/amar/internal/metrics"
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

type NamespaceChangedMsg struct{}
type scrapeTickMsg struct{}
type scrapeDoneMsg struct{}
type navigateToInstallMsg struct{}
type navigateToDashboardMsg struct{}
type navigateToAppMsg struct{ appName string }
type quitMsg struct{}
type switchAppMsg struct{ delta int }

type App struct {
	namespace      *docker.Namespace
	scraper        *metrics.MetricsScraper
	dockerScraper  *docker.Scraper
	currentIndex   int
	currentScreen  Component
	lastSize       tea.WindowSizeMsg
	eventChan      <-chan struct{}
	watchCtx       context.Context
	watchCancel    context.CancelFunc
}

func NewApp(ns *docker.Namespace) App {
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
	if len(apps) > 0 {
		screen = NewDashboard(ns, apps[0], scraper, dockerScraper)
	} else {
		screen = NewInstall(ns)
	}

	return App{
		namespace:      ns,
		scraper:        scraper,
		dockerScraper:  dockerScraper,
		currentIndex:   0,
		currentScreen:  screen,
		eventChan:      eventChan,
		watchCtx:       ctx,
		watchCancel:    cancel,
	}
}

func (m App) Init() tea.Cmd {
	return tea.Batch(
		m.currentScreen.Init(),
		m.watchForChanges(),
		m.runScrape(),
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
	case NamespaceChangedMsg:
		m.currentScreen, _ = m.currentScreen.Update(msg)
		return m, m.watchForChanges()
	case scrapeTickMsg:
		return m, m.runScrape()
	case scrapeDoneMsg:
		return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return scrapeTickMsg{} })
	case navigateToInstallMsg:
		m.currentScreen = NewInstall(m.namespace)
		m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
		return m, m.currentScreen.Init()
	case navigateToAppMsg:
		_ = m.namespace.Refresh(m.watchCtx)
		apps := m.namespace.Applications()
		for i, app := range apps {
			if app.Settings.Name == msg.appName {
				m.currentIndex = i
				m.currentScreen = NewDashboard(m.namespace, app, m.scraper, m.dockerScraper)
				m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
				return m, m.currentScreen.Init()
			}
		}
		return m, nil
	case navigateToDashboardMsg:
		apps := m.namespace.Applications()
		if len(apps) > 0 && m.currentIndex < len(apps) {
			m.currentScreen = NewDashboard(m.namespace, apps[m.currentIndex], m.scraper, m.dockerScraper)
			m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
			return m, m.currentScreen.Init()
		}
		m.shutdown()
		return m, tea.Quit
	case quitMsg:
		m.shutdown()
		return m, tea.Quit
	case switchAppMsg:
		return m.switchApp(msg.delta)
	}

	var cmd tea.Cmd
	m.currentScreen, cmd = m.currentScreen.Update(msg)
	return m, cmd
}

func (m App) View() tea.View {
	view := tea.View{AltScreen: true}
	view.SetContent(m.currentScreen.View())
	return view
}

func Run(ns *docker.Namespace) error {
	app := NewApp(ns)
	_, err := tea.NewProgram(app).Run()
	return err
}

// Private

func (m App) shutdown() {
	m.watchCancel()
}

func (m App) runScrape() tea.Cmd {
	return func() tea.Msg {
		m.scraper.Scrape(m.watchCtx)
		m.dockerScraper.Scrape(m.watchCtx)
		return scrapeDoneMsg{}
	}
}

func (m App) switchApp(delta int) (tea.Model, tea.Cmd) {
	apps := m.namespace.Applications()
	if len(apps) == 0 {
		return m, nil
	}

	newIndex := m.currentIndex + delta
	if newIndex < 0 {
		newIndex = len(apps) - 1
	} else if newIndex >= len(apps) {
		newIndex = 0
	}

	if newIndex == m.currentIndex {
		return m, nil
	}

	m.currentIndex = newIndex
	m.currentScreen = NewDashboard(m.namespace, apps[newIndex], m.scraper, m.dockerScraper)
	m.currentScreen, _ = m.currentScreen.Update(m.lastSize)
	return m, m.currentScreen.Init()
}

func (m App) watchForChanges() tea.Cmd {
	return func() tea.Msg {
		_, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return NamespaceChangedMsg{}
	}
}
