package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/gliff/tui"
)

// mockComponent is a simple component for testing.
type mockComponent struct {
	width      int
	height     int
	initCalled bool
	initCmd    tui.Cmd
	messages   []tui.Msg
	renderText string
}

func (m *mockComponent) Init() tui.Cmd {
	m.initCalled = true
	return m.initCmd
}

func (m *mockComponent) Update(msg tui.Msg) tui.Cmd {
	m.messages = append(m.messages, msg)
	switch msg := msg.(type) {
	case tui.ComponentSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return nil
}

func (m *mockComponent) Render() string { return m.renderText }

func TestCalculateSizes_TwoFillOddTotal(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)
	stack.height = 101
	stack.width = 80

	sizes := stack.calculateSizes()

	assert.Equal(t, 51, sizes[0], "first Fill should get 51")
	assert.Equal(t, 50, sizes[1], "second Fill should get 50")
	assert.Equal(t, 101, sizes[0]+sizes[1], "total should be 101")
}

func TestCalculateSizes_ThreeFillWithRemainder(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	c3 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
		Child{Component: c3, Size: Fill{}},
	)
	stack.height = 100
	stack.width = 80

	sizes := stack.calculateSizes()

	// 100 / 3 = 33 remainder 1, so first gets 34
	assert.Equal(t, 34, sizes[0], "first Fill should get 34")
	assert.Equal(t, 33, sizes[1], "second Fill should get 33")
	assert.Equal(t, 33, sizes[2], "third Fill should get 33")
	assert.Equal(t, 100, sizes[0]+sizes[1]+sizes[2], "total should be 100")
}

func TestCalculateSizes_PercentWithRounding(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Percent(25)},
		Child{Component: c2, Size: Percent(75)},
	)
	stack.height = 101
	stack.width = 80

	sizes := stack.calculateSizes()

	// 25% of 101 = 25.25 -> 25
	// 75% of 101 = 75.75 -> 75
	// Total = 100, need to add 1 to reach 101
	assert.Equal(t, 26, sizes[0], "first Percent should get 26")
	assert.Equal(t, 75, sizes[1], "second Percent should get 75")
	assert.Equal(t, 101, sizes[0]+sizes[1], "total should be 101")
}

func TestCalculateSizes_MixedFixedPercentFill(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	c3 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},   // fixed 10
		Child{Component: c2, Size: Percent(50)}, // 50% of 100 = 50
		Child{Component: c3, Size: Fill{}},      // remaining = 40
	)
	stack.height = 100
	stack.width = 80

	sizes := stack.calculateSizes()

	assert.Equal(t, 10, sizes[0], "Fixed should get 10")
	assert.Equal(t, 50, sizes[1], "Percent should get 50")
	assert.Equal(t, 40, sizes[2], "Fill should get 40")
	assert.Equal(t, 100, sizes[0]+sizes[1]+sizes[2], "total should be 100")
}

func TestCalculateSizes_ZeroRemainingForFill(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(100)},
		Child{Component: c2, Size: Fill{}},
	)
	stack.height = 100
	stack.width = 80

	sizes := stack.calculateSizes()

	assert.Equal(t, 100, sizes[0], "Fixed should get 100")
	assert.Equal(t, 0, sizes[1], "Fill should get 0")
	assert.Equal(t, 100, sizes[0]+sizes[1], "total should be 100")
}

func TestCalculateSizes_NegativeRemainingForFill(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(120)},
		Child{Component: c2, Size: Fill{}},
	)
	stack.height = 100
	stack.width = 80

	sizes := stack.calculateSizes()

	// Fixed exceeds total, Fill should get 0
	// Note: Fixed is preserved even if it exceeds container
	assert.Equal(t, 120, sizes[0], "Fixed should get 120")
	assert.Equal(t, 0, sizes[1], "Fill should get 0 when remaining is negative")
	// Total will exceed container size (120 > 100), but Fixed is honored
}

func TestCalculateSizes_HorizontalDirection(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)
	stack.height = 24
	stack.width = 80

	sizes := stack.calculateSizes()

	assert.Equal(t, 40, sizes[0], "first Fill should get 40")
	assert.Equal(t, 40, sizes[1], "second Fill should get 40")
	assert.Equal(t, 80, sizes[0]+sizes[1], "total should be 80")
}

func TestUpdate_SendsComponentSizeMsg(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},
		Child{Component: c2, Size: Fill{}},
	)

	// Simulate WindowSizeMsg
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 100})

	assert.Equal(t, 80, c1.width, "c1 width")
	assert.Equal(t, 10, c1.height, "c1 height")
	assert.Equal(t, 80, c2.width, "c2 width")
	assert.Equal(t, 90, c2.height, "c2 height")
}

