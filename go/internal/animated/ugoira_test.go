package animated

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

func pngBytes(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidFrame(4, 4, c)); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func zipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func TestExtractZipOrder(t *testing.T) {
	data := zipBytes(t, map[string][]byte{
		"frame_0.png": pngBytes(t, color.RGBA{R: 255, A: 255}),
		"frame_1.png": pngBytes(t, color.RGBA{G: 255, A: 255}),
		"frame_2.png": pngBytes(t, color.RGBA{B: 255, A: 255}),
	})
	frames := []pixiv.UgoiraFrame{
		{File: "frame_2.png", Delay: 200},
		{File: "frame_0.png", Delay: 100},
		{File: "frame_1.png", Delay: 50},
	}

	f, err := ExtractZip(data, frames)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	want := []color.RGBA{
		{R: 0, G: 0, B: 255, A: 255},
		{R: 255, G: 0, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 255},
	}
	wantDelay := []time.Duration{200 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond}
	if len(f.Images) != len(frames) {
		t.Fatalf("frames = %d, want %d", len(f.Images), len(frames))
	}
	for i := range frames {
		if got := f.Images[i].Bounds(); got != image.Rect(0, 0, 4, 4) {
			t.Errorf("frame %d bounds = %v", i, got)
		}
		if got := f.Images[i].RGBAAt(2, 2); got != want[i] {
			t.Errorf("frame %d pixel = %v, want %v", i, got, want[i])
		}
		if f.Delays[i] != wantDelay[i] {
			t.Errorf("frame %d delay = %v, want %v", i, f.Delays[i], wantDelay[i])
		}
	}
}

func TestExtractZipCorrupt(t *testing.T) {
	frames := []pixiv.UgoiraFrame{{File: "frame_0.png", Delay: 100}}
	if _, err := ExtractZip([]byte("not a zip"), frames); err == nil {
		t.Error("ExtractZip of a broken zip = nil error, want an error")
	}
	if _, err := ExtractZip(nil, frames); err == nil {
		t.Error("ExtractZip of an empty zip = nil error, want an error")
	}
	if _, err := ExtractZip(nil, nil); err == nil {
		t.Error("ExtractZip of no frames = nil error, want an error")
	}
	data := zipBytes(t, map[string][]byte{"frame_0.png": pngBytes(t, color.RGBA{A: 255})})
	if _, err := ExtractZip(data, []pixiv.UgoiraFrame{{File: "missing.png"}}); err == nil {
		t.Error("ExtractZip of a missing frame = nil error, want an error")
	}
	if _, err := ExtractZip(data, []pixiv.UgoiraFrame{{File: "frame_0.png"}}); err != nil {
		t.Errorf("ExtractZip of a valid zip: %v", err)
	}
}
