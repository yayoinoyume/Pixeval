package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestHistoryAddDedupeAndLimit checks a new entry goes first, a work seen
// again moves to the top instead of repeating, and the list keeps its limit.
func TestHistoryAddDedupeAndLimit(t *testing.T) {
	h := &History{}
	for i := int64(1); i <= historyLimit+5; i++ {
		h.Add(HistoryEntry{ID: i, Title: fmt.Sprintf("作品%d", i)})
	}
	if got := len(h.Entries); got != historyLimit {
		t.Fatalf("entries %d, want %d", got, historyLimit)
	}
	if got := h.Entries[0].ID; got != historyLimit+5 {
		t.Errorf("first ID %d, want %d", got, historyLimit+5)
	}
	if got := h.Entries[len(h.Entries)-1].ID; got != 6 {
		t.Errorf("last ID %d, want 6", got)
	}

	h.Add(HistoryEntry{ID: 100, Title: "重看"})
	if got := len(h.Entries); got != historyLimit {
		t.Fatalf("entries %d after adding an old work, want %d", got, historyLimit)
	}
	if h.Entries[0].ID != 100 || h.Entries[0].Title != "重看" {
		t.Errorf("first entry %+v, want the re-opened work", h.Entries[0])
	}
	n := 0
	for _, e := range h.Entries {
		if e.ID == 100 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("work 100 appears %d times, want 1", n)
	}
}

// TestHistoryLoadSave checks a saved history loads back as it was, in a file
// only the user can read.
func TestHistoryLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "history.json")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h := &History{}
	h.Add(HistoryEntry{
		ID:     7,
		Title:  "作品",
		Artist: "画师",
		Cover:  "https://example.invalid/7.jpg",
		At:     at,
	})
	if err := h.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %o, want 600", got)
	}

	got := &History{}
	if err := got.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	entries := got.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("entries %d, want 1", len(entries))
	}
	if entries[0].ID != 7 || entries[0].Title != "作品" || entries[0].Artist != "画师" {
		t.Errorf("entry %+v, want the saved one", entries[0])
	}
	if !entries[0].At.Equal(at) {
		t.Errorf("time %v, want %v", entries[0].At, at)
	}
}

// TestHistoryBadJSON checks a broken file leaves an empty history rather than
// a half read one.
func TestHistoryBadJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := historyPath()
	if err != nil {
		t.Fatalf("historyPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := (&History{}).Load(path); err == nil {
		t.Error("Load accepted a broken file")
	}
	if got := len(loadHistory().Snapshot()); got != 0 {
		t.Errorf("entries %d for a broken file, want 0", got)
	}
}

// waitBookmark waits for the bookmark request to land as want says.
func waitBookmark(t *testing.T, a *App, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if a.viewer != nil {
			if got, _ := a.viewer.bookmarkState(); got == want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("bookmark did not become %v", want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestViewerBookmark checks the button bookmarks the work, then removes the
// bookmark, and that its label follows the work's state.
func TestViewerBookmark(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	var calls []string
	a.bookmarkFn = func(_ context.Context, id int64, on bool) error {
		calls = append(calls, fmt.Sprintf("%d:%v", id, on))
		return nil
	}
	w := viewerWork()
	a.openViewer(&w, 0)

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("收藏") {
		t.Fatalf("bookmark button missing: %q", tt.Texts())
	}
	if err := tt.Click("收藏"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBookmark(t, a, true)

	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("已收藏") {
		t.Errorf("bookmarked label missing: %q", tt.Texts())
	}
	if err := tt.Click("已收藏"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBookmark(t, a, false)

	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("收藏") {
		t.Errorf("unbookmarked label missing: %q", tt.Texts())
	}
	if got := fmt.Sprint(calls); got != "[42:true 42:false]" {
		t.Errorf("calls %s, want [42:true 42:false]", got)
	}
}

// TestOpenViewerRecordsHistory checks opening a work puts it at the top of the
// history.
func TestOpenViewerRecordsHistory(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.history = &History{}
	w := viewerWork()
	a.openViewer(&w, 0)
	entries := a.history.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("entries %d, want 1", len(entries))
	}
	if entries[0].ID != w.ID || entries[0].Title != w.Title || entries[0].Cover != w.ThumbURL() {
		t.Errorf("entry %+v, want the opened work", entries[0])
	}
}

// TestHistoryPageEmpty checks the history page says there is nothing yet.
func TestHistoryPageEmpty(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.history = &History{}
	tt := ui.NewTester(a.View, 1000, 700)
	navGo(t, tt, "历史")
	if a.route != routeHistory {
		t.Errorf("route %q, want %q", a.route, routeHistory)
	}
	if !tt.HasText("还没有浏览记录") {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
	if tt.HasText("清空历史") {
		t.Errorf("clear button shown with no history: %q", tt.Texts())
	}
}

// TestHistoryPageEntries checks the history page draws its entries and clears
// them.
func TestHistoryPageEntries(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.history = &History{}
	a.history.Add(HistoryEntry{
		ID: 1, Title: "作品一", Artist: "画师一",
		Cover: "https://example.invalid/1.jpg", At: time.Now(),
	})
	a.history.Add(HistoryEntry{
		ID: 2, Title: "作品二", Artist: "画师二",
		Cover: "https://example.invalid/2.jpg", At: time.Now(),
	})
	a.route = routeHistory

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("作品一") || !tt.HasText("作品二") {
		t.Errorf("titles missing: %q", tt.Texts())
	}
	if !tt.HasText("2 条记录") {
		t.Errorf("count missing: %q", tt.Texts())
	}
	if !tt.HasText("清空历史") {
		t.Fatalf("clear button missing: %q", tt.Texts())
	}
	if err := tt.Click("清空历史"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := len(a.history.Snapshot()); got != 0 {
		t.Errorf("entries %d after clearing, want 0", got)
	}
	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("还没有浏览记录") {
		t.Errorf("empty state missing after clearing: %q", tt.Texts())
	}
}

// TestHistoryGridFitsWidth checks the history grid chooses columns from the
// page width rather than a fixed four, and keeps the cards near the masonry
// width.
func TestHistoryGridFitsWidth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.CardWidth = 240
	cases := []struct {
		name string
		w, h int
		cols int
		card float32
	}{
		{"narrow", 960, 700, 3, 248},
		{"wide", 1280, 820, 4, 261},
		{"wider", 1600, 900, 5, 268.8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cols int
			var card float32
			tt := ui.NewTester(func(ctx *ui.Context) {
				cols, card = a.historyGrid(ctx)
			}, c.w, c.h)
			tt.Frame()
			if cols != c.cols {
				t.Errorf("columns %d, want %d", cols, c.cols)
			}
			if math.Abs(float64(card-c.card)) > 0.01 {
				t.Errorf("card width %v, want %v", card, c.card)
			}
		})
	}
}
