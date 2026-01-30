package docker

import (
	"bufio"
	"context"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

const DefaultLogBufferSize = 10000

type LogLine struct {
	Content  string
	IsStderr bool
}

type LogStreamerSettings struct {
	BufferSize int
}

func (s LogStreamerSettings) withDefaults() LogStreamerSettings {
	if s.BufferSize == 0 {
		s.BufferSize = DefaultLogBufferSize
	}
	return s
}

type logsClient interface {
	ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

type LogStreamer struct {
	settings LogStreamerSettings
	client   logsClient

	mu      sync.RWMutex
	lines   []LogLine
	head    int
	count   int
	version uint64
	ready   bool
	cancel  context.CancelFunc
}

func NewLogStreamer(ns *Namespace, settings LogStreamerSettings) *LogStreamer {
	settings = settings.withDefaults()
	return &LogStreamer{
		settings: settings,
		client:   ns.client,
		lines:    make([]LogLine, settings.BufferSize),
	}
}

func NewLogStreamerForTest(settings LogStreamerSettings) *LogStreamer {
	settings = settings.withDefaults()
	return &LogStreamer{
		settings: settings,
		lines:    make([]LogLine, settings.BufferSize),
	}
}

func (s *LogStreamer) Start(ctx context.Context, containerName string) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	go s.runStream(streamCtx, containerName)
}

func (s *LogStreamer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Fetch returns the last n lines in chronological order (oldest first).
func (s *LogStreamer) Fetch(n int) []LogLine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	available := min(n, s.count)
	if available == 0 {
		return nil
	}

	result := make([]LogLine, available)
	startIdx := (s.head - s.count + len(s.lines)) % len(s.lines)
	offset := s.count - available

	for i := range available {
		idx := (startIdx + offset + i) % len(s.lines)
		result[i] = s.lines[idx]
	}

	return result
}

func (s *LogStreamer) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

func (s *LogStreamer) Version() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

func (s *LogStreamer) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Private

func (s *LogStreamer) runStream(ctx context.Context, containerName string) {
	for {
		s.streamLogs(ctx, containerName)

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
			// Retry after brief delay if stream disconnected
		}
	}
}

func (s *LogStreamer) streamLogs(ctx context.Context, containerName string) {
	info, err := s.client.ContainerInspect(ctx, containerName)
	if err != nil {
		return
	}
	isTTY := info.Config != nil && info.Config.Tty

	reader, err := s.client.ContainerLogs(ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "10000",
	})
	if err != nil {
		return
	}
	defer reader.Close()

	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	if isTTY {
		s.scanLines(reader, false)
	} else {
		s.demuxAndScan(reader)
	}
}

func (s *LogStreamer) demuxAndScan(reader io.Reader) {
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	go func() {
		_, _ = stdcopy.StdCopy(stdoutW, stderrW, reader)
		stdoutW.Close()
		stderrW.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.scanLines(stdoutR, false)
	}()

	go func() {
		defer wg.Done()
		s.scanLines(stderrR, true)
	}()

	wg.Wait()
}

func (s *LogStreamer) scanLines(reader io.Reader, isStderr bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		s.addLine(LogLine{
			Content:  scanner.Text(),
			IsStderr: isStderr,
		})
	}
}

func (s *LogStreamer) addLine(line LogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lines[s.head] = line
	s.head = (s.head + 1) % len(s.lines)
	if s.count < len(s.lines) {
		s.count++
	}
	s.version++
}
