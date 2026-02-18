package tui

type Msg any

type Cmd func() Msg

type Component interface {
	Init() Cmd
	Update(Msg) Cmd
	Render() string
}

// KeyMsg represents keyboard input.
type KeyMsg struct {
	Key
}

// WindowSizeMsg represents terminal size changes.
type WindowSizeMsg struct {
	Width  int
	Height int
}

// QuitMsg signals that the application should exit.
type QuitMsg struct{}

// MouseMsg represents mouse input.
type MouseMsg struct {
	Button     MouseButton
	Type       MouseEventType
	X, Y       int // Window-absolute coordinates (0-indexed)
	RelX, RelY int // Component-relative coordinates (0-indexed)
	Target     string
}

// ComponentSizeMsg is sent to components within layouts to inform them of their allocated size.
type ComponentSizeMsg struct {
	Width  int
	Height int
}

// BatchMsg is a slice of commands to execute concurrently.
type BatchMsg []Cmd

// Batch combines multiple commands into a single BatchMsg command.
func Batch(cmds ...Cmd) Cmd {
	// Filter out nil commands
	var valid []Cmd
	for _, cmd := range cmds {
		if cmd != nil {
			valid = append(valid, cmd)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return func() Msg {
		return BatchMsg(valid)
	}
}
