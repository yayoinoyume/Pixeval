package saucenao

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSearchRequestsAndParses checks the request's path and query, and the
// fields the result cards draw.
func TestSearchRequestsAndParses(t *testing.T) {
	const body = `{"header":{"status":0},"results":[` +
		`{"header":{"similarity":"97.4","thumbnail":"https://img.example/thumb.jpg"},` +
		`"data":{"title":"作品标题","creator":"画师",` +
		`"author_url":"https://www.pixiv.net/users/1",` +
		`"ext_urls":["https://www.pixiv.net/artworks/1","https://example.com/2"]}},` +
		`{"header":{"similarity":"41.2"},` +
		`"data":{"creator":["作者A","作者B"],"ext_urls":["https://example.com/3"]}}]}`
	var gotMethod, gotPath, gotOutput, gotKey, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotMethod, gotPath = r.Method, r.URL.Path
		gotOutput, gotKey, gotURL = q.Get("output_type"), q.Get("api_key"), q.Get("url")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL}
	res, err := c.Search(context.Background(), "secret key", "https://example.com/a b.jpg")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/search.php" {
		t.Errorf("path = %q, want /search.php", gotPath)
	}
	if gotOutput != "2" {
		t.Errorf("output_type = %q, want 2", gotOutput)
	}
	if gotKey != "secret key" {
		t.Errorf("api_key = %q, want the key", gotKey)
	}
	if gotURL != "https://example.com/a b.jpg" {
		t.Errorf("url = %q, want the image URL", gotURL)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("entries = %+v, want two", res.Entries)
	}
	first := res.Entries[0]
	if first.Similarity != 97.4 {
		t.Errorf("similarity = %v, want 97.4", first.Similarity)
	}
	if first.Thumbnail != "https://img.example/thumb.jpg" {
		t.Errorf("thumbnail = %q", first.Thumbnail)
	}
	if first.Title != "作品标题" || first.Creator != "画师" {
		t.Errorf("title/creator = %q/%q", first.Title, first.Creator)
	}
	if first.AuthorURL != "https://www.pixiv.net/users/1" {
		t.Errorf("author_url = %q", first.AuthorURL)
	}
	if len(first.ExtURLs) != 2 || first.ExtURLs[0] != "https://www.pixiv.net/artworks/1" {
		t.Errorf("ext_urls = %v", first.ExtURLs)
	}
	if second := res.Entries[1]; second.Creator != "作者A, 作者B" {
		t.Errorf("list creator = %q, want both names", second.Creator)
	}
}

// TestSearchEmptyAndErrors checks an empty answer, a refused request and a
// failed status each come back as the error the page tells apart.
func TestSearchEmptyAndErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"empty", http.StatusOK, `{"header":{"status":0},"results":[]}`, ErrNoResults},
		{"rate limit", http.StatusTooManyRequests, ``, ErrRateLimited},
		{"failed status", http.StatusOK, `{"header":{"status":1},"results":[]}`, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			t.Cleanup(srv.Close)
			c := &Client{BaseURL: srv.URL}
			_, err := c.Search(context.Background(), "key", "https://example.com/a.jpg")
			if err == nil {
				t.Fatal("Search succeeded, want an error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestSearchRejectsBadJSON checks a broken answer is an error, not an empty
// result.
func TestSearchRejectsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{not json`)
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL}
	if _, err := c.Search(context.Background(), "key", "https://example.com/a.jpg"); err == nil {
		t.Fatal("Search accepted bad JSON")
	}
}

// TestNewClientUsesPublicSite checks the default client talks to SauceNAO.
func TestNewClientUsesPublicSite(t *testing.T) {
	if got := NewClient().BaseURL; got != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", got, DefaultBaseURL)
	}
}
