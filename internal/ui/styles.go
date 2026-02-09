package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
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
}

var Colors = colors{
	Primary:    lipgloss.Color("#FF69B4"),
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
}

type styles struct {
	Title         lipgloss.Style
	SubTitle      lipgloss.Style
	Label         lipgloss.Style
	Input         lipgloss.Style
	Button        lipgloss.Style
	ButtonPrimary lipgloss.Style
}

var Styles = styles{
	Title: lipgloss.NewStyle().
		Foreground(Colors.Primary).
		Bold(true),
	SubTitle: lipgloss.NewStyle().
		Foreground(Colors.Secondary).
		Underline(true),
	Label: lipgloss.NewStyle().
		Bold(true),
	Input: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(0, 1).
		MarginBottom(1),
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

func (s styles) TitleBox(width int, title string, extra ...string) string {
	innerWidth := width - 2
	titleLine := lipgloss.Place(innerWidth, 1, lipgloss.Center, lipgloss.Center,
		s.Title.Render(title))
	lines := []string{titleLine}
	if len(extra) > 0 {
		lines = append(lines, "")
		lines = append(lines, extra...)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Width(width).
		Render(content)
}

func (s styles) HelpLine(width int, content string) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}

func (s styles) CenteredLine(width int, content string) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(content)
}

func CenteredLayer(content string, containerWidth, containerHeight int) *lipgloss.Layer {
	x := (containerWidth - lipgloss.Width(content)) / 2
	y := (containerHeight - lipgloss.Height(content)) / 2
	return lipgloss.NewLayer(content).X(x).Y(y)
}

