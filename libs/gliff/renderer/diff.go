package renderer

import (
	"fmt"
	"strings"
)

// ScrollOp represents a scroll operation.
type ScrollOp struct {
	Direction int // Positive = scroll up (content moves up), Negative = scroll down
	Amount    int // Number of lines
	Top       int // Top of scroll region (0-indexed)
	Bottom    int // Bottom of scroll region (0-indexed, inclusive)
}

// detectScroll analyzes old and new buffers to find scroll operations.
// This implements the Heckel algorithm as used by ncurses:
//
// 1. Find lines that are unique in both old and new (appear exactly once)
// 2. These unique matching lines are anchor points
// 3. Expand from anchors to find contiguous blocks of moved lines
// 4. Validate hunks: require 3+ lines and reasonable shift distance
//
// Returns nil if no beneficial scroll was detected.
func detectScroll(old, new *Buffer) *ScrollOp {
	if old.Height != new.Height || old.Height < 3 {
		return nil
	}

	height := old.Height

	// Build occurrence tables for hashes
	// Count how many times each hash appears in old and new
	oldCount := make(map[uint64]int)
	newCount := make(map[uint64]int)
	oldIndex := make(map[uint64]int) // Last index where hash appears in old
	newIndex := make(map[uint64]int) // Last index where hash appears in new

	for i := range height {
		oldCount[old.hashes[i]]++
		oldIndex[old.hashes[i]] = i
		newCount[new.hashes[i]]++
		newIndex[new.hashes[i]] = i
	}

	// Find unique lines: lines whose hash appears exactly once in both
	// These are our anchor points (Heckel's key insight)
	// newToOld[i] = j means new line i matches old line j
	newToOld := make([]int, height)
	for i := range newToOld {
		newToOld[i] = -1 // -1 means no match
	}

	for i := range height {
		h := new.hashes[i]
		if newCount[h] == 1 && oldCount[h] == 1 {
			// Unique in both - this is an anchor
			newToOld[i] = oldIndex[h]
		}
	}

	// Expand from anchors: if line i matches old line j,
	// check if i+1 matches j+1 (same hash), and so on
	for i := range height {
		if newToOld[i] >= 0 {
			// Expand forward
			for ni, oi := i+1, newToOld[i]+1; ni < height && oi < height; ni, oi = ni+1, oi+1 {
				if newToOld[ni] >= 0 {
					break // Already matched
				}
				if new.hashes[ni] == old.hashes[oi] {
					newToOld[ni] = oi
				} else {
					break
				}
			}
			// Expand backward
			for ni, oi := i-1, newToOld[i]-1; ni >= 0 && oi >= 0; ni, oi = ni-1, oi-1 {
				if newToOld[ni] >= 0 {
					break // Already matched
				}
				if new.hashes[ni] == old.hashes[oi] {
					newToOld[ni] = oi
				} else {
					break
				}
			}
		}
	}

	// Find the largest contiguous block with consistent offset
	// A "hunk" is a range of new lines that all map to old lines with the same shift
	bestStart := -1
	bestSize := 0
	bestShift := 0

	i := 0
	for i < height {
		if newToOld[i] < 0 {
			i++
			continue
		}

		// Start of a potential hunk
		start := i
		shift := newToOld[i] - i
		size := 1

		// Extend while consecutive and same shift
		for i+size < height && newToOld[i+size] == newToOld[i]+size {
			size++
		}

		// ncurses validation: require 3+ lines, and shift not too large relative to size
		// Formula: size >= 3 && size + min(size/8, 2) >= abs(shift)
		minExtra := max(size/8, 2)
		absShift := shift
		if absShift < 0 {
			absShift = -absShift
		}

		if size >= 3 && size+minExtra >= absShift {
			if size > bestSize {
				bestStart = start
				bestSize = size
				bestShift = shift
			}
		}

		i += size
	}

	if bestStart < 0 || bestShift == 0 {
		return nil
	}

	// Convert to scroll operation
	// bestShift > 0 means old lines were higher (scroll up to see them)
	// bestShift < 0 means old lines were lower (scroll down to see them)
	if bestShift > 0 {
		// Content moved up - scroll up
		return &ScrollOp{
			Direction: 1,
			Amount:    bestShift,
			Top:       bestStart,
			Bottom:    bestStart + bestSize - 1 + bestShift,
		}
	} else {
		// Content moved down - scroll down
		return &ScrollOp{
			Direction: -1,
			Amount:    -bestShift,
			Top:       bestStart + bestShift,
			Bottom:    bestStart + bestSize - 1,
		}
	}
}

// LineChange represents a change to a single line.
type LineChange struct {
	Row      int
	Spans    []ChangeSpan
	ClearEOL bool // Whether to clear to end of line after last span
	ClearCol int  // Column to position cursor before clearing (used when spans is empty)
}

