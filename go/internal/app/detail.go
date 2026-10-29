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
const viewerDetailWidth = 360

// detailTagLimit is how many of a work's tags the card shows before the
// reader asks for the rest; a long list would push the description and the
// comments far down.
const detailTagLimit = 6

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

// viewerInfo is the editorial rail beside the artwork: title, author, quiet
// numbers, tags, the work's own words and the comments. It is transparent on
// the room's wash; one hairline separates it from the picture.
func (a *App) viewerInfo(c *ui.Context, t tokens, v *viewerState) {
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
	ui.Box(c).Width(width).Shrink(0).FillHeight().
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).
		Role(ui.RoleGroup).Label(a.t("viewer.info")).Children(func() {
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Padding(t.SpaceXS, t.SpaceL, t.Space2XL, t.Space2XL).
				Gap(t.SpaceL).Children(func() {
				a.detailTitle(c, t, info)
				a.detailAuthor(c, t, v, info.User)
				a.detailStats(c, t, info)
				a.detailMeta(c, t, info)
				a.detailTags(c, t, v, info.Tags)
				if caption := plainCaption(info.Caption); caption != "" {
					ui.Text(c, caption).FontSize(t.TextBody).LineHeight(1.7).
						TextColor(t.Text)
				}
				if info.Series != nil {
					a.detailSeries(c, t, *info.Series)
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
					// Comments need a session; signed out, the rail says so
					// instead of drawing a composer that cannot post.
					if a.session != nil && a.comments != nil {
						a.commentsSection(c, t, a.comments)
					} else {
						ui.Text(c, a.t("comments.login")).FontSize(t.TextSmall).
							TextColor(t.TextMuted)
					}
				}
			})
		})
	})
}

// detailTitle is the work's name, in the display serif. It comes before the
// author so the page reads like a catalogue entry, not like a profile card.
func (a *App) detailTitle(c *ui.Context, t tokens, w pixiv.Illustration) {
	if w.Title == "" {
		return
	}
	displayText(c, w.Title).FontSize(t.TextDetail).FontWeight(500).
		LineHeight(1.16).TextColor(t.Text)
}

// detailAuthor is the author line: a small avatar, the name, the account or
// ID, and a quiet chevron. The whole line opens the author's page.
func (a *App) detailAuthor(c *ui.Context, t tokens, v *viewerState, user pixiv.User) {
	if user.ID <= 0 {
		return
	}
	b := ui.ButtonBase(c).FillWidth().Radius(t.RadiusS).
		Padding(t.SpaceS, t.SpaceS, t.SpaceS, 0).Gap(t.SpaceM).
		Label(a.t("user.open"))
	if b.Hovered() {
		b = b.Background(t.Surface.Alpha(0.45))
	}
	if b.Clicked() {
		a.closeViewer()
		a.openUser(user.ID)
	}
	b.Children(func() {
		url := user.ProfileImageUrls.Px50x50
		avatar := ui.Box(c).Size(32, 32).Shrink(0).Radius(t.RadiusFull).
			Background(t.AccentSoft).Clip().Center().
			Label(a.tf("a11y.avatar", user.Name))
		avatar.Children(func() {
			if url != "" && !v.imageFailedURL(url) {
				if bmp := a.images.Thumb(url, 32, 32, a.viewerImageReady(v, url, 32)); bmp != nil {
					ui.Image(c, bmp).Fill().Fit(ui.Cover)
					return
				}
			}
			ui.Text(c, initialOf(user.Name)).FontSize(t.TextSmall).FontWeight(600).
				TextColor(t.Accent)
		})
		ui.Column(c).Grow(1).Gap(1).MinWidth(0).Children(func() {
			ui.Text(c, user.Name).FontSize(t.TextSmall).FontWeight(600).TextColor(t.Text).
				MaxLines(1)
			sub := fmt.Sprintf("ID %d", user.ID)
			if user.Account != "" {
				sub = "@" + user.Account
			}
			ui.Text(c, sub).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
		})
		ui.Icon(c, iconRight).Size(14, 14).TextColor(t.TextMuted)
	})
}

// detailStats is the work's two counts, on one quiet line. They are numbers
// in the interface's text face, not oversized display figures.
func (a *App) detailStats(c *ui.Context, t tokens, w pixiv.Illustration) {
	metric := func(n int, label string) {
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, detailCount(n)).FontSize(t.TextBody).FontWeight(600).
				FontFeatures("tnum").TextColor(t.Text)
			ui.Text(c, label).FontSize(t.TextTiny).TextColor(t.TextMuted)
		})
	}
	ui.Row(c).Gap(t.SpaceL).AlignItems(ui.Center).Children(func() {
		metric(w.TotalBookmarks, a.t("viewer.bookmarkCount"))
		ui.Box(c).Width(1).Height(26).Background(t.Border)
		metric(w.TotalView, a.t("viewer.views"))
	})
}

