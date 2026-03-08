package userstats

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	flushInterval      = time.Minute
	connectedThreshold = 5 * time.Second
	maxRetryDelay      = 30 * time.Second
	initialFlushDelay  = 10 * time.Second
	scannerBufSize     = 64 * 1024
	scannerMaxSize     = 1024 * 1024
)

type dockerClient interface {
	copyClient
	ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

type Scraper struct {
	namespace     string
	containerName string

	mu    sync.RWMutex
	store *BucketStore
	live  map[string]*liveServiceData
}

type liveServiceData struct {
	buckets     [NumBuckets]*hyperloglog.Sketch
	bucketHours [NumBuckets]int64
}

type logEntry struct {
	Time       time.Time `json:"time"`
	RemoteAddr string    `json:"remote_addr"`
	Service    string    `json:"service"`
}

func NewScraper(namespace string) *Scraper {
	return &Scraper{
		namespace:     namespace,
		containerName: namespace + "-proxy",
		store:         &BucketStore{Services: make(map[string]*ServiceData)},
		live:          make(map[string]*liveServiceData),
	}
}

func (s *Scraper) Run(ctx context.Context) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Error("Creating Docker client for user stats", "error", err)
		return
	}

	s.run(ctx, c)
}

// Private

func (s *Scraper) run(ctx context.Context, c dockerClient) {
	s.loadPersistedState(ctx, c)

	retryDelay := time.Second
	for {
		start := time.Now()
		s.streamLogs(ctx, c)
		connected := time.Since(start) >= connectedThreshold

		if connected {
			s.flush(ctx, c)
			retryDelay = time.Second
		} else {
			retryDelay = min(retryDelay*2, maxRetryDelay)
		}

		select {
		case <-ctx.Done():
			s.flush(context.Background(), c)
			return
		case <-time.After(retryDelay):
		}
	}
}

func (s *Scraper) loadPersistedState(ctx context.Context, c copyClient) {
	store, err := Load(ctx, c, s.containerName)
	if err != nil {
		slog.Error("Loading user stats", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.store = store
	s.live = make(map[string]*liveServiceData)

	for name, svc := range store.Services {
		live := &liveServiceData{}
		for i := range NumBuckets {
			if svc.Buckets[i] != nil {
				sketch := deserializeSketch(svc.Buckets[i])
				if sketch != nil {
					live.buckets[i] = sketch
				}
			}
			live.bucketHours[i] = svc.BucketHours[i]
		}
		s.live[name] = live
	}
}

func (s *Scraper) streamLogs(ctx context.Context, c dockerClient) {
	opts := container.LogsOptions{
		ShowStdout: true,
		Follow:     true,
	}

	s.mu.RLock()
	if !s.store.LastTimestamp.IsZero() {
		opts.Since = s.store.LastTimestamp.Format(time.RFC3339Nano)
	}
	s.mu.RUnlock()

	reader, err := c.ContainerLogs(ctx, s.containerName, opts)
	if err != nil {
		return
	}
	defer reader.Close()

	// Flush periodically in a separate goroutine, since the scan loop
	// blocks on reading and may not get a chance to flush for a while.
	// Use a short initial delay so backlog data is flushed quickly on
	// first start, then switch to the normal interval.
	flushCtx, flushCancel := context.WithCancel(ctx)
	defer flushCancel()

	go s.flushLoop(flushCtx, c)

	s.consumeStream(ctx, c, reader)
}

func (s *Scraper) flushLoop(ctx context.Context, c copyClient) {
	// Short initial delay so stats from the log backlog are available quickly
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialFlushDelay):
		s.flush(ctx, c)
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flush(ctx, c)
		}
	}
}

func (s *Scraper) consumeStream(ctx context.Context, c dockerClient, reader io.ReadCloser) {
	info, err := c.ContainerInspect(ctx, s.containerName)
	if err != nil {
		s.scanLines(ctx, reader)
		return
	}

	if info.Config != nil && info.Config.Tty {
		s.scanLines(ctx, reader)
	} else {
		s.demuxAndScan(ctx, reader)
	}
}

func (s *Scraper) demuxAndScan(ctx context.Context, reader io.Reader) {
	stdoutR, stdoutW := io.Pipe()

	go func() {
		_, _ = stdcopy.StdCopy(stdoutW, io.Discard, reader)
		stdoutW.Close()
	}()

	s.scanLines(ctx, stdoutR)
}

func (s *Scraper) scanLines(ctx context.Context, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, scannerBufSize), scannerMaxSize)

	for scanner.Scan() {
		s.processLine(scanner.Bytes())

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *Scraper) processLine(line []byte) {
	var entry logEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}

	if entry.Service == "" || entry.RemoteAddr == "" {
		return
	}

	addr := cleanRemoteAddr(entry.RemoteAddr)
	if isLoopback(addr) {
		return
	}

	unixHour := entry.Time.Unix() / 3600
	if unixHour <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	live, ok := s.live[entry.Service]
	if !ok {
		live = &liveServiceData{}
		s.live[entry.Service] = live
	}

	idx := unixHour % NumBuckets
	if live.bucketHours[idx] != unixHour {
		live.buckets[idx] = hyperloglog.New()
		live.bucketHours[idx] = unixHour
	}
	live.buckets[idx].Insert([]byte(addr))

	if entry.Time.After(s.store.LastTimestamp) {
		s.store.LastTimestamp = entry.Time
	}
}

func (s *Scraper) flush(ctx context.Context, c copyClient) {
	storeBytes, summaryBytes := s.prepareFlush()

	if storeBytes != nil {
		if err := writeToContainer(ctx, c, s.containerName, binaryFileName, storeBytes); err != nil {
			slog.Error("Saving user stats", "error", err)
		}
	}
	if summaryBytes != nil {
		if err := writeToContainer(ctx, c, s.containerName, summaryFileName, summaryBytes); err != nil {
			slog.Error("Saving user stats summary", "error", err)
		}
	}
}

func (s *Scraper) prepareFlush() ([]byte, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syncLiveToStore()

	storeBytes, err := encodeStore(s.store)
	if err != nil {
		slog.Error("Encoding user stats", "error", err)
	}

	summary := computeSummary(s.store)
	summaryBytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		slog.Error("Marshaling user stats summary", "error", err)
	}

	return storeBytes, summaryBytes
}

func (s *Scraper) syncLiveToStore() {
	for name, live := range s.live {
		svc, ok := s.store.Services[name]
		if !ok {
			svc = &ServiceData{}
			s.store.Services[name] = svc
		}

		for i := range NumBuckets {
			svc.BucketHours[i] = live.bucketHours[i]
			if live.buckets[i] != nil {
				data, err := live.buckets[i].MarshalBinary()
				if err == nil {
					svc.Buckets[i] = data
				}
			} else {
				svc.Buckets[i] = nil
			}
		}
	}
}

// Helpers

func isLoopback(addr string) bool {
	return addr == "127.0.0.1" || addr == "::1"
}

func cleanRemoteAddr(addr string) string {
	if i := strings.IndexByte(addr, ','); i >= 0 {
		addr = addr[:i]
	}
	return strings.TrimSpace(addr)
}
