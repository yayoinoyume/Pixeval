package app

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/filter"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/saucenao"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// route is the page the shell shows.
type route string

const (
	routeHome             route = "home"
	routeNew              route = "new"
	routeMyPixiv          route = "mypixiv"
	routeRecommendedUsers route = "recommendedUsers"
	routeSearch           route = "search"
	routeSpotlight        route = "spotlight"
	routeSubs             route = "subscriptions"
	routeDownloads        route = "downloads"
	routeHistory          route = "history"
	routeBookmarks        route = "bookmarks"
	routeWatchLater       route = "watchLater"
	routeSettings         route = "settings"
	routeUser             route = "user"
	routeUserRelated      route = "userRelated"
	routeUserMyPixiv      route = "userMyPixiv"
	routeSeries           route = "series"
	routeSeriesList       route = "seriesList"
	routeNovel            route = "novel"
	routeRelated          route = "related"
	routeHelp             route = "help"
	routeAbout            route = "about"
)

// routeLabel is the navigation label of the page.
func (a *App) routeLabel(r route) string {
	switch r {
	case routeRelated:
		return a.t("related.title")
	case routeSeriesList:
		return a.t("nav.series")
	case routeUserRelated:
		return a.t("user.relatedUsers")
	case routeUserMyPixiv:
		return a.t("user.myPixiv")
	case routeRecommendedUsers:
		return a.t("recommendedUsers.title")
	case routeHelp:
		return a.t("help.title")
	case routeAbout:
		return a.t("about.title")
	}
	return a.t("nav." + string(r))
}

// pageTitle is the page header's name. It is the navigation label except for
// the series watchlist, whose header names the list rather than the group.
func (a *App) pageTitle(r route) string {
	if r == routeSeriesList {
		return a.t("series.watchlistTitle")
	}
	return a.routeLabel(r)
}

// navGroup is one rail group: the pages its menu lists, the glyph on the
// group's button and the key its name is read from. The rail shows few
// destinations; the groups hold the rest one click away.
type navGroup struct {
	id     string
	glyph  *ui.SVG
	routes []route
}

// navGroups are the rail's groups, in the order they show under the pages
// that stand alone.
var navGroups = []navGroup{
	{id: "works", glyph: iconImage, routes: []route{routeNew, routeSeriesList, routeMyPixiv, routeSpotlight, routeSubs}},
	{id: "users", glyph: iconUser, routes: []route{routeRecommendedUsers}},
	{id: "library", glyph: iconSeries, routes: []route{routeHistory, routeDownloads, routeWatchLater}},
}

// navMenuState is the rail's open group menu: a group id, "" when none is
// open. The app runs one window, so one menu is open at a time.
var navMenuState struct {
	open string
}

// View builds the shell: the navigation on the left, the page title and the
// sign-in state above the page. The viewer takes the whole window instead.
func (a *App) View(c *ui.Context) {
	a.applyTheme(c)
	a.applySavedSettings(c)
	a.showNotice(c)
	a.shortcuts(c)
	if a.viewer != nil {
		a.viewerPage(c)
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.nav(c)
		ui.Column(c).Grow(1).FillHeight().Children(func() {
			a.header(c)
			a.page(c)
		})
	})
}

// shortcuts registers the window's keys, which work on every page and in the
// viewer, whatever has the keyboard focus.
func (a *App) shortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyK, func() {
		a.goRoute(routeSearch)
		// The search box takes the focus on the frame after this one.
		a.search.focusQuery = true
	})
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.goRoute(routeSettings) })
	c.OnShortcut(ui.Cmd, ui.KeyH, func() { a.goRoute(routeHistory) })
	c.OnShortcut(ui.Cmd, ui.KeyD, func() { a.goRoute(routeDownloads) })
	c.OnShortcut(0, ui.KeyEscape, a.escape)
	if a.viewer != nil {
		c.OnShortcut(0, ui.KeyLeft, func() { a.viewerGo(-1) })
		c.OnShortcut(0, ui.KeyRight, func() { a.viewerGo(1) })
	}
}

// goRoute shows r, closing the viewer that covers the pages and the rail's
// open group menu.
func (a *App) goRoute(r route) {
	a.closeViewer()
	navMenuState.open = ""
	a.route = r
}

// escape closes the viewer, or leaves the user, series or novel page for the
// page it was opened from.
func (a *App) escape() {
	if a.viewer != nil {
		a.closeViewer()
		return
	}
	switch a.route {
	case routeUser, routeSeries, routeNovel, routeRelated, routeUserRelated,
		routeUserMyPixiv, routeHelp, routeAbout:
		a.goBack()
	}
}

