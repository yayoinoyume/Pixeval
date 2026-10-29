package app

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/artcolor"
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
	// The page's colors are cached for the seed, appearance and text scale
	// they were made from. The view runs on the main thread, so the fields
	// need no lock.
	themeSeed   ui.Color
	themeDark   bool
	themeScale  float32
	themeHigh   bool
	themeTokens tokens
	themeSet    bool
	// viewW and viewH are the size of the image area in the last frame, which
	// the pan bounds and the zoom's anchor need.
	viewW, viewH float32

	mu      sync.Mutex
	loadKey string     // the page URL the load fields belong to
	bmp     *ui.Bitmap // the loaded original, nil until it is there
	failed  bool       // the load for loadKey failed

	// bookmarking is set while the bookmark request runs, so the button is
	// disabled until it lands; bookmarked is the work's state, which flips
	// only once the request succeeds. bookmarkGen counts the bookmarks that
	// succeeded, so a detail fetch that started before one cannot write back
	// the old state. The work itself is never written after openViewer, so
	// copying it cannot race with this.
	bookmarking bool
	bookmarked  bool
	bookmarkGen int

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
				v.bookmarkGen++
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

// viewerPage draws the work page: a room tinted by the artwork, the artwork
// itself in a quiet stage, and an editorial rail of facts and comments. The
// room follows the window's light or dark appearance; only its tint comes
// from the picture.
func (a *App) viewerPage(c *ui.Context) {
	v := a.viewer
	if v == nil {
		return
	}
	t := a.viewerTokens(c)
	// t is already at the desktop's text scale and, when asked for, at high
	// contrast; the stock widgets must not scale it a second time.
	c.SetTheme(themeFromTokens(t, 1))
	bmp, failed := a.viewerImage(v)
	ui.Column(c).Fill().FillHeight().
		LinearGradient(ui.LinearGradient{From: t.WashTop, To: t.WashBottom, Angle: 180, Oklab: true}).
		Children(func() {
			a.viewerTop(c, t)
			ui.Row(c).Grow(1).FillWidth().MinHeight(0).
				Padding(0, contentPadding, 0, contentPadding).
				Gap(t.Space2XL).AlignItems(ui.Stretch).Children(func() {
				a.viewerArt(c, t, v, bmp, failed)
				a.viewerInfo(c, t, v)
			})
			a.viewerActions(c, t, v, failed)
		})
}

// viewerTokens returns the base theme with the current page's palette. The
// cover's seed is available as soon as the feed has drawn it; the original's
// seed replaces it when the larger image lands. The result is cached for the
// seed, appearance, scale and contrast it was made from.
func (a *App) viewerTokens(c *ui.Context) tokens {
	v := a.viewer
	if v == nil {
		return themeOf(c)
	}
	dark := c.Theme().Dark
	scale := textScale(c)
	high := c.Preferences().HighContrast

	var seed ui.Color
	var have bool
	if s, ok := a.images.Seed(v.work.PageURL(v.page)); ok {
		seed, have = s, true
	}
	if !have {
		if s, ok := a.images.Seed(v.work.ThumbURL()); ok {
			seed, have = s, true
		}
	}
	if !have {
		if s, ok := a.images.Seed(v.work.CoverURL()); ok {
			seed, have = s, true
		}
	}
	if v.themeSet && v.themeSeed == seed && v.themeDark == dark && v.themeScale == scale && v.themeHigh == high {
		return v.themeTokens
	}

	t := tokensFor(dark)
	if have {
		t = t.withPalette(artcolor.Derive(seed, dark))
	}
	t = t.scaled(scale)
	if high {
		t = t.highContrast()
	}
	v.themeSeed, v.themeDark, v.themeScale, v.themeHigh = seed, dark, scale, high
	v.themeTokens, v.themeSet = t, true
	return t
}

