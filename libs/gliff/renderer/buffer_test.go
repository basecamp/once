package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(10, 5)

	assert.Equal(t, 10, b.Width)
	assert.Equal(t, 5, b.Height)
	assert.Len(t, b.Cells, 5)
	assert.Len(t, b.Cells[0], 10)

	// Check cells are initialized to empty
	for i := range 5 {
		for j := range 10 {
			assert.Equal(t, ' ', b.Cells[i][j].Rune, "cell[%d][%d]", i, j)
		}
	}
}

func TestBuffer_SetContent(t *testing.T) {
	b := NewBuffer(10, 3)
	b.SetContent("Hello\nWorld")

	// Check first line
	expected := []rune{'H', 'e', 'l', 'l', 'o', ' ', ' ', ' ', ' ', ' '}
	for i, r := range expected {
		assert.Equal(t, r, b.Cells[0][i].Rune, "row 0, col %d", i)
	}

	// Check second line starts with 'W'
	assert.Equal(t, 'W', b.Cells[1][0].Rune)
}

func TestBuffer_Resize(t *testing.T) {
	b := NewBuffer(10, 5)
	b.SetContent("XXXXX")

	// Resize larger
	b.Resize(15, 8)

	assert.Equal(t, 15, b.Width)
	assert.Equal(t, 8, b.Height)

	// Check original content preserved
	assert.Equal(t, 'X', b.Cells[0][0].Rune)

	// Resize smaller
	b.Resize(3, 2)

	assert.Equal(t, 3, b.Width)
	assert.Equal(t, 2, b.Height)
}

func TestBuffer_Clone(t *testing.T) {
	b := NewBuffer(5, 3)
	b.SetContent("Test")

	clone := b.Clone()

	assert.Equal(t, b.Width, clone.Width)
	assert.Equal(t, b.Height, clone.Height)

	// Verify content is copied
	assert.Equal(t, 'T', clone.Cells[0][0].Rune)

	// Verify it's a deep copy
	clone.Cells[0][0].Rune = 'Z'
	assert.NotEqual(t, 'Z', b.Cells[0][0].Rune, "clone modification should not affect original")
}

func TestBuffer_Line(t *testing.T) {
	b := NewBuffer(5, 3)
	b.SetContent("ABC")

	line := b.Line(0)
	require.NotNil(t, line)
	assert.Len(t, line, 5)

	// Out of bounds
	assert.Nil(t, b.Line(-1))
	assert.Nil(t, b.Line(10))
}

func TestBuffer_LineHash(t *testing.T) {
	b := NewBuffer(5, 2)
	b.SetContent("AAAAA\nBBBBB")

	hash0 := b.LineHash(0)
	hash1 := b.LineHash(1)

	assert.NotEqual(t, hash0, hash1, "different lines")

	// Same content should have same hash
	b2 := NewBuffer(5, 2)
	b2.SetContent("AAAAA\nBBBBB")

	assert.Equal(t, b.LineHash(0), b2.LineHash(0))
}

func TestLinesEqual(t *testing.T) {
	a := NewBuffer(5, 2)
	b := NewBuffer(5, 2)

	a.SetContent("Hello\nWorld")
	b.SetContent("Hello\nWorld")

	assert.True(t, LinesEqual(a, b, 0))

	// Modify one
	b.Cells[0][0].Rune = 'X'
	b.computeHashes()

	assert.False(t, LinesEqual(a, b, 0))
}

func TestLinesEqual_DifferentWidths(t *testing.T) {
	a := NewBuffer(5, 2)
	b := NewBuffer(10, 2)

	a.SetContent("Hello")
	b.SetContent("Hello")

	assert.False(t, LinesEqual(a, b, 0))
}

func TestHashLine_Consistency(t *testing.T) {
	cells := make([]Cell, 10)
	for i := range cells {
		cells[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	}

	hash1 := hashLine(cells)
	hash2 := hashLine(cells)

	assert.Equal(t, hash1, hash2)
}

func TestHashLine_StyleMatters(t *testing.T) {
	cells1 := make([]Cell, 5)
	cells2 := make([]Cell, 5)

	for i := range cells1 {
		cells1[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
		cells2[i] = Cell{Rune: 'A', Width: 1, Style: Style{Bold: true}}
	}

	hash1 := hashLine(cells1)
	hash2 := hashLine(cells2)

	assert.NotEqual(t, hash1, hash2)
}

func TestHashLine_ColorMatters(t *testing.T) {
	cells1 := make([]Cell, 5)
	cells2 := make([]Cell, 5)

	for i := range cells1 {
		cells1[i] = Cell{Rune: 'A', Width: 1, Style: Style{FG: BasicColor(1)}}
		cells2[i] = Cell{Rune: 'A', Width: 1, Style: Style{FG: BasicColor(2)}}
	}

	hash1 := hashLine(cells1)
	hash2 := hashLine(cells2)

	assert.NotEqual(t, hash1, hash2)
}

func BenchmarkBuffer_SetContent(b *testing.B) {
	buf := NewBuffer(80, 24)
	var content strings.Builder
	content.WriteString("This is a test line\n")
	for range 23 {
		content.WriteString("This is a test line\n")
	}

	for b.Loop() {
		buf.SetContent(content.String())
	}
}

func BenchmarkBuffer_Clone(b *testing.B) {
	buf := NewBuffer(80, 24)
	buf.SetContent("Test content repeated across the screen")

	for b.Loop() {
		buf.Clone()
	}
}

func BenchmarkBuffer_ComputeHashes(b *testing.B) {
	buf := NewBuffer(80, 24)
	buf.SetContent("Test content for hashing benchmark")

	for b.Loop() {
		buf.computeHashes()
	}
}
