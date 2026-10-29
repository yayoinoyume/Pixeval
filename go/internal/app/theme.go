package app

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/artcolor"
)

// The window's design tokens: one palette and one scale for every view. A view
// reads them with themeOf(c); applyTheme installs the same values into the
// widgets' ui.Theme, so stock controls and hand-built parts share one look.
//
// The direction is an editorial gallery, not Material You: warm paper and ink,
// hairlines instead of floating cards, two radii for pictures and four for
// controls, and a serif for the few display lines. The artwork supplies the
// color on its own page; the rest of the app stays quiet.
const (
	navWidth       = 80
	contentPadding = 40
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

	// The old Material role names remain as aliases for the pages that were
	// written against them. New code should use the role it means above.
	Primary                 ui.Color
	OnPrimary               ui.Color
	PrimaryContainer        ui.Color
	OnPrimaryContainer      ui.Color
	Secondary               ui.Color
	OnSecondary             ui.Color
	SecondaryContainer      ui.Color
	OnSecondaryContainer    ui.Color
	Tertiary                ui.Color
	OnTertiary              ui.Color
	TertiaryContainer       ui.Color
	OnTertiaryContainer     ui.Color
	SurfaceContainerLow     ui.Color
	SurfaceContainer        ui.Color
	SurfaceContainerHigh    ui.Color
	SurfaceContainerHighest ui.Color
	OutlineVariant          ui.Color
	InverseSurface          ui.Color
	InverseOnSurface        ui.Color

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

	// WashTop and WashBottom are the two ends of the work page's room: the
	// artwork's hue at a low chroma over paper or ink.
	WashTop    ui.Color
	WashBottom ui.Color

	// The viewer's chrome follows the window, but keeps its own names so a
	// work page can tint it from the artwork without changing the shell.
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

	RadiusXS   float32
	RadiusS    float32
	RadiusM    float32
	RadiusL    float32
	RadiusXL   float32
	Radius2XL  float32
	RadiusFull float32

	TextMicro   float32
	TextDetail  float32
	TextDisplay float32
	TextTitle   float32
	TextBody    float32
	TextSmall   float32
	TextTiny    float32
}

