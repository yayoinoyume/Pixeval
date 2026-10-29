package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/novel"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// novelBody is a body with two paragraphs and one ruby reading.
const novelBody = "星之故事\n[chapter:第一章]\n这是第一段。\n这是[[rb:漢字>かんじ]]的故事。"

// longParagraph is long enough to wrap at the narrowest reading column.
var longParagraph = strings.Repeat("阅读栏宽度测试。", 30)

// longNovelBody carries a short paragraph to measure the text at and a long
// one whose lines the reading width sets.
var longNovelBody = "星之故事\n这是第一段。\n" + longParagraph

// newNovelApp is an app whose novel body comes from fetch, without a network.
func newNovelApp(fetch func(context.Context, int64) (string, error)) *App {
	a := New()
	a.novelContent = fetch
	a.images = imagex.New(nil, "")
	return a
}

// englishNovelApp is newNovelApp speaking English, whose bookmark labels do
// not collide with the navigation's own.
func englishNovelApp(fetch func(context.Context, int64) (string, error)) *App {
	a := newNovelApp(fetch)
	a.settings.Language = i18n.LangEnUS
	a.loadLanguage()
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

// TestNovelPageBookmark checks the bookmark button calls the API with the
// novel's id and flips the shown state only after success, both ways.
func TestNovelPageBookmark(t *testing.T) {
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7}}
	calls := make(chan string, 2)
	a.novelBookmarkFn = func(_ context.Context, id int64, on bool) error {
		calls <- fmt.Sprintf("%d:%v", id, on)
		return nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("Bookmark"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	select {
	case got := <-calls:
		if got != "7:true" {
			t.Errorf("bookmark call %q, want 7:true", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the bookmark button did not request a bookmark")
	}
	waitNovel(t, a, func(v novelPageView) bool { return v.bookmarked && !v.bookmarking })
	tt.Frame()
	if _, ok := tt.Find("Bookmarked"); !ok {
		t.Errorf("bookmarked state missing: %q", tt.Texts())
	}

	if err := tt.Click("Bookmarked"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	select {
	case got := <-calls:
		if got != "7:false" {
			t.Errorf("unbookmark call %q, want 7:false", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the bookmark button did not request an unbookmark")
	}
	waitNovel(t, a, func(v novelPageView) bool { return !v.bookmarked && !v.bookmarking })
	tt.Frame()
	if _, ok := tt.Find("Bookmark"); !ok {
		t.Errorf("unbookmarked state missing: %q", tt.Texts())
	}
}

// TestNovelPageBookmarkStartsFromList checks the button starts from the
// bookmark state the novel was listed with.
func TestNovelPageBookmarkStartsFromList(t *testing.T) {
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7}}
	a.searchNovels = &novelFeedState{items: []pixiv.Novel{{ID: 7, IsBookmarked: true}}}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	if _, ok := tt.Find("Bookmarked"); !ok {
		t.Errorf("listed bookmark state missing: %q", tt.Texts())
	}
}

// TestNovelPageBookmarkFailure checks a failed request keeps the shown state
// and leaves the reason as a notice.
func TestNovelPageBookmarkFailure(t *testing.T) {
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7}}
	release := make(chan struct{})
	a.novelBookmarkFn = func(context.Context, int64, bool) error {
		<-release
		return errors.New("网络错误")
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("Bookmark"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	waitNovel(t, a, func(v novelPageView) bool { return !v.bookmarking })
	if a.novelView.snapshot().bookmarked {
		t.Error("a failed bookmark left the novel bookmarked")
	}
	if want := a.tf("novel.bookmarkFailed", "网络错误"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	tt.Frame()
	if _, ok := tt.Find("Bookmark"); !ok {
		t.Errorf("bookmark button missing after failure: %q", tt.Texts())
	}
}

// TestNovelPageBookmarkHiddenWhenLoggedOut checks the button is not drawn
// without a signed-in account.
func TestNovelPageBookmarkHiddenWhenLoggedOut(t *testing.T) {
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	for _, label := range []string{"Bookmark", "Bookmarked"} {
		if _, ok := tt.Find(label); ok {
			t.Errorf("%q drawn while logged out: %q", label, tt.Texts())
		}
	}
}

// TestNovelPageUsesTypography checks the body draws at the saved text size,
// line spacing and reading width, and redraws when a value changes.
func TestNovelPageUsesTypography(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newNovelApp(func(context.Context, int64) (string, error) { return longNovelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1400, 900)
	base, ok := tt.Find("这是第一段。")
	if !ok {
		t.Fatalf("paragraph missing: %q", tt.Texts())
	}

	a.settings.NovelFontSize = novelFontSizeMax
	tt.Frame()
	big, ok := tt.Find("这是第一段。")
	if !ok {
		t.Fatalf("paragraph missing after the size change: %q", tt.Texts())
	}
	if big.H <= base.H {
		t.Errorf("text height %v at size %d, want more than %v at %d", big.H, novelFontSizeMax, base.H, novelFontSizeDefault)
	}

	a.settings.NovelFontSize = novelFontSizeDefault
	a.settings.NovelLineHeight = novelLineHeightMin
	tt.Frame()
	compact, ok := tt.Find("这是第一段。")
	if !ok {
		t.Fatalf("paragraph missing after the spacing change: %q", tt.Texts())
	}
	if compact.H >= base.H {
		t.Errorf("text height %v at line height %v, want less than %v at %v", compact.H, novelLineHeightMin, base.H, novelLineHeightDefault)
	}

	a.settings.NovelLineHeight = novelLineHeightDefault
	a.settings.NovelWidth = novelWidthMin
	tt.Frame()
	narrow, ok := tt.Find(longParagraph)
	if !ok {
		t.Fatalf("long paragraph missing: %q", tt.Texts())
	}
	a.settings.NovelWidth = novelWidthMax
	tt.Frame()
	wide, ok := tt.Find(longParagraph)
	if !ok {
		t.Fatalf("long paragraph missing after the width change: %q", tt.Texts())
	}
	if wide.W <= narrow.W {
		t.Errorf("line width %v at width %v, want more than %v at %v", wide.W, novelWidthMax, narrow.W, novelWidthMin)
	}
}

// TestNovelTypographyControls checks the buttons change the settings, stop at
// the range's ends, and write the result.
func TestNovelTypographyControls(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1400, 900)

	click := func(key string, up bool) {
		t.Helper()
		word := a.t("novel.decrease")
		if up {
			word = a.t("novel.increase")
		}
		if err := tt.Click(word + " " + a.t(key)); err != nil {
			t.Fatalf("Click: %v", err)
		}
	}
	click("novel.fontSize", false)
	click("novel.fontSize", false)
	click("novel.fontSize", false)
	// A fourth press is past the range's end and must change nothing.
	click("novel.fontSize", false)
	click("novel.lineHeight", true)
	click("novel.width", false)

	if a.settings.NovelFontSize != novelFontSizeMin {
		t.Errorf("font size %d, want %d", a.settings.NovelFontSize, novelFontSizeMin)
	}
	if want := novelLineHeightDefault + novelLineHeightStep; a.settings.NovelLineHeight != want {
		t.Errorf("line height %v, want %v", a.settings.NovelLineHeight, want)
	}
	if want := float64(novelWidthDefault - novelWidthStep); a.settings.NovelWidth != want {
		t.Errorf("width %v, want %v", a.settings.NovelWidth, want)
	}

	// The save waits for a pause after the last press; the clamped press above
	// changed nothing, so the settings saved are the ones the others left.
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil &&
			got.NovelFontSize == novelFontSizeMin &&
			got.NovelLineHeight == novelLineHeightDefault+novelLineHeightStep &&
			got.NovelWidth == float64(novelWidthDefault-novelWidthStep) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("saved settings %+v, load error %v; want the pressed values", got, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestNovelTypographySaveKeepsLaterSettings checks the paused save writes the
// settings as they are then: a preference changed after the typography press
// is kept, not overwritten by an older copy.
func TestNovelTypographySaveKeepsLaterSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1400, 900)

	if err := tt.Click(a.t("novel.increase") + " " + a.t("novel.fontSize")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	wantSize := novelFontSizeDefault + novelFontSizeStep
	waitSettings(t, path, func(s *Settings) bool { return s.NovelFontSize == wantSize })
	// The pause's save has gone through; let its result out of the one-slot
	// channel so the second save can report too.
	<-a.settingsSaved

	// Another page changes a preference before the reading page saves again.
	a.settings.Theme = themeDark
	a.saveNovelTypography()
	waitSettings(t, path, func(s *Settings) bool { return s.NovelFontSize == wantSize && s.Theme == themeDark })
}

// waitSettings polls the saved file until want holds.
func waitSettings(t *testing.T, path string, want func(*Settings) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil && want(got) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("saved settings %+v, load error %v; want does not hold", got, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestNovelImageMaxWidth checks a body image never draws wider than the
// reading column, whose narrowest setting is below the image's own limit.
func TestNovelImageMaxWidth(t *testing.T) {
	for _, tt := range []struct{ reading, want float32 }{
		{novelWidthMin, novelWidthMin},
		{novelWidthDefault, novelImageWidth},
		{novelWidthMax, novelImageWidth},
	} {
		if got := novelImageMaxWidth(tt.reading); got != tt.want {
			t.Errorf("max width %v at reading width %v, want %v", got, tt.reading, tt.want)
		}
	}
}

// TestNovelDownloadSavesText checks the download action writes the title, the
// author and the body as a text file in the download directory.
func TestNovelDownloadSavesText(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.downloadsDir = t.TempDir()
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	a.novelView.mu.Lock()
	a.novelView.author = "作者A"
	a.novelView.mu.Unlock()

	a.downloadNovel(a.novelView)
	data, err := os.ReadFile(filepath.Join(a.downloadsDir, "星之故事.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"星之故事", "作者A", "第一章", "这是第一段。", "这是漢字(かんじ)的故事。"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%q missing from saved text:\n%s", want, data)
		}
	}
	if want := a.t("download.novelSaved"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
}

// TestNovelDownloadWaitsForBody checks a click before the body lands saves it
// once it does.
func TestNovelDownloadWaitsForBody(t *testing.T) {
	release := make(chan struct{})
	a := newNovelApp(func(context.Context, int64) (string, error) {
		<-release
		return novelBody, nil
	})
	a.downloadsDir = t.TempDir()
	a.openNovel(7, "星之故事")
	a.downloadNovel(a.novelView)
	close(release)

	path := filepath.Join(a.downloadsDir, "星之故事.txt")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the pending download never wrote the novel")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNovelDownloadFailureNotice checks a save that cannot write says so.
func TestNovelDownloadFailureNotice(t *testing.T) {
	a := englishNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading })
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	a.downloadsDir = blocked
	a.downloadNovel(a.novelView)
	if !strings.HasPrefix(a.notice, "Could not save the novel:") {
		t.Errorf("notice %q, want a save failure", a.notice)
	}
}

// TestNovelFileNameIsSafe checks a saved novel's name drops what no file
// system takes, and falls back to the id without a usable title.
func TestNovelFileNameIsSafe(t *testing.T) {
	if got, want := novelFileName("a/b:c*?d", 7), "a_b_c__d.txt"; got != want {
		t.Errorf("name %q, want %q", got, want)
	}
	if got, want := novelFileName(" .. ", 7), "novel_7.txt"; got != want {
		t.Errorf("name %q, want %q", got, want)
	}
}

// novelCommentFixture is one novel comment by an author.
func novelCommentFixture(id int64, author, text string) pixiv.Comment {
	return pixiv.Comment{ID: id, Comment: text, User: pixiv.User{Name: author}}
}

// TestNovelPageShowsComments checks a signed-in reader sees the loaded
// comments below the body, with their author, date and text.
func TestNovelPageShowsComments(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(_ context.Context, id int64, offset int) (*pixiv.Page[pixiv.Comment], error) {
		if id != 7 || offset != 0 {
			t.Errorf("fetched novel %d offset %d, want 7:0", id, offset)
		}
		return &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{{
			ID:         1,
			Comment:    "第一条评论",
			User:       pixiv.User{Name: "作者甲"},
			CreateDate: time.Date(2024, 5, 1, 10, 0, 0, 0, time.Local),
		}}}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.loading && !v.commentsLoading && len(v.comments) == 1
	})

	tt := ui.NewTester(a.View, 1000, 700)
	for _, want := range []string{a.t("novel.comments"), "第一条评论", "作者甲", "2024-05-01 10:00"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestNovelPageCommentsEmpty checks a first page with no comments says so.
func TestNovelPageCommentsEmpty(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(context.Context, int64, int) (*pixiv.Page[pixiv.Comment], error) {
		return &pixiv.Page[pixiv.Comment]{}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.loading && !v.commentsLoading && v.commentsErr == ""
	})

	tt := ui.NewTester(a.View, 1000, 700)
	if want := a.t("novel.commentsEmpty"); !tt.HasText(want) {
		t.Errorf("%q missing: %q", want, tt.Texts())
	}
}

// TestNovelPageCommentsFailureRetry checks a failed comment load shows the
// reason and the retry button loads the first page again.
func TestNovelPageCommentsFailureRetry(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(context.Context, int64, int) (*pixiv.Page[pixiv.Comment], error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return nil, errors.New("网络失败")
		}
		return &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{
			novelCommentFixture(1, "作者甲", "重试后的评论"),
		}}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return v.commentsErr != "" })

	tt := ui.NewTester(a.View, 1000, 700)
	if want := a.tf("novel.commentsFailed", "网络失败"); !tt.HasText(want) {
		t.Errorf("%q missing: %q", want, tt.Texts())
	}
	if err := tt.Click(a.t("common.retry")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.commentsLoading && len(v.comments) == 1 && v.commentsErr == ""
	})

	tt.Frame()
	if !tt.HasText("重试后的评论") {
		t.Errorf("retried comment missing: %q", tt.Texts())
	}
}

// TestNovelPageCommentsLoadMore checks the next page asks for the offset after
// the comments already loaded and appends its items.
func TestNovelPageCommentsLoadMore(t *testing.T) {
	var mu sync.Mutex
	var offsets []int
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(_ context.Context, id int64, offset int) (*pixiv.Page[pixiv.Comment], error) {
		mu.Lock()
		offsets = append(offsets, offset)
		mu.Unlock()
		if id != 7 {
			t.Errorf("novel %d, want 7", id)
		}
		if offset == 0 {
			return &pixiv.Page[pixiv.Comment]{
				Items:   []pixiv.Comment{novelCommentFixture(1, "作者甲", "第一条评论")},
				NextURL: "next",
			}, nil
		}
		return &pixiv.Page[pixiv.Comment]{
			Items: []pixiv.Comment{novelCommentFixture(2, "作者乙", "第二条评论")},
		}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.loading && !v.commentsLoading && len(v.comments) == 1 && v.commentsNext != ""
	})

	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("novel.loadMoreComments")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitNovel(t, a, func(v novelPageView) bool {
		return !v.commentsLoading && len(v.comments) == 2 && v.commentsNext == ""
	})

	mu.Lock()
	got := append([]int(nil), offsets...)
	mu.Unlock()
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("fetched offsets %v, want [0 1]", got)
	}
	tt.Frame()
	if !tt.HasText("第二条评论") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
}

