package pixiv

import (
	"net/http"
	"sync"
	"testing"
)

// TestClientTransportPool checks the shared client can keep a whole cover
// batch's connections idle instead of redoing TLS for every one of them.
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
