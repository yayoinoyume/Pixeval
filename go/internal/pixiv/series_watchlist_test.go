package pixiv

import (
	"context"
	"net/http"
	"testing"
)

const seriesWatchlistBody = `{"series":[{"id":42,"title":"系列四十二",` +
	`"url":"https://i.pximg.net/series.jpg",` +
	`"user":{"id":9,"name":"someone"},` +
	`"published_content_count":7,` +
	`"last_published_content_datetime":"2026-10-01T00:00:00Z"}],` +
	`"next_url":"https://app-api.pixiv.net/v1/watchlist/manga?offset=30"}`

// TestSeriesWatchlistRequestsEndpoint checks each kind hits its watchlist
// endpoint with the R18 filter and decodes the series and its cursor.
func TestSeriesWatchlistRequestsEndpoint(t *testing.T) {
	for _, tt := range []struct {
		kind SeriesKind
		path string
	}{
		{MangaSeries, "/v1/watchlist/manga"},
		{NovelSeries, "/v1/watchlist/novel"},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
			var got recordedRequest
			c := newWriteClient(t, recordInto(&got, seriesWatchlistBody))

			page, err := c.SeriesWatchlist(context.Background(), tt.kind)
			if err != nil {
				t.Fatalf("SeriesWatchlist: %v", err)
			}
			if got.method != http.MethodGet {
				t.Errorf("method = %q, want GET", got.method)
			}
			if got.path != tt.path {
				t.Errorf("path = %q, want %q", got.path, tt.path)
			}
			if q := got.query.Get("filter"); q != "for_android" {
				t.Errorf("filter = %q, want for_android", q)
			}
			if got.auth != "Bearer test-token" {
				t.Errorf("Authorization = %q, want a Bearer token", got.auth)
			}
			if len(page.Items) != 1 {
				t.Fatalf("items = %+v, want one series", page.Items)
			}
			s := page.Items[0]
			if s.ID != 42 || s.Title != "系列四十二" || s.User.Name != "someone" ||
				s.CoverURL != "https://i.pximg.net/series.jpg" || s.PublishedContentCount != 7 ||
				s.LastPublishedContent.IsZero() {
				t.Errorf("series = %+v", s)
			}
			if page.NextURL != "https://app-api.pixiv.net/v1/watchlist/manga?offset=30" {
				t.Errorf("next_url = %q", page.NextURL)
			}
		})
	}
}

// TestSeriesWatchlistFilterFollowsAllowR18 checks the watchlist asks Pixiv to
// filter R18 out unless the user allows it.
func TestSeriesWatchlistFilterFollowsAllowR18(t *testing.T) {
	for _, allow := range []bool{false, true} {
		var got recordedRequest
		c := newWriteClient(t, recordInto(&got, `{"series":[],"next_url":""}`))
		c.SetAllowR18(allow)
		if _, err := c.SeriesWatchlist(context.Background(), MangaSeries); err != nil {
			t.Fatalf("SeriesWatchlist (AllowR18=%v): %v", allow, err)
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

// TestNextSeriesWatchlistLoadsTheCursor checks the next page comes from the URL
// Pixiv handed out, decoded the same way.
func TestNextSeriesWatchlistLoadsTheCursor(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, seriesWatchlistBody))

	next := c.baseURL + "/v1/watchlist/manga?offset=30"
	page, err := c.NextSeriesWatchlist(context.Background(), next)
	if err != nil {
		t.Fatalf("NextSeriesWatchlist: %v", err)
	}
	if got.path != "/v1/watchlist/manga" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("offset") != "30" {
		t.Errorf("query = %v", got.query)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 42 {
		t.Fatalf("items = %+v, want the decoded series", page.Items)
	}
}