// nav is the window's rail: the brand, the few destinations that stand
// alone, the groups, and the settings held to the bottom. Each destination is
// a glyph over its name, as the rail draws it; a group opens its menu beside
// the rail. The rail does not scroll: its destinations fit the design window,
// so Settings is always in reach.
func (a *App) nav(c *ui.Context) {
	t := themeOf(c)
	ui.Column(c).Width(navWidth).FillHeight().Padding(t.SpaceXL, t.SpaceS, t.SpaceL, t.SpaceS).
		Gap(t.SpaceXS).Background(t.Bg).Shrink(0).AlignItems(ui.Center).
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).
		Role(ui.RoleGroup).Label(a.t("nav.region")).Children(func() {
		a.brand(c)
		a.navItem(c, routeHome)
		a.navItem(c, routeSearch)
		a.navItem(c, routeBookmarks)
		for _, g := range navGroups {
			a.navGroupItem(c, g)
		}
		ui.Spacer(c)
		a.navItem(c, routeSettings)
	})
}

// brand is the app's mark and wordmark at the top of the rail.
func (a *App) brand(c *ui.Context) {
	t := themeOf(c)
	ui.Column(c).AlignItems(ui.Center).Gap(t.SpaceXS).Padding(0, 0, t.SpaceM, 0).Children(func() {
		ui.Box(c).Size(30, 30).Radius(t.RadiusXS).Background(t.Text).Center().
			Label("Pixeval Go").Tooltip("Pixeval Go " + appVersion()).Children(func() {
			ui.Icon(c, iconLogo).Size(18, 18).TextColor(t.Bg)
		})
		ui.Text(c, "Pixeval Go").FontSize(t.TextMicro).LetterSpacing(0.3).TextColor(t.TextMuted)
	})
}

// navItem is one destination: a glyph over its name, with a rule under it
// when the page is showing. The rail keeps the short name; the page header
// may name the page more fully.
func (a *App) navItem(c *ui.Context, r route) {
	t := themeOf(c)
	selected := a.route == r
	label := a.t("nav." + string(r))
	b := ui.ButtonBase(c).Key("nav."+string(r)).Width(80).Height(52).
		Radius(t.RadiusS).Padding(6, 0, t.SpaceXS, 0).Gap(1).
		Column().AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(label)
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	fg := t.TextMuted
	if selected {
		fg = t.Text
	} else if b.Hovered() {
		fg = t.Text
	}
	b.Children(func() {
		ui.Icon(c, navGlyph(r)).Size(18, 18).TextColor(fg)
		ui.Text(c, label).FontSize(t.TextMicro).FontWeight(500).TextColor(fg).MaxLines(1)
		bar := t.Accent
		if !selected {
			bar = ui.Transparent
		}
		ui.Box(c).Width(14).Height(2).Background(bar)
	})
	if b.Clicked() {
		a.goRoute(r)
	}
}

// navGroupItem is one group: its glyph over its name and a caret, with a rule
// under it while one of its pages is showing. Its button opens the group's
// menu beside the rail, and choosing a page there closes it.
func (a *App) navGroupItem(c *ui.Context, g navGroup) {
	t := themeOf(c)
	label := a.t("nav." + g.id)
	active := a.navGroupRoute(g)
	selected := active != ""
	tip := label
	if selected {
		tip += " · " + a.t("nav."+string(active))
	}
	b := ui.ButtonBase(c).Key("nav.group."+g.id).Width(80).Height(52).
		Radius(t.RadiusS).Padding(6, 0, t.SpaceXS, 0).Gap(1).
		Column().AlignItems(ui.Center).Justify(ui.Center).
		Label(label).Tooltip(tip)
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	fg := t.TextMuted
	if selected {
		fg = t.Text
	} else if b.Hovered() {
		fg = t.Text
	}
	b.Children(func() {
		ui.Icon(c, g.glyph).Size(18, 18).TextColor(fg)
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(t.TextMicro).FontWeight(500).TextColor(fg).MaxLines(1)
			ui.Icon(c, iconRight).Size(9, 9).TextColor(fg)
		})
		bar := t.Accent
		if !selected {
			bar = ui.Transparent
		}
		ui.Box(c).Width(14).Height(2).Background(bar)
	})
	if b.Clicked() {
		if navMenuState.open == g.id {
			navMenuState.open = ""
		} else {
			navMenuState.open = g.id
		}
	}
	open := navMenuState.open == g.id
	ui.PopoverBase(c, b, &open, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorEnd, ui.AnchorStart).MarginStart(t.SpaceS).
			Background(t.Surface).Border(1, t.Border).Radius(t.RadiusS).
			Padding(t.SpaceXS).MinWidth(190)
		panel.Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, r := range g.routes {
					a.navGroupMenuRow(c, t, r)
				}
			})
		})
	})
	// A press outside the menu closed it: drop the rail's record of it, so
	// the next frame does not open it again.
	if !open && navMenuState.open == g.id {
		navMenuState.open = ""
	}
}

