package app

import (
	"context"
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

	// detail is the work's full record from the detail endpoint, which the
	// feed's copy lacks; detailErr and detailLoading follow the fetch. The
	// work keeps feeding the image and the pager, so its copy stays
	// untouched.
	detail        *pixiv.Illustration
	detailErr     string
	detailLoading bool
	// closed marks a viewer the app let go, so a fetch still in flight drops
	// its result instead of writing to a state nothing shows.
	closed bool
	// failedImages holds the thumbnail and avatar URLs that did not load, so
	// a broken one is not asked for again every frame.
	failedImages map[string]bool

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

// bookmarkState reports whether the work is bookmarked, and whether a request
// is still running.
func (v *viewerState) bookmarkState() (bookmarked, bookmarking bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.bookmarked, v.bookmarking
}

// detailState returns the detail the info card draws and where its fetch got.
func (v *viewerState) detailState() (detail *pixiv.Illustration, errText string, loading bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.detail, v.detailErr, v.detailLoading
}

// imageFailedURL reports whether a thumbnail or avatar URL failed already.
func (v *viewerState) imageFailedURL(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.failedImages[url]
}

// markImageFailed remembers a URL that did not load.
func (v *viewerState) markImageFailed(url string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.failedImages == nil {
		v.failedImages = map[string]bool{}
	}
	v.failedImages[url] = true
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
	if old := a.viewer; old != nil {
		old.mu.Lock()
		old.closed = true
		old.mu.Unlock()
	}
	a.viewer, a.comments, a.commentsOpen = v, &commentState{workID: w.ID}, true
	a.recordHistory(*w)
	// Start the original at once, so the first frame already has a load to
	// show rather than one to start.
	a.viewerImage(v)
	// The feed's copy of a work is thin: the detail fills the info card, and
	// the comments load with it. Both need a session, so a signed-out viewer
	// draws the feed's data and asks for neither.
	if a.session != nil {
		a.loadViewerDetail(v)
		a.loadComments(a.comments, false)
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

// closeViewer returns to the page under the viewer.
func (a *App) closeViewer() {
	if a.viewer == nil {
		return
	}
	v := a.viewer
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()
	a.viewer = nil
	// Drop the panel with the viewer: a page still in flight then writes to a
	// state nothing shows, instead of reviving it on the next work.
	a.comments, a.commentsOpen = nil, false
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
				a.notice = a.tf("viewer.bookmarkFailed", err.Error())
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

// viewerPage draws the detail view: a close row, the artwork beside the
// scrollable info card, and the action bar under them. The room stays dark in
// either theme, as a picture viewer should.
func (a *App) viewerPage(c *ui.Context) {
	v := a.viewer
	if v == nil {
		return
	}
	viewerTheme := designTheme(true, textScale(c))
	if c.Preferences().HighContrast {
		applyHighContrast(viewerTheme, true)
	}
	c.SetTheme(viewerTheme)
	bmp, failed := a.viewerImage(v)
	t := themeOf(c)
	ui.Column(c).Fill().FillHeight().Background(t.Bg).
		Padding(t.SpaceM, contentPadding, t.SpaceL, contentPadding).Gap(t.SpaceM).
		Children(func() {
			a.viewerTop(c)
			ui.Row(c).Grow(1).FillWidth().MinHeight(0).Gap(t.SpaceL).
				AlignItems(ui.Stretch).Children(func() {
				a.viewerArt(c, v, bmp, failed)
				a.viewerInfo(c, v)
			})
			a.viewerActions(c, v, failed)
		})
}

// viewerTop is the viewer's first row: the close pill on the left and the
// zoom hint on the right.
func (a *App) viewerTop(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Height(48).FillWidth().AlignItems(ui.Center).Children(func() {
		b := ui.ButtonBase(c).Height(36).Radius(t.RadiusFull).Padding(0, t.SpaceL).
			Gap(t.SpaceS).Background(t.SurfaceAlt).Label(a.t("viewer.close"))
		if b.Hovered() {
			b = b.Background(t.SurfaceHover)
		}
		if b.Clicked() {
			a.closeViewer()
		}
		b.Children(func() {
			ui.Icon(c, iconClose).Size(14, 14).TextColor(t.Text)
			ui.Text(c, a.t("viewer.close")).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text)
		})
		ui.Spacer(c)
		ui.Text(c, a.t("viewer.zoomHint")).FontSize(t.TextSmall).TextColor(t.TextMuted)
	})
}

// viewerArt is the artwork's frame: the original with the viewer's zoom and
// pan, and the page thumbnails in its bottom left corner.
func (a *App) viewerArt(c *ui.Context, v *viewerState, bmp *ui.Bitmap, failed bool) {
	t := themeOf(c)
	area := ui.Box(c).Grow(1).FillHeight().MinWidth(0).Clip().Radius(t.RadiusXL).
		Background(t.SurfaceAlt).Focusable().AutoFocus().
		HandleInput(func(ev ui.InputEvent) bool {
			// The pointer belongs to the artwork and the thumbnails over it;
			// the frame keeps the keys, the wheel and the pinch.
			switch ev.Kind {
			case ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp:
				return false
			}
			return a.viewerInput(v, ev)
		})
	if b := area.LayoutBounds(); b.W > 0 && b.H > 0 {
		v.viewW, v.viewH = b.W, b.H
	}
	area.Children(func() {
		switch {
		case bmp != nil:
			img := ui.Image(c, bmp).Fill().Fit(ui.Contain).
				Scale(v.zoom, v.zoom).Translate(v.panX, v.panY).
				Label(a.tf("viewer.image", v.work.Title, v.page+1, pageCount(v.work)))
			img.HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp:
					return a.viewerInput(v, ev)
				}
				return false
			})
			if img.DoubleClicked() {
				v.resetView()
			}
		case failed:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("viewer.imageFailed")).TextColor(t.Danger)
			})
		default:
			ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
		}
		a.viewerPageThumbs(c, v)
	})
}

