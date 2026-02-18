package components

import (
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/basecamp/gliff/tui"
)

const cursorChar = "█"

type EchoMode int

const (
	EchoNormal EchoMode = iota
	EchoPassword
)

type TextField struct {
	value            []rune
	placeholder      string
	placeholderColor color.Color
	charLimit        int
	width            int
	echoMode         EchoMode
	focused          bool
	cursor           int
	offset           int // scroll offset for text longer than width
	blink            bool
	blinkTag         int
}

type textFieldBlinkMsg struct {
	field *TextField
	tag   int
}

func NewTextField() *TextField {
	return &TextField{
		placeholderColor: color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff},
		charLimit:        256,
		width:            20,
		blink:            true,
	}
}

func (t *TextField) Init() tui.Cmd {
	if t.focused {
		return t.blinkCmd()
	}
	return nil
}

func (t *TextField) Update(msg tui.Msg) tui.Cmd {
	if !t.focused {
		return nil
	}

	switch msg := msg.(type) {
	case textFieldBlinkMsg:
		if msg.field != t || msg.tag != t.blinkTag {
			return nil
		}
		t.blink = !t.blink
		return t.blinkCmd()

	case tui.KeyMsg:
		t.blink = true
		switch msg.Type {
		case tui.KeyLeft:
			if t.cursor > 0 {
				t.cursor--
				t.clampOffset()
			}
		case tui.KeyRight:
			if t.cursor < len(t.value) {
				t.cursor++
				t.clampOffset()
			}
		case tui.KeyHome, tui.KeyCtrlA:
			t.cursor = 0
			t.clampOffset()
		case tui.KeyEnd, tui.KeyCtrlE:
			t.cursor = len(t.value)
			t.clampOffset()
		case tui.KeyBackspace:
			if t.cursor > 0 {
				t.value = append(t.value[:t.cursor-1], t.value[t.cursor:]...)
				t.cursor--
				t.clampOffset()
			}
		case tui.KeyDelete:
			if t.cursor < len(t.value) {
				t.value = append(t.value[:t.cursor], t.value[t.cursor+1:]...)
			}
		case tui.KeyCtrlU:
			t.value = t.value[t.cursor:]
			t.cursor = 0
			t.clampOffset()
		case tui.KeyCtrlK:
			t.value = t.value[:t.cursor]
		case tui.KeyCtrlW:
			if t.cursor > 0 {
				// Delete word backward
				i := t.cursor - 1
				for i > 0 && t.value[i] == ' ' {
					i--
				}
				for i > 0 && t.value[i-1] != ' ' {
					i--
				}
				t.value = append(t.value[:i], t.value[t.cursor:]...)
				t.cursor = i
				t.clampOffset()
			}
		case tui.KeyRune:
			if t.charLimit <= 0 || len(t.value) < t.charLimit {
				t.value = append(t.value[:t.cursor], append([]rune{msg.Rune}, t.value[t.cursor:]...)...)
				t.cursor++
				t.clampOffset()
			}
		}
	}

	return nil
}

func (t *TextField) Render() string {
	availWidth := t.width
	if availWidth <= 0 {
		return ""
	}

	var display string
	if len(t.value) == 0 {
		display = t.renderPlaceholder(availWidth)
	} else {
		display = t.renderValue(availWidth)
	}

	return display
}

func (t *TextField) Value() string {
	return string(t.value)
}

func (t *TextField) SetValue(s string) {
	t.value = []rune(s)
	t.cursor = len(t.value)
	t.clampOffset()
}

func (t *TextField) Focus() tui.Cmd {
	t.focused = true
	t.blink = true
	t.blinkTag++
	return t.blinkCmd()
}

func (t *TextField) Blur() {
	t.focused = false
	t.blinkTag++
}

func (t *TextField) Focused() bool {
	return t.focused
}

func (t *TextField) SetPlaceholder(s string) {
	t.placeholder = s
}

func (t *TextField) SetPlaceholderColor(c color.Color) {
	t.placeholderColor = c
}

func (t *TextField) SetCharLimit(n int) {
	t.charLimit = n
}

func (t *TextField) SetWidth(w int) {
	t.width = w
	t.clampOffset()
}

func (t *TextField) SetEchoMode(mode EchoMode) {
	t.echoMode = mode
}

// Private

func (t *TextField) blinkCmd() tui.Cmd {
	field := t
	tag := t.blinkTag
	return tui.After(530*time.Millisecond, func(time.Time) tui.Msg {
		return textFieldBlinkMsg{field: field, tag: tag}
	})
}

func (t *TextField) clampOffset() {
	// Don't scroll further right than necessary
	if maxOffset := max(len(t.value)-t.width+1, 0); t.offset > maxOffset {
		t.offset = maxOffset
	}
	// Ensure cursor is visible within the width
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+t.width {
		t.offset = t.cursor - t.width + 1
	}
}

func (t *TextField) displayRunes() []rune {
	if t.echoMode == EchoPassword {
		return []rune(strings.Repeat("•", len(t.value)))
	}
	return t.value
}

func (t *TextField) renderPlaceholder(width int) string {
	ph := t.placeholder
	if displayWidth(ph) > width {
		ph = truncateToWidth(ph, width)
	}

	phRunes := []rune(ph)
	contentWidth := displayWidth(ph)

	var result strings.Builder
	if t.focused && t.blink {
		if len(phRunes) > 0 {
			result.WriteString(cursorChar)
			if len(phRunes) > 1 {
				result.WriteString(colorToANSI(t.placeholderColor, false))
				result.WriteString(string(phRunes[1:]))
				result.WriteString("\x1b[0m")
			}
		} else {
			result.WriteString(cursorChar)
			contentWidth = 1
		}
	} else {
		result.WriteString(colorToANSI(t.placeholderColor, false))
		result.WriteString(ph)
		result.WriteString("\x1b[0m")
	}

	if contentWidth < width {
		result.WriteString(strings.Repeat(" ", width-contentWidth))
	}

	return result.String()
}

func (t *TextField) renderValue(width int) string {
	display := t.displayRunes()

	// Calculate visible portion
	end := min(t.offset+width, len(display))
	visible := display[t.offset:end]

	var result strings.Builder
	cursorPos := t.cursor - t.offset

	for i, r := range visible {
		if t.focused && i == cursorPos && t.blink {
			result.WriteString("\x1b[7m") // reverse video
			result.WriteRune(r)
			result.WriteString("\x1b[27m") // reset reverse
		} else {
			result.WriteRune(r)
		}
	}

	// Cursor at end of text
	if t.focused && cursorPos == len(visible) && t.blink {
		result.WriteString(cursorChar)
	}

	// Pad to width
	rendered := result.String()
	w := displayWidthRunes(visible)
	if t.focused && cursorPos == len(visible) && t.blink {
		w++
	}
	if w < width {
		rendered += strings.Repeat(" ", width-w)
	}

	return rendered
}

// Helpers

func displayWidthRunes(runes []rune) int {
	n := 0
	for _, r := range runes {
		n += utf8.RuneLen(r)
	}
	return displayWidth(string(runes))
}
