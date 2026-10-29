package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// searchHistoryLimit caps the words kept, so the file cannot grow without
// bound.
const searchHistoryLimit = 50

// SearchHistory is the words the user searched for, newest first. The view
// reads it on the main thread; mu guards it from the goroutine that saves it.
type SearchHistory struct {
	mu    sync.Mutex
	Words []string `json:"words"`
}

// searchHistoryPath is where the searched words live: the user's config
// directory, like the settings.
func searchHistoryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "search_history.json"), nil
}

// loadSearchHistory reads the saved words, or an empty list when there is
// none or the file cannot be read.
func loadSearchHistory() *SearchHistory {
	h := &SearchHistory{}
	path, err := searchHistoryPath()
	if err != nil {
		log.Printf("search history: %v", err)
		return h
	}
	if err := h.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("search history: %v", err)
	}
	return h
}

// Load reads the words at path into h. A missing or broken file leaves h as
// it was.
func (h *SearchHistory) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Words []string `json:"words"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	if len(file.Words) > searchHistoryLimit {
		file.Words = file.Words[:searchHistoryLimit]
	}
	h.mu.Lock()
	h.Words = file.Words
	h.mu.Unlock()
	return nil
}

// Save writes h to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (h *SearchHistory) Save(path string) error {
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

// Add puts word first, drops an older copy of it, and keeps the list at
// searchHistoryLimit. A blank word is not a search.
func (h *SearchHistory) Add(word string) {
	word = strings.TrimSpace(word)
	if word == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	words := make([]string, 0, len(h.Words)+1)
	words = append(words, word)
	for _, old := range h.Words {
		if old != word {
			words = append(words, old)
		}
	}
	if len(words) > searchHistoryLimit {
		words = words[:searchHistoryLimit]
	}
	h.Words = words
}

// Remove drops word from the list.
func (h *SearchHistory) Remove(word string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	words := make([]string, 0, len(h.Words))
	for _, old := range h.Words {
		if old != word {
			words = append(words, old)
		}
	}
	h.Words = words
}

// Clear empties the list.
func (h *SearchHistory) Clear() {
	h.mu.Lock()
	h.Words = nil
	h.mu.Unlock()
}

// Snapshot returns the words, newest first, for the view to draw.
func (h *SearchHistory) Snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.Words...)
}

// recordSearch puts word at the top of the history and saves it in the
// background.
func (a *App) recordSearch(word string) {
	if a.searchHistory == nil {
		return
	}
	a.searchHistory.Add(word)
	a.saveSearchHistory()
}

// saveSearchHistory writes the history off the main thread. Only the running
// window saves: a headless view or a test has no business writing the user's
// file.
func (a *App) saveSearchHistory() {
	if a.searchHistory == nil || a.win == nil {
		return
	}
	path, err := searchHistoryPath()
	if err != nil {
		log.Printf("search history: %v", err)
		return
	}
	h := a.searchHistory
	go func() {
		if err := h.Save(path); err != nil {
			log.Printf("search history: %v", err)
		}
	}()
}

// removeSearchWord drops word from the history and saves it in the background.
func (a *App) removeSearchWord(word string) {
	if a.searchHistory == nil {
		return
	}
	a.searchHistory.Remove(word)
	a.saveSearchHistory()
}

// clearSearchHistory empties the history and saves it in the background.
func (a *App) clearSearchHistory() {
	if a.searchHistory == nil {
		return
	}
	a.searchHistory.Clear()
	a.saveSearchHistory()
}
