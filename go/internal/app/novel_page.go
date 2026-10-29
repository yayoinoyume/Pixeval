package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/novel"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The widest a body image may draw and the height of the block that stands in
// for an image still loading.
const (
	novelImageWidth = 680
	novelImageBlock = 120
)

// novelViewState is one open novel. The view runs on the main thread; the load
// goroutine writes the document fields under mu.
type novelViewState struct {
	novelID int64
	title   string
	// author is the writer the novel was listed with, saved with the text.
	author string
	scroll ui.ScrollState

	mu      sync.Mutex
	doc     *novel.Document
	loading bool
	err     string
	// pendingSave is set when the download button was pressed before the body
	// landed, so the save happens once it does.
	pendingSave bool
	// bookmarked is the novel's bookmark state, taken from the list it was
	// opened from, and bookmarking marks a request in flight.
	bookmarked  bool
	bookmarking bool
	// imageURLs are the body images whose original URL is known, and
	// imageLoading marks the illustration lookups in flight, so one image is
	// looked up once.
	imageURLs    map[novel.Image]string
	imageLoading map[novel.Image]bool
	// saveTimer writes a typography change after a short pause, so holding a
	// button writes one file with the value it ends on.
	saveTimer *time.Timer
	// restore is the reading place to return to once the body is laid out.
	// lastFraction is where the page was last saved, so a frame that moved
	// nothing writes nothing, and restorePainted marks the first layout pass.
	restore        float64
	lastFraction   float64
	restorePainted bool
	// progressTimer writes the reading place after a pause in the scrolling.
	progressTimer *time.Timer
}

// novelPageView is what the novel page draws.
type novelPageView struct {
	doc         *novel.Document
	loading     bool
	err         string
	bookmarked  bool
	bookmarking bool
	imageURLs   map[novel.Image]string
}

func (v *novelViewState) snapshot() novelPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	urls := make(map[novel.Image]string, len(v.imageURLs))
	for img, url := range v.imageURLs {
		urls[img] = url
	}
	return novelPageView{
		doc:         v.doc,
		loading:     v.loading,
		err:         v.err,
		bookmarked:  v.bookmarked,
		bookmarking: v.bookmarking,
		imageURLs:   urls,
	}
}

// openNovel shows the novel's reading page: the body loads off the main
// thread and fills the page when it lands.
func (a *App) openNovel(novelID int64, title string) {
	if novelID <= 0 {
		return
	}
	// A novel opened over another keeps the first one's place at once, without
	// waiting for its pause to pass.
	a.flushNovelProgress(a.novelView)
	place := novelRestoreFraction(a.novelPlace(novelID))
	v := &novelViewState{
		novelID:      novelID,
		title:        title,
		author:       a.novelAuthor(novelID),
		loading:      true,
		bookmarked:   a.novelBookmarked(novelID),
		imageURLs:    map[novel.Image]string{},
		imageLoading: map[novel.Image]bool{},
		restore:      place,
		lastFraction: place,
	}
	a.backRoute, a.route, a.novelView = a.route, routeNovel, v
	go a.loadNovel(v)
}

// novelPlace is how far into the novel the reader was, or 0 when the place is
// not known.
func (a *App) novelPlace(novelID int64) float64 {
	if a.novelProgress == nil {
		return 0
	}
	return a.novelProgress.Get(novelID)
}

// novelListed returns the loaded list entry of a novel, if one holds it. The
// click passes only the id and title, so the loaded novel feeds are asked.
func (a *App) novelListed(novelID int64) (pixiv.Novel, bool) {
	feeds := []*novelFeedState{a.searchNovels}
	if a.userView != nil {
		feeds = append(feeds, a.userView.novelFeed)
	}
	for _, st := range feeds {
		if st == nil {
			continue
		}
		for _, n := range st.snapshot().items {
			if n.ID == novelID {
				return n, true
			}
		}
	}
	return pixiv.Novel{}, false
}

// novelBookmarked reports the bookmark state the novel was listed with.
func (a *App) novelBookmarked(novelID int64) bool {
	n, ok := a.novelListed(novelID)
	return ok && n.IsBookmarked
}

