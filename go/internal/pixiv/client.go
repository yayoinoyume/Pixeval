package pixiv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const imageReferer = "https://www.pixiv.net"

// Client talks to the Pixiv app API. Requests are serialized with a minimum
// interval, and a 429 pauses every request until Retry-After.
type Client struct {
	baseURL  string
	hc       *http.Client
	mu       sync.Mutex
	last     time.Time
	cooldown time.Duration
	token    *Token
	onLimit  func(retryAfter time.Duration)
	backoff  []time.Duration

	// proxy is the Proxy function the transport asks for every request. It is
	// swapped atomically, so changing the proxy never writes a transport field
	// while requests are in flight.
	proxy atomic.Value

	// AllowR18 asks Pixiv for every work instead of filtering R18 out. It is
	// set through SetAllowR18 and read through filterParam, both under mu.
	AllowR18 bool
}

func NewClient() *Client {
	c := &Client{
		baseURL:  appBaseURL,
		cooldown: 700 * time.Millisecond,
		backoff:  []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second},
	}
	c.proxy.Store(http.ProxyFromEnvironment)
	c.hc = &http.Client{
		Timeout: 100 * time.Second,
		// A feed loads six covers at once; the default pool of two idle
		// connections per host would redo TLS for the rest of every batch.
		Transport: &http.Transport{
			Proxy:                 c.proxyFor,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return c
}

// proxyFor asks the proxy function SetProxy last stored.
func (c *Client) proxyFor(req *http.Request) (*url.URL, error) {
	return c.proxy.Load().(func(*http.Request) (*url.URL, error))(req)
}

// HTTP returns the shared HTTP client, for image downloads and the web login.
func (c *Client) HTTP() *http.Client { return c.hc }

// SetProxy makes every request go through raw, or through the environment's
// proxy again when raw is empty. A URL the transport cannot use is rejected
// and the current proxy stays.
func (c *Client) SetProxy(raw string) error {
	proxy := http.ProxyFromEnvironment
	if raw = strings.TrimSpace(raw); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("proxy: %w", err)
		}
		switch u.Scheme {
		case "http", "https", "socks5":
		default:
			return fmt.Errorf("proxy: unsupported scheme %q", u.Scheme)
		}
		if u.Host == "" {
			return errors.New("proxy: missing host")
		}
		proxy = http.ProxyURL(u)
	}
	c.proxy.Store(proxy)
	return nil
}

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

// SetAllowR18 turns the R18 filter off, so Pixiv returns every work.
func (c *Client) SetAllowR18(allow bool) {
	c.mu.Lock()
	c.AllowR18 = allow
	c.mu.Unlock()
}

// filterParam is the filter the app API wants for R18 works, and empty when
// the user allows them.
func (c *Client) filterParam() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AllowR18 {
		return ""
	}
	return "for_android"
}

// setFilter adds the R18 filter to q unless the user allows R18 works.
func (c *Client) setFilter(q url.Values) {
	if f := c.filterParam(); f != "" {
		q.Set("filter", f)
	}
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
	return c.do(ctx, http.MethodGet, rawURL, "", nil, out)
}

// post performs a rate limited form POST with the same pacing, auth and
// decoding as get. Only a 429 is retried: a POST may already have reached
// Pixiv, so repeating it could duplicate a comment or a bookmark. out may be
// nil when the response body is not needed.
func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	const contentType = "application/x-www-form-urlencoded"
	return c.do(ctx, http.MethodPost, c.baseURL+path, contentType, []byte(form.Encode()), out)
}

// rawText asks do for the undecoded body, for endpoints that answer a page
// instead of JSON.
type rawText struct{ text *string }

func (c *Client) do(ctx context.Context, method, rawURL, contentType string, payload []byte, out any) error {
	for attempt := 0; ; attempt++ {
		retry, err := c.doOnce(ctx, method, rawURL, contentType, payload, out)
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

func (c *Client) doOnce(ctx context.Context, method, rawURL, contentType string, payload []byte, out any) (retry bool, err error) {
	if err = c.wait(ctx); err != nil {
		return false, err
	}
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reqBody)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != nil && tok.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", appUserAgent)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.hc.Do(req)
	if err != nil {
		// A failed POST may have been sent already, so it is not retried.
		return method == http.MethodGet, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return method == http.MethodGet, err
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		d := retryAfter(resp)
		c.pause(d)
		return true, fmt.Errorf("pixiv: rate limited for %s", d)
	case resp.StatusCode == http.StatusUnauthorized:
		return false, errors.New("pixiv: unauthorized (token expired or revoked)")
	case resp.StatusCode == http.StatusForbidden:
		return false, ErrForbidden
	case resp.StatusCode == http.StatusNotFound:
		return false, ErrNotFound
	case resp.StatusCode == http.StatusBadRequest && method == http.MethodGet && isNextURLExhausted(body):
		return false, ErrNoMore
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooEarly:
		return method == http.MethodGet, fmt.Errorf("pixiv: %s: %s", resp.Status, truncate(string(body), 200))
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("pixiv: %s: %s", resp.Status, truncate(string(body), 200))
	}
	if out == nil {
		return false, nil
	}
	if raw, ok := out.(*rawText); ok {
		*raw.text = string(body)
		return false, nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("pixiv: decode: %w", err)
	}
	return false, nil
}

// ErrNoMore means the list has no more pages.
var ErrNoMore = errors.New("pixiv: no more pages")

// ErrForbidden means the token may not perform the request (403).
var ErrForbidden = errors.New("pixiv: forbidden")

// ErrNotFound means the work, user or comment does not exist (404).
var ErrNotFound = errors.New("pixiv: not found")

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

// PageURL builds an app API URL for the default host.
func PageURL(path string, q url.Values) string {
	return apiURL(appBaseURL, path, q)
}

// pageURL builds an app API URL against this client's base, so a test server
// can stand in for the real host.
func (c *Client) pageURL(path string, q url.Values) string {
	return apiURL(c.baseURL, path, q)
}

func apiURL(base, path string, q url.Values) string {
	return base + path + "?" + q.Encode()
}
