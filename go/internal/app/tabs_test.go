package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// These tests cover the tab strip: the tabs a window starts with, opening,
// switching and closing them, their keys, and how they lay out and read.

// TestTabsStartWithOneHomeTab checks a fresh window has one tab showing Home.
func TestTabsStartWithOneHomeTab(t *testing.T) {
	a := newHomeApp(t)
	if len(a.tabs) != 1 || a.tabIndex != 0 {
		t.Fatalf("tabs %v at %d, want one at 0", a.tabs, a.tabIndex)
	}
	if a.tabs[0].route != routeHome {
		t.Errorf("tab route %q, want %q", a.tabs[0].route, routeHome)
	}
}

// TestTabsNewAndSwitch checks the new-tab shortcut opens Home after the
// active tab, and that choosing a tab brings back the page and the page its
// back control returns to.
func TestTabsNewAndSwitch(t *testing.T) {
	a := newHomeApp(t)
	a.backRoute = routeSearch
	a.goRoute(routeUser)
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyT)
	if len(a.tabs) != 2 || a.tabIndex != 1 {
		t.Fatalf("tabs %v at %d after new tab, want two at 1", a.tabs, a.tabIndex)
	}
	if a.route != routeHome || a.backRoute != "" {
		t.Errorf("new tab shows %q back %q, want %q back empty", a.route, a.backRoute, routeHome)
	}
	tt.Frame()
	if _, ok := tt.Find(a.tf("tabs.label", a.pageTitle(routeHome))); !ok {
		t.Fatalf("the new tab's label is missing: %q", tt.Texts())
	}

	if err := tt.Click(a.tf("tabs.label", a.pageTitle(routeUser))); err != nil {
		t.Fatalf("Click the user tab: %v", err)
	}
	if a.tabIndex != 0 || a.route != routeUser || a.backRoute != routeSearch {
		t.Errorf("tab %d shows %q back %q, want 0 %q back %q",
			a.tabIndex, a.route, a.backRoute, routeUser, routeSearch)
	}
	if a.tabs[1].route != routeHome {
		t.Errorf("the other tab shows %q, want %q", a.tabs[1].route, routeHome)
	}
}

// TestTabsKeepTheirOwnUserPages checks two tabs can hold different user
// pages: switching restores each user's own page rather than the other's.
func TestTabsKeepTheirOwnUserPages(t *testing.T) {
	a := newHomeApp(t)
	a.openUser(7)
	first := a.userView
	a.newTab()
	if a.userView != nil {
		t.Fatalf("the new tab kept the user page of the tab before: %+v", a.userView)
	}
	a.openUser(9)
	second := a.userView
	if second == nil {
		t.Fatal("the second user page did not open")
	}
	if second == first {
		t.Fatal("the second user page reused the first one's state")
	}

	a.switchTab(0)
	if a.route != routeUser || a.userView != first {
		t.Errorf("tab 0 shows %q page %p, want %q page %p", a.route, a.userView, routeUser, first)
	}
	if a.userView == nil || a.userView.userID != 7 {
		t.Errorf("tab 0 shows user %v, want user 7", a.userView)
	}
	if got := a.tabs[1].state.userView; got != second {
		t.Errorf("tab 1 saved user page %p, want %p", got, second)
	}

	a.switchTab(1)
	if a.userView != second {
		t.Errorf("tab 1 shows user page %p, want %p", a.userView, second)
	}
	if a.userView == nil || a.userView.userID != 9 {
		t.Errorf("tab 1 shows user %v, want user 9", a.userView)
	}
}

