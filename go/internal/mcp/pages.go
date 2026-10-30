package mcp

import (
	"context"
	"errors"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// PageBackend is a Backend whose work lists expose Pixiv's next-page URL, so
// the list tools can hand out a cursor and more can read the page after. A
// Backend that does not implement it keeps the old one-page tools and no
// cursor, rather than a cursor that leads nowhere.
type PageBackend interface {
	SearchPage(ctx context.Context, query string) (*pixiv.Page[pixiv.Illustration], error)
	RecommendedPage(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error)
	RankingPage(ctx context.Context, mode string) (*pixiv.Page[pixiv.Illustration], error)
	UserIllustsPage(ctx context.Context, id int64, kind string) (*pixiv.Page[pixiv.Illustration], error)
	UserBookmarksPage(ctx context.Context, userID int64, private bool) (*pixiv.Page[pixiv.Illustration], error)
	RelatedIllustsPage(ctx context.Context, id int64) (*pixiv.Page[pixiv.Illustration], error)
	NewWorksPage(ctx context.Context, kind string) (*pixiv.Page[pixiv.Illustration], error)
	FollowingWorksPage(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error)
	MyPixivWorksPage(ctx context.Context, kind string) (*pixiv.Page[pixiv.Illustration], error)
	NextIllustsPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.Illustration], error)
}

// AccountBackend is a Backend that knows which Pixiv account is signed in, so
// a cursor cannot be replayed after an account switch. A Backend that does not
// implement it gets no account binding, and its cursors live for the TTL only.
type AccountBackend interface {
	AccountID() int64
}

// pagedTools are the list tools that return a cursor when the backend can
// page. They are also what capabilities reports as pageable.
var pagedTools = []string{
	"search_illustrations", "recommended", "ranking",
	"user_illusts", "user_bookmarks", "related_illustrations",
	"new_works", "following_works", "my_pixiv_works",
}

// accountIDOf is the signed-in Pixiv account, or 0 when b does not report one.
func accountIDOf(b Backend) int64 {
	if ab, ok := b.(AccountBackend); ok {
		return ab.AccountID()
	}
	return 0
}

// pageOf fetches one page from a backend that can page. ok is false when the
// backend keeps the old unpaged behaviour.
func pageOf(b Backend, fetch func(PageBackend) (*pixiv.Page[pixiv.Illustration], error)) (page *pixiv.Page[pixiv.Illustration], ok bool, err error) {
	pb, ok := b.(PageBackend)
	if !ok {
		return nil, false, nil
	}
	page, err = fetch(pb)
	if page == nil && err == nil {
		page = &pixiv.Page[pixiv.Illustration]{}
	}
	return page, true, err
}

// pagedWorksResult formats one page of works and mints the cursor more reads.
// Works beyond limit stay in the cursor, so a small limit never skips works,
// and the cursor is minted whenever Pixiv reported more, even when limit cut
// the page before its end.
func pagedWorksResult(cursors *cursorStore, accountID int64, kind string, works []pixiv.Illustration, nextURL string, limit int) callResult {
	var cursor string
	switch {
	case len(works) > limit:
		cursor = cursors.Put(kind, accountID, nextURL, works[limit:])
		works = works[:limit]
	case nextURL != "":
		cursor = cursors.Put(kind, accountID, nextURL, nil)
	}
	return worksWithCursor(works, cursor)
}

// worksWithCursor is worksResult with the cursor more takes, when there is one.
func worksWithCursor(works []pixiv.Illustration, cursor string) callResult {
	result := worksResult(works)
	if cursor != "" {
		result.Content[0].Text += "\n\nnextCursor: " + cursor
	}
	return result
}

// nextWorksPage fetches the page a cursor's next URL points to; the more tool
// uses it for every pageable list.
func nextWorksPage(ctx context.Context, b Backend, nextURL string) (*pixiv.Page[pixiv.Illustration], error) {
	pb, ok := b.(PageBackend)
	if !ok {
		return nil, errors.New("more is unavailable: this backend cannot page lists")
	}
	page, err := pb.NextIllustsPage(ctx, nextURL)
	if err != nil {
		return nil, err
	}
	if page == nil {
		page = &pixiv.Page[pixiv.Illustration]{}
	}
	return page, nil
}
