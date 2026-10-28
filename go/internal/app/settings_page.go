package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
)

// The check fetches a manifest; the install downloads an archive.
const (
	updateCheckTimeout   = 60 * time.Second
	updateInstallTimeout = 30 * time.Minute
)

// updateState is what the settings page knows about updates: a build that
// cannot update itself, the last check, and the update it found.
type updateState struct {
	off        bool
	checking   bool
	installing bool
	version    string
	latest     bool
	err        string
	pending    *mygo.Update
}

// themeChoices are the theme labels the settings page shows.
func (a *App) themeChoices() []string {
	return []string{a.t("settings.themeSystem"), a.t("settings.themeLight"), a.t("settings.themeDark")}
}

// themeLabel is the label the settings page shows for a saved theme.
func (a *App) themeLabel(theme string) string {
	switch theme {
	case themeLight:
		return a.t("settings.themeLight")
	case themeDark:
		return a.t("settings.themeDark")
	default:
		return a.t("settings.themeSystem")
	}
}

// themeValue is the theme a settings page label stands for.
func (a *App) themeValue(label string) string {
	switch label {
	case a.t("settings.themeLight"):
		return themeLight
	case a.t("settings.themeDark"):
		return themeDark
	default:
		return themeSystem
	}
}

// languageChoices are the language labels the settings page shows.
func (a *App) languageChoices() []string {
	return []string{
		a.t("settings.langSystem"),
		a.t("settings.langZh"),
		a.t("settings.langEn"),
		a.t("settings.langFr"),
		a.t("settings.langRu"),
	}
}

// languageLabel is the label the settings page shows for a saved language.
func (a *App) languageLabel(lang string) string {
	switch lang {
	case i18n.LangZhHans:
		return a.t("settings.langZh")
	case i18n.LangEnUS:
		return a.t("settings.langEn")
	case i18n.LangFrFR:
		return a.t("settings.langFr")
	case i18n.LangRuRU:
		return a.t("settings.langRu")
	default:
		return a.t("settings.langSystem")
	}
}

// languageValue is the language a settings page label stands for.
func (a *App) languageValue(label string) string {
	switch label {
	case a.t("settings.langZh"):
		return i18n.LangZhHans
	case a.t("settings.langEn"):
		return i18n.LangEnUS
	case a.t("settings.langFr"):
		return i18n.LangFrFR
	case a.t("settings.langRu"):
		return i18n.LangRuRU
	default:
		return i18n.LangAuto
	}
}

// settingsPage draws the preferences. The controls edit the app's settings in
// place, so the theme applies on the next frame, and the download directory on
// the next download.
func (a *App) settingsPage(c *ui.Context) {
	s := a.settings
	if s == nil {
		return
	}
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Padding(24).Gap(16).Children(func() {
		ui.Form(c, func() {
			theme := a.themeLabel(s.Theme)
			ui.Field(c, a.t("settings.theme"), func() {
				if ui.Select(c, &theme, a.themeChoices()).Changed() {
					s.Theme = a.themeValue(theme)
				}
			})
			language := a.languageLabel(s.Language)
			ui.Field(c, a.t("settings.language"), func() {
				if ui.Select(c, &language, a.languageChoices()).Changed() {
					s.Language = a.languageValue(language)
					// The theme applies on the frame it changes in; the language
					// applies at once too, like the rest of the page's controls.
					a.loadLanguage()
					c.Invalidate()
				}
			})
			ui.Field(c, a.t("settings.downloadDir"), func() {
				ui.TextInput(c, &s.DownloadDir).Width(360)
			}).Description(a.t("settings.downloadDirHint"))
			concurrency := float64(s.DownloadConcurrency)
			ui.Field(c, a.t("settings.concurrency"), func() {
				ui.NumberInput(c, &concurrency, 1, 8, 1)
			})
			s.DownloadConcurrency = int(concurrency)
			ui.Field(c, a.t("settings.showR18"), func() {
				ui.Switch(c, &s.ShowR18)
			}).Description(a.t("settings.showR18Hint"))
			width := s.CardWidth
			ui.Field(c, a.t("settings.cardWidth"), func() {
				ui.NumberInput(c, &width, 180, 360, 10)
			})
			s.CardWidth = width
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, a.t("common.save")).Clicked() {
				a.saveSettings()
			}
		})
		ui.Text(c, a.t("shortcuts.hint")).FontSize(12).TextColor(t.TextMuted)
		if a.settingsErr != "" {
			ui.Text(c, a.settingsErr).TextColor(t.Danger)
		}
		a.updatesSection(c)
	})
}

