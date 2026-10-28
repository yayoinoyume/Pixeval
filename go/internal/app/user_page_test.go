package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// pageBodies is the app API's JSON for a user page and a series page, keyed by
// the path prefix that asks for it.
func pageBodies() map[string]string {
	return map[string]string{
		"/v1/user/detail": `{"user":{"id":7,"name":"画师七","account":"painter7","comment":"画风景"},` +
			`"profile":{"total_illusts":1}}`,
		"/v1/user/illusts": `{"illusts":[{"id":11,"title":"作品十一",` +
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
	paths  []string
}

func (tr *pageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	tr.paths = append(tr.paths, req.URL.Path)
	body := ""
	for prefix, b := range tr.bodies {
		if strings.HasPrefix(req.URL.Path, prefix) {
			body = b
			break
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

// newPageApp is an app whose API answers from pageBodies, without images or a
// network.
func newPageApp() *App {
	a := New()
	a.client.HTTP().Transport = &pageTransport{bodies: pageBodies()}
	a.client.SetCooldown(0)
	a.images = imagex.New(nil, "")
	return a
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
	for _, want := range []string{"画师七", "painter7", "画风景", "关注", "作品十一"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestUserPageFollow checks the follow button follows the user and shows the
// new state.
func TestUserPageFollow(t *testing.T) {
	a := newPageApp()
	calls := make(chan string, 2)
	a.followFn = func(_ context.Context, id int64, on bool) error {
		calls <- fmt.Sprintf("%d:%v", id, on)
		return nil
	}
	a.openUser(7)
	waitUser(t, a, func(v userPageView) bool { return !v.loading })
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("关注"); err != nil {
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
	if !tt.HasText("已关注") {
		t.Errorf("followed state missing: %q", tt.Texts())
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
