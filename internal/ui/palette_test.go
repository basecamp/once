package ui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPaletteHasANSICodes(t *testing.T) {
	p := defaultPalette()

	assert.Equal(t, color.Color(lipgloss.Red), p.Red)
	assert.Equal(t, color.Color(lipgloss.Green), p.Green)
	assert.Equal(t, color.Color(lipgloss.Blue), p.Blue)
	assert.Equal(t, color.Color(lipgloss.White), p.White)
	assert.Equal(t, color.Color(lipgloss.BrightBlack), p.BrightBlack)

	assert.Equal(t, color.Color(lipgloss.Red), p.FocusOrange)
	assert.Nil(t, p.BackgroundTint)
	assert.Equal(t, color.Color(lipgloss.BrightBlack), p.LightText)
	assert.Equal(t, color.Color(lipgloss.BrightBlue), p.Primary)
}

func TestDefaultPaletteSemanticAliases(t *testing.T) {
	p := defaultPalette()

	assert.Equal(t, p.LightText, p.Border)
	assert.Equal(t, p.LightText, p.Muted)
	assert.Equal(t, p.FocusOrange, p.Focused)
	assert.Equal(t, p.FocusOrange, p.Warning)
	assert.Equal(t, p.Red, p.Error)
	assert.Equal(t, p.Green, p.Success)
}

func TestDefaultPaletteNoNilColors(t *testing.T) {
	p := defaultPalette()

	allColors := []color.Color{
		p.Black, p.Red, p.Green, p.Yellow, p.Blue, p.Magenta, p.Cyan, p.White,
		p.BrightBlack, p.BrightRed, p.BrightGreen, p.BrightYellow,
		p.BrightBlue, p.BrightMagenta, p.BrightCyan, p.BrightWhite,
		p.FocusOrange, p.LightText,
		p.Border, p.Muted, p.Focused, p.Primary, p.Error, p.Success, p.Warning,
	}
	for i, c := range allColors {
		assert.NotNil(t, c, "color at index %d should not be nil", i)
	}
	assert.Nil(t, p.BackgroundTint)
}

func TestSynthesizeOrangeDarkTheme(t *testing.T) {
	// Tokyo Night blue
	blue, _ := colorful.Hex("#7AA2F7")
	orange := synthesizeOrange(blue)

	cf, ok := colorful.MakeColor(orange)
	require.True(t, ok)

	_, c, h := cf.OkLch()
	assert.InDelta(t, 55, h, 25, "hue should be in the orange range")
	assert.Greater(t, c, 0.08, "should have visible chroma")
	assert.True(t, cf.IsValid(), "should be a valid RGB color")
}

func TestSynthesizeOrangeLightTheme(t *testing.T) {
	// Light theme blue
	blue, _ := colorful.Hex("#2e7de9")
	orange := synthesizeOrange(blue)

	cf, ok := colorful.MakeColor(orange)
	require.True(t, ok)

	_, _, h := cf.OkLch()
	assert.InDelta(t, 55, h, 25, "hue should be in the orange range")
	assert.True(t, cf.IsValid(), "should be a valid RGB color")
}

func TestSynthesizeOrangeHueClamping(t *testing.T) {
	// Extreme blue that would produce a non-orange complement
	blue, _ := colorful.Hex("#0000ff")
	orange := synthesizeOrange(blue)

	cf, ok := colorful.MakeColor(orange)
	require.True(t, ok)

	// OKLCH→RGB clamping can shift hue by a few degrees
	_, _, h := cf.OkLch()
	assert.GreaterOrEqual(t, h, 30.0, "hue should be approximately clamped to orange range")
	assert.LessOrEqual(t, h, 80.0, "hue should be approximately clamped to orange range")
}

func TestSynthesizeTintDarkBackground(t *testing.T) {
	bg, _ := colorful.Hex("#1a1b26")
	tint := synthesizeTint(bg)

	bgCf, _ := colorful.MakeColor(bg)
	tintCf, ok := colorful.MakeColor(tint)
	require.True(t, ok)

	bgL, _, _ := bgCf.OkLch()
	tintL, _, _ := tintCf.OkLch()

	assert.Less(t, tintL, bgL, "tint should be darker than background")
	assert.InDelta(t, bgL-0.015, tintL, 0.01)
}

func TestSynthesizeTintLightBackground(t *testing.T) {
	bg, _ := colorful.Hex("#d5d6db")
	tint := synthesizeTint(bg)

	bgCf, _ := colorful.MakeColor(bg)
	tintCf, ok := colorful.MakeColor(tint)
	require.True(t, ok)

	bgL, _, _ := bgCf.OkLch()
	tintL, _, _ := tintCf.OkLch()

	assert.Less(t, tintL, bgL, "tint should be darker than background")
}

