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

// The preview row fits as many covers as the page has room for, between three
// and six, from a starting width. A narrow window starts from the smaller
// width: a fifth column then fits at 1024, so the three default sections keep
// their titles inside the window.
const (
	homeCoverBase       = 180
	homeCoverNarrowBase = 150
)

// homeNarrowWidth is the window width at or below which the preview covers
// start from homeCoverNarrowBase.
const homeNarrowWidth = 1100

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

// homeCardTitle names a card in its title row and its full feed's header. A
// ranking card names its period in parentheses, so the header keeps it too.
func (a *App) homeCardTitle(card homeCard) string {
	title := a.homeSourceLabel(card.Source)
	if card.Source == homeSourceRanking {
		return a.tf("home.rankingTitle", title, a.rankModeLabel(card.normalizedRankMode()))
	}
	return title
}

// homeRankModes are the periods a ranking card's edit menu offers, in its
// order.
var homeRankModes = []pixiv.RankMode{
	pixiv.Day, pixiv.Week, pixiv.Month, pixiv.DayMale, pixiv.DayFemale,
	pixiv.WeekOriginal, pixiv.WeekRookie,
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
// mode the page opens with the bar that adds a card and returns to the default
// ones, and each card takes a frame with the controls for it. The cards are
// copied here so a control that changes them shows on the next frame, not
// halfway down this one.
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
			if a.homeEditing {
				a.homeEditBar(c)
			}
			for i := range cards {
				a.homeCardView(c, i, &cards[i])
			}
		})
	})
}

// homeEditButton toggles the page's card editing from the header. While the
// page edits it is the way out, filled with the accent so the state shows in
// the header too.
func (a *App) homeEditButton(c *ui.Context) {
	t := themeOf(c)
	if a.homeEditing {
		if ui.PrimaryButton(c, a.t("home.editDone")).Height(36).Radius(t.RadiusS).
			Padding(0, t.SpaceL).Clicked() {
			a.homeEditing = false
		}
		return
	}
	if ui.Button(c, a.t("home.edit")).Height(36).Radius(t.RadiusS).
		Padding(0, t.SpaceL).Clicked() {
		a.homeEditing = true
	}
}

// homeEditBar is the top of the edit page: it says the edits save as they
// happen, offers the six sources a card can be added for, and the way back to
// the first run's cards.
func (a *App) homeEditBar(c *ui.Context) {
	t := themeOf(c)
	ui.Row(c).FillWidth().Gap(t.SpaceM).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.t("home.editing")).FontSize(t.TextSmall).FontWeight(600).
			TextColor(t.Accent)
		ui.Text(c, a.t("home.editingHint")).FontSize(t.TextSmall).
			TextColor(t.TextMuted).Grow(1).MaxLines(2)
		ui.MenuButton(c, a.t("home.addCard"), func(m *ui.Menu) {
			for _, source := range homeCardSources {
				label := a.homeSourceLabel(source)
				if m.Item(label).Disabled(a.hasHomeCard(source)).Chosen() {
					a.addHomeCard(c, source)
				}
			}
		}).Height(32)
		if ui.Button(c, a.t("home.resetCards")).Height(32).Clicked() {
			a.resetHomeCards(c)
		}
	})
}

// homeCardControls is one card's controls in its title row while editing: move
// it up or down in the page, or take it off. The first and last cards cannot
// move past the page's ends; the last card still offers removal, which says
// why it stays.
func (a *App) homeCardControls(c *ui.Context, i int, card homeCard) {
	t := themeOf(c)
	ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
		if iconButton(c, iconUp, a.homeCardActionLabel("home.moveUp", card),
			32, t.TextMuted, t.SurfaceAlt).Disabled(i == 0).Clicked() {
			a.moveHomeCard(c, i, i-1)
		}
		if iconButton(c, iconDown, a.homeCardActionLabel("home.moveDown", card),
			32, t.TextMuted, t.SurfaceAlt).Disabled(i == len(a.homeCards)-1).Clicked() {
			a.moveHomeCard(c, i, i+1)
		}
		if iconButton(c, iconTrash, a.homeCardActionLabel("home.removeCard", card),
			32, t.Danger, t.SurfaceAlt).Clicked() {
			a.removeHomeCard(c, i)
		}
	})
}

