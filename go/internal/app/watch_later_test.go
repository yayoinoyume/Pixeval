package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestWatchLaterAddDedupeAndLimit checks a new entry goes first, a work saved
// again moves to the top instead of repeating, and the list keeps its limit.
func TestWatchLaterAddDedupeAndLimit(t *testing.T) {
	w := &WatchLater{}
	for i := int64(1); i <= watchLaterLimit+5; i++ {
		w.Add(WatchLaterEntry{ID: i, Title: fmt.Sprintf("作品%d", i)})
	}
	if got := len(w.Entries); got != watchLaterLimit {
		t.Fatalf("entries %d, want %d", got, watchLaterLimit)
	}
	if got := w.Entries[0].ID; got != watchLaterLimit+5 {
		t.Errorf("first ID %d, want %d", got, watchLaterLimit+5)
	}

	w.Add(WatchLaterEntry{ID: 100, Title: "再看"})
	if got := len(w.Entries); got != watchLaterLimit {
		t.Fatalf("entries %d after saving an old work, want %d", got, watchLaterLimit)
	}
	if w.Entries[0].ID != 100 || w.Entries[0].Title != "再看" {
		t.Errorf("first entry %+v, want the re-saved work", w.Entries[0])
	}
	n := 0
	for _, e := range w.Entries {
		if e.ID == 100 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("work 100 appears %d times, want 1", n)
	}
}

// TestWatchLaterRemove checks removing drops only the named work.
func TestWatchLaterRemove(t *testing.T) {
	w := &WatchLater{}
	w.Add(WatchLaterEntry{ID: 1, Title: "一"})
	w.Add(WatchLaterEntry{ID: 2, Title: "二"})
	if !w.Has(1) || !w.Has(2) {
		t.Fatalf("Has: both works should be saved, got %+v", w.Snapshot())
	}
	w.Remove(1)
	if w.Has(1) {
		t.Error("work 1 still saved after Remove")
	}
	if got := w.Snapshot(); len(got) != 1 || got[0].ID != 2 {
		t.Errorf("entries %+v, want work 2 only", got)
	}
}

// TestWatchLaterLoadSave checks a saved list loads back as it was, in a file
// only the user can read.
func TestWatchLaterLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "watch_later.json")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	w := &WatchLater{}
	w.Add(WatchLaterEntry{
		ID:     7,
		Title:  "作品",
		Artist: "画师",
		Cover:  "https://example.invalid/7.jpg",
		At:     at,
	})
	if err := w.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %o, want 600", got)
	}

	got := &WatchLater{}
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

// TestLoadWatchLaterFromConfigDir checks a start reads the list the user
// saved last time.
func TestLoadWatchLaterFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := watchLaterPath()
	if err != nil {
		t.Fatalf("watchLaterPath: %v", err)
	}
	saved := &WatchLater{}
	saved.Add(WatchLaterEntry{ID: 9, Title: "留着"})
	if err := saved.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a := New()
	if !a.inWatchLater(9) {
		t.Errorf("work 9 not loaded: %+v", a.watchLater.Snapshot())
	}
}

