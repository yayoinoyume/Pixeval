package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The viewer's zoom bounds, and the step one key press or wheel notch takes.
const (
	viewerMinZoom  = 0.5
	viewerMaxZoom  = 6
	viewerZoomStep = 1.1
)

// bookmarkTimeout bounds one bookmark request, so a stuck one cannot leave
// the button disabled forever.
const bookmarkTimeout = 30 * time.Second

// viewerState is the full-screen viewer of one work: the page shown, where
// the user zoomed and panned it to, and the original being loaded. The view
// runs on the main thread; mu guards the load and bookmark fields, which
// goroutines write.
type viewerState struct {
	work       pixiv.Illustration
	page       int
	zoom       float32
	panX, panY float32
	// viewW and viewH are the size of the image area in the last frame, which
	// the pan bounds and the zoom's anchor need.
	viewW, viewH float32

	mu      sync.Mutex
	loadKey string     // the page URL the load fields belong to
	bmp     *ui.Bitmap // the loaded original, nil until it is there
	failed  bool       // the load for loadKey failed

	// bookmarking is set while the bookmark request runs, so the button is
	// disabled until it lands; bookmarked is the work's state, which flips
	// only once the request succeeds. The work itself is never written after
	// openViewer, so copying it cannot race with this.
	bookmarking bool
	bookmarked  bool

	// drag is the pointer dragging the image; lastX and lastY are where it
	// was at the previous move.
	drag         bool
	lastX, lastY float32
}

// pageCount is the work's number of pages, at least one.
func pageCount(w pixiv.Illustration) int {
	return pageCountOf(w.PageCount)
}

// pageCountOf is the number of pages a page count names, at least one.
func pageCountOf(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// bookmarked reports whether the work is bookmarked, and whether a request
// is still running.
func (v *viewerState) bookmarkState() (bookmarked, bookmarking bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.bookmarked, v.bookmarking
}

// openViewer shows w full screen over the page's content, at page.
func (a *App) openViewer(w *pixiv.Illustration, page int) {
	if w == nil {
		return
	}
	if n := pageCountOf(w.PageCount); page < 0 {
		page = 0
	} else if page >= n {
		page = n - 1
	}
	v := &viewerState{work: *w, page: page, zoom: 1, bookmarked: w.IsBookmarked}
	a.viewer = v
	a.recordHistory(*w)
	// Start the original at once, so the first frame already has a load to
	// show rather than one to start.
	a.viewerImage(v)
	if a.win != nil {
		a.win.Invalidate()
	}
}

// closeViewer returns to the page under the viewer.
func (a *App) closeViewer() {
	if a.viewer == nil {
		return
	}
	a.viewer = nil
	if a.win != nil {
		a.win.Invalidate()
	}
}

// viewerImage returns the current page's original and whether it failed. It
// starts the load when the page changed, and preloads the pages beside it.
func (a *App) viewerImage(v *viewerState) (bmp *ui.Bitmap, failed bool) {
	url := v.work.PageURL(v.page)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loadKey != url {
		v.loadKey = url
		v.bmp, v.failed = nil, false
		v.bmp = a.images.Original(url, func() {
			v.mu.Lock()
			if v.loadKey == url {
				// A cached original is a load that worked; a missing one is a
				// failure, which only the retry button loads again.
				v.bmp = a.images.Cached(url)
				v.failed = v.bmp == nil
			}
			v.mu.Unlock()
			a.update(func() {})
		})
		// The pages beside this one load ahead, so a page turn shows at once.
		for _, p := range []int{v.page - 1, v.page + 1} {
			if p >= 0 && p < pageCountOf(v.work.PageCount) {
				a.images.Original(v.work.PageURL(p), nil)
			}
		}
	}
	return v.bmp, v.failed
}

// toggleBookmark adds or removes the work's bookmark off the main thread. The
// button is disabled while the request runs, and the work's state flips only
// once it succeeds.
func (a *App) toggleBookmark(v *viewerState) {
	v.mu.Lock()
	if v.bookmarking || a.bookmarkFn == nil {
		v.mu.Unlock()
		return
	}
	v.bookmarking = true
	on, id := !v.bookmarked, v.work.ID
	v.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		err := a.bookmarkFn(ctx, id, on)
		a.update(func() {
			v.mu.Lock()
			v.bookmarking = false
			if err == nil {
				v.bookmarked = on
			}
			v.mu.Unlock()
			if err != nil {
				a.notice = "收藏失败：" + err.Error()
			}
		})
	}()
}

// viewerGo turns the page by delta, stopping at either end.
func (a *App) viewerGo(delta int) {
	v := a.viewer
	if v == nil {
		return
	}
	page := v.page + delta
	if page < 0 || page >= pageCountOf(v.work.PageCount) {
		return
	}
	v.page = page
	v.resetView()
	v.mu.Lock()
	v.loadKey = "" // the next frame loads the new page
	v.mu.Unlock()
	a.update(func() {})
}

// viewerReload loads the current page again, after a failure.
func (a *App) viewerReload(v *viewerState) {
	v.mu.Lock()
	v.loadKey = ""
	v.failed = false
	v.mu.Unlock()
	a.update(func() {})
}

// resetView puts the image back at its size, centered.
func (v *viewerState) resetView() {
	v.zoom = 1
	v.panX, v.panY = 0, 0
}

// zoomAt multiplies the zoom around the point at px, py, measured from the
// middle of the image area, so what is under the pointer stays put.
func (v *viewerState) zoomAt(scale, px, py float32) {
	if !(scale > 0) {
		return
	}
	if v.zoom <= 0 {
		v.zoom = 1
	}
	old := v.zoom
	v.zoom = min(max(old*scale, viewerMinZoom), viewerMaxZoom)
	f := v.zoom / old
	v.panX = px*(1-f) + v.panX*f
	v.panY = py*(1-f) + v.panY*f
	v.clampPan()
}

