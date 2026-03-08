package docker

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLogsClient struct {
	logs      string
	isTTY     bool
	delivered chan struct{}
}

func (m *mockLogsClient) ContainerLogs(ctx context.Context, containerName string, options container.LogsOptions) (io.ReadCloser, error) {
	return &notifyingReadCloser{
		Reader:    bytes.NewReader([]byte(m.logs)),
		delivered: m.delivered,
	}, nil
}

func (m *mockLogsClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return container.InspectResponse{
		Config: &container.Config{Tty: m.isTTY},
	}, nil
}

type notifyingReadCloser struct {
	*bytes.Reader
	delivered chan struct{}
	notified  bool
}

func (r *notifyingReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if !r.notified && r.delivered != nil && n > 0 {
		r.notified = true
		select {
		case r.delivered <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (r *notifyingReadCloser) Close() error {
	return nil
}

func newTestLogStreamer(client logsClient, bufferSize int) *LogStreamer {
	return &LogStreamer{
		settings: LogStreamerSettings{BufferSize: bufferSize}.withDefaults(),
		client:   client,
		lines:    make([]LogLine, bufferSize),
	}
}

func TestLogStreamerFetch(t *testing.T) {
	s := newTestLogStreamer(nil, 10)

	s.addLine(LogLine{Content: "line 1"})
	s.addLine(LogLine{Content: "line 2"})
	s.addLine(LogLine{Content: "line 3"})

	lines := s.Fetch(10)
	assert.Len(t, lines, 3)
	assert.Equal(t, "line 1", lines[0].Content, "oldest line should be first")
	assert.Equal(t, "line 3", lines[2].Content, "newest line should be last")
}

func TestLogStreamerFetchWithLimit(t *testing.T) {
	s := newTestLogStreamer(nil, 10)

	for i := range 5 {
		s.addLine(LogLine{Content: string(rune('a' + i))})
	}

	lines := s.Fetch(2)
	assert.Len(t, lines, 2)
	assert.Equal(t, "d", lines[0].Content)
	assert.Equal(t, "e", lines[1].Content)
}

func TestLogStreamerFetchEmpty(t *testing.T) {
	s := newTestLogStreamer(nil, 10)
	assert.Nil(t, s.Fetch(10))
}

func TestLogStreamerRingBufferWrap(t *testing.T) {
	s := newTestLogStreamer(nil, 3)

	for i := range 5 {
		s.addLine(LogLine{Content: string(rune('a' + i))})
	}

	lines := s.Fetch(10)
	require.Len(t, lines, 3)
	assert.Equal(t, "c", lines[0].Content)
	assert.Equal(t, "d", lines[1].Content)
	assert.Equal(t, "e", lines[2].Content)
}

func TestLogStreamerCount(t *testing.T) {
	s := newTestLogStreamer(nil, 5)

	assert.Equal(t, 0, s.Count())

	s.addLine(LogLine{Content: "line 1"})
	s.addLine(LogLine{Content: "line 2"})
	assert.Equal(t, 2, s.Count())

	for range 10 {
		s.addLine(LogLine{Content: "more"})
	}
	assert.Equal(t, 5, s.Count(), "count should not exceed buffer size")
}

func TestLogStreamerVersion(t *testing.T) {
	s := newTestLogStreamer(nil, 5)

	assert.Equal(t, uint64(0), s.Version())

	s.addLine(LogLine{Content: "line 1"})
	assert.Equal(t, uint64(1), s.Version())

	s.addLine(LogLine{Content: "line 2"})
	s.addLine(LogLine{Content: "line 3"})
	assert.Equal(t, uint64(3), s.Version())

	// Version continues incrementing even after buffer wraps
	for range 10 {
		s.addLine(LogLine{Content: "more"})
	}
	assert.Equal(t, uint64(13), s.Version())
}

func TestLogStreamerStderrFlag(t *testing.T) {
	s := newTestLogStreamer(nil, 10)

	s.addLine(LogLine{Content: "stdout line", IsStderr: false})
	s.addLine(LogLine{Content: "stderr line", IsStderr: true})
	s.addLine(LogLine{Content: "another stdout", IsStderr: false})

	lines := s.Fetch(10)
	require.Len(t, lines, 3)
	assert.False(t, lines[0].IsStderr)
	assert.True(t, lines[1].IsStderr)
	assert.False(t, lines[2].IsStderr)
}

func TestLogStreamerSettingsDefaults(t *testing.T) {
	settings := LogStreamerSettings{}
	settings = settings.withDefaults()
	assert.Equal(t, DefaultLogBufferSize, settings.BufferSize)

	custom := LogStreamerSettings{BufferSize: 500}
	custom = custom.withDefaults()
	assert.Equal(t, 500, custom.BufferSize)
}

func TestLogStreamerScanLinesTTY(t *testing.T) {
	client := &mockLogsClient{
		logs:  "line one\nline two\nline three\n",
		isTTY: true,
	}

	s := newTestLogStreamer(client, 10)
	s.streamLogs(context.Background(), "test-container")

	lines := s.Fetch(10)
	require.Len(t, lines, 3)
	assert.Equal(t, "line one", lines[0].Content)
	assert.Equal(t, "line two", lines[1].Content)
	assert.Equal(t, "line three", lines[2].Content)
}
