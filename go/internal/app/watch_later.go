package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// watchLaterLimit caps the entries kept, so the file cannot grow without bound.
const watchLaterLimit = 200

// WatchLaterEntry is one work the user saved for later, and when.
type WatchLaterEntry struct {
	ID     int64     `json:"id"`
	Title  string    `json:"title"`
	Artist string    `json:"artist"`
	Cover  string    `json:"cover"`
	At     time.Time `json:"at"`
}

// WatchLater is the works the user saved for later, newest first. The view
// reads it on the main thread; mu guards it from the goroutine that saves it.
type WatchLater struct {
	mu      sync.Mutex
	Entries []WatchLaterEntry `json:"entries"`
}

// watchLaterPath is where the list lives: the user's config directory.
func watchLaterPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "watch_later.json"), nil
}

// loadWatchLater reads the saved list, or an empty one when there is none or
// the file cannot be read.
func loadWatchLater() *WatchLater {
	w := &WatchLater{}
	path, err := watchLaterPath()
	if err != nil {
		log.Printf("watch later: %v", err)
		return w
	}
	if err := w.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("watch later: %v", err)
	}
	return w
}

// Load reads the list at path into w. A missing or broken file leaves w as it
// was.
func (w *WatchLater) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Entries []WatchLaterEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	w.mu.Lock()
	w.Entries = file.Entries
	w.mu.Unlock()
	return nil
}

// Save writes w to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (w *WatchLater) Save(path string) error {
	w.mu.Lock()
	data, err := json.MarshalIndent(w, "", "  ")
	w.mu.Unlock()
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
// list at watchLaterLimit.
func (w *WatchLater) Add(e WatchLaterEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries := make([]WatchLaterEntry, 0, len(w.Entries)+1)
	entries = append(entries, e)
	for _, old := range w.Entries {
		if old.ID != e.ID {
			entries = append(entries, old)
		}
	}
	if len(entries) > watchLaterLimit {
		entries = entries[:watchLaterLimit]
	}
	w.Entries = entries
}

// Remove drops the entry for id, leaving the rest in place.
func (w *WatchLater) Remove(id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries := make([]WatchLaterEntry, 0, len(w.Entries))
	for _, e := range w.Entries {
		if e.ID != id {
			entries = append(entries, e)
		}
	}
	w.Entries = entries
}

// Has reports whether the work id is saved for later.
func (w *WatchLater) Has(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.Entries {
		if e.ID == id {
			return true
		}
	}
	return false
}

// Clear empties the list.
func (w *WatchLater) Clear() {
	w.mu.Lock()
	w.Entries = nil
	w.mu.Unlock()
}

// Snapshot returns the entries, newest first, for the view to draw.
func (w *WatchLater) Snapshot() []WatchLaterEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]WatchLaterEntry(nil), w.Entries...)
}

// addWatchLater puts w at the top of the list and saves it in the background.
func (a *App) addWatchLater(w pixiv.Illustration) {
	if a.watchLater == nil {
		return
	}
	a.watchLater.Add(WatchLaterEntry{
		ID:     w.ID,
		Title:  w.Title,
		Artist: w.User.Name,
		Cover:  w.ThumbURL(),
		At:     time.Now(),
	})
	a.saveWatchLater()
}

// removeWatchLater drops a work from the list and saves it in the background.
func (a *App) removeWatchLater(id int64) {
	if a.watchLater == nil {
		return
	}
	a.watchLater.Remove(id)
	a.saveWatchLater()
}

// toggleWatchLater adds w, or removes it when it is already saved.
func (a *App) toggleWatchLater(w pixiv.Illustration) {
	if a.inWatchLater(w.ID) {
		a.removeWatchLater(w.ID)
		return
	}
	a.addWatchLater(w)
}

// inWatchLater reports whether a work is saved for later.
func (a *App) inWatchLater(id int64) bool {
	return a.watchLater != nil && a.watchLater.Has(id)
}

// clearWatchLater empties the list and saves it in the background.
func (a *App) clearWatchLater() {
	if a.watchLater == nil {
		return
	}
	a.watchLater.Clear()
	a.saveWatchLater()
}

// saveWatchLater writes the list off the main thread. Only the running window
// saves: a headless view or a test has no business writing the user's file.
func (a *App) saveWatchLater() {
	if a.watchLater == nil || a.win == nil {
		return
	}
	path, err := watchLaterPath()
	if err != nil {
		log.Printf("watch later: %v", err)
		return
	}
	w := a.watchLater
	go func() {
		if err := w.Save(path); err != nil {
			log.Printf("watch later: %v", err)
		}
	}()
}

// watchLaterMenuLabel is the card menu's name for the action: the work's
// saved state says whether it adds it or removes it.
func (a *App) watchLaterMenuLabel(id int64) string {
	if a.inWatchLater(id) {
		return a.t("watchLater.remove")
	}
	return a.t("watchLater.add")
}

// watchLaterPage draws the works saved for later, newest first.
func (a *App) watchLaterPage(c *ui.Context) {
	t := themeOf(c)
	var entries []WatchLaterEntry
	if a.watchLater != nil {
		entries = a.watchLater.Snapshot()
	}
	if len(entries) == 0 {
		emptyState(c, iconHistory, a.t("watchLater.empty"), a.t("watchLater.emptyHint"), nil)
		return
	}
	// The list is a grid of the same cards the history page draws, so it uses
	// the history page's fit rather than a second one.
	cols, cardW := a.historyGrid(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, t.SpaceS, contentPadding).
			Justify(ui.End).Children(func() {
			if ui.Button(c, a.t("watchLater.clear")).Clicked() {
				a.clearWatchLater()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(0, contentPadding, contentPadding, contentPadding).
				Gap(cardGap).Children(func() {
				for i := 0; i < len(entries); i += cols {
					row := entries[i:min(i+cols, len(entries))]
					ui.Row(c).Gap(cardGap).AlignItems(ui.Start).Children(func() {
						for _, e := range row {
							a.watchLaterCard(c, e, cardW)
						}
					})
				}
			})
		})
	})
}

// watchLaterCard draws one saved entry: its cover, title and author, and the
// control that drops it. Clicking the card fetches the work and opens the
// viewer.
func (a *App) watchLaterCard(c *ui.Context, e WatchLaterEntry, width float32) {
	t := themeOf(c)
	card, cover := cardShell(c)
	card = card.Width(width).Focusable().
		Label(a.tf("a11y.card", e.Title, e.Artist)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openSavedWork(e.ID, "watchLater.openFailed")
	})
	cover.Grow(0).Height(width * 3 / 2)
	cover.Children(func() {
		if e.Cover == "" {
			return // an entry saved without a cover has no image to draw
		}
		if a.watchLaterFailed[e.Cover] {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(e.Cover, coverWidth, coverHeight, a.watchLaterCoverReady(e.Cover)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", e.Title))
		}
	})
	card.Children(func() {
		ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, e.Title).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).MaxLines(1)
				ui.Text(c, e.Artist).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
			})
			if iconButton(c, iconClose, a.t("watchLater.remove"), 24, t.TextMuted, t.SurfaceAlt).Clicked() {
				a.removeWatchLater(e.ID)
			}
		})
	})
}

// watchLaterCoverReady returns the callback a cover load calls: a probe that
// finds nothing means the load failed, and the cover is a placeholder from
// then on rather than a load started on every frame.
func (a *App) watchLaterCoverReady(key string) func() {
	return func() {
		failed := a.images.Thumb(key, coverWidth, coverHeight, nil) == nil
		a.update(func() {
			if failed {
				a.watchLaterFailed[key] = true
			}
		})
	}
}
