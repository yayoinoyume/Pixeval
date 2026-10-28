package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

type illustList struct {
	Illusts []Illustration `json:"illusts"`
	NextURL string         `json:"next_url"`
}

// Recommended is the home feed of works Pixiv picked for the user.
func (c *Client) Recommended(ctx context.Context) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("filter", "for_android")
	return c.illustPage(ctx, PageURL("/v1/illust/recommended", q))
}

// SearchOptions are the filters of an illustration search.
type SearchOptions struct {
	Word      string
	Sort      string // date_desc, date_asc, popular_desc
	Target    string // partial_match_for_tags, exact_match_for_tags, title_and_caption
	Filter    string
	StartDate string
	EndDate   string
	MinWidth  int
	MaxWidth  int
	MinHeight int
	MaxHeight int
}

// SearchIllustrations searches works by word.
func (c *Client) SearchIllustrations(ctx context.Context, opts SearchOptions) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("word", opts.Word)
	if opts.Sort == "" {
		opts.Sort = "date_desc"
	}
	if opts.Target == "" {
		opts.Target = "partial_match_for_tags"
	}
	if opts.Filter == "" {
		opts.Filter = "for_android"
	}
	q.Set("sort", opts.Sort)
	q.Set("search_target", opts.Target)
	q.Set("filter", opts.Filter)
	if opts.StartDate != "" {
		q.Set("start_date", opts.StartDate)
	}
	if opts.EndDate != "" {
		q.Set("end_date", opts.EndDate)
	}
	addInt := func(key string, v int) {
		if v > 0 {
			q.Set(key, strconv.Itoa(v))
		}
	}
	addInt("width_min", opts.MinWidth)
	addInt("width_max", opts.MaxWidth)
	addInt("height_min", opts.MinHeight)
	addInt("height_max", opts.MaxHeight)
	return c.illustPage(ctx, PageURL("/v1/search/illust", q))
}

// Illustration fetches one work by ID.
func (c *Client) Illustration(ctx context.Context, id int64) (*Illustration, error) {
	q := url.Values{}
	q.Set("illust_id", strconv.FormatInt(id, 10))
	q.Set("filter", "for_android")
	var r struct {
		Illust Illustration `json:"illust"`
	}
	if err := c.get(ctx, PageURL("/v1/illust/detail", q), &r); err != nil {
		return nil, err
	}
	return &r.Illust, nil
}

// NextIllustrations loads the page a NextURL points to.
func (c *Client) NextIllustrations(ctx context.Context, nextURL string) (*Page[Illustration], error) {
	return c.illustPage(ctx, nextURL)
}

func (c *Client) illustPage(ctx context.Context, rawURL string) (*Page[Illustration], error) {
	var r illustList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[Illustration]{Items: r.Illusts, NextURL: r.NextURL}, nil
}
