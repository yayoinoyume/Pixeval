package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// homeNoNetwork fails every request, so a test that draws the home page never
// reaches Pixiv.
type homeNoNetwork struct{}

func (homeNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("home test: no network")
}

// newHomeApp is an English app signed in, with images but no network.
func newHomeApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.client.HTTP().Transport = homeNoNetwork{}
	a.images = imagex.New(nil, "")
	a.session = &pixiv.Token{User: pixiv.TokenUser{ID: 1, Name: "测试用户"}}
	a.client.SetToken(a.session)
	return a
}

// armHomeCard gives a card's preview a page and lets it load.
func armHomeCard(st *browseState, items []pixiv.Illustration) {
	st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: items}, nil
	})
	st.arm()
}

// TestHomeDefaultCards checks a fresh layout is the three cards the page
// promises, in order, with the day ranking.
func TestHomeDefaultCards(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	want := []homeCardSource{homeSourceRecommended, homeSourceFollowing, homeSourceRanking}
	if len(a.homeCards) != len(want) {
		t.Fatalf("cards %d, want %d", len(a.homeCards), len(want))
	}
	for i, source := range want {
		if a.homeCards[i].Source != source {
			t.Errorf("card %d source %q, want %q", i, a.homeCards[i].Source, source)
		}
		if a.homeCards[i].feed == nil {
			t.Errorf("card %d has no preview feed", i)
		}
	}
	if mode := a.homeCards[2].RankMode; mode != pixiv.Day {
		t.Errorf("ranking card mode %q, want %q", mode, pixiv.Day)
	}
	if got := a.homeCardTitle(a.homeCards[2]); got != "排行榜 · 日榜" {
		t.Errorf("ranking title %q, want 排行榜 · 日榜", got)
	}
}

// TestHomeCardsLoadOnePage checks every card asks for its first page once and
// never pages past it.
func TestHomeCardsLoadOnePage(t *testing.T) {
	a := newHomeApp(t)
	var mu sync.Mutex
	calls := make([]int, len(a.homeCards))
	for i := range a.homeCards {
		items := fakeWorks(i*100, 2)
		st := a.homeCards[i].feed
		st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
			mu.Lock()
			calls[i]++
			mu.Unlock()
			return &pixiv.Page[pixiv.Illustration]{
				Items:   items,
				NextURL: "https://example.invalid/next",
			}, nil
		})
		st.arm()
	}
	tt := ui.NewTester(a.View, 1024, 720)
	for i := range a.homeCards {
		waitBrowse(t, a.homeCards[i].feed, "load the card's first page",
			func(v browseView) bool { return len(v.items) == 2 })
	}
	tt.Frame()
	tt.Frame()
	mu.Lock()
	got := append([]int(nil), calls...)
	mu.Unlock()
	for i, n := range got {
		if n != 1 {
			t.Errorf("card %d fetched %d times, want 1", i, n)
		}
	}
	if !tt.HasText("排行榜 · 日榜") {
		t.Fatalf("the ranking card title is missing: %q", tt.Texts())
	}
}

// TestHomeViewAllOpensFullFeed checks a card's View all opens its source on a
// new feed, with the source name in the header, and Escape returns home.
func TestHomeViewAllOpensFullFeed(t *testing.T) {
	a := newHomeApp(t)
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, fakeWorks(i*100, 2))
	}
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()

	card := a.homeCards[0]
	label := a.t("home.viewAll") + " · " + a.homeCardTitle(card)
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
	if a.route != routeFeed {
		t.Fatalf("route %q after View all, want %q", a.route, routeFeed)
	}
	if a.feed == nil {
		t.Fatal("the full feed is nil")
	}
	if a.feed.card.Source != card.Source {
		t.Errorf("full feed source %q, want %q", a.feed.card.Source, card.Source)
	}
	if a.feed.st == card.feed {
		t.Fatal("the full feed reused the preview's browse state")
	}
	tt.Frame()
	if !tt.HasText(a.homeSourceLabel(card.Source)) {
		t.Errorf("the feed header does not name %q: %q", a.homeSourceLabel(card.Source), tt.Texts())
	}

	tt.Key(0, ui.KeyEscape)
	if a.route != routeHome {
		t.Errorf("route %q after Escape, want %q", a.route, routeHome)
	}
	if a.feed != nil {
		t.Error("the full feed survived going back home")
	}
}

