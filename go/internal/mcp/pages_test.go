package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// pagedFake is a fakeBackend whose work lists also page: every list returns
// page, and more reads nextPage through NextIllustsPage.
type pagedFake struct {
	*fakeBackend
	page     *pixiv.Page[pixiv.Illustration]
	nextPage *pixiv.Page[pixiv.Illustration]
	pageErr  error
	nextErr  error
	account  int64
	nextURL  string
	calls    []string

	userPage        *pixiv.Page[pixiv.UserPreview]
	nextUserPage    *pixiv.Page[pixiv.UserPreview]
	novelPage       *pixiv.Page[pixiv.Novel]
	nextNovelPage   *pixiv.Page[pixiv.Novel]
	commentPage     *pixiv.Page[pixiv.Comment]
	nextCommentPage *pixiv.Page[pixiv.Comment]
}

func (f *pagedFake) note(name string) { f.calls = append(f.calls, name) }

func (f *pagedFake) called(name string) bool {
	for _, call := range f.calls {
		if call == name {
			return true
		}
	}
	return false
}

func (f *pagedFake) AccountID() int64 { return f.account }

func (f *pagedFake) SearchPage(context.Context, string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("search_illustrations")
	return f.page, f.pageErr
}

func (f *pagedFake) RecommendedPage(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("recommended")
	return f.page, f.pageErr
}

func (f *pagedFake) RankingPage(context.Context, string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("ranking")
	return f.page, f.pageErr
}

func (f *pagedFake) UserIllustsPage(context.Context, int64, string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("user_illusts")
	return f.page, f.pageErr
}

func (f *pagedFake) UserBookmarksPage(context.Context, int64, bool) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("user_bookmarks")
	return f.page, f.pageErr
}

func (f *pagedFake) RelatedIllustsPage(context.Context, int64) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("related_illustrations")
	return f.page, f.pageErr
}

func (f *pagedFake) NewWorksPage(context.Context, string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("new_works")
	return f.page, f.pageErr
}

func (f *pagedFake) FollowingWorksPage(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("following_works")
	return f.page, f.pageErr
}

func (f *pagedFake) MyPixivWorksPage(context.Context, string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("my_pixiv_works")
	return f.page, f.pageErr
}

func (f *pagedFake) NextIllustsPage(_ context.Context, nextURL string) (*pixiv.Page[pixiv.Illustration], error) {
	f.note("next")
	f.nextURL = nextURL
	return f.nextPage, f.nextErr
}

func (f *pagedFake) SearchUsersPage(context.Context, string) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("search_users")
	return f.userPage, f.pageErr
}

func (f *pagedFake) FollowingUsersPage(context.Context, int64) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("following_users")
	return f.userPage, f.pageErr
}

func (f *pagedFake) FollowersPage(context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("followers")
	return f.userPage, f.pageErr
}

func (f *pagedFake) MyPixivUsersPage(context.Context, int64) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("my_pixiv_users")
	return f.userPage, f.pageErr
}

func (f *pagedFake) RelatedUsersPage(context.Context, int64) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("related_users")
	return f.userPage, f.pageErr
}

func (f *pagedFake) RecommendedUsersPage(context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("recommended_users")
	return f.userPage, f.pageErr
}

func (f *pagedFake) NextUserPreviewsPage(_ context.Context, nextURL string) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("next_users")
	f.nextURL = nextURL
	return f.nextUserPage, f.nextErr
}

func (f *pagedFake) NextRecommendedUsersPage(_ context.Context, nextURL string) (*pixiv.Page[pixiv.UserPreview], error) {
	f.note("next_recommended")
	f.nextURL = nextURL
	return f.nextUserPage, f.nextErr
}

func (f *pagedFake) SearchNovelsPage(context.Context, string) (*pixiv.Page[pixiv.Novel], error) {
	f.note("search_novels")
	return f.novelPage, f.pageErr
}

