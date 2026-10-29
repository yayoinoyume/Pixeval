package app

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// scrollNovelBody is long enough that the reading page scrolls at a small
// window.
var scrollNovelBody = "长篇小说\n" + strings.Repeat("这是用来撑开阅读页的段落。\n", 80)

// TestNovelProgressRoundTrip checks the places survive a save and load, and
// that only the user can read the file.
func TestNovelProgressRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "novel_progress.json")
	p := &NovelProgress{}
	p.Set(3, 0.25)
	p.Set(9, 0.5)
	if err := p.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode %v, want 0600", perm)
	}

	q := &NovelProgress{}
	if err := q.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, want := range []struct {
		id    int64
		place float64
	}{{3, 0.25}, {9, 0.5}} {
		if got := q.Get(want.id); got != want.place {
			t.Errorf("novel %d at %v, want %v", want.id, got, want.place)
		}
	}
	if got := q.Get(4); got != 0 {
		t.Errorf("unknown novel at %v, want 0", got)
	}
}

// TestNovelProgressLimit checks the file keeps at most the cap and the newest
// place.
func TestNovelProgressLimit(t *testing.T) {
	p := &NovelProgress{}
	for i := int64(1); i <= novelProgressLimit+1; i++ {
		p.Set(i, 0.5)
	}
	path := filepath.Join(t.TempDir(), "novel_progress.json")
	if err := p.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	q := &NovelProgress{}
	if err := q.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(q.Places) > novelProgressLimit {
		t.Errorf("%d places, want at most %d", len(q.Places), novelProgressLimit)
	}
	if got := q.Get(novelProgressLimit + 1); got == 0 {
		t.Error("the newest place was dropped")
	}
}

// TestNovelProgressBrokenFile checks a broken file reports the error and
// keeps what was already loaded.
func TestNovelProgressBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "novel_progress.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p := &NovelProgress{}
	p.Set(5, 0.4)
	if err := p.Load(path); err == nil {
		t.Fatal("a broken file loaded without an error")
	}
	if got := p.Get(5); got != 0.4 {
		t.Errorf("place after a broken load %v, want 0.4", got)
	}

	if err := p.Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file loaded without an error")
	}
}

// TestNovelRestoreFraction checks places at either end are not returned to.
func TestNovelRestoreFraction(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{
		{0, 0},
		{0.005, 0},
		{novelProgressEdge, 0},
		{1 - novelProgressEdge, 0},
		{0.995, 0},
		{1, 0},
		{0.02, 0.02},
		{0.5, 0.5},
		{0.97, 0.97},
	} {
		if got := novelRestoreFraction(tt.in); got != tt.want {
			t.Errorf("restore %v, want %v", got, tt.want)
		}
	}
}

// scrollNovelApp opens scrollNovelBody in a small window and returns the
// tester showing it.
func scrollNovelApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newNovelApp(func(context.Context, int64) (string, error) { return scrollNovelBody, nil })
	a.openNovel(7, "长篇小说")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 800, 400)
	v := a.novelView
	if v.scroll.MaxY <= 0 {
		t.Fatalf("the body does not scroll: %+v", v.scroll)
	}
	return a, tt
}

// TestNovelPageRestoresSavedPlace checks a saved place scrolls the body there
// once it is laid out.
func TestNovelPageRestoresSavedPlace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newNovelApp(func(context.Context, int64) (string, error) { return scrollNovelBody, nil })
	a.novelProgress.Set(7, 0.5)
	a.openNovel(7, "长篇小说")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	ui.NewTester(a.View, 800, 400)

	v := a.novelView
	if v.scroll.MaxY <= 0 {
		t.Fatalf("the body does not scroll: %+v", v.scroll)
	}
	if want := 0.5 * float64(v.scroll.MaxY); math.Abs(float64(v.scroll.Y)-want) > 1 {
		t.Errorf("scroll %v, want %v", v.scroll.Y, want)
	}
}

// TestNovelPageIgnoresEdgePlace checks a place at either end opens the novel
// at the top.
func TestNovelPageIgnoresEdgePlace(t *testing.T) {
	for _, place := range []float64{0, novelProgressEdge, 1, 0.995} {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		a := newNovelApp(func(context.Context, int64) (string, error) { return scrollNovelBody, nil })
		a.novelProgress.Set(7, place)
		a.openNovel(7, "长篇小说")
		waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
		ui.NewTester(a.View, 800, 400)
		if y := a.novelView.scroll.Y; y != 0 {
			t.Errorf("place %v opened at %v, want the top", place, y)
		}
	}
}

// TestNovelPageSavesPlaceAfterPause checks scrolling remembers the place at
// once and writes it after a pause, and that opening the novel again returns
// to it.
func TestNovelPageSavesPlaceAfterPause(t *testing.T) {
	old := novelProgressDelay
	novelProgressDelay = 20 * time.Millisecond
	defer func() { novelProgressDelay = old }()

	a, tt := scrollNovelApp(t)
	v := a.novelView
	const want = 0.4
	v.scroll.Y = float32(want) * v.scroll.MaxY
	tt.Frame()

	if got := a.novelProgress.Get(7); math.Abs(got-want) > 0.01 {
		t.Errorf("place remembered as %v, want %v", got, want)
	}
	path, err := novelProgressPath()
	if err != nil {
		t.Fatalf("novelProgressPath: %v", err)
	}
	waitNovelProgress(t, path, 7, want)

	// Leaving and opening the novel again brings the place back.
	a.goBack()
	a.openNovel(7, "长篇小说")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	ui.NewTester(a.View, 800, 400)
	if y := a.novelView.scroll.Y; math.Abs(float64(y)-want*float64(a.novelView.scroll.MaxY)) > 1 {
		t.Errorf("reopened at %v, want %v", y, want*a.novelView.scroll.MaxY)
	}
}

// waitNovelProgress polls the saved places until the novel is at want.
func waitNovelProgress(t *testing.T, path string, novelID int64, want float64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p := &NovelProgress{}
		if err := p.Load(path); err == nil && math.Abs(p.Get(novelID)-want) < 0.01 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("novel %d not saved at %v in %s", novelID, want, path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
