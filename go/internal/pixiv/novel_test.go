package pixiv

import (
	"context"
	"net/http"
	"testing"
)

// The reader endpoint answers an HTML page with the novel embedded, not JSON.
const novelPage = `<!doctype html>
<html><head><title>novel</title></head>
<body>
<script>window.__NOVEL__ = {"novel":{"id":123,"text":"[chapter:章]\n本文 [pixivimage:1-2]"}
,"other":{"text":"not the novel","close":"} in a string"}};</script>
</body></html>`

func TestNovelContent(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, novelPage))

	text, err := c.NovelContent(context.Background(), 123)
	if err != nil {
		t.Fatalf("NovelContent: %v", err)
	}
	if want := "[chapter:章]\n本文 [pixivimage:1-2]"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/webview/v2/novel" {
		t.Errorf("path = %q", got.path)
	}
	if got.query.Get("id") != "123" {
		t.Errorf("query = %v, want id=123", got.query)
	}
	if got.query.Get("novel_id") != "" {
		t.Errorf("query = %v, endpoint takes id, not novel_id", got.query)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
}

func TestNovelContentRequiresNovelObject(t *testing.T) {
	c := newWriteClient(t, recordInto(&recordedRequest{}, `<html><body>nothing here</body></html>`))
	if _, err := c.NovelContent(context.Background(), 1); err == nil {
		t.Fatal("NovelContent: want an error for a page without a novel object")
	}
}

func TestObjectEnd(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`{"a":"} not the end"}`, len(`{"a":"} not the end"}`)},
		{`  {"a":1}  `, 9},
		{`not an object`, 0},
		{`{"a":`, 0},
	}
	for _, c := range cases {
		if got := objectEnd(c.in); got != c.want {
			t.Errorf("objectEnd(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
