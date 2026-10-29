package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// homeCardSource is one feed the home page can show as a card.
type homeCardSource string

// The six work sources a home card may show.
const (
	homeSourceRecommended homeCardSource = "recommended"
	homeSourceFollowing   homeCardSource = "following"
	homeSourceRanking     homeCardSource = "ranking"
	homeSourceNew         homeCardSource = "new"
	homeSourceMyPixiv     homeCardSource = "myPixiv"
	homeSourceBookmarks   homeCardSource = "bookmarks"
)

// homeCard is one card on the home page: its source, the ranking mode when it
// shows a ranking, and the one page its preview loaded.
type homeCard struct {
	Source   homeCardSource `json:"source"`
	RankMode pixiv.RankMode `json:"rankMode,omitempty"`
	feed     *browseState
}

// homeLayout is what home_layout.json holds: the cards in the order the page
// draws them.
type homeLayout struct {
	Cards []homeCard `json:"cards"`
}

// homeFeedView is the full feed a card's "view all" opened. It is a new feed,
// not the card's preview, so the page can page past the first page.
type homeFeedView struct {
	card homeCard
	st   *browseState
}

// homeCardSources are the sources the add-card menu offers, in its order.
var homeCardSources = []homeCardSource{
	homeSourceRecommended, homeSourceFollowing, homeSourceRanking,
	homeSourceNew, homeSourceMyPixiv, homeSourceBookmarks,
}

// homeCoverBase is the preview cover width the column count starts from. The
// row fits as many covers as the page has room for, between three and six.
const homeCoverBase = 180

// defaultHomeLayout is the first run's home page.
func defaultHomeLayout() []homeCard {
	cards := []homeCard{
		{Source: homeSourceRecommended},
		{Source: homeSourceFollowing},
		{Source: homeSourceRanking, RankMode: pixiv.Day},
	}
	for i := range cards {
		cards[i].feed = &browseState{}
	}
	return cards
}

// homeLayoutPath is where the home layout lives, beside the settings.
func homeLayoutPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "home_layout.json"), nil
}

// loadHomeLayout reads the saved cards, or the default when there are none or
// the file cannot be understood. A damaged file never keeps the page empty.
func loadHomeLayout() []homeCard {
	path, err := homeLayoutPath()
	if err != nil {
		log.Printf("home layout: %v", err)
		return defaultHomeLayout()
	}
	cards, err := loadHomeLayoutFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("home layout: %v", err)
		}
		return defaultHomeLayout()
	}
	return cards
}

// loadHomeLayoutFile reads one home layout file and fills in the feeds the
// view needs. Unknown sources are dropped.
func loadHomeLayoutFile(path string) ([]homeCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var layout homeLayout
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, err
	}
	cards := make([]homeCard, 0, len(layout.Cards))
	for _, card := range layout.Cards {
		if !card.Source.valid() {
			continue
		}
		card.RankMode = card.normalizedRankMode()
		card.feed = &browseState{}
		cards = append(cards, card)
	}
	if len(cards) == 0 {
		return nil, errors.New("no cards")
	}
	return cards, nil
}

// saveHomeLayout writes the cards in one step, so a crash leaves the old file
// rather than half of a new one.
func saveHomeLayout(cards []homeCard) error {
	path, err := homeLayoutPath()
	if err != nil {
		return err
	}
	return saveHomeLayoutFile(path, cards)
}

