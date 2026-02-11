package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStatsClient struct {
	containers []container.Summary
	stats      map[string]container.StatsResponse
	delivered  chan struct{}
}

func (m *mockStatsClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	return m.containers, nil
}

func (m *mockStatsClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	stats := m.stats[containerID]
	data, _ := json.Marshal(stats)
	return container.StatsResponseReader{Body: &notifyingReader{
		Reader:   bytes.NewReader(data),
		delivered: m.delivered,
	}}, nil
}

type notifyingReader struct {
	*bytes.Reader
	delivered chan struct{}
	notified  bool
}

func (r *notifyingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if !r.notified && r.delivered != nil {
		r.notified = true
		select {
		case r.delivered <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (r *notifyingReader) Close() error {
	return nil
}

func newTestScraper(client statsClient) *Scraper {
	return &Scraper{
		settings: ScraperSettings{BufferSize: 10}.withDefaults(),
		client:   client,
		prefix:   "test-app-",
		apps:     make(map[string]*appData),
		streams:  make(map[string]*streamInfo),
	}
}

func TestScraperFetch(t *testing.T) {
	s := newTestScraper(nil)

	for i := range 3 {
		s.recordSample("myapp", Sample{
			Timestamp:   time.Now(),
			CPUPercent:  float64(i + 1),
			MemoryBytes: uint64((i + 1) * 1000),
		})
	}

	samples := s.Fetch("myapp", 10)
	assert.Len(t, samples, 3)
	assert.Equal(t, 3.0, samples[0].CPUPercent, "newest sample should be first")
	assert.Equal(t, 1.0, samples[2].CPUPercent, "oldest sample should be last")
}

func TestScraperFetchWithLimit(t *testing.T) {
	s := newTestScraper(nil)

	for i := range 5 {
		s.recordSample("myapp", Sample{CPUPercent: float64(i + 1)})
	}

	samples := s.Fetch("myapp", 2)
	assert.Len(t, samples, 2)
	assert.Equal(t, 5.0, samples[0].CPUPercent)
	assert.Equal(t, 4.0, samples[1].CPUPercent)
}

func TestScraperFetchUnknownApp(t *testing.T) {
	s := newTestScraper(nil)
	assert.Nil(t, s.Fetch("unknown", 10))
}

func TestScraperRingBufferWrap(t *testing.T) {
	s := &Scraper{
		settings: ScraperSettings{BufferSize: 3}.withDefaults(),
		apps:     make(map[string]*appData),
	}

	for i := range 5 {
		s.recordSample("myapp", Sample{CPUPercent: float64(i + 1)})
	}

	samples := s.Fetch("myapp", 10)
	assert.Len(t, samples, 3)
	assert.Equal(t, 5.0, samples[0].CPUPercent)
	assert.Equal(t, 4.0, samples[1].CPUPercent)
	assert.Equal(t, 3.0, samples[2].CPUPercent)
}

func TestScraperScrapeFindsContainers(t *testing.T) {
	delivered := make(chan struct{}, 2)
	client := &mockStatsClient{
		containers: []container.Summary{
			{ID: "abc123", Names: []string{"/test-app-myapp-xyz"}, State: "running"},
			{ID: "def456", Names: []string{"/test-app-other-123"}, State: "running"},
			{ID: "ghi789", Names: []string{"/unrelated-container"}, State: "running"},
		},
		stats: map[string]container.StatsResponse{
			"abc123": makeStats(50.0, 1000),
			"def456": makeStats(25.0, 2000),
		},
		delivered: delivered,
	}

	s := newTestScraper(client)
	s.Scrape(context.Background())

	// Wait for both streams to receive stats
	<-delivered
	<-delivered
	scrapeUntil(t, s, "myapp", 50.0)

	samples := s.Fetch("myapp", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 50.0, samples[0].CPUPercent)
	assert.Equal(t, uint64(1000), samples[0].MemoryBytes)

	samples = s.Fetch("other", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 25.0, samples[0].CPUPercent)
}

func TestScraperIgnoresStoppedContainers(t *testing.T) {
	client := &mockStatsClient{
		containers: []container.Summary{
			{ID: "abc123", Names: []string{"/test-app-myapp-xyz"}, State: "exited"},
		},
		stats: map[string]container.StatsResponse{},
	}

	s := newTestScraper(client)
	s.Scrape(context.Background())

	assert.Nil(t, s.Fetch("myapp", 1))
}

func TestScraperRecordsZerosForStoppedApp(t *testing.T) {
	delivered := make(chan struct{}, 1)
	client := &mockStatsClient{
		containers: []container.Summary{
			{ID: "abc123", Names: []string{"/test-app-myapp-xyz"}, State: "running"},
		},
		stats: map[string]container.StatsResponse{
			"abc123": makeStats(50.0, 1000),
		},
		delivered: delivered,
	}

	s := newTestScraper(client)
	s.Scrape(context.Background())
	<-delivered
	scrapeUntil(t, s, "myapp", 50.0)

	samples := s.Fetch("myapp", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 50.0, samples[0].CPUPercent)

	// Stop the container
	client.containers = []container.Summary{
		{ID: "abc123", Names: []string{"/test-app-myapp-xyz"}, State: "exited"},
	}
	s.Scrape(context.Background())

	samples = s.Fetch("myapp", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 0.0, samples[0].CPUPercent)
	assert.Equal(t, uint64(0), samples[0].MemoryBytes)
}

func TestScraperReconnectsOnContainerChange(t *testing.T) {
	delivered := make(chan struct{}, 1)
	client := &mockStatsClient{
		containers: []container.Summary{
			{ID: "abc123", Names: []string{"/test-app-myapp-xyz"}, State: "running"},
		},
		stats: map[string]container.StatsResponse{
			"abc123": makeStats(50.0, 1000),
		},
		delivered: delivered,
	}

	s := newTestScraper(client)
	s.Scrape(context.Background())
	<-delivered
	scrapeUntil(t, s, "myapp", 50.0)

	samples := s.Fetch("myapp", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 50.0, samples[0].CPUPercent)

	// Simulate container redeployment with new ID and different stats
	client.containers = []container.Summary{
		{ID: "newcontainer456", Names: []string{"/test-app-myapp-abc"}, State: "running"},
	}
	client.stats = map[string]container.StatsResponse{
		"newcontainer456": makeStats(75.0, 2000),
	}

	s.Scrape(context.Background())
	<-delivered
	scrapeUntil(t, s, "myapp", 75.0)

	samples = s.Fetch("myapp", 1)
	require.Len(t, samples, 1)
	assert.Equal(t, 75.0, samples[0].CPUPercent)
	assert.Equal(t, uint64(2000), samples[0].MemoryBytes)
}

func TestCalculateCPUPercent(t *testing.T) {
	stats := &container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 200000000},
			SystemUsage: 1000000000,
			OnlineCPUs:  4,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100000000},
			SystemUsage: 500000000,
		},
	}

	// CPU delta = 100000000, System delta = 500000000
	// Percent = (100000000 / 500000000) * 4 * 100 = 80%
	assert.Equal(t, 80.0, calculateCPUPercent(stats))
}

