package pixiv

import (
	"context"
	"net/url"
	"time"
)

// SeriesKind is the kind of series a watchlist holds.
type SeriesKind string

const (
	MangaSeries SeriesKind = "manga"
	NovelSeries SeriesKind = "novel"
)

// SeriesSummary is one series in the watchlist.
type SeriesSummary struct {
	ID                    int64     `json:"id"`
	Title                 string    `json:"title"`
	CoverURL              string    `json:"url"`
	User                  User      `json:"user"`
	PublishedContentCount int       `json:"published_content_count"`
	LastPublishedContent  time.Time `json:"last_published_content_datetime"`
}

// SeriesWatchlist lists the series of one kind the signed-in user follows.
func (c *Client) SeriesWatchlist(ctx context.Context, kind SeriesKind) (*Page[SeriesSummary], error) {
	q := url.Values{}
	c.setFilter(q)
	return c.seriesWatchlistPage(ctx, c.pageURL("/v1/watchlist/"+string(kind), q))
}

// NextSeriesWatchlist loads the page a NextURL from a watchlist points to.
func (c *Client) NextSeriesWatchlist(ctx context.Context, nextURL string) (*Page[SeriesSummary], error) {
	return c.seriesWatchlistPage(ctx, nextURL)
}

func (c *Client) seriesWatchlistPage(ctx context.Context, rawURL string) (*Page[SeriesSummary], error) {
	var r struct {
		Series  []SeriesSummary `json:"series"`
		NextURL string          `json:"next_url"`
	}
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[SeriesSummary]{Items: r.Series, NextURL: r.NextURL}, nil
}
