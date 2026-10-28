package pixiv

import (
	"context"
	"net/url"
	"time"
)

// Novel is one novel of a list: the fields a card and the reader need. It
// lives with the list rather than in models.go, which holds the works.
type Novel struct {
	ID             int64     `json:"id"`
	Title          string    `json:"title"`
	Caption        string    `json:"caption"`
	User           User      `json:"user"`
	ImageUrls      ImageUrls `json:"image_urls"`
	PageCount      int       `json:"page_count"`
	CreateDate     time.Time `json:"create_date"`
	IsBookmarked   bool      `json:"is_bookmarked"`
	TotalBookmarks int       `json:"total_bookmarks"`
}

// novelCoverAspect is the shape of a novel cover. The list carries no width or
// height, and Pixiv draws novel covers portrait.
const novelCoverAspect = 3.0 / 4.0

// CoverURL is the largest thumbnail, used in grids.
func (n *Novel) CoverURL() string {
	switch {
	case n.ImageUrls.Large != "":
		return n.ImageUrls.Large
	case n.ImageUrls.Medium != "":
		return n.ImageUrls.Medium
	default:
		return n.ImageUrls.SquareMedium
	}
}

// Aspect is width divided by height, which the list does not carry for a
// novel.
func (n *Novel) Aspect() float64 { return novelCoverAspect }

// SearchNovels searches novels by word. It takes the same options as an
// illustration search, and only their word, sort and target.
func (c *Client) SearchNovels(ctx context.Context, opts SearchOptions) (*Page[Novel], error) {
	q := url.Values{}
	q.Set("word", opts.Word)
	if opts.Sort == "" {
		opts.Sort = "date_desc"
	}
	if opts.Target == "" {
		opts.Target = "partial_match_for_tags"
	}
	if opts.Filter == "" {
		opts.Filter = c.filterParam()
	}
	q.Set("sort", opts.Sort)
	q.Set("search_target", opts.Target)
	if opts.Filter != "" {
		q.Set("filter", opts.Filter)
	}
	return c.novelPage(ctx, c.pageURL("/v1/search/novel", q))
}

// NextNovels loads the page a NextURL points to.
func (c *Client) NextNovels(ctx context.Context, nextURL string) (*Page[Novel], error) {
	return c.novelPage(ctx, nextURL)
}

func (c *Client) novelPage(ctx context.Context, rawURL string) (*Page[Novel], error) {
	var r struct {
		Novels  []Novel `json:"novels"`
		NextURL string  `json:"next_url"`
	}
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[Novel]{Items: r.Novels, NextURL: r.NextURL}, nil
}