func TestUpdate_SendsComponentSizeMsgHorizontal(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(20)},
		Child{Component: c2, Size: Fill{}},
	)

	// Simulate WindowSizeMsg
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 24})

	assert.Equal(t, 20, c1.width, "c1 width")
	assert.Equal(t, 24, c1.height, "c1 height")
	assert.Equal(t, 60, c2.width, "c2 width")
	assert.Equal(t, 24, c2.height, "c2 height")
}

// --- Init tests ---

func TestInit_CallsInitOnAllChildren(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	c3 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
		Child{Component: c3, Size: Fill{}},
	)

	stack.Init()

	assert.True(t, c1.initCalled, "Init should be called on c1")
	assert.True(t, c2.initCalled, "Init should be called on c2")
	assert.True(t, c3.initCalled, "Init should be called on c3")
}

func TestInit_ReturnsBatchOfChildCommands(t *testing.T) {
	msg1 := struct{ id int }{1}
	msg2 := struct{ id int }{2}
	c1 := &mockComponent{initCmd: func() tui.Msg { return msg1 }}
	c2 := &mockComponent{initCmd: func() tui.Msg { return msg2 }}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)

	cmd := stack.Init()

	require.NotNil(t, cmd, "Init should return a command")
	result := cmd()
	batch, ok := result.(tui.BatchMsg)
	require.True(t, ok, "expected BatchMsg, got %T", result)
	assert.Len(t, batch, 2, "expected 2 commands in batch")
}

func TestInit_ReturnsNilWhenNoChildCommands(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)

	cmd := stack.Init()

	assert.Nil(t, cmd, "Init should return nil when children return no commands")
}

// --- Message passing tests ---

func TestUpdate_PassesNonSizeMessagesToAllChildren(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)

	customMsg := struct{ data string }{"test"}
	stack.Update(customMsg)

	assert.Len(t, c1.messages, 1, "c1 should receive 1 message")
	assert.Len(t, c2.messages, 1, "c2 should receive 1 message")
	assert.Equal(t, customMsg, c1.messages[0], "c1 should receive customMsg")
	assert.Equal(t, customMsg, c2.messages[0], "c2 should receive customMsg")
}

func TestUpdate_HandlesComponentSizeMsg(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},
		Child{Component: c2, Size: Fill{}},
	)

	stack.Update(tui.ComponentSizeMsg{Width: 80, Height: 100})

	assert.Equal(t, 80, c1.width, "c1 width")
	assert.Equal(t, 10, c1.height, "c1 height")
	assert.Equal(t, 80, c2.width, "c2 width")
	assert.Equal(t, 90, c2.height, "c2 height")
}

func TestUpdate_KeyMsgPassedToAllChildren(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fill{}},
		Child{Component: c2, Size: Fill{}},
	)

	keyMsg := tui.KeyMsg{}
	stack.Update(keyMsg)

	require.Len(t, c1.messages, 1, "c1 should receive 1 message")
	_, ok := c1.messages[0].(tui.KeyMsg)
	assert.True(t, ok, "c1 should receive KeyMsg")
	require.Len(t, c2.messages, 1, "c2 should receive 1 message")
	_, ok = c2.messages[0].(tui.KeyMsg)
	assert.True(t, ok, "c2 should receive KeyMsg")
}

// --- Render tests ---

func TestRender_EmptyLayout(t *testing.T) {
	stack := NewStackLayout(Vertical)

	result := stack.Render()

	assert.Equal(t, "", result)
}

func TestRender_VerticalJoinsChildrenWithNewlines(t *testing.T) {
	c1 := &mockComponent{renderText: "AAA"}
	c2 := &mockComponent{renderText: "BBB"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(1)},
		Child{Component: c2, Size: Fixed(1)},
	)
	stack.width = 3
	stack.height = 2
	stack.childSizes = []int{1, 1}

	result := stack.Render()

	assert.Equal(t, "AAA\nBBB", result)
}

func TestRender_VerticalPadsShortLines(t *testing.T) {
	c1 := &mockComponent{renderText: "A"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(1)},
	)
	stack.width = 5
	stack.height = 1
	stack.childSizes = []int{1}

	result := stack.Render()

	assert.Equal(t, "A    ", result)
}

