package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeFetcher serves canned works and errors per subscription ID, with no
// network.
type fakeFetcher struct {
	mu    sync.Mutex
	works map[string][]pixiv.Illustration
	errs  map[string]error
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context, sub Subscription) ([]pixiv.Illustration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err := f.errs[sub.ID]; err != nil {
		return nil, err
	}
	return f.works[sub.ID], nil
}

func (f *fakeFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func work(id int64, at time.Time) pixiv.Illustration {
	return pixiv.Illustration{ID: id, Title: fmt.Sprintf("work-%d", id), CreateDate: at}
}

func TestCheckReturnsOnlyWorksNewerThanLastChecked(t *testing.T) {
	last := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	st := &Store{}
	st.Add(Subscription{ID: "user-1", Kind: KindUser, TargetID: 42, LastChecked: last})
	f := &fakeFetcher{works: map[string][]pixiv.Illustration{
		"user-1": {work(1, last.Add(-time.Hour)), work(2, last), work(3, last.Add(time.Hour))},
	}}
	svc := New(f)
	svc.Store = st

	got, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("works = %+v, want only work 3", got)
	}
	if checked := st.List()[0].LastChecked; !checked.Equal(last.Add(time.Hour)) {
		t.Errorf("LastChecked = %v, want the newest CreateDate", checked)
	}
}

func TestCheckReturnsOneCopyAcrossSubscriptions(t *testing.T) {
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	w := work(7, at)
	st := &Store{Subscriptions: []Subscription{
		{ID: "user-1", Kind: KindUser, TargetID: 1},
		{ID: "series-1", Kind: KindSeries, TargetID: 2},
	}}
	f := &fakeFetcher{works: map[string][]pixiv.Illustration{
		"user-1":   {w},
		"series-1": {w},
	}}
	svc := New(f)
	svc.Store = st

	got, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("works = %+v, want the shared work once", got)
	}
}

func TestCheckPersistsLastChecked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	at := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	st := &Store{}
	st.Add(Subscription{ID: "series-1", Kind: KindSeries, TargetID: 9, TargetName: "a series"})
	svc := New(&fakeFetcher{works: map[string][]pixiv.Illustration{
		"series-1": {work(11, at)},
	}})
	svc.Store = st

	if _, err := svc.Check(context.Background()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err := st.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %04o, want 0600", perm)
	}

	loaded := &Store{}
	if err := loaded.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	subs := loaded.List()
	if len(subs) != 1 {
		t.Fatalf("loaded %d subscriptions, want 1", len(subs))
	}
	sub := subs[0]
	if sub.ID != "series-1" || sub.Kind != KindSeries || sub.TargetID != 9 || sub.TargetName != "a series" {
		t.Errorf("subscription = %+v, want the saved fields", sub)
	}
	if !sub.LastChecked.Equal(at) {
		t.Errorf("LastChecked = %v, want %v", sub.LastChecked, at)
	}
}

func TestCheckKeepsGoingWhenOneSubscriptionFails(t *testing.T) {
	at := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	st := &Store{Subscriptions: []Subscription{
		{ID: "broken", Kind: KindUser, TargetID: 1},
		{ID: "ok", Kind: KindSeries, TargetID: 2},
	}}
	f := &fakeFetcher{
		works: map[string][]pixiv.Illustration{"ok": {work(5, at)}},
		errs:  map[string]error{"broken": errors.New("boom")},
	}
	svc := New(f)
	svc.Store = st

	got, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("works = %+v, want work 5 from the healthy subscription", got)
	}
	for _, sub := range st.List() {
		if sub.ID == "broken" && !sub.LastChecked.IsZero() {
			t.Errorf("failed subscription advanced LastChecked to %v", sub.LastChecked)
		}
	}
}

func TestStoreAddStopsAtMaxSubscriptions(t *testing.T) {
	st := &Store{}
	for i := 0; i < maxSubscriptions; i++ {
		if !st.Add(Subscription{ID: fmt.Sprintf("sub-%d", i)}) {
			t.Fatalf("Add(%d) was refused below the limit", i)
		}
	}
	if st.Add(Subscription{ID: "overflow"}) {
		t.Error("Add accepted an entry past the limit")
	}
	if st.Add(Subscription{ID: "sub-0"}) {
		t.Error("Add accepted a duplicate ID")
	}
	if !st.Remove("sub-0") || st.Remove("sub-0") {
		t.Error("Remove did not report presence, then absence")
	}
	if n := len(st.List()); n != maxSubscriptions-1 {
		t.Errorf("len = %d, want %d", n, maxSubscriptions-1)
	}
}

func TestStoreLoadKeepsAtMostMaxSubscriptions(t *testing.T) {
	file := subscriptionFile{}
	for i := 0; i <= maxSubscriptions; i++ {
		file.Subscriptions = append(file.Subscriptions, Subscription{ID: fmt.Sprintf("sub-%d", i)})
	}
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	st := &Store{}
	if err := st.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if n := len(st.List()); n != maxSubscriptions {
		t.Errorf("len = %d, want %d", n, maxSubscriptions)
	}
}

func TestStoreLoadBadJSONKeepsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	if err := os.WriteFile(path, []byte(`{"subscriptions":`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	st := &Store{}
	st.Add(Subscription{ID: "kept", Kind: KindUser, TargetID: 3})
	if err := st.Load(path); err == nil {
		t.Fatal("Load accepted broken JSON")
	}
	if subs := st.List(); len(subs) != 1 || subs[0].ID != "kept" {
		t.Fatalf("store = %+v, want the existing entry kept", subs)
	}
}

func TestStartReportsNewWorksUntilStopped(t *testing.T) {
	at := time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC)
	st := &Store{Subscriptions: []Subscription{{ID: "user-1", Kind: KindUser, TargetID: 1}}}
	f := &fakeFetcher{works: map[string][]pixiv.Illustration{"user-1": {work(9, at)}}}
	svc := New(f)
	svc.Store = st
	got := make(chan []pixiv.Illustration, 1)
	svc.OnNew = func(works []pixiv.Illustration) { got <- works }

	svc.Start(context.Background(), time.Millisecond)
	select {
	case works := <-got:
		if len(works) != 1 || works[0].ID != 9 {
			t.Fatalf("works = %+v, want work 9", works)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnNew was never called")
	}
	svc.Stop()

	before := f.callCount()
	time.Sleep(20 * time.Millisecond)
	if after := f.callCount(); after != before {
		t.Fatalf("checks kept running after Stop: %d -> %d", before, after)
	}
}
