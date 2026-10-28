package filter

import (
	"strings"
	"testing"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// testWork is a work that satisfies the default filter in each test unless the
// test overrides a field.
func testWork() *pixiv.Illustration {
	return &pixiv.Illustration{
		ID:             100,
		Title:          "猫の日",
		Type:           "illust",
		XRestrict:      1,
		Tags:           []pixiv.Tag{{Name: "cat", TranslatedName: "猫"}, {Name: "原神", TranslatedName: "Genshin Impact"}},
		User:           pixiv.User{Name: "画师A"},
		IsBookmarked:   true,
		TotalBookmarks: 10,
		TotalView:      999,
		IllustAIType:   1,
	}
}

func parseErrorAt(t *testing.T, err error) (int, string) {
	t.Helper()
	pe, ok := err.(*parseError)
	if !ok {
		t.Fatalf("error %v (%T) is not a parseError", err, err)
	}
	return pe.pos, pe.msg
}

func TestParseEmptyMatchesAll(t *testing.T) {
	for _, expr := range []string{"", "   ", "\t\n"} {
		parsed, err := Parse(expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", expr, err)
		}
		if !parsed.Match(testWork()) || !parsed.Match(&pixiv.Illustration{}) {
			t.Errorf("Parse(%q) should match every work", expr)
		}
	}
}

func TestParsePrecedence(t *testing.T) {
	plain := testWork()
	plain.XRestrict = 0
	plain.IllustAIType = 0
	plain.Type = "illust"

	// AND binds tighter than OR: is_r18 OR (is_ai AND is_manga).
	parsed, err := Parse(`is_r18 OR is_ai AND is_manga`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Match(plain) {
		t.Error("plain work should not match: left is false and the AND branch is false")
	}
	plain.IllustAIType = 1
	if parsed.Match(plain) {
		t.Error("ai illust should not match the AND branch")
	}
	plain.Type = "manga"
	if !parsed.Match(plain) {
		t.Error("ai manga should match the AND branch")
	}

	// Parentheses regroup to (is_r18 OR is_ai) AND is_manga.
	parsed, err = Parse(`(is_r18 OR is_ai) AND is_manga`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	plain.Type = "illust"
	if parsed.Match(plain) {
		t.Error("ai illust should not match (is_r18 OR is_ai) AND is_manga")
	}
	plain.Type = "manga"
	if !parsed.Match(plain) {
		t.Error("ai manga should match (is_r18 OR is_ai) AND is_manga")
	}
}

func TestParseNotPrecedence(t *testing.T) {
	work := testWork()
	work.XRestrict = 0
	work.IsBookmarked = true

	parsed, err := Parse(`NOT is_r18 AND is_bookmarked`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !parsed.Match(work) {
		t.Error("NOT should bind to is_r18 only")
	}

	parsed, err = Parse(`NOT id > 100`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !parsed.Match(work) {
		t.Error("NOT should negate the whole comparison, so id 100 passes")
	}
}

func TestParseCaseInsensitive(t *testing.T) {
	parsed, err := Parse(`ID = 100 aNd NOT IS_R18`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	work := testWork()
	work.XRestrict = 0
	if !parsed.Match(work) {
		t.Error("keywords and field names should be case-insensitive")
	}
}

func TestParseErrorsHavePosition(t *testing.T) {
	tests := []struct {
		expr string
		pos  int
		want string
	}{
		{`id >`, 5, "expected number, got end of input"},
		{`(id > 1`, 8, "expected ')', got end of input"},
		{`id > 1)`, 7, `unexpected ')'`},
		{`foo = 1`, 1, `unknown field "foo"`},
		{`id ~ "x"`, 4, `field "id" does not support '~'`},
		{`id = "x"`, 6, `expected number, got "x"`},
		{`title > 5`, 7, `field "title" does not support '>'`},
		{`is_manga = 1`, 10, `field "is_manga" is a flag and takes no operator`},
		{`title = cat`, 9, "expected string, got \"cat\""},
		{`id > 1 2`, 8, `unexpected "2"`},
		{`()`, 2, `expected a field, got ')'`},
	}

	for _, tt := range tests {
		_, err := Parse(tt.expr)
		if err == nil {
			t.Errorf("Parse(%q): got nil error", tt.expr)
			continue
		}
		pos, msg := parseErrorAt(t, err)
		if pos != tt.pos || msg != tt.want {
			t.Errorf("Parse(%q) = position %d %q, want position %d %q", tt.expr, pos, msg, tt.pos, tt.want)
		}
		if !strings.Contains(err.Error(), "position") {
			t.Errorf("Parse(%q): error %q should name the position", tt.expr, err)
		}
	}
}

func TestParseErrorPositionCountsRunes(t *testing.T) {
	// The unclosed quote sits after the two Chinese runes of "标题".
	_, err := Parse(`title = "猫`)
	if err == nil {
		t.Fatal("got nil error")
	}
	pos, _ := parseErrorAt(t, err)
	if pos != 9 {
		t.Fatalf("position = %d, want 9", pos)
	}
}
