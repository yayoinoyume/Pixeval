package app

import (
	"context"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// avatarSize is the profile image's side in the user page header.
const avatarSize = 96

// The user page's works tabs, in the order they show: illustrations, manga and
// novels.
const (
	userTabIllust = iota
	userTabManga
	userTabNovel
)

// userViewState is one user page: the profile and the user's works tabs. The
// view runs on the main thread and reads the profile through snapshot; the
// load goroutine writes it under mu.
type userViewState struct {
	// feed is the illustration tab's works; mangaFeed and novelFeed are the
	// other tabs'. Each keeps its own pages and place, so switching tabs loses
	// nothing.
	feed      *browseState
	mangaFeed *browseState
	novelFeed *novelFeedState
	// tab is the works tab shown, one of userTabIllust, userTabManga and
	// userTabNovel.
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

// openUser shows userID's page: the works tabs start here, and the profile
// fills in when the fetch lands.
func (a *App) openUser(userID int64) {
	if userID <= 0 {
		return
	}
	illust := &browseState{}
	illust.setFetch(a.userFeed(illust, userID, "illust"))
	illust.arm()
	manga := &browseState{}
	manga.setFetch(a.userFeed(manga, userID, "manga"))
	manga.arm()
	novels := &novelFeedState{}
	novels.setFetch(a.userNovelFeed(novels, userID))
	novels.arm()
	v := &userViewState{feed: illust, mangaFeed: manga, novelFeed: novels, loading: true}
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
					})
				}
			})
		})
		ui.Row(c).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
			textTabs(c, t, &v.tab, a.t("user.tabIllust"), a.t("user.tabManga"), a.t("user.tabNovel"))
		})
		switch v.tab {
		case userTabManga:
			a.browsePage(c, v.mangaFeed)
		case userTabNovel:
			a.novelResultsFor(c, v.novelFeed, a.t("user.noNovels"))
		default:
			a.browsePage(c, v.feed)
		}
	})
}

// userAvatar draws the profile image, the name's initial until it loads, and
// the initial again if the image fails.
func (a *App) userAvatar(c *ui.Context, v *userViewState, s userPageView) {
	t := themeOf(c)
	url := s.user.ProfileImageUrls.Px50x50
	box := ui.Box(c).Width(avatarSize).Height(avatarSize).Shrink(0).
		Radius(t.RadiusFull).Background(t.SurfaceAlt).Clip().
		Label(a.tf("a11y.avatar", s.user.Name))
	if url == "" || s.avatarFailed {
		box.Center().Children(func() {
			ui.Text(c, initialOf(s.user.Name)).FontSize(t.TextDisplay).Bold().TextColor(t.TextMuted)
		})
		return
	}
	box.Children(func() {
		if bmp := a.images.Thumb(url, avatarSize, avatarSize, a.avatarReady(v, url)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover)
		}
	})
}

// avatarReady returns the callback an avatar load calls: a probe that finds
// nothing means the load failed, and the avatar stays a block from then on
// rather than loading again every frame.
func (a *App) avatarReady(v *userViewState, url string) func() {
	return func() {
		if a.images.Thumb(url, avatarSize, avatarSize, nil) != nil {
			return
		}
		a.update(func() {
			v.mu.Lock()
			v.avatarFailed = true
			v.mu.Unlock()
		})
	}
}

// followButton draws the follow state and sends the request its click asks
// for.
func (a *App) followButton(c *ui.Context, v *userViewState, s userPageView) {
	t := themeOf(c)
	label := a.t("user.follow")
	var b ui.Element
	if s.user.IsFollowed {
		b = ui.Button(c, a.t("user.following")).TextColor(t.Accent)
	} else {
		b = ui.PrimaryButton(c, label)
	}
	b = b.Disabled(s.followBusy)
	if b.Clicked() {
		a.toggleFollow(v)
	}
}

// toggleFollow follows or unfollows the user off the main thread. The button
// is disabled while the request runs, and the user's state flips only once it
// succeeds.
func (a *App) toggleFollow(v *userViewState) {
	v.mu.Lock()
	if v.followBusy || a.followFn == nil {
		v.mu.Unlock()
		return
	}
	v.followBusy = true
	on, id := !v.user.IsFollowed, v.user.ID
	v.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		err := a.followFn(ctx, id, on)
		a.update(func() {
			v.mu.Lock()
			v.followBusy = false
			if err == nil {
				v.user.IsFollowed = on
			}
			v.mu.Unlock()
			if err != nil {
				a.notice = a.tf("user.followFailed", err.Error())
			}
		})
	}()
}
