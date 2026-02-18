package tui

// Key represents a keyboard input event.
type Key struct {
	Type KeyType // Type of key
	Rune rune    // Character for KeyRune type
	Mod  ModKey  // Modifier keys (for special keys)
}

// KeyType identifies the type of key event.
type KeyType int

const (
	KeyRune KeyType = iota // Regular character (check Key.Rune)
	KeyCtrlA
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlG
	KeyCtrlH // Backspace on some terminals
	KeyTab   // Ctrl+I
	KeyCtrlJ // Enter on some terminals
	KeyCtrlK
	KeyCtrlL
	KeyEnter // Ctrl+M
	KeyCtrlN
	KeyCtrlO
	KeyCtrlP
	KeyCtrlQ
	KeyCtrlR
	KeyCtrlS
	KeyCtrlT
	KeyCtrlU
	KeyCtrlV
	KeyCtrlW
	KeyCtrlX
	KeyCtrlY
	KeyCtrlZ
	KeyEscape
	KeyBackspace
	KeyUp
	KeyDown
	KeyRight
	KeyLeft
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyDelete
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyShiftTab
	KeyUnknown
)

// ModKey represents modifier keys.
type ModKey int

const (
	ModNone  ModKey = 0
	ModShift ModKey = 1 << iota
	ModAlt
	ModCtrl
)

// String returns a human-readable representation of the key.
func (k Key) String() string {
	switch k.Type {
	case KeyRune:
		return string(k.Rune)
	case KeyCtrlC:
		return "Ctrl+C"
	case KeyEnter:
		return "Enter"
	case KeyTab:
		return "Tab"
	case KeyEscape:
		return "Escape"
	case KeyBackspace:
		return "Backspace"
	case KeyUp:
		return "Up"
	case KeyDown:
		return "Down"
	case KeyLeft:
		return "Left"
	case KeyRight:
		return "Right"
	default:
		return "Unknown"
	}
}
