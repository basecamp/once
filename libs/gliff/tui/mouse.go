package tui

// MouseButton identifies which mouse button was used.
type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
	MouseNone
)

// MouseEventType identifies the type of mouse event.
type MouseEventType int

const (
	MousePress MouseEventType = iota
	MouseRelease
	MouseMotion
)
