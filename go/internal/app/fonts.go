package app

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// The display face is the one place the app leaves the system's UI font: page
// titles and work titles are serif, so the gallery reads as editorial rather
// than as a control panel. Body copy stays in the system sans, which keeps
// Chinese, Japanese and Cyrillic consistent with the rest of the desktop.
var (
	displayFamily = sync.OnceValue(func() string {
		return firstFont(
			"Noto Serif CJK SC", "Source Han Serif SC", "Songti SC",
			"Noto Serif CJK JP", "Noto Serif JP", "Georgia",
			"DejaVu Serif", "serif",
		)
	})
	interfaceFamily = sync.OnceValue(func() string {
		// The empty string asks for the system font, which has the best
		// coverage for the four locales the app ships.
		return ""
	})
)

func displayFont() string { return displayFamily() }

func uiFont() string { return interfaceFamily() }

func firstFont(names ...string) string {
	families := ui.FontFamilies()
	for _, want := range names {
		for _, f := range families {
			if strings.EqualFold(f.Name, want) {
				return f.Name
			}
		}
	}
	return ""
}

// displayText starts a text element with the serif display face. An empty
// family is the system's, which is also the fallback when the preferred serif
// is not installed.
func displayText(c *ui.Context, s string) ui.Element {
	e := ui.Text(c, s)
	if f := displayFont(); f != "" {
		e = e.Font(f)
	}
	return e
}

// withDisplayFont gives an already-built text element the display face.
func withDisplayFont(e ui.Element) ui.Element {
	if f := displayFont(); f != "" {
		return e.Font(f)
	}
	return e
}
