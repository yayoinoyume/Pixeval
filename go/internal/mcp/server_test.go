package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeBackend answers the tools without the network and records the calls.
type fakeBackend struct {
	works          []pixiv.Illustration
	users          []pixiv.UserPreview
	user           *pixiv.UserDetail
	novels         []pixiv.Novel
	comments       []pixiv.Comment
	saved          []SavedWork
	downloads      []DownloadedWork
	subs           []SubscriptionInfo
	articles       []pixiv.SpotlightArticle
	series         []pixiv.SeriesSummary
	tags           []pixiv.TrendingTag
	summary        string
	macro          string
	history        []HistoryWork
	novelBody      string
	recommend      []pixiv.UserPreview
	illustCmt      []pixiv.Comment
	userDetailList []pixiv.UserDetail
	dlTasks        []DownloadTask
	err            error

	searchQuery string
	searchLimit int
	limit       int
	mode        string
	detailID    int64
	bookmarkID  int64
	bookmarkOn  bool
	queuedID    int64
	queued      int
	status      string

	searchUsersWord  string
	userDetailID     int64
	userIllustsID    int64
	userIllustsKind  string
	bookmarksID      int64
	bookmarksPrivate bool
	novelsWord       string
	novelCommentsID  int64
	relatedID        int64
	newKind          string
	myPixivKind      string
	followingUsersID int64
	followersCalls   int
	myPixivUsersID   int64
	relatedUsersID   int64
	seriesKind       string
	novelContentID   int64
	commentsID       int64
	worksIDs         []int64
	usersIDs         []int64

	watchLaterID   int64
	watchLaterOn   bool
	followID       int64
	followOn       bool
	followPrivate  bool
	controlTaskID  string
	controlAction  string
	addSubKind     string
	addSubID       int64
	addSubName     string
	removeSubKind  string
	removeSubID    int64
	syncCalls      int
	syncNew        int
	addCommentID   int64
	addCommentText string
	deleteCmtID    int64
	setMacroText   string
}

func (f *fakeBackend) Search(_ context.Context, query string, limit int) ([]pixiv.Illustration, error) {
	f.searchQuery, f.searchLimit = query, limit
	return f.works, f.err
}

func (f *fakeBackend) Recommended(_ context.Context, limit int) ([]pixiv.Illustration, error) {
	f.limit = limit
	return f.works, f.err
}

func (f *fakeBackend) Detail(_ context.Context, id int64) (*pixiv.Illustration, error) {
	f.detailID = id
	if f.err != nil {
		return nil, f.err
	}
	if len(f.works) == 0 {
		return nil, errors.New("no work")
	}
	w := f.works[0]
	return &w, nil
}

func (f *fakeBackend) Ranking(_ context.Context, mode string, limit int) ([]pixiv.Illustration, error) {
	f.mode, f.limit = mode, limit
	return f.works, f.err
}

func (f *fakeBackend) SetBookmark(_ context.Context, id int64, on bool) error {
	f.bookmarkID, f.bookmarkOn = id, on
	return f.err
}

func (f *fakeBackend) QueueDownload(_ context.Context, id int64) (int, error) {
	f.queuedID = id
	return f.queued, f.err
}

func (f *fakeBackend) Status() string { return f.status }

func (f *fakeBackend) SearchUsers(_ context.Context, word string) ([]pixiv.UserPreview, error) {
	f.searchUsersWord = word
	return f.users, f.err
}

func (f *fakeBackend) UserDetail(_ context.Context, id int64) (*pixiv.UserDetail, error) {
	f.userDetailID = id
	if f.err != nil {
		return nil, f.err
	}
	if f.user == nil {
		return nil, errors.New("no user")
	}
	return f.user, nil
}

func (f *fakeBackend) UserIllusts(_ context.Context, id int64, kind string) ([]pixiv.Illustration, error) {
	f.userIllustsID, f.userIllustsKind = id, kind
	return f.works, f.err
}

func (f *fakeBackend) UserBookmarks(_ context.Context, userID int64, private bool) ([]pixiv.Illustration, error) {
	f.bookmarksID, f.bookmarksPrivate = userID, private
	return f.works, f.err
}

func (f *fakeBackend) SearchNovels(_ context.Context, word string) ([]pixiv.Novel, error) {
	f.novelsWord = word
	return f.novels, f.err
}

func (f *fakeBackend) NovelComments(_ context.Context, novelID int64) ([]pixiv.Comment, error) {
	f.novelCommentsID = novelID
	return f.comments, f.err
}

func (f *fakeBackend) WatchLater(_ context.Context) ([]SavedWork, error) {
	return f.saved, f.err
}

func (f *fakeBackend) DownloadHistory(_ context.Context) ([]DownloadedWork, error) {
	return f.downloads, f.err
}

func (f *fakeBackend) Subscriptions(_ context.Context) ([]SubscriptionInfo, error) {
	return f.subs, f.err
}

func (f *fakeBackend) RelatedIllusts(_ context.Context, id int64) ([]pixiv.Illustration, error) {
	f.relatedID = id
	return f.works, f.err
}

func (f *fakeBackend) Spotlight(_ context.Context) ([]pixiv.SpotlightArticle, error) {
	return f.articles, f.err
}

func (f *fakeBackend) NewWorks(_ context.Context, kind string) ([]pixiv.Illustration, error) {
	f.newKind = kind
	return f.works, f.err
}

