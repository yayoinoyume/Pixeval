package download

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func historyEntry(id string, at time.Time) HistoryEntry {
	return HistoryEntry{
		ID:          id,
		IllustID:    42,
		Title:       "作品 " + id,
		Artist:      "画师",
		Dest:        filepath.Join("dir", id+".jpg"),
		Page:        2,
		Source:      SourceSubscription,
		CompletedAt: at,
	}
}

// TestHistoryAddNewestFirstAndDedupes checks a repeated task moves to the top
// instead of appearing twice.
func TestHistoryAddNewestFirstAndDedupes(t *testing.T) {
	h := NewHistory()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	h.Add(historyEntry("1_0", base))
	h.Add(historyEntry("2_0", base.Add(time.Minute)))
	h.Add(historyEntry("1_0", base.Add(2*time.Minute)))

	got := h.Snapshot()
	if len(got) != 2 {
		t.Fatalf("history has %d entries, want 2: %+v", len(got), got)
	}
	if got[0].ID != "1_0" || got[1].ID != "2_0" {
		t.Fatalf("history order %q, %q, want the repeated task first", got[0].ID, got[1].ID)
	}
	if want := base.Add(2 * time.Minute); !got[0].CompletedAt.Equal(want) {
		t.Errorf("completed at %v, want %v", got[0].CompletedAt, want)
	}
}

// TestHistoryAddStopsAtLimit checks the oldest entries fall off once the cap is
// reached.
func TestHistoryAddStopsAtLimit(t *testing.T) {
	h := NewHistory()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.Local)
	for i := 0; i < historyLimit+10; i++ {
		h.Add(historyEntry(fmt.Sprintf("%d_0", i), base.Add(time.Duration(i)*time.Second)))
	}
	got := h.Snapshot()
	if len(got) != historyLimit {
		t.Fatalf("history has %d entries, want %d", len(got), historyLimit)
	}
	if got[0].ID != fmt.Sprintf("%d_0", historyLimit+9) {
		t.Errorf("newest entry %q, want the last added", got[0].ID)
	}
	if got[len(got)-1].ID != "10_0" {
		t.Errorf("oldest entry %q, want 10_0", got[len(got)-1].ID)
	}
}

// TestHistorySaveLoadRoundTrip checks every field survives a save and load, and
// that the file is private.
func TestHistorySaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_history.json")
	want := historyEntry("1_0", time.Date(2026, 5, 1, 12, 30, 0, 0, time.Local))
	h := NewHistory()
	h.Add(want)
	if err := h.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind: %v", err)
	}

	loaded, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	got := loaded.Snapshot()
	if len(got) != 1 {
		t.Fatalf("loaded %d entries, want 1: %+v", len(got), got)
	}
	if got[0].ID != want.ID || got[0].IllustID != want.IllustID ||
		got[0].Title != want.Title || got[0].Artist != want.Artist ||
		got[0].Dest != want.Dest || got[0].Page != want.Page ||
		got[0].Source != want.Source || !got[0].CompletedAt.Equal(want.CompletedAt) {
		t.Errorf("loaded entry %+v, want %+v", got[0], want)
	}
}

// TestLoadHistoryMissingIsEmpty checks the first run starts with no records.
func TestLoadHistoryMissingIsEmpty(t *testing.T) {
	h, err := LoadHistory(filepath.Join(t.TempDir(), "missing", "download_history.json"))
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if got := h.Snapshot(); len(got) != 0 {
		t.Fatalf("loaded %d entries, want none", len(got))
	}
}

// TestLoadHistoryBrokenIsEmpty checks a damaged file is reported without a
// panic, so the app can start empty.
func TestLoadHistoryBrokenIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(path)
	if err == nil {
		t.Fatal("LoadHistory accepted a broken file")
	}
	if got := h.Snapshot(); len(got) != 0 {
		t.Fatalf("loaded %d entries from a broken file, want none", len(got))
	}
}

// TestHistorySaveReplacesTheOldFile checks a second save replaces the content
// and leaves no temporary file.
func TestHistorySaveReplacesTheOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download_history.json")
	h := NewHistory()
	h.Add(historyEntry("1_0", time.Now()))
	if err := h.Save(path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	h.Clear()
	h.Add(historyEntry("2_0", time.Now()))
	if err := h.Save(path); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	loaded, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	got := loaded.Snapshot()
	if len(got) != 1 || got[0].ID != "2_0" {
		t.Errorf("loaded %+v, want only 2_0", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind: %v", err)
	}
}