func (f *pagedFake) NextNovelsPage(_ context.Context, nextURL string) (*pixiv.Page[pixiv.Novel], error) {
	f.note("next_novels")
	f.nextURL = nextURL
	return f.nextNovelPage, f.nextErr
}

func (f *pagedFake) CommentsPage(context.Context, int64) (*pixiv.Page[pixiv.Comment], error) {
	f.note("comments")
	return f.commentPage, f.pageErr
}

func (f *pagedFake) NovelCommentsPage(context.Context, int64) (*pixiv.Page[pixiv.Comment], error) {
	f.note("novel_comments")
	return f.commentPage, f.pageErr
}

func (f *pagedFake) NextCommentsPage(_ context.Context, nextURL string) (*pixiv.Page[pixiv.Comment], error) {
	f.note("next_comments")
	f.nextURL = nextURL
	return f.nextCommentPage, f.nextErr
}

// cursorFromText is the nextCursor line a list tool appended, or "".
func cursorFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "nextCursor: "); ok {
			return rest
		}
	}
	return ""
}

// TestPagedListToolsReturnCursor checks every pageable work list hands out a
// cursor, and keeps the works beyond limit in it rather than dropping them.
func TestPagedListToolsReturnCursor(t *testing.T) {
	page := &pixiv.Page[pixiv.Illustration]{
		Items:   []pixiv.Illustration{{ID: 1, Title: "one"}, {ID: 2, Title: "two"}},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=2",
	}
	cases := []struct{ name, args string }{
		{"search_illustrations", `{"query":"rain","limit":1}`},
		{"recommended", `{"limit":1}`},
		{"ranking", `{"limit":1}`},
		{"user_illusts", `{"id":7,"limit":1}`},
		{"user_bookmarks", `{"limit":1}`},
		{"related_illustrations", `{"id":9,"limit":1}`},
		{"new_works", `{"limit":1}`},
		{"following_works", `{"limit":1}`},
		{"my_pixiv_works", `{"limit":1}`},
	}
	for _, tc := range cases {
		b := &pagedFake{fakeBackend: &fakeBackend{}, page: page}
		text := callToolText(t, newTestServer(b), tc.name, tc.args)
		if !strings.Contains(text, "#1 ") || strings.Contains(text, "#2 ") {
			t.Errorf("%s: text %q, want only the first work", tc.name, text)
		}
		if cursorFromText(text) == "" {
			t.Errorf("%s: text %q misses nextCursor", tc.name, text)
		}
		if !b.called(tc.name) {
			t.Errorf("%s did not use the paged backend method", tc.name)
		}
	}
}

// TestMoreReadsPendingThenNextPage checks more first returns the works the
// limit left behind, then fetches Pixiv's next page with the stored URL.
func TestMoreReadsPendingThenNextPage(t *testing.T) {
	first := &pixiv.Page[pixiv.Illustration]{
		Items:   []pixiv.Illustration{{ID: 1}, {ID: 2}, {ID: 3}},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=3",
	}
	second := &pixiv.Page[pixiv.Illustration]{Items: []pixiv.Illustration{{ID: 4}}}
	b := &pagedFake{fakeBackend: &fakeBackend{}, page: first, nextPage: second}
	h := newTestServer(b)

	cursor := cursorFromText(callToolText(t, h, "search_illustrations", `{"query":"rain","limit":2}`))
	if cursor == "" {
		t.Fatal("first page has no cursor")
	}
	more := callToolText(t, h, "more", fmt.Sprintf(`{"cursor":%q,"count":2}`, cursor))
	if !strings.Contains(more, "#3 ") {
		t.Errorf("more text %q, want the work limit left behind", more)
	}
	if b.called("next") {
		t.Error("more fetched the next page before returning the pending work")
	}
	cursor = cursorFromText(more)
	if cursor == "" {
		t.Fatal("pending page has no cursor for the next page")
	}
	more = callToolText(t, h, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#4 ") {
		t.Errorf("more text %q, want the next page's work", more)
	}
	if b.nextURL != "https://app-api.pixiv.net/v1/next?offset=3" {
		t.Errorf("more fetched %q", b.nextURL)
	}
	if cursorFromText(more) != "" {
		t.Errorf("last page still has a cursor: %q", more)
	}
}

