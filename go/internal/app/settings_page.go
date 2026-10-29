package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
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

// settingsRow draws one preference: its name and explanation on the left,
// its control on the right.
type settingsRow func(label, hint string, control func())

// settingsPad is the horizontal inset of a settings card's title and rows.
const settingsPad float32 = 20

// blockedListShown is how many blocked users the settings card lists; the
// rest are counted by the card's count text only.
const blockedListShown = 10

// blockedTagListShown is how many blocked tags the settings card lists; the
// rest are counted by the card's count text only.
const blockedTagListShown = 10

// settingsPage draws the preferences as cards, so a setting is found by
// what it is about rather than by scanning one long column.
func (a *App) settingsPage(c *ui.Context) {
	s := a.settings
	if s == nil {
		return
	}
	t := themeOf(c)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Row(c).Padding(t.SpaceXL, contentPadding).Children(func() {
				ui.Column(c).Width(min(contentWidth(c), 900)).Gap(t.SpaceL).Children(func() {
					a.settingsGroup(c, a.t("settings.groupAppearance"), func(row settingsRow) {
						theme := a.themeLabel(s.Theme)
						row(a.t("settings.theme"), "", func() {
							if ui.Select(c, &theme, a.themeChoices()).Width(200).Height(36).Label(a.t("settings.theme")).Changed() {
								s.Theme = a.themeValue(theme)
							}
						})
						language := a.languageLabel(s.Language)
						row(a.t("settings.language"), "", func() {
							if ui.Select(c, &language, a.languageChoices()).Width(200).Height(36).Label(a.t("settings.language")).Changed() {
								s.Language = a.languageValue(language)
								// The theme applies on the frame it changes in; the language
								// applies at once too, like the rest of the page's controls.
								a.loadLanguage()
								c.Invalidate()
							}
						})
					})

					a.settingsGroup(c, a.t("settings.groupDownloads"), func(row settingsRow) {
						row(a.t("settings.downloadDir"), a.t("settings.downloadDirHint"), func() {
							ui.TextInput(c, &s.DownloadDir).Width(360).Height(36).Label(a.t("settings.downloadDir"))
						})
						row(a.t("settings.concurrency"), "", func() {
							a.numberStepper(c, &s.DownloadConcurrency, 1, 8, 1, a.t("settings.concurrency"))
						})
						row(a.t("settings.downloadMacro"), a.t("settings.downloadMacroHint"), func() {
							ui.TextInput(c, &s.DownloadMacro).Width(360).Height(36).
								Label(a.t("settings.downloadMacro"))
						})
					})

					a.settingsGroup(c, a.t("settings.groupNetwork"), func(row settingsRow) {
						row(a.t("settings.proxy"), a.t("settings.proxyHint"), func() {
							ui.TextInput(c, &s.ProxyURL).Width(360).Height(36).Label(a.t("settings.proxy"))
						})
					})

					a.settingsGroup(c, a.t("settings.groupCache"), func(row settingsRow) {
						row(a.t("settings.cacheLimit"), a.t("settings.cacheHint"), func() {
							a.numberStepper(c, &s.ImageCacheLimitMB, imageCacheLimitMin, imageCacheLimitMax, 64,
								a.t("settings.cacheLimit"))
						})
						row(a.t("settings.cacheSize"), "", func() {
							ui.Text(c, cacheSizeText(a.cacheUsageSnapshot())).FontSize(t.TextSmall).
								TextColor(t.TextMuted)
							if ui.Button(c, a.t("settings.cacheClear")).Clicked() {
								a.clearImageCache()
								c.Toast(a.t("settings.cacheCleared"))
								c.Invalidate()
							}
						})
					})

					a.settingsGroup(c, a.t("settings.groupContent"), func(row settingsRow) {
						row(a.t("settings.showR18"), a.t("settings.showR18Hint"), func() {
							switchControl(c, &s.ShowR18, a.t("settings.showR18"))
						})
						row(a.t("settings.cardWidth"), "", func() {
							a.numberStepper(c, &s.CardWidth, 180, 360, 10, a.t("settings.cardWidth"))
						})
					})

					a.settingsGroup(c, a.t("settings.groupAbout"), func(row settingsRow) {
						a.settingsToggleRow(c, a.t("settings.groupShortcuts"), &a.shortcutsOpen)
						if a.shortcutsOpen {
							ui.Text(c, a.t("shortcuts.hint")).FontSize(t.TextTiny).
								TextColor(t.TextMuted).Padding(0, settingsPad, t.SpaceM, settingsPad).MaxLines(3)
						}
						ui.Box(c).Height(1).FillWidth().Background(t.Border)
						row(a.t("settings.updates"), "", func() {
							st := a.updateSnapshot()
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
						a.updateStatus(c)
					})

					a.settingsGroup(c, a.t("settings.groupSubscriptions"), func(row settingsRow) {
						row(a.t("subs.autoDownload"), a.t("subs.autoDownloadHint"), func() {
							switchControl(c, &s.AutoDownloadSubscriptions, a.t("subs.autoDownload"))
						})
						row(a.t("subs.interval"), a.t("subs.intervalHint"), func() {
							a.numberStepper(c, &s.SubscriptionIntervalMinutes,
								subscriptionIntervalMin, subscriptionIntervalMax, 5, a.t("subs.interval"))
						})
					})

					a.settingsGroup(c, a.t("block.user"), func(row settingsRow) {
						users := a.blockedUsersSnapshot()
						row(a.tf("block.count", len(users)), "", func() {
							if ui.Button(c, a.t("block.clear")).Disabled(len(users) == 0).Clicked() {
								a.clearBlockedUsers()
							}
						})
						if len(users) == 0 {
							ui.Text(c, a.t("block.empty")).FontSize(t.TextTiny).TextColor(t.TextMuted).
								Padding(t.SpaceS, settingsPad, t.SpaceM, settingsPad)
							return
						}
						for _, u := range users[:min(len(users), blockedListShown)] {
							u := u
							name := u.Name
							if name == "" {
								name = fmt.Sprintf("#%d", u.ID)
							}
							row(name, "", func() {
								if ui.Button(c, a.t("block.remove")).Clicked() {
									a.unblockUser(u.ID)
								}
							})
						}
					})

					a.settingsGroup(c, a.t("block.tag"), func(row settingsRow) {
						tags := a.blockedTagsSnapshot()
						row(a.tf("block.tagCount", len(tags)), "", func() {
							if ui.Button(c, a.t("block.tagClear")).Disabled(len(tags) == 0).Clicked() {
								a.clearBlockedTags()
							}
						})
						if len(tags) == 0 {
							ui.Text(c, a.t("block.tagEmpty")).FontSize(t.TextTiny).TextColor(t.TextMuted).
								Padding(t.SpaceS, settingsPad, t.SpaceM, settingsPad)
							return
						}
						for _, tag := range tags[:min(len(tags), blockedTagListShown)] {
							tag := tag
							row(tag.Name, "", func() {
								if ui.Button(c, a.t("block.tagRemove")).Clicked() {
									a.unblockTag(tag.Name)
								}
							})
						}
					})

					a.settingsGroup(c, a.t("settings.groupData"), func(row settingsRow) {
						row("", "", func() {
							if ui.Button(c, a.t("settings.clearBrowseHistory")).Clicked() {
								a.clearHistory()
								c.Toast(a.t("settings.historyCleared"))
							}
						})
						row("", "", func() {
							if ui.Button(c, a.t("settings.clearDownloadHistory")).Clicked() {
								// It leaves its own notice, which the next frame toasts.
								a.clearDownloadHistory()
							}
						})
						row("", "", func() {
							if ui.Button(c, a.t("settings.clearSearchHistory")).Clicked() {
								a.clearSearchHistory()
								c.Toast(a.t("settings.historyCleared"))
							}
						})
						row("", "", func() {
							if ui.Button(c, a.t("settings.clearNovelProgress")).Clicked() {
								a.clearNovelProgress()
								c.Toast(a.t("settings.historyCleared"))
							}
						})
						ui.Box(c).Height(1).FillWidth().Background(t.Border)
						row(a.t("settings.resetHint"), "", func() {
							if ui.Button(c, a.t("settings.resetSettings")).Clicked() {
								a.resetSettings()
							}
						})
					})
				})
			})
		})
		a.settingsFooter(c)
	})
}

