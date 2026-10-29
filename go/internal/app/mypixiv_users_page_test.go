package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// myPixivUsersApp is an English app whose MyPixiv-users list answers with
// body, and whose session is signed in as user 7.
func myPixivUsersApp(body string) *App {
	bodies := pageBodies()
	bodies["/v1/user/mypixiv"] = body
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	return a
}

// openMyPixivUsers opens user 7's page, waits for its works, and clicks the
// MyPixiv entry.
func openMyPixivUsers(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()
	if err := tt.Click(a.t("user.myPixiv")); err != nil {
		t.Fatalf("Click MyPixiv users: %v", err)
	}
}

// TestMyPixivUsersEntry checks the entry loads the list only when it opens,
// draws its users, and going back keeps the user page's works and way back.
func TestMyPixivUsersEntry(t *testing.T) {
	a := myPixivUsersApp(`{"user_previews":[{"user":{"id":8,"name":"好友八",` +
		`"account":"friend8"}}],"next_url":""}`)
	a.route = routeHistory
	tt := ui.NewTester(a.View, 1000, 700)
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt.Frame()
	waitBrowse(t, a.userView.feed, "load", func(v browseView) bool { return len(v.items) == 1 })
	tt.Frame()

	tr := a.client.HTTP().Transport.(*pageTransport)
	if containsPart(tr.requestedURLs(), "/v1/user/mypixiv") {
		t.Fatalf("opening the user page fetched the MyPixiv list: %v", tr.requestedURLs())
	}

	userPage := a.userView
	if err := tt.Click(a.t("user.myPixiv")); err != nil {
		t.Fatalf("Click MyPixiv users: %v", err)
	}
	if a.route != routeUserMyPixiv {
		t.Fatalf("route %q, want %q", a.route, routeUserMyPixiv)
	}
	if a.myPixivUsers == nil || a.myPixivUsers.userID != 7 {
		t.Fatalf("MyPixiv users view %+v, want user 7", a.myPixivUsers)
	}
	tt.Frame()
	waitUsers(t, a.myPixivUsers.feed, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	for _, want := range []string{a.t("user.myPixiv"), "好友八", "@friend8"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if !containsPart(tr.requestedURLs(), "/v1/user/mypixiv") ||
		!containsPart(tr.requestedURLs(), "user_id=7") {
		t.Errorf("requests = %v, want the MyPixiv list of user 7", tr.requestedURLs())
	}

	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back: %v", err)
	}
	if a.route != routeUser {
		t.Errorf("route %q after going back, want %q", a.route, routeUser)
	}
	if a.userView != userPage {
		t.Error("the user page is not the one the MyPixiv list was opened from")
	}
	if a.backRoute != routeHistory {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeHistory)
	}
	if a.myPixivUsers != nil {
		t.Error("the MyPixiv users page is still open")
	}
	tt.Frame()
	if v := a.userView.feed.snapshot(); len(v.items) != 1 || v.items[0].ID != 11 {
		t.Errorf("user works after going back = %+v, want work 11", v.items)
	}
	if n := countPart(tr.requestedURLs(), "type=illust"); n != 1 {
		t.Errorf("illustrations requested %d times, want 1: %v", n, tr.requestedURLs())
	}
}

// TestMyPixivUsersPages checks the list loads its next page from the cursor
// the first one returns.
func TestMyPixivUsersPages(t *testing.T) {
	a := myPixivUsersApp(`{"user_previews":[{"user":{"id":8,"name":"好友八"}}],` +
		`"next_url":"https://app-api.pixiv.net/v1/user/mypixiv?user_id=7&offset=1"}`)
	a.client.HTTP().Transport.(*pageTransport).byURL = map[string]string{
		"/v1/user/mypixiv?user_id=7&offset=1": `{"user_previews":[{"user":{"id":10,"name":"好友十"}}],` +
			`"next_url":""}`,
	}
	tt := ui.NewTester(a.View, 1000, 700)
	openMyPixivUsers(t, a, tt)
	deadline := time.Now().Add(2 * time.Second)
	for len(a.myPixivUsers.feed.snapshot().users) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the list has %d users, want 2",
				len(a.myPixivUsers.feed.snapshot().users))
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if !tt.HasText("好友十") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	if got := tr.requestedURLs(); len(got) == 0 || !strings.Contains(got[len(got)-1], "offset=1") {
		t.Errorf("requests = %v, want the cursor's page last", got)
	}
}

// TestMyPixivUsersEmpty checks a list with no users says so.
func TestMyPixivUsersEmpty(t *testing.T) {
	a := myPixivUsersApp(`{"user_previews":[],"next_url":""}`)
	tt := ui.NewTester(a.View, 1000, 700)
	openMyPixivUsers(t, a, tt)
	tt.Frame()
	waitUsers(t, a.myPixivUsers.feed, func(v userListView) bool {
		return v.started && len(v.users) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noMyPixiv")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestMyPixivUsersFailed checks a failed page shows its error and a retry.
func TestMyPixivUsersFailed(t *testing.T) {
	a := myPixivUsersApp(`{"user_previews":`)
	tt := ui.NewTester(a.View, 1000, 700)
	openMyPixivUsers(t, a, tt)
	tt.Frame()
	waitUsers(t, a.myPixivUsers.feed, func(v userListView) bool { return v.err != "" })
	tt.Frame()
	failed := strings.Split(a.t("user.myPixivLoadFailed"), "%s")[0]
	if !tt.HasText(failed) {
		t.Errorf("failure state missing: %q", tt.Texts())
	}
	if !tt.HasText(a.t("common.retry")) {
		t.Errorf("retry button missing: %q", tt.Texts())
	}
}
