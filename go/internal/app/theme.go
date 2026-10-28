package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// The window's design tokens: one palette and one scale for every view. A view
// reads them with themeOf(c); applyTheme installs the same values into the
// widgets' ui.Theme, so stock controls and hand-built parts share one look.
const (
	navWidth       = 88
	contentPadding = 28
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

	// The Material You roles a view may name instead of one of the older
	// aliases above: Bg is the surface, Surface the container-low role, and
	// so on, so a new view can say which role it means.
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
	Radius2XL  float32
	RadiusFull float32

	TextDetail  float32
	TextDisplay float32
	TextTitle   float32
	TextBody    float32
	TextSmall   float32
	TextTiny    float32
}

// designTokens returns the dark or the light set. Both are Material You
// roles from one seed, #9b7bb8: the dark set is the light one's roles in the
// dark scheme, not a separate palette.
func designTokens(dark bool) tokens {
	t := tokens{
		Dark: dark,

		SpaceXS: 4, SpaceS: 8, SpaceM: 12, SpaceL: 16, SpaceXL: 24, Space2XL: 32,
		RadiusS: 8, RadiusM: 12, RadiusL: 16, RadiusXL: 24, Radius2XL: 32, RadiusFull: 999,
		TextDetail: 36, TextDisplay: 30, TextTitle: 20, TextBody: 14, TextSmall: 13, TextTiny: 11,

		ViewerBg:        ui.Hex("#0a0b0e"),
		ViewerBar:       ui.RGBA(22, 25, 32, 0.9),
		ViewerBarBorder: ui.RGBA(255, 255, 255, 0.1),
		ViewerBarHover:  ui.RGBA(255, 255, 255, 0.09),
		ViewerBarText:   ui.Hex("#f2f4f8"),
		ViewerBarMuted:  ui.Hex("#a6adbb"),
	}
	if dark {
		t.Bg = ui.Hex("#151217")             // surface
		t.Surface = ui.Hex("#1d1a20")        // surface-container-low
		t.SurfaceAlt = ui.Hex("#211e24")     // surface-container
		t.SurfaceHover = ui.Hex("#2c282e")   // surface-container-high
		t.SurfacePressed = ui.Hex("#373339") // surface-container-highest
		t.Border = ui.Hex("#4a454e")         // outline-variant
		t.BorderStrong = ui.Hex("#958e99")   // outline
		t.Text = ui.Hex("#e8e1e8")           // on-surface
		t.TextMuted = ui.Hex("#cbc4cf")      // on-surface-variant

		t.Accent = ui.Hex("#dcb8ff")     // primary
		t.AccentSoft = ui.Hex("#573471") // primary-container
		t.AccentFill = ui.Hex("#dcb8ff") // primary
		t.AccentHover = ui.Hex("#e9ccff")
		t.AccentPressed = ui.Hex("#c9a2ea")
		t.AccentText = ui.Hex("#3f1d5a") // on-primary

		t.OnPrimaryContainer = ui.Hex("#f2dbff")
		t.Secondary = ui.Hex("#d3c0dd")
		t.OnSecondary = ui.Hex("#382e41")
		t.SecondaryContainer = ui.Hex("#4f4459")
		t.OnSecondaryContainer = ui.Hex("#eedcf8")
		t.Tertiary = ui.Hex("#eeb8cd")
		t.OnTertiary = ui.Hex("#472635")
		t.TertiaryContainer = ui.Hex("#5f3b4a")
		t.OnTertiaryContainer = ui.Hex("#ffd8e6")

		t.Danger = ui.Hex("#ffb4ab") // error
		t.Warning = ui.Hex("#f2b35b")
		t.Success = ui.Hex("#5ddc9a")
		t.Selection = ui.RGBA(220, 184, 255, 0.3)
		t.Focus = ui.RGBA(220, 184, 255, 0.75)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.22)
		t.Shadow = ui.RGBA(0, 0, 0, 0.55)
		t.Inverse = ui.Hex("#e8e1e8")     // inverse-surface
		t.InverseText = ui.Hex("#322f35") // inverse-on-surface
	} else {
		t.Bg = ui.Hex("#fdf7ff")             // surface
		t.Surface = ui.Hex("#f7f0fa")        // surface-container-low
		t.SurfaceAlt = ui.Hex("#f1eaf4")     // surface-container
		t.SurfaceHover = ui.Hex("#ebe4ee")   // surface-container-high
		t.SurfacePressed = ui.Hex("#e5dee9") // surface-container-highest
		t.Border = ui.Hex("#ccc4d0")         // outline-variant
		t.BorderStrong = ui.Hex("#7b7582")   // outline
		t.Text = ui.Hex("#1c1a1f")           // on-surface
		t.TextMuted = ui.Hex("#4a4550")      // on-surface-variant

		t.Accent = ui.Hex("#6b4a86")     // primary
		t.AccentSoft = ui.Hex("#eedbff") // primary-container
		t.AccentFill = ui.Hex("#6b4a86") // primary
		t.AccentHover = ui.Hex("#5a3c74")
		t.AccentPressed = ui.Hex("#4a305f")
		t.AccentText = ui.Hex("#ffffff") // on-primary

		t.OnPrimaryContainer = ui.Hex("#2a0f42")
		t.Secondary = ui.Hex("#63596e")
		t.OnSecondary = ui.Hex("#ffffff")
		t.SecondaryContainer = ui.Hex("#eadcf2")
		t.OnSecondaryContainer = ui.Hex("#201a26")
		t.Tertiary = ui.Hex("#7c5265")
		t.OnTertiary = ui.Hex("#ffffff")
		t.TertiaryContainer = ui.Hex("#ffd8e6")
		t.OnTertiaryContainer = ui.Hex("#31101f")

		t.Danger = ui.Hex("#ba1a1a") // error
		t.Warning = ui.Hex("#a85f00")
		t.Success = ui.Hex("#0f7b4f")
		t.Selection = ui.RGBA(107, 74, 134, 0.22)
		t.Focus = ui.RGBA(107, 74, 134, 0.65)
		t.Scrollbar = ui.RGBA(28, 26, 31, 0.26)
		t.Shadow = ui.RGBA(43, 34, 49, 0.14)
		t.Inverse = ui.Hex("#322f35")     // inverse-surface
		t.InverseText = ui.Hex("#f5eef7") // inverse-on-surface
	}
	// The older names are the same roles: a view keeps its spelling while a
	// new one names the Material role it means.
	t.Primary = t.Accent
	t.OnPrimary = t.AccentText
	t.PrimaryContainer = t.AccentSoft
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

// themeOf returns the tokens the current frame draws with, adjusted for the
// desktop's high contrast setting.
func themeOf(c *ui.Context) tokens {
	t := tokensFor(c.Theme().Dark)
	if s := textScale(c); s != 1 {
		t.TextDetail *= s
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
		t.Accent = readableAccent(a, t.Text, t.Surface)
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