func (f *fakeBackend) FollowingWorks(_ context.Context) ([]pixiv.Illustration, error) {
	return f.works, f.err
}

func (f *fakeBackend) MyPixivWorks(_ context.Context, kind string) ([]pixiv.Illustration, error) {
	f.myPixivKind = kind
	return f.works, f.err
}

func (f *fakeBackend) FollowingUsers(_ context.Context, userID int64) ([]pixiv.UserPreview, error) {
	f.followingUsersID = userID
	return f.users, f.err
}

func (f *fakeBackend) Followers(_ context.Context) ([]pixiv.UserPreview, error) {
	f.followersCalls++
	return f.users, f.err
}

func (f *fakeBackend) MyPixivUsers(_ context.Context, userID int64) ([]pixiv.UserPreview, error) {
	f.myPixivUsersID = userID
	return f.users, f.err
}

func (f *fakeBackend) RelatedUsers(_ context.Context, userID int64) ([]pixiv.UserPreview, error) {
	f.relatedUsersID = userID
	return f.users, f.err
}

func (f *fakeBackend) SeriesWatchlist(_ context.Context, kind string) ([]pixiv.SeriesSummary, error) {
	f.seriesKind = kind
	return f.series, f.err
}

func (f *fakeBackend) TrendingTags(_ context.Context) ([]pixiv.TrendingTag, error) {
	return f.tags, f.err
}

func (f *fakeBackend) SettingsSummary(_ context.Context) (string, error) {
	return f.summary, f.err
}

func (f *fakeBackend) DownloadMacro(_ context.Context) (string, error) {
	return f.macro, f.err
}

func (f *fakeBackend) History(_ context.Context) ([]HistoryWork, error) {
	return f.history, f.err
}

func (f *fakeBackend) NovelContent(_ context.Context, id int64) (string, error) {
	f.novelContentID = id
	return f.novelBody, f.err
}

func (f *fakeBackend) RecommendedUsers(_ context.Context) ([]pixiv.UserPreview, error) {
	return f.recommend, f.err
}

func (f *fakeBackend) Comments(_ context.Context, illustID int64) ([]pixiv.Comment, error) {
	f.commentsID = illustID
	return f.illustCmt, f.err
}

func (f *fakeBackend) Works(_ context.Context, ids []int64) ([]pixiv.Illustration, error) {
	f.worksIDs = ids
	return f.works, f.err
}

func (f *fakeBackend) Users(_ context.Context, ids []int64) ([]pixiv.UserDetail, error) {
	f.usersIDs = ids
	return f.userDetailList, f.err
}

func (f *fakeBackend) DownloadTasks(_ context.Context) ([]DownloadTask, error) {
	return f.dlTasks, f.err
}

func (f *fakeBackend) SetWatchLater(_ context.Context, id int64, on bool) error {
	f.watchLaterID, f.watchLaterOn = id, on
	return f.err
}

func (f *fakeBackend) FollowUser(_ context.Context, userID int64, on, private bool) error {
	f.followID, f.followOn, f.followPrivate = userID, on, private
	return f.err
}

func (f *fakeBackend) ControlDownload(_ context.Context, taskID, action string) error {
	f.controlTaskID, f.controlAction = taskID, action
	return f.err
}

func (f *fakeBackend) AddSubscription(_ context.Context, kind string, targetID int64, name string) error {
	f.addSubKind, f.addSubID, f.addSubName = kind, targetID, name
	return f.err
}

func (f *fakeBackend) RemoveSubscription(_ context.Context, kind string, targetID int64) error {
	f.removeSubKind, f.removeSubID = kind, targetID
	return f.err
}

func (f *fakeBackend) SyncSubscriptions(_ context.Context) (int, error) {
	f.syncCalls++
	return f.syncNew, f.err
}

func (f *fakeBackend) AddComment(_ context.Context, illustID int64, text string) (*pixiv.Comment, error) {
	f.addCommentID, f.addCommentText = illustID, text
	if f.err != nil {
		return nil, f.err
	}
	return &pixiv.Comment{ID: 77, Comment: text}, nil
}

func (f *fakeBackend) DeleteComment(_ context.Context, commentID int64) error {
	f.deleteCmtID = commentID
	return f.err
}

func (f *fakeBackend) SetDownloadMacro(_ context.Context, text string) error {
	f.setMacroText = text
	return f.err
}

// reply is a response with the fields a test checks left raw.
type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// testToken is the bearer token the test servers require and the test client
// sends.
const testToken = "test-token"

// newTestServer is a server whose client knows testToken.
func newTestServer(b Backend) *Server {
	return New(b, testToken)
}

// call posts body to the server and decodes the reply.
func call(t *testing.T, h http.Handler, body string) reply {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	var r reply
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return r
}

// toolResult is the result of a tools/call.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func decodeResult(t *testing.T, r reply, out any) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		t.Fatalf("decode result %s: %v", r.Result, err)
	}
}

func sampleWorks() []pixiv.Illustration {
	return []pixiv.Illustration{
		{
			ID:             101,
			Title:          "Rain",
			User:           pixiv.User{Name: "Ame"},
			Tags:           []pixiv.Tag{{Name: "original"}},
			TotalBookmarks: 42,
			Width:          800,
			Height:         1200,
			PageCount:      2,
		},
	}
}

