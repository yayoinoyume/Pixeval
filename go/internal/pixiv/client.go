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

// The hosts domain fronting can redirect, and the IP lists the original
// Pixeval ships with.
const (
	pixivAppAPIHost = "app-api.pixiv.net"
	pixivImageHost  = "i.pximg.net"
	pixivOAuthHost  = "oauth.secure.pixiv.net"
)

// DefaultAppAPIResolver is the app API IP list used when the setting is empty.
func DefaultAppAPIResolver() []string { return []string{"104.18.42.239", "172.64.145.17"} }

// DefaultImageResolver is the image IP list used when the setting is empty.
func DefaultImageResolver() []string {
	return []string{"210.140.139.134", "210.140.139.135", "210.140.139.136", "210.140.139.137"}
}

// DefaultOAuthResolver is the OAuth IP list used when the setting is empty.
func DefaultOAuthResolver() []string { return []string{"104.18.42.239", "172.64.145.17"} }

// domainFrontingConfig is one immutable fronting setting plus the round robin
// position. SetDomainFronting replaces the whole pointer, so a request never
// reads a half changed list.
type domainFrontingConfig struct {
	enabled bool
	app     []string
	image   []string
	oauth   []string
	next    atomic.Uint64
}

// hostIPs returns the IP list for host, or nil when fronting leaves it alone.
func (c *domainFrontingConfig) hostIPs(host string) []string {
	switch strings.ToLower(strings.TrimSuffix(host, ".")) {
	case pixivAppAPIHost:
		return c.app
	case pixivImageHost:
		return c.image
	case pixivOAuthHost:
		return c.oauth
	default:
		return nil
	}
}

// frontedAddress picks the address to dial for host:port, or false when the
// host is not fronted. n selects the list entry, so the caller can round robin
// and a test can ask for a position without a network.
func frontedAddress(host, port string, cfg *domainFrontingConfig, n uint64) (string, bool) {
	if cfg == nil || !cfg.enabled {
		return "", false
	}
	ips := cfg.hostIPs(host)
	if len(ips) == 0 {
		return "", false
	}
	return net.JoinHostPort(ips[n%uint64(len(ips))], port), true
}

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

	// dialer is the transport's dialer. fronting is the domain fronting setting
	// it consults; mirror is the image mirror's scheme and host, empty when off.
	dialer   *net.Dialer
	fronting atomic.Pointer[domainFrontingConfig]
	mirror   atomic.Value

	// AllowR18 asks Pixiv for every work instead of filtering R18 out. It is
	// set through SetAllowR18 and read through filterParam, both under mu.
	AllowR18 bool
}

func NewClient() *Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	c := &Client{
		baseURL:  appBaseURL,
		cooldown: 700 * time.Millisecond,
		backoff:  []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second},
		dialer:   dialer,
	}
	c.proxy.Store(http.ProxyFromEnvironment)
	c.mirror.Store("")
	c.hc = &http.Client{
		Timeout: 100 * time.Second,
		// A feed loads six covers at once; the default pool of two idle
		// connections per host would redo TLS for the rest of every batch.
		Transport: &http.Transport{
			Proxy:                 c.proxyFor,
			DialContext:           c.dialContext,
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

// SetDomainFronting makes the shared transport dial Pixiv's app API, image and
// OAuth hosts at the given IPs, round robin, while Host and TLS SNI stay the
// real names. This is IP resolution, not TLS fragmentation. A proxy set with
// SetProxy wins: the transport then dials the proxy, so no fronted address is
// used. Empty lists fall back to the defaults the original Pixeval ships.
func (c *Client) SetDomainFronting(enabled bool, appIPs, imageIPs, oauthIPs []string) {
	c.fronting.Store(&domainFrontingConfig{
		enabled: enabled,
		app:     resolverIPs(appIPs, DefaultAppAPIResolver()),
		image:   resolverIPs(imageIPs, DefaultImageResolver()),
		oauth:   resolverIPs(oauthIPs, DefaultOAuthResolver()),
	})
}

// resolverIPs keeps the valid IPs and their order, and falls back to fallback
// when none are left.
func resolverIPs(ips, fallback []string) []string {
	out := make([]string, 0, len(ips))
	seen := make(map[string]bool, len(ips))
	for _, raw := range ips {
		raw = strings.TrimSpace(raw)
		if net.ParseIP(raw) == nil || seen[raw] {
			continue
		}
		seen[raw] = true
		out = append(out, raw)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

// dialContext dials the fronted address when the setting covers the host, and
// the original address otherwise. The transport keeps the request's host for
// the Host header and TLS SNI.
func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialAddr := addr
	if cfg := c.fronting.Load(); cfg != nil {
		if host, port, err := net.SplitHostPort(addr); err == nil {
			if fronted, ok := frontedAddress(host, port, cfg, cfg.next.Add(1)-1); ok {
				dialAddr = fronted
			}
		}
	}
	return c.dialer.DialContext(ctx, network, dialAddr)
}

// SetImageMirror makes GetImage fetch i.pximg.net images from the mirror's
// scheme and host, keeping the path and query. Empty, or a URL the app cannot
// use, turns the mirror off.
func (c *Client) SetImageMirror(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		c.mirror.Store("")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		c.mirror.Store("")
		return
	}
	c.mirror.Store(u.Scheme + "://" + u.Host)
}

// imageURL applies the image mirror to an i.pximg.net URL.
func (c *Client) imageURL(raw string) string {
	base, _ := c.mirror.Load().(string)
	if base == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), pixivImageHost) {
		return raw
	}
	m, err := url.Parse(base)
	if err != nil || m.Host == "" {
		return raw
	}
	u.Scheme = m.Scheme
	u.Host = m.Host
	return u.String()
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
	imageURL = c.imageURL(imageURL)
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