func TestRender_VerticalTruncatesLongLines(t *testing.T) {
	c1 := &mockComponent{renderText: "ABCDEFGH"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(1)},
	)
	stack.width = 5
	stack.height = 1
	stack.childSizes = []int{1}

	result := stack.Render()

	assert.Equal(t, "ABCDE", result)
}

func TestRender_VerticalPadsShortChildren(t *testing.T) {
	c1 := &mockComponent{renderText: "A"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(3)},
	)
	stack.width = 3
	stack.height = 3
	stack.childSizes = []int{3}

	result := stack.Render()

	// Should have 3 lines: "A  " and two empty lines "   "
	assert.Equal(t, "A  \n   \n   ", result)
}

func TestRender_VerticalTruncatesTallChildren(t *testing.T) {
	c1 := &mockComponent{renderText: "A\nB\nC\nD"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(2)},
	)
	stack.width = 1
	stack.height = 2
	stack.childSizes = []int{2}

	result := stack.Render()

	assert.Equal(t, "A\nB", result)
}

func TestRender_HorizontalJoinsChildrenSideBySide(t *testing.T) {
	c1 := &mockComponent{renderText: "AA\nAA"}
	c2 := &mockComponent{renderText: "BB\nBB"}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(2)},
		Child{Component: c2, Size: Fixed(2)},
	)
	stack.width = 4
	stack.height = 2
	stack.childSizes = []int{2, 2}

	result := stack.Render()

	assert.Equal(t, "AABB\nAABB", result)
}

func TestRender_HorizontalPadsShortLines(t *testing.T) {
	c1 := &mockComponent{renderText: "A\nA"}
	c2 := &mockComponent{renderText: "B\nB"}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(3)},
		Child{Component: c2, Size: Fixed(3)},
	)
	stack.width = 6
	stack.height = 2
	stack.childSizes = []int{3, 3}

	result := stack.Render()

	assert.Equal(t, "A  B  \nA  B  ", result)
}

func TestRender_HorizontalTruncatesLongLines(t *testing.T) {
	c1 := &mockComponent{renderText: "AAAA\nAAAA"}
	c2 := &mockComponent{renderText: "BBBB\nBBBB"}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(2)},
		Child{Component: c2, Size: Fixed(2)},
	)
	stack.width = 4
	stack.height = 2
	stack.childSizes = []int{2, 2}

	result := stack.Render()

	assert.Equal(t, "AABB\nAABB", result)
}

func TestRender_HorizontalPadsShortChildren(t *testing.T) {
	c1 := &mockComponent{renderText: "A"}
	c2 := &mockComponent{renderText: "B\nB\nB"}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(2)},
		Child{Component: c2, Size: Fixed(2)},
	)
	stack.width = 4
	stack.height = 3
	stack.childSizes = []int{2, 2}

	result := stack.Render()

	// c1 only has 1 line, should be padded for rows 2 and 3
	assert.Equal(t, "A B \n  B \n  B ", result)
}

func TestRender_VerticalMultipleChildrenLayout(t *testing.T) {
	header := &mockComponent{renderText: "=HEADER="}
	content := &mockComponent{renderText: "Line1\nLine2\nLine3"}
	footer := &mockComponent{renderText: "=FOOTER="}
	stack := NewStackLayout(Vertical,
		Child{Component: header, Size: Fixed(1)},
		Child{Component: content, Size: Fixed(3)},
		Child{Component: footer, Size: Fixed(1)},
	)
	stack.width = 8
	stack.height = 5
	stack.childSizes = []int{1, 3, 1}

	result := stack.Render()

	assert.Equal(t, "=HEADER=\nLine1   \nLine2   \nLine3   \n=FOOTER=", result)
}

// --- Unicode tests ---

func TestRender_UnicodeBoxDrawingCharacters(t *testing.T) {
	// Box drawing characters are 3 bytes each in UTF-8 but 1 display column
	c1 := &mockComponent{renderText: "═══"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(1)},
	)
	stack.width = 5
	stack.height = 1
	stack.childSizes = []int{1}

	result := stack.Render()

	// Should pad to 5 columns: "═══" (3 cols) + "  " (2 spaces)
	assert.Equal(t, "═══  ", result)
}

func TestRender_UnicodeTruncation(t *testing.T) {
	c1 := &mockComponent{renderText: "═════════"}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(1)},
	)
	stack.width = 5
	stack.height = 1
	stack.childSizes = []int{1}

	result := stack.Render()

	// Should truncate to 5 display columns
	assert.Equal(t, "═════", result)
}

