package pixiv

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"
)

const bookmarkFeedBody = `{"illusts":[{"id":7,"title":"work","user":{"id":9,"name":"someone"}}],` +
	`"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=42&restrict=public&offset=48&limit=48"}`

func TestBookmarkedIllustrationsRequestsTab(t *testing.T) {
	cases := []struct {
		name    string
		private bool
		offset  int
		want    string
	}{
		{"public", false, 0, "public"},
		{"private", true, 48, "private"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got recordedRequest
			c := newWriteClient(t, recordInto(&got, bookmarkFeedBody))

			page, err := c.BookmarkedIllustrations(context.Background(), 42, tc.private, tc.offset)
			if err != nil {
				t.Fatalf("BookmarkedIllustrations: %v", err)
			}
			if got.method != http.MethodGet {
				t.Errorf("method = %q, want GET", got.method)
			}
			if got.path != "/v1/user/bookmarks/illust" {
				t.Errorf("path = %q", got.path)
			}
			q := got.query
			if q.Get("user_id") != "42" || q.Get("restrict") != tc.want ||
				q.Get("filter") != "for_android" || q.Get("limit") != "48" ||
				q.Get("offset") != strconv.Itoa(tc.offset) {
				t.Errorf("query = %v, want user_id=42 restrict=%s offset=%d limit=48", q, tc.want, tc.offset)
			}
			if got.auth != "Bearer test-token" {
				t.Errorf("Authorization = %q, want a Bearer token", got.auth)
			}
			if len(page.Items) != 1 || page.Items[0].ID != 7 {
				t.Fatalf("items = %+v, want the decoded illust", page.Items)
			}
			if page.NextURL == "" {
				t.Error("next_url is empty")
			}
		})
	}
}

func TestBookmarkedIllustrationsRespectsR18Filter(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, bookmarkFeedBody))
	c.SetAllowR18(true)

	if _, err := c.BookmarkedIllustrations(context.Background(), 42, false, 0); err != nil {
		t.Fatalf("BookmarkedIllustrations: %v", err)
	}
	if filter := got.query.Get("filter"); filter != "" {
		t.Errorf("filter = %q with R18 allowed, want none", filter)
	}
}

func TestBookmarkedIllustrationsPagingPassesNextURL(t *testing.T) {
	var paths, offsets []string
	c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		offsets = append(offsets, r.URL.Query().Get("offset"))
		io.WriteString(w, `{"illusts":[],"next_url":"http://`+r.Host+
			`/v1/user/bookmarks/illust?user_id=42&restrict=public&offset=48&limit=48"}`)
	})
	ctx := context.Background()

	first, err := c.BookmarkedIllustrations(ctx, 42, false, 0)
	if err != nil {
		t.Fatalf("BookmarkedIllustrations: %v", err)
	}
	if _, err := c.NextIllustrations(ctx, first.NextURL); err != nil {
		t.Fatalf("NextIllustrations: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1/user/bookmarks/illust" || paths[1] != "/v1/user/bookmarks/illust" {
		t.Errorf("paths = %v", paths)
	}
	if len(offsets) != 2 || offsets[0] != "0" || offsets[1] != "48" {
		t.Errorf("offsets = %v, want 0 then 48", offsets)
	}
}
