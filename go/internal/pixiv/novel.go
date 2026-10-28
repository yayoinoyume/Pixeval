package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// NovelContent fetches the body of a novel as Pixiv returns it: the string the
// reading view hands to a renderer, not plain text.
func (c *Client) NovelContent(ctx context.Context, novelID int64) (string, error) {
	q := url.Values{}
	q.Set("novel_id", strconv.FormatInt(novelID, 10))
	var r struct {
		Text string `json:"novel_text"`
	}
	if err := c.get(ctx, c.pageURL("/webview/v2/novel", q), &r); err != nil {
		return "", err
	}
	return r.Text, nil
}
