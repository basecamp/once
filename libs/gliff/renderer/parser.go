package renderer

import (
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// parseState tracks the parser state while processing input.
type parseState int

const (
	parseNormal parseState = iota
	parseEscape            // Saw ESC
	parseCSI               // Saw ESC [
)

// parser converts a string with ANSI escape sequences into a 2D cell grid.
type parser struct {
	width  int
	height int
	cells  [][]Cell
	style  Style

	// Current position
	row, col int

	// Parser state
	state  parseState
	params []int // CSI parameter accumulator
	curNum string
}

// parseContent parses a string into a 2D cell buffer.
// The string may contain ANSI escape sequences for styling.
// Lines are separated by \n. The output is sized to width x height.
func parseContent(content string, width, height int) [][]Cell {
	p := &parser{
		width:  width,
		height: height,
		style:  DefaultStyle(),
	}
	p.initCells()

	for _, r := range content {
		p.processRune(r)
	}

	return p.cells
}

// parseContentInto parses a string into an existing cell buffer.
// The buffer must already be allocated and cleared.
func parseContentInto(content string, cells [][]Cell, width, height int) {
	p := &parser{
		width:  width,
		height: height,
		cells:  cells,
		style:  DefaultStyle(),
	}

	for _, r := range content {
		p.processRune(r)
	}
}

// initCells initializes the cell buffer with empty cells.
func (p *parser) initCells() {
	p.cells = make([][]Cell, p.height)
	for i := range p.cells {
		p.cells[i] = make([]Cell, p.width)
		for j := range p.cells[i] {
			p.cells[i][j] = EmptyCell()
		}
	}
}

// processRune handles a single rune of input.
func (p *parser) processRune(r rune) {
	switch p.state {
	case parseNormal:
		p.handleNormal(r)
	case parseEscape:
		p.handleEscape(r)
	case parseCSI:
		p.handleCSI(r)
	}
}

// handleNormal handles runes in normal (non-escape) mode.
func (p *parser) handleNormal(r rune) {
	switch r {
	case '\x1b': // ESC
		p.state = parseEscape
	case '\n':
		p.row++
		p.col = 0
	case '\r':
		p.col = 0
	case '\t':
		// Tab to next 8-column boundary
		nextTab := ((p.col / 8) + 1) * 8
		for p.col < nextTab && p.col < p.width {
			p.setCell(' ', 1)
		}
	default:
		if r >= 32 { // Printable character
			w := runewidth.RuneWidth(r)
			p.setCell(r, w)
		}
		// Control characters (except those handled above) are ignored
	}
}

// handleEscape handles runes after seeing ESC.
func (p *parser) handleEscape(r rune) {
	switch r {
	case '[':
		p.state = parseCSI
		p.params = nil
		p.curNum = ""
	default:
		// Unknown escape sequence; ignore and return to normal
		p.state = parseNormal
	}
}

// handleCSI handles runes in CSI (Control Sequence Introducer) mode.
func (p *parser) handleCSI(r rune) {
	switch {
	case r >= '0' && r <= '9':
		p.curNum += string(r)
	case r == ';':
		p.pushParam()
	case r == 'm':
		// SGR (Select Graphic Rendition)
		p.pushParam()
		p.handleSGR()
		p.state = parseNormal
	case r >= 0x40 && r <= 0x7E:
		// Other CSI sequences - we ignore them for now
		// (cursor movement, etc. in input doesn't make sense)
		p.state = parseNormal
	default:
		// Invalid CSI sequence
		p.state = parseNormal
	}
}

// pushParam pushes the current numeric parameter.
func (p *parser) pushParam() {
	if p.curNum == "" {
		p.params = append(p.params, 0) // Default parameter
	} else {
		n, _ := strconv.Atoi(p.curNum)
		p.params = append(p.params, n)
	}
	p.curNum = ""
}

// handleSGR processes SGR (Select Graphic Rendition) parameters.
func (p *parser) handleSGR() {
	if len(p.params) == 0 {
		p.params = []int{0} // Default to reset
	}

	i := 0
	for i < len(p.params) {
		param := p.params[i]
		i++

		switch param {
		case 0:
			p.style = DefaultStyle()
		case 1:
			p.style.Bold = true
		case 2:
			p.style.Dim = true
		case 3:
			p.style.Italic = true
		case 4:
			p.style.Underline = true
		case 5:
			p.style.Blink = true
		case 7:
			p.style.Reverse = true
		case 8:
			p.style.Hidden = true
		case 9:
			p.style.Strikethrough = true
		case 22:
			p.style.Bold = false
			p.style.Dim = false
		case 23:
			p.style.Italic = false
		case 24:
			p.style.Underline = false
		case 25:
			p.style.Blink = false
		case 27:
			p.style.Reverse = false
		case 28:
			p.style.Hidden = false
		case 29:
			p.style.Strikethrough = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			p.style.FG = BasicColor(uint8(param - 30))
		case 38:
			// Extended foreground color
			if i < len(p.params) {
				switch p.params[i] {
				case 5: // 256 color
					if i+1 < len(p.params) {
						p.style.FG = PaletteColor(uint8(p.params[i+1]))
						i += 2
					} else {
						i++
					}
				case 2: // RGB
					if i+3 < len(p.params) {
						p.style.FG = RGBColor(
							uint8(p.params[i+1]),
							uint8(p.params[i+2]),
							uint8(p.params[i+3]),
						)
						i += 4
					} else {
						i++
					}
				default:
					i++
				}
			}
		case 39:
			p.style.FG = DefaultColor()
		case 40, 41, 42, 43, 44, 45, 46, 47:
			p.style.BG = BasicColor(uint8(param - 40))
		case 48:
			// Extended background color
			if i < len(p.params) {
				switch p.params[i] {
				case 5: // 256 color
					if i+1 < len(p.params) {
						p.style.BG = PaletteColor(uint8(p.params[i+1]))
						i += 2
					} else {
						i++
					}
				case 2: // RGB
					if i+3 < len(p.params) {
						p.style.BG = RGBColor(
							uint8(p.params[i+1]),
							uint8(p.params[i+2]),
							uint8(p.params[i+3]),
						)
						i += 4
					} else {
						i++
					}
				default:
					i++
				}
			}
		case 49:
			p.style.BG = DefaultColor()
		case 90, 91, 92, 93, 94, 95, 96, 97:
			p.style.FG = BasicColor(uint8(param - 90 + 8))
		case 100, 101, 102, 103, 104, 105, 106, 107:
			p.style.BG = BasicColor(uint8(param - 100 + 8))
		}
	}
}

// setCell writes a character to the current position with the current style.
func (p *parser) setCell(r rune, width int) {
	if p.row >= p.height {
		return // Off-screen
	}

	// Handle characters based on width
	switch width {
	case 2:
		// Need space for both cells
		if p.col+1 >= p.width {
			// Wide char doesn't fit; fill with space and move to next line
			if p.col < p.width {
				p.cells[p.row][p.col] = Cell{Rune: ' ', Width: 1, Style: p.style}
			}
			p.row++
			p.col = 0
			if p.row >= p.height {
				return
			}
		}
		// First cell holds the character
		p.cells[p.row][p.col] = Cell{Rune: r, Width: 2, Style: p.style}
		p.col++
		// Second cell is a continuation marker
		if p.col < p.width {
			p.cells[p.row][p.col] = Cell{Rune: 0, Width: 0, Style: p.style}
			p.col++
		}
	case 1:
		if p.col < p.width {
			p.cells[p.row][p.col] = Cell{Rune: r, Width: 1, Style: p.style}
			p.col++
		}
	}
	// Zero-width characters (combining marks) are currently ignored
	// A more complete implementation would attach them to the previous character
}

// stripANSI removes ANSI escape sequences from a string.
// Useful for calculating the display width of styled text.
func stripANSI(s string) string {
	var result strings.Builder
	state := parseNormal

	for _, r := range s {
		switch state {
		case parseNormal:
			if r == '\x1b' {
				state = parseEscape
			} else {
				result.WriteRune(r)
			}
		case parseEscape:
			if r == '[' {
				state = parseCSI
			} else {
				state = parseNormal
			}
		case parseCSI:
			if r >= 0x40 && r <= 0x7E {
				state = parseNormal
			}
			// Consume CSI parameters
		}
	}

	return result.String()
}
