package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// viewerDetail is a work whose detail record carries what the feed's copy
// lacks: tags, stats, a caption, a series and a full author.
func viewerDetail() pixiv.Illustration {
	w := viewerWork()
	w.User = pixiv.User{
		ID:               204811,
		Name:             "しろ",
		ProfileImageUrls: pixiv.ProfileImageUrls{Px50x50: "https://example.invalid/avatar.jpg"},
	}
	w.Tags = []pixiv.Tag{
		{Name: "オリジナル", TranslatedName: "原创"},
		{Name: "少女"},
	}
	w.TotalBookmarks = 12480
	w.TotalView = 86200
	w.Caption = "夏の終わりに。青い光の中で佇む少女を描きました。"
	w.Series = &pixiv.Series{ID: 5, Title: "青の記憶"}
	w.CreateDate = time.Date(2026, 10, 24, 21, 30, 0, 0, time.Local)
	w.IsBookmarked = true
	return w
}

// waitViewerDetail waits for the info card's detail fetch to settle as want
// says.
func waitViewerDetail(t *testing.T, a *App, what string, want func(*pixiv.Illustration, string) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if a.viewer != nil {
			detail, errText, _ := a.viewer.detailState()
			if want(detail, errText) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("viewer detail did not %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestViewerDetailFetch checks the detail request fills the info card with the
// fields the feed's copy does not carry.
func TestViewerDetailFetch(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.session = &pixiv.Token{}
	detail := viewerDetail()
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return &detail, nil
	}
	w := viewerWork()
	a.openViewer(&w, 0)
	waitViewerDetail(t, a, "load", func(d *pixiv.Illustration, _ string) bool { return d != nil })
	if bookmarked, _ := a.viewer.bookmarkState(); !bookmarked {
		t.Error("the detail's bookmark state did not land")
	}

	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{
		"しろ", "ID 204811", "查看器作品",
		"12,480", "86,200", "收藏", "浏览",
		"2026-10-24 21:30", "42",
		"オリジナル", "原创", "少女",
		"夏の終わりに。", "青の記憶",
	} {
		if !tt.HasText(want) {
			t.Errorf("%q not shown: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("作者"); !ok {
		t.Errorf("the author row has no label: %q", tt.Texts())
	}
	if _, ok := tt.Find("作品详情"); !ok {
		t.Errorf("the info card has no label: %q", tt.Texts())
	}
}

// TestViewerDetailFailure checks a failed detail keeps the feed's data and
// offers a retry that fetches again.
func TestViewerDetailFailure(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.session = &pixiv.Token{}
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return nil, errors.New("网络错误")
	}
	w := viewerWork()
	a.openViewer(&w, 0)
	waitViewerDetail(t, a, "fail", func(_ *pixiv.Illustration, errText string) bool { return errText != "" })

	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("查看器作品") {
		t.Errorf("the feed's title is gone: %q", tt.Texts())
	}
	if !tt.HasText("网络错误") {
		t.Errorf("the failure is not shown: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Fatalf("no retry: %q", tt.Texts())
	}

	detail := viewerDetail()
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return &detail, nil
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitViewerDetail(t, a, "load after the retry", func(d *pixiv.Illustration, _ string) bool {
		return d != nil
	})
}

// TestViewerTagSearch checks clicking a tag searches for it and leaves the
// viewer.
func TestViewerTagSearch(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.session = &pixiv.Token{}
	// The search itself must not reach the network either.
	a.client.HTTP().Transport = newImageTransport(nil)
	detail := viewerDetail()
	detail.Tags = []pixiv.Tag{{Name: "オリジナル"}}
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return &detail, nil
	}
	w := viewerWork()
	a.openViewer(&w, 0)
	waitViewerDetail(t, a, "load", func(d *pixiv.Illustration, _ string) bool { return d != nil })

	tt := ui.NewTester(a.View, 1280, 820)
	if err := tt.Click("オリジナル"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.route != routeSearch {
		t.Errorf("route %q after the tag click, want %q", a.route, routeSearch)
	}
	if a.search.query != "オリジナル" {
		t.Errorf("search query %q, want %q", a.search.query, "オリジナル")
	}
	if a.viewer != nil {
		t.Error("the viewer stayed open")
	}
}

// TestViewerPageThumbs checks a thumbnail turns to its page.
func TestViewerPageThumbs(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)

	tt := ui.NewTester(a.View, 1280, 820)
	if err := tt.Click("第 2 页"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if got := a.viewer.page; got != 1 {
		t.Errorf("page %d after the thumbnail, want 1", got)
	}
	if err := tt.Click("第 1 页"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if got := a.viewer.page; got != 0 {
		t.Errorf("page %d after the first thumbnail, want 0", got)
	}
}

// TestViewerDetailScrolls checks a tall info card scrolls inside itself: the
// composer starts below the window and comes into view without moving the
// page.
func TestViewerDetailScrolls(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.session = &pixiv.Token{}
	detail := viewerDetail()
	detail.Caption = strings.Repeat("長い説明。", 120)
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return &detail, nil
	}
	w := viewerWork()
	a.openViewer(&w, 0)
	waitViewerDetail(t, a, "load", func(d *pixiv.Illustration, _ string) bool { return d != nil })

	tt := ui.NewTester(a.View, 1000, 700)
	before, ok := tt.Find("发送")
	if !ok {
		t.Fatalf("the composer is missing: %q", tt.Texts())
	}
	if before.Y < 700 {
		t.Fatalf("the composer is already in view at %+v; the fixture is too short", before)
	}
	// The card's center at 1000 wide: 32 of padding, a 320 card, and a gap.
	tt.Scroll(808, 350, 0, 4000)
	tt.Frame()
	after, ok := tt.Find("发送")
	if !ok {
		t.Fatalf("the composer is missing after the scroll: %q", tt.Texts())
	}
	if after.Y < 0 || after.Y+after.H > 700 {
		t.Errorf("the composer did not scroll into view: %+v", after)
	}
}

// the design targets: nothing it draws reaches past the right or bottom edge.
func TestViewerDetailLayout(t *testing.T) {
	for _, size := range []struct {
		w, h int
	}{{1280, 820}, {1000, 700}} {
		a := newViewerApp(newImageTransport(nil))
		a.session = &pixiv.Token{}
		detail := viewerDetail()
		a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
			return &detail, nil
		}
		w := viewerWork()
		a.openViewer(&w, 0)
		waitViewerDetail(t, a, "load", func(d *pixiv.Illustration, _ string) bool { return d != nil })

		tt := ui.NewTester(a.View, size.w, size.h)
		for _, label := range []string{"关闭", "作品详情", "复制链接", "在浏览器打开"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Errorf("%dx%d: %q is missing", size.w, size.h, label)
				continue
			}
			if r.X < 0 || r.Y < 0 || r.X+r.W > float32(size.w)+0.5 || r.Y+r.H > float32(size.h)+0.5 {
				t.Errorf("%dx%d: %q at %+v is outside the window", size.w, size.h, label, r)
			}
		}
	}
}
