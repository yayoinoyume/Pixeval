package app

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestShellLoggedOut checks the home page shows the login button to a
// signed-out user. Find is exact, so the prompt text alone does not pass.
func TestShellLoggedOut(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if _, ok := tt.Find("登录 Pixiv"); !ok {
		t.Errorf("login button missing: %q", tt.Texts())
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
	if !tt.HasText("下载目录") {
		t.Errorf("settings page missing: %q", tt.Texts())
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

// TestSessionLifecycle checks a saved token restores the session and a logout
// removes the token file.
func TestSessionLifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := tokenPath()
	if err != nil {
		t.Fatalf("tokenPath: %v", err)
	}
	tok := &pixiv.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    3600,
		User:         pixiv.TokenUser{Name: "测试用户"},
	}
	if err := pixiv.SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	a := New()
	a.restoreSession()
	if a.session == nil {
		t.Fatal("session not restored")
	}
	if got := a.userName(); got != "测试用户" {
		t.Errorf("userName %q", got)
	}

	a.logout()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("token file still at %s after logout (stat err %v)", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestShellRail checks the rail is 88 wide with every destination inside it
// and the settings held to the bottom, as the design draws it.
func TestShellRail(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1024, 768)
	tt.Frame()
	for _, label := range []string{"首页", "搜索", "追更", "下载", "历史", "设置"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%s missing: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > navWidth+1 {
			t.Errorf("%s at %+v, outside the %v rail", label, r, navWidth)
		}
	}
	home, _ := tt.Find("首页")
	settings, _ := tt.Find("设置")
	if settings.Y <= home.Y {
		t.Errorf("settings at %v is not under the destinations (%v)", settings.Y, home.Y)
	}
	if settings.Y < 768/2 {
		t.Errorf("settings at %v is not held to the bottom", settings.Y)
	}
}

// TestShellFitsWindow checks the rail and the app bar fit the narrow and the
// design window, with nothing pushed past the right or bottom edge.
func TestShellFitsWindow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{1024, 768}, {1280, 820}} {
		a := New()
		tt := ui.NewTester(a.View, size.w, size.h)
		tt.Frame()
		r, ok := tt.Find("登录 Pixiv")
		if !ok {
			t.Fatalf("%dx%d: login button missing: %q", size.w, size.h, tt.Texts())
		}
		if r.X+r.W > float32(size.w) || r.Y+r.H > float32(size.h) {
			t.Errorf("%dx%d: login button %+v outside the window", size.w, size.h, r)
		}
		for _, label := range []string{"首页", "设置"} {
			r, ok := tt.Find(label)
			if !ok || r.X+r.W > float32(size.w) || r.Y+r.H > float32(size.h) {
				t.Errorf("%dx%d: %s %+v outside the window", size.w, size.h, label, r)
			}
		}
	}
}
