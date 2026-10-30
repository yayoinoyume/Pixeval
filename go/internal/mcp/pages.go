package mcp

import (
	"context"
	"errors"
	"fmt"

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

// UserPageBackend is a Backend whose user lists expose Pixiv's next-page URL.
// It is optional like PageBackend: a backend that only implements the unpaged
// user methods keeps the old one-page tools and no cursor.
type UserPageBackend interface {
	SearchUsersPage(ctx context.Context, word string) (*pixiv.Page[pixiv.UserPreview], error)
	FollowingUsersPage(ctx context.Context, userID int64) (*pixiv.Page[pixiv.UserPreview], error)
	FollowersPage(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error)
	MyPixivUsersPage(ctx context.Context, userID int64) (*pixiv.Page[pixiv.UserPreview], error)
	RelatedUsersPage(ctx context.Context, userID int64) (*pixiv.Page[pixiv.UserPreview], error)
	RecommendedUsersPage(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error)
	NextUserPreviewsPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.UserPreview], error)
	NextRecommendedUsersPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.UserPreview], error)
}

// NovelPageBackend is a Backend whose novel lists expose Pixiv's next-page URL.
type NovelPageBackend interface {
	SearchNovelsPage(ctx context.Context, word string) (*pixiv.Page[pixiv.Novel], error)
	NextNovelsPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.Novel], error)
}

// CommentPageBackend is a Backend whose comment lists expose Pixiv's
// next-page URL.
type CommentPageBackend interface {
	CommentsPage(ctx context.Context, illustID int64) (*pixiv.Page[pixiv.Comment], error)
	NovelCommentsPage(ctx context.Context, novelID int64) (*pixiv.Page[pixiv.Comment], error)
	NextCommentsPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.Comment], error)
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
	"search_users", "following_users", "followers", "my_pixiv_users",
	"related_users", "recommended_users",
	"search_novels", "comments", "novel_comments",
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
	return pagedResult(cursors, accountID, kind, works, nextURL, limit, worksResult)
}

// pagedUsersResult is pagedWorksResult for user lists.
func pagedUsersResult(cursors *cursorStore, accountID int64, kind string, users []pixiv.UserPreview, nextURL string, limit int) callResult {
	return pagedResult(cursors, accountID, kind, users, nextURL, limit, usersResult)
}

// pagedNovelsResult is pagedWorksResult for novel lists.
func pagedNovelsResult(cursors *cursorStore, accountID int64, kind string, novels []pixiv.Novel, nextURL string, limit int) callResult {
	return pagedResult(cursors, accountID, kind, novels, nextURL, limit, novelsResult)
}

// pagedCommentsResult is pagedWorksResult for comment lists.
func pagedCommentsResult(cursors *cursorStore, accountID int64, kind string, comments []pixiv.Comment, nextURL string, limit int) callResult {
	return pagedResult(cursors, accountID, kind, comments, nextURL, limit, commentsResult)
}

// pagedResult formats one page of a list and mints the cursor more reads. Items
// beyond limit stay in the cursor, so a small limit never skips any, and the
// cursor is minted whenever Pixiv reported more, even when limit cut the page
// before its end.
func pagedResult[T any](cursors *cursorStore, accountID int64, kind string, items []T, nextURL string, limit int, format func([]T) callResult) callResult {
	var cursor string
	switch {
	case len(items) > limit:
		cursor = cursors.Put(kind, accountID, nextURL, items[limit:])
		items = items[:limit]
	case nextURL != "":
		cursor = cursors.Put(kind, accountID, nextURL, nil)
	}
	return withCursor(format(items), cursor)
}

// withCursor is a list result with the cursor more takes, when there is one.
func withCursor(result callResult, cursor string) callResult {
	if cursor != "" {
		result.Content[0].Text += "\n\nnextCursor: " + cursor
	}
	return result
}

