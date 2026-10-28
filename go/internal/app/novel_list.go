package app

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// The kinds of result the search page shows, in the order its tabs show them.
const (
	searchIllust = iota
	searchNovel
)

// novelFeedView is what the view draws of a novel list, read in one go. The
// slices are the state's own: callers read them and must not change them.
type novelFeedView struct {
	items   []pixiv.Novel
	tiles   []widgets.MasonryItem
	loading bool
	started bool
	err     string
}

// novelFeedState is the novel list of a search: the novels loaded so far, the
// cursor of the next page, and the masonry's place. The view runs on the main
// thread; the load goroutine changes the fields under mu.
type novelFeedState struct {
	mu      sync.Mutex
	items   []pixiv.Novel
	tiles   []widgets.MasonryItem
	next    string
	loading bool
	// started is set once a page loaded, so an empty next ends the list
	// instead of asking for the first page again.
	started bool
	err     string
	// gen identifies the list the load in flight belongs to: a new search
	// starts the next one and drops the results of the old.
	gen   int
	fetch func(ctx context.Context) (*pixiv.Page[pixiv.Novel], error)
	// failed holds the covers that did not load, which are not asked for
	// again.
	failed map[string]bool

	// armed is set when a page may load: once the search page has a search
	// for the novel tab to show.
	armed bool

	// masonry is the list's place, and tilesWidth the column width its tiles
	// were built for; the view rebuilds them when the setting changes.
	masonry    widgets.MasonryState
	tilesWidth float64
}

// snapshot returns what the view draws.
func (st *novelFeedState) snapshot() novelFeedView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return novelFeedView{
		items:   st.items,
		tiles:   st.tiles,
		loading: st.loading,
		started: st.started,
		err:     st.err,
	}
}

// cursor is the next page's URL, empty for the first.
func (st *novelFeedState) cursor() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.next
}

func (st *novelFeedState) setFetch(fetch func(ctx context.Context) (*pixiv.Page[pixiv.Novel], error)) {
	st.mu.Lock()
	st.fetch = fetch
	st.mu.Unlock()
}

// arm allows the list to load.
func (st *novelFeedState) arm() {
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()
}

func (st *novelFeedState) isArmed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed
}

// canLoad reports whether another page may be fetched: not while one loads,
// not after a failure (the retry button clears it), and not once the list
// ended.
func (st *novelFeedState) canLoad() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed && !st.loading && st.err == "" && (!st.started || st.next != "")
}

// restart drops the loaded novels for a new search, and drops the page a load
// still in flight returns.
func (st *novelFeedState) restart() {
	st.mu.Lock()
	st.gen++
	st.items = nil
	st.tiles = nil
	st.next = ""
	st.started = false
	st.err = ""
	st.loading = false
	st.armed = true
	st.mu.Unlock()
}

// retry clears a failure, so the next frame loads the page again.
func (st *novelFeedState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// add appends a page's novels to the list, with the tiles they draw as. st.mu
// must be held.
func (st *novelFeedState) add(items []pixiv.Novel, width float64) {
	st.items = append(st.items, items...)
	st.tiles = append(st.tiles, novelTiles(items, width)...)
}

// novelTiles are the tiles of items at the column width width.
func novelTiles(items []pixiv.Novel, width float64) []widgets.MasonryItem {
	tiles := make([]widgets.MasonryItem, len(items))
	for i := range items {
		tiles[i] = widgets.MasonryItem{Key: items[i].ID, Aspect: cardAspect(items[i].Aspect(), width)}
	}
	return tiles
}

// tilesFor returns the tiles of items at width, and keeps them for the next
// frame.
func (st *novelFeedState) tilesFor(items []pixiv.Novel, width float64) []widgets.MasonryItem {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.tilesWidth = width
	st.tiles = novelTiles(items, width)
	return st.tiles
}

// imageFailed reports whether a cover failed to load.
func (st *novelFeedState) imageFailed(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.failed[key]
}

func (st *novelFeedState) failImage(key string) {
	st.mu.Lock()
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	st.failed[key] = true
	st.mu.Unlock()
}

// novelImageReady returns the callback a cover load calls, which keeps a broken
// URL from being asked for again every frame.
func (a *App) novelImageReady(st *novelFeedState, key string) func() {
	return func() {
		if a.images.Thumb(key, coverWidth, coverHeight, nil) == nil {
			st.failImage(key)
		}
		a.update(func() {})
	}
}

// startNovelSearch replaces the novel list with a search for word. The page is
// not fetched here: it loads when the novel tab first draws it.
func (a *App) startNovelSearch(word string) {
	st := a.searchNovels
	st.restart()
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.Novel], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextNovels(ctx, next)
		}
		return a.client.SearchNovels(ctx, pixiv.SearchOptions{Word: word})
	})
	st.masonry = widgets.MasonryState{}
	st.tilesWidth = 0
}

