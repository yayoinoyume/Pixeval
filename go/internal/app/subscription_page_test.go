package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// stubFetcher answers every subscription with the same works, without a
// network.
type stubFetcher struct{ works []pixiv.Illustration }

func (f stubFetcher) Fetch(context.Context, subscription.Subscription) ([]pixiv.Illustration, error) {
	return f.works, nil
}

// newSubsTest is an app whose subscriptions are the given ones and whose check
// uses fetcher, on the subscription page.
func newSubsTest(t *testing.T, fetcher subscription.Fetcher, subs ...subscription.Subscription) (*App, *ui.Tester) {
	t.Helper()
	a := New()
	a.subsStore = &subscription.Store{}
	for _, sub := range subs {
		a.subsStore.Add(sub)
	}
	a.subs = subscription.New(fetcher)
	a.subs.Store = a.subsStore
	a.route = routeSubs
	return a, ui.NewTester(a.View, 1000, 700)
}

// waitNewWorks waits for the check to leave want works on the page.
func waitNewWorks(t *testing.T, a *App, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if works, checking, _ := a.subsSnapshot(); len(works) == want && !checking {
			return
		}
		if time.Now().After(deadline) {
			works, checking, _ := a.subsSnapshot()
			t.Fatalf("new works %d, checking %v, want %d", len(works), checking, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSubscriptionsPageEmpty checks an app with no subscriptions says so
// instead of drawing an empty list.
func TestSubscriptionsPageEmpty(t *testing.T) {
	_, tt := newSubsTest(t, stubFetcher{})
	if !tt.HasText("还没有追更订阅") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !tt.HasText("立即检查") {
		t.Errorf("check button missing: %q", tt.Texts())
	}
}

// TestSubscriptionsPageCheckShowsNewWorks checks a click on the check button
// fetches the subscriptions and draws the works it found.
func TestSubscriptionsPageCheckShowsNewWorks(t *testing.T) {
	last := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sub := subscription.Subscription{
		ID: "user-7", Kind: subscription.KindUser, TargetID: 7,
		TargetName: "画师七", LastChecked: last,
	}
	fetcher := stubFetcher{works: []pixiv.Illustration{
		{ID: 1, Title: "新作一", CreateDate: last.Add(24 * time.Hour), User: pixiv.User{Name: "画师七"}},
		{ID: 2, Title: "新作二", CreateDate: last.Add(48 * time.Hour), User: pixiv.User{Name: "画师七"}},
	}}
	a, tt := newSubsTest(t, fetcher, sub)
	if !tt.HasText("画师七") {
		t.Fatalf("subscription row missing: %q", tt.Texts())
	}
	if err := tt.Click("立即检查"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitNewWorks(t, a, 2)
	tt.Frame()
	for _, title := range []string{"新作一", "新作二"} {
		if !tt.HasText(title) {
			t.Errorf("%s not shown: %q", title, tt.Texts())
		}
	}
}

// TestSubscriptionsPageRemoveEmptiesList checks deleting the one subscription
// leaves the page with its empty state.
func TestSubscriptionsPageRemoveEmptiesList(t *testing.T) {
	sub := subscription.Subscription{
		ID: "series-3", Kind: subscription.KindSeries, TargetID: 3, TargetName: "系列三",
	}
	a, tt := newSubsTest(t, stubFetcher{}, sub)
	if err := tt.Click("删除"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := a.subsStore.List(); len(got) != 0 {
		t.Fatalf("subscriptions %+v, want none", got)
	}
	tt.Frame()
	if !tt.HasText("还没有追更订阅") {
		t.Errorf("empty state missing: %q", tt.Texts())
	}
}

// TestSubscriptionToggle checks the user page button adds a subscription and a
// second click removes it.
func TestSubscriptionToggle(t *testing.T) {
	a, _ := newSubsTest(t, stubFetcher{})
	v := &userViewState{feed: &browseState{}}
	v.mu.Lock()
	v.user = pixiv.User{ID: 7, Name: "画师七"}
	v.mu.Unlock()
	a.userView = v
	// The nav shows 追更 too, so the page is the whole view here.
	tt := ui.NewTester(a.userPage, 1000, 700)
	if err := tt.Click("追更"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if !a.isSubscribed(subscription.KindUser, 7) {
		t.Fatalf("subscriptions %+v, want user 7", a.subsStore.List())
	}
	tt.Frame()
	if !tt.HasText("已订阅") {
		t.Errorf("subscribed label missing: %q", tt.Texts())
	}
	if err := tt.Click("已订阅"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.isSubscribed(subscription.KindUser, 7) {
		t.Errorf("subscriptions %+v, want none", a.subsStore.List())
	}
}

// TestSubscriptionIntervalFromSettings checks the check loop uses the saved
// minutes, and an unusable value falls back to the default.
func TestSubscriptionIntervalFromSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	if got := a.subscriptionInterval(); got != 15*time.Minute {
		t.Errorf("default interval %v, want 15m", got)
	}
	a.settings.SubscriptionIntervalMinutes = 30
	if got := a.subscriptionInterval(); got != 30*time.Minute {
		t.Errorf("saved interval %v, want 30m", got)
	}
	a.settings.SubscriptionIntervalMinutes = 0
	if got := a.subscriptionInterval(); got != 15*time.Minute {
		t.Errorf("zero interval %v, want 15m", got)
	}
	a.settings.SubscriptionIntervalMinutes = subscriptionIntervalMax + 1
	if got := a.subscriptionInterval(); got != 15*time.Minute {
		t.Errorf("oversized interval %v, want 15m", got)
	}
}

// newWork is one work with pages pages for the auto download tests.
func newWork(id int64, pages int) pixiv.Illustration {
	return pixiv.Illustration{ID: id, Title: "新作", PageCount: pages, User: pixiv.User{Name: "画师"}}
}

// TestAutoDownloadQueuesNewWorks checks a background check queues the pages
// when the setting is on.
func TestAutoDownloadQueuesNewWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, _ := newSubsTest(t, stubFetcher{})
	a.downloadsDir = t.TempDir()
	a.settings.AutoDownloadSubscriptions = true
	a.subs.OnNew = a.onNewSubscriptions
	a.subs.OnNew([]pixiv.Illustration{newWork(1, 2)})

	if tasks := a.downloads.Snapshot(); len(tasks) != 2 {
		t.Fatalf("%d tasks queued, want 2", len(tasks))
	}
	if want := a.tf("subs.autoQueued", 2); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	if works, _, _ := a.subsSnapshot(); len(works) != 1 {
		t.Errorf("%d new works shown, want 1", len(works))
	}
}

// TestAutoDownloadOffOnlyShowsNewWorks checks a background check only lists
// the works when the setting is off.
func TestAutoDownloadOffOnlyShowsNewWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, _ := newSubsTest(t, stubFetcher{})
	a.downloadsDir = t.TempDir()
	a.subs.OnNew = a.onNewSubscriptions
	a.subs.OnNew([]pixiv.Illustration{newWork(1, 2)})

	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Fatalf("%d tasks queued with auto download off, want 0", len(tasks))
	}
	if works, _, _ := a.subsSnapshot(); len(works) != 1 {
		t.Errorf("%d new works shown, want 1", len(works))
	}
}

// TestAutoDownloadWithoutDownloadDir checks a missing download directory says
// so instead of queueing nothing silently.
func TestAutoDownloadWithoutDownloadDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a, _ := newSubsTest(t, stubFetcher{})
	a.downloadsDir = ""
	a.settings.AutoDownloadSubscriptions = true
	a.subs.OnNew = a.onNewSubscriptions
	a.subs.OnNew([]pixiv.Illustration{newWork(1, 2)})

	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Fatalf("%d tasks queued without a directory, want 0", len(tasks))
	}
	if want := a.t("subs.autoNoDir"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
}

// TestSubscriptionsPath checks the list is saved next to the settings, in the
// user's config directory.
func TestSubscriptionsPath(t *testing.T) {
	path, err := subscriptionsPath()
	if err != nil {
		t.Fatalf("subscriptionsPath: %v", err)
	}
	if want := "pixeval-go/subscriptions.json"; !strings.HasSuffix(path, want) {
		t.Errorf("path %q, want a suffix %q", path, want)
	}
}
