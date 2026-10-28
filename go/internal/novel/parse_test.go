package novel

import (
	"strings"
	"testing"
)

const twoChapters = `<!DOCTYPE html>
<html><head><title>ignored</title></head>
<body>
<h1>  夜の  図書館 </h1>
<div class="_chapter">第一章 出会い</div>
<p>　静かな<ruby><rb>図書館</rb><rt>としょかん</rt></ruby>で、少女は本を読んでいた。</p>
<p>一行目<br>二行目　　続き</p>
<p> &amp; 記号 &lt;タグ&gt; </p>
<p><img src="https://i.pximg.net/novel/1.png" alt=""></p>
<p></p>
<h2>第二章 別れ</h2>
<p>同じ<img src="https://i.pximg.net/novel/1.png">画像と<img src="https://i.pximg.net/novel/2.png">。</p>
<h4>小見出し</h4>
</body></html>`

func TestParseChapters(t *testing.T) {
	doc, err := Parse(twoChapters)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Title != "夜の 図書館" {
		t.Errorf("Title = %q", doc.Title)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("chapters = %d, want 2", len(doc.Chapters))
	}
	if got := doc.Chapters[0].Title; got != "第一章 出会い" {
		t.Errorf("chapter 1 title = %q", got)
	}
	if got := doc.Chapters[1].Title; got != "第二章 別れ" {
		t.Errorf("chapter 2 title = %q", got)
	}
}

func TestParseParagraphText(t *testing.T) {
	doc, err := Parse(twoChapters)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{
		"静かな図書館で、少女は本を読んでいた。",
		"一行目\n二行目 続き",
		"& 記号 <タグ>",
	}
	got := doc.Chapters[0].Paragraphs
	if len(got) != len(want) {
		t.Fatalf("paragraphs = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Text != w {
			t.Errorf("paragraph %d text = %q, want %q", i, got[i].Text, w)
		}
	}

	second := doc.Chapters[1].Paragraphs
	if len(second) != 2 {
		t.Fatalf("chapter 2 paragraphs = %d, want 2: %+v", len(second), second)
	}
	if !second[1].Heading || second[1].Text != "小見出し" {
		t.Errorf("heading paragraph = %+v", second[1])
	}
}

func TestParseRuby(t *testing.T) {
	doc, err := Parse(twoChapters)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ruby := doc.Chapters[0].Paragraphs[0].Ruby
	want := []Ruby{{Base: "図書館", Text: "としょかん"}}
	if len(ruby) != len(want) {
		t.Fatalf("ruby = %+v, want %+v", ruby, want)
	}
	for i := range want {
		if ruby[i] != want[i] {
			t.Errorf("ruby %d = %+v, want %+v", i, ruby[i], want[i])
		}
	}
	if got := doc.Chapters[0].Paragraphs[1].Ruby; len(got) != 0 {
		t.Errorf("prose paragraph has ruby %+v", got)
	}
}

func TestParseImagesDeduplicated(t *testing.T) {
	doc, err := Parse(twoChapters)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"https://i.pximg.net/novel/1.png", "https://i.pximg.net/novel/2.png"}
	if len(doc.Images) != len(want) {
		t.Fatalf("images = %v, want %v", doc.Images, want)
	}
	for i := range want {
		if doc.Images[i] != want[i] {
			t.Errorf("image %d = %q, want %q", i, doc.Images[i], want[i])
		}
	}
}

func TestParseUntitledBody(t *testing.T) {
	doc, err := Parse("<div>ただの文</div>\n<div>次の文</div>")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Chapters) != 1 || doc.Chapters[0].Title != "" {
		t.Fatalf("chapters = %+v, want one untitled chapter", doc.Chapters)
	}
	if got := len(doc.Chapters[0].Paragraphs); got != 2 {
		t.Fatalf("paragraphs = %d, want 2: %+v", got, doc.Chapters[0].Paragraphs)
	}
}

func TestParseBrokenMarkupDoesNotPanic(t *testing.T) {
	bodies := []string{
		"",
		"<p>a<ruby><rb>字</rb><rt>音</p ><div class=_chapter>章",
		"<p><ruby><rt>だけ</rt></ruby></p>",
		"<h2></h2><p>   </p>",
		"<img src=\"\"><img>",
	}
	for _, body := range bodies {
		doc, err := Parse(body)
		if err != nil {
			t.Errorf("Parse(%q): %v", body, err)
			continue
		}
		if doc == nil {
			t.Errorf("Parse(%q) returned nil document", body)
		}
	}
}

func TestParseRubyWithoutBase(t *testing.T) {
	doc, err := Parse("<p><ruby><rt>よみ</rt></ruby></p>")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Chapters) != 0 && len(doc.Chapters[0].Paragraphs) != 0 {
		t.Errorf("empty base produced %+v", doc.Chapters)
	}
	if strings.TrimSpace(doc.Title) != "" {
		t.Errorf("Title = %q", doc.Title)
	}
}