// viewerPageThumbs draws up to three page thumbnails over the artwork's
// bottom left corner, the current page outlined with the accent.
func (a *App) viewerPageThumbs(c *ui.Context, v *viewerState) {
	n := pageCount(v.work)
	if n <= 1 {
		return
	}
	t := themeOf(c)
	first := max(min(v.page-1, n-3), 0)
	ui.Row(c).Absolute().Left(t.SpaceL).Bottom(t.SpaceL).Gap(t.SpaceS).
		Role(ui.RoleGroup).Label(a.t("viewer.pageThumbs")).Children(func() {
		for p := first; p < first+3 && p < n; p++ {
			a.viewerPageThumb(c, v, p)
		}
	})
}

// viewerPageThumb is one page's thumbnail: the page's own picture, or a block
// while it loads or after it failed. A thumbnail asks for the page's smaller
// rendition where the work has one.
func (a *App) viewerPageThumb(c *ui.Context, v *viewerState, page int) {
	t := themeOf(c)
	url := pageThumbURL(v.work, page)
	b := ui.ButtonBase(c).Size(44, 44).Radius(t.RadiusM).Padding(2).
		Background(t.Surface).Label(a.tf("viewer.pageThumb", page+1))
	if page == v.page {
		b = b.Border(2, t.Accent)
	}
	if b.Clicked() && page != v.page {
		a.viewerGo(page - v.page)
	}
	b.Children(func() {
		if url == "" || v.imageFailedURL(url) {
			return
		}
		if bmp := a.images.Thumb(url, 88, 88, a.viewerImageReady(v, url, 88)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(t.RadiusS)
		}
	})
}

// viewerImageReady returns the callback a thumbnail or avatar load calls: a
// probe that finds nothing means the load failed, and the image stays a block
// from then on rather than loading again every frame.
func (a *App) viewerImageReady(v *viewerState, url string, size int) func() {
	return func() {
		if a.images.Thumb(url, size, size, nil) == nil {
			v.markImageFailed(url)
		}
		a.update(func() {})
	}
}

// viewerActions is the bottom bar: the work's actions on the left and the
// pager on the right.
func (a *App) viewerActions(c *ui.Context, v *viewerState, failed bool) {
	t := themeOf(c)
	n := pageCount(v.work)
	bookmarked, bookmarking := v.bookmarkState()
	ui.Row(c).FillWidth().Height(72).Padding(t.SpaceS, t.SpaceM).Gap(t.SpaceS).
		AlignItems(ui.Center).Background(t.SurfaceAlt).Radius(t.RadiusXL).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			glyph, label := iconHeart, a.t("viewer.bookmark")
			if bookmarked {
				glyph, label = iconHeartFill, a.t("viewer.bookmarked")
			}
			b := a.viewerPill(c, glyph, label, t.AccentFill, t.AccentText)
			if b.Disabled(bookmarking).Clicked() {
				a.toggleBookmark(v)
			}
			if failed {
				if b := a.viewerPill(c, iconRefresh, a.t("common.retry"), t.Surface, t.Danger); b.Clicked() {
					a.viewerReload(v)
				}
			}
			if b := a.viewerPill(c, iconDownload, a.t("viewer.download"), t.Surface, t.Text); b.Clicked() {
				queued, ok := a.downloadWork(v.work)
				switch {
				case !ok:
					c.Toast(a.t("viewer.noDownloadDir"))
				case queued == 0:
					c.Toast(a.t("viewer.alreadyQueued"))
				default:
					c.Toast(a.tf("viewer.queued", queued))
				}
			}
			bg, fg := t.Surface, t.Text
			if a.commentsOpen {
				bg, fg = t.AccentSoft, t.Accent
			}
			if b := a.viewerPill(c, iconComment, a.t("comments.title"), bg, fg); b.Clicked() {
				a.toggleComments()
			}
			if b := a.viewerPill(c, nil, a.t("viewer.copyLink"), t.SurfaceAlt, t.TextMuted); b.Clicked() {
				a.copyWorkLink(c, v.work.ID)
			}
			if b := a.viewerPill(c, nil, a.t("viewer.openBrowser"), t.SurfaceAlt, t.TextMuted); b.Clicked() {
				a.openWorkLink(c, v.work.ID)
			}
		})
		ui.Row(c).Shrink(0).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			if iconButton(c, iconLeft, a.t("viewer.prev"), 36, t.Text, t.SurfaceHover).
				Disabled(v.page <= 0).Clicked() {
				a.viewerGo(-1)
			}
			ui.Text(c, a.viewerPageLabel(v.page+1, n)).FontSize(t.TextSmall).
				TextColor(t.TextMuted)
			if iconButton(c, iconRight, a.t("viewer.next"), 36, t.Text, t.SurfaceHover).
				Disabled(v.page >= n-1).Clicked() {
				a.viewerGo(1)
			}
		})
	})
}

// viewerPill is one rounded action in the viewer's bar: an icon, when there is
// one, and its label. A nil glyph draws the label alone.
func (a *App) viewerPill(c *ui.Context, glyph *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	t := themeOf(c)
	b := ui.ButtonBase(c).Height(40).Radius(t.RadiusFull).Padding(0, t.SpaceL).
		Gap(t.SpaceS).Background(bg).Label(label)
	if b.Hovered() {
		b = b.Background(bg.Mix(fg, 0.12))
	}
	b.Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).Size(16, 16).TextColor(fg)
		}
		ui.Text(c, label).FontSize(t.TextSmall).FontWeight(600).TextColor(fg)
	})
	return b
}
