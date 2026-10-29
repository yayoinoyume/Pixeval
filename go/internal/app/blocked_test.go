package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestBlockedUsersAddDedupeAndLimit checks a new block goes first, a user
// blocked again moves to the top instead of repeating, and the list keeps its
// limit.
func TestBlockedUsersAddDedupeAndLimit(t *testing.T) {
	b := &BlockedUsers{}
	for i := int64(1); i <= blockedUserLimit+5; i++ {
		b.Add(BlockedUser{ID: i, Name: fmt.Sprintf("用户%d", i)})
	}
	if got := len(b.Users); got != blockedUserLimit {
		t.Fatalf("users %d, want %d", got, blockedUserLimit)
	}
	if got := b.Users[0].ID; got != blockedUserLimit+5 {
		t.Errorf("first ID %d, want %d", got, blockedUserLimit+5)
	}

	b.Add(BlockedUser{ID: 100, Name: "新名字"})
	if got := len(b.Users); got != blockedUserLimit {
		t.Fatalf("users %d after blocking an old user, want %d", got, blockedUserLimit)
	}
	if b.Users[0].ID != 100 || b.Users[0].Name != "新名字" {
		t.Errorf("first entry %+v, want the re-blocked user", b.Users[0])
	}
	n := 0
	for _, u := range b.Users {
		if u.ID == 100 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("user 100 appears %d times, want 1", n)
	}
}

// TestBlockedUsersRemove checks removing drops only the named user.
func TestBlockedUsersRemove(t *testing.T) {
	b := &BlockedUsers{}
	b.Add(BlockedUser{ID: 1, Name: "一"})
	b.Add(BlockedUser{ID: 2, Name: "二"})
	if !b.Has(1) || !b.Has(2) {
		t.Fatalf("Has: both users should be blocked, got %+v", b.Snapshot())
	}
	b.Remove(1)
	if b.Has(1) {
		t.Error("user 1 still blocked after Remove")
	}
	if got := b.Snapshot(); len(got) != 1 || got[0].ID != 2 {
		t.Errorf("entries %+v, want user 2 only", got)
	}
}

// TestBlockedUsersLoadSave checks a saved list loads back as it was, in a file
// only the user can read.
func TestBlockedUsersLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "blocked_users.json")
	b := &BlockedUsers{}
	b.Add(BlockedUser{ID: 7, Name: "画师七"})
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

	got := &BlockedUsers{}
	if err := got.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	users := got.Snapshot()
	if len(users) != 1 || users[0].ID != 7 || users[0].Name != "画师七" {
		t.Errorf("users %+v, want the saved one", users)
	}
}

// TestLoadBlockedUsersFromConfigDir checks a start reads the list the user
// saved last time.
func TestLoadBlockedUsersFromConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := blockedUsersPath()
	if err != nil {
		t.Fatalf("blockedUsersPath: %v", err)
	}
	saved := &BlockedUsers{}
	saved.Add(BlockedUser{ID: 9, Name: "画师九"})
	if err := saved.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a := New()
	if !a.isBlocked(9) {
		t.Errorf("user 9 not loaded: %+v", a.blocked.Snapshot())
	}
}

// TestBrowseFiltersBlockedAuthors checks a loaded page leaves the blocked
// author's works out and keeps the page cursor.
func TestBrowseFiltersBlockedAuthors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	works := []pixiv.Illustration{
		{ID: 1, Title: "作品一", User: pixiv.User{ID: 7, Name: "画师七"}, Width: 480, Height: 720},
		{ID: 2, Title: "作品二", User: pixiv.User{ID: 8, Name: "画师八"}, Width: 480, Height: 720},
	}
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: works, NextURL: "next"}, nil
	}}
	a := New()
	a.images = imagex.New(nil, "")
	a.blocked.Add(BlockedUser{ID: 7, Name: "画师七"})
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

// TestUserListFiltersBlockedUsers checks a loaded user list leaves blocked
// users out.
func TestUserListFiltersBlockedUsers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.images = imagex.New(nil, "")
	a.blocked.Add(BlockedUser{ID: 8, Name: "关注八"})
	st := a.searchUsers
	st.setFetch(func(context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
		return &pixiv.Page[pixiv.UserPreview]{Items: []pixiv.UserPreview{
			{User: pixiv.User{ID: 8, Name: "关注八"}},
			{User: pixiv.User{ID: 9, Name: "粉丝九"}},
		}}, nil
	})
	st.arm()
	a.loadMoreUsers(st)
	waitUsers(t, st, func(v userListView) bool { return v.started })
	v := st.snapshot()
	if len(v.users) != 1 || v.users[0].User.ID != 9 {
		t.Fatalf("users %+v, want only user 9", v.users)
	}
}

// TestBrowseCardMenuBlockAuthor checks the card's menu blocks the author and
// takes their works out of the list at once.
func TestBrowseCardMenuBlockAuthor(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	works := []pixiv.Illustration{
		{ID: 1, Title: "作品一", User: pixiv.User{ID: 7, Name: "画师七"}, Width: 480, Height: 720},
		{ID: 2, Title: "作品二", User: pixiv.User{ID: 8, Name: "画师八"}, Width: 480, Height: 720},
	}
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: works}, nil
	}}
	a, tt := newBrowseTest(st)
	tt.Frame()
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 2 })
	tt.Frame()

	if err := tt.RightClick("作品一"); err != nil {
		t.Fatalf("RightClick: %v", err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, a.t("block.author")) {
		t.Fatalf("menu = %v, want the block-author item", menu)
	}
	if err := tt.ChooseMenuItem(a.t("block.author")); err != nil {
		t.Fatalf("ChooseMenuItem: %v", err)
	}
	if !a.isBlocked(7) {
		t.Fatal("author 7 not blocked")
	}
	if v := st.snapshot(); len(v.items) != 1 || v.items[0].ID != 2 {
		t.Errorf("items %+v after blocking, want only work 2", v.items)
	}
}

// TestUserPageBlockButton checks the profile's block button blocks the user
// and returns to the page it was opened from.
func TestUserPageBlockButton(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newPageApp()
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()

	if err := tt.Click(a.t("block.user")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if !a.isBlocked(7) {
		t.Fatal("user 7 not blocked")
	}
	if a.route != routeHome {
		t.Errorf("route %q after blocking, want %q", a.route, routeHome)
	}
}

// TestSettingsPageUnblockUser checks the settings card lists a blocked user
// and offers to unblock them.
func TestSettingsPageUnblockUser(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.blocked.Add(BlockedUser{ID: 7, Name: "画师七"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	if !tt.HasText(a.tf("block.count", 1)) {
		t.Fatalf("count missing: %q", tt.Texts())
	}
	if !tt.HasText("画师七") {
		t.Fatalf("blocked user not listed: %q", tt.Texts())
	}
	if err := tt.Click(a.t("block.remove")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.isBlocked(7) {
		t.Error("user 7 still blocked after the remove button")
	}
	tt.Frame()
	if !tt.HasText(a.t("block.empty")) {
		t.Errorf("empty text missing: %q", tt.Texts())
	}
}

// TestSettingsPageClearBlockedUsers checks the settings card's clear button
// empties the list.
func TestSettingsPageClearBlockedUsers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.blocked.Add(BlockedUser{ID: 7, Name: "画师七"})
	a.blocked.Add(BlockedUser{ID: 8, Name: "画师八"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	if !tt.HasText(a.tf("block.count", 2)) {
		t.Fatalf("count missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("block.clear")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := a.blockedUsersSnapshot(); len(got) != 0 {
		t.Errorf("blocked users %+v after clearing, want none", got)
	}
}
