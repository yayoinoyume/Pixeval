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
// empty date means yesterday in Japan, whose ranking is already published.
func (c *Client) Ranking(ctx context.Context, mode RankMode, date string) (*Page[Illustration], error) {
	if mode == "" {
		mode = Day
	}
	if date == "" {
		date = defaultRankingDate(time.Now())
	}
	q := url.Values{}
	q.Set("mode", string(mode))
	q.Set("date", date)
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v1/illust/ranking", q))
}

// Following is the public feed of works by followed users.
func (c *Client) Following(ctx context.Context) (*Page[Illustration], error) {
	q := url.Values{}
	q.Set("restrict", "public")
	c.setFilter(q)
	return c.illustPage(ctx, c.pageURL("/v2/illust/follow", q))
}

// defaultRankingDate is the ranking date used when the caller passes none.
// Pixiv publishes a day's ranking only after it ends, so today is still empty.
func defaultRankingDate(now time.Time) string {
	return jstDate(now.AddDate(0, 0, -1))
}

// jstDate formats t as a Pixiv ranking date in Japan.
func jstDate(t time.Time) string {
	return t.In(jst).Format("2006-01-02")
}
