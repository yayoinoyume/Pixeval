package pixiv

import (
	"context"
	"net/http"
	"testing"
)

const novelCommentsBody = `{"comments":[{"id":7,"comment":"nice","user":{"id":9,"name":"someone"},` +
	`"date":"2024-05-01T10:00:00+09:00","has_replies":false}],` +
	`"next_url":"https://app-api.pixiv.net/v1/novel/comments?novel_id=123&offset=1"}`

func TestNovelCommentsReadsPage(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, novelCommentsBody))

	page, err := c.NovelComments(context.Background(), 123, 0)
	if err != nil {
		t.Fatalf("NovelComments: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/novel/comments" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("novel_id") != "123" || q.Get("offset") != "0" {
		t.Errorf("query = %v", q)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	cm := page.Items[0]
	if cm.ID != 7 || cm.Comment != "nice" || cm.User.Name != "someone" {
		t.Errorf("comment = %+v", cm)
	}
	if cm.CreateDate.IsZero() || cm.CreateDate.Year() != 2024 {
		t.Errorf("date = %v, want 2024 parsed from the date field", cm.CreateDate)
	}
	if page.NextURL == "" {
		t.Error("next_url is empty")
	}
}

func TestNovelCommentsOffset(t *testing.T) {
	const body = `{"comments":[],"next_url":""}`
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, body))

	page, err := c.NovelComments(context.Background(), 456, 10)
	if err != nil {
		t.Fatalf("NovelComments: %v", err)
	}
	if q := got.query; q.Get("novel_id") != "456" || q.Get("offset") != "10" {
		t.Errorf("query = %v, want novel_id=456 offset=10", q)
	}
	if len(page.Items) != 0 {
		t.Errorf("items = %d, want none", len(page.Items))
	}
	if page.NextURL != "" {
		t.Errorf("next_url = %q, want none", page.NextURL)
	}
}
