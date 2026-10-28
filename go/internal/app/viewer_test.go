package app

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

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

// testPNG is a tiny image the test transport serves instead of a download.
var testPNG = func() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 6))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// imageTransport serves testPNG without a network, counts the downloads of
// each URL, and fails the first fail[url] calls of it.
type imageTransport struct {
	mu    sync.Mutex
	calls map[string]int
	fail  map[string]int
}

func newImageTransport(fail map[string]int) *imageTransport {
	return &imageTransport{calls: map[string]int{}, fail: fail}
}

func (tr *imageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	tr.mu.Lock()
	tr.calls[url]++
	n, fails := tr.calls[url], tr.fail[url]
	tr.mu.Unlock()
	if n <= fails {
		return nil, errors.New("下载失败")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(testPNG)),
		Request:    req,
	}, nil
}

func (tr *imageTransport) count(url string) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.calls[url]
}

// newViewerApp is an app whose images come from tr, without a network. It is
// signed out, so opening the viewer fetches neither the detail nor the
// comments.
func newViewerApp(tr *imageTransport) *App {
	a := New()
	a.session = nil
	client := pixiv.NewClient()
	client.HTTP().Transport = tr
	a.images = imagex.New(client, "")
	return a
}

// viewerLoad returns the viewer's image and failure under its lock.
func viewerLoad(v *viewerState) (bmp *ui.Bitmap, failed bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.bmp, v.failed
}

