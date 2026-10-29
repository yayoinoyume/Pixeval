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
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v1/illust/recommended", q))
}

// WorkKind is the content type a works feed asks for: illustrations or manga.
type WorkKind string

const (
	Illust WorkKind = "illust"
	Manga  WorkKind = "manga"
)

// NewIllustrations is the site-wide feed of works just posted, of one kind.
func (c *Client) NewIllustrations(ctx context.Context, kind WorkKind) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("content_type", string(kind))
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v1/illust/new", q))
}

// MyPixivIllustrations is the feed of works by the users the signed-in user
// follows, of one kind.
func (c *Client) MyPixivIllustrations(ctx context.Context, kind WorkKind) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("content_type", string(kind))
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v2/illust/mypixiv", q))
}

// RelatedIllustrations is the feed of works Pixiv lists beside illustID.
func (c *Client) RelatedIllustrations(ctx context.Context, illustID int64) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("illust_id", strconv.FormatInt(illustID, 10))
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v1/illust/related", q))
}

// TrendingTag is a tag Pixiv lists as trending, with its translation.
type TrendingTag struct {
	Tag            string `json:"tag"`
	TranslatedName string `json:"translated_name"`
}

// TrendingTags returns the illustration tags Pixiv lists as trending now.
func (c *Client) TrendingTags(ctx context.Context) ([]TrendingTag, error) {
	q := url.Values{}
	c.setFilter(q)
	var r struct {
		TrendTags []TrendingTag `json:"trend_tags"`
	}
	if err := c.get(ctx, c.pageURL("/v1/trending-tags/illust", q), &r); err != nil {
		return nil, err
	}
	return r.TrendTags, nil
}

// SearchOptions are the filters of an illustration search.
type SearchOptions struct {
	Word      string
	Sort      string // date_desc, date_asc, popular_desc
	Target    string // partial_match_for_tags, exact_match_for_tags, title_and_caption
	Filter    string // empty means the client's R18 filter
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
		opts.Filter = c.filterParam()
	}
	q.Set("sort", opts.Sort)
	q.Set("search_target", opts.Target)
	if opts.Filter != "" {
		q.Set("filter", opts.Filter)
	}
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
	return c.illustPage(ctx, c.pageURL("/v1/search/illust", q))
}

// Illustration fetches one work by ID.
func (c *Client) Illustration(ctx context.Context, id int64) (*Illustration, error) {
	q := url.Values{}
	q.Set("illust_id", strconv.FormatInt(id, 10))
	q.Set("filter", "for_android")
	var r struct {
		Illust Illustration `json:"illust"`
	}
	if err := c.get(ctx, c.pageURL("/v1/illust/detail", q), &r); err != nil {
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