// updatesSection draws the update check and what its last run found. The
// check and the install run off the main thread and land in updateSt.
func (a *App) updatesSection(c *ui.Context) {
	t := c.Theme()
	st := a.updateSnapshot()
	ui.Text(c, a.t("settings.updates")).Bold()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, a.t("settings.checkUpdate")).Disabled(st.off || st.checking || st.installing).Clicked() {
			a.checkUpdate()
			c.Invalidate()
		}
		if st.version != "" {
			if ui.Button(c, a.t("settings.installRestart")).Disabled(st.installing).Clicked() {
				a.installUpdate()
				c.Invalidate()
			}
		}
	})
	switch {
	case st.off:
		ui.Text(c, a.t("settings.updatesOff")).TextColor(t.TextMuted)
	case st.err != "":
		ui.Text(c, st.err).TextColor(t.Danger)
	case st.installing:
		ui.Text(c, a.t("settings.updateInstalling")).TextColor(t.TextMuted)
	case st.version != "":
		ui.Text(c, a.tf("settings.updateAvailable", st.version))
	case st.latest:
		ui.Text(c, a.t("settings.updateLatest")).TextColor(t.TextMuted)
	}
}

// updateSnapshot reads the update state the page draws.
func (a *App) updateSnapshot() updateState {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateSt
}

// checkUpdate asks the feed for a newer version off the main thread.
func (a *App) checkUpdate() {
	if a.updater == nil {
		return
	}
	a.updateMu.Lock()
	if a.updateSt.off || a.updateSt.checking || a.updateSt.installing {
		a.updateMu.Unlock()
		return
	}
	a.updateSt.checking = true
	a.updateSt.err = ""
	a.updateSt.version = ""
	a.updateSt.latest = false
	a.updateSt.pending = nil
	a.updateMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		u, err := a.updater.Check(ctx)
		a.update(func() {
			a.updateMu.Lock()
			a.updateSt.checking = false
			switch {
			case err != nil:
				a.updateSt.err = a.tf("settings.updateCheckFailed", err.Error())
			case u == nil:
				a.updateSt.latest = true
			default:
				a.updateSt.version = u.Version
				a.updateSt.pending = u
			}
			a.updateMu.Unlock()
		})
	}()
}

// installUpdate installs the update the last check found, then restarts the
// app so that the new version runs.
func (a *App) installUpdate() {
	a.updateMu.Lock()
	u := a.updateSt.pending
	if a.updater == nil || u == nil || a.updateSt.installing {
		a.updateMu.Unlock()
		return
	}
	a.updateSt.installing = true
	a.updateSt.err = ""
	a.updateMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateInstallTimeout)
		defer cancel()
		err := a.updater.Install(ctx, u)
		a.update(func() {
			a.updateMu.Lock()
			if err != nil {
				a.updateSt.installing = false
				a.updateSt.err = a.tf("settings.updateInstallFailed", err.Error())
				a.updateMu.Unlock()
				return
			}
			a.updateMu.Unlock()
			mygo.App.Relaunch()
		})
	}()
}

// applySavedSettings picks up a save that finished off the main thread: it
// applies the download directory and concurrency, and shows the toast.
func (a *App) applySavedSettings(c *ui.Context) {
	select {
	case err := <-a.settingsSaved:
		if err != nil {
			a.settingsErr = a.tf("settings.saveFailed", err.Error())
			return
		}
		a.settingsErr = ""
		a.downloadsDir = settingsDownloadDir(a.settings)
		if a.downloads != nil {
			a.downloads.SetConcurrency(a.settings.DownloadConcurrency)
		}
		c.Toast(a.t("settings.saved"))
		// The masonry keeps its last layout until the next frame; ask for it
		// so a new column width shows at once.
		c.Invalidate()
	default:
	}
}

// saveSettings writes the settings off the main thread, so the view never
// blocks on the file.
func (a *App) saveSettings() {
	if a.settings == nil {
		return
	}
	path, err := settingsPath()
	if err != nil {
		a.settingsErr = a.tf("settings.pathFailed", err.Error())
		return
	}
	if a.settingsSaved == nil {
		a.settingsSaved = make(chan error, 1)
	}
	s := *a.settings
	// The R18 filter follows the switch that was just saved.
	a.client.SetAllowR18(s.ShowR18)
	dirLabel := a.t("settings.downloadDir")
	go func() {
		if dir := s.DownloadDir; dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				a.settingsSaved <- fmt.Errorf("%s: %w", dirLabel, err)
				return
			}
		}
		a.settingsSaved <- Save(path, &s)
	}()
}