// TestMoreRejectsBadCursor checks a forged, missing or expired cursor is an
// invalid-params error, not a panic.
func TestMoreRejectsBadCursor(t *testing.T) {
	b := &pagedFake{fakeBackend: &fakeBackend{}, page: &pixiv.Page[pixiv.Illustration]{
		Items:   []pixiv.Illustration{{ID: 1}},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=1",
	}}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "recommended", `{}`))
	if cursor == "" {
		t.Fatal("no cursor to test with")
	}

	for _, tc := range []struct{ name, args, want string }{
		{"missing", `{}`, "cursor is required"},
		{"forged", `{"cursor":"pixeval:forged"}`, "invalid"},
		{"expired", fmt.Sprintf(`{"cursor":%q}`, cursor), "expired"},
	} {
		if tc.name == "expired" {
			// The cursor was minted just now; move the store's clock past its TTL.
			srv.cursors.now = func() time.Time { return time.Now().Add(cursorTTL + time.Second) }
		}
		r := call(t, srv, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"more","arguments":%s}}`, tc.args))
		if r.Error == nil || r.Error.Code != codeInvalidParams || !strings.Contains(r.Error.Message, tc.want) {
			t.Errorf("%s cursor: reply %+v, want %q", tc.name, r, tc.want)
		}
	}
}

// TestMoreRejectsAnotherAccount checks a cursor is not readable after the
// signed-in Pixiv account changes.
func TestMoreRejectsAnotherAccount(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		account:     7,
		page: &pixiv.Page[pixiv.Illustration]{
			Items:   []pixiv.Illustration{{ID: 1}},
			NextURL: "https://app-api.pixiv.net/v1/next?offset=1",
		},
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "recommended", `{}`))
	if cursor == "" {
		t.Fatal("no cursor to test with")
	}
	b.account = 8
	r := call(t, srv, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"more","arguments":{"cursor":%q}}}`, cursor))
	if r.Error == nil || r.Error.Code != codeInvalidParams || !strings.Contains(r.Error.Message, "another Pixiv account") {
		t.Fatalf("reply %+v, want an account error", r)
	}
}

// TestPagedListErrorIsResult checks a paging failure is still a tool result.
func TestPagedListErrorIsResult(t *testing.T) {
	b := &pagedFake{fakeBackend: &fakeBackend{}, pageErr: errors.New("boom")}
	body := `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"search_illustrations","arguments":{"query":"rain"}}}`
	var got toolResult
	decodeResult(t, call(t, newTestServer(b), body), &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "boom") {
		t.Fatalf("result %+v", got)
	}
}

// TestMoreNextPageErrorIsResult checks a failed next page is a tool result.
func TestMoreNextPageErrorIsResult(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		page: &pixiv.Page[pixiv.Illustration]{
			Items:   []pixiv.Illustration{{ID: 1}},
			NextURL: "https://app-api.pixiv.net/v1/next?offset=1",
		},
		nextErr: errors.New("boom"),
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "recommended", `{}`))
	if cursor == "" {
		t.Fatal("no cursor to test with")
	}
	var got toolResult
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"more","arguments":{"cursor":%q}}}`, cursor)
	decodeResult(t, call(t, srv, body), &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "boom") {
		t.Fatalf("result %+v", got)
	}
}

// TestMoreKeepsCursorAfterFetchFailure checks a failed next page leaves the
// cursor readable, so the caller retries with the same token instead of
// restarting the list, and that a successful retry still consumes it.
func TestMoreKeepsCursorAfterFetchFailure(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		page: &pixiv.Page[pixiv.Illustration]{
			Items:   []pixiv.Illustration{{ID: 1}},
			NextURL: "https://app-api.pixiv.net/v1/next?offset=1",
		},
		nextErr: errors.New("boom"),
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "recommended", `{}`))
	if cursor == "" {
		t.Fatal("no cursor to test with")
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"more","arguments":{"cursor":%q}}}`, cursor)

	var failed toolResult
	decodeResult(t, call(t, srv, body), &failed)
	if !failed.IsError || !strings.Contains(failed.Content[0].Text, "retry with the same cursor") {
		t.Fatalf("failed result %+v, want a retry hint", failed)
	}

	b.nextErr = nil
	b.nextPage = &pixiv.Page[pixiv.Illustration]{Items: []pixiv.Illustration{{ID: 2}}}
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#2 ") {
		t.Errorf("more text %q, want the retried page's work", more)
	}

	r := call(t, srv, body)
	if r.Error == nil || r.Error.Code != codeInvalidParams || !strings.Contains(r.Error.Message, "invalid") {
		t.Fatalf("reply %+v, want the consumed cursor to be invalid", r)
	}
}

