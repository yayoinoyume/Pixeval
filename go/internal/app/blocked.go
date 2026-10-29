package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// blockedUserLimit caps the entries kept, so the file cannot grow without
// bound.
const blockedUserLimit = 500

// BlockedUser is one user whose works and lists are hidden.
type BlockedUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// BlockedUsers is the users the viewer blocked, newest first. The view reads
// it on the main thread; mu guards it from the goroutine that saves it.
type BlockedUsers struct {
	mu    sync.Mutex
	Users []BlockedUser `json:"users"`
}

// blockedUsersPath is where the list lives: the user's config directory.
func blockedUsersPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "blocked_users.json"), nil
}

// loadBlockedUsers reads the saved list, or an empty one when there is none or
// the file cannot be read.
func loadBlockedUsers() *BlockedUsers {
	b := &BlockedUsers{}
	path, err := blockedUsersPath()
	if err != nil {
		log.Printf("blocked users: %v", err)
		return b
	}
	if err := b.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("blocked users: %v", err)
	}
	return b
}

// Load reads the list at path into b. A missing or broken file leaves b as it
// was.
func (b *BlockedUsers) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Users []BlockedUser `json:"users"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	b.mu.Lock()
	b.Users = file.Users
	b.mu.Unlock()
	return nil
}

// Save writes b to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (b *BlockedUsers) Save(path string) error {
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

// Add puts u first, drops an older entry for the same user, and keeps the
// list at blockedUserLimit.
func (b *BlockedUsers) Add(u BlockedUser) {
	b.mu.Lock()
	defer b.mu.Unlock()
	users := make([]BlockedUser, 0, len(b.Users)+1)
	users = append(users, u)
	for _, old := range b.Users {
		if old.ID != u.ID {
			users = append(users, old)
		}
	}
	if len(users) > blockedUserLimit {
		users = users[:blockedUserLimit]
	}
	b.Users = users
}

// Remove drops the entry for id, leaving the rest in place.
func (b *BlockedUsers) Remove(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	users := make([]BlockedUser, 0, len(b.Users))
	for _, u := range b.Users {
		if u.ID != id {
			users = append(users, u)
		}
	}
	b.Users = users
}

// Has reports whether the user id is blocked.
func (b *BlockedUsers) Has(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, u := range b.Users {
		if u.ID == id {
			return true
		}
	}
	return false
}

// Clear empties the list.
func (b *BlockedUsers) Clear() {
	b.mu.Lock()
	b.Users = nil
	b.mu.Unlock()
}

// Snapshot returns the entries, newest first, for the view to draw.
func (b *BlockedUsers) Snapshot() []BlockedUser {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BlockedUser(nil), b.Users...)
}

// isBlocked reports whether the user's works and lists are hidden.
func (a *App) isBlocked(id int64) bool {
	return a.blocked != nil && a.blocked.Has(id)
}

// filterBlockedWorks drops the works of blocked users and works carrying a
// blocked tag from a loaded page.
func (a *App) filterBlockedWorks(items []pixiv.Illustration) []pixiv.Illustration {
	kept := make([]pixiv.Illustration, 0, len(items))
	for _, w := range items {
		if !a.isBlocked(w.User.ID) && !a.workHasBlockedTag(w) {
			kept = append(kept, w)
		}
	}
	return kept
}

// filterBlockedUsers drops blocked users from a loaded list page.
func (a *App) filterBlockedUsers(users []pixiv.UserPreview) []pixiv.UserPreview {
	kept := make([]pixiv.UserPreview, 0, len(users))
	for _, u := range users {
		if !a.isBlocked(u.User.ID) {
			kept = append(kept, u)
		}
	}
	return kept
}

// blockUser hides the user's works and lists, drops them from the lists loaded
// already, and saves the block.
func (a *App) blockUser(u pixiv.User) {
	if u.ID <= 0 || a.blocked == nil {
		return
	}
	a.blocked.Add(BlockedUser{ID: u.ID, Name: u.Name})
	a.removeBlockedFromLists(u.ID)
	a.saveBlockedUsers()
	a.notice = a.tf("block.userAdded", u.Name)
}

// unblockUser drops the block on id and saves the list.
func (a *App) unblockUser(id int64) {
	if a.blocked == nil {
		return
	}
	a.blocked.Remove(id)
	a.saveBlockedUsers()
}

// clearBlockedUsers empties the list and saves it.
func (a *App) clearBlockedUsers() {
	if a.blocked == nil {
		return
	}
	a.blocked.Clear()
	a.saveBlockedUsers()
}

// blockedUsersSnapshot returns the blocked users for the settings page.
func (a *App) blockedUsersSnapshot() []BlockedUser {
	if a.blocked == nil {
		return nil
	}
	return a.blocked.Snapshot()
}

// removeBlockedFromLists drops the user's works and entries from the feeds and
// lists loaded already, so a block takes effect on the current page at once.
func (a *App) removeBlockedFromLists(userID int64) {
	feeds := []*browseState{a.newFeed, a.myPixivFeed, a.search, a.bookmarks}
	feeds = append(feeds, a.homeFeeds[:]...)
	if a.related != nil {
		feeds = append(feeds, a.related.feed)
	}
	if a.userView != nil {
		feeds = append(feeds, a.userView.feed, a.userView.mangaFeed,
			a.userView.bookmarksFeed, a.userView.bookmarksPrivateFeed)
	}
	for _, st := range feeds {
		if st != nil {
			st.removeAuthor(userID)
		}
	}
	lists := []*userListState{a.searchUsers}
	if a.userView != nil {
		lists = append(lists, a.userView.followingFeed, a.userView.followersFeed)
	}
	for _, st := range lists {
		if st != nil {
			st.removeUser(userID)
		}
	}
}

// saveBlockedUsers writes the list off the main thread. Only the running
// window saves: a headless view or a test has no business writing the user's
// file.
func (a *App) saveBlockedUsers() {
	if a.blocked == nil || a.win == nil {
		return
	}
	path, err := blockedUsersPath()
	if err != nil {
		log.Printf("blocked users: %v", err)
		return
	}
	b := a.blocked
	go func() {
		if err := b.Save(path); err != nil {
			log.Printf("blocked users: %v", err)
		}
	}()
}
