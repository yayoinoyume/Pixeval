// Package animated holds animated works as RGBA frames with their delays: GIF
// bytes decoded, the frame zip of a Pixiv ugoira, and the way back to a GIF.
package animated

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/gif"
	"io"
	"time"

	"golang.org/x/image/draw"
)

// gifDelay is the GIF delay unit: one hundredth of a second.
const gifDelay = 10 * time.Millisecond

// Frames is a decoded animation. Every frame has the same bounds, which are
// also the bounds of the animation. Loop is the GIF loop count: 0 repeats
// forever, -1 means the file has no loop extension and plays once.
type Frames struct {
	Images []*image.RGBA
	Delays []time.Duration
	Loop   int
}

// DecodeGIF decodes every frame of a GIF. A frame that covers only part of the
// canvas is placed on a transparent canvas; disposal methods are not applied,
// so a frame that relies on the previous one is not merged into it.
func DecodeGIF(r io.Reader) (*Frames, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, fmt.Errorf("animated: decode gif: %w", err)
	}
	if len(g.Image) == 0 {
		return nil, errors.New("animated: gif has no frames")
	}
	if len(g.Delay) != len(g.Image) {
		return nil, errors.New("animated: gif frame and delay counts differ")
	}
	canvas := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	f := &Frames{
		Images: make([]*image.RGBA, len(g.Image)),
		Delays: make([]time.Duration, len(g.Image)),
		Loop:   g.LoopCount,
	}
	for i, img := range g.Image {
		f.Images[i] = toRGBA(img, canvas)
		f.Delays[i] = time.Duration(g.Delay[i]) * gifDelay
	}
	return f, nil
}

// EncodeGIF writes the frames as a GIF. Loop is written as is, so 0 repeats
// forever; per-frame delays are rounded to the 10ms GIF unit.
func EncodeGIF(f *Frames) ([]byte, error) {
	if f == nil || len(f.Images) == 0 {
		return nil, errors.New("animated: no frames to encode")
	}
	if len(f.Delays) != len(f.Images) {
		return nil, errors.New("animated: frame and delay counts differ")
	}
	g := &gif.GIF{
		Image:     make([]*image.Paletted, len(f.Images)),
		Delay:     make([]int, len(f.Images)),
		LoopCount: f.Loop,
	}
	for i, img := range f.Images {
		if img == nil {
			return nil, fmt.Errorf("animated: frame %d is nil", i)
		}
		b := img.Bounds()
		p := image.NewPaletted(b, palette.Plan9)
		draw.FloydSteinberg.Draw(p, b, img, b.Min)
		g.Image[i] = p
		g.Delay[i] = int((f.Delays[i] + gifDelay/2) / gifDelay)
		if g.Delay[i] < 0 {
			g.Delay[i] = 0
		}
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, fmt.Errorf("animated: encode gif: %w", err)
	}
	return buf.Bytes(), nil
}

// toRGBA converts img to an RGBA image of canvas size, placing it at its own
// bounds.
func toRGBA(img image.Image, canvas image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(canvas)
	b := img.Bounds()
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}
