// Package imagex loads illustration images: the downloaded bytes go to a disk
// cache, the decoded bitmaps to a memory cache, and thumbnails are scaled down
// once and kept.
//
// Loading is asynchronous. Thumb and Original return the bitmap when it is
// already in memory, and nil while it is being fetched; onReady is called on
// the loading goroutine once the bitmap is there, and calling Thumb or
// Original again then returns it. The caller redraws from onReady with
// Window.Update.
package imagex

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"

	// Decoders for the formats Pixiv serves. GIF decodes its first frame.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	defaultBudget    = 256 << 20
	defaultDiskLimit = 512 << 20
	maxDownloads     = 6
	downloadTimeout  = 2 * time.Minute
)

// Loader caches illustrations from a Pixiv client. It is safe for concurrent
// use.
type Loader struct {
	dir      string
	download func(ctx context.Context, url string) ([]byte, error)

	mu        sync.Mutex
	mem       map[string]*list.Element // memory key -> element of *cached
	lru       *list.List               // front is most recently used
	memBytes  int64
	budget    int64
	diskLimit int64
	inflight  map[string][]func() // memory key -> callbacks of the load in progress

	diskMu sync.Mutex // serializes disk writes and eviction
	sem    chan struct{}
}

type cached struct {
	key   string
	bmp   *ui.Bitmap
	bytes int64
}

// New returns a loader caching in memory and, when diskDir is not empty, on
// disk under it.
func New(client *pixiv.Client, diskDir string) *Loader {
	l := &Loader{
		dir:       diskDir,
		mem:       map[string]*list.Element{},
		lru:       list.New(),
		budget:    defaultBudget,
		diskLimit: defaultDiskLimit,
		inflight:  map[string][]func(){},
		sem:       make(chan struct{}, maxDownloads),
	}
	if client != nil {
		l.download = client.GetImage
	} else {
		l.download = func(context.Context, string) ([]byte, error) {
			return nil, errors.New("imagex: no pixiv client")
		}
	}
	return l
}

// SetBudget sets the memory the decoded bitmaps may take, estimated as
// w*h*4 per bitmap, and drops the least recently used ones beyond it.
func (l *Loader) SetBudget(bytes int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.budget = bytes
	l.trim()
}

// SetDiskLimit sets the disk the cached bytes may take and trims it.
func (l *Loader) SetDiskLimit(bytes int64) {
	l.mu.Lock()
	l.diskLimit = bytes
	l.mu.Unlock()

	l.diskMu.Lock()
	l.evictDisk()
	l.diskMu.Unlock()
}

// Thumb returns the illustration at key scaled down to fit w×h, never up. It
// returns nil while the image loads and calls onReady once it is there.
func (l *Loader) Thumb(key string, w, h int, onReady func()) *ui.Bitmap {
	memKey := fmt.Sprintf("%s@%dx%d", key, w, h)
	return l.bitmap(memKey, key, onReady, func(img image.Image) image.Image {
		return fit(img, w, h)
	})
}

// Original returns the illustration at key at its own size. It returns nil
// while the image loads and calls onReady once it is there.
func (l *Loader) Original(key string, onReady func()) *ui.Bitmap {
	return l.bitmap(key+"@original", key, onReady, nil)
}

// bitmap returns the cached bitmap of memKey, or starts loading it and returns
// nil. One load per memory key runs at a time; callers that arrive meanwhile
// wait for it.
func (l *Loader) bitmap(memKey, url string, onReady func(), scale func(image.Image) image.Image) *ui.Bitmap {
	l.mu.Lock()
	if el, ok := l.mem[memKey]; ok {
		l.lru.MoveToFront(el)
		bmp := el.Value.(*cached).bmp
		l.mu.Unlock()
		return bmp
	}
	if _, ok := l.inflight[memKey]; ok {
		if onReady != nil {
			l.inflight[memKey] = append(l.inflight[memKey], onReady)
		}
		l.mu.Unlock()
		return nil
	}
	if onReady != nil {
		l.inflight[memKey] = []func(){onReady}
	} else {
		l.inflight[memKey] = nil
	}
	l.mu.Unlock()

	go l.load(memKey, url, scale)
	return nil
}

// load decodes one image, caches it and calls the waiting callbacks. A failed
// load caches nothing, so the next call tries again.
func (l *Loader) load(memKey, url string, scale func(image.Image) image.Image) {
	l.sem <- struct{}{}
	l.decode(memKey, url, scale)
	<-l.sem

	l.mu.Lock()
	wait := l.inflight[memKey]
	delete(l.inflight, memKey)
	l.mu.Unlock()

	for _, f := range wait {
		f()
	}
}

func (l *Loader) decode(memKey, url string, scale func(image.Image) image.Image) {
	data, err := l.bytes(url)
	if err != nil {
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	if scale != nil {
		img = scale(img)
	}
	b := img.Bounds()
	l.put(memKey, ui.NewBitmap(img), int64(b.Dx())*int64(b.Dy())*4)
}

// bytes returns the image data from the disk cache, or downloads it and writes
// it there.
func (l *Loader) bytes(url string) ([]byte, error) {
	if data, ok := l.diskGet(url); ok {
		return data, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	data, err := l.download(ctx, url)
	if err != nil {
		return nil, err
	}
	l.diskPut(url, data)
	return data, nil
}

// put caches a bitmap, replacing the memory key's older one and trimming the
// least recently used bitmaps beyond the budget. Dropping the last reference
// lets a bitmap and its pixels go; ReleasePixels is the main thread's, so it
// is not called here.
func (l *Loader) put(memKey string, bmp *ui.Bitmap, size int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.mem[memKey]; ok {
		l.lru.Remove(el)
		delete(l.mem, memKey)
		l.memBytes -= el.Value.(*cached).bytes
	}
	l.mem[memKey] = l.lru.PushFront(&cached{key: memKey, bmp: bmp, bytes: size})
	l.memBytes += size
	l.trim()
}

// trim drops the least recently used bitmaps beyond the budget. l.mu must be
// held.
func (l *Loader) trim() {
	for l.memBytes > l.budget {
		el := l.lru.Back()
		if el == nil {
			return
		}
		l.lru.Remove(el)
		c := el.Value.(*cached)
		delete(l.mem, c.key)
		l.memBytes -= c.bytes
	}
}

// fit scales img down to fit in w×h, keeping its aspect ratio. An image that
// already fits is returned as it is.
func fit(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || sw <= w && sh <= h {
		return img
	}
	tw, th := w, sh*w/sw
	if th > h {
		th, tw = h, sw*h/sh
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
