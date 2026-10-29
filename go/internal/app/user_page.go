package app

import (
	"context"
	"errors"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
	"github.com/yayoinoyume/pixeval-go/internal/ui/widgets"
)

// avatarSize is the profile image's side in the user page header.
const avatarSize = 96

// userCardAvatar is the avatar's side in a following or followers list.
const userCardAvatar = 40

// The user page's tabs, in the order they show: the works tabs (illustrations,
// manga and novels), then the user's following and followers, and the works
// the user bookmarked.
const (
	userTabIllust = iota
	userTabManga
	userTabNovel
	userTabFollowing
	userTabFollowers
	userTabBookmarks
)

// userViewState is one user page: the profile and the user's works tabs. The
// view runs on the main thread and reads the profile through snapshot; the
// load goroutine writes it under mu.
type userViewState struct {
	// userID is the page's user, whose feeds arm as their tabs show.
	userID int64
	// feed is the illustration tab's works; mangaFeed and novelFeed are the
	// other tabs'. followingFeed and followersFeed are the user's following
	// and followers lists. Each tab keeps its own pages and place, so
	// switching tabs loses nothing.
	feed          *browseState
	mangaFeed     *browseState
	novelFeed     *novelFeedState
	followingFeed *userListState
	followersFeed *userListState
	// bookmarksFeed is the bookmarks tab's public works, bookmarksPrivateFeed
	// its private ones, and bookmarksTab the sub-tab shown, bookmarksPublic or
	// bookmarksPrivate. Only the signed-in user's own page offers the private
	// list.
	bookmarksFeed        *browseState
	bookmarksPrivateFeed *browseState
	bookmarksTab         int
	// tab is the tab shown, one of userTabIllust, userTabManga, userTabNovel,
	// userTabFollowing, userTabFollowers and userTabBookmarks.
	tab int

	mu           sync.Mutex
	user         pixiv.User
	profile      pixiv.Profile
	loading      bool
	err          string
	avatarFailed bool
	followBusy   bool
}

// userPageView is what the user page draws.
type userPageView struct {
	user         pixiv.User
	profile      pixiv.Profile
	loading      bool
	err          string
	avatarFailed bool
	followBusy   bool
}

func (v *userViewState) snapshot() userPageView {
	v.mu.Lock()
	defer v.mu.Unlock()
	return userPageView{
		user:         v.user,
		profile:      v.profile,
		loading:      v.loading,
		err:          v.err,
		avatarFailed: v.avatarFailed,
		followBusy:   v.followBusy,
	}
}

// comment is the profile text, or the user's own comment when the profile
// carries none.
func (uv *userPageView) comment() string {
	if uv.profile.Comment != "" {
		return uv.profile.Comment
	}
	return uv.user.Comment
}

// openUser shows userID's page: only the tab it opens on fetches from here,
// and the others fetch when they first show. The profile fills in when the
// fetch lands.
func (a *App) openUser(userID int64) {
	if userID <= 0 {
		return
	}
	illust := &browseState{}
	illust.setFetch(a.userFeed(illust, userID, "illust"))
	illust.arm()
	v := &userViewState{userID: userID, feed: illust, mangaFeed: &browseState{},
		novelFeed: &novelFeedState{}, followingFeed: &userListState{},
		followersFeed: &userListState{}, bookmarksFeed: &browseState{},
		bookmarksPrivateFeed: &browseState{}, loading: true}
	a.backRoute, a.route, a.userView = a.route, routeUser, v
	go a.loadUser(v, userID)
}

// userFeed maps a works tab to its pages: the first from UserIllusts, the
// later ones from the cursor.
func (a *App) userFeed(st *browseState, userID int64, kind string) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		return a.client.UserIllusts(ctx, userID, kind)
	}
}

// userNovelFeed maps the novel tab to its pages: the first from UserNovels,
// the later ones from the cursor.
func (a *App) userNovelFeed(st *novelFeedState, userID int64) func(context.Context) (*pixiv.Page[pixiv.Novel], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Novel], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextNovels(ctx, next)
		}
		return a.client.UserNovels(ctx, userID)
	}
}

// userBookmarksFeed maps a bookmarks sub-tab to its pages: the first from
// BookmarkedIllustrations, the later ones from the cursor.
func (a *App) userBookmarksFeed(st *browseState, userID int64, private bool) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextIllustrations(ctx, next)
		}
		return a.client.BookmarkedIllustrations(ctx, userID, private, 0)
	}
}

