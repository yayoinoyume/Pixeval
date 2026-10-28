package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// browseTimeout bounds one page fetch, so a stuck request cannot hold the
// feed loading forever.
const browseTimeout = 30 * time.Second

const (
	cardWidth   = 240
	cardGap     = 12
	coverWidth  = 480
	coverHeight = 720
	// cardText is the height the title and author take under a cover.
	cardText = 44
)

// browseView is what the view draws, read from the state in one go. The
// slices are the state's own: callers read them and must not change them.
type browseView struct {
	items   []pixiv.Illustration
	tiles   []widgets.MasonryItem
	loading bool
	started bool
	err     string
}

// browseState is one feed: the works loaded so far, the cursor of the next
// page, and the masonry's place. The view runs on the main thread; the load
// goroutine changes the fields under mu.
type browseState struct {
	mu      sync.Mutex
	items   []pixiv.Illustration
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
	fetch func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error)
	// failed holds the covers that did not load, which are not asked for
	// again: a failed load calls back too, so retrying on every redraw
	// would spin on a broken URL.
	failed map[string]bool

	// query is the search box's text, edited on the main thread. armed is
	// set when a page may load: once the window runs for the home feed, on
	// the first submitted word for search.
	query string
	armed bool

	// masonry is the feed's place, touched only by the view.
	masonry widgets.MasonryState
	// tilesWidth is the column width v.tiles were built for; the view rebuilds
	// them when the setting changes.
	tilesWidth float64
}

// homeTabCount is the number of home tabs, one feed each.
const homeTabCount = 3

// The home tabs, in the order ui.Tabs shows them: recommended, following and
// ranking.
const (
	homeTabFollowing = iota + 1
	homeTabRanking
)

// newHomeFeeds makes one empty feed per home tab.
func newHomeFeeds() [homeTabCount]*browseState {
	feeds := [homeTabCount]*browseState{}
	for i := range feeds {
		feeds[i] = &browseState{}
	}
	return feeds
}

// armHome sets a home tab's fetch and lets it load, once. The first page is
// fetched as the tab draws it; a tab already armed keeps its pages and place.
func (a *App) armHome(tab int) {
	st := a.homeFeeds[tab]
	if st.isArmed() {
		return
	}
	st.setFetch(a.homeFetch(st, tab))
	st.arm()
}

// homeFetch maps a home tab to its first page. Later pages come from the
// feed's cursor, which each feed reads under mu.
func (a *App) homeFetch(st *browseState, tab int) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		switch tab {
		case homeTabFollowing:
			return a.client.Following(ctx)
		case homeTabRanking:
			return a.client.Ranking(ctx, pixiv.Day, "")
		default: // tab 0, recommended
			return a.client.Recommended(ctx)
		}
	}
}

// cursor is the next page's URL, empty for the first.
func (st *browseState) cursor() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.next
}

func (st *browseState) setFetch(fetch func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error)) {
	st.mu.Lock()
	st.fetch = fetch
	st.mu.Unlock()
}

// arm allows the feed to load, once the window can draw it.
func (st *browseState) arm() {
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()
}

func (st *browseState) isArmed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed
}

// snapshot returns what the view draws.
func (st *browseState) snapshot() browseView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return browseView{
		items:   st.items,
		tiles:   st.tiles,
		loading: st.loading,
		started: st.started,
		err:     st.err,
	}
}

// canLoad reports whether another page may be fetched: not while one loads,
// not after a failure (the retry button clears it), and not once the list
// ended.
func (st *browseState) canLoad() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed && !st.loading && st.err == "" && (!st.started || st.next != "")
}

// loadMore fetches the next page off the main thread and redraws with it.
func (a *App) loadMore(st *browseState) {
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
			for _, w := range page.Items {
				st.items = append(st.items, w)
				st.tiles = append(st.tiles, widgets.MasonryItem{Key: w.ID, Aspect: cardAspect(w.Aspect(), width)})
			}
			st.next = page.NextURL
		}
		st.mu.Unlock()

		a.update(func() {})
	}()
}

