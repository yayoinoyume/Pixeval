package animated

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"

	// Ugoira frames are JPEG in the zip and PNG in the test fixture.
	_ "image/jpeg"
	_ "image/png"
)

// ExtractZip decodes the frames of an ugoira zip in the order the metadata
// lists them, taking each delay from that metadata in milliseconds.
func ExtractZip(data []byte, frames []pixiv.UgoiraFrame) (*Frames, error) {
	if len(frames) == 0 {
		return nil, errors.New("animated: ugoira has no frames")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("animated: open zip: %w", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	f := &Frames{
		Images: make([]*image.RGBA, len(frames)),
		Delays: make([]time.Duration, len(frames)),
	}
	for i, uf := range frames {
		zf, ok := files[uf.File]
		if !ok {
			return nil, fmt.Errorf("animated: zip has no frame %q", uf.File)
		}
		img, err := decodeFile(zf)
		if err != nil {
			return nil, fmt.Errorf("animated: frame %q: %w", uf.File, err)
		}
		f.Images[i] = toRGBA(img, img.Bounds())
		f.Delays[i] = time.Duration(uf.Delay) * time.Millisecond
	}
	return f, nil
}

// decodeFile decodes one zip entry.
func decodeFile(zf *zip.File) (image.Image, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	return img, nil
}
