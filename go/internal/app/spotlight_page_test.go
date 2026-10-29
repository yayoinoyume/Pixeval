package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// newSpotlightApp is a signed-in app on the spotlight page, without images or
// a network. The caller sets spotlightFn before it builds a tester, so the
// first frame loads from the fake.
func newSpotlightApp(t *testing.T) *App {
	t.Helper()
	a := New()
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 1, Name: "测试用户"}}
	a.route = routeSpotlight
	return a
}

// fakeSpotlight returns n articles starting at first.
func fakeSpotlight(first, n int) []pixiv.SpotlightArticle {
	articles := make([]pixiv.SpotlightArticle, n)
	for i := range articles {
		articles[i] = pixiv.SpotlightArticle{
			ID:          strconv.Itoa(first + i),
			Title:       fmt.Sprintf("特辑 %d", first+i),
			Thumbnail:   fmt.Sprintf("https://example.com/%d.jpg", first+i),
			ArticleURL:  fmt.Sprintf("https://www.pixivision.net/a/%d", first+i),
			PublishDate: "2026-10-01 12:00:00",
		}
	}
	return articles
}

// waitSpotlight waits for the list to reach a state.
func waitSpotlight(t *testing.T, st *spotlightState, what string, want func(v spotlightView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("spotlight did not %s: items=%d loading=%v err=%q",
				what, len(v.items), v.loading, v.err)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSpotlightLoggedOut checks a signed-out user is asked to sign in and
// nothing is fetched.
func TestSpotlightLoggedOut(t *testing.T) {
	a := New()
	a.images = imagex.New(nil, "")
	a.route = routeSpotlight
	a.spotlightFn = func(context.Context, string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		t.Error("spotlight fetched while signed out")
		return &pixiv.Page[pixiv.SpotlightArticle]{}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("登录 Pixiv 后开始浏览") {
		t.Errorf("login prompt missing: %q", tt.Texts())
	}
}

// TestSpotlightLoads checks the cards show each article's title and date.
func TestSpotlightLoads(t *testing.T) {
	a := newSpotlightApp(t)
	a.spotlightFn = func(context.Context, string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		return &pixiv.Page[pixiv.SpotlightArticle]{Items: fakeSpotlight(0, 2)}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitSpotlight(t, a.spotlight, "load the articles", func(v spotlightView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("特辑 0") || !tt.HasText("特辑 1") {
		t.Errorf("article titles missing: %q", tt.Texts())
	}
	if !tt.HasText("2026-10-01") {
		t.Errorf("publication date missing: %q", tt.Texts())
	}
}

// TestSpotlightEmpty checks an ended list with no articles shows the empty
// state instead of loading forever.
func TestSpotlightEmpty(t *testing.T) {
	a := newSpotlightApp(t)
	a.spotlightFn = func(context.Context, string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		return &pixiv.Page[pixiv.SpotlightArticle]{}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitSpotlight(t, a.spotlight, "load an empty page", func(v spotlightView) bool { return v.started })
	tt.Frame()
	if !tt.HasText(a.t("spotlight.empty")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestSpotlightLoadFailed checks a failed page shows its error and a retry.
func TestSpotlightLoadFailed(t *testing.T) {
	a := newSpotlightApp(t)
	a.spotlightFn = func(context.Context, string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		return nil, errors.New("boom")
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitSpotlight(t, a.spotlight, "fail the page", func(v spotlightView) bool { return v.err != "" })
	tt.Frame()
	if !tt.HasText("加载特辑失败") {
		t.Errorf("error missing: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Errorf("retry missing: %q", tt.Texts())
	}
}

// TestSpotlightLoadsNextPageAtEnd checks scrolling to the end asks for the
// next page through the cursor.
func TestSpotlightLoadsNextPageAtEnd(t *testing.T) {
	a := newSpotlightApp(t)
	var mu sync.Mutex
	var nexts []string
	a.spotlightFn = func(_ context.Context, next string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		mu.Lock()
		nexts = append(nexts, next)
		mu.Unlock()
		if next == "" {
			return &pixiv.Page[pixiv.SpotlightArticle]{Items: fakeSpotlight(0, 20), NextURL: "next"}, nil
		}
		return &pixiv.Page[pixiv.SpotlightArticle]{Items: fakeSpotlight(20, 20)}, nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitSpotlight(t, a.spotlight, "load the first page", func(v spotlightView) bool { return len(v.items) == 20 })
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	waitSpotlight(t, a.spotlight, "load the second page", func(v spotlightView) bool { return len(v.items) == 40 })

	mu.Lock()
	defer mu.Unlock()
	if len(nexts) < 2 || nexts[0] != "" || nexts[1] != "next" {
		t.Errorf("cursors = %v, want the first page then next", nexts)
	}
}

// TestSpotlightClickOpensArticle checks a click opens the article's page in
// the browser, not in the app.
func TestSpotlightClickOpensArticle(t *testing.T) {
	a := newSpotlightApp(t)
	a.spotlightFn = func(context.Context, string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		return &pixiv.Page[pixiv.SpotlightArticle]{Items: fakeSpotlight(0, 1)}, nil
	}
	var mu sync.Mutex
	var opened []string
	a.openExternalFn = func(url string) error {
		mu.Lock()
		opened = append(opened, url)
		mu.Unlock()
		return nil
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitSpotlight(t, a.spotlight, "load the article", func(v spotlightView) bool { return len(v.items) == 1 })
	tt.Frame()
	if err := tt.Click("特辑 0"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 1 || opened[0] != "https://www.pixivision.net/a/0" {
		t.Errorf("opened = %v, want the article URL", opened)
	}
}
