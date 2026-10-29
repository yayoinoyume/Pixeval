package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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

// TestShowR18ReachesClient checks the saved setting is the client's R18
// filter: on at startup, and again after the settings page saves it.
func TestShowR18ReachesClient(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "pixeval-go", "settings.json")
	saved := Default()
	saved.ShowR18 = true
	saved.DownloadDir = t.TempDir()
	if err := Save(path, saved); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a := New()
	if !a.client.AllowR18 {
		t.Error("AllowR18 off for a saved showR18=true")
	}

	a.settings.ShowR18 = false
	a.saveSettings()
	select {
	case err := <-a.settingsSaved:
		if err != nil {
			t.Fatalf("saveSettings: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("settings save did not finish")
	}
	if a.client.AllowR18 {
		t.Error("AllowR18 on after showR18 was turned off and saved")
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
	if !st.filterOpen {
		if err := tt.Click("筛选"); err != nil {
			t.Fatalf("open filter: %v", err)
		}
		tt.Frame()
	}
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

// TestFilterCollapsedNamesExpression checks the collapsed bar keeps showing the
// expression in force, so a filtered feed that stops paging is not read as one
// that ran out, and that its clear control drops the filter.
func TestFilterCollapsedNamesExpression(t *testing.T) {
	_, st, tt := newFilterFeed(t)
	runFilter(t, st, tt, "bookmarks>100")
	if st.filterOpen {
		t.Fatal("the box stayed open after apply")
	}
	if !tt.HasText("已筛选：bookmarks>100") {
		t.Fatalf("the applied expression is not named: %q", tt.Texts())
	}
	checkShown(t, tt, "作品 1")

	if err := tt.Click("清除筛选"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	tt.Frame()
	if st.filterExpr != nil || st.filterText != "" {
		t.Fatalf("filter kept after clear: expr=%v text=%q", st.filterExpr, st.filterText)
	}
	if tt.HasText("已筛选") {
		t.Errorf("the cleared expression is still named: %q", tt.Texts())
	}
	checkShown(t, tt, "作品 0", "作品 1", "作品 2")
}

// TestFilterCollapsedKeepsAppliedExpression checks the collapsed bar names the
// expression in force, not the one being typed: a draft edited and then closed
// without applying leaves the filter that passes the works shown.
func TestFilterCollapsedKeepsAppliedExpression(t *testing.T) {
	_, st, tt := newFilterFeed(t)
	runFilter(t, st, tt, "bookmarks>100")
	if err := tt.Click("筛选"); err != nil {
		t.Fatalf("open filter: %v", err)
	}
	tt.Frame()
	st.filterText = "bookmarks>400"
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("close filter: %v", err)
	}
	tt.Frame()
	if st.filterOpen {
		t.Fatal("the box stayed open")
	}
	if tt.HasText("已筛选：bookmarks>400") {
		t.Errorf("the collapsed bar shows the draft: %q", tt.Texts())
	}
	if !tt.HasText("已筛选：bookmarks>100") {
		t.Fatalf("the collapsed bar does not name the applied expression: %q", tt.Texts())
	}
	checkShown(t, tt, "作品 1")
}

// TestBrowseCardMetrics checks the feed keeps the card width the settings
// choose, the design's gap, and the cover size the thumbnails are cached at.
func TestBrowseCardMetrics(t *testing.T) {
	if cardWidth != 240 {
		t.Errorf("card width %v, want the default 240", cardWidth)
	}
	if cardGap != 20 {
		t.Errorf("card gap %v, want 20", cardGap)
	}
	if coverWidth != 480 || coverHeight != 720 {
		t.Errorf("cover %vx%v, want 480x720", coverWidth, coverHeight)
	}
	if contentPadding != 40 {
		t.Errorf("content padding %v, want 40", contentPadding)
	}
}

// feedPixiv is a fake app API that records the feed requests it receives and
// answers each path with the body its body func makes, by 0-based call count.
type feedPixiv struct {
	mu    sync.Mutex
	paths []string
	query []url.Values
	body  func(n int, path, host string) string
}

func (p *feedPixiv) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		n := 0
		for _, path := range p.paths {
			if path == r.URL.Path {
				n++
			}
		}
		p.paths = append(p.paths, r.URL.Path)
		p.query = append(p.query, r.URL.Query())
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, p.body(n, r.URL.Path, r.Host))
	})
}

