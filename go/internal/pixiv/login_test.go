package pixiv

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestParseCallback(t *testing.T) {
	code, err := ParseCallback("pixiv://account/login?code=abc123&via=login")
	if err != nil {
		t.Fatalf("ParseCallback: %v", err)
	}
	if code != "abc123" {
		t.Errorf("code = %q, want abc123", code)
	}

	for _, u := range []string{
		"pixiv://account/login",
		"pixiv://account/login?error=access_denied&error_description=denied",
		"https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=abc123",
		"pixiv://account/other?code=abc123",
		"://not a url",
	} {
		if _, err = ParseCallback(u); err == nil {
			t.Errorf("ParseCallback(%q) = nil error, want error", u)
		}
	}

	// Errors must not carry the code back into a log or a dialog.
	if _, err = ParseCallback("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=secret-code"); err == nil {
		t.Error("ParseCallback of a foreign URL = nil error")
	} else if strings.Contains(err.Error(), "secret-code") {
		t.Errorf("error echoes the code: %v", err)
	}
}

func TestSaveLoadToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	saved := &Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    3600,
		TokenType:    "bearer",
		User:         TokenUser{ID: 42, Name: "name"},
	}
	if err := SaveToken(path, saved); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
	if _, err = os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}

	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != saved.AccessToken || loaded.RefreshToken != saved.RefreshToken {
		t.Errorf("loaded token = %+v", loaded)
	}
	if loaded.User.ID != 42 || loaded.User.Name != "name" {
		t.Errorf("loaded user = %+v", loaded.User)
	}
	if loaded.ExpiresAt.Before(time.Now().Add(50 * time.Minute)) {
		t.Errorf("ExpiresAt = %v, want about an hour from now", loaded.ExpiresAt)
	}
	if loaded.Expired() {
		t.Errorf("fresh token reports expired")
	}
}

func TestLoadTokenRejectsMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadToken(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("LoadToken of a missing file = nil error")
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"saved_at":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(empty); err == nil {
		t.Error("LoadToken without a token = nil error")
	}
}

func TestSaveTokenOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := SaveToken(path, &Token{AccessToken: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(path, &Token{AccessToken: "new"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f tokenFile
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if f.Token.AccessToken != "new" {
		t.Errorf("access token = %q, want new", f.Token.AccessToken)
	}
	if err = SaveToken(path, nil); err == nil {
		t.Error("SaveToken(nil) = nil error")
	}
}

func TestGeneratePKCE(t *testing.T) {
	charset := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	// 32 random bytes in base64url without padding.
	for name, s := range map[string]string{"verifier": verifier, "challenge": challenge} {
		if len(s) != 43 {
			t.Errorf("%s length = %d, want 43", name, len(s))
		}
		if !charset.MatchString(s) {
			t.Errorf("%s = %q, want base64url characters", name, s)
		}
	}
	if verifier == challenge {
		t.Error("verifier equals challenge")
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Errorf("challenge = %q, want S256 %q", challenge, want)
	}
	other, _, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if other == verifier {
		t.Error("two verifiers are equal")
	}
}

func TestTokenExpired(t *testing.T) {
	cases := []struct {
		name  string
		token Token
		want  bool
	}{
		{"no access token", Token{ExpiresAt: time.Now().Add(time.Hour)}, true},
		{"fresh", Token{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}, false},
		{"about to expire", Token{AccessToken: "a", ExpiresAt: time.Now().Add(2 * time.Minute)}, true},
		{"expired", Token{AccessToken: "a", ExpiresAt: time.Now().Add(-time.Minute)}, true},
	}
	for _, c := range cases {
		if got := c.token.Expired(); got != c.want {
			t.Errorf("%s: Expired() = %v, want %v", c.name, got, c.want)
		}
	}
}
