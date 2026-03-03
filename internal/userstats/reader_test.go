package userstats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderFetch(t *testing.T) {
	r := &Reader{}
	r.summary = &Summary{
		UpdatedAt: time.Now(),
		Services: map[string]ServiceSummary{
			"campfire": {UniqueUsers24h: 42, UniqueUsers7d: 231},
		},
	}

	stats := r.Fetch("campfire")
	require.NotNil(t, stats)
	assert.Equal(t, uint64(42), stats.UniqueUsers24h)
	assert.Equal(t, uint64(231), stats.UniqueUsers7d)
}

func TestReaderFetchUnknownService(t *testing.T) {
	r := &Reader{}
	r.summary = &Summary{
		UpdatedAt: time.Now(),
		Services:  map[string]ServiceSummary{},
	}

	assert.Nil(t, r.Fetch("unknown"))
}

func TestReaderFetchNoSummary(t *testing.T) {
	r := &Reader{}
	assert.Nil(t, r.Fetch("campfire"))
}
