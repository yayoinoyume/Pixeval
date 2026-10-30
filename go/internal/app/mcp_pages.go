package app

import (
	"context"
	"errors"

	"github.com/yayoinoyume/pixeval-go/internal/mcp"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The MCP paged list tools: each returns Pixiv's page with its next_url, so the
// server can hand the client a cursor and more can read the page after. The
// unpaged Backend methods stay for the tools that only need one page.
var (
	_ mcp.PageBackend    = (*App)(nil)
	_ mcp.AccountBackend = (*App)(nil)
)

// AccountID is the signed-in Pixiv account, or 0 when logged out; the MCP
// cursor store binds cursors to it so a switch invalidates them.
func (a *App) AccountID() int64 {
	return a.signedInUserID()
}

// SearchPage returns the first page of a keyword search; the MCP paged search.
func (a *App) SearchPage(ctx context.Context, query string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.SearchIllustrations(ctx, pixiv.SearchOptions{Word: query})
}

// RecommendedPage returns Pixiv's feed for the user; the MCP paged feed.
func (a *App) RecommendedPage(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.Recommended(ctx)
}

// RankingPage returns the top works of mode; the MCP paged ranking.
func (a *App) RankingPage(ctx context.Context, mode string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.Ranking(ctx, pixiv.RankMode(mode), "")
}

// UserIllustsPage returns a user's works of kind; the MCP paged user works.
func (a *App) UserIllustsPage(ctx context.Context, id int64, kind string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.UserIllusts(ctx, id, kind)
}

// UserBookmarksPage returns a user's bookmarks; a zero userID means the
// signed-in account. The MCP paged bookmarks tool.
func (a *App) UserBookmarksPage(ctx context.Context, userID int64, private bool) (*pixiv.Page[pixiv.Illustration], error) {
	if userID <= 0 {
		userID = a.signedInUserID()
	}
	if userID <= 0 {
		return nil, errors.New("not signed in")
	}
	return a.client.BookmarkedIllustrations(ctx, userID, private, 0)
}

// RelatedIllustsPage returns the works Pixiv relates to id; the MCP paged
// related works.
func (a *App) RelatedIllustsPage(ctx context.Context, id int64) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.RelatedIllustrations(ctx, id)
}

// NewWorksPage returns the site-wide newest works of kind; the MCP paged new
// works tool.
func (a *App) NewWorksPage(ctx context.Context, kind string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.NewIllustrations(ctx, pixiv.WorkKind(kind))
}

// FollowingWorksPage returns the public works of followed users; the MCP paged
// following works.
func (a *App) FollowingWorksPage(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.Following(ctx)
}

// MyPixivWorksPage returns works by the account's MyPixiv users; the MCP paged
// MyPixiv works.
func (a *App) MyPixivWorksPage(ctx context.Context, kind string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.MyPixivIllustrations(ctx, pixiv.WorkKind(kind))
}

// NextIllustsPage loads the page a cursor's next URL points to; the MCP more
// tool.
func (a *App) NextIllustsPage(ctx context.Context, nextURL string) (*pixiv.Page[pixiv.Illustration], error) {
	return a.client.NextIllustrations(ctx, nextURL)
}
