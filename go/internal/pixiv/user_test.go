package pixiv

import (
	"context"
	"io"
	"net/http"
	"testing"
)

const userDetailBody = `{
  "user": {"id": 42, "name": "someone", "account": "someone_account"},
  "profile": {
    "webpage": "https://example.com",
    "gender": "female",
    "birth": "1-1",
    "region": "Tokyo",
    "total_follow_users": 12,
    "total_illusts": 34,
    "total_manga": 5,
    "is_premium": true
  }
}`

func TestUserDetailDecodesProfile(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, userDetailBody))

	detail, err := c.UserDetail(context.Background(), 42)
	if err != nil {
		t.Fatalf("UserDetail: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/user/detail" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("user_id") != "42" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
	}
	if got.auth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want a Bearer token", got.auth)
	}
	if detail.User.ID != 42 || detail.User.Name != "someone" {
		t.Errorf("user = %+v", detail.User)
	}
	p := detail.Profile
	if p.Webpage != "https://example.com" || p.Gender != "female" || p.Birth != "1-1" ||
		p.Region != "Tokyo" || !p.IsPremium {
		t.Errorf("profile = %+v", p)
	}
	if p.TotalFollowUsers != 12 || p.TotalIllusts != 34 || p.TotalManga != 5 {
		t.Errorf("profile counts = %+v", p)
	}
}

func TestUserIllustsRequestsKind(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"manga", "manga"},
		{"illust", "illust"},
		{"", "illust"},
	}
	for _, tc := range cases {
		var got recordedRequest
		c := newWriteClient(t, recordInto(&got, feedBody))

		page, err := c.UserIllusts(context.Background(), 42, tc.kind)
		if err != nil {
			t.Fatalf("UserIllusts(%q): %v", tc.kind, err)
		}
		if got.path != "/v1/user/illusts" {
			t.Errorf("path = %q", got.path)
		}
		if q := got.query; q.Get("user_id") != "42" || q.Get("type") != tc.want ||
			q.Get("filter") != "for_android" {
			t.Errorf("UserIllusts(%q) query = %v, want type=%s", tc.kind, q, tc.want)
		}
		if len(page.Items) != 1 || page.Items[0].ID != 7 {
			t.Fatalf("items = %+v, want the decoded illust", page.Items)
		}
		if page.NextURL == "" {
			t.Error("next_url is empty")
		}
	}
}

const userNovelsBody = `{"novels":[{"id":7,"title":"a novel",` +
	`"user":{"id":9,"name":"someone"},"page_count":3}],` +
	`"next_url":"https://app-api.pixiv.net/v1/user/novels?user_id=42&offset=30"}`

func TestUserNovelsRequestsUserAndFilter(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, userNovelsBody))

	page, err := c.UserNovels(context.Background(), 42)
	if err != nil {
		t.Fatalf("UserNovels: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/user/novels" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("user_id") != "42" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want one novel", page.Items)
	}
	n := page.Items[0]
	if n.ID != 7 || n.Title != "a novel" || n.User.ID != 9 || n.PageCount != 3 {
		t.Errorf("novel = %+v", n)
	}
	if page.NextURL == "" {
		t.Error("next_url is empty")
	}
}

func TestUserNovelsFilterFollowsAllowR18(t *testing.T) {
	for _, allow := range []bool{false, true} {
		var got recordedRequest
		c := newWriteClient(t, recordInto(&got, userNovelsBody))
		c.SetAllowR18(allow)

		if _, err := c.UserNovels(context.Background(), 42); err != nil {
			t.Fatalf("UserNovels (AllowR18=%v): %v", allow, err)
		}
		want := "for_android"
		if allow {
			want = ""
		}
		if filter := got.query.Get("filter"); filter != want {
			t.Errorf("filter = %q (AllowR18=%v), want %q", filter, allow, want)
		}
	}
}