// userIsSelf reports whether userID is the signed-in user, whose page offers
// the private bookmarks list.
func (a *App) userIsSelf(userID int64) bool {
	if a.session == nil || a.session.User.ID == 0 {
		return false
	}
	return int64(a.session.User.ID) == userID
}

// userListView is what a following or followers list draws. users is the
// state's own: callers read it and must not change it.
type userListView struct {
	users   []pixiv.UserPreview
	loading bool
	started bool
	err     string
}

// userListState is one user list: the users loaded so far, the cursor of the
// next page, and the list's place. The view runs on the main thread; the load
// goroutine changes the fields under mu.
type userListState struct {
	mu      sync.Mutex
	users   []pixiv.UserPreview
	next    string
	loading bool
	// started is set once a page loaded, so an empty next ends the list
	// instead of asking for the first page again.
	started bool
	// gen identifies the list the load in flight belongs to: a new search
	// starts the next one and drops the results of the old.
	gen   int
	err   string
	fetch func(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error)
	armed bool
	// failed holds the avatars that did not load, which are not asked for
	// again.
	failed map[string]bool

	// list is the list's place, touched only by the view.
	list ui.ListState
}

// userListFeed maps a user list to its pages: the first from first, the later
// ones from the cursor.
func (a *App) userListFeed(st *userListState, userID int64, first func(context.Context, int64) (*pixiv.Page[pixiv.UserPreview], error)) func(context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error) {
		if next := st.cursor(); next != "" {
			return a.client.NextUserPreviews(ctx, next)
		}
		return first(ctx, userID)
	}
}

// snapshot returns what the view draws.
func (st *userListState) snapshot() userListView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return userListView{
		users:   st.users,
		loading: st.loading,
		started: st.started,
		err:     st.err,
	}
}

// cursor is the next page's URL, empty for the first.
func (st *userListState) cursor() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.next
}

func (st *userListState) setFetch(fetch func(ctx context.Context) (*pixiv.Page[pixiv.UserPreview], error)) {
	st.mu.Lock()
	st.fetch = fetch
	st.mu.Unlock()
}

// isArmed reports whether the list may load.
func (st *userListState) isArmed() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed
}

// arm allows the list to load, once its tab draws it.
func (st *userListState) arm() {
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()
}

// restart starts the list over, dropping the pages loaded until now and the
// results of a load in flight.
func (st *userListState) restart() {
	st.mu.Lock()
	st.gen++
	st.users = nil
	st.next = ""
	st.started = false
	st.err = ""
	st.loading = false
	st.armed = true
	st.mu.Unlock()
}

// canLoad reports whether another page may be fetched: not while one loads,
// not after a failure (the retry button clears it), and not once the list
// ended.
func (st *userListState) canLoad() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.armed && !st.loading && st.err == "" && (!st.started || st.next != "")
}

// retry clears a failure, so the next frame loads the page again.
func (st *userListState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// avatarFailed reports whether a user's avatar failed to load.
func (st *userListState) avatarFailed(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.failed[key]
}

func (st *userListState) failAvatar(key string) {
	st.mu.Lock()
	if st.failed == nil {
		st.failed = map[string]bool{}
	}
	st.failed[key] = true
	st.mu.Unlock()
}

// loadMoreUsers fetches the next page of a user list off the main thread and
// redraws with it.
func (a *App) loadMoreUsers(st *userListState) {
	st.mu.Lock()
	if !st.armed || st.loading || st.err != "" || st.started && st.next == "" {
		st.mu.Unlock()
		return
	}
	st.loading = true
	gen, fetch := st.gen, st.fetch
	st.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		page, err := fetch(ctx)

		st.mu.Lock()
		if gen != st.gen {
			// A new search replaced this list; its page is not wanted.
			st.mu.Unlock()
			return
		}
		st.loading = false
		switch {
		case errors.Is(err, pixiv.ErrNoMore):
			st.next = ""
			st.started = true
		case err != nil:
			st.err = err.Error()
		default:
			st.started = true
			st.users = append(st.users, page.Items...)
			st.next = page.NextURL
		}
		st.mu.Unlock()

		a.update(func() {})
	}()
}

// loadUser fetches the profile off the main thread and leaves it to the next
// frame.
func (a *App) loadUser(v *userViewState, userID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	detail, err := a.client.UserDetail(ctx, userID)
	a.update(func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.loading = false
		if err != nil {
			v.err = err.Error()
			return
		}
		v.user, v.profile = detail.User, detail.Profile
	})
}