func TestInitialize(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	r := call(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var got struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	decodeResult(t, r, &got)
	if got.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocolVersion %q", got.ProtocolVersion)
	}
	if _, ok := got.Capabilities["tools"]; !ok {
		t.Errorf("capabilities %v, want tools", got.Capabilities)
	}
	if got.ServerInfo.Name != "pixeval-go" || got.ServerInfo.Version != "0.1.0" {
		t.Errorf("serverInfo %+v", got.ServerInfo)
	}
}

func TestToolsList(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	r := call(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var got struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	decodeResult(t, r, &got)
	want := []string{
		"search_illustrations", "recommended", "illustration_detail", "ranking",
		"set_bookmark", "queue_download", "status",
		"search_users", "user_detail", "user_illusts", "user_bookmarks",
		"search_novels", "novel_comments", "watch_later", "download_history", "subscriptions",
		"related_illustrations", "spotlight",
		"new_works", "following_works", "my_pixiv_works",
		"following_users", "followers", "my_pixiv_users", "related_users",
		"series_watchlist", "trending_tags", "settings_summary",
		"capabilities", "help", "download_macro", "analyze_download_macro",
		"history", "extensions", "novel_content", "recommended_users", "comments",
		"works", "users", "download_tasks", "analyze_work_filter",
		"set_watch_later", "follow_user", "control_download",
		"add_subscription", "remove_subscription", "sync_subscriptions",
		"add_comment", "delete_comment", "set_download_macro", "more",
	}
	if len(got.Tools) != len(want) {
		t.Fatalf("%d tools, want %d", len(got.Tools), len(want))
	}
	for i, name := range want {
		if got.Tools[i].Name != name {
			t.Errorf("tool %d is %q, want %q", i, got.Tools[i].Name, name)
		}
		if len(got.Tools[i].InputSchema) == 0 {
			t.Errorf("%s has no input schema", name)
		}
	}
}

func TestSearchCall(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":3,"method":"tools/call",
		"params":{"name":"search_illustrations","arguments":{"query":"rain","limit":3}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if b.searchQuery != "rain" || b.searchLimit != 3 {
		t.Errorf("backend got query %q limit %d", b.searchQuery, b.searchLimit)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" {
		t.Fatalf("content %+v", got.Content)
	}
	text := got.Content[0].Text
	for _, want := range []string{"#101", "Rain", "Ame", "https://www.pixiv.net/artworks/101"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q misses %q", text, want)
		}
	}
}

func TestSearchDefaultLimit(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":4,"method":"tools/call",
		"params":{"name":"search_illustrations","arguments":{"query":"rain"}}}`)
	decodeResult(t, r, &toolResult{})
	if b.searchLimit != defaultLimit {
		t.Errorf("limit %d, want %d", b.searchLimit, defaultLimit)
	}
}

func TestToolErrorIsResult(t *testing.T) {
	b := &fakeBackend{err: errors.New("boom")}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":5,"method":"tools/call",
		"params":{"name":"recommended","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError {
		t.Fatalf("isError false: %+v", got)
	}
	if len(got.Content) != 1 || !strings.Contains(got.Content[0].Text, "boom") {
		t.Errorf("content %+v", got.Content)
	}
}

func TestUnknownTool(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":6,"method":"tools/call",
		"params":{"name":"nope","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestUnknownMethod(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
	if r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Fatalf("reply %+v", r)
	}
}

func TestParseError(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":8,`)
	if r.Error == nil || r.Error.Code != codeParseError {
		t.Fatalf("reply %+v", r)
	}
	if string(r.ID) != "null" {
		t.Errorf("id %q, want null", r.ID)
	}
}

func TestNotificationHasNoBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	newTestServer(&fakeBackend{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("status %d, want %d", rec.Code, http.StatusAccepted)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body %q, want empty", rec.Body.String())
	}
}

func TestGetIsMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	newTestServer(&fakeBackend{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d", rec.Code)
	}
}

// callToolText runs a tool and returns its text, failing on an isError result.
func callToolText(t *testing.T, h http.Handler, name, args string) string {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args)
	var got toolResult
	decodeResult(t, call(t, h, body), &got)
	if got.IsError {
		t.Fatalf("%s returned an error: %+v", name, got.Content)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" {
		t.Fatalf("%s content %+v", name, got.Content)
	}
	return got.Content[0].Text
}

