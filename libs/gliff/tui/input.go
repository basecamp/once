package tui

import (
	"os"
	"strings"
	"unicode/utf8"
)

// inputReader reads from stdin and sends keys and mouse events to channels.
type inputReader struct {
	in      *os.File
	keyCh   chan Key
	mouseCh chan MouseMsg
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// newInputReader creates and starts an input reader.
func newInputReader(in *os.File) *inputReader {
	ir := &inputReader{
		in:      in,
		keyCh:   make(chan Key, 32),
		mouseCh: make(chan MouseMsg, 32),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	go ir.run()
	return ir
}

// Keys returns the channel of key events.
func (ir *inputReader) Keys() <-chan Key {
	return ir.keyCh
}

// Mouse returns the channel of mouse events.
func (ir *inputReader) Mouse() <-chan MouseMsg {
	return ir.mouseCh
}

// Stop stops the input reader.
// Note: The reader goroutine may not exit immediately if blocked on Read(),
// but it will exit on the next input or when stdin is closed.
func (ir *inputReader) Stop() {
	close(ir.stopCh)
	// Don't wait for doneCh - the goroutine is likely blocked on Read()
	// and we don't want to delay terminal restoration.
}

// run is the main input reading loop.
func (ir *inputReader) run() {
	defer close(ir.doneCh)
	defer close(ir.keyCh)
	defer close(ir.mouseCh)

	buf := make([]byte, 64)
	for {
		select {
		case <-ir.stopCh:
			return
		default:
		}

		// Read with a small buffer
		// Note: This will block, which is fine since we're in a goroutine
		n, err := ir.in.Read(buf)
		if err != nil {
			return
		}

		// Parse the input bytes into keys and mouse events
		keys, mouseEvents := parseInput(buf[:n])
		for _, key := range keys {
			select {
			case ir.keyCh <- key:
			case <-ir.stopCh:
				return
			}
		}
		for _, mouse := range mouseEvents {
			select {
			case ir.mouseCh <- mouse:
			case <-ir.stopCh:
				return
			}
		}
	}
}

// parseInput converts raw input bytes into Key and MouseMsg events.
func parseInput(data []byte) ([]Key, []MouseMsg) {
	var keys []Key
	var mouseEvents []MouseMsg
	i := 0

	for i < len(data) {
		// Check for escape sequences
		if data[i] == 0x1b {
			if i+1 < len(data) && data[i+1] == '[' {
				// CSI sequence - could be key or mouse
				key, mouse, consumed := parseCSIInput(data[i:])
				if mouse != nil {
					mouseEvents = append(mouseEvents, *mouse)
				} else {
					keys = append(keys, key)
				}
				i += consumed
				continue
			} else if i+1 < len(data) && data[i+1] == 'O' {
				// SS3 sequence (some function keys)
				key, consumed := parseSS3(data[i:])
				keys = append(keys, key)
				i += consumed
				continue
			} else if i+1 >= len(data) {
				// Lone escape
				keys = append(keys, Key{Type: KeyEscape})
				i++
				continue
			}
			// Escape followed by something else - could be Alt+key
			// For now, just return escape and let next iteration handle the rest
			keys = append(keys, Key{Type: KeyEscape})
			i++
			continue
		}

		// Control characters (0x00-0x1F)
		if data[i] < 0x20 {
			key := parseControlChar(data[i])
			keys = append(keys, key)
			i++
			continue
		}

		// Backspace (0x7F)
		if data[i] == 0x7F {
			keys = append(keys, Key{Type: KeyBackspace})
			i++
			continue
		}

		// Regular UTF-8 character
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8, skip byte
			i++
			continue
		}
		keys = append(keys, Key{Type: KeyRune, Rune: r})
		i += size
	}

	return keys, mouseEvents
}

// parseControlChar converts a control character byte to a Key.
func parseControlChar(b byte) Key {
	switch b {
	case 0x01:
		return Key{Type: KeyCtrlA}
	case 0x02:
		return Key{Type: KeyCtrlB}
	case 0x03:
		return Key{Type: KeyCtrlC}
	case 0x04:
		return Key{Type: KeyCtrlD}
	case 0x05:
		return Key{Type: KeyCtrlE}
	case 0x06:
		return Key{Type: KeyCtrlF}
	case 0x07:
		return Key{Type: KeyCtrlG}
	case 0x08:
		return Key{Type: KeyBackspace} // Ctrl+H
	case 0x09:
		return Key{Type: KeyTab} // Ctrl+I
	case 0x0A:
		return Key{Type: KeyEnter} // Ctrl+J (LF)
	case 0x0B:
		return Key{Type: KeyCtrlK}
	case 0x0C:
		return Key{Type: KeyCtrlL}
	case 0x0D:
		return Key{Type: KeyEnter} // Ctrl+M (CR)
	case 0x0E:
		return Key{Type: KeyCtrlN}
	case 0x0F:
		return Key{Type: KeyCtrlO}
	case 0x10:
		return Key{Type: KeyCtrlP}
	case 0x11:
		return Key{Type: KeyCtrlQ}
	case 0x12:
		return Key{Type: KeyCtrlR}
	case 0x13:
		return Key{Type: KeyCtrlS}
	case 0x14:
		return Key{Type: KeyCtrlT}
	case 0x15:
		return Key{Type: KeyCtrlU}
	case 0x16:
		return Key{Type: KeyCtrlV}
	case 0x17:
		return Key{Type: KeyCtrlW}
	case 0x18:
		return Key{Type: KeyCtrlX}
	case 0x19:
		return Key{Type: KeyCtrlY}
	case 0x1A:
		return Key{Type: KeyCtrlZ}
	default:
		return Key{Type: KeyUnknown}
	}
}

// parseCSIInput parses a CSI escape sequence (ESC [).
// Returns a Key and nil mouse for keyboard input, or zero Key and mouse for mouse input.
func parseCSIInput(data []byte) (Key, *MouseMsg, int) {
	if len(data) < 3 {
		return Key{Type: KeyEscape}, nil, 1
	}

	// Check for SGR mouse sequence: ESC [ <
	if data[2] == '<' {
		mouse, consumed := parseSGRMouse(data)
		if mouse != nil {
			return Key{}, mouse, consumed
		}
		// Failed to parse mouse, treat as unknown
		return Key{Type: KeyUnknown}, nil, consumed
	}

	// Skip ESC [
	i := 2

	// Collect parameter bytes (0x30-0x3F)
	var params strings.Builder
	for i < len(data) && data[i] >= 0x30 && data[i] <= 0x3F {
		params.WriteString(string(data[i]))
		i++
	}

	// Collect intermediate bytes (0x20-0x2F)
	for i < len(data) && data[i] >= 0x20 && data[i] <= 0x2F {
		i++
	}

	// Final byte (0x40-0x7E)
	if i >= len(data) {
		return Key{Type: KeyEscape}, nil, 1
	}
	final := data[i]
	i++

	// Decode based on final byte and params
	switch final {
	case 'A':
		return Key{Type: KeyUp}, nil, i
	case 'B':
		return Key{Type: KeyDown}, nil, i
	case 'C':
		return Key{Type: KeyRight}, nil, i
	case 'D':
		return Key{Type: KeyLeft}, nil, i
	case 'H':
		return Key{Type: KeyHome}, nil, i
	case 'F':
		return Key{Type: KeyEnd}, nil, i
	case 'Z':
		return Key{Type: KeyShiftTab}, nil, i
	case '~':
		switch params.String() {
		case "1":
			return Key{Type: KeyHome}, nil, i
		case "2":
			return Key{Type: KeyInsert}, nil, i
		case "3":
			return Key{Type: KeyDelete}, nil, i
		case "4":
			return Key{Type: KeyEnd}, nil, i
		case "5":
			return Key{Type: KeyPageUp}, nil, i
		case "6":
			return Key{Type: KeyPageDown}, nil, i
		case "15":
			return Key{Type: KeyF5}, nil, i
		case "17":
			return Key{Type: KeyF6}, nil, i
		case "18":
			return Key{Type: KeyF7}, nil, i
		case "19":
			return Key{Type: KeyF8}, nil, i
		case "20":
			return Key{Type: KeyF9}, nil, i
		case "21":
			return Key{Type: KeyF10}, nil, i
		case "23":
			return Key{Type: KeyF11}, nil, i
		case "24":
			return Key{Type: KeyF12}, nil, i
		}
	}

	return Key{Type: KeyUnknown}, nil, i
}

// parseSGRMouse parses an SGR extended mouse sequence: ESC [ < Cb ; Cx ; Cy M/m
// Format: ESC [ < button ; x ; y M (press) or m (release)
func parseSGRMouse(data []byte) (*MouseMsg, int) {
	// Minimum: ESC [ < N ; N ; N M = 10 bytes for single digits
	if len(data) < 9 {
		return nil, 3
	}

	// Skip ESC [ <
	i := 3

	// Parse button code
	buttonCode := 0
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		buttonCode = buttonCode*10 + int(data[i]-'0')
		i++
	}
	if i >= len(data) || data[i] != ';' {
		return nil, i
	}
	i++ // skip ;

	// Parse X coordinate (1-indexed in protocol)
	x := 0
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		x = x*10 + int(data[i]-'0')
		i++
	}
	if i >= len(data) || data[i] != ';' {
		return nil, i
	}
	i++ // skip ;

	// Parse Y coordinate (1-indexed in protocol)
	y := 0
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		y = y*10 + int(data[i]-'0')
		i++
	}
	if i >= len(data) {
		return nil, i
	}

	// Final byte: M for press, m for release
	final := data[i]
	i++
	if final != 'M' && final != 'm' {
		return nil, i
	}

	// Decode button and event type
	var button MouseButton
	var eventType MouseEventType

	if final == 'm' {
		eventType = MouseRelease
	} else {
		eventType = MousePress
	}

	// Check for motion (bit 5 set)
	if buttonCode&32 != 0 {
		eventType = MouseMotion
		buttonCode &^= 32 // clear motion bit
	}

	// Decode button from lower bits
	switch buttonCode & 0x03 {
	case 0:
		button = MouseLeft
	case 1:
		button = MouseMiddle
	case 2:
		button = MouseRight
	case 3:
		button = MouseNone // release or motion with no button
	}

	// Check for wheel events (bit 6 set)
	if buttonCode&64 != 0 {
		if buttonCode&0x01 == 0 {
			button = MouseWheelUp
		} else {
			button = MouseWheelDown
		}
	}

	// Convert to 0-indexed coordinates
	msg := &MouseMsg{
		Button: button,
		Type:   eventType,
		X:      x - 1,
		Y:      y - 1,
		RelX:   x - 1,
		RelY:   y - 1,
	}

	return msg, i
}

// parseSS3 parses an SS3 escape sequence (ESC O).
func parseSS3(data []byte) (Key, int) {
	if len(data) < 3 {
		return Key{Type: KeyEscape}, 1
	}

	switch data[2] {
	case 'A':
		return Key{Type: KeyUp}, 3
	case 'B':
		return Key{Type: KeyDown}, 3
	case 'C':
		return Key{Type: KeyRight}, 3
	case 'D':
		return Key{Type: KeyLeft}, 3
	case 'H':
		return Key{Type: KeyHome}, 3
	case 'F':
		return Key{Type: KeyEnd}, 3
	case 'P':
		return Key{Type: KeyF1}, 3
	case 'Q':
		return Key{Type: KeyF2}, 3
	case 'R':
		return Key{Type: KeyF3}, 3
	case 'S':
		return Key{Type: KeyF4}, 3
	}

	return Key{Type: KeyUnknown}, 3
}
