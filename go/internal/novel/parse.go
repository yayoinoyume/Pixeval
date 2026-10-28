// Package novel turns a Pixiv novel body into chapters of paragraphs, keeping
// ruby readings and image references apart from the plain text.
//
// The body is not HTML: it is Pixiv's own token text, with lines as paragraphs
// and [chapter:] / [[rb:]] / [pixivimage:] / [uploadedimage:] / [newpage]
// markers inline.
package novel

import (
	"errors"
	"strconv"
	"strings"
)

// ErrEmpty reports a body with nothing to read.
var ErrEmpty = errors.New("novel: empty body")

// Ruby is one phonetic gloss: Base is the written form, Text the reading.
type Ruby struct {
	Base string
	Text string
}

// Paragraph is one block of a chapter. Text is the readable text with runs of
// whitespace collapsed; Ruby is empty for plain prose.
type Paragraph struct {
	Text    string
	Ruby    []Ruby
	Heading bool
}

// Chapter is a group of paragraphs opened by a [chapter:] marker; a novel
// without markers has one chapter with an empty Title.
type Chapter struct {
	Title      string
	Paragraphs []Paragraph
}

// Image is an embedded image reference, in document order and without
// duplicates. A [pixivimage:] sets IllustID and Page (1-based); an
// [uploadedimage:] sets UploadID. Building the CDN URL is left to the caller,
// which knows the image type and host.
type Image struct {
	IllustID   int64
	Page       int
	UploadedID int64
}

// Document is a parsed novel body. Title stays empty: the body carries neither
// the novel nor the chapter metadata, so a caller fills it from the novel detail.
type Document struct {
	Title    string
	Chapters []Chapter
	Images   []Image
}

// Parse reads a novel body. Broken markers are kept as literal text instead of
// failing the whole document; only an empty body reports an error.
func Parse(body string) (*Document, error) {
	if strings.TrimSpace(body) == "" {
		return &Document{}, ErrEmpty
	}
	p := &parser{doc: &Document{}, seen: map[Image]bool{}}
	for _, line := range strings.Split(body, "\n") {
		p.line(strings.TrimSuffix(line, "\r"))
		p.flush()
	}
	return p.doc, nil
}

type parser struct {
	doc  *Document
	seen map[Image]bool
	text strings.Builder
	ruby []Ruby
}

// line appends one line, applying the markers it contains.
func (p *parser) line(s string) {
	for {
		i := strings.IndexByte(s, '[')
		if i < 0 {
			p.text.WriteString(s)
			return
		}
		p.text.WriteString(s[:i])
		tok, n, ok := nextToken(s[i:])
		if !ok {
			// Not a marker: keep the bracket and rescan.
			p.text.WriteByte('[')
			s = s[i+1:]
			continue
		}
		p.apply(tok)
		s = s[i+n:]
	}
}

// flush ends the paragraph being collected, dropping it when it is blank.
func (p *parser) flush() {
	para := Paragraph{Text: clean(p.text.String()), Ruby: p.ruby}
	p.text.Reset()
	p.ruby = nil
	if para.Text == "" && len(para.Ruby) == 0 {
		return
	}
	ch := p.chapter()
	ch.Paragraphs = append(ch.Paragraphs, para)
}

// chapter returns the chapter new paragraphs belong to, opening an untitled one
// when the body starts without a marker.
func (p *parser) chapter() *Chapter {
	if len(p.doc.Chapters) == 0 {
		p.doc.Chapters = append(p.doc.Chapters, Chapter{})
	}
	return &p.doc.Chapters[len(p.doc.Chapters)-1]
}

func (p *parser) apply(t token) {
	switch t.kind {
	case kindChapter:
		p.flush()
		if t.value != "" {
			p.doc.Chapters = append(p.doc.Chapters, Chapter{Title: t.value})
		}
	case kindRuby:
		p.text.WriteString(t.base)
		if t.base != "" && t.value != "" {
			p.ruby = append(p.ruby, Ruby{Base: t.base, Text: t.value})
		}
	case kindIllust:
		p.image(Image{IllustID: t.id, Page: t.page})
	case kindUpload:
		p.image(Image{UploadedID: t.id})
	case kindNewPage:
		p.flush()
	}
}

func (p *parser) image(img Image) {
	if p.seen[img] {
		return
	}
	p.seen[img] = true
	p.doc.Images = append(p.doc.Images, img)
}

type tokenKind int

const (
	kindNone tokenKind = iota
	kindChapter
	kindRuby
	kindIllust
	kindUpload
	kindNewPage
)

type token struct {
	kind  tokenKind
	base  string
	value string
	id    int64
	page  int
}

const (
	tokenChapter   = "[chapter:"
	tokenRuby      = "[[rb:"
	tokenIllust    = "[pixivimage:"
	tokenUpload    = "[uploadedimage:"
	tokenNewPage   = "[newpage]"
	tokenRubyEnd   = "]]"
	tokenPlainEnd  = "]"
	tokenRubySplit = ">"
)

// nextToken recognises the marker at the start of s and reports its length.
func nextToken(s string) (token, int, bool) {
	if strings.HasPrefix(s, tokenNewPage) {
		return token{kind: kindNewPage}, len(tokenNewPage), true
	}
	if rest, ok := strings.CutPrefix(s, tokenRuby); ok {
		if body, n, ok := until(rest, tokenRubyEnd); ok {
			if base, reading, ok := strings.Cut(body, tokenRubySplit); ok {
				return token{kind: kindRuby, base: strings.TrimSpace(base), value: strings.TrimSpace(reading)},
					len(tokenRuby) + n, true
			}
		}
		return token{}, 0, false
	}
	if rest, ok := strings.CutPrefix(s, tokenChapter); ok {
		if title, n, ok := until(rest, tokenPlainEnd); ok {
			return token{kind: kindChapter, value: strings.TrimSpace(title)}, len(tokenChapter) + n, true
		}
		return token{}, 0, false
	}
	if rest, ok := strings.CutPrefix(s, tokenIllust); ok {
		body, n, ok := until(rest, tokenPlainEnd)
		if !ok {
			return token{}, 0, false
		}
		id, page, ok := illustRef(body)
		if !ok {
			return token{}, 0, false
		}
		return token{kind: kindIllust, id: id, page: page}, len(tokenIllust) + n, true
	}
	if rest, ok := strings.CutPrefix(s, tokenUpload); ok {
		body, n, ok := until(rest, tokenPlainEnd)
		if !ok {
			return token{}, 0, false
		}
		id, err := strconv.ParseInt(body, 10, 64)
		if err != nil || id <= 0 {
			return token{}, 0, false
		}
		return token{kind: kindUpload, id: id}, len(tokenUpload) + n, true
	}
	return token{}, 0, false
}

// until splits off the content before end and reports the consumed length,
// including end itself.
func until(s, end string) (string, int, bool) {
	i := strings.Index(s, end)
	if i < 0 {
		return "", 0, false
	}
	return s[:i], i + len(end), true
}

// illustRef reads the body of a [pixivimage:] marker: an ID, optionally
// followed by "-" and a 1-based page. Anything else, including stray spaces,
// is not a reference.
func illustRef(body string) (id int64, page int, ok bool) {
	ids, pages, hasPage := strings.Cut(body, "-")
	var err error
	if id, err = strconv.ParseInt(ids, 10, 64); err != nil || id <= 0 {
		return 0, 0, false
	}
	page = 1
	if hasPage {
		if page, err = strconv.Atoi(pages); err != nil || page <= 0 {
			return 0, 0, false
		}
	}
	return id, page, true
}

// clean collapses whitespace inside a line and trims the paragraph's edges.
func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
