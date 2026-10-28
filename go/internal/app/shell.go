package app

import (
	"github.com/egoist/mygo/ui"
)

// route is the page the shell shows.
type route string

const (
	routeHome      route = "home"
	routeSearch    route = "search"
	routeDownloads route = "downloads"
	routeHistory   route = "history"
	routeSettings  route = "settings"
)

// routeLabel is the navigation label of the page.
func (a *App) routeLabel(r route) string {
	return a.t("nav." + string(r))
}

var navRoutes = []route{routeHome, routeSearch, routeDownloads, routeHistory, routeSettings}

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
	case routeDownloads:
		a.downloadsPage(c)
	case routeHistory:
		a.historyPage(c)
	case routeSettings:
		a.settingsPage(c)
	default:
		t := c.Theme()
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.tf("nav.placeholder", a.routeLabel(a.route))).TextColor(t.TextMuted)
		})
	}
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