// TestTabsKeepTheirOwnFeedsAndTitles checks two tabs can hold different full
// feeds, and each tab keeps the title its feed opened with rather than the
// one the other tab shows.
func TestTabsKeepTheirOwnFeedsAndTitles(t *testing.T) {
	const width, height = 1024, 720
	a := newHomeApp(t)
	firstCard, secondCard := a.homeCards[0], a.homeCards[1]
	a.openHomeFeed(firstCard)
	firstFeed, firstTitle := a.feed, a.homeCardTitle(firstCard)
	a.newTab()
	a.openHomeFeed(secondCard)
	secondFeed, secondTitle := a.feed, a.homeCardTitle(secondCard)
	if firstFeed == secondFeed {
		t.Fatal("the second feed reused the first one's state")
	}
	if firstTitle == secondTitle {
		t.Fatalf("both cards are titled %q, the test needs two names", firstTitle)
	}

	a.switchTab(0)
	if a.route != routeFeed || a.feed != firstFeed {
		t.Errorf("tab 0 shows %q feed %p, want %q feed %p", a.route, a.feed, routeFeed, firstFeed)
	}
	if a.tabs[0].title != firstTitle || a.tabs[1].title != secondTitle {
		t.Errorf("tab titles %q and %q, want %q and %q",
			a.tabs[0].title, a.tabs[1].title, firstTitle, secondTitle)
	}

	tt := ui.NewTester(a.View, width, height)
	tt.Frame()
	for _, title := range []string{firstTitle, secondTitle} {
		r, ok := tt.Find(a.tf("tabs.label", title))
		if !ok {
			t.Errorf("the tab titled %q is missing while the other feed shows: %q", title, tt.Texts())
			continue
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("the tab titled %q at %v overflows the %d wide window", title, r, width)
		}
	}

	a.switchTab(1)
	if a.feed != secondFeed {
		t.Errorf("tab 1 shows feed %p, want %p", a.feed, secondFeed)
	}
}

// TestTabsNewTabStartsWithEmptyPages checks a new tab brings its own empty
// pages, so the pages whose state the window normally creates still draw
// instead of sharing the tab before them.
func TestTabsNewTabStartsWithEmptyPages(t *testing.T) {
	a := newHomeApp(t)
	old := a.pageState()
	a.newTab()
	st := a.pageState()
	if st.spotlight == nil || st.newFeed == nil || st.myPixivFeed == nil ||
		st.bookmarks == nil || st.search == nil || st.searchNovels == nil ||
		st.searchUsers == nil || st.recommendedUsers == nil ||
		st.myFollowing == nil || st.myFollowers == nil || st.myPixivUsersList == nil {
		t.Fatal("the new tab is missing page state")
	}
	if st.spotlight == old.spotlight || st.newFeed == old.newFeed ||
		st.myPixivFeed == old.myPixivFeed || st.bookmarks == old.bookmarks ||
		st.search == old.search || st.recommendedUsers == old.recommendedUsers {
		t.Error("the new tab shares a page with the tab before")
	}

	tt := ui.NewTester(a.View, 1024, 720)
	for _, r := range []route{routeNew, routeMyPixiv, routeBookmarks, routeSearch,
		routeRecommendedUsers, routeMyFollowing, routeMyFollowers, routeMyPixivUsers,
		routeSpotlight, routeSeriesList} {
		a.goRoute(r)
		tt.Frame()
	}
}

// TestTabsCloseLastDropsItsPage checks closing the last tab returns Home and
// drops the page it held, title included, rather than keeping a stale page.
func TestTabsCloseLastDropsItsPage(t *testing.T) {
	a := newHomeApp(t)
	a.openUser(7)
	a.syncTab()
	a.closeTab(0)
	if len(a.tabs) != 1 || a.tabIndex != 0 {
		t.Fatalf("tabs %v at %d after closing the last, want one at 0", a.tabs, a.tabIndex)
	}
	if a.route != routeHome || a.userView != nil {
		t.Errorf("route %q user page %p after closing the last, want %q and none",
			a.route, a.userView, routeHome)
	}
	if want := a.pageTitle(routeHome); a.tabs[0].title != want {
		t.Errorf("the remaining tab is titled %q, want %q", a.tabs[0].title, want)
	}
}

