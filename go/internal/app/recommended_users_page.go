package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// recommendedUsersPage draws the users Pixiv recommends to the signed-in user.
// The list arms the first time the page draws, so another page never fetches
// it ahead of a visit.
func (a *App) recommendedUsersPage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	st := a.recommendedUsers
	if !st.isArmed() {
		st.setFetch(func(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
			if next := st.cursor(); next != "" {
				return a.client.NextRecommendedUsers(ctx, next)
			}
			return a.client.RecommendedUsers(ctx)
		})
		st.arm()
	}
	a.userListPage(c, st, a.t("user.noRecommendedUsers"), "common.loadFailed")
}

// relatedUsersViewState is the related-users page of one user: its list, the
// user page it was opened from, and that page's own way back.
type relatedUsersViewState struct {
	userID    int64
	feed      *userListState
	user      *userViewState
	backRoute route
}

// openRelatedUsers shows the users Pixiv lists beside userID. The user page
// behind it is kept whole, so going back returns to its tabs and place.
func (a *App) openRelatedUsers(userID int64) {
	if userID <= 0 {
		return
	}
	st := &userListState{}
	st.setFetch(a.userListFeed(st, userID, a.client.RelatedUsers))
	st.arm()
	a.relatedUsers = &relatedUsersViewState{
		userID:    userID,
		feed:      st,
		user:      a.userView,
		backRoute: a.backRoute,
	}
	a.backRoute, a.route = a.route, routeUserRelated
}

// relatedUsersPage draws the related users of the page's user.
func (a *App) relatedUsersPage(c *ui.Context) {
	r := a.relatedUsers
	if r == nil {
		return
	}
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.userListPage(c, r.feed, a.t("user.noRelatedUsers"), "common.loadFailed")
}
