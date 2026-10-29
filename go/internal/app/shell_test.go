package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/saucenao"
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
	tt := ui.NewTester(a.View, 1024, 720)
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
// the rail's bottom when the window is shorter than the design window.
func TestShellRailSettingsReachableInShortWindow(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1280, 600)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("Click Settings: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q, want %q", a.route, routeSettings)
	}
}

// TestShellRailGroupsReachDestinations checks the rail's groups hold the pages
// that are not top-level: a group opens its menu and a row shows its page at
// the design window's size, without scrolling the rail.
func TestShellRailGroupsReachDestinations(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	if _, ok := tt.Find("新作"); ok {
		t.Fatalf("新作 is in the rail before its group opens: %q", tt.Texts())
	}
	for _, c := range []struct {
		group string
		page  string
		want  route
	}{
		{"作品", "新作", routeNew},
		{"作品", "系列", routeSeriesList},
		{"作品", "MyPixiv", routeMyPixiv},
		{"作品", "特辑", routeSpotlight},
		{"作品", "追更", routeSubs},
		{"用户", "推荐作者", routeRecommendedUsers},
		{"库", "历史", routeHistory},
		{"库", "下载", routeDownloads},
		{"库", "稍后再看", routeWatchLater},
	} {
		if err := tt.Click(c.group); err != nil {
			t.Fatalf("open %s: %v", c.group, err)
		}
		tt.Frame()
		if err := tt.Click(c.page); err != nil {
			t.Fatalf("Click %s in %s: %v", c.page, c.group, err)
		}
		tt.Frame()
		if a.route != c.want {
			t.Errorf("route %q after %s, want %q", a.route, c.page, c.want)
		}
		if navMenuState.open != "" {
			t.Errorf("the %s menu stayed open after choosing %s", c.group, c.page)
		}
	}
}

// TestShellRailDoesNotScroll checks the rail does not scroll: its
// destinations stay put when the pointer scrolls over it, and Settings stays
// in reach at the design window.
func TestShellRailDoesNotScroll(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	home, ok := tt.Find("首页")
	if !ok {
		t.Fatalf("首页 missing: %q", tt.Texts())
	}
	settings, ok := tt.Find("设置")
	if !ok {
		t.Fatalf("设置 missing: %q", tt.Texts())
	}
	if settings.Y+settings.H > 720 {
		t.Errorf("Settings at %+v is outside the 720 window", settings)
	}
	tt.Scroll(30, 400, 0, 400)
	tt.Frame()
	homeAfter, _ := tt.Find("首页")
	settingsAfter, _ := tt.Find("设置")
	if homeAfter.Y != home.Y || settingsAfter.Y != settings.Y {
		t.Errorf("the rail scrolled: home %v -> %v, settings %v -> %v",
			home.Y, homeAfter.Y, settings.Y, settingsAfter.Y)
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("Click Settings: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q, want %q", a.route, routeSettings)
	}
}