func (p *feedPixiv) requests() (paths []string, query []url.Values) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.paths...), append([]url.Values(nil), p.query...)
}

// worksJSON is n works numbered from first, with next as the next page's URL.
func worksJSON(first, n int, next string) string {
	var b strings.Builder
	b.WriteString(`{"illusts":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"title":"作品 %d","user":{"id":9,"name":"作者"},"width":480,"height":720}`,
			first+i, first+i)
	}
	fmt.Fprintf(&b, `],"next_url":%q}`, next)
	return b.String()
}

// feedAPI is a fake app API's handler, which the feed tests point the client
// at instead of the network.
type feedAPI interface{ handler() http.Handler }

// newFeedTestApp is a signed-in app whose feeds load from fake, at the design
// window's narrow width.
func newFeedTestApp(t *testing.T, fake feedAPI) (*App, *ui.Tester) {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse the test server URL: %v", err)
	}
	a := New()
	a.client.HTTP().Transport = e2eTransport{target: target}
	a.client.SetCooldown(0)
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	return a, ui.NewTester(a.View, 1024, 768)
}

// TestNewPageLoadsNewFeed checks the new works destination asks the app API
// for the site-wide feed and opens a work.
func TestNewPageLoadsNewFeed(t *testing.T) {
	fake := &feedPixiv{body: func(int, string, string) string { return worksJSON(1, 2, "") }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	if a.route != routeNew {
		t.Fatalf("route %q, want %q", a.route, routeNew)
	}
	waitBrowse(t, a.newFeed, "load the new works",
		func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 1") {
		t.Fatalf("the feed is not shown: %q", tt.Texts())
	}
	paths, query := fake.requests()
	if len(paths) != 1 || paths[0] != "/v1/illust/new" {
		t.Fatalf("requests %v, want /v1/illust/new", paths)
	}
	if q := query[0]; q.Get("filter") != "for_android" || q.Get("offset") != "" {
		t.Errorf("query = %v, want the first page's R18 filter", q)
	}
	if err := tt.Click("作品 1"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.viewer == nil || a.viewer.work.ID != 1 {
		t.Fatalf("the tile did not open work 1: %+v", a.viewer)
	}
}

// TestMyPixivPageLoadsFollowedUsersWorks checks the MyPixiv destination asks
// the app API for the followed users' feed.
func TestMyPixivPageLoadsFollowedUsersWorks(t *testing.T) {
	fake := &feedPixiv{body: func(int, string, string) string { return worksJSON(5, 2, "") }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "MyPixiv")
	if a.route != routeMyPixiv {
		t.Fatalf("route %q, want %q", a.route, routeMyPixiv)
	}
	waitBrowse(t, a.myPixivFeed, "load the followed users' works",
		func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("作品 5") {
		t.Fatalf("the feed is not shown: %q", tt.Texts())
	}
	paths, query := fake.requests()
	if len(paths) != 1 || paths[0] != "/v2/illust/mypixiv" {
		t.Fatalf("requests %v, want /v2/illust/mypixiv", paths)
	}
	if q := query[0]; q.Get("filter") != "for_android" || q.Get("offset") != "" {
		t.Errorf("query = %v, want the first page's R18 filter", q)
	}
}

// TestNewFeedPagesWithCursor checks the new works feed loads its next page
// from the cursor the previous page returned.
func TestNewFeedPagesWithCursor(t *testing.T) {
	fake := &feedPixiv{body: func(n int, path, host string) string {
		if n == 0 {
			return worksJSON(0, 20, "http://"+host+"/v1/illust/new?filter=for_android&offset=30")
		}
		return worksJSON(20, 20, "")
	}}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "load the first page",
		func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	waitBrowse(t, a.newFeed, "load the next page",
		func(v browseView) bool { return len(v.items) == 40 })
	paths, query := fake.requests()
	if len(paths) != 2 {
		t.Fatalf("requests %v, want 2", paths)
	}
	if q := query[1]; q.Get("offset") != "30" || q.Get("filter") != "for_android" {
		t.Errorf("next page query = %v, want the cursor's offset", q)
	}
}

// TestMyPixivEmptyFeed checks an empty feed shows the empty state instead of
// an error.
func TestMyPixivEmptyFeed(t *testing.T) {
	fake := &feedPixiv{body: func(int, string, string) string {
		return `{"illusts":[],"next_url":""}`
	}}
	_, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "MyPixiv")
	// The empty state draws once a page settled; wait for it on the frame.
	deadline := time.Now().Add(2 * time.Second)
	for {
		tt.Frame()
		if tt.HasText("没有作品") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("empty state missing: %q", tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// kindPixiv answers the works feeds by content type, so a test can tell which
// kind a page's feed holds: illustrations are numbered from 1, manga from 500.
type kindPixiv struct {
	mu    sync.Mutex
	paths []string
	query []url.Values
	body  func(q url.Values, host string) string
}

func (p *kindPixiv) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.Path)
		p.query = append(p.query, q)
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, p.body(q, r.Host))
	})
}

