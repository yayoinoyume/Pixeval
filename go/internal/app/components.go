package app

import (
	"runtime/debug"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// iconButton is a square, icon-only button: the glyph carries the action and
// the label names it for the keyboard, the pointer's tooltip and assistive
// technology.
func iconButton(c *ui.Context, glyph *ui.SVG, label string, size float32, fg, hover ui.Color) ui.Element {
	t := themeOf(c)
	b := ui.ButtonBase(c).Size(size, size).Radius(t.RadiusS).Label(label).Tooltip(label)
	if b.Hovered() {
		b = b.Background(hover)
	}
	b.Children(func() {
		ui.Icon(c, glyph).Size(size*0.5, size*0.5).TextColor(fg)
	})
	return b
}

// cardShell is the frame shared by the work, novel and history cards: a
// surface card that lifts its border and shadow when the pointer is over it.
// It returns the card and its cover box; the caller adds the cover's contents,
// the text block, and the card's label and click.
func cardShell(c *ui.Context) (ui.Element, *ui.Element) {
	t := themeOf(c)
	card := ui.Column(c).Background(t.Surface).Radius(t.RadiusL).
		Border(1, t.Border).Clip()
	if card.Hovered() {
		card = card.Border(1, t.Accent).Shadow(0, 8, 24, 0, t.Shadow)
	}
	var cover ui.Element
	card.Children(func() {
		cover = ui.Box(c).Grow(1).FillWidth().Background(t.SurfaceAlt)
	})
	return card, &cover
}

// emptyState is the one shape every quiet page uses: a centered card with a
// large glyph, a short title, a line of explanation and an optional action.
func emptyState(c *ui.Context, glyph *ui.SVG, title, hint string, action func()) {
	t := themeOf(c)
	ui.Column(c).Grow(1).Center().Padding(t.Space2XL).Children(func() {
		ui.Column(c).Width(460).MaxWidthPercent(100).Padding(t.Space2XL).AlignItems(ui.Center).
			Background(t.Surface).Radius(t.RadiusXL).Border(1, t.Border).Children(func() {
			ui.Icon(c, glyph).Size(48, 48).TextColor(t.TextMuted)
			ui.Text(c, title).FontSize(t.TextDisplay).Bold().TextColor(t.Text).
				TextAlign(ui.Center).Padding(t.SpaceL, 0, 0, 0)
			if hint != "" {
				ui.Text(c, hint).FontSize(t.TextSmall).TextColor(t.TextMuted).
					TextAlign(ui.Center).MaxWidth(360).Padding(t.SpaceS, 0, 0, 0)
			}
			if action != nil {
				ui.Row(c).Padding(t.SpaceXL, 0, 0, 0).Children(action)
			}
		})
	})
}

// skeletonCard is one placeholder tile of a loading feed: a cover and the two
// lines of text under it.
func skeletonCard(c *ui.Context, width float32) {
	t := themeOf(c)
	ui.Column(c).Width(width).Gap(t.SpaceS).Children(func() {
		ui.Box(c).FillWidth().Height(width * 4 / 3).Radius(t.RadiusL).Background(t.SurfaceAlt)
		ui.Box(c).FillWidth().Height(12).Radius(t.RadiusS).Background(t.SurfaceAlt)
		ui.Box(c).Width(width * 0.45).Height(10).Radius(t.RadiusS).Background(t.SurfaceAlt)
	})
}

// skeletonFeed stands in for the first page of a feed. It is still, so it
// reads as a layout taking shape rather than as motion.
func skeletonFeed(c *ui.Context, card float64) {
	gap := float32(cardGap)
	winW, _ := c.Size()
	avail := winW - navWidth - 2*contentPadding
	cols := 1
	if card > 0 {
		cols = min(max(int((avail+gap)/(float32(card)+gap)), 1), 6)
	}
	ui.Column(c).Padding(contentPadding).Gap(gap).Children(func() {
		for row := 0; row < 2; row++ {
			ui.Row(c).Gap(gap).Children(func() {
				for i := 0; i < cols; i++ {
					skeletonCard(c, float32(card))
				}
			})
		}
	})
}

// initialOf is the letter an avatar shows for a name.
func initialOf(name string) string {
	for _, r := range name {
		if r != ' ' {
			return string(r)
		}
	}
	return "?"
}

// appVersion is the version the brand shows: the packaged app's, the module
// version of a source build, or "dev" for a plain go build.
func appVersion() string {
	v := mygo.App.Version()
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		}
	}
	if v == "" || v == "(devel)" {
		return "dev"
	}
	if v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

// contentWidth is the width a page may use inside the navigation and the
// page's padding.
func contentWidth(c *ui.Context) float32 {
	winW, _ := c.Size()
	return max(winW-navWidth-2*contentPadding, 0)
}
