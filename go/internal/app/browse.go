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
	cardWidth = 240
	cardGap   = 12
	// cardText is the height the title and author take under a cover.
	cardText = 44
)

// browseState is one feed: the works loaded so far, the cursor of the next
// page, and the masonry's place. The view runs on the main thread; the load
// goroutine changes the fields under mu.
type browseState struct {
	mu      sync.Mutex
	items   []pixiv.Illustration
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

	// query is the search box's text, edited on the main thread. armed is
	// set when a page may load: once the window runs for the home feed, on
	// the first submitted word for search.
	query string
	armed bool

	// masonry is the feed's place, touched only by the view.
	masonry widgets.MasonryState
}

// newHomeState is the recommended feed. Its fetch reads the cursor under mu,
// as the load goroutine and the view both reach it.
func newHomeState(client *pixiv.Client) *browseState {
	st := &browseState{}
	st.fetch = func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		st.mu.Lock()
		next := st.next
		st.mu.Unlock()
		if next == "" {
			return client.Recommended(ctx)
		}
		return client.NextIllustrations(ctx, next)
	}
	return st
}

// status returns what the view draws.
func (st *browseState) status() (items []pixiv.Illustration, loading bool, errMsg string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.items, st.loading, st.err
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
			st.items = append(st.items, page.Items...)
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

// startSearch replaces the results with a search for the box's word.
func (a *App) startSearch(st *browseState) {
	word := strings.TrimSpace(st.query)
	if word == "" {
		return
	}
	st.mu.Lock()
	st.gen++
	st.items = nil
	st.next = ""
	st.started = false
	st.err = ""
	st.loading = false
	st.armed = true
	st.fetch = func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		st.mu.Lock()
		next := st.next
		st.mu.Unlock()
		if next == "" {
			return a.client.SearchIllustrations(ctx, pixiv.SearchOptions{Word: word})
		}
		return a.client.NextIllustrations(ctx, next)
	}
	st.mu.Unlock()
	st.masonry = widgets.MasonryState{}
	a.loadMore(st)
}

// cardAspect is a cover's aspect with the title and author under it, so the
// masonry leaves them room.
func cardAspect(aspect float64) float64 {
	if !(aspect > 0) {
		aspect = 1
	}
	return cardWidth / (cardWidth/aspect + cardText)
}

// browsePage draws a feed, and loads its next page once it is at the end.
func (a *App) browsePage(c *ui.Context, st *browseState) {
	t := c.Theme()
	items, _, errMsg := st.status()
	if errMsg != "" {
		ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "加载失败："+errMsg).TextColor(t.Danger).Grow(1)
			if ui.Button(c, "重试").Clicked() {
				st.retry()
			}
		})
	}
	if len(items) == 0 {
		msg := "没有作品"
		if !st.started {
			msg = "加载中…"
		}
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, msg).TextColor(t.TextMuted)
		})
	} else {
		tiles := make([]widgets.MasonryItem, len(items))
		for i := range items {
			tiles[i] = widgets.MasonryItem{Key: items[i].ID, Aspect: cardAspect(items[i].Aspect())}
		}
		widgets.Masonry(c, &st.masonry, tiles, cardWidth, cardGap, func(i int) {
			a.browseCard(c, items, i)
		})
	}
	if st.canLoad() && (len(items) == 0 || st.masonry.AtEnd()) {
		a.loadMore(st)
	}
}

// browseCard draws one tile: the cover, its title and the author. A cover not
// loaded yet is a plain block.
func (a *App) browseCard(c *ui.Context, items []pixiv.Illustration, i int) {
	work := items[i]
	t := c.Theme()
	ui.Column(c).Fill().Gap(4).Padding(2).Cursor(ui.CursorPointer).OnClick(func() {
		a.openViewer(&items[i], 0)
	}).Children(func() {
		ui.Box(c).Grow(1).FillWidth().Radius(8).Background(t.Surface).Clip().Children(func() {
			if bmp := a.images.Thumb(work.CoverURL(), 480, 720, func() { a.update(func() {}) }); bmp != nil {
				ui.Image(c, bmp).Fill()
			}
		})
		ui.Text(c, work.Title).MaxLines(1)
		ui.Text(c, work.User.Name).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
	})
}

// openViewer shows a work's pages; the viewer is a later milestone.
func (a *App) openViewer(w *pixiv.Illustration, page int) {}

// searchPage draws the search box and its results.
func (a *App) searchPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	st := a.search
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(8).Padding(12).Children(func() {
			ui.TextInput(c, &st.query).Grow(1).Placeholder("搜索作品")
			if ui.Button(c, "搜索").Clicked() {
				a.startSearch(st)
			}
		})
		if !st.armed {
			t := c.Theme()
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "输入关键词开始搜索").TextColor(t.TextMuted)
			})
			return
		}
		a.browsePage(c, st)
	})
}
