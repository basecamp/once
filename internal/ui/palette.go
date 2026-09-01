package ui

import (
	"image/color"
	"log/slog"
	"math"
	"os"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

// Palette holds all colors used by the UI. ANSI color fields always contain
// BasicColor values so the terminal applies its own theme. The synthesized
// colors (FocusOrange, BackgroundTint, LightText) are true-color RGB when
// the terminal supports it, or ANSI fallbacks otherwise.
type Palette struct {
	// ANSI 16 — always BasicColor values for rendering
	Black, Red, Green, Yellow, Blue, Magenta, Cyan, White                                                 color.Color
	BrightBlack, BrightRed, BrightGreen, BrightYellow, BrightBlue, BrightMagenta, BrightCyan, BrightWhite color.Color

	// Synthesized (true-color RGB when supported, ANSI fallbacks otherwise)
	FocusOrange    color.Color
	BackgroundTint color.Color
	LightText      color.Color

	// Semantic aliases
	Border  color.Color // = LightText
	Muted   color.Color // = LightText
	Focused color.Color // = FocusOrange
	Primary color.Color // = Blue or BrightBlue (better contrast)
	Error   color.Color // = Red
	Success color.Color // = Green
	Warning color.Color // = FocusOrange

	isDark         bool
	trueColor      bool
	gradientGreen  colorful.Color
	gradientOrange colorful.Color
}

// Gradient interpolates between green and FocusOrange in OKLCH.
// t=0 returns green, t=1 returns orange. When true color is not
// supported, the gradient is clamped to ANSI green/yellow/red.
func (p *Palette) Gradient(t float64) color.Color {
	t = max(0, min(1, t))
	if !p.trueColor {
		switch {
		case t < 0.33:
			return p.Green
		case t < 0.67:
			return p.Yellow
		default:
			return p.Red
		}
	}
	return p.gradientGreen.BlendOkLch(p.gradientOrange, t)
}

// HealthColor returns the palette color for the given health state.
func (p *Palette) HealthColor(h HealthState) color.Color {
	switch h {
	case healthWarning:
		return p.Warning
	case healthError:
		return p.Error
	default:
		return p.Success
	}
}

// SupportsTrueColor reports whether the terminal is likely to support
// 24-bit color output.
func (p *Palette) SupportsTrueColor() bool {
	return p.trueColor
}

// Detect queries the terminal for colors and updates the palette
// accordingly. If detection succeeds (all 18 colors received),
// synthesized colors are computed from the detected RGB values.
// If COLORTERM indicates true-color support, synthesized colors
// are computed from fallback samples. Otherwise the ANSI defaults
// from defaultPalette are kept.
func (p *Palette) Detect(timeout time.Duration) {
	colors, ok := DetectTerminalColors(timeout)
	p.apply(colors, ok)

	if !p.SupportsTrueColor() {
		slog.Info("True color output is not enabled")
	}
}

// defaultPalette returns a palette with ANSI BasicColor values for all
// color fields. This is the starting point; Detect may upgrade the
// synthesized colors to true-color RGB.
func defaultPalette() *Palette {
	p := &Palette{
		Black:         lipgloss.Black,
		Red:           lipgloss.Red,
		Green:         lipgloss.Green,
		Yellow:        lipgloss.Yellow,
		Blue:          lipgloss.Blue,
		Magenta:       lipgloss.Magenta,
		Cyan:          lipgloss.Cyan,
		White:         lipgloss.White,
		BrightBlack:   lipgloss.BrightBlack,
		BrightRed:     lipgloss.BrightRed,
		BrightGreen:   lipgloss.BrightGreen,
		BrightYellow:  lipgloss.BrightYellow,
		BrightBlue:    lipgloss.BrightBlue,
		BrightMagenta: lipgloss.BrightMagenta,
		BrightCyan:    lipgloss.BrightCyan,
		BrightWhite:   lipgloss.BrightWhite,

		FocusOrange: lipgloss.Red,
		LightText:   lipgloss.BrightBlack,
		Primary:     lipgloss.BrightBlue,

		isDark: true,
	}
	p.setAliases()
	return p
}

// ApplyPalette sets the package-level Colors variable and rebuilds
// all package-level style variables that depend on colors.
func ApplyPalette(p *Palette) {
	Colors = p
	rebuildStyles()
}

// Private

func (p *Palette) apply(colors detectedColors, ok bool) {
	colorterm := os.Getenv("COLORTERM")
	p.trueColor = ok || colorterm == "truecolor" || colorterm == "24bit"

	if !p.trueColor {
		return
	}

	var samples [sampleCount]colorful.Color
	if ok {
		samples = colors.Colors
		l, _, _ := samples[sampleBackground].OkLch()
		p.isDark = l < 0.5
		p.Primary = pickPrimary(samples)
	} else {
		samples = defaultSamples()
	}

	p.FocusOrange = synthesizeOrange(samples[int(ansi.Blue)])
	p.BackgroundTint = synthesizeTint(samples[sampleBackground])
	p.LightText = synthesizeLightText(
		samples[sampleBackground],
		samples[sampleForeground],
		samples[int(ansi.Blue)],
	)

	p.gradientGreen = samples[int(ansi.Green)]
	p.gradientOrange, _ = colorful.MakeColor(p.FocusOrange)

	p.setAliases()
}

func (p *Palette) setAliases() {
	p.Border = p.LightText
	p.Muted = p.LightText
	p.Focused = p.FocusOrange
	p.Error = p.Red
	p.Success = p.Green
	p.Warning = p.FocusOrange
}

// synthesizeOrange produces a warm orange as the OKLCH complement of blue,
// with hue clamped to the 35°–75° range.
func synthesizeOrange(blue colorful.Color) color.Color {
	l, c, h := blue.OkLch()

	// Complement: rotate 180°
	h = math.Mod(h+180, 360)

	// Clamp hue to orange band
	h = max(35, min(75, h))

	// Ensure usable chroma and lightness
	c = max(c, 0.10)
	l = max(0.55, min(0.85, l))

	return colorful.OkLch(l, c, h).Clamped()
}

// synthesizeTint darkens the background by an absolute OKLCH lightness delta.
func synthesizeTint(bg colorful.Color) color.Color {
	l, c, h := bg.OkLch()
	l = max(l-0.015, 0)
	return colorful.OkLch(l, c, h).Clamped()
}

// synthesizeLightText produces a subdued blue-grey for secondary text.
// It blends 35% from background toward foreground in lightness,
// with a touch of chroma on the blue axis.
func synthesizeLightText(bg, fg, blue colorful.Color) color.Color {
	bgL, _, _ := bg.OkLch()
	fgL, _, _ := fg.OkLch()
	_, blueC, blueH := blue.OkLch()

	l := bgL + 0.35*(fgL-bgL)
	c := min(blueC*0.15, 0.04)
	return colorful.OkLch(l, c, blueH).Clamped()
}

// pickPrimary chooses the better of Blue and BrightBlue for contrast
// against the background.
func pickPrimary(samples [sampleCount]colorful.Color) color.Color {
	bgL, _, _ := samples[sampleBackground].OkLch()
	blueL, _, _ := samples[int(ansi.Blue)].OkLch()
	brightL, _, _ := samples[int(ansi.BrightBlue)].OkLch()

	if math.Abs(brightL-bgL) >= math.Abs(blueL-bgL) {
		return lipgloss.BrightBlue
	}
	return lipgloss.Blue
}

// defaultSamples returns fallback RGB values for OKLCH calculations.
// These are never emitted to the terminal.
func defaultSamples() [sampleCount]colorful.Color {
	hex := func(s string) colorful.Color {
		c, _ := colorful.Hex(s)
		return c
	}

	var s [sampleCount]colorful.Color
	// Standard dark-theme defaults (xterm-like)
	s[int(ansi.Black)] = hex("#000000")
	s[int(ansi.Red)] = hex("#cc0000")
	s[int(ansi.Green)] = hex("#50fa7b")
	s[int(ansi.Yellow)] = hex("#cdcd00")
	s[int(ansi.Blue)] = hex("#7AA2F7")
	s[int(ansi.Magenta)] = hex("#cd00cd")
	s[int(ansi.Cyan)] = hex("#00cdcd")
	s[int(ansi.White)] = hex("#e5e5e5")
	s[int(ansi.BrightBlack)] = hex("#7f7f7f")
	s[int(ansi.BrightRed)] = hex("#ff0000")
	s[int(ansi.BrightGreen)] = hex("#00ff00")
	s[int(ansi.BrightYellow)] = hex("#ffff00")
	s[int(ansi.BrightBlue)] = hex("#5c5cff")
	s[int(ansi.BrightMagenta)] = hex("#ff00ff")
	s[int(ansi.BrightCyan)] = hex("#00ffff")
	s[int(ansi.BrightWhite)] = hex("#ffffff")
	s[sampleForeground] = hex("#c0caf5")
	s[sampleBackground] = hex("#1a1b26")
	return s
}
