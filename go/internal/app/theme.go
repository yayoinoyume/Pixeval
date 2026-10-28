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
		t.AccentHover = ui.Hex("#2a63d8")
		t.AccentPressed = ui.Hex("#1f56b8")
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

var (
	darkTokens  = designTokens(true)
	lightTokens = designTokens(false)
)

// tokensFor returns the cached palette for dark or light. The colors are
// parsed once, not on every frame or every card.
func tokensFor(dark bool) tokens {
	if dark {
		return darkTokens
	}
	return lightTokens
}

// themeOf returns the tokens the current frame draws with, adjusted for the
// desktop's high contrast setting.
func themeOf(c *ui.Context) tokens {
	t := tokensFor(c.Theme().Dark)
	if s := textScale(c); s != 1 {
		t.TextDisplay *= s
		t.TextTitle *= s
		t.TextBody *= s
		t.TextSmall *= s
		t.TextTiny *= s
	}
	// A system accent replaces the design's blue in every custom part, so
	// the navigation, brand and buttons do not show two accents at once.
	// designTheme leaves our own fill in place for an explicit theme.
	if a := c.Theme().Accent; a.A != 0 && (a.R != t.AccentFill.R || a.G != t.AccentFill.G || a.B != t.AccentFill.B) {
		t.Accent = a
		t.AccentFill = a
		t.AccentHover = c.Theme().AccentHover
		t.AccentPressed = c.Theme().AccentPressed
		t.AccentText = c.Theme().AccentText
		t.AccentSoft = a.Alpha(0.16)
	}
	if c.Preferences().HighContrast {
		t.Border = t.BorderStrong
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.45)
		t.Focus = t.Accent
	}
	return t
}

// accentText is the text color on a filled accent, as mygo's own theme picks
// it: white unless the accent is light enough that near black stands out.
func accentText(a ui.Color) ui.Color {
	l := uint8(0.2126*float32(a.R) + 0.7152*float32(a.G) + 0.0722*float32(a.B) + 0.5)
	if l > 165 {
		return ui.RGB(24, 24, 27)
	}
	return ui.RGB(255, 255, 255)
}

// textScale is the desktop's text scale, 1 when it has none.
func textScale(c *ui.Context) float32 {
	if s := c.Preferences().TextScale; s > 0 {
		return s
	}
	return 1
}

// designTheme is the tokens as the widgets' theme: controls take the alt
// surface so they stand out on a card, and the accent fill is the primary
// button's face. FontSize follows the desktop's text scale.
func designTheme(dark bool, scale float32) *ui.Theme {
	t := tokensFor(dark)
	if scale <= 0 {
		scale = 1
	}
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
		FontSize:       t.TextBody * scale,
	}
}

// applyHighContrast strengthens a theme for the desktop's high contrast
// setting: darker borders and secondary text, and an opaque focus ring.
func applyHighContrast(theme *ui.Theme, dark bool) {
	t := tokensFor(dark)
	theme.Border = t.BorderStrong
	theme.TextMuted = t.TextMuted.Mix(t.Text, 0.45)
	theme.Focus = t.Accent
}