// retry clears a failure, so the next frame loads the page again.
func (st *browseState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// restart drops the loaded pages for a new list, and drops the page a load
// still in flight returns.
func (st *browseState) restart() {
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

// startSearch replaces the results with a search for the box's word.
func (a *App) startSearch(st *browseState) {
	word := strings.TrimSpace(st.query)
	if word == "" {
		return
	}
	st.restart()
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		return a.client.SearchIllustrations(ctx, pixiv.SearchOptions{Word: word})
	})
	st.masonry = widgets.MasonryState{}
	a.loadMore(st)
}

// cardAspect is a cover's aspect with the title and author under it, so the
// masonry leaves them room.
func cardAspect(aspect, width float64) float64 {
	if !(aspect > 0) {
		aspect = 1
	}
	return width / (width/aspect + cardText)
}

// imageFailed reports whether a cover failed to load.
func (st *browseState) imageFailed(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.failed[key]
}

func (st *browseState) failImage(key string) {
	st.mu.Lock()
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	st.failed[key] = true
	st.mu.Unlock()
}

// imageReady returns the callback a cover load calls. Thumb caches a decoded
// image before it calls back, so a probe that finds nothing means the load
// failed. The probe starts one more load of a broken URL, which has no
// callback and so ends there; the cover is a placeholder from then on.
func (a *App) imageReady(st *browseState, key string) func() {
	return func() {
		if a.images.Thumb(key, coverWidth, coverHeight, nil) == nil {
			st.failImage(key)
		}
		a.update(func() {})
	}
}

// browsePage draws a feed, and loads its next page once it is at the end.
func (a *App) browsePage(c *ui.Context, st *browseState) {
	t := c.Theme()
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tf("common.loadFailed", v.err)).TextColor(t.Danger).Grow(1)
			if ui.Button(c, a.t("common.retry")).Clicked() {
				st.retry()
			}
		})
		if len(v.items) == 0 {
			// Nothing to show under the error, and no page to load.
			return
		}
	}
	if len(v.items) == 0 {
		msg := a.t("common.noWorks")
		if !v.started {
			msg = a.t("common.loading")
		}
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, msg).TextColor(t.TextMuted)
		})
	} else {
		width := a.masonryWidth()
		tiles := v.tiles
		if width != st.tilesWidth {
			st.tilesWidth = width
			tiles = make([]widgets.MasonryItem, len(v.items))
			for i, w := range v.items {
				tiles[i] = widgets.MasonryItem{Key: w.ID, Aspect: cardAspect(w.Aspect(), width)}
			}
		}
		widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
			a.browseCard(c, st, v.items, i)
		})
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMore(st)
	}
}

// browseCard draws one tile: the cover, its title and the author. A cover not
// loaded yet, or one that failed, is a plain block.
func (a *App) browseCard(c *ui.Context, st *browseState, items []pixiv.Illustration, i int) {
	work := items[i]
	key := work.CoverURL()
	t := c.Theme()
	ui.Column(c).Fill().Gap(4).Padding(2).Cursor(ui.CursorPointer).OnClick(func() {
		a.openViewer(&items[i], 0)
	}).Children(func() {
		ui.Box(c).Grow(1).FillWidth().Radius(8).Background(t.Surface).Clip().Children(func() {
			if st.imageFailed(key) {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, a.t("common.coverFailed")).FontSize(12).TextColor(t.TextMuted)
				})
				return
			}
			if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.imageReady(st, key)); bmp != nil {
				ui.Image(c, bmp).Fill()
			}
		})
		ui.Text(c, work.Title).MaxLines(1)
		ui.Text(c, work.User.Name).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
	})
}

// searchPage draws the search box and its results.
func (a *App) searchPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	st := a.search
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(8).Padding(12).Children(func() {
			ui.TextInput(c, &st.query).Grow(1).Placeholder(a.t("search.placeholder"))
			if ui.Button(c, a.t("search.action")).Clicked() {
				a.startSearch(st)
			}
		})
		if !st.isArmed() {
			t := c.Theme()
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, a.t("search.hint")).TextColor(t.TextMuted)
			})
			return
		}
		a.browsePage(c, st)
	})
}
