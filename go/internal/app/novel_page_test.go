package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/novel"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// novelBody is a body with two paragraphs and one ruby reading.
const novelBody = "星之故事\n[chapter:第一章]\n这是第一段。\n这是[[rb:漢字>かんじ]]的故事。"

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

// novelImageBody has one body image on page 1 of work 42.
const novelImageBody = "正文。\n[pixivimage:42-1]"

// novelIllustration is the work a body image marker points at, with its page 1
// original at bodyImageURL.
const bodyImageURL = "https://i.pximg.net/img-original/img/2024/01/01/00/00/00/42_p0.jpg"

func novelIllustration() *pixiv.Illustration {
	return &pixiv.Illustration{MetaPages: []pixiv.MetaPage{
		{ImageUrls: pixiv.ImageUrls{Original: bodyImageURL}},
	}}
}

// bodyImage is the reference novelImageBody produces.
var bodyImage = novel.Image{IllustID: 42, Page: 1}

// TestNovelPageLoadsBodyImage checks a body image marker looks up the work's
// page URL, and the page draws a block until the picture lands.
func TestNovelPageLoadsBodyImage(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelImageBody, nil })
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return novelIllustration(), nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.loading && v.imageURLs[bodyImage] == bodyImageURL
	})

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("正文。") {
		t.Errorf("body missing: %q", tt.Texts())
	}
}

// TestNovelPageFetchesBodyImageOnce checks a body image is looked up once, so
// a retry or a second frame does not ask the API again.
func TestNovelPageFetchesBodyImageOnce(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelImageBody, nil })
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return novelIllustration(), nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.loading && v.imageURLs[bodyImage] == bodyImageURL
	})
	ui.NewTester(a.View, 1000, 700)
	a.loadNovelImages(a.novelView, []novel.Image{bodyImage})

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("fetched %d times, want 1", calls)
	}
}

// TestNovelPageShowsUnsupportedUploadedImage checks an uploaded image, which
// the app cannot fetch yet, reads as a note instead of a picture.
func TestNovelPageShowsUnsupportedUploadedImage(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) {
		return "正文。\n[uploadedimage:555]", nil
	})
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		t.Error("an uploaded image must not be fetched as an illustration")
		return nil, errors.New("不应该拉取")
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	if want := "正文图片暂不支持"; !tt.HasText(want) {
		t.Errorf("%q missing: %q", want, tt.Texts())
	}
}
