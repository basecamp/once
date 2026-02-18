package renderer

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a test screen
func newTestScreen(width, height int) (*Screen, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	s := NewScreen(buf, width, height)
	return s, buf
}

func TestScreen_InitialRender(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("Hello\nWorld")

	output := buf.String()

	assert.Contains(t, output, CursorHome)
	assert.Contains(t, output, EraseScreen)
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "World")
}

func TestScreen_NoChangeNoDiff(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	content := "Static"
	s.Render(content)

	buf.Reset()

	s.Render(content)

	output := buf.String()
	assert.Equal(t, "", output)
}

func TestScreen_SingleCharChange(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("AAAA\nBBBB\nCCCC")
	buf.Reset()

	s.Render("AAXA\nBBBB\nCCCC")

	output := buf.String()

	assert.Contains(t, output, "X")
	assert.Contains(t, output, "\x1b[1;3H")
	assert.NotContains(t, output, EraseScreen)
}

func TestScreen_ScrollUp(t *testing.T) {
	s, buf := newTestScreen(10, 5)

	s.Render("Line1\nLine2\nLine3\nLine4\nLine5")
	buf.Reset()

	s.Render("Line2\nLine3\nLine4\nLine5\nLine6")

	output := buf.String()

	assert.Contains(t, output, "Line6")
	assert.NotContains(t, output, "Line2")
	assert.NotContains(t, output, "Line3")
	assert.NotContains(t, output, "Line4")
	assert.NotContains(t, output, "Line5")
}

func TestScreen_ScrollDown(t *testing.T) {
	s, buf := newTestScreen(10, 5)

	s.Render("Line1\nLine2\nLine3\nLine4\nLine5")
	buf.Reset()

	s.Render("Line0\nLine1\nLine2\nLine3\nLine4")

	output := buf.String()
	assert.Contains(t, output, "Line0")
	assert.NotContains(t, output, "Line1")
	assert.NotContains(t, output, "Line2")
	assert.NotContains(t, output, "Line3")
	assert.NotContains(t, output, "Line4")
}

func TestScreen_MiddleLineChange(t *testing.T) {
	s, buf := newTestScreen(20, 5)

	s.Render("Line1\nLine2\nLine3\nLine4\nLine5")
	buf.Reset()

	s.Render("Line1\nLine2\nMODIFIED\nLine4\nLine5")

	output := buf.String()

	assert.Contains(t, output, "MODIFIED")
	assert.NotContains(t, output, "Line1")
	assert.NotContains(t, output, "Line5")
}

func TestScreen_StylePreservation(t *testing.T) {
	s, buf := newTestScreen(20, 2)

	s.Render("\x1b[31mRed\x1b[0m Normal")
	buf.Reset()

	s.Render("\x1b[31mRed\x1b[0m Changed")

	output := buf.String()
	assert.Contains(t, output, "Changed")
}

func TestScreen_ForceRedraw(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("Content")
	buf.Reset()

	s.ForceRedraw()
	s.Render("Content")

	output := buf.String()
	assert.Contains(t, output, CursorHome)
}

func TestScreen_Clear(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("Content")
	buf.Reset()

	s.Clear()

	output := buf.String()
	assert.Contains(t, output, EraseScreen)
}

func TestScreen_Size(t *testing.T) {
	s, _ := newTestScreen(80, 24)

	w, h := s.Size()
	assert.Equal(t, 80, w)
	assert.Equal(t, 24, h)
}

func TestScreen_Resize(t *testing.T) {
	s, buf := newTestScreen(10, 5)

	s.Render("Hello")
	buf.Reset()

	s.Resize(20, 10)

	w, h := s.Size()
	assert.Equal(t, 20, w)
	assert.Equal(t, 10, h)

	s.Render("Hello")
	output := buf.String()

	assert.Contains(t, output, CursorHome)
}

func TestScreen_MultipleRenders(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("Frame1")

	buf.Reset()
	s.Render("Frame2")

	output1 := buf.String()
	assert.Contains(t, output1, "2")

	buf.Reset()
	s.Render("Frame2")

	output2 := buf.String()
	assert.Equal(t, "", output2)

	buf.Reset()
	s.Render("Frame1")

	output3 := buf.String()
	assert.Contains(t, output3, "1")
}

func TestScreen_WideCharacters(t *testing.T) {
	s, buf := newTestScreen(10, 1)

	s.Render("A中B")

	output := buf.String()

	assert.Contains(t, output, "A")
	assert.Contains(t, output, "中")
	assert.Contains(t, output, "B")
}

func TestScreen_ANSI256Color(t *testing.T) {
	s, buf := newTestScreen(10, 1)

	s.Render("\x1b[38;5;196mRed256")

	output := buf.String()
	assert.Contains(t, output, "Red256")
}

func TestScreen_RGBColor(t *testing.T) {
	s, buf := newTestScreen(10, 1)

	s.Render("\x1b[38;2;255;0;128mPink")

	output := buf.String()
	assert.Contains(t, output, "Pink")
}

func TestScreen_EmptyRender(t *testing.T) {
	s, buf := newTestScreen(10, 3)

	s.Render("")
	output := buf.String()

	assert.Contains(t, output, CursorHome)
}

func TestScreen_LinesLongerThanWidth(t *testing.T) {
	s, buf := newTestScreen(5, 2)

	s.Render("1234567890")

	output := buf.String()

	assert.Contains(t, output, "12345")

	w, _ := s.Size()
	require.Len(t, s.current.Cells[0], w)
}

