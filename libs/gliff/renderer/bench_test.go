package renderer

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// generateContent creates a screen-sized content string
func generateContent(width, height int, fill rune) string {
	var sb strings.Builder
	for row := range height {
		for range width {
			sb.WriteRune(fill)
		}
		if row < height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// generateContentWithLine creates content with a specific line modified
func generateContentWithLine(width, height int, fill rune, lineNum int, lineContent string) string {
	var sb strings.Builder
	for row := range height {
		if row == lineNum {
			sb.WriteString(lineContent)
			// Pad to width
			for i := len(lineContent); i < width; i++ {
				sb.WriteRune(' ')
			}
		} else {
			for range width {
				sb.WriteRune(fill)
			}
		}
		if row < height-1 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

func BenchmarkRender_FullScreen_Small(b *testing.B) {
	s, _ := newTestScreen(80, 24)
	content := generateContent(80, 24, 'X')

	for b.Loop() {
		s.needsFullRedraw = true
		s.Render(content)
	}
}

func BenchmarkRender_FullScreen_Large(b *testing.B) {
	s, _ := newTestScreen(200, 50)
	content := generateContent(200, 50, 'X')

	for b.Loop() {
		s.needsFullRedraw = true
		s.Render(content)
	}
}

func BenchmarkRender_NoChange_Small(b *testing.B) {
	s, _ := newTestScreen(80, 24)
	content := generateContent(80, 24, 'X')

	// Initial render
	s.Render(content)

	for b.Loop() {
		s.Render(content)
	}
}

func BenchmarkRender_NoChange_Large(b *testing.B) {
	s, _ := newTestScreen(200, 50)
	content := generateContent(200, 50, 'X')

	// Initial render
	s.Render(content)

	for b.Loop() {
		s.Render(content)
	}
}

func BenchmarkRender_SingleCellChange(b *testing.B) {
	s, _ := newTestScreen(80, 24)
	content1 := generateContent(80, 24, 'X')
	content2 := generateContent(80, 24, 'X')

	// Modify one character
	content2Runes := []rune(content2)
	content2Runes[80*12+40] = 'O' // Middle of screen
	content2 = string(content2Runes)

	// Initial render
	s.Render(content1)

	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			s.Render(content2)
		} else {
			s.Render(content1)
		}
	}
}

func BenchmarkRender_SingleLineChange(b *testing.B) {
	s, _ := newTestScreen(80, 24)
	content1 := generateContent(80, 24, 'X')
	content2 := generateContentWithLine(80, 24, 'X', 12, "MODIFIED LINE CONTENT HERE")

	// Initial render
	s.Render(content1)

	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			s.Render(content2)
		} else {
			s.Render(content1)
		}
	}
}

func BenchmarkRender_ScrollUp(b *testing.B) {
	s, _ := newTestScreen(80, 24)

	// Generate unique lines for scroll detection
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %04d content here padded to be longer", i)
		// Pad to 80 chars
		for len(lines[i]) < 80 {
			lines[i] += " "
		}
	}

	// Initial: lines 0-23
	content1 := strings.Join(lines[0:24], "\n")
	// Scrolled: lines 1-24
	content2 := strings.Join(lines[1:25], "\n")

	// Initial render
	s.Render(content1)

	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			s.Render(content2)
		} else {
			s.Render(content1)
		}
	}
}

func BenchmarkRender_MultipleLineChanges(b *testing.B) {
	s, _ := newTestScreen(80, 24)
	content1 := generateContent(80, 24, 'X')

	// Create content with 5 lines changed
	var sb strings.Builder
	for row := range 24 {
		if row == 3 || row == 7 || row == 11 || row == 15 || row == 19 {
			sb.WriteString(strings.Repeat("O", 80))
		} else {
			sb.WriteString(strings.Repeat("X", 80))
		}
		if row < 23 {
			sb.WriteRune('\n')
		}
	}
	content2 := sb.String()

	// Initial render
	s.Render(content1)

	for i := 0; b.Loop(); i++ {
		if i%2 == 0 {
			s.Render(content2)
		} else {
			s.Render(content1)
		}
	}
}

func BenchmarkRender_WithColors(b *testing.B) {
	s, _ := newTestScreen(80, 24)

	// Content with various colors
	var sb strings.Builder
	colors := []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m"}
	for row := range 24 {
		for col := range 80 {
			if col%20 == 0 {
				sb.WriteString(colors[(row+col/20)%4])
			}
			sb.WriteRune('X')
		}
		sb.WriteString("\x1b[0m")
		if row < 23 {
			sb.WriteRune('\n')
		}
	}
	content := sb.String()

	// Initial render
	s.Render(content)

	for b.Loop() {
		s.needsFullRedraw = true
		s.Render(content)
	}
}

