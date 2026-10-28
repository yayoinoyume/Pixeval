package animated

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"
)

func solidFrame(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestEncodeGIFRoundTrip(t *testing.T) {
	in := &Frames{
		Loop: 0,
		Images: []*image.RGBA{
			solidFrame(8, 6, color.RGBA{R: 255, A: 255}),
			solidFrame(8, 6, color.RGBA{G: 255, A: 255}),
			solidFrame(8, 6, color.RGBA{B: 255, A: 255}),
		},
		Delays: []time.Duration{100 * time.Millisecond, 40 * time.Millisecond, 70 * time.Millisecond},
	}

	data, err := EncodeGIF(in)
	if err != nil {
		t.Fatalf("EncodeGIF: %v", err)
	}
	out, err := DecodeGIF(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeGIF: %v", err)
	}
	if len(out.Images) != len(in.Images) {
		t.Fatalf("frames = %d, want %d", len(out.Images), len(in.Images))
	}
	if len(out.Delays) != len(in.Delays) {
		t.Fatalf("delays = %d, want %d", len(out.Delays), len(in.Delays))
	}
	if out.Loop != 0 {
		t.Errorf("Loop = %d, want 0", out.Loop)
	}
	for i := range out.Images {
		if got := out.Images[i].Bounds(); got != image.Rect(0, 0, 8, 6) {
			t.Errorf("frame %d bounds = %v", i, got)
		}
		if out.Delays[i] != in.Delays[i] {
			t.Errorf("frame %d delay = %v, want %v", i, out.Delays[i], in.Delays[i])
		}
	}
	r, _, _, _ := out.Images[0].At(4, 3).RGBA()
	if r>>8 < 200 {
		t.Errorf("first frame red = %d, want a red frame", r>>8)
	}
}

func TestDecodeGIFCorrupt(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("not a gif"),
		[]byte("GIF89a"),
		{},
	} {
		if _, err := DecodeGIF(bytes.NewReader(data)); err == nil {
			t.Errorf("DecodeGIF(%q) = nil error, want an error", data)
		}
	}
}

func TestEncodeGIFNoFrames(t *testing.T) {
	if _, err := EncodeGIF(&Frames{}); err == nil {
		t.Error("EncodeGIF of no frames = nil error, want an error")
	}
}