// viewerTop is the page's first line: a quiet close control and the zoom
// hint. It has no bar of its own; the room's wash runs behind it.
func (a *App) viewerTop(c *ui.Context, t tokens) {
	ui.Row(c).Height(64).FillWidth().AlignItems(ui.Center).
		Padding(0, contentPadding).KeepFocus().Children(func() {
		b := ui.ButtonBase(c).Height(32).Radius(t.RadiusS).Padding(0, t.SpaceS).
			Gap(t.SpaceS).Label(a.t("viewer.close"))
		if b.Hovered() {
			b = b.Background(t.Surface.Alpha(0.55))
		}
		if b.Clicked() {
			a.closeViewer()
		}
		b.Children(func() {
			ui.Icon(c, iconClose).Size(14, 14).TextColor(t.Text)
			ui.Text(c, a.t("viewer.close")).FontSize(t.TextSmall).FontWeight(500).
				LetterSpacing(0.3).TextColor(t.Text)
		})
		ui.Spacer(c)
		ui.Text(c, a.t("viewer.zoomHint")).FontSize(t.TextTiny).TextColor(t.TextMuted)
	})
}

// viewerArt is the stage: the original with the viewer's zoom and pan, a
// hairline frame, and the page thumbnails underneath when there is more than
// one page. The stage stays a neutral paper or ink so the artwork's color is
// judged against a neutral, not against its own tint.
func (a *App) viewerArt(c *ui.Context, t tokens, v *viewerState, bmp *ui.Bitmap, failed bool) {
	n := pageCount(v.work)
	ui.Column(c).Grow(1).FillHeight().MinWidth(0).Gap(t.SpaceM).Children(func() {
		area := ui.Box(c).Grow(1).FillWidth().MinHeight(0).Clip().
			Focusable().AutoFocus().
			HandleInput(func(ev ui.InputEvent) bool {
				// The pointer belongs to the artwork; the frame keeps the
				// keys, the wheel and the pinch.
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
					ui.Text(c, a.t("viewer.imageFailed")).FontSize(t.TextSmall).TextColor(t.Danger)
				})
			default:
				ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
			}
		})
		if n > 1 {
			a.viewerPageThumbs(c, t, v)
		}
	})
}

// viewerPageThumbs is the strip under the artwork: up to five page pictures,
// the current page outlined by a single accent rule.
func (a *App) viewerPageThumbs(c *ui.Context, t tokens, v *viewerState) {
	n := pageCount(v.work)
	// The detail's page records carry the smaller renditions the feed's
	// copy lacks, so the thumbnails use them when it has landed.
	work := v.work
	if detail, _, _ := v.detailState(); detail != nil {
		work = *detail
	}
	first := max(min(v.page-2, n-5), 0)
	ui.Row(c).FillWidth().Height(56).AlignItems(ui.Center).Justify(ui.Center).
		Gap(t.SpaceS).Role(ui.RoleGroup).Label(a.t("viewer.pageThumbs")).Children(func() {
		for p := first; p < first+5 && p < n; p++ {
			a.viewerPageThumb(c, t, v, work, p)
		}
	})
}

