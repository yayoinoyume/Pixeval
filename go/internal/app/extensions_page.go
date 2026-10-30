package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	ext "github.com/yayoinoyume/pixeval-go/internal/extensions"
)

// openExtensions shows the extension page, which returns to the settings page
// it was opened from.
func (a *App) openExtensions() {
	a.backRoute = routeSettings
	a.goRoute(routeExtensions)
}

// extensionsPage lists the installed local extensions, lets the user enable
// or disable one, reload the folder, and open it in the file manager. An
// extension is a separate executable; this page never loads a .NET component.
func (a *App) extensionsPage(c *ui.Context) {
	t := themeOf(c)
	var list []ext.Extension
	var invalid []ext.InvalidExtension
	if a.extensions != nil {
		list = a.extensions.Extensions()
		invalid = a.extensions.InvalidExtensions()
	}
	a.infoPage(c, func() {
		ui.Row(c).FillWidth().Gap(t.SpaceS).Justify(ui.End).Children(func() {
			if ui.Button(c, a.t("ext.openFolder")).Clicked() {
				a.openExtensionsFolder(c)
			}
			if ui.Button(c, a.t("ext.reload")).Clicked() {
				a.reloadExtensions(c)
			}
		})
		if a.extensions == nil {
			a.infoCard(c, func() {
				ui.Text(c, a.t("ext.hostUnavailable")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Danger)
			})
			return
		}
		if len(list) == 0 && len(invalid) == 0 {
			a.infoCard(c, func() {
				ui.Column(c).Gap(t.SpaceXS).Children(func() {
					ui.Text(c, a.t("ext.empty")).FontSize(t.TextTitle).FontWeight(600).TextColor(t.Text)
					ui.Text(c, a.t("ext.emptyHint")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.TextMuted)
				})
			})
		}
		for _, item := range invalid {
			a.invalidExtensionCard(c, item)
		}
		for _, item := range list {
			a.extensionCard(c, item)
		}
		a.infoCard(c, func() {
			ui.Column(c).Gap(t.SpaceS).Children(func() {
				ui.Text(c, a.t("ext.protocol")).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
				ui.Text(c, a.t("ext.security")).FontSize(t.TextSmall).LineHeight(1.6).TextColor(t.TextMuted)
			})
		})
	})
}

// extensionCard is one installed extension: its name and version, the enable
// switch, what it declares, and what the host last saw it do.
func (a *App) extensionCard(c *ui.Context, item ext.Extension) {
	t := themeOf(c)
	a.infoCard(c, func() {
		ui.Row(c).FillWidth().Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, item.Name).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).MaxLines(1)
				ui.Text(c, a.tf("ext.version", item.Version)).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
			})
			if ui.Button(c, a.tf("ext.reloadOne", item.Name)).Clicked() {
				a.reloadExtension(c, item.ID)
			}
			a.extensionSwitch(c, item)
		})
		if item.Description != "" {
			ui.Text(c, item.Description).FontSize(t.TextBody).LineHeight(1.6).TextColor(t.Text)
		}
		if len(item.Capabilities) > 0 {
			ui.Text(c, a.tf("ext.capabilities", strings.Join(item.Capabilities, ", "))).
				FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(2)
		}
		statusKey := "ext.statusDisabled"
		statusColor := t.TextMuted
		switch item.Status {
		case ext.StatusStopped:
			statusKey = "ext.statusStopped"
		case ext.StatusRunning:
			statusKey, statusColor = "ext.statusRunning", t.Accent
		case ext.StatusError:
			statusKey, statusColor = "ext.statusError", t.Danger
		}
		ui.Text(c, a.t(statusKey)).FontSize(t.TextSmall).TextColor(statusColor)
		if item.Error != "" {
			ui.Text(c, item.Error).FontSize(t.TextTiny).TextColor(t.Danger).MaxLines(3)
		}
	})
}

// invalidExtensionCard shows a folder whose manifest could not be read, so a
// broken install is visible and can be retried after it is fixed.
func (a *App) invalidExtensionCard(c *ui.Context, item ext.InvalidExtension) {
	t := themeOf(c)
	a.infoCard(c, func() {
		ui.Row(c).FillWidth().Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, a.tf("ext.invalid", item.Folder)).FontSize(t.TextBody).FontWeight(600).TextColor(t.Danger).MaxLines(1)
				ui.Text(c, item.Error).FontSize(t.TextTiny).TextColor(t.Danger).MaxLines(3)
			})
			if ui.Button(c, a.tf("ext.reloadOne", item.Folder)).Clicked() {
				a.reloadExtension(c, item.Folder)
			}
		})
	})
}

// extensionSwitch is the enable control of one extension. It applies the new
// state at once, so the next frame draws the host's answer.
func (a *App) extensionSwitch(c *ui.Context, item ext.Extension) {
	t := themeOf(c)
	on := item.Enabled
	sw := ui.SwitchBase(c, &on).Size(40, 24).Radius(t.RadiusFull).
		Label(a.tf("ext.toggle", item.Name))
	sw.Children(func() {
		track := t.BorderStrong
		if on {
			track = t.AccentFill
		}
		ui.Box(c).Fill().Radius(t.RadiusFull).Background(track)
		knob := ui.Box(c).Size(20, 20).Radius(t.RadiusFull).Background(t.AccentText).
			Absolute().Top(2)
		if on {
			knob = knob.Right(2)
		} else {
			knob = knob.Left(2)
		}
	})
	if sw.Changed() {
		if err := a.extensions.SetEnabled(item.ID, on); err != nil {
			c.Toast(a.tf("ext.saveFailed", err.Error()))
		}
	}
}

// openExtensionsFolder opens the folder the user installs extensions in.
func (a *App) openExtensionsFolder(c *ui.Context) {
	if a.extensions == nil {
		return
	}
	open := a.openPathFn
	if open == nil {
		open = mygo.Shell.OpenPath
	}
	if err := open(a.extensions.Dir()); err != nil {
		c.Toast(a.tf("ext.openFailed", err.Error()))
	}
}

// reloadExtensions reads the folder again and stops the processes it started.
func (a *App) reloadExtensions(c *ui.Context) {
	if a.extensions == nil {
		return
	}
	if err := a.extensions.Reload(); err != nil {
		c.Toast(a.tf("ext.reloadFailed", err.Error()))
	}
}

// reloadExtension reads one folder again and stops its process.
func (a *App) reloadExtension(c *ui.Context, id string) {
	if a.extensions == nil {
		return
	}
	if err := a.extensions.ReloadExtension(id); err != nil {
		c.Toast(a.tf("ext.reloadFailed", err.Error()))
	}
}

// extensionsRow is the settings row that opens the extension page.
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
