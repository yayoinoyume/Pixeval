package app

import (
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
)

// themeChoices are the theme labels the settings page shows.
var themeChoices = []string{"跟随系统", "浅色", "深色"}

// themeLabel is the label the settings page shows for a saved theme.
func themeLabel(theme string) string {
	switch theme {
	case themeLight:
		return "浅色"
	case themeDark:
		return "深色"
	default:
		return "跟随系统"
	}
}

// themeValue is the theme a settings page label stands for.
func themeValue(label string) string {
	switch label {
	case "浅色":
		return themeLight
	case "深色":
		return themeDark
	default:
		return themeSystem
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
			theme := themeLabel(s.Theme)
			ui.Field(c, "主题", func() {
				if ui.Select(c, &theme, themeChoices).Changed() {
					s.Theme = themeValue(theme)
				}
			})
			ui.Field(c, "下载目录", func() {
				ui.TextInput(c, &s.DownloadDir).Width(360)
			}).Description("留空表示默认目录，不存在时自动创建")
			concurrency := float64(s.DownloadConcurrency)
			ui.Field(c, "下载并发", func() {
				ui.NumberInput(c, &concurrency, 1, 8, 1)
			})
			s.DownloadConcurrency = int(concurrency)
			ui.Field(c, "显示 R18", func() {
				ui.Switch(c, &s.ShowR18)
			}).Description("R18 过滤将在 M2 生效")
			width := s.CardWidth
			ui.Field(c, "瀑布流列宽", func() {
				ui.NumberInput(c, &width, 180, 360, 10)
			})
			s.CardWidth = width
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "保存").Clicked() {
				a.saveSettings()
			}
		})
		if a.settingsErr != "" {
			ui.Text(c, a.settingsErr).TextColor(t.Danger)
		}
	})
}

// applySavedSettings picks up a save that finished off the main thread: it
// applies the download directory and concurrency, and shows the toast.
func (a *App) applySavedSettings(c *ui.Context) {
	select {
	case err := <-a.settingsSaved:
		if err != nil {
			a.settingsErr = "保存失败：" + err.Error()
			return
		}
		a.settingsErr = ""
		a.downloadsDir = settingsDownloadDir(a.settings)
		if a.downloads != nil {
			a.downloads.SetConcurrency(a.settings.DownloadConcurrency)
		}
		c.Toast("已保存")
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
		a.settingsErr = "无法保存设置：" + err.Error()
		return
	}
	if a.settingsSaved == nil {
		a.settingsSaved = make(chan error, 1)
	}
	s := *a.settings
	go func() {
		if dir := s.DownloadDir; dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				a.settingsSaved <- fmt.Errorf("下载目录：%w", err)
				return
			}
		}
		a.settingsSaved <- Save(path, &s)
	}()
}