// navGroupRoute is the group's page that is showing, or "" when the page is
// outside the group.
func (a *App) navGroupRoute(g navGroup) route {
	for _, r := range g.routes {
		if a.route == r {
			return r
		}
	}
	return ""
}

// navGroupMenuRow is one page in a group's menu. The page showing is marked
// with the accent rule and accent text; choosing a row shows that page and
// closes the menu.
func (a *App) navGroupMenuRow(c *ui.Context, t tokens, r route) {
	label := a.routeLabel(r)
	selected := a.route == r
	row := ui.ButtonBase(c).FillWidth().Height(34).Radius(t.RadiusS).
		Padding(0, t.SpaceS).Gap(t.SpaceS).Row().AlignItems(ui.Center).
		Label(label).OnClick(func() {
		a.goRoute(r)
	})
	if row.Hovered() {
		row = row.Background(t.SurfaceAlt)
	}
	fg := t.Text
	if selected {
		fg = t.Accent
	}
	row.Children(func() {
		bar := t.Accent
		if !selected {
			bar = ui.Transparent
		}
		ui.Box(c).Width(2).Height(16).Background(bar)
		ui.Text(c, label).FontSize(t.TextSmall).FontWeight(500).TextColor(fg).MaxLines(1)
	})
}

// navGlyph is the rail's glyph for a destination that stands alone:
// bookmarks has its own heart, the rest follow navIcon.
func navGlyph(r route) *ui.SVG {
	if r == routeBookmarks {
		return iconHeart
	}
	return navIcon(r)
}

// header is the Material app bar: the page title and the account state above
// the page.
func (a *App) header(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Height(84).Gap(t.SpaceM).Padding(0, contentPadding).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if a.route == routeUser || a.route == routeSeries || a.route == routeRelated ||
			a.route == routeUserRelated || a.route == routeUserMyPixiv ||
			a.route == routeHelp || a.route == routeAbout {
			if iconButton(c, iconLeft, a.t("common.back"), 32, t.TextMuted, t.SurfaceAlt).Clicked() {
				a.goBack()
			}
		}
		displayText(c, a.pageTitle(a.route)).FontSize(t.TextDisplay).FontWeight(500).
			LineHeight(1.12).TextColor(t.Text).Grow(1).MaxLines(1).MinWidth(0)
		if a.session == nil {
			if ui.PrimaryButton(c, a.t("nav.login")).Height(36).Radius(t.RadiusS).
				Padding(0, t.SpaceL).Clicked() {
				a.ShowLogin()
			}
			return
		}
		a.accountMenu(c)
	})
}

// accountMenuState is the header account menu's state: whether its panel is
// open and the avatars that failed to load. The app runs one window, so one
// menu is open at a time; a changed session closes it.
var accountMenuState struct {
	open   bool
	userID int64
	failed map[string]bool
}

// accountMenu is the signed-in account area: a quiet way out and the account
// pill, whose panel lists the saved accounts with their avatars, adds one,
// removes any but the one in use, and signs out.
func (a *App) accountMenu(c *ui.Context) {
	t := themeOf(c)
	name := a.userName()
	if accountMenuState.userID != a.accounts.ActiveID() {
		accountMenuState.open = false
		accountMenuState.userID = a.accounts.ActiveID()
	}
	ui.Row(c).Height(36).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
		// Sign out comes before the pill, so the pill stays the last stop the
		// keyboard reaches in the header.
		if iconButton(c, iconLogout, a.t("nav.logout"), 28, t.TextMuted, t.SurfaceAlt).Clicked() {
			a.logout()
		}
		b := ui.ButtonBase(c).Height(32).Radius(t.RadiusS).Padding(t.SpaceXS).
			Gap(t.SpaceS).Row().AlignItems(ui.Center).
			Label(a.tf("account.current", name)).Tooltip(a.t("account.switch"))
		b.Children(func() {
			ui.Box(c).Size(28, 28).Radius(t.RadiusFull).Background(t.AccentSoft).Center().Children(func() {
				ui.Text(c, initialOf(name)).FontSize(t.TextTiny).FontWeight(600).TextColor(t.Accent)
			})
			ui.Text(c, name).FontSize(t.TextSmall).FontWeight(500).TextColor(t.Text).MaxLines(1)
		})
		if b.Clicked() {
			accountMenuState.open = !accountMenuState.open
		}
		ui.Popover(c, b, &accountMenuState.open, func() {
			a.accountMenuPanel(c)
		})
	})
}

