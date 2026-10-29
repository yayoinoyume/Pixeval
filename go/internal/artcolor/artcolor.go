// Package artcolor derives a quiet interface palette from one illustration.
//
// The seed is the artwork's most characteristic color, not its average: an
// average of a blue sky, a red coat and a white face turns gray. The palette
// then clamps that color to a contrast-safe accent and tints the surfaces by
// only a few points, so the picture keeps the color and the interface stays
// readable.
package artcolor

import (
	"image"
	"image/color"
	"math"

	"github.com/egoist/mygo/ui"
)

// Palette is the set of colors a work's page uses. It is intentionally small:
// an accent, a neutral surface set and the two ends of the room's wash.
type Palette struct {
	Accent        ui.Color
	AccentSoft    ui.Color
	AccentText    ui.Color
	AccentHover   ui.Color
	AccentPressed ui.Color

	Bg             ui.Color
	Surface        ui.Color
	SurfaceAlt     ui.Color
	SurfaceHover   ui.Color
	SurfacePressed ui.Color
	Border         ui.Color
	BorderStrong   ui.Color
	Text           ui.Color
	TextMuted      ui.Color

	WashTop    ui.Color
	WashBottom ui.Color
	Shadow     ui.Color
}

// Extract returns the artwork's characteristic color. A picture without
// enough color (a black-and-white drawing) returns a neutral gray, which the
// palette turns into an ink accent rather than inventing a hue.
func Extract(img image.Image) ui.Color {
	if img == nil {
		return ui.Color{}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ui.Color{}
	}

	// Sample a bounded grid. A 4K original then costs the same as a cover;
	// the colors, not the pixels, are what the seed needs.
	const maxSamples = 4096
	step := 1
	if w*h > maxSamples {
		step = int(math.Ceil(math.Sqrt(float64(w*h) / maxSamples)))
	}

	type bin struct {
		n                int
		l, a, bb, chroma float64
	}
	bins := make(map[[3]int]*bin, 512)
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 128 {
				continue
			}
			o := fromColor(ui.RGB(c.R, c.G, c.B))
			li := int(clamp(o.L*10, 0, 9.999))
			ai := int(clamp((o.A+0.4)/0.8*10, 0, 9.999))
			bi := int(clamp((o.B+0.4)/0.8*10, 0, 9.999))
			k := [3]int{li, ai, bi}
			v := bins[k]
			if v == nil {
				v = &bin{}
				bins[k] = v
			}
			v.n++
			v.l += float64(o.L)
			v.a += float64(o.A)
			v.bb += float64(o.B)
			v.chroma += math.Hypot(float64(o.A), float64(o.B))
			total++
		}
	}
	if total == 0 {
		return ui.Color{}
	}

	// Choose the most characteristic color, not the largest one. A white
	// dress or a dark pair of hair can cover most of the picture and still
	// say nothing about it; the saturated hair ribbon or neon sign is the
	// color a person remembers. Weight chroma over area, but require the
	// color to cover at least half a percent of the sample so a one-pixel
	// highlight cannot choose the whole interface.
	var best *bin
	bestScore := -1.0
	var dominant *bin
	dominantScore := -1.0
	for _, v := range bins {
		n := float64(v.n)
		l := v.l / n
		ch := v.chroma / n
		mid := 1 - math.Abs(l-0.55)*1.5
		if mid < 0 {
			mid = 0
		}
		plain := n * (0.25 + mid)
		if l < 0.10 || l > 0.92 {
			plain *= 0.1
		}
		if plain > dominantScore {
			dominant, dominantScore = v, plain
		}
		if n < float64(total)*0.005 || l < 0.06 || l > 0.94 {
			continue
		}
		score := ch * math.Pow(n/float64(total), 0.35) * (0.35 + mid)
		if score > bestScore {
			best, bestScore = v, score
		}
	}
	if best == nil {
		best = dominant
	}
	if best == nil {
		return ui.Color{}
	}
	n := float64(best.n)
	l := best.l / n
	a := best.a / n
	bb := best.bb / n
	ch := math.Hypot(a, bb)
	if ch < 0.018 {
		// Near gray: keep it neutral, with no invented hue.
		return ui.Oklch(float32(l), 0, 0)
	}
	hue := float32(math.Atan2(bb, a) * 180 / math.Pi)
	if hue < 0 {
		hue += 360
	}
	return ui.Oklch(float32(l), float32(ch), hue)
}

// Derive turns a seed into a contrast-safe light or dark palette. The accent
// is the seed with its lightness and chroma clamped to a readable range; the
// surfaces are the seed's hue at a much lower chroma, so the interface reads
// as tinted paper or tinted ink rather than as a colored theme.
func Derive(seed ui.Color, dark bool) Palette {
	s := fromColor(seed)
	c := float32(math.Hypot(float64(s.A), float64(s.B)))
	h := float32(math.Atan2(float64(s.B), float64(s.A)) * 180 / math.Pi)
	if h < 0 {
		h += 360
	}
	// A picture's characteristic color is often a pastel; keep enough of
	// its chroma in the interface that the tint is visible, without turning
	// a muted drawing neon. A nearly gray picture stays neutral.
	switch {
	case c < 0.025:
		c = 0
	case c < 0.08:
		c = 0.08
	default:
		c = min(c*1.45, 0.18)
	}

	if dark {
		return darkPalette(h, c, s.L)
	}
	return lightPalette(h, c, s.L)
}