func (p *kindPixiv) requests() (paths []string, query []url.Values) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.paths...), append([]url.Values(nil), p.query...)
}

// kindWorksBody is two works of q's kind, titled so a test tells them from the
// other kind's.
func kindWorksBody(q url.Values) string {
	first := 1
	if q.Get("content_type") == "manga" {
		first = 500
	}
	return worksJSON(first, 2, "")
}

// TestNewPageKindTabsAskTheirContentType checks the new works page starts on
// illustrations and its manga tab asks for manga, dropping the works shown.
func TestNewPageKindTabsAskTheirContentType(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string { return kindWorksBody(q) }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "load the illustrations",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 1 })
	if _, q := fake.requests(); len(q) != 1 || q[0].Get("content_type") != "illust" {
		t.Fatalf("first requests = %v, want one illust page", q)
	}

	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "load the manga",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 500 })
	tt.Frame()
	if tt.HasText("作品 1") {
		t.Errorf("the illustration page stayed under the manga tab: %q", tt.Texts())
	}
	_, q := fake.requests()
	if len(q) != 2 {
		t.Fatalf("requests = %d, want the two first pages", len(q))
	}
	if got := q[1]; got.Get("content_type") != "manga" || got.Get("offset") != "" {
		t.Errorf("manga query = %v, want content_type=manga and no cursor", got)
	}
	if a.route != routeNew {
		t.Errorf("route %q, want the new works page", a.route)
	}
	if err := tt.Click("作品 500"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.viewer == nil || a.viewer.work.ID != 500 {
		t.Fatalf("the manga tile did not open work 500: %+v", a.viewer)
	}
}

// TestMyPixivKindTabsAskTheirContentType checks the MyPixiv page's manga tab
// asks its own feed for manga, dropping the illustrations shown.
func TestMyPixivKindTabsAskTheirContentType(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string { return kindWorksBody(q) }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "MyPixiv")
	waitBrowse(t, a.myPixivFeed, "load the illustrations",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 1 })
	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.myPixivFeed, "load the manga",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 500 })
	paths, q := fake.requests()
	if len(paths) != 2 || paths[0] != "/v2/illust/mypixiv" || paths[1] != "/v2/illust/mypixiv" {
		t.Fatalf("paths = %v, want the MyPixiv feed twice", paths)
	}
	if got := q[1]; got.Get("content_type") != "manga" || got.Get("offset") != "" {
		t.Errorf("manga query = %v, want content_type=manga and no cursor", got)
	}
}