// settingsFooter keeps the save action in reach however far the page is
// scrolled.
func (a *App) settingsFooter(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).Gap(t.SpaceL).AlignItems(ui.Center).Padding(t.SpaceM, contentPadding).
		Background(t.Bg).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		if ui.PrimaryButton(c, a.t("common.save")).Clicked() {
			a.saveSettings()
		}
		if a.settingsErr != "" {
			ui.Text(c, a.settingsErr).FontSize(t.TextSmall).TextColor(t.Danger).
				Grow(1).MaxLines(2)
		}
	})
}

// settingsGroup is a titled card of preference rows.
func (a *App) settingsGroup(c *ui.Context, title string, body func(row settingsRow)) {
	t := themeOf(c)
	ui.Column(c).FillWidth().Background(t.Surface).Radius(t.RadiusL).
		Border(1, t.Border).Clip().Children(func() {
		ui.Text(c, title).FontSize(t.TextSmall).TextColor(t.TextMuted).
			Padding(t.SpaceM, settingsPad, t.SpaceXS, settingsPad)
		first := true
		row := func(label, hint string, control func()) {
			if !first {
				ui.Box(c).Height(1).FillWidth().Background(t.Border)
			}
			first = false
			a.settingsRow(c, label, hint, control)
		}
		body(row)
	})
}

