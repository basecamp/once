package components

import (
	"strings"
	"testing"

	"github.com/basecamp/gliff/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextField_InitialState(t *testing.T) {
	tf := NewTextField()
	assert.Equal(t, "", tf.Value())
	assert.False(t, tf.Focused())
}

func TestTextField_SetValue(t *testing.T) {
	tf := NewTextField()
	tf.SetValue("hello")
	assert.Equal(t, "hello", tf.Value())
}

func TestTextField_FocusAndBlur(t *testing.T) {
	tf := NewTextField()

	cmd := tf.Focus()
	assert.True(t, tf.Focused())
	assert.NotNil(t, cmd)

	tf.Blur()
	assert.False(t, tf.Focused())
}

func TestTextField_TypeCharacters(t *testing.T) {
	tf := newFocusedField(20)

	typeText(tf, "hello")
	assert.Equal(t, "hello", tf.Value())
}

func TestTextField_Backspace(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello")

	tf.Update(keyMsg(tui.KeyBackspace))
	assert.Equal(t, "hell", tf.Value())

	tf.Update(keyMsg(tui.KeyBackspace))
	assert.Equal(t, "hel", tf.Value())
}

func TestTextField_BackspaceAtStart(t *testing.T) {
	tf := newFocusedField(20)
	tf.Update(keyMsg(tui.KeyBackspace))
	assert.Equal(t, "", tf.Value())
}

func TestTextField_Delete(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello")

	// Move cursor to start, then delete
	tf.Update(keyMsg(tui.KeyHome))
	tf.Update(keyMsg(tui.KeyDelete))
	assert.Equal(t, "ello", tf.Value())
}

func TestTextField_CursorMovement(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "abc")
	assert.Equal(t, 3, tf.cursor)

	tf.Update(keyMsg(tui.KeyLeft))
	assert.Equal(t, 2, tf.cursor)

	tf.Update(keyMsg(tui.KeyLeft))
	assert.Equal(t, 1, tf.cursor)

	tf.Update(keyMsg(tui.KeyRight))
	assert.Equal(t, 2, tf.cursor)

	tf.Update(keyMsg(tui.KeyHome))
	assert.Equal(t, 0, tf.cursor)

	tf.Update(keyMsg(tui.KeyEnd))
	assert.Equal(t, 3, tf.cursor)
}

func TestTextField_CursorBoundaries(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "ab")

	tf.Update(keyMsg(tui.KeyLeft))
	tf.Update(keyMsg(tui.KeyLeft))
	tf.Update(keyMsg(tui.KeyLeft)) // should not go below 0
	assert.Equal(t, 0, tf.cursor)

	tf.Update(keyMsg(tui.KeyEnd))
	tf.Update(keyMsg(tui.KeyRight)) // should not go past end
	assert.Equal(t, 2, tf.cursor)
}

func TestTextField_InsertAtCursor(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "ac")

	tf.Update(keyMsg(tui.KeyLeft))
	typeText(tf, "b")
	assert.Equal(t, "abc", tf.Value())
}

func TestTextField_CtrlU(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello world")

	tf.Update(keyMsg(tui.KeyLeft))
	tf.Update(keyMsg(tui.KeyLeft))
	tf.Update(keyMsg(tui.KeyCtrlU))
	assert.Equal(t, "ld", tf.Value())
	assert.Equal(t, 0, tf.cursor)
}

func TestTextField_CtrlK(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello world")

	tf.Update(keyMsg(tui.KeyHome))
	for range 5 {
		tf.Update(keyMsg(tui.KeyRight))
	}
	tf.Update(keyMsg(tui.KeyCtrlK))
	assert.Equal(t, "hello", tf.Value())
}

func TestTextField_CtrlW(t *testing.T) {
	tf := newFocusedField(40)
	typeText(tf, "hello world")

	tf.Update(keyMsg(tui.KeyCtrlW))
	assert.Equal(t, "hello ", tf.Value())

	tf.Update(keyMsg(tui.KeyCtrlW))
	assert.Equal(t, "", tf.Value())
}

