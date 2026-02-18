package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
)

type colors struct {
	Primary    color.Color
	Secondary  color.Color
	Background color.Color
	Text       color.Color
	TextDark   color.Color
	Focused    color.Color
	Border     color.Color
	Success    color.Color
	Warning    color.Color
	Error      color.Color
	Info       color.Color
	Muted      color.Color
	PanelBg    color.Color
}

var Colors = colors{
	Primary:    lipgloss.Color("#7AA2F7"),
	Secondary:  lipgloss.Color("#9f3"),
	Background: lipgloss.Color("#000000"),
	Text:       lipgloss.Color("#FFFFFF"),
	TextDark:   lipgloss.Color("#000000"),
	Focused:    lipgloss.Color("#FFA500"),
	Border:     lipgloss.Color("#6272a4"),
	Success:    lipgloss.Color("#50fa7b"),
	Warning:    lipgloss.Color("#f1fa8c"),
	Error:      lipgloss.Color("#ff5555"),
	Info:       lipgloss.Color("#8be9fd"),
	Muted:      lipgloss.Color("#bd93f9"),
	PanelBg: compat.AdaptiveColor{
		Light: lipgloss.Color("#e8e8e8"),
		Dark:  lipgloss.Color("#1a1b26"),
	},
}

type styles struct {
	Title         lipgloss.Style
	Label         lipgloss.Style
	Input         lipgloss.Style
	Button        lipgloss.Style
	ButtonPrimary lipgloss.Style
}

var Styles = styles{
	Title: lipgloss.NewStyle().
		Foreground(Colors.Primary).
		Bold(true),
	Label: lipgloss.NewStyle().
		Bold(true),
	Input: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(0, 1),
	Button: lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border),
	ButtonPrimary: lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Primary),
}

func (s styles) Focus(base lipgloss.Style, focused bool) lipgloss.Style {
	if focused {
		return base.BorderForeground(Colors.Focused)
	}
	return base
}

func (s styles) TitleRule(width int, crumbs ...string) string {
	label := " " + strings.Join(append([]string{"ONCE"}, crumbs...), " · ") + " "
	ruleWidth := max(width-2, len(label)) // subtract end caps
	side := (ruleWidth - len(label)) / 2
	remainder := ruleWidth - len(label) - side*2
	line := "╶" + strings.Repeat("─", side) + label + strings.Repeat("─", side+remainder) + "╴"
	return lipgloss.NewStyle().Foreground(Colors.Border).Render(line)
}

func (s styles) HelpLine(width int, content string) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}

func (s styles) CenteredLine(width int, content string) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}

// OverlayCenter composites fg centered on top of bg within the given dimensions.
func OverlayCenter(bg, fg string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for len(bgLines) < height {
		bgLines = append(bgLines, strings.Repeat(" ", width))
	}

	fgHeight := len(fgLines)
	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}

	topOffset := (height - fgHeight) / 2
	leftOffset := (width - fgWidth) / 2

	for i, fgLine := range fgLines {
		bgIdx := topOffset + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}

		bgLine := bgLines[bgIdx]

		left := ansi.Truncate(bgLine, leftOffset, "")
		if w := ansi.StringWidth(left); w < leftOffset {
			left += strings.Repeat(" ", leftOffset-w)
		}

		if w := ansi.StringWidth(fgLine); w < fgWidth {
			fgLine += strings.Repeat(" ", fgWidth-w)
		}

		right := ansi.TruncateLeft(bgLine, leftOffset+fgWidth, "")

		bgLines[bgIdx] = left + "\x1b[0m" + fgLine + "\x1b[0m" + right
	}

	return strings.Join(bgLines, "\n")
}

// WithBackground re-applies a background color after any SGR reset sequences
// within the content, so that inner styled elements don't clear the outer
// background. Resets with no visible content following on the same line are
// left alone, preventing the background from bleeding past the panel edge.
func WithBackground(bg color.Color, content string) string {
	bgSeq := ansi.NewStyle().BackgroundColor(bg).String()
	p := ansi.NewParser()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = applyBackgroundToLine(line, bgSeq, p)
	}
	return strings.Join(lines, "\n")
}

// Helpers

func applyBackgroundToLine(line, bgSeq string, p *ansi.Parser) string {
	var result strings.Builder
	remaining := line
	var state byte
	for len(remaining) > 0 {
		seq, _, n, newState := ansi.DecodeSequence(remaining, state, p)
		state = newState
		result.WriteString(seq)
		if isSGRReset(seq, p) && hasVisibleContent(remaining[n:]) {
			result.WriteString(bgSeq)
		}
		remaining = remaining[n:]
	}
	return result.String()
}

func isSGRReset(seq string, p *ansi.Parser) bool {
	if !ansi.HasCsiPrefix(seq) {
		return false
	}

	cmd := ansi.Cmd(p.Command())
	if cmd.Final() != 'm' || cmd.Prefix() != 0 || cmd.Intermediate() != 0 {
		return false
	}

	params := p.Params()
	return len(params) == 0 || (len(params) == 1 && params[0].Param(-1) == 0)
}

func hasVisibleContent(s string) bool {
	var state byte
	remaining := s
	for len(remaining) > 0 {
		_, width, n, newState := ansi.DecodeSequence(remaining, state, nil)
		state = newState
		if width > 0 {
			return true
		}
		remaining = remaining[n:]
	}
	return false
}
