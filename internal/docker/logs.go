package docker

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

const (
	DefaultLogBufferSize = 10000
	defaultTailLines     = "10000"
	scannerBufSize       = 64 * 1024
	scannerMaxSize       = 1024 * 1024
)

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
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

type LogStreamer struct {
	settings LogStreamerSettings
	client   logsClient

	mu      sync.RWMutex
	lines   *RingBuffer[LogLine]
	version uint64
	ready   bool
	cancel  context.CancelFunc
}

func NewLogStreamer(ns *Namespace, settings LogStreamerSettings) *LogStreamer {
	settings = settings.withDefaults()
	var client logsClient
	if ns != nil {
		client = ns.client
	}
	return &LogStreamer{
		settings: settings,
		client:   client,
		lines:    NewRingBuffer[LogLine](settings.BufferSize),
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
	return s.lines.FetchOldestFirst(n)
}

func (s *LogStreamer) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lines.Len()
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
		case <-time.After(streamRetryDelay):
			// Retry after brief delay if stream disconnected
		}
	}
}

func (s *LogStreamer) streamLogs(ctx context.Context, containerName string) {
	info, err := s.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err != nil {
		return
	}
	isTTY := info.Container.Config != nil && info.Container.Config.Tty

	reader, err := s.client.ContainerLogs(ctx, containerName, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       defaultTailLines,
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
	scanner.Buffer(make([]byte, 0, scannerBufSize), scannerMaxSize)

	for scanner.Scan() {
		s.addLine(LogLine{
			Content:  sanitizeLogLine(scanner.Text()),
			IsStderr: isStderr,
		})
	}
}

func (s *LogStreamer) addLine(line LogLine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lines.Add(line)
	s.version++
}

// Helpers

// sanitizeLogLine stops a container's output from controlling the operator's
// terminal. Only SGR colour sequences survive. Every other escape sequence is
// dropped (an OSC 8 hyperlink keeps its visible text), as are DEL, C0 controls
// other than tab, and C1 controls in both their 8-bit and UTF-8 forms.
func sanitizeLogLine(line string) string {
	var out strings.Builder
	out.Grow(len(line))

	var state byte
	for len(line) > 0 {
		seq, _, n, newState := ansi.DecodeSequence(line, state, nil)
		switch {
		case isSGR(seq):
			out.WriteString(seq)
		case !isControlIntroducer(seq[0]):
			writePrintable(&out, seq)
		}
		state = newState
		line = line[n:]
	}

	return out.String()
}

func isSGR(seq string) bool {
	if len(seq) < 3 || !strings.HasPrefix(seq, "\x1b[") || seq[len(seq)-1] != 'm' {
		return false
	}
	for _, c := range []byte(seq[2 : len(seq)-1]) {
		if (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

func isControlIntroducer(c byte) bool {
	return c != '\t' && (c < 0x20 || c == 0x7f || (c >= 0x80 && c <= 0x9f))
}

func writePrintable(out *strings.Builder, text string) {
	for _, r := range text {
		if r == '\t' || (r >= 0x20 && (r < 0x7f || r > 0x9f)) {
			out.WriteRune(r)
		}
	}
}
