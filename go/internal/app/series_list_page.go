package app

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// The series watchlist tabs, in the order textTabs shows them.
const (
	seriesMangaTab = iota
	seriesNovelTab
	seriesTabCount
)

// seriesKind is the watchlist kind a tab stands for.
func seriesKind(tab int) pixiv.SeriesKind {
	if tab == seriesNovelTab {
		return pixiv.NovelSeries
	}
	return pixiv.MangaSeries
}

// seriesListView is what the view draws of a series watchlist, read in one go.
// The slices are the state's own: callers read them and must not change them.
type seriesListView struct {
	items   []pixiv.SeriesSummary
	tiles   []widgets.MasonryItem
	loading bool
	started bool
	err     string
}

// seriesListState is one series watchlist: the series loaded so far, the
// cursor of the next page, and the masonry's place. The view runs on the main
// thread; the load goroutine changes the fields under mu.
type seriesListState struct {
	mu      sync.Mutex
	items   []pixiv.SeriesSummary
	tiles   []widgets.MasonryItem
	next    string
	loading bool
	// started is set once a page loaded, so an empty next ends the list
	// instead of asking for the first page again.
	started bool
	err     string
	fetch   func(ctx context.Context) (*pixiv.Page[pixiv.SeriesSummary], error)
	// failed holds the covers that did not load, which are not asked for
	// again.
	failed map[string]bool

	// masonry is the list's place; tilesWidth and tilesText are what its tiles
	// were built for, so the view rebuilds them when either changes.
	masonry    widgets.MasonryState
	tilesWidth float64
	tilesText  float64
}

func (st *seriesListState) snapshot() seriesListView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return seriesListView{
		items:   st.items,
		tiles:   st.tiles,
		loading: st.loading,
		started: st.started,
		err:     st.err,
	}
}

// cursor is the next page's URL, empty for the first.
func (st *seriesListState) cursor() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.next
}

func (st *seriesListState) setFetch(fetch func(ctx context.Context) (*pixiv.Page[pixiv.SeriesSummary], error)) {
	st.mu.Lock()
	st.fetch = fetch
	st.mu.Unlock()
}

// canLoad reports whether another page may be fetched: not while one loads,
// not after a failure (the retry button clears it), and not once the list
// ended.
func (st *seriesListState) canLoad() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.fetch != nil && !st.loading && st.err == "" && (!st.started || st.next != "")
}

// retry clears a failure, so the next frame loads the page again.
func (st *seriesListState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// add appends a page's series to the list, with the tiles they draw as. st.mu
// must be held.
func (st *seriesListState) add(items []pixiv.SeriesSummary, width, textHeight float64) {
	st.items = append(st.items, items...)
	st.tiles = append(st.tiles, seriesTiles(items, width, textHeight)...)
}

// seriesTiles are the tiles of items at the column width width.
func seriesTiles(items []pixiv.SeriesSummary, width, textHeight float64) []widgets.MasonryItem {
	tiles := make([]widgets.MasonryItem, len(items))
	for i := range items {
		tiles[i] = widgets.MasonryItem{Key: items[i].ID, Aspect: cardAspect(seriesCoverAspect, width, textHeight)}
	}
	return tiles
}

// seriesCoverAspect is the cover shape a series card assumes.
const seriesCoverAspect = 3.0 / 4.0

// tilesFor returns the tiles of items at width, and keeps them for the next
// frame.
func (st *seriesListState) tilesFor(items []pixiv.SeriesSummary, width, textHeight float64) []widgets.MasonryItem {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.tilesWidth, st.tilesText = width, textHeight
	st.tiles = seriesTiles(items, width, textHeight)
	return st.tiles
}

// imageFailed reports whether a cover failed to load.
func (st *seriesListState) imageFailed(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.failed[key]
}

func (st *seriesListState) failImage(key string) {
	st.mu.Lock()
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	st.failed[key] = true
	st.mu.Unlock()
}

// seriesImageReady returns the callback a cover load calls, which keeps a
// broken URL from being asked for again every frame.
func (a *App) seriesImageReady(st *seriesListState, key string) func() {
	return func() {
		if a.images.Thumb(key, coverWidth, coverHeight, nil) == nil {
			st.failImage(key)
		}
		a.update(func() {})
	}
}

// seriesFeed is the watchlist tab's list, made on first use.
func (a *App) seriesFeed(tab int) *seriesListState {
	if a.seriesFeeds[tab] == nil {
		a.seriesFeeds[tab] = &seriesListState{}
	}
	return a.seriesFeeds[tab]
}

// armSeriesFeed gives a tab its page fetcher once: the kind's first page, then
// the cursor each later page returns.
func (a *App) armSeriesFeed(st *seriesListState, tab int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.fetch != nil {
		return
	}
	kind := seriesKind(tab)
	st.fetch = func(ctx context.Context) (*pixiv.Page[pixiv.SeriesSummary], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextSeriesWatchlist(ctx, next)
		}
		return a.client.SeriesWatchlist(ctx, kind)
	}
}

