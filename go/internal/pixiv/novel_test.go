package pixiv

import (
	"context"
	"net/http"
	"testing"
)

func TestNovelContent(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"novel_text":"<p>本文</p>"}`))

	text, err := c.NovelContent(context.Background(), 123)
	if err != nil {
		t.Fatalf("NovelContent: %v", err)
	}
	if text != "<p>本文</p>" {
		t.Errorf("text = %q", text)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/webview/v2/novel" {
		t.Errorf("path = %q", got.path)
	}
	if got.query.Get("novel_id") != "123" {
		t.Errorf("query = %v", got.query)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
}
