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

func TestRankingDefaultsToTodayInJapan(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	if _, err := c.Ranking(context.Background(), "", ""); err != nil {
		t.Fatalf("Ranking: %v", err)
	}
	date := got.query.Get("date")
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(date) {
		t.Fatalf("date = %q, want YYYY-MM-DD", date)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		t.Errorf("date = %q is not a real date: %v", date, err)
	}
}

func TestJSTDateUsesJapanZone(t *testing.T) {
	// 20:00 UTC is already the next day in Japan.
	utc := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	if got := jstDate(utc); got != "2026-10-29" {
		t.Errorf("jstDate(%v) = %q, want 2026-10-29", utc, got)
	}
}