func TestSynthesizeLightTextDarkTheme(t *testing.T) {
	bg, _ := colorful.Hex("#1a1b26")
	fg, _ := colorful.Hex("#c0caf5")
	blue, _ := colorful.Hex("#7AA2F7")

	lt := synthesizeLightText(bg, fg, blue)

	cf, ok := colorful.MakeColor(lt)
	require.True(t, ok)

	l, c, _ := cf.OkLch()
	assert.Greater(t, l, 0.3, "should be visible on dark bg")
	assert.Less(t, l, 0.55, "should be subdued, not bright")
	assert.Less(t, c, 0.05, "should be low chroma")
	assert.Greater(t, c, 0.005, "should have slight blue tint")
}

func TestSynthesizeLightTextLightTheme(t *testing.T) {
	bg, _ := colorful.Hex("#d5d6db")
	fg, _ := colorful.Hex("#343b58")
	blue, _ := colorful.Hex("#2e7de9")

	lt := synthesizeLightText(bg, fg, blue)

	cf, ok := colorful.MakeColor(lt)
	require.True(t, ok)

	l, c, _ := cf.OkLch()
	assert.Less(t, l, 0.80, "should be darker than light bg")
	assert.Greater(t, l, 0.5, "should still be readable")
	assert.Less(t, c, 0.05, "should be low chroma")
	assert.Greater(t, c, 0.005, "should have slight blue tint")
}

func TestApplyNoDetection(t *testing.T) {
	t.Setenv("COLORTERM", "")

	p := defaultPalette()
	p.apply(detectedColors{}, false)

	assert.False(t, p.SupportsTrueColor())
	assert.Equal(t, color.Color(lipgloss.Red), p.FocusOrange)
	assert.Nil(t, p.BackgroundTint)
	assert.Equal(t, color.Color(lipgloss.BrightBlack), p.LightText)
	assert.Equal(t, color.Color(lipgloss.BrightBlue), p.Primary)
}

func TestApplyCompleteDetection(t *testing.T) {
	t.Setenv("COLORTERM", "")

	p := defaultPalette()
	colors, ok := detectFrom(newMockTTY([]byte(fullDarkResponse())))
	require.True(t, ok)

	p.apply(colors, true)

	assert.True(t, p.SupportsTrueColor())
	assert.True(t, p.isDark)

	// Synthesized colors should be true-color RGB, not ANSI
	orangeCf, ok := colorful.MakeColor(p.FocusOrange)
	require.True(t, ok)
	_, _, h := orangeCf.OkLch()
	assert.GreaterOrEqual(t, h, 30.0)
	assert.LessOrEqual(t, h, 80.0)

	assert.NotNil(t, p.BackgroundTint)
	assert.NotNil(t, p.LightText)
}

func TestApplyCOLORTERMOverride(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")

	p := defaultPalette()
	p.apply(detectedColors{}, false)

	assert.True(t, p.SupportsTrueColor())

	// Should synthesize from default samples
	assert.NotNil(t, p.FocusOrange)
	assert.NotNil(t, p.BackgroundTint)
	assert.NotNil(t, p.LightText)

	// FocusOrange should be RGB, not the ANSI fallback
	_, isBasic := p.FocusOrange.(ansi.BasicColor)
	assert.False(t, isBasic, "FocusOrange should be RGB when truecolor is supported")
}

func TestApplyCOLORTERM24Bit(t *testing.T) {
	t.Setenv("COLORTERM", "24bit")

	p := defaultPalette()
	p.apply(detectedColors{}, false)

	assert.True(t, p.SupportsTrueColor())
}

func TestApplyLightTheme(t *testing.T) {
	t.Setenv("COLORTERM", "")

	p := defaultPalette()
	colors, ok := detectFrom(newMockTTY([]byte(fullLightResponse())))
	require.True(t, ok)

	p.apply(colors, true)

	assert.True(t, p.SupportsTrueColor())
	assert.False(t, p.isDark)
}

