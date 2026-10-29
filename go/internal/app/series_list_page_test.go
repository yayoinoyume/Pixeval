package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// seriesWatchlistBodies answers both watchlist kinds with one series each.
func seriesWatchlistBodies() map[string]string {
	return map[string]string{
		"/v1/watchlist/manga": `{"series":[{"id":3,"title":"漫画系列三",` +
			`"user":{"id":7,"name":"画师七"},"url":"https://i/manga.jpg",` +
			`"published_content_count":12}],"next_url":""}`,
		"/v1/watchlist/novel": `{"series":[{"id":4,"title":"小说系列四",` +
			`"user":{"id":8,"name":"作者八"},"url":"https://i/novel.jpg",` +
			`"published_content_count":5}],"next_url":""}`,
	}
}

// newSeriesListTest is a signed-in app on the series watchlist whose API
// answers from bodies, without images or a network.
func newSeriesListTest(bodies map[string]string) (*App, *ui.Tester) {
	a := newPageAppWithBodies(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	a.route = routeSeriesList
	return a, ui.NewTester(a.View, 1000, 700)
}

// waitSeriesList waits for a series list to settle as want says.
func waitSeriesList(t *testing.T, st *seriesListState, want func(v seriesListView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("series list did not settle: series=%d loading=%v err=%q",
				len(v.items), v.loading, v.err)
		}
		time.Sleep(time.Millisecond)
	}
}

// seriesRequested reports whether the app asked for path with the filter.
func seriesRequested(tr *pageTransport, path string) bool {
	for _, u := range tr.requestedURLs() {
		if strings.HasPrefix(u, path) && strings.Contains(u, "filter=for_android") {
			return true
		}
	}
	return false
}

