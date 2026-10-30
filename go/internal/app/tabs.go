package app

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// tabBarHeight is the strip of tabs under the header.
const tabBarHeight = 38

// topBarHeight is the header row and tab strip together at the default text
// scale, so the page starts where it did before the strip existed.
const topBarHeight = 84

// tabMaxWidth is the widest a tab grows before its title truncates.
const tabMaxWidth = 180

// appTab is one tab: the page it shows, the page its back control returns
// to, its own title and a snapshot of the pages' data, so two tabs can show
// different users, series or feeds without overwriting each other.
type appTab struct {
	route     route
	backRoute route
	title     string
	state     tabPageState
}

// tabPageState is the data the pages show, as one tab left it. Each tab holds
// its own, so switching restores that tab's user page or feed rather than the
// one another tab opened last.
type tabPageState struct {
	// feed is the full home feed a card's "view all" opened.
	feed *homeFeedView
	// userView, seriesView, related, novelView and spotlight are the detail
	// pages a tab opened.
	userView   *userViewState
	seriesView *seriesViewState
	related    *relatedViewState
	novelView  *novelViewState
	spotlight  *spotlightState
	// newFeed and myPixivFeed are the works feeds, each with the kind it
	// shows.
	newFeed     *browseState
	myPixivFeed *browseState
	newKind     int
	myPixivKind int
	// bookmarks is the bookmarks feed and bookmarksTab its public or private
	// tab.
	bookmarks    *browseState
	bookmarksTab int
	// relatedUsers and myPixivUsers are a user's related and MyPixiv lists.
	relatedUsers *relatedUsersViewState
	myPixivUsers *myPixivUsersViewState
	// recommendedUsers, myFollowing, myFollowers and myPixivUsersList are the
	// people lists that stand alone in the rail.
	recommendedUsers *userListState
	myFollowing      *userListState
	myFollowers      *userListState
	myPixivUsersList *userListState
	// search, searchNovels and searchUsers are the three searches, and
	// searchKind the one the page shows. The filters below are the
	// illustration search's controls, so each tab keeps its own filters beside
	// its own results.
	search          *browseState
	searchNovels    *novelFeedState
	searchUsers     *userListState
	searchKind      int
	searchSort      string
	searchTarget    string
	searchStartDate string
	searchEndDate   string
	searchMinWidth  string
	searchMaxWidth  string
	searchMinHeight string
	searchMaxHeight string
	searchErr       string
	// seriesFeeds are the series watchlist's lists, one per kind, and
	// seriesTab the kind shown.
	seriesFeeds [seriesTabCount]*seriesListState
	seriesTab   int
}

// pageState reads the data the pages show.
func (a *App) pageState() tabPageState {
	return tabPageState{
		feed:             a.feed,
		userView:         a.userView,
		seriesView:       a.seriesView,
		related:          a.related,
		novelView:        a.novelView,
		spotlight:        a.spotlight,
		newFeed:          a.newFeed,
		myPixivFeed:      a.myPixivFeed,
		newKind:          a.newKind,
		myPixivKind:      a.myPixivKind,
		bookmarks:        a.bookmarks,
		bookmarksTab:     a.bookmarksTab,
		relatedUsers:     a.relatedUsers,
		myPixivUsers:     a.myPixivUsers,
		recommendedUsers: a.recommendedUsers,
		myFollowing:      a.myFollowing,
		myFollowers:      a.myFollowers,
		myPixivUsersList: a.myPixivUsersList,
		search:           a.search,
		searchNovels:     a.searchNovels,
		searchUsers:      a.searchUsers,
		searchKind:       a.searchKind,
		searchSort:       a.searchSort,
		searchTarget:     a.searchTarget,
		searchStartDate:  a.searchStartDate,
		searchEndDate:    a.searchEndDate,
		searchMinWidth:   a.searchMinWidth,
		searchMaxWidth:   a.searchMaxWidth,
		searchMinHeight:  a.searchMinHeight,
		searchMaxHeight:  a.searchMaxHeight,
		searchErr:        a.searchErr,
		seriesFeeds:      a.seriesFeeds,
		seriesTab:        a.seriesTab,
	}
}

