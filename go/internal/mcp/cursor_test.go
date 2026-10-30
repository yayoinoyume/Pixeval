package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestCursorPutTake checks a cursor is random, carries its page, and is single
// use.
func TestCursorPutTake(t *testing.T) {
	s := newCursorStore()
	pending := []pixiv.Illustration{{ID: 2}}
	token := s.Put("search_illustrations", 7, "https://api/next?offset=2", pending)
	if len(token) < 20 {
		t.Fatalf("token %q is too short to be unguessable", token)
	}
	if other := s.Put("search_illustrations", 7, "", nil); other == token {
		t.Fatal("two cursors share a token")
	}
	entry, err := s.Take(token, 7, "search_illustrations")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if entry.nextURL != "https://api/next?offset=2" || len(entry.pending) != 1 || entry.pending[0].ID != 2 {
		t.Errorf("entry %+v", entry)
	}
	if _, err := s.Take(token, 7, "search_illustrations"); err == nil {
		t.Error("a consumed cursor was read twice")
	}
}

// TestCursorExpires checks a cursor stops working after its TTL.
func TestCursorExpires(t *testing.T) {
	s := newCursorStore()
	now := time.Now()
	s.now = func() time.Time { return now }
	token := s.Put("ranking", 0, "https://api/next", nil)
	now = now.Add(cursorTTL + time.Second)
	_, err := s.Take(token, 0, "ranking")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Take after TTL = %v, want an expired error", err)
	}
}

// TestCursorRejectsAnotherAccount checks a cursor cannot be read after the
// Pixiv account changes, and is dropped rather than left for the old account.
func TestCursorRejectsAnotherAccount(t *testing.T) {
	s := newCursorStore()
	token := s.Put("user_bookmarks", 7, "https://api/next", nil)
	_, err := s.Take(token, 8, "user_bookmarks")
	if err == nil || !strings.Contains(err.Error(), "another Pixiv account") {
		t.Fatalf("Take for another account = %v", err)
	}
	if _, err := s.Take(token, 7, "user_bookmarks"); err == nil {
		t.Error("a cursor rejected for another account stayed usable")
	}
}

// TestCursorRejectsAnotherKind checks a cursor names its tool, and a mismatched
// read leaves the cursor intact.
func TestCursorRejectsAnotherKind(t *testing.T) {
	s := newCursorStore()
	token := s.Put("search_illustrations", 0, "https://api/next", nil)
	_, err := s.Take(token, 0, "ranking")
	if err == nil || !strings.Contains(err.Error(), "search_illustrations results, not ranking") {
		t.Fatalf("Take as ranking = %v", err)
	}
	if _, err := s.Take(token, 0, "search_illustrations"); err != nil {
		t.Errorf("Take after a kind mismatch: %v", err)
	}
}

// TestCursorRejectsForgedToken checks a token the server never made is refused.
func TestCursorRejectsForgedToken(t *testing.T) {
	s := newCursorStore()
	if _, err := s.Take("pixeval:forged", 0, ""); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("Take forged = %v", err)
	}
}
