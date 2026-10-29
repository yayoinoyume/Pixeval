package app

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
)

// newHelpApp is a settings page that can open the help and about pages.
func newHelpApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	return a, ui.NewTester(a.View, 1000, 1400)
}

// TestSettingsOpensHelpAndAbout checks the About card opens both pages and
// their back buttons return to the settings page.
func TestSettingsOpensHelpAndAbout(t *testing.T) {
	a, tt := newHelpApp(t)
	if err := tt.Click(a.t("help.title")); err != nil {
		t.Fatalf("Click help: %v", err)
	}
	if a.route != routeHelp {
		t.Fatalf("route %q after help, want %q", a.route, routeHelp)
	}
	tt.Frame()
	for _, want := range []string{a.t("help.shortcuts"), a.t("help.features")} {
		if !tt.HasText(want) {
			t.Errorf("%q missing from help: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back from help: %v", err)
	}
	if a.route != routeSettings {
		t.Fatalf("route %q after back, want %q", a.route, routeSettings)
	}
	tt.Frame()

	if err := tt.Click(a.t("about.title") + " Pixeval Go"); err != nil {
		t.Fatalf("Click about: %v", err)
	}
	if a.route != routeAbout {
		t.Fatalf("route %q after about, want %q", a.route, routeAbout)
	}
	tt.Frame()
	for _, want := range []string{"Pixeval Go", a.tf("about.version", appVersion()), a.t("about.license")} {
		if !tt.HasText(want) {
			t.Errorf("%q missing from about: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back from about: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q after back, want %q", a.route, routeSettings)
	}
}

// TestAboutPageOpensGitHub checks the repository link uses the injected
// opener, so the test does not leave the app.
func TestAboutPageOpensGitHub(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	var opened []string
	a.openExternalFn = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	a.route = routeAbout
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("about.github")); err != nil {
		t.Fatalf("Click GitHub: %v", err)
	}
	if len(opened) != 1 || opened[0] != pixevalGoURL {
		t.Errorf("opened %q, want [%q]", opened, pixevalGoURL)
	}
}

// TestHelpPageLinkFailureToasts checks a link that cannot open is reported.
func TestHelpPageLinkFailureToasts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.openExternalFn = func(string) error { return errors.New("没有打开程序") }
	a.route = routeHelp
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("about.github")); err != nil {
		t.Fatalf("Click GitHub: %v", err)
	}
	tt.Frame()
	if want := a.tf("about.openFailed", "没有打开程序"); !tt.HasText(want) {
		t.Errorf("%q missing: %q", want, tt.Texts())
	}
}