func TestTextField_CharLimit(t *testing.T) {
	tf := newFocusedField(20)
	tf.SetCharLimit(5)

	typeText(tf, "abcdefgh")
	assert.Equal(t, "abcde", tf.Value())
}

func TestTextField_IgnoresInputWhenBlurred(t *testing.T) {
	tf := NewTextField()
	tf.SetWidth(20)

	tf.Update(runeKeyMsg('a'))
	assert.Equal(t, "", tf.Value())
}

func TestTextField_EchoPassword(t *testing.T) {
	tf := newFocusedField(20)
	tf.SetEchoMode(EchoPassword)
	typeText(tf, "secret")
	assert.Equal(t, "secret", tf.Value())

	tf.Blur()
	rendered := tf.Render()
	stripped := stripANSI(rendered)
	assert.NotContains(t, stripped, "secret")
	assert.Contains(t, stripped, "••••••")
}

func TestTextField_RenderPlaceholder_Unfocused(t *testing.T) {
	tf := NewTextField()
	tf.SetPlaceholder("type here")
	tf.SetWidth(20)

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	assert.Contains(t, stripped, "type here")
	assert.NotContains(t, stripped, cursorChar)
	assert.Equal(t, 20, displayWidth(rendered))
}

func TestTextField_RenderPlaceholder_FocusedBlinkOn(t *testing.T) {
	tf := NewTextField()
	tf.SetPlaceholder("type here")
	tf.SetWidth(20)
	tf.Focus()
	// Focus sets blink=true

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	assert.True(t, strings.HasPrefix(stripped, cursorChar), "should start with cursor")
	assert.Contains(t, stripped, "ype here")
	assert.Equal(t, 20, displayWidth(rendered))
}

func TestTextField_RenderPlaceholder_FocusedBlinkOff(t *testing.T) {
	tf := NewTextField()
	tf.SetPlaceholder("type here")
	tf.SetWidth(20)
	tf.Focus()
	tf.blink = false

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	assert.Contains(t, stripped, "type here")
	assert.NotContains(t, stripped, cursorChar)
	assert.Equal(t, 20, displayWidth(rendered))
}

func TestTextField_RenderPlaceholder_FocusedNoPlaceholder(t *testing.T) {
	tf := NewTextField()
	tf.SetWidth(10)
	tf.Focus()

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	assert.True(t, strings.HasPrefix(stripped, cursorChar), "should show cursor even without placeholder")
	assert.Equal(t, 10, displayWidth(rendered))
}

func TestTextField_RenderValue(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello")

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	assert.Contains(t, stripped, "hello")
	assert.Equal(t, 20, displayWidth(rendered))
}

func TestTextField_RenderValueShowsCursor(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hello")
	// Cursor is at end, blink is on after typing

	rendered := tf.Render()
	stripped := stripANSI(rendered)

	// Cursor block should appear at end of text
	assert.Contains(t, stripped, "hello"+cursorChar)
}

func TestTextField_RenderZeroWidth(t *testing.T) {
	tf := NewTextField()
	tf.SetWidth(0)

	assert.Equal(t, "", tf.Render())
}

func TestTextField_PlaceholderDisappearsOnInput(t *testing.T) {
	tf := NewTextField()
	tf.SetPlaceholder("type here")
	tf.SetWidth(20)
	tf.Focus()

	// Before typing, placeholder visible
	stripped := stripANSI(tf.Render())
	assert.Contains(t, stripped, "ype here")

	// Type a character
	typeText(tf, "x")

	stripped = stripANSI(tf.Render())
	assert.NotContains(t, stripped, "ype here")
	assert.Contains(t, stripped, "x")
}

func TestTextField_PlaceholderReappearsOnDelete(t *testing.T) {
	tf := NewTextField()
	tf.SetPlaceholder("type here")
	tf.SetWidth(20)
	tf.Focus()

	typeText(tf, "x")
	tf.Update(keyMsg(tui.KeyBackspace))

	stripped := stripANSI(tf.Render())
	assert.Contains(t, stripped, "ype here")
}

