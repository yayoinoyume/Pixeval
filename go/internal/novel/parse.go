// Package novel turns a novel body into chapters of paragraphs, keeping ruby
// readings and embedded images apart from the plain text.
package novel

import (
	"strings"

	"golang.org/x/net/html"
)

// Ruby is one phonetic gloss: Base is the written form, Text the reading.
type Ruby struct {
	Base string
	Text string
}

// Paragraph is one block of a chapter. Text is the readable text with runs of
// whitespace collapsed and line breaks kept; Ruby is empty for plain prose.
type Paragraph struct {
	Text    string
	Ruby    []Ruby
	Heading bool
}

// Chapter is a titled group of paragraphs; an untitled novel has one chapter
// with an empty Title.
type Chapter struct {
	Title      string
	Paragraphs []Paragraph
}

// Document is a parsed novel body. Images holds absolute or page-relative URLs
// in document order, without duplicates.
type Document struct {
	Title    string
	Chapters []Chapter
	Images   []string
}

// Parse reads a novel body as HTML. Unknown or broken markup is tolerated
// rather than rejected, so Parse only fails when nothing can be read.
func Parse(body string) (*Document, error) {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	p := &parser{doc: &Document{}, images: map[string]bool{}}
	p.walk(root)
	return p.doc, nil
}

type parser struct {
	doc    *Document
	images map[string]bool
}

func (p *parser) walk(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			if text := clean(c.Data); text != "" {
				p.addText(text)
			}
			continue
		}
		if c.Type != html.ElementNode {
			continue
		}
		// A chapter marker carries the title itself; descending would read the
		// title twice. Nested paragraphs inside a titled element are lost.
		if hasClass(c, "_chapter") {
			p.addChapter(nodeText(c))
			continue
		}
		switch c.Data {
		case "script", "style", "head":
			continue
		case "img":
			p.addImage(c)
			continue
		case "h1":
			if p.doc.Title == "" {
				p.doc.Title = clean(nodeText(c))
			}
			continue
		case "h2", "h3":
			p.addChapter(nodeText(c))
			continue
		case "h4", "h5", "h6":
			p.addParagraph(c, true)
			continue
		case "p":
			p.addParagraph(c, false)
			continue
		}
		p.walk(c)
	}
}

func (p *parser) addChapter(title string) {
	if title = clean(title); title == "" {
		return
	}
	p.doc.Chapters = append(p.doc.Chapters, Chapter{Title: title})
}

// chapter returns the chapter new paragraphs belong to, starting an untitled
// one when the body opens with prose.
func (p *parser) chapter() *Chapter {
	if n := len(p.doc.Chapters); n > 0 {
		return &p.doc.Chapters[n-1]
	}
	p.doc.Chapters = append(p.doc.Chapters, Chapter{})
	return &p.doc.Chapters[len(p.doc.Chapters)-1]
}

// addText keeps prose that sits outside a <p> from being dropped.
func (p *parser) addText(text string) {
	ch := p.chapter()
	ch.Paragraphs = append(ch.Paragraphs, Paragraph{Text: text})
}

func (p *parser) addParagraph(n *html.Node, heading bool) {
	var in inline
	in.parser = p
	in.children(n)
	para := Paragraph{Text: clean(in.text.String()), Ruby: in.ruby, Heading: heading}
	if para.Text == "" && len(para.Ruby) == 0 {
		return
	}
	ch := p.chapter()
	ch.Paragraphs = append(ch.Paragraphs, para)
}

func (p *parser) addImage(n *html.Node) {
	src := attr(n, "src")
	if src == "" || p.images[src] {
		return
	}
	p.images[src] = true
	p.doc.Images = append(p.doc.Images, src)
}

// inline collects the text, ruby and images of one block.
type inline struct {
	parser *parser
	text   strings.Builder
	ruby   []Ruby
}

func (in *inline) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			in.text.WriteString(c.Data)
		case html.ElementNode:
			switch c.Data {
			case "script", "style":
			case "br":
				in.text.WriteString("\n")
			case "img":
				in.parser.addImage(c)
			case "ruby":
				base, reading := rubyParts(c)
				in.text.WriteString(base)
				if base != "" && reading != "" {
					in.ruby = append(in.ruby, Ruby{Base: base, Text: reading})
				}
			default:
				in.children(c)
			}
		}
	}
}

// rubyParts splits <ruby> into the written form and its reading. <rb> and bare
// text are the base, <rt> the reading, and <rp> fallback parentheses are
// dropped.
func rubyParts(n *html.Node) (base, reading string) {
	var b, r strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "rt" {
			r.WriteString(nodeText(c))
		} else if c.Type != html.ElementNode || c.Data != "rp" {
			b.WriteString(nodeText(c))
		}
	}
	return clean(b.String()), clean(r.String())
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// clean collapses whitespace per line and trims the block's edges, keeping the
// line breaks a <br> introduced.
func clean(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