// novelAuthor is the writer the novel was listed with, empty when unknown.
func (a *App) novelAuthor(novelID int64) string {
	n, _ := a.novelListed(novelID)
	return n.User.Name
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
		v.loading = false
		if err != nil {
			v.err = err.Error()
			pending := v.pendingSave
			v.pendingSave = false
			v.mu.Unlock()
			if pending {
				a.notice = a.tf("download.novelFailed", err.Error())
			}
			return
		}
		v.doc = doc
		pending := v.pendingSave
		v.pendingSave = false
		go a.loadNovelImages(v, doc.Images)
		v.mu.Unlock()
		if pending {
			a.saveNovelText(v, doc)
		}
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

// toggleNovelBookmark adds or removes the novel's bookmark off the main
// thread. The button is disabled while the request runs, and the state flips
// only once it succeeds, so a failure leaves it unchanged.
func (a *App) toggleNovelBookmark(v *novelViewState) {
	v.mu.Lock()
	if v.bookmarking || a.novelBookmarkFn == nil {
		v.mu.Unlock()
		return
	}
	v.bookmarking = true
	on, id := !v.bookmarked, v.novelID
	v.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		err := a.novelBookmarkFn(ctx, id, on)
		a.update(func() {
			v.mu.Lock()
			defer v.mu.Unlock()
			// The notice comes before the state change: a test that waits on
			// the state then also sees the message.
			if err != nil {
				a.notice = a.tf("novel.bookmarkFailed", err.Error())
			} else {
				v.bookmarked = on
			}
			v.bookmarking = false
		})
	}()
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

// downloadNovel saves the loaded body as text, loading it first when the
// click came before it landed.
func (a *App) downloadNovel(v *novelViewState) {
	v.mu.Lock()
	doc := v.doc
	if doc == nil {
		start := !v.loading
		v.pendingSave = true
		if start {
			v.loading, v.err = true, ""
		}
		v.mu.Unlock()
		if start {
			go a.loadNovel(v)
		}
		return
	}
	v.mu.Unlock()
	a.saveNovelText(v, doc)
	a.update(func() {})
}

// saveNovelText writes the novel as a text file in the download directory and
// leaves the outcome as a notice.
func (a *App) saveNovelText(v *novelViewState, doc *novel.Document) {
	if a.downloadsDir == "" {
		a.notice = a.t("viewer.noDownloadDir")
		return
	}
	v.mu.Lock()
	title, author, novelID := v.title, v.author, v.novelID
	v.mu.Unlock()
	if title == "" {
		title = doc.Title
	}
	path := filepath.Join(a.downloadsDir, novelFileName(title, novelID))
	if err := os.WriteFile(path, []byte(novelText(title, author, doc)), 0o644); err != nil {
		a.notice = a.tf("download.novelFailed", err.Error())
		return
	}
	a.notice = a.t("download.novelSaved")
}

// novelFileName is the safe name a saved novel gets: its title, or its id
// when the title has nothing usable.
func novelFileName(title string, id int64) string {
	name := downloadSafeSegment(downloadContextName(title))
	if name == "" {
		name = fmt.Sprintf("novel_%d", id)
	}
	return name + ".txt"
}

// novelText is the saved file's text: the title, the author when known, and the
// chapters, with ruby readings written after their base.
func novelText(title, author string, doc *novel.Document) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title)
		b.WriteString("\n")
	}
	if author != "" {
		b.WriteString(author)
		b.WriteString("\n")
	}
	for _, ch := range doc.Chapters {
		b.WriteString("\n")
		if ch.Title != "" {
			b.WriteString(ch.Title)
			b.WriteString("\n\n")
		}
		for _, p := range ch.Paragraphs {
			b.WriteString(novelParagraphText(p))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// novelPage draws the novel: the back button and the title above the body.
func (a *App) novelPage(c *ui.Context) {
	v := a.novelView
	if v == nil {
		return
	}
	t := themeOf(c)
	s := v.snapshot()
	a.trackNovelProgress(v)
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
			if a.session != nil {
				label := a.t("novel.bookmark")
				if s.bookmarked {
					label = a.t("novel.bookmarked")
				}
				if b := ui.Button(c, label); b.Disabled(s.bookmarking).Clicked() {
					a.toggleNovelBookmark(v)
				}
			}
			if ui.Button(c, a.t("viewer.download")).Clicked() {
				a.downloadNovel(v)
			}
			if s.err != "" {
				if ui.Button(c, a.t("common.retry")).Clicked() {
					a.retryNovel(v)
				}
			}
		})
		a.novelTypographyRow(c, v)
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

// novelTypographyRow draws the reading page's typography controls: the text
// size, the line spacing and the reading column's width, each changed by a
// pair of text buttons and saved after a short pause.
func (a *App) novelTypographyRow(c *ui.Context, v *novelViewState) {
	st := a.novelStyle()
	t := themeOf(c)
	ui.Row(c).Gap(t.SpaceL).Padding(0, contentPadding, t.SpaceM, contentPadding).
		AlignItems(ui.Center).Children(func() {
		a.novelTypographyControl(c, a.t("novel.fontSize"), fmt.Sprintf("%g", st.fontSize), func(dir int) {
			a.adjustNovelFontSize(v, dir)
		})
		a.novelTypographyControl(c, a.t("novel.lineHeight"), fmt.Sprintf("%g", st.lineHeight), func(dir int) {
			a.adjustNovelLineHeight(v, dir)
		})
		a.novelTypographyControl(c, a.t("novel.width"), fmt.Sprintf("%g", st.width), func(dir int) {
			a.adjustNovelWidth(v, dir)
		})
	})
}

// novelTypographyControl draws one typography setting: its name, the value it
// has and the buttons moving it down and up.
func (a *App) novelTypographyControl(c *ui.Context, label, value string, adjust func(dir int)) {
	t := themeOf(c)
	ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(t.TextSmall).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(t.TextSmall).TextColor(t.Text)
		if ui.Button(c, "−").Label(a.t("novel.decrease") + " " + label).Clicked() {
			adjust(-1)
		}
		if ui.Button(c, "＋").Label(a.t("novel.increase") + " " + label).Clicked() {
			adjust(1)
		}
	})
}

