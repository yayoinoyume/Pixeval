package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// recommendedUsersApp is an English app whose recommended-authors list answers
// with body, and whose session is signed in as user 7.
func recommendedUsersApp(body string) *App {
	bodies := pageBodies()
	bodies["/v1/user/recommended"] = body
	a := englishPageApp(bodies)
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 7, Name: "画师七"}}
	a.client.SetToken(a.session)
	return a
}

// TestRecommendedUsersPageLoadsOnDemand checks the page fetches the list only
// when it shows, draws its users and opens a user's page on a click.
func TestRecommendedUsersPageLoadsOnDemand(t *testing.T) {
	a := recommendedUsersApp(`{"users":[{"id":8,"name":"推荐八","account":"rec8"}],` +
		`"next_url":""}`)
	tt := ui.NewTester(a.View, 1000, 700)
	tt.Frame()
	tr := a.client.HTTP().Transport.(*pageTransport)
	if containsPart(tr.requestedURLs(), "/v1/user/recommended") {
		t.Fatalf("the list loaded before the page showed: %v", tr.requestedURLs())
	}

	a.goRoute(routeRecommendedUsers)
	tt.Frame()
	waitUsers(t, a.recommendedUsers, func(v userListView) bool { return len(v.users) == 1 })
	tt.Frame()
	for _, want := range []string{a.t("recommendedUsers.title"), "推荐八", "@rec8"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
	if !containsPart(tr.requestedURLs(), "/v1/user/recommended") ||
		!containsPart(tr.requestedURLs(), "filter=for_android") {
		t.Errorf("requests = %v, want the recommended list with the filter", tr.requestedURLs())
	}

	if err := tt.Click("推荐八"); err != nil {
		t.Fatalf("Click user card: %v", err)
	}
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	if a.route != routeUser {
		t.Errorf("route %q after the click, want %q", a.route, routeUser)
	}
	if a.backRoute != routeRecommendedUsers {
		t.Errorf("backRoute %q, want %q", a.backRoute, routeRecommendedUsers)
	}
	if !containsPart(tr.requestedURLs(), "user_id=8") {
		t.Errorf("requests = %v, want the detail of user 8", tr.requestedURLs())
	}
}

// TestRecommendedUsersPagePages checks the list loads its next page from the
// cursor the first one returns.
func TestRecommendedUsersPagePages(t *testing.T) {
	a := recommendedUsersApp(`{"users":[{"id":8,"name":"推荐八"}],` +
		`"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=1"}`)
	a.client.HTTP().Transport.(*pageTransport).byURL = map[string]string{
		"/v1/user/recommended?offset=1": `{"users":[{"id":10,"name":"推荐十"}],` +
			`"next_url":""}`,
	}
	tt := ui.NewTester(a.View, 1000, 700)
	a.goRoute(routeRecommendedUsers)
	deadline := time.Now().Add(2 * time.Second)
	for len(a.recommendedUsers.snapshot().users) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the list has %d users, want 2",
				len(a.recommendedUsers.snapshot().users))
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if !tt.HasText("推荐十") {
		t.Errorf("second page missing: %q", tt.Texts())
	}
	tr := a.client.HTTP().Transport.(*pageTransport)
	if got := tr.requestedURLs(); len(got) == 0 || !strings.Contains(got[len(got)-1], "offset=1") {
		t.Errorf("requests = %v, want the cursor's page last", got)
	}
}

// TestRecommendedUsersPageEmpty checks a list with no users says so.
func TestRecommendedUsersPageEmpty(t *testing.T) {
	a := recommendedUsersApp(`{"users":[],"next_url":""}`)
	tt := ui.NewTester(a.View, 1000, 700)
	a.goRoute(routeRecommendedUsers)
	tt.Frame()
	waitUsers(t, a.recommendedUsers, func(v userListView) bool {
		return v.started && len(v.users) == 0
	})
	tt.Frame()
	if !tt.HasText(a.t("user.noRecommendedUsers")) {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
	for _, want := range []string{a.t("user.recommendedEmptyHint"), a.t("user.recommendedEmptyAction")} {
		if !tt.HasText(want) {
			t.Errorf("empty state missing %q: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(a.t("user.recommendedEmptyAction")); err != nil {
		t.Fatalf("Click the empty state's action: %v", err)
	}
	if a.route != routeSearch {
		t.Errorf("route %q after the action, want %q", a.route, routeSearch)
	}
}
