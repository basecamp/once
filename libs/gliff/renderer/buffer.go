package renderer

// Buffer represents a screen buffer as a 2D grid of cells.
type Buffer struct {
	Cells  [][]Cell
	Width  int
	Height int
	hashes []uint64 // Line hashes for scroll detection
}

// NewBuffer creates a new buffer with the given dimensions.
func NewBuffer(width, height int) *Buffer {
	b := &Buffer{
		Width:  width,
		Height: height,
		hashes: make([]uint64, height),
	}
	b.Cells = make([][]Cell, height)
	for i := range b.Cells {
		b.Cells[i] = make([]Cell, width)
		for j := range b.Cells[i] {
			b.Cells[i][j] = EmptyCell()
		}
	}
	b.computeHashes()
	return b
}

// SetContent parses content and populates the buffer.
func (b *Buffer) SetContent(content string) {
	b.Clear()
	parseContentInto(content, b.Cells, b.Width, b.Height)
	b.computeHashes()
}

// Clear resets all cells to empty without reallocating.
func (b *Buffer) Clear() {
	empty := EmptyCell()
	for i := range b.Cells {
		for j := range b.Cells[i] {
			b.Cells[i][j] = empty
		}
	}
}

// Resize changes the buffer dimensions, preserving what content fits.
func (b *Buffer) Resize(width, height int) {
	newCells := make([][]Cell, height)
	for i := range newCells {
		newCells[i] = make([]Cell, width)
		for j := range newCells[i] {
			newCells[i][j] = EmptyCell()
		}
		// Copy from old buffer if possible
		if i < b.Height && i < len(b.Cells) {
			for j := 0; j < width && j < b.Width && j < len(b.Cells[i]); j++ {
				newCells[i][j] = b.Cells[i][j]
			}
		}
	}

	b.Cells = newCells
	b.Width = width
	b.Height = height
	b.hashes = make([]uint64, height)
	b.computeHashes()
}

// Clone creates a deep copy of the buffer.
func (b *Buffer) Clone() *Buffer {
	clone := &Buffer{
		Width:  b.Width,
		Height: b.Height,
		hashes: make([]uint64, b.Height),
	}
	clone.Cells = make([][]Cell, b.Height)
	for i := range clone.Cells {
		clone.Cells[i] = make([]Cell, b.Width)
		copy(clone.Cells[i], b.Cells[i])
		clone.hashes[i] = b.hashes[i]
	}
	return clone
}

// computeHashes calculates the hash for each line.
// Used for scroll detection.
func (b *Buffer) computeHashes() {
	for i := range b.Height {
		b.hashes[i] = hashLine(b.Cells[i])
	}
}

// hashLine computes a hash of a line's content.
// Uses the same algorithm as ncurses: result = (result << 5) + result + char
// This is equivalent to result * 33 + char (djb2 hash variant).
func hashLine(cells []Cell) uint64 {
	var h uint64 = 5381 // djb2 starting value

	for i := range cells {
		c := &cells[i]
		// Hash rune (primary content)
		h = (h << 5) + h + uint64(c.Rune)
		// Hash style attributes compactly
		if c.Style.FG.Type != ColorDefault || c.Style.BG.Type != ColorDefault {
			h = (h << 5) + h + uint64(c.Style.FG.Type)<<24 + uint64(c.Style.FG.Value)
			h = (h << 5) + h + uint64(c.Style.BG.Type)<<24 + uint64(c.Style.BG.Value)
		}
		if c.Style.Bold || c.Style.Underline || c.Style.Reverse {
			var flags uint64
			if c.Style.Bold {
				flags |= 1
			}
			if c.Style.Underline {
				flags |= 2
			}
			if c.Style.Reverse {
				flags |= 4
			}
			h = (h << 5) + h + flags
		}
	}
	return h
}

// Line returns the cells for a specific line index.
func (b *Buffer) Line(i int) []Cell {
	if i < 0 || i >= b.Height {
		return nil
	}
	return b.Cells[i]
}

// LineHash returns the hash for a specific line.
func (b *Buffer) LineHash(i int) uint64 {
	if i < 0 || i >= b.Height {
		return 0
	}
	return b.hashes[i]
}

// LinesEqual returns true if the line at index i in both buffers is identical.
func LinesEqual(a, b *Buffer, i int) bool {
	if a.Width != b.Width {
		return false
	}
	if i < 0 || i >= a.Height || i >= b.Height {
		return false
	}
	// Quick check with hash
	if a.hashes[i] != b.hashes[i] {
		return false
	}
	// Full comparison (hash collision check)
	for j := range a.Width {
		if !a.Cells[i][j].Equal(b.Cells[i][j]) {
			return false
		}
	}
	return true
}
