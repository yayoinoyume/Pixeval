package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSystemThemeFollowsDark checks the system setting takes the desktop's
// appearance instead of keeping whichever theme the app last drew.
func TestSystemThemeFollowsDark(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.Theme = themeSystem
	tt := ui.NewTester(a.View, 400, 300)
	tt.SetDark(true)
	tt.Frame()
	checkBackground(t, tt, darkTokens.Bg)
	tt.SetDark(false)
	tt.Frame()
	checkBackground(t, tt, lightTokens.Bg)
}

// checkBackground reads a pixel in the header, away from the navigation and
// the title, and compares it with the theme's background.
func checkBackground(t *testing.T, tt *ui.Tester, want ui.Color) {
	t.Helper()
	img := tt.Image()
	r, g, b, _ := img.At(img.Bounds().Dx()*3/4, 10).RGBA()
	got := ui.RGB(uint8(r>>8), uint8(g>>8), uint8(b>>8))
	if got.R != want.R || got.G != want.G || got.B != want.B {
		t.Errorf("background %v, want %v", got, want)
	}
}

// TestDesignThemeScalesText checks the desktop's text scale reaches the
// widgets' font size.
func TestDesignThemeScalesText(t *testing.T) {
	if got, want := designTheme(true, 1.5).FontSize, darkTokens.TextBody*1.5; got != want {
		t.Errorf("font size %v, want %v", got, want)
	}
}

// TestSystemAccentTextContrast checks a light system accent gets near black
// text, as mygo's own theme picks it, and that the custom tokens take the
// same accent.
func TestSystemAccentTextContrast(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.Theme = themeSystem
	var text ui.Color
	var tok tokens
	tt := ui.NewTester(func(c *ui.Context) {
		a.View(c)
		text = c.Theme().AccentText
		tok = themeOf(c)
	}, 400, 300)
	tt.SetPreferences(ui.Preferences{Accent: ui.Hex("#ffd400")})
	tt.Frame()
	if text.R != 24 || text.G != 24 || text.B != 27 {
		t.Errorf("accent text %v, want near black", text)
	}
	if tok.AccentFill.R != 255 || tok.AccentFill.G != 212 || tok.AccentFill.B != 0 {
		t.Errorf("token fill %v, want the raw system accent", tok.AccentFill)
	}
	if got := contrastRatio(tok.Accent, lightTokens.Surface); got < 4.5 {
		t.Errorf("foreground accent contrast %v, want at least 4.5", got)
	}
	if tok.AccentText.R != 24 || tok.AccentText.G != 24 || tok.AccentText.B != 27 {
		t.Errorf("token accent text %v, want near black", tok.AccentText)
	}
	tt.SetPreferences(ui.Preferences{Accent: ui.Hex("#1f6feb")})
	tt.Frame()
	if text.R != 255 || text.G != 255 || text.B != 255 {
		t.Errorf("accent text %v, want white on a dark accent", text)
	}
}

// TestThemeTokensFollowTextScale checks the hand-built text tokens scale with
// the desktop, not just the widgets' FontSize.
func TestThemeTokensFollowTextScale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	var tok tokens
	tt := ui.NewTester(func(c *ui.Context) {
		a.View(c)
		tok = themeOf(c)
	}, 400, 300)
	tt.SetPreferences(ui.Preferences{TextScale: 1.25})
	tt.Frame()
	if want := darkTokens.TextBody * 1.25; tok.TextBody != want {
		t.Errorf("body text %v, want %v", tok.TextBody, want)
	}
}

// TestCardTextHeightScales checks the masonry leaves more room for a card's
// text when the desktop scales text, so the card body is not clipped.
func TestCardTextHeightScales(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	var tok tokens
	tt := ui.NewTester(func(c *ui.Context) {
		a.View(c)
		tok = themeOf(c)
	}, 400, 300)
	tt.SetPreferences(ui.Preferences{TextScale: 2})
	tt.Frame()
	if got, base := cardTextHeight(tok), cardTextHeight(lightTokens); got <= base {
		t.Errorf("card text height %v at scale 2, want more than %v", got, base)
	}
}

// sameColor reports whether two colors show the same pixel.
func sameColor(a, b ui.Color) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B && a.A == b.A
}