// navGo shows a rail destination: a top-level one directly, and one inside a
// group through the group's menu.
func navGo(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	group := map[string]string{
		"新作":      "作品",
		"MyPixiv": "作品",
		"特辑":      "作品",
		"追更":      "作品",
		"推荐作者":    "用户",
		"历史":      "库",
		"下载":      "库",
		"稍后再看":    "库",
	}[label]
	if group != "" {
		if err := tt.Click(group); err != nil {
			t.Fatalf("open %s: %v", group, err)
		}
		tt.Frame()
	}
	if err := tt.Click(label); err != nil {
		t.Fatalf("Click %s: %v", label, err)
	}
	tt.Frame()
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

// TestShellRail checks the rail shows its seven top-level destinations inside
// its width at the design window, with Settings held to the bottom and the
// grouped pages out of the rail until their menu opens.
func TestShellRail(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	for _, label := range []string{"首页", "搜索", "收藏", "作品", "用户", "库", "设置"} {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%s missing: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > navWidth+1 {
			t.Errorf("%s at %+v, outside the %v rail", label, r, navWidth)
		}
		if r.Y < 0 || r.Y+r.H > 720 {
			t.Errorf("%s at %+v, outside the window", label, r)
		}
	}
	for _, label := range []string{"新作", "MyPixiv", "特辑", "追更", "推荐作者", "历史", "下载", "稍后再看"} {
		if _, ok := tt.Find(label); ok {
			t.Errorf("%s is in the rail before its group opens", label)
		}
	}
	home, _ := tt.Find("首页")
	settings, _ := tt.Find("设置")
	if settings.Y <= home.Y {
		t.Errorf("settings at %v is not under the destinations (%v)", settings.Y, home.Y)
	}
	if settings.Y < 720/2 {
		t.Errorf("settings at %v is not held to the bottom", settings.Y)
	}
}

// TestShellFitsWindow checks the rail and the app bar fit the narrow and the
// design window, with nothing pushed past the right or bottom edge.
func TestShellFitsWindow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{1024, 720}, {1024, 768}, {1280, 820}} {
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
	if err := tt.Click("小说"); err != nil {
		t.Fatalf("Click 小说 tab: %v", err)
	}
	tt.Frame()
	if tt.HasText("起始日期") {
		t.Errorf("the date filter is shown on the novel tab: %q", tt.Texts())
	}
	searchUserTab(t, a, tt)
	if tt.HasText("起始日期") {
		t.Errorf("the date filter is shown on the user tab: %q", tt.Texts())
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

// searchTabsFocused reports whether the search tab line has the keyboard
// focus, whichever tab is chosen.
func searchTabsFocused(tt *ui.Tester, a *App) bool {
	// The user tab's name is also the rail's Users group, so only the labels
	// no rail item shares can tell the tab line apart from the rail.
	for _, key := range []string{"search.illust", "search.novel", "search.sauce"} {
		if tt.Focused(a.t(key)) {
			return true
		}
	}
	return false
}

// searchUserTab selects the user tab of the search page. The rail's Users
// group shares the tab's name, so the tab line is reached by the keyboard
// rather than by a text click.
func searchUserTab(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	if a.searchKind == searchUser {
		return
	}
	// A long saved history would put the tab line past a fixed number of Tab
	// presses, and the tab shares its name with the rail's Users group.
	a.searchHistory = &SearchHistory{}
	tt.Frame()
	if !searchTabsFocused(tt, a) {
		reached := false
		for i := 0; i < 20 && !reached; i++ {
			tt.Key(0, ui.KeyTab)
			reached = searchTabsFocused(tt, a)
		}
		if !reached {
			t.Fatalf("Tab never reached the search tabs: %q", tt.Texts())
		}
	}
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyRight)
	if a.searchKind != searchUser {
		t.Fatalf("searchKind %d, want %d", a.searchKind, searchUser)
	}
	tt.Frame()
}

// userTab shows the user tab of a search and waits for its list.
func userTab(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	searchUserTab(t, a, tt)
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
	searchUserTab(t, a, tt)
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
	searchUserTab(t, a, tt)
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

// newSauceApp is a signed-in app on the search page with a reverse image
// search function that answers without a network.
func newSauceApp(t *testing.T, search func(context.Context, string, string) (*saucenao.Result, error)) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := New()
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.route = routeSearch
	a.searchKind = searchSauce
	a.settings.SauceNAOAPIKey = "secret-key"
	a.sauceSearchFn = search
	return a
}

// waitSauce waits for the reverse image search goroutine to settle the state
// as want.
func waitSauce(t *testing.T, st *sauceState, what string, want func(v sauceView) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v := st.snapshot(); want(v) {
			return
		}
		if time.Now().After(deadline) {
			v := st.snapshot()
			t.Fatalf("reverse image search did not %s: loading=%v searched=%v err=%q results=%d",
				what, v.loading, v.searched, v.err, len(v.items))
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSauceTabPromptsForKey checks the fourth search tab exists and a missing
// key sends the user to the settings page instead of searching.
func TestSauceTabPromptsForKey(t *testing.T) {
	a := newSauceApp(t, nil)
	a.settings.SauceNAOAPIKey = ""
	a.searchKind = searchIllust
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if err := tt.Click("以图搜图"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.searchKind != searchSauce {
		t.Fatalf("searchKind %d, want %d", a.searchKind, searchSauce)
	}
	tt.Frame()
	if !tt.HasText("先在设置中填写 SauceNAO API Key") {
		t.Errorf("key prompt missing: %q", tt.Texts())
	}
}

// TestSauceSearchShowsResults checks the URL and key reach the search, and a
// result shows its similarity, title, creator and source.
func TestSauceSearchShowsResults(t *testing.T) {
	var mu sync.Mutex
	var gotKey, gotURL string
	a := newSauceApp(t, func(_ context.Context, key, imageURL string) (*saucenao.Result, error) {
		mu.Lock()
		gotKey, gotURL = key, imageURL
		mu.Unlock()
		return &saucenao.Result{Entries: []saucenao.Entry{{
			Similarity: 97.4,
			Title:      "作品标题",
			Creator:    "画师",
			AuthorURL:  "https://www.pixiv.net/users/1",
			ExtURLs:    []string{"https://example.com/source"},
		}}}, nil
	})
	a.sauceURL = "https://example.com/image.jpg"
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if err := tt.Click("查找来源"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitSauce(t, a.sauce, "load the source", func(v sauceView) bool {
		return !v.loading && len(v.items) == 1
	})
	tt.Frame()
	for _, text := range []string{"97.4%", "作品标题", "画师", "https://www.pixiv.net/users/1"} {
		if !tt.HasText(text) {
			t.Errorf("%q missing: %q", text, tt.Texts())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if gotKey != "secret-key" {
		t.Errorf("key = %q, want secret-key", gotKey)
	}
	if gotURL != "https://example.com/image.jpg" {
		t.Errorf("url = %q, want the image URL", gotURL)
	}
}

// TestSauceSearchEmptyAndFailed checks an empty answer says so, and a failed
// one shows why.
func TestSauceSearchEmptyAndFailed(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		a := newSauceApp(t, func(context.Context, string, string) (*saucenao.Result, error) {
			return nil, saucenao.ErrNoResults
		})
		a.sauceURL = "https://example.com/image.jpg"
		tt := ui.NewTester(a.View, 1000, 700)
		tt.Frame()
		if err := tt.Click("查找来源"); err != nil {
			t.Fatalf("Click: %v", err)
		}
		waitSauce(t, a.sauce, "end empty", func(v sauceView) bool {
			return v.searched && !v.loading && v.err == ""
		})
		tt.Frame()
		if !tt.HasText("没有找到匹配的来源") {
			t.Errorf("empty state missing: %q", tt.Texts())
		}
	})
	t.Run("failed", func(t *testing.T) {
		a := newSauceApp(t, func(context.Context, string, string) (*saucenao.Result, error) {
			return nil, errors.New("offline")
		})
		a.sauceURL = "https://example.com/image.jpg"
		tt := ui.NewTester(a.View, 1000, 700)
		tt.Frame()
		if err := tt.Click("查找来源"); err != nil {
			t.Fatalf("Click: %v", err)
		}
		waitSauce(t, a.sauce, "fail", func(v sauceView) bool {
			return v.searched && !v.loading && v.err != ""
		})
		tt.Frame()
		if !tt.HasText("查找失败：offline") {
			t.Errorf("failure missing: %q", tt.Texts())
		}
	})
}

// TestSauceResultOpensSource checks a click on a result's source opens it in
// the browser, not in the app.
func TestSauceResultOpensSource(t *testing.T) {
	a := newSauceApp(t, func(context.Context, string, string) (*saucenao.Result, error) {
		return &saucenao.Result{Entries: []saucenao.Entry{{
			Similarity: 90,
			Title:      "作品标题",
			AuthorURL:  "https://www.pixiv.net/users/1",
		}}}, nil
	})
	var opened []string
	a.openExternalFn = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	a.sauceURL = "https://example.com/image.jpg"
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if err := tt.Click("查找来源"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitSauce(t, a.sauce, "load the source", func(v sauceView) bool {
		return !v.loading && len(v.items) == 1
	})
	tt.Frame()
	if err := tt.Click("https://www.pixiv.net/users/1"); err != nil {
		t.Fatalf("Click source: %v", err)
	}
	if len(opened) != 1 || opened[0] != "https://www.pixiv.net/users/1" {
		t.Errorf("opened = %v, want the source URL", opened)
	}
}

// waitSearchTags waits for the search page's trending tags to land.
func waitSearchTags(t *testing.T, a *App, want func(tags []pixiv.TrendingTag) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.searchSideMu.Lock()
		tags := a.searchTags
		a.searchSideMu.Unlock()
		if want(tags) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("trending tags did not land: %+v", tags)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitLinkError waits for the search page's pasted-link failure.
func waitLinkError(t *testing.T, a *App) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.searchSideMu.Lock()
		msg := a.searchLinkErr
		a.searchSideMu.Unlock()
		if msg != "" {
			return msg
		}
		if time.Now().After(deadline) {
			t.Fatal("the pasted link did not say why it failed")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSearchTrendingChipsSearch checks the search page loads the trending
// tags once, shows them under the box, and a chip searches its tag.
func TestSearchTrendingChipsSearch(t *testing.T) {
	a := newSearchApp(pageBodies())
	a.searchHistory = &SearchHistory{}
	a.search.query = ""
	var loads atomic.Int32
	a.trendingFn = func(context.Context) ([]pixiv.TrendingTag, error) {
		loads.Add(1)
		return []pixiv.TrendingTag{{Tag: "猫"}, {Tag: "风景", TranslatedName: "landscape"}}, nil
	}

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	waitSearchTags(t, a, func(tags []pixiv.TrendingTag) bool { return len(tags) == 2 })
	tt.Frame()
	for _, want := range []string{"趋势标签", "猫", "风景"} {
		if !tt.HasText(want) {
			t.Fatalf("%q missing: %q", want, tt.Texts())
		}
	}
	tt.Frame()
	if got := loads.Load(); got != 1 {
		t.Errorf("trending loads = %d, want one", got)
	}

	if err := tt.Click("猫"); err != nil {
		t.Fatalf("Click chip: %v", err)
	}
	if a.search.query != "猫" {
		t.Errorf("query %q after the chip click, want 猫", a.search.query)
	}
	waitBrowse(t, a.search, "load the chip's search",
		func(v browseView) bool { return v.started })
	queries := illustSearchQueries(t, a)
	if len(queries) != 1 || queries[0].Get("word") != "猫" {
		t.Errorf("illustration searches %v, want one for 猫", queries)
	}
}

// TestParsePixivLink checks the search box reads a whole Pixiv URL's page and
// id, and treats anything else as a search word.
func TestParsePixivLink(t *testing.T) {
	cases := []struct {
		raw  string
		link pixivLink
		ok   bool
	}{
		{"https://www.pixiv.net/artworks/123", pixivLink{linkWork, 123}, true},
		{"https://pixiv.net/i/123", pixivLink{linkWork, 123}, true},
		{"https://www.pixiv.net/en/artworks/456?foo=1", pixivLink{linkWork, 456}, true},
		{"https://pixiv.net/n/789", pixivLink{linkNovel, 789}, true},
		{"https://www.pixiv.net/users/42/", pixivLink{linkUser, 42}, true},
		{"  https://pixiv.net/artworks/7  ", pixivLink{linkWork, 7}, true},
		{"12345", pixivLink{}, false},
		{"pixiv.net/artworks/123", pixivLink{}, false},
		{"https://example.com/artworks/1", pixivLink{}, false},
		{"https://pixiv.net/artworks", pixivLink{}, false},
		{"https://pixiv.net/artworks/abc", pixivLink{}, false},
	}
	for _, tc := range cases {
		got, ok := parsePixivLink(tc.raw)
		if ok != tc.ok || got != tc.link {
			t.Errorf("parsePixivLink(%q) = %+v, %v; want %+v, %v", tc.raw, got, ok, tc.link, tc.ok)
		}
	}
}

// TestSearchPixivLinksOpenPages checks a pasted Pixiv URL opens its page
// instead of searching for the URL, for a work, a novel and a user.
func TestSearchPixivLinksOpenPages(t *testing.T) {
	t.Run("work", func(t *testing.T) {
		a := newSearchApp(pageBodies())
		a.history = &History{}
		a.searchHistory = &SearchHistory{}
		var fetched atomic.Int64
		a.illustrationFn = func(_ context.Context, id int64) (*pixiv.Illustration, error) {
			fetched.Store(id)
			return &pixiv.Illustration{ID: id, Title: "链接作品", PageCount: 1}, nil
		}
		a.search.query = "https://www.pixiv.net/artworks/123"
		a.startSearch(a.search)

		// Opening records the work in the history, whose lock orders the test
		// after the goroutine that opened it.
		deadline := time.Now().Add(2 * time.Second)
		for len(a.history.Snapshot()) == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the linked work did not open")
			}
			time.Sleep(time.Millisecond)
		}
		if got := fetched.Load(); got != 123 {
			t.Errorf("fetched work %d, want 123", got)
		}
		if a.viewer == nil || a.viewer.work.ID != 123 {
			t.Fatalf("viewer = %+v, want work 123", a.viewer)
		}
		if got := illustSearchQueries(t, a); len(got) != 0 {
			t.Errorf("illustration searches %v, want none", got)
		}
		if got := a.searchHistory.Snapshot(); len(got) != 0 {
			t.Errorf("words %v, want none", got)
		}
	})

	t.Run("novel", func(t *testing.T) {
		a := newSearchApp(pageBodies())
		a.searchHistory = &SearchHistory{}
		a.novelContent = func(context.Context, int64) (string, error) { return "正文", nil }
		a.search.query = "https://pixiv.net/n/456"
		a.startSearch(a.search)

		if a.route != routeNovel {
			t.Errorf("route %q, want %q", a.route, routeNovel)
		}
		if a.novelView == nil || a.novelView.novelID != 456 {
			t.Fatalf("novel view = %+v, want novel 456", a.novelView)
		}
		if got := illustSearchQueries(t, a); len(got) != 0 {
			t.Errorf("illustration searches %v, want none", got)
		}
		if got := a.searchHistory.Snapshot(); len(got) != 0 {
			t.Errorf("words %v, want none", got)
		}
	})

	t.Run("user", func(t *testing.T) {
		a := newSearchApp(pageBodies())
		a.searchHistory = &SearchHistory{}
		a.search.query = "https://www.pixiv.net/users/789"
		a.startSearch(a.search)

		if a.route != routeUser {
			t.Errorf("route %q, want %q", a.route, routeUser)
		}
		if a.userView == nil || a.userView.userID != 789 {
			t.Fatalf("user view = %+v, want user 789", a.userView)
		}
		if got := illustSearchQueries(t, a); len(got) != 0 {
			t.Errorf("illustration searches %v, want none", got)
		}
		if got := a.searchHistory.Snapshot(); len(got) != 0 {
			t.Errorf("words %v, want none", got)
		}
	})
}

// TestSearchPixivLinkFailureShowsMessage checks a linked work that cannot be
// fetched stays on the search page with the reason under the box.
func TestSearchPixivLinkFailureShowsMessage(t *testing.T) {
	a := newSearchApp(pageBodies())
	a.searchHistory = &SearchHistory{}
	a.illustrationFn = func(context.Context, int64) (*pixiv.Illustration, error) {
		return nil, errors.New("网络错误")
	}
	a.search.query = "https://www.pixiv.net/artworks/123"
	a.startSearch(a.search)

	msg := waitLinkError(t, a)
	want := a.tf("search.openLinkFailed", "网络错误")
	if msg != want {
		t.Errorf("link message %q, want %q", msg, want)
	}
	if a.viewer != nil {
		t.Errorf("viewer open for a work that failed")
	}
	if got := a.searchHistory.Snapshot(); len(got) != 0 {
		t.Errorf("words %v, want none", got)
	}

	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	if !tt.HasText(want) {
		t.Errorf("the link failure is not shown: %q", tt.Texts())
	}
}

// TestSearchBareNumberStillSearches checks only a whole Pixiv URL opens a
// page: a plain number and another site's link stay search words.
func TestSearchBareNumberStillSearches(t *testing.T) {
	for _, word := range []string{"12345", "https://example.com/artworks/123", "pixiv.net/artworks/123"} {
		t.Run(word, func(t *testing.T) {
			a := newSearchApp(pageBodies())
			a.searchHistory = &SearchHistory{}
			a.search.query = word
			a.startSearch(a.search)
			waitBrowse(t, a.search, "load the search",
				func(v browseView) bool { return v.started })

			if a.viewer != nil || a.route != routeSearch {
				t.Errorf("viewer %v, route %q; want the search page", a.viewer, a.route)
			}
			queries := illustSearchQueries(t, a)
			if len(queries) != 1 || queries[0].Get("word") != word {
				t.Errorf("illustration searches %v, want one for %q", queries, word)
			}
			if got := a.searchHistory.Snapshot(); len(got) != 1 || got[0] != word {
				t.Errorf("words %v, want [%s]", got, word)
			}
		})
	}
}
