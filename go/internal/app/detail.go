package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// viewerDetailWidth is the info card's width, narrowed as the window shrinks
// so the artwork keeps most of it.
const viewerDetailWidth = 380

// detailTagLimit is how many of a work's tags the card shows; a long list
// would push the description and the comments far down.
const detailTagLimit = 5

// loadViewerDetail fetches the work's full record off the main thread and
// leaves it to the info card. A result that lands after the viewer closed is
// dropped, and a bookmark request already running wins over the record's
// state.
func (a *App) loadViewerDetail(v *viewerState) {
	if v == nil || a.illustrationFn == nil {
		return
	}
	v.mu.Lock()
	if v.detailLoading {
		v.mu.Unlock()
		return
	}
	v.detailLoading, v.detailErr = true, ""
	gen := v.bookmarkGen
	id := v.work.ID
	v.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		w, err := a.illustrationFn(ctx, id)
		a.update(func() {
			v.mu.Lock()
			defer v.mu.Unlock()
			v.detailLoading = false
			if v.closed {
				return
			}
			if err != nil {
				v.detailErr = err.Error()
				return
			}
			v.detail = w
			// A bookmark that finished after this fetch started is newer
			// than the record it returns.
			if v.bookmarkGen == gen && !v.bookmarking {
				v.bookmarked = w.IsBookmarked
			}
		})
	}()
}

// viewerInfo is the scrollable card beside the artwork: the work's author,
// title, stats, metadata, tags, description, series and comments.
func (a *App) viewerInfo(c *ui.Context, v *viewerState) {
	t := themeOf(c)
	width := float32(viewerDetailWidth)
	if winW, _ := c.Size(); winW < 960 {
		width = 300
	} else if winW < 1100 {
		width = 320
	}
	detail, errText, loading := v.detailState()
	info := v.work
	if detail != nil {
		info = *detail
	}
	ui.Scroll(c).Width(width).Shrink(0).FillHeight().Radius(t.RadiusXL).
		Background(t.Surface).Role(ui.RoleGroup).Label(a.t("viewer.info")).
		Children(func() {
			ui.Column(c).Padding(t.SpaceXL).Gap(t.SpaceL).Children(func() {
				a.detailAuthor(c, v, info.User)
				ui.Text(c, info.Title).FontSize(t.TextDetail).FontWeight(600).TextColor(t.Text)
				a.detailStats(c, info)
				a.detailMeta(c, info)
				a.detailTags(c, info.Tags)
				if info.Caption != "" {
					ui.Text(c, info.Caption).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.TextMuted)
				}
				if info.Series != nil {
					a.detailSeries(c, *info.Series)
				}
				switch {
				case loading:
					ui.Text(c, a.t("common.loading")).FontSize(t.TextTiny).TextColor(t.TextMuted)
				case errText != "":
					ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.tf("viewer.detailFailed", errText)).
							FontSize(t.TextSmall).TextColor(t.Danger).Grow(1).MaxLines(3)
						if ui.Button(c, a.t("common.retry")).Clicked() {
							a.loadViewerDetail(v)
						}
					})
				}
				if a.commentsOpen {
					// Comments need a session; signed out, the card says so
					// instead of drawing a composer that cannot post.
					if a.session != nil && a.comments != nil {
						a.commentsSection(c, a.comments)
					} else {
						ui.Text(c, a.t("comments.login")).FontSize(t.TextSmall).
							TextColor(t.TextMuted)
					}
				}
			})
		})
}

// detailAuthor is the author row: the avatar or its initial, the name and the
// user ID. Clicking it leaves the viewer for the user's page.
func (a *App) detailAuthor(c *ui.Context, v *viewerState, user pixiv.User) {
	if user.ID <= 0 {
		return
	}
	t := themeOf(c)
	b := ui.ButtonBase(c).Radius(t.RadiusM).Padding(t.SpaceS).Gap(t.SpaceM).
		Label(a.t("user.open"))
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	if b.Clicked() {
		a.closeViewer()
		a.openUser(user.ID)
	}
	b.Children(func() {
		url := user.ProfileImageUrls.Px50x50
		avatar := ui.Box(c).Size(44, 44).Shrink(0).Radius(t.RadiusFull).
			Background(t.AccentSoft).Clip().Center().
			Label(a.tf("a11y.avatar", user.Name))
		avatar.Children(func() {
			if url != "" && !v.imageFailedURL(url) {
				if bmp := a.images.Thumb(url, 44, 44, a.viewerImageReady(v, url, 44)); bmp != nil {
					ui.Image(c, bmp).Fill().Fit(ui.Cover)
					return
				}
			}
			ui.Text(c, initialOf(user.Name)).FontSize(t.TextTitle).FontWeight(600).TextColor(t.Accent)
		})
		ui.Column(c).Gap(2).MinWidth(0).Children(func() {
			ui.Text(c, user.Name).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).
				MaxLines(1)
			ui.Text(c, fmt.Sprintf("ID %d", user.ID)).FontSize(t.TextTiny).
				FontWeight(500).TextColor(t.Accent)
		})
	})
}

// detailStats is the bookmark and view counts, side by side.
func (a *App) detailStats(c *ui.Context, w pixiv.Illustration) {
	t := themeOf(c)
	stat := func(n int, label string) {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, detailCount(n)).FontSize(t.TextTitle).FontWeight(600).TextColor(t.Text)
			ui.Text(c, label).FontSize(t.TextTiny).TextColor(t.TextMuted)
		})
	}
	ui.Row(c).Gap(t.Space2XL).Children(func() {
		stat(w.TotalBookmarks, a.t("viewer.bookmarkCount"))
		stat(w.TotalView, a.t("viewer.views"))
	})
}

