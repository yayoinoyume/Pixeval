package app

import (
	"github.com/egoist/mygo/ui"
)

// myPixivUsersViewState is the MyPixiv-users page of one user: its list, the
// user page it was opened from, and that page's own way back.
type myPixivUsersViewState struct {
	userID    int64
	feed      *userListState
	user      *userViewState
	backRoute route
}

// openMyPixivUsers shows the users userID added to MyPixiv. The user page
// behind it is kept whole, so going back returns to its tabs and place.
func (a *App) openMyPixivUsers(userID int64) {
	if userID <= 0 {
		return
	}
	st := &userListState{}
	st.setFetch(a.userListFeed(st, userID, a.client.UserMyPixiv))
	st.arm()
	a.myPixivUsers = &myPixivUsersViewState{
		userID:    userID,
		feed:      st,
		user:      a.userView,
		backRoute: a.backRoute,
	}
	a.backRoute, a.route = a.route, routeUserMyPixiv
}

// myPixivUsersPage draws the MyPixiv users of the page's user.
func (a *App) myPixivUsersPage(c *ui.Context) {
	m := a.myPixivUsers
	if m == nil {
		return
	}
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.userListPage(c, m.feed, a.t("user.noMyPixiv"), "user.myPixivLoadFailed")
}