// accountMenuPanel is the account pill's panel: one row per saved account, a
// rule, then the way to add one and to sign out.
func (a *App) accountMenuPanel(c *ui.Context) {
	t := themeOf(c)
	active := a.accounts.ActiveID()
	ui.Column(c).MinWidth(250).Gap(t.SpaceXS).Children(func() {
		for _, acc := range a.accounts.Snapshot() {
			a.accountMenuRow(c, t, acc, acc.UserID == active)
		}
		ui.Box(c).Height(1).Margin(t.SpaceXS, 0).Background(t.Border)
		a.accountMenuAction(c, t, a.t("account.add"), func() {
			accountMenuState.open = false
			a.ShowLogin()
		})
		a.accountMenuAction(c, t, a.t("nav.logout"), func() {
			accountMenuState.open = false
			a.logout()
		})
	})
}

// accountMenuRow draws one saved account: its avatar, name and account handle,
// a check on the one in use, and a way to remove any other. The row switches
// to its account, or closes the panel when it already is that account.
func (a *App) accountMenuRow(c *ui.Context, t tokens, acc Account, active bool) {
	label := accountLabel(acc)
	ui.Row(c).FillWidth().Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
		row := ui.ButtonBase(c).Grow(1).Height(44).Radius(t.RadiusS).
			Padding(t.SpaceXS, t.SpaceS).Gap(t.SpaceS).Row().AlignItems(ui.Center).
			Label(label)
		if row.Hovered() {
			row = row.Background(t.SurfaceAlt)
		}
		if active {
			row.Label(a.tf("account.current", label)).OnClick(func() {
				accountMenuState.open = false
			})
		} else {
			row.OnClick(func() {
				accountMenuState.open = false
				a.switchAccount(acc.UserID)
			})
		}
		row.Children(func() {
			a.avatar(c, 28, acc.AvatarURL, label, accountMenuState.failed[acc.AvatarURL], func() {
				if accountMenuState.failed == nil {
					accountMenuState.failed = map[string]bool{}
				}
				accountMenuState.failed[acc.AvatarURL] = true
			})
			ui.Column(c).Grow(1).Gap(1).MinWidth(0).Children(func() {
				ui.Text(c, label).FontSize(t.TextSmall).FontWeight(600).TextColor(t.Text).MaxLines(1)
				if acc.Account != "" {
					ui.Text(c, "@"+acc.Account).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
				}
			})
			if active {
				ui.Text(c, "✓").FontSize(t.TextSmall).TextColor(t.Accent)
			}
		})
		if active {
			return
		}
		removeLabel := a.tf("account.delete", label)
		remove := ui.ButtonBase(c).Height(28).Radius(t.RadiusXS).Padding(0, t.SpaceS).
			Label(removeLabel).OnClick(func() { a.deleteAccount(acc.UserID) })
		fg := t.TextMuted
		if remove.Hovered() {
			fg = t.Danger
			remove = remove.Background(t.SurfaceAlt)
		}
		remove.Children(func() {
			ui.Text(c, removeLabel).FontSize(t.TextMicro).TextColor(fg)
		})
	})
}

// accountMenuAction draws one full-width action at the foot of the account
// panel, as Add account and Sign out are.
func (a *App) accountMenuAction(c *ui.Context, t tokens, label string, do func()) {
	b := ui.ButtonBase(c).FillWidth().Height(32).Radius(t.RadiusS).
		Padding(0, t.SpaceS).Gap(t.SpaceS).Row().AlignItems(ui.Center).
		Label(label).OnClick(do)
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(t.TextSmall).TextColor(t.Text)
	})
}

// accountLabel names a saved account in the menu: its display name, else its
// account id, else its user id.
func accountLabel(acc Account) string {
	if acc.Name != "" {
		return acc.Name
	}
	if acc.Account != "" {
		return acc.Account
	}
	return strconv.FormatInt(acc.UserID, 10)
}

func (a *App) page(c *ui.Context) {
	switch a.route {
	case routeHome:
		a.homePage(c)
	case routeNew:
		a.newPage(c)
	case routeMyPixiv:
		a.myPixivPage(c)
	case routeRecommendedUsers:
		a.recommendedUsersPage(c)
	case routeSearch:
		a.searchPage(c)
	case routeSpotlight:
		a.spotlightPage(c)
	case routeSubs:
		a.subscriptionsPage(c)
	case routeDownloads:
		a.downloadsPage(c)
	case routeHistory:
		a.historyPage(c)
	case routeWatchLater:
		a.watchLaterPage(c)
	case routeBookmarks:
		a.bookmarksPage(c)
	case routeSettings:
		a.settingsPage(c)
	case routeUser:
		a.userPage(c)
	case routeUserRelated:
		a.relatedUsersPage(c)
	case routeUserMyPixiv:
		a.myPixivUsersPage(c)
	case routeSeries:
		a.seriesPage(c)
	case routeSeriesList:
		a.seriesListPage(c)
	case routeNovel:
		a.novelPage(c)
	case routeRelated:
		a.relatedPage(c)
	case routeHelp:
		a.helpPage(c)
	case routeAbout:
		a.aboutPage(c)
	default:
		t := themeOf(c)
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.tf("nav.placeholder", a.routeLabel(a.route))).TextColor(t.TextMuted)
		})
	}
}

