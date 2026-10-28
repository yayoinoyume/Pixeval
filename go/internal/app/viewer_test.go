package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// viewerWork is a three-page work with an original for each page.
func viewerWork() pixiv.Illustration {
	return pixiv.Illustration{
		ID:        42,
		Title:     "查看器作品",
		PageCount: 3,
		Width:     800,
		Height:    1200,
		ImageUrls: pixiv.ImageUrls{Large: "https://example.invalid/large.jpg"},
		MetaPages: []pixiv.MetaPage{
			{ImageUrls: pixiv.ImageUrls{Original: "https://example.invalid/0.jpg"}},
			{ImageUrls: pixiv.ImageUrls{Original: "https://example.invalid/1.jpg"}},
			{ImageUrls: pixiv.ImageUrls{Original: "https://example.invalid/2.jpg"}},
		},
	}
}

// newViewerApp is an app whose image loads fail at once, without a network.
func newViewerApp() *App {
	a := New()
	a.images = imagex.New(nil, "")
	return a
}

// TestOpenViewer checks opening a work's page fills the viewer with it.
func TestOpenViewer(t *testing.T) {
	a := newViewerApp()
	w := viewerWork()
	a.openViewer(&w, 1)
	if a.viewer == nil {
		t.Fatal("viewer not open")
	}
	if got := a.viewer.page; got != 1 {
		t.Errorf("page %d, want 1", got)
	}
	if got := a.viewer.work.ID; got != w.ID {
		t.Errorf("work %d, want %d", got, w.ID)
	}
	if got := a.viewer.zoom; got != 1 {
		t.Errorf("zoom %v, want 1", got)
	}
}

// TestViewerPageBounds checks the first and the last page do not turn past
// the work.
func TestViewerPageBounds(t *testing.T) {
	a := newViewerApp()
	w := viewerWork()
	a.openViewer(&w, 0)
	a.viewerGo(-1)
	if got := a.viewer.page; got != 0 {
		t.Errorf("page %d after going back from the first, want 0", got)
	}
	a.viewerGo(1)
	a.viewerGo(1)
	if got := a.viewer.page; got != 2 {
		t.Errorf("page %d, want 2", got)
	}
	a.viewerGo(1)
	if got := a.viewer.page; got != 2 {
		t.Errorf("page %d after going on from the last, want 2", got)
	}
}

// TestCloseViewer checks closing leaves no viewer to draw.
func TestCloseViewer(t *testing.T) {
	a := newViewerApp()
	w := viewerWork()
	a.openViewer(&w, 0)
	a.closeViewer()
	if a.viewer != nil {
		t.Fatal("viewer still open")
	}
}

// TestViewerLoading checks the frame still names the work and the page while
// the original loads.
func TestViewerLoading(t *testing.T) {
	a := newViewerApp()
	v := &viewerState{work: viewerWork(), page: 1, zoom: 1}
	v.loadKey = v.work.PageURL(v.page) // a load is in flight
	a.viewer = v
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("查看器作品") {
		t.Errorf("title not shown: %q", tt.Texts())
	}
	if !tt.HasText("2/3") {
		t.Errorf("page not shown: %q", tt.Texts())
	}
}
