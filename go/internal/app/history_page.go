package app

import (
	"context"
	"fmt"
	"log"

	"github.com/egoist/mygo/ui"
)

// historyColumns is how many cards one row of the history grid holds.
const historyColumns = 4

// historyPage draws the works the user opened, newest first.
func (a *App) historyPage(c *ui.Context) {
	t := c.Theme()
	var entries []HistoryEntry
	if a.history != nil {
		entries = a.history.Snapshot()
	}
	if len(entries) == 0 {
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "还没有浏览记录").TextColor(t.TextMuted)
			ui.Text(c, "打开作品后会自动记录").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 条记录", len(entries))).Grow(1).TextColor(t.TextMuted)
			if ui.Button(c, "清空历史").Clicked() {
				a.clearHistory()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(12).Gap(cardGap).Children(func() {
				for i := 0; i < len(entries); i += historyColumns {
					row := entries[i:min(i+historyColumns, len(entries))]
					ui.Row(c).Gap(cardGap).AlignItems(ui.Start).Children(func() {
						for _, e := range row {
							a.historyCard(c, e)
						}
					})
				}
			})
		})
	})
}

// historyCard draws one entry: its cover, title and author. Clicking it
// fetches the work and opens the viewer.
func (a *App) historyCard(c *ui.Context, e HistoryEntry) {
	t := c.Theme()
	ui.Column(c).Width(cardWidth).Gap(4).Padding(2).Cursor(ui.CursorPointer).OnClick(func() {
		a.openHistory(e)
	}).Children(func() {
		ui.Box(c).Width(cardWidth).Height(cardWidth * 3 / 2).Radius(8).
			Background(t.Surface).Clip().Children(func() {
			if e.Cover == "" {
				return // an entry saved without a cover has no image to draw
			}
			if a.historyFailed[e.Cover] {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "封面加载失败").FontSize(12).TextColor(t.TextMuted)
				})
				return
			}
			if bmp := a.images.Thumb(e.Cover, coverWidth, coverHeight, a.historyCoverReady(e.Cover)); bmp != nil {
				ui.Image(c, bmp).Fill()
			}
		})
		ui.Text(c, e.Title).MaxLines(1)
		ui.Text(c, e.Artist).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
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
				a.notice = "打开作品失败：" + err.Error()
				return
			}
			a.openViewer(w, 0)
		})
	}()
}