// viewerPageThumb is one page's thumbnail: the page's own picture, or a
// quiet block while it loads or after it failed.
func (a *App) viewerPageThumb(c *ui.Context, t tokens, v *viewerState, work pixiv.Illustration, page int) {
	url := pageThumbURL(work, page)
	b := ui.ButtonBase(c).Size(48, 48).Radius(t.RadiusXS).Padding(2).
		Background(t.Surface).Border(1, t.Border).KeepFocus().
		Label(a.tf("viewer.pageThumb", page+1))
	if page == v.page {
		b = b.Border(2, t.Accent)
	}
	if b.Hovered() {
		b = b.Border(1, t.BorderStrong)
	}
	if b.Clicked() && page != v.page {
		a.viewerGo(page - v.page)
	}
	b.Children(func() {
		if url == "" || v.imageFailedURL(url) {
			return
		}
		if bmp := a.images.Thumb(url, 96, 96, a.viewerImageReady(v, url, 96)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(t.RadiusXS)
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

// viewerActions is the bottom line: the work's actions on the left and the
// pager on the right, separated from the room by one hairline. There is no
// floating bar and no pill; the primary action is the only filled control.
func (a *App) viewerActions(c *ui.Context, t tokens, v *viewerState, failed bool) {
	n := pageCount(v.work)
	bookmarked, bookmarking := v.bookmarkState()
	ui.Row(c).FillWidth().Height(64).Padding(0, contentPadding).Gap(t.SpaceXS).
		AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
		KeepFocus().Children(func() {
		glyph, label := iconHeart, a.t("viewer.bookmark")
		if bookmarked {
			glyph, label = iconHeartFill, a.t("viewer.bookmarked")
		}
		if b := a.viewerPrimary(c, t, glyph, label); b.Disabled(bookmarking).Clicked() {
			a.toggleBookmark(v)
		}
		if failed {
			if b := a.viewerAction(c, t, iconRefresh, a.t("common.retry"), false); b.Clicked() {
				a.viewerReload(v)
			}
		}
		if b := a.viewerAction(c, t, iconDownload, a.t("viewer.download"), false); b.Clicked() {
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
		if b := a.viewerAction(c, t, iconComment, a.t("comments.title"), a.commentsOpen); b.Clicked() {
			a.toggleComments()
		}
		if b := a.viewerAction(c, t, nil, a.t("viewer.copyLink"), false); b.Clicked() {
			a.copyWorkLink(c, v.work.ID)
		}
		if b := a.viewerAction(c, t, nil, a.t("viewer.openBrowser"), false); b.Clicked() {
			a.openWorkLink(c, v.work.ID)
		}
		ui.Spacer(c)
		if a.viewerIconButton(c, t, iconLeft, a.t("viewer.prev"), t.Text).Disabled(v.page <= 0).Clicked() {
			a.viewerGo(-1)
		}
		ui.Text(c, a.viewerPageLabel(v.page+1, n)).FontSize(t.TextTiny).
			FontFeatures("tnum").TextColor(t.TextMuted)
		if a.viewerIconButton(c, t, iconRight, a.t("viewer.next"), t.Text).Disabled(v.page >= n-1).Clicked() {
			a.viewerGo(1)
		}
	})
}

// viewerPrimary is the single filled action: bookmark. It is a rectangle
// with the control radius, not a pill.
func (a *App) viewerPrimary(c *ui.Context, t tokens, glyph *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c).Height(36).Radius(t.RadiusS).Padding(0, t.SpaceL).
		Gap(t.SpaceS).Background(t.AccentFill).Label(label)
	if b.Hovered() {
		b = b.Background(t.AccentHover)
	}
	b.Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).Size(16, 16).TextColor(t.AccentText)
		}
		ui.Text(c, label).FontSize(t.TextSmall).FontWeight(600).TextColor(t.AccentText)
	})
	return b
}

// viewerAction is a quiet text action: an icon when it has one, and its name.
// The active state uses the artwork's accent, not a filled pill.
func (a *App) viewerAction(c *ui.Context, t tokens, glyph *ui.SVG, label string, active bool) ui.Element {
	fg := t.TextMuted
	if active {
		fg = t.Accent
	}
	b := ui.ButtonBase(c).Height(36).Radius(t.RadiusS).Padding(0, t.SpaceM).
		Gap(t.SpaceS).Label(label)
	if b.Hovered() {
		b = b.Background(t.Surface.Alpha(0.55))
		fg = t.Text
	}
	b.Children(func() {
		if glyph != nil {
			ui.Icon(c, glyph).Size(16, 16).TextColor(fg)
		}
		ui.Text(c, label).FontSize(t.TextSmall).FontWeight(500).TextColor(fg)
	})
	return b
}

// viewerIconButton is a square pager control that follows the work's palette,
// unlike the shell's iconButton, which reads the window's base theme.
func (a *App) viewerIconButton(c *ui.Context, t tokens, glyph *ui.SVG, label string, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c).Size(36, 36).Radius(t.RadiusS).Label(label).Tooltip(label)
	if b.Hovered() {
		b = b.Background(t.Surface.Alpha(0.55))
	}
	b.Children(func() {
		ui.Icon(c, glyph).Size(16, 16).TextColor(fg)
	})
	return b
}