// TestNewPageKindSwitchBackRestartsTheFeed checks switching back to
// illustrations asks their first page again, not the page the manga tab left.
func TestNewPageKindSwitchBackRestartsTheFeed(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string { return kindWorksBody(q) }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "load the illustrations",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 1 })
	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "load the manga",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 500 })
	tt.Frame()
	if err := tt.Click("插画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "load the illustrations again",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 1 })
	_, q := fake.requests()
	if len(q) != 3 {
		t.Fatalf("requests = %d, want one first page per switch", len(q))
	}
	if got := q[2]; got.Get("content_type") != "illust" || got.Get("offset") != "" {
		t.Errorf("switch-back query = %v, want an illust first page", got)
	}
}

// TestNewPageMangaPagesWithCursor checks the manga tab loads its next page
// from the cursor, keeping content_type=manga.
func TestNewPageMangaPagesWithCursor(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string {
		if q.Get("offset") != "" {
			return worksJSON(520, 20, "")
		}
		return worksJSON(500, 20,
			"http://"+host+"/v1/illust/new?content_type="+q.Get("content_type")+"&filter=for_android&offset=30")
	}}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "load the manga",
		func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "load the manga first page",
		func(v browseView) bool { return len(v.items) >= 20 && v.items[0].ID == 500 })
	// The page may already have asked for its cursor while the tester drew it.
	if v := a.newFeed.snapshot(); len(v.items) == 20 {
		a.loadMore(a.newFeed)
	}
	waitBrowse(t, a.newFeed, "load the manga next page",
		func(v browseView) bool { return len(v.items) == 40 })
	_, q := fake.requests()
	if len(q) != 3 {
		t.Fatalf("requests = %d, want the illust page then the two manga pages", len(q))
	}
	if got := q[2]; got.Get("content_type") != "manga" || got.Get("offset") != "30" {
		t.Errorf("next page query = %v, want the manga cursor's offset", got)
	}
}

// TestNewPageMangaEmptyFeed checks an empty manga page shows the empty state,
// not an error, once its first page settled.
func TestNewPageMangaEmptyFeed(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string { return `{"illusts":[],"next_url":""}` }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "settle the illustration page",
		func(v browseView) bool { return v.started })
	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "settle the manga page", func(v browseView) bool { return v.started })
	tt.Frame()
	if !tt.HasText("没有作品") {
		t.Fatalf("empty state missing: %q", tt.Texts())
	}
	_, q := fake.requests()
	if len(q) != 2 || q[1].Get("content_type") != "manga" {
		t.Errorf("requests = %v, want an illust page then a manga one", q)
	}
}

// TestNewPageKindSurvivesLeavingThePage checks the page comes back on the kind
// it was left on, with its feed, instead of asking for the first kind again.
func TestNewPageKindSurvivesLeavingThePage(t *testing.T) {
	fake := &kindPixiv{body: func(q url.Values, host string) string { return kindWorksBody(q) }}
	a, tt := newFeedTestApp(t, fake)
	tt.Frame()
	navGo(t, tt, "新作")
	waitBrowse(t, a.newFeed, "load the illustrations",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 1 })
	tt.Frame()
	if err := tt.Click("漫画"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.newFeed, "load the manga",
		func(v browseView) bool { return len(v.items) == 2 && v.items[0].ID == 500 })
	tt.Frame()
	if err := tt.Click("首页"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	navGo(t, tt, "新作")
	tt.Frame()
	if a.newKind != feedManga {
		t.Errorf("kind = %d, want the manga tab still chosen", a.newKind)
	}
	if v := a.newFeed.snapshot(); len(v.items) != 2 || v.items[0].ID != 500 {
		t.Errorf("the manga feed was not kept: %+v", v.items)
	}
	// The home page loaded a feed of its own on the way back; only the new
	// works requests say whether its kind was kept.
	paths, _ := fake.requests()
	newPages := 0
	for _, path := range paths {
		if path == "/v1/illust/new" {
			newPages++
		}
	}
	if newPages != 2 {
		t.Errorf("new works requests = %d, want the two first pages only", newPages)
	}
}

