package renderer

import (
	"strings"
	"testing"
)

func BenchmarkExampleFrame(b *testing.B) {
	s, _ := newTestScreen(80, 24)

	for i := 0; b.Loop(); i++ {
		x := i % 68
		y := i % 23
		content := strings.Repeat("\n", y) + strings.Repeat(" ", x) + "Hello world!"
		s.Render(content)
	}
}

func BenchmarkParseInPlace(b *testing.B) {
	content := strings.Repeat("\n", 12) + strings.Repeat(" ", 30) + "Hello world!"
	buf := NewBuffer(80, 24)
	for b.Loop() {
		buf.SetContent(content)
	}
}
