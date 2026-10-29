package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// TestShellLoggedOut checks the home page shows the login button to a
// signed-out user. Find is exact, so the prompt text alone does not pass.
func TestShellLoggedOut(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1000, 700)
	if _, ok := tt.Find("登录 Pixiv"); !ok {
		t.Errorf("login button missing: %q", tt.Texts())
	}
	if !tt.HasText("首页") {
		t.Errorf("texts %q", tt.Texts())
	}
}

// TestShellNavSwitches checks a click on the navigation changes the page and
// its title.
func TestShellNavSwitches(t *testing.T) {
	a := New()
	// The rail is taller than 700 with every destination it has, so the
	// window is the app's own size for the click to land.
	tt := ui.NewTester(a.View, 1280, 820)
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q, want %q", a.route, routeSettings)
	}
	if !tt.HasText("下载目录") {
		t.Errorf("settings page missing: %q", tt.Texts())
	}
	// The nav item and the header both show 设置.
	n := 0
	for _, s := range tt.Texts() {
		if s == "设置" {
			n++
		}
	}
	if n < 2 {
		t.Errorf("title not shown: %q", tt.Texts())
	}
}

// TestShellRailSettingsReachableInShortWindow checks Settings stays pinned at
// the rail's bottom when the window is shorter than every destination.
func TestShellRailSettingsReachableInShortWindow(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1280, 790)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("Click Settings: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q, want %q", a.route, routeSettings)
	}
}

// TestShellRailScrollsDestinations checks a destination below the rail's
// viewport comes into reach by scrolling the rail.
func TestShellRailScrollsDestinations(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1280, 600)
	tt.Frame()
	if err := tt.Click("稍后再看"); err == nil {
		t.Fatal("the last destination was reachable without scrolling")
	}
	tt.Scroll(30, 400, 0, 400)
	tt.Frame()
	if err := tt.Click("稍后再看"); err != nil {
		t.Fatalf("Click after scrolling: %v", err)
	}
	if a.route != routeWatchLater {
		t.Errorf("route %q, want %q", a.route, routeWatchLater)
	}
}

// TestShellLoggedIn checks a signed-in session shows the user's name instead
// of the login prompt.
func TestShellLoggedIn(t *testing.T) {
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("测试用户") {
		t.Errorf("texts %q", tt.Texts())
	}
	if tt.HasText("登录 Pixiv") {
		t.Errorf("login prompt shown while signed in: %q", tt.Texts())
	}
}

// TestSessionLifecycle checks a saved token restores the session and a logout
// removes the token file.
func TestSessionLifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := tokenPath()
	if err != nil {
		t.Fatalf("tokenPath: %v", err)
	}
	tok := &pixiv.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    3600,
		User:         pixiv.TokenUser{Name: "测试用户"},
	}
	if err := pixiv.SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	a := New()
	a.restoreSession()
	if a.session == nil {
		t.Fatal("session not restored")
	}
	if got := a.userName(); got != "测试用户" {
		t.Errorf("userName %q", got)
	}

	a.logout()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("token file still at %s after logout (stat err %v)", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestShellRail checks the rail is 88 wide with every destination inside it
// and the settings held to the bottom, as the design draws it.
func TestShellRail(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1024, 768)
	tt.Frame()
	for _, label := range []string{"首页", "新作", "MyPixiv", "搜索", "追更", "下载", "历史", "设置"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%s missing: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > navWidth+1 {
			t.Errorf("%s at %+v, outside the %v rail", label, r, navWidth)
		}
	}
	home, _ := tt.Find("首页")
	settings, _ := tt.Find("设置")
	if settings.Y <= home.Y {
		t.Errorf("settings at %v is not under the destinations (%v)", settings.Y, home.Y)
	}
	if settings.Y < 768/2 {
		t.Errorf("settings at %v is not held to the bottom", settings.Y)
	}
}

// TestShellFitsWindow checks the rail and the app bar fit the narrow and the
// design window, with nothing pushed past the right or bottom edge.
func TestShellFitsWindow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{1024, 768}, {1280, 820}} {
		a := New()
		tt := ui.NewTester(a.View, size.w, size.h)
		tt.Frame()
		r, ok := tt.Find("登录 Pixiv")
		if !ok {
			t.Fatalf("%dx%d: login button missing: %q", size.w, size.h, tt.Texts())
		}
		if r.X+r.W > float32(size.w) || r.Y+r.H > float32(size.h) {
			t.Errorf("%dx%d: login button %+v outside the window", size.w, size.h, r)
		}
		for _, label := range []string{"首页", "设置"} {
			r, ok := tt.Find(label)
			if !ok || r.X+r.W > float32(size.w) || r.Y+r.H > float32(size.h) {
				t.Errorf("%dx%d: %s %+v outside the window", size.w, size.h, label, r)
			}
		}
	}
}

