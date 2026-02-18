package renderer

import (
	"fmt"
	"strings"
)

// ColorType represents the type of color encoding.
type ColorType uint8

const (
	ColorDefault ColorType = iota // No color / terminal default
	ColorBasic                    // Basic 16 colors (0-15)
	Color256                      // 256-color palette (0-255)
	ColorRGB                      // 24-bit true color
)

// Color represents a foreground or background color.
type Color struct {
	Type  ColorType
	Value uint32 // Basic: 0-15, 256: 0-255, RGB: 0xRRGGBB
}

// DefaultColor returns a color representing the terminal default.
func DefaultColor() Color {
	return Color{Type: ColorDefault}
}

// BasicColor returns a basic 16-color palette color.
func BasicColor(index uint8) Color {
	return Color{Type: ColorBasic, Value: uint32(index & 0x0F)}
}

// PaletteColor returns a 256-color palette color.
func PaletteColor(index uint8) Color {
	return Color{Type: Color256, Value: uint32(index)}
}

// RGBColor returns a 24-bit true color.
func RGBColor(r, g, b uint8) Color {
	return Color{Type: ColorRGB, Value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
}

// Equal returns true if two colors are identical.
func (c Color) Equal(other Color) bool {
	return c.Type == other.Type && c.Value == other.Value
}

// Style represents the visual attributes of a cell.
type Style struct {
	FG            Color
	BG            Color
	Bold          bool
	Dim           bool
	Italic        bool
	Underline     bool
	Blink         bool
	Reverse       bool
	Hidden        bool
	Strikethrough bool
}

// DefaultStyle returns a style with all defaults (no attributes).
func DefaultStyle() Style {
	return Style{
		FG: DefaultColor(),
		BG: DefaultColor(),
	}
}

// Equal returns true if two styles are identical.
func (s Style) Equal(other Style) bool {
	return s.FG.Equal(other.FG) &&
		s.BG.Equal(other.BG) &&
		s.Bold == other.Bold &&
		s.Dim == other.Dim &&
		s.Italic == other.Italic &&
		s.Underline == other.Underline &&
		s.Blink == other.Blink &&
		s.Reverse == other.Reverse &&
		s.Hidden == other.Hidden &&
		s.Strikethrough == other.Strikethrough
}

// IsDefault returns true if this style has no attributes set.
func (s Style) IsDefault() bool {
	return s.Equal(DefaultStyle())
}

// Cell represents a single character cell on the terminal.
type Cell struct {
	Rune  rune  // The character (0 for continuation of wide char)
	Width int   // Display width: 0 for continuation, 1 for normal, 2 for wide
	Style Style // Visual attributes
}

// EmptyCell returns a cell representing an empty space with default style.
func EmptyCell() Cell {
	return Cell{Rune: ' ', Width: 1, Style: DefaultStyle()}
}

// Equal returns true if two cells are identical.
func (c Cell) Equal(other Cell) bool {
	return c.Rune == other.Rune && c.Width == other.Width && c.Style.Equal(other.Style)
}

// sgrSequence returns the SGR escape sequence to transition from one style to another.
// If from is nil, assumes we're starting from reset state.
func sgrSequence(from, to Style) string {
	// If target is default, just reset
	if to.IsDefault() {
		if from.IsDefault() {
			return ""
		}
		return SGRReset
	}

	// Determine if it's cheaper to reset and set everything fresh
	// vs. incrementally change attributes
	var incremental strings.Builder
	var fresh strings.Builder
	var codes []string

	// Build fresh sequence (after reset)
	freshCodes := styleToSGRCodes(to)
	if len(freshCodes) > 0 {
		fresh.WriteString(CSI)
		fresh.WriteString("0") // reset
		for _, code := range freshCodes {
			fresh.WriteString(";")
			fresh.WriteString(code)
		}
		fresh.WriteString("m")
	} else {
		fresh.WriteString(SGRReset)
	}

	// Build incremental sequence
	if !from.FG.Equal(to.FG) {
		codes = append(codes, colorToSGR(to.FG, true))
	}
	if !from.BG.Equal(to.BG) {
		codes = append(codes, colorToSGR(to.BG, false))
	}
	if from.Bold != to.Bold {
		if to.Bold {
			codes = append(codes, fmt.Sprintf("%d", SGRBold))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRBoldOff))
		}
	}
	if from.Dim != to.Dim {
		if to.Dim {
			codes = append(codes, fmt.Sprintf("%d", SGRDim))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRDimOff))
		}
	}
	if from.Italic != to.Italic {
		if to.Italic {
			codes = append(codes, fmt.Sprintf("%d", SGRItalic))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRItalicOff))
		}
	}
	if from.Underline != to.Underline {
		if to.Underline {
			codes = append(codes, fmt.Sprintf("%d", SGRUnderline))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRUnderlineOff))
		}
	}
	if from.Blink != to.Blink {
		if to.Blink {
			codes = append(codes, fmt.Sprintf("%d", SGRBlink))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRBlinkOff))
		}
	}
	if from.Reverse != to.Reverse {
		if to.Reverse {
			codes = append(codes, fmt.Sprintf("%d", SGRReverse))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRReverseOff))
		}
	}
	if from.Hidden != to.Hidden {
		if to.Hidden {
			codes = append(codes, fmt.Sprintf("%d", SGRHidden))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRHiddenOff))
		}
	}
	if from.Strikethrough != to.Strikethrough {
		if to.Strikethrough {
			codes = append(codes, fmt.Sprintf("%d", SGRStrikethrough))
		} else {
			codes = append(codes, fmt.Sprintf("%d", SGRStrikethroughOff))
		}
	}

	if len(codes) > 0 {
		incremental.WriteString(CSI)
		incremental.WriteString(strings.Join(codes, ";"))
		incremental.WriteString("m")
	}

	// Return the shorter one
	incStr := incremental.String()
	freshStr := fresh.String()

	if len(incStr) == 0 {
		return ""
	}
	if len(incStr) <= len(freshStr) {
		return incStr
	}
	return freshStr
}

