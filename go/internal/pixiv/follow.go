package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// Follow follows a user. A private follow is hidden from the user's profile.
func (c *Client) Follow(ctx context.Context, userID int64, private bool) error {
	form := url.Values{}
	form.Set("user_id", strconv.FormatInt(userID, 10))
	form.Set("restrict", restrictParam(private))
	return c.post(ctx, "/v1/user/follow/add", form, nil)
}

// Unfollow stops following a user.
func (c *Client) Unfollow(ctx context.Context, userID int64) error {
	form := url.Values{}
	form.Set("user_id", strconv.FormatInt(userID, 10))
	return c.post(ctx, "/v1/user/follow/delete", form, nil)
}
