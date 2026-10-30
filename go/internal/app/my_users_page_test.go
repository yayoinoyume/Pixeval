package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// mySocialUsersApp is an English app whose own people lists answer with their
// sample users, and whose session is signed in as user 7.
func mySocialUsersApp() *App {
	bodies := pageBodies()
	bodies["/v1/user/mypixiv"] = `{"user_previews":[{"user":{"id":10,"name":"好友十",` +
		`"account":"friend10"}}],"next_url":""}`
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	return a
}

// mySocialPage is one of the Users group's own-list pages: its menu label,
// route, list and the request its first page asks for.
type mySocialPage struct {
	label  string
	route  route
	state  func(*App) *userListState
	path   string
	params []string
	user   string
}

func mySocialPages(a *App) []mySocialPage {
	return []mySocialPage{
		{a.t("nav.myFollowing"), routeMyFollowing, func(a *App) *userListState { return a.myFollowing },
			"/v1/user/following", []string{"user_id=7", "restrict=public"}, "关注八"},
		{a.t("nav.myFollowers"), routeMyFollowers, func(a *App) *userListState { return a.myFollowers },
			"/v1/user/follower", []string{"user_id=7"}, "粉丝九"},
		{a.t("nav.myPixivUsers"), routeMyPixivUsers, func(a *App) *userListState { return a.myPixivUsersList },
			"/v1/user/mypixiv", []string{"user_id=7"}, "好友十"},
	}
}

// TestMySocialUsersNav checks the Users menu lists the three own lists after
// the recommended authors, that choosing each shows its page and asks for the
// signed-in user's list, and that the rail keeps its seven top-level items.
func TestMySocialUsersNav(t *testing.T) {
	a := mySocialUsersApp()
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	topLevel := []string{
		a.t("nav.home"), a.t("nav.search"), a.t("nav.bookmarks"),
		a.t("nav.works"), a.t("nav.users"), a.t("nav.library"), a.t("nav.settings"),
	}
	found := 0
	for _, label := range topLevel {
		if _, ok := tt.Find(label); ok {
			found++
		} else {
			t.Errorf("the rail's top-level %q is missing: %q", label, tt.Texts())
		}
	}
	if found != 7 {
		t.Errorf("the rail has %d top-level labels, want 7", found)
	}

	pages := mySocialPages(a)
	if err := tt.Click(a.t("nav.users")); err != nil {
		t.Fatalf("open Users: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find(a.t("nav.recommendedUsers")); !ok {
		t.Errorf("the recommended authors are missing from the Users menu: %q", tt.Texts())
	}
	var ys []float32
	for _, p := range pages {
		r, ok := tt.Find(p.label)
		if !ok {
			t.Fatalf("%q is missing from the Users menu: %q", p.label, tt.Texts())
		}
		ys = append(ys, r.Y)
	}
	for i := 1; i < len(ys); i++ {
		if ys[i] <= ys[i-1] {
			t.Errorf("the Users menu is not in order: %v", ys)
		}
	}

	tr := a.client.HTTP().Transport.(*pageTransport)
	for i, p := range pages {
		if i > 0 {
			if err := tt.Click(a.t("nav.users")); err != nil {
				t.Fatalf("open Users: %v", err)
			}
			tt.Frame()
		}
		if err := tt.Click(p.label); err != nil {
			t.Fatalf("Click %s: %v", p.label, err)
		}
		tt.Frame()
		if a.route != p.route {
			t.Fatalf("route %q after %s, want %q", a.route, p.label, p.route)
		}
		if navMenuState.open != "" {
			t.Errorf("the Users menu stayed open after choosing %s", p.label)
		}
		waitUsers(t, p.state(a), func(v userListView) bool { return len(v.users) == 1 })
		tt.Frame()
		if !tt.HasText(p.user) {
			t.Errorf("%q missing from %s: %q", p.user, p.label, tt.Texts())
		}
		requested := false
		for _, u := range tr.requestedURLs() {
			if !strings.Contains(u, p.path) {
				continue
			}
			requested = true
			for _, param := range p.params {
				if !strings.Contains(u, param) {
					t.Errorf("%s request %q, want %q", p.label, u, param)
				}
			}
		}
		if !requested {
			t.Errorf("%s requested none of %v: %v", p.label, tr.requestedURLs(), p.path)
		}
	}
}

// TestMySocialUsersEmpty checks each own list with no users says so.
func TestMySocialUsersEmpty(t *testing.T) {
	bodies := pageBodies()
	for _, path := range []string{"/v1/user/following", "/v1/user/follower", "/v1/user/mypixiv"} {
		bodies[path] = `{"user_previews":[],"next_url":""}`
	}
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	if err := tt.Click(a.t("nav.users")); err != nil {
		t.Fatalf("open Users: %v", err)
	}
	tt.Frame()

	for i, p := range mySocialPages(a) {
		if i > 0 {
			if err := tt.Click(a.t("nav.users")); err != nil {
				t.Fatalf("open Users: %v", err)
			}
			tt.Frame()
		}
		if err := tt.Click(p.label); err != nil {
			t.Fatalf("Click %s: %v", p.label, err)
		}
		tt.Frame()
		waitUsers(t, p.state(a), func(v userListView) bool {
			return v.started && len(v.users) == 0
		})
		tt.Frame()
		empty := a.t("user.noMyFollowing")
		switch p.route {
		case routeMyFollowers:
			empty = a.t("user.noMyFollowers")
		case routeMyPixivUsers:
			empty = a.t("user.noMyPixiv")
		}
		if !tt.HasText(empty) {
			t.Errorf("%s footer %q missing: %q", p.label, empty, tt.Texts())
		}
	}
}

// TestMySocialUsersFailure checks a list that fails shows the failure and a
// retry button.
func TestMySocialUsersFailure(t *testing.T) {
	bodies := pageBodies()
	bodies["/v1/user/following"] = `{`
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	if err := tt.Click(a.t("nav.users")); err != nil {
		t.Fatalf("open Users: %v", err)
	}
	tt.Frame()
	if err := tt.Click(a.t("nav.myFollowing")); err != nil {
		t.Fatalf("Click My following: %v", err)
	}
	tt.Frame()
	waitUsers(t, a.myFollowing, func(v userListView) bool { return v.err != "" })
	tt.Frame()
	if !tt.HasText(a.t("common.retry")) {
		t.Errorf("the retry button is missing: %q", tt.Texts())
	}
	failed := false
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "Cannot load the following list:") {
			failed = true
		}
	}
	if !failed {
		t.Errorf("the failure message is missing: %q", tt.Texts())
	}
}

