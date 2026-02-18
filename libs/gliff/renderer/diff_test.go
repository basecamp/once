package renderer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffLine_NoChange(t *testing.T) {
	cells := make([]Cell, 5)
	for i := range cells {
		cells[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	}

	change := diffLine(0, cells, cells)
	assert.Nil(t, change)
}

func TestDiffLine_SingleCharChange(t *testing.T) {
	old := make([]Cell, 5)
	new := make([]Cell, 5)
	for i := range old {
		old[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
		new[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	}
	new[2] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}

	change := diffLine(0, old, new)
	require.NotNil(t, change)
	require.Len(t, change.Spans, 1)

	assert.Equal(t, 2, change.Spans[0].Col)
	assert.Len(t, change.Spans[0].Cells, 1)
	assert.Equal(t, 'X', change.Spans[0].Cells[0].Rune)
}

func TestDiffLine_MultipleSpans(t *testing.T) {
	old := make([]Cell, 10)
	new := make([]Cell, 10)
	for i := range old {
		old[i] = Cell{Rune: '.', Width: 1, Style: DefaultStyle()}
		new[i] = Cell{Rune: '.', Width: 1, Style: DefaultStyle()}
	}
	new[1] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	new[7] = Cell{Rune: 'B', Width: 1, Style: DefaultStyle()}

	change := diffLine(0, old, new)
	require.NotNil(t, change)
	require.Len(t, change.Spans, 2)

	assert.Equal(t, 1, change.Spans[0].Col)
	assert.Equal(t, 'A', change.Spans[0].Cells[0].Rune)
	assert.Equal(t, 7, change.Spans[1].Col)
	assert.Equal(t, 'B', change.Spans[1].Cells[0].Rune)
}

func TestDetectScroll_ScrollUp(t *testing.T) {
	old := NewBuffer(10, 5)
	new := NewBuffer(10, 5)

	lines := []string{"AAAAAAAAAA", "BBBBBBBBBB", "CCCCCCCCCC", "DDDDDDDDDD", "EEEEEEEEEE"}
	for i, line := range lines {
		for j, r := range line {
			old.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	old.computeHashes()

	newLines := []string{"BBBBBBBBBB", "CCCCCCCCCC", "DDDDDDDDDD", "EEEEEEEEEE", "FFFFFFFFFF"}
	for i, line := range newLines {
		for j, r := range line {
			new.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	new.computeHashes()

	scroll := detectScroll(old, new)
	require.NotNil(t, scroll)

	assert.Equal(t, 1, scroll.Direction)
	assert.Equal(t, 1, scroll.Amount)
}

func TestDetectScroll_ScrollDown(t *testing.T) {
	old := NewBuffer(10, 5)
	new := NewBuffer(10, 5)

	lines := []string{"AAAAAAAAAA", "BBBBBBBBBB", "CCCCCCCCCC", "DDDDDDDDDD", "EEEEEEEEEE"}
	for i, line := range lines {
		for j, r := range line {
			old.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	old.computeHashes()

	newLines := []string{"XXXXXXXXXX", "AAAAAAAAAA", "BBBBBBBBBB", "CCCCCCCCCC", "DDDDDDDDDD"}
	for i, line := range newLines {
		for j, r := range line {
			new.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	new.computeHashes()

	scroll := detectScroll(old, new)
	require.NotNil(t, scroll)

	assert.Equal(t, -1, scroll.Direction)
}

func TestDetectScroll_NoScroll(t *testing.T) {
	old := NewBuffer(10, 5)
	new := NewBuffer(10, 5)

	for i := range 5 {
		for j := range 10 {
			old.Cells[i][j] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
			new.Cells[i][j] = Cell{Rune: 'B', Width: 1, Style: DefaultStyle()}
		}
	}
	old.computeHashes()
	new.computeHashes()

	scroll := detectScroll(old, new)
	assert.Nil(t, scroll)
}

func TestDiff_NoChanges(t *testing.T) {
	old := NewBuffer(10, 3)
	new := NewBuffer(10, 3)

	for i := range 3 {
		for j := range 10 {
			old.Cells[i][j] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
			new.Cells[i][j] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
		}
	}
	old.computeHashes()
	new.computeHashes()

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	assert.Equal(t, "", output)
}

func TestDiff_SingleCellChange(t *testing.T) {
	old := NewBuffer(10, 3)
	new := NewBuffer(10, 3)

	for i := range 3 {
		for j := range 10 {
			old.Cells[i][j] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
			new.Cells[i][j] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
		}
	}
	new.Cells[1][5] = Cell{Rune: 'O', Width: 1, Style: DefaultStyle()}
	old.computeHashes()
	new.computeHashes()

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	assert.Contains(t, output, "O")
	assert.Contains(t, output, "\x1b[2;6H") // cursor position
}

func TestFullRedraw(t *testing.T) {
	buf := NewBuffer(5, 2)
	buf.SetContent("Hello\nWorld")

	output := FullRedraw(buf)

	assert.True(t, strings.HasPrefix(output, CursorHome+EraseScreen))
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "World")
}

func TestRenderChange_WithStyle(t *testing.T) {
	change := &LineChange{
		Row: 0,
		Spans: []ChangeSpan{
			{
				Col: 0,
				Cells: []Cell{
					{Rune: 'R', Width: 1, Style: Style{FG: BasicColor(1)}},
				},
			},
		},
	}

	style := DefaultStyle()
	output, newStyle := renderChange(change, style, 10)

	assert.Contains(t, output, "31") // red SGR
	assert.Equal(t, uint32(1), newStyle.FG.Value)
}

func TestDiff_StyleChange(t *testing.T) {
	old := NewBuffer(5, 1)
	new := NewBuffer(5, 1)

	old.Cells[0][0] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	new.Cells[0][0] = Cell{Rune: 'A', Width: 1, Style: Style{Bold: true}}

	old.computeHashes()
	new.computeHashes()

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	hasBold := strings.Contains(output, "\x1b[1m") || strings.Contains(output, ";1m") || strings.Contains(output, ";1;")
	assert.True(t, hasBold, "output: %q", output)
}

func TestApplyScroll(t *testing.T) {
	op := &ScrollOp{
		Direction: 1,
		Amount:    2,
		Top:       0,
		Bottom:    9,
	}

	output := applyScroll(op, 10)
	assert.Contains(t, output, "\x1b[2S")
}

func TestApplyScroll_WithRegion(t *testing.T) {
	op := &ScrollOp{
		Direction: 1,
		Amount:    1,
		Top:       2,
		Bottom:    7,
	}

	output := applyScroll(op, 10)

	assert.Contains(t, output, "\x1b[3;8r") // scroll region
	assert.Contains(t, output, "\x1b[r")    // reset
}

func TestSimulateScroll_Up(t *testing.T) {
	buf := NewBuffer(5, 4)

	lines := []rune{'A', 'B', 'C', 'D'}
	for i, r := range lines {
		for j := range 5 {
			buf.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	buf.computeHashes()

	op := &ScrollOp{
		Direction: 1,
		Amount:    1,
		Top:       0,
		Bottom:    3,
	}

	result := simulateScroll(buf, op)

	assert.Equal(t, 'B', result.Cells[0][0].Rune)
	assert.Equal(t, 'C', result.Cells[1][0].Rune)
	assert.Equal(t, 'D', result.Cells[2][0].Rune)
	assert.Equal(t, ' ', result.Cells[3][0].Rune)
}

func TestSimulateScroll_Down(t *testing.T) {
	buf := NewBuffer(5, 4)

	lines := []rune{'A', 'B', 'C', 'D'}
	for i, r := range lines {
		for j := range 5 {
			buf.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	buf.computeHashes()

	op := &ScrollOp{
		Direction: -1,
		Amount:    1,
		Top:       0,
		Bottom:    3,
	}

	result := simulateScroll(buf, op)

	assert.Equal(t, ' ', result.Cells[0][0].Rune)
	assert.Equal(t, 'A', result.Cells[1][0].Rune)
	assert.Equal(t, 'B', result.Cells[2][0].Rune)
	assert.Equal(t, 'C', result.Cells[3][0].Rune)
}

func TestDiffLine_ClearEOL(t *testing.T) {
	old := make([]Cell, 10)
	new := make([]Cell, 10)

	for i := range old {
		old[i] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
	}

	for i := range new {
		if i < 3 {
			new[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
		} else {
			new[i] = Cell{Rune: ' ', Width: 1, Style: DefaultStyle()}
		}
	}

	change := diffLine(0, old, new)
	require.NotNil(t, change)

	assert.True(t, change.ClearEOL)
}

func TestDiffLine_ClearEOL_OnlySpaces(t *testing.T) {
	old := make([]Cell, 10)
	new := make([]Cell, 10)

	styledStyle := Style{FG: RGBColor(255, 0, 0)}

	for i := range 5 {
		old[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	}
	for i := 5; i < 10; i++ {
		old[i] = Cell{Rune: 'X', Width: 1, Style: styledStyle}
	}

	for i := range 5 {
		new[i] = Cell{Rune: 'A', Width: 1, Style: DefaultStyle()}
	}
	for i := 5; i < 10; i++ {
		new[i] = Cell{Rune: ' ', Width: 1, Style: DefaultStyle()}
	}

	change := diffLine(0, old, new)
	require.NotNil(t, change)

	assert.True(t, change.ClearEOL)
	assert.Empty(t, change.Spans)
	assert.Equal(t, 5, change.ClearCol)
}

func TestRenderChange_ClearEOL_EmptySpans(t *testing.T) {
	change := &LineChange{
		Row:      2,
		Spans:    nil,
		ClearEOL: true,
		ClearCol: 5,
	}

	output, _ := renderChange(change, DefaultStyle(), 20)

	assert.Contains(t, output, "\x1b[3;6H")
	assert.Contains(t, output, EraseLineRight)
}

func TestDetectScroll_LogsScenario(t *testing.T) {
	height := 45
	width := 30

	old := NewBuffer(width, height)
	new := NewBuffer(width, height)

	old.SetContent("Bytes: 350")
	for i := 1; i <= 40; i++ {
		line := fmt.Sprintf("This is log line %d", 99+i)
		for j, r := range line {
			if j < width {
				old.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
			}
		}
	}
	old.computeHashes()

	new.SetContent("Bytes: 365")
	for i := 1; i <= 40; i++ {
		line := fmt.Sprintf("This is log line %d", 100+i)
		for j, r := range line {
			if j < width {
				new.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
			}
		}
	}
	new.computeHashes()

	scrollOp := detectScroll(old, new)
	require.NotNil(t, scrollOp)

	t.Logf("Scroll detected: direction=%d, amount=%d, top=%d, bottom=%d",
		scrollOp.Direction, scrollOp.Amount, scrollOp.Top, scrollOp.Bottom)

	assert.Equal(t, 1, scrollOp.Direction)
	assert.Equal(t, 1, scrollOp.Amount)
}

func TestDiff_LogsScenario_ByteCount(t *testing.T) {
	height := 45
	width := 30

	old := NewBuffer(width, height)
	new := NewBuffer(width, height)

	for j, r := range "Bytes: 350" {
		if j < width {
			old.Cells[0][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	for i := 1; i <= 40; i++ {
		line := fmt.Sprintf("This is log line %d", 99+i)
		for j, r := range line {
			if j < width {
				old.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
			}
		}
	}
	old.computeHashes()

	for j, r := range "Bytes: 365" {
		if j < width {
			new.Cells[0][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	for i := 1; i <= 40; i++ {
		line := fmt.Sprintf("This is log line %d", 100+i)
		for j, r := range line {
			if j < width {
				new.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
			}
		}
	}
	new.computeHashes()

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	t.Logf("Diff output length: %d bytes", len(output))
	t.Logf("Diff output: %q", output)

	assert.LessOrEqual(t, len(output), 200)
}

func TestDiff_LogsScenario_UsingSetContent(t *testing.T) {
	height := 45
	width := 80

	old := NewBuffer(width, height)
	new := NewBuffer(width, height)

	var oldLines, newLines []string
	for i := 1; i <= 40; i++ {
		oldLines = append(oldLines, fmt.Sprintf("This is log line %d", 99+i))
		newLines = append(newLines, fmt.Sprintf("This is log line %d", 100+i))
	}

	oldContent := "Bytes: 350\n" + strings.Join(oldLines, "\n")
	newContent := "Bytes: 365\n" + strings.Join(newLines, "\n")

	old.SetContent(oldContent)
	new.SetContent(newContent)

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	t.Logf("Diff output length: %d bytes", len(output))
	if len(output) > 100 {
		t.Logf("First 200 chars of output: %q", output[:min(200, len(output))])
	}

	assert.LessOrEqual(t, len(output), 200)
}

func TestDiff_LogsScenario_SmallTerminal(t *testing.T) {
	height := 24
	width := 80

	old := NewBuffer(width, height)
	new := NewBuffer(width, height)

	var oldLines, newLines []string
	for i := 1; i <= 40; i++ {
		oldLines = append(oldLines, fmt.Sprintf("This is log line %d", 99+i))
		newLines = append(newLines, fmt.Sprintf("This is log line %d", 100+i))
	}

	oldContent := "Bytes: 350\n" + strings.Join(oldLines, "\n")
	newContent := "Bytes: 365\n" + strings.Join(newLines, "\n")

	old.SetContent(oldContent)
	new.SetContent(newContent)

	t.Logf("Terminal height: %d, content lines: 41", height)
	t.Logf("Old line 1: %s", string(extractLineText(old.Cells[1])))
	t.Logf("New line 1: %s", string(extractLineText(new.Cells[1])))

	style := DefaultStyle()
	output, _ := Diff(old, new, &style)

	t.Logf("Diff output length: %d bytes", len(output))

	if strings.Contains(output, "S") {
		t.Log("Scroll sequence detected in output")
	} else {
		t.Log("NO scroll sequence in output")
	}
}

func extractLineText(cells []Cell) []rune {
	var result []rune
	for _, c := range cells {
		if c.Rune != ' ' && c.Rune != 0 {
			result = append(result, c.Rune)
		} else if len(result) > 0 && c.Rune == ' ' {
			result = append(result, ' ')
		}
	}
	return result
}
