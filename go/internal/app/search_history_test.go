package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSearchHistoryAddDedupeAndLimit checks a new word goes first, a word
// searched again moves to the top instead of repeating, and the list keeps
// its limit.
func TestSearchHistoryAddDedupeAndLimit(t *testing.T) {
	h := &SearchHistory{}
	for i := 1; i <= searchHistoryLimit+5; i++ {
		h.Add(fmt.Sprintf("词%d", i))
	}
	if got := len(h.Words); got != searchHistoryLimit {
		t.Fatalf("words %d, want %d", got, searchHistoryLimit)
	}
	if got := h.Words[0]; got != fmt.Sprintf("词%d", searchHistoryLimit+5) {
		t.Errorf("first word %q, want the newest", got)
	}

	h.Add("词3")
	if got := len(h.Words); got != searchHistoryLimit {
		t.Fatalf("words %d after adding an old word, want %d", got, searchHistoryLimit)
	}
	if got := h.Words[0]; got != "词3" {
		t.Errorf("first word %q, want the re-searched one", got)
	}
	n := 0
	for _, word := range h.Words {
		if word == "词3" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("word 词3 appears %d times, want 1", n)
	}
	if got := h.Words[1]; got != fmt.Sprintf("词%d", searchHistoryLimit+5) {
		t.Errorf("second word %q, want the previous newest", got)
	}
}

// TestSearchHistoryRemoveAndClear checks one word can go without touching the
// rest, and clearing empties the list.
func TestSearchHistoryRemoveAndClear(t *testing.T) {
	h := &SearchHistory{}
	h.Add("猫")
	h.Add("狗")
	h.Add("鸟")
	h.Remove("狗")
	got := h.Snapshot()
	if len(got) != 2 || got[0] != "鸟" || got[1] != "猫" {
		t.Fatalf("words %v, want [鸟 猫]", got)
	}
	h.Clear()
	if got := h.Snapshot(); len(got) != 0 {
		t.Errorf("words %v after Clear, want none", got)
	}
}

// TestSearchHistoryLoadSave checks the words load back as they were, in a
// file only the user can read.
func TestSearchHistoryLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "search_history.json")
	h := &SearchHistory{}
	h.Add("猫")
	h.Add("狗")
	if err := h.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %o, want 600", got)
	}

	got := &SearchHistory{}
	if err := got.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	words := got.Snapshot()
	if len(words) != 2 || words[0] != "狗" || words[1] != "猫" {
		t.Errorf("words %v, want [狗 猫]", words)
	}
}

// TestSearchHistoryBrokenFile checks a broken file leaves an empty history
// rather than blocking the start.
func TestSearchHistoryBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := searchHistoryPath()
	if err != nil {
		t.Fatalf("searchHistoryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := (&SearchHistory{}).Load(path); err == nil {
		t.Error("Load accepted a broken file")
	}
	if got := len(loadSearchHistory().Snapshot()); got != 0 {
		t.Errorf("words %d for a broken file, want 0", got)
	}
}

// TestSearchHistoryRecordsSearch checks a started search records its word.
func TestSearchHistoryRecordsSearch(t *testing.T) {
	a := newSearchApp(pageBodies())
	a.searchHistory = &SearchHistory{}
	a.startSearch(a.search)
	waitBrowse(t, a.search, "load the first search",
		func(v browseView) bool { return v.started })
	words := a.searchHistory.Snapshot()
	if len(words) != 1 || words[0] != "猫" {
		t.Errorf("words %v, want [猫]", words)
	}
}

// TestSearchHistoryChipsSearchDeleteAndClear checks a chip searches its word,
// a chip's delete button removes just that word, and the clear button empties
// the list.
func TestSearchHistoryChipsSearchDeleteAndClear(t *testing.T) {
	a := newSearchApp(pageBodies())
	a.searchHistory = &SearchHistory{}
	a.searchHistory.Add("猫")
	a.searchHistory.Add("狗")
	a.search.query = ""

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	for _, want := range []string{"搜索历史", "猫", "狗", "清空历史"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing: %q", want, tt.Texts())
		}
	}

	if err := tt.Click("猫"); err != nil {
		t.Fatalf("Click chip: %v", err)
	}
	if a.search.query != "猫" {
		t.Errorf("query %q after the chip click, want 猫", a.search.query)
	}
	waitBrowse(t, a.search, "load the chip's search",
		func(v browseView) bool { return v.started })
	if got := a.searchHistory.Snapshot(); len(got) == 0 || got[0] != "猫" {
		t.Errorf("words %v after the chip click, want 猫 first", got)
	}

	tt.Frame()
	if err := tt.Click(a.tf("search.deleteHistory", "狗")); err != nil {
		t.Fatalf("Click delete: %v", err)
	}
	got := a.searchHistory.Snapshot()
	if len(got) != 1 || got[0] != "猫" {
		t.Errorf("words %v after deleting 狗, want [猫]", got)
	}

	tt.Frame()
	if err := tt.Click("清空历史"); err != nil {
		t.Fatalf("Click clear: %v", err)
	}
	if got := a.searchHistory.Snapshot(); len(got) != 0 {
		t.Errorf("words %v after clear, want none", got)
	}
}
