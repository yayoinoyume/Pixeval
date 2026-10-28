package app

import (
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

var navRoutes = []route{routeHome, routeSearch, routeSubs, routeDownloads, routeHistory, routeSettings}

// View builds the shell: the navigation on the left, the page title and the
// sign-in state above the page. The viewer takes the whole window instead.
func (a *App) View(c *ui.Context) {
	a.applyTheme(c)
	a.applySavedSettings(c)
	a.showNotice(c)
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

func (a *App) nav(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(180).Padding(12).Gap(4).Background(t.Surface).Shrink(0).Children(func() {
		ui.Text(c, "Pixeval Go").FontSize(18).Bold().Padding(8)
		for _, r := range navRoutes {
			if ui.Button(c, a.routeLabel(r)).Clicked() {
				a.route = r
			}
		}
	})
}

func (a *App) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(12).Padding(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		if a.route == routeUser || a.route == routeSeries {
			if ui.Button(c, a.t("common.back")).Clicked() {
				a.goBack()
			}
		}
		ui.Text(c, a.routeLabel(a.route)).FontSize(22).Bold().Grow(1)
		if a.session == nil {
			if ui.Button(c, a.t("nav.login")).Clicked() {
				a.ShowLogin()
			}
			return
		}
		ui.Text(c, a.tf("nav.loggedIn", a.userName())).TextColor(t.TextMuted)
		if ui.Button(c, a.t("nav.logout")).Clicked() {
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
		t := c.Theme()
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
	if a.settings == nil {
		return
	}
	switch a.settings.Theme {
	case themeLight:
		c.SetTheme(ui.LightTheme())
	case themeDark:
		c.SetTheme(ui.DarkTheme())
	}
}

// homePage draws the home tabs above the feed of the one chosen. Pixiv's
// recommendation also needs a token, so every tab asks a signed-out user to
// sign in instead of fetching.
func (a *App) homePage(c *ui.Context) {
	if ui.Tabs(c, &a.homeTab, a.t("home.recommended"), a.t("home.following"), a.t("home.ranking")).Padding(12, 12, 0, 12).Changed() {
		a.armHome(a.homeTab)
	}
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.browsePage(c, a.homeFeeds[a.homeTab])
	})
}

// filterBar draws a feed's filter expression box. Each feed keeps its own
// expression, so switching tabs or searching again keeps it.
func (a *App) filterBar(c *ui.Context, st *browseState) {
	t := c.Theme()
	ui.Column(c).Gap(4).Padding(12, 12, 0, 12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &st.filterText).Grow(1).Placeholder(a.t("filter.placeholder"))
			if ui.Button(c, a.t("filter.action")).Clicked() {
				st.applyFilter()
			}
		})
		if st.filterErr != "" {
			ui.Text(c, a.tf("filter.invalid", st.filterErr)).TextColor(t.Danger)
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
}

// feedTiles returns the works a feed draws and the tiles they draw as: the
// ones its applied filter passes, or all of them. The tiles are built again
// while a filter is applied, since the passing set grows as pages load.
func feedTiles(st *browseState, all []pixiv.Illustration, width float64) ([]pixiv.Illustration, []widgets.MasonryItem) {
	items := all
	if st.filterExpr != nil {
		items = make([]pixiv.Illustration, 0, len(all))
		for i := range all {
			if st.filterExpr.Match(&all[i]) {
				items = append(items, all[i])
			}
		}
	}
	tiles := st.tiles
	if st.filterExpr != nil || width != st.tilesWidth {
		st.tilesWidth = width
		tiles = make([]widgets.MasonryItem, len(items))
		for i, w := range items {
			tiles[i] = widgets.MasonryItem{Key: w.ID, Aspect: cardAspect(w.Aspect(), width)}
		}
	}
	return items, tiles
}

// loginPrompt guides a signed-out user to login.
func (a *App) loginPrompt(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Text(c, a.t("login.prompt")).TextColor(t.TextMuted)
		if ui.Button(c, a.t("login.title")).Clicked() {
			a.ShowLogin()
		}
	})
}