// styleToSGRCodes returns the SGR codes needed to set the given style from reset.
func styleToSGRCodes(s Style) []string {
	var codes []string

	if s.Bold {
		codes = append(codes, fmt.Sprintf("%d", SGRBold))
	}
	if s.Dim {
		codes = append(codes, fmt.Sprintf("%d", SGRDim))
	}
	if s.Italic {
		codes = append(codes, fmt.Sprintf("%d", SGRItalic))
	}
	if s.Underline {
		codes = append(codes, fmt.Sprintf("%d", SGRUnderline))
	}
	if s.Blink {
		codes = append(codes, fmt.Sprintf("%d", SGRBlink))
	}
	if s.Reverse {
		codes = append(codes, fmt.Sprintf("%d", SGRReverse))
	}
	if s.Hidden {
		codes = append(codes, fmt.Sprintf("%d", SGRHidden))
	}
	if s.Strikethrough {
		codes = append(codes, fmt.Sprintf("%d", SGRStrikethrough))
	}
	if s.FG.Type != ColorDefault {
		codes = append(codes, colorToSGR(s.FG, true))
	}
	if s.BG.Type != ColorDefault {
		codes = append(codes, colorToSGR(s.BG, false))
	}

	return codes
}

// colorToSGR returns the SGR code string for a color.
func colorToSGR(c Color, foreground bool) string {
	switch c.Type {
	case ColorDefault:
		if foreground {
			return fmt.Sprintf("%d", SGRFGDefault)
		}
		return fmt.Sprintf("%d", SGRBGDefault)
	case ColorBasic:
		if foreground {
			if c.Value < 8 {
				return fmt.Sprintf("%d", SGRFGBlack+int(c.Value))
			}
			return fmt.Sprintf("%d", SGRFGBrightBlack+int(c.Value-8))
		}
		if c.Value < 8 {
			return fmt.Sprintf("%d", SGRBGBlack+int(c.Value))
		}
		return fmt.Sprintf("%d", SGRBGBrightBlack+int(c.Value-8))
	case Color256:
		if foreground {
			return fmt.Sprintf("38;5;%d", c.Value)
		}
		return fmt.Sprintf("48;5;%d", c.Value)
	case ColorRGB:
		r := (c.Value >> 16) & 0xFF
		g := (c.Value >> 8) & 0xFF
		b := c.Value & 0xFF
		if foreground {
			return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
		}
		return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	}
	return ""
}