func TestReadToolsCallBackend(t *testing.T) {
	savedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	b := &fakeBackend{
		works: sampleWorks(),
		users: []pixiv.UserPreview{{User: pixiv.User{ID: 7, Name: "Ame", Account: "ame", Comment: "hello"}}},
		user: &pixiv.UserDetail{
			User: pixiv.User{ID: 7, Name: "Ame", Account: "ame"},
			Profile: pixiv.Profile{
				TotalIllusts: 2, TotalManga: 1, TotalNovels: 2,
				TotalIllustBookmarksPublic: 5, Webpage: "https://ame.example",
			},
		},
		novels: []pixiv.Novel{{
			ID: 9, Title: "Rain Story", User: pixiv.User{Name: "Ame"},
			TotalBookmarks: 3, PageCount: 4,
		}},
		comments: []pixiv.Comment{{
			ID: 55, Comment: "great read", User: pixiv.User{Name: "Ame"},
			CreateDate: savedAt,
		}},
		saved: []SavedWork{{ID: 101, Title: "Rain", Artist: "Ame", SavedAt: savedAt}},
		downloads: []DownloadedWork{{
			IllustID: 101, Title: "Rain", Artist: "Ame", Dest: "/tmp/rain.png",
			Page: 1, CompletedAt: savedAt,
		}},
		subs: []SubscriptionInfo{{
			Kind: "user", TargetID: 7, TargetName: "Ame", LastChecked: savedAt,
		}},
		articles: []pixiv.SpotlightArticle{{
			ID: 1, Title: "Article", ArticleURL: "https://www.pixivision.net/a/1",
			PublishDate: "2026-10-01",
		}},
	}
	h := newTestServer(b)

	if text := callToolText(t, h, "search_users", `{"word":"ame","limit":3}`); b.searchUsersWord != "ame" || !strings.Contains(text, "#7 Ame (@ame)") {
		t.Errorf("search_users word %q text %q", b.searchUsersWord, text)
	}
	if text := callToolText(t, h, "user_detail", `{"id":7}`); b.userDetailID != 7 ||
		!strings.Contains(text, "3 works, 2 novels, 5 public bookmarks") ||
		!strings.Contains(text, "https://ame.example") {
		t.Errorf("user_detail id %d text %q", b.userDetailID, text)
	}
	if text := callToolText(t, h, "user_illusts", `{"id":7,"kind":"manga","limit":2}`); b.userIllustsID != 7 || b.userIllustsKind != "manga" || !strings.Contains(text, "#101") {
		t.Errorf("user_illusts id %d kind %q text %q", b.userIllustsID, b.userIllustsKind, text)
	}
	if text := callToolText(t, h, "user_illusts", `{"id":7}`); b.userIllustsKind != "illust" || !strings.Contains(text, "#101") {
		t.Errorf("user_illusts default kind %q text %q", b.userIllustsKind, text)
	}
	if text := callToolText(t, h, "user_bookmarks", `{"private":true,"limit":2}`); b.bookmarksID != 0 || !b.bookmarksPrivate || !strings.Contains(text, "#101") {
		t.Errorf("user_bookmarks id %d private %v text %q", b.bookmarksID, b.bookmarksPrivate, text)
	}
	if text := callToolText(t, h, "search_novels", `{"word":"rain"}`); b.novelsWord != "rain" ||
		!strings.Contains(text, "#9 Rain Story — Ame") {
		t.Errorf("search_novels word %q text %q", b.novelsWord, text)
	}
	if text := callToolText(t, h, "novel_comments", `{"id":9,"limit":5}`); b.novelCommentsID != 9 ||
		!strings.Contains(text, "#55 Ame") ||
		!strings.Contains(text, "2026-10-01 12:00") ||
		!strings.Contains(text, "great read") {
		t.Errorf("novel_comments id %d text %q", b.novelCommentsID, text)
	}
	if text := callToolText(t, h, "watch_later", `{}`); !strings.Contains(text, "#101 Rain — Ame") ||
		!strings.Contains(text, "saved 2026-10-01T12:00:00Z") {
		t.Errorf("watch_later text %q", text)
	}
	if text := callToolText(t, h, "download_history", `{}`); !strings.Contains(text, "page 2 → /tmp/rain.png") {
		t.Errorf("download_history text %q", text)
	}
	if text := callToolText(t, h, "subscriptions", `{}`); !strings.Contains(text, "user: Ame (#7)") {
		t.Errorf("subscriptions text %q", text)
	}
	if text := callToolText(t, h, "related_illustrations", `{"id":101}`); b.relatedID != 101 || !strings.Contains(text, "#101") {
		t.Errorf("related_illustrations id %d text %q", b.relatedID, text)
	}
	if text := callToolText(t, h, "spotlight", `{}`); !strings.Contains(text, "Article") ||
		!strings.Contains(text, "https://www.pixivision.net/a/1") {
		t.Errorf("spotlight text %q", text)
	}
}

