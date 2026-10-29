package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// newBookmarksApp is a signed-in app on the bookmarks page, without images or
// a network. The caller sets bookmarksPageFn before it builds a tester, so the
// first frame loads from the fake.
func newBookmarksApp(t *testing.T) *App {
	t.Helper()
	a := New()
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 1, Name: "测试用户"}}
	a.route = routeBookmarks
	return a
}

// TestBookmarksLoggedOut checks a signed-out user is asked to sign in and
// nothing is fetched.
func TestBookmarksLoggedOut(t *testing.T) {
	a := New()
	a.images = imagex.New(nil, "")
	a.route = routeBookmarks
	a.bookmarksPageFn = func(context.Context, bool, string) (*pixiv.Page[pixiv.Illustration], error) {
		t.Error("bookmarks fetched while signed out")
		return &pixiv.Page[pixiv.Illustration]{}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("登录 Pixiv 后开始浏览") {
		t.Errorf("login prompt missing: %q", tt.Texts())
	}
	if !tt.HasText("公开") || !tt.HasText("私密") {
		t.Errorf("tabs missing: %q", tt.Texts())
	}
}

// TestBookmarksTabsSwitch checks the private tab loads its own works, and the
// public tab's works do not stay in the list.
func TestBookmarksTabsSwitch(t *testing.T) {
	a := newBookmarksApp(t)
	var mu sync.Mutex
	var tabs []bool
	a.bookmarksPageFn = func(_ context.Context, private bool, _ string) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		tabs = append(tabs, private)
		mu.Unlock()
		if private {
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(100, 2)}, nil
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitBrowse(t, a.bookmarks, "load the public tab", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Fatalf("public works missing: %q", tt.Texts())
	}

	if err := tt.Click("私密"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.bookmarks, "load the private tab", func(v browseView) bool {
		return len(v.items) == 2 && v.items[0].ID == 100
	})
	tt.Frame()
	if tt.HasText("作品 0") {
		t.Errorf("the public tab's works stayed after the switch: %q", tt.Texts())
	}
	if !tt.HasText("作品 100") {
		t.Errorf("private works missing: %q", tt.Texts())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(tabs) < 2 || tabs[0] || !tabs[1] {
		t.Errorf("tabs fetched = %v, want public then private", tabs)
	}
}

// TestBookmarksLoadsNextPageAtEnd checks scrolling to the end asks for the
// next page through the cursor.
func TestBookmarksLoadsNextPageAtEnd(t *testing.T) {
	a := newBookmarksApp(t)
	var mu sync.Mutex
	var nexts []string
	a.bookmarksPageFn = func(_ context.Context, _ bool, next string) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		nexts = append(nexts, next)
		mu.Unlock()
		if next == "" {
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20), NextURL: "next"}, nil
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(20, 20)}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitBrowse(t, a.bookmarks, "load the first page", func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	waitBrowse(t, a.bookmarks, "load the second page", func(v browseView) bool { return len(v.items) == 40 })

	mu.Lock()
	defer mu.Unlock()
	if len(nexts) < 2 || nexts[0] != "" || nexts[1] != "next" {
		t.Errorf("cursors = %v, want the first page then next", nexts)
	}
}

// TestBookmarksUnbookmark checks the card's menu removes the bookmark and the
// work leaves the list.
func TestBookmarksUnbookmark(t *testing.T) {
	a := newBookmarksApp(t)
	a.bookmarksPageFn = func(context.Context, bool, string) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	}
	type call struct {
		id int64
		on bool
	}
	calls := make(chan call, 1)
	a.bookmarkFn = func(_ context.Context, id int64, on bool) error {
		calls <- call{id: id, on: on}
		return nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitBrowse(t, a.bookmarks, "load the works", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()

	if err := tt.RightClick("作品 1"); err != nil {
		t.Fatalf("RightClick: %v", err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "取消收藏") {
		t.Fatalf("menu = %v, want the unbookmark item", menu)
	}
	if err := tt.ChooseMenuItem("取消收藏"); err != nil {
		t.Fatalf("ChooseMenuItem: %v", err)
	}
	waitBrowse(t, a.bookmarks, "drop the work", func(v browseView) bool {
		return len(v.items) == 1 && v.items[0].ID == 0
	})
	select {
	case c := <-calls:
		if c.id != 1 || c.on {
			t.Errorf("bookmark call = %+v, want id 1 off", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the unbookmark request did not run")
	}
}

// TestBookmarksUnbookmarkFailureReloads checks a failed request loads the tab
// again, so the work the list dropped comes back.
func TestBookmarksUnbookmarkFailureReloads(t *testing.T) {
	a := newBookmarksApp(t)
	var mu sync.Mutex
	fetches := 0
	a.bookmarksPageFn = func(context.Context, bool, string) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		fetches++
		mu.Unlock()
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	}
	// The request waits until the menu's frame is over, as a window would
	// order it; then the failure reloads the page.
	release := make(chan struct{})
	a.bookmarkFn = func(context.Context, int64, bool) error {
		<-release
		return errors.New("网络错误")
	}
	// The page alone: a full View would read the failure notice while the
	// request goroutine writes it, which only a window would order.
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { a.bookmarksPage(c) })
	}, 1000, 700)
	waitBrowse(t, a.bookmarks, "load the works", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()

	if err := tt.RightClick("作品 1"); err != nil {
		t.Fatalf("RightClick: %v", err)
	}
	if err := tt.ChooseMenuItem("取消收藏"); err != nil {
		t.Fatalf("ChooseMenuItem: %v", err)
	}
	close(release)
	waitBrowse(t, a.bookmarks, "drop the loaded works", func(v browseView) bool { return len(v.items) == 0 })
	tt.Frame()
	waitBrowse(t, a.bookmarks, "reload the tab", func(v browseView) bool { return len(v.items) == 2 })
	mu.Lock()
	n := fetches
	mu.Unlock()
	if n < 2 {
		t.Errorf("fetches = %d, want the tab loaded again", n)
	}
	tt.Frame()
	if !tt.HasText("作品 1") {
		t.Errorf("the restored work is missing: %q", tt.Texts())
	}
}
