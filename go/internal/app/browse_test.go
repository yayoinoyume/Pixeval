package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeWorks makes n works numbered from first, tall enough to fill a page.
func fakeWorks(first, n int) []pixiv.Illustration {
	works := make([]pixiv.Illustration, n)
	for i := range works {
		works[i] = pixiv.Illustration{
			ID:     int64(first + i),
			Title:  fmt.Sprintf("作品 %d", first+i),
			User:   pixiv.User{Name: "作者"},
			Width:  480,
			Height: 720,
		}
	}
	return works
}

// newBrowseTest is an app whose feed loads from fetch, without images or a
// network.
func newBrowseTest(st *browseState) (*App, *ui.Tester) {
	a := New()
	a.images = imagex.New(nil, "")
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			a.browsePage(c, st)
		})
	}, 1000, 700)
	return a, tt
}

// waitBrowse waits for the load goroutine to settle the state as want.
func waitBrowse(t *testing.T, st *browseState, what string, want func(items []pixiv.Illustration, errMsg string) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		items, _, errMsg := st.status()
		if want(items, errMsg) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("feed did not %s: items=%d err=%q", what, len(items), errMsg)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestBrowseShowsWorks checks the first page's covers, titles and authors are
// on the page.
func TestBrowseShowsWorks(t *testing.T) {
	st := &browseState{armed: true, fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(items []pixiv.Illustration, _ string) bool { return len(items) == 20 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !tt.HasText("作者") {
		t.Errorf("author not shown: %q", tt.Texts())
	}
}

// TestBrowseLoadsNextPageAtEnd checks scrolling to the end asks for the next
// page once.
func TestBrowseLoadsNextPageAtEnd(t *testing.T) {
	calls := make(chan int, 4)
	var mu sync.Mutex
	n := 0
	st := &browseState{armed: true, fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		n++
		call := n
		mu.Unlock()
		calls <- call
		if call == 1 {
			return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20), NextURL: "next"}, nil
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(20, 20)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(items []pixiv.Illustration, _ string) bool { return len(items) == 20 })
	if got := <-calls; got != 1 {
		t.Fatalf("call %d, want 1", got)
	}
	tt.Frame()
	tt.Scroll(500, 400, 0, 4000)
	tt.Frame()
	select {
	case got := <-calls:
		if got != 2 {
			t.Fatalf("call %d, want 2", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scrolling to the end did not load the next page")
	}
}

// TestBrowseErrorRetry checks a failed page shows the error and a retry that
// loads again.
func TestBrowseErrorRetry(t *testing.T) {
	var mu sync.Mutex
	n := 0
	st := &browseState{armed: true, fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		mu.Lock()
		n++
		call := n
		mu.Unlock()
		if call == 1 {
			return nil, errors.New("网络错误")
		}
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 3)}, nil
	}}
	_, tt := newBrowseTest(st)
	waitBrowse(t, st, "fail", func(_ []pixiv.Illustration, errMsg string) bool { return errMsg != "" })
	tt.Frame()
	if !tt.HasText("网络错误") {
		t.Errorf("error not shown: %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Fatalf("retry button not shown: %q", tt.Texts())
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitBrowse(t, st, "retry", func(items []pixiv.Illustration, _ string) bool { return len(items) == 3 })
	tt.Frame()
	if !tt.HasText("作品 0") {
		t.Errorf("texts %q", tt.Texts())
	}
}
