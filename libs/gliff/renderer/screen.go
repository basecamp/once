package renderer

import (
	"io"
	"sync"
)

// Screen is the main interface for terminal rendering.
// It provides ncurses-like efficient screen updates using hash-based
// scroll detection and cell-based diffing.
type Screen struct {
	// Buffers for diff-based rendering
	current *Buffer // What's currently on screen
	pending *Buffer // What we want on screen
	mu      sync.Mutex

	// Current style state (for efficient style transitions)
	currentStyle Style

	// Whether we need a full redraw (e.g., after resize)
	needsFullRedraw bool

	// Output writer
	output io.Writer

	// Whether we've rendered at least once
	initialized bool
}

// NewScreen creates a new Screen with the given output writer and dimensions.
// The output writer receives all terminal escape sequences and content.
// Typically this would be a Terminal instance or a buffer for testing.
func NewScreen(output io.Writer, width, height int) *Screen {
	return &Screen{
		currentStyle:    DefaultStyle(),
		output:          output,
		current:         NewBuffer(width, height),
		pending:         NewBuffer(width, height),
		needsFullRedraw: true,
		initialized:     false,
	}
}

// Size returns the current screen dimensions.
func (s *Screen) Size() (width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.Width, s.current.Height
}

// Resize updates the screen dimensions.
// This should be called when the terminal is resized.
func (s *Screen) Resize(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current.Resize(width, height)
	s.pending.Resize(width, height)
	s.needsFullRedraw = true
}

// Render updates the screen with new content.
// The content string may contain multiple lines separated by newlines,
// and may include ANSI escape sequences for styling.
//
// Render uses efficient diff-based updates:
// - Hash-based scroll detection for bulk line movement
// - Cell-based diffing for minimal character updates
// - Optimized cursor movement and attribute changes
//
// Returns the number of bytes written to the terminal.
func (s *Screen) Render(content string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse content into pending buffer
	s.pending.SetContent(content)

	var output string
	if s.needsFullRedraw || !s.initialized {
		output = FullRedraw(s.pending)
		s.currentStyle = DefaultStyle()
		s.needsFullRedraw = false
		s.initialized = true
	} else {
		output, s.currentStyle = Diff(s.current, s.pending, &s.currentStyle)
	}

	s.writeString(output)

	// Swap buffers
	s.current, s.pending = s.pending, s.current

	return len(output)
}

// ForceRedraw forces a complete screen redraw on the next Render call.
// Useful after external writes to the terminal or suspected corruption.
func (s *Screen) ForceRedraw() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needsFullRedraw = true
}

// Clear clears the screen immediately.
func (s *Screen) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writeString(CursorHome)
	s.writeString(EraseScreen)

	// Reset buffers
	width, height := s.current.Width, s.current.Height
	s.current = NewBuffer(width, height)
	s.pending = NewBuffer(width, height)
	s.currentStyle = DefaultStyle()
}

// writeString writes a string to the output.
func (s *Screen) writeString(str string) {
	io.WriteString(s.output, str)
}
