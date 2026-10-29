package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// The bookmarks page's tabs: the works bookmarked publicly and privately.
const (
	bookmarksPublic = iota
	bookmarksPrivate
)

// bookmarksPage draws the signed-in user's bookmarks, one tab per visibility.
func (a *App) bookmarksPage(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
		if textTabs(c, t, &a.bookmarksTab, a.t("bookmarks.public"), a.t("bookmarks.private")) {
			a.restartBookmarks()
		}
	})
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.armBookmarks()
	a.bookmarksFeed(c)
}

// armBookmarks gives the shown tab its page fetcher, once.
func (a *App) armBookmarks() {
	st := a.bookmarks
	if st.isArmed() {
		return
	}
	st.setFetch(a.bookmarkFetch(st, a.bookmarksTab == bookmarksPrivate))
	st.arm()
}

// restartBookmarks drops the loaded works and points the feed at the tab now
// shown, so a page still loading for the other tab is dropped.
func (a *App) restartBookmarks() {
	st := a.bookmarks
	st.restart()
	st.setFetch(a.bookmarkFetch(st, a.bookmarksTab == bookmarksPrivate))
}

// bookmarkFetch fetches the next page of a bookmarks tab: the first page when
// the feed has no cursor.
func (a *App) bookmarkFetch(st *browseState, private bool) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return a.bookmarksPageFn(ctx, private, st.cursor())
	}
}

// bookmarksFeed draws the bookmarks list, and loads its next page at the end.
func (a *App) bookmarksFeed(c *ui.Context) {
	t := themeOf(c)
	st := a.bookmarks
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
			return
		}
	}
	width := a.masonryWidth()
	textHeight := cardTextHeight(t)
	a.cardText = textHeight
	items, tiles := feedTiles(st, v.items, v.tiles, width, textHeight)
	switch {
	case len(items) > 0:
		ui.Column(c).Grow(1).Padding(t.SpaceM, contentPadding, contentPadding, contentPadding).Children(func() {
			widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
				a.bookmarkCard(c, st, items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconHeart, a.t("bookmarks.empty"), a.t("bookmarks.emptyHint"), nil)
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMore(st)
	}
}

// bookmarkCard draws one bookmarked work. Its menu opens the author's page
// and drops the bookmark, which leaves the list at once.
func (a *App) bookmarkCard(c *ui.Context, st *browseState, items []pixiv.Illustration, i int) {
	work := items[i]
	key := work.ThumbURL()
	t := themeOf(c)
	card, cover := cardShell(c)
	titleColor := t.Text
	if card.Hovered() {
		titleColor = t.Accent
	}
	card = card.Fill().Focusable().
		Label(a.tf("a11y.card", work.Title, work.User.Name)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openViewer(&items[i], 0)
	}).ContextMenu(func(m *ui.Menu) {
		if m.Item(a.t("user.open")).Chosen() {
			a.openUser(work.User.ID)
		}
		if m.Item(a.t("bookmarks.unbookmark")).Chosen() {
			a.unbookmark(st, work.ID)
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
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, work.Title).FontSize(t.TextBody).FontWeight(600).
				TextColor(titleColor).MaxLines(1)
			ui.Text(c, work.User.Name).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
		})
	})
}

// unbookmark takes a work out of the list and removes its bookmark off the
// main thread. A failed request loads the tab again, since the work is still
// bookmarked on Pixiv, and says why.
func (a *App) unbookmark(st *browseState, id int64) {
	if a.bookmarkFn == nil {
		return
	}
	if !st.remove(id) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		err := a.bookmarkFn(ctx, id, false)
		a.update(func() {
			if err != nil {
				a.restartBookmarks()
				a.notice = a.tf("bookmarks.unbookmarkFailed", err.Error())
			}
		})
	}()
}

// remove takes the work id out of the feed.
func (st *browseState) remove(id int64) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, w := range st.items {
		if w.ID != id {
			continue
		}
		st.items = append(st.items[:i], st.items[i+1:]...)
		if i < len(st.tiles) {
			st.tiles = append(st.tiles[:i], st.tiles[i+1:]...)
		}
		return true
	}
	return false
}