// TestMoreKeepsLastPageTail checks a page with no next URL still keeps the
// works limit cut, so a short limit never drops the end of the list.
func TestMoreKeepsLastPageTail(t *testing.T) {
	b := &pagedFake{fakeBackend: &fakeBackend{}, page: &pixiv.Page[pixiv.Illustration]{
		Items: []pixiv.Illustration{{ID: 1}, {ID: 2}},
	}}
	h := newTestServer(b)
	cursor := cursorFromText(callToolText(t, h, "recommended", `{"limit":1}`))
	if cursor == "" {
		t.Fatal("the work after limit was dropped")
	}
	more := callToolText(t, h, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#2 ") {
		t.Errorf("more text %q, want the work limit left behind", more)
	}
	if b.called("next") {
		t.Error("more fetched a page that does not exist")
	}
}

// TestPagedToolsAreDefined checks capabilities does not name a tool that is
// missing from tools/list.
func TestPagedToolsAreDefined(t *testing.T) {
	names := map[string]bool{}
	for _, def := range toolDefs {
		names[def.Name] = true
	}
	for _, name := range pagedTools {
		if !names[name] {
			t.Errorf("pagedTools names unknown tool %q", name)
		}
	}
}

// TestUnpagedBackendHasNoCursor checks a backend without paging keeps the old
// output and never fakes a cursor.
func TestUnpagedBackendHasNoCursor(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	text := callToolText(t, newTestServer(b), "search_illustrations", `{"query":"rain"}`)
	if strings.Contains(text, "nextCursor") {
		t.Errorf("unpaged backend returned a cursor: %q", text)
	}
	if !strings.Contains(text, "#101 ") {
		t.Errorf("text %q misses the work", text)
	}
}

// TestPagedUserListToolsReturnCursor checks every pageable user list hands out
// a cursor, and keeps the users beyond limit in it rather than dropping them.
func TestPagedUserListToolsReturnCursor(t *testing.T) {
	page := &pixiv.Page[pixiv.UserPreview]{
		Items: []pixiv.UserPreview{
			{User: pixiv.User{ID: 1, Name: "one"}},
			{User: pixiv.User{ID: 2, Name: "two"}},
		},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=2",
	}
	cases := []struct{ name, args string }{
		{"search_users", `{"word":"a","limit":1}`},
		{"following_users", `{"id":7,"limit":1}`},
		{"followers", `{"limit":1}`},
		{"my_pixiv_users", `{"id":7,"limit":1}`},
		{"related_users", `{"id":7,"limit":1}`},
		{"recommended_users", `{"limit":1}`},
	}
	for _, tc := range cases {
		b := &pagedFake{fakeBackend: &fakeBackend{}, userPage: page}
		text := callToolText(t, newTestServer(b), tc.name, tc.args)
		if !strings.Contains(text, "#1 ") || strings.Contains(text, "#2 ") {
			t.Errorf("%s: text %q, want only the first user", tc.name, text)
		}
		if cursorFromText(text) == "" {
			t.Errorf("%s: text %q misses nextCursor", tc.name, text)
		}
		if !b.called(tc.name) {
			t.Errorf("%s did not use the paged backend method", tc.name)
		}
	}
}

