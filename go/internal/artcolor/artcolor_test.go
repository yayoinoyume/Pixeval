package artcolor

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func solid(c color.RGBA, w, h int) image.Image {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetRGBA(x, y, c)
		}
	}
	return m
}

func chroma(c ui.Color) float32 {
	o := fromColor(c)
	return float32(math.Hypot(float64(o.A), float64(o.B)))
}

func hue(c ui.Color) float32 {
	o := fromColor(c)
	h := float32(math.Atan2(float64(o.B), float64(o.A)) * 180 / math.Pi)
	if h < 0 {
		h += 360
	}
	return h
}

func TestExtractKeepsAStrongColor(t *testing.T) {
	got := Extract(solid(color.RGBA{210, 62, 58, 255}, 64, 64))
	if got.A == 0 {
		t.Fatal("no seed")
	}
	if chroma(got) < 0.08 {
		t.Errorf("chroma %v for a saturated red, want at least 0.08", chroma(got))
	}
	if h := hue(got); h < 15 || h > 55 {
		t.Errorf("hue %v for red, want 15..55", h)
	}
}

func TestExtractFindsTheFigureNotThePaper(t *testing.T) {
	m := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			c := color.RGBA{246, 243, 236, 255} // warm white paper
			if x > 25 && x < 95 && y > 25 && y < 95 {
				c = color.RGBA{44, 96, 170, 255} // a blue figure
			}
			m.SetRGBA(x, y, c)
		}
	}
	got := Extract(m)
	if h := hue(got); h < 220 || h > 290 {
		t.Errorf("hue %v, want the blue figure's hue", h)
	}
	if chroma(got) < 0.05 {
		t.Errorf("chroma %v, want the figure's color", chroma(got))
	}
}

func TestExtractReturnsGrayForABlackAndWhiteImage(t *testing.T) {
	got := Extract(solid(color.RGBA{128, 128, 128, 255}, 32, 32))
	if got.A == 0 {
		t.Fatal("no seed")
	}
	if c := chroma(got); c > 0.02 {
		t.Errorf("chroma %v for gray, want a neutral seed", c)
	}
}

func TestDeriveKeepsTextReadable(t *testing.T) {
	seeds := []ui.Color{
		ui.Hex("#d23e3a"), // red
		ui.Hex("#2c60aa"), // blue
		ui.Hex("#d8c34a"), // yellow
		ui.Hex("#8a8a8a"), // gray
		ui.Hex("#f2e8de"), // near white
		ui.Hex("#171512"), // near black
	}
	for _, seed := range seeds {
		for _, dark := range []bool{false, true} {
			p := Derive(seed, dark)
			mode := "light"
			if dark {
				mode = "dark"
			}
			checks := []struct {
				name string
				fg   ui.Color
				bg   ui.Color
			}{
				{"body text", p.Text, p.WashBottom},
				{"body text on surface", p.Text, p.Surface},
				{"muted text", p.TextMuted, p.WashBottom},
				{"accent", p.Accent, p.WashBottom},
				{"accent text", p.AccentText, p.Accent},
			}
			for _, c := range checks {
				if got := contrast(c.fg, c.bg); got < 4.5 {
					t.Errorf("%s %s contrast %v (%v on %v), want at least 4.5", mode, c.name, got, c.fg, c.bg)
				}
			}
		}
	}
}

func TestDeriveChangesWithTheArtwork(t *testing.T) {
	red := Derive(ui.Hex("#d23e3a"), false)
	blue := Derive(ui.Hex("#2c60aa"), false)
	if hue(red.Accent) == hue(blue.Accent) {
		t.Errorf("red and blue accents have the same hue %v", hue(red.Accent))
	}
	if red.Accent == blue.Accent {
		t.Error("different seeds produced the same accent")
	}
}
