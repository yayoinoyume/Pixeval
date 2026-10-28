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