func BenchmarkParseContent_Plain(b *testing.B) {
	content := generateContent(80, 24, 'X')

	for b.Loop() {
		parseContent(content, 80, 24)
	}
}

func BenchmarkParseContent_WithColors(b *testing.B) {
	var sb strings.Builder
	for range 24 {
		sb.WriteString("\x1b[31m")
		for range 80 {
			sb.WriteRune('X')
		}
		sb.WriteString("\x1b[0m\n")
	}
	content := sb.String()

	for b.Loop() {
		parseContent(content, 80, 24)
	}
}

func BenchmarkDiff_NoChanges(b *testing.B) {
	old := NewBuffer(80, 24)
	new := NewBuffer(80, 24)

	content := generateContent(80, 24, 'X')
	old.SetContent(content)
	new.SetContent(content)

	style := DefaultStyle()

	for b.Loop() {
		Diff(old, new, &style)
	}
}

func BenchmarkDiff_AllChanged(b *testing.B) {
	old := NewBuffer(80, 24)
	new := NewBuffer(80, 24)

	old.SetContent(generateContent(80, 24, 'X'))
	new.SetContent(generateContent(80, 24, 'O'))

	style := DefaultStyle()

	for b.Loop() {
		Diff(old, new, &style)
	}
}

func BenchmarkHashLine(b *testing.B) {
	cells := make([]Cell, 80)
	for i := range cells {
		cells[i] = Cell{Rune: 'X', Width: 1, Style: DefaultStyle()}
	}

	for b.Loop() {
		hashLine(cells)
	}
}

func BenchmarkDetectScroll(b *testing.B) {
	old := NewBuffer(80, 24)
	new := NewBuffer(80, 24)

	// Generate content that will trigger scroll detection
	for i := range 24 {
		line := fmt.Sprintf("Line %02d", i)
		for j, r := range line {
			old.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	old.computeHashes()

	// Scrolled up by 1
	for i := range 24 {
		srcLine := i + 1
		if srcLine >= 24 {
			srcLine = 0
		}
		line := fmt.Sprintf("Line %02d", srcLine)
		for j, r := range line {
			new.Cells[i][j] = Cell{Rune: r, Width: 1, Style: DefaultStyle()}
		}
	}
	new.computeHashes()

	for b.Loop() {
		detectScroll(old, new)
	}
}

func BenchmarkSGRSequence_NoChange(b *testing.B) {
	style := Style{FG: BasicColor(1), Bold: true}

	for b.Loop() {
		sgrSequence(style, style)
	}
}

func BenchmarkSGRSequence_ColorChange(b *testing.B) {
	from := Style{FG: BasicColor(1)}
	to := Style{FG: BasicColor(2)}

	for b.Loop() {
		sgrSequence(from, to)
	}
}

func BenchmarkSGRSequence_Reset(b *testing.B) {
	from := Style{FG: BasicColor(1), Bold: true, Underline: true}
	to := DefaultStyle()

	for b.Loop() {
		sgrSequence(from, to)
	}
}

// BenchmarkRender_Realistic simulates a realistic TUI update scenario
func BenchmarkRender_Realistic_StatusLine(b *testing.B) {
	s, _ := newTestScreen(80, 24)

	// Static content with changing status line
	baseContent := strings.Repeat(strings.Repeat(".", 80)+"\n", 22)

	// Initial render
	s.Render(baseContent + "Status: Ready" + strings.Repeat(" ", 67) + "\n")

	for i := 0; b.Loop(); i++ {
		status := fmt.Sprintf("Status: Processing %d", i)
		padding := strings.Repeat(" ", 80-len(status))
		s.Render(baseContent + status + padding + "\n")
	}
}

// BenchmarkFullRedraw measures full screen redraw performance
func BenchmarkFullRedraw(b *testing.B) {
	buf := NewBuffer(80, 24)
	buf.SetContent(generateContent(80, 24, 'X'))

	for b.Loop() {
		FullRedraw(buf)
	}
}

// BenchmarkWrite measures raw output performance
func BenchmarkWrite_Small(b *testing.B) {
	var buf bytes.Buffer
	content := "Hello, World!"

	for b.Loop() {
		buf.Reset()
		buf.WriteString(content)
	}
}

func BenchmarkWrite_FullScreen(b *testing.B) {
	var buf bytes.Buffer
	content := generateContent(80, 24, 'X')

	for b.Loop() {
		buf.Reset()
		buf.WriteString(content)
	}
}
