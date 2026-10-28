package app

import (
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
)

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
	return []string{a.t("settings.langSystem"), a.t("settings.langZh"), a.t("settings.langEn")}
}

// languageLabel is the label the settings page shows for a saved language.
func (a *App) languageLabel(lang string) string {
	switch lang {
	case i18n.LangZhHans:
		return a.t("settings.langZh")
	case i18n.LangEnUS:
		return a.t("settings.langEn")
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
		if a.settingsErr != "" {
			ui.Text(c, a.settingsErr).TextColor(t.Danger)
		}
	})
}

// applySavedSettings picks up a save that finished off the main thread: it
// applies the language, the download directory and concurrency, and shows the
// toast.
func (a *App) applySavedSettings(c *ui.Context) {
	select {
	case err := <-a.settingsSaved:
		if err != nil {
			a.settingsErr = a.tf("settings.saveFailed", err.Error())
			return
		}
		a.settingsErr = ""
		a.loadLanguage()
		a.setTitle()
		a.downloadsDir = settingsDownloadDir(a.settings)
		if a.downloads != nil {
			a.downloads.SetConcurrency(a.settings.DownloadConcurrency)
		}
		c.Toast(a.t("settings.saved"))
		// The masonry keeps its last layout until the next frame; ask for it
		// so a new column width or language shows at once.
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
