package ui

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRGB16Bit(t *testing.T) {
	c, ok := parseRGB("rgb:ffff/0000/8080")
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.R, 0.001)
	assert.InDelta(t, 0.0, c.G, 0.001)
	assert.InDelta(t, 0.502, c.B, 0.01)
}

func TestParseRGB8Bit(t *testing.T) {
	c, ok := parseRGB("rgb:ff/00/80")
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.R, 0.001)
	assert.InDelta(t, 0.0, c.G, 0.001)
	assert.InDelta(t, 0.502, c.B, 0.01)
}

func TestParseRGB1Digit(t *testing.T) {
	c, ok := parseRGB("rgb:f/0/8")
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.R, 0.001)
	assert.InDelta(t, 0.0, c.G, 0.001)
	assert.InDelta(t, 0.533, c.B, 0.01)
}

func TestParseRGB3Digit(t *testing.T) {
	c, ok := parseRGB("rgb:fff/000/800")
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.R, 0.001)
	assert.InDelta(t, 0.0, c.G, 0.001)
	assert.InDelta(t, 0.500, c.B, 0.01)
}

func TestParseRGBInvalid(t *testing.T) {
	_, ok := parseRGB("not-rgb")
	assert.False(t, ok)

	_, ok = parseRGB("rgb:ff/gg/00")
	assert.False(t, ok)

	_, ok = parseRGB("rgb:ff/00")
	assert.False(t, ok)
}

func newTestDetector(data string) *detector {
	return &detector{reader: bufio.NewReader(strings.NewReader(data))}
}

func TestReadForegroundColor(t *testing.T) {
	d := newTestDetector("\x1b]10;rgb:c0c0/caca/f5f5\x07")
	da1, err := d.readNext()
	require.NoError(t, err)
	assert.False(t, da1)
	assert.True(t, d.colors.Detected[sampleForeground])
	assert.InDelta(t, 0.753, d.colors.Colors[sampleForeground].R, 0.01)
}

func TestReadBackgroundColor(t *testing.T) {
	d := newTestDetector("\x1b]11;rgb:1a1a/1b1b/2626\x07")
	da1, err := d.readNext()
	require.NoError(t, err)
	assert.False(t, da1)
	assert.True(t, d.colors.Detected[sampleBackground])
	assert.InDelta(t, 0.102, d.colors.Colors[sampleBackground].R, 0.01)
}

func TestReadANSIColor(t *testing.T) {
	d := newTestDetector("\x1b]4;4;rgb:7a7a/a2a2/f7f7\x07")
	da1, err := d.readNext()
	require.NoError(t, err)
	assert.False(t, da1)
	assert.True(t, d.colors.Detected[4]) // blue
	assert.InDelta(t, 0.478, d.colors.Colors[4].R, 0.01)
}

func TestReadColorWithSTTerminator(t *testing.T) {
	d := newTestDetector("\x1b]10;rgb:ffff/ffff/ffff\x1b\\")
	da1, err := d.readNext()
	require.NoError(t, err)
	assert.False(t, da1)
	assert.True(t, d.colors.Detected[sampleForeground])
	assert.InDelta(t, 1.0, d.colors.Colors[sampleForeground].R, 0.001)
}

func TestReadDA1(t *testing.T) {
	d := newTestDetector("\x1b[?62;c")
	da1, err := d.readNext()
	require.NoError(t, err)
	assert.True(t, da1)
}

func TestReadMultipleResponses(t *testing.T) {
	d := newTestDetector(
		"\x1b]10;rgb:c0c0/caca/f5f5\x07" +
			"\x1b]11;rgb:1a1a/1b1b/2626\x07" +
			"\x1b]4;2;rgb:5050/fafa/7b7b\x07",
	)

	for range 3 {
		da1, err := d.readNext()
		require.NoError(t, err)
		assert.False(t, da1)
	}

	assert.True(t, d.colors.Detected[sampleForeground])
	assert.True(t, d.colors.Detected[sampleBackground])
	assert.True(t, d.colors.Detected[2])
}

func TestDetectedColorsDefaultEmpty(t *testing.T) {
	d := detectedColors{}
	for i := range sampleCount {
		assert.False(t, d.Detected[i])
		assert.Equal(t, colorful.Color{}, d.Colors[i])
	}
}

// mockTTY simulates a terminal that responds to OSC queries.
// It discards writes (the query) and feeds back the canned response.
type mockTTY struct {
	io.Reader
	io.Writer
}

func newMockTTY(response []byte) *mockTTY {
	return &mockTTY{
		Reader: bytes.NewReader(response),
		Writer: io.Discard,
	}
}

func (m *mockTTY) Read(p []byte) (int, error)  { return m.Reader.Read(p) }
func (m *mockTTY) Write(p []byte) (int, error) { return m.Writer.Write(p) }