// novelStyle is the typography the reading page draws its body with, from the
// settings.
type novelStyle struct {
	fontSize   float32
	lineHeight float32
	width      float32
}

func (a *App) novelStyle() novelStyle {
	st := novelStyle{novelFontSizeDefault, novelLineHeightDefault, novelWidthDefault}
	if a.settings != nil {
		st.fontSize = float32(a.settings.NovelFontSize)
		st.lineHeight = float32(a.settings.NovelLineHeight)
		st.width = float32(a.settings.NovelWidth)
	}
	return st
}

// adjustNovelFontSize moves the body text size by dir, stopping at the range's
// ends, and saves it.
func (a *App) adjustNovelFontSize(v *novelViewState, dir int) {
	if a.settings == nil {
		return
	}
	size := min(max(a.settings.NovelFontSize+dir*novelFontSizeStep, novelFontSizeMin), novelFontSizeMax)
	if size == a.settings.NovelFontSize {
		return
	}
	a.settings.NovelFontSize = size
	a.scheduleNovelSave(v)
}

// adjustNovelLineHeight moves the line spacing by dir, one step at a time and
// stopping at the range's ends, and saves it.
func (a *App) adjustNovelLineHeight(v *novelViewState, dir int) {
	if a.settings == nil {
		return
	}
	height := a.settings.NovelLineHeight + float64(dir)*novelLineHeightStep
	// A step is a tenth, so round it back to one to keep 1.7000000000000002
	// out of the settings file.
	height = math.Round(height*10) / 10
	height = min(max(height, novelLineHeightMin), novelLineHeightMax)
	if height == a.settings.NovelLineHeight {
		return
	}
	a.settings.NovelLineHeight = height
	a.scheduleNovelSave(v)
}

// adjustNovelWidth moves the reading column's width by dir, stopping at the
// range's ends, and saves it.
func (a *App) adjustNovelWidth(v *novelViewState, dir int) {
	if a.settings == nil {
		return
	}
	width := a.settings.NovelWidth + float64(dir)*novelWidthStep
	width = min(max(width, novelWidthMin), novelWidthMax)
	if width == a.settings.NovelWidth {
		return
	}
	a.settings.NovelWidth = width
	a.scheduleNovelSave(v)
}

// novelSaveDelay is how long after the last typography press the settings are
// written, so holding a button writes one file with the value it ends on.
const novelSaveDelay = 200 * time.Millisecond

// scheduleNovelSave writes the settings after a short pause, on the main
// thread. It saves them as they are then, not as they were when a control
// moved, so a preference changed in the pause is not overwritten.
func (a *App) scheduleNovelSave(v *novelViewState) {
	if v.saveTimer != nil {
		v.saveTimer.Stop()
	}
	v.saveTimer = time.AfterFunc(novelSaveDelay, func() {
		a.update(func() { a.saveNovelTypography() })
	})
}

// saveNovelTypography writes the settings a typography control changed, the
// way the settings page does: on the main thread, off the file.
func (a *App) saveNovelTypography() {
	a.saveSettings()
}

// novelProgressDelay is how long after the last scroll the reading place is
// written, so a scrolling gesture writes one file, not one per frame.
var novelProgressDelay = 500 * time.Millisecond

// novelProgressEpsilon is how much the place must move before it counts as a
// new one; smaller moves are jitter from a layout change.
const novelProgressEpsilon = 0.002

