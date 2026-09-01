package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Colors is the active palette. It is initialized with a default palette
// at package init and may be replaced by ApplyPalette after terminal
// color detection.
var Colors = defaultPalette()

type styles struct {
	Title         lipgloss.Style
	Label         lipgloss.Style
	Input         lipgloss.Style
	Button        lipgloss.Style
	ButtonPrimary lipgloss.Style
}

var Styles = buildStyles()

var borderStyle = lipgloss.NewStyle().Foreground(Colors.Border)

// rebuildStyles reconstructs all package-level style variables from the
// current Colors palette. Called by ApplyPalette.
func rebuildStyles() {
	Styles = buildStyles()
	borderStyle = lipgloss.NewStyle().Foreground(Colors.Border)
	rebuildStarfieldStyles()
	rebuildLogoStyles()
}

func buildStyles() styles {
	return styles{
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
}

func (s styles) Focus(base lipgloss.Style, focused bool) lipgloss.Style {
	if focused {
		return base.BorderForeground(Colors.Focused)
	}
	return base
}

func (s styles) WithError(base lipgloss.Style, hasError bool) lipgloss.Style {
	if hasError {
		return base.BorderForeground(Colors.Error)
	}
	return base
}

func (s styles) TitleRule(width int, crumbs ...string) string {
	label := " " + strings.Join(append([]string{"ONCE"}, crumbs...), " · ") + " "
	labelWidth := lipgloss.Width(label)
	ruleWidth := max(width-2, labelWidth) // subtract end caps
	side := (ruleWidth - labelWidth) / 2
	remainder := ruleWidth - labelWidth - side*2
	line := "╶" + strings.Repeat("─", side) + label + strings.Repeat("─", side+remainder) + "╴"
	return lipgloss.NewStyle().Foreground(Colors.Border).Render(line)
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

		bgLines[bgIdx] = left + ansi.ResetStyle + fgLine + ansi.ResetStyle + right
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

// Box drawing helpers for bordered panels used across chart, metric card,
// visits card, and disk gauge components.

func boxTop(title string, innerWidth int) string {
	titleLen := lipgloss.Width(title)
	fill := max(innerWidth-1-titleLen, 0) // 1 for dash before title
	return borderStyle.Render("╭─" + title + strings.Repeat("─", fill) + "╮")
}

func boxBottom(innerWidth int) string {
	return borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
}

func boxSide() string {
	return borderStyle.Render("│")
}

// distributeWidths divides total evenly among count items, distributing any
// remainder one pixel at a time to the first items.
func distributeWidths(total, count int) []int {
	if count <= 0 {
		return nil
	}
	base := total / count
	rem := total % count
	widths := make([]int, count)
	for i := range widths {
		widths[i] = base
		if i < rem {
			widths[i]++
		}
	}
	return widths
}

func padOrTruncate(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-w)
}

func formatValueLine(valueStr, limitLabel string, inner int) string {
	if limitLabel != "" {
		limitStr := lipgloss.NewStyle().Foreground(Colors.Border).Render("·" + limitLabel)
		gap := max(inner-lipgloss.Width(valueStr)-lipgloss.Width(limitStr)-1, 0)
		return valueStr + strings.Repeat(" ", gap) + limitStr + " "
	}
	return padOrTruncate(valueStr, inner)
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