func TestFeedAndSocialReadTools(t *testing.T) {
	published := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	b := &fakeBackend{
		works: sampleWorks(),
		users: []pixiv.UserPreview{{User: pixiv.User{ID: 7, Name: "Ame", Account: "ame"}}},
		series: []pixiv.SeriesSummary{{
			ID: 3, Title: "Rain Series", User: pixiv.User{ID: 7, Name: "Ame"},
			PublishedContentCount: 4, LastPublishedContent: published,
		}},
		tags:    []pixiv.TrendingTag{{Tag: "rain", TranslatedName: "雨"}},
		summary: "theme: dark",
	}
	h := newTestServer(b)

	if text := callToolText(t, h, "new_works", `{"kind":"manga","limit":2}`); b.newKind != "manga" || !strings.Contains(text, "#101") {
		t.Errorf("new_works kind %q text %q", b.newKind, text)
	}
	if text := callToolText(t, h, "new_works", `{}`); b.newKind != "illust" || !strings.Contains(text, "#101") {
		t.Errorf("new_works default kind %q text %q", b.newKind, text)
	}
	if text := callToolText(t, h, "following_works", `{}`); !strings.Contains(text, "#101") {
		t.Errorf("following_works text %q", text)
	}
	if text := callToolText(t, h, "my_pixiv_works", `{"kind":"manga"}`); b.myPixivKind != "manga" || !strings.Contains(text, "#101") {
		t.Errorf("my_pixiv_works kind %q text %q", b.myPixivKind, text)
	}
	if text := callToolText(t, h, "following_users", `{"id":7}`); b.followingUsersID != 7 || !strings.Contains(text, "#7 Ame") {
		t.Errorf("following_users id %d text %q", b.followingUsersID, text)
	}
	if text := callToolText(t, h, "following_users", `{}`); b.followingUsersID != 0 || !strings.Contains(text, "#7 Ame") {
		t.Errorf("following_users default id %d text %q", b.followingUsersID, text)
	}
	if text := callToolText(t, h, "followers", `{}`); b.followersCalls != 1 || !strings.Contains(text, "#7 Ame") {
		t.Errorf("followers calls %d text %q", b.followersCalls, text)
	}
	if text := callToolText(t, h, "my_pixiv_users", `{"id":7}`); b.myPixivUsersID != 7 || !strings.Contains(text, "#7 Ame") {
		t.Errorf("my_pixiv_users id %d text %q", b.myPixivUsersID, text)
	}
	if text := callToolText(t, h, "related_users", `{"id":7}`); b.relatedUsersID != 7 || !strings.Contains(text, "#7 Ame") {
		t.Errorf("related_users id %d text %q", b.relatedUsersID, text)
	}
	if text := callToolText(t, h, "series_watchlist", `{"kind":"novel"}`); b.seriesKind != "novel" ||
		!strings.Contains(text, "#3 Rain Series — Ame") ||
		!strings.Contains(text, "https://www.pixiv.net/novel/series/3") {
		t.Errorf("series_watchlist kind %q text %q", b.seriesKind, text)
	}
	if text := callToolText(t, h, "series_watchlist", `{}`); b.seriesKind != "manga" ||
		!strings.Contains(text, "https://www.pixiv.net/user/7/series/3") {
		t.Errorf("series_watchlist default kind %q text %q", b.seriesKind, text)
	}
	if text := callToolText(t, h, "trending_tags", `{}`); !strings.Contains(text, "rain (雨)") {
		t.Errorf("trending_tags text %q", text)
	}
	if text := callToolText(t, h, "settings_summary", `{}`); !strings.Contains(text, "theme: dark") {
		t.Errorf("settings_summary text %q", text)
	}
}

func TestFeedAndSocialReadToolBadArgs(t *testing.T) {
	cases := []struct{ name, args string }{
		{"new_works", `{"kind":"novel"}`},
		{"my_pixiv_works", `{"kind":"novel"}`},
		{"series_watchlist", `{"kind":"illust"}`},
		{"following_users", `{"id":-1}`},
		{"my_pixiv_users", `{}`},
		{"related_users", `{"id":0}`},
		// Arguments that are not an object are refused too, including for the
		// tools that take none.
		{"status", `"oops"`},
		{"settings_summary", `"oops"`},
	}
	for _, tc := range cases {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tc.name, tc.args)
		r := call(t, newTestServer(&fakeBackend{}), body)
		if r.Error == nil || r.Error.Code != codeInvalidParams {
			t.Errorf("%s: reply %+v", tc.name, r)
		}
	}
}

// TestRankingMode checks the ranking tool accepts the seven periods its schema
// lists and refuses any other mode as invalid params.
func TestRankingMode(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	h := newTestServer(b)
	if got := len(rankModeNames()); got != 7 {
		t.Fatalf("rank modes %v, want the seven the schema lists", rankModeNames())
	}
	for _, mode := range rankModeNames() {
		callToolText(t, h, "ranking", fmt.Sprintf(`{"mode":%q}`, mode))
		if b.mode != mode {
			t.Errorf("ranking mode %q reached the backend as %q", mode, b.mode)
		}
	}
	for _, mode := range []string{"hour", "day-male", "DAY"} {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"ranking","arguments":{"mode":%q}}}`, mode)
		if r := call(t, h, body); r.Error == nil || r.Error.Code != codeInvalidParams {
			t.Errorf("ranking mode %q: reply %+v", mode, r)
		}
	}
}

func usersWithIDs(n int) []pixiv.UserPreview {
	users := make([]pixiv.UserPreview, n)
	for i := range users {
		users[i] = pixiv.UserPreview{User: pixiv.User{ID: int64(i + 1), Name: "u"}}
	}
	return users
}

func TestReadToolDefaultLimit(t *testing.T) {
	text := callToolText(t, newTestServer(&fakeBackend{users: usersWithIDs(30)}),
		"search_users", `{"word":"u"}`)
	if !strings.Contains(text, "#20 ") {
		t.Errorf("text misses item 20: %q", text)
	}
	if strings.Contains(text, "#21 ") {
		t.Errorf("text has item 21: %q", text)
	}
}

func TestReadToolLimitCapped(t *testing.T) {
	text := callToolText(t, newTestServer(&fakeBackend{users: usersWithIDs(60)}),
		"search_users", `{"word":"u","limit":999}`)
	if !strings.Contains(text, "#50 ") {
		t.Errorf("text misses item 50: %q", text)
	}
	if strings.Contains(text, "#51 ") {
		t.Errorf("text has item 51: %q", text)
	}
}

func TestNovelCommentsLimit(t *testing.T) {
	comments := make([]pixiv.Comment, 60)
	for i := range comments {
		comments[i] = pixiv.Comment{ID: int64(i + 1), Comment: "c"}
	}
	h := newTestServer(&fakeBackend{comments: comments})

	if text := callToolText(t, h, "novel_comments", `{"id":1}`); !strings.Contains(text, "#20 ") || strings.Contains(text, "#21 ") {
		t.Errorf("default limit text %q", text)
	}
	if text := callToolText(t, h, "novel_comments", `{"id":1,"limit":999}`); !strings.Contains(text, "#50 ") || strings.Contains(text, "#51 ") {
		t.Errorf("capped limit text %q", text)
	}
}

func TestReadToolErrorIsResult(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{err: errors.New("boom")}),
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"search_users","arguments":{"word":"u"}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "boom") {
		t.Errorf("result %+v", got)
	}
}

func TestUserIllustsRejectsUnknownKind(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"user_illusts","arguments":{"id":7,"kind":"novel"}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

// postAs posts body with header as the Authorization value, or none when it is
// empty, and returns the recorded reply.
func postAs(t *testing.T, h http.Handler, header, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestRequestWithoutTokenIsUnauthorized checks a caller that sends no token
// gets 401 and nothing of the token back.
func TestRequestWithoutTokenIsUnauthorized(t *testing.T) {
	rec := postAs(t, newTestServer(&fakeBackend{}), "",
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if body := rec.Body.String(); strings.Contains(body, testToken) {
		t.Errorf("body %q leaks the token", body)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Errorf("WWW-Authenticate %q", got)
	}
}

// TestRequestWithWrongTokenIsUnauthorized checks a caller that guesses wrong
// is refused, whether the guess is another token or a bad header shape.
func TestRequestWithWrongTokenIsUnauthorized(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	for _, header := range []string{
		"Bearer wrong-token",
		"Bearer ",
		testToken,
		"Basic " + testToken,
		"Bearer " + testToken + " extra",
	} {
		rec := postAs(t, h, header, body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, want %d", header, rec.Code, http.StatusUnauthorized)
		}
	}
}

// TestRequestWithTokenIsAnswered checks the right token, in any case of the
// scheme, reaches the tools.
func TestRequestWithTokenIsAnswered(t *testing.T) {
	rec := postAs(t, newTestServer(&fakeBackend{}), "bearer "+testToken,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"result"`) {
		t.Errorf("body %q, want a result", rec.Body.String())
	}
}

