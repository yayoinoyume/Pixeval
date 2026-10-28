package app

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The viewer's zoom bounds, and the step one key press or wheel notch takes.
const (
	viewerMinZoom  = 0.5
	viewerMaxZoom  = 6
	viewerZoomStep = 1.1
)

// viewerState is the full-screen viewer of one work: the page shown, where
// the user zoomed and panned it to, and the original being loaded. The view
// runs on the main thread; mu guards the load fields, which imagex's callback
// writes from its loading goroutine.
type viewerState struct {
	work       pixiv.Illustration
	page       int
	zoom       float32
	panX, panY float32

	mu       sync.Mutex
	loadKey  string     // the page URL the load fields belong to
	bmp      *ui.Bitmap // the loaded original, nil until it is there
	loadDone bool       // the load for loadKey ended
	fetched  bool       // the view took the load's result

	// drag is the pointer dragging the image; lastX and lastY are where it
	// was at the previous move.
	drag         bool
	lastX, lastY float32
}

// pageCount is the work's number of pages, at least one.
func pageCount(w pixiv.Illustration) int {
	if w.PageCount < 1 {
		return 1
	}
	return w.PageCount
}

// openViewer shows w full screen over the page's content, at page.
func (a *App) openViewer(w *pixiv.Illustration, page int) {
	if w == nil {
		return
	}
	if n := pageCount(*w); page < 0 {
		page = 0
	} else if page >= n {
		page = n - 1
	}
	v := &viewerState{work: *w, page: page, zoom: 1}
	a.viewer = v
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

// viewerImage returns the current page's original and whether its load ended.
// It starts the load when the page changed, and preloads the pages beside it.
func (a *App) viewerImage(v *viewerState) (bmp *ui.Bitmap, loadDone bool) {
	url := v.work.PageURL(v.page)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loadKey != url {
		v.loadKey = url
		v.bmp, v.loadDone, v.fetched = nil, false, false
		v.bmp = a.images.Original(url, func() {
			v.mu.Lock()
			if v.loadKey == url {
				v.loadDone = true
			}
			v.mu.Unlock()
			a.update(func() {})
		})
		// The pages beside this one load ahead, so a page turn shows at once.
		for _, p := range []int{v.page - 1, v.page + 1} {
			if p >= 0 && p < pageCount(v.work) {
				a.images.Original(v.work.PageURL(p), nil)
			}
		}
	}
	if v.loadDone && !v.fetched {
		// The load ended: a cached bitmap is the success, and a nil one the
		// failure, so the viewer never asks imagex to load the page again.
		v.fetched = true
		v.bmp = a.images.Original(url, nil)
	}
	return v.bmp, v.loadDone
}

// viewerGo turns the page by delta, stopping at either end.
func (a *App) viewerGo(delta int) {
	v := a.viewer
	if v == nil {
		return
	}
	page := v.page + delta
	if page < 0 || page >= pageCount(v.work) {
		return
	}
	v.page = page
	v.resetView()
	v.mu.Lock()
	v.loadKey = "" // the next frame loads the new page
	v.mu.Unlock()
	a.update(func() {})
}

// resetView puts the image back at its size, centered.
func (v *viewerState) resetView() {
	v.zoom = 1
	v.panX, v.panY = 0, 0
}

// zoomBy multiplies the zoom, within the viewer's bounds.
func (v *viewerState) zoomBy(scale float32) {
	v.zoom = min(max(v.zoom*scale, viewerMinZoom), viewerMaxZoom)
}

// viewerInput takes the viewer's keys, its wheel, pinch and drag.
func (a *App) viewerInput(v *viewerState, ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputKeyDown:
		switch ev.Key {
		case ui.KeyEscape:
			a.closeViewer()
		case ui.KeyLeft:
			a.viewerGo(-1)
		case ui.KeyRight:
			a.viewerGo(1)
		case ui.KeyEqual: // the + key, shifted or not
			v.zoomBy(viewerZoomStep)
		case ui.KeyMinus:
			v.zoomBy(1 / viewerZoomStep)
		case ui.Key0:
			v.resetView()
		default:
			return false
		}
		return true
	case ui.InputZoom:
		if ev.Scale > 0 {
			v.zoomBy(ev.Scale)
		}
		return true
	case ui.InputScroll:
		// A wheel notch zooms; a touchpad's precise scroll is left alone.
		if ev.Precise || ev.DY == 0 {
			return false
		}
		if ev.DY < 0 {
			v.zoomBy(viewerZoomStep)
		} else {
			v.zoomBy(1 / viewerZoomStep)
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

// viewerPage draws the viewer over the content area: the page's original,
// and the toolbar under it.
func (a *App) viewerPage(c *ui.Context) {
	v := a.viewer
	if v == nil {
		return
	}
	bmp, loadDone := a.viewerImage(v)
	t := c.Theme()
	ui.Column(c).Fill().FillHeight().Background(t.Background).Children(func() {
		ui.Box(c).Grow(1).FillWidth().Clip().Background(t.Background).
			Focusable().AutoFocus().
			HandleInput(func(ev ui.InputEvent) bool { return a.viewerInput(v, ev) }).
			Children(func() {
				switch {
				case bmp != nil:
					ui.Image(c, bmp).Fill().Fit(ui.Contain).
						Scale(v.zoom, v.zoom).Translate(v.panX, v.panY)
				case loadDone:
					a.viewerFailure(c, v)
				default:
					ui.Column(c).Fill().Center().Children(func() {
						ui.Spinner(c)
					})
				}
			})
		a.viewerBar(c, v)
	})
}

// viewerFailure offers a retry once the original failed to load.
func (a *App) viewerFailure(c *ui.Context, v *viewerState) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "图片加载失败").TextColor(t.Danger)
		if ui.Button(c, "重试").Clicked() {
			v.mu.Lock()
			v.loadKey = "" // the next frame loads the page again
			v.mu.Unlock()
			a.update(func() {})
		}
	})
}

// viewerBar is the work's title, the page count and the viewer's buttons.
func (a *App) viewerBar(c *ui.Context, v *viewerState) {
	t := c.Theme()
	n := pageCount(v.work)
	ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).KeepFocus().Children(func() {
		ui.Text(c, v.work.Title).Grow(1).MaxLines(1)
		ui.Text(c, fmt.Sprintf("%d/%d", v.page+1, n)).TextColor(t.TextMuted)
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
