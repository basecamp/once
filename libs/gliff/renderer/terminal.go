package renderer

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/term"
)

// Terminal handles low-level terminal operations.
type Terminal struct {
	in  *os.File
	out *os.File

	origState *term.State // Original terminal state for restoration
	width     int
	height    int

	mu         sync.RWMutex
	resizeCh   chan struct{}
	resizeStop chan struct{}
	resizeDone chan struct{}
}

// NewTerminal creates a new terminal handler.
func NewTerminal() (*Terminal, error) {
	t := &Terminal{
		in:         os.Stdin,
		out:        os.Stdout,
		resizeCh:   make(chan struct{}, 1),
		resizeStop: make(chan struct{}),
		resizeDone: make(chan struct{}),
	}

	// Get initial terminal size
	if err := t.updateSize(); err != nil {
		return nil, err
	}

	return t, nil
}

// EnterFullScreen prepares the terminal for full-screen TUI mode.
// This enters raw mode, switches to the alternate screen buffer, and hides the cursor.
func (t *Terminal) EnterFullScreen() error {
	fd := int(t.in.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	t.origState = oldState
	t.out.WriteString(AltScreenEnable)
	t.out.WriteString(CursorHide)
	t.out.WriteString(MouseTrackingEnable)
	t.out.WriteString(MouseSGREnable)
	return nil
}

// ExitFullScreen restores the terminal to its original state.
// This shows the cursor, exits the alternate screen, and restores the terminal mode.
func (t *Terminal) ExitFullScreen() error {
	t.out.WriteString(MouseSGRDisable)
	t.out.WriteString(MouseTrackingDisable)
	t.out.WriteString(CursorShow)
	t.out.WriteString(AltScreenDisable)
	if t.origState == nil {
		return nil
	}
	return term.Restore(int(t.in.Fd()), t.origState)
}

// Size returns the current terminal dimensions.
func (t *Terminal) Size() (width, height int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.width, t.height
}

// Resized returns a channel that receives when the terminal is resized.
func (t *Terminal) Resized() <-chan struct{} {
	return t.resizeCh
}

// updateSize queries the terminal size and updates internal state.
func (t *Terminal) updateSize() error {
	fd := int(t.out.Fd())
	width, height, err := term.GetSize(fd)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.width = width
	t.height = height
	t.mu.Unlock()

	return nil
}

// StartResizeListener starts listening for SIGWINCH signals.
func (t *Terminal) StartResizeListener() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	go func() {
		defer close(t.resizeDone)
		for {
			select {
			case <-sigCh:
				t.updateSize()
				// Non-blocking send to resize channel
				select {
				case t.resizeCh <- struct{}{}:
				default:
				}
			case <-t.resizeStop:
				signal.Stop(sigCh)
				return
			}
		}
	}()
}

// StopResizeListener stops listening for resize signals.
func (t *Terminal) StopResizeListener() {
	close(t.resizeStop)
	<-t.resizeDone
}

// Write writes bytes to the terminal output.
func (t *Terminal) Write(p []byte) (n int, err error) {
	return t.out.Write(p)
}

// WriteString writes a string to the terminal output.
func (t *Terminal) WriteString(s string) (n int, err error) {
	return t.out.WriteString(s)
}

// Flush ensures all output is written (os.File doesn't buffer, but this
// provides a consistent interface).
func (t *Terminal) Flush() error {
	return t.out.Sync()
}
