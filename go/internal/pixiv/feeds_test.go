package pixiv

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"
)

const feedBody = `{"illusts":[{"id":7,"title":"work","user":{"id":9,"name":"someone"}}],` +
	`"next_url":"https://app-api.pixiv.net/v2/illust/follow?restrict=public&offset=30"}`

func TestFollowingRequestsPublicFeed(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	page, err := c.Following(context.Background())
	if err != nil {
		t.Fatalf("Following: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v2/illust/follow" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("restrict") != "public" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
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
}

func TestRankingRequestsModeAndDate(t *testing.T) {
	cases := []struct {
		mode     RankMode
		date     string
		wantMode string
		wantDate string
	}{
		{Week, "2026-10-28", "week", "2026-10-28"},
		{DayMale, "2026-01-05", "day_male", "2026-01-05"},
		{WeekOriginal, "2026-02-28", "week_original", "2026-02-28"},
		{"", "2026-10-28", "day", "2026-10-28"},
	}
	for _, tc := range cases {
		var got recordedRequest
		c := newWriteClient(t, recordInto(&got, feedBody))

		page, err := c.Ranking(context.Background(), tc.mode, tc.date)
		if err != nil {
			t.Fatalf("Ranking(%q, %q): %v", tc.mode, tc.date, err)
		}
		if got.method != http.MethodGet {
			t.Errorf("method = %q, want GET", got.method)
		}
		if got.path != "/v1/illust/ranking" {
			t.Errorf("path = %q", got.path)
		}
		if q := got.query; q.Get("mode") != tc.wantMode || q.Get("date") != tc.wantDate ||
			q.Get("filter") != "for_android" {
			t.Errorf("Ranking(%q, %q) query = %v, want mode=%s date=%s",
				tc.mode, tc.date, q, tc.wantMode, tc.wantDate)
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
	}
}

func TestRankingDefaultsToYesterdayInJapan(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	if _, err := c.Ranking(context.Background(), "", ""); err != nil {
		t.Fatalf("Ranking: %v", err)
	}
	date := got.query.Get("date")
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(date) {
		t.Fatalf("date = %q, want YYYY-MM-DD", date)
	}
	if want := jstDate(time.Now().AddDate(0, 0, -1)); date != want {
		t.Errorf("date = %q, want yesterday in Japan %q", date, want)
	}
}

// TestDefaultRankingDateUsesJSTYesterday pins the date around a JST midnight,
// where a UTC-based yesterday would pick the wrong day.
func TestDefaultRankingDateUsesJSTYesterday(t *testing.T) {
	// 2026-10-29 00:30 JST is still 2026-10-28 in UTC.
	now := time.Date(2026, 10, 28, 15, 30, 0, 0, time.UTC)
	if got, want := defaultRankingDate(now), "2026-10-28"; got != want {
		t.Errorf("defaultRankingDate(%v) = %q, want %q", now, got, want)
	}
}

// TestR18FilterFollowsAllowR18 checks the feed and search requests filter R18
// out unless the client allows it.
func TestR18FilterFollowsAllowR18(t *testing.T) {
	endpoints := map[string]func(*Client) error{
		"recommended": func(c *Client) error { _, err := c.Recommended(context.Background()); return err },
		"following":   func(c *Client) error { _, err := c.Following(context.Background()); return err },
		"ranking":     func(c *Client) error { _, err := c.Ranking(context.Background(), Day, "2026-10-28"); return err },
		"search": func(c *Client) error {
			_, err := c.SearchIllustrations(context.Background(), SearchOptions{Word: "x"})
			return err
		},
	}
	for _, allow := range []bool{false, true} {
		for name, call := range endpoints {
			var got recordedRequest
			c := newWriteClient(t, recordInto(&got, `{"illusts":[],"next_url":""}`))
			c.SetAllowR18(allow)
			if err := call(c); err != nil {
				t.Fatalf("%s (AllowR18=%v): %v", name, allow, err)
			}
			want := "for_android"
			if allow {
				want = ""
			}
			if q := got.query.Get("filter"); q != want {
				t.Errorf("%s (AllowR18=%v): filter = %q, want %q", name, allow, q, want)
			}
		}
	}
}

// TestSearchRespectsExplicitFilter checks a caller's own filter wins over the
// client's R18 setting.
func TestSearchRespectsExplicitFilter(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"illusts":[],"next_url":""}`))
	c.SetAllowR18(true)
	opts := SearchOptions{Word: "x", Filter: "for_ios"}
	if _, err := c.SearchIllustrations(context.Background(), opts); err != nil {
		t.Fatalf("SearchIllustrations: %v", err)
	}
	if q := got.query.Get("filter"); q != "for_ios" {
		t.Errorf("filter = %q, want for_ios", q)
	}
}

func TestJSTDateUsesJapanZone(t *testing.T) {
	// 20:00 UTC is already the next day in Japan.
	utc := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	if got := jstDate(utc); got != "2026-10-29" {
		t.Errorf("jstDate(%v) = %q, want 2026-10-29", utc, got)
	}
}
