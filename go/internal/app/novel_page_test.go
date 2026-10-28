package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
)

// novelBody is a body with two paragraphs and one ruby reading.
const novelBody = `<h1>星之故事</h1><h2>第一章</h2>` +
	`<p>这是第一段。</p><p>这是<ruby>漢字<rt>かんじ</rt></ruby>的故事。</p>`

// newNovelApp is an app whose novel body comes from fetch, without a network.
func newNovelApp(fetch func(context.Context, int64) (string, error)) *App {
	a := New()
	a.novelContent = fetch
	a.images = imagex.New(nil, "")
	return a
}

// waitNovel waits for the novel page to settle as want says.
func waitNovel(t *testing.T, a *App, want func(v novelPageView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := a.novelView; v != nil && want(v.snapshot()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the novel page did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNovelPageShowsTitleAndParagraph checks opening a novel draws its title,
// chapter and body text.
func TestNovelPageShowsTitleAndParagraph(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	if a.route != routeNovel {
		t.Fatalf("route %q, want %q", a.route, routeNovel)
	}
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	for _, want := range []string{"星之故事", "第一章", "这是第一段。"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestNovelPageShowsRuby checks a ruby reading reads as Base(Reading).
func TestNovelPageShowsRuby(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	if want := "这是漢字(かんじ)的故事。"; !tt.HasText(want) {
		t.Errorf("%q missing: %q", want, tt.Texts())
	}
}

// TestNovelPageShowsRetry checks a failed load shows the error and a retry
// button.
func TestNovelPageShowsRetry(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) {
		return "", errors.New("网络失败")
	})
	a.openNovel(7, "坏掉的小说")
	waitNovel(t, a, func(v novelPageView) bool { return v.err != "" })

	tt := ui.NewTester(a.View, 1000, 700)
	for _, want := range []string{"网络失败", "重试"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}
