// Package gliff provides efficient terminal rendering with ncurses-like optimizations.
package renderer

// ANSI escape sequences used by this library.
// All sequences assume a modern terminal with full ANSI support.
const (
	// Escape sequence introducer
	ESC = "\x1b"
	CSI = ESC + "["

	// Screen modes
	AltScreenEnable  = CSI + "?1049h"
	AltScreenDisable = CSI + "?1049l"

	// Cursor visibility
	CursorHide = CSI + "?25l"
	CursorShow = CSI + "?25h"

	// Cursor movement
	CursorHome     = CSI + "H"      // Move to (1,1)
	CursorUp       = CSI + "%dA"    // Move up n rows
	CursorDown     = CSI + "%dB"    // Move down n rows
	CursorForward  = CSI + "%dC"    // Move right n columns
	CursorBackward = CSI + "%dD"    // Move left n columns
	CursorPosition = CSI + "%d;%dH" // Move to (row, col) - 1-indexed
	CursorColumn   = CSI + "%dG"    // Move to column n - 1-indexed
	CursorSave     = CSI + "s"      // Save cursor position
	CursorRestore  = CSI + "u"      // Restore cursor position

	// Scrolling
	ScrollUp   = CSI + "%dS" // Scroll up n lines (content moves up, blank at bottom)
	ScrollDown = CSI + "%dT" // Scroll down n lines (content moves down, blank at top)

	// Scroll region (set top and bottom margins)
	ScrollRegionSet   = CSI + "%d;%dr" // Set scroll region (top, bottom) - 1-indexed
	ScrollRegionReset = CSI + "r"      // Reset scroll region to full screen

	// Erasing
	EraseScreen      = CSI + "2J" // Erase entire screen
	EraseScreenBelow = CSI + "J"  // Erase from cursor to end of screen
	EraseScreenAbove = CSI + "1J" // Erase from start of screen to cursor
	EraseLine        = CSI + "2K" // Erase entire line
	EraseLineRight   = CSI + "K"  // Erase from cursor to end of line (default)
	EraseLineLeft    = CSI + "1K" // Erase from start of line to cursor

	// SGR (Select Graphic Rendition) - text attributes
	SGRReset = CSI + "0m" // Reset all attributes

	// SGR attribute codes (used with fmt.Sprintf(CSI+"%dm", code))
	SGRBold          = 1
	SGRDim           = 2
	SGRItalic        = 3
	SGRUnderline     = 4
	SGRBlink         = 5
	SGRReverse       = 7
	SGRHidden        = 8
	SGRStrikethrough = 9

	// SGR attribute off codes
	SGRBoldOff          = 22 // Also turns off dim
	SGRDimOff           = 22
	SGRItalicOff        = 23
	SGRUnderlineOff     = 24
	SGRBlinkOff         = 25
	SGRReverseOff       = 27
	SGRHiddenOff        = 28
	SGRStrikethroughOff = 29

	// SGR foreground colors (basic 8)
	SGRFGBlack   = 30
	SGRFGRed     = 31
	SGRFGGreen   = 32
	SGRFGYellow  = 33
	SGRFGBlue    = 34
	SGRFGMagenta = 35
	SGRFGCyan    = 36
	SGRFGWhite   = 37
	SGRFGDefault = 39

	// SGR background colors (basic 8)
	SGRBGBlack   = 40
	SGRBGRed     = 41
	SGRBGGreen   = 42
	SGRBGYellow  = 43
	SGRBGBlue    = 44
	SGRBGMagenta = 45
	SGRBGCyan    = 46
	SGRBGWhite   = 47
	SGRBGDefault = 49

	// SGR bright foreground colors
	SGRFGBrightBlack   = 90
	SGRFGBrightRed     = 91
	SGRFGBrightGreen   = 92
	SGRFGBrightYellow  = 93
	SGRFGBrightBlue    = 94
	SGRFGBrightMagenta = 95
	SGRFGBrightCyan    = 96
	SGRFGBrightWhite   = 97

	// SGR bright background colors
	SGRBGBrightBlack   = 100
	SGRBGBrightRed     = 101
	SGRBGBrightGreen   = 102
	SGRBGBrightYellow  = 103
	SGRBGBrightBlue    = 104
	SGRBGBrightMagenta = 105
	SGRBGBrightCyan    = 106
	SGRBGBrightWhite   = 107

	// 256-color and RGB color format strings
	// Usage: fmt.Sprintf(SGRFG256, colorIndex) or fmt.Sprintf(SGRFGRGB, r, g, b)
	SGRFG256 = CSI + "38;5;%dm"
	SGRBG256 = CSI + "48;5;%dm"
	SGRFGRGB = CSI + "38;2;%d;%d;%dm"
	SGRBGRGB = CSI + "48;2;%d;%d;%dm"

	// Mouse tracking (SGR extended mode)
	MouseTrackingEnable  = CSI + "?1000h"
	MouseTrackingDisable = CSI + "?1000l"
	MouseSGREnable       = CSI + "?1006h"
	MouseSGRDisable      = CSI + "?1006l"
)
