package tui

import (
	"os"

	"github.com/basecamp/gliff/renderer"
)

type screen interface {
	Render(content string) int
	Resize(width, height int)
}

type Application struct {
	component    Component
	mouseTracker *MouseTracker
}

func NewApplication(c Component) *Application {
	return &Application{
		component:    c,
		mouseTracker: defaultMouseTracker,
	}
}

func (a *Application) Run() error {
	// Create and setup terminal
	term, err := renderer.NewTerminal()
	if err != nil {
		return err
	}

	if err := term.EnterFullScreen(); err != nil {
		return err
	}
	defer term.ExitFullScreen()

	// Create screen and message channel
	width, height := term.Size()
	screen := renderer.NewScreen(term, width, height)
	msgs := make(chan Msg)

	// Start event sources
	defer a.handleResizeEvents(term, msgs)()
	defer a.handleInputEvents(msgs)()

	// Initialize component and run event loop
	a.initialize(screen, msgs, width, height)
	return a.eventLoop(screen, msgs)
}

func (a *Application) handleResizeEvents(term *renderer.Terminal, msgs chan<- Msg) (stop func()) {
	term.StartResizeListener()
	go func() {
		for range term.Resized() {
			w, h := term.Size()
			msgs <- WindowSizeMsg{Width: w, Height: h}
		}
	}()
	return term.StopResizeListener
}

func (a *Application) handleInputEvents(msgs chan<- Msg) (stop func()) {
	input := newInputReader(os.Stdin)
	go func() {
		for key := range input.Keys() {
			msgs <- KeyMsg{key}
		}
	}()
	go func() {
		for mouse := range input.Mouse() {
			msgs <- mouse
		}
	}()
	return input.Stop
}

func (a *Application) initialize(s screen, msgs chan Msg, width, height int) {
	a.runCmd(a.component.Init(), msgs)
	a.runCmd(a.component.Update(WindowSizeMsg{Width: width, Height: height}), msgs)
	s.Render(a.mouseTracker.Sweep(a.component.Render()))
}

func (a *Application) eventLoop(s screen, msgs chan Msg) error {
	for msg := range msgs {
		switch m := msg.(type) {
		case QuitMsg:
			return nil
		case WindowSizeMsg:
			s.Resize(m.Width, m.Height)
		case BatchMsg:
			for _, cmd := range m {
				a.runCmd(cmd, msgs)
			}
			continue
		case MouseMsg:
			m.Target = a.mouseTracker.Resolve(m.X, m.Y)
			msg = m
		}

		cmd := a.component.Update(msg)
		s.Render(a.mouseTracker.Sweep(a.component.Render()))
		a.runCmd(cmd, msgs)
	}
	return nil
}

func (a *Application) runCmd(cmd Cmd, msgs chan<- Msg) {
	if cmd == nil {
		return
	}
	go func() {
		if msg := cmd(); msg != nil {
			msgs <- msg
		}
	}()
}
