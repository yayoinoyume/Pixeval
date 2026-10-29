package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// tokenFor is a signed-in token for a test user, valid for an hour.
func tokenFor(id int64, name string) *pixiv.Token {
	return &pixiv.Token{
		AccessToken:  fmt.Sprintf("access-%d", id),
		RefreshToken: fmt.Sprintf("refresh-%d", id),
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour),
		User:         pixiv.TokenUser{ID: pixiv.FlexInt64(id), Name: name, Account: name},
	}
}

// TestAccountsSaveLoadRoundTrip checks the accounts file keeps every field and
// only the user can read it.
func TestAccountsSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	now := time.Now().Truncate(time.Second)
	accs := &Accounts{}
	accs.Upsert(tokenFor(7, "七"), now)
	if err := accs.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode %o, want 600", perm)
	}
	loaded := &Accounts{}
	if err := loaded.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveID() != 7 {
		t.Errorf("active %d, want 7", loaded.ActiveID())
	}
	saved := loaded.Snapshot()
	if len(saved) != 1 {
		t.Fatalf("saved %d accounts, want 1", len(saved))
	}
	got := saved[0]
	if got.UserID != 7 || got.Name != "七" || got.Account != "七" {
		t.Errorf("account %+v", got)
	}
	if got.Token.RefreshToken != "refresh-7" || got.Token.AccessToken != "access-7" {
		t.Errorf("token %+v", got.Token)
	}
	if !got.LastUsed.Equal(now) {
		t.Errorf("last used %v, want %v", got.LastUsed, now)
	}
}

// TestAccountsUpsertDedupesAndCaps checks one entry per user, the newest first
// and the list capped.
func TestAccountsUpsertDedupesAndCaps(t *testing.T) {
	accs := &Accounts{}
	now := time.Now()
	for id := int64(1); id <= accountLimit+2; id++ {
		accs.Upsert(tokenFor(id, fmt.Sprintf("u%d", id)), now)
	}
	saved := accs.Snapshot()
	if len(saved) != accountLimit {
		t.Fatalf("saved %d accounts, want %d", len(saved), accountLimit)
	}
	if saved[0].UserID != accountLimit+2 || saved[len(saved)-1].UserID != 3 {
		t.Errorf("order %d..%d", saved[0].UserID, saved[len(saved)-1].UserID)
	}
	updated := tokenFor(5, "更新")
	accs.Upsert(updated, now.Add(time.Minute))
	saved = accs.Snapshot()
	if len(saved) != accountLimit {
		t.Fatalf("saved %d accounts after update, want %d", len(saved), accountLimit)
	}
	if saved[0].UserID != 5 || saved[0].Name != "更新" || saved[0].Token.AccessToken != "access-5" {
		t.Errorf("updated entry %+v", saved[0])
	}
	n := 0
	for _, acc := range saved {
		if acc.UserID == 5 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("user 5 saved %d times", n)
	}
	if accs.ActiveID() != 5 {
		t.Errorf("active %d, want 5", accs.ActiveID())
	}
}

