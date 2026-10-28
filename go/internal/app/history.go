package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// historyLimit caps the entries kept, so the file cannot grow without bound.
const historyLimit = 200

// HistoryEntry is one work the user opened, and when.
type HistoryEntry struct {
	ID     int64     `json:"id"`
	Title  string    `json:"title"`
	Artist string    `json:"artist"`
	Cover  string    `json:"cover"`
	At     time.Time `json:"at"`
}

// History is the works the user opened, newest first. The view reads it on
// the main thread; mu guards it from the goroutine that saves it.
type History struct {
	mu      sync.Mutex
	Entries []HistoryEntry `json:"entries"`
}

// historyPath is where the history lives: the user's config directory.
func historyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "history.json"), nil
}

// loadHistory reads the saved history, or an empty one when there is none or
// the file cannot be read.
func loadHistory() *History {
	h := &History{}
	path, err := historyPath()
	if err != nil {
		log.Printf("history: %v", err)
		return h
	}
	if err := h.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("history: %v", err)
	}
	return h
}

// Load reads the history at path into h. A missing or broken file leaves h
// as it was.
func (h *History) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Entries []HistoryEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	h.mu.Lock()
	h.Entries = file.Entries
	h.mu.Unlock()
	return nil
}

// Save writes h to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (h *History) Save(path string) error {
	h.mu.Lock()
	data, err := json.MarshalIndent(h, "", "  ")
	h.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Add puts e first, drops an older entry for the same work, and keeps the
// list at historyLimit.
func (h *History) Add(e HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := make([]HistoryEntry, 0, len(h.Entries)+1)
	entries = append(entries, e)
	for _, old := range h.Entries {
		if old.ID != e.ID {
			entries = append(entries, old)
		}
	}
	if len(entries) > historyLimit {
		entries = entries[:historyLimit]
	}
	h.Entries = entries
}

// Clear empties the history.
func (h *History) Clear() {
	h.mu.Lock()
	h.Entries = nil
	h.mu.Unlock()
}

// Snapshot returns the entries, newest first, for the view to draw.
func (h *History) Snapshot() []HistoryEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]HistoryEntry(nil), h.Entries...)
}

// recordHistory puts w at the top of the history and saves it in the
// background.
func (a *App) recordHistory(w pixiv.Illustration) {
	if a.history == nil {
		return
	}
	a.history.Add(HistoryEntry{
		ID:     w.ID,
		Title:  w.Title,
		Artist: w.User.Name,
		Cover:  w.CoverURL(),
		At:     time.Now(),
	})
	a.saveHistory()
}

// saveHistory writes the history off the main thread. Only the running window
// saves: a headless view or a test has no business writing the user's file.
func (a *App) saveHistory() {
	if a.history == nil || a.win == nil {
		return
	}
	path, err := historyPath()
	if err != nil {
		log.Printf("history: %v", err)
		return
	}
	h := a.history
	go func() {
		if err := h.Save(path); err != nil {
			log.Printf("history: %v", err)
		}
	}()
}

// clearHistory empties the history and saves it in the background.
func (a *App) clearHistory() {
	if a.history == nil {
		return
	}
	a.history.Clear()
	a.saveHistory()
}