// loadMoreSeries fetches the next page of series off the main thread and
// redraws with it.
func (a *App) loadMoreSeries(st *seriesListState) {
	st.mu.Lock()
	if st.fetch == nil || st.loading || st.err != "" || st.started && st.next == "" {
		st.mu.Unlock()
		return
	}
	st.loading = true
	fetch := st.fetch
	st.mu.Unlock()
	// Read the width and text height on the main thread; the load goroutine
	// must not touch the settings the view edits.
	width, textHeight := a.masonryWidth(), a.cardText

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		page, err := fetch(ctx)

		st.mu.Lock()
		st.loading = false
		switch {
		case errors.Is(err, pixiv.ErrNoMore):
			st.next = ""
			st.started = true
		case err != nil:
			st.err = err.Error()
		default:
			st.started = true
			st.add(page.Items, width, textHeight)
			st.next = page.NextURL
		}
		st.mu.Unlock()

		a.update(func() {})
	}()
}

// seriesListPage draws the watchlist of the chosen kind: manga or novel
// series.
func (a *App) seriesListPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	t := themeOf(c)
	ui.Row(c).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
		textTabs(c, t, &a.seriesTab, a.t("common.manga"), a.t("common.novel"))
	})
	st := a.seriesFeed(a.seriesTab)
	a.armSeriesFeed(st, a.seriesTab)
	a.seriesFeedPage(c, st)
}

// seriesFeedPage draws one watchlist, and loads its next page once it is at
// the end.
func (a *App) seriesFeedPage(c *ui.Context, st *seriesListState) {
	t := themeOf(c)
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, contentPadding).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(18, 18).TextColor(t.Danger)
			ui.Text(c, a.tf("series.loadFailed", v.err)).FontSize(t.TextSmall).
				TextColor(t.Danger).Grow(1)
			if ui.Button(c, a.t("common.retry")).Clicked() {
				st.retry()
			}
		})
		if len(v.items) == 0 {
			// Nothing to show under the error, and no page to load.
			return
		}
	}
	width := a.masonryWidth()
	textHeight := cardTextHeight(t)
	a.cardText = textHeight
	tiles := v.tiles
	if width != st.tilesWidth || textHeight != st.tilesText {
		tiles = st.tilesFor(v.items, width, textHeight)
	}
	switch {
	case len(v.items) > 0:
		ui.Column(c).Grow(1).Padding(t.SpaceL, contentPadding, contentPadding, contentPadding).Children(func() {
			widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
				a.seriesCard(c, st, v.items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconSeries, a.t("series.watchlistEmpty"), a.t("series.watchlistEmptyHint"), func() {
			if ui.Button(c, a.t("user.myPixivEmptyAction")).Radius(t.RadiusS).Clicked() {
				a.goRoute(routeSearch)
			}
		})
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMoreSeries(st)
	}
}

// seriesCard draws one series: the cover, its title, author and published
// count. A cover not loaded yet, or one that failed, is a plain block.
func (a *App) seriesCard(c *ui.Context, st *seriesListState, items []pixiv.SeriesSummary, i int) {
	s := items[i]
	key := s.CoverURL
	t := themeOf(c)
	card, cover := cardShell(c)
	card = card.Fill().Focusable().
		Label(a.tf("a11y.card", s.Title, s.User.Name)).
		Cursor(ui.CursorPointer).OnClick(func() {
		if a.seriesTab == seriesNovelTab {
			// The reader has no novel series page yet; say so instead of
			// opening a page that would show the wrong thing.
			a.notice = a.t("series.novelUnsupported")
			return
		}
		a.openSeries(s.ID)
	})
	cover.Children(func() {
		if st.imageFailed(key) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.seriesImageReady(st, key)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", s.Title))
		}
	})
	card.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, s.Title).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).MaxLines(1)
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, s.User.Name).FontSize(t.TextTiny).TextColor(t.TextMuted).
					MaxLines(1).Grow(1)
				ui.Text(c, a.tf("series.worksCount", s.PublishedContentCount)).FontSize(t.TextTiny).
					TextColor(t.TextMuted)
			})
		})
	})
}