// TestMySocialUsersReturn checks a user's page opened from an own list returns
// to that list, and Escape leaves the list alone.
func TestMySocialUsersReturn(t *testing.T) {
	a := mySocialUsersApp()
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	if err := tt.Click(a.t("nav.users")); err != nil {
		t.Fatalf("open Users: %v", err)
	}
	tt.Frame()
	if err := tt.Click(a.t("nav.myFollowing")); err != nil {
		t.Fatalf("Click My following: %v", err)
	}
	tt.Frame()
	waitUsers(t, a.myFollowing, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.route != routeMyFollowing {
		t.Errorf("route %q after Escape, want %q", a.route, routeMyFollowing)
	}

	if err := tt.Click("关注八"); err != nil {
		t.Fatalf("Click the user: %v", err)
	}
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	if a.route != routeUser || a.backRoute != routeMyFollowing {
		t.Fatalf("route %q backRoute %q, want %q and %q",
			a.route, a.backRoute, routeUser, routeMyFollowing)
	}
	tt.Frame()
	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back: %v", err)
	}
	tt.Frame()
	if a.route != routeMyFollowing {
		t.Errorf("route %q after going back, want %q", a.route, routeMyFollowing)
	}
	if v := a.myFollowing.snapshot(); len(v.users) != 1 || v.users[0].User.ID != 8 {
		t.Errorf("the list after going back = %+v, want user 8", v.users)
	}
}

// TestMySocialUsersClearedOnAccountSwitch checks the own lists drop the
// account before them and load the next account's lists.
func TestMySocialUsersClearedOnAccountSwitch(t *testing.T) {
	a := mySocialUsersApp()
	tt := ui.NewTester(a.View, 1024, 720)
	a.goRoute(routeMyFollowing)
	tt.Frame()
	waitUsers(t, a.myFollowing, func(v userListView) bool { return len(v.users) == 1 })
	old := a.myFollowing

	a.applySession(&pixiv.Token{User: pixiv.TokenUser{ID: 9, Name: "画师九"}})
	if a.myFollowing == old {
		t.Fatal("the following list kept the account before")
	}
	if v := a.myFollowing.snapshot(); len(v.users) != 0 || v.started {
		t.Fatalf("the next account's list starts with %+v, want empty", v)
	}
	// The switch returns to Home in one tab, so the list loads when its page
	// is shown again.
	a.goRoute(routeMyFollowing)
	tt.Frame()
	waitUsers(t, a.myFollowing, func(v userListView) bool { return len(v.users) == 1 })
	tr := a.client.HTTP().Transport.(*pageTransport)
	if n := countPart(tr.requestedURLs(), "user_id=9"); n != 1 {
		t.Errorf("requests for user 9 = %d, want 1: %v", n, tr.requestedURLs())
	}
	for _, u := range tr.requestedURLs() {
		if strings.Contains(u, "/v1/user/following") && strings.Contains(u, "user_id=9") {
			return
		}
	}
	t.Errorf("requests = %v, want the following list of user 9", tr.requestedURLs())
}
