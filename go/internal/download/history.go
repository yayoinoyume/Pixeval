package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// historyLimit caps the completed downloads kept, so the file cannot grow
// without bound.
const historyLimit = 500

// HistoryEntry is one finished download, and when it finished.
type HistoryEntry struct {
	ID          string    `json:"id"`
	IllustID    int64     `json:"illustId"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Dest        string    `json:"dest"`
	Page        int       `json:"page"`
	Source      string    `json:"source,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

// History is the completed downloads, newest first. The view reads it on the
// main thread; mu guards it from the queue's change callbacks.
type History struct {
	mu      sync.Mutex
	entries []HistoryEntry
}

// historyFile is the on-disk history.
type historyFile struct {
	Entries []HistoryEntry `json:"entries"`
}

// NewHistory returns an empty history.
func NewHistory() *History { return &History{} }

// LoadHistory reads the history saved at path. A missing file is an empty
// history; a broken one is reported with an empty history, so the app can
// start without records.
func LoadHistory(path string) (*History, error) {
	h := NewHistory()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return h, nil
		}
		return h, err
	}
	var file historyFile
	if err := json.Unmarshal(data, &file); err != nil {
		return h, fmt.Errorf("download history: %w", err)
	}
	h.entries = file.Entries
	return h, nil
}

// Save writes the history to path as JSON only the user can read, replacing
// the file in one step so a crash cannot leave a half written one. The lock is
// held across the write, so a clear or a second save cannot interleave.
func (h *History) Save(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := json.Marshal(historyFile{Entries: h.entries})
	if err != nil {
		return err
	}
	return writeDownloads(path, data)
}

// Add puts e first, drops an older entry for the same task, and keeps the list
// at historyLimit.
func (h *History) Add(e HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := make([]HistoryEntry, 0, min(len(h.entries)+1, historyLimit))
	entries = append(entries, e)
	for _, old := range h.entries {
		if old.ID != e.ID {
			entries = append(entries, old)
		}
	}
	if len(entries) > historyLimit {
		entries = entries[:historyLimit]
	}
	h.entries = entries
}

// Clear empties the history.
func (h *History) Clear() {
	h.mu.Lock()
	h.entries = nil
	h.mu.Unlock()
}

// Snapshot returns the entries, newest first, for the view to draw.
func (h *History) Snapshot() []HistoryEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]HistoryEntry(nil), h.entries...)
}
