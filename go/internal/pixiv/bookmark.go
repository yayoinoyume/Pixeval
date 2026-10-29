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

// bookmarkPageLimit is how many works one bookmarks page asks for.
const bookmarkPageLimit = 48

// BookmarkedIllustrations lists the works userID bookmarked, public or
// private. offset is the page's first work, 0 for the first page; later
// pages come from the page's NextURL through NextIllustrations.
func (c *Client) BookmarkedIllustrations(ctx context.Context, userID int64, private bool, offset int) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	q.Set("restrict", restrictParam(private))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(bookmarkPageLimit))
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v1/user/bookmarks/illust", q))
}

// restrictParam is the visibility value both bookmarks and follows send.
func restrictParam(private bool) string {
	if private {
		return "private"
	}
	return "public"
}