// TestSeriesListTabsRequestEachKind checks the manga and novel tabs ask their
// own watchlist endpoint and draw the series' title and author.
func TestSeriesListTabsRequestEachKind(t *testing.T) {
	a, tt := newSeriesListTest(seriesWatchlistBodies())
	tt.Frame()
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	tt.Frame()
	for _, want := range []string{a.t("series.watchlistTitle"), "漫画系列三", "画师七"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if got := a.seriesFeed(seriesMangaTab).snapshot().items[0].CoverURL; got != "https://i/manga.jpg" {
		t.Errorf("manga cover = %q, want the url field", got)
	}

	if err := tt.Click(a.t("common.novel")); err != nil {
		t.Fatalf("Click novel tab: %v", err)
	}
	waitSeriesList(t, a.seriesFeed(seriesNovelTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	tt.Frame()
	for _, want := range []string{"小说系列四", "作者八"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if got := a.seriesFeed(seriesNovelTab).snapshot().items[0].CoverURL; got != "https://i/novel.jpg" {
		t.Errorf("novel cover = %q, want the url field", got)
	}

	tr := a.client.HTTP().Transport.(*pageTransport)
	for _, path := range []string{"/v1/watchlist/manga", "/v1/watchlist/novel"} {
		if !seriesRequested(tr, path) {
			t.Errorf("no filtered request for %s: %v", path, tr.requestedURLs())
		}
	}
}

// TestSeriesListEmpty checks an empty watchlist shows its own empty state.
func TestSeriesListEmpty(t *testing.T) {
	bodies := map[string]string{
		"/v1/watchlist/manga": `{"series":[],"next_url":""}`,
		"/v1/watchlist/novel": `{"series":[],"next_url":""}`,
	}
	a, tt := newSeriesListTest(bodies)
	tt.Frame()
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return v.started && len(v.items) == 0
	})
	tt.Frame()
	for _, want := range []string{a.t("series.watchlistEmpty"), a.t("series.watchlistEmptyHint"), a.t("user.myPixivEmptyAction")} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(a.t("user.myPixivEmptyAction")); err != nil {
		t.Fatalf("Click the empty state's action: %v", err)
	}
	if a.route != routeSearch {
		t.Errorf("route %q after the action, want %q", a.route, routeSearch)
	}
}

// TestSeriesListFailureRetry checks a failed watchlist shows the reason and
// loads again from the retry button.
func TestSeriesListFailureRetry(t *testing.T) {
	a := newPageAppWithBodies(nil)
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	a.route = routeSeriesList
	calls := 0
	a.seriesFeed(seriesMangaTab).setFetch(func(context.Context) (*pixiv.Page[pixiv.SeriesSummary], error) {
		calls++
		if calls == 1 {
			return nil, errors.New("boom")
		}
		return &pixiv.Page[pixiv.SeriesSummary]{Items: []pixiv.SeriesSummary{{
			ID: 3, Title: "系列三", User: pixiv.User{Name: "画师七"},
		}}}, nil
	})
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return v.err != ""
	})
	tt.Frame()
	if !tt.HasText(a.tf("series.loadFailed", "boom")) {
		t.Fatalf("the failure is missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("common.retry")); err != nil {
		t.Fatalf("Click retry: %v", err)
	}
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	tt.Frame()
	if !tt.HasText("系列三") {
		t.Errorf("the retried series is missing: %q", tt.Texts())
	}
	if calls != 2 {
		t.Errorf("fetch called %d times, want 2", calls)
	}
}

// TestSeriesListMangaCardOpensSeries checks clicking a manga series opens the
// existing series page.
func TestSeriesListMangaCardOpensSeries(t *testing.T) {
	a, tt := newSeriesListTest(seriesWatchlistBodies())
	tt.Frame()
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	tt.Frame()
	card := a.tf("a11y.card", "漫画系列三", "画师七")
	if err := tt.Click(card); err != nil {
		t.Fatalf("Click %q: %v", card, err)
	}
	if a.route != routeSeries {
		t.Errorf("route %q after the click, want %q", a.route, routeSeries)
	}
	if a.seriesView == nil {
		t.Fatal("the series page did not open")
	}
	waitSeries(t, a, func(v seriesPageView) bool { return !v.loading })
}

// TestSeriesListPages checks the next page is requested from the watchlist's
// own next_url and its series are appended.
func TestSeriesListPages(t *testing.T) {
	bodies := seriesWatchlistBodies()
	bodies["/v1/watchlist/manga"] = `{"series":[{"id":3,"title":"漫画系列三",` +
		`"user":{"id":7,"name":"画师七"},"url":"https://i/manga.jpg",` +
		`"published_content_count":12}],` +
		`"next_url":"https://app-api.pixiv.net/v1/watchlist/manga?offset=1"}`
	a, tt := newSeriesListTest(bodies)
	tr := a.client.HTTP().Transport.(*pageTransport)
	tr.byURL = map[string]string{
		"/v1/watchlist/manga?offset=1": `{"series":[{"id":5,"title":"漫画系列五",` +
			`"user":{"id":7,"name":"画师七"},"url":"https://i/manga5.jpg",` +
			`"published_content_count":2}],"next_url":""}`,
	}
	tt.Frame()
	deadline := time.Now().Add(2 * time.Second)
	for len(a.seriesFeed(seriesMangaTab).snapshot().items) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("series list has %d series, want 2",
				len(a.seriesFeed(seriesMangaTab).snapshot().items))
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if !tt.HasText("漫画系列五") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
	paged := false
	for _, u := range tr.requestedURLs() {
		if strings.Contains(u, "/v1/watchlist/manga?offset=1") {
			paged = true
			break
		}
	}
	if !paged {
		t.Errorf("no request for the next_url: %v", tr.requestedURLs())
	}
}

// TestSeriesListNovelCardExplainsUnsupported checks clicking a novel series
// leaves the list and says the novel series page is not there yet.
func TestSeriesListNovelCardExplainsUnsupported(t *testing.T) {
	a, tt := newSeriesListTest(seriesWatchlistBodies())
	tt.Frame()
	waitSeriesList(t, a.seriesFeed(seriesMangaTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	if err := tt.Click(a.t("common.novel")); err != nil {
		t.Fatalf("Click novel tab: %v", err)
	}
	waitSeriesList(t, a.seriesFeed(seriesNovelTab), func(v seriesListView) bool {
		return len(v.items) == 1
	})
	tt.Frame()
	card := a.tf("a11y.card", "小说系列四", "作者八")
	if err := tt.Click(card); err != nil {
		t.Fatalf("Click %q: %v", card, err)
	}
	if a.route != routeSeriesList {
		t.Errorf("route %q after the click, want %q", a.route, routeSeriesList)
	}
	want := a.t("series.novelUnsupported")
	if a.notice != want && !tt.HasText(want) {
		t.Errorf("notice %q neither queued nor shown: %q", want, tt.Texts())
	}
}
