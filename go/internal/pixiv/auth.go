package pixiv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	oauthBaseURL = "https://oauth.secure.pixiv.net"
	appBaseURL   = "https://app-api.pixiv.net"
	clientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	clientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	redirectURI  = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback"
	appUserAgent = "PixivAndroidApp/6.140.2 (Android 15.0)"
)

// Token is the OAuth session. ExpiresAt is computed from the response and is
// not serialized.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"`
	TokenType    string    `json:"token_type"`
	User         TokenUser `json:"user"`
	ExpiresAt    time.Time `json:"-"`
}

type TokenUser struct {
	// ID is a string in the token response and a number elsewhere, so it takes
	// either form.
	ID               FlexInt64        `json:"id"`
	Name             string           `json:"name"`
	Account          string           `json:"account"`
	IsPremium        bool             `json:"is_premium"`
	ProfileImageUrls ProfileImageUrls `json:"profile_image_urls"`
}

// FlexInt64 reads a JSON number or a string holding one.
type FlexInt64 int64

func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("pixiv: %q is not an id: %w", s, err)
	}
	*f = FlexInt64(n)
	return nil
}

// Expired reports whether the access token should be refreshed.
func (t *Token) Expired() bool {
	return t.AccessToken == "" || time.Now().After(t.ExpiresAt.Add(-5*time.Minute))
}

// GeneratePKCE returns a verifier and its S256 challenge, both base64url
// without padding, as Pixiv expects.
func GeneratePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// LoginURL is the page the embedded web view opens.
func LoginURL(challenge string) string {
	q := url.Values{}
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("client", "pixiv-android")
	return appBaseURL + "/web/v1/login?" + q.Encode()
}

// ExchangeCode turns an authorization code into a token.
func ExchangeCode(ctx context.Context, hc *http.Client, code, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("include_policy", "true")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", redirectURI)
	return requestToken(ctx, hc, form)
}

// RefreshToken renews an expired access token.
func RefreshToken(ctx context.Context, hc *http.Client, refresh string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("include_policy", "true")
	form.Set("refresh_token", refresh)
	return requestToken(ctx, hc, form)
}

func requestToken(ctx context.Context, hc *http.Client, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthBaseURL+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", appUserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("pixiv auth: " + resp.Status + ": " + truncate(string(body), 300))
	}
	var t Token
	if err = json.Unmarshal(body, &t); err != nil {
		return nil, err
	}
	if t.AccessToken == "" {
		return nil, errors.New("pixiv auth: empty access token")
	}
	t.ExpiresAt = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	return &t, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
