package pixiv

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestNovelBookmarkWrites(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{}`))
	ctx := context.Background()

	if err := c.AddNovelBookmark(ctx, 321, true); err != nil {
		t.Fatalf("AddNovelBookmark: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v2/novel/bookmark/add",
		url.Values{"novel_id": {"321"}, "restrict": {"private"}})

	if err := c.AddNovelBookmark(ctx, 654, false); err != nil {
		t.Fatalf("AddNovelBookmark public: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v2/novel/bookmark/add",
		url.Values{"novel_id": {"654"}, "restrict": {"public"}})

	if err := c.RemoveNovelBookmark(ctx, 321); err != nil {
		t.Fatalf("RemoveNovelBookmark: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/novel/bookmark/delete",
		url.Values{"novel_id": {"321"}})
}