// ChangeSpan represents a contiguous span of changed cells.
type ChangeSpan struct {
	Col   int
	Cells []Cell
}

// diffLines computes the changes needed to transform old line to new line.
// Returns nil if the lines are identical.
func diffLine(row int, old, new []Cell) *LineChange {
	if len(old) != len(new) {
		// Different widths - rewrite whole line
		return &LineChange{
			Row: row,
			Spans: []ChangeSpan{
				{Col: 0, Cells: new},
			},
		}
	}

	// Find changed regions
	var spans []ChangeSpan
	inChange := false
	changeStart := 0

	for i := range len(old) {
		if !old[i].Equal(new[i]) {
			if !inChange {
				inChange = true
				changeStart = i
			}
		} else {
			if inChange {
				// End of changed region
				spans = append(spans, ChangeSpan{
					Col:   changeStart,
					Cells: new[changeStart:i],
				})
				inChange = false
			}
		}
	}
	// Handle final change region
	if inChange {
		spans = append(spans, ChangeSpan{
			Col:   changeStart,
			Cells: new[changeStart:],
		})
	}

	if len(spans) == 0 {
		return nil // No changes
	}

	// Optimization: check if it's cheaper to clear the line and rewrite
	// vs. doing sparse updates
	sparseLen := 0
	for _, span := range spans {
		// Cursor movement cost + content
		sparseLen += 6 + len(span.Cells) // Rough estimate
	}
	fullLen := len(new)

	// Determine if we should clear to EOL
	// If the last span extends to the end and new line has trailing spaces
	// we can use clear-to-EOL instead of writing spaces
	clearEOL := false
	clearCol := 0
	if len(spans) > 0 {
		lastSpan := spans[len(spans)-1]
		lastEnd := lastSpan.Col + len(lastSpan.Cells)
		if lastEnd == len(new) {
			// Check if trailing cells are default-styled spaces
			for i := len(lastSpan.Cells) - 1; i >= 0; i-- {
				c := lastSpan.Cells[i]
				if c.Rune == ' ' && c.Style.IsDefault() {
					clearEOL = true
					lastSpan.Cells = lastSpan.Cells[:i]
				} else {
					break
				}
			}
			if len(lastSpan.Cells) == 0 {
				// Track where to clear from before removing the span
				clearCol = lastSpan.Col
				spans = spans[:len(spans)-1]
			} else {
				spans[len(spans)-1] = lastSpan
			}
		}
	}

	// If sparse updates cost more than full line rewrite (with some margin),
	// just rewrite the whole line
	if sparseLen > fullLen*2 && len(spans) > 2 {
		return &LineChange{
			Row: row,
			Spans: []ChangeSpan{
				{Col: 0, Cells: new},
			},
		}
	}

	return &LineChange{
		Row:      row,
		Spans:    spans,
		ClearEOL: clearEOL,
		ClearCol: clearCol,
	}
}

// Diff computes all the changes needed to transform old buffer to new buffer.
// It returns the escape sequences and content to write to the terminal.
func Diff(old, new *Buffer, currentStyle *Style) (string, Style) {
	var out strings.Builder
	style := *currentStyle

	// Scroll detection is expensive. Only do it when there are many changed lines,
	// which suggests content may have scrolled.
	changedLines := 0
	for i := 0; i < new.Height && i < old.Height; i++ {
		if old.hashes[i] != new.hashes[i] {
			changedLines++
		}
	}

	// Only attempt scroll detection if many lines changed (potential scroll)
	if changedLines > new.Height/2 {
		scrollOp := detectScroll(old, new)
		if scrollOp != nil {
			// Apply scroll
			scrollSeq := applyScroll(scrollOp, new.Height)
			out.WriteString(scrollSeq)

			// After scroll, update our understanding of what's on screen
			// by simulating the scroll on the old buffer
			old = simulateScroll(old, scrollOp)
		}
	}

	// Now diff line by line
	for row := range new.Height {
		if row >= old.Height {
			// New line - write it all
			change := &LineChange{
				Row: row,
				Spans: []ChangeSpan{
					{Col: 0, Cells: new.Cells[row]},
				},
			}
			seq, newStyle := renderChange(change, style, new.Width)
			out.WriteString(seq)
			style = newStyle
			continue
		}

		// Check if lines are identical via hash
		// FNV-1a is good enough - collisions are astronomically unlikely
		if old.hashes[row] == new.hashes[row] {
			continue // No change
		}

		// Compute line diff
		change := diffLine(row, old.Cells[row], new.Cells[row])
		if change != nil {
			seq, newStyle := renderChange(change, style, new.Width)
			out.WriteString(seq)
			style = newStyle
		}
	}

	return out.String(), style
}

