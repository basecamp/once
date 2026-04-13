package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
	"github.com/basecamp/once/internal/system"
)

func TestFormatDuration(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		assert.Equal(t, "0s", formatDuration(0))
		assert.Equal(t, "1s", formatDuration(1*time.Second))
		assert.Equal(t, "45s", formatDuration(45*time.Second))
		assert.Equal(t, "59s", formatDuration(59*time.Second))
	})

	t.Run("minutes", func(t *testing.T) {
		assert.Equal(t, "1m", formatDuration(1*time.Minute))
		assert.Equal(t, "30m", formatDuration(30*time.Minute))
		assert.Equal(t, "59m", formatDuration(59*time.Minute))
		assert.Equal(t, "1m", formatDuration(1*time.Minute+30*time.Second))
	})

	t.Run("hours", func(t *testing.T) {
		assert.Equal(t, "1h", formatDuration(1*time.Hour))
		assert.Equal(t, "2h", formatDuration(2*time.Hour))
		assert.Equal(t, "3h 45m", formatDuration(3*time.Hour+45*time.Minute))
		assert.Equal(t, "23h 59m", formatDuration(23*time.Hour+59*time.Minute))
	})

	t.Run("days", func(t *testing.T) {
		assert.Equal(t, "1d", formatDuration(24*time.Hour))
		assert.Equal(t, "2d", formatDuration(48*time.Hour))
		assert.Equal(t, "1d 1h", formatDuration(25*time.Hour))
		assert.Equal(t, "2d 2h", formatDuration(50*time.Hour))
		assert.Equal(t, "7d 12h", formatDuration(7*24*time.Hour+12*time.Hour))
	})
}

func TestDashboardKeyboardSelectsPanel(t *testing.T) {
	d := testDashboard(3)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	assert.Equal(t, 0, d.selectedIndex)

	d, _ = updateDashboard(d, keyPressMsg("down"))
	assert.Equal(t, 1, d.selectedIndex)

	d, _ = updateDashboard(d, keyPressMsg("down"))
	assert.Equal(t, 2, d.selectedIndex)

	// Can't go past last
	d, _ = updateDashboard(d, keyPressMsg("down"))
	assert.Equal(t, 2, d.selectedIndex)

	d, _ = updateDashboard(d, keyPressMsg("up"))
	assert.Equal(t, 1, d.selectedIndex)

	d, _ = updateDashboard(d, keyPressMsg("up"))
	assert.Equal(t, 0, d.selectedIndex)

	// Can't go before first
	d, _ = updateDashboard(d, keyPressMsg("up"))
	assert.Equal(t, 0, d.selectedIndex)
}

func TestDashboardKeyboardJK(t *testing.T) {
	d := testDashboard(3)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, keyPressMsg("j"))
	assert.Equal(t, 1, d.selectedIndex)

	d, _ = updateDashboard(d, keyPressMsg("k"))
	assert.Equal(t, 0, d.selectedIndex)
}

func TestDashboard_ActionsMenuOpensOnA(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, keyPressMsg("a"))
	assert.NotNil(t, d.overlay)
}

func TestDashboard_ActionsMenuCloses(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, keyPressMsg("a"))
	assert.NotNil(t, d.overlay)

	d, _ = updateDashboard(d, ActionsMenuCloseMsg{})
	assert.Nil(t, d.overlay)
}

func TestDashboard_ActionsMenuStartStop(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, ActionsMenuSelectMsg{app: d.apps[0], action: ActionsMenuStartStop})
	assert.True(t, d.toggling)
}

func TestDashboard_ActionsMenuRemove(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	_, cmd := updateDashboard(d, ActionsMenuSelectMsg{app: d.apps[0], action: ActionsMenuRemove})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToRemoveMsg)
	require.True(t, ok, "expected NavigateToRemoveMsg, got %T", msg)
	assert.Equal(t, d.apps[0], navMsg.App)
}

func TestDashboard_OldStartStopKeyRemoved(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, keyPressMsg("o"))
	assert.False(t, d.toggling)
}

func TestDashboard_SettingsMenuStillWorks(t *testing.T) {
	d := testDashboard(1)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d, _ = updateDashboard(d, keyPressMsg("s"))
	assert.NotNil(t, d.overlay)
}

func TestDashboard_ToggleDetails(t *testing.T) {
	d := testDashboard(2)
	d.width = 120
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	assert.True(t, dashboardShowDetails)
	fullHeight := d.panels[0].Height(dashboardShowDetails)

	d, _ = updateDashboard(d, keyPressMsg("d"))
	assert.False(t, dashboardShowDetails)
	compactHeight := d.panels[0].Height(dashboardShowDetails)
	assert.Less(t, compactHeight, fullHeight)

	// Toggle back
	d, _ = updateDashboard(d, keyPressMsg("d"))
	assert.True(t, dashboardShowDetails)
}

func TestDashboard_ToggleDetailsEmptyState(t *testing.T) {
	d := testDashboard(0)
	d, _ = updateDashboard(d, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Should not panic
	_, _ = updateDashboard(d, keyPressMsg("d"))
	assert.True(t, dashboardShowDetails) // unchanged — guarded by len(m.apps) > 0
}

func TestDashboard_SelectPanelAfterAppsRemoved(t *testing.T) {
	d := testDashboard(3)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	// Simulate all apps disappearing (e.g. `once teardown` in another shell).
	d.apps = nil
	d.buildPanels()

	// Regression: #49 — scrollToSelection used to index into an empty m.panels.
	assert.NotPanics(t, func() {
		d.selectPanel(0)
	})
}

func TestDashboard_EmptyStateShowsMessage(t *testing.T) {
	d := testDashboard(0)
	d, _ = updateDashboard(d, tea.WindowSizeMsg{Width: 80, Height: 24})

	view := d.View()
	assert.Contains(t, view, "There are no applications installed")
	assert.Contains(t, view, "new app")
}

// Helpers

func testDashboard(numApps int) Dashboard {
	apps := make([]*docker.Application, numApps)
	for i := range numApps {
		apps[i] = &docker.Application{
			Running: true,
			Settings: docker.ApplicationSettings{
				Name: fmt.Sprintf("app-%d", i),
				Host: fmt.Sprintf("app-%d.example.com", i),
			},
		}
	}

	scraper := metrics.NewMetricsScraper(metrics.ScraperSettings{})
	dockerScraper := &docker.Scraper{}
	systemScraper := system.NewScraper(system.ScraperSettings{BufferSize: 10})

	dashboardShowDetails = true
	return NewDashboard(nil, apps, 0, scraper, dockerScraper, systemScraper, nil)
}

func updateDashboard(d Dashboard, msg tea.Msg) (Dashboard, tea.Cmd) {
	comp, cmd := d.Update(msg)
	return comp.(Dashboard), cmd
}
