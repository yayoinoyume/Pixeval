package pixiv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	form   url.Values
	auth   string
	ctype  string
}

// newWriteClient points a client at a test server with no pacing.
func newWriteClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.SetToken(&Token{AccessToken: "test-token"})
	return c
}

// recordInto captures the request and answers 200 with body.
func recordInto(got *recordedRequest, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			form:   r.PostForm,
			auth:   r.Header.Get("Authorization"),
			ctype:  r.Header.Get("Content-Type"),
		}
		io.WriteString(w, body)
	}
}

func checkWrite(t *testing.T, got recordedRequest, method, path string, form url.Values) {
	t.Helper()
	if got.method != method {
		t.Errorf("method = %q, want %q", got.method, method)
	}
	if got.path != path {
		t.Errorf("path = %q, want %q", got.path, path)
	}
	if enc := got.form.Encode(); enc != form.Encode() {
		t.Errorf("form = %q, want %q", enc, form.Encode())
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
	if got.ctype != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want form encoding", got.ctype)
	}
}

func TestNewClientUsesAppBaseURL(t *testing.T) {
	if got := NewClient().baseURL; got != appBaseURL {
		t.Errorf("baseURL = %q, want %q", got, appBaseURL)
	}
}

func TestBookmarkWrites(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{}`))
	ctx := context.Background()

	if err := c.AddBookmark(ctx, 123, true); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v2/illust/bookmark/add",
		url.Values{"illust_id": {"123"}, "restrict": {"private"}})

	if err := c.AddBookmark(ctx, 456, false); err != nil {
		t.Fatalf("AddBookmark public: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v2/illust/bookmark/add",
		url.Values{"illust_id": {"456"}, "restrict": {"public"}})

	if err := c.RemoveBookmark(ctx, 123); err != nil {
		t.Fatalf("RemoveBookmark: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/illust/bookmark/delete",
		url.Values{"illust_id": {"123"}})
}

func TestFollowWrites(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{}`))
	ctx := context.Background()

	if err := c.Follow(ctx, 42, true); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/user/follow/add",
		url.Values{"user_id": {"42"}, "restrict": {"private"}})

	if err := c.Follow(ctx, 42, false); err != nil {
		t.Fatalf("Follow public: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/user/follow/add",
		url.Values{"user_id": {"42"}, "restrict": {"public"}})

	if err := c.Unfollow(ctx, 42); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/user/follow/delete",
		url.Values{"user_id": {"42"}})
}

func TestCommentsReadsPage(t *testing.T) {
	const body = `{"comments":[{"id":7,"comment":"nice","user":{"id":9,"name":"someone"},` +
		`"date":"2024-05-01T10:00:00+09:00","has_replies":true,"parent_comment":{"id":3,"comment":"root"}}],` +
		`"next_url":"https://app-api.pixiv.net/v3/illust/comments?illust_id=123&offset=10"}`
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, body))

	page, err := c.Comments(context.Background(), 123, 0)
	if err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v3/illust/comments" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("illust_id") != "123" || q.Get("offset") != "0" {
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
	if !cm.HasReplies {
		t.Error("has_replies = false, want true")
	}
	if cm.ParentComment == nil || cm.ParentComment.ID != 3 {
		t.Errorf("parent_comment = %+v", cm.ParentComment)
	}
	if page.NextURL == "" {
		t.Error("next_url is empty")
	}
}

func TestCommentWrites(t *testing.T) {
	const body = `{"comment":{"id":1,"comment":"hello world","date":"2024-05-01T10:00:00+09:00"}}`
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, body))
	ctx := context.Background()

	cm, err := c.AddComment(ctx, 123, "hello world")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/illust/comment/add",
		url.Values{"illust_id": {"123"}, "comment": {"hello world"}})
	if cm == nil || cm.ID != 1 || cm.Comment != "hello world" || cm.CreateDate.IsZero() {
		t.Errorf("AddComment returned %+v", cm)
	}

	if err := c.DeleteComment(ctx, 99); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	checkWrite(t, got, http.MethodPost, "/v1/illust/comment/delete",
		url.Values{"comment_id": {"99"}})
}

func TestReadEndpointUsesBaseURL(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"illusts":[],"next_url":""}`))

	if _, err := c.Recommended(context.Background()); err != nil {
		t.Fatalf("Recommended: %v", err)
	}
	if got.method != http.MethodGet || got.path != "/v1/illust/recommended" {
		t.Errorf("request = %s %s, want GET /v1/illust/recommended", got.method, got.path)
	}
}

func TestRateLimitPausesRequests(t *testing.T) {
	var calls int
	var limit time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.backoff = []time.Duration{0}
	c.OnRateLimit(func(d time.Duration) { limit = d })

	start := time.Now()
	if err := c.RemoveBookmark(context.Background(), 1); err != nil {
		t.Fatalf("RemoveBookmark: %v", err)
	}
	elapsed := time.Since(start)
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (429 then success)", calls)
	}
	if limit != time.Second {
		t.Errorf("onLimit = %v, want 1s", limit)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("elapsed = %v, want the 429 pause to be honoured", elapsed)
	}
}

func TestServerErrorRetriesReads(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"comments":[],"next_url":""}`)
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.backoff = []time.Duration{time.Millisecond}

	if _, err := c.Comments(context.Background(), 1, 0); err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (500 then success)", calls)
	}
}

func TestPostServerErrorIsNotRetried(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.backoff = []time.Duration{0}

	if _, err := c.AddComment(context.Background(), 1, "hi"); err == nil {
		t.Fatal("AddComment with 500 = nil error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (a POST is not replayed)", calls)
	}
}

func TestStatusSentinels(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
		}))
		c := NewClient()
		c.baseURL = srv.URL
		c.SetCooldown(0)
		err := c.RemoveBookmark(context.Background(), 1)
		srv.Close()
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: err = %v, want %v", tc.status, err, tc.want)
		}
	}
}

func TestUnauthorizedIsNotRetried(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.backoff = []time.Duration{0}

	if err := c.DeleteComment(context.Background(), 1); err == nil {
		t.Fatal("DeleteComment with 401 = nil error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (401 is final)", calls)
	}
}