// armUserTab lets a tab's feed load the first time its tab shows, so opening a
// user page fetches only the tab it opens on. A tab already armed keeps its
// pages and place.
func (a *App) armUserTab(v *userViewState, tab int) {
	switch tab {
	case userTabManga:
		if v.mangaFeed.isArmed() {
			return
		}
		v.mangaFeed.setFetch(a.userFeed(v.mangaFeed, v.userID, "manga"))
		v.mangaFeed.arm()
	case userTabNovel:
		if v.novelFeed.isArmed() {
			return
		}
		v.novelFeed.setFetch(a.userNovelFeed(v.novelFeed, v.userID))
		v.novelFeed.arm()
	case userTabFollowing:
		if v.followingFeed.isArmed() {
			return
		}
		v.followingFeed.setFetch(a.userListFeed(v.followingFeed, v.userID, a.client.UserFollowing))
		v.followingFeed.arm()
	case userTabFollowers:
		if v.followersFeed.isArmed() {
			return
		}
		v.followersFeed.setFetch(a.userListFeed(v.followersFeed, v.userID, a.client.UserFollowers))
		v.followersFeed.arm()
	case userTabBookmarks:
		st, private := v.bookmarksFeed, false
		if a.userIsSelf(v.userID) && v.bookmarksTab == bookmarksPrivate {
			st, private = v.bookmarksPrivateFeed, true
		}
		if st.isArmed() {
			return
		}
		st.setFetch(a.userBookmarksFeed(st, v.userID, private))
		st.arm()
	}
}

// resetUserBookmarks starts the sub-tab now shown over, so switching
// visibility reloads that list; the page's other feeds keep their pages.
func (a *App) resetUserBookmarks(v *userViewState) {
	st, private := v.bookmarksFeed, false
	if v.bookmarksTab == bookmarksPrivate {
		st, private = v.bookmarksPrivateFeed, true
	}
	st.restart()
	st.setFetch(a.userBookmarksFeed(st, v.userID, private))
}

// userPage draws the profile above the user's works tabs.
func (a *App) userPage(c *ui.Context) {
	v := a.userView
	if v == nil {
		return
	}
	t := themeOf(c)
	s := v.snapshot()
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Padding(t.SpaceL, contentPadding, 0, contentPadding).Children(func() {
			ui.Row(c).FillWidth().Gap(t.SpaceL).Padding(t.SpaceL).AlignItems(ui.Start).
				Background(t.Surface).Radius(t.RadiusL).Border(1, t.Border).Children(func() {
				a.userAvatar(c, v, s)
				ui.Column(c).Grow(1).Gap(t.SpaceXS).MinWidth(0).Children(func() {
					switch {
					case s.err != "":
						ui.Text(c, a.tf("user.loadFailed", s.err)).FontSize(t.TextSmall).TextColor(t.Danger)
					case s.loading:
						ui.Text(c, a.t("common.loading")).FontSize(t.TextSmall).TextColor(t.TextMuted)
					default:
						ui.Text(c, s.user.Name).FontSize(t.TextTitle).Bold().TextColor(t.Text).MaxLines(1)
						if s.user.Account != "" {
							ui.Text(c, "@"+s.user.Account).FontSize(t.TextSmall).TextColor(t.TextMuted)
						}
						if comment := s.comment(); comment != "" {
							ui.Text(c, comment).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(3)
						}
					}
				})
				if s.err == "" && !s.loading {
					ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
						a.followButton(c, v, s)
						a.subscriptionButton(c, subscription.KindUser, s.user.ID, s.user.Name)
						if ui.Button(c, a.t("user.relatedUsers")).Clicked() {
							a.openRelatedUsers(s.user.ID)
						}
					})
				}
			})
		})
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
			textTabs(c, t, &v.tab, a.t("user.tabIllust"), a.t("user.tabManga"),
				a.t("user.tabNovel"), a.t("user.tabFollowing"), a.t("user.tabFollowers"),
				a.t("user.tabBookmarks"))
			switch v.tab {
			case userTabIllust:
				a.downloadLoadedButton(c, v.feed.snapshot().items)
			case userTabManga:
				a.downloadLoadedButton(c, v.mangaFeed.snapshot().items)
			}
		})
		if v.tab == userTabBookmarks && a.userIsSelf(v.userID) {
			ui.Row(c).Padding(0, contentPadding, 0, contentPadding).Children(func() {
				if textTabs(c, t, &v.bookmarksTab,
					a.t("user.bookmarksPublic"), a.t("user.bookmarksPrivate")) {
					a.resetUserBookmarks(v)
				}
			})
		}
		a.armUserTab(v, v.tab)
		switch v.tab {
		case userTabManga:
			a.browsePage(c, v.mangaFeed)
		case userTabNovel:
			a.novelResultsFor(c, v.novelFeed, a.t("user.noNovels"))
		case userTabFollowing:
			a.userListPage(c, v.followingFeed, a.t("user.noFollowing"), "user.followingLoadFailed")
		case userTabFollowers:
			a.userListPage(c, v.followersFeed, a.t("user.noFollowers"), "user.followersLoadFailed")
		case userTabBookmarks:
			st := v.bookmarksFeed
			if a.userIsSelf(v.userID) && v.bookmarksTab == bookmarksPrivate {
				st = v.bookmarksPrivateFeed
			}
			a.userBookmarksPage(c, st, a.t("user.noBookmarks"))
		default:
			a.browsePage(c, v.feed)
		}
	})
}