// userPageOf fetches one user-list page from a backend that can page users.
func userPageOf(b Backend, fetch func(UserPageBackend) (*pixiv.Page[pixiv.UserPreview], error)) (page *pixiv.Page[pixiv.UserPreview], ok bool, err error) {
	ub, ok := b.(UserPageBackend)
	if !ok {
		return nil, false, nil
	}
	page, err = fetch(ub)
	if page == nil && err == nil {
		page = &pixiv.Page[pixiv.UserPreview]{}
	}
	return page, true, err
}

// novelPageOf fetches one novel-list page from a backend that can page novels.
func novelPageOf(b Backend, fetch func(NovelPageBackend) (*pixiv.Page[pixiv.Novel], error)) (page *pixiv.Page[pixiv.Novel], ok bool, err error) {
	nb, ok := b.(NovelPageBackend)
	if !ok {
		return nil, false, nil
	}
	page, err = fetch(nb)
	if page == nil && err == nil {
		page = &pixiv.Page[pixiv.Novel]{}
	}
	return page, true, err
}

// commentPageOf fetches one comment-list page from a backend that can page
// comments.
func commentPageOf(b Backend, fetch func(CommentPageBackend) (*pixiv.Page[pixiv.Comment], error)) (page *pixiv.Page[pixiv.Comment], ok bool, err error) {
	cb, ok := b.(CommentPageBackend)
	if !ok {
		return nil, false, nil
	}
	page, err = fetch(cb)
	if page == nil && err == nil {
		page = &pixiv.Page[pixiv.Comment]{}
	}
	return page, true, err
}

// nextWorksPage fetches the page a work cursor's next URL points to.
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

// nextUsersPage fetches the next user page. Recommended users come from a
// different endpoint and shape than the other user lists.
func nextUsersPage(ctx context.Context, b Backend, kind, nextURL string) (*pixiv.Page[pixiv.UserPreview], error) {
	ub, ok := b.(UserPageBackend)
	if !ok {
		return nil, errors.New("more is unavailable: this backend cannot page user lists")
	}
	var page *pixiv.Page[pixiv.UserPreview]
	var err error
	if kind == "recommended_users" {
		page, err = ub.NextRecommendedUsersPage(ctx, nextURL)
	} else {
		page, err = ub.NextUserPreviewsPage(ctx, nextURL)
	}
	if err != nil {
		return nil, err
	}
	if page == nil {
		page = &pixiv.Page[pixiv.UserPreview]{}
	}
	return page, nil
}

// nextNovelsPage fetches the page a cursor's next URL points to.
func nextNovelsPage(ctx context.Context, b Backend, nextURL string) (*pixiv.Page[pixiv.Novel], error) {
	nb, ok := b.(NovelPageBackend)
	if !ok {
		return nil, errors.New("more is unavailable: this backend cannot page novel lists")
	}
	page, err := nb.NextNovelsPage(ctx, nextURL)
	if err != nil {
		return nil, err
	}
	if page == nil {
		page = &pixiv.Page[pixiv.Novel]{}
	}
	return page, nil
}

// nextCommentsPage fetches the page a cursor's next URL points to.
func nextCommentsPage(ctx context.Context, b Backend, nextURL string) (*pixiv.Page[pixiv.Comment], error) {
	cb, ok := b.(CommentPageBackend)
	if !ok {
		return nil, errors.New("more is unavailable: this backend cannot page comment lists")
	}
	page, err := cb.NextCommentsPage(ctx, nextURL)
	if err != nil {
		return nil, err
	}
	if page == nil {
		page = &pixiv.Page[pixiv.Comment]{}
	}
	return page, nil
}

// nextPageFailure keeps a taken cursor alive and tells the caller to retry it,
// so a network failure does not cost the caller its place in the list.
func nextPageFailure(cursors *cursorStore, token string, entry cursorEntry, err error) callResult {
	cursors.Restore(token, entry)
	return failure(fmt.Errorf("%w; retry with the same cursor", err))
}
