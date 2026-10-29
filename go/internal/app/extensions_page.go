package app

import (
	"github.com/egoist/mygo/ui"
)

// openExtensions shows the extension information page, which returns to the
// settings page it was opened from.
func (a *App) openExtensions() {
	a.backRoute = routeSettings
	a.goRoute(routeExtensions)
}

// extensionsPage explains that this build has no extension host. It offers no
// add, enable or remove actions, because the extension files are .NET native
// components that the Go version cannot load.
func (a *App) extensionsPage(c *ui.Context) {
	t := themeOf(c)
	a.infoPage(c, func() {
		a.infoCard(c, func() {
			ui.Column(c).Gap(t.SpaceXS).Children(func() {
				ui.Text(c, a.t("ext.hostMissing")).FontSize(t.TextTitle).FontWeight(600).TextColor(t.Text)
				ui.Text(c, a.t("ext.hostMissingHint")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.TextMuted)
			})
		})
		a.infoCard(c, func() {
			ui.Text(c, a.t("ext.why")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
		})
		a.infoCard(c, func() {
			ui.Text(c, a.t("ext.available")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
		})
		a.githubButton(c)
	})
}

// extensionsRow is the settings row that opens the extension information page.
func (a *App) extensionsRow(c *ui.Context) {
	t := themeOf(c)
	b := ui.ButtonBase(c).FillWidth().MinHeight(52).Padding(t.SpaceS, settingsPad).Gap(t.SpaceXL).
		AlignItems(ui.Center).Label(a.t("nav.extensions")).Tooltip(a.t("ext.settingsHint"))
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	b.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.t("nav.extensions")).FontSize(t.TextBody).TextColor(t.Text)
			ui.Text(c, a.t("ext.settingsHint")).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(2)
		})
		ui.Icon(c, iconRight).Size(16, 16).TextColor(t.TextMuted)
	})
	if b.Clicked() {
		a.openExtensions()
	}
}
