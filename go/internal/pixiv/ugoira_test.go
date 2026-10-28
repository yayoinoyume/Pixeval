package pixiv

import (
	"context"
	"net/http"
	"testing"
)

func TestUgoiraMetadata(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"ugoira_metadata":{
		"zip_urls":{"medium":"https://i.pximg.net/ugoira/123.zip"},
		"frames":[{"file":"000.jpg","delay":80},{"file":"001.jpg","delay":120}]}}`))

	m, err := c.UgoiraMetadata(context.Background(), 123)
	if err != nil {
		t.Fatalf("UgoiraMetadata: %v", err)
	}
	if m.ZipURL != "https://i.pximg.net/ugoira/123.zip" {
		t.Errorf("ZipURL = %q", m.ZipURL)
	}
	if len(m.Frames) != 2 || m.Frames[0].File != "000.jpg" || m.Frames[0].Delay != 80 ||
		m.Frames[1].File != "001.jpg" || m.Frames[1].Delay != 120 {
		t.Errorf("Frames = %v", m.Frames)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/ugoira/metadata" {
		t.Errorf("path = %q", got.path)
	}
	if got.query.Get("illust_id") != "123" {
		t.Errorf("query = %v", got.query)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
}