// rankingPixiv is a fake app API that records the ranking modes it is asked
// for and answers with two works.
type rankingPixiv struct {
	mu          sync.Mutex
	modes       []string
	recommended int
}

func (p *rankingPixiv) handler() http.Handler {
	const body = `{"illusts":[` +
		`{"id":1,"title":"作品 1","user":{"id":9,"name":"作者"},"width":480,"height":720},` +
		`{"id":2,"title":"作品 2","user":{"id":9,"name":"作者"},"width":480,"height":720}],` +
		`"next_url":""}`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		switch r.URL.Path {
		case "/v1/illust/ranking":
			p.modes = append(p.modes, r.URL.Query().Get("mode"))
		case "/v1/illust/recommended":
			p.recommended++
		}
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	})
}

func (p *rankingPixiv) rankingModes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.modes...)
}

func (p *rankingPixiv) recommendedCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.recommended
}

// TestRankModeRequestsModeAndResetsFeed checks the ranking tab starts on the
// day ranking, asks for the mode chosen, and starts the feed over on a switch
// while the other home feeds keep their pages.
func TestRankModeRequestsModeAndResetsFeed(t *testing.T) {
	fake := &rankingPixiv{}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse the test server URL: %v", err)
	}

	a := New()
	a.client.HTTP().Transport = e2eTransport{target: target}
	a.client.SetCooldown(0)
	a.images = imagex.New(nil, "")
	a.applySession(&pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}})
	tt := ui.NewTester(a.View, 1000, 700)

	waitBrowse(t, a.homeFeeds[0], "load the recommended feed",
		func(v browseView) bool { return len(v.items) == 2 })
	if err := tt.Click("排行"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, a.homeFeeds[homeTabRanking], "load the day ranking",
		func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("日榜") {
		t.Fatalf("the day mode is not shown: %q", tt.Texts())
	}
	if got := fake.rankingModes(); len(got) != 1 || got[0] != "day" {
		t.Fatalf("ranking modes %v, want [day]", got)
	}

	a.setRankMode(pixiv.Week)
	if v := a.homeFeeds[homeTabRanking].snapshot(); len(v.items) != 0 {
		t.Fatalf("the week switch kept the day pages: items=%d", len(v.items))
	}
	tt.Frame()
	waitBrowse(t, a.homeFeeds[homeTabRanking], "load the week ranking",
		func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("周榜") {
		t.Fatalf("the week mode is not shown: %q", tt.Texts())
	}
	if got := fake.rankingModes(); got[len(got)-1] != "week" {
		t.Fatalf("ranking modes %v, want the last one week", got)
	}

	a.setRankMode(pixiv.DayMale)
	tt.Frame()
	waitBrowse(t, a.homeFeeds[homeTabRanking], "load the male day ranking",
		func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()
	if !tt.HasText("男性向日榜") {
		t.Fatalf("the male day mode is not shown: %q", tt.Texts())
	}
	if got := fake.rankingModes(); got[len(got)-1] != "day_male" {
		t.Fatalf("ranking modes %v, want the last one day_male", got)
	}

	if v := a.homeFeeds[0].snapshot(); len(v.items) != 2 {
		t.Fatalf("the recommended feed lost its pages: items=%d", len(v.items))
	}
	if got := fake.recommendedCalls(); got != 1 {
		t.Fatalf("recommended fetched %d times, want 1", got)
	}
}

// newSelectFeed is an app drawing a loaded feed of works, without images or a
// network.
func newSelectFeed(t *testing.T, works []pixiv.Illustration) (*App, *browseState, *ui.Tester) {
	t.Helper()
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: works}, nil
	}}
	a, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == len(works) })
	tt.Frame()
	return a, st, tt
}