// TestEmptyTokenRefusesEveryone checks a server made without a token stays
// closed, so a missing setting cannot expose the tools.
func TestEmptyTokenRefusesEveryone(t *testing.T) {
	h := New(&fakeBackend{}, "")
	for _, header := range []string{"", "Bearer ", "Bearer " + testToken} {
		rec := postAs(t, h, header, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, want %d", header, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestCapabilitiesCall(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":20,"method":"tools/call","params":{"name":"capabilities","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || len(got.Content) != 1 {
		t.Fatalf("result %+v", got)
	}
	for _, want := range []string{
		"pixeval-go", "2025-06-18", "bearer", "capabilities", "history",
		fmt.Sprintf("list limit: %d by default", defaultLimit), "paging:",
	} {
		if !strings.Contains(got.Content[0].Text, want) {
			t.Errorf("text misses %q: %s", want, got.Content[0].Text)
		}
	}
}

func TestHelpCall(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"help","arguments":{"tool":"history"}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || len(got.Content) != 1 {
		t.Fatalf("result %+v", got)
	}
	for _, want := range []string{"history", "limit"} {
		if !strings.Contains(got.Content[0].Text, want) {
			t.Errorf("text misses %q: %s", want, got.Content[0].Text)
		}
	}
}

func TestHelpListsEveryTool(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":22,"method":"tools/call","params":{"name":"help","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || !strings.Contains(got.Content[0].Text, "comments") {
		t.Fatalf("result %+v", got)
	}
}

func TestHelpRejectsUnknownTool(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":23,"method":"tools/call","params":{"name":"help","arguments":{"tool":"nope"}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestDownloadMacroCall(t *testing.T) {
	b := &fakeBackend{macro: "test/@{id}"}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":24,"method":"tools/call","params":{"name":"download_macro","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || got.Content[0].Text != "test/@{id}" {
		t.Fatalf("result %+v", got)
	}
}

func TestDownloadMacroRejectsBadArguments(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":25,"method":"tools/call","params":{"name":"download_macro","arguments":"x"}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestAnalyzeDownloadMacroCall(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":26,"method":"tools/call","params":{"name":"analyze_download_macro","arguments":{"text":"@{is_pic_set?set:one}/@{id}.@{ext}"}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || !strings.Contains(got.Content[0].Text, "valid") {
		t.Fatalf("result %+v", got)
	}
	if !strings.Contains(got.Content[0].Text, "123456.jpg") {
		t.Errorf("sample missing: %s", got.Content[0].Text)
	}
}

func TestAnalyzeDownloadMacroRejectsEmptyText(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":27,"method":"tools/call","params":{"name":"analyze_download_macro","arguments":{"text":" "}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestAnalyzeDownloadMacroReportsSyntaxError(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":28,"method":"tools/call","params":{"name":"analyze_download_macro","arguments":{"text":"@{nope}"}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "unknown macro") {
		t.Fatalf("result %+v", got)
	}
}

func TestHistoryCall(t *testing.T) {
	b := &fakeBackend{history: []HistoryWork{{ID: 9, Title: "Rain", Artist: "Ame", OpenedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}}}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":29,"method":"tools/call","params":{"name":"history","arguments":{"limit":5}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError {
		t.Fatalf("result %+v", got)
	}
	for _, want := range []string{"#9", "Rain", "Ame", "opened 2024-01-02T00:00:00Z"} {
		if !strings.Contains(got.Content[0].Text, want) {
			t.Errorf("text misses %q: %s", want, got.Content[0].Text)
		}
	}
}

func TestHistoryRejectsBadLimit(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":30,"method":"tools/call","params":{"name":"history","arguments":{"limit":"x"}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestExtensionsCall(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"extensions","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || !strings.Contains(got.Content[0].Text, "no extension host") {
		t.Fatalf("result %+v", got)
	}
}

func TestExtensionsRejectsBadArguments(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":32,"method":"tools/call","params":{"name":"extensions","arguments":"x"}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestNovelContentCall(t *testing.T) {
	b := &fakeBackend{novelBody: "Once upon a time."}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":33,"method":"tools/call","params":{"name":"novel_content","arguments":{"id":77}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || got.Content[0].Text != "Once upon a time." {
		t.Fatalf("result %+v", got)
	}
	if b.novelContentID != 77 {
		t.Errorf("id %d, want 77", b.novelContentID)
	}
}

func TestNovelContentRejectsMissingID(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":34,"method":"tools/call","params":{"name":"novel_content","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestRecommendedUsersCall(t *testing.T) {
	b := &fakeBackend{recommend: []pixiv.UserPreview{{User: pixiv.User{ID: 5, Name: "Cat"}}}}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":35,"method":"tools/call","params":{"name":"recommended_users","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || !strings.Contains(got.Content[0].Text, "#5 Cat") {
		t.Fatalf("result %+v", got)
	}
}

func TestRecommendedUsersRejectsBadLimit(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":36,"method":"tools/call","params":{"name":"recommended_users","arguments":{"limit":"x"}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestCommentsCall(t *testing.T) {
	b := &fakeBackend{illustCmt: []pixiv.Comment{{ID: 3, Comment: "nice", User: pixiv.User{Name: "Ame"}}}}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":37,"method":"tools/call","params":{"name":"comments","arguments":{"id":101}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if got.IsError || !strings.Contains(got.Content[0].Text, "nice") {
		t.Fatalf("result %+v", got)
	}
	if b.commentsID != 101 {
		t.Errorf("id %d, want 101", b.commentsID)
	}
}

func TestCommentsRejectsMissingID(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":38,"method":"tools/call","params":{"name":"comments","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestWorksAndUsersCalls(t *testing.T) {
	b := &fakeBackend{
		works: sampleWorks(),
		userDetailList: []pixiv.UserDetail{{
			User:    pixiv.User{ID: 7, Name: "Ame"},
			Profile: pixiv.Profile{TotalIllusts: 2},
		}},
	}
	h := newTestServer(b)

	text := callToolText(t, h, "works", `{"ids":[101,0,101,102],"limit":5}`)
	if want := []int64{101, 102}; !slices.Equal(b.worksIDs, want) {
		t.Errorf("works ids %v, want %v", b.worksIDs, want)
	}
	if !strings.Contains(text, "#101 Rain — Ame") {
		t.Errorf("works text %q", text)
	}

	text = callToolText(t, h, "users", `{"ids":[7,7,8]}`)
	if want := []int64{7, 8}; !slices.Equal(b.usersIDs, want) {
		t.Errorf("users ids %v, want %v", b.usersIDs, want)
	}
	if !strings.Contains(text, "#7 Ame") {
		t.Errorf("users text %q", text)
	}
}

func TestWorksAndUsersRejectEmptyIDs(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	for _, name := range []string{"works", "users"} {
		r := call(t, h, `{"jsonrpc":"2.0","id":39,"method":"tools/call","params":{"name":"`+name+`","arguments":{"ids":[]}}}`)
		if r.Error == nil || r.Error.Code != codeInvalidParams {
			t.Errorf("%s reply %+v", name, r)
		}
	}
}

func TestDownloadTasksCall(t *testing.T) {
	b := &fakeBackend{dlTasks: []DownloadTask{{
		ID: "t1", IllustID: 101, Title: "Rain", Artist: "Ame",
		Page: 1, State: "running", Bytes: 10, Total: 20,
	}}}
	text := callToolText(t, newTestServer(b), "download_tasks", `{}`)
	if !strings.Contains(text, "running #101 Rain — Ame") ||
		!strings.Contains(text, "page 2, 10/20 bytes") {
		t.Errorf("download_tasks text %q", text)
	}
}

func TestDownloadTasksRejectsBadArguments(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":40,"method":"tools/call","params":{"name":"download_tasks","arguments":5}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestAnalyzeWorkFilterCall(t *testing.T) {
	text := callToolText(t, newTestServer(&fakeBackend{}), "analyze_work_filter",
		`{"text":"bookmarks > 10 and tag = \"rain\""}`)
	if !strings.Contains(text, "work filter is valid") {
		t.Errorf("text %q", text)
	}
}

func TestAnalyzeWorkFilterRejectsMissingText(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":41,"method":"tools/call","params":{"name":"analyze_work_filter","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestAnalyzeWorkFilterReportsSyntaxError(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}),
		`{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"name":"analyze_work_filter","arguments":{"text":"bookmarks >"}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "expected") {
		t.Fatalf("result %+v", got)
	}
}

func TestWriteToolsCallBackend(t *testing.T) {
	b := &fakeBackend{}
	h := newTestServer(b)

	if text := callToolText(t, h, "set_watch_later", `{"id":101,"watch_later":false}`); !strings.Contains(text, "removed") {
		t.Errorf("set_watch_later text %q", text)
	}
	if b.watchLaterID != 101 || b.watchLaterOn {
		t.Errorf("set_watch_later got id %d on %t", b.watchLaterID, b.watchLaterOn)
	}
	callToolText(t, h, "set_watch_later", `{"id":101}`)
	if !b.watchLaterOn {
		t.Error("set_watch_later default did not add")
	}

	if text := callToolText(t, h, "follow_user", `{"userId":7,"followed":false}`); !strings.Contains(text, "unfollowed") {
		t.Errorf("follow_user text %q", text)
	}
	if b.followID != 7 || b.followOn {
		t.Errorf("follow_user got id %d on %t", b.followID, b.followOn)
	}
	callToolText(t, h, "follow_user", `{"userId":7,"private":true}`)
	if !b.followOn || !b.followPrivate {
		t.Errorf("follow_user private got on %t private %t", b.followOn, b.followPrivate)
	}

	if text := callToolText(t, h, "control_download", `{"id":"t1","action":"resume"}`); !strings.Contains(text, "resumed") {
		t.Errorf("control_download text %q", text)
	}
	if b.controlTaskID != "t1" || b.controlAction != "resume" {
		t.Errorf("control_download got %q %q", b.controlTaskID, b.controlAction)
	}
	callToolText(t, h, "control_download", `{"id":"t1"}`)
	if b.controlAction != "pause" {
		t.Errorf("control_download default action %q", b.controlAction)
	}

	callToolText(t, h, "add_subscription", `{"kind":"series","targetId":9,"name":"Rain"}`)
	if b.addSubKind != "series" || b.addSubID != 9 || b.addSubName != "Rain" {
		t.Errorf("add_subscription got %q %d %q", b.addSubKind, b.addSubID, b.addSubName)
	}
	callToolText(t, h, "remove_subscription", `{"kind":"user","targetId":7}`)
	if b.removeSubKind != "user" || b.removeSubID != 7 {
		t.Errorf("remove_subscription got %q %d", b.removeSubKind, b.removeSubID)
	}

	b.syncNew = 2
	if text := callToolText(t, h, "sync_subscriptions", `{}`); !strings.Contains(text, "2 new work(s)") {
		t.Errorf("sync_subscriptions text %q", text)
	}
	if b.syncCalls != 1 {
		t.Errorf("sync_subscriptions calls %d", b.syncCalls)
	}

	if text := callToolText(t, h, "add_comment", `{"id":101,"content":"  hi  "}`); !strings.Contains(text, "comment 77 added to work 101") {
		t.Errorf("add_comment text %q", text)
	}
	if b.addCommentID != 101 || b.addCommentText != "hi" {
		t.Errorf("add_comment got id %d text %q", b.addCommentID, b.addCommentText)
	}
	if text := callToolText(t, h, "delete_comment", `{"commentId":55}`); !strings.Contains(text, "deleted") {
		t.Errorf("delete_comment text %q", text)
	}
	if b.deleteCmtID != 55 {
		t.Errorf("delete_comment got %d", b.deleteCmtID)
	}

	if text := callToolText(t, h, "set_download_macro", `{"text":"@{artist_name}/@{id}.@{ext}"}`); !strings.Contains(text, "download macro set") {
		t.Errorf("set_download_macro text %q", text)
	}
	if b.setMacroText != "@{artist_name}/@{id}.@{ext}" {
		t.Errorf("set_download_macro got %q", b.setMacroText)
	}
}

func TestWriteToolsReportBackendFailure(t *testing.T) {
	b := &fakeBackend{err: errors.New("boom")}
	r := call(t, newTestServer(b),
		`{"jsonrpc":"2.0","id":50,"method":"tools/call","params":{"name":"set_watch_later","arguments":{"id":1}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError || !strings.Contains(got.Content[0].Text, "boom") {
		t.Fatalf("result %+v", got)
	}
}

func TestWriteToolsRejectBadArguments(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	cases := []struct {
		name string
		args string
	}{
		{"set_watch_later", `{}`},
		{"follow_user", `{}`},
		{"control_download", `{}`},
		{"control_download", `{"id":"t1","action":"boom"}`},
		{"add_subscription", `{"kind":"tag","targetId":1}`},
		{"add_subscription", `{"kind":"user"}`},
		{"remove_subscription", `{"kind":"tag","targetId":1}`},
		{"remove_subscription", `{"kind":"user"}`},
		{"sync_subscriptions", `5`},
		{"add_comment", `{"id":1,"content":" "}`},
		{"add_comment", `{"id":1,"content":"` + strings.Repeat("a", 141) + `"}`},
		{"delete_comment", `{}`},
		{"set_download_macro", `{"text":" "}`},
	}
	for _, tc := range cases {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":51,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tc.name, tc.args)
		r := call(t, h, body)
		if r.Error == nil || r.Error.Code != codeInvalidParams {
			t.Errorf("%s %s reply %+v", tc.name, tc.args, r)
		}
	}
}