// waitViewer waits for the viewer's load to settle as want says.
func waitViewer(t *testing.T, a *App, what string, want func(bmp *ui.Bitmap, failed bool) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if a.viewer != nil {
			if bmp, failed := viewerLoad(a.viewer); want(bmp, failed) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("viewer did not %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestOpenViewer checks opening a work's page fills the viewer with it.
func TestOpenViewer(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
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

// TestOpenViewerPageRange checks a page outside the work opens on the nearest
// page inside it.
func TestOpenViewerPageRange(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 9)
	if got := a.viewer.page; got != 2 {
		t.Errorf("page %d for 9, want 2", got)
	}
	a.openViewer(&w, -1)
	if got := a.viewer.page; got != 0 {
		t.Errorf("page %d for -1, want 0", got)
	}
}

// TestViewerPageBounds checks the first and the last page do not turn past
// the work.
func TestViewerPageBounds(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
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

// TestViewerSinglePage checks a work with one page, or without a page count,
// does not turn.
func TestViewerSinglePage(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := pixiv.Illustration{ID: 7, Title: "单页", Width: 100, Height: 100}
	a.openViewer(&w, 0)
	if got := pageCount(a.viewer.work); got != 1 {
		t.Errorf("pageCount %d, want 1", got)
	}
	a.viewerGo(1)
	a.viewerGo(-1)
	if got := a.viewer.page; got != 0 {
		t.Errorf("page %d, want 0", got)
	}
}

// TestCloseViewer checks closing leaves no viewer to draw.
func TestCloseViewer(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
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
	a := newViewerApp(newImageTransport(nil))
	v := &viewerState{work: viewerWork(), page: 1, zoom: 1}
	v.loadKey = v.work.PageURL(v.page) // a load is in flight
	a.viewer = v
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("查看器作品") {
		t.Errorf("title not shown: %q", tt.Texts())
	}
	if !tt.HasText("第 2 页，共 3 页") {
		t.Errorf("page not shown: %q", tt.Texts())
	}
}

// TestViewerLoaded checks a loaded original is shown without a failure.
func TestViewerLoaded(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	waitViewer(t, a, "load", func(bmp *ui.Bitmap, _ bool) bool { return bmp != nil })
	tt := ui.NewTester(a.View, 1000, 700)
	if tt.HasText("图片加载失败") {
		t.Errorf("failure shown for a loaded image: %q", tt.Texts())
	}
	if tt.HasText("重试") {
		t.Errorf("retry shown for a loaded image: %q", tt.Texts())
	}
	if !tt.HasText("第 1 页，共 3 页") {
		t.Errorf("page not shown: %q", tt.Texts())
	}
	if _, ok := tt.Find("作品图片：查看器作品（第 1/3 页）"); !ok {
		t.Errorf("the image has no label: %q", tt.Texts())
	}
}

// TestViewerFailedRetry checks a failed load offers a retry, that the failure
// does not download again by itself, and that the retry loads once more.
func TestViewerFailedRetry(t *testing.T) {
	w := viewerWork()
	url := w.PageURL(0)
	tr := newImageTransport(map[string]int{url: 1})
	a := newViewerApp(tr)
	a.openViewer(&w, 0)
	waitViewer(t, a, "fail", func(_ *ui.Bitmap, failed bool) bool { return failed })
	if got := tr.count(url); got != 1 {
		t.Fatalf("downloads %d after the failure, want 1", got)
	}
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("图片加载失败") {
		t.Errorf("failure not shown: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Fatalf("retry not shown: %q", tt.Texts())
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitViewer(t, a, "load after the retry", func(bmp *ui.Bitmap, _ bool) bool { return bmp != nil })
	if got := tr.count(url); got != 2 {
		t.Errorf("downloads %d after the retry, want 2", got)
	}
}

// TestViewerPanClamp checks the image stops at the view's edges when it is
// larger than the view, and stays centered when it fits.
func TestViewerPanClamp(t *testing.T) {
	v := &viewerState{zoom: 2, viewW: 1000, viewH: 600}
	v.bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	// Contained at 6x, then zoomed twice: 1200 wide and high.
	v.panX, v.panY = 5000, -5000
	v.clampPan()
	if v.panX != 100 || v.panY != -300 {
		t.Errorf("pan %v,%v want 100,-300", v.panX, v.panY)
	}
	v.zoom = 1
	v.panX, v.panY = 200, 200
	v.clampPan()
	if v.panX != 0 || v.panY != 0 {
		t.Errorf("pan %v,%v for an image that fits, want 0,0", v.panX, v.panY)
	}
}

// TestViewerZoomAnchor checks zooming keeps the point under the pointer in
// place.
func TestViewerZoomAnchor(t *testing.T) {
	v := &viewerState{zoom: 1, viewW: 1000, viewH: 600}
	v.bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	v.zoomAt(2, 100, 0)
	if v.zoom != 2 {
		t.Fatalf("zoom %v, want 2", v.zoom)
	}
	if v.panX != -100 || v.panY != 0 {
		t.Errorf("pan %v,%v want -100,0", v.panX, v.panY)
	}
}

// TestViewerShortcuts checks the viewer takes the window's keys without the
// navigation under it, and that Escape brings the shell back.
func TestViewerShortcuts(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	tt := ui.NewTester(a.View, 1000, 700)
	if tt.HasText("首页") {
		t.Errorf("the shell is drawn under the viewer: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyRight)
	if got := a.viewer.page; got != 1 {
		t.Errorf("page %d after the right key, want 1", got)
	}
	tt.Key(0, ui.KeyLeft)
	if got := a.viewer.page; got != 0 {
		t.Errorf("page %d after the left key, want 0", got)
	}
	tt.Key(0, ui.KeyEscape)
	if a.viewer != nil {
		t.Fatal("escape did not close the viewer")
	}
	if !tt.HasText("首页") {
		t.Errorf("shell not back: %q", tt.Texts())
	}
}

// TestViewerZoomAfterActionClick checks clicking an action keeps the canvas
// focused, so the zoom keys still reach it.
func TestViewerZoomAfterActionClick(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("评论"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Key(0, ui.KeyEqual)
	if got := a.viewer.zoom; got <= 1 {
		t.Errorf("zoom %v after the + key, want more than 1", got)
	}
}

// TestViewerPageThumbsUseDetail checks the thumbnails take the smaller
// renditions the detail carries, not the feed's originals.
func TestViewerPageThumbsUseDetail(t *testing.T) {
	tr := newImageTransport(nil)
	a := newViewerApp(tr)
	w := viewerWork()
	detail := w
	detail.ImageUrls.Medium = "https://example.invalid/0-medium.jpg"
	detail.MetaPages[1].ImageUrls.Medium = "https://example.invalid/1-medium.jpg"
	detail.MetaPages[2].ImageUrls.Medium = "https://example.invalid/2-medium.jpg"
	a.openViewer(&w, 0)
	a.viewer.mu.Lock()
	a.viewer.detail = &detail
	a.viewer.mu.Unlock()

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	for _, url := range []string{detail.ImageUrls.Medium,
		detail.MetaPages[1].ImageUrls.Medium, detail.MetaPages[2].ImageUrls.Medium} {
		deadline := time.Now().Add(2 * time.Second)
		for tr.count(url) == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("the thumbnail %q was not requested", url)
			}
			time.Sleep(time.Millisecond)
		}
	}
}
