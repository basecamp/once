package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLogsClient struct {
	logs      string
	isTTY     bool
	delivered chan struct{}
}

func (m *mockLogsClient) ContainerLogs(ctx context.Context, containerName string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	return &notifyingReadCloser{
		Reader:    bytes.NewReader([]byte(m.logs)),
		delivered: m.delivered,
	}, nil
}

func (m *mockLogsClient) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{
		Container: container.InspectResponse{
			Config: &container.Config{Tty: m.isTTY},
		},
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
		lines:    NewRingBuffer[LogLine](bufferSize),
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

func TestSanitizeLogLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "GET /up 200", "GET /up 200"},
		{"colour kept", "\x1b[1;32mok\x1b[0m done", "\x1b[1;32mok\x1b[0m done"},
		{"truecolor with colons kept", "\x1b[38:2::10:20:30mx\x1b[m", "\x1b[38:2::10:20:30mx\x1b[m"},
		{"hyperlink reduced to its text", "see \x1b]8;;https://example.com\x1b\\docs\x1b]8;;\x1b\\ here", "see docs here"},
		{"hyperlink with BEL terminator", "\x1b]8;id=1;https://example.com\x07docs\x1b]8;;\x07", "docs"},
		{"unterminated hyperlink", "a\x1b]8;;https://example.com", "a"},
		{"clipboard write removed", "a\x1b]52;c;aGVsbG8=\x07b", "ab"},
		{"window title removed", "\x1b]0;title\x07\x1b]2;title\x1b\\x", "x"},
		{"clear screen and home removed", "\x1b[2J\x1b[Hx", "x"},
		{"cursor moves removed", "a\x1b[5A\x1b[10;20Hb\x1b[3Cc\x1b7\x1b8", "abc"},
		{"private and prefixed SGR-like removed", "\x1b[?1049h\x1b[>4;2m\x1b[?25lx", "x"},
		{"other sequences removed", "\x1bPq#0\x1b\\\x1b_k\x1b\\\x1bc\x1b(Bx", "x"},
		{"incomplete sequences removed", "a\x1b[1;2", "a"},
		{"trailing escape removed", "a\x1b", "a"},
		{"8-bit C1 removed", "a\x9b2Jb\x9d0;t\x9cc\x85d", "abcd"},
		{"UTF-8 C1 removed", "a\u009b2Jb\u009d52;c;x\u009cc\u0085d", "a2Jb52;c;xcd"},
		{"BEL and backspace removed", "a\x07b\x08c", "abc"},
		{"other C0 and DEL removed", "a\x00b\rc\x0bd\x7fe", "abcde"},
		{"tab kept", "a\tb", "a\tb"},
		{"UTF-8 kept", "café 日本語 \U0001F600 \U0001F44D\U0001F3FD é", "café 日本語 \U0001F600 \U0001F44D\U0001F3FD é"},
		{"forged mouse marker removed", "\x1b[0zclick\x1b[0z", "click"},
		{"forged C1 mouse marker removed", "\x9b0za\u009b0z", "a0z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeLogLine(tt.in))
		})
	}
}

func TestSanitizeLogLineLeavesOnlySGR(t *testing.T) {
	var all strings.Builder
	for b := range 256 {
		all.WriteByte(byte(b))
		all.WriteString("\x1b")
		all.WriteByte(byte(b))
		all.WriteString("\x1b[1")
		all.WriteByte(byte(b))
	}

	out := sanitizeLogLine(all.String())
	for i := 0; i < len(out); i++ {
		if out[i] == 0x1b {
			end := strings.IndexByte(out[i:], 'm')
			require.NotEqual(t, -1, end)
			assert.True(t, isSGR(out[i:i+end+1]), "%q", out[i:i+end+1])
			i += end
			continue
		}
		assert.False(t, out[i] < 0x20 && out[i] != '\t' || out[i] == 0x7f, "control byte %#x at %d", out[i], i)
	}
	for _, r := range out {
		assert.False(t, r >= 0x80 && r <= 0x9f, "C1 rune %U", r)
	}
}

func TestLogStreamerSanitizesLines(t *testing.T) {
	client := &mockLogsClient{
		logs:  "\x1b[31mred\x1b[0m\n\x1b]0;title\x07plain\n",
		isTTY: true,
	}

	s := newTestLogStreamer(client, 10)
	s.streamLogs(context.Background(), "test-container")

	lines := s.Fetch(10)
	require.Len(t, lines, 2)
	assert.Equal(t, "\x1b[31mred\x1b[0m", lines[0].Content)
	assert.Equal(t, "plain", lines[1].Content)
}
