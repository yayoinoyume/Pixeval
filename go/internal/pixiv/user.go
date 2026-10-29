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

// UserNovels lists a user's novels.
func (c *Client) UserNovels(ctx context.Context, userID int64) (*Page[Novel], error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	c.setFilter(q)
	return c.novelPage(ctx, c.pageURL("/v1/user/novels", q))
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

// UserMyPixiv lists the users userID added to MyPixiv.
func (c *Client) UserMyPixiv(ctx context.Context, userID int64) (*Page[UserPreview], error) {
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(userID, 10))
	c.setFilter(q)
	return c.userPreviewPage(ctx, c.pageURL("/v1/user/mypixiv", q))
}

// RecommendedUsers lists the users Pixiv recommends to the signed-in user.
func (c *Client) RecommendedUsers(ctx context.Context) (*Page[UserPreview], error) {
	q := url.Values{}
	c.setFilter(q)
	return c.recommendedUserPage(ctx, c.pageURL("/v1/user/recommended", q))
}

// NextRecommendedUsers loads the page a recommended list's NextURL points to.
// Its pages carry users, not the user_previews the other user lists use.
func (c *Client) NextRecommendedUsers(ctx context.Context, nextURL string) (*Page[UserPreview], error) {
	return c.recommendedUserPage(ctx, nextURL)
}

// RelatedUsers lists the users Pixiv lists beside userID.
func (c *Client) RelatedUsers(ctx context.Context, userID int64) (*Page[UserPreview], error) {
	q := url.Values{}
	q.Set("seed_user_id", strconv.FormatInt(userID, 10))
	c.setFilter(q)
	return c.userPreviewPage(ctx, c.pageURL("/v1/user/related", q))
}

// SearchUsers searches users by word.
func (c *Client) SearchUsers(ctx context.Context, word string) (*Page[UserPreview], error) {
	q := url.Values{}
	q.Set("word", word)
	c.setFilter(q)
	return c.userPreviewPage(ctx, c.pageURL("/v1/search/user", q))
}

// NextUserPreviews loads the page a NextURL from a user list points to.
func (c *Client) NextUserPreviews(ctx context.Context, nextURL string) (*Page[UserPreview], error) {
	return c.userPreviewPage(ctx, nextURL)
}

type userPreviewList struct {
	Previews []UserPreview `json:"user_previews"`
	NextURL  string        `json:"next_url"`
}

type recommendedUserList struct {
	Users   []User `json:"users"`
	NextURL string `json:"next_url"`
}

// recommendedUserPage decodes a user list whose entries are plain users,
// wrapping each in the UserPreview the lists draw.
func (c *Client) recommendedUserPage(ctx context.Context, rawURL string) (*Page[UserPreview], error) {
	var r recommendedUserList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	previews := make([]UserPreview, len(r.Users))
	for i, u := range r.Users {
		previews[i] = UserPreview{User: u}
	}
	return &Page[UserPreview]{Items: previews, NextURL: r.NextURL}, nil
}

func (c *Client) userPreviewPage(ctx context.Context, rawURL string) (*Page[UserPreview], error) {
	var r userPreviewList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[UserPreview]{Items: r.Previews, NextURL: r.NextURL}, nil
}
