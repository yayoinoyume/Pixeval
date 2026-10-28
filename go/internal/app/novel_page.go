package app

import (
	"context"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/novel"
)

// The reading column, the widest a body image may draw and the height of the
// block that stands in for an image still loading.
const (
	novelTextWidth  = 760
	novelImageWidth = 680
	novelImageBlock = 120
)

// novelViewState is one open novel. The view runs on the main thread; the load
// goroutine writes the document fields under mu.
type novelViewState struct {
	novelID int64
	title   string
	scroll  ui.ScrollState

	mu      sync.Mutex
	doc     *novel.Document
	loading bool
	err     string
	// imageURLs are the body images whose original URL is known, and
	// imageLoading marks the illustration lookups in flight, so one image is
	// looked up once.
	imageURLs    map[novel.Image]string
	imageLoading map[novel.Image]bool
}

// novelPageView is what the novel page draws.
type novelPageView struct {
	doc       *novel.Document
	loading   bool
	err       string
	imageURLs map[novel.Image]string
}

func (v *novelViewState) snapshot() novelPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	urls := make(map[novel.Image]string, len(v.imageURLs))
	for img, url := range v.imageURLs {
		urls[img] = url
	}
	return novelPageView{doc: v.doc, loading: v.loading, err: v.err, imageURLs: urls}
}

// openNovel shows the novel's reading page: the body loads off the main
// thread and fills the page when it lands.
func (a *App) openNovel(novelID int64, title string) {
	if novelID <= 0 {
		return
	}
	v := &novelViewState{
		novelID:      novelID,
		title:        title,
		loading:      true,
		imageURLs:    map[novel.Image]string{},
		imageLoading: map[novel.Image]bool{},
	}
	a.backRoute, a.route, a.novelView = a.route, routeNovel, v
	go a.loadNovel(v)
}

// loadNovel fetches and parses the novel off the main thread, then shows it.
func (a *App) loadNovel(v *novelViewState) {
	ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	body, err := a.novelContent(ctx, v.novelID)
	var doc *novel.Document
	if err == nil {
		doc, err = novel.Parse(body)
	}
	a.update(func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.loading = false
		if err != nil {
			v.err = err.Error()
			return
		}
		v.doc = doc
		go a.loadNovelImages(v, doc.Images)
	})
}

// loadNovelImages looks up the original URL of every body image that is a
// Pixiv illustration, one goroutine each, off the main thread.
func (a *App) loadNovelImages(v *novelViewState, images []novel.Image) {
	for _, img := range images {
		if img.IllustID <= 0 || a.illustrationFn == nil {
			continue
		}
		v.mu.Lock()
		if _, have := v.imageURLs[img]; have || v.imageLoading[img] {
			v.mu.Unlock()
			continue
		}
		v.imageLoading[img] = true
		v.mu.Unlock()
		go a.loadNovelImage(v, img)
	}
}

// loadNovelImage fetches one image's illustration and keeps its page URL. The
// page is 1-based; PageURL takes a 0-based index.
func (a *App) loadNovelImage(v *novelViewState, img novel.Image) {
	ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	work, err := a.illustrationFn(ctx, img.IllustID)
	var url string
	if err == nil {
		url = work.PageURL(img.Page - 1)
	}
	a.update(func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		delete(v.imageLoading, img)
		if url != "" {
			v.imageURLs[img] = url
		}
	})
}

// retryNovel loads the body again after a failure.
func (a *App) retryNovel(v *novelViewState) {
	v.mu.Lock()
	if v.loading {
		v.mu.Unlock()
		return
	}
	v.loading, v.err = true, ""
	v.mu.Unlock()
	go a.loadNovel(v)
}

