package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/basecamp/gliff/tui"
	"github.com/stretchr/testify/assert"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
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

	d.Update(keyMsg(tui.KeyDown, 0))
	assert.Equal(t, 1, d.selectedIndex)

	d.Update(keyMsg(tui.KeyDown, 0))
	assert.Equal(t, 2, d.selectedIndex)

	// Can't go past last
	d.Update(keyMsg(tui.KeyDown, 0))
	assert.Equal(t, 2, d.selectedIndex)

	d.Update(keyMsg(tui.KeyUp, 0))
	assert.Equal(t, 1, d.selectedIndex)

	d.Update(keyMsg(tui.KeyUp, 0))
	assert.Equal(t, 0, d.selectedIndex)

	// Can't go before first
	d.Update(keyMsg(tui.KeyUp, 0))
	assert.Equal(t, 0, d.selectedIndex)
}

func TestDashboardKeyboardJK(t *testing.T) {
	d := testDashboard(3)
	d.width = 80
	d.height = 40
	d.updateViewportSize()
	d.rebuildViewportContent()

	d.Update(runeMsg('j'))
	assert.Equal(t, 1, d.selectedIndex)

	d.Update(runeMsg('k'))
	assert.Equal(t, 0, d.selectedIndex)
}

// Helpers

func testDashboard(numApps int) *Dashboard {
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

	return NewDashboard(nil, apps, 0, scraper, dockerScraper)
}
