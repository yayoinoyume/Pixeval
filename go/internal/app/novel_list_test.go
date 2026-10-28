package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeNovels makes n novels numbered from first, which openNovel accepts.
func fakeNovels(first, n int) []pixiv.Novel {
	novels := make([]pixiv.Novel, n)
	for i := range novels {
		novels[i] = pixiv.Novel{
			ID:        int64(first + i),
			Title:     fmt.Sprintf("小说 %d", first+i),
			User:      pixiv.User{Name: "作者"},
			PageCount: 3,
		}
	}
	return novels
}

// newNovelSearchTest is a signed-in app on the search page whose novel list
// comes from fetch, without images or a network.
func newNovelSearchTest(fetch func(context.Context) (*pixiv.Page[pixiv.Novel], error)) (*App, *ui.Tester) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	a.images = imagex.New(nil, "")
	a.novelContent = func(context.Context, int64) (string, error) { return novelBody, nil }
	a.route = routeSearch
	// The illustration tab is armed, so the page draws its tabs and not the
	// first-search hint; its own results come from a fake feed.
	a.search.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
	})
	a.search.arm()
	a.searchNovels.setFetch(fetch)
	a.searchNovels.arm()
	return a, ui.NewTester(a.View, 1000, 700)
}

// waitNovels waits for the novel list to settle as want says.
func waitNovels(t *testing.T, st *novelFeedState, want func(v novelFeedView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("novel list did not settle: items=%d loading=%v err=%q",
				len(v.items), v.loading, v.err)
		}
		time.Sleep(time.Millisecond)
	}
}

// novelTab shows the novel tab of a loaded search and waits for its list.
func novelTab(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click(a.t("search.novel")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.searchKind != searchNovel {
		t.Fatalf("searchKind %d, want %d", a.searchKind, searchNovel)
	}
	waitNovels(t, a.searchNovels, func(v novelFeedView) bool { return len(v.items) == 2 })
	tt.Frame()
}

// TestSearchNovelTabListsNovels checks the search page's novel tab draws a
// novel's title, author and page count.
func TestSearchNovelTabListsNovels(t *testing.T) {
	a, tt := newNovelSearchTest(func(context.Context) (*pixiv.Page[pixiv.Novel], error) {
		return &pixiv.Page[pixiv.Novel]{Items: fakeNovels(7, 2)}, nil
	})
	tt.Frame()
	novelTab(t, a, tt)
	for _, want := range []string{"小说 7", "作者", a.tf("novel.pages", 3)} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestNovelCardOpensReader checks clicking a novel's card opens the reader on
// that novel.
func TestNovelCardOpensReader(t *testing.T) {
	a, tt := newNovelSearchTest(func(context.Context) (*pixiv.Page[pixiv.Novel], error) {
		return &pixiv.Page[pixiv.Novel]{Items: fakeNovels(7, 2)}, nil
	})
	tt.Frame()
	novelTab(t, a, tt)

	card := a.tf("a11y.card", "小说 7", "作者")
	if err := tt.Click(card); err != nil {
		t.Fatalf("Click %q: %v", card, err)
	}
	if a.route != routeNovel {
		t.Fatalf("route %q after the click, want %q", a.route, routeNovel)
	}
	if a.novelView == nil || a.novelView.novelID != 7 || a.novelView.title != "小说 7" {
		t.Fatalf("novel view %+v, want novel 7", a.novelView)
	}
	if a.backRoute != routeSearch {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeSearch)
	}
}