// goBack leaves a user, series or related page for the viewer or page it was
// opened from.
func (a *App) goBack() {
	if a.route == routeUserRelated && a.relatedUsers != nil {
		// The user page behind the list comes back whole, with its tabs and
		// place, and its own way back.
		r := a.relatedUsers
		a.userView, a.backRoute = r.user, r.backRoute
		a.relatedUsers, a.route = nil, routeUser
		return
	}
	if a.route == routeUserMyPixiv && a.myPixivUsers != nil {
		// The user page behind the list comes back whole, with its tabs and
		// place, and its own way back.
		m := a.myPixivUsers
		a.userView, a.backRoute = m.user, m.backRoute
		a.myPixivUsers, a.route = nil, routeUser
		return
	}
	if a.route == routeRelated && a.related != nil {
		r := a.related
		a.related = r.parent
		a.viewer, a.comments, a.commentsOpen = r.viewer, r.comments, r.commentsOpen
		// A nested related page goes back to the viewer it was opened from,
		// which sits over the parent related page.
		if a.related != nil {
			return
		}
		if a.backRoute == "" {
			a.backRoute = routeHome
		}
		a.route = a.backRoute
		return
	}
	if a.backRoute == "" {
		a.backRoute = routeHome
	}
	a.route = a.backRoute
}

// applyTheme makes the window use the saved theme; the system's is the one
// the frame starts with when the setting follows it.
func (a *App) applyTheme(c *ui.Context) {
	prefs := c.Preferences()
	dark := c.Theme().Dark
	if a.settings != nil {
		switch a.settings.Theme {
		case themeLight:
			dark = false
		case themeDark:
			dark = true
		}
	}
	theme := designTheme(dark, textScale(c))
	if prefs.HighContrast {
		applyHighContrast(theme, dark)
	}
	// Following the system keeps the desktop's accent; an explicit light or
	// dark choice keeps the app's own blue.
	if (a.settings == nil || a.settings.Theme == themeSystem) && prefs.Accent.A != 0 {
		accent := prefs.Accent
		theme.Accent = accent
		theme.AccentText = accentText(accent)
		if dark {
			theme.AccentHover = accent.Mix(ui.RGB(255, 255, 255), 0.15)
			theme.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.15)
		} else {
			theme.AccentHover = accent.Mix(ui.RGB(0, 0, 0), 0.08)
			theme.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.16)
		}
		if prefs.HighContrast {
			theme.Focus = accent
		} else {
			theme.Focus = accent.Alpha(0.65)
		}
	}
	c.SetTheme(theme)
	// The desktop asks for less motion: every widget animation jumps to its
	// target, while the layouts themselves are still.
	if prefs.ReduceMotion {
		c.DisableAnimations()
	}
}

// textTabs is the app's tab line: quiet words with one accent rule under the
// chosen tab. TabsBase gives it the tab list's roles and keys, the arrows and
// Home/End moving the choice; the look stays a word over a 2px rule.
func textTabs(c *ui.Context, t tokens, selected *int, labels ...string) bool {
	tabs := ui.TabsBase(c, selected, len(labels))
	tabs.List.Gap(t.SpaceXL).AlignItems(ui.End).Children(func() {
		for i, label := range labels {
			tab := tabs.Tab(i).Height(40).Radius(0).Padding(0).Gap(t.SpaceS).
				Column().AlignItems(ui.Start).Label(label)
			fg := t.TextMuted
			if *selected == i {
				fg = t.Text
			} else if tab.Hovered() {
				fg = t.Text
			}
			bar := t.Accent
			if *selected != i {
				bar = ui.Transparent
			}
			tab.Children(func() {
				ui.Text(c, label).FontSize(t.TextSmall).FontWeight(500).TextColor(fg)
				ui.Box(c).FillWidth().Height(2).Background(bar)
			})
		}
	})
	return tabs.List.Changed()
}

// homePage draws the home tabs above the feed of the one chosen. Pixiv's
// recommendation also needs a token, so every tab asks a signed-out user to
// sign in instead of fetching.
func (a *App) homePage(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
		if textTabs(c, t, &a.homeTab, a.t("home.recommended"), a.t("home.following"), a.t("home.ranking")) {
			a.armHome(a.homeTab)
		}
	})
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		if a.homeTab == homeTabRanking {
			ui.Row(c).Padding(0, contentPadding, 0, contentPadding).Children(func() {
				mode := a.rankModeLabel(a.rankMode)
				if ui.Select(c, &mode, a.rankModeChoices()).Width(200).Height(36).Label(a.t("ranking.mode")).Changed() {
					a.setRankMode(a.rankModeValue(mode))
				}
			})
		}
		a.browsePage(c, a.homeFeeds[a.homeTab])
	})
}