// TestTabsFollowBack checks going back records the page it returns to on the
// tab showing.
func TestTabsFollowBack(t *testing.T) {
	a := newHomeApp(t)
	a.backRoute = routeSearch
	a.goRoute(routeUser)
	a.goBack()
	if a.route != routeSearch {
		t.Fatalf("route %q after back, want %q", a.route, routeSearch)
	}
	if a.tabs[0].route != routeSearch || a.tabs[0].backRoute != a.backRoute {
		t.Errorf("tab %+v after back, want route %q and back %q",
			a.tabs[0], routeSearch, a.backRoute)
	}
}

// TestTabsCloseActiveShowsLeftNeighbour checks closing the tab showing brings
// back the one to its left.
func TestTabsCloseActiveShowsLeftNeighbour(t *testing.T) {
	a := newHomeApp(t)
	a.goRoute(routeSettings)
	a.newTab()
	a.goRoute(routeHistory)
	a.newTab()
	a.goRoute(routeDownloads)
	if len(a.tabs) != 3 || a.tabIndex != 2 {
		t.Fatalf("tabs %v at %d, want three at 2", a.tabs, a.tabIndex)
	}

	a.closeTab(a.tabIndex)
	if len(a.tabs) != 2 || a.tabIndex != 1 {
		t.Fatalf("tabs %v at %d after close, want two at 1", a.tabs, a.tabIndex)
	}
	if a.route != routeHistory {
		t.Errorf("route %q after close, want %q", a.route, routeHistory)
	}
	if a.tabs[0].route != routeSettings || a.tabs[1].route != routeHistory {
		t.Errorf("tabs %v, want settings and history", a.tabs)
	}
}

// TestTabsCloseInactiveKeepsTheActiveOne checks closing another tab leaves the
// page showing alone.
func TestTabsCloseInactiveKeepsTheActiveOne(t *testing.T) {
	a := newHomeApp(t)
	a.goRoute(routeSettings)
	a.newTab()
	a.goRoute(routeHistory)
	a.newTab()
	a.goRoute(routeDownloads)

	a.closeTab(0)
	if len(a.tabs) != 2 || a.tabIndex != 1 || a.route != routeDownloads {
		t.Errorf("tabs %v at %d showing %q, want two at 1 showing %q",
			a.tabs, a.tabIndex, a.route, routeDownloads)
	}
}

// TestTabsCloseButtonClosesInFrame checks a real click on a tab's close
// control closes it in that frame: the strip shortens the tab list while it
// still draws, so it must not index the list by the length it started with.
func TestTabsCloseButtonClosesInFrame(t *testing.T) {
	cases := []struct {
		name      string
		routes    []route
		close     int
		wantTabs  []route
		wantIndex int
		wantRoute route
	}{
		{
			name:      "first",
			routes:    []route{routeSettings, routeHistory, routeDownloads},
			close:     0,
			wantTabs:  []route{routeHistory, routeDownloads},
			wantIndex: 1,
			wantRoute: routeDownloads,
		},
		{
			name:      "middle",
			routes:    []route{routeSettings, routeHistory, routeDownloads},
			close:     1,
			wantTabs:  []route{routeSettings, routeDownloads},
			wantIndex: 1,
			wantRoute: routeDownloads,
		},
		{
			name:      "last",
			routes:    []route{routeSettings, routeHistory},
			close:     1,
			wantTabs:  []route{routeSettings},
			wantIndex: 0,
			wantRoute: routeSettings,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newHomeApp(t)
			a.goRoute(tc.routes[0])
			for _, r := range tc.routes[1:] {
				a.newTab()
				a.goRoute(r)
			}
			tt := ui.NewTester(a.View, 1024, 720)
			tt.Frame()

			closed := a.tf("tabs.close", a.pageTitle(tc.routes[tc.close]))
			if err := tt.Click(closed); err != nil {
				t.Fatalf("Click the %s tab's close control: %v", tc.name, err)
			}
			tt.Frame()

			if len(a.tabs) != len(tc.wantTabs) || a.tabIndex != tc.wantIndex {
				t.Fatalf("tabs %v at %d after closing the %s, want %v at %d",
					a.tabs, a.tabIndex, tc.name, tc.wantTabs, tc.wantIndex)
			}
			for i, want := range tc.wantTabs {
				if a.tabs[i].route != want {
					t.Errorf("tab %d shows %q, want %q", i, a.tabs[i].route, want)
				}
			}
			if a.route != tc.wantRoute {
				t.Errorf("route %q after closing the %s, want %q", a.route, tc.name, tc.wantRoute)
			}
			if _, ok := tt.Find(closed); ok {
				t.Errorf("the closed tab's close control %q is still drawn", closed)
			}
		})
	}
}

