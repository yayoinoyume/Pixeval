package app

import (
	"testing"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestAccountIDFollowsSession checks the MCP cursor binding sees the signed-in
// account, and nothing while logged out.
func TestAccountIDFollowsSession(t *testing.T) {
	a := New()
	if got := a.AccountID(); got != 0 {
		t.Errorf("logged out AccountID = %d, want 0", got)
	}
	a.client.SetToken(&pixiv.Token{User: pixiv.TokenUser{ID: 42}})
	if got := a.AccountID(); got != 42 {
		t.Errorf("AccountID = %d, want 42", got)
	}
}
