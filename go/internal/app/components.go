package app

import "github.com/egoist/mygo/ui"

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

// emptyState is the one shape every quiet page uses: a large glyph, a short
// title, a line of explanation and an optional action.
func emptyState(c *ui.Context, glyph *ui.SVG, title, hint string, action func()) {
	t := themeOf(c)
	ui.Column(c).Grow(1).Center().Padding(t.Space2XL).Children(func() {
		ui.Box(c).Size(88, 88).Radius(t.RadiusXL).Background(t.SurfaceAlt).Center().Children(func() {
			ui.Icon(c, glyph).Size(48, 48).TextColor(t.TextMuted)
		})
		ui.Text(c, title).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).
			TextAlign(ui.Center).Padding(t.SpaceL, 0, 0, 0)
		if hint != "" {
			ui.Text(c, hint).FontSize(t.TextSmall).TextColor(t.TextMuted).
				TextAlign(ui.Center).MaxWidth(380).Padding(t.SpaceS, 0, 0, 0)
		}
		if action != nil {
			ui.Row(c).Padding(t.SpaceL, 0, 0, 0).Children(action)
		}
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

// contentWidth is the width a page may use inside the navigation and the
// page's padding.
func contentWidth(c *ui.Context) float32 {
	winW, _ := c.Size()
	return max(winW-navWidth-2*contentPadding, 0)
}