// TestTabsLastTabReturnsHome checks the last tab is not closed: it goes back
// to Home instead.
func TestTabsLastTabReturnsHome(t *testing.T) {
	a := newHomeApp(t)
	a.goRoute(routeSettings)

	a.closeTab(0)
	if len(a.tabs) != 1 || a.tabIndex != 0 {
		t.Fatalf("tabs %v at %d after closing the last, want one at 0", a.tabs, a.tabIndex)
	}
	if a.route != routeHome || a.tabs[0].route != routeHome {
		t.Errorf("route %q tab %q, want %q", a.route, a.tabs[0].route, routeHome)
	}
}

// TestTabsShortcuts checks the keys open, cycle and close tabs.
func TestTabsShortcuts(t *testing.T) {
	a := newHomeApp(t)
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	tt.Key(ui.Cmd, ui.KeyT)
	if len(a.tabs) != 2 || a.tabIndex != 1 {
		t.Fatalf("tabs %v at %d after the new-tab key, want two at 1", a.tabs, a.tabIndex)
	}
	tt.Key(ui.Cmd, ui.KeyTab)
	if a.tabIndex != 0 {
		t.Errorf("tab %d after the next-tab key, want 0", a.tabIndex)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyTab)
	if a.tabIndex != 1 {
		t.Errorf("tab %d after the previous-tab key, want 1", a.tabIndex)
	}
	tt.Key(ui.Cmd, ui.KeyW)
	if len(a.tabs) != 1 || a.tabIndex != 0 || a.route != routeHome {
		t.Errorf("tabs %v at %d showing %q after the close-tab key, want one home tab",
			a.tabs, a.tabIndex, a.route)
	}
}

// TestTabsAccountSwitchResetsToOneHomeTab checks a switched account never
// keeps a tab that showed the account before it.
func TestTabsAccountSwitchResetsToOneHomeTab(t *testing.T) {
	a := newHomeApp(t)
	a.goRoute(routeSettings)
	a.newTab()
	a.goRoute(routeHistory)

	a.applySession(&pixiv.Token{User: pixiv.TokenUser{ID: 2, Name: "另一个用户"}})
	if len(a.tabs) != 1 || a.tabIndex != 0 {
		t.Fatalf("tabs %v at %d after the switch, want one at 0", a.tabs, a.tabIndex)
	}
	if a.route != routeHome || a.backRoute != "" {
		t.Errorf("route %q back %q after the switch, want %q back empty",
			a.route, a.backRoute, routeHome)
	}
	if a.tabs[0].route != routeHome {
		t.Errorf("tab route %q after the switch, want %q", a.tabs[0].route, routeHome)
	}
}

