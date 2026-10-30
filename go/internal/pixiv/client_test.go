package pixiv

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
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
	c.SetDomainFronting(true, DomainFrontingFragmentation, nil, []string{"not-an-ip", "198.51.100.7"}, nil)
	cfg := c.fronting.Load()
	if cfg == nil || !cfg.enabled {
		t.Fatal("fronting not enabled")
	}
	if cfg.mode != DomainFrontingFragmentation {
		t.Errorf("mode %q, want %q", cfg.mode, DomainFrontingFragmentation)
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

// TestNormalizeDomainFrontingType checks the saved mode picks one the client
// implements, and anything else falls back to the original default.
func TestNormalizeDomainFrontingType(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", DomainFrontingFragmentation},
		{"fragmentation", DomainFrontingFragmentation},
		{" Fragmentation ", DomainFrontingFragmentation},
		{"resolver", DomainFrontingResolver},
		{"RESOLVER", DomainFrontingResolver},
		{"desync", DomainFrontingFragmentation},
	} {
		if got := NormalizeDomainFrontingType(tt.in); got != tt.want {
			t.Errorf("NormalizeDomainFrontingType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSetDomainFrontingMode checks the mode reaches the config, normalized.
func TestSetDomainFrontingMode(t *testing.T) {
	c := NewClient()
	c.SetDomainFronting(true, DomainFrontingResolver, nil, nil, nil)
	if got := c.fronting.Load().mode; got != DomainFrontingResolver {
		t.Errorf("mode %q, want %q", got, DomainFrontingResolver)
	}
	c.SetDomainFronting(true, "not-a-mode", nil, nil, nil)
	if got := c.fronting.Load().mode; got != DomainFrontingFragmentation {
		t.Errorf("mode %q, want %q", got, DomainFrontingFragmentation)
	}
}

// recordingConn records each Write as its own segment.
type recordingConn struct {
	net.Conn
	writes [][]byte
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

// TestFragmentConnSplitsFirstWrite checks the first write leaves in more than
// one segment with every byte intact, and later writes are not split.
func TestFragmentConnSplitsFirstWrite(t *testing.T) {
	rec := &recordingConn{}
	c := &fragmentConn{Conn: rec}
	first := []byte("the client hello")
	n, err := c.Write(first)
	if err != nil || n != len(first) {
		t.Fatalf("Write = %d, %v, want %d, nil", n, err, len(first))
	}
	if len(rec.writes) < 2 {
		t.Fatalf("first write made %d segments, want at least 2", len(rec.writes))
	}
	if got := bytes.Join(rec.writes, nil); !bytes.Equal(got, first) {
		t.Errorf("segments carry %q, want %q", got, first)
	}
	second := []byte("application data")
	if _, err := c.Write(second); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if len(rec.writes) != 3 {
		t.Fatalf("later write made %d segments, want 1", len(rec.writes)-2)
	}
	if got := rec.writes[2]; !bytes.Equal(got, second) {
		t.Errorf("later segment carries %q, want %q", got, second)
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
	c.SetDomainFronting(true, DomainFrontingResolver, []string{"127.0.0.1"}, nil, nil)
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

// TestClientDialTLSFronted checks a fronted TLS dial reaches the resolver IP
// and returns a TLS conn that keeps the real host name. The listener is local,
// so no real network is reached.
func TestClientDialTLSFronted(t *testing.T) {
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
	c.SetDomainFronting(true, DomainFrontingFragmentation, []string{"127.0.0.1"}, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := c.dialTLSContext(ctx, "tcp", net.JoinHostPort(pixivAppAPIHost, port))
	if err != nil {
		t.Fatalf("dialTLSContext: %v", err)
	}
	if _, ok := conn.(*tls.Conn); !ok {
		t.Errorf("conn is %T, want *tls.Conn", conn)
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
	if tr.DialTLSContext == nil {
		t.Error("DialTLSContext is nil, want the domain fronting TLS dialer")
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

// TestIsNextURLExhausted checks only Pixiv's offset limit reads as the end of
// the list; any other 400 body stays an ordinary error.
func TestIsNextURLExhausted(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want bool
	}{
		{"html", "<html><body>Bad Request</body></html>", false},
		{"empty", "", false},
		{"plain offset limit", `{"error":{"message":"offset must be no more than 5000"}}`, true},
		{"nested offset limit", `{"error":{"message":"{\"offset\":[\"offset must be no more than 5000\"]}"}}`, true},
		{"other message", `{"error":{"message":"invalid request"}}`, false},
		{"other nested message", `{"error":{"message":"{\"offset\":[\"offset must be a number\"]}"}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNextURLExhausted([]byte(tt.body)); got != tt.want {
				t.Errorf("isNextURLExhausted(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestBadRequestStatusClassification checks a 400 only becomes ErrNoMore for
// the offset limit; other bodies keep the status and body in the error.
func TestBadRequestStatusClassification(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want error
	}{
		{"offset limit", `{"error":{"message":"{\"offset\":[\"offset must be no more than 5000\"]}"}}`, ErrNoMore},
		{"html", "<html>bad request</html>", nil},
		{"empty", "", nil},
		{"other message", `{"error":{"message":"invalid request"}}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, tt.body)
			})
			_, err := c.Recommended(context.Background())
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("Recommended: err = %v, want %v", err, tt.want)
				}
				return
			}
			if err == nil || errors.Is(err, ErrNoMore) {
				t.Fatalf("Recommended: err = %v, want an ordinary 400 error", err)
			}
			if !strings.Contains(err.Error(), "400 Bad Request") {
				t.Errorf("err = %q, want the status", err)
			}
			if tt.body != "" && !strings.Contains(err.Error(), tt.body) {
				t.Errorf("err = %q, want the body", err)
			}
		})
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
