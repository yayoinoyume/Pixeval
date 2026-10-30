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

// appTab is one tab: the page it shows and the page its back control returns
// to. The pages themselves are the app's shared state, so a tab remembers
// where it was, not its own copy of the data.
type appTab struct {
	route     route
	backRoute route
}

// homeTabs is the one Home tab a window starts with, and the one an account
// switch leaves.
func homeTabs() []appTab {
	return []appTab{{route: routeHome}}
}

// syncTab records the page showing on the active tab, so switching away and
// back restores it.
func (a *App) syncTab() {
	if a.tabIndex < 0 || a.tabIndex >= len(a.tabs) {
		return
	}
	a.tabs[a.tabIndex] = appTab{route: a.route, backRoute: a.backRoute}
}

// newTab opens Home in a tab after the active one and shows it.
func (a *App) newTab() {
	a.syncTab()
	a.tabs = append(a.tabs, appTab{route: routeHome})
	a.tabIndex = len(a.tabs) - 1
	a.showTab()
}

// showTab makes the active tab's page the one showing.
func (a *App) showTab() {
	tab := a.tabs[a.tabIndex]
	a.closeViewer()
	navMenuState.open = ""
	a.route, a.backRoute = tab.route, tab.backRoute
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
		a.tabs[0] = appTab{route: routeHome}
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
	title := a.pageTitle(tab.route)
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
