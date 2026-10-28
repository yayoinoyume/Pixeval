// Command capture renders the app's pages through the headless tester and
// writes them as PNGs. A development aid: it needs no display.
package main

import (
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/yayoinoyume/pixeval-go/internal/app"
)

func main() {
	// A capture is a preview, not the user's session: keep its settings out of
	// the real config so the theme and the paths are the defaults.
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		if dir, err := os.MkdirTemp("", "pixeval-capture-"); err == nil {
			os.Setenv("XDG_CONFIG_HOME", dir)
			defer os.RemoveAll(dir)
		}
	}
	out := os.Getenv("OUT")
	if out == "" {
		out = "/tmp/shots"
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	a := app.New()
	theme := ui.DarkTheme
	if os.Getenv("THEME") == "light" {
		theme = ui.LightTheme
	}
	view := func(c *ui.Context) {
		c.SetTheme(theme())
		a.View(c)
	}
	tt := ui.NewTester(view, 1280, 820)
	pages := []struct{ name, button string }{
		{"01-home", ""},
		{"02-search", "搜索"},
		{"03-subs", "追更"},
		{"04-downloads", "下载"},
		{"05-history", "历史"},
		{"06-settings", "设置"},
	}
	for _, p := range pages {
		if p.button != "" {
			if err := tt.Click(p.button); err != nil {
				log.Printf("%s: click %q: %v", p.name, p.button, err)
			}
		}
		tt.Frame()
		f, err := os.Create(filepath.Join(out, p.name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			log.Printf("%s: encode: %v", p.name, err)
		}
		f.Close()
		log.Println("wrote", p.name, "texts:", tt.Texts())
	}
}