// homeRankModeMenu changes a ranking card's period while editing. The menu
// names the period the card shows, and offers the ones rankModeLabel knows.
func (a *App) homeRankModeMenu(c *ui.Context, i int, card homeCard) {
	mode := card.normalizedRankMode()
	b := ui.MenuButton(c, a.rankModeLabel(mode), func(m *ui.Menu) {
		for _, option := range homeRankModes {
			if m.Item(a.rankModeLabel(option)).Checked(option == mode).Chosen() {
				a.setHomeRankMode(c, i, option)
			}
		}
	}).Height(32)
	b.Label(a.tf("home.actionFor", a.t("ranking.mode"), a.homeCardTitle(card)))
}

// setHomeRankMode puts a ranking card on another period, loads its first page
// again and saves the page.
func (a *App) setHomeRankMode(c *ui.Context, i int, mode pixiv.RankMode) {
	if i < 0 || i >= len(a.homeCards) {
		return
	}
	card := &a.homeCards[i]
	if card.Source != homeSourceRanking || card.normalizedRankMode() == mode {
		return
	}
	card.RankMode = mode
	card.feed = &browseState{}
	a.saveHomeCards(c)
}

// homeCardActionLabel names one card's edit control, for its tooltip, the
// keyboard and assistive technology.
func (a *App) homeCardActionLabel(key string, card homeCard) string {
	return a.tf("home.actionFor", a.t(key), a.homeCardTitle(card))
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

// homeCardView draws one card: its title row with the actions for it, and a
// row of covers. While editing the card takes a temporary frame, and its title
// row carries the controls that move, retarget or remove it.
func (a *App) homeCardView(c *ui.Context, i int, card *homeCard) {
	t := themeOf(c)
	st := card.feed
	if st == nil {
		card.feed = &browseState{}
		st = card.feed
	}
	v := st.snapshot()
	title := a.homeCardTitle(*card)
	frame := ui.Column(c).FillWidth().Gap(t.SpaceM)
	// The frame takes room from the card, so the covers are fitted to what is
	// left inside it.
	inset := float32(0)
	if a.homeEditing {
		frame = frame.Padding(t.SpaceM).Radius(t.RadiusM).
			Background(t.Surface).Border(1, t.Border)
		inset = t.SpaceM + 1
	}
	frame.Children(func() {
		ui.Row(c).FillWidth().Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
			displayText(c, title).FontSize(t.TextTitle).FontWeight(500).
				TextColor(t.Text).Grow(1).MinWidth(0).MaxLines(2)
			if a.homeEditing {
				if card.Source == homeSourceRanking {
					a.homeRankModeMenu(c, i, *card)
				}
				a.homeCardControls(c, i, *card)
			}
			a.homeViewAll(c, *card)
		})
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
			cols, width := homeCoverGrid(c, inset)
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
			cols, width := homeCoverGrid(c, inset)
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

// homeViewAll is a card's section action, at the end of its title row: it
// opens the card's full feed as a new browse feed.
func (a *App) homeViewAll(c *ui.Context, card homeCard) {
	t := themeOf(c)
	label := a.tf("home.actionFor", a.t("home.viewAll"), a.homeCardTitle(card))
	b := ui.ButtonBase(c).Height(32).Radius(t.RadiusS).Padding(0, t.SpaceS).
		Label(label)
	if b.Hovered() {
		b = b.Background(t.SurfaceAlt)
	}
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
// has room for, between three and six, all the same 3:4 shape. inset is the
// room a card's edit frame takes on each side.
func homeCoverGrid(c *ui.Context, inset float32) (int, float32) {
	gap := float32(cardGap)
	base := float32(homeCoverBase)
	if winW, _ := c.Size(); winW <= homeNarrowWidth {
		base = homeCoverNarrowBase
	}
	avail := max(contentWidth(c)-2*inset, 0)
	cols := int((avail + gap) / (base + gap))
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