// sauceSearchTimeout bounds one reverse image search.
const sauceSearchTimeout = 30 * time.Second

// sauceState is the reverse image search's answer. The view runs on the main
// thread; the request goroutine changes the fields under mu.
type sauceState struct {
	mu       sync.Mutex
	items    []saucenao.Entry
	loading  bool
	searched bool
	err      string
}

// sauceView is what the reverse image search page draws.
type sauceView struct {
	items    []saucenao.Entry
	loading  bool
	searched bool
	err      string
}

func (st *sauceState) snapshot() sauceView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return sauceView{items: st.items, loading: st.loading, searched: st.searched, err: st.err}
}

// startSauceSearch looks up the box's image URL and shows the sources when it
// lands.
func (a *App) startSauceSearch() {
	imageURL := strings.TrimSpace(a.sauceURL)
	apiKey := ""
	if a.settings != nil {
		apiKey = strings.TrimSpace(a.settings.SauceNAOAPIKey)
	}
	search := a.sauceSearchFn
	if imageURL == "" || apiKey == "" || search == nil || a.sauce == nil {
		return
	}
	st := a.sauce
	st.mu.Lock()
	if st.loading {
		st.mu.Unlock()
		return
	}
	st.loading, st.searched, st.err, st.items = true, true, "", nil
	st.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sauceSearchTimeout)
		defer cancel()
		res, err := search(ctx, apiKey, imageURL)

		st.mu.Lock()
		st.loading = false
		switch {
		case errors.Is(err, saucenao.ErrNoResults):
			st.items = nil
		case err != nil:
			st.err = err.Error()
		case res == nil || len(res.Entries) == 0:
			st.items = nil
		default:
			st.items = res.Entries
		}
		st.mu.Unlock()
		a.update(func() {})
	}()
}

// saucePage draws the reverse image search: the URL box and its result cards,
// or the way to configure the key it needs.
func (a *App) saucePage(c *ui.Context, t tokens) {
	apiKey := ""
	if a.settings != nil {
		apiKey = strings.TrimSpace(a.settings.SauceNAOAPIKey)
	}
	if apiKey == "" {
		emptyState(c, iconImage, a.t("search.sauceNoKey"), a.t("search.sauceHint"), func() {
			if ui.PrimaryButton(c, a.t("nav.settings")).Radius(t.RadiusS).Clicked() {
				a.goRoute(routeSettings)
			}
		})
		return
	}
	v := a.sauce.snapshot()
	ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, 0, contentPadding).
		AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, &a.sauceURL).Grow(1).
			Placeholder(a.t("search.sauceURL")).Label(a.t("search.sauceURL"))
		if ui.PrimaryButton(c, a.t("search.sauceSearch")).Disabled(v.loading).Clicked() {
			a.startSauceSearch()
		}
	})
	ui.Row(c).Padding(t.SpaceXS, contentPadding, 0, contentPadding).Children(func() {
		ui.Text(c, a.t("search.sauceHint")).FontSize(t.TextTiny).TextColor(t.TextMuted)
	})
	if v.err != "" {
		ui.Row(c).Padding(t.SpaceXS, contentPadding, 0, contentPadding).Children(func() {
			ui.Text(c, a.tf("search.sauceFailed", v.err)).FontSize(t.TextSmall).TextColor(t.Danger)
		})
	}
	if v.searched && !v.loading && v.err == "" && len(v.items) == 0 {
		emptyState(c, iconImage, a.t("search.sauceEmpty"), "", nil)
		return
	}
	if len(v.items) == 0 {
		return
	}
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(t.SpaceS).Padding(t.SpaceM, contentPadding, t.SpaceL, contentPadding).Children(func() {
			for _, entry := range v.items {
				a.sauceResultCard(c, t, entry)
			}
		})
	})
}

// sauceResultCard shows one source: how closely it matches, its title and
// creator, and a link that opens it in the browser.
func (a *App) sauceResultCard(c *ui.Context, t tokens, entry saucenao.Entry) {
	ui.Column(c).FillWidth().Background(t.Surface).Radius(t.RadiusM).
		Border(1, t.Border).Padding(t.SpaceM).Gap(t.SpaceXS).Children(func() {
		ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strconv.FormatFloat(entry.Similarity, 'f', 1, 64)+"%").FontSize(t.TextSmall).
				FontWeight(600).TextColor(t.Accent)
			if entry.Title != "" {
				ui.Text(c, entry.Title).FontSize(t.TextBody).FontWeight(500).TextColor(t.Text).
					Grow(1).MaxLines(2)
			}
		})
		if entry.Creator != "" {
			ui.Text(c, entry.Creator).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(1)
		}
		if source := sauceSource(entry); source != "" {
			b := ui.ButtonBase(c).Padding(0).Label(source).OnClick(func() {
				a.openSauceSource(c, source)
			})
			b.Children(func() {
				ui.Text(c, source).FontSize(t.TextSmall).TextColor(t.Accent).MaxLines(1)
			})
		}
	})
}

