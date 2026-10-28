package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// UserDetail fetches a user and their profile.
func (c *Client) UserDetail(ctx context.Context, userID int64) (*UserDetail, error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	q.Set("filter", "for_android")
	var r UserDetail
	if err := c.get(ctx, c.pageURL("/v1/user/detail", q), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UserIllusts lists a user's works. kind is "illust" or "manga"; an empty
// kind means illust.
func (c *Client) UserIllusts(ctx context.Context, userID int64, kind string) (*Page[Illustration], error) {
	if kind == "" {
		kind = "illust"
	}
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	q.Set("type", kind)
	q.Set("filter", "for_android")
	return c.illustPage(ctx, c.pageURL("/v1/user/illusts", q))
}

// UserFollowing lists the users a user follows publicly.
func (c *Client) UserFollowing(ctx context.Context, userID int64) (*Page[UserPreview], error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	q.Set("restrict", "public")
	q.Set("filter", "for_android")
	return c.userPreviewPage(ctx, c.pageURL("/v1/user/following", q))
}

// UserFollowers lists the users following a user.
func (c *Client) UserFollowers(ctx context.Context, userID int64) (*Page[UserPreview], error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	q.Set("filter", "for_android")
	return c.userPreviewPage(ctx, c.pageURL("/v1/user/follower", q))
}

// NextUserPreviews loads the page a NextURL from a user list points to.
func (c *Client) NextUserPreviews(ctx context.Context, nextURL string) (*Page[UserPreview], error) {
	return c.userPreviewPage(ctx, nextURL)
}

type userPreviewList struct {
	Previews []UserPreview `json:"user_previews"`
	NextURL  string        `json:"next_url"`
}

func (c *Client) userPreviewPage(ctx context.Context, rawURL string) (*Page[UserPreview], error) {
	var r userPreviewList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[UserPreview]{Items: r.Previews, NextURL: r.NextURL}, nil
}
