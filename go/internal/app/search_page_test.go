package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestSearchPageFitsNarrowWindow checks the search page in a 1024 by 720
// window: every filter stays inside the width, the advanced filters sit under
// their heading, and the rows keep the room the density pass gave them.
func TestSearchPageFitsNarrowWindow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newSearchApp(pageBodies())
	a.searchKind = searchIllust
	a.trendingFn = nil
	a.searchTags = []pixiv.TrendingTag{{Tag: "猫"}, {Tag: "风景"}, {Tag: "オリジナル"}}
	const width, height = 1024, 720
	tt := ui.NewTester(a.View, width, height)
	tt.Frame()

	labels := []string{
		a.t("search.illust"), a.t("search.novel"), a.t("search.sauce"),
		a.t("search.sort"), a.t("search.target"), a.t("filter.title"),
		a.t("search.trending"), a.t("search.startDate"), a.t("search.endDate"),
		a.t("search.minWidth"), a.t("search.maxWidth"),
		a.t("search.minHeight"), a.t("search.maxHeight"),
	}
	boxes := make(map[string]ui.Rect, len(labels))
	for _, label := range labels {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
		boxes[label] = r
	}

	// The date and size fields belong to the advanced group, under its
	// heading, and the tabs, order row and filter group each keep a gap above
	// them rather than sitting flush.
	heading := boxes[a.t("filter.title")]
	if start := boxes[a.t("search.startDate")]; heading.Y >= start.Y {
		t.Errorf("the filter heading at %v is not above the start date at %v", heading, start)
	}
	if tabs, trend := boxes[a.t("search.illust")], boxes[a.t("search.trending")]; tabs.Y-(trend.Y+trend.H) < 12 {
		t.Errorf("the tabs at %v sit %v below the trending row at %v", tabs, tabs.Y-(trend.Y+trend.H), trend)
	}
	if sort, tabs := boxes[a.t("search.sort")], boxes[a.t("search.illust")]; sort.Y-(tabs.Y+tabs.H) < 8 {
		t.Errorf("the order row at %v sits %v below the tabs at %v", sort, sort.Y-(tabs.Y+tabs.H), tabs)
	}
	if heading.Y-(boxes[a.t("search.sort")].Y+boxes[a.t("search.sort")].H) < 8 {
		t.Errorf("the filter group at %v sits too close to the order row", heading)
	}

	// The result area may run past the window, but the fixed controls stay
	// usable at the top.
	if boxes[a.t("search.startDate")].Y+boxes[a.t("search.startDate")].H > height {
		t.Errorf("the start date at %v is below the %d high window", boxes[a.t("search.startDate")], height)
	}
}

// TestSearchBoxStartsSearch checks the box the density pass moved still takes
// a word and starts the search on Enter.
func TestSearchBoxStartsSearch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newSearchApp(pageBodies())
	a.search.query = ""
	a.trendingFn = nil
	a.searchTags = nil
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	box, ok := tt.Find(a.t("search.placeholder"))
	if !ok {
		t.Fatalf("search box missing: %q", tt.Texts())
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	tt.Frame()
	tt.Type("猫")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	waitBrowse(t, a.search, "start the search", func(v browseView) bool { return v.started })
	if a.search.query != "猫" {
		t.Errorf("query %q after Enter, want 猫", a.search.query)
	}
}
