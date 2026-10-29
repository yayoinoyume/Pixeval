package pixiv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const spotlightBody = `{"spotlight_articles":[{"id":12345,"title":"特辑标题",` +
	`"pure_title":"特辑标题（纯）","thumbnail":"https://i.pximg.net/spotlight.jpg",` +
	`"article_url":"https://www.pixivision.net/zh/a/12345",` +
	`"publish_date":"2026-10-01 12:00:00"}],` +
	`"next_url":"https://app-api.pixiv.net/v1/spotlight/articles?category=all&offset=10"}`

// TestSpotlightArticlesRequestsAndParses checks the path, the R18 filter and
// the decoded article fields.
func TestSpotlightArticlesRequestsAndParses(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, spotlightBody))

	page, err := c.SpotlightArticles(context.Background())
	if err != nil {
		t.Fatalf("SpotlightArticles: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/spotlight/articles" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("category") != "all" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v, want category=all and filter=for_android", q)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want one article", page.Items)
	}
	article := page.Items[0]
	if article.ID != 12345 || article.Title != "特辑标题" || article.PureTitle != "特辑标题（纯）" {
		t.Errorf("article = %+v", article)
	}
	if article.Thumbnail != "https://i.pximg.net/spotlight.jpg" {
		t.Errorf("thumbnail = %q", article.Thumbnail)
	}
	if article.ArticleURL != "https://www.pixivision.net/zh/a/12345" {
		t.Errorf("article_url = %q", article.ArticleURL)
	}
	if article.PublishDate != "2026-10-01 12:00:00" {
		t.Errorf("publish_date = %q", article.PublishDate)
	}
	if page.NextURL == "" {
		t.Error("next_url is empty")
	}
}

// TestSpotlightArticleNumericID checks the live feed's numeric id decodes
// into the article: the API sends a number, not the string an earlier
// fixture used.
func TestSpotlightArticleNumericID(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got,
		`{"spotlight_articles":[{"id":9876543210123}],"next_url":""}`))

	page, err := c.SpotlightArticles(context.Background())
	if err != nil {
		t.Fatalf("SpotlightArticles: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 9876543210123 {
		t.Fatalf("items = %+v, want the numeric id", page.Items)
	}
}

// TestSpotlightArticlesFilterFollowsAllowR18 checks allowing R18 works drops
// the filter, as the other feeds do.
func TestSpotlightArticlesFilterFollowsAllowR18(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, `{"spotlight_articles":[],"next_url":""}`))
	c.SetAllowR18(true)

	if _, err := c.SpotlightArticles(context.Background()); err != nil {
		t.Fatalf("SpotlightArticles: %v", err)
	}
	if q := got.query; q.Has("filter") {
		t.Errorf("query = %v, want no filter", q)
	}
}

// TestSpotlightArticlesNextPage checks a next_url loads through the cursor and
// keeps its own query.
func TestSpotlightArticlesNextPage(t *testing.T) {
	var srv *httptest.Server
	var offsets, categories []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		categories = append(categories, r.URL.Query().Get("category"))
		if len(offsets) == 1 {
			fmt.Fprintf(w, `{"spotlight_articles":[{"id":1}],`+
				`"next_url":"%s/v1/spotlight/articles?category=all&offset=10"}`, srv.URL)
			return
		}
		io.WriteString(w, `{"spotlight_articles":[{"id":2}],"next_url":""}`)
	}))
	t.Cleanup(srv.Close)
	c := NewClient()
	c.baseURL = srv.URL
	c.SetCooldown(0)
	c.SetToken(&Token{AccessToken: "test-token"})

	page, err := c.SpotlightArticles(context.Background())
	if err != nil {
		t.Fatalf("SpotlightArticles: %v", err)
	}
	next, err := c.NextSpotlightArticles(context.Background(), page.NextURL)
	if err != nil {
		t.Fatalf("NextSpotlightArticles: %v", err)
	}
	if len(offsets) != 2 || offsets[0] != "" || offsets[1] != "10" {
		t.Errorf("offsets = %v, want the first page then 10", offsets)
	}
	if len(categories) != 2 || categories[0] != "all" || categories[1] != "all" {
		t.Errorf("categories = %v, want all on both pages", categories)
	}
	if len(next.Items) != 1 || next.Items[0].ID != 2 {
		t.Errorf("next items = %+v, want the second page", next.Items)
	}
	if next.NextURL != "" {
		t.Errorf("next_url = %q, want empty", next.NextURL)
	}
}
