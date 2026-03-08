package system

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Sample struct {
	CPUPercent float64
	NumCPUs    int
	MemTotal   uint64
	MemUsed    uint64
	DiskTotal  uint64
	DiskFree   uint64
	DiskUsed   uint64
	DiskErr    error
}

const defaultBufferSize = 200

type ScraperSettings struct {
	BufferSize   int
	DiskPath     string
	DiskFallback string
}

func (s ScraperSettings) withDefaults() ScraperSettings {
	if s.BufferSize == 0 {
		s.BufferSize = defaultBufferSize
	}
	if s.DiskFallback == "" {
		s.DiskFallback = "/"
	}
	return s
}

type Scraper struct {
	settings    ScraperSettings
	mu          sync.RWMutex
	samples     []Sample
	head, count int
	numCPUs     int
	memTotal    uint64
}

func NewScraper(settings ScraperSettings) *Scraper {
	settings = settings.withDefaults()

	numCPUs, err := cpu.Counts(true)
	if err != nil {
		slog.Warn("failed to get CPU count, defaulting to 1", "err", err)
		numCPUs = 1
	}

	return &Scraper{
		settings: settings,
		samples:  make([]Sample, settings.BufferSize),
		numCPUs:  numCPUs,
	}
}

func (s *Scraper) NumCPUs() int {
	return s.numCPUs
}

func (s *Scraper) MemTotal() uint64 {
	return s.memTotal
}

func (s *Scraper) Fetch(n int) []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	available := min(n, s.count)
	result := make([]Sample, available)
	for i := range available {
		idx := (s.head - 1 - i + len(s.samples)) % len(s.samples)
		result[i] = s.samples[idx]
	}
	return result
}

func (s *Scraper) Scrape(ctx context.Context) {
	sample := Sample{
		NumCPUs: s.numCPUs,
	}

	percents, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(percents) > 0 {
		sample.CPUPercent = percents[0] * float64(s.numCPUs)
	}

	if vmStat, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		sample.MemTotal = vmStat.Total
		sample.MemUsed = vmStat.Used
		if s.memTotal == 0 {
			s.memTotal = vmStat.Total
		}
	}

	diskPath := s.diskPath()
	if diskPath != "" {
		total, used, free, err := statDisk(diskPath)
		sample.DiskTotal = total
		sample.DiskUsed = used
		sample.DiskFree = free
		sample.DiskErr = err
	}

	s.mu.Lock()
	s.samples[s.head] = sample
	s.head = (s.head + 1) % len(s.samples)
	if s.count < len(s.samples) {
		s.count++
	}
	s.mu.Unlock()
}

// Private

func (s *Scraper) diskPath() string {
	if s.settings.DiskPath != "" {
		if _, err := os.Stat(s.settings.DiskPath); err == nil {
			return s.settings.DiskPath
		}
	}
	return s.settings.DiskFallback
}