// fullDarkResponse returns a terminal response with all 18 colors
// (Tokyo Night-like dark theme) followed by a DA1 sentinel.
func fullDarkResponse() string {
	return "" +
		"\x1b]10;rgb:c0c0/caca/f5f5\x07" + // foreground
		"\x1b]11;rgb:1a1a/1b1b/2626\x07" + // background
		"\x1b]4;0;rgb:1515/1616/1e1e\x07" + // black
		"\x1b]4;1;rgb:f7f7/7676/8e8e\x07" + // red
		"\x1b]4;2;rgb:9e9e/cece/6a6a\x07" + // green
		"\x1b]4;3;rgb:e0e0/afaf/6868\x07" + // yellow
		"\x1b]4;4;rgb:7a7a/a2a2/f7f7\x07" + // blue
		"\x1b]4;5;rgb:bbbb/9a9a/f7f7\x07" + // magenta
		"\x1b]4;6;rgb:7d7d/cfcf/ffff\x07" + // cyan
		"\x1b]4;7;rgb:a9a9/b1b1/d6d6\x07" + // white
		"\x1b]4;8;rgb:4141/4444/6868\x07" + // bright black
		"\x1b]4;9;rgb:ffff/0000/7c7c\x07" + // bright red
		"\x1b]4;10;rgb:7373/daca/a3a3\x07" + // bright green
		"\x1b]4;11;rgb:ffff/9e9e/6464\x07" + // bright yellow
		"\x1b]4;12;rgb:7d7d/cfcf/ffff\x07" + // bright blue
		"\x1b]4;13;rgb:bbbb/9a9a/f7f7\x07" + // bright magenta
		"\x1b]4;14;rgb:0d0d/b9b9/d7d7\x07" + // bright cyan
		"\x1b]4;15;rgb:c0c0/caca/f5f5\x07" + // bright white
		"\x1b[?62;22c" // DA1 sentinel
}

// fullLightResponse returns a terminal response with all 18 colors
// (light theme) followed by a DA1 sentinel.
func fullLightResponse() string {
	return "" +
		"\x1b]10;rgb:3434/3b3b/5858\x07" + // dark foreground
		"\x1b]11;rgb:d5d5/d6d6/dbdb\x07" + // light background
		"\x1b]4;0;rgb:0f0f/0f0f/1414\x07" + // black
		"\x1b]4;1;rgb:f5f5/2a2a/6565\x07" + // red
		"\x1b]4;2;rgb:5858/7c7c/0c0c\x07" + // green
		"\x1b]4;3;rgb:8c8c/6c6c/3e3e\x07" + // yellow
		"\x1b]4;4;rgb:2e2e/7d7d/e9e9\x07" + // blue
		"\x1b]4;5;rgb:9854/f1f1/4343\x07" + // magenta
		"\x1b]4;6;rgb:0707/8787/8787\x07" + // cyan
		"\x1b]4;7;rgb:6060/6060/7070\x07" + // white
		"\x1b]4;8;rgb:a1a1/a6a6/c5c5\x07" + // bright black
		"\x1b]4;9;rgb:f5f5/2a2a/6565\x07" + // bright red
		"\x1b]4;10;rgb:5858/7c7c/0c0c\x07" + // bright green
		"\x1b]4;11;rgb:8c8c/6c6c/3e3e\x07" + // bright yellow
		"\x1b]4;12;rgb:2e2e/7d7d/e9e9\x07" + // bright blue
		"\x1b]4;13;rgb:9854/f1f1/4343\x07" + // bright magenta
		"\x1b]4;14;rgb:0707/8787/8787\x07" + // bright cyan
		"\x1b]4;15;rgb:c0c0/caca/f5f5\x07" + // bright white
		"\x1b[?62;c" // DA1 sentinel
}

func TestDetectFromDarkTheme(t *testing.T) {
	mock := newMockTTY([]byte(fullDarkResponse()))
	d, ok := detectFrom(mock)

	assert.True(t, ok)
	assert.True(t, d.complete())

	assert.InDelta(t, 0.753, d.Colors[sampleForeground].R, 0.01)
	assert.InDelta(t, 0.102, d.Colors[sampleBackground].R, 0.01)
	assert.InDelta(t, 0.478, d.Colors[4].R, 0.01) // blue
}

func TestDetectFromLightTheme(t *testing.T) {
	mock := newMockTTY([]byte(fullLightResponse()))
	d, ok := detectFrom(mock)

	assert.True(t, ok)
	assert.True(t, d.complete())

	// Light background should have high lightness
	bgL, _, _ := d.Colors[sampleBackground].OkLch()
	assert.Greater(t, bgL, 0.8)

	// Dark foreground should have low lightness
	fgL, _, _ := d.Colors[sampleForeground].OkLch()
	assert.Less(t, fgL, 0.4)
}

func TestDetectFromPartialWithSentinel(t *testing.T) {
	response := "" +
		"\x1b]11;rgb:1a1a/1b1b/2626\x07" +
		"\x1b[?62;c"

	mock := newMockTTY([]byte(response))
	_, ok := detectFrom(mock)

	assert.False(t, ok)
}

func TestDetectFromPartialNoSentinel(t *testing.T) {
	response := "" +
		"\x1b]10;rgb:c0c0/caca/f5f5\x07" +
		"\x1b]11;rgb:1a1a/1b1b/2626\x07" +
		"\x1b]4;4;rgb:7a7a/a2a2/f7f7\x07"

	mock := newMockTTY([]byte(response))
	_, ok := detectFrom(mock)

	assert.False(t, ok)
}

func TestDetectFromSentinelOnly(t *testing.T) {
	mock := newMockTTY([]byte("\x1b[?62;c"))
	_, ok := detectFrom(mock)

	assert.False(t, ok)
}

func TestDetectFromNoOSCSupport(t *testing.T) {
	mock := newMockTTY([]byte{})
	_, ok := detectFrom(mock)

	assert.False(t, ok)
}

func TestDetectFromTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	rw := &mockTTY{Reader: pr, Writer: io.Discard}

	start := time.Now()
	time.AfterFunc(50*time.Millisecond, func() { pr.Close() })
	_, ok := detectFrom(rw)
	elapsed := time.Since(start)

	assert.False(t, ok)
	assert.Less(t, elapsed, 200*time.Millisecond)
}