func TestUserNovelsPagingPassesNextURL(t *testing.T) {
	var paths []string
	c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{"novels":[],"next_url":"http://`+r.Host+
			`/v1/user/novels?user_id=42&offset=30"}`)
	})
	ctx := context.Background()

	first, err := c.UserNovels(ctx, 42)
	if err != nil {
		t.Fatalf("UserNovels: %v", err)
	}
	second, err := c.NextNovels(ctx, first.NextURL)
	if err != nil {
		t.Fatalf("NextNovels: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1/user/novels" || paths[1] != "/v1/user/novels" {
		t.Errorf("paths = %v", paths)
	}
	if len(first.Items) != 0 || len(second.Items) != 0 {
		t.Errorf("empty lists decoded as %+v / %+v", first.Items, second.Items)
	}
}

const userPreviewBody = `{"user_previews":[{"user":{"id":9,"name":"fan"},` +
	`"illusts":[{"id":7,"title":"work"}],"is_muted":true}],` +
	`"next_url":"https://app-api.pixiv.net/v1/user/follower?offset=30"}`

func TestUserFollowingAndFollowers(t *testing.T) {
	var followingReq, followerReq recordedRequest
	c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/following":
			recordInto(&followingReq, userPreviewBody)(w, r)
		case "/v1/user/follower":
			recordInto(&followerReq, userPreviewBody)(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()

	following, err := c.UserFollowing(ctx, 42)
	if err != nil {
		t.Fatalf("UserFollowing: %v", err)
	}
	if followingReq.method != http.MethodGet {
		t.Errorf("following method = %q, want GET", followingReq.method)
	}
	if q := followingReq.query; q.Get("user_id") != "42" ||
		q.Get("restrict") != "public" || q.Get("filter") != "for_android" {
		t.Errorf("following query = %v", q)
	}

	followers, err := c.UserFollowers(ctx, 42)
	if err != nil {
		t.Fatalf("UserFollowers: %v", err)
	}
	if q := followerReq.query; q.Get("user_id") != "42" ||
		q.Get("restrict") != "" || q.Get("filter") != "for_android" {
		t.Errorf("follower query = %v", q)
	}

	for name, page := range map[string]*Page[UserPreview]{
		"following": following,
		"followers": followers,
	} {
		if len(page.Items) != 1 {
			t.Fatalf("%s items = %+v", name, page.Items)
		}
		p := page.Items[0]
		if p.User.ID != 9 || len(p.Illusts) != 1 || p.Illusts[0].ID != 7 || !p.IsMuted {
			t.Errorf("%s preview = %+v", name, p)
		}
		if page.NextURL == "" {
			t.Errorf("%s next_url is empty", name)
		}
	}
}

func TestUserListPagingPassesNextURL(t *testing.T) {
	var paths []string
	c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{"user_previews":[],"next_url":"http://`+r.Host+
			`/v1/user/follower?offset=30"}`)
	})
	ctx := context.Background()

	first, err := c.UserFollowers(ctx, 42)
	if err != nil {
		t.Fatalf("UserFollowers: %v", err)
	}
	second, err := c.NextUserPreviews(ctx, first.NextURL)
	if err != nil {
		t.Fatalf("NextUserPreviews: %v", err)
	}
	if len(paths) != 2 || paths[0] != "/v1/user/follower" || paths[1] != "/v1/user/follower" {
		t.Errorf("paths = %v", paths)
	}
	if len(first.Items) != 0 || len(second.Items) != 0 {
		t.Errorf("empty lists decoded as %+v / %+v", first.Items, second.Items)
	}
}

const seriesBody = `{
  "illust_series_detail": {
    "id": 77,
    "title": "a series",
    "description": "about it",
    "cover_image_urls": {"large": "https://example.com/cover.jpg"},
    "work_illusts_count": 2,
    "is_concluded": true
  },
  "illusts": [{"id": 7, "title": "work"}, {"id": 8, "title": "another"}],
  "next_url": "https://app-api.pixiv.net/v1/illust/series?offset=30"
}`

func TestSeriesWorksDecodesDetailAndIllusts(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, seriesBody))

	page, err := c.SeriesWorks(context.Background(), 77)
	if err != nil {
		t.Fatalf("SeriesWorks: %v", err)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/v1/illust/series" {
		t.Errorf("path = %q", got.path)
	}
	if q := got.query; q.Get("illust_series_id") != "77" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
	}
	if page.Detail.ID != 77 || page.Detail.Title != "a series" ||
		page.Detail.CoverImageURLs.Large != "https://example.com/cover.jpg" ||
		page.Detail.WorkIllustsCount != 2 || !page.Detail.IsConcluded {
		t.Errorf("detail = %+v", page.Detail)
	}
	if len(page.Illusts) != 2 || page.Illusts[1].ID != 8 {
		t.Errorf("illusts = %+v", page.Illusts)
	}
	if page.NextURL == "" {
		t.Error("next_url is empty")
	}
}

func TestSeriesDetailUsesIDPath(t *testing.T) {
	var got recordedRequest
	c := newWriteClient(t, recordInto(&got, seriesBody))

	page, err := c.SeriesDetail(context.Background(), 77)
	if err != nil {
		t.Fatalf("SeriesDetail: %v", err)
	}
	if got.path != "/v1/illust/series/77" {
		t.Errorf("path = %q, want /v1/illust/series/77", got.path)
	}
	if got.query.Get("filter") != "for_android" {
		t.Errorf("query = %v", got.query)
	}
	if page.Detail.ID != 77 || len(page.Illusts) != 2 {
		t.Errorf("page = %+v", page)
	}
}

func TestSeriesPagingPassesNextURL(t *testing.T) {
	var paths []string
	c := newWriteClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		io.WriteString(w, `{"illust_series_detail":{"id":77},"illusts":[],`+
			`"next_url":"http://`+r.Host+`/v1/illust/series?offset=30"}`)
	})
	ctx := context.Background()

	first, err := c.SeriesWorks(ctx, 77)
	if err != nil {
		t.Fatalf("SeriesWorks: %v", err)
	}
	second, err := c.NextSeries(ctx, first.NextURL)
	if err != nil {
		t.Fatalf("NextSeries: %v", err)
	}
	if len(paths) != 2 || paths[1] != "/v1/illust/series" {
		t.Errorf("paths = %v", paths)
	}
	if len(first.Illusts) != 0 || len(second.Illusts) != 0 {
		t.Errorf("empty series lists decoded as %+v / %+v", first.Illusts, second.Illusts)
	}
	if second.Detail.ID != 77 {
		t.Errorf("series detail id = %d, want 77", second.Detail.ID)
	}
}
