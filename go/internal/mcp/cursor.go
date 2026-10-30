package mcp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// cursorTTL is how long a cursor a list tool handed out stays readable. A
// client that waits longer runs the list tool again.
const cursorTTL = 10 * time.Minute

// cursorMaxEntries bounds the store regardless of TTL: a client that calls the
// list tools in a tight loop would otherwise pin one paused list per call for
// the whole TTL. At the cap the entry that expires first is evicted.
const cursorMaxEntries = 1024

// cursorEntry is one paused work list: the next Pixiv page to fetch and the
// works of the current page the caller has not seen yet. Keeping the tail of
// the page means a limit smaller than Pixiv's page size skips nothing.
type cursorEntry struct {
	kind    string
	account int64
	nextURL string
	pending []pixiv.Illustration
	expires time.Time
}

// cursorStore keeps the cursors the list tools hand out. A token is random, so
// a client cannot guess one, and single use: more consumes it and mints a
// fresh token for the page after.
type cursorStore struct {
	mu      sync.Mutex
	entries map[string]cursorEntry
	now     func() time.Time
}

func newCursorStore() *cursorStore {
	return &cursorStore{entries: map[string]cursorEntry{}, now: time.Now}
}

// Put stores one paused list under a fresh token and returns it. Expired
// entries are dropped on the way and the store never holds more than
// cursorMaxEntries, so its memory is bounded even under a flood of cursors.
func (s *cursorStore) Put(kind string, account int64, nextURL string, pending []pixiv.Illustration) string {
	token := rand.Text()
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(token, cursorEntry{
		kind:    kind,
		account: account,
		nextURL: nextURL,
		pending: pending,
		expires: now.Add(cursorTTL),
	}, now)
	return token
}

// Restore puts a taken cursor back under the same token so a failed page fetch
// can be retried with it. The original expiry is kept, so a restore never
// extends the cursor's life; an entry that already expired is not restored.
func (s *cursorStore) Restore(token string, entry cursorEntry) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !now.Before(entry.expires) {
		return
	}
	s.putLocked(token, entry, now)
}

// putLocked stores entry under token after dropping expired entries and, when
// the store is still at the cap, the entry that expires first. Callers hold
// s.mu.
func (s *cursorStore) putLocked(token string, entry cursorEntry, now time.Time) {
	for key, old := range s.entries {
		if !now.Before(old.expires) {
			delete(s.entries, key)
		}
	}
	if _, exists := s.entries[token]; !exists && len(s.entries) >= cursorMaxEntries {
		var oldest string
		var oldestExpiry time.Time
		for key, old := range s.entries {
			if oldest == "" || old.expires.Before(oldestExpiry) {
				oldest, oldestExpiry = key, old.expires
			}
		}
		delete(s.entries, oldest)
	}
	s.entries[token] = entry
}

// Take consumes token and returns its entry. A non-empty kind must match the
// kind the cursor was made for. Every failure names its reason, so a client can
// tell a forged token from an expired one, and none of them panics.
func (s *cursorStore) Take(token string, account int64, kind string) (cursorEntry, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[token]
	if !ok {
		return cursorEntry{}, errors.New("cursor is invalid or expired; run the list tool again")
	}
	if !now.Before(entry.expires) {
		delete(s.entries, token)
		return cursorEntry{}, errors.New("cursor has expired; run the list tool again")
	}
	if entry.account != account {
		// The account changed, so the works behind the cursor are not the
		// caller's to read: drop it rather than leave it for the old account.
		delete(s.entries, token)
		return cursorEntry{}, errors.New("cursor belongs to another Pixiv account; run the list tool again")
	}
	if kind != "" && entry.kind != kind {
		return cursorEntry{}, fmt.Errorf("cursor belongs to %s results, not %s", entry.kind, kind)
	}
	delete(s.entries, token)
	return entry, nil
}