// clampPan keeps the image from being dragged off the view: one larger than
// the view stops at its edges, and one that fits stays centered.
func (v *viewerState) clampPan() {
	v.mu.Lock()
	bmp := v.bmp
	v.mu.Unlock()
	if bmp == nil || v.viewW <= 0 || v.viewH <= 0 {
		v.panX, v.panY = 0, 0
		return
	}
	bw, bh := bmp.Size()
	if bw <= 0 || bh <= 0 {
		v.panX, v.panY = 0, 0
		return
	}
	// The image is contained in the view, then scaled about its middle.
	contained := min(v.viewW/float32(bw), v.viewH/float32(bh))
	halfW := float32(bw) * contained * v.zoom / 2
	halfH := float32(bh) * contained * v.zoom / 2
	maxX := max(halfW-v.viewW/2, 0)
	maxY := max(halfH-v.viewH/2, 0)
	v.panX = min(max(v.panX, -maxX), maxX)
	v.panY = min(max(v.panY, -maxY), maxY)
}

// viewerInput takes the viewer's zoom keys, wheel, pinch and drag.
func (a *App) viewerInput(v *viewerState, ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		switch ev.Key {
		case ui.KeyEqual: // the + key, shifted or not
			v.zoomAt(viewerZoomStep, 0, 0)
		case ui.KeyMinus:
			v.zoomAt(1/viewerZoomStep, 0, 0)
		case ui.Key0:
			v.resetView()
		default:
			return false
		}
		return true
	case ui.InputZoom:
		px, py := ev.X-v.viewW/2, ev.Y-v.viewH/2
		v.zoomAt(ev.Scale, px, py)
		return true
	case ui.InputScroll:
		// A wheel notch zooms; a touchpad's precise scroll is left alone.
		if ev.Precise || ev.DY == 0 {
			return false
		}
		px, py := ev.X-v.viewW/2, ev.Y-v.viewH/2
		if ev.DY < 0 {
			v.zoomAt(viewerZoomStep, px, py)
		} else {
			v.zoomAt(1/viewerZoomStep, px, py)
		}
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		v.drag, v.lastX, v.lastY = true, ev.X, ev.Y
		return true
	case ui.InputPointerMove:
		if !v.drag {
			return false
		}
		v.panX += ev.X - v.lastX
		v.panY += ev.Y - v.lastY
		v.lastX, v.lastY = ev.X, ev.Y
		v.clampPan()
		return true
	case ui.InputPointerUp:
		if !v.drag {
			return false
		}
		v.drag = false
		return true
	}
	return false
}

// viewerPage draws the viewer over the whole window: the page's original,
// and the toolbar under it.
func (a *App) viewerPage(c *ui.Context) {
	v := a.viewer
	if v == nil {
		return
	}
	bmp, failed := a.viewerImage(v)
	t := c.Theme()
	// The window's keys, so they work whatever has the focus.
	c.OnShortcut(0, ui.KeyEscape, a.closeViewer)
	c.OnShortcut(0, ui.KeyLeft, func() { a.viewerGo(-1) })
	c.OnShortcut(0, ui.KeyRight, func() { a.viewerGo(1) })
	ui.Column(c).Fill().FillHeight().Background(t.Background).Children(func() {
		area := ui.Box(c).Grow(1).FillWidth().Clip().Background(t.Background).
			Focusable().AutoFocus().
			HandleInput(func(ev ui.InputEvent) bool { return a.viewerInput(v, ev) })
		if b := area.LayoutBounds(); b.W > 0 && b.H > 0 {
			v.viewW, v.viewH = b.W, b.H
		}
		area.Children(func() {
			switch {
			case bmp != nil:
				ui.Image(c, bmp).Fill().Fit(ui.Contain).
					Scale(v.zoom, v.zoom).Translate(v.panX, v.panY)
			case failed:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "图片加载失败").TextColor(t.Danger)
				})
			default:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Spinner(c)
				})
			}
		})
		a.viewerBar(c, v, failed)
	})
}

// viewerBar is the work's title, the page count and the viewer's buttons.
func (a *App) viewerBar(c *ui.Context, v *viewerState, failed bool) {
	t := c.Theme()
	n := pageCountOf(v.work.PageCount)
	ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).KeepFocus().Children(func() {
		ui.Text(c, v.work.Title).Grow(1).MaxLines(1)
		ui.Text(c, fmt.Sprintf("%d/%d", v.page+1, n)).TextColor(t.TextMuted)
		if failed {
			if ui.Button(c, "重试").Clicked() {
				a.viewerReload(v)
			}
		}
		bookmarked, bookmarking := v.bookmarkState()
		label := "收藏"
		if bookmarked {
			label = "已收藏"
		}
		bookmark := ui.Button(c, label).Disabled(bookmarking)
		if bookmarked {
			bookmark = bookmark.TextColor(t.Accent)
		}
		if bookmark.Clicked() {
			a.toggleBookmark(v)
		}
		if ui.Button(c, "下载").Clicked() {
			queued, ok := a.downloadWork(v.work)
			switch {
			case !ok:
				c.Toast("无法创建下载目录")
			case queued == 0:
				c.Toast("已在下载队列中")
			default:
				c.Toast(fmt.Sprintf("已加入 %d 页到下载队列", queued))
			}
		}
		if ui.Button(c, "上一页").Disabled(v.page <= 0).Clicked() {
			a.viewerGo(-1)
		}
		if ui.Button(c, "下一页").Disabled(v.page >= n-1).Clicked() {
			a.viewerGo(1)
		}
		if ui.Button(c, "关闭").Clicked() {
			a.closeViewer()
		}
	})
}
