package imagex

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// testImage is a w×h gradient, big enough that a JPEG of it is not tiny.
func testImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	return img
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage(w, h)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeDownload serves data by URL and counts the calls.
type fakeDownload struct {
	data  map[string][]byte
	err   error
	calls atomic.Int64

	mu      sync.Mutex
	active  int
	maxSeen int
	hold    time.Duration
}

func (f *fakeDownload) get(ctx context.Context, url string) ([]byte, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.active++
	if f.active > f.maxSeen {
		f.maxSeen = f.active
	}
	hold := f.hold
	f.mu.Unlock()
	if hold > 0 {
		time.Sleep(hold)
	}
	f.mu.Lock()
	f.active--
	f.mu.Unlock()

	f.mu.Lock()
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	data, ok := f.data[url]
	if !ok {
		return nil, errors.New("fake: unknown url")
	}
	return data, nil
}

func newTestLoader(t *testing.T, f *fakeDownload) *Loader {
	t.Helper()
	l := New(nil, t.TempDir())
	l.download = f.get
	return l
}

// thumb loads a thumbnail and returns the bitmap once it is there.
func thumb(t *testing.T, l *Loader, key string, w, h int) *ui.Bitmap {
	t.Helper()
	done := make(chan struct{})
	if bmp := l.Thumb(key, w, h, func() { close(done) }); bmp != nil {
		return bmp
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("thumb: load timed out")
	}
	bmp := l.Thumb(key, w, h, nil)
	if bmp == nil {
		t.Fatal("thumb: not cached after load")
	}
	return bmp
}

func TestThumbScalesDown(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{
		"u/jpeg": testJPEG(t, 800, 600),
		"u/png":  testPNG(t, 800, 600),
	}}
	l := newTestLoader(t, f)

	for _, key := range []string{"u/jpeg", "u/png"} {
		bmp := thumb(t, l, key, 200, 200)
		if w, h := bmp.Size(); w != 200 || h != 150 {
			t.Errorf("%s: size %dx%d, want 200x150", key, w, h)
		}
	}
}

func TestThumbDoesNotScaleUp(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{"u": testJPEG(t, 400, 300)}}
	l := newTestLoader(t, f)

	if w, h := thumb(t, l, "u", 1000, 1000).Size(); w != 400 || h != 300 {
		t.Errorf("size %dx%d, want 400x300", w, h)
	}
}

func TestOriginalKeepsSize(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{"u": testPNG(t, 320, 240)}}
	l := newTestLoader(t, f)

	done := make(chan struct{})
	if bmp := l.Original("u", func() { close(done) }); bmp != nil {
		t.Fatal("original: loaded synchronously")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("original: load timed out")
	}
	bmp := l.Original("u", nil)
	if bmp == nil {
		t.Fatal("original: not cached after load")
	}
	if w, h := bmp.Size(); w != 320 || h != 240 {
		t.Errorf("size %dx%d, want 320x240", w, h)
	}
}

func TestDownloadsOncePerKey(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{"u": testJPEG(t, 200, 200)}}
	l := newTestLoader(t, f)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done := make(chan struct{})
			l.Thumb("u", 100, 100, func() { close(done) })
			<-done
		}()
	}
	wg.Wait()
	thumb(t, l, "u", 100, 100)

	if n := f.calls.Load(); n != 1 {
		t.Errorf("downloads %d, want 1", n)
	}
}

func TestDiskCacheSurvivesLoader(t *testing.T) {
	dir := t.TempDir()
	f := &fakeDownload{data: map[string][]byte{"u": testJPEG(t, 200, 200)}}

	first := New(nil, dir)
	first.download = f.get
	thumb(t, first, "u", 100, 100)

	second := New(nil, dir)
	second.download = func(context.Context, string) ([]byte, error) {
		t.Error("downloaded again, want a disk hit")
		return nil, errors.New("no network")
	}
	if w, h := thumb(t, second, "u", 50, 50).Size(); w != 50 || h != 50 {
		t.Errorf("size %dx%d, want 50x50", w, h)
	}
	if n := f.calls.Load(); n != 1 {
		t.Errorf("downloads %d, want 1", n)
	}
}

func TestDiskLimitEvicts(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{
		"u1": testJPEG(t, 200, 200),
		"u2": testJPEG(t, 200, 200),
	}}
	l := newTestLoader(t, f)
	l.SetDiskLimit(1)

	thumb(t, l, "u1", 100, 100)
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%d files left, want 0", len(entries))
	}
}

func TestMemoryLRUEvicts(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{
		"u1": testJPEG(t, 100, 100),
		"u2": testJPEG(t, 100, 100),
	}}
	l := newTestLoader(t, f)
	// Room for one 100x100 bitmap only.
	l.SetBudget(100 * 100 * 4)

	thumb(t, l, "u1", 100, 100)
	thumb(t, l, "u2", 100, 100)

	if bmp := l.Thumb("u1", 100, 100, nil); bmp != nil {
		t.Error("u1 still cached, want it evicted")
	}
	if bmp := l.Thumb("u2", 100, 100, nil); bmp == nil {
		t.Error("u2 evicted, want it cached")
	}
	if n := f.calls.Load(); n != 2 {
		t.Errorf("downloads %d, want 2", n)
	}
}

func TestFailureIsNotCached(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{"u": testJPEG(t, 100, 100)}, err: errors.New("boom")}
	l := newTestLoader(t, f)

	done := make(chan struct{})
	l.Thumb("u", 50, 50, func() { close(done) })
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("load timed out")
	}
	if bmp := l.Thumb("u", 50, 50, nil); bmp != nil {
		t.Fatal("failed load cached a bitmap")
	}

	f.mu.Lock()
	f.err = nil
	f.mu.Unlock()
	if w, h := thumb(t, l, "u", 50, 50).Size(); w != 50 || h != 50 {
		t.Errorf("size %dx%d, want 50x50", w, h)
	}
	if n := f.calls.Load(); n < 2 {
		t.Errorf("downloads %d, want a retry", n)
	}
}

func TestConcurrentLimit(t *testing.T) {
	f := &fakeDownload{data: map[string][]byte{}, hold: 30 * time.Millisecond}
	for i := 0; i < 24; i++ {
		f.data[string(rune('a'+i))] = testJPEG(t, 80, 80)
	}
	l := newTestLoader(t, f)

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			done := make(chan struct{})
			l.Thumb(key, 40, 40, func() { close(done) })
			<-done
		}(string(rune('a' + i)))
	}
	wg.Wait()

	f.mu.Lock()
	max := f.maxSeen
	f.mu.Unlock()
	if max > maxDownloads {
		t.Errorf("%d downloads at once, want at most %d", max, maxDownloads)
	}
	if max < 2 {
		t.Errorf("%d downloads at once, want the limit used", max)
	}
}