// TestSelectionModePicksInsteadOfOpening checks the select switch turns a
// tile's click into a pick, a second click drops it, and the exit leaves the
// feed opening works again.
func TestSelectionModePicksInsteadOfOpening(t *testing.T) {
	a, st, tt := newSelectFeed(t, fakeWorks(0, 3))
	if err := tt.Click("选择"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if !st.selecting {
		t.Fatal("selection mode off after its switch")
	}
	tt.Frame()
	if err := tt.Click("作品 1"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if !st.selected[1] {
		t.Errorf("the click did not pick work 1: %v", st.selected)
	}
	if a.viewer != nil {
		t.Fatalf("a pick opened the viewer: %+v", a.viewer)
	}
	tt.Frame()
	if err := tt.Click("作品 1"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if st.selected[1] {
		t.Error("a second click did not drop the pick")
	}

	if err := tt.Click("退出选择"); err != nil {
		t.Fatalf("exit: %v", err)
	}
	if st.selecting || len(st.selected) != 0 {
		t.Fatalf("the exit kept the mode or the picks: selecting=%v selected=%v", st.selecting, st.selected)
	}
	tt.Frame()
	if err := tt.Click("作品 1"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.viewer == nil || a.viewer.work.ID != 1 {
		t.Fatalf("a tile outside selection mode did not open work 1: %+v", a.viewer)
	}
}

// TestSelectionSelectAllDownloadsPicked checks the batch queues the picked
// works after select all, and that download with nothing picked says so.
func TestSelectionSelectAllDownloadsPicked(t *testing.T) {
	a, st, tt := newSelectFeed(t, fakeWorks(0, 3))
	a.downloadsDir = t.TempDir()
	if err := tt.Click("选择"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatalf("select all: %v", err)
	}
	if len(st.selected) != 3 {
		t.Fatalf("selected %d works, want 3", len(st.selected))
	}
	tt.Frame()
	if !tt.HasText("已选 3 项") {
		t.Errorf("the pick count is not shown: %q", tt.Texts())
	}
	if err := tt.Click("下载选中"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 3 {
		t.Fatalf("queued %d tasks, want 3", len(tasks))
	}

	if err := tt.Click("取消全选"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(st.selected) != 0 {
		t.Fatalf("clear kept %d picks", len(st.selected))
	}
	if err := tt.Click("下载选中"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if want := a.t("selection.none"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 3 {
		t.Errorf("queued %d tasks after an empty pick, want 3", len(tasks))
	}
}

// TestSelectionDownloadSkipsUgoira checks a picked animation is not queued and
// the notice points at the viewer, as the loaded-batch button does.
func TestSelectionDownloadSkipsUgoira(t *testing.T) {
	a, _, tt := newSelectFeed(t, []pixiv.Illustration{ugoiraWork(), fakeWorks(0, 1)[0]})
	a.downloadsDir = t.TempDir()
	if err := tt.Click("选择"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatalf("select all: %v", err)
	}
	if err := tt.Click("下载选中"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 1 {
		t.Fatalf("queued %d tasks, want the 1 still image", len(tasks))
	}
	if want := a.tf("selection.ugoiraSkipped", 1); !strings.Contains(a.notice, want) {
		t.Errorf("notice %q does not say %q", a.notice, want)
	}
}

// TestSelectionDownloadWithoutDir checks saving off queues nothing and says
// why.
func TestSelectionDownloadWithoutDir(t *testing.T) {
	a, _, tt := newSelectFeed(t, fakeWorks(0, 2))
	a.downloadsDir = ""
	if err := tt.Click("选择"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatalf("select all: %v", err)
	}
	if err := tt.Click("下载选中"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if want := a.t("viewer.noDownloadDir"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Errorf("queued %d tasks without a directory, want 0", len(tasks))
	}
}

// TestSelectionClearedOnRestart checks a new list starts outside selection
// mode with no picks left from the old one.
func TestSelectionClearedOnRestart(t *testing.T) {
	_, st, tt := newSelectFeed(t, fakeWorks(0, 3))
	if err := tt.Click("选择"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatalf("select all: %v", err)
	}
	if len(st.selected) != 3 {
		t.Fatalf("selected %d works, want 3", len(st.selected))
	}
	st.restart()
	if st.selecting || len(st.selected) != 0 {
		t.Fatalf("the restart kept the mode or the picks: selecting=%v selected=%v", st.selecting, st.selected)
	}
}