func TestApplyNoNilColors(t *testing.T) {
	t.Setenv("COLORTERM", "")

	checkAllColors := func(t *testing.T, p *Palette) {
		t.Helper()
		allColors := []color.Color{
			p.Black, p.Red, p.Green, p.Yellow, p.Blue, p.Magenta, p.Cyan, p.White,
			p.BrightBlack, p.BrightRed, p.BrightGreen, p.BrightYellow,
			p.BrightBlue, p.BrightMagenta, p.BrightCyan, p.BrightWhite,
			p.FocusOrange, p.BackgroundTint, p.LightText,
			p.Border, p.Muted, p.Focused, p.Primary, p.Error, p.Success, p.Warning,
		}
		for i, c := range allColors {
			assert.NotNil(t, c, "color at index %d should not be nil", i)
		}
	}

	t.Run("no detection", func(t *testing.T) {
		p := defaultPalette()
		p.apply(detectedColors{}, false)
		assert.Nil(t, p.BackgroundTint)
	})

	t.Run("complete detection", func(t *testing.T) {
		p := defaultPalette()
		colors, ok := detectFrom(newMockTTY([]byte(fullDarkResponse())))
		require.True(t, ok)
		p.apply(colors, true)
		checkAllColors(t, p)
	})

	t.Run("COLORTERM override", func(t *testing.T) {
		t.Setenv("COLORTERM", "truecolor")
		p := defaultPalette()
		p.apply(detectedColors{}, false)
		checkAllColors(t, p)
	})
}

func TestGradientTrueColor(t *testing.T) {
	t.Setenv("COLORTERM", "")

	p := defaultPalette()
	colors, ok := detectFrom(newMockTTY([]byte(fullDarkResponse())))
	require.True(t, ok)
	p.apply(colors, true)

	// t=0 should be close to green sample
	g0, ok := colorful.MakeColor(p.Gradient(0))
	require.True(t, ok)
	assert.True(t, g0.IsValid())

	// t=1 should be close to orange
	g1, ok := colorful.MakeColor(p.Gradient(1))
	require.True(t, ok)
	orangeSample, _ := colorful.MakeColor(p.FocusOrange)
	assert.InDelta(t, orangeSample.R, g1.R, 0.01)

	// Midpoint should be valid
	mid, ok := colorful.MakeColor(p.Gradient(0.5))
	require.True(t, ok)
	assert.True(t, mid.IsValid())
}

func TestGradientANSIFallback(t *testing.T) {
	t.Setenv("COLORTERM", "")

	p := defaultPalette()
	p.apply(detectedColors{}, false)

	assert.Equal(t, p.Green, p.Gradient(0))
	assert.Equal(t, p.Green, p.Gradient(0.1))
	assert.Equal(t, p.Yellow, p.Gradient(0.5))
	assert.Equal(t, p.Red, p.Gradient(0.9))
	assert.Equal(t, p.Red, p.Gradient(1.0))
}

func TestANSISlotsNeverEmitTruecolor(t *testing.T) {
	p := defaultPalette()

	// All 16 ANSI slots should be BasicColor, not RGB
	ansiColors := []color.Color{
		p.Black, p.Red, p.Green, p.Yellow, p.Blue, p.Magenta, p.Cyan, p.White,
		p.BrightBlack, p.BrightRed, p.BrightGreen, p.BrightYellow,
		p.BrightBlue, p.BrightMagenta, p.BrightCyan, p.BrightWhite,
	}

	for i, c := range ansiColors {
		_, isBasic := c.(ansi.BasicColor)
		assert.True(t, isBasic, "ANSI color %d should be BasicColor, got %T", i, c)
	}
}

func TestApplyPaletteRebuildStyles(t *testing.T) {
	original := Colors
	defer func() { ApplyPalette(original) }()

	p := defaultPalette()
	ApplyPalette(p)

	assert.Equal(t, p, Colors)
}

func TestPickPrimaryPrefersBrightBlue(t *testing.T) {
	p := defaultPalette()
	assert.Equal(t, color.Color(lipgloss.BrightBlue), p.Primary)
}

func TestPickPrimaryWithDetection(t *testing.T) {
	var samples [sampleCount]colorful.Color

	// Dark bg, dim Blue, bright BrightBlue
	samples[sampleBackground], _ = colorful.Hex("#1a1b26")
	samples[int(ansi.Blue)], _ = colorful.Hex("#2222aa")       // dim blue
	samples[int(ansi.BrightBlue)], _ = colorful.Hex("#7dcfff") // bright blue

	// BrightBlue has better contrast against dark bg
	assert.Equal(t, color.Color(lipgloss.BrightBlue), pickPrimary(samples))
}

func TestHealthStateColors(t *testing.T) {
	p := defaultPalette()

	assert.Equal(t, p.Success, healthNormal.Color())
	assert.Equal(t, p.Warning, healthWarning.Color())
	assert.Equal(t, p.Error, healthError.Color())
}