// TestViewerWatchLater checks the viewer's action saves the work, then takes
// it out again, and that its label follows the work's state.
func TestViewerWatchLater(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.watchLater = &WatchLater{}
	w := viewerWork()
	a.openViewer(&w, 0)

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("稍后再看") {
		t.Fatalf("watch-later action missing: %q", tt.Texts())
	}
	if err := tt.Click("稍后再看"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if !a.inWatchLater(w.ID) {
		t.Fatalf("work %d not saved: %+v", w.ID, a.watchLater.Snapshot())
	}
	entries := a.watchLater.Snapshot()
	if len(entries) != 1 || entries[0].Title != w.Title || entries[0].Cover != w.ThumbURL() {
		t.Errorf("entry %+v, want the open work", entries)
	}

	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("已加入") {
		t.Errorf("added label missing: %q", tt.Texts())
	}
	if err := tt.Click("已加入"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.inWatchLater(w.ID) {
		t.Errorf("work %d still saved after clicking the added action", w.ID)
	}
}

// TestBrowseCardMenuWatchLater checks the card's menu saves the work and then
// offers to take it out.
func TestBrowseCardMenuWatchLater(t *testing.T) {
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	}}
	a, tt := newBrowseTest(st)
	a.watchLater = &WatchLater{}
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()

	if err := tt.RightClick("作品 1"); err != nil {
		t.Fatalf("RightClick: %v", err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "稍后再看") {
		t.Fatalf("menu = %v, want the watch-later item", menu)
	}
	if err := tt.ChooseMenuItem("稍后再看"); err != nil {
		t.Fatalf("ChooseMenuItem: %v", err)
	}
	if !a.inWatchLater(1) {
		t.Fatalf("work 1 not saved: %+v", a.watchLater.Snapshot())
	}

	tt.Frame()
	if err := tt.RightClick("作品 1"); err != nil {
		t.Fatalf("RightClick: %v", err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "移出稍后再看") {
		t.Fatalf("menu = %v, want the remove item", menu)
	}
	if err := tt.ChooseMenuItem("移出稍后再看"); err != nil {
		t.Fatalf("ChooseMenuItem: %v", err)
	}
	if a.inWatchLater(1) {
		t.Error("work 1 still saved after the remove item")
	}
}

// newWatchLaterApp is an app with an empty list and no network: opening a
// work fails through the fake fetch rather than asking Pixiv.
func newWatchLaterApp() *App {
	a := New()
	a.session = nil
	a.watchLater = &WatchLater{}
	a.watchLaterFailed = map[string]bool{}
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return nil, errors.New("no network in tests")
	}
	return a
}

// TestWatchLaterPageEmpty checks the page says there is nothing saved.
func TestWatchLaterPageEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newWatchLaterApp()
	tt := ui.NewTester(a.View, 1000, 700)
	navGo(t, tt, "稍后再看")
	if a.route != routeWatchLater {
		t.Errorf("route %q, want %q", a.route, routeWatchLater)
	}
	if !tt.HasText("还没有稍后再看的作品") {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
	if tt.HasText("清空列表") {
		t.Errorf("clear button shown with nothing saved: %q", tt.Texts())
	}
}

// savedWorks is a list of two works, as the page shows them.
func savedWorks() *WatchLater {
	w := &WatchLater{}
	w.Add(WatchLaterEntry{ID: 2, Title: "作品二", Artist: "画师二", At: time.Now()})
	w.Add(WatchLaterEntry{ID: 1, Title: "作品一", Artist: "画师一", At: time.Now()})
	return w
}

// TestWatchLaterPageEntries checks the page draws its entries, drops one and
// clears the rest.
func TestWatchLaterPageEntries(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newWatchLaterApp()
	a.watchLater = savedWorks()
	a.route = routeWatchLater

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("作品一") || !tt.HasText("作品二") {
		t.Errorf("titles missing: %q", tt.Texts())
	}
	if !tt.HasText("画师一") {
		t.Errorf("author missing: %q", tt.Texts())
	}
	if err := tt.Click("移出稍后再看"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	entries := a.watchLater.Snapshot()
	if len(entries) != 1 || entries[0].ID != 2 {
		t.Fatalf("entries %+v after removing the first card, want work 2", entries)
	}
	if a.viewer != nil {
		t.Error("removing a card opened the work")
	}

	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("清空列表") {
		t.Fatalf("clear button missing: %q", tt.Texts())
	}
	if err := tt.Click("清空列表"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := len(a.watchLater.Snapshot()); got != 0 {
		t.Errorf("entries %d after clearing, want 0", got)
	}
	tt = ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("还没有稍后再看的作品") {
		t.Errorf("empty state missing after clearing: %q", tt.Texts())
	}
}

// TestWatchLaterPageOpensWork checks a card fetches its work and opens it,
// through the same by-id open the history page uses.
func TestWatchLaterPageOpensWork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newViewerApp(newImageTransport(nil))
	a.watchLater = savedWorks()
	a.history = &History{}
	a.route = routeWatchLater
	want := viewerWork()
	// The fetch waits until the click's frame is over, as a window orders it:
	// otherwise the open would write the state the frame is reading.
	release := make(chan struct{})
	a.illustrationFn = func(_ context.Context, id int64) (*pixiv.Illustration, error) {
		<-release
		if id != 1 {
			return nil, fmt.Errorf("fetched work %d, want 1", id)
		}
		return &want, nil
	}

	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("作品一"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	// Opening records the work in the history, whose lock orders the test
	// after the goroutine that opened it.
	deadline := time.Now().Add(2 * time.Second)
	for len(a.history.Snapshot()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the saved work did not open")
		}
		time.Sleep(time.Millisecond)
	}
	if a.viewer == nil {
		t.Fatal("the saved work did not open")
	}
	if got := a.viewer.work.ID; got != want.ID {
		t.Errorf("opened work %d, want %d", got, want.ID)
	}
}

// TestWatchLaterCardPutsCoverAboveText checks a saved card draws its cover at
// the card's width and 3:2 shape, with the title under it, as the history
// page's cards do.
func TestWatchLaterCardPutsCoverAboveText(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newViewerApp(newImageTransport(nil))
	a.watchLater = &WatchLater{}
	a.watchLater.Add(WatchLaterEntry{ID: 1, Title: "作品一", Artist: "画师一",
		Cover: "https://example.invalid/1.jpg", At: time.Now()})
	a.route = routeWatchLater

	tt := ui.NewTester(a.View, 1000, 700)
	deadline := time.Now().Add(2 * time.Second)
	var cover ui.Rect
	for {
		tt.Frame()
		if r, ok := tt.Find(a.tf("a11y.cover", "作品一")); ok {
			cover = r
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cover missing: %q", tt.Texts())
		}
		time.Sleep(time.Millisecond)
	}
	card, ok := tt.Find(a.tf("a11y.card", "作品一", "画师一"))
	if !ok {
		t.Fatalf("card missing: %q", tt.Texts())
	}
	title, ok := tt.Find("作品一")
	if !ok {
		t.Fatalf("title missing: %q", tt.Texts())
	}
	if cover.W != card.W {
		t.Errorf("cover width %v, want the card's %v", cover.W, card.W)
	}
	if want := card.W * 3 / 2; cover.H < want-0.5 || cover.H > want+0.5 {
		t.Errorf("cover height %v, want %v", cover.H, want)
	}
	if title.Y < cover.Y+cover.H {
		t.Errorf("title at y=%v, want it under the cover ending at %v",
			title.Y, cover.Y+cover.H)
	}
}

// TestWatchLaterCardWithoutCoverKeepsItsBox checks an entry saved without a
// cover still leaves the cover's box, with the text under it.
func TestWatchLaterCardWithoutCoverKeepsItsBox(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newViewerApp(newImageTransport(nil))
	a.watchLater = &WatchLater{}
	a.watchLater.Add(WatchLaterEntry{ID: 2, Title: "没有封面", Artist: "画师二",
		At: time.Now()})
	a.route = routeWatchLater

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
	if _, ok := tt.Find(a.tf("a11y.cover", "没有封面")); ok {
		t.Error("a missing cover drew an image")
	}
	if tt.HasText(a.t("common.coverFailed")) {
		t.Errorf("a missing cover is not a failed load: %q", tt.Texts())
	}
}