// TestTabsFitNarrowWindow checks the strip stays inside the 1024x720 window
// however many tabs are open, and that the tabs themselves scroll sideways
// rather than pushing the new-tab button out.
func TestTabsFitNarrowWindow(t *testing.T) {
	const width, height = 1024, 720
	a := newHomeApp(t)
	for i := 0; i < 12; i++ {
		a.newTab()
	}
	tt := ui.NewTester(a.View, width, height)
	tt.Frame()

	newTab := a.t("tabs.new")
	r, ok := tt.Find(newTab)
	if !ok || r.W == 0 {
		t.Fatalf("the new-tab button is not laid out: %q", tt.Texts())
	}
	if r.X < 0 || r.X+r.W > width {
		t.Errorf("the new-tab button at %v overflows the %d wide window", r, width)
	}

	firstLabel := a.tf("tabs.label", a.pageTitle(routeHome))
	first, ok := tt.Find(firstLabel)
	if !ok || first.W == 0 {
		t.Fatalf("the first tab is not laid out: %q", tt.Texts())
	}
	if first.X < navWidth || first.X+first.W > width {
		t.Errorf("the first tab at %v overflows the %d wide window", first, width)
	}
	if first.Y < topBarHeight-tabBarHeight || first.Y+first.H > topBarHeight {
		t.Errorf("the first tab at %v is outside the strip above the page", first)
	}

	before := first.X
	tt.Scroll(width/2, first.Y+first.H/2, 400, 0)
	tt.Frame()
	after, ok := tt.Find(firstLabel)
	if !ok {
		t.Fatalf("the first tab is missing after scrolling: %q", tt.Texts())
	}
	if after.X >= before {
		t.Errorf("the strip did not scroll: the first tab moved from %v to %v", before, after.X)
	}
}

// TestTabsA11yLabels checks each tab and its close control are named for
// assistive technology without the middle dot, and that the English names
// read as the design says.
func TestTabsA11yLabels(t *testing.T) {
	a := newHomeApp(t)
	a.goRoute(routeSettings)
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	title := a.pageTitle(routeSettings)
	for _, label := range []string{
		a.tf("tabs.label", title),
		a.tf("tabs.close", title),
		a.t("tabs.new"),
	} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("label %q is missing: %q", label, tt.Texts())
		}
		if strings.Contains(label, "·") {
			t.Errorf("label %q carries the middle dot", label)
		}
	}
	if r, ok := tt.Find(a.tf("tabs.close", title)); !ok || r.W != 22 || r.H != 22 {
		t.Errorf("the close control is %v, want 22 by 22", r)
	}

	en, err := i18n.Load(i18n.LangEnUS)
	if err != nil {
		t.Fatalf("Load en-US: %v", err)
	}
	if got := en.Tf("tabs.label", "Home"); got != "Tab: Home" {
		t.Errorf("en tabs.label reads %q, want %q", got, "Tab: Home")
	}
	if got := en.Tf("tabs.close", "Home"); got != "Close tab: Home" {
		t.Errorf("en tabs.close reads %q, want %q", got, "Close tab: Home")
	}
	if got := en.T("tabs.new"); got != "New tab" {
		t.Errorf("en tabs.new reads %q, want %q", got, "New tab")
	}
}

// TestTabsSpeakEveryLanguage checks every shipped language names the tabs and
// reads the tab label as its own words, not English fallback.
func TestTabsSpeakEveryLanguage(t *testing.T) {
	en, err := i18n.Load(i18n.LangEnUS)
	if err != nil {
		t.Fatalf("Load en-US: %v", err)
	}
	for _, lang := range []string{i18n.LangZhHans, i18n.LangFrFR, i18n.LangRuRU} {
		b, err := i18n.Load(lang)
		if err != nil {
			t.Fatalf("Load %s: %v", lang, err)
		}
		for _, key := range []string{"tabs.new", "tabs.label", "tabs.close"} {
			if got := b.T(key); got == en.T(key) {
				t.Errorf("%s %s reads %q, the English text", lang, key, got)
			}
		}
		if got := b.Tf("tabs.label", "Pix"); !strings.Contains(got, "Pix") {
			t.Errorf("%s tabs.label reads %q, want it to carry the title", lang, got)
		}
	}
}
