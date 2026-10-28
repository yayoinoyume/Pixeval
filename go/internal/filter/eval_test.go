package filter

import (
	"testing"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

func mustParse(t *testing.T, expr string) Expr {
	t.Helper()
	parsed, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", expr, err)
	}
	return parsed
}

func checkMatch(t *testing.T, expr string, work *pixiv.Illustration, want bool) {
	t.Helper()
	if got := mustParse(t, expr).Match(work); got != want {
		t.Errorf("%s on work %d: got %v, want %v", expr, work.ID, got, want)
	}
}

func TestEvalNumeric(t *testing.T) {
	work := testWork()
	tests := []struct {
		expr string
		want bool
	}{
		{`id = 100`, true},
		{`id != 100`, false},
		{`id > 99`, true},
		{`id >= 100`, true},
		{`id < 100`, false},
		{`bookmarks = 10`, true},
		{`bookmarks >= 10`, true},
		{`bookmarks > 10`, false},
		{`views < 1000`, true},
		{`views <= 998`, false},
	}

	for _, tt := range tests {
		checkMatch(t, tt.expr, work, tt.want)
	}
}

func TestEvalText(t *testing.T) {
	work := testWork()
	tests := []struct {
		expr string
		want bool
	}{
		{`title ~ "猫"`, true},
		{`title !~ "猫"`, false},
		{`title ~ "犬"`, false},
		{`title = "猫の日"`, true},
		{`title != "猫の日"`, false},
		{`user = "画师A"`, true},
		{`user ~ "画师"`, true},
		{`user !~ "画师"`, false},
	}

	for _, tt := range tests {
		checkMatch(t, tt.expr, work, tt.want)
	}
}

func TestEvalTagMatchesOriginalAndTranslatedName(t *testing.T) {
	work := testWork()
	tests := []struct {
		expr string
		want bool
	}{
		{`tag = "原神"`, true},
		{`tag = "Genshin Impact"`, true},
		{`tag ~ "Genshin"`, true},
		{`tag ~ "原"`, true},
		{`tag = "missing"`, false},
		{`tag != "原神"`, false},
		{`tag != "missing"`, true},
		{`tag !~ "cat"`, false},
		{`tag !~ "missing"`, true},
	}

	for _, tt := range tests {
		checkMatch(t, tt.expr, work, tt.want)
	}

	work.Tags = nil
	checkMatch(t, `tag != "原神"`, work, true)
	checkMatch(t, `tag !~ "原神"`, work, true)
}

func TestEvalFlags(t *testing.T) {
	tests := []struct {
		expr string
		edit func(*pixiv.Illustration)
		want bool
	}{
		{`is_r18`, func(w *pixiv.Illustration) { w.XRestrict = 1 }, true},
		{`is_r18`, func(w *pixiv.Illustration) { w.XRestrict = 2 }, true},
		{`is_r18`, func(w *pixiv.Illustration) { w.XRestrict = 0 }, false},
		{`is_ai`, func(w *pixiv.Illustration) { w.IllustAIType = 1 }, true},
		{`is_ai`, func(w *pixiv.Illustration) { w.IllustAIType = 2 }, false},
		{`is_bookmarked`, func(w *pixiv.Illustration) { w.IsBookmarked = true }, true},
		{`is_bookmarked`, func(w *pixiv.Illustration) { w.IsBookmarked = false }, false},
		{`is_manga`, func(w *pixiv.Illustration) { w.Type = "manga" }, true},
		{`is_manga`, func(w *pixiv.Illustration) { w.Type = "illust" }, false},
	}

	for _, tt := range tests {
		work := testWork()
		tt.edit(work)
		if got := mustParse(t, tt.expr).Match(work); got != tt.want {
			t.Errorf("%s on %+v: got %v, want %v", tt.expr, work, got, tt.want)
		}
	}
}

func TestEvalLogic(t *testing.T) {
	work := testWork()
	tests := []struct {
		expr string
		want bool
	}{
		{`is_r18 AND (bookmarks >= 10 OR views > 10000)`, true},
		{`is_r18 AND (bookmarks > 10 OR views > 10000)`, false},
		{`is_manga OR title ~ "猫"`, true},
		{`NOT (is_r18 OR is_ai)`, false},
		{`NOT is_manga`, true},
		{`NOT NOT is_r18`, true},
	}

	for _, tt := range tests {
		checkMatch(t, tt.expr, work, tt.want)
	}
}
