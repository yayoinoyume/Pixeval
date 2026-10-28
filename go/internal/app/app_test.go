package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
)

// TestMain runs the package's tests in Chinese with a temporary config
// directory: the visible texts are predictable, and no saved settings leak in.
// A test that checks another language sets LANG itself with t.Setenv.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pixeval-app-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("LANG", "zh_CN.UTF-8")
	os.Setenv("LC_ALL", "")
	// Detect reads LC_MESSAGES before LANG, so the runner's value must go too.
	os.Setenv("LC_MESSAGES", "")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestViewRenders(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 480, 360)
	if !tt.HasText("Pixeval Go") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// TestLanguageFollowsSystem checks a first run follows LANG.
func TestLanguageFollowsSystem(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	a := New()
	if a.lang != i18n.LangEnUS {
		t.Errorf("lang %q, want %q", a.lang, i18n.LangEnUS)
	}
	if got := a.t("nav.home"); got != "Home" {
		t.Errorf("nav.home %q, want Home", got)
	}
}

// TestLanguageSettingChangesNavigation checks a saved language switches the
// shell's texts.
func TestLanguageSettingChangesNavigation(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("Settings") {
		t.Fatalf("English navigation missing: %q", tt.Texts())
	}

	a.settings.Language = i18n.LangZhHans
	// The language select applies its change on the frame it fires; the test
	// drives the same switch without a Select to click.
	a.loadLanguage()
	tt.Frame()
	if !tt.HasText("设置") {
		t.Errorf("Chinese navigation missing after the switch: %q", tt.Texts())
	}
	if tt.HasText("Settings") {
		t.Errorf("English navigation still shown: %q", tt.Texts())
	}
}
