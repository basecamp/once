package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseContent_PlainText(t *testing.T) {
	content := "Hello"
	cells := parseContent(content, 10, 1)

	require.Len(t, cells, 1)
	require.Len(t, cells[0], 10)

	expected := []rune{'H', 'e', 'l', 'l', 'o', ' ', ' ', ' ', ' ', ' '}
	for i, r := range expected {
		assert.Equal(t, r, cells[0][i].Rune, "cell[0][%d]", i)
	}
}

func TestParseContent_MultiLine(t *testing.T) {
	content := "Line1\nLine2\nLine3"
	cells := parseContent(content, 10, 5)

	require.Len(t, cells, 5)

	assert.Equal(t, 'L', cells[0][0].Rune)
	assert.Equal(t, 'L', cells[1][0].Rune)
	assert.Equal(t, 'L', cells[2][0].Rune)
	assert.Equal(t, ' ', cells[3][0].Rune)
}

func TestParseContent_BasicColors(t *testing.T) {
	content := "\x1b[31mRed\x1b[0m"
	cells := parseContent(content, 10, 1)

	assert.Equal(t, 'R', cells[0][0].Rune)
	assert.Equal(t, ColorBasic, cells[0][0].Style.FG.Type)
	assert.Equal(t, uint32(1), cells[0][0].Style.FG.Value)

	assert.True(t, cells[0][3].Style.IsDefault())
}

func TestParseContent_256Colors(t *testing.T) {
	content := "\x1b[38;5;200mX"
	cells := parseContent(content, 5, 1)

	assert.Equal(t, Color256, cells[0][0].Style.FG.Type)
	assert.Equal(t, uint32(200), cells[0][0].Style.FG.Value)
}

func TestParseContent_RGBColors(t *testing.T) {
	content := "\x1b[38;2;255;128;64mX"
	cells := parseContent(content, 5, 1)

	assert.Equal(t, ColorRGB, cells[0][0].Style.FG.Type)

	expectedValue := uint32(255)<<16 | uint32(128)<<8 | uint32(64)
	assert.Equal(t, expectedValue, cells[0][0].Style.FG.Value)
}

func TestParseContent_Attributes(t *testing.T) {
	content := "\x1b[1;4mBU\x1b[0m"
	cells := parseContent(content, 5, 1)

	assert.True(t, cells[0][0].Style.Bold)
	assert.True(t, cells[0][0].Style.Underline)
}

func TestParseContent_WideCharacters(t *testing.T) {
	content := "A中B"
	cells := parseContent(content, 10, 1)

	assert.Equal(t, 'A', cells[0][0].Rune)
	assert.Equal(t, 1, cells[0][0].Width)

	assert.Equal(t, '中', cells[0][1].Rune)
	assert.Equal(t, 2, cells[0][1].Width)

	assert.Equal(t, 0, cells[0][2].Width) // continuation

	assert.Equal(t, 'B', cells[0][3].Rune)
}

func TestParseContent_Tab(t *testing.T) {
	content := "A\tB"
	cells := parseContent(content, 20, 1)

	assert.Equal(t, 'A', cells[0][0].Rune)

	for i := 1; i < 8; i++ {
		assert.Equal(t, ' ', cells[0][i].Rune, "position %d", i)
	}
	assert.Equal(t, 'B', cells[0][8].Rune)
}

func TestParseContent_CarriageReturn(t *testing.T) {
	content := "ABC\rX"
	cells := parseContent(content, 10, 1)

	assert.Equal(t, 'X', cells[0][0].Rune)
	assert.Equal(t, 'B', cells[0][1].Rune)
}

func TestParseContent_Overflow(t *testing.T) {
	content := "ABCDEFGHIJ"
	cells := parseContent(content, 5, 1)

	expected := []rune{'A', 'B', 'C', 'D', 'E'}
	for i, r := range expected {
		assert.Equal(t, r, cells[0][i].Rune, "position %d", i)
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"plain text", "plain text"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;4;31mformatted\x1b[0m text", "formatted text"},
		{"no\x1b[Aescape", "noescape"},
	}

	for _, tc := range tests {
		result := stripANSI(tc.input)
		assert.Equal(t, tc.expected, result, "stripANSI(%q)", tc.input)
	}
}

func TestParseContent_BrightColors(t *testing.T) {
	content := "\x1b[91mX"
	cells := parseContent(content, 5, 1)

	assert.Equal(t, ColorBasic, cells[0][0].Style.FG.Type)
	assert.Equal(t, uint32(9), cells[0][0].Style.FG.Value) // bright red
}

func TestParseContent_BackgroundColor(t *testing.T) {
	content := "\x1b[44mX"
	cells := parseContent(content, 5, 1)

	assert.Equal(t, ColorBasic, cells[0][0].Style.BG.Type)
	assert.Equal(t, uint32(4), cells[0][0].Style.BG.Value) // blue
}
