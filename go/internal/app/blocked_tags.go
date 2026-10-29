package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// blockedTagLimit caps the entries kept, so the file cannot grow without
// bound.
const blockedTagLimit = 500

// BlockedTag is one illustration tag whose works are hidden.
type BlockedTag struct {
	Name string `json:"name"`
}

// BlockedTags is the tags the viewer blocked, newest first. Tags compare
// ignoring case and surrounding spaces. The view reads it on the main thread;
// mu guards it from the goroutine that saves it.
type BlockedTags struct {
	mu   sync.Mutex
	Tags []BlockedTag `json:"tags"`
}

// blockedTagsPath is where the list lives: the user's config directory.
func blockedTagsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "blocked_tags.json"), nil
}

// loadBlockedTags reads the saved list, or an empty one when there is none or
// the file cannot be read.
func loadBlockedTags() *BlockedTags {
	b := &BlockedTags{}
	path, err := blockedTagsPath()
	if err != nil {
		log.Printf("blocked tags: %v", err)
		return b
	}
	if err := b.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("blocked tags: %v", err)
	}
	return b
}

// Load reads the list at path into b. A missing or broken file leaves b as it
// was.
func (b *BlockedTags) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Tags []BlockedTag `json:"tags"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	b.mu.Lock()
	b.Tags = file.Tags
	b.mu.Unlock()
	return nil
}

// Save writes b to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (b *BlockedTags) Save(path string) error {
	b.mu.Lock()
	data, err := json.MarshalIndent(b, "", "  ")
	b.mu.Unlock()
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

// tagKey is a tag name as the comparisons see it: without surrounding spaces
// and in one case.
func tagKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Add puts the tag first, drops an older entry for the same tag ignoring case
// and spaces, and keeps the list at blockedTagLimit.
func (b *BlockedTags) Add(t BlockedTag) {
	name := strings.TrimSpace(t.Name)
	key := tagKey(name)
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tags := make([]BlockedTag, 0, len(b.Tags)+1)
	tags = append(tags, BlockedTag{Name: name})
	for _, old := range b.Tags {
		if tagKey(old.Name) != key {
			tags = append(tags, old)
		}
	}
	if len(tags) > blockedTagLimit {
		tags = tags[:blockedTagLimit]
	}
	b.Tags = tags
}

// Remove drops the entry for name, leaving the rest in place.
func (b *BlockedTags) Remove(name string) {
	key := tagKey(name)
	if key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tags := make([]BlockedTag, 0, len(b.Tags))
	for _, tag := range b.Tags {
		if tagKey(tag.Name) != key {
			tags = append(tags, tag)
		}
	}
	b.Tags = tags
}

// Has reports whether the tag is blocked, ignoring case and spaces.
func (b *BlockedTags) Has(name string) bool {
	key := tagKey(name)
	if key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tag := range b.Tags {
		if tagKey(tag.Name) == key {
			return true
		}
	}
	return false
}

// Clear empties the list.
func (b *BlockedTags) Clear() {
	b.mu.Lock()
	b.Tags = nil
	b.mu.Unlock()
}

// Snapshot returns the entries, newest first, for the view to draw.
func (b *BlockedTags) Snapshot() []BlockedTag {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BlockedTag(nil), b.Tags...)
}

// isTagBlocked reports whether the tag's works are hidden.
func (a *App) isTagBlocked(name string) bool {
	return a.blockedTags != nil && a.blockedTags.Has(name)
}

// workHasBlockedTag reports whether the work carries a blocked tag, in either
// its original or translated name.
func (a *App) workHasBlockedTag(w pixiv.Illustration) bool {
	if a.blockedTags == nil {
		return false
	}
	for _, tag := range w.Tags {
		if a.isTagBlocked(tag.Name) || a.isTagBlocked(tag.TranslatedName) {
			return true
		}
	}
	return false
}

// blockTag hides works carrying name, takes them out of the lists and the
// viewer already loaded, and saves the block.
func (a *App) blockTag(name string) {
	name = strings.TrimSpace(name)
	if name == "" || a.blockedTags == nil {
		return
	}
	a.blockedTags.Add(BlockedTag{Name: name})
	a.removeBlockedTagsFromLists()
	if a.viewerBlockedByTags() {
		a.closeViewer()
	}
	a.saveBlockedTags()
	a.notice = a.tf("block.tagAdded", name)
}

// viewerBlockedByTags reports whether the open viewer shows a work carrying a
// blocked tag. The detail's tag list is the fuller one once it loaded.
func (a *App) viewerBlockedByTags() bool {
	if a.viewer == nil {
		return false
	}
	if detail, _, _ := a.viewer.detailState(); detail != nil {
		if a.workHasBlockedTag(*detail) {
			return true
		}
	}
	return a.workHasBlockedTag(a.viewer.work)
}

// unblockTag drops the block on name and saves the list.
func (a *App) unblockTag(name string) {
	if a.blockedTags == nil {
		return
	}
	a.blockedTags.Remove(name)
	a.saveBlockedTags()
}

// clearBlockedTags empties the list and saves it.
func (a *App) clearBlockedTags() {
	if a.blockedTags == nil {
		return
	}
	a.blockedTags.Clear()
	a.saveBlockedTags()
}

// blockedTagsSnapshot returns the blocked tags for the settings page.
func (a *App) blockedTagsSnapshot() []BlockedTag {
	if a.blockedTags == nil {
		return nil
	}
	return a.blockedTags.Snapshot()
}

// removeBlockedTagsFromLists drops the works carrying a blocked tag from the
// feeds loaded already, so a block takes effect on the current page at once.
func (a *App) removeBlockedTagsFromLists() {
	feeds := []*browseState{a.newFeed, a.myPixivFeed, a.search, a.bookmarks}
	feeds = append(feeds, a.homeCardFeeds()...)
	if a.feed != nil {
		feeds = append(feeds, a.feed.st)
	}
	if a.related != nil {
		feeds = append(feeds, a.related.feed)
	}
	if a.userView != nil {
		feeds = append(feeds, a.userView.feed, a.userView.mangaFeed,
			a.userView.bookmarksFeed, a.userView.bookmarksPrivateFeed)
	}
	for _, st := range feeds {
		if st != nil {
			st.removeWorks(a.workHasBlockedTag)
		}
	}
}

// saveBlockedTags writes the list off the main thread. Only the running
// window saves: a headless view or a test has no business writing the user's
// file.
func (a *App) saveBlockedTags() {
	if a.blockedTags == nil || a.win == nil {
		return
	}
	path, err := blockedTagsPath()
	if err != nil {
		log.Printf("blocked tags: %v", err)
		return
	}
	b := a.blockedTags
	go func() {
		if err := b.Save(path); err != nil {
			log.Printf("blocked tags: %v", err)
		}
	}()
}