func TestTextField_Scrolling(t *testing.T) {
	tf := newFocusedField(5)
	typeText(tf, "abcdefgh")

	// Text is 8 chars but width is 5, so it should scroll
	rendered := tf.Render()
	stripped := stripANSI(rendered)

	// Cursor is at end (position 8), visible window should show the tail
	assert.Contains(t, stripped, "fgh")
	assert.NotContains(t, stripped, "abc")
}

func TestTextField_BlinkToggle(t *testing.T) {
	tf := newFocusedField(20)
	typeText(tf, "hi")

	// Blink is on after typing
	r1 := tf.Render()
	assert.Contains(t, stripANSI(r1), cursorChar)

	// Toggle blink off
	tf.Update(textFieldBlinkMsg{field: tf, tag: tf.blinkTag})
	r2 := tf.Render()
	assert.NotContains(t, stripANSI(r2), cursorChar)

	// Toggle blink on
	tf.Update(textFieldBlinkMsg{field: tf, tag: tf.blinkTag})
	r3 := tf.Render()
	assert.Contains(t, stripANSI(r3), cursorChar)
}

func TestTextField_BlinkResetOnKeypress(t *testing.T) {
	tf := newFocusedField(20)
	tf.blink = false

	typeText(tf, "a")
	assert.True(t, tf.blink, "keypress should reset blink to true")
}

func TestTextField_BlinkMsgIgnoredForWrongField(t *testing.T) {
	tf := newFocusedField(20)
	other := NewTextField()

	tf.blink = true
	tf.Update(textFieldBlinkMsg{field: other, tag: tf.blinkTag})
	assert.True(t, tf.blink, "blink should not toggle for wrong field")
}

func TestTextField_BlinkMsgIgnoredForStaleTag(t *testing.T) {
	tf := newFocusedField(20)
	staleTag := tf.blinkTag

	tf.Blur()  // increments blinkTag
	tf.Focus() // increments blinkTag again

	tf.blink = true
	tf.Update(textFieldBlinkMsg{field: tf, tag: staleTag})
	assert.True(t, tf.blink, "blink should not toggle for stale tag")
}

func TestTextField_InitReturnsCmdWhenFocused(t *testing.T) {
	tf := NewTextField()
	tf.Focus()
	cmd := tf.Init()
	require.NotNil(t, cmd)
}

func TestTextField_InitReturnsNilWhenBlurred(t *testing.T) {
	tf := NewTextField()
	cmd := tf.Init()
	assert.Nil(t, cmd)
}

func TestTextField_RenderPlaceholder_Width(t *testing.T) {
	widths := []int{1, 5, 10, 50}
	for _, w := range widths {
		tf := NewTextField()
		tf.SetPlaceholder("placeholder text here")
		tf.SetWidth(w)

		// Unfocused
		assert.Equal(t, w, displayWidth(tf.Render()), "unfocused width=%d", w)

		// Focused blink on
		tf.Focus()
		assert.Equal(t, w, displayWidth(tf.Render()), "focused blink-on width=%d", w)

		// Focused blink off
		tf.blink = false
		assert.Equal(t, w, displayWidth(tf.Render()), "focused blink-off width=%d", w)
	}
}

// Helpers

func newFocusedField(width int) *TextField {
	tf := NewTextField()
	tf.SetWidth(width)
	tf.Focus()
	return tf
}

func typeText(tf *TextField, s string) {
	for _, r := range s {
		tf.Update(runeKeyMsg(r))
	}
}

func keyMsg(keyType tui.KeyType) tui.Msg {
	return tui.KeyMsg{Key: tui.Key{Type: keyType}}
}

func runeKeyMsg(r rune) tui.Msg {
	return tui.KeyMsg{Key: tui.Key{Type: tui.KeyRune, Rune: r}}
}
