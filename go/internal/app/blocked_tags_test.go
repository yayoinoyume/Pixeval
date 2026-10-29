package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// TestBlockedTagsAddDedupeAndLimit checks a new tag goes first, the same tag
// added again in another case moves to the top instead of repeating, and the
// list keeps its limit.
func TestBlockedTagsAddDedupeAndLimit(t *testing.T) {
	b := &BlockedTags{}
	for i := 1; i <= blockedTagLimit+5; i++ {
		b.Add(BlockedTag{Name: fmt.Sprintf("tag%d", i)})
	}
	if got := len(b.Tags); got != blockedTagLimit {
		t.Fatalf("tags %d, want %d", got, blockedTagLimit)
	}
	if got, want := b.Tags[0].Name, fmt.Sprintf("tag%d", blockedTagLimit+5); got != want {
		t.Errorf("first tag %q, want %q", got, want)
	}

	b.Add(BlockedTag{Name: "  TAG100 "})
	if got := len(b.Tags); got != blockedTagLimit {
		t.Fatalf("tags %d after blocking an old tag, want %d", got, blockedTagLimit)
	}
	if got := b.Tags[0].Name; got != "TAG100" {
		t.Errorf("first tag %q, want the re-blocked tag", got)
	}
	n := 0
	for _, tag := range b.Tags {
		if tagKey(tag.Name) == tagKey("tag100") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("tag100 appears %d times, want 1", n)
	}
}

// TestBlockedTagsIgnoreCaseAndSpace checks adding, finding and removing a tag
// all compare without case or surrounding spaces.
func TestBlockedTagsIgnoreCaseAndSpace(t *testing.T) {
	b := &BlockedTags{}
	b.Add(BlockedTag{Name: "  Illustration "})
	if !b.Has("illustration") {
		t.Fatalf("tags %+v, want the tag found in another case", b.Snapshot())
	}
	if !b.Has(" ILLUSTRATION  ") {
		t.Error("Has does not ignore surrounding spaces")
	}
	if got := b.Snapshot(); len(got) != 1 || got[0].Name != "Illustration" {
		t.Errorf("tags %+v, want the name saved without its spaces", got)
	}

	b.Remove("ILLUSTRATION")
	if b.Has("illustration") {
		t.Error("tag still blocked after Remove")
	}
	if got := b.Snapshot(); len(got) != 0 {
		t.Errorf("tags %+v after removing, want none", got)
	}
}

// TestBlockedTagsLoadSave checks a saved list loads back as it was, in a file
// only the user can read.
func TestBlockedTagsLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "blocked_tags.json")
	b := &BlockedTags{}
	b.Add(BlockedTag{Name: "オリジナル"})
	if err := b.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %o, want 600", got)
	}

	got := &BlockedTags{}
	if err := got.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tags := got.Snapshot()
	if len(tags) != 1 || tags[0].Name != "オリジナル" {
		t.Errorf("tags %+v, want the saved one", tags)
	}
}

// TestLoadBlockedTagsFromConfigDir checks a start reads the list the user
// saved last time.
func TestLoadBlockedTagsFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := blockedTagsPath()
	if err != nil {
		t.Fatalf("blockedTagsPath: %v", err)
	}
	saved := &BlockedTags{}
	saved.Add(BlockedTag{Name: "風景"})
	if err := saved.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a := New()
	if !a.isTagBlocked("風景") {
		t.Errorf("tag not loaded: %+v", a.blockedTags.Snapshot())
	}
}

// TestBrowseFiltersBlockedTags checks a loaded page leaves works carrying a
// blocked tag out, comparing without case or spaces, and keeps the cursor.
func TestBrowseFiltersBlockedTags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	works := []pixiv.Illustration{
		{ID: 1, Title: "作品一", User: pixiv.User{ID: 7}, Width: 480, Height: 720,
			Tags: []pixiv.Tag{{Name: "R-18"}}},
		{ID: 2, Title: "作品二", User: pixiv.User{ID: 8}, Width: 480, Height: 720,
			Tags: []pixiv.Tag{{Name: "風景"}}},
		{ID: 3, Title: "作品三", User: pixiv.User{ID: 9}, Width: 480, Height: 720,
			Tags: []pixiv.Tag{{Name: "オリジナル", TranslatedName: " Original "}}},
	}
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: works, NextURL: "next"}, nil
	}}
	a := New()
	a.images = imagex.New(nil, "")
	a.blockedTags.Add(BlockedTag{Name: " r-18 "})
	a.blockedTags.Add(BlockedTag{Name: "original"})
	st.arm()
	a.loadMore(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return v.started })

	v := st.snapshot()
	if len(v.items) != 1 || v.items[0].ID != 2 {
		t.Fatalf("items %+v, want only work 2", v.items)
	}
	if len(v.tiles) != 1 {
		t.Errorf("tiles %d, want 1", len(v.tiles))
	}
	if got := st.cursor(); got != "next" {
		t.Errorf("cursor %q, want %q", got, "next")
	}
}

// TestBlockTagRemovesWorkFromLists checks blocking a tag takes the works that
// carry it out of a feed loaded already, tiles and all.
func TestBlockTagRemovesWorkFromLists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.images = imagex.New(nil, "")
	a.search.items = []pixiv.Illustration{
		{ID: 1, Tags: []pixiv.Tag{{Name: "オリジナル"}}},
		{ID: 2, Tags: []pixiv.Tag{{Name: "風景"}}},
	}
	a.search.tiles = []widgets.MasonryItem{{Key: 1}, {Key: 2}}

	a.blockTag(" オリジナル ")
	if !a.blockedTags.Has("オリジナル") {
		t.Fatal("tag not blocked")
	}
	if got := a.search.snapshot().items; len(got) != 1 || got[0].ID != 2 {
		t.Errorf("items %+v after blocking, want only work 2", got)
	}
	if got := a.search.snapshot().tiles; len(got) != 1 || got[0].Key != 2 {
		t.Errorf("tiles %+v after blocking, want only work 2's", got)
	}
}
