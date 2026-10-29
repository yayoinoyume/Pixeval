package pixiv

import (
	"context"
	"net/url"
)

// SpotlightArticle is one Pixivision article in the spotlight list.
type SpotlightArticle struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	PureTitle   string `json:"pure_title"`
	Thumbnail   string `json:"thumbnail"`
	ArticleURL  string `json:"article_url"`
	PublishDate string `json:"publish_date"`
}

type spotlightList struct {
	Articles []SpotlightArticle `json:"spotlight_articles"`
	NextURL  string             `json:"next_url"`
}

// SpotlightArticles lists the Pixivision articles Pixiv features.
func (c *Client) SpotlightArticles(ctx context.Context) (*Page[SpotlightArticle], error) {
	q := url.Values{}
	q.Set("category", "all")
	c.setFilter(q)
	return c.spotlightPage(ctx, c.pageURL("/v1/spotlight/articles", q))
}

// NextSpotlightArticles loads the page a NextURL points to.
func (c *Client) NextSpotlightArticles(ctx context.Context, nextURL string) (*Page[SpotlightArticle], error) {
	return c.spotlightPage(ctx, nextURL)
}

func (c *Client) spotlightPage(ctx context.Context, rawURL string) (*Page[SpotlightArticle], error) {
	var r spotlightList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[SpotlightArticle]{Items: r.Articles, NextURL: r.NextURL}, nil
}
