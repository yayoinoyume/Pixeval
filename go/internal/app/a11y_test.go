package app

import (
	"context"
	"strings"
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
	for _, want := range []string{"首页", "搜索", "收藏", "作品", "用户", "资料库", "设置", "编辑卡片", "退出"} {
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

// TestHomeLabelsCarryNoSeparatorDot checks the home page names its cards
// without the middle dot: a screen reader reads a colon, not a glyph.
func TestHomeLabelsCarryNoSeparatorDot(t *testing.T) {
	a := newHomeApp(t)
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, fakeWorks(i*100, 2))
	}
	for _, card := range a.homeCards {
		labels := []string{homeViewAllLabel(a, card)}
		for _, key := range []string{"home.moveUp", "home.moveDown", "home.removeCard"} {
			labels = append(labels, a.homeCardActionLabel(key, card))
		}
		for _, label := range labels {
			if strings.Contains(label, "·") {
				t.Errorf("label %q carries the middle dot", label)
			}
		}
	}
}

// TestHomeCardTitlesAreNotTabs checks the home page no longer offers a tab
// line: the card titles are names, not tab stops, and Tab reaches the cards'
// View all actions instead.
func TestHomeCardTitlesAreNotTabs(t *testing.T) {
	a := newHomeApp(t)
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, fakeWorks(i*100, 2))
	}
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	viewAll := homeViewAllLabel(a, a.homeCards[0])
	reached := false
	for i := 0; i < 40 && !reached; i++ {
		tt.Key(0, ui.KeyTab)
		for _, title := range []string{
			a.homeCardTitle(a.homeCards[0]),
			a.homeCardTitle(a.homeCards[1]),
			a.homeCardTitle(a.homeCards[2]),
		} {
			if tt.Focused(title) {
				t.Fatalf("the card title %q took the focus as a tab did", title)
			}
		}
		reached = tt.Focused(viewAll)
	}
	if !reached {
		t.Fatalf("Tab never reached %q: %q", viewAll, tt.Texts())
	}
}
