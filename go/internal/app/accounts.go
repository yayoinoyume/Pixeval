package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// accountLimit caps the saved accounts, so the file cannot grow without bound.
const accountLimit = 10

// Account is one saved sign-in. Token keeps its credentials, so a switch does
// not ask for the password again.
type Account struct {
	UserID   int64       `json:"userId"`
	Name     string      `json:"name"`
	Account  string      `json:"account"`
	Token    pixiv.Token `json:"token"`
	LastUsed time.Time   `json:"lastUsed"`
}

// Accounts are the saved sign-ins and the one in use. The view reads it on the
// main thread; mu guards it from the goroutine that saves it.
type Accounts struct {
	mu           sync.Mutex
	ActiveUserID int64     `json:"activeUserId"`
	Saved        []Account `json:"accounts"`
}

// accountsPath is where the saved accounts live: the user's config directory.
func accountsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "accounts.json"), nil
}

// loadAccounts reads the saved accounts. A missing file migrates the single
// token.json an earlier version kept, so the upgrade stays signed in.
func loadAccounts() *Accounts {
	accs := &Accounts{}
	path, err := accountsPath()
	if err != nil {
		log.Printf("accounts: %v", err)
		return accs
	}
	switch err := accs.Load(path); {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		accs.migrateTokenFile(path)
	default:
		log.Printf("accounts: %v", err)
	}
	return accs
}

// migrateTokenFile turns token.json into the first saved account. token.json
// stays in place: it is still the current session's file.
func (as *Accounts) migrateTokenFile(accountsFile string) {
	path, err := tokenPath()
	if err != nil {
		log.Printf("accounts: %v", err)
		return
	}
	tok, err := pixiv.LoadToken(path)
	if err != nil {
		// A missing or broken token is a logged-out start, not a migration.
		return
	}
	as.Upsert(tok, time.Now())
	if err := as.Save(accountsFile); err != nil {
		log.Printf("accounts: migrate: %v", err)
	}
}

// Load reads the accounts at path into as. It leaves out an entry without a
// user or a refresh token, which a switch could not use, and an active id that
// no saved account has.
func (as *Accounts) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		ActiveUserID int64     `json:"activeUserId"`
		Saved        []Account `json:"accounts"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	saved := make([]Account, 0, len(file.Saved))
	active := int64(0)
	for _, acc := range file.Saved {
		if acc.UserID == 0 || acc.Token.RefreshToken == "" {
			continue
		}
		saved = append(saved, acc)
		if acc.UserID == file.ActiveUserID {
			active = acc.UserID
		}
	}
	as.mu.Lock()
	as.Saved, as.ActiveUserID = saved, active
	as.mu.Unlock()
	return nil
}

// Save writes as to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one.
func (as *Accounts) Save(path string) error {
	as.mu.Lock()
	data, err := json.MarshalIndent(as, "", "  ")
	as.mu.Unlock()
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

// Snapshot is the saved accounts, newest first.
func (as *Accounts) Snapshot() []Account {
	as.mu.Lock()
	defer as.mu.Unlock()
	return append([]Account(nil), as.Saved...)
}

// ActiveID is the user id of the account in use, or 0 when none is.
func (as *Accounts) ActiveID() int64 {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.ActiveUserID
}

// Get returns the saved account for userID.
func (as *Accounts) Get(userID int64) (Account, bool) {
	as.mu.Lock()
	defer as.mu.Unlock()
	for _, acc := range as.Saved {
		if acc.UserID == userID {
			return acc, true
		}
	}
	return Account{}, false
}

// Upsert puts t's account first with a fresh token, drops an older entry for
// the same user, keeps the list at accountLimit and makes the account active.
func (as *Accounts) Upsert(t *pixiv.Token, now time.Time) {
	if t == nil || t.User.ID == 0 {
		return
	}
	userID := int64(t.User.ID)
	as.mu.Lock()
	defer as.mu.Unlock()
	saved := make([]Account, 0, len(as.Saved)+1)
	saved = append(saved, Account{
		UserID:   userID,
		Name:     t.User.Name,
		Account:  t.User.Account,
		Token:    *t,
		LastUsed: now,
	})
	for _, old := range as.Saved {
		if old.UserID != userID {
			saved = append(saved, old)
		}
	}
	if len(saved) > accountLimit {
		saved = saved[:accountLimit]
	}
	as.Saved, as.ActiveUserID = saved, userID
}

// rememberAccount adds tok's account to the saved list and makes it active.
func (a *App) rememberAccount(t *pixiv.Token) error {
	a.accounts.Upsert(t, time.Now())
	path, err := accountsPath()
	if err != nil {
		return err
	}
	return a.accounts.Save(path)
}

// errSwitchSuperseded reports a switch whose refresh finished after a newer
// switch was asked for: it must not overwrite the newer one.
var errSwitchSuperseded = errors.New("accounts: switch superseded")

// switchAccount makes the saved account userID the one in use. An expired
// token is refreshed off the main thread; a failed switch keeps the account in
// use and says so. Switches run one at a time, in the order they were asked
// for, and one whose refresh finishes late drops itself.
func (a *App) switchAccount(userID int64) {
	acc, ok := a.accounts.Get(userID)
	if !ok || a.accounts.ActiveID() == userID {
		return
	}
	tok := acc.Token
	seq := a.switchSeq.Add(1)
	go func() {
		a.switchMu.Lock()
		defer a.switchMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := a.activateToken(ctx, &tok, seq)
		switch {
		case errors.Is(err, errSwitchSuperseded):
			// A newer switch is waiting: it owns the session now.
			return
		case err != nil:
			log.Printf("accounts: switch: %v", err)
			// Only the newest request may say the switch failed.
			if a.switchSeq.Load() == seq {
				a.update(func() { a.notice = a.t("account.switchFailed") })
			}
		}
	}()
}

// activateToken makes tok the session: it renews an expired token, saves it
// as token.json, records its account and shows its pages. seq is its place in
// the switch order; a newer request leaves the session alone.
func (a *App) activateToken(ctx context.Context, tok *pixiv.Token, seq uint64) error {
	if tok.Expired() {
		fresh, err := a.refreshTokenFn(ctx, tok.RefreshToken)
		if err != nil {
			return err
		}
		tok = fresh
	}
	if a.switchSeq.Load() != seq {
		return errSwitchSuperseded
	}
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := pixiv.SaveToken(path, tok); err != nil {
		return err
	}
	if err := a.rememberAccount(tok); err != nil {
		// The token file is the session: a list that cannot be written must
		// not undo the switch.
		log.Printf("accounts: %v", err)
	}
	a.applySession(tok)
	return nil
}
