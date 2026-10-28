package app

import (
	"context"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/novel"
)

// The reading column and the widest a body image may draw.
const (
	novelTextWidth  = 760
	novelImageWidth = 680
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
}

// novelPageView is what the novel page draws.
type novelPageView struct {
	doc     *novel.Document
	loading bool
	err     string
}

func (v *novelViewState) snapshot() novelPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	return novelPageView{doc: v.doc, loading: v.loading, err: v.err}
}

// openNovel shows the novel's reading page: the body loads off the main
// thread and fills the page when it lands.
func (a *App) openNovel(novelID int64, title string) {
	if novelID <= 0 {
		return
	}
	v := &novelViewState{novelID: novelID, title: title, loading: true}
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
	t := c.Theme()
	s := v.snapshot()
	title := v.title
	if title == "" && s.doc != nil {
		title = s.doc.Title
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(12).Padding(16).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, a.t("common.back")).Clicked() {
				a.goBack()
			}
			ui.Text(c, title).FontSize(22).Bold().Grow(1).MaxLines(1)
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
			ui.Column(c).Grow(1).Center().Padding(16).Children(func() {
				ui.Text(c, a.tf("novel.loadFailed", s.err)).TextColor(t.Danger)
			})
		default:
			a.novelBody(c, v, s.doc)
		}
	})
}

// novelBody draws the chapters and paragraphs, with the body images after
// them.
func (a *App) novelBody(c *ui.Context, v *novelViewState, doc *novel.Document) {
	if doc == nil {
		return
	}
	ui.Scroll(c).Grow(1).TrackScroll(&v.scroll).Children(func() {
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Column(c).MaxWidth(novelTextWidth).Grow(1).Padding(24).Gap(12).Children(func() {
				for _, ch := range doc.Chapters {
					if ch.Title != "" {
						ui.Text(c, ch.Title).FontSize(20).Bold()
					}
					for _, p := range ch.Paragraphs {
						novelParagraph(c, p)
					}
				}
				// TODO(M2): body images are novel.Image{IllustID, Page}; drawing
				// them needs the illustration detail for each ID first.
			})
		})
	})
}

// novelParagraph draws one paragraph, bold when it is a heading, with its ruby
// readings written after the written form.
func novelParagraph(c *ui.Context, p novel.Paragraph) {
	text := novelParagraphText(p)
	if text == "" {
		return
	}
	t := ui.Text(c, text)
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