// sauceSource is the link a result opens: its author page when it has one,
// else the first source link.
func sauceSource(entry saucenao.Entry) string {
	candidates := make([]string, 0, 1+len(entry.ExtURLs))
	candidates = append(candidates, entry.AuthorURL)
	candidates = append(candidates, entry.ExtURLs...)
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
			return raw
		}
	}
	return ""
}

// openSauceSource opens a result's source in the default browser.
func (a *App) openSauceSource(c *ui.Context, raw string) {
	open := a.openExternalFn
	if open == nil {
		open = mygo.Shell.OpenExternal
	}
	if err := open(raw); err != nil {
		c.Toast(a.tf("search.sauceFailed", err.Error()))
	}
}

// searchArmed reports whether the result list the search page shows may load.
func (a *App) searchArmed() bool {
	switch a.searchKind {
	case searchNovel:
		return a.searchNovels.isArmed()
	case searchUser:
		return a.searchUsers.isArmed()
	}
	return a.search.isArmed()
}

// pixivLinkKind is the page a pasted Pixiv link opens.
type pixivLinkKind int

const (
	linkWork pixivLinkKind = iota + 1
	linkNovel
	linkUser
)

// pixivLink is the page a pasted Pixiv link names.
type pixivLink struct {
	kind pixivLinkKind
	id   int64
}

// parsePixivLink reads the work, novel or user a whole Pixiv URL names. Only
// an explicit URL counts: a typed number stays a search word.
func parsePixivLink(raw string) (pixivLink, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return pixivLink{}, false
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return pixivLink{}, false
	}
	if host := strings.TrimPrefix(strings.ToLower(u.Host), "www."); host != "pixiv.net" {
		return pixivLink{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return pixivLink{}, false
	}
	// A link may carry a locale, as pixiv.net/en/artworks/123 does.
	kind, idText := parts[len(parts)-2], parts[len(parts)-1]
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return pixivLink{}, false
	}
	switch kind {
	case "artworks", "i":
		return pixivLink{kind: linkWork, id: id}, true
	case "n":
		return pixivLink{kind: linkNovel, id: id}, true
	case "users":
		return pixivLink{kind: linkUser, id: id}, true
	}
	return pixivLink{}, false
}

// openPixivLink shows the page a pasted Pixiv link names. A work's viewer
// needs the work's detail, which loads off the main thread; the other pages
// load themselves.
func (a *App) openPixivLink(link pixivLink) {
	switch link.kind {
	case linkNovel:
		a.openNovel(link.id, "")
	case linkUser:
		a.openUser(link.id)
	case linkWork:
		a.openLinkedWork(link.id)
	}
}

// openLinkedWork fetches the work a pasted link names off the main thread and
// opens it. A failure stays on the search page, under its box.
func (a *App) openLinkedWork(id int64) {
	fetch := a.illustrationFn
	if fetch == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		w, err := fetch(ctx, id)
		a.update(func() {
			if err != nil {
				log.Printf("search: open work %d: %v", id, err)
				a.searchSideMu.Lock()
				a.searchLinkErr = a.tf("search.openLinkFailed", err.Error())
				a.searchSideMu.Unlock()
				return
			}
			a.openViewer(w, 0)
		})
	}()
}

// startSearch replaces the results with a search for the box's word. Both
// tabs get it: the novel list loads when its tab first draws it.
func (a *App) startSearch(st *browseState) {
	word := strings.TrimSpace(st.query)
	if word == "" {
		return
	}
	a.searchSideMu.Lock()
	a.searchLinkErr = ""
	a.searchSideMu.Unlock()
	if link, ok := parsePixivLink(word); ok {
		a.openPixivLink(link)
		return
	}
	// Read the options on the main thread; the load goroutine must not read
	// the fields the search page edits. Another tab's search ignores the
	// illustration filters, which it does not show.
	opts := pixiv.SearchOptions{Word: word, Sort: a.searchSort, Target: a.searchTarget}
	if a.searchKind == searchIllust {
		var err error
		opts, err = a.searchOptions(word)
		if err != nil {
			a.searchErr = err.Error()
			return
		}
	}
	a.searchErr = ""
	a.recordSearch(word)
	st.restart()
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		return a.client.SearchIllustrations(ctx, opts)
	})
	st.masonry = widgets.MasonryState{}
	a.startNovelSearch(word)
	a.startUserSearch(word)
	if a.searchKind == searchNovel {
		a.loadMoreNovels(a.searchNovels)
		return
	}
	if a.searchKind == searchUser {
		a.loadMoreUsers(a.searchUsers)
		return
	}
	a.loadMore(st)
}