func TestScreen_MoreLinesThanHeight(t *testing.T) {
	s, _ := newTestScreen(10, 2)

	s.Render("Line1\nLine2\nLine3\nLine4")

	w, h := s.Size()
	assert.Equal(t, 10, w)
	assert.Equal(t, 2, h)
}

func TestScreen_TrailingSpaces(t *testing.T) {
	s, buf := newTestScreen(10, 1)

	s.Render("ABC")
	buf.Reset()

	s.Render("X")

	output := buf.String()
	assert.Contains(t, output, "X")
}

// Rendering efficiency tests
//
// These tests verify that the renderer uses scroll sequences and
// cell-level diffing to minimize terminal output.

func TestScreen_AppendLineUsesScroll(t *testing.T) {
	width, height := 80, 24

	lines := make([]string, height)
	for i := range height {
		lines[i] = fmt.Sprintf("Log entry %04d", i)
	}

	s, buf := newTestScreen(width, height)
	s.Render(strings.Join(lines, "\n"))
	buf.Reset()

	// Append a new line, shifting everything up by one (like tailing a log)
	newLines := append(lines[1:], "Log entry 0024")
	n := s.Render(strings.Join(newLines, "\n"))

	output := buf.String()

	// Should use a scroll sequence rather than rewriting all 24 lines
	assert.Contains(t, output, "\x1b[1S", "expected scroll-up sequence")
	assert.Contains(t, output, "0024", "new line content should be written")

	// The output should be far smaller than a full redraw
	fullRedrawSize := width * height
	assert.Less(t, n, fullRedrawSize/4,
		"scroll + write new line should be much smaller than full redraw (%d bytes)", n)
}

func TestScreen_ScrollWithFixedHeader(t *testing.T) {
	width, height := 80, 20

	header := "=== Application Logs ==="
	lines := make([]string, height)
	lines[0] = header
	for i := 1; i < height; i++ {
		lines[i] = fmt.Sprintf("Line %d", i)
	}

	s, buf := newTestScreen(width, height)
	s.Render(strings.Join(lines, "\n"))
	buf.Reset()

	// Scroll the body, keep the header, change it slightly
	newLines := make([]string, height)
	newLines[0] = "=== Application Logs ==="
	for i := 1; i < height; i++ {
		newLines[i] = fmt.Sprintf("Line %d", i+1)
	}
	n := s.Render(strings.Join(newLines, "\n"))

	output := buf.String()

	// Should use a scroll region that preserves the header
	assert.Contains(t, output, "\x1b[", "expected ANSI sequences")
	assert.NotContains(t, output, "Application Logs", "header should not be rewritten")

	fullRedrawSize := width * height
	assert.Less(t, n, fullRedrawSize/4,
		"partial scroll should be much smaller than full redraw (%d bytes)", n)
}

func TestScreen_SingleLineChangeIsSmall(t *testing.T) {
	width, height := 80, 24

	lines := make([]string, height)
	for i := range height {
		lines[i] = fmt.Sprintf("Status: line %d is OK", i)
	}

	s, buf := newTestScreen(width, height)
	s.Render(strings.Join(lines, "\n"))
	buf.Reset()

	// Change one word on one line
	lines[12] = "Status: line 12 is ERROR"
	n := s.Render(strings.Join(lines, "\n"))

	output := buf.String()

	// Should only update the changed characters, not rewrite the line
	assert.Contains(t, output, "ERROR")
	assert.NotContains(t, output, "line 0", "unchanged lines should not appear")
	assert.NotContains(t, output, "line 1 ", "unchanged lines should not appear")

	// A single word change should be tiny
	assert.Less(t, n, 50, "single word change should need very few bytes (%d)", n)
}

func TestScreen_IdenticalContentProducesNoOutput(t *testing.T) {
	width, height := 80, 24

	lines := make([]string, height)
	for i := range height {
		lines[i] = fmt.Sprintf("Static content line %d with some text", i)
	}
	content := strings.Join(lines, "\n")

	s, buf := newTestScreen(width, height)
	s.Render(content)
	buf.Reset()

	n := s.Render(content)

	assert.Equal(t, 0, n, "identical content should produce zero bytes of output")
	assert.Equal(t, "", buf.String())
}

func TestScreen_InsertLineInMiddleUsesScroll(t *testing.T) {
	width, height := 80, 20

	lines := make([]string, height)
	for i := range height {
		lines[i] = fmt.Sprintf("Original line %d", i)
	}

	s, buf := newTestScreen(width, height)
	s.Render(strings.Join(lines, "\n"))
	buf.Reset()

	// Insert a new line at position 5, pushing everything below it down.
	// The last line falls off the screen.
	newLines := make([]string, height)
	copy(newLines[:5], lines[:5])
	newLines[5] = ">>> INSERTED LINE <<<"
	copy(newLines[6:], lines[5:height-1])

	n := s.Render(strings.Join(newLines, "\n"))

	output := buf.String()

	// Should use a scroll region for the lower portion
	assert.Contains(t, output, "INSERTED", "inserted line content should be written")

	// The top 5 unchanged lines should not be rewritten
	assert.NotContains(t, output, "Original line 0")
	assert.NotContains(t, output, "Original line 1")

	fullRedrawSize := width * height
	assert.Less(t, n, fullRedrawSize/2,
		"inserting a line should be cheaper than full redraw (%d bytes)", n)
}