// TestPagedNovelSearchReturnsCursor checks the novel search pages and keeps
// the novels beyond limit in its cursor.
func TestPagedNovelSearchReturnsCursor(t *testing.T) {
	b := &pagedFake{fakeBackend: &fakeBackend{}, novelPage: &pixiv.Page[pixiv.Novel]{
		Items:   []pixiv.Novel{{ID: 1, Title: "one"}, {ID: 2, Title: "two"}},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=2",
	}}
	text := callToolText(t, newTestServer(b), "search_novels", `{"word":"a","limit":1}`)
	if !strings.Contains(text, "#1 ") || strings.Contains(text, "#2 ") {
		t.Errorf("text %q, want only the first novel", text)
	}
	if cursorFromText(text) == "" {
		t.Errorf("text %q misses nextCursor", text)
	}
	if !b.called("search_novels") {
		t.Error("search_novels did not use the paged backend method")
	}
}

// TestPagedCommentToolsReturnCursor checks both comment lists page and keep
// the comments beyond limit in their cursor.
func TestPagedCommentToolsReturnCursor(t *testing.T) {
	page := &pixiv.Page[pixiv.Comment]{
		Items:   []pixiv.Comment{{ID: 1, Comment: "one"}, {ID: 2, Comment: "two"}},
		NextURL: "https://app-api.pixiv.net/v1/next?offset=2",
	}
	for _, name := range []string{"comments", "novel_comments"} {
		b := &pagedFake{fakeBackend: &fakeBackend{}, commentPage: page}
		text := callToolText(t, newTestServer(b), name, `{"id":7,"limit":1}`)
		if !strings.Contains(text, "#1 ") || strings.Contains(text, "#2 ") {
			t.Errorf("%s: text %q, want only the first comment", name, text)
		}
		if cursorFromText(text) == "" {
			t.Errorf("%s: text %q misses nextCursor", name, text)
		}
		if !b.called(name) {
			t.Errorf("%s did not use the paged backend method", name)
		}
	}
}

// TestMoreUserPageReadsPendingThenNext checks a user cursor first returns the
// users the limit left behind, then fetches the next Pixiv page with the
// stored URL.
func TestMoreUserPageReadsPendingThenNext(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		userPage: &pixiv.Page[pixiv.UserPreview]{
			Items: []pixiv.UserPreview{
				{User: pixiv.User{ID: 1}},
				{User: pixiv.User{ID: 2}},
				{User: pixiv.User{ID: 3}},
			},
			NextURL: "https://app-api.pixiv.net/v1/user/following?offset=3",
		},
		nextUserPage: &pixiv.Page[pixiv.UserPreview]{
			Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}},
		},
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "following_users", `{"id":7,"limit":2}`))
	if cursor == "" {
		t.Fatal("first page has no cursor")
	}
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#3 ") {
		t.Errorf("more text %q, want the user limit left behind", more)
	}
	if b.called("next_users") {
		t.Error("more fetched the next page before returning the pending user")
	}
	cursor = cursorFromText(more)
	if cursor == "" {
		t.Fatal("pending page has no cursor for the next page")
	}
	more = callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#4 ") {
		t.Errorf("more text %q, want the next page's user", more)
	}
	if b.nextURL != "https://app-api.pixiv.net/v1/user/following?offset=3" {
		t.Errorf("more fetched %q", b.nextURL)
	}
	if cursorFromText(more) != "" {
		t.Errorf("last page still has a cursor: %q", more)
	}
}