// searchPixiv is a fake app API that records the illustration searches it
// receives: the first answers two works, later ones a single work, so a test
// can tell a restarted list from the one before it.
type searchPixiv struct {
	mu   sync.Mutex
	reqs []searchPixivRequest
}

type searchPixivRequest struct {
	word   string
	sort   string
	target string
}

func (p *searchPixiv) handler() http.Handler {
	const twoWorks = `{"illusts":[` +
		`{"id":1,"title":"作品 1","user":{"id":9,"name":"作者"},"width":480,"height":720},` +
		`{"id":2,"title":"作品 2","user":{"id":9,"name":"作者"},"width":480,"height":720}],` +
		`"next_url":""}`
	const oneWork = `{"illusts":[` +
		`{"id":3,"title":"作品 3","user":{"id":9,"name":"作者"},"width":480,"height":720}],` +
		`"next_url":""}`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		p.mu.Lock()
		p.reqs = append(p.reqs, searchPixivRequest{
			word:   q.Get("word"),
			sort:   q.Get("sort"),
			target: q.Get("search_target"),
		})
		n := len(p.reqs)
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			io.WriteString(w, twoWorks)
			return
		}
		io.WriteString(w, oneWork)
	})
}

func (p *searchPixiv) requests() []searchPixivRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]searchPixivRequest(nil), p.reqs...)
}

// TestSearchOptionsReachSearch checks the search page shows the order and
// match choices, sends them with the search, and starts the results over when
// the choices change and the search runs again.
func TestSearchOptionsReachSearch(t *testing.T) {
	fake := &searchPixiv{}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse the test server URL: %v", err)
	}

	a := New()
	a.client.HTTP().Transport = e2eTransport{target: target}
	a.client.SetCooldown(0)
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.route = routeSearch
	a.search.query = "猫"
	a.searchSort = searchSortPopular
	a.searchTarget = searchTargetTitleCaption
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !tt.HasText("按收藏数") {
		t.Fatalf("the sort choice is not shown: %q", tt.Texts())
	}
	if !tt.HasText("标题与简介") {
		t.Fatalf("the match choice is not shown: %q", tt.Texts())
	}

	a.startSearch(a.search)
	waitBrowse(t, a.search, "load the first search",
		func(v browseView) bool { return len(v.items) == 2 })
	got := fake.requests()
	if len(got) != 1 {
		t.Fatalf("requests %v, want 1", got)
	}
	if want := (searchPixivRequest{"猫", searchSortPopular, searchTargetTitleCaption}); got[0] != want {
		t.Fatalf("first search %+v, want %+v", got[0], want)
	}

	a.searchSort = searchSortDateAsc
	a.searchTarget = searchTargetExact
	a.startSearch(a.search)
	if v := a.search.snapshot(); len(v.items) != 0 {
		t.Fatalf("the second search kept the first results: items=%d", len(v.items))
	}
	waitBrowse(t, a.search, "load the second search",
		func(v browseView) bool { return len(v.items) == 1 })
	got = fake.requests()
	if len(got) != 2 {
		t.Fatalf("requests %v, want 2", got)
	}
	if want := (searchPixivRequest{"猫", searchSortDateAsc, searchTargetExact}); got[1] != want {
		t.Fatalf("second search %+v, want %+v", got[1], want)
	}
}

