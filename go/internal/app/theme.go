package app

import "github.com/egoist/mygo/ui"

// The window's design tokens: one palette and one scale for every view. A view
// reads them with themeOf(c); applyTheme installs the same values into the
// widgets' ui.Theme, so stock controls and hand-built parts share one look.
const (
	navWidth       = 200
	contentPadding = 32
)

// tokens is one theme's palette, spacing, radii and type scale.
type tokens struct {
	Dark bool

	Bg             ui.Color
	Surface        ui.Color
	SurfaceAlt     ui.Color
	SurfaceHover   ui.Color
	SurfacePressed ui.Color
	Border         ui.Color
	BorderStrong   ui.Color
	Text           ui.Color
	TextMuted      ui.Color

	Accent        ui.Color
	AccentSoft    ui.Color
	AccentFill    ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color
	AccentText    ui.Color

	Danger      ui.Color
	Warning     ui.Color
	Success     ui.Color
	Selection   ui.Color
	Focus       ui.Color
	Scrollbar   ui.Color
	Shadow      ui.Color
	Inverse     ui.Color
	InverseText ui.Color

	// The viewer is a dark room in either theme, so its chrome has its own
	// tokens instead of following the window.
	ViewerBg        ui.Color
	ViewerBar       ui.Color
	ViewerBarBorder ui.Color
	ViewerBarHover  ui.Color
	ViewerBarText   ui.Color
	ViewerBarMuted  ui.Color

	SpaceXS  float32
	SpaceS   float32
	SpaceM   float32
	SpaceL   float32
	SpaceXL  float32
	Space2XL float32

	RadiusS    float32
	RadiusM    float32
	RadiusL    float32
	RadiusXL   float32
	RadiusFull float32

	TextDisplay float32
	TextTitle   float32
	TextBody    float32
	TextSmall   float32
	TextTiny    float32
}

// designTokens returns the dark or the light set. The dark set is the first
// one: the app is a picture browser, and the viewer is always dark.
func designTokens(dark bool) tokens {
	t := tokens{
		Dark: dark,

		SpaceXS: 4, SpaceS: 8, SpaceM: 12, SpaceL: 16, SpaceXL: 24, Space2XL: 32,
		RadiusS: 6, RadiusM: 10, RadiusL: 12, RadiusXL: 16, RadiusFull: 999,
		TextDisplay: 28, TextTitle: 20, TextBody: 15, TextSmall: 13, TextTiny: 11,

		ViewerBg:        ui.Hex("#0a0b0e"),
		ViewerBar:       ui.RGBA(22, 25, 32, 0.9),
		ViewerBarBorder: ui.RGBA(255, 255, 255, 0.1),
		ViewerBarHover:  ui.RGBA(255, 255, 255, 0.09),
		ViewerBarText:   ui.Hex("#f2f4f8"),
		ViewerBarMuted:  ui.Hex("#a6adbb"),
	}
	if dark {
		t.Bg = ui.Hex("#0e1014")
		t.Surface = ui.Hex("#161920")
		t.SurfaceAlt = ui.Hex("#1c2029")
		t.SurfaceHover = ui.Hex("#222733")
		t.SurfacePressed = ui.Hex("#2a3040")
		t.Border = ui.Hex("#272c36")
		t.BorderStrong = ui.Hex("#353c49")
		t.Text = ui.Hex("#eef1f6")
		t.TextMuted = ui.Hex("#98a1b2")

		t.Accent = ui.Hex("#7fb0ff")
		t.AccentSoft = ui.Hex("#1b2b46")
		t.AccentFill = ui.Hex("#2f6fed")
		t.AccentHover = ui.Hex("#3d7bfa")
		t.AccentPressed = ui.Hex("#255cc4")
		t.AccentText = ui.Hex("#ffffff")

		t.Danger = ui.Hex("#ff7b7b")
		t.Warning = ui.Hex("#f2b35b")
		t.Success = ui.Hex("#5ddc9a")
		t.Selection = ui.RGBA(127, 176, 255, 0.3)
		t.Focus = ui.RGBA(127, 176, 255, 0.75)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
		t.Shadow = ui.RGBA(0, 0, 0, 0.55)
		t.Inverse = ui.Hex("#f2f4f8")
		t.InverseText = ui.Hex("#0e1014")
		return t
	}

	t.Bg = ui.Hex("#f5f6f8")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceAlt = ui.Hex("#eef1f6")
	t.SurfaceHover = ui.Hex("#e7ebf2")
	t.SurfacePressed = ui.Hex("#dde3ed")
	t.Border = ui.Hex("#e1e5ec")
	t.BorderStrong = ui.Hex("#cbd2dd")
	t.Text = ui.Hex("#141821")
	t.TextMuted = ui.Hex("#5b6474")

	t.Accent = ui.Hex("#1a63d6")
	t.AccentSoft = ui.Hex("#e7efff")
	t.AccentFill = ui.Hex("#1f6feb")
	t.AccentHover = ui.Hex("#1a63d6")
	t.AccentPressed = ui.Hex("#155cc7")
	t.AccentText = ui.Hex("#ffffff")

	t.Danger = ui.Hex("#d92d20")
	t.Warning = ui.Hex("#a85f00")
	t.Success = ui.Hex("#0f7b4f")
	t.Selection = ui.RGBA(26, 99, 214, 0.22)
	t.Focus = ui.RGBA(26, 99, 214, 0.65)
	t.Scrollbar = ui.RGBA(20, 24, 33, 0.26)
	t.Shadow = ui.RGBA(20, 28, 45, 0.14)
	t.Inverse = ui.Hex("#141821")
	t.InverseText = ui.Hex("#ffffff")
	return t
}

// themeOf returns the tokens the current frame draws with.
func themeOf(c *ui.Context) tokens {
	return designTokens(c.Theme().Dark)
}

// designTheme is the tokens as the widgets' theme: controls take the alt
// surface so they stand out on a card, and the accent fill is the primary
// button's face.
func designTheme(dark bool) *ui.Theme {
	t := designTokens(dark)
	return &ui.Theme{
		Dark:           dark,
		Background:     t.Bg,
		Surface:        t.SurfaceAlt,
		SurfaceHover:   t.SurfaceHover,
		SurfacePressed: t.SurfacePressed,
		Border:         t.Border,
		Text:           t.Text,
		TextMuted:      t.TextMuted,
		Accent:         t.AccentFill,
		AccentHover:    t.AccentHover,
		AccentPressed:  t.AccentPressed,
		AccentText:     t.AccentText,
		Danger:         t.Danger,
		Warning:        t.Warning,
		Success:        t.Success,
		Selection:      t.Selection,
		Focus:          t.Focus,
		Inverse:        t.Inverse,
		InverseText:    t.InverseText,
		Scrollbar:      t.Scrollbar,
		ScrollbarWidth: 8,
		Radius:         t.RadiusM,
		Spacing:        4,
		FontSize:       15,
	}
}
