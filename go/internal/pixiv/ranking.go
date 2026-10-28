package pixiv

import (
	"context"
	"net/url"
	"time"
)

// RankMode is the period and audience of the illustration ranking.
type RankMode string

const (
	Day          RankMode = "day"
	Week         RankMode = "week"
	Month        RankMode = "month"
	DayMale      RankMode = "day_male"
	DayFemale    RankMode = "day_female"
	WeekOriginal RankMode = "week_original"
	WeekRookie   RankMode = "week_rookie"
)

// jst is the zone Pixiv uses for ranking dates.
var jst = time.FixedZone("JST", 9*3600)

// Ranking lists the top works of a period. An empty mode means Day, and an
// empty date means today in Japan.
func (c *Client) Ranking(ctx context.Context, mode RankMode, date string) (*Page[Illustration], error) {
	if mode == "" {
		mode = Day
	}
	if date == "" {
		date = jstDate(time.Now())
	}
	q := url.Values{}
	q.Set("mode", string(mode))
	q.Set("date", date)
	q.Set("filter", "for_android")
	return c.illustPage(ctx, c.pageURL("/v1/illust/ranking", q))
}

// Following is the public feed of works by followed users.
func (c *Client) Following(ctx context.Context) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("restrict", "public")
	q.Set("filter", "for_android")
	return c.illustPage(ctx, c.pageURL("/v2/illust/follow", q))
}

// jstDate formats t as a Pixiv ranking date in Japan.
func jstDate(t time.Time) string {
	return t.In(jst).Format("2006-01-02")
}