// TestAccountsLoadSkipsIncomplete checks a broken entry is left out instead of
// failing the list, and an active id no entry has is dropped.
func TestAccountsLoadSkipsIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	body := `{"activeUserId":7,"accounts":[
		{"userId":7,"name":"no token"},
		{"userId":0,"name":"no user","token":{"refresh_token":"r"}},
		{"userId":8,"name":"八","token":{"refresh_token":"r2"}}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	accs := &Accounts{}
	if err := accs.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	saved := accs.Snapshot()
	if len(saved) != 1 || saved[0].UserID != 8 {
		t.Fatalf("saved %+v, want only user 8", saved)
	}
	if accs.ActiveID() != 0 {
		t.Errorf("active %d, want 0: the active account was dropped", accs.ActiveID())
	}
}

// TestLoadAccountsMigratesTokenFile checks the upgrade keeps the single
// token.json sign-in as the first saved account, leaving token.json in place.
func TestLoadAccountsMigratesTokenFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tokenFile, err := tokenPath()
	if err != nil {
		t.Fatalf("tokenPath: %v", err)
	}
	if err := pixiv.SaveToken(tokenFile, tokenFor(11, "十一")); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	accs := loadAccounts()
	if accs.ActiveID() != 11 {
		t.Errorf("active %d, want 11", accs.ActiveID())
	}
	saved := accs.Snapshot()
	if len(saved) != 1 || saved[0].Token.RefreshToken != "refresh-11" {
		t.Fatalf("saved %+v", saved)
	}
	file, err := accountsPath()
	if err != nil {
		t.Fatalf("accountsPath: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("accounts file not written: %v", err)
	}
	if _, err := os.Stat(tokenFile); err != nil {
		t.Errorf("token.json removed by the migration: %v", err)
	}
}

// TestLoginFinishUpsertsAccount checks a finished sign-in saves its account
// and makes it active.
func TestLoginFinishUpsertsAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	tok := tokenFor(21, "二十一")
	l := &loginWindow{app: a}
	l.finish(tok, "保存失败")
	if a.session == nil || int64(a.session.User.ID) != 21 {
		t.Fatalf("session %+v, want user 21", a.session)
	}
	if a.accounts.ActiveID() != 21 {
		t.Errorf("active %d, want 21", a.accounts.ActiveID())
	}
	saved := a.accounts.Snapshot()
	if len(saved) != 1 || saved[0].Token.RefreshToken != "refresh-21" {
		t.Fatalf("saved %+v", saved)
	}
	path, err := accountsPath()
	if err != nil {
		t.Fatalf("accountsPath: %v", err)
	}
	onDisk := &Accounts{}
	if err := onDisk.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if onDisk.ActiveID() != 21 {
		t.Errorf("active on disk %d, want 21", onDisk.ActiveID())
	}
}

// TestSwitchAccountUpdatesSession checks a switch adopts the account's token
// everywhere and drops the pages of the account before it.
func TestSwitchAccountUpdatesSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	first, second := tokenFor(31, "三十一"), tokenFor(32, "三十二")
	a.accounts.Upsert(first, time.Now())
	a.applySession(first)
	a.accounts.Upsert(second, time.Now())
	a.accounts.Upsert(first, time.Now())
	a.bookmarks = &browseState{}
	oldBookmarks := a.bookmarks
	a.route, a.userView = routeUser, &userViewState{}

	if err := a.activateToken(context.Background(), second); err != nil {
		t.Fatalf("activateToken: %v", err)
	}
	if a.session == nil || int64(a.session.User.ID) != 32 {
		t.Fatalf("session %+v, want user 32", a.session)
	}
	if tok := a.client.Token(); tok == nil || int64(tok.User.ID) != 32 {
		t.Errorf("client token %+v, want user 32", tok)
	}
	if a.accounts.ActiveID() != 32 {
		t.Errorf("active %d, want 32", a.accounts.ActiveID())
	}
	if a.bookmarks == oldBookmarks || a.userView != nil || a.route != routeHome {
		t.Errorf("pages kept: route %q, user view %v", a.route, a.userView != nil)
	}
	path, err := tokenPath()
	if err != nil {
		t.Fatalf("tokenPath: %v", err)
	}
	tok, err := pixiv.LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if int64(tok.User.ID) != 32 {
		t.Errorf("token.json user %d, want 32", tok.User.ID)
	}
}

// TestSwitchAccountKeepsCurrentOnRefreshFailure checks a failed refresh leaves
// the account in use and its token file alone.
func TestSwitchAccountKeepsCurrentOnRefreshFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	current, expired := tokenFor(41, "四十一"), tokenFor(42, "四十二")
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	a.accounts.Upsert(current, time.Now())
	a.applySession(current)
	path, err := tokenPath()
	if err != nil {
		t.Fatalf("tokenPath: %v", err)
	}
	if err := pixiv.SaveToken(path, current); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	a.accounts.Upsert(expired, time.Now())
	a.accounts.Upsert(current, time.Now())
	a.refreshTokenFn = func(context.Context, string) (*pixiv.Token, error) {
		return nil, errors.New("refresh failed")
	}

	if err := a.activateToken(context.Background(), expired); err == nil {
		t.Fatal("activateToken with a failed refresh returned no error")
	}
	if a.session == nil || int64(a.session.User.ID) != 41 {
		t.Fatalf("session %+v, want user 41 kept", a.session)
	}
	if tok := a.client.Token(); tok == nil || int64(tok.User.ID) != 41 {
		t.Errorf("client token %+v, want user 41", tok)
	}
	if a.accounts.ActiveID() != 41 {
		t.Errorf("active %d, want 41", a.accounts.ActiveID())
	}
	tok, err := pixiv.LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if int64(tok.User.ID) != 41 {
		t.Errorf("token.json user %d, want 41", tok.User.ID)
	}
}

// TestAccountMenuListsSavedAccounts checks the account pill opens a menu of
// the saved accounts, with the way to add one and to sign out.
func TestAccountMenuListsSavedAccounts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	first, second := tokenFor(51, "五十一"), tokenFor(52, "五十二")
	a.accounts.Upsert(second, time.Now())
	a.accounts.Upsert(first, time.Now())
	a.applySession(first)

	tt := ui.NewTester(a.View, 1280, 900)
	if err := tt.Click("五十一"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	menu := tt.Menu()
	for _, want := range []string{"五十一", "五十二", a.t("account.add"), a.t("nav.logout")} {
		if !slices.Contains(menu, want) {
			t.Errorf("menu %q lacks %q", menu, want)
		}
	}
}