// startUserSearch replaces the user list with a search for word. The page is
// not fetched here: it loads when the user tab first draws it.
func (a *App) startUserSearch(word string) {
	st := a.searchUsers
	st.restart()
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextUserPreviews(ctx, next)
		}
		return a.client.SearchUsers(ctx, word)
	})
}

// filterBar draws a feed's filter expression box. Each feed keeps its own
// expression, so switching tabs or searching again keeps it. A collapsed bar
// still names the expression in force: a filtered feed stops paging, and an
// applied filter the user cannot see would look like a feed that ran dry.
func (a *App) filterBar(c *ui.Context, st *browseState) {
	t := themeOf(c)
	if !st.filterOpen {
		ui.Row(c).Padding(0, contentPadding, 0, contentPadding).Gap(t.SpaceS).
			AlignItems(ui.Center).Justify(ui.End).Children(func() {
			if st.filterExpr != nil {
				ui.Text(c, a.tf("filter.active", st.filterApplied)).FontSize(t.TextTiny).
					TextColor(t.Accent).MaxLines(1)
				if iconButton(c, iconClose, a.t("filter.clear"), 24, t.TextMuted, t.SurfaceAlt).Clicked() {
					st.clearFilter()
				}
			}
			b := ui.ButtonBase(c).Height(28).Radius(t.RadiusS).Padding(0, t.SpaceS).
				Label(a.t("filter.title"))
			fg := t.TextMuted
			if b.Hovered() {
				fg = t.Text
			}
			if b.Clicked() {
				st.filterOpen = true
			}
			b.Children(func() {
				ui.Icon(c, iconSearch).Size(13, 13).TextColor(fg)
				ui.Text(c, a.t("filter.title")).FontSize(t.TextTiny).TextColor(fg)
			})
		})
		return
	}
	ui.Column(c).Gap(t.SpaceXS).Padding(0, contentPadding, 0, contentPadding).Children(func() {
		ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &st.filterText).Width(300).
				Placeholder(a.t("filter.placeholder")).Label(a.t("filter.placeholder"))
			if ui.Button(c, a.t("filter.action")).Clicked() {
				st.applyFilter()
				if st.filterErr == "" {
					st.filterOpen = false
				}
			}
			if iconButton(c, iconClose, a.t("viewer.close"), 28, t.TextMuted, t.SurfaceAlt).Clicked() {
				st.filterOpen = false
			}
		})
		if st.filterErr != "" {
			ui.Text(c, a.tf("filter.invalid", st.filterErr)).FontSize(t.TextSmall).TextColor(t.Danger)
		}
	})
}

// applyFilter shows only the works the box's expression passes; an empty
// expression shows every work again.
func (st *browseState) applyFilter() {
	text := strings.TrimSpace(st.filterText)
	var expr filter.Expr
	if text != "" {
		parsed, err := filter.Parse(text)
		if err != nil {
			st.filterErr = err.Error()
			return
		}
		expr = parsed
	}
	st.filterErr = ""
	st.filterApplied = text
	st.filterExpr = expr
	// The place and the tiles were laid out for the works shown until now.
	st.masonry = widgets.MasonryState{}
	st.tilesWidth = 0
	st.tilesText = 0
}

// clearFilter drops the applied expression and shows every work again.
func (st *browseState) clearFilter() {
	st.filterText = ""
	st.applyFilter()
}

// feedTiles returns the works a feed draws and the tiles they draw as: the
// ones its applied filter passes, or all of them. The tiles are built again
// while a filter is applied, since the passing set grows as pages load.
func feedTiles(st *browseState, all []pixiv.Illustration, tiles []widgets.MasonryItem, width, textHeight float64) ([]pixiv.Illustration, []widgets.MasonryItem) {
	items := all
	if st.filterExpr != nil {
		items = make([]pixiv.Illustration, 0, len(all))
		for i := range all {
			if st.filterExpr.Match(&all[i]) {
				items = append(items, all[i])
			}
		}
	}
	if st.filterExpr != nil || width != st.tilesWidth || textHeight != st.tilesText {
		st.tilesWidth, st.tilesText = width, textHeight
		tiles = make([]widgets.MasonryItem, len(items))
		for i, w := range items {
			tiles[i] = widgets.MasonryItem{Key: w.ID, Aspect: cardAspect(w.Aspect(), width, textHeight)}
		}
	}
	return items, tiles
}

// loginPrompt guides a signed-out user to login.
func (a *App) loginPrompt(c *ui.Context) {
	hint := a.t("login.promptHint")
	if !a.webviewOK {
		// The window cannot show Pixiv's page, so name the path that works.
		hint = a.t("login.noWebView")
	}
	emptyState(c, iconUser, a.t("login.prompt"), hint, func() {
		if ui.PrimaryButton(c, a.t("login.title")).Radius(themeOf(c).RadiusS).Clicked() {
			a.ShowLogin()
		}
	})
}
