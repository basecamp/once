package components

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/basecamp/gliff/tui"
)

// ansiPattern matches ANSI escape sequences.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// displayWidth returns the display width of a string in terminal columns.
// This correctly handles wide characters (CJK, emoji) that take 2 columns,
// and ignores ANSI escape sequences.
func displayWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// truncateToWidth truncates a string to fit within the target display width.
// This correctly handles wide characters that take 2 columns and preserves ANSI codes.
func truncateToWidth(s string, width int) string {
	// For strings with ANSI codes, we need to be more careful
	stripped := stripANSI(s)
	if runewidth.StringWidth(stripped) <= width {
		return s
	}

	// Find all ANSI sequences and their positions
	matches := ansiPattern.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return runewidth.Truncate(s, width, "")
	}

	// Build result by iterating through the string
	var result strings.Builder
	visibleWidth := 0
	i := 0
	matchIdx := 0

	for i < len(s) && visibleWidth < width {
		// Check if we're at an ANSI sequence
		if matchIdx < len(matches) && i == matches[matchIdx][0] {
			// Copy the ANSI sequence as-is
			result.WriteString(s[matches[matchIdx][0]:matches[matchIdx][1]])
			i = matches[matchIdx][1]
			matchIdx++
			continue
		}

		// Regular character - check its width
		r, size := []rune(s[i:])[0], len(string([]rune(s[i:])[0]))
		rw := runewidth.RuneWidth(r)
		if visibleWidth+rw > width {
			break
		}
		result.WriteRune(r)
		visibleWidth += rw
		i += size
	}

	// Append any remaining ANSI sequences (like reset codes)
	for matchIdx < len(matches) {
		if matches[matchIdx][0] >= i {
			result.WriteString(s[matches[matchIdx][0]:matches[matchIdx][1]])
		}
		matchIdx++
	}

	return result.String()
}

// fitToWidth pads or truncates a string to exactly match the target width.
// When padding, spaces are inserted before any trailing ANSI reset sequences
// so that padding inherits the current styling.
func fitToWidth(s string, width int) string {
	w := displayWidth(s)
	if w < width {
		padding := strings.Repeat(" ", width-w)
		// Find trailing ANSI sequences (typically reset codes) and insert padding before them
		// This ensures padding inherits the current background color
		trailingANSI, remaining := extractTrailingANSI(s)
		if trailingANSI != "" {
			return remaining + padding + trailingANSI
		}
		return s + padding
	} else if w > width {
		return truncateToWidth(s, width)
	}
	return s
}

// extractTrailingANSI separates trailing ANSI escape sequences from visible content.
// Returns (trailingANSI, remainingContent).
func extractTrailingANSI(s string) (string, string) {
	loc := ansiPattern.FindStringIndex(s)
	if loc == nil {
		return "", s
	}
	// Check if this ANSI sequence is at the end of the visible content
	afterMatch := s[loc[1]:]
	if displayWidth(afterMatch) == 0 {
		// Everything after this point is ANSI codes
		return s[loc[0]:], s[:loc[0]]
	}
	// There's visible content after this ANSI code, find where trailing ANSI starts
	nextStart := loc[1]
	if nextStart >= len(s) {
		return "", s
	}
	suffix := s[nextStart:]
	nextLoc := ansiPattern.FindStringIndex(suffix)
	if nextLoc == nil {
		return "", s
	}
	return s[nextStart+nextLoc[0]:], s[:nextStart+nextLoc[0]]
}

// Direction specifies the layout direction.
type Direction int

const (
	Vertical Direction = iota
	Horizontal
)

// Fixed represents a fixed number of rows (vertical) or columns (horizontal).
type Fixed int

// Percent represents a percentage of the total available space.
type Percent int

// Fill represents an equal share of the remaining space after Fixed and Percent allocations.
type Fill struct{}

// Child represents a component in the layout with its size specification.
type Child struct {
	Component tui.Component
	Size      any // Fixed, Percent, or Fill
}

// StackLayout arranges child components either vertically or horizontally.
type StackLayout struct {
	direction      Direction
	children       []Child
	width          int
	height         int
	childSizes     []int
	childPositions []int // starting position of each child (cumulative offsets)
}

// NewStackLayout creates a new StackLayout with the given direction and children.
func NewStackLayout(direction Direction, children ...Child) *StackLayout {
	return &StackLayout{
		direction:      direction,
		children:       children,
		childSizes:     make([]int, len(children)),
		childPositions: make([]int, len(children)),
	}
}

// Init initializes all children and returns a batch of their commands.
func (s *StackLayout) Init() tui.Cmd {
	cmds := make([]tui.Cmd, len(s.children))
	for i, child := range s.children {
		cmds[i] = child.Component.Init()
	}
	return tui.Batch(cmds...)
}

// Update handles messages and propagates them to children.
func (s *StackLayout) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s.recalculateSizes()
	case tui.ComponentSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s.recalculateSizes()
	case tui.MouseMsg:
		// Route mouse events only to the child that contains the point
		childIdx := s.childIndexAt(msg.RelX, msg.RelY)
		if childIdx >= 0 {
			translated := s.translateMouseForChild(msg, childIdx)
			return s.children[childIdx].Component.Update(translated)
		}
		return nil
	}

	// Pass other messages to all children
	cmds := make([]tui.Cmd, len(s.children))
	for i, child := range s.children {
		cmds[i] = child.Component.Update(msg)
	}
	return tui.Batch(cmds...)
}