// TestMaterialPalette checks the light and the dark set are the Material You
// roles of the design, from the one seed, and that the older names are those
// roles.
func TestMaterialPalette(t *testing.T) {
	light := lightTokens
	roles := []struct {
		name string
		got  ui.Color
		want ui.Color
	}{
		{"surface", light.Bg, ui.Hex("#fdf7ff")},
		{"surface-container-low", light.Surface, ui.Hex("#f7f0fa")},
		{"surface-container", light.SurfaceAlt, ui.Hex("#f1eaf4")},
		{"surface-container-high", light.SurfaceHover, ui.Hex("#ebe4ee")},
		{"surface-container-highest", light.SurfacePressed, ui.Hex("#e5dee9")},
		{"on-surface", light.Text, ui.Hex("#1c1a1f")},
		{"on-surface-variant", light.TextMuted, ui.Hex("#4a4550")},
		{"outline-variant", light.Border, ui.Hex("#ccc4d0")},
		{"primary", light.Accent, ui.Hex("#6b4a86")},
		{"primary-container", light.AccentSoft, ui.Hex("#eedbff")},
		{"on-primary", light.AccentText, ui.Hex("#ffffff")},
		{"error", light.Danger, ui.Hex("#ba1a1a")},
		{"inverse-surface", light.Inverse, ui.Hex("#322f35")},
		{"inverse-on-surface", light.InverseText, ui.Hex("#f5eef7")},
	}
	for _, r := range roles {
		if !sameColor(r.got, r.want) {
			t.Errorf("%s %v, want %v", r.name, r.got, r.want)
		}
	}
	if !sameColor(light.Primary, light.Accent) || !sameColor(light.SurfaceContainerLow, light.Surface) ||
		!sameColor(light.OutlineVariant, light.Border) || !sameColor(light.InverseOnSurface, light.InverseText) {
		t.Error("the Material roles are not the older names")
	}

	dark := darkTokens
	if !dark.Dark || !sameColor(dark.Bg, ui.Hex("#151217")) || !sameColor(dark.Accent, ui.Hex("#dcb8ff")) {
		t.Errorf("dark set %v / %v, want the dark roles from the same seed", dark.Bg, dark.Accent)
	}
}

// TestMaterialContrast checks text stays readable in both sets: body text and
// the filled primary at 4.5:1, secondary text and the tonal pairs at 3:1.
func TestMaterialContrast(t *testing.T) {
	for _, tt := range []struct {
		name string
		tok  tokens
	}{{"light", lightTokens}, {"dark", darkTokens}} {
		tok := tt.tok
		checks := []struct {
			name string
			fg   ui.Color
			bg   ui.Color
			min  float32
		}{
			{"body text", tok.Text, tok.Bg, 4.5},
			{"body text on a card", tok.Text, tok.SurfaceContainerLow, 4.5},
			{"secondary text", tok.TextMuted, tok.SurfaceContainerLow, 3},
			{"primary text", tok.Accent, tok.SurfaceContainerLow, 4.5},
			{"text on the primary fill", tok.AccentText, tok.AccentFill, 4.5},
			{"text on the primary container", tok.OnPrimaryContainer, tok.PrimaryContainer, 4.5},
			{"text on the secondary container", tok.OnSecondaryContainer, tok.SecondaryContainer, 4.5},
			{"text on the tertiary container", tok.OnTertiaryContainer, tok.TertiaryContainer, 4.5},
			{"error text", tok.Danger, tok.SurfaceContainerLow, 4.5},
		}
		for _, c := range checks {
			if got := contrastRatio(c.fg, c.bg); got < c.min {
				t.Errorf("%s %s contrast %v, want at least %v", tt.name, c.name, got, c.min)
			}
		}
	}
}

// TestDesignScale checks the window uses the design's rail, padding, radii
// and type scale.
func TestDesignScale(t *testing.T) {
	tok := lightTokens
	if navWidth != 88 || contentPadding != 28 {
		t.Errorf("rail %v, padding %v, want 88 and 28", navWidth, contentPadding)
	}
	radii := []struct {
		name string
		got  float32
		want float32
	}{
		{"small", tok.RadiusS, 8},
		{"medium", tok.RadiusM, 12},
		{"large", tok.RadiusL, 16},
		{"extra large", tok.RadiusXL, 24},
		{"extra extra large", tok.Radius2XL, 32},
		{"full", tok.RadiusFull, 999},
	}
	for _, r := range radii {
		if r.got != r.want {
			t.Errorf("radius %s %v, want %v", r.name, r.got, r.want)
		}
	}
	sizes := []struct {
		name string
		got  float32
		want float32
	}{
		{"detail", tok.TextDetail, 36},
		{"display", tok.TextDisplay, 30},
		{"title", tok.TextTitle, 20},
		{"body", tok.TextBody, 14},
		{"small", tok.TextSmall, 13},
		{"tiny", tok.TextTiny, 11},
	}
	for _, s := range sizes {
		if s.got != s.want {
			t.Errorf("text %s %v, want %v", s.name, s.got, s.want)
		}
	}
}