// TestHomeSignedOutShowsLogin checks a signed-out home asks for a sign-in and
// fetches no card.
func TestHomeSignedOutShowsLogin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.images = imagex.New(nil, "")
	var mu sync.Mutex
	calls := 0
	for i := range a.homeCards {
		st := a.homeCards[i].feed
		st.setFetch(func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 2)}, nil
		})
		st.arm()
	}
	tt := ui.NewTester(a.View, 1024, 720)
	tt.Frame()
	if !tt.HasText(a.t("login.prompt")) {
		t.Fatalf("the login prompt is missing: %q", tt.Texts())
	}
	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 0 {
		t.Fatalf("cards fetched %d pages while signed out", got)
	}
}

// TestHomeLayoutBadJSONFallsBack checks a damaged layout file leaves the three
// default cards rather than an empty page.
func TestHomeLayoutBadJSONFallsBack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := homeLayoutPath()
	if err != nil {
		t.Fatalf("homeLayoutPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"cards":[{"source":`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cards := loadHomeLayout()
	if len(cards) != 3 || cards[0].Source != homeSourceRecommended ||
		cards[1].Source != homeSourceFollowing || cards[2].Source != homeSourceRanking {
		t.Fatalf("bad layout loaded as %+v", cards)
	}
	if cards[2].RankMode != pixiv.Day || cards[2].feed == nil {
		t.Fatalf("default ranking card %+v", cards[2])
	}
}

// TestHomeLayoutSavesOnlySourceAndMode checks the file keeps what the card is,
// not its loaded works, and is readable by its owner only.
func TestHomeLayoutSavesOnlySourceAndMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := homeLayoutPath()
	if err != nil {
		t.Fatalf("homeLayoutPath: %v", err)
	}
	cards := []homeCard{
		{Source: homeSourceFollowing, RankMode: pixiv.Week, feed: &browseState{}},
		{Source: homeSourceRanking, RankMode: pixiv.DayFemale, feed: &browseState{}},
	}
	if err := saveHomeLayoutFile(path, cards); err != nil {
		t.Fatalf("saveHomeLayoutFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode %o, want 600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw struct {
		Cards []map[string]any `json:"cards"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(raw.Cards) != 2 {
		t.Fatalf("saved %d cards, want 2", len(raw.Cards))
	}
	if len(raw.Cards[0]) != 1 || raw.Cards[0]["source"] != string(homeSourceFollowing) {
		t.Errorf("following card saved as %v", raw.Cards[0])
	}
	if len(raw.Cards[1]) != 2 || raw.Cards[1]["rankMode"] != string(pixiv.DayFemale) {
		t.Errorf("ranking card saved as %v", raw.Cards[1])
	}
	loaded, err := loadHomeLayoutFile(path)
	if err != nil {
		t.Fatalf("loadHomeLayoutFile: %v", err)
	}
	if len(loaded) != 2 || loaded[1].RankMode != pixiv.DayFemale || loaded[1].feed == nil {
		t.Fatalf("loaded %+v", loaded)
	}
}

// TestHomeAccountSwitchClearsCards checks switching accounts keeps the layout
// and drops every work the previous account loaded.
func TestHomeAccountSwitchClearsCards(t *testing.T) {
	a := newHomeApp(t)
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, fakeWorks(i*100, 2))
	}
	tt := ui.NewTester(a.View, 1024, 720)
	for i := range a.homeCards {
		waitBrowse(t, a.homeCards[i].feed, "load the card", func(v browseView) bool { return len(v.items) == 2 })
	}
	tt.Frame()
	sources := make([]homeCardSource, len(a.homeCards))
	for i := range a.homeCards {
		sources[i] = a.homeCards[i].Source
	}

	a.applySession(&pixiv.Token{User: pixiv.TokenUser{ID: 2, Name: "另一个用户"}})
	if len(a.homeCards) != len(sources) {
		t.Fatalf("cards %d after the switch, want %d", len(a.homeCards), len(sources))
	}
	for i := range a.homeCards {
		if a.homeCards[i].Source != sources[i] {
			t.Errorf("card %d source %q after the switch, want %q", i, a.homeCards[i].Source, sources[i])
		}
		if v := a.homeCards[i].feed.snapshot(); len(v.items) != 0 || v.started {
			t.Errorf("card %d kept the old account's works: items=%d started=%v", i, len(v.items), v.started)
		}
	}
}

// TestHomeBlockedAuthorLeavesCards checks blocking an author drops their works
// from the card previews already loaded.
func TestHomeBlockedAuthorLeavesCards(t *testing.T) {
	a := newHomeApp(t)
	works := fakeWorks(0, 2)
	for i := range works {
		works[i].User.ID = 7
	}
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, works)
	}
	tt := ui.NewTester(a.View, 1024, 720)
	for i := range a.homeCards {
		waitBrowse(t, a.homeCards[i].feed, "load the card", func(v browseView) bool { return len(v.items) == 2 })
	}
	tt.Frame()

	a.blockUser(pixiv.User{ID: 7, Name: "作者"})
	for i := range a.homeCards {
		if v := a.homeCards[i].feed.snapshot(); len(v.items) != 0 {
			t.Errorf("card %d kept the blocked author's works: items=%d", i, len(v.items))
		}
	}
	tt.Frame()
	if _, ok := tt.Find(a.tf("a11y.card", works[0].Title, works[0].User.Name)); ok {
		t.Error("the blocked work is still drawn on a card")
	}
}

// TestHomeFitsNarrowWindow checks the cards fit the 1024x720 window and scroll
// as one page rather than in an inner scroller.
func TestHomeFitsNarrowWindow(t *testing.T) {
	const width = 1024
	a := newHomeApp(t)
	for i := range a.homeCards {
		armHomeCard(a.homeCards[i].feed, fakeWorks(i*100, 6))
	}
	tt := ui.NewTester(a.View, width, 720)
	for i := range a.homeCards {
		waitBrowse(t, a.homeCards[i].feed, "load the card", func(v browseView) bool { return len(v.items) == 6 })
	}
	tt.Frame()

	for _, label := range []string{
		a.homeCardTitle(a.homeCards[0]),
		a.homeCardTitle(a.homeCards[1]),
		a.t("home.viewAll") + " · " + a.homeCardTitle(a.homeCards[0]),
	} {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
	}
	for i := 0; i < 6; i++ {
		label := a.tf("a11y.card", fakeWorks(i, 1)[0].Title, "作者")
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			continue // the row shows only the covers that fit
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("cover %q at %v overflows the %d wide window", label, r, width)
		}
	}

	before, ok := tt.Find(a.homeCardTitle(a.homeCards[0]))
	if !ok {
		t.Fatalf("the first card title is missing: %q", tt.Texts())
	}
	tt.Scroll(width/2, 400, 0, 400)
	tt.Frame()
	after, ok := tt.Find(a.homeCardTitle(a.homeCards[0]))
	if !ok {
		t.Fatalf("the first card title is missing after scrolling: %q", tt.Texts())
	}
	if after.Y >= before.Y {
		t.Errorf("the page did not scroll as one: title moved from %v to %v", before, after)
	}
	tt.Scroll(width/2, 400, 0, 200)
	tt.Frame()
	for _, label := range []string{
		a.homeCardTitle(a.homeCards[2]),
		a.t("home.viewAll") + " · " + a.homeCardTitle(a.homeCards[2]),
	} {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out after scrolling: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
	}
}
