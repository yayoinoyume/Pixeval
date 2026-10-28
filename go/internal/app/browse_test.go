package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeWorks makes n works numbered from first, tall enough to fill a page.
func fakeWorks(first, n int) []pixiv.Illustration {
	works := make([]pixiv.Illustration, n)
	for i := range works {
		works[i] = pixiv.Illustration{
			ID:     int64(first + i),
			Title:  fmt.Sprintf("作品 %d", first+i),
			User:   pixiv.User{Name: "作者"},
			Width:  480,
			Height: 720,
		}
	}
	return works
}

// newBrowseTest is an app whose feed loads from fetch, without images or a
// network.
func newBrowseTest(st *browseState) (*App, *ui.Tester) {
	a := New()
	a.images = imagex.New(nil, "")
	st.arm()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			a.browsePage(c, st)
		})
	}, 1000, 700)
	return a, tt
}

// waitBrowse waits for the load goroutine to settle the state as want.
func waitBrowse(t *testing.T, st *browseState, what string, want func(v browseView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("feed did not %s: items=%d loading=%v err=%q", what, len(v.items), v.loading, v.err)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestBrowseShowsWorks checks the first page's covers, titles and authors are
// on the page.
func TestBrowseShowsWorks(t *testing.T) {
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !tt.HasText("作者") {
		t.Errorf("author not shown: %q", tt.Texts())
	}
}

// TestBrowseLoadsNextPageAtEnd checks scrolling to the end asks for the next
// page once.
func TestBrowseLoadsNextPageAtEnd(t *testing.T) {
	calls := make(chan int, 4)
	var mu sync.Mutex
	n := 0
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		n++
		call := n
		mu.Unlock()
		calls <- call
		if call == 1 {
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20), NextURL: "next"}, nil
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(20, 20)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 20 })
	if got := <-calls; got != 1 {
		t.Fatalf("call %d, want 1", got)
	}
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	select {
	case got := <-calls:
		if got != 2 {
			t.Fatalf("call %d, want 2", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scrolling to the end did not load the next page")
	}
}

// TestBrowseErrorRetry checks a failed page shows the error and a retry that
// loads again.
func TestBrowseErrorRetry(t *testing.T) {
	var mu sync.Mutex
	n := 0
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		n++
		call := n
		mu.Unlock()
		if call == 1 {
			return nil, errors.New("网络错误")
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 3)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "fail", func(v browseView) bool { return v.err != "" })
	tt.Frame()
	if !tt.HasText("网络错误") {
		t.Errorf("error not shown: %q", tt.Texts())
	}
	if tt.HasText("加载中…") {
		t.Errorf("empty state shown under the error: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Fatalf("retry button not shown: %q", tt.Texts())
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, st, "retry", func(v browseView) bool { return len(v.items) == 3 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// TestBrowseSearchRestartDropsOldPage checks a page still in flight for the
// previous list is not added to the new one.
func TestBrowseSearchRestartDropsOldPage(t *testing.T) {
	a := New()
	a.images = imagex.New(nil, "")
	release := make(chan struct{})
	st := &browseState{}
	st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		<-release
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20)}, nil
	})
	st.arm()
	a.loadMore(st)

	st.restart()
	st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(100, 2)}, nil
	})
	a.loadMore(st)
	waitBrowse(t, st, "load the new list", func(v browseView) bool { return len(v.items) == 2 })

	// The old page lands now and must be dropped.
	close(release)
	time.Sleep(50 * time.Millisecond)
	if v := st.snapshot(); len(v.items) != 2 {
		t.Fatalf("old page added: items=%d", len(v.items))
	}
}

// TestBrowseNoMoreStopsPaging checks the end of the list is not asked for
// again.
func TestBrowseNoMoreStopsPaging(t *testing.T) {
	var mu sync.Mutex
	n := 0
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		n++
		call := n
		mu.Unlock()
		if call == 1 {
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20), NextURL: "next"}, nil
		}
		return nil, pixiv.ErrNoMore
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		calls := n
		mu.Unlock()
		if calls == 2 && !st.snapshot().loading {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("end of list not reached: calls=%d", calls)
		}
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	tt.Frame()
	mu.Lock()
	calls := n
	mu.Unlock()
	if calls != 2 {
		t.Fatalf("calls %d, want 2", calls)
	}
	if v := st.snapshot(); v.err != "" {
		t.Fatalf("end of list shown as an error: %q", v.err)
	}
}

// fakeHomeFeed is an armed home feed that counts its calls and returns two
// works numbered from first.
func fakeHomeFeed(first int, calls *int, mu *sync.Mutex) *browseState {
	st := &browseState{}
	st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		*calls = *calls + 1
		mu.Unlock()
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(first, 2)}, nil
	})
	st.arm()
	return st
}

