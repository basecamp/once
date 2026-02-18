package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColor_Equal(t *testing.T) {
	tests := []struct {
		name     string
		a, b     Color
		expected bool
	}{
		{"both default", DefaultColor(), DefaultColor(), true},
		{"same basic", BasicColor(1), BasicColor(1), true},
		{"different basic", BasicColor(1), BasicColor(2), false},
		{"same 256", PaletteColor(100), PaletteColor(100), true},
		{"different 256", PaletteColor(100), PaletteColor(101), false},
		{"same RGB", RGBColor(255, 128, 64), RGBColor(255, 128, 64), true},
		{"different RGB", RGBColor(255, 128, 64), RGBColor(255, 128, 65), false},
		{"different types", BasicColor(1), PaletteColor(1), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.a.Equal(tc.b))
		})
	}
}

func TestStyle_Equal(t *testing.T) {
	s1 := Style{FG: BasicColor(1), Bold: true}
	s2 := Style{FG: BasicColor(1), Bold: true}
	s3 := Style{FG: BasicColor(1), Bold: false}
	s4 := Style{FG: BasicColor(2), Bold: true}

	assert.True(t, s1.Equal(s2), "identical styles")
	assert.False(t, s1.Equal(s3), "different Bold")
	assert.False(t, s1.Equal(s4), "different FG")
}

func TestStyle_IsDefault(t *testing.T) {
	assert.True(t, DefaultStyle().IsDefault())

	styled := Style{Bold: true}
	assert.False(t, styled.IsDefault(), "Bold style")

	colored := Style{FG: BasicColor(1)}
	assert.False(t, colored.IsDefault(), "colored style")
}

func TestCell_Equal(t *testing.T) {
	c1 := Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	c2 := Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	c3 := Cell{Rune: 'B', Width: 1, Style: DefaultStyle()}
	c4 := Cell{Rune: 'A', Width: 2, Style: DefaultStyle()}
	c5 := Cell{Rune: 'A', Width: 1, Style: Style{Bold: true}}

	assert.True(t, c1.Equal(c2), "identical cells")
	assert.False(t, c1.Equal(c3), "different runes")
	assert.False(t, c1.Equal(c4), "different widths")
	assert.False(t, c1.Equal(c5), "different styles")
}

func TestEmptyCell(t *testing.T) {
	c := EmptyCell()
	assert.Equal(t, ' ', c.Rune)
	assert.Equal(t, 1, c.Width)
	assert.True(t, c.Style.IsDefault())
}

func TestSGRSequence_NoChange(t *testing.T) {
	style := DefaultStyle()
	seq := sgrSequence(style, style)
	assert.Equal(t, "", seq)
}

func TestSGRSequence_ToDefault(t *testing.T) {
	from := Style{Bold: true}
	to := DefaultStyle()
	seq := sgrSequence(from, to)
	assert.Equal(t, SGRReset, seq)
}

func TestSGRSequence_Bold(t *testing.T) {
	from := DefaultStyle()
	to := Style{Bold: true}
	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "1")
}

func TestSGRSequence_BasicForeground(t *testing.T) {
	from := DefaultStyle()
	to := Style{FG: BasicColor(1)} // Red

	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "31") // Red foreground
}

func TestSGRSequence_BrightForeground(t *testing.T) {
	from := DefaultStyle()
	to := Style{FG: BasicColor(9)} // Bright red

	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "91") // Bright red
}

func TestSGRSequence_256Color(t *testing.T) {
	from := DefaultStyle()
	to := Style{FG: PaletteColor(200)}

	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "38;5;200")
}

func TestSGRSequence_RGBColor(t *testing.T) {
	from := DefaultStyle()
	to := Style{FG: RGBColor(255, 128, 64)}

	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "38;2;255;128;64")
}

func TestSGRSequence_Background(t *testing.T) {
	from := DefaultStyle()
	to := Style{BG: BasicColor(4)} // Blue background

	seq := sgrSequence(from, to)
	assert.Contains(t, seq, "44") // Blue background
}

func TestSGRSequence_Multiple(t *testing.T) {
	from := DefaultStyle()
	to := Style{FG: BasicColor(1), Bold: true, Underline: true}

	seq := sgrSequence(from, to)

	assert.Contains(t, seq, "1")  // bold
	assert.Contains(t, seq, "4")  // underline
	assert.Contains(t, seq, "31") // red
}

func TestSGRSequence_IncrementalVsFresh(t *testing.T) {
	from := Style{Bold: true, Italic: true, Underline: true, FG: BasicColor(1)}
	to := Style{Bold: true}

	seq := sgrSequence(from, to)
	assert.LessOrEqual(t, len(seq), 20)
}

func TestColorToSGR_Defaults(t *testing.T) {
	fgDefault := colorToSGR(DefaultColor(), true)
	assert.Equal(t, "39", fgDefault)

	bgDefault := colorToSGR(DefaultColor(), false)
	assert.Equal(t, "49", bgDefault)
}

func TestStyleToSGRCodes_Empty(t *testing.T) {
	codes := styleToSGRCodes(DefaultStyle())
	assert.Empty(t, codes)
}

func TestStyleToSGRCodes_AllAttributes(t *testing.T) {
	s := Style{
		FG:            BasicColor(1),
		BG:            BasicColor(4),
		Bold:          true,
		Dim:           true,
		Italic:        true,
		Underline:     true,
		Blink:         true,
		Reverse:       true,
		Hidden:        true,
		Strikethrough: true,
	}

	codes := styleToSGRCodes(s)

	expected := []string{"1", "2", "3", "4", "5", "7", "8", "9", "31", "44"}
	for _, e := range expected {
		assert.Contains(t, codes, e)
	}
}
