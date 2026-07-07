package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPullProgressTracker(t *testing.T) {
	events := `{"status":"Pulling from basecamp/once-campfire","id":"latest"}
{"status":"Pulling fs layer","id":"layer1"}
{"status":"Pulling fs layer","id":"layer2"}
{"status":"Downloading","progressDetail":{"current":0,"total":1000},"id":"layer1"}
{"status":"Downloading","progressDetail":{"current":500,"total":1000},"id":"layer1"}
{"status":"Downloading","progressDetail":{"current":0,"total":2000},"id":"layer2"}
{"status":"Downloading","progressDetail":{"current":1000,"total":1000},"id":"layer1"}
{"status":"Download complete","id":"layer1"}
{"status":"Downloading","progressDetail":{"current":2000,"total":2000},"id":"layer2"}
{"status":"Download complete","id":"layer2"}
{"status":"Pull complete","id":"layer1"}
{"status":"Pull complete","id":"layer2"}
`

	var progressUpdates []DeployProgress
	callback := func(p DeployProgress) {
		progressUpdates = append(progressUpdates, p)
	}

	tracker := newPullProgressTracker(callback)
	err := tracker.Track(strings.NewReader(events))

	assert.NoError(t, err)
	assert.NotEmpty(t, progressUpdates)

	lastUpdate := progressUpdates[len(progressUpdates)-1]
	assert.Equal(t, DeployStageDownloading, lastUpdate.Stage)
	assert.Equal(t, 100, lastUpdate.Percentage)

	assertNeverDecreases(t, progressUpdates)
}

func TestPullProgressTrackerWithCachedLayers(t *testing.T) {
	events := `{"status":"Pulling from basecamp/once-campfire","id":"latest"}
{"status":"Pulling fs layer","id":"layer1"}
{"status":"Already exists","id":"layer1"}
`

	var progressUpdates []DeployProgress
	callback := func(p DeployProgress) {
		progressUpdates = append(progressUpdates, p)
	}

	tracker := newPullProgressTracker(callback)
	err := tracker.Track(strings.NewReader(events))

	assert.NoError(t, err)
	assert.NotEmpty(t, progressUpdates)

	lastUpdate := progressUpdates[len(progressUpdates)-1]
	assert.Equal(t, 100, lastUpdate.Percentage)
}

// Helpers

func assertNeverDecreases(t *testing.T, updates []DeployProgress) {
	t.Helper()
	lastPct := 0
	for i, u := range updates {
		if u.Percentage < lastPct {
			t.Errorf("progress decreased at index %d: %d -> %d", i, lastPct, u.Percentage)
		}
		lastPct = u.Percentage
	}
}
