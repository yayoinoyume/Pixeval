// Package pixiv is the Pixiv API client: authentication, rate limiting and
// the endpoints the app reads. It does not import the UI.
package pixiv

import "time"

type ImageUrls struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
}

type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

type ProfileImageUrls struct {
	Px16x16 string `json:"px_16x16"`
	Px50x50 string `json:"px_50x50"`
}

type User struct {
	ID               int64            `json:"id"`
	Name             string           `json:"name"`
	Account          string           `json:"account"`
	Comment          string           `json:"comment"`
	IsFollowed       bool             `json:"is_followed"`
	ProfileImageUrls ProfileImageUrls `json:"profile_image_urls"`
}

// UserDetail is a user with the profile block the detail endpoint returns.
type UserDetail struct {
	User    User    `json:"user"`
	Profile Profile `json:"profile"`
}

// Profile is the public profile of a user.
type Profile struct {
	Webpage                    string `json:"webpage"`
	Gender                     string `json:"gender"`
	Birth                      string `json:"birth"`
	BirthDay                   string `json:"birth_day"`
	BirthYear                  int    `json:"birth_year"`
	Region                     string `json:"region"`
	Job                        string `json:"job"`
	TotalFollowUsers           int    `json:"total_follow_users"`
	TotalMyPixivUsers          int    `json:"total_mypixiv_users"`
	TotalIllusts               int    `json:"total_illusts"`
	TotalManga                 int    `json:"total_manga"`
	TotalNovels                int    `json:"total_novels"`
	TotalIllustBookmarksPublic int    `json:"total_illust_bookmarks_public"`
	TotalIllustSeries          int    `json:"total_illust_series"`
	TotalNovelSeries           int    `json:"total_novel_series"`
	BackgroundImageURL         string `json:"background_image_url"`
	TwitterAccount             string `json:"twitter_account"`
	TwitterURL                 string `json:"twitter_url"`
	IsPremium                  bool   `json:"is_premium"`
	IsFollowed                 bool   `json:"is_followed"`
	IsMypixiv                  bool   `json:"is_mypixiv"`
	IsBlocked                  bool   `json:"is_blocked"`
	Comment                    string `json:"comment"`
}

// UserPreview is one entry of a following or follower list.
type UserPreview struct {
	User    User           `json:"user"`
	Illusts []Illustration `json:"illusts"`
	IsMuted bool           `json:"is_muted"`
}

type MetaPage struct {
	ImageUrls ImageUrls `json:"image_urls"`
}

type MetaSinglePage struct {
	OriginalImageURL string `json:"original_image_url"`
}

type Series struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// SeriesDetail is the description of an illustration series.
type SeriesDetail struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	CoverImageURLs   ImageUrls `json:"cover_image_urls"`
	User             User      `json:"user"`
	WorkIllustsCount int       `json:"work_illusts_count"`
	CreateDate       time.Time `json:"create_date"`
	UpdateDate       time.Time `json:"update_date"`
	IsConcluded      bool      `json:"is_concluded"`
	IsOriginal       bool      `json:"is_original"`
}

// SeriesPage is one series with a page of its works.
type SeriesPage struct {
	Detail  SeriesDetail   `json:"-"`
	Illusts []Illustration `json:"-"`
	NextURL string         `json:"next_url"`
}

type Illustration struct {
	ID              int64          `json:"id"`
	Title           string         `json:"title"`
	Type            string         `json:"type"`
	Caption         string         `json:"caption"`
	Restrict        int            `json:"restrict"`
	XRestrict       int            `json:"x_restrict"`
	Tags            []Tag          `json:"tags"`
	User            User           `json:"user"`
	CreateDate      time.Time      `json:"create_date"`
	ImageUrls       ImageUrls      `json:"image_urls"`
	IsBookmarked    bool           `json:"is_bookmarked"`
	TotalBookmarks  int            `json:"total_bookmarks"`
	TotalView       int            `json:"total_view"`
	PageCount       int            `json:"page_count"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	SanityLevel     int            `json:"sanity_level"`
	MetaSinglePage  MetaSinglePage `json:"meta_single_page"`
	MetaPages       []MetaPage     `json:"meta_pages"`
	IllustAIType    int            `json:"illust_ai_type"`
	IllustBookStyle int            `json:"illust_book_style"`
	Series          *Series        `json:"series"`
}

// CoverURL is the largest thumbnail, used in grids.
func (w *Illustration) CoverURL() string {
	switch {
	case w.ImageUrls.Large != "":
		return w.ImageUrls.Large
	case w.ImageUrls.Medium != "":
		return w.ImageUrls.Medium
	default:
		return w.ImageUrls.SquareMedium
	}
}

// ThumbURL is the smallest thumbnail, used where a card is a few hundred
// pixels wide: Medium is about half the pixels of Large to decode.
func (w *Illustration) ThumbURL() string {
	switch {
	case w.ImageUrls.Medium != "":
		return w.ImageUrls.Medium
	case w.ImageUrls.Large != "":
		return w.ImageUrls.Large
	default:
		return w.ImageUrls.SquareMedium
	}
}

// Aspect is width divided by height; 1 when the work has no size.
func (w *Illustration) Aspect() float64 {
	if w.Width <= 0 || w.Height <= 0 {
		return 1
	}
	return float64(w.Width) / float64(w.Height)
}

// PageURL is the original image of one page (0-based).
func (w *Illustration) PageURL(page int) string {
	if page <= 0 && w.MetaSinglePage.OriginalImageURL != "" {
		return w.MetaSinglePage.OriginalImageURL
	}
	if page >= 0 && page < len(w.MetaPages) {
		return w.MetaPages[page].ImageUrls.Original
	}
	return w.ImageUrls.Large
}

// PageURLs are the originals of every page.
func (w *Illustration) PageURLs() []string {
	if w.PageCount <= 1 && w.MetaSinglePage.OriginalImageURL != "" {
		return []string{w.MetaSinglePage.OriginalImageURL}
	}
	urls := make([]string, 0, len(w.MetaPages))
	for _, p := range w.MetaPages {
		urls = append(urls, p.ImageUrls.Original)
	}
	if len(urls) == 0 {
		urls = append(urls, w.CoverURL())
	}
	return urls
}

// Page is one page of a list, with the cursor to the next.
type Page[T any] struct {
	Items   []T    `json:"-"`
	NextURL string `json:"next_url"`
}
