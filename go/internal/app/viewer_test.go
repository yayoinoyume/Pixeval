package app

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/animated"
	"github.com/yayoinoyume/pixeval-go/internal/artcolor"
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

// newViewerApp is an app whose images come through rt, without a network. It
// is signed out, so opening the viewer fetches neither the detail nor the
// comments.
func newViewerApp(rt http.RoundTripper) *App {
	a := New()
	a.session = nil
	client := pixiv.NewClient()
	client.HTTP().Transport = rt
	a.client = client
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

// The three colors the palette tests serve: one per page, and one for the
// cover's rendition.
var (
	seedRed   = color.NRGBA{R: 200, G: 40, B: 40, A: 255}
	seedBlue  = color.NRGBA{R: 40, G: 60, B: 200, A: 255}
	seedGreen = color.NRGBA{R: 40, G: 200, B: 60, A: 255}
)

// seedPNG is a tiny image of one solid color, which artcolor reads as exactly
// that color's seed.
func seedPNG(c color.NRGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// colorTransport serves one image per URL and fails every other, so a test
// knows which renditions decoded and which did not.
type colorTransport struct {
	bodies map[string][]byte
}

func (tr *colorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := tr.bodies[req.URL.String()]
	if !ok {
		return nil, errors.New("下载失败")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}, nil
}

// waitSeed waits for an image's color to be known and returns it.
func waitSeed(t *testing.T, a *App, url string) ui.Color {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if s, ok := a.images.Seed(url); ok {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("the seed of %q never arrived", url)
		}
		time.Sleep(time.Millisecond)
	}
}

// paletteFor is the token set a page made from seed draws with.
func paletteFor(seed ui.Color, dark bool) tokens {
	return tokensFor(dark).withPalette(artcolor.Derive(seed, dark))
}

// TestViewerSeedHoldsOnPageTurn checks turning to a page nothing has decoded
// yet keeps the previous page's color, instead of taking the color of the
// cover's rendition: the cover is a different image with different colors, so
// falling back to it makes the room flash a color neither page has.
func TestViewerSeedHoldsOnPageTurn(t *testing.T) {
	w := viewerWork()
	page0, page1 := w.PageURL(0), w.PageURL(1)
	// The cover's rendition, which the page thumbnails and the feed's card
	// decode under their own URL.
	cover := pageThumbURL(w, 0)
	if cover == page0 {
		t.Fatalf("the fixture's cover is the page's original: %q", cover)
	}
	a := newViewerApp(&colorTransport{bodies: map[string][]byte{
		page0: seedPNG(seedRed),
		cover: seedPNG(seedGreen),
	}})
	a.openViewer(&w, 0)
	// The thumbnails ask for the cover's rendition as the frame draws them.
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	red := waitSeed(t, a, page0)
	green := waitSeed(t, a, cover)
	if sameColor(red, green) {
		t.Fatal("the fixture's page and cover have the same color")
	}
	tt.Frame()
	if !sameColor(a.viewer.themeSeed, red) {
		t.Fatalf("seed %v for the page shown, want %v", a.viewer.themeSeed, red)
	}
	a.viewerGo(1)
	tt.Frame()
	if !sameColor(a.viewer.themeSeed, red) {
		t.Errorf("seed %v after the turn, want the previous page's %v, not the cover's %v",
			a.viewer.themeSeed, red, green)
	}
	if got, want := a.viewer.themeTokens.Bg, paletteFor(red, a.viewer.themeDark).Bg; !sameColor(got, want) {
		t.Errorf("room %v after the turn, want the red page's %v", got, want)
	}
	if got, unwanted := a.viewer.themeTokens.Bg, paletteFor(green, a.viewer.themeDark).Bg; sameColor(got, unwanted) {
		t.Errorf("room %v took the cover's color", got)
	}
	if s, ok := a.images.Seed(page1); ok {
		t.Fatalf("the fixture decoded page 1's own image: %v", s)
	}
}

// TestViewerSeedFollowsPage checks the room takes a page's own color once its
// original decodes: red, then blue, never stuck on the first page.
func TestViewerSeedFollowsPage(t *testing.T) {
	w := viewerWork()
	page0, page1 := w.PageURL(0), w.PageURL(1)
	a := newViewerApp(&colorTransport{bodies: map[string][]byte{
		page0: seedPNG(seedRed),
		page1: seedPNG(seedBlue),
	}})
	a.openViewer(&w, 0)
	red := waitSeed(t, a, page0)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !sameColor(a.viewer.themeSeed, red) {
		t.Fatalf("seed %v for the first page, want %v", a.viewer.themeSeed, red)
	}
	// The page beside the first loads ahead, so its color is known before the
	// turn in the ordinary case.
	blue := waitSeed(t, a, page1)
	a.viewerGo(1)
	tt.Frame()
	if !sameColor(a.viewer.themeSeed, blue) {
		t.Fatalf("seed %v for the second page, want %v", a.viewer.themeSeed, blue)
	}
	if got, want := a.viewer.themeTokens.Bg, paletteFor(blue, a.viewer.themeDark).Bg; !sameColor(got, want) {
		t.Errorf("room %v for the second page, want %v", got, want)
	}
}

// ugoiraZipURL is the zip the fake metadata points at.
const ugoiraZipURL = "https://example.invalid/ugoira.zip"

// ugoiraWork is a one-page animation.
func ugoiraWork() pixiv.Illustration {
	return pixiv.Illustration{
		ID:        77,
		Title:     "动图作品",
		Type:      "ugoira",
		PageCount: 1,
		Width:     4,
		Height:    4,
		ImageUrls: pixiv.ImageUrls{Large: "https://example.invalid/ugoira-cover.jpg"},
		MetaSinglePage: pixiv.MetaSinglePage{
			OriginalImageURL: "https://example.invalid/ugoira-still.jpg",
		},
	}
}

// ugoiraZip is a zip of n solid frames, which animated.ExtractZip decodes in
// the order the metadata lists.
func ugoiraZip(t *testing.T, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < n; i++ {
		w, err := zw.Create(fmt.Sprintf("frame_%d.png", i))
		if err != nil {
			t.Fatalf("zip.Create: %v", err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		c := color.RGBA{R: uint8(40 + 40*i), G: 120, B: 40, A: 255}
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		if err := png.Encode(w, img); err != nil {
			t.Fatalf("png.Encode: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// ugoiraMetadataJSON is the metadata endpoint's answer for n frames.
func ugoiraMetadataJSON(n int) []byte {
	frames := make([]string, n)
	for i := range frames {
		frames[i] = fmt.Sprintf(`{"file":"frame_%d.png","delay":20}`, i)
	}
	return []byte(fmt.Sprintf(`{"ugoira_metadata":{"zip_urls":{"medium":"%s"},"frames":[%s]}}`,
		ugoiraZipURL, strings.Join(frames, ",")))
}

// ugoiraTransport serves the animation's metadata and zip, and a PNG for the
// covers and avatars the viewer asks for, without a network.
type ugoiraTransport struct {
	frames  int
	zipData []byte

	mu       sync.Mutex
	metaFail bool
	zipErr   error
	zipCalls int
}

func (tr *ugoiraTransport) setMetaFail(fail bool) {
	tr.mu.Lock()
	tr.metaFail = fail
	tr.mu.Unlock()
}

func (tr *ugoiraTransport) zipCount() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.zipCalls
}

func (tr *ugoiraTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	metaFail, zipErr := tr.metaFail, tr.zipErr
	isZip := req.URL.String() == ugoiraZipURL
	if isZip {
		tr.zipCalls++
	}
	tr.mu.Unlock()
	if strings.HasSuffix(req.URL.Path, "/v1/ugoira/metadata") {
		if metaFail {
			// A 404 is a failure the client does not retry, so the test does
			// not wait through the backoff.
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		return testResponse(req, "application/json", ugoiraMetadataJSON(tr.frames)), nil
	}
	if isZip {
		if zipErr != nil {
			return nil, zipErr
		}
		return testResponse(req, "application/zip", tr.zipData), nil
	}
	return testResponse(req, "image/png", testPNG), nil
}

// testResponse is a 200 answer of body in one content type.
func testResponse(req *http.Request, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

// waitUgoira waits for the animation's state to settle as want says.
func waitUgoira(t *testing.T, v *viewerState, what string, want func(frames *animated.Frames, frame int, playing, loading, failed bool) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		frames, frame, playing, loading, failed := v.ugoiraState()
		if want(frames, frame, playing, loading, failed) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ugoira did not %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestViewerUgoiraPlays checks the frames load once, the animation advances,
// and the play control pauses it.
func TestViewerUgoiraPlays(t *testing.T) {
	tr := &ugoiraTransport{frames: 3, zipData: ugoiraZip(t, 3)}
	a := newViewerApp(tr)
	w := ugoiraWork()
	a.openViewer(&w, 0)
	waitUgoira(t, a.viewer, "load", func(frames *animated.Frames, _ int, playing, loading, failed bool) bool {
		return frames != nil && playing && !loading && !failed
	})
	frames, frame, _, _, _ := a.viewer.ugoiraState()
	if len(frames.Images) != 3 {
		t.Fatalf("frames %d, want 3", len(frames.Images))
	}
	if len(frames.Delays) != 3 || frames.Delays[0] != 20*time.Millisecond {
		t.Fatalf("delays %v, want three of 20ms", frames.Delays)
	}
	waitUgoira(t, a.viewer, "advance", func(_ *animated.Frames, f int, _ bool, _, _ bool) bool {
		return f != frame
	})

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if _, ok := tt.Find("动图：动图作品"); !ok {
		t.Errorf("the frame has no label: %q", tt.Texts())
	}
	if err := tt.Click("暂停"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	_, paused, playing, _, _ := a.viewer.ugoiraState()
	if playing {
		t.Fatal("still playing after pause")
	}
	time.Sleep(100 * time.Millisecond)
	if _, now, _, _, _ := a.viewer.ugoiraState(); now != paused {
		t.Errorf("frame moved from %d to %d while paused", paused, now)
	}
	if got := tr.zipCount(); got != 1 {
		t.Errorf("zip downloads %d, want 1", got)
	}
}

// TestViewerUgoiraStops checks switching the work and closing the viewer both
// stop the animation.
func TestViewerUgoiraStops(t *testing.T) {
	tr := &ugoiraTransport{frames: 2, zipData: ugoiraZip(t, 2)}
	a := newViewerApp(tr)
	w := ugoiraWork()
	a.openViewer(&w, 0)
	waitUgoira(t, a.viewer, "load", func(frames *animated.Frames, _ int, playing, _, _ bool) bool {
		return frames != nil && playing
	})
	old := a.viewer
	other := viewerWork()
	a.openViewer(&other, 0)
	if _, _, playing, _, _ := old.ugoiraState(); playing {
		t.Fatal("the replaced viewer still plays")
	}
	_, frame, _, _, _ := old.ugoiraState()
	time.Sleep(100 * time.Millisecond)
	if _, now, _, _, _ := old.ugoiraState(); now != frame {
		t.Errorf("the replaced viewer moved from frame %d to %d", frame, now)
	}

	a.openViewer(&w, 0)
	waitUgoira(t, a.viewer, "load again", func(frames *animated.Frames, _ int, playing, _, _ bool) bool {
		return frames != nil && playing
	})
	closed := a.viewer
	a.closeViewer()
	if _, _, playing, _, _ := closed.ugoiraState(); playing {
		t.Fatal("the closed viewer still plays")
	}
	_, frame, _, _, _ = closed.ugoiraState()
	time.Sleep(100 * time.Millisecond)
	if _, now, _, _, _ := closed.ugoiraState(); now != frame {
		t.Errorf("the closed viewer moved from frame %d to %d", frame, now)
	}
}

// TestViewerUgoiraFailed checks a failed load says so and that the retry
// fetches the frames again.
func TestViewerUgoiraFailed(t *testing.T) {
	tr := &ugoiraTransport{
		frames:   2,
		zipData:  ugoiraZip(t, 2),
		metaFail: true,
	}
	a := newViewerApp(tr)
	w := ugoiraWork()
	a.openViewer(&w, 0)
	waitUgoira(t, a.viewer, "fail", func(_ *animated.Frames, _ int, _, _, failed bool) bool {
		return failed
	})
	if got := tr.zipCount(); got != 0 {
		t.Fatalf("zip downloads %d after the metadata failed, want 0", got)
	}
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !tt.HasText("动图加载失败") {
		t.Errorf("failure not shown: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Fatalf("retry not shown: %q", tt.Texts())
	}
	tr.setMetaFail(false)
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitUgoira(t, a.viewer, "load after the retry", func(frames *animated.Frames, _ int, playing, _, _ bool) bool {
		return frames != nil && playing
	})
}