// settingsRow places the control at the end of a row at least 44 high. A
// long hint may take two lines and grow the row.
func (a *App) settingsRow(c *ui.Context, label, hint string, control func()) {
	t := themeOf(c)
	ui.Row(c).FillWidth().MinHeight(44).Padding(0, settingsPad).Gap(t.SpaceXL).
		AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(100).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(t.TextBody).TextColor(t.Text)
			if hint != "" {
				ui.Text(c, hint).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(2)
			}
		})
		ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Shrink(0).Children(control)
	})
}

// numberStepper is a labelled number field with square minus and plus
// buttons. The label is on the text field itself, so the focus announces the
// setting instead of an anonymous role group.
func (a *App) numberStepper[T int | float64](c *ui.Context, value *T, lo, hi, step T, label string) {
	text := ui.Local(c.Root(), "stepper."+label, func() string {
		return strconv.FormatFloat(float64(*value), 'f', -1, 64)
	})
	field := ui.TextInput(c, text).Width(72).Height(36).Label(label)
	focused := ui.Local(c.Root(), "stepper.focus."+label, func() bool { return false })
	if field.Changed() {
		if n, err := strconv.ParseFloat(strings.TrimSpace(*text), 64); err == nil {
			*value = T(min(max(n, float64(lo)), float64(hi)))
		}
	}
	if field.Submitted() || *focused && !field.Focused() {
		*text = strconv.FormatFloat(float64(*value), 'f', -1, 64)
	}
	*focused = field.Focused()
	dec := a.t("settings.decrease")
	inc := a.t("settings.increase")
	if a.stepperButton(c, iconMinus, label+" "+dec).Clicked() {
		*value = T(max(float64(*value)-float64(step), float64(lo)))
		*text = strconv.FormatFloat(float64(*value), 'f', -1, 64)
	}
	if a.stepperButton(c, iconPlus, label+" "+inc).Clicked() {
		*value = T(min(float64(*value)+float64(step), float64(hi)))
		*text = strconv.FormatFloat(float64(*value), 'f', -1, 64)
	}
}

// stepperButton is one square button of a number stepper.
func (a *App) stepperButton(c *ui.Context, glyph *ui.SVG, label string) ui.Element {
	t := themeOf(c)
	b := ui.ButtonBase(c).Size(36, 36).Radius(t.RadiusS).Label(label).Tooltip(label).
		Background(t.SurfaceAlt)
	if b.Hovered() {
		b = b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, glyph).Size(16, 16).TextColor(t.TextMuted)
	})
	return b
}

// settingsToggleRow is a whole-row disclosure control: its label, and a
// chevron that turns as the hint below opens.
func (a *App) settingsToggleRow(c *ui.Context, label string, open *bool) {
	t := themeOf(c)
	b := ui.ButtonBase(c).Height(44).FillWidth().Padding(0, settingsPad).Gap(t.SpaceS).
		AlignItems(ui.Center).Justify(ui.Start)
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(t.TextBody).TextColor(t.Text).Grow(1)
		deg := float32(90)
		if *open {
			deg = 270
		}
		ui.Icon(c, iconRight).Size(16, 16).TextColor(t.TextMuted).Rotate(deg)
	})
	if b.Clicked() {
		*open = !*open
	}
}