// saveHomeLayoutFile saves a layout to path with the file mode the settings use.
func saveHomeLayoutFile(path string, cards []homeCard) error {
	layout := homeLayout{Cards: make([]homeCard, 0, len(cards))}
	for _, card := range cards {
		card.RankMode = card.normalizedRankMode()
		card.feed = nil
		layout.Cards = append(layout.Cards, card)
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".home_layout-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

// valid reports whether the card source is one the page knows.
func (s homeCardSource) valid() bool {
	switch s {
	case homeSourceRecommended, homeSourceFollowing, homeSourceRanking,
		homeSourceNew, homeSourceMyPixiv, homeSourceBookmarks:
		return true
	}
	return false
}

// normalizedRankMode keeps a ranking card on a period the client knows and
// clears the mode for every other source.
func (c homeCard) normalizedRankMode() pixiv.RankMode {
	if c.Source != homeSourceRanking {
		return ""
	}
	switch c.RankMode {
	case pixiv.Day, pixiv.Week, pixiv.Month, pixiv.DayMale, pixiv.DayFemale,
		pixiv.WeekOriginal, pixiv.WeekRookie:
		return c.RankMode
	}
	return pixiv.Day
}

// homeSourceLabel is the card's name: the source's, with the period a ranking
// card shows.
func (a *App) homeSourceLabel(source homeCardSource) string {
	return a.t("home.source." + string(source))
}

// homeCardTitle names a card in its title row and its full feed's header.
func (a *App) homeCardTitle(card homeCard) string {
	title := a.homeSourceLabel(card.Source)
	if card.Source == homeSourceRanking {
		return title + " · " + a.rankModeLabel(card.normalizedRankMode())
	}
	return title
}

// rankModeLabel is the label a ranking card shows for a ranking mode.
func (a *App) rankModeLabel(mode pixiv.RankMode) string {
	switch mode {
	case pixiv.Week:
		return a.t("ranking.week")
	case pixiv.Month:
		return a.t("ranking.month")
	case pixiv.DayMale:
		return a.t("ranking.dayMale")
	case pixiv.DayFemale:
		return a.t("ranking.dayFemale")
	case pixiv.WeekOriginal:
		return a.t("ranking.weekOriginal")
	case pixiv.WeekRookie:
		return a.t("ranking.weekRookie")
	default:
		return a.t("ranking.day")
	}
}

// armHomeCards gives every card its source's first page and lets it load, once.
func (a *App) armHomeCards() {
	if a.session == nil {
		return
	}
	for i := range a.homeCards {
		card := &a.homeCards[i]
		if card.feed == nil {
			card.feed = &browseState{}
		}
		if card.feed.isArmed() {
			continue
		}
		card.feed.setFetch(a.homeCardFetch(card))
		card.feed.arm()
	}
}

// homeCardFetch maps a card to its first page. Later pages come from the
// feed's cursor, though a card preview only asks for the first.
func (a *App) homeCardFetch(card *homeCard) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return a.homeSourceFetch(card.feed, card.Source, card.RankMode)
}

// homeSourceFetch pages one home source: the first page by source, then the
// cursor each later page returns.
func (a *App) homeSourceFetch(st *browseState, source homeCardSource, mode pixiv.RankMode) func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
	return func(ctx context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		if next := st.cursor(); next != "" {
			if source == homeSourceBookmarks {
				return a.bookmarksPageFn(ctx, false, next)
			}
			return a.client.NextIllustrations(ctx, next)
		}
		switch source {
		case homeSourceFollowing:
			return a.client.Following(ctx)
		case homeSourceRanking:
			return a.client.Ranking(ctx, mode, "")
		case homeSourceNew:
			return a.client.NewIllustrations(ctx, pixiv.Illust)
		case homeSourceMyPixiv:
			return a.client.MyPixivIllustrations(ctx, pixiv.Illust)
		case homeSourceBookmarks:
			return a.bookmarksPageFn(ctx, false, "")
		default:
			return a.client.Recommended(ctx)
		}
	}
}

// homePage draws the cards as one column, each with its first page. A
// signed-out user is asked to sign in instead of fetching anything. In edit
// mode each card offers its controls and the page ends with the ways to add
// a card and to return to the default cards. The cards are copied here so a
// control that changes them shows on the next frame, not halfway down this
// one.
func (a *App) homePage(c *ui.Context) {
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.armHomeCards()
	t := themeOf(c)
	cards := a.homeCards
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(t.SpaceL, contentPadding, t.Space2XL, contentPadding).
			Gap(t.Space2XL).Children(func() {
			for i := range cards {
				a.homeCardView(c, i, &cards[i])
			}
			if a.homeEditing {
				a.homeEditTrailer(c)
			}
		})
	})
}

// homeEditButton toggles the page's card editing from the header, and names
// the way out while editing.
func (a *App) homeEditButton(c *ui.Context) {
	t := themeOf(c)
	label := a.t("home.edit")
	if a.homeEditing {
		label = a.t("home.editDone")
	}
	if ui.Button(c, label).Height(36).Radius(t.RadiusS).
		Padding(0, t.SpaceL).Clicked() {
		a.homeEditing = !a.homeEditing
	}
}

// homeCardControls is one card's edit row: move it up or down in the page, or
// take it off. The first and last cards cannot move past the page's ends; the
// last card still offers removal, which says why it stays.
func (a *App) homeCardControls(c *ui.Context, i int, card homeCard) {
	t := themeOf(c)
	ui.Row(c).FillWidth().Gap(t.SpaceXS).Justify(ui.End).AlignItems(ui.Center).
		Children(func() {
			up := iconButton(c, iconUp, a.homeCardActionLabel("home.moveUp", card),
				28, t.TextMuted, t.SurfaceAlt).Disabled(i == 0)
			if up.Clicked() {
				a.moveHomeCard(c, i, i-1)
			}
			down := iconButton(c, iconDown, a.homeCardActionLabel("home.moveDown", card),
				28, t.TextMuted, t.SurfaceAlt).Disabled(i == len(a.homeCards)-1)
			if down.Clicked() {
				a.moveHomeCard(c, i, i+1)
			}
			if iconButton(c, iconClose, a.homeCardActionLabel("home.removeCard", card),
				28, t.Danger, t.SurfaceAlt).Clicked() {
				a.removeHomeCard(c, i)
			}
		})
}