// trackNovelProgress remembers where the reader scrolled to, and writes it
// after a pause.
func (a *App) trackNovelProgress(v *novelViewState) {
	if a.novelProgress == nil || v.restore > 0 || v.scroll.MaxY <= 0 {
		return
	}
	fraction := float64(v.scroll.Y / v.scroll.MaxY)
	if math.Abs(fraction-v.lastFraction) < novelProgressEpsilon {
		return
	}
	v.lastFraction = fraction
	a.novelProgress.Set(v.novelID, fraction)
	a.scheduleNovelProgressSave(v)
}

// scheduleNovelProgressSave writes the place after a pause, on the main
// thread. The place itself is already remembered, so a page left before the
// pause ends still keeps it.
func (a *App) scheduleNovelProgressSave(v *novelViewState) {
	if v.progressTimer != nil {
		v.progressTimer.Stop()
	}
	v.progressTimer = time.AfterFunc(novelProgressDelay, func() {
		a.update(func() { a.saveNovelProgress() })
	})
}

// flushNovelProgress saves the page's place at once, for a novel opened over
// it.
func (a *App) flushNovelProgress(v *novelViewState) {
	if v == nil || v.progressTimer == nil {
		return
	}
	v.progressTimer.Stop()
	v.progressTimer = nil
	a.saveNovelProgress()
}

// saveNovelProgress writes the places off the main thread, so the view never
// blocks on the file.
func (a *App) saveNovelProgress() {
	if a.novelProgress == nil {
		return
	}
	path, err := novelProgressPath()
	if err != nil {
		log.Printf("novel progress: %v", err)
		return
	}
	p := a.novelProgress
	go func() {
		if err := p.Save(path); err != nil {
			log.Printf("novel progress: %v", err)
		}
	}()
}

// restoreNovelScroll scrolls to the saved place once the body has a range.
// The first frame lays the body out and asks for another, so the place is
// applied with the real height; a body shorter than the page has no range and
// keeps no place.
func (a *App) restoreNovelScroll(c *ui.Context, v *novelViewState) {
	if v.restore <= 0 {
		return
	}
	if v.scroll.MaxY <= 0 {
		if v.restorePainted {
			v.restore, v.lastFraction = 0, 0
			return
		}
		v.restorePainted = true
		c.Invalidate()
		return
	}
	v.scroll.Y = float32(v.restore) * v.scroll.MaxY
	v.lastFraction = v.restore
	v.restore = 0
}

// novelBody draws the chapters and paragraphs, with the body images after
// them.
func (a *App) novelBody(c *ui.Context, v *novelViewState, s novelPageView) {
	doc := s.doc
	if doc == nil {
		return
	}
	t := themeOf(c)
	st := a.novelStyle()
	a.restoreNovelScroll(c, v)
	ui.Scroll(c).Grow(1).TrackScroll(&v.scroll).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).MaxWidth(st.width).Grow(1).Padding(t.Space2XL).Gap(t.SpaceL).Children(func() {
				for _, ch := range doc.Chapters {
					if ch.Title != "" {
						ui.Text(c, ch.Title).FontSize(t.TextTitle).Bold().TextColor(t.Text)
					}
					for _, p := range ch.Paragraphs {
						novelParagraph(c, p, st)
					}
				}
				for _, img := range doc.Images {
					a.novelImage(c, img, s.imageURLs[img], st)
				}
			})
		})
	})
}

// novelImage draws one body image in place of its marker: the picture once it
// loads, a block while it loads, and a note for an uploaded image, which the
// app cannot fetch yet. An image never draws wider than the reading column.
func (a *App) novelImage(c *ui.Context, img novel.Image, url string, st novelStyle) {
	t := themeOf(c)
	maxWidth := novelImageMaxWidth(st.width)
	if img.UploadedID > 0 {
		ui.Text(c, a.t("novel.imageUnsupported")).FontSize(t.TextSmall).TextColor(t.TextMuted)
		return
	}
	if bmp := a.novelImageBitmap(url); bmp != nil {
		ui.Image(c, bmp).MaxWidth(maxWidth).Radius(t.RadiusXS).Label(a.t("a11y.novelImage"))
		return
	}
	ui.Box(c).FillWidth().MaxWidth(maxWidth).Height(novelImageBlock).
		Radius(t.RadiusXS).Background(t.SurfaceAlt).Label(a.t("a11y.novelImage"))
}

// novelImageMaxWidth is the widest a body image may draw: the image's own
// limit, or the reading column when it is narrower.
func novelImageMaxWidth(readingWidth float32) float32 {
	return min(float32(novelImageWidth), readingWidth)
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
func novelParagraph(c *ui.Context, p novel.Paragraph, st novelStyle) {
	text := novelParagraphText(p)
	if text == "" {
		return
	}
	t := ui.Text(c, text).FontSize(st.fontSize).LineHeight(st.lineHeight)
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