// restorePageState makes st the data the pages show.
func (a *App) restorePageState(st tabPageState) {
	a.feed = st.feed
	a.userView = st.userView
	a.seriesView = st.seriesView
	a.related = st.related
	a.novelView = st.novelView
	a.spotlight = st.spotlight
	a.newFeed = st.newFeed
	a.myPixivFeed = st.myPixivFeed
	a.newKind = st.newKind
	a.myPixivKind = st.myPixivKind
	a.bookmarks = st.bookmarks
	a.bookmarksTab = st.bookmarksTab
	a.relatedUsers = st.relatedUsers
	a.myPixivUsers = st.myPixivUsers
	a.recommendedUsers = st.recommendedUsers
	a.myFollowing = st.myFollowing
	a.myFollowers = st.myFollowers
	a.myPixivUsersList = st.myPixivUsersList
	a.search = st.search
	a.searchNovels = st.searchNovels
	a.searchUsers = st.searchUsers
	a.searchKind = st.searchKind
	a.searchSort = st.searchSort
	a.searchTarget = st.searchTarget
	a.searchStartDate = st.searchStartDate
	a.searchEndDate = st.searchEndDate
	a.searchMinWidth = st.searchMinWidth
	a.searchMaxWidth = st.searchMaxWidth
	a.searchMinHeight = st.searchMinHeight
	a.searchMaxHeight = st.searchMaxHeight
	a.searchErr = st.searchErr
	a.seriesFeeds = st.seriesFeeds
	a.seriesTab = st.seriesTab
}

// newTabState is the data a new tab starts with: empty pages, so a page
// visited for the first time on that tab has state to fill.
func newTabState() tabPageState {
	return tabPageState{
		spotlight:        &spotlightState{},
		newFeed:          &browseState{},
		myPixivFeed:      &browseState{},
		bookmarks:        &browseState{},
		search:           &browseState{},
		searchNovels:     &novelFeedState{},
		searchUsers:      &userListState{},
		recommendedUsers: &userListState{},
		myFollowing:      &userListState{},
		myFollowers:      &userListState{},
		myPixivUsersList: &userListState{},
		searchSort:       searchSortDateDesc,
		searchTarget:     searchTargetPartial,
	}
}

// feeds are the works feeds the tab holds, so a block drops its works from
// every tab's lists, not only the showing one.
func (st tabPageState) feeds() []*browseState {
	feeds := []*browseState{st.newFeed, st.myPixivFeed, st.bookmarks, st.search}
	if st.feed != nil {
		feeds = append(feeds, st.feed.st)
	}
	if st.userView != nil {
		feeds = append(feeds, st.userView.feed, st.userView.mangaFeed,
			st.userView.bookmarksFeed, st.userView.bookmarksPrivateFeed)
	}
	if st.seriesView != nil {
		feeds = append(feeds, st.seriesView.feed)
	}
	if st.related != nil {
		feeds = append(feeds, st.related.feed)
	}
	return feeds
}

// tabFeeds are the works feeds the open tabs hold, the showing tab's live
// pages included, so a block prunes what is not showing too.
func (a *App) tabFeeds() []*browseState {
	feeds := a.pageState().feeds()
	for i := range a.tabs {
		feeds = append(feeds, a.tabs[i].state.feeds()...)
	}
	return feeds
}

// homeTabs is the one Home tab a window starts with, and the one an account
// switch leaves.
func (a *App) homeTabs() []appTab {
	return []appTab{{route: routeHome, title: a.pageTitle(routeHome), state: newTabState()}}
}

// syncTab records the page, back page, title and page data showing on the
// active tab, so switching away and back restores them.
func (a *App) syncTab() {
	if a.tabIndex < 0 || a.tabIndex >= len(a.tabs) {
		return
	}
	st := a.pageState()
	a.tabs[a.tabIndex] = appTab{
		route:     a.route,
		backRoute: a.backRoute,
		title:     a.tabTitle(a.route, st),
		state:     st,
	}
}

// tabTitle names a tab: the user, series or novel its page loaded, or the
// page's own name until one does.
func (a *App) tabTitle(r route, st tabPageState) string {
	switch r {
	case routeUser:
		if st.userView != nil {
			if name := st.userView.snapshot().user.Name; name != "" {
				return name
			}
		}
	case routeSeries:
		if st.seriesView != nil {
			if title := st.seriesView.snapshot().detail.Title; title != "" {
				return title
			}
		}
	case routeNovel:
		if st.novelView != nil && st.novelView.title != "" {
			return st.novelView.title
		}
	}
	return a.pageTitle(r)
}