// TestNovelPageCommentsWaitForBody checks a body that fails loads no comments.
func TestNovelPageCommentsWaitForBody(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) {
		return "", errors.New("正文失败")
	})
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(context.Context, int64, int) (*pixiv.Page[pixiv.Comment], error) {
		t.Error("comments fetched before the body loaded")
		return &pixiv.Page[pixiv.Comment]{}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return v.err != "" })
}

// TestNovelPageCommentsClearOnSwitch checks opening another novel starts with
// an empty comment list instead of the previous novel's.
func TestNovelPageCommentsClearOnSwitch(t *testing.T) {
	a := newNovelApp(func(context.Context, int64) (string, error) { return novelBody, nil })
	a.session = &pixiv.Token{}
	a.fetchNovelCommentPage = func(_ context.Context, id int64, _ int) (*pixiv.Page[pixiv.Comment], error) {
		if id == 7 {
			return &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{
				novelCommentFixture(1, "作者甲", "旧小说的评论"),
			}}, nil
		}
		return &pixiv.Page[pixiv.Comment]{}, nil
	}
	a.openNovel(7, "星之故事")
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading && len(v.comments) == 1 })

	a.openNovel(8, "另一本")
	if got := a.novelView.snapshot().comments; len(got) != 0 {
		t.Errorf("comments after switching = %d, want none", len(got))
	}
	waitNovel(t, a, func(v novelPageView) bool { return !v.loading && !v.commentsLoading })
	if got := a.novelView.snapshot().comments; len(got) != 0 {
		t.Errorf("comments after the second load = %d, want none", len(got))
	}
}
