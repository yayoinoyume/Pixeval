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
	routeSettings  route = "settings"
)

func (r route) label() string {
	switch r {
	case routeSearch:
		return "搜索"
	case routeDownloads:
		return "下载"
	case routeSettings:
		return "设置"
	default:
		return "首页"
	}
}

var navRoutes = []route{routeHome, routeSearch, routeDownloads, routeSettings}

// View builds the shell: the navigation on the left, the page title and the
// sign-in state above the page. The viewer takes the whole window instead.
func (a *App) View(c *ui.Context) {
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
			if ui.Button(c, r.label()).Clicked() {
				a.route = r
			}
		}
	})
}

func (a *App) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(12).Padding(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, a.route.label()).FontSize(22).Bold().Grow(1)
		if a.session == nil {
			if ui.Button(c, "登录").Clicked() {
				a.ShowLogin()
			}
			return
		}
		ui.Text(c, "已登录："+a.userName()).TextColor(t.TextMuted)
		if ui.Button(c, "退出").Clicked() {
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
	default:
		t := c.Theme()
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.route.label()+"（M1 占位）").TextColor(t.TextMuted)
		})
	}
}

func (a *App) homePage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		a.browsePage(c, a.home)
	})
}

// loginPrompt guides a signed-out user to login.
func (a *App) loginPrompt(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Text(c, "登录 Pixiv 后开始浏览").TextColor(t.TextMuted)
		if ui.Button(c, "登录 Pixiv").Clicked() {
			a.ShowLogin()
		}
	})
}