// novelPage draws the novel: the back button and the title above the body.
func (a *App) novelPage(c *ui.Context) {
	v := a.novelView
	if v == nil {
		return
	}
	t := themeOf(c)
	s := v.snapshot()
	title := v.title
	if title == "" && s.doc != nil {
		title = s.doc.Title
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceL, contentPadding, 0, contentPadding).
			AlignItems(ui.Center).Children(func() {
			if iconButton(c, iconLeft, a.t("common.back"), 32, t.TextMuted, t.SurfaceAlt).Clicked() {
				a.goBack()
			}
			ui.Text(c, title).FontSize(t.TextTitle).Bold().TextColor(t.Text).Grow(1).MaxLines(1)
			if s.err != "" {
				if ui.Button(c, a.t("common.retry")).Clicked() {
					a.retryNovel(v)
				}
			}
		})
		switch {
		case s.loading:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		case s.err != "":
			ui.Column(c).Grow(1).Center().Padding(t.SpaceL).Children(func() {
				ui.Text(c, a.tf("novel.loadFailed", s.err)).FontSize(t.TextSmall).TextColor(t.Danger)
			})
		default:
			a.novelBody(c, v, s)
		}
	})
}

// novelBody draws the chapters and paragraphs, with the body images after
// them.
func (a *App) novelBody(c *ui.Context, v *novelViewState, s novelPageView) {
	doc := s.doc
	if doc == nil {
		return
	}
	t := themeOf(c)
	ui.Scroll(c).Grow(1).TrackScroll(&v.scroll).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).MaxWidth(novelTextWidth).Grow(1).Padding(t.Space2XL).Gap(t.SpaceL).Children(func() {
				for _, ch := range doc.Chapters {
					if ch.Title != "" {
						ui.Text(c, ch.Title).FontSize(t.TextTitle).Bold().TextColor(t.Text)
					}
					for _, p := range ch.Paragraphs {
						novelParagraph(c, p)
					}
				}
				for _, img := range doc.Images {
					a.novelImage(c, img, s.imageURLs[img])
				}
			})
		})
	})
}

// novelImage draws one body image in place of its marker: the picture once it
// loads, a block while it loads, and a note for an uploaded image, which the
// app cannot fetch yet.
func (a *App) novelImage(c *ui.Context, img novel.Image, url string) {
	t := themeOf(c)
	if img.UploadedID > 0 {
		ui.Text(c, a.t("novel.imageUnsupported")).FontSize(t.TextSmall).TextColor(t.TextMuted)
		return
	}
	if bmp := a.novelImageBitmap(url); bmp != nil {
		ui.Image(c, bmp).MaxWidth(novelImageWidth).Radius(t.RadiusM).Label(a.t("a11y.novelImage"))
		return
	}
	ui.Box(c).FillWidth().MaxWidth(novelImageWidth).Height(novelImageBlock).
		Radius(t.RadiusM).Background(t.SurfaceAlt).Label(a.t("a11y.novelImage"))
}

// novelImageBitmap returns the image's bitmap, starting its load the first time
// and asking for a redraw when it lands.
func (a *App) novelImageBitmap(url string) *ui.Bitmap {
	if url == "" {
		return nil
	}
	return a.images.Original(url, func() { a.update(func() {}) })
}

// novelParagraph draws one paragraph, bold when it is a heading, with its ruby
// readings written after the written form.
func novelParagraph(c *ui.Context, p novel.Paragraph) {
	text := novelParagraphText(p)
	if text == "" {
		return
	}
	t := ui.Text(c, text).LineHeight(1.8)
	if p.Heading {
		t = t.Bold()
	}
}

// novelParagraphText writes each ruby base as Base(Reading) in place of the
// base alone. Bases appear in the text in the order the parser found them.
func novelParagraphText(p novel.Paragraph) string {
	if len(p.Ruby) == 0 {
		return p.Text
	}
	var b strings.Builder
	rest := p.Text
	for _, r := range p.Ruby {
		if r.Base == "" || r.Text == "" {
			continue
		}
		i := strings.Index(rest, r.Base)
		if i < 0 {
			continue
		}
		b.WriteString(rest[:i])
		b.WriteString(r.Base)
		b.WriteString("(")
		b.WriteString(r.Text)
		b.WriteString(")")
		rest = rest[i+len(r.Base):]
	}
	b.WriteString(rest)
	return b.String()
}
