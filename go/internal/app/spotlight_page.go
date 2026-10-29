package app

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// spotlightCoverAspect is the shape of a Pixivision cover: its 1200x630 card.
const spotlightCoverAspect = 1200.0 / 630.0

// spotlightView is what the view draws, read from the state in one go. The
// slices are the state's own: callers read them and must not change them.
type spotlightView struct {
	items   []pixiv.SpotlightArticle
	tiles   []widgets.MasonryItem
	loading bool
	started bool
	err     string
}

// spotlightState is the spotlight list: the articles loaded so far, the
// cursor of the next page, and the masonry's place. The view runs on the main
// thread; the load goroutine changes the fields under mu.
type spotlightState struct {
	mu      sync.Mutex
	items   []pixiv.SpotlightArticle
	tiles   []widgets.MasonryItem
	next    string
	loading bool
	// started is set once a page loaded, so an empty next ends the list
	// instead of asking for the first page again.
	started bool
	err     string
	// gen identifies the list the load in flight belongs to: a restart drops
	// the results of the old.
	gen   int
	fetch func(ctx context.Context) (*pixiv.Page[pixiv.SpotlightArticle], error)
	// failed holds the covers that did not load, which are not asked for
	// again.
	failed map[string]bool
	// armed is set when a page may load: once the page is first drawn.
	armed bool

	// masonry is the list's place; tilesWidth and tilesText are what its tiles
	// were built for, so the view rebuilds them when either changes.
	masonry    widgets.MasonryState
	tilesWidth float64
	tilesText  float64
}

// snapshot returns what the view draws.
func (st *spotlightState) snapshot() spotlightView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return spotlightView{
		items:   st.items,
		tiles:   st.tiles,
		loading: st.loading,
		started: st.started,
		err:     st.err,
	}
}

// cursor is the next page's URL, empty for the first.
func (st *spotlightState) cursor() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.next
}

func (st *spotlightState) setFetch(fetch func(ctx context.Context) (*pixiv.Page[pixiv.SpotlightArticle], error)) {
	st.mu.Lock()
	st.fetch = fetch
	st.mu.Unlock()
}

func (st *spotlightState) arm() {
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()
}

func (st *spotlightState) isArmed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed
}

// canLoad reports whether another page may be fetched: not while one loads,
// not after a failure (the retry button clears it), and not once the list
// ended.
func (st *spotlightState) canLoad() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed && !st.loading && st.err == "" && (!st.started || st.next != "")
}

// retry clears a failure, so the next frame loads the page again.
func (st *spotlightState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// add appends a page's articles with the tiles they draw as. st.mu must be
// held.
func (st *spotlightState) add(items []pixiv.SpotlightArticle, width, textHeight float64) {
	st.items = append(st.items, items...)
	st.tiles = append(st.tiles, spotlightTiles(items, width, textHeight)...)
}

// spotlightTiles are the tiles of items at the column width width.
func spotlightTiles(items []pixiv.SpotlightArticle, width, textHeight float64) []widgets.MasonryItem {
	tiles := make([]widgets.MasonryItem, len(items))
	for i := range items {
		tiles[i] = widgets.MasonryItem{
			Key:    items[i].ID,
			Aspect: cardAspect(spotlightCoverAspect, width, textHeight),
		}
	}
	return tiles
}

// tilesFor returns the tiles of items at width, and keeps them for the next
// frame.
func (st *spotlightState) tilesFor(items []pixiv.SpotlightArticle, width, textHeight float64) []widgets.MasonryItem {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.tilesWidth, st.tilesText = width, textHeight
	st.tiles = spotlightTiles(items, width, textHeight)
	return st.tiles
}

// imageFailed reports whether a cover failed to load.
func (st *spotlightState) imageFailed(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.failed[key]
}

func (st *spotlightState) failImage(key string) {
	st.mu.Lock()
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	st.failed[key] = true
	st.mu.Unlock()
}

// spotlightImageReady returns the callback a cover load calls, which keeps a
// broken URL from being asked for again every frame.
func (a *App) spotlightImageReady(st *spotlightState, key string) func() {
	return func() {
		if a.images.Thumb(key, coverWidth, coverHeight, nil) == nil {
			st.failImage(key)
		}
		a.update(func() {})
	}
}

// armSpotlight gives the list its page fetcher, once.
func (a *App) armSpotlight() {
	st := a.spotlight
	if st.isArmed() {
		return
	}
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.SpotlightArticle], error) {
		return a.spotlightFn(ctx, st.cursor())
	})
	st.arm()
}

// loadMoreSpotlight fetches the next page off the main thread and redraws with
// it.
func (a *App) loadMoreSpotlight(st *spotlightState) {
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
			// A restart replaced this list; its page is not wanted.
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

// spotlightPage draws the Pixivision articles Pixiv features. Pixiv wants a
// signed-in account for the list, as the original app does.
func (a *App) spotlightPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.armSpotlight()
	a.spotlightFeed(c)
}

// spotlightFeed draws the article list, and loads its next page at the end.
func (a *App) spotlightFeed(c *ui.Context) {
	t := themeOf(c)
	st := a.spotlight
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, contentPadding).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(18, 18).TextColor(t.Danger)
			ui.Text(c, a.tf("spotlight.loadFailed", v.err)).FontSize(t.TextSmall).
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
				a.spotlightCard(c, st, v.items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconImage, a.t("spotlight.empty"), "", nil)
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMoreSpotlight(st)
	}
}

// spotlightCard draws one article: its cover, title and publication date.
// Clicking it opens the article in the browser.
func (a *App) spotlightCard(c *ui.Context, st *spotlightState, items []pixiv.SpotlightArticle, i int) {
	article := items[i]
	key := article.Thumbnail
	t := themeOf(c)
	card, cover := cardShell(c)
	card = card.Fill().Focusable().Label(article.Title).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openSpotlight(c, article.ArticleURL)
	})
	cover.Children(func() {
		if key == "" {
			return // an article without a cover has no image to draw
		}
		if st.imageFailed(key) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.spotlightImageReady(st, key)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", article.Title))
		}
	})
	card.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, article.Title).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).MaxLines(1)
			ui.Text(c, spotlightDate(article.PublishDate)).FontSize(t.TextTiny).TextColor(t.TextMuted)
		})
	})
}

// openSpotlight opens an article in the default browser.
func (a *App) openSpotlight(c *ui.Context, url string) {
	if url == "" {
		return
	}
	open := a.openExternalFn
	if open == nil {
		open = mygo.Shell.OpenExternal
	}
	if err := open(url); err != nil {
		c.Toast(a.tf("spotlight.openFailed", err.Error()))
	}
}

// spotlightDate is the day an article was published: the API sends a
// timestamp, and the list shows its day.
func spotlightDate(published string) string {
	if len(published) >= len("2006-01-02") {
		return published[:len("2006-01-02")]
	}
	return published
}
