package pixiv

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tokenFile is the token on disk: the token plus when it was written, so
// ExpiresAt, which the token does not serialize, is rebuilt on load.
type tokenFile struct {
	Token   *Token    `json:"token"`
	SavedAt time.Time `json:"saved_at"`
}

// SaveToken writes t to path as JSON only the user can read. The file is
// written beside path and renamed over it, so a failed write leaves the
// previous token in place.
func SaveToken(path string, t *Token) error {
	if t == nil {
		return errors.New("pixiv auth: nil token")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tokenFile{Token: t, SavedAt: time.Now()}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// LoadToken reads a token saved by SaveToken. ExpiresAt is estimated from
// the time the file was saved and the token's lifetime.
func LoadToken(path string) (*Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f tokenFile
	if err = json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Token == nil {
		return nil, errors.New("pixiv auth: token file has no token")
	}
	f.Token.ExpiresAt = f.SavedAt.Add(time.Duration(f.Token.ExpiresIn) * time.Second)
	return f.Token, nil
}

// ParseCallback reads the authorization code of the redirect the login page
// makes, pixiv://account/login?code=... .
func ParseCallback(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		// The URL carries the code: report it without the value.
		return "", errors.New("pixiv auth: invalid callback URL")
	}
	if u.Scheme != "pixiv" || u.Host != "account" || strings.TrimSuffix(u.Path, "/") != "/login" {
		// The query carries the code: name the URL without it.
		return "", fmt.Errorf("pixiv auth: not a login callback: %s://%s%s", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("pixiv auth: %s %s", e, q.Get("error_description"))
	}
	code := q.Get("code")
	if code == "" {
		return "", errors.New("pixiv auth: callback has no code")
	}
	return code, nil
}
