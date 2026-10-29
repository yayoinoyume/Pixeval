package pixiv

import (
	"context"
	"net/url"
	"strconv"
)

// SeriesWorks lists the works of an illustration series.
func (c *Client) SeriesWorks(ctx context.Context, seriesID int64) (*SeriesPage, error) {
	q := url.Values{}
	q.Set("illust_series_id", strconv.FormatInt(seriesID, 10))
	c.setFilter(q)
	return c.seriesPage(ctx, c.pageURL("/v1/illust/series", q))
}

// SeriesDetail fetches a series and its works by ID.
func (c *Client) SeriesDetail(ctx context.Context, seriesID int64) (*SeriesPage, error) {
	q := url.Values{}
	c.setFilter(q)
	return c.seriesPage(ctx, c.pageURL("/v1/illust/series/"+strconv.FormatInt(seriesID, 10), q))
}

// NextSeries loads the page a NextURL from a series points to.
func (c *Client) NextSeries(ctx context.Context, nextURL string) (*SeriesPage, error) {
	return c.seriesPage(ctx, nextURL)
}

type seriesList struct {
	Detail  SeriesDetail   `json:"illust_series_detail"`
	Illusts []Illustration `json:"illusts"`
	NextURL string         `json:"next_url"`
}

func (c *Client) seriesPage(ctx context.Context, rawURL string) (*SeriesPage, error) {
	var r seriesList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &SeriesPage{Detail: r.Detail, Illusts: r.Illusts, NextURL: r.NextURL}, nil
}