// loadMoreNovels fetches the next page of novels off the main thread and
// redraws with it.
func (a *App) loadMoreNovels(st *novelFeedState) {
	st.mu.Lock()
	if !st.armed || st.loading || st.err != "" || st.started && st.next == "" {
		st.mu.Unlock()
		return
	}
	st.loading = true
	gen, fetch := st.gen, st.fetch
	st.mu.Unlock()
	// Read the width on the main thread; the load goroutine must not touch
	// the settings the view edits.
	width := a.masonryWidth()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		page, err := fetch(ctx)

		st.mu.Lock()
		if gen != st.gen {
			// A new search replaced this list; its page is not wanted.
			st.mu.Unlock()
			return
		}
		st.loading = false
		switch {
		case errors.Is(err, pixiv.ErrNoMore):
			st.next = ""
			st.started = true
		case err != nil:
			st.err = err.Error()
		default:
			st.started = true
			st.add(page.Items, width)
			st.next = page.NextURL
		}
		st.mu.Unlock()

		a.update(func() {})
	}()
}

// novelResults draws the novel list, and loads its next page once it is at the
// end.
func (a *App) novelResults(c *ui.Context) {
	t := themeOf(c)
	st := a.searchNovels
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, contentPadding).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(18, 18).TextColor(t.Danger)
			ui.Text(c, a.tf("common.loadFailed", v.err)).FontSize(t.TextSmall).
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
	tiles := v.tiles
	if width != st.tilesWidth {
		tiles = st.tilesFor(v.items, width)
	}
	switch {
	case len(v.items) > 0:
		ui.Column(c).Grow(1).Padding(t.SpaceL, contentPadding, contentPadding, contentPadding).Children(func() {
			widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
				a.novelCard(c, st, v.items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconImage, a.t("common.noWorks"), "", nil)
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMoreNovels(st)
	}
}

// novelCard draws one novel: the cover, its title, author and page count. A
// cover not loaded yet, or one that failed, is a plain block. The card lifts
// its border and shadow when the pointer is over it.
func (a *App) novelCard(c *ui.Context, st *novelFeedState, items []pixiv.Novel, i int) {
	novel := items[i]
	key := novel.CoverURL()
	t := themeOf(c)
	card, cover := cardShell(c)
	card = card.Fill().Focusable().
		Label(a.tf("a11y.card", novel.Title, novel.User.Name)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openNovel(novel.ID, novel.Title)
	})
	cover.Children(func() {
		if st.imageFailed(key) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.novelImageReady(st, key)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", novel.Title))
		}
	})
	card.Children(func() {
		ui.Column(c).Padding(t.SpaceS, t.SpaceM, t.SpaceM, t.SpaceM).Gap(2).Children(func() {
			ui.Text(c, novel.Title).FontSize(t.TextSmall).TextColor(t.Text).MaxLines(1)
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, novel.User.Name).FontSize(t.TextTiny).TextColor(t.TextMuted).
					MaxLines(1).Grow(1)
				ui.Text(c, a.tf("novel.pages", novel.PageCount)).FontSize(t.TextTiny).
					TextColor(t.TextMuted)
			})
		})
	})
}