// retitleTabs recomputes every tab's title in the current language, the ones
// not showing included. Each tab's page state is restored while its title is
// worked out, because a feed, user, series or novel tab names itself from the
// page it holds.
func (a *App) retitleTabs() {
	if len(a.tabs) == 0 {
		return
	}
	a.syncTab()
	showing := a.pageState()
	for i := range a.tabs {
		a.restorePageState(a.tabs[i].state)
		a.tabs[i].title = a.tabTitle(a.tabs[i].route, a.tabs[i].state)
	}
	a.restorePageState(showing)
}

// newTab opens Home in a tab after the active one and shows it.
func (a *App) newTab() {
	a.syncTab()
	a.tabs = append(a.tabs, appTab{route: routeHome, title: a.pageTitle(routeHome), state: newTabState()})
	a.tabIndex = len(a.tabs) - 1
	a.showTab()
}

// showTab makes the active tab's page, title and data the ones showing.
func (a *App) showTab() {
	tab := a.tabs[a.tabIndex]
	a.closeViewer()
	navMenuState.open = ""
	a.route, a.backRoute = tab.route, tab.backRoute
	a.restorePageState(tab.state)
}

// switchTab shows tab i.
func (a *App) switchTab(i int) {
	if i < 0 || i >= len(a.tabs) || i == a.tabIndex {
		return
	}
	a.syncTab()
	a.tabIndex = i
	a.showTab()
}

// cycleTab shows the tab dir places after the active one, wrapping at either
// end: 1 for the next tab, -1 for the previous one.
func (a *App) cycleTab(dir int) {
	if len(a.tabs) < 2 {
		return
	}
	a.switchTab((a.tabIndex + dir + len(a.tabs)) % len(a.tabs))
}

// closeTab closes tab i. Closing the active tab shows the one to its left;
// the last tab is not closed but returns to Home.
func (a *App) closeTab(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	if len(a.tabs) == 1 {
		a.tabs[0] = appTab{route: routeHome, title: a.pageTitle(routeHome), state: newTabState()}
		a.tabIndex = 0
		a.showTab()
		return
	}
	a.syncTab()
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if i < a.tabIndex {
		a.tabIndex--
	} else if i == a.tabIndex {
		a.tabIndex = max(i-1, 0)
	}
	a.showTab()
}

// tabBar is the strip of open tabs under the header: the tabs scroll
// sideways, and the button at the strip's right opens a new one.
func (a *App) tabBar(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Height(tabBarHeight).Gap(t.SpaceS).Padding(0, contentPadding).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).FillHeight().Gap(t.SpaceXS).
			AlignItems(ui.Center).Children(func() {
			// A close control shortens a.tabs while this frame still draws
			// the strip, so the loop walks a snapshot and tabItem never
			// indexes the live list.
			tabs := slices.Clone(a.tabs)
			for i := range tabs {
				a.tabItem(c, t, tabs[i], i)
			}
		})
		if iconButton(c, iconPlus, a.t("tabs.new"), 28, t.TextMuted, t.SurfaceAlt).Clicked() {
			a.newTab()
		}
	})
}

// tabItem draws one tab: its page's title, its own close control, and an
// accent rule under it while it is the one showing.
func (a *App) tabItem(c *ui.Context, t tokens, tab appTab, i int) {
	title := tab.title
	active := i == a.tabIndex
	bg := ui.Transparent
	if active {
		bg = t.Surface
	}
	ui.Column(c).Height(32).MaxWidth(tabMaxWidth).Shrink(0).Radius(t.RadiusS).
		Background(bg).Clip().Children(func() {
		ui.Row(c).Grow(1).Padding(0, t.SpaceXS).Gap(t.SpaceXS).
			AlignItems(ui.Center).Children(func() {
			label := a.tf("tabs.label", title)
			b := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(28).
				Radius(t.RadiusXS).Padding(0, t.SpaceXS).Label(label).Tooltip(title)
			if !active && b.Hovered() {
				b = b.Background(t.SurfaceAlt)
			}
			fg := t.TextMuted
			if active {
				fg = t.Text
			}
			b.Children(func() {
				ui.Text(c, title).FontSize(t.TextSmall).FontWeight(500).
					TextColor(fg).MaxLines(1)
			})
			if b.Clicked() {
				a.switchTab(i)
			}
			if iconButton(c, iconClose, a.tf("tabs.close", title), 22, t.TextMuted, t.SurfaceAlt).Clicked() {
				a.closeTab(i)
			}
		})
		bar := ui.Transparent
		if active {
			bar = t.Accent
		}
		ui.Box(c).FillWidth().Height(2).Background(bar)
	})
}
