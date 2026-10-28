package app

import (
	"context"
	"log"

	"github.com/egoist/mygo/ui"
)

// historyPage draws the works the user opened, newest first.
func (a *App) historyPage(c *ui.Context) {
	t := themeOf(c)
	var entries []HistoryEntry
	if a.history != nil {
		entries = a.history.Snapshot()
	}
	if len(entries) == 0 {
		emptyState(c, iconHistory, a.t("history.empty"), a.t("history.emptyHint"), nil)
		return
	}
	cols, cardW := a.historyGrid(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, t.SpaceS, contentPadding).
			AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tf("history.count", len(entries))).FontSize(t.TextSmall).
				TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, a.t("history.clear")).Clicked() {
				a.clearHistory()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(0, contentPadding, contentPadding, contentPadding).
				Gap(cardGap).Children(func() {
				for i := 0; i < len(entries); i += cols {
					row := entries[i:min(i+cols, len(entries))]
					ui.Row(c).Gap(cardGap).AlignItems(ui.Start).Children(func() {
						for _, e := range row {
							a.historyCard(c, e, cardW)
						}
					})
				}
			})
		})
	})
}

// historyGrid fits the cards to the page: it starts from the feed's card width
// and adds a column rather than letting a card grow much wider than the
// setting.
func (a *App) historyGrid(c *ui.Context) (int, float32) {
	gap := float32(cardGap)
	avail := contentWidth(c)
	base := float32(a.masonryWidth())
	if base <= 0 {
		base = cardWidth
	}
	cols := max(int((avail+gap)/(base+gap)), 1)
	for cols < 6 {
		w := (avail - gap*float32(cols-1)) / float32(cols)
		if w <= base*1.25 {
			break
		}
		cols++
	}
	cols = min(cols, 6)
	return cols, (avail - gap*float32(cols-1)) / float32(cols)
}

// historyCard draws one entry: its cover, title and author. Clicking it
// fetches the work and opens the viewer.
func (a *App) historyCard(c *ui.Context, e HistoryEntry, width float32) {
	t := themeOf(c)
	card := ui.Column(c).Width(width).Background(t.Surface).Radius(t.RadiusL).
		Border(1, t.Border).Clip().Focusable().
		Label(a.tf("a11y.card", e.Title, e.Artist)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openHistory(e)
	})
	if card.Hovered() {
		card = card.Border(1, t.Accent).Shadow(0, 8, 24, 0, t.Shadow)
	}
	card.Children(func() {
		ui.Box(c).FillWidth().Height(width * 3 / 2).Background(t.SurfaceAlt).Children(func() {
			if e.Cover == "" {
				return // an entry saved without a cover has no image to draw
			}
			if a.historyFailed[e.Cover] {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
				})
				return
			}
			if bmp := a.images.Thumb(e.Cover, coverWidth, coverHeight, a.historyCoverReady(e.Cover)); bmp != nil {
				ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", e.Title))
			}
		})
		ui.Column(c).Padding(t.SpaceS, t.SpaceM, t.SpaceM, t.SpaceM).Gap(2).Children(func() {
			ui.Text(c, e.Title).FontSize(t.TextSmall).TextColor(t.Text).MaxLines(1)
			ui.Text(c, e.Artist).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
		})
	})
}

// historyCoverReady returns the callback a cover load calls: a probe that
// finds nothing means the load failed, and the cover is a placeholder from
// then on rather than a load started on every frame.
func (a *App) historyCoverReady(key string) func() {
	return func() {
		failed := a.images.Thumb(key, coverWidth, coverHeight, nil) == nil
		a.update(func() {
			if failed {
				a.historyFailed[key] = true
			}
		})
	}
}

// openHistory fetches the work an entry names off the main thread, then opens
// it in the viewer.
func (a *App) openHistory(e HistoryEntry) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		w, err := a.client.Illustration(ctx, e.ID)
		a.update(func() {
			if err != nil {
				log.Printf("history: open %d: %v", e.ID, err)
				a.notice = a.tf("history.openFailed", err.Error())
				return
			}
			a.openViewer(w, 0)
		})
	}()
}
