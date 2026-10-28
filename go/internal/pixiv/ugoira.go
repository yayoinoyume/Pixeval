package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// UgoiraFrame is one file of an ugoira zip and how long it shows, in
// milliseconds.
type UgoiraFrame struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

// UgoiraMetadata is the frame list of an ugoira work and the URL of its zip.
// The zip needs the image Referer, so it is downloaded with GetImage.
type UgoiraMetadata struct {
	ZipURL string
	Frames []UgoiraFrame
}

// UgoiraMetadata fetches the frames and the zip URL of an ugoira work.
func (c *Client) UgoiraMetadata(ctx context.Context, illustID int64) (*UgoiraMetadata, error) {
	q := url.Values{}
	q.Set("illust_id", strconv.FormatInt(illustID, 10))
	var r struct {
		Meta struct {
			ZipURLs struct {
				Medium string `json:"medium"`
			} `json:"zip_urls"`
			Frames []UgoiraFrame `json:"frames"`
		} `json:"ugoira_metadata"`
	}
	if err := c.get(ctx, c.pageURL("/v1/ugoira/metadata", q), &r); err != nil {
		return nil, err
	}
	return &UgoiraMetadata{ZipURL: r.Meta.ZipURLs.Medium, Frames: r.Meta.Frames}, nil
}
