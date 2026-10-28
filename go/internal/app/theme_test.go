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