func lightPalette(h, c, seedL float32) Palette {
	surface := oklch(0.998, 0.002, h)
	bg := oklch(0.972, 0.006, h)
	text := oklch(0.205, 0.012, h)
	muted := oklch(0.47, 0.014, h)
	border := oklch(0.885, 0.008, h)
	borderStrong := oklch(0.72, 0.012, h)
	surfaceAlt := oklch(0.945, 0.009, h)
	surfaceHover := oklch(0.918, 0.012, h)
	surfacePressed := oklch(0.888, 0.014, h)
	washTop := oklch(0.968, min(0.032, c*0.24), h)
	washBottom := oklch(0.915, min(0.042, c*0.30), h)

	l := clamp(seedL, 0.40, 0.55)
	if h >= 55 && h <= 100 {
		// Yellow and yellow-green at the same lightness read brighter than
		// the measured luminance suggests; deepen them a little.
		l = min(l, 0.47)
	}
	accent := oklch(l, c, h)
	for i := 0; i < 24 && (contrast(accent, surface) < 4.5 || contrast(accent, washBottom) < 4.5); i++ {
		l -= 0.015
		accent = oklch(l, c, h)
	}
	accentHover := oklch(max(l-0.055, 0.28), c, h)
	accentPressed := oklch(max(l-0.10, 0.24), c, h)
	accentSoft := oklch(0.935, min(0.040, c*0.24), h)

	return Palette{
		Accent:         accent,
		AccentSoft:     accentSoft,
		AccentText:     bestText(accent, oklch(0.99, 0, h), oklch(0.16, 0, h)),
		AccentHover:    accentHover,
		AccentPressed:  accentPressed,
		Bg:             bg,
		Surface:        surface,
		SurfaceAlt:     surfaceAlt,
		SurfaceHover:   surfaceHover,
		SurfacePressed: surfacePressed,
		Border:         border,
		BorderStrong:   borderStrong,
		Text:           text,
		TextMuted:      muted,
		WashTop:        washTop,
		WashBottom:     washBottom,
		Shadow:         ui.RGBA(28, 22, 18, 0.14),
	}
}

func darkPalette(h, c, seedL float32) Palette {
	surface := oklch(0.135, 0.010, h)
	bg := oklch(0.100, 0.008, h)
	text := oklch(0.935, 0.008, h)
	muted := oklch(0.700, 0.014, h)
	border := oklch(0.255, 0.012, h)
	borderStrong := oklch(0.430, 0.016, h)
	surfaceAlt := oklch(0.165, 0.012, h)
	surfaceHover := oklch(0.205, 0.015, h)
	surfacePressed := oklch(0.245, 0.018, h)
	washTop := oklch(0.145, min(0.042, c*0.30), h)
	washBottom := oklch(0.078, min(0.032, c*0.24), h)

	l := clamp(seedL, 0.68, 0.84)
	accent := oklch(l, c, h)
	for i := 0; i < 24 && (contrast(accent, surface) < 4.5 || contrast(accent, washBottom) < 4.5); i++ {
		l += 0.012
		accent = oklch(l, c, h)
	}
	accentHover := oklch(min(l+0.045, 0.92), c, h)
	accentPressed := oklch(min(l+0.02, 0.90), c, h)
	accentSoft := oklch(0.215, min(0.050, c*0.32), h)

	return Palette{
		Accent:         accent,
		AccentSoft:     accentSoft,
		AccentText:     bestText(accent, oklch(0.99, 0, h), oklch(0.12, 0, h)),
		AccentHover:    accentHover,
		AccentPressed:  accentPressed,
		Bg:             bg,
		Surface:        surface,
		SurfaceAlt:     surfaceAlt,
		SurfaceHover:   surfaceHover,
		SurfacePressed: surfacePressed,
		Border:         border,
		BorderStrong:   borderStrong,
		Text:           text,
		TextMuted:      muted,
		WashTop:        washTop,
		WashBottom:     washBottom,
		Shadow:         ui.RGBA(0, 0, 0, 0.58),
	}
}

// oklch clamps the inputs to the range ui.Oklch accepts and returns the sRGB
// color; ui.Oklch keeps the wide-gamut color when the window can show it.
func oklch(l, c, h float32) ui.Color {
	return ui.Oklch(clamp(l, 0, 1), max(c, 0), h)
}

// bestText picks white or ink by whichever contrasts more with fill. A
// palette must never leave the primary button unreadable, even for a
// middle-lightness accent.
func bestText(fill, light, dark ui.Color) ui.Color {
	if contrast(light, fill) >= contrast(dark, fill) {
		return light
	}
	return dark
}

func clamp(v, lo, hi float32) float32 {
	if math.IsNaN(float64(v)) {
		return lo
	}
	return min(max(v, lo), hi)
}

// contrast is WCAG's contrast ratio between two opaque colors.
func contrast(a, b ui.Color) float32 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

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

// oklab is the color in the Oklab space, with A and B in the usual range.
type oklab struct {
	L, A, B float32
}

// fromColor converts sRGB to Oklab using Björn Ottosson's matrices.
func fromColor(c ui.Color) oklab {
	r := srgbToLinear(float64(c.R) / 255)
	g := srgbToLinear(float64(c.G) / 255)
	b := srgbToLinear(float64(c.B) / 255)

	l := 0.4122214708*r + 0.5363325363*g + 0.0514459929*b
	m := 0.2119034982*r + 0.6806995451*g + 0.1073969566*b
	s := 0.0883024619*r + 0.2817188376*g + 0.6299787005*b

	l_ := math.Cbrt(l)
	m_ := math.Cbrt(m)
	s_ := math.Cbrt(s)

	return oklab{
		L: float32(0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_),
		A: float32(1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_),
		B: float32(0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_),
	}
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
