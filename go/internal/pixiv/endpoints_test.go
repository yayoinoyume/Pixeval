package pixiv

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestNewIllustrationsRequestsNewFeed(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	page, err := c.NewIllustrations(context.Background())
	if err != nil {
		t.Fatalf("NewIllustrations: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/illust/new" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("filter") != "for_android" || q.Get("content_type") != "illust" || len(q) != 2 {
		t.Errorf("query = %v, want the R18 filter and content_type=illust", q)
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

func TestMyPixivIllustrationsRequestsMyPixivFeed(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	page, err := c.MyPixivIllustrations(context.Background())
	if err != nil {
		t.Fatalf("MyPixivIllustrations: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/illust/mypixiv" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("filter") != "for_android" || len(q) != 1 {
		t.Errorf("query = %v, want only the R18 filter", q)
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

// TestFeedEndpointsPageWithNextURL checks both feeds hand their next_url to
// NextIllustrations, which asks the same path for the next page.
func TestFeedEndpointsPageWithNextURL(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		contentType string
		first       func(*Client) (*Page[Illustration], error)
	}{
		{"new", "/v1/illust/new", "illust", func(c *Client) (*Page[Illustration], error) {
			return c.NewIllustrations(context.Background())
		}},
		{"mypixiv", "/v1/illust/mypixiv", "", func(c *Client) (*Page[Illustration], error) {
			return c.MyPixivIllustrations(context.Background())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs []recordedRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqs = append(reqs, recordedRequest{
					method: r.Method,
					path:   r.URL.Path,
					query:  r.URL.Query(),
					auth:   r.Header.Get("Authorization"),
				})
				w.Header().Set("Content-Type", "application/json")
				if len(reqs) == 1 {
					next := "http://" + r.Host + tc.path + "?filter=for_android&offset=30"
					if tc.contentType != "" {
						next += "&content_type=" + tc.contentType
					}
					io.WriteString(w, `{"illusts":[{"id":1,"title":"first"}],"next_url":`+strconv.Quote(next)+`}`)
					return
				}
				io.WriteString(w, `{"illusts":[{"id":2,"title":"second"}],"next_url":""}`)
			}))
			t.Cleanup(srv.Close)

			c := NewClient()
			c.baseURL = srv.URL
			c.SetCooldown(0)
			c.SetToken(&Token{AccessToken: "test-token"})

			page, err := tc.first(c)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if len(page.Items) != 1 || page.Items[0].ID != 1 || page.NextURL == "" {
				t.Fatalf("%s first page = %+v, next %q", tc.name, page.Items, page.NextURL)
			}

			next, err := c.NextIllustrations(context.Background(), page.NextURL)
			if err != nil {
				t.Fatalf("%s next page: %v", tc.name, err)
			}
			if len(next.Items) != 1 || next.Items[0].ID != 2 || next.NextURL != "" {
				t.Fatalf("%s next page = %+v, next %q", tc.name, next.Items, next.NextURL)
			}

			if len(reqs) != 2 {
				t.Fatalf("%s requests = %d, want 2", tc.name, len(reqs))
			}
			if reqs[0].path != tc.path || reqs[1].path != tc.path {
				t.Errorf("%s paths = %q, %q, want both %q", tc.name, reqs[0].path, reqs[1].path, tc.path)
			}
			if q := reqs[0].query; q.Get("filter") != "for_android" || q.Get("content_type") != tc.contentType || q.Get("offset") != "" {
				t.Errorf("%s first query = %v, want the R18 filter and content_type %q", tc.name, q, tc.contentType)
			}
			if q := reqs[1].query; q.Get("filter") != "for_android" || q.Get("content_type") != tc.contentType || q.Get("offset") != "30" {
				t.Errorf("%s next query = %v, want the cursor's offset", tc.name, q)
			}
			if reqs[1].auth != "Bearer test-token" {
				t.Errorf("%s next Authorization = %q, want a Bearer token", tc.name, reqs[1].auth)
			}
		})
	}
}

// TestNewFeedFilterFollowsAllowR18 checks the two feeds drop R18 works unless
// the client allows them, as the other feeds do.
func TestNewFeedFilterFollowsAllowR18(t *testing.T) {
	endpoints := map[string]func(*Client) error{
		"new":     func(c *Client) error { _, err := c.NewIllustrations(context.Background()); return err },
		"mypixiv": func(c *Client) error { _, err := c.MyPixivIllustrations(context.Background()); return err },
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

// TestRelatedIllustrationsRequestsSource checks the related feed asks for the
// work it belongs to and carries the R18 filter, decoding illusts and next_url.
func TestRelatedIllustrationsRequestsSource(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, feedBody))

	page, err := c.RelatedIllustrations(context.Background(), 123)
	if err != nil {
		t.Fatalf("RelatedIllustrations: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/illust/related" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("illust_id") != "123" || q.Get("filter") != "for_android" || len(q) != 2 {
		t.Errorf("query = %v, want illust_id=123 and the R18 filter", q)
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

// TestRelatedIllustrationsPagesWithNextURL checks the related feed's next_url
// pages through NextIllustrations, and the R18 filter follows the setting.
func TestRelatedIllustrationsPagesWithNextURL(t *testing.T) {
	for _, allow := range []bool{false, true} {
		var reqs []recordedRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqs = append(reqs, recordedRequest{
				method: r.Method,
				path:   r.URL.Path,
				query:  r.URL.Query(),
			})
			w.Header().Set("Content-Type", "application/json")
			if len(reqs) == 1 {
				next := "http://" + r.Host + "/v1/illust/related?illust_id=123&offset=30"
				if !allow {
					next += "&filter=for_android"
				}
				io.WriteString(w, `{"illusts":[{"id":1,"title":"first"}],"next_url":`+strconv.Quote(next)+`}`)
				return
			}
			io.WriteString(w, `{"illusts":[{"id":2,"title":"second"}],"next_url":""}`)
		}))
		t.Cleanup(srv.Close)

		c := NewClient()
		c.baseURL = srv.URL
		c.SetCooldown(0)
		c.SetAllowR18(allow)

		page, err := c.RelatedIllustrations(context.Background(), 123)
		if err != nil {
			t.Fatalf("RelatedIllustrations (AllowR18=%v): %v", allow, err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != 1 || page.NextURL == "" {
			t.Fatalf("first page = %+v, next %q", page.Items, page.NextURL)
		}
		next, err := c.NextIllustrations(context.Background(), page.NextURL)
		if err != nil {
			t.Fatalf("next page: %v", err)
		}
		if len(next.Items) != 1 || next.Items[0].ID != 2 || next.NextURL != "" {
			t.Fatalf("next page = %+v, next %q", next.Items, next.NextURL)
		}
		if len(reqs) != 2 || reqs[0].path != "/v1/illust/related" || reqs[1].path != "/v1/illust/related" {
			t.Fatalf("requests = %+v, want two related pages", reqs)
		}
		wantFilter := "for_android"
		if allow {
			wantFilter = ""
		}
		if q := reqs[0].query; q.Get("illust_id") != "123" || q.Get("filter") != wantFilter || q.Get("offset") != "" {
			t.Errorf("first query = %v, want illust_id=123, filter %q and no cursor", q, wantFilter)
		}
		if q := reqs[1].query; q.Get("illust_id") != "123" || q.Get("filter") != wantFilter || q.Get("offset") != "30" {
			t.Errorf("next query = %v, want the cursor's offset", q)
		}
	}
}
