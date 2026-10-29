package app

import (
	"context"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// seriesViewState is one series page: the description and the series' works
// feed. The view runs on the main thread and reads the detail through
// snapshot; the load goroutine writes it under mu.
type seriesViewState struct {
	feed *browseState

	mu       sync.Mutex
	detail   pixiv.SeriesDetail
	loading  bool
	err      string
	descOpen bool
}

// seriesPageView is what the series page draws.
type seriesPageView struct {
	detail   pixiv.SeriesDetail
	loading  bool
	err      string
	descOpen bool
}

func (v *seriesViewState) snapshot() seriesPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	return seriesPageView{detail: v.detail, loading: v.loading, err: v.err, descOpen: v.descOpen}
}

// setDescOpen expands the description, or folds it back to a few lines.
func (v *seriesViewState) setDescOpen(open bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.descOpen = open
}

// openSeries shows seriesID's page: the detail and the first page of works
// come from one fetch, so the feed is empty until it lands.
func (a *App) openSeries(seriesID int64) {
	if seriesID <= 0 {
		return
	}
	v := &seriesViewState{feed: &browseState{}, loading: true}
	a.backRoute, a.route, a.seriesView = a.route, routeSeries, v
	go a.loadSeries(v, seriesID)
}

// loadSeries fetches the series and its first page of works off the main
// thread: the detail fills the page and the works seed the feed.
func (a *App) loadSeries(v *seriesViewState, seriesID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	page, err := a.client.SeriesDetail(ctx, seriesID)
	a.update(func() {
		v.mu.Lock()
		v.loading = false
		if err != nil {
			v.err = err.Error()
			v.mu.Unlock()
			return
		}
		v.detail = page.Detail
		v.mu.Unlock()
		st := v.feed
		st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
			next, err := a.client.NextSeries(ctx, st.cursor())
			if err != nil {
				return nil, err
			}
			return &pixiv.Page[pixiv.Illustration]{Items: next.Illusts, NextURL: next.NextURL}, nil
		})
		st.seed(page.Illusts, page.NextURL, a.masonryWidth(), a.cardText)
	})
}

// seed fills a fresh feed with a page already fetched, so the first frame
// shows the works without asking for the page again. Later pages come from
// the feed's cursor.
func (st *browseState) seed(items []pixiv.Illustration, next string, width, textHeight float64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.add(items, width, textHeight)
	st.next, st.started, st.armed = next, true, true
}

// seriesPage draws the series' description above its works.
func (a *App) seriesPage(c *ui.Context) {
	v := a.seriesView
	if v == nil {
		return
	}
	t := themeOf(c)
	s := v.snapshot()
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Padding(t.SpaceL, contentPadding, 0, contentPadding).Children(func() {
			ui.Column(c).FillWidth().Padding(t.SpaceL).Gap(t.SpaceXS).
				Background(t.Surface).Radius(t.RadiusL).Border(1, t.Border).Children(func() {
				switch {
				case s.err != "":
					ui.Text(c, a.tf("series.loadFailed", s.err)).FontSize(t.TextSmall).TextColor(t.Danger)
				case s.loading:
					ui.Text(c, a.t("common.loading")).FontSize(t.TextSmall).TextColor(t.TextMuted)
				default:
					ui.Text(c, s.detail.Title).FontSize(t.TextTitle).Bold().TextColor(t.Text).MaxLines(1)
					a.seriesAuthor(c, t, s.detail.User)
					a.seriesMeta(c, t, s.detail)
					a.seriesDescription(c, t, v, s)
					ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
						a.subscriptionButton(c, subscription.KindSeries, s.detail.ID, s.detail.Title)
						a.downloadLoadedButton(c, v.feed.snapshot().items)
					})
				}
			})
		})
		a.browsePage(c, v.feed)
	})
}

// seriesAuthor is the series' author, on a quiet line that opens the author's
// page.
func (a *App) seriesAuthor(c *ui.Context, t tokens, user pixiv.User) {
	if user.ID <= 0 || user.Name == "" {
		return
	}
	b := ui.ButtonBase(c).Radius(t.RadiusS).Padding(0, t.SpaceS, 0, 0).
		Gap(t.SpaceXS).Label(a.t("series.author"))
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
	}
	if b.Clicked() {
		a.openUser(user.ID)
	}
	b.Children(func() {
		ui.Text(c, user.Name).FontSize(t.TextSmall).FontWeight(600).TextColor(t.Text).MaxLines(1)
		ui.Icon(c, iconRight).Size(14, 14).TextColor(t.TextMuted)
	})
}

// seriesMeta is the series' facts: how many works it holds, whether it is
// finished, and when it was created and last updated.
func (a *App) seriesMeta(c *ui.Context, t tokens, s pixiv.SeriesDetail) {
	var facts []string
	if s.WorkIllustsCount > 0 {
		facts = append(facts, a.tf("series.worksCount", s.WorkIllustsCount))
	}
	if s.IsConcluded {
		facts = append(facts, a.t("series.concluded"))
	} else {
		facts = append(facts, a.t("series.ongoing"))
	}
	switch {
	case !s.CreateDate.IsZero() && !s.UpdateDate.IsZero():
		facts = append(facts, detailDate(s.CreateDate)+" → "+detailDate(s.UpdateDate))
	case !s.CreateDate.IsZero():
		facts = append(facts, detailDate(s.CreateDate))
	case !s.UpdateDate.IsZero():
		facts = append(facts, detailDate(s.UpdateDate))
	}
	ui.Row(c).Wrap().Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
		for i, fact := range facts {
			if i > 0 {
				ui.Text(c, "·").FontSize(t.TextTiny).TextColor(t.TextMuted)
			}
			ui.Text(c, fact).FontSize(t.TextSmall).TextColor(t.TextMuted)
		}
	})
}

// seriesDescription is the series' own words: a few lines until the reader
// expands it, with the toggle only when there is more to read.
func (a *App) seriesDescription(c *ui.Context, t tokens, v *seriesViewState, s seriesPageView) {
	desc := s.detail.Description
	if desc == "" {
		return
	}
	if s.descOpen {
		ui.Text(c, desc).FontSize(t.TextSmall).TextColor(t.TextMuted)
	} else {
		ui.Text(c, desc).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(seriesDescLines)
	}
	if !descOverflows(desc, seriesDescWidth(c, t), t.TextSmall, seriesDescLines) {
		return
	}
	label := a.t("series.showMore")
	if s.descOpen {
		label = a.t("series.showLess")
	}
	if ui.Button(c, label).Clicked() {
		v.setDescOpen(!s.descOpen)
	}
}

// seriesDescLines is how many lines of the series' description the page shows
// before the reader expands it.
const seriesDescLines = 3

// seriesDescWidth is the width the description's text has in the card.
func seriesDescWidth(c *ui.Context, t tokens) float32 {
	return max(contentWidth(c)-2*t.SpaceL, 0)
}

// descOverflows reports whether text takes more than maxRows rows at width.
// The page decides before the text is laid out, so the wrap is estimated: a
// wide rune (CJK, kana and the fullwidth forms) is one type size, the rest a
// little over half of it.
func descOverflows(text string, width, size float32, maxRows int) bool {
	if size <= 0 {
		return false
	}
	if width <= 0 {
		return true
	}
	rows := 0
	for _, para := range strings.Split(text, "\n") {
		used := float32(0)
		rows++
		for _, r := range para {
			w := size * 0.55
			if r >= 0x2e80 {
				w = size
			}
			if used+w > width {
				rows++
				used = 0
			}
			used += w
		}
		if rows > maxRows {
			return true
		}
	}
	return false
}
