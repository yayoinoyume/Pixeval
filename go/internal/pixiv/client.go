package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const imageReferer = "https://www.pixiv.net"

// Client talks to the Pixiv app API. Requests are serialized with a minimum
// interval, and a 429 pauses every request until Retry-After.
type Client struct {
	hc       *http.Client
	mu       sync.Mutex
	last     time.Time
	cooldown time.Duration
	token    *Token
	onLimit  func(retryAfter time.Duration)
	backoff  []time.Duration
}

func NewClient() *Client {
	return &Client{
		hc:       &http.Client{Timeout: 100 * time.Second},
		cooldown: 700 * time.Millisecond,
		backoff:  []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second},
	}
}

// HTTP returns the shared HTTP client, for image downloads and the web login.
func (c *Client) HTTP() *http.Client { return c.hc }

// SetToken replaces the session token.
func (c *Client) SetToken(t *Token) {
	c.mu.Lock()
	c.token = t
	c.mu.Unlock()
}

// Token returns the current token, or nil.
func (c *Client) Token() *Token {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// SetCooldown sets the minimum interval between app API requests.
func (c *Client) SetCooldown(d time.Duration) {
	c.mu.Lock()
	c.cooldown = d
	c.mu.Unlock()
}

// OnRateLimit registers a callback for 429 responses.
func (c *Client) OnRateLimit(f func(retryAfter time.Duration)) {
	c.mu.Lock()
	c.onLimit = f
	c.mu.Unlock()
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	last, cd := c.last, c.cooldown
	c.mu.Unlock()
	if d := time.Until(last.Add(cd)); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *Client) pause(d time.Duration) {
	c.mu.Lock()
	if until := time.Now().Add(d); until.After(c.last) {
		c.last = until
	}
	cb := c.onLimit
	c.mu.Unlock()
	if cb != nil {
		cb(d)
	}
}

// get performs a rate limited GET with retries and JSON decoding.
func (c *Client) get(ctx context.Context, rawURL string, out any) error {
	for attempt := 0; ; attempt++ {
		retry, err := c.getOnce(ctx, rawURL, out)
		if !retry || attempt >= len(c.backoff) {
			return err
		}
		t := time.NewTimer(c.backoff[attempt])
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (c *Client) getOnce(ctx context.Context, rawURL string, out any) (retry bool, err error) {
	if err = c.wait(ctx); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != nil && tok.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	req.Header.Set("User-Agent", appUserAgent)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.hc.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return true, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		d := retryAfter(resp)
		c.pause(d)
		return true, fmt.Errorf("pixiv: rate limited for %s", d)
	case resp.StatusCode == http.StatusUnauthorized:
		return false, errors.New("pixiv: unauthorized (token expired or revoked)")
	case resp.StatusCode == http.StatusBadRequest && isNextURLExhausted(body):
		return false, ErrNoMore
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooEarly:
		return true, fmt.Errorf("pixiv: %s: %s", resp.Status, truncate(string(body), 200))
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("pixiv: %s: %s", resp.Status, truncate(string(body), 200))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("pixiv: decode: %w", err)
	}
	return false, nil
}

// ErrNoMore means the list has no more pages.
var ErrNoMore = errors.New("pixiv: no more pages")

func isNextURLExhausted(body []byte) bool {
	// Pixiv answers 400 when next_url runs past the last page.
	return len(body) > 0
}

func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return time.Minute
	}
	if secs, err := time.ParseDuration(v + "s"); err == nil {
		return secs
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return time.Minute
}

// GetImage downloads an illustration image with the Referer the CDN needs.
func (c *Client) GetImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", imageReferer)
	req.Header.Set("User-Agent", appUserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// PageURL builds an app API URL.
func PageURL(path string, q url.Values) string {
	return appBaseURL + path + "?" + q.Encode()
}
