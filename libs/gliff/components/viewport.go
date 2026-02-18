package components

import (
	"strings"

	"github.com/basecamp/gliff/tui"
)

type Viewport struct {
	width    int
	height   int
	yOffset  int
	softWrap bool
	lines    []string
	content  string
}

func NewViewport() *Viewport {
	return &Viewport{}
}

func (v *Viewport) Init() tui.Cmd {
	return nil
}

func (v *Viewport) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.KeyMsg:
		switch msg.Type {
		case tui.KeyUp:
			v.scrollUp(1)
		case tui.KeyDown:
			v.scrollDown(1)
		case tui.KeyPageUp:
			v.scrollUp(v.height)
		case tui.KeyPageDown:
			v.scrollDown(v.height)
		case tui.KeyHome:
			v.yOffset = 0
		case tui.KeyEnd:
			v.GotoBottom()
		}
	}
	return nil
}

func (v *Viewport) Render() string {
	if v.height <= 0 || v.width <= 0 {
		return ""
	}

	lines := v.visibleLines()

	// Pad to exact height
	for len(lines) < v.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (v *Viewport) SetContent(s string) {
	v.content = s
	v.rebuildLines()
}

func (v *Viewport) SetWidth(w int) {
	v.width = w
	v.rebuildLines()
}

func (v *Viewport) SetHeight(h int) {
	v.height = h
	v.clampOffset()
}

func (v *Viewport) Width() int {
	return v.width
}

func (v *Viewport) Height() int {
	return v.height
}

func (v *Viewport) YOffset() int {
	return v.yOffset
}

func (v *Viewport) SetYOffset(n int) {
	v.yOffset = n
	v.clampOffset()
}

func (v *Viewport) AtBottom() bool {
	if len(v.lines) <= v.height {
		return true
	}
	return v.yOffset >= len(v.lines)-v.height
}

func (v *Viewport) GotoBottom() {
	if len(v.lines) > v.height {
		v.yOffset = len(v.lines) - v.height
	} else {
		v.yOffset = 0
	}
}

func (v *Viewport) SetSoftWrap(on bool) {
	v.softWrap = on
	v.rebuildLines()
}

// Private

func (v *Viewport) scrollUp(n int) {
	v.yOffset = max(v.yOffset-n, 0)
}

func (v *Viewport) scrollDown(n int) {
	v.yOffset += n
	v.clampOffset()
}

func (v *Viewport) clampOffset() {
	maxOffset := max(len(v.lines)-v.height, 0)
	v.yOffset = max(0, min(v.yOffset, maxOffset))
}

func (v *Viewport) visibleLines() []string {
	if len(v.lines) == 0 {
		return nil
	}

	start := v.yOffset
	if start >= len(v.lines) {
		start = max(len(v.lines)-1, 0)
	}

	end := min(start+v.height, len(v.lines))
	return v.lines[start:end]
}

func (v *Viewport) rebuildLines() {
	if v.content == "" {
		v.lines = nil
		return
	}

	raw := strings.Split(v.content, "\n")

	if !v.softWrap || v.width <= 0 {
		v.lines = raw
		v.clampOffset()
		return
	}

	var wrapped []string
	for _, line := range raw {
		wrapped = append(wrapped, v.wrapLine(line)...)
	}
	v.lines = wrapped
	v.clampOffset()
}

func (v *Viewport) wrapLine(line string) []string {
	if v.width <= 0 {
		return []string{line}
	}

	w := displayWidth(line)
	if w <= v.width {
		return []string{line}
	}

	var result []string
	remaining := line
	for displayWidth(remaining) > v.width {
		cut := truncateToWidth(remaining, v.width)
		result = append(result, cut)
		remaining = remaining[len(cut):]
	}
	if len(remaining) > 0 {
		result = append(result, remaining)
	}

	return result
}