// recalculateSizes computes child sizes and sends ComponentSizeMsg to each child.
func (s *StackLayout) recalculateSizes() tui.Cmd {
	s.childSizes = s.calculateSizes()
	s.calculatePositions()

	cmds := make([]tui.Cmd, len(s.children))
	for i, child := range s.children {
		var sizeMsg tui.ComponentSizeMsg
		if s.direction == Vertical {
			sizeMsg = tui.ComponentSizeMsg{Width: s.width, Height: s.childSizes[i]}
		} else {
			sizeMsg = tui.ComponentSizeMsg{Width: s.childSizes[i], Height: s.height}
		}
		cmds[i] = child.Component.Update(sizeMsg)
	}
	return tui.Batch(cmds...)
}

// calculateSizes computes the size allocation for each child.
func (s *StackLayout) calculateSizes() []int {
	total := s.height
	if s.direction == Horizontal {
		total = s.width
	}

	sizes := make([]int, len(s.children))
	remaining := total
	var fillIndices []int

	// First pass: allocate Fixed sizes
	for i, child := range s.children {
		if fixed, ok := child.Size.(Fixed); ok {
			sizes[i] = int(fixed)
			remaining -= sizes[i]
		}
	}

	// Second pass: allocate Percent sizes
	for i, child := range s.children {
		if pct, ok := child.Size.(Percent); ok {
			sizes[i] = total * int(pct) / 100
			remaining -= sizes[i]
		}
	}

	// Third pass: collect Fill components
	for i, child := range s.children {
		if _, ok := child.Size.(Fill); ok {
			fillIndices = append(fillIndices, i)
		}
	}

	// Allocate remaining space to Fill components
	if len(fillIndices) > 0 {
		if remaining < 0 {
			remaining = 0
		}
		base := remaining / len(fillIndices)
		remainder := remaining % len(fillIndices)

		for j, idx := range fillIndices {
			sizes[idx] = base
			if j < remainder {
				sizes[idx]++
			}
		}
	}

	// Final pass: ensure total matches exactly (distribute rounding errors to non-Fixed components)
	actualTotal := 0
	for _, size := range sizes {
		actualTotal += size
	}
	diff := total - actualTotal
	for i := range sizes {
		if diff == 0 {
			break
		}
		// Skip Fixed components - only adjust Percent and Fill
		if _, ok := s.children[i].Size.(Fixed); ok {
			continue
		}
		if sizes[i] > 0 || diff > 0 {
			if diff > 0 {
				sizes[i]++
				diff--
			} else if sizes[i] > 0 {
				sizes[i]--
				diff++
			}
		}
	}

	return sizes
}

// calculatePositions computes the starting position of each child.
func (s *StackLayout) calculatePositions() {
	pos := 0
	for i := range s.children {
		s.childPositions[i] = pos
		pos += s.childSizes[i]
	}
}

// childIndexAt returns the index of the child at the given relative coordinates,
// or -1 if no child contains the point.
func (s *StackLayout) childIndexAt(relX, relY int) int {
	// Determine which coordinate matters based on direction
	coord := relY
	if s.direction == Horizontal {
		coord = relX
	}

	for i := range s.children {
		start := s.childPositions[i]
		end := start + s.childSizes[i]
		if coord >= start && coord < end {
			return i
		}
	}
	return -1
}

// translateMouseForChild adjusts the relative coordinates for a specific child.
func (s *StackLayout) translateMouseForChild(msg tui.MouseMsg, childIdx int) tui.MouseMsg {
	translated := msg
	if s.direction == Vertical {
		translated.RelY = msg.RelY - s.childPositions[childIdx]
	} else {
		translated.RelX = msg.RelX - s.childPositions[childIdx]
	}
	return translated
}

// Render renders all children and joins them according to the layout direction.
func (s *StackLayout) Render() string {
	if len(s.children) == 0 {
		return ""
	}

	if s.direction == Vertical {
		return s.renderVertical()
	}
	return s.renderHorizontal()
}

// renderVertical renders children stacked vertically.
func (s *StackLayout) renderVertical() string {
	var parts []string

	for i, child := range s.children {
		rendered := child.Component.Render()
		targetHeight := s.childSizes[i]

		lines := strings.Split(rendered, "\n")

		// Pad or truncate to exact height
		if len(lines) < targetHeight {
			emptyLine := strings.Repeat(" ", s.width)
			for len(lines) < targetHeight {
				lines = append(lines, emptyLine)
			}
		} else if len(lines) > targetHeight {
			lines = lines[:targetHeight]
		}

		// Pad or truncate each line to width
		for j, line := range lines {
			lines[j] = fitToWidth(line, s.width)
		}

		parts = append(parts, strings.Join(lines, "\n"))
	}

	return strings.Join(parts, "\n")
}

// renderHorizontal renders children arranged side by side.
func (s *StackLayout) renderHorizontal() string {
	// Render each child and split into lines
	childLines := make([][]string, len(s.children))
	for i, child := range s.children {
		rendered := child.Component.Render()
		childLines[i] = strings.Split(rendered, "\n")
	}

	// Build output line by line
	var outputLines []string
	for row := 0; row < s.height; row++ {
		var lineParts []string
		for i, lines := range childLines {
			var line string
			if row < len(lines) {
				line = lines[row]
			}

			targetWidth := s.childSizes[i]
			line = fitToWidth(line, targetWidth)
			lineParts = append(lineParts, line)
		}
		outputLines = append(outputLines, strings.Join(lineParts, ""))
	}

	return strings.Join(outputLines, "\n")
}