// applyScroll generates the escape sequences for a scroll operation.
func applyScroll(op *ScrollOp, screenHeight int) string {
	var out strings.Builder

	// Set scroll region if needed
	if op.Top != 0 || op.Bottom != screenHeight-1 {
		out.WriteString(fmt.Sprintf(ScrollRegionSet, op.Top+1, op.Bottom+1))
	}

	// Position cursor within scroll region (some terminals require this)
	// Move to the top-left of the scroll region
	out.WriteString(fmt.Sprintf(CursorPosition, op.Top+1, 1))

	// Perform scroll
	if op.Direction > 0 {
		// Scroll up
		out.WriteString(fmt.Sprintf(ScrollUp, op.Amount))
	} else {
		// Scroll down
		out.WriteString(fmt.Sprintf(ScrollDown, op.Amount))
	}

	// Reset scroll region
	if op.Top != 0 || op.Bottom != screenHeight-1 {
		out.WriteString(ScrollRegionReset)
	}

	return out.String()
}

// simulateScroll applies a scroll operation to a buffer copy.
// Returns a new buffer representing what would be on screen after scrolling.
func simulateScroll(buf *Buffer, op *ScrollOp) *Buffer {
	result := buf.Clone()

	if op.Direction > 0 {
		// Scroll up: move lines up, clear at bottom
		for i := op.Top; i <= op.Bottom-op.Amount; i++ {
			if i+op.Amount <= op.Bottom {
				copy(result.Cells[i], buf.Cells[i+op.Amount])
				result.hashes[i] = buf.hashes[i+op.Amount]
			}
		}
		// Clear the bottom lines that are now blank
		for i := op.Bottom - op.Amount + 1; i <= op.Bottom; i++ {
			for j := range result.Cells[i] {
				result.Cells[i][j] = EmptyCell()
			}
			result.hashes[i] = hashLine(result.Cells[i])
		}
	} else {
		// Scroll down: move lines down, clear at top
		for i := op.Bottom; i >= op.Top+op.Amount; i-- {
			if i-op.Amount >= op.Top {
				copy(result.Cells[i], buf.Cells[i-op.Amount])
				result.hashes[i] = buf.hashes[i-op.Amount]
			}
		}
		// Clear the top lines that are now blank
		for i := op.Top; i < op.Top+op.Amount; i++ {
			for j := range result.Cells[i] {
				result.Cells[i][j] = EmptyCell()
			}
			result.hashes[i] = hashLine(result.Cells[i])
		}
	}

	return result
}

// renderChange generates the escape sequences to apply a line change.
func renderChange(change *LineChange, currentStyle Style, width int) (string, Style) {
	var out strings.Builder
	style := currentStyle

	for _, span := range change.Spans {
		// Move cursor to position
		out.WriteString(fmt.Sprintf(CursorPosition, change.Row+1, span.Col+1))

		// Write cells
		for _, cell := range span.Cells {
			// Handle style change
			if !cell.Style.Equal(style) {
				seq := sgrSequence(style, cell.Style)
				out.WriteString(seq)
				style = cell.Style
			}

			// Write character
			if cell.Width == 0 {
				// Skip continuation cells (they follow wide characters)
				continue
			}
			if cell.Rune == 0 {
				out.WriteRune(' ')
			} else {
				out.WriteRune(cell.Rune)
			}
		}
	}

	// Clear to end of line if needed
	if change.ClearEOL {
		// If no spans were written, position cursor first
		if len(change.Spans) == 0 {
			out.WriteString(fmt.Sprintf(CursorPosition, change.Row+1, change.ClearCol+1))
		}
		// Reset style before clearing (so we clear with default background)
		if !style.IsDefault() {
			out.WriteString(SGRReset)
			style = DefaultStyle()
		}
		out.WriteString(EraseLineRight)
	}

	return out.String(), style
}

// FullRedraw generates the escape sequences to completely redraw the screen.
func FullRedraw(buf *Buffer) string {
	var out strings.Builder
	style := DefaultStyle()

	// Clear screen and home cursor
	out.WriteString(CursorHome)
	out.WriteString(EraseScreen)

	// Write all lines
	for row := range buf.Height {
		fmt.Fprintf(&out, CursorPosition, row+1, 1)

		for _, cell := range buf.Cells[row] {
			if cell.Width == 0 {
				continue // Skip continuation cells
			}

			// Handle style change
			if !cell.Style.Equal(style) {
				seq := sgrSequence(style, cell.Style)
				out.WriteString(seq)
				style = cell.Style
			}

			// Write character
			if cell.Rune == 0 {
				out.WriteRune(' ')
			} else {
				out.WriteRune(cell.Rune)
			}
		}
	}

	// Reset style at end
	if !style.IsDefault() {
		out.WriteString(SGRReset)
	}

	return out.String()
}
