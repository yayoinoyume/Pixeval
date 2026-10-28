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
