package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	prom "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const (
	defaultBufferSize = 200
	httpTimeout       = 5 * time.Second
)

// Sample represents request counts for a single scrape interval
type Sample struct {
	Timestamp    time.Time
	Success      int64
	ClientErrors int64
	ServerErrors int64
}

// ScraperSettings configures the metrics scraper
type ScraperSettings struct {
	Port       int
	BufferSize int
}

func (s ScraperSettings) withDefaults() ScraperSettings {
	if s.BufferSize == 0 {
		s.BufferSize = defaultBufferSize
	}
	return s
}

// MetricsScraper periodically scrapes Prometheus metrics from kamal-proxy
type MetricsScraper struct {
	settings ScraperSettings
	client   *http.Client

	mu        sync.RWMutex
	services  map[string]*serviceData
	lastError error
}

type serviceData struct {
	samples      []Sample
	head         int
	count        int
	prevCounters *counterState
}

type counterState struct {
	success      float64
	clientErrors float64
	serverErrors float64
}

func NewMetricsScraper(settings ScraperSettings) *MetricsScraper {
	settings = settings.withDefaults()
	return &MetricsScraper{
		settings: settings,
		client:   &http.Client{Timeout: httpTimeout},
		services: make(map[string]*serviceData),
	}
}

// Fetch returns the last n samples for a service, ordered from newest to oldest.
// If fewer than n samples exist, only the available samples are returned.
func (s *MetricsScraper) Fetch(service string, n int) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.services[service]
	if !ok {
		return nil
	}

	available := min(n, data.count)
	result := make([]Sample, available)
	for i := range available {
		idx := (data.head - 1 - i + len(data.samples)) % len(data.samples)
		result[i] = data.samples[idx]
	}

	return result
}

func (s *MetricsScraper) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *MetricsScraper) Scrape(ctx context.Context) {
	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", s.settings.Port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		s.setError(fmt.Errorf("creating request: %w", err))
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.setError(fmt.Errorf("fetching metrics: %w", err))
		return
	}
	defer resp.Body.Close()

	counters, err := s.parseMetrics(resp.Body)
	if err != nil {
		s.setError(fmt.Errorf("parsing metrics: %w", err))
		return
	}

	s.setError(nil)
	s.recordSamples(counters)
}

// Private

func (s *MetricsScraper) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
}

func (s *MetricsScraper) parseMetrics(body io.Reader) (map[string]*counterState, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(body)
	if err != nil {
		return nil, err
	}

	counters := make(map[string]*counterState)

	family, ok := families["kamal_proxy_http_requests_total"]
	if !ok {
		return counters, nil
	}

	for _, metric := range family.GetMetric() {
		service := getLabel(metric, "service")
		if service == "" {
			continue
		}

		state, ok := counters[service]
		if !ok {
			state = &counterState{}
			counters[service] = state
		}

		statusCode := getStatusCode(metric)
		count := metric.GetCounter().GetValue()

		switch {
		case statusCode >= 100 && statusCode < 400:
			state.success += count
		case statusCode >= 400 && statusCode < 500:
			state.clientErrors += count
		case statusCode >= 500:
			state.serverErrors += count
		}
	}

	return counters, nil
}

func (s *MetricsScraper) recordSamples(counters map[string]*counterState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for service, current := range counters {
		data, ok := s.services[service]
		if !ok {
			data = &serviceData{
				samples: make([]Sample, s.settings.BufferSize),
			}
			s.services[service] = data
		}

		sample := Sample{Timestamp: now}
		if data.prevCounters != nil {
			sample.Success = safeDelta(current.success, data.prevCounters.success)
			sample.ClientErrors = safeDelta(current.clientErrors, data.prevCounters.clientErrors)
			sample.ServerErrors = safeDelta(current.serverErrors, data.prevCounters.serverErrors)
		}
		data.prevCounters = current

		data.samples[data.head] = sample
		data.head = (data.head + 1) % len(data.samples)
		if data.count < len(data.samples) {
			data.count++
		}
	}
}

// Helpers

func getLabel(metric *prom.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

func getStatusCode(metric *prom.Metric) int {
	status := getLabel(metric, "status")
	var code int
	fmt.Sscanf(status, "%d", &code)
	return code
}

func safeDelta(current, prev float64) int64 {
	if current < prev {
		return int64(current)
	}
	return int64(current - prev)
}
