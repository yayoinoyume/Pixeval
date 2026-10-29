package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// These tests cover what the app offers to the keyboard and to screen
// readers: the window's shortcuts, the focus order, and the labels the
// views give the elements they draw themselves.

// TestShellShortcuts checks the window's keys switch pages without the
// pointer, and that the search shortcut focuses the search box.
func TestShellShortcuts(t *testing.T) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1000, 700)
	for _, c := range []struct {
		key  ui.Key
		want route
	}{
		{ui.KeyComma, routeSettings},
		{ui.KeyH, routeHistory},
		{ui.KeyD, routeDownloads},
	} {
		tt.Key(ui.Cmd, c.key)
		if a.route != c.want {
			t.Errorf("route %q after the shortcut, want %q", a.route, c.want)
		}
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if a.route != routeSearch {
		t.Fatalf("route %q after the search shortcut, want %q", a.route, routeSearch)
	}
	if !tt.Focused("搜索作品") {
		t.Errorf("the search box does not have the focus: %q", tt.Texts())
	}
}

// TestShellTabOrder checks the keys reach the navigation first, in its order,
// and the header's controls after it.
func TestShellTabOrder(t *testing.T) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1000, 700)
	for _, want := range []string{"首页", "新作", "MyPixiv", "推荐作者", "搜索", "特辑", "追更", "下载", "历史", "收藏", "稍后再看", "设置", "退出"} {
		tt.Key(0, ui.KeyTab)
		if !tt.Focused(want) {
			t.Fatalf("tab did not land on %q", want)
		}
	}
}

// TestShellEscapeLeavesUserPage checks Escape returns from a user page to the
// page it was opened from.
func TestShellEscapeLeavesUserPage(t *testing.T) {
	a := newPageApp()
	a.route = routeHome
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Key(0, ui.KeyEscape)
	if a.route != routeHome {
		t.Errorf("route %q after escape, want %q", a.route, routeHome)
	}
}

// TestBrowseCardLabels checks a card names its work and author for a screen
// reader, and that the keyboard reaches it right after the feed's filter
// control, with nothing in between.
func TestBrowseCardLabels(t *testing.T) {
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	card := "作品 0，作者：作者"
	if _, ok := tt.Find(card); !ok {
		t.Fatalf("card label %q missing: %q", card, tt.Texts())
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("筛选") {
		t.Fatalf("the first stop is not the filter control: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused(card) {
		t.Errorf("the filter control is not followed by the first card: %q", tt.Texts())
	}
}

// TestHomeTabsKeyboard checks the home tab line is one stop of Tab and takes
// the keys of a tab list: the arrows and Home/End move the choice, and the
// focus follows it.
func TestHomeTabsKeyboard(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	// The navigation and the sign-in button come first; the tab line is one
	// stop, on the tab chosen.
	reached := false
	for i := 0; i < 16 && !reached; i++ {
		tt.Key(0, ui.KeyTab)
		reached = tt.Focused("推荐")
	}
	if !reached {
		t.Fatalf("Tab never reached the chosen tab: %q", tt.Texts())
	}
	if tt.Focused("关注") {
		t.Fatalf("an unchosen tab took the focus: %q", tt.Texts())
	}

	tt.Key(0, ui.KeyRight)
	if a.homeTab != 1 {
		t.Fatalf("homeTab %d after the right key, want 1", a.homeTab)
	}
	if !tt.Focused("关注") {
		t.Errorf("the focus did not follow the choice: %q", tt.Texts())
	}

	tt.Key(0, ui.KeyEnd)
	if a.homeTab != homeTabRanking {
		t.Fatalf("homeTab %d after End, want %d", a.homeTab, homeTabRanking)
	}
	tt.Key(0, ui.KeyHome)
	if a.homeTab != 0 {
		t.Fatalf("homeTab %d after Home, want 0", a.homeTab)
	}
	tt.Key(0, ui.KeyLeft)
	if a.homeTab != homeTabRanking {
		t.Fatalf("homeTab %d after the left key from the first, want %d", a.homeTab, homeTabRanking)
	}
}