func TestRender_HorizontalWithUnicode(t *testing.T) {
	c1 := &mockComponent{renderText: "│A│\n│B│"}
	c2 := &mockComponent{renderText: "│X│\n│Y│"}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(4)},
		Child{Component: c2, Size: Fixed(4)},
	)
	stack.width = 8
	stack.height = 2
	stack.childSizes = []int{4, 4}

	result := stack.Render()

	// Each component is 3 display cols, padded to 4
	assert.Equal(t, "│A│ │X│ \n│B│ │Y│ ", result)
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"═══", 3},
		{"│A│", 3},
		{"", 0},
		{"├──┤", 4},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, displayWidth(tc.input), "displayWidth(%q)", tc.input)
	}
}

func TestDisplayWidth_ANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"no ANSI", "hello", 5},
		{"with background color", "\x1b[48;5;208mhello\x1b[49m", 5},
		{"with foreground color", "\x1b[31mred\x1b[0m", 3},
		{"multiple codes", "\x1b[1m\x1b[31mbold red\x1b[0m", 8},
		{"empty with codes", "\x1b[31m\x1b[0m", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, displayWidth(tc.input))
		})
	}
}

func TestFitToWidth_ANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "pad plain text",
			input:    "hi",
			width:    5,
			expected: "hi   ",
		},
		{
			name:     "pad with trailing reset",
			input:    "\x1b[48;5;208mhi\x1b[49m",
			width:    5,
			expected: "\x1b[48;5;208mhi   \x1b[49m",
		},
		{
			name:     "no padding needed",
			input:    "\x1b[48;5;208mhello\x1b[49m",
			width:    5,
			expected: "\x1b[48;5;208mhello\x1b[49m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, fitToWidth(tc.input, tc.width))
		})
	}
}

func TestFitToWidth(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 8, "hello   "},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"═══", 5, "═══  "},
		{"═════", 3, "═══"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, fitToWidth(tc.input, tc.width), "fitToWidth(%q, %d)", tc.input, tc.width)
	}
}

// --- Mouse routing tests ---

func TestMouseRouting_VerticalHitTest(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	c3 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},
		Child{Component: c2, Size: Fixed(20)},
		Child{Component: c3, Size: Fixed(10)},
	)
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 40})

	tests := []struct {
		name     string
		relY     int
		expected int // expected child index, or -1 for none
	}{
		{"first row of c1", 0, 0},
		{"last row of c1", 9, 0},
		{"first row of c2", 10, 1},
		{"last row of c2", 29, 1},
		{"first row of c3", 30, 2},
		{"last row of c3", 39, 2},
		{"out of bounds", 40, -1},
		{"negative", -1, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, stack.childIndexAt(0, tc.relY))
		})
	}
}

func TestMouseRouting_HorizontalHitTest(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(30)},
		Child{Component: c2, Size: Fixed(50)},
	)
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 24})

	tests := []struct {
		name     string
		relX     int
		expected int
	}{
		{"first col of c1", 0, 0},
		{"last col of c1", 29, 0},
		{"first col of c2", 30, 1},
		{"last col of c2", 79, 1},
		{"out of bounds", 80, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, stack.childIndexAt(tc.relX, 0))
		})
	}
}

func TestMouseRouting_CoordinateTranslation(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},
		Child{Component: c2, Size: Fixed(20)},
	)
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 30})

	// Send a mouse event in the second child's area
	msg := tui.MouseMsg{
		Button: tui.MouseLeft,
		Type:   tui.MousePress,
		X:      5,
		Y:      15,
		RelX:   5,
		RelY:   15,
	}
	stack.Update(msg)

	// c1 should not receive the mouse event
	hasMouseMsg := false
	for _, m := range c1.messages {
		if _, ok := m.(tui.MouseMsg); ok {
			hasMouseMsg = true
		}
	}
	assert.False(t, hasMouseMsg, "c1 should not receive mouse event targeted at c2")

	// c2 should receive the mouse event with translated RelY
	var receivedMsg *tui.MouseMsg
	for _, m := range c2.messages {
		if mouse, ok := m.(tui.MouseMsg); ok {
			receivedMsg = &mouse
		}
	}
	require.NotNil(t, receivedMsg, "c2 should receive mouse event")
	assert.Equal(t, 5, receivedMsg.X, "window X should be preserved")
	assert.Equal(t, 15, receivedMsg.Y, "window Y should be preserved")
	assert.Equal(t, 5, receivedMsg.RelY, "RelY should be translated (15 - 10)")
	assert.Equal(t, 5, receivedMsg.RelX, "RelX should be unchanged for vertical layout")
}