// illustSearchQueries returns the illustration searches the app asked for.
func illustSearchQueries(t *testing.T, a *App) []url.Values {
	t.Helper()
	tr, ok := a.client.HTTP().Transport.(*pageTransport)
	if !ok {
		t.Fatal("the transport is not the page transport")
	}
	var queries []url.Values
	for _, u := range tr.requestedURLs() {
		if !strings.HasPrefix(u, "/v1/search/illust?") {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil {
			t.Fatalf("parse %q: %v", u, err)
		}
		queries = append(queries, parsed.Query())
	}
	return queries
}

// TestSearchAdvancedFiltersReachSearch checks the dates and sizes reach the
// search, with a blank or 0 size sent as no limit.
func TestSearchAdvancedFiltersReachSearch(t *testing.T) {
	a := newSearchApp(pageBodies())
	a.searchStartDate = "2020-01-02"
	a.searchEndDate = "2021-03-04"
	a.searchMinWidth = "600"
	a.searchMaxWidth = "0"
	a.searchMinHeight = ""
	a.searchMaxHeight = "4000"
	a.startSearch(a.search)
	waitBrowse(t, a.search, "load the filtered search",
		func(v browseView) bool { return v.started })

	queries := illustSearchQueries(t, a)
	if len(queries) != 1 {
		t.Fatalf("illustration searches %v, want one", queries)
	}
	q := queries[0]
	want := map[string]string{
		"start_date": "2020-01-02",
		"end_date":   "2021-03-04",
		"width_min":  "600",
		"height_max": "4000",
	}
	for key, value := range want {
		if got := q.Get(key); got != value {
			t.Errorf("%s %q, want %q", key, got, value)
		}
	}
	for _, key := range []string{"width_max", "height_min"} {
		if q.Has(key) {
			t.Errorf("%s sent as %q, want no limit", key, q.Get(key))
		}
	}
}

// TestSearchAdvancedFiltersRejectBadInput checks a bad date or size stops the
// search with its message, without asking the API or recording the word.
func TestSearchAdvancedFiltersRejectBadInput(t *testing.T) {
	cases := []struct {
		name string
		set  func(*App)
		key  string
	}{
		{"bad date", func(a *App) { a.searchStartDate = "2020-13-01" }, "search.invalidDate"},
		{"bad size", func(a *App) { a.searchMinWidth = "abc" }, "search.invalidSize"},
		{"negative size", func(a *App) { a.searchMaxHeight = "-5" }, "search.invalidSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newSearchApp(pageBodies())
			a.searchHistory = &SearchHistory{}
			tc.set(a)
			a.startSearch(a.search)
			if got, want := a.searchErr, a.t(tc.key); got != want {
				t.Errorf("searchErr %q, want %q", got, want)
			}
			if got := illustSearchQueries(t, a); len(got) != 0 {
				t.Errorf("illustration searches %v, want none", got)
			}
			if got := a.searchHistory.Snapshot(); len(got) != 0 {
				t.Errorf("words %v, want none", got)
			}
		})
	}
}

// TestSearchFiltersHiddenOutsideIllust checks the date and size fields stay
// on the illustration tab.
func TestSearchFiltersHiddenOutsideIllust(t *testing.T) {
	a := newSearchApp(pageBodies())
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !tt.HasText("起始日期") {
		t.Fatalf("the date filter is missing: %q", tt.Texts())
	}
	for _, tab := range []string{"小说", "用户"} {
		if err := tt.Click(tab); err != nil {
			t.Fatalf("Click %s tab: %v", tab, err)
		}
		tt.Frame()
		if tt.HasText("起始日期") {
			t.Errorf("the date filter is shown on the %s tab: %q", tab, tt.Texts())
		}
	}
}

