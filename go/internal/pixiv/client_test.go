package pixiv

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestFrontedAddress checks the dial address comes from the list for the
// matched host, round robin, and stays untouched otherwise.
func TestFrontedAddress(t *testing.T) {
	cfg := &domainFrontingConfig{
		enabled: true,
		app:     []string{"203.0.113.1", "203.0.113.2"},
		image:   []string{"198.51.100.7"},
		oauth:   []string{"203.0.113.9"},
	}
	for _, tt := range []struct {
		name string
		host string
		n    uint64
		want string
		ok   bool
	}{
		{"app first", pixivAppAPIHost, 0, "203.0.113.1:443", true},
		{"app second", pixivAppAPIHost, 1, "203.0.113.2:443", true},
		{"app wraps", pixivAppAPIHost, 2, "203.0.113.1:443", true},
		{"image", pixivImageHost, 0, "198.51.100.7:443", true},
		{"oauth", pixivOAuthHost, 0, "203.0.113.9:443", true},
		{"case and dot", "APP-API.PIXIV.NET.", 0, "203.0.113.1:443", true},
		{"other host", "www.pixiv.net", 0, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := frontedAddress(tt.host, "443", cfg, tt.n)
			if ok != tt.ok || got != tt.want {
				t.Errorf("frontedAddress(%q, %d) = %q, %v, want %q, %v", tt.host, tt.n, got, ok, tt.want, tt.ok)
			}
		})
	}
	if _, ok := frontedAddress(pixivAppAPIHost, "443", &domainFrontingConfig{enabled: false, app: cfg.app}, 0); ok {
		t.Error("frontedAddress used a disabled config")
	}
}

// TestSetDomainFrontingDefaults checks an empty list falls back to the
// original Pixeval's IPs and invalid entries are dropped.
func TestSetDomainFrontingDefaults(t *testing.T) {
	c := NewClient()
	c.SetDomainFronting(true, nil, []string{"not-an-ip", "198.51.100.7"}, nil)
	cfg := c.fronting.Load()
	if cfg == nil || !cfg.enabled {
		t.Fatal("fronting not enabled")
	}
	if len(cfg.app) != len(DefaultAppAPIResolver()) {
		t.Errorf("app list %q, want the default %q", cfg.app, DefaultAppAPIResolver())
	}
	if len(cfg.image) != 1 || cfg.image[0] != "198.51.100.7" {
		t.Errorf("image list %q, want the valid IP only", cfg.image)
	}
	if len(cfg.oauth) != len(DefaultOAuthResolver()) {
		t.Errorf("oauth list %q, want the default %q", cfg.oauth, DefaultOAuthResolver())
	}
}

// TestImageMirror checks the mirror replaces an i.pximg.net URL's scheme and
// host while keeping the path and query, and that clearing it restores the
// original URL.
func TestImageMirror(t *testing.T) {
	c := NewClient()
	raw := "https://i.pximg.net/img-original/img/2024/01/01/00/00/00/42_p0.jpg?foo=bar"
	c.SetImageMirror("https://mirror.example/base/")
	if got, want := c.imageURL(raw), "https://mirror.example/img-original/img/2024/01/01/00/00/00/42_p0.jpg?foo=bar"; got != want {
		t.Errorf("imageURL = %q, want %q", got, want)
	}
	c.SetImageMirror("http://mirror.example:8080")
	if got, want := c.imageURL(raw), "http://mirror.example:8080/img-original/img/2024/01/01/00/00/00/42_p0.jpg?foo=bar"; got != want {
		t.Errorf("imageURL = %q, want %q", got, want)
	}
	if got := c.imageURL("https://example.invalid/42.jpg"); got != "https://example.invalid/42.jpg" {
		t.Errorf("non Pixiv image rewritten to %q", got)
	}
	c.SetImageMirror("")
	if got := c.imageURL(raw); got != raw {
		t.Errorf("cleared mirror imageURL = %q, want %q", got, raw)
	}
	c.SetImageMirror("ftp://mirror.example")
	if got := c.imageURL(raw); got != raw {
		t.Errorf("invalid mirror imageURL = %q, want %q", got, raw)
	}
}

