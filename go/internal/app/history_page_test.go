package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// historyPageApp is an English app on the history page holding entries,
// without a network: every cover load fails.
func historyPageApp(entries ...HistoryEntry) *App {
	a := englishPageApp(pageBodies())
	a.history = &History{}
	for _, e := range entries {
		a.history.Add(e)
	}
	a.route = routeHistory
	return a
}

// TestHistoryCardPutsCoverAboveText checks the card keeps the cover's box at
// the card's width and 3:2 shape, and draws the title and author under it
// rather than over a collapsed cover.
func TestHistoryCardPutsCoverAboveText(t *testing.T) {
	a := historyPageApp(HistoryEntry{ID: 1, Title: "第一个作品", Artist: "画师一",
		Cover: "https://example.invalid/1.jpg", At: time.Now()})
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()

	card, ok := tt.Find(a.tf("a11y.card", "第一个作品", "画师一"))
	if !ok {
		t.Fatalf("card missing: %q", tt.Texts())
	}
	title, ok := tt.Find("第一个作品")
	if !ok {
		t.Fatalf("title missing: %q", tt.Texts())
	}
	coverHeight := card.W * 3 / 2
	if card.H < coverHeight {
		t.Errorf("card height %v, want at least the cover's %v", card.H, coverHeight)
	}
	if title.Y < card.Y+coverHeight {
		t.Errorf("title at y=%v, want it under the cover ending at %v",
			title.Y, card.Y+coverHeight)
	}

	failed, ok := tt.Find(a.t("common.coverFailed"))
	if !ok {
		t.Fatalf("cover placeholder missing: %q", tt.Texts())
	}
	center := failed.Y + failed.H/2
	if want := card.Y + coverHeight/2; center < want-coverHeight/4 || center > want+coverHeight/4 {
		t.Errorf("cover placeholder centered at %v, want near %v", center, want)
	}
}

// TestHistoryCardWithoutCoverKeepsItsShape checks an entry saved without a
// cover still leaves the cover's box, with the text under it.
func TestHistoryCardWithoutCoverKeepsItsShape(t *testing.T) {
	a := historyPageApp(HistoryEntry{ID: 2, Title: "没有封面", Artist: "画师二",
		At: time.Now()})
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()

	card, ok := tt.Find(a.tf("a11y.card", "没有封面", "画师二"))
	if !ok {
		t.Fatalf("card missing: %q", tt.Texts())
	}
	title, ok := tt.Find("没有封面")
	if !ok {
		t.Fatalf("title missing: %q", tt.Texts())
	}
	coverHeight := card.W * 3 / 2
	if card.H < coverHeight {
		t.Errorf("card height %v, want at least the cover's %v", card.H, coverHeight)
	}
	if title.Y < card.Y+coverHeight {
		t.Errorf("title at y=%v, want it under the cover ending at %v",
			title.Y, card.Y+coverHeight)
	}
	if tt.HasText(a.t("common.coverFailed")) {
		t.Errorf("a missing cover is not a failed load: %q", tt.Texts())
	}
}