// userBookmarksPage draws one bookmarks sub-tab, and loads its next page once
// it is at the end. emptyTitle is the title its empty state shows.
func (a *App) userBookmarksPage(c *ui.Context, st *browseState, emptyTitle string) {
	t := themeOf(c)
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, contentPadding).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(18, 18).TextColor(t.Danger)
			ui.Text(c, a.tf("common.loadFailed", v.err)).FontSize(t.TextSmall).
				TextColor(t.Danger).Grow(1)
			if ui.Button(c, a.t("common.retry")).Clicked() {
				st.retry()
			}
		})
		if len(v.items) == 0 {
			// Nothing to show under the error, and no page to load.
			return
		}
	}
	width := a.masonryWidth()
	textHeight := cardTextHeight(t)
	a.cardText = textHeight
	items, tiles := feedTiles(st, v.items, v.tiles, width, textHeight)
	switch {
	case len(items) > 0:
		ui.Column(c).Grow(1).Padding(t.SpaceM, contentPadding, contentPadding, contentPadding).Children(func() {
			widgets.Masonry(c, &st.masonry, tiles, width, cardGap, func(i int) {
				a.browseCard(c, st, items, i)
			})
		})
	case !v.started:
		skeletonFeed(c, width)
	default:
		emptyState(c, iconHeart, emptyTitle, "", nil)
	}
	if st.canLoad() && (len(v.items) == 0 || st.masonry.AtEnd()) {
		a.loadMore(st)
	}
}

// userListPage draws a following or followers list, and loads its next page
// once it is at the end. emptyTitle is the title its empty state shows, and
// failKey the message a failure shows.
func (a *App) userListPage(c *ui.Context, st *userListState, emptyTitle, failKey string) {
	t := themeOf(c)
	v := st.snapshot()
	if v.err != "" {
		ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, contentPadding).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconAlert).Size(18, 18).TextColor(t.Danger)
			ui.Text(c, a.tf(failKey, v.err)).FontSize(t.TextSmall).
				TextColor(t.Danger).Grow(1)
			if ui.Button(c, a.t("common.retry")).Clicked() {
				st.retry()
			}
		})
		if len(v.users) == 0 {
			// Nothing to show under the error, and no page to load.
			return
		}
	}
	switch {
	case len(v.users) > 0:
		ui.List(c, &st.list, len(v.users)).Grow(1).Rows(func(row ui.ListRow) {
			a.userCard(row.Context, st, v.users, row.Index)
		})
	case !v.started:
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, a.t("common.loading")).FontSize(t.TextSmall).TextColor(t.TextMuted)
		})
	default:
		emptyState(c, iconUser, emptyTitle, "", nil)
	}
	if st.canLoad() && (len(v.users) == 0 || st.list.AtEnd()) {
		a.loadMoreUsers(st)
	}
}

// userCard draws one user of a following or followers list: the avatar, the
// name and the account. A click opens that user's page.
func (a *App) userCard(c *ui.Context, st *userListState, users []pixiv.UserPreview, i int) {
	user := users[i].User
	url := user.ProfileImageUrls.Px50x50
	t := themeOf(c)
	row := ui.Row(c).FillWidth().Gap(t.SpaceM).AlignItems(ui.Center).
		Padding(t.SpaceS, contentPadding, t.SpaceS, contentPadding).Radius(t.RadiusM).
		Focusable().Label(user.Name).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openUser(user.ID)
	})
	if row.Hovered() {
		row = row.Background(t.SurfaceAlt)
	}
	row.Children(func() {
		a.avatar(c, userCardAvatar, url, user.Name, st.avatarFailed(url), func() {
			st.failAvatar(url)
		})
		ui.Column(c).Grow(1).Gap(t.SpaceXS).MinWidth(0).Children(func() {
			ui.Text(c, user.Name).FontSize(t.TextBody).FontWeight(600).TextColor(t.Text).MaxLines(1)
			if user.Account != "" {
				ui.Text(c, "@"+user.Account).FontSize(t.TextSmall).TextColor(t.TextMuted).MaxLines(1)
			}
		})
	})
}