// TestImageMirrorUsesTransport checks GetImage asks the transport for the
// rewritten URL, without reaching the network.
func TestImageMirrorUsesTransport(t *testing.T) {
	c := NewClient()
	c.SetImageMirror("https://mirror.example")
	var got string
	c.hc.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.URL.String()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
	})
	if _, err := c.GetImage(t.Context(), "https://i.pximg.net/a.jpg?q=1"); err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if want := "https://mirror.example/a.jpg?q=1"; got != want {
		t.Errorf("request URL = %q, want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// TestClientDialAddress checks the transport's dialer connects to the fronted
// IP for a Pixiv host. The listener is local, so no real network is reached.
func TestClientDialAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	accepted := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
		accepted <- err
	}()

	c := NewClient()
	c.SetDomainFronting(true, []string{"127.0.0.1"}, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := c.dialContext(ctx, "tcp", net.JoinHostPort(pixivAppAPIHost, port))
	if err != nil {
		t.Fatalf("dialContext: %v", err)
	}
	conn.Close()
	if err := <-accepted; err != nil {
		t.Fatalf("Accept: %v", err)
	}
}
func TestClientTransportPool(t *testing.T) {
	tr, ok := NewClient().HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if tr.MaxIdleConnsPerHost < 6 {
		t.Errorf("MaxIdleConnsPerHost = %d, want at least 6", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxIdleConns < 12 {
		t.Errorf("MaxIdleConns = %d, want at least 12", tr.MaxIdleConns)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 = false, want true")
	}
	if tr.Proxy == nil {
		t.Error("Proxy is nil, want the environment proxy")
	}
}

// TestSetProxy checks the shared client sends requests through the configured
// proxy and falls back to the environment's when it is cleared.
func TestSetProxy(t *testing.T) {
	c := NewClient()
	tr, ok := c.HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.SetProxy("socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	got, err := tr.Proxy(req)
	if err != nil || got == nil || got.String() != "socks5://127.0.0.1:1080" {
		t.Errorf("proxy = %v, %v, want socks5://127.0.0.1:1080", got, err)
	}

	if err := c.SetProxy(""); err != nil {
		t.Fatalf("SetProxy empty: %v", err)
	}
	got, err = tr.Proxy(req)
	want, wantErr := http.ProxyFromEnvironment(req)
	if (err != nil) != (wantErr != nil) || (got == nil) != (want == nil) ||
		(got != nil && want != nil && got.String() != want.String()) {
		t.Errorf("proxy = %v, %v, want the environment's %v, %v", got, err, want, wantErr)
	}
}

// TestSetProxyRejectsInvalid checks a URL the transport cannot dial is
// reported and leaves the current proxy in place.
func TestSetProxyRejectsInvalid(t *testing.T) {
	c := NewClient()
	tr := c.HTTP().Transport.(*http.Transport)
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	for _, raw := range []string{"ftp://127.0.0.1:21", "http://", "://bad"} {
		if err := c.SetProxy(raw); err == nil {
			t.Errorf("SetProxy(%q) accepted an invalid proxy", raw)
		}
		got, err := tr.Proxy(req)
		if err != nil || got == nil || got.String() != "http://127.0.0.1:7890" {
			t.Errorf("proxy after SetProxy(%q) = %v, %v, want the previous one", raw, got, err)
		}
	}
}

// TestSetProxyConcurrent checks switching the proxy while requests ask for it
// (the same call the transport makes per request) stays race free.
func TestSetProxyConcurrent(t *testing.T) {
	c := NewClient()
	tr := c.HTTP().Transport.(*http.Transport)
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net", nil)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := tr.Proxy(req); err != nil {
					t.Errorf("Proxy: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		if err := c.SetProxy("http://127.0.0.1:7890"); err != nil {
			t.Errorf("SetProxy: %v", err)
		}
		if err := c.SetProxy(""); err != nil {
			t.Errorf("SetProxy empty: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}