// homeEditTrailer is the end of the edit page: the six sources a card can be
// added for, and the way back to the first run's cards.
func (a *App) homeEditTrailer(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).FillWidth().Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
		ui.MenuButton(c, a.t("home.addCard"), func(m *ui.Menu) {
			for _, source := range homeCardSources {
				label := a.homeSourceLabel(source)
				if m.Item(label).Disabled(a.hasHomeCard(source)).Chosen() {
					a.addHomeCard(c, source)
				}
			}
		})
		if ui.Button(c, a.t("home.resetCards")).Clicked() {
			a.resetHomeCards(c)
		}
	})
}

// homeCardActionLabel names one card's edit control, for its tooltip, the
// keyboard and assistive technology.
func (a *App) homeCardActionLabel(key string, card homeCard) string {
	return a.t(key) + " · " + a.homeCardTitle(card)
}

// hasHomeCard reports whether the page already shows source.
func (a *App) hasHomeCard(source homeCardSource) bool {
	for i := range a.homeCards {
		if a.homeCards[i].Source == source {
			return true
		}
	}
	return false
}

// moveHomeCard puts the card at i at j and saves the order. The page draws
// the previous frame's cards, so the move shows on the next frame.
func (a *App) moveHomeCard(c *ui.Context, i, j int) {
	if i < 0 || j < 0 || i >= len(a.homeCards) || j >= len(a.homeCards) || i == j {
		return
	}
	next := make([]homeCard, len(a.homeCards))
	copy(next, a.homeCards)
	next[i], next[j] = next[j], next[i]
	a.homeCards = next
	a.saveHomeCards(c)
}

// removeHomeCard takes the card at i off the page and saves the rest. The
// last card stays, and a toast says why.
func (a *App) removeHomeCard(c *ui.Context, i int) {
	if len(a.homeCards) <= 1 {
		c.Toast(a.t("home.lastCard"))
		return
	}
	if i < 0 || i >= len(a.homeCards) {
		return
	}
	next := make([]homeCard, 0, len(a.homeCards)-1)
	next = append(next, a.homeCards[:i]...)
	next = append(next, a.homeCards[i+1:]...)
	a.homeCards = next
	a.saveHomeCards(c)
}

// addHomeCard appends source's card and saves the page. A source already on
// the page is not added twice; a ranking card starts on the day ranking.
func (a *App) addHomeCard(c *ui.Context, source homeCardSource) {
	if !source.valid() || a.hasHomeCard(source) {
		return
	}
	card := homeCard{Source: source, feed: &browseState{}}
	if source == homeSourceRanking {
		card.RankMode = pixiv.Day
	}
	a.homeCards = append(a.homeCards, card)
	a.saveHomeCards(c)
}

// resetHomeCards puts back the first run's cards and saves them.
func (a *App) resetHomeCards(c *ui.Context) {
	a.homeCards = defaultHomeLayout()
	a.saveHomeCards(c)
}

// saveHomeCards writes an edit now, so the saved page always matches what the
// page shows. A failed save is reported and leaves the edit on screen.
func (a *App) saveHomeCards(c *ui.Context) {
	if err := saveHomeLayout(a.homeCards); err != nil {
		log.Printf("home layout: %v", err)
		c.Toast(a.tf("settings.saveFailed", err.Error()))
	}
}