func TestMouseRouting_HorizontalCoordinateTranslation(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Horizontal,
		Child{Component: c1, Size: Fixed(20)},
		Child{Component: c2, Size: Fixed(60)},
	)
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 24})

	// Send a mouse event in the second child's area
	msg := tui.MouseMsg{
		Button: tui.MouseLeft,
		Type:   tui.MousePress,
		X:      35,
		Y:      10,
		RelX:   35,
		RelY:   10,
	}
	stack.Update(msg)

	// c2 should receive the mouse event with translated RelX
	var receivedMsg *tui.MouseMsg
	for _, m := range c2.messages {
		if mouse, ok := m.(tui.MouseMsg); ok {
			receivedMsg = &mouse
		}
	}
	require.NotNil(t, receivedMsg, "c2 should receive mouse event")
	assert.Equal(t, 35, receivedMsg.X, "window X should be preserved")
	assert.Equal(t, 10, receivedMsg.Y, "window Y should be preserved")
	assert.Equal(t, 15, receivedMsg.RelX, "RelX should be translated (35 - 20)")
	assert.Equal(t, 10, receivedMsg.RelY, "RelY should be unchanged for horizontal layout")
}

func TestMouseRouting_NestedLayouts(t *testing.T) {
	// Simulate the demo layout: vertical stack with horizontal middle
	innerC1 := &mockComponent{}
	innerC2 := &mockComponent{}
	innerStack := NewStackLayout(Horizontal,
		Child{Component: innerC1, Size: Fixed(40)},
		Child{Component: innerC2, Size: Fixed(40)},
	)

	header := &mockComponent{}
	footer := &mockComponent{}
	outerStack := NewStackLayout(Vertical,
		Child{Component: header, Size: Fixed(1)},
		Child{Component: innerStack, Size: Fixed(22)},
		Child{Component: footer, Size: Fixed(1)},
	)
	outerStack.Update(tui.WindowSizeMsg{Width: 80, Height: 24})

	// Click in innerC2 (right panel in the middle row)
	msg := tui.MouseMsg{
		Button: tui.MouseLeft,
		Type:   tui.MousePress,
		X:      50,
		Y:      10,
		RelX:   50,
		RelY:   10,
	}
	outerStack.Update(msg)

	// header should not receive the event
	for _, m := range header.messages {
		_, ok := m.(tui.MouseMsg)
		assert.False(t, ok, "header should not receive mouse event")
	}

	// innerC2 should receive the event with fully translated coordinates
	var receivedMsg *tui.MouseMsg
	for _, m := range innerC2.messages {
		if mouse, ok := m.(tui.MouseMsg); ok {
			receivedMsg = &mouse
		}
	}
	require.NotNil(t, receivedMsg, "innerC2 should receive mouse event")
	// Window coords preserved
	assert.Equal(t, 50, receivedMsg.X, "window X should be preserved")
	assert.Equal(t, 10, receivedMsg.Y, "window Y should be preserved")
	// RelY: outer translates Y=10 to RelY=9 (10 - 1 for header)
	// RelX: inner translates X=50 to RelX=10 (50 - 40 for innerC1)
	assert.Equal(t, 10, receivedMsg.RelX, "RelX should be 10 (50 - 40)")
	assert.Equal(t, 9, receivedMsg.RelY, "RelY should be 9 (10 - 1)")
}

func TestMouseRouting_BoundaryConditions(t *testing.T) {
	c1 := &mockComponent{}
	c2 := &mockComponent{}
	stack := NewStackLayout(Vertical,
		Child{Component: c1, Size: Fixed(10)},
		Child{Component: c2, Size: Fixed(10)},
	)
	stack.Update(tui.WindowSizeMsg{Width: 80, Height: 20})

	// Test at exact boundary (first row of c2)
	msg := tui.MouseMsg{
		Button: tui.MouseLeft,
		Type:   tui.MousePress,
		X:      0,
		Y:      10,
		RelX:   0,
		RelY:   10,
	}
	stack.Update(msg)

	// c1 should not receive it
	for _, m := range c1.messages {
		_, ok := m.(tui.MouseMsg)
		assert.False(t, ok, "c1 should not receive mouse event at Y=10 (boundary)")
	}

	// c2 should receive it
	var receivedMsg *tui.MouseMsg
	for _, m := range c2.messages {
		if mouse, ok := m.(tui.MouseMsg); ok {
			receivedMsg = &mouse
		}
	}
	require.NotNil(t, receivedMsg, "c2 should receive mouse event at Y=10")
	assert.Equal(t, 0, receivedMsg.RelY, "RelY at boundary should be 0")
}