// detailMeta is the publication date and the work ID, above a rule.
func (a *App) detailMeta(c *ui.Context, w pixiv.Illustration) {
	t := themeOf(c)
	col := func(label, value string) {
		ui.Column(c).Gap(2).MinWidth(0).Children(func() {
			ui.Text(c, label).FontSize(t.TextTiny).TextColor(t.TextMuted)
			ui.Text(c, value).FontSize(t.TextSmall).TextColor(t.Text)
		})
	}
	ui.Row(c).Gap(t.SpaceXL).Padding(t.SpaceM, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		col(a.t("viewer.published"), detailDate(w.CreateDate))
		col(a.t("viewer.workID"), strconv.FormatInt(w.ID, 10))
	})
}

// detailTags is the work's tags as chips; clicking one searches for it.
func (a *App) detailTags(c *ui.Context, tags []pixiv.Tag) {
	if len(tags) == 0 {
		return
	}
	t := themeOf(c)
	ui.Row(c).Wrap().Gap(t.SpaceS).Role(ui.RoleGroup).Label(a.t("viewer.tags")).
		Children(func() {
			for i := 0; i < len(tags) && i < detailTagLimit; i++ {
				a.detailTag(c, tags[i])
			}
		})
}

// detailTag is one tag chip: the tag, and its translation under it when the
// API has one.
func (a *App) detailTag(c *ui.Context, tag pixiv.Tag) {
	t := themeOf(c)
	word := tag.Name
	if word == "" {
		word = tag.TranslatedName
	}
	if word == "" {
		return
	}
	b := ui.ButtonBase(c).Radius(t.RadiusFull).Padding(t.SpaceS, t.SpaceM).
		Background(t.AccentSoft).Label(word)
	if b.Hovered() {
		b = b.Background(t.AccentSoft.Mix(t.Accent, 0.2))
	}
	if b.Clicked() {
		a.searchTag(word)
	}
	b.Children(func() {
		ui.Column(c).Gap(1).AlignItems(ui.Start).Children(func() {
			ui.Text(c, word).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Accent)
			if tag.TranslatedName != "" && tag.Name != "" {
				ui.Text(c, tag.TranslatedName).FontSize(t.TextTiny).TextColor(t.TextMuted)
			}
		})
	})
}

// detailSeries is the series row: its icon, title and chevron, which opens
// the series page.
func (a *App) detailSeries(c *ui.Context, s pixiv.Series) {
	t := themeOf(c)
	b := ui.ButtonBase(c).Radius(t.RadiusM).Padding(t.SpaceS).Gap(t.SpaceM).
		Label(a.tf("viewer.seriesLabel", s.Title))
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	if b.Clicked() {
		a.closeViewer()
		a.openSeries(s.ID)
	}
	b.Children(func() {
		ui.Icon(c, iconSeries).Size(16, 16).TextColor(t.Accent)
		ui.Text(c, s.Title).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text).
			Grow(1).MaxLines(2).MinWidth(0)
		ui.Icon(c, iconRight).Size(14, 14).TextColor(t.TextMuted)
	})
}

// searchTag searches for a tag's word and shows the results, closing the
// viewer on the way.
func (a *App) searchTag(word string) {
	if word == "" {
		return
	}
	a.search.query = word
	a.goRoute(routeSearch)
	a.startSearch(a.search)
}

// workURL is the work's page on pixiv.net.
func workURL(id int64) string {
	return fmt.Sprintf("https://www.pixiv.net/artworks/%d", id)
}

// copyWorkLink puts the work's page URL on the clipboard.
func (a *App) copyWorkLink(c *ui.Context, id int64) {
	if err := mygo.Clipboard.Write(transfer.TextData(workURL(id))); err != nil {
		c.Toast(a.tf("viewer.copyFailed", err.Error()))
		return
	}
	c.Toast(a.t("viewer.copied"))
}

// openWorkLink opens the work's page in the default browser.
func (a *App) openWorkLink(c *ui.Context, id int64) {
	if err := mygo.Shell.OpenExternal(workURL(id)); err != nil {
		c.Toast(a.tf("viewer.openFailed", err.Error()))
	}
}

// viewerPageLabel is the pager's text: the page and the work's page count.
func (a *App) viewerPageLabel(page, n int) string {
	return a.tf("viewer.pageOf", page, n)
}

// pageThumbURL is the smallest rendition of a page the work offers. The
// feed's copy often carries only page 0's large image and the originals of
// the other pages, so the fallback is the original.
func pageThumbURL(w pixiv.Illustration, page int) string {
	if page == 0 {
		if w.ImageUrls.Medium != "" {
			return w.ImageUrls.Medium
		}
		if w.ImageUrls.Large != "" {
			return w.ImageUrls.Large
		}
	}
	if page >= 0 && page < len(w.MetaPages) {
		if u := w.MetaPages[page].ImageUrls.Medium; u != "" {
			return u
		}
		if u := w.MetaPages[page].ImageUrls.Large; u != "" {
			return u
		}
	}
	return w.PageURL(page)
}

// detailCount is a count with thousands separators, as the info card shows
// the bookmark and view totals.
func detailCount(n int) string {
	s := strconv.Itoa(n)
	if n < 0 || len(s) <= 3 {
		return s
	}
	start := len(s) % 3
	if start == 0 {
		start = 3
	}
	var b strings.Builder
	b.WriteString(s[:start])
	for i := start; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// detailDate is a work's publication time in the card's fixed format.
func detailDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}