// switchControl is a 40 by 24 switch, the size of a settings row rather than
// the smaller default control.
func switchControl(c *ui.Context, on *bool, label string) {
	t := themeOf(c)
	b := ui.SwitchBase(c, on).Size(40, 24).Radius(t.RadiusFull).Label(label)
	b.Children(func() {
		track := t.BorderStrong
		if *on {
			track = t.AccentFill
		}
		ui.Box(c).Fill().Radius(t.RadiusFull).Background(track)
		knob := ui.Box(c).Size(20, 20).Radius(t.RadiusFull).Background(t.AccentText).
			Absolute().Top(2)
		if *on {
			knob = knob.Right(2)
		} else {
			knob = knob.Left(2)
		}
	})
}

// cacheUsageSnapshot is the cache size the page shows, measured at most once
// a second: a redraw every frame would otherwise scan the cache directory
// every frame. Only the main thread reads and writes it.
func (a *App) cacheUsageSnapshot() int64 {
	if time.Since(a.cacheUsageAt) > time.Second {
		if a.images != nil {
			a.cacheUsage = a.images.DiskUsage()
		}
		a.cacheUsageAt = time.Now()
	}
	return a.cacheUsage
}

// clearImageCache deletes the cached image bytes and makes the next frame
// measure what is left.
func (a *App) clearImageCache() {
	if a.images != nil {
		a.images.ClearDisk()
	}
	a.cacheUsageAt = time.Time{}
}

// cacheSizeText is a cache size in the unit a person reads.
func cacheSizeText(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%d B", bytes)
}

// updateStatus is the line under the update row: what the last check found,
// or why it cannot run.
func (a *App) updateStatus(c *ui.Context) {
	t := themeOf(c)
	st := a.updateSnapshot()
	text := ""
	color := t.TextMuted
	switch {
	case st.off:
		text = a.t("settings.updatesOff")
	case st.err != "":
		text, color = st.err, t.Danger
	case st.installing:
		text = a.t("settings.updateInstalling")
	case st.version != "":
		text, color = a.tf("settings.updateAvailable", st.version), t.Text
	case st.latest:
		text = a.t("settings.updateLatest")
	}
	if text == "" {
		return
	}
	ui.Text(c, text).FontSize(t.TextSmall).TextColor(color).
		Padding(0, settingsPad, t.SpaceM, settingsPad).MaxLines(2)
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
			a.settingsResetPending = false
			a.settingsErr = a.tf("settings.saveFailed", err.Error())
			return
		}
		a.settingsErr = ""
		a.downloadsDir = settingsDownloadDir(a.settings)
		if a.downloads != nil {
			a.downloads.SetConcurrency(a.settings.DownloadConcurrency)
		}
		// A smaller limit applies to the files already cached, so the save may
		// delete some, and the page measures the cache again.
		if a.images != nil {
			a.images.SetDiskLimit(imageCacheLimitBytes(a.settings))
			a.cacheUsageAt = time.Time{}
		}
		if err := a.client.SetProxy(a.settings.ProxyURL); err != nil {
			a.settingsErr = a.t("settings.proxyInvalid")
			return
		}
		a.restartSubscriptions()
		msg := a.t("settings.saved")
		if a.settingsResetPending {
			a.settingsResetPending = false
			msg = a.t("settings.resetDone")
		}
		c.Toast(msg)
		// The masonry keeps its last layout until the next frame; ask for it
		// so a new column width shows at once.
		c.Invalidate()
	default:
	}
}

// resetSettings puts every preference back to its default and saves it. The
// accounts, the download queue and the record files are other stores, so a
// reset leaves them alone.
func (a *App) resetSettings() {
	if a.settings == nil {
		return
	}
	*a.settings = *Default()
	// A reset language applies at once, like the language select.
	a.loadLanguage()
	a.settingsResetPending = true
	a.saveSettings()
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
	// A template that does not parse would name no file, so the save keeps the
	// last good one and says why.
	if strings.TrimSpace(a.settings.DownloadMacro) == "" {
		a.settings.DownloadMacro = download.DefaultMacro
	}
	if _, err := download.Parse(a.settings.DownloadMacro); err != nil {
		a.settingsErr = a.t("settings.downloadMacroInvalid")
		return
	}
	// A proxy the transport cannot dial is reported and nothing is saved, so
	// the running client keeps the last working one.
	if !validProxyURL(a.settings.ProxyURL) {
		a.settingsErr = a.t("settings.proxyInvalid")
		return
	}
	a.settings.ProxyURL = strings.TrimSpace(a.settings.ProxyURL)
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