// homeCardView draws one card: its edit row while editing, its title, a row
// of covers and the way to its full page.
func (a *App) homeCardView(c *ui.Context, i int, card *homeCard) {
	t := themeOf(c)
	st := card.feed
	if st == nil {
		card.feed = &browseState{}
		st = card.feed
	}
	v := st.snapshot()
	title := a.homeCardTitle(*card)
	ui.Column(c).FillWidth().Gap(t.SpaceM).Children(func() {
		if a.homeEditing {
			a.homeCardControls(c, i, *card)
		}
		displayText(c, title).FontSize(t.TextTitle).FontWeight(500).
			TextColor(t.Text).MaxLines(1)
		switch {
		case v.err != "":
			ui.Row(c).Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconAlert).Size(16, 16).TextColor(t.Danger)
				ui.Text(c, a.tf("common.loadFailed", v.err)).FontSize(t.TextSmall).
					TextColor(t.Danger).Grow(1).MaxLines(2)
				if ui.Button(c, a.t("common.retry")).Clicked() {
					st.retry()
				}
			})
		case len(v.items) > 0:
			cols, width := homeCoverGrid(c)
			items := v.items
			if len(items) > cols {
				items = items[:cols]
			}
			ui.Row(c).FillWidth().Gap(cardGap).AlignItems(ui.Start).Children(func() {
				for i := range items {
					a.homeCover(c, st, items, i, width)
				}
			})
		case !v.started:
			cols, width := homeCoverGrid(c)
			ui.Row(c).FillWidth().Gap(cardGap).AlignItems(ui.Start).Children(func() {
				for i := 0; i < cols; i++ {
					ui.Box(c).Width(width).Height(width * 4 / 3).
						Radius(t.RadiusXS).Background(t.SurfaceAlt)
				}
			})
		default:
			ui.Column(c).FillWidth().Padding(t.SpaceXL, 0).AlignItems(ui.Center).
				Children(func() {
					ui.Icon(c, iconImage).Size(22, 22).TextColor(t.TextMuted)
					ui.Text(c, a.t("home.cardsEmpty")).FontSize(t.TextBody).
						TextColor(t.Text).Padding(t.SpaceS, 0, 0, 0)
					ui.Text(c, a.t("home.cardsEmptyHint")).FontSize(t.TextSmall).
						TextColor(t.TextMuted).Padding(t.SpaceXS, 0, 0, 0)
				})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			a.homeViewAll(c, *card)
		})
	})
	if st.canLoad() && !v.started {
		a.loadMore(st)
	}
}

// homeCover draws one preview: the cover alone, opening the viewer on its
// work. The row's cells are the works passed in, so a click opens that row's
// copy.
func (a *App) homeCover(c *ui.Context, st *browseState, items []pixiv.Illustration, i int, width float32) {
	work := items[i]
	key := work.ThumbURL()
	t := themeOf(c)
	box := ui.Box(c).Width(width).Height(width * 4 / 3).Radius(t.RadiusXS).
		Background(t.SurfaceAlt).Clip().Focusable().
		Label(a.tf("a11y.card", work.Title, work.User.Name)).
		Cursor(ui.CursorPointer).OnClick(func() {
		a.openViewer(&items[i], 0)
	})
	if box.Hovered() {
		box = box.Background(t.SurfaceHover)
	}
	box.Children(func() {
		if st.imageFailed(key) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, a.t("common.coverFailed")).FontSize(t.TextTiny).TextColor(t.TextMuted)
			})
			return
		}
		if bmp := a.images.Thumb(key, coverWidth, coverHeight, a.imageReady(st, key)); bmp != nil {
			ui.Image(c, bmp).Fill().Fit(ui.Cover).Label(a.tf("a11y.cover", work.Title))
		}
	})
}

// homeViewAll opens a card's full feed as a new browse feed.
func (a *App) homeViewAll(c *ui.Context, card homeCard) {
	t := themeOf(c)
	title := a.homeCardTitle(card)
	b := ui.ButtonBase(c).Padding(0).Radius(0).
		Label(a.t("home.viewAll") + " · " + title)
	b.Children(func() {
		ui.Text(c, a.t("home.viewAll")).FontSize(t.TextSmall).TextColor(t.Accent)
	})
	if b.Clicked() {
		a.openHomeFeed(card)
	}
}

// openHomeFeed shows a card's source on its own page, from a new feed so the
// preview keeps its own first page.
func (a *App) openHomeFeed(card homeCard) {
	st := &browseState{}
	st.setFetch(a.homeSourceFetch(st, card.Source, card.normalizedRankMode()))
	st.arm()
	a.feed = &homeFeedView{card: card, st: st}
	a.backRoute = routeHome
	a.goRoute(routeFeed)
}

// feedPage draws a home card's full feed.
func (a *App) feedPage(c *ui.Context) {
	if a.feed == nil {
		a.goRoute(routeHome)
		return
	}
	if a.session == nil {
		a.loginPrompt(c)
		return
	}
	a.browsePage(c, a.feed.st)
}

// homeCoverGrid fits the preview row to the page: as many covers as the width
// has room for, between three and six, all the same 3:4 shape.
func homeCoverGrid(c *ui.Context) (int, float32) {
	gap := float32(cardGap)
	avail := contentWidth(c)
	cols := int((avail + gap) / (homeCoverBase + gap))
	cols = min(max(cols, 3), 6)
	return cols, (avail - gap*float32(cols-1)) / float32(cols)
}

// homeCardFeeds are the cards' preview feeds, for dropping blocked works from
// what is already loaded. The full feed page, when open, is its own state and
// the caller adds it too.
func (a *App) homeCardFeeds() []*browseState {
	feeds := make([]*browseState, 0, len(a.homeCards))
	for i := range a.homeCards {
		if a.homeCards[i].feed != nil {
			feeds = append(feeds, a.homeCards[i].feed)
		}
	}
	return feeds
}