// TestHomeTabsLoadTheirOwnFeed checks each home tab loads its own feed and
// that switching back keeps the pages it already has.
func TestHomeTabsLoadTheirOwnFeed(t *testing.T) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.images = imagex.New(nil, "")
	var mu sync.Mutex
	calls := make([]int, homeTabCount)
	a.homeFeeds = [homeTabCount]*browseState{
		fakeHomeFeed(0, &calls[0], &mu),
		fakeHomeFeed(100, &calls[1], &mu),
		fakeHomeFeed(200, &calls[2], &mu),
	}
	tt := ui.NewTester(a.View, 1000, 700)
	waitBrowse(t, a.homeFeeds[0], "load the recommended feed", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Fatalf("recommended feed not shown: %q", tt.Texts())
	}

	if err := tt.Click("关注"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.homeTab != 1 {
		t.Fatalf("homeTab %d, want 1", a.homeTab)
	}
	waitBrowse(t, a.homeFeeds[1], "load the following feed", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 100") {
		t.Fatalf("following feed not shown: %q", tt.Texts())
	}

	if err := tt.Click("排行"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.homeTab != homeTabRanking {
		t.Fatalf("homeTab %d, want %d", a.homeTab, homeTabRanking)
	}
	waitBrowse(t, a.homeFeeds[homeTabRanking], "load the ranking feed", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 200") {
		t.Fatalf("ranking feed not shown: %q", tt.Texts())
	}

	if err := tt.Click("推荐"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Fatalf("recommended feed lost on switch: %q", tt.Texts())
	}
	mu.Lock()
	got := append([]int(nil), calls...)
	mu.Unlock()
	if got[0] != 1 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("fetch calls %v, want [1 1 1]", got)
	}
}

// TestHomeFollowingNeedsLogin checks a signed-out user switching to the
// following tab gets the login prompt and no page is fetched.
func TestHomeFollowingNeedsLogin(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("关注"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.homeTab != 1 {
		t.Fatalf("homeTab %d, want 1", a.homeTab)
	}
	if !tt.HasText("登录 Pixiv") {
		t.Fatalf("login prompt missing: %q", tt.Texts())
	}
	if !a.homeFeeds[1].isArmed() {
		t.Error("following tab not armed on switch")
	}
	if v := a.homeFeeds[1].snapshot(); v.started || v.loading || len(v.items) != 0 {
		t.Fatalf("feed fetched while signed out: %+v", v)
	}
	if a.homeFeeds[0].isArmed() {
		t.Error("recommended tab armed without being drawn")
	}
}

// TestLoginArmsHomeFeed checks a session arms the home tab it will show. The
// window is missing, so the update runs at once and nothing is fetched.
func TestLoginArmsHomeFeed(t *testing.T) {
	a := New()
	a.applySession(&pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}})
	if !a.homeFeeds[0].isArmed() {
		t.Fatal("home feed not armed after login")
	}
	if v := a.homeFeeds[0].snapshot(); v.started || v.loading || len(v.items) != 0 {
		t.Fatalf("feed fetched before it is drawn: %+v", v)
	}
}

// filterWorks is three works, the second one's bookmarks above what the filter
// tests ask for.
func filterWorks() []pixiv.Illustration {
	works := fakeWorks(0, 3)
	works[1].TotalBookmarks = 500
	return works
}

// newFilterFeed is an app drawing a loaded feed of filterWorks, filter box and
// all, without images or a network.
func newFilterFeed(t *testing.T) (*App, *browseState, *ui.Tester) {
	t.Helper()
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: filterWorks()}, nil
	}}
	a, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 3 })
	tt.Frame()
	return a, st, tt
}

// runFilter types expr in the box and clicks apply.
func runFilter(t *testing.T, st *browseState, tt *ui.Tester, expr string) {
	t.Helper()
	st.filterText = expr
	if err := tt.Click("应用"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
}

// checkShown checks the feed draws exactly the works wanted, of the three
// filterWorks made.
func checkShown(t *testing.T, tt *ui.Tester, want ...string) {
	t.Helper()
	shown := map[string]bool{}
	for _, title := range want {
		shown[title] = true
	}
	for n := 0; n < 3; n++ {
		title := fmt.Sprintf("作品 %d", n)
		if tt.HasText(title) != shown[title] {
			t.Errorf("%s shown %v, want %v: %q", title, !shown[title], shown[title], tt.Texts())
		}
	}
}

// TestFilterShowsMatchingWorks checks an applied expression leaves the works
// it passes only, and that a tile still opens the work it shows.
func TestFilterShowsMatchingWorks(t *testing.T) {
	a, st, tt := newFilterFeed(t)
	runFilter(t, st, tt, "bookmarks>100")
	if st.filterErr != "" {
		t.Fatalf("filterErr %q", st.filterErr)
	}
	checkShown(t, tt, "作品 1")
	if err := tt.Click("作品 1"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.viewer == nil || a.viewer.work.ID != 1 {
		t.Fatalf("filtered tile did not open work 1: %+v", a.viewer)
	}
}

// TestFilterErrorKeepsList checks a malformed expression shows why and leaves
// the works shown as they were.
func TestFilterErrorKeepsList(t *testing.T) {
	_, st, tt := newFilterFeed(t)
	runFilter(t, st, tt, "bookmarks>")
	if st.filterErr == "" || st.filterExpr != nil {
		t.Fatalf("malformed expression applied: expr=%v err=%q", st.filterExpr, st.filterErr)
	}
	if !tt.HasText("过滤表达式无效") {
		t.Errorf("error not shown: %q", tt.Texts())
	}
	checkShown(t, tt, "作品 0", "作品 1", "作品 2")
}

// TestFilterClearedRestoresList checks an empty expression shows every work
// again.
func TestFilterClearedRestoresList(t *testing.T) {
	_, st, tt := newFilterFeed(t)
	runFilter(t, st, tt, "bookmarks>100")
	checkShown(t, tt, "作品 1")
	runFilter(t, st, tt, "")
	if st.filterExpr != nil || st.filterErr != "" {
		t.Fatalf("cleared filter kept: expr=%v err=%q", st.filterExpr, st.filterErr)
	}
	checkShown(t, tt, "作品 0", "作品 1", "作品 2")
}
