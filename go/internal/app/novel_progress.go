package app

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// novelProgressLimit caps the novels remembered, so the file cannot grow
// without bound.
const novelProgressLimit = 500

// novelProgressEdge is how close to either end of a novel a saved place must
// be to count as nothing: reading starts at the top and the end is reached
// anyway, so returning there would only surprise the reader.
const novelProgressEdge = 0.01

// NovelProgress remembers how far into each novel the reader was, as the
// place in the scroll range: 0 at the top and 1 at the end. The view reads it
// on the main thread; mu guards it from the goroutine that saves it.
type NovelProgress struct {
	mu     sync.Mutex
	Places map[int64]float64 `json:"places"`
}

// novelProgressPath is where the reading places live: the user's config
// directory, like the settings.
func novelProgressPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "novel_progress.json"), nil
}

// loadNovelProgress reads the saved places, or an empty set when there is
// none or the file cannot be read.
func loadNovelProgress() *NovelProgress {
	p := &NovelProgress{}
	path, err := novelProgressPath()
	if err != nil {
		log.Printf("novel progress: %v", err)
		return p
	}
	if err := p.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("novel progress: %v", err)
	}
	return p
}

// Load reads the places at path into p. A missing or broken file leaves p as
// it was.
func (p *NovelProgress) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Places map[int64]float64 `json:"places"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Places = file.Places
	if p.Places == nil {
		p.Places = map[int64]float64{}
	}
	return nil
}

// Save writes the places to path as JSON only the user can read, replacing
// the file in one step so a crash cannot leave a half written one.
func (p *NovelProgress) Save(path string) error {
	p.mu.Lock()
	data, err := json.MarshalIndent(struct {
		Places map[int64]float64 `json:"places"`
	}{p.Places}, "", "  ")
	p.mu.Unlock()
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

// Set remembers the novel's place. A place at the very top forgets the novel
// instead, and the list stays at novelProgressLimit.
func (p *NovelProgress) Set(novelID int64, fraction float64) {
	if novelID <= 0 || math.IsNaN(fraction) || math.IsInf(fraction, 0) {
		return
	}
	fraction = min(max(fraction, 0), 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Places == nil {
		p.Places = map[int64]float64{}
	}
	if fraction <= 0 {
		delete(p.Places, novelID)
		return
	}
	if _, have := p.Places[novelID]; !have && len(p.Places) >= novelProgressLimit {
		// No order is kept, so any entry may go: the novel read longest ago
		// is not known, and a stale one is the best this file can do.
		for old := range p.Places {
			delete(p.Places, old)
			break
		}
	}
	p.Places[novelID] = fraction
}

// Get returns the novel's saved place, or 0 when it has none.
func (p *NovelProgress) Get(novelID int64) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Places[novelID]
}

// novelRestoreFraction is the place worth returning to, or 0 for one too
// close to either end of the novel.
func novelRestoreFraction(fraction float64) float64 {
	if fraction <= novelProgressEdge || fraction >= 1-novelProgressEdge {
		return 0
	}
	return fraction
}
