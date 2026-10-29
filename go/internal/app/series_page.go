package app

import (
	"context"
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

	mu      sync.Mutex
	detail  pixiv.SeriesDetail
	loading bool
	err     string
}

// seriesPageView is what the series page draws.
type seriesPageView struct {
	detail  pixiv.SeriesDetail
	loading bool
	err     string
}

func (v *seriesViewState) snapshot() seriesPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	return seriesPageView{detail: v.detail, loading: v.loading, err: v.err}
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
					if s.detail.Description != "" {
						ui.Text(c, s.detail.Description).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(3)
					}
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
