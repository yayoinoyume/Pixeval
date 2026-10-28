package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// AddBookmark bookmarks a work. A private bookmark is visible only to the user.
func (c *Client) AddBookmark(ctx context.Context, illustID int64, private bool) error {
	form := url.Values{}
	form.Set("illust_id", strconv.FormatInt(illustID, 10))
	form.Set("restrict", restrictParam(private))
	return c.post(ctx, "/v2/illust/bookmark/add", form, nil)
}

// RemoveBookmark removes the user's bookmark from a work.
func (c *Client) RemoveBookmark(ctx context.Context, illustID int64) error {
	form := url.Values{}
	form.Set("illust_id", strconv.FormatInt(illustID, 10))
	return c.post(ctx, "/v1/illust/bookmark/delete", form, nil)
}

// restrictParam is the visibility value both bookmarks and follows send.
func restrictParam(private bool) string {
	if private {
		return "private"
	}
	return "public"
}