func TestCalculateCPUPercentZeroDelta(t *testing.T) {
	stats := &container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100000000},
			SystemUsage: 500000000,
			OnlineCPUs:  4,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100000000},
			SystemUsage: 500000000,
		},
	}

	assert.Equal(t, 0.0, calculateCPUPercent(stats))
}

// Helpers

// scrapeUntil polls Scrape + Fetch until the expected CPU value appears.
// The delivered channel signals that the stream goroutine has started reading,
// but data.latest may not be updated yet, so we need to poll.
func scrapeUntil(t *testing.T, s *Scraper, appName string, expectedCPU float64) {
	t.Helper()
	require.Eventually(t, func() bool {
		s.Scrape(context.Background())
		samples := s.Fetch(appName, 1)
		return len(samples) > 0 && samples[0].CPUPercent == expectedCPU
	}, time.Second, 10*time.Millisecond)
}

func makeStats(cpuPercent float64, memoryBytes uint64) container.StatsResponse {
	// Reverse-engineer CPU values to get desired percentage with 4 CPUs
	// percent = (cpuDelta / systemDelta) * numCPUs * 100
	// cpuDelta = percent * systemDelta / (numCPUs * 100)
	systemDelta := uint64(500000000)
	cpuDelta := uint64(cpuPercent * float64(systemDelta) / 400)

	return container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100000000 + cpuDelta},
			SystemUsage: 500000000 + systemDelta,
			OnlineCPUs:  4,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 100000000},
			SystemUsage: 500000000,
		},
		MemoryStats: container.MemoryStats{
			Usage: memoryBytes,
		},
	}
}
