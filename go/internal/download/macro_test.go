package download

import (
	"strings"
	"testing"
	"time"
)

var macroTestTime = time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)

func render(t *testing.T, template string, ctx Context) string {
	t.Helper()
	tmpl, err := Parse(template)
	if err != nil {
		t.Fatalf("Parse(%q): %v", template, err)
	}

	out, err := tmpl.Render(ctx)
	if err != nil {
		t.Fatalf("Render(%q): %v", template, err)
	}

	return out
}

func TestDefaultMacro(t *testing.T) {
	base := Context{
		ID:          42,
		Title:       "标题B",
		ArtistName:  "画家A",
		PublishTime: macroTestTime,
		Ext:         "jpg",
		IsPicOne:    true,
	}

	tests := []struct {
		name string
		ctx  Context
		want string
	}{
		{
			name: "single image",
			ctx:  base,
			want: `//[画家A] 42.jpg`,
		},
		{
			name: "picture set",
			ctx: func() Context {
				c := base
				c.Ext = "png"
				c.IsPicOne = false
				c.IsPicSet = true
				c.PicSetIndex = 1
				return c
			}(),
			want: `/[画家A] 标题B/[画家A] 42p1.png`,
		},
		{
			name: "series",
			ctx: func() Context {
				c := base
				c.ID = 7
				c.Ext = "gif"
				c.IsPicOne = false
				c.IsSeries = true
				c.SeriesID = 9
				c.SeriesTitle = "系列"
				return c
			}(),
			want: `//[画家A] 7.gif`,
		},
		{
			name: "group subscription",
			ctx: func() Context {
				c := base
				c.IsGroup = true
				c.GroupID = 5
				return c
			}(),
			want: `5//[画家A] 42.jpg`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := render(t, DefaultMacro, tc.ctx); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNestedConditions(t *testing.T) {
	template := `@{is_group?g@{group_id}:@{is_pic_set?s@{pic_set_index}:@{is_series?ser:one}}}`

	tests := []struct {
		name string
		ctx  Context
		want string
	}{
		{"group wins", Context{IsGroup: true, GroupID: 3, IsPicSet: true, PicSetIndex: 2}, "g3"},
		{"picture set", Context{IsPicSet: true, PicSetIndex: 2}, "s2"},
		{"series", Context{IsSeries: true}, "ser"},
		{"single image", Context{IsPicOne: true}, "one"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := render(t, template, tc.ctx); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatting(t *testing.T) {
	ctx := Context{
		ID:          7,
		Title:       "AbC",
		ArtistName:  "PAINTER",
		PublishTime: macroTestTime,
		PicSetIndex: 12,
	}

	tests := []struct {
		template string
		want     string
	}{
		{`@{title:u}`, "ABC"},
		{`@{title:l}`, "abc"},
		{`@{artist_name:l}`, "painter"},
		{`@{id:000}`, "007"},
		{`@{id:0}`, "7"},
		{`@{pic_set_index:0000}`, "0012"},
		{`@{publish_time:yyyy-M-d}`, "2024-3-5"},
		{`@{publish_time:yyyy-MM-dd}`, "2024-03-05"},
		{`@{publish_time:yyyy/MM/dd}`, "2024/03/05"},
		{`@{publish_time}`, "2024-3-5"},
		{`id@{title:u}end`, "idABCend"},
	}

	for _, tc := range tests {
		if got := render(t, tc.template, ctx); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.template, got, tc.want)
		}
	}
}

func TestFormatIntKeepsSignOutsideWidth(t *testing.T) {
	if got := render(t, `@{artist_id:000}`, Context{ArtistID: -5}); got != "-005" {
		t.Fatalf("got %q, want %q", got, "-005")
	}
}

func TestEscapedAtSign(t *testing.T) {
	tests := []struct {
		template string
		want     string
	}{
		{`@@`, "@"},
		{`a@@b`, "a@b"},
		{`@@{id}`, "@{id}"},
		{`@{id}@@`, "42@"},
	}

	for _, tc := range tests {
		if got := render(t, tc.template, Context{ID: 42}); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.template, got, tc.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{"unclosed macro", `@{id`},
		{"unclosed conditional", `@{is_group?x`},
		{"missing conditional separator", `@{is_group?x}`},
		{"unknown key", `@{nope}`},
		{"empty name", `@{}`},
		{"stray at sign", `a@b`},
		{"format on condition", `@{is_pic_set:u?x:y}`},
		{"branches on value", `@{id?x:y}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.template); err == nil {
				t.Fatalf("Parse(%q) succeeded, want error", tc.template)
			}
		})
	}
}

func TestRenderRejectsUnsupportedFormat(t *testing.T) {
	tests := []string{`@{title:upper}`, `@{id:abc}`}
	for _, template := range tests {
		tmpl, err := Parse(template)
		if err != nil {
			t.Fatalf("Parse(%q): %v", template, err)
		}

		if _, err = tmpl.Render(Context{Title: "x", ID: 1}); err == nil {
			t.Errorf("Render(%q) succeeded, want error", template)
		}
	}
}

func TestParseErrorReportsOffset(t *testing.T) {
	template := `abc@{nope}`
	_, err := Parse(template)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "offset 5") {
		t.Fatalf("error %q does not mention the key offset", err)
	}
}

func TestBracesOutsideMacrosArePlainText(t *testing.T) {
	if got := render(t, `a}b`, Context{}); got != "a}b" {
		t.Fatalf("got %q, want %q", got, "a}b")
	}
}

func TestTextIsPassedThroughUnchanged(t *testing.T) {
	ctx := Context{Title: "中文/标题", ArtistName: "画家"}

	got := render(t, `@{artist_name} @{title}.jpg`, ctx)
	want := "画家 中文/标题.jpg"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
