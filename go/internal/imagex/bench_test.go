package imagex

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
)

// benchJPEG is the bytes of a w×h JPEG, encoded once for a benchmark.
func benchJPEG(b *testing.B, w, h int) []byte {
	b.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(w, h), nil); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

var benchBitmap *image.RGBA

// BenchmarkDecodeThumb measures the load path of a cover: decode a 1200x1700
// JPEG and scale it to 480x680.
func BenchmarkDecodeThumb(b *testing.B) {
	data := benchJPEG(b, 1200, 1700)
	b.ReportAllocs()
	for b.Loop() {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		benchBitmap = fit(img, 480, 680).(*image.RGBA)
	}
}

// BenchmarkDecodeMediumCover is the same load path from the Medium cover
// (540x765), for comparison with the Large one in BenchmarkDecodeThumb.
func BenchmarkDecodeMediumCover(b *testing.B) {
	data := benchJPEG(b, 540, 765)
	b.ReportAllocs()
	for b.Loop() {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		benchBitmap = fit(img, 480, 680).(*image.RGBA)
	}
}

// BenchmarkFit measures the scale alone, from an already decoded image.
func BenchmarkFit(b *testing.B) {
	img := testImage(1200, 1700)
	b.ReportAllocs()
	for b.Loop() {
		benchBitmap = fit(img, 480, 680).(*image.RGBA)
	}
}
