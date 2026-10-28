package pixiv

import (
	"net/http"
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
