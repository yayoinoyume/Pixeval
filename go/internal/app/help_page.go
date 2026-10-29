package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// pixevalGoURL is the project's repository, which the help and about pages
// link to.
const pixevalGoURL = "https://github.com/yayoinoyume/pixeval-go"

// openHelp shows the help page, which returns to the settings page it was
// opened from.
func (a *App) openHelp() {
	a.backRoute = routeSettings
	a.goRoute(routeHelp)
}

// openAbout shows the about page, which returns to the settings page it was
// opened from.
func (a *App) openAbout() {
	a.backRoute = routeSettings
	a.goRoute(routeAbout)
}

// helpPage lists the shortcuts, says what the app does, and links to the
// repository.
func (a *App) helpPage(c *ui.Context) {
	t := themeOf(c)
	a.infoPage(c, func() {
		a.infoCard(c, func() {
			ui.Text(c, a.t("help.shortcuts")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
		})
		a.infoCard(c, func() {
			ui.Text(c, a.t("help.features")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
		})
		a.githubButton(c)
	})
}

// aboutPage names the app and its version, states the license, and links to
// the repository.
func (a *App) aboutPage(c *ui.Context) {
	t := themeOf(c)
	a.infoPage(c, func() {
		a.infoCard(c, func() {
			ui.Column(c).Gap(t.SpaceXS).Children(func() {
				ui.Text(c, "Pixeval Go").FontSize(t.TextTitle).FontWeight(600).TextColor(t.Text)
				ui.Text(c, a.tf("about.version", appVersion())).FontSize(t.TextBody).TextColor(t.TextMuted)
				ui.Text(c, a.t("about.license")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
			})
		})
		a.githubButton(c)
	})
}

// infoPage gives a help or about page its scroll, width and padding.
func (a *App) infoPage(c *ui.Context, body func()) {
	t := themeOf(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Row(c).Padding(t.SpaceXL, contentPadding).Children(func() {
				ui.Column(c).Width(min(contentWidth(c), 900)).Gap(t.SpaceL).Children(body)
			})
		})
	})
}

// infoCard is a card of text or content, like a settings card.
func (a *App) infoCard(c *ui.Context, body func()) {
	t := themeOf(c)
	ui.Column(c).FillWidth().Background(t.Surface).Radius(t.RadiusL).
		Border(1, t.Border).Clip().Padding(t.SpaceL, settingsPad).Children(body)
}

// githubButton opens the repository in the default browser.
func (a *App) githubButton(c *ui.Context) {
	if ui.Button(c, a.t("about.github")).Clicked() {
		a.openLink(c, pixevalGoURL)
	}
}

// openLink opens a URL in the browser, and reports a failure as a toast.
func (a *App) openLink(c *ui.Context, url string) {
	open := a.openExternalFn
	if open == nil {
		open = mygo.Shell.OpenExternal
	}
	if err := open(url); err != nil {
		c.Toast(a.tf("about.openFailed", err.Error()))
	}
}
