package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/filter"
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
	// focusQuery asks the search box for the keyboard focus on the next
	// frame, which the window's search shortcut sets.
	focusQuery bool

	// filterText is the expression being typed, filterExpr the one applied
	// (nil shows every work) and filterErr why the last apply failed. The
	// view owns these three: only the main thread reads or writes them.
	filterText string
	filterExpr filter.Expr
	filterErr  string

	// masonry is the feed's place, touched only by the view.
	masonry widgets.MasonryState
	// tilesWidth is the column width and tilesText the card text height
	// v.tiles were built for; the view rebuilds them when either changes.
	tilesWidth float64
	tilesText  float64
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
	// Read the width and text height on the main thread; the load goroutine
	// must not touch the settings the view edits.
	width, textHeight := a.masonryWidth(), a.cardText

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
			st.add(page.Items, width, textHeight)
			st.next = page.NextURL
		}
		st.mu.Unlock()

		a.update(func() {})
	}()
}

// add appends a page's works to the feed, with the tiles they draw as. st.mu
// must be held.
func (st *browseState) add(items []pixiv.Illustration, width, textHeight float64) {
	for _, w := range items {
		st.items = append(st.items, w)
		st.tiles = append(st.tiles, widgets.MasonryItem{Key: w.ID, Aspect: cardAspect(w.Aspect(), width, textHeight)})
	}
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
	st.tilesText = 0
	st.next = ""
	st.started = false
	st.err = ""
	st.loading = false
	st.armed = true
	st.mu.Unlock()
}

// cardAspect is a cover's aspect with the title and author under it, so the
// masonry leaves them room.
func cardAspect(aspect, width, textHeight float64) float64 {
	if !(aspect > 0) {
		aspect = 1
	}
	return width / (width/aspect + textHeight)
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
	t := themeOf(c)
	a.filterBar(c, st)
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
	textHeight := cardTextHeight(t)
	a.cardText = textHeight
	items, tiles := feedTiles(st, v.items, v.tiles, width, textHeight)
	switch {
	case len(items) > 0:
		ui.Column(c).Grow(1).Padding(t.SpaceL, contentPadding, contentPadding, contentPadding).Children(func() {
			widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
				a.browseCard(c, st, items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconImage, a.t("common.noWorks"), "", nil)
	}
	// A filtered feed is not paged on past its first page: a strict expression
	// would fetch page after page looking for a match that is not in them.
	if st.canLoad() && (len(v.items) == 0 || st.filterExpr == nil && st.masonry.AtEnd()) {
		a.loadMore(st)
	}
}

// browseCard draws one tile: the cover, its title and the author. A cover not
// loaded yet, or one that failed, is a plain block. The card lifts its border
// and shadow when the pointer is over it.
func (a *App) browseCard(c *ui.Context, st *browseState, items []pixiv.Illustration, i int) {
	work := items[i]
	key := work.CoverURL()
	t := themeOf(c)
	card, cover := cardShell(c)
	card = card.Fill().Focusable().
		Label(a.tf("a11y.card", work.Title, work.User.Name)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openViewer(&items[i], 0)
	}).ContextMenu(func(m *ui.Menu) {
		// Right click opens the author's page: the card's click is the work.
		if m.Item(a.t("user.open")).Chosen() {
			a.openUser(work.User.ID)
		}
	})
	cover.Children(func() {
		if st.imageFailed(key) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.imageReady(st, key)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", work.Title))
		}
	})
	card.Children(func() {
		ui.Column(c).Padding(t.SpaceS, t.SpaceM, t.SpaceM, t.SpaceM).Gap(2).Children(func() {
			ui.Text(c, work.Title).FontSize(t.TextSmall).TextColor(t.Text).MaxLines(1)
			ui.Text(c, work.User.Name).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
		})
	})
}