// detailMeta is the publication time and the work number, small and
// monospaced by the tabular figures, without boxed labels.
func (a *App) detailMeta(c *ui.Context, t tokens, w pixiv.Illustration) {
	ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
		ui.Text(c, detailDate(w.CreateDate)).FontSize(t.TextTiny).
			FontFeatures("tnum").TextColor(t.TextMuted)
		ui.Text(c, "·").FontSize(t.TextTiny).TextColor(t.TextMuted)
		ui.Text(c, a.t("viewer.workID")).FontSize(t.TextTiny).TextColor(t.TextMuted)
		ui.Text(c, strconv.FormatInt(w.ID, 10)).FontSize(t.TextTiny).
			FontFeatures("tnum").TextColor(t.TextMuted)
	})
}

// detailTags is the tag list as quiet linked words, not filled pills. A
// translation sits under its original in the smallest type. A list longer
// than the card's room shows its first tags until the reader asks for all.
func (a *App) detailTags(c *ui.Context, t tokens, v *viewerState, tags []pixiv.Tag) {
	if len(tags) == 0 {
		return
	}
	shown := tags
	if !v.tagsOpen && len(tags) > detailTagLimit {
		shown = tags[:detailTagLimit]
	}
	ui.Column(c).Gap(t.SpaceS).Children(func() {
		ui.Text(c, a.t("viewer.tags")).FontSize(t.TextTiny).LetterSpacing(0.5).
			TextColor(t.TextMuted)
		ui.Row(c).Wrap().Gap(t.SpaceM).Role(ui.RoleGroup).Label(a.t("viewer.tags")).
			Children(func() {
				for _, tag := range shown {
					a.detailTag(c, t, tag)
				}
			})
		if len(tags) <= detailTagLimit {
			return
		}
		label := a.t("detail.showAllTags")
		if v.tagsOpen {
			label = a.t("detail.showFewerTags")
		}
		if ui.Button(c, label).FontSize(t.TextTiny).Clicked() {
			v.tagsOpen = !v.tagsOpen
		}
	})
}

// detailTag is one linked tag: its word, its translation when the API has
// one, and a quiet action to block it.
func (a *App) detailTag(c *ui.Context, t tokens, tag pixiv.Tag) {
	word := tag.Name
	if word == "" {
		word = tag.TranslatedName
	}
	if word == "" {
		return
	}
	ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
		fg := t.Accent
		b := ui.ButtonBase(c).Padding(0).Gap(2).Label(word)
		if b.Hovered() {
			fg = t.Text
		}
		if b.Clicked() {
			a.searchTag(word)
		}
		b.Children(func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "#").FontSize(t.TextMicro).TextColor(t.TextMuted)
				ui.Text(c, word).FontSize(t.TextSmall).FontWeight(500).TextColor(fg)
				if tag.TranslatedName != "" && tag.Name != "" {
					ui.Text(c, "·").FontSize(t.TextMicro).TextColor(t.TextMuted)
					ui.Text(c, tag.TranslatedName).FontSize(t.TextMicro).TextColor(t.TextMuted)
				}
			})
		})
		block := ui.ButtonBase(c).Padding(0, 2).Label(a.t("block.tag"))
		if block.Clicked() {
			a.blockTag(word)
		}
		block.Children(func() {
			ui.Text(c, a.t("block.tag")).FontSize(t.TextMicro).TextColor(t.TextMuted)
		})
	})
}

// detailSeries is the series line: a quiet rule, the series name and a
// chevron to its page.
func (a *App) detailSeries(c *ui.Context, t tokens, s pixiv.Series) {
	ui.Column(c).Gap(t.SpaceS).Padding(t.SpaceM, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, a.t("nav.series")).FontSize(t.TextTiny).LetterSpacing(0.5).
			TextColor(t.TextMuted)
		b := ui.ButtonBase(c).FillWidth().Radius(t.RadiusS).Padding(0).
			Gap(t.SpaceS).Label(a.tf("viewer.seriesLabel", s.Title))
		if b.Hovered() {
			b = b.Background(t.Surface.Alpha(0.45))
		}
		if b.Clicked() {
			a.closeViewer()
			a.openSeries(s.ID)
		}
		b.Children(func() {
			ui.Icon(c, iconSeries).Size(15, 15).TextColor(t.Accent)
			ui.Text(c, s.Title).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text).
				Grow(1).MaxLines(2).MinWidth(0)
			ui.Icon(c, iconRight).Size(14, 14).TextColor(t.TextMuted)
		})
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
