package pixiv

import (
	"context"
	"net/http"
	"testing"
)

const novelListBody = `{"novels":[{"id":42,"title":"雪の話","caption":"简介",` +
	`"user":{"id":9,"name":"someone"},"image_urls":{"large":"https://i/cover.jpg"},` +
	`"page_count":3,"create_date":"2026-10-01T00:00:00Z","is_bookmarked":true,` +
	`"total_bookmarks":12}],` +
	`"next_url":"https://app-api.pixiv.net/v1/search/novel?word=x&offset=30"}`

// TestSearchNovelsRequestsEndpoint checks the novel search hits its endpoint
// with the word and the defaults, and decodes the list.
func TestSearchNovelsRequestsEndpoint(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, novelListBody))

	page, err := c.SearchNovels(context.Background(), SearchOptions{Word: "雪"})
	if err != nil {
		t.Fatalf("SearchNovels: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/search/novel" {
		t.Errorf("path = %q", got.path)
	}
	q := got.query
	if q.Get("word") != "雪" || q.Get("sort") != "date_desc" ||
		q.Get("search_target") != "partial_match_for_tags" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want one novel", page.Items)
	}
	n := page.Items[0]
	if n.ID != 42 || n.Title != "雪の話" || n.PageCount != 3 || n.User.Name != "someone" ||
		!n.IsBookmarked || n.TotalBookmarks != 12 || n.CreateDate.IsZero() {
		t.Errorf("novel = %+v", n)
	}
	if got := n.CoverURL(); got != "https://i/cover.jpg" {
		t.Errorf("CoverURL = %q", got)
	}
	if !(n.Aspect() > 0) {
		t.Errorf("Aspect = %v, want a positive shape", n.Aspect())
	}
	if page.NextURL != "https://app-api.pixiv.net/v1/search/novel?word=x&offset=30" {
		t.Errorf("next_url = %q", page.NextURL)
	}
}

// TestSearchNovelsFilterFollowsAllowR18 checks the novel search asks Pixiv to
// filter R18 out unless the user allows it.
func TestSearchNovelsFilterFollowsAllowR18(t *testing.T) {
	for _, allow := range []bool{false, true} {
		var got recordedRequest
		c := newWriteClient(t, recordInto(&got, `{"novels":[],"next_url":""}`))
		c.SetAllowR18(allow)
		if _, err := c.SearchNovels(context.Background(), SearchOptions{Word: "x"}); err != nil {
			t.Fatalf("SearchNovels (AllowR18=%v): %v", allow, err)
		}
		want := "for_android"
		if allow {
			want = ""
		}
		if q := got.query.Get("filter"); q != want {
			t.Errorf("filter = %q (AllowR18=%v), want %q", q, allow, want)
		}
	}
}

// TestSearchNovelsRespectsExplicitFilter checks a caller's own filter wins over
// the client's R18 setting.
func TestSearchNovelsRespectsExplicitFilter(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"novels":[],"next_url":""}`))
	c.SetAllowR18(true)
	if _, err := c.SearchNovels(context.Background(), SearchOptions{Word: "x", Filter: "for_ios"}); err != nil {
		t.Fatalf("SearchNovels: %v", err)
	}
	if q := got.query.Get("filter"); q != "for_ios" {
		t.Errorf("filter = %q, want for_ios", q)
	}
}

// TestNextNovelsLoadsTheCursor checks the next page comes from the URL Pixiv
// handed out, decoded the same way.
func TestNextNovelsLoadsTheCursor(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, novelListBody))

	next := c.baseURL + "/v1/search/novel?word=x&offset=30"
	page, err := c.NextNovels(context.Background(), next)
	if err != nil {
		t.Fatalf("NextNovels: %v", err)
	}
	if got.path != "/v1/search/novel" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("word") != "x" || q.Get("offset") != "30" {
		t.Errorf("query = %v", q)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 42 {
		t.Fatalf("items = %+v, want the decoded novel", page.Items)
	}
}
