package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// newExtensionsApp is a settings page tall enough to show the Extensions row
// at the bottom of the About card.
func newExtensionsApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	return a, ui.NewTester(a.View, 1000, 2400)
}

// TestSettingsOpensExtensions checks the settings row opens the extension
// information page, which has no add, enable or remove actions, and returns
// to the settings page.
func TestSettingsOpensExtensions(t *testing.T) {
	a, tt := newExtensionsApp(t)
	tt.Frame()
	if _, ok := tt.Find(a.t("ext.settingsHint")); !ok {
		t.Fatalf("the extensions row is missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("ext.settingsHint")); err != nil {
		t.Fatalf("Click extensions: %v", err)
	}
	if a.route != routeExtensions {
		t.Fatalf("route %q after extensions, want %q", a.route, routeExtensions)
	}
	tt.Frame()
	for _, want := range []string{
		a.t("ext.hostMissing"),
		a.t("ext.hostMissingHint"),
		a.t("ext.why"),
		a.t("ext.available"),
	} {
		if !tt.HasText(want) {
			t.Errorf("%q missing from extensions: %q", want, tt.Texts())
		}
	}
	for _, text := range tt.Texts() {
		for _, fake := range []string{
			"添加扩展", "启用扩展", "卸载扩展", "打开文件夹",
			"add extension", "enable extension", "uninstall", "open folder",
			"ajouter une extension", "activer l'extension", "désinstaller", "ouvrir le dossier",
			"добавить расширение", "включить расширение", "удалить расширение", "открыть папку",
		} {
			if strings.Contains(strings.ToLower(text), fake) {
				t.Errorf("extensions page offers a fake action %q: %q", fake, text)
			}
		}
	}
	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back from extensions: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q after back, want %q", a.route, routeSettings)
	}
}

// TestExtensionsPageEscapeReturnsToSettings checks Esc leaves the extension
// information page for the settings page it came from.
func TestExtensionsPageEscapeReturnsToSettings(t *testing.T) {
	a, tt := newExtensionsApp(t)
	tt.Frame()
	if err := tt.Click(a.t("ext.settingsHint")); err != nil {
		t.Fatalf("Click extensions: %v", err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.route != routeSettings {
		t.Errorf("route %q after Esc, want %q", a.route, routeSettings)
	}
}

// TestExtensionsPageFitsNarrowWindow checks the extension information page in
// a 1024 by 720 window: its title, paragraphs and GitHub button stay inside
// the width instead of running off the side.
func TestExtensionsPageFitsNarrowWindow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeExtensions
	const width, height = 1024, 720
	tt := ui.NewTester(a.View, width, height)
	tt.Frame()

	for _, label := range []string{
		a.t("nav.extensions"),
		a.t("ext.hostMissing"),
		a.t("ext.hostMissingHint"),
		a.t("ext.why"),
		a.t("ext.available"),
		a.t("about.github"),
	} {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
	}
}