// TestMoreRecommendedUsersUsesRecommendedPage checks recommended users page
// through their own endpoint, whose pages carry plain users.
func TestMoreRecommendedUsersUsesRecommendedPage(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		userPage: &pixiv.Page[pixiv.UserPreview]{
			Items:   []pixiv.UserPreview{{User: pixiv.User{ID: 1}}, {User: pixiv.User{ID: 2}}},
			NextURL: "https://app-api.pixiv.net/v1/user/recommended?offset=2",
		},
		nextUserPage: &pixiv.Page[pixiv.UserPreview]{
			Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}},
		},
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "recommended_users", `{"limit":2}`))
	if cursor == "" {
		t.Fatal("first page has no cursor")
	}
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#4 ") {
		t.Errorf("more text %q, want the next page's user", more)
	}
	if !b.called("next_recommended") || b.called("next_users") {
		t.Errorf("recommended users used calls %v, want next_recommended", b.calls)
	}
}

// TestMoreNovelPageReadsPendingThenNext checks a novel cursor returns the
// novels beyond limit, then fetches the next page.
func TestMoreNovelPageReadsPendingThenNext(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		novelPage: &pixiv.Page[pixiv.Novel]{
			Items:   []pixiv.Novel{{ID: 1}, {ID: 2}, {ID: 3}},
			NextURL: "https://app-api.pixiv.net/v1/search/novel?offset=3",
		},
		nextNovelPage: &pixiv.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 4}}},
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "search_novels", `{"word":"a","limit":2}`))
	if cursor == "" {
		t.Fatal("first page has no cursor")
	}
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#3 ") {
		t.Errorf("more text %q, want the novel limit left behind", more)
	}
	cursor = cursorFromText(more)
	if cursor == "" {
		t.Fatal("pending page has no cursor for the next page")
	}
	more = callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#4 ") {
		t.Errorf("more text %q, want the next page's novel", more)
	}
	if !b.called("next_novels") {
		t.Error("more did not use the novel next page method")
	}
	if cursorFromText(more) != "" {
		t.Errorf("last page still has a cursor: %q", more)
	}
}

// TestMoreCommentPageReadsPendingThenNext checks a comment cursor returns the
// comments beyond limit, then fetches the next page.
func TestMoreCommentPageReadsPendingThenNext(t *testing.T) {
	b := &pagedFake{
		fakeBackend: &fakeBackend{},
		commentPage: &pixiv.Page[pixiv.Comment]{
			Items:   []pixiv.Comment{{ID: 1}, {ID: 2}, {ID: 3}},
			NextURL: "https://app-api.pixiv.net/v1/illust/comments?offset=3",
		},
		nextCommentPage: &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 4}}},
	}
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, "comments", `{"id":7,"limit":2}`))
	if cursor == "" {
		t.Fatal("first page has no cursor")
	}
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#3 ") {
		t.Errorf("more text %q, want the comment limit left behind", more)
	}
	cursor = cursorFromText(more)
	if cursor == "" {
		t.Fatal("pending page has no cursor for the next page")
	}
	more = callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, "#4 ") {
		t.Errorf("more text %q, want the next page's comment", more)
	}
	if !b.called("next_comments") {
		t.Error("more did not use the comment next page method")
	}
	if cursorFromText(more) != "" {
		t.Errorf("last page still has a cursor: %q", more)
	}
}