// designTokens returns the dark or the light set. These are quiet, warm
// neutrals; the accent is terracotta, and a work page replaces it with the
// artwork's own color.
func designTokens(dark bool) tokens {
	t := tokens{
		Dark: dark,

		SpaceXS: 4, SpaceS: 8, SpaceM: 12, SpaceL: 16, SpaceXL: 24, Space2XL: 32,
		// Two radii are enough for pictures and rules (2), four for controls
		// (4), eight for a panel that truly needs an edge. Nothing else is
		// rounded; avatars alone are circles.
		RadiusXS: 2, RadiusS: 4, RadiusM: 6, RadiusL: 8, RadiusXL: 8, Radius2XL: 8, RadiusFull: 999,
		TextMicro: 10, TextDetail: 36, TextDisplay: 30, TextTitle: 22, TextBody: 15, TextSmall: 13, TextTiny: 11,
	}
	if dark {
		t.Bg = ui.Hex("#12100e")
		t.Surface = ui.Hex("#1a1714")
		t.SurfaceAlt = ui.Hex("#211d19")
		t.SurfaceHover = ui.Hex("#2a2520")
		t.SurfacePressed = ui.Hex("#342e28")
		t.Border = ui.Hex("#332c25")
		t.BorderStrong = ui.Hex("#5c5146")
		t.Text = ui.Hex("#f3ede4")
		t.TextMuted = ui.Hex("#a89c8d")

		t.Accent = ui.Hex("#e2a17f")
		t.AccentSoft = ui.Hex("#38241a")
		t.AccentFill = ui.Hex("#c97a52")
		t.AccentHover = ui.Hex("#da8a61")
		t.AccentPressed = ui.Hex("#b56a46")
		t.AccentText = ui.Hex("#21130c")

		t.Danger = ui.Hex("#f2b8b5")
		t.Warning = ui.Hex("#e0b36a")
		t.Success = ui.Hex("#8fd3b0")
		t.Selection = ui.RGBA(201, 122, 82, 0.26)
		t.Focus = ui.RGBA(226, 161, 127, 0.75)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
		t.Shadow = ui.RGBA(0, 0, 0, 0.55)
		t.Inverse = ui.Hex("#f3ede4")
		t.InverseText = ui.Hex("#12100e")

		t.WashTop = ui.Hex("#171411")
		t.WashBottom = ui.Hex("#0b0908")
	} else {
		t.Bg = ui.Hex("#f7f3ed")
		t.Surface = ui.Hex("#fffdfa")
		t.SurfaceAlt = ui.Hex("#f0eae1")
		t.SurfaceHover = ui.Hex("#e8e1d6")
		t.SurfacePressed = ui.Hex("#dfd7ca")
		t.Border = ui.Hex("#ded6ca")
		t.BorderStrong = ui.Hex("#b8aea0")
		t.Text = ui.Hex("#191613")
		t.TextMuted = ui.Hex("#675f54")

		t.Accent = ui.Hex("#8c4a2f")
		t.AccentSoft = ui.Hex("#f1e2d6")
		t.AccentFill = ui.Hex("#7c3e27")
		t.AccentHover = ui.Hex("#68331f")
		t.AccentPressed = ui.Hex("#542818")
		t.AccentText = ui.Hex("#fff8f2")

		t.Danger = ui.Hex("#b42318")
		t.Warning = ui.Hex("#8a5a00")
		t.Success = ui.Hex("#2f6b4f")
		t.Selection = ui.RGBA(124, 62, 39, 0.18)
		t.Focus = ui.RGBA(124, 62, 39, 0.65)
		t.Scrollbar = ui.RGBA(25, 22, 19, 0.24)
		t.Shadow = ui.RGBA(40, 30, 20, 0.12)
		t.Inverse = ui.Hex("#191613")
		t.InverseText = ui.Hex("#f7f3ed")

		t.WashTop = ui.Hex("#f7f3ed")
		t.WashBottom = ui.Hex("#efe7dc")
	}
	t.ViewerBg = t.Bg
	t.ViewerBar = t.Surface
	t.ViewerBarBorder = t.Border
	t.ViewerBarHover = t.SurfaceHover
	t.ViewerBarText = t.Text
	t.ViewerBarMuted = t.TextMuted

	// The aliases keep the pages that still speak Material's role names in
	// the same palette. They are the same colors, not a second theme.
	t.Primary = t.Accent
	t.OnPrimary = t.AccentText
	t.PrimaryContainer = t.AccentSoft
	t.OnPrimaryContainer = t.Text
	t.Secondary = t.TextMuted
	t.OnSecondary = t.Bg
	t.SecondaryContainer = t.SurfaceAlt
	t.OnSecondaryContainer = t.Text
	t.Tertiary = t.Accent
	t.OnTertiary = t.AccentText
	t.TertiaryContainer = t.AccentSoft
	t.OnTertiaryContainer = t.Text
	t.SurfaceContainerLow = t.Surface
	t.SurfaceContainer = t.SurfaceAlt
	t.SurfaceContainerHigh = t.SurfaceHover
	t.SurfaceContainerHighest = t.SurfacePressed
	t.OutlineVariant = t.Border
	t.InverseSurface = t.Inverse
	t.InverseOnSurface = t.InverseText
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

// withPalette overrides the interface colors with one artwork's palette. The
// scale, radii and type stay the window's.
func (t tokens) withPalette(p artcolor.Palette) tokens {
	t.Bg = p.Bg
	t.Surface = p.Surface
	t.SurfaceAlt = p.SurfaceAlt
	t.SurfaceHover = p.SurfaceHover
	t.SurfacePressed = p.SurfacePressed
	t.Border = p.Border
	t.BorderStrong = p.BorderStrong
	t.Text = p.Text
	t.TextMuted = p.TextMuted

	t.Accent = p.Accent
	t.AccentSoft = p.AccentSoft
	t.AccentFill = p.Accent
	t.AccentHover = p.AccentHover
	t.AccentPressed = p.AccentPressed
	t.AccentText = p.AccentText

	t.WashTop = p.WashTop
	t.WashBottom = p.WashBottom
	t.Shadow = p.Shadow

	t.ViewerBg = p.WashBottom
	t.ViewerBar = p.Surface.Alpha(0.92)
	t.ViewerBarBorder = p.Border
	t.ViewerBarHover = p.SurfaceHover
	t.ViewerBarText = p.Text
	t.ViewerBarMuted = p.TextMuted

	t.Primary = t.Accent
	t.OnPrimary = t.AccentText
	t.PrimaryContainer = t.AccentSoft
	t.OnPrimaryContainer = t.Text
	t.Secondary = t.TextMuted
	t.OnSecondary = t.Bg
	t.SecondaryContainer = t.SurfaceAlt
	t.OnSecondaryContainer = t.Text
	t.Tertiary = t.Accent
	t.OnTertiary = t.AccentText
	t.TertiaryContainer = t.AccentSoft
	t.OnTertiaryContainer = t.Text
	t.SurfaceContainerLow = t.Surface
	t.SurfaceContainer = t.SurfaceAlt
	t.SurfaceContainerHigh = t.SurfaceHover
	t.SurfaceContainerHighest = t.SurfacePressed
	t.OutlineVariant = t.Border
	t.InverseSurface = t.Inverse
	t.InverseOnSurface = t.InverseText
	return t
}

// scaled returns the tokens with their text sizes at the desktop's scale.
func (t tokens) scaled(scale float32) tokens {
	if scale <= 0 || scale == 1 {
		return t
	}
	t.TextMicro *= scale
	t.TextDetail *= scale
	t.TextDisplay *= scale
	t.TextTitle *= scale
	t.TextBody *= scale
	t.TextSmall *= scale
	t.TextTiny *= scale
	return t
}

// highContrast strengthens a set of tokens for the desktop's high contrast
// setting: stronger rules, darker secondary text and an opaque focus ring.
func (t tokens) highContrast() tokens {
	t.Border = t.BorderStrong
	t.TextMuted = t.TextMuted.Mix(t.Text, 0.45)
	t.Focus = t.Accent
	return t
}

// themeOf returns the tokens the current frame draws with, adjusted for the
// desktop's high contrast setting.
func themeOf(c *ui.Context) tokens {
	t := tokensFor(c.Theme().Dark).scaled(textScale(c))
	// A system accent replaces the design's terracotta in every custom part,
	// so the navigation, brand and buttons do not show two accents at once.
	// designTheme leaves our own fill in place for an explicit theme.
	if a := c.Theme().Accent; a.A != 0 && (a.R != t.AccentFill.R || a.G != t.AccentFill.G || a.B != t.AccentFill.B) {
		t.Accent = readableAccent(a, t.Text, t.Surface)
		t.AccentFill = a
		t.AccentHover = c.Theme().AccentHover
		t.AccentPressed = c.Theme().AccentPressed
		t.AccentText = c.Theme().AccentText
		t.AccentSoft = a.Alpha(0.16)
	}
	if c.Preferences().HighContrast {
		t = t.highContrast()
	}
	return t
}

// readableAccent is the accent for text and icons: the accent itself when it
// has enough contrast on the surface, otherwise mixed toward the theme's text
// until it does. Filled accents keep the raw color.
func readableAccent(accent, text, bg ui.Color) ui.Color {
	if contrastRatio(accent, bg) >= 4.5 {
		return accent
	}
	for i := 1; i <= 20; i++ {
		c := accent.Mix(text, float32(i)/20)
		if contrastRatio(c, bg) >= 4.5 {
			return c
		}
	}
	return text
}

// contrastRatio is WCAG's contrast between two opaque colors.
func contrastRatio(a, b ui.Color) float32 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// luminance is a color's relative luminance.
func luminance(c ui.Color) float32 {
	f := func(v uint8) float32 {
		x := float32(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return float32(math.Pow(float64((x+0.055)/1.055), 2.4))
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
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
	return themeFromTokens(tokensFor(dark), scale)
}

// themeFromTokens makes the stock widgets follow a token set, including a
// work page's artwork palette.
func themeFromTokens(t tokens, scale float32) *ui.Theme {
	if scale <= 0 {
		scale = 1
	}
	return &ui.Theme{
		Dark:           t.Dark,
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
		Radius:         t.RadiusS,
		Spacing:        4,
		FontSize:       t.TextBody * scale,
		Font:           uiFont(),
	}
}

// applyHighContrast strengthens a theme for the desktop's high contrast
// setting: darker borders and secondary text, and an opaque focus ring.
func applyHighContrast(theme *ui.Theme, dark bool) {
	t := tokensFor(dark).highContrast()
	theme.Border = t.Border
	theme.TextMuted = t.TextMuted
	theme.Focus = t.Focus
}