// newSearchApp is a signed-in app on the search page whose API answers from
// bodies, without images or a network.
func newSearchApp(bodies map[string]string) *App {
	if _, ok := bodies["/v1/search/illust"]; !ok {
		bodies["/v1/search/illust"] = `{"illusts":[],"next_url":""}`
	}
	a := newPageAppWithBodies(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	a.route = routeSearch
	a.search.query = "猫"
	return a
}

// userSearchQueries returns the user searches the app asked for.
func userSearchQueries(t *testing.T, a *App) []url.Values {
	t.Helper()
	tr, ok := a.client.HTTP().Transport.(*pageTransport)
	if !ok {
		t.Fatal("the transport is not the page transport")
	}
	var queries []url.Values
	for _, u := range tr.requestedURLs() {
		if !strings.HasPrefix(u, "/v1/search/user?") {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil {
			t.Fatalf("parse %q: %v", u, err)
		}
		queries = append(queries, parsed.Query())
	}
	return queries
}

// userTab shows the user tab of a search and waits for its list.
func userTab(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click(a.t("search.user")); err != nil {
		t.Fatalf("Click user tab: %v", err)
	}
	if a.searchKind != searchUser {
		t.Fatalf("searchKind %d, want %d", a.searchKind, searchUser)
	}
	waitUsers(t, a.searchUsers, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
}

// TestSearchUserTabRequestsWordAndFilter checks the user tab searches the
// search box's word with the R18 filter and draws the users.
func TestSearchUserTabRequestsWordAndFilter(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/search/user"] = `{"user_previews":[{"user":{"id":5,"name":"猫画师",` +
		`"account":"cat"}}],"next_url":""}`
	a := newSearchApp(bodies)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	a.startSearch(a.search)
	tt.Frame()
	userTab(t, a, tt)

	for _, want := range []string{"猫画师", "@cat"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	queries := userSearchQueries(t, a)
	if len(queries) != 1 {
		t.Fatalf("user searches = %v, want one", queries)
	}
	if q := queries[0]; q.Get("word") != "猫" || q.Get("filter") != "for_android" {
		t.Errorf("query = %v", q)
	}
}

// TestSearchUserTabPages checks the user tab loads its next page from the
// cursor.
func TestSearchUserTabPages(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/search/user"] = `{"user_previews":[{"user":{"id":5,"name":"猫画师"}}],` +
		`"next_url":"https://app-api.pixiv.net/v1/search/user?word=猫&offset=1"}`
	a := newSearchApp(bodies)
	a.client.HTTP().Transport.(*pageTransport).byURL = map[string]string{
		"/v1/search/user?word=猫&offset=1": `{"user_previews":[{"user":{"id":6,"name":"猫助手"}}],` +
			`"next_url":""}`,
	}
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	a.startSearch(a.search)
	tt.Frame()
	if err := tt.Click(a.t("search.user")); err != nil {
		t.Fatalf("Click user tab: %v", err)
	}
	// The list draws its first page, sees its end, and loads the next one.
	deadline := time.Now().Add(2 * time.Second)
	for len(a.searchUsers.snapshot().users) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("user list has %d users, want 2", len(a.searchUsers.snapshot().users))
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if !tt.HasText("猫助手") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
}

// TestSearchUserTabEmpty checks the user tab's empty state when no user
// matches.
func TestSearchUserTabEmpty(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/search/user"] = `{"user_previews":[],"next_url":""}`
	a := newSearchApp(bodies)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	a.startSearch(a.search)
	tt.Frame()
	if err := tt.Click(a.t("search.user")); err != nil {
		t.Fatalf("Click user tab: %v", err)
	}
	waitUsers(t, a.searchUsers, func(v userListView) bool {
		return v.started && len(v.users) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noSearchResults")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestSearchUserCardOpensUser checks clicking a searched user opens that
// user's page.
func TestSearchUserCardOpensUser(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/search/user"] = `{"user_previews":[{"user":{"id":5,"name":"猫画师"}}],` +
		`"next_url":""}`
	a := newSearchApp(bodies)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	a.startSearch(a.search)
	tt.Frame()
	userTab(t, a, tt)

	if err := tt.Click("猫画师"); err != nil {
		t.Fatalf("Click the user: %v", err)
	}
	if a.route != routeUser {
		t.Fatalf("route %q after the click, want %q", a.route, routeUser)
	}
	if a.userView == nil || a.userView.userID != 5 {
		t.Fatalf("user view %+v, want user 5", a.userView)
	}
	if a.backRoute != routeSearch {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeSearch)
	}
}

// TestSearchAgainResetsUsers checks a new search drops the users the last one
// loaded and asks for the new word.
func TestSearchAgainResetsUsers(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/search/user"] = `{"user_previews":[{"user":{"id":5,"name":"猫画师"}}],` +
		`"next_url":""}`
	a := newSearchApp(bodies)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	a.startSearch(a.search)
	tt.Frame()
	userTab(t, a, tt)
	waitBrowse(t, a.search, "load the illustration search",
		func(v browseView) bool { return v.started })

	a.search.query = "犬"
	// The next search answers with a different user, so a list that kept the
	// last search's users would end with two.
	bodies["/v1/search/user"] = `{"user_previews":[{"user":{"id":6,"name":"犬画师"}}],` +
		`"next_url":""}`
	a.startSearch(a.search)
	waitUsers(t, a.searchUsers, func(v userListView) bool {
		return len(v.users) == 1 && v.users[0].User.Name == "犬画师"
	})
	queries := userSearchQueries(t, a)
	if len(queries) != 2 {
		t.Fatalf("user searches = %v, want two", queries)
	}
	if q := queries[1]; q.Get("word") != "犬" {
		t.Errorf("second query = %v, want the new word", q)
	}
}