// retriedMore checks a failed next page keeps the cursor, and the retry under
// the same token loads the page and consumes the cursor.
func retriedMore(t *testing.T, b *pagedFake, tool, args, want string) {
	t.Helper()
	srv := newTestServer(b)
	cursor := cursorFromText(callToolText(t, srv, tool, args))
	if cursor == "" {
		t.Fatalf("%s: no cursor to test with", tool)
	}
	b.nextErr = errors.New("boom")
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"more","arguments":{"cursor":%q}}}`, cursor)
	var failed toolResult
	decodeResult(t, call(t, srv, body), &failed)
	if !failed.IsError || !strings.Contains(failed.Content[0].Text, "retry with the same cursor") {
		t.Fatalf("%s: failed result %+v, want a retry hint", tool, failed)
	}
	b.nextErr = nil
	more := callToolText(t, srv, "more", fmt.Sprintf(`{"cursor":%q}`, cursor))
	if !strings.Contains(more, want) {
		t.Errorf("%s: more text %q, want %q", tool, more, want)
	}
	r := call(t, srv, body)
	if r.Error == nil || r.Error.Code != codeInvalidParams || !strings.Contains(r.Error.Message, "invalid") {
		t.Fatalf("%s: reply %+v, want the consumed cursor to be invalid", tool, r)
	}
}

// TestMoreNewKindsKeepCursorAfterFetchFailure checks every new list kind can
// retry a failed next page with the same cursor.
func TestMoreNewKindsKeepCursorAfterFetchFailure(t *testing.T) {
	t.Run("users", func(t *testing.T) {
		b := &pagedFake{
			fakeBackend: &fakeBackend{},
			userPage: &pixiv.Page[pixiv.UserPreview]{
				Items:   []pixiv.UserPreview{{User: pixiv.User{ID: 1}}},
				NextURL: "https://app-api.pixiv.net/v1/search/user?offset=1",
			},
			nextUserPage: &pixiv.Page[pixiv.UserPreview]{
				Items: []pixiv.UserPreview{{User: pixiv.User{ID: 4}}},
			},
		}
		retriedMore(t, b, "search_users", `{"word":"a"}`, "#4 ")
	})
	t.Run("novels", func(t *testing.T) {
		b := &pagedFake{
			fakeBackend: &fakeBackend{},
			novelPage: &pixiv.Page[pixiv.Novel]{
				Items:   []pixiv.Novel{{ID: 1}},
				NextURL: "https://app-api.pixiv.net/v1/search/novel?offset=1",
			},
			nextNovelPage: &pixiv.Page[pixiv.Novel]{Items: []pixiv.Novel{{ID: 4}}},
		}
		retriedMore(t, b, "search_novels", `{"word":"a"}`, "#4 ")
	})
	t.Run("comments", func(t *testing.T) {
		b := &pagedFake{
			fakeBackend: &fakeBackend{},
			commentPage: &pixiv.Page[pixiv.Comment]{
				Items:   []pixiv.Comment{{ID: 1}},
				NextURL: "https://app-api.pixiv.net/v1/illust/comments?offset=1",
			},
			nextCommentPage: &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{{ID: 4}}},
		}
		retriedMore(t, b, "comments", `{"id":7}`, "#4 ")
	})
}

// TestUnpagedNewListsHaveNoCursor checks a backend without the new optional
// interfaces keeps the old one-page output for users, novels and comments.
func TestUnpagedNewListsHaveNoCursor(t *testing.T) {
	users := []pixiv.UserPreview{{User: pixiv.User{ID: 7, Name: "Ame"}}}
	b := &fakeBackend{
		users:     users,
		recommend: users,
		novels:    []pixiv.Novel{{ID: 8, Title: "Novel"}},
		comments:  []pixiv.Comment{{ID: 9, Comment: "Comment"}},
		illustCmt: []pixiv.Comment{{ID: 9, Comment: "Comment"}},
	}
	srv := newTestServer(b)
	cases := []struct{ name, args, want string }{
		{"search_users", `{"word":"a"}`, "#7 Ame"},
		{"following_users", `{}`, "#7 Ame"},
		{"followers", `{}`, "#7 Ame"},
		{"my_pixiv_users", `{"id":7}`, "#7 Ame"},
		{"related_users", `{"id":7}`, "#7 Ame"},
		{"recommended_users", `{}`, "#7 Ame"},
		{"search_novels", `{"word":"a"}`, "#8 "},
		{"comments", `{"id":1}`, "#9 "},
		{"novel_comments", `{"id":1}`, "#9 "},
	}
	for _, tc := range cases {
		text := callToolText(t, srv, tc.name, tc.args)
		if strings.Contains(text, "nextCursor") {
			t.Errorf("%s: unpaged backend returned a cursor: %q", tc.name, text)
		}
		if !strings.Contains(text, tc.want) {
			t.Errorf("%s: text %q misses %q", tc.name, text, tc.want)
		}
	}
}
