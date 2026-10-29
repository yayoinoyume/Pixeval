package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// pageBodies is the app API's JSON for a user page, its works and user lists,
// and a series page, keyed by the path prefix that asks for it.
func pageBodies() map[string]string {
	return map[string]string{
		"/v1/user/detail": `{"user":{"id":7,"name":"画师七","account":"painter7","comment":"画风景"},` +
			`"profile":{"total_illusts":1}}`,
		"/v1/user/illusts": `{"illusts":[{"id":11,"title":"作品十一",` +
			`"user":{"id":7,"name":"画师七"},"width":480,"height":720}],"next_url":""}`,
		"/v1/user/novels": `{"novels":[{"id":31,"title":"小说三十一",` +
			`"user":{"id":7,"name":"画师七"},"page_count":3}],"next_url":""}`,
		"/v1/user/following": `{"user_previews":[{"user":{"id":8,"name":"关注八",` +
			`"account":"following8"}}],"next_url":""}`,
		"/v1/user/follower": `{"user_previews":[{"user":{"id":9,"name":"粉丝九",` +
			`"account":"follower9"}}],"next_url":""}`,
		"/v1/user/bookmarks/illust": `{"illusts":[{"id":41,"title":"收藏四十一",` +
			`"user":{"id":7,"name":"画师七"},"width":480,"height":720}],"next_url":""}`,
		"/v1/illust/series/": `{"illust_series_detail":{"id":3,"title":"系列三","description":"系列简介"},` +
			`"illusts":[{"id":21,"title":"系列作品","user":{"id":7,"name":"画师七"},` +
			`"width":480,"height":720}],"next_url":""}`,
	}
}

// pageTransport answers the app API from bodies without a network, and counts
// the paths it answered.
type pageTransport struct {
	mu     sync.Mutex
	bodies map[string]string
	// byType answers /v1/user/illusts by its type parameter, when set.
	byType map[string]string
	// byURL answers a request URI exactly, when set.
	byURL map[string]string
	paths []string
	urls  []string
}

func (tr *pageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	tr.paths = append(tr.paths, req.URL.Path)
	tr.urls = append(tr.urls, req.URL.RequestURI())
	body := tr.byURL[req.URL.RequestURI()]
	if body == "" && tr.byType != nil && req.URL.Path == "/v1/user/illusts" {
		body = tr.byType[req.URL.Query().Get("type")]
	}
	if body == "" {
		for prefix, b := range tr.bodies {
			if strings.HasPrefix(req.URL.Path, prefix) {
				body = b
				break
			}
		}
	}
	tr.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// requestedURLs returns the URLs the transport answered, without their host.
func (tr *pageTransport) requestedURLs() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return append([]string(nil), tr.urls...)
}

// newPageApp is an app whose API answers from pageBodies, without images or a
// network.
func newPageApp() *App {
	return newPageAppWithBodies(pageBodies())
}

// newPageAppWithBodies is newPageApp with the API bodies a test needs.
func newPageAppWithBodies(bodies map[string]string) *App {
	a := New()
	a.client.HTTP().Transport = &pageTransport{
		bodies: bodies,
		byType: map[string]string{
			"illust": bodies["/v1/user/illusts"],
			"manga": `{"illusts":[{"id":12,"title":"漫画十二",` +
				`"user":{"id":7,"name":"画师七"},"width":480,"height":720}],"next_url":""}`,
		},
	}
	a.client.SetCooldown(0)
	a.images = imagex.New(nil, "")
	return a
}

// englishPageApp is newPageAppWithBodies speaking English, whose following tab
// and follow button do not read the same word.
func englishPageApp(bodies map[string]string) *App {
	a := newPageAppWithBodies(bodies)
	a.settings.Language = i18n.LangEnUS
	a.loadLanguage()
	return a
}

