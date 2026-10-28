package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/filter"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// route is the page the shell shows.
type route string

const (
	routeHome      route = "home"
	routeSearch    route = "search"
	routeSubs      route = "subscriptions"
	routeDownloads route = "downloads"
	routeHistory   route = "history"
	routeSettings  route = "settings"
	routeUser      route = "user"
	routeSeries    route = "series"
	routeNovel     route = "novel"
)

// routeLabel is the navigation label of the page.
func (a *App) routeLabel(r route) string {
	return a.t("nav." + string(r))
}

var (
	browseRoutes  = []route{routeHome, routeSearch, routeSubs}
	libraryRoutes = []route{routeDownloads, routeHistory}
)

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

// goRoute shows r, closing the viewer that covers the pages.
func (a *App) goRoute(r route) {
	a.closeViewer()
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
	case routeUser, routeSeries, routeNovel:
		a.goBack()
	}
}

// nav is the window's navigation: the brand, the browsing group, the library
// group and, held to the bottom, the settings.
func (a *App) nav(c *ui.Context) {
	t := themeOf(c)
	ui.Column(c).Width(navWidth).FillHeight().Padding(0, t.SpaceM, t.SpaceM, t.SpaceM).
		Gap(t.SpaceXS).Background(t.Surface).Shrink(0).
		BorderWidth(0, 0, 0, 1).BorderColor(t.Border).
		Role(ui.RoleGroup).Label(a.t("nav.region")).Children(func() {
		a.brand(c)
		ui.Box(c).Height(1).FillWidth().Background(t.Border).MarginY(t.SpaceS)
		a.navGroupLabel(c, a.t("nav.groupBrowse"))
		for _, r := range browseRoutes {
			a.navItem(c, r)
		}
		a.navGroupLabel(c, a.t("nav.groupLibrary"))
		for _, r := range libraryRoutes {
			a.navItem(c, r)
		}
		ui.Spacer(c)
		ui.Box(c).Height(1).FillWidth().Background(t.Border).MarginY(t.SpaceS)
		a.navItem(c, routeSettings)
	})
}

// brand is the app's mark and wordmark at the top of the navigation.
func (a *App) brand(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Height(64).Padding(0, t.SpaceS).Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(28, 28).Radius(t.RadiusM).Background(t.AccentFill).Center().Children(func() {
			ui.Icon(c, iconLogo).Size(17, 17).TextColor(t.AccentText)
		})
		ui.Column(c).Gap(0).Children(func() {
			ui.RichText(c,
				ui.Span{Text: "Pixeval ", Size: t.TextBody, Weight: 700, Color: t.Text},
				ui.Span{Text: "Go", Size: t.TextBody, Weight: 700, Color: t.Accent},
			)
			ui.Text(c, appVersion()).FontSize(t.TextTiny).TextColor(t.TextMuted)
		})
	})
}

// navGroupLabel names a group of destinations. It is a quiet label, not a
// heading: the pages carry the headings.
func (a *App) navGroupLabel(c *ui.Context, label string) {
	t := themeOf(c)
	ui.Text(c, label).FontSize(t.TextTiny).TextColor(t.TextMuted).
		Padding(t.SpaceL, t.SpaceS, t.SpaceXS, t.SpaceS)
}

// navItem is one destination: its glyph, its name, a hover face and, when
// chosen, an accent face with a bar at its left edge.
func (a *App) navItem(c *ui.Context, r route) {
	t := themeOf(c)
	selected := a.route == r
	label := a.routeLabel(r)
	b := ui.ButtonBase(c).Key("nav."+string(r)).Height(38).FillWidth().
		Radius(t.RadiusM).Padding(0, t.SpaceS, 0, 0).Gap(t.SpaceS).
		AlignItems(ui.Center).Justify(ui.Start)
	hovered := b.Hovered()
	switch {
	case selected:
		b = b.Background(t.AccentSoft)
	case hovered:
		b = b.Background(t.SurfaceAlt)
	}
	fg := t.TextMuted
	switch {
	case selected:
		fg = t.Accent
	case hovered:
		fg = t.Text
	}
	b.Children(func() {
		if selected {
			ui.Box(c).Width(3).Height(16).Radius(2).Background(t.Accent)
		} else {
			ui.Box(c).Width(3).Height(16)
		}
		ui.Icon(c, navIcon(r)).Size(18, 18).TextColor(fg)
		ui.Text(c, label).FontSize(t.TextBody).TextColor(fg).MaxLines(1)
	})
	if b.Clicked() {
		a.goRoute(r)
	}
}

// header is the page title and the account state above the page.
func (a *App) header(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Height(64).Gap(t.SpaceM).Padding(0, contentPadding).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if a.route == routeUser || a.route == routeSeries {
			if iconButton(c, iconLeft, a.t("common.back"), 32, t.TextMuted, t.SurfaceAlt).Clicked() {
				a.goBack()
			}
		}
		ui.Text(c, a.routeLabel(a.route)).FontSize(t.TextTitle).Bold().
			LineHeight(1.2).TextColor(t.Text).Grow(1).MaxLines(1).MinWidth(0)
		if a.session == nil {
			if ui.PrimaryButton(c, a.t("nav.login")).Clicked() {
				a.ShowLogin()
			}
			return
		}
		a.userBadge(c)
	})
}

