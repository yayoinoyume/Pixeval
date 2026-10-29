package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// myUsersPage draws one of the signed-in user's own people lists. The list
// arms the first time its page shows, so another page never fetches it ahead
// of a visit, and it loads the current account's list.
func (a *App) myUsersPage(c *ui.Context, st *userListState, first func(context.Context, int64) (*pixiv.Page[pixiv.UserPreview], error), emptyTitle, failKey string) {
	if a.session == nil || a.session.User.ID == 0 {
		a.loginPrompt(c)
		return
	}
	if !st.isArmed() {
		st.setFetch(a.userListFeed(st, int64(a.session.User.ID), first))
		st.arm()
	}
	a.userListPage(c, st, emptyTitle, failKey)
}

// myFollowingPage draws the users the signed-in user follows.
func (a *App) myFollowingPage(c *ui.Context) {
	a.myUsersPage(c, a.myFollowing, a.client.UserFollowing,
		a.t("user.noMyFollowing"), "user.followingLoadFailed")
}

// myFollowersPage draws the users following the signed-in user.
func (a *App) myFollowersPage(c *ui.Context) {
	a.myUsersPage(c, a.myFollowers, a.client.UserFollowers,
		a.t("user.noMyFollowers"), "user.followersLoadFailed")
}

// myPixivUsersListPage draws the signed-in user's MyPixiv users.
func (a *App) myPixivUsersListPage(c *ui.Context) {
	a.myUsersPage(c, a.myPixivUsersList, a.client.UserMyPixiv,
		a.t("user.noMyPixiv"), "user.myPixivLoadFailed")
}