// waitUsers waits for a user list to settle as want says.
func waitUsers(t *testing.T, st *userListState, want func(v userListView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("user list did not settle: users=%d loading=%v err=%q",
				len(v.users), v.loading, v.err)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitUser waits for the user page's profile to settle as want says.
func waitUser(t *testing.T, a *App, want func(v userPageView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := a.userView; v != nil && want(v.snapshot()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the user page did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

// waitSeries waits for the series page's detail to settle as want says.
func waitSeries(t *testing.T, a *App, want func(v seriesPageView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := a.seriesView; v != nil && want(v.snapshot()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the series page did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestUserPageShowsUser checks opening a user draws the profile, its follow
// button and the user's works.
func TestUserPageShowsUser(t *testing.T) {
	a := newPageApp()
	a.openUser(7)
	if a.route != routeUser {
		t.Fatalf("route %q, want %q", a.route, routeUser)
	}
	waitUser(t, a, func(v userPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	for _, want := range []string{"画师七", "painter7", "画风景", "关注", "作品十一",
		"投稿插画", "投稿漫画"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestUserPageTabsKeepFeeds checks the works tabs request their own kind and
// keep each feed's loaded page when another tab is shown.
func TestUserPageTabsKeepFeeds(t *testing.T) {
	a := newPageApp()
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	if a.userView.tab != userTabIllust {
		t.Fatalf("tab %d, want the illustration tab", a.userView.tab)
	}
	tt.Frame()
	if !tt.HasText("作品十一") {
		t.Errorf("illustration tab missing: %q", tt.Texts())
	}

	if err := tt.Click(a.t("user.tabManga")); err != nil {
		t.Fatalf("Click manga tab: %v", err)
	}
	if a.userView.tab != userTabManga {
		t.Fatalf("tab %d, want the manga tab", a.userView.tab)
	}
	waitBrowse(t, a.userView.mangaFeed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	if !tt.HasText("漫画十二") {
		t.Errorf("manga tab missing: %q", tt.Texts())
	}

	if err := tt.Click(a.t("user.tabNovel")); err != nil {
		t.Fatalf("Click novel tab: %v", err)
	}
	waitNovels(t, a.userView.novelFeed, func(v novelFeedView) bool { return len(v.items) == 1 })
	tt.Frame()
	if !tt.HasText("小说三十一") {
		t.Errorf("novel tab missing: %q", tt.Texts())
	}

	if err := tt.Click(a.t("user.tabIllust")); err != nil {
		t.Fatalf("Click illustration tab: %v", err)
	}
	tt.Frame()
	if v := a.userView.feed.snapshot(); len(v.items) != 1 || v.items[0].ID != 11 {
		t.Errorf("illustration feed after switching back = %+v, want work 11", v.items)
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	illustLoads, novelLoads := 0, 0
	for _, u := range tr.requestedURLs() {
		if strings.Contains(u, "/v1/user/illusts") && strings.Contains(u, "type=illust") {
			illustLoads++
		}
		if strings.Contains(u, "/v1/user/novels") {
			novelLoads++
		}
	}
	if illustLoads != 1 || novelLoads != 1 {
		t.Errorf("requests = %v, want one illustration page and one novel page", tr.requestedURLs())
	}
}

// TestUserPageNovelEmpty checks the novel tab's empty state.
func TestUserPageNovelEmpty(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/novels"] = `{"novels":[],"next_url":""}`
	a := newPageAppWithBodies(bodies)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	if err := tt.Click(a.t("user.tabNovel")); err != nil {
		t.Fatalf("Click novel tab: %v", err)
	}
	waitNovels(t, a.userView.novelFeed, func(v novelFeedView) bool {
		return v.started && len(v.items) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noNovels")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestUserPageNovelOpensReader checks a novel card opens the reader on that
// novel.
func TestUserPageNovelOpensReader(t *testing.T) {
	a := newPageApp()
	a.novelContent = func(context.Context, int64) (string, error) { return novelBody, nil }
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	if err := tt.Click(a.t("user.tabNovel")); err != nil {
		t.Fatalf("Click novel tab: %v", err)
	}
	waitNovels(t, a.userView.novelFeed, func(v novelFeedView) bool { return len(v.items) == 1 })
	tt.Frame()

	card := a.tf("a11y.card", "小说三十一", "画师七")
	if err := tt.Click(card); err != nil {
		t.Fatalf("Click %q: %v", card, err)
	}
	if a.route != routeNovel {
		t.Fatalf("route %q after the click, want %q", a.route, routeNovel)
	}
	if a.novelView == nil || a.novelView.novelID != 31 || a.novelView.title != "小说三十一" {
		t.Fatalf("novel view %+v, want novel 31", a.novelView)
	}
	if a.backRoute != routeUser {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeUser)
	}
}

// TestUserPageFollow checks the follow button follows the user publicly and
// shows the new state.
func TestUserPageFollow(t *testing.T) {
	a := newPageApp()
	calls := make(chan string, 2)
	a.followFn = func(_ context.Context, id int64, on bool) error {
		calls <- fmt.Sprintf("%d:%v", id, on)
		return nil
	}
	a.followPrivateFn = func(_ context.Context, id int64) error {
		calls <- fmt.Sprintf("private %d", id)
		return nil
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("user.follow")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	select {
	case got := <-calls:
		if got != "7:true" {
			t.Errorf("follow call %q, want 7:true", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the follow button did not request a follow")
	}
	waitUser(t, a, func(v userPageView) bool { return v.user.IsFollowed })
	tt.Frame()
	if _, ok := tt.Find(a.t("user.unfollow")); !ok {
		t.Errorf("unfollow button missing: %q", tt.Texts())
	}
	if _, ok := tt.Find(a.t("user.followPrivate")); ok {
		t.Errorf("the private follow is still offered: %q", tt.Texts())
	}
}

// TestUserPagePrivateFollow checks the private follow button sends a private
// follow and shows the new state.
func TestUserPagePrivateFollow(t *testing.T) {
	a := newPageApp()
	calls := make(chan string, 2)
	a.followFn = func(_ context.Context, id int64, on bool) error {
		calls <- fmt.Sprintf("%d:%v", id, on)
		return nil
	}
	a.followPrivateFn = func(_ context.Context, id int64) error {
		calls <- fmt.Sprintf("private %d", id)
		return nil
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("user.followPrivate")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	select {
	case got := <-calls:
		if got != "private 7" {
			t.Errorf("call %q, want a private follow of 7", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the private follow button did not request a follow")
	}
	waitUser(t, a, func(v userPageView) bool { return v.user.IsFollowed })
	tt.Frame()
	if _, ok := tt.Find(a.t("user.unfollow")); !ok {
		t.Errorf("unfollow button missing: %q", tt.Texts())
	}
}

// TestUserPageUnfollow checks a followed user's button unfollows and offers
// both follows again.
func TestUserPageUnfollow(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/detail"] = `{"user":{"id":7,"name":"画师七","account":"painter7","is_followed":true},` +
		`"profile":{"total_illusts":1}}`
	a := newPageAppWithBodies(bodies)
	calls := make(chan string, 2)
	a.followFn = func(_ context.Context, id int64, on bool) error {
		calls <- fmt.Sprintf("%d:%v", id, on)
		return nil
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading && v.user.IsFollowed })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if err := tt.Click(a.t("user.unfollow")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	select {
	case got := <-calls:
		if got != "7:false" {
			t.Errorf("unfollow call %q, want 7:false", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the unfollow button did not request an unfollow")
	}
	waitUser(t, a, func(v userPageView) bool { return !v.user.IsFollowed })
	tt.Frame()
	for _, want := range []string{a.t("user.follow"), a.t("user.followPrivate")} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestUserPageFollowFailure checks a failed follow keeps the state and leaves
// the reason as a notice.
func TestUserPageFollowFailure(t *testing.T) {
	a := newPageApp()
	release := make(chan struct{})
	a.followFn = func(context.Context, int64, bool) error {
		<-release
		return errors.New("网络错误")
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("user.follow")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	waitUser(t, a, func(v userPageView) bool { return !v.followBusy })
	if a.userView.snapshot().user.IsFollowed {
		t.Error("a failed follow left the user followed")
	}
	if want := a.tf("user.followFailed", "网络错误"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	tt.Frame()
	if _, ok := tt.Find(a.t("user.follow")); !ok {
		t.Errorf("follow button missing after failure: %q", tt.Texts())
	}
}

// TestUserPagePrivateFollowFailure checks a failed private follow keeps the
// state and leaves the reason as a notice.
func TestUserPagePrivateFollowFailure(t *testing.T) {
	a := newPageApp()
	release := make(chan struct{})
	a.followPrivateFn = func(context.Context, int64) error {
		<-release
		return errors.New("网络错误")
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click(a.t("user.followPrivate")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	waitUser(t, a, func(v userPageView) bool { return !v.followBusy })
	if a.userView.snapshot().user.IsFollowed {
		t.Error("a failed private follow left the user followed")
	}
	if want := a.tf("user.followPrivateFailed", "网络错误"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	tt.Frame()
	if _, ok := tt.Find(a.t("user.followPrivate")); !ok {
		t.Errorf("private follow button missing after failure: %q", tt.Texts())
	}
}

// TestUserPageSocialTabs checks the following and followers tabs request
// their own list and keep each list's page when another tab is shown.
func TestUserPageSocialTabs(t *testing.T) {
	a := englishPageApp(pageBodies())
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab: %v", err)
	}
	if a.userView.tab != userTabFollowing {
		t.Fatalf("tab %d, want the following tab", a.userView.tab)
	}
	waitUsers(t, a.userView.followingFeed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	for _, want := range []string{"关注八", "@following8"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}

	if err := tt.Click(a.t("user.tabFollowers")); err != nil {
		t.Fatalf("Click followers tab: %v", err)
	}
	waitUsers(t, a.userView.followersFeed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	for _, want := range []string{"粉丝九", "@follower9"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}

	// The following list kept its page: coming back shows it again without
	// asking for it again.
	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab again: %v", err)
	}
	tt.Frame()
	if v := a.userView.followingFeed.snapshot(); len(v.users) != 1 || v.users[0].User.ID != 8 {
		t.Errorf("following list after switching back = %+v, want user 8", v.users)
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	following, followers := 0, 0
	for _, u := range tr.requestedURLs() {
		switch {
		case strings.Contains(u, "/v1/user/following"):
			following++
		case strings.Contains(u, "/v1/user/follower"):
			followers++
		}
	}
	if following != 1 || followers != 1 {
		t.Errorf("requests = %v, want one following page and one followers page", tr.requestedURLs())
	}
}

// TestUserPageBookmarksPublicOnly checks another user's bookmarks tab asks
// for the public list, offers no private list, and opens a work in the viewer.
func TestUserPageBookmarksPublicOnly(t *testing.T) {
	a := englishPageApp(pageBodies())
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	tr := a.client.HTTP().Transport.(*pageTransport)
	if got := tr.requestedURLs(); containsPart(got, "/v1/user/bookmarks") {
		t.Fatalf("opening the page fetched bookmarks: %v", got)
	}

	// The navigation's own Bookmarks item shares the tab's label, so the test
	// chooses the tab instead of clicking that text.
	a.userView.tab = userTabBookmarks
	tt.Frame()
	waitBrowse(t, a.userView.bookmarksFeed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	if !tt.HasText("收藏四十一") {
		t.Errorf("bookmarks tab missing: %q", tt.Texts())
	}
	if tt.HasText(a.t("user.bookmarksPrivate")) {
		t.Errorf("private sub-tab on another user's page: %q", tt.Texts())
	}
	if !containsPart(tr.requestedURLs(), "restrict=public") ||
		!containsPart(tr.requestedURLs(), "user_id=7") {
		t.Errorf("requests = %v, want the public list for user 7", tr.requestedURLs())
	}
	if containsPart(tr.requestedURLs(), "restrict=private") {
		t.Errorf("requests = %v, want no private list", tr.requestedURLs())
	}

	card := a.tf("a11y.card", "收藏四十一", "画师七")
	if err := tt.Click(card); err != nil {
		t.Fatalf("Click %q: %v", card, err)
	}
	if a.viewer == nil || a.viewer.work.ID != 41 {
		t.Fatalf("viewer %+v, want work 41", a.viewer)
	}
}

// TestUserPageOwnBookmarksVisibility checks the signed-in user's own page
// offers the private list, and switching visibility restarts only that list.
func TestUserPageOwnBookmarksVisibility(t *testing.T) {
	a := englishPageApp(pageBodies())
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	a.userView.tab = userTabBookmarks
	tt.Frame()
	waitBrowse(t, a.userView.bookmarksFeed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	for _, want := range []string{a.t("user.bookmarksPublic"), a.t("user.bookmarksPrivate")} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}

	if err := tt.Click(a.t("user.bookmarksPrivate")); err != nil {
		t.Fatalf("Click private sub-tab: %v", err)
	}
	if a.userView.bookmarksTab != bookmarksPrivate {
		t.Fatal("the private sub-tab is not selected")
	}
	waitBrowse(t, a.userView.bookmarksPrivateFeed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	if !tt.HasText("收藏四十一") {
		t.Errorf("private list missing: %q", tt.Texts())
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	if !containsPart(tr.requestedURLs(), "restrict=private") ||
		!containsPart(tr.requestedURLs(), "user_id=7") {
		t.Errorf("requests = %v, want the private list for user 7", tr.requestedURLs())
	}

	if err := tt.Click(a.t("user.bookmarksPublic")); err != nil {
		t.Fatalf("Click public sub-tab: %v", err)
	}
	waitBrowse(t, a.userView.bookmarksFeed, "load", func(v browseView) bool { return len(v.items) == 1 })
	deadline := time.Now().Add(2 * time.Second)
	for countPart(tr.requestedURLs(), "restrict=public") < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("public list not reloaded: %v", tr.requestedURLs())
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	if got := countPart(tr.requestedURLs(), "type=illust"); got != 1 {
		t.Errorf("illustrations requested %d times, want 1: %v", got, tr.requestedURLs())
	}
	if got := countPart(tr.requestedURLs(), "restrict=private"); got != 1 {
		t.Errorf("private list requested %d times, want 1: %v", got, tr.requestedURLs())
	}
}

// TestUserPageBookmarksEmpty checks the bookmarks tab's empty state.
func TestUserPageBookmarksEmpty(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/bookmarks/illust"] = `{"illusts":[],"next_url":""}`
	a := englishPageApp(bodies)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	a.userView.tab = userTabBookmarks
	tt.Frame()
	waitBrowse(t, a.userView.bookmarksFeed, "load", func(v browseView) bool {
		return v.started && len(v.items) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noBookmarks")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// containsPart reports whether a requested URL holds part.
func containsPart(urls []string, part string) bool {
	return countPart(urls, part) > 0
}

// countPart counts the requested URLs holding part.
func countPart(urls []string, part string) int {
	n := 0
	for _, u := range urls {
		if strings.Contains(u, part) {
			n++
		}
	}
	return n
}

// TestUserPageFollowingEmpty checks the following tab's empty state.
func TestUserPageFollowingEmpty(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/following"] = `{"user_previews":[],"next_url":""}`
	a := englishPageApp(bodies)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab: %v", err)
	}
	waitUsers(t, a.userView.followingFeed, func(v userListView) bool {
		return v.started && len(v.users) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noFollowing")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestUserPageFollowersFailed checks the followers tab shows its own failure,
// and the following tab still loads.
func TestUserPageFollowersFailed(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/follower"] = `{"user_previews":`
	a := englishPageApp(bodies)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	if err := tt.Click(a.t("user.tabFollowers")); err != nil {
		t.Fatalf("Click followers tab: %v", err)
	}
	waitUsers(t, a.userView.followersFeed, func(v userListView) bool { return v.err != "" })
	tt.Frame()
	failed := strings.Split(a.t("user.followersLoadFailed"), "%s")[0]
	if !tt.HasText(failed) {
		t.Errorf("failure state missing: %q", tt.Texts())
	}
	if !tt.HasText(a.t("common.retry")) {
		t.Errorf("retry button missing: %q", tt.Texts())
	}

	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab: %v", err)
	}
	waitUsers(t, a.userView.followingFeed, func(v userListView) bool { return len(v.users) == 1 })
	if v := a.userView.followersFeed.snapshot(); v.err == "" {
		t.Errorf("followers failure cleared by the other tab: %+v", v)
	}
}

// TestUserPageFollowingPages checks the following tab loads its next page
// from the cursor.
func TestUserPageFollowingPages(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/following"] = `{"user_previews":[{"user":{"id":8,"name":"关注八"}}],` +
		`"next_url":"https://app-api.pixiv.net/v1/user/following?offset=1"}`
	a := englishPageApp(bodies)
	a.client.HTTP().Transport.(*pageTransport).byURL = map[string]string{
		"/v1/user/following?offset=1": `{"user_previews":[{"user":{"id":10,"name":"关注十"}}],` +
			`"next_url":""}`,
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab: %v", err)
	}
	// The list draws its first page, sees its end, and loads the next one, so
	// draw frames until both pages are in.
	deadline := time.Now().Add(2 * time.Second)
	for len(a.userView.followingFeed.snapshot().users) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("following list has %d users, want 2",
				len(a.userView.followingFeed.snapshot().users))
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if !tt.HasText("关注十") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	if got := tr.requestedURLs(); len(got) == 0 || !strings.Contains(got[len(got)-1], "offset=1") {
		t.Errorf("requests = %v, want the cursor's page last", got)
	}
}

// TestUserPageLoadsTabsOnDemand checks opening a user page fetches only the
// tab it shows, and a tab already loaded is not fetched again.
func TestUserPageLoadsTabsOnDemand(t *testing.T) {
	a := englishPageApp(pageBodies())
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()

	tr := a.client.HTTP().Transport.(*pageTransport)
	requests := func(part string) int {
		n := 0
		for _, u := range tr.requestedURLs() {
			if strings.Contains(u, part) {
				n++
			}
		}
		return n
	}
	for _, part := range []string{"type=manga", "/v1/user/novels", "/v1/user/following",
		"/v1/user/follower", "/v1/user/bookmarks"} {
		if n := requests(part); n != 0 {
			t.Errorf("opening the page fetched %s: %v", part, tr.requestedURLs())
		}
	}

	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab: %v", err)
	}
	waitUsers(t, a.userView.followingFeed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()

	// Switching away and back fetches nothing: each tab kept its page.
	if err := tt.Click(a.t("user.tabIllust")); err != nil {
		t.Fatalf("Click illustration tab: %v", err)
	}
	tt.Frame()
	if err := tt.Click(a.t("user.tabFollowing")); err != nil {
		t.Fatalf("Click following tab again: %v", err)
	}
	tt.Frame()
	if n := requests("/v1/user/following"); n != 1 {
		t.Errorf("following requested %d times, want 1: %v", n, tr.requestedURLs())
	}
	if n := requests("type=illust"); n != 1 {
		t.Errorf("illustrations requested %d times, want 1: %v", n, tr.requestedURLs())
	}
	if v := a.userView.followingFeed.snapshot(); len(v.users) != 1 {
		t.Errorf("following list after switching back = %+v, want one user", v.users)
	}
}

// TestUserPageCardOpensUser checks a card of a following or followers list
// opens that user's page.
func TestUserPageCardOpensUser(t *testing.T) {
	a := englishPageApp(pageBodies())
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })

	if err := tt.Click(a.t("user.tabFollowers")); err != nil {
		t.Fatalf("Click followers tab: %v", err)
	}
	waitUsers(t, a.userView.followersFeed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	if err := tt.Click("粉丝九"); err != nil {
		t.Fatalf("Click user card: %v", err)
	}
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	if a.route != routeUser {
		t.Errorf("route %q after the click, want %q", a.route, routeUser)
	}
	if a.backRoute != routeUser {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeUser)
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	opened := false
	for _, u := range tr.requestedURLs() {
		if strings.Contains(u, "/v1/user/detail") && strings.Contains(u, "user_id=9") {
			opened = true
		}
	}
	if !opened {
		t.Errorf("no detail request for user 9: %v", tr.requestedURLs())
	}
}

// TestSeriesPageShowsSeries checks opening a series draws its title,
// description and works.
func TestSeriesPageShowsSeries(t *testing.T) {
	a := newPageApp()
	a.openSeries(3)
	if a.route != routeSeries {
		t.Fatalf("route %q, want %q", a.route, routeSeries)
	}
	waitSeries(t, a, func(v seriesPageView) bool { return !v.loading })

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.seriesView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	for _, want := range []string{"系列三", "系列简介", "系列作品"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestUserPageBack checks the back button returns to the page the user page
// was opened from.
func TestUserPageBack(t *testing.T) {
	a := newPageApp()
	a.route = routeHistory
	a.openUser(7)
	if a.backRoute != routeHistory {
		t.Fatalf("backRoute %q, want %q", a.backRoute, routeHistory)
	}
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("返回"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.route != routeHistory {
		t.Errorf("route %q after going back, want %q", a.route, routeHistory)
	}
}

// TestBackWithoutRouteGoesHome checks a page opened without one before it
// still has somewhere to go back to.
func TestBackWithoutRouteGoesHome(t *testing.T) {
	a := newPageApp()
	a.openSeries(3)
	a.goBack()
	if a.route != routeHome {
		t.Errorf("route %q with no route to go back to, want %q", a.route, routeHome)
	}
}

// TestViewerOpensUser checks the viewer's author button leaves the viewer and
// opens the author's page.
func TestViewerOpensUser(t *testing.T) {
	a := newPageApp()
	w := pixiv.Illustration{ID: 42, Title: "查看器作品", Width: 800, Height: 1200,
		User: pixiv.User{ID: 7, Name: "画师七"}}
	a.openViewer(&w, 0)
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("作者"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.viewer != nil {
		t.Error("the viewer is still open")
	}
	if a.route != routeUser {
		t.Errorf("route %q, want %q", a.route, routeUser)
	}
}

// TestUserPageDownloadLoaded checks the illustration and manga tabs each
// queue the works they have loaded.
func TestUserPageDownloadLoaded(t *testing.T) {
	a := newPageApp()
	a.downloadsDir = t.TempDir()
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load the illustrations", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	label := a.tf("download.allLoaded", 1)
	if !tt.HasText(label) {
		t.Fatalf("%q missing: %q", label, tt.Texts())
	}
	if err := tt.Click(label); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 1 {
		t.Errorf("%d tasks queued from the illustration tab, want 1", len(tasks))
	}

	if err := tt.Click(a.t("user.tabManga")); err != nil {
		t.Fatalf("Click manga tab: %v", err)
	}
	waitBrowse(t, a.userView.mangaFeed, "load the manga", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	if err := tt.Click(label); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 2 {
		t.Errorf("%d tasks queued after the manga tab, want 2", len(tasks))
	}
}

// TestUserPageRelatedUsersEntry checks the profile's related-users entry opens
// that page on a visit, and going back returns to the user page with its works
// and its own way back intact.
func TestUserPageRelatedUsersEntry(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/related"] = `{"user_previews":[{"user":{"id":10,"name":"相关十",` +
		`"account":"related10"}}],"next_url":""}`
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	a.route = routeHistory
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()

	tr := a.client.HTTP().Transport.(*pageTransport)
	if containsPart(tr.requestedURLs(), "/v1/user/related") {
		t.Fatalf("opening the user page fetched the related list: %v", tr.requestedURLs())
	}

	userPage := a.userView
	if err := tt.Click(a.t("user.relatedUsers")); err != nil {
		t.Fatalf("Click related users: %v", err)
	}
	if a.route != routeUserRelated {
		t.Fatalf("route %q, want %q", a.route, routeUserRelated)
	}
	if a.relatedUsers == nil || a.relatedUsers.userID != 7 {
		t.Fatalf("related users view %+v, want user 7", a.relatedUsers)
	}
	tt.Frame()
	waitUsers(t, a.relatedUsers.feed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	for _, want := range []string{a.t("user.relatedUsers"), "相关十", "@related10"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if !containsPart(tr.requestedURLs(), "/v1/user/related") ||
		!containsPart(tr.requestedURLs(), "user_id=7") {
		t.Errorf("requests = %v, want the related list of user 7", tr.requestedURLs())
	}

	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back: %v", err)
	}
	if a.route != routeUser {
		t.Errorf("route %q after going back, want %q", a.route, routeUser)
	}
	if a.userView != userPage {
		t.Error("the user page is not the one the related list was opened from")
	}
	if a.backRoute != routeHistory {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeHistory)
	}
	if a.relatedUsers != nil {
		t.Error("the related users page is still open")
	}
	tt.Frame()
	if v := a.userView.feed.snapshot(); len(v.items) != 1 || v.items[0].ID != 11 {
		t.Errorf("user works after going back = %+v, want work 11", v.items)
	}
	if n := countPart(tr.requestedURLs(), "type=illust"); n != 1 {
		t.Errorf("illustrations requested %d times, want 1: %v", n, tr.requestedURLs())
	}
}
