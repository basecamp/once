package ui

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// WithRawTerminal switches /dev/tty to raw mode, calls fn, and
// restores the terminal to its original mode when fn returns.
func WithRawTerminal(fn func() error) error {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return err
	}

	saved, err := term.MakeRaw(tty.Fd())
	if err != nil {
		tty.Close()
		return err
	}

	defer func() {
		term.Restore(tty.Fd(), saved)
		tty.Close()
	}()

	return fn()
}
