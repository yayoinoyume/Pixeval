package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestShellLoggedOut checks the home page guides a signed-out user to login.
func TestShellLoggedOut(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("登录 Pixiv") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !tt.HasText("首页") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// TestShellNavSwitches checks a click on the navigation changes the page and
// its title.
func TestShellNavSwitches(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q, want %q", a.route, routeSettings)
	}
	if !tt.HasText("设置（M1 占位）") {
		t.Errorf("texts %q", tt.Texts())
	}
	// The nav item and the header both show 设置.
	n := 0
	for _, s := range tt.Texts() {
		if s == "设置" {
			n++
		}
	}
	if n < 2 {
		t.Errorf("title not shown: %q", tt.Texts())
	}
}

// TestShellLoggedIn checks a signed-in session shows the user's name instead
// of the login prompt.
func TestShellLoggedIn(t *testing.T) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("测试用户") {
		t.Errorf("texts %q", tt.Texts())
	}
	if tt.HasText("登录 Pixiv") {
		t.Errorf("login prompt shown while signed in: %q", tt.Texts())
	}
}