// userAvatar draws the profile image, the name's initial until it loads, and
// the initial again if the image fails.
func (a *App) userAvatar(c *ui.Context, v *userViewState, s userPageView) {
	a.avatar(c, avatarSize, s.user.ProfileImageUrls.Px50x50, s.user.Name, s.avatarFailed, func() {
		v.mu.Lock()
		v.avatarFailed = true
		v.mu.Unlock()
	})
}

// avatar draws a user's profile image at side, the name's initial until it
// loads, and the initial again once failed says the load failed. onFail
// records a failed load.
func (a *App) avatar(c *ui.Context, side int, url, name string, failed bool, onFail func()) {
	t := themeOf(c)
	box := ui.Box(c).Width(float32(side)).Height(float32(side)).Shrink(0).
		Radius(t.RadiusFull).Background(t.SurfaceAlt).Clip().
		Label(a.tf("a11y.avatar", name))
	// The header's avatar takes the display size; a list's smaller one takes
	// the title size, which fits it.
	font := t.TextDisplay
	if side < avatarSize {
		font = t.TextTitle
	}
	if url == "" || failed {
		box.Center().Children(func() {
			ui.Text(c, initialOf(name)).FontSize(font).Bold().TextColor(t.TextMuted)
		})
		return
	}
	box.Children(func() {
		if bmp := a.images.Thumb(url, side, side, a.avatarReady(url, side, onFail)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover)
		}
	})
}

// avatarReady returns the callback an avatar load calls: a probe that finds
// nothing means the load failed, and onFail records it so the avatar stays a
// block from then on rather than loading again every frame.
func (a *App) avatarReady(url string, side int, onFail func()) func() {
	return func() {
		if a.images.Thumb(url, side, side, nil) != nil {
			return
		}
		a.update(onFail)
	}
}

// followButton draws the follow state and sends the request its click asks
// for: a user followed already gets one button that unfollows, any other user
// gets a public follow and a private follow.
func (a *App) followButton(c *ui.Context, v *userViewState, s userPageView) {
	t := themeOf(c)
	if s.user.IsFollowed {
		b := ui.Button(c, a.t("user.unfollow")).TextColor(t.Accent).Disabled(s.followBusy)
		if b.Clicked() {
			a.toggleFollow(v)
		}
		return
	}
	if b := ui.PrimaryButton(c, a.t("user.follow")).Disabled(s.followBusy); b.Clicked() {
		a.toggleFollow(v)
	}
	if b := ui.Button(c, a.t("user.followPrivate")).Disabled(s.followBusy); b.Clicked() {
		a.followPrivate(v)
	}
}

// toggleFollow follows or unfollows the user off the main thread.
func (a *App) toggleFollow(v *userViewState) {
	if a.followFn == nil {
		return
	}
	v.mu.Lock()
	on, id := !v.user.IsFollowed, v.user.ID
	v.mu.Unlock()
	a.setFollow(v, on, "user.followFailed", func(ctx context.Context) error {
		return a.followFn(ctx, id, on)
	})
}

// followPrivate privately follows the user off the main thread, so the follow
// stays off the user's profile.
func (a *App) followPrivate(v *userViewState) {
	if a.followPrivateFn == nil {
		return
	}
	v.mu.Lock()
	id := v.user.ID
	v.mu.Unlock()
	a.setFollow(v, true, "user.followPrivateFailed", func(ctx context.Context) error {
		return a.followPrivateFn(ctx, id)
	})
}

// setFollow runs a follow request off the main thread. The buttons are disabled
// while it runs, the user's state becomes on only once it succeeds, and a
// failure leaves the state and toasts failKey.
func (a *App) setFollow(v *userViewState, on bool, failKey string, fn func(ctx context.Context) error) {
	v.mu.Lock()
	if v.followBusy {
		v.mu.Unlock()
		return
	}
	v.followBusy = true
	v.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		err := fn(ctx)
		a.update(func() {
			v.mu.Lock()
			defer v.mu.Unlock()
			// The notice comes before the state change: a test that waits on
			// the state then also sees the message.
			if err != nil {
				a.notice = a.tf(failKey, err.Error())
			}
			v.followBusy = false
			if err == nil {
				v.user.IsFollowed = on
			}
		})
	}()
}
