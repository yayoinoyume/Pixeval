package novel

import (
	"errors"
	"testing"
)

const body = `[chapter:第一章 出会い]
　静かな[[rb:図書館>としょかん]]で、少女は本を読んでいた。
一行目
二行目
[pixivimage:12345678-2]
[newpage]
続きの段落
[chapter:第二章 別れ]
同じ[pixivimage:12345678-2]画像と[pixivimage:87654321]。
[uploadedimage:555]
`

func TestParseChapters(t *testing.T) {
	doc, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Chapters) != 2 {
		t.Fatalf("chapters = %d, want 2: %+v", len(doc.Chapters), doc.Chapters)
	}
	if got := doc.Chapters[0].Title; got != "第一章 出会い" {
		t.Errorf("chapter 1 title = %q", got)
	}
	if got := doc.Chapters[1].Title; got != "第二章 別れ" {
		t.Errorf("chapter 2 title = %q", got)
	}
}

func TestParseParagraphs(t *testing.T) {
	doc, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{
		"静かな図書館で、少女は本を読んでいた。",
		"一行目",
		"二行目",
		"続きの段落",
	}
	got := doc.Chapters[0].Paragraphs
	if len(got) != len(want) {
		t.Fatalf("paragraphs = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Text != w {
			t.Errorf("paragraph %d = %q, want %q", i, got[i].Text, w)
		}
	}
	second := doc.Chapters[1].Paragraphs
	if len(second) != 1 || second[0].Text != "同じ画像と。" {
		t.Errorf("chapter 2 paragraphs = %+v", second)
	}
}

func TestParseRuby(t *testing.T) {
	doc, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ruby := doc.Chapters[0].Paragraphs[0].Ruby
	if len(ruby) != 1 || ruby[0] != (Ruby{Base: "図書館", Text: "としょかん"}) {
		t.Fatalf("ruby = %+v", ruby)
	}
	if got := doc.Chapters[0].Paragraphs[1].Ruby; len(got) != 0 {
		t.Errorf("prose paragraph has ruby %+v", got)
	}
}

func TestParseImages(t *testing.T) {
	doc, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Image{
		{IllustID: 12345678, Page: 2},
		{IllustID: 87654321, Page: 1},
		{UploadedID: 555},
	}
	if len(doc.Images) != len(want) {
		t.Fatalf("images = %+v, want %+v", doc.Images, want)
	}
	for i := range want {
		if doc.Images[i] != want[i] {
			t.Errorf("image %d = %+v, want %+v", i, doc.Images[i], want[i])
		}
	}
}

func TestParseNewPageSplitsParagraph(t *testing.T) {
	doc, err := Parse("前半[newpage]後半")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := doc.Chapters[0].Paragraphs
	if len(got) != 2 || got[0].Text != "前半" || got[1].Text != "後半" {
		t.Fatalf("paragraphs = %+v", got)
	}
}

func TestParseUntitledBody(t *testing.T) {
	doc, err := Parse("ただの文\n\n次の文")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Chapters) != 1 || doc.Chapters[0].Title != "" {
		t.Fatalf("chapters = %+v, want one untitled chapter", doc.Chapters)
	}
	if got := len(doc.Chapters[0].Paragraphs); got != 2 {
		t.Fatalf("paragraphs = %d, want 2", got)
	}
}

func TestParseKeepsBrokenMarkersAsText(t *testing.T) {
	bodies := map[string]string{
		"[[rb:base":          "[[rb:base",
		"[chapter:未終":        "[chapter:未終",
		"[pixivimage:abc]":   "[pixivimage:abc]",
		"[pixivimage:1-2-3]": "[pixivimage:1-2-3]",
		"[uploadedimage:0]":  "[uploadedimage:0]",
		"末尾の[":               "末尾の[",
	}
	for in, want := range bodies {
		doc, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if len(doc.Chapters) != 1 || len(doc.Chapters[0].Paragraphs) != 1 {
			t.Errorf("Parse(%q) = %+v", in, doc.Chapters)
			continue
		}
		if got := doc.Chapters[0].Paragraphs[0].Text; got != want {
			t.Errorf("Parse(%q) text = %q, want %q", in, got, want)
		}
	}
}

func TestParseEmptyBody(t *testing.T) {
	for _, in := range []string{"", "  \n\t\r\n"} {
		doc, err := Parse(in)
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("Parse(%q) error = %v, want ErrEmpty", in, err)
		}
		if doc == nil || len(doc.Chapters) != 0 || len(doc.Images) != 0 {
			t.Errorf("Parse(%q) document = %+v, want empty", in, doc)
		}
	}
}