// userBadge is the signed-in account: an initial, the name and a quiet way
// out.
func (a *App) userBadge(c *ui.Context) {
	t := themeOf(c)
	name := a.userName()
	ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(28, 28).Radius(t.RadiusFull).Background(t.AccentSoft).Center().Children(func() {
			ui.Text(c, initialOf(name)).FontSize(t.TextTiny).FontWeight(600).TextColor(t.Accent)
		})
		ui.Text(c, name).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(1)
		if iconButton(c, iconLogout, a.t("nav.logout"), 30, t.TextMuted, t.SurfaceAlt).Clicked() {
			a.logout()
		}
	})
}

func (a *App) page(c *ui.Context) {
	switch a.route {
	case routeHome:
		a.homePage(c)
	case routeSearch:
		a.searchPage(c)
	case routeSubs:
		a.subscriptionsPage(c)
	case routeDownloads:
		a.downloadsPage(c)
	case routeHistory:
		a.historyPage(c)
	case routeSettings:
		a.settingsPage(c)
	case routeUser:
		a.userPage(c)
	case routeSeries:
		a.seriesPage(c)
	case routeNovel:
		a.novelPage(c)
	default:
		t := themeOf(c)
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.tf("nav.placeholder", a.routeLabel(a.route))).TextColor(t.TextMuted)
		})
	}
}

// goBack leaves a user page or series page for the page it was opened from.
func (a *App) goBack() {
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

// homePage draws the home tabs above the feed of the one chosen. Pixiv's
// recommendation also needs a token, so every tab asks a signed-out user to
// sign in instead of fetching.
func (a *App) homePage(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Padding(t.SpaceL, contentPadding, 0, contentPadding).Children(func() {
		if ui.Segmented(c, &a.homeTab, a.t("home.recommended"), a.t("home.following"), a.t("home.ranking")).Changed() {
			a.armHome(a.homeTab)
		}
	})
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.browsePage(c, a.homeFeeds[a.homeTab])
	})
}

// searchPage draws the search box and its results.
func (a *App) searchPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	st := a.search
	t := themeOf(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, 0, contentPadding).
			AlignItems(ui.Center).Children(func() {
			box := ui.TextInput(c, &st.query).Grow(1).
				Placeholder(a.t("search.placeholder")).Label(a.t("search.placeholder"))
			if st.focusQuery {
				st.focusQuery = false
				box = box.Focus()
			}
			if ui.PrimaryButton(c, a.t("search.action")).Clicked() {
				a.startSearch(st)
			}
		})
		ui.Row(c).Padding(t.SpaceM, contentPadding, 0, contentPadding).Children(func() {
			ui.Segmented(c, &a.searchKind, a.t("search.illust"), a.t("search.novel"))
		})
		if !a.searchArmed() {
			emptyState(c, iconSearch, a.t("search.hint"), "", nil)
			return
		}
		if a.searchKind == searchNovel {
			a.novelResults(c)
			return
		}
		a.browsePage(c, st)
	})
}

// searchArmed reports whether the result list the search page shows may load.
func (a *App) searchArmed() bool {
	if a.searchKind == searchNovel {
		return a.searchNovels.isArmed()
	}
	return a.search.isArmed()
}

// startSearch replaces the results with a search for the box's word. Both
// tabs get it: the novel list loads when its tab first draws it.
func (a *App) startSearch(st *browseState) {
	word := strings.TrimSpace(st.query)
	if word == "" {
		return
	}
	st.restart()
	st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		return a.client.SearchIllustrations(ctx, pixiv.SearchOptions{Word: word})
	})
	st.masonry = widgets.MasonryState{}
	a.startNovelSearch(word)
	if a.searchKind == searchNovel {
		a.loadMoreNovels(a.searchNovels)
		return
	}
	a.loadMore(st)
}

// filterBar draws a feed's filter expression box. Each feed keeps its own
// expression, so switching tabs or searching again keeps it.
func (a *App) filterBar(c *ui.Context, st *browseState) {
	t := themeOf(c)
	ui.Column(c).Gap(t.SpaceXS).Padding(t.SpaceL, contentPadding, 0, contentPadding).Children(func() {
		ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &st.filterText).Grow(1).Placeholder(a.t("filter.placeholder"))
			if ui.Button(c, a.t("filter.action")).Clicked() {
				st.applyFilter()
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
	st.filterExpr = expr
	// The place and the tiles were laid out for the works shown until now.
	st.masonry = widgets.MasonryState{}
	st.tilesWidth = 0
	st.tilesText = 0
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
		if ui.PrimaryButton(c, a.t("login.title")).Clicked() {
			a.ShowLogin()
		}
	})
}
