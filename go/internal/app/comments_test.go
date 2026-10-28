package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// commentFixture is one comment by an author.
func commentFixture(id int64, author, text string) pixiv.Comment {
	return pixiv.Comment{ID: id, Comment: text, User: pixiv.User{Name: author}}
}

// waitComments waits for the panel's load to settle as want says.
func waitComments(t *testing.T, st *commentState, what string, want func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if want() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("comments did not %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCommentsPanelRenders checks two comments show their text and author.
func TestCommentsPanelRenders(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.items = []pixiv.Comment{
		commentFixture(1, "作者甲", "第一条评论"),
		commentFixture(2, "作者乙", "第二条评论"),
	}
	st.mu.Unlock()
	tt := ui.NewTester(a.View, 1280, 800)
	for _, want := range []string{"评论", "第一条评论", "作者甲", "第二条评论", "作者乙"} {
		if !tt.HasText(want) {
			t.Errorf("%q not shown: %q", want, tt.Texts())
		}
	}
}

// TestCommentsSendClearsDraftAndReloads checks sending posts the draft and
// loads the first page again.
func TestCommentsSendClearsDraftAndReloads(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.draft = "新评论"
	st.mu.Unlock()

	var mu sync.Mutex
	var offsets []int
	var added string
	a.fetchCommentPage = func(_ context.Context, id int64, offset int) (*pixiv.Page[pixiv.Comment], error) {
		mu.Lock()
		offsets = append(offsets, offset)
		mu.Unlock()
		if id != w.ID {
			t.Errorf("work %d, want %d", id, w.ID)
		}
		return &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{commentFixture(3, "作者丙", "新评论")}}, nil
	}
	a.addCommentFn = func(_ context.Context, id int64, text string) (*pixiv.Comment, error) {
		mu.Lock()
		added = text
		mu.Unlock()
		return &pixiv.Comment{ID: 3}, nil
	}

	tt := ui.NewTester(a.View, 1280, 800)
	if err := tt.Click("发送"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitComments(t, st, "reload after sending", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.sending && st.draft == "" && len(st.items) == 1
	})
	mu.Lock()
	gotText, gotOffsets := added, append([]int(nil), offsets...)
	mu.Unlock()
	if gotText != "新评论" {
		t.Errorf("posted %q, want 新评论", gotText)
	}
	if len(gotOffsets) != 1 || gotOffsets[0] != 0 {
		t.Errorf("fetched offsets %v, want [0]", gotOffsets)
	}
}

// TestCommentsEmptyStateHiddenWithMorePages checks an empty first page with a
// next page shows the load button rather than "no comments yet".
func TestCommentsEmptyStateHiddenWithMorePages(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.next = "page2"
	st.mu.Unlock()

	tt := ui.NewTester(a.View, 1280, 800)
	if tt.HasText("还没有评论") {
		t.Errorf("empty state shown with a next page: %q", tt.Texts())
	}
	if !tt.HasText("加载更多") {
		t.Errorf("load more not shown: %q", tt.Texts())
	}
}

// TestCommentsLoadMoreStartsAtLoadedCount checks the second page asks for the
// offset after the comments already loaded.
func TestCommentsLoadMoreStartsAtLoadedCount(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.items = []pixiv.Comment{
		commentFixture(1, "作者甲", "第一条评论"),
		commentFixture(2, "作者乙", "第二条评论"),
	}
	st.next = "page2"
	st.offset = 2
	st.mu.Unlock()

	var mu sync.Mutex
	var offsets []int
	a.fetchCommentPage = func(_ context.Context, _ int64, offset int) (*pixiv.Page[pixiv.Comment], error) {
		mu.Lock()
		offsets = append(offsets, offset)
		mu.Unlock()
		return &pixiv.Page[pixiv.Comment]{
			Items:   []pixiv.Comment{commentFixture(3, "作者丙", "第三条评论")},
			NextURL: "page3",
		}, nil
	}

	tt := ui.NewTester(a.View, 1280, 800)
	for _, want := range []int{3, 4} {
		if err := tt.Click("加载更多"); err != nil {
			t.Fatalf("Click: %v", err)
		}
		waitComments(t, st, "load the next page", func() bool {
			st.mu.Lock()
			defer st.mu.Unlock()
			return !st.loading && len(st.items) == want
		})
	}

	mu.Lock()
	got := append([]int(nil), offsets...)
	mu.Unlock()
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("fetched offsets %v, want [2 3]", got)
	}
}

// TestCommentsLoadEndsAtNoMore checks ErrNoMore ends the list instead of
// leaving a next page behind.
func TestCommentsLoadEndsAtNoMore(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.next = "page2"
	st.mu.Unlock()
	a.fetchCommentPage = func(_ context.Context, _ int64, _ int) (*pixiv.Page[pixiv.Comment], error) {
		return nil, pixiv.ErrNoMore
	}

	a.loadComments(st, false)
	waitComments(t, st, "reach the end", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.loading
	})

	st.mu.Lock()
	next, started, errText := st.next, st.started, st.err
	st.mu.Unlock()
	if next != "" || !started || errText != "" {
		t.Fatalf("next %q, started %v, err %q; want the list ended", next, started, errText)
	}
}

// TestCommentsSendFailureKeepsDraft checks a failed send leaves the composer
// and the reply target as they were.
func TestCommentsSendFailureKeepsDraft(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	reply := commentFixture(1, "作者甲", "第一条评论")
	st.mu.Lock()
	st.started = true
	st.draft = "没发出去"
	st.replyTo = &reply
	st.mu.Unlock()
	release := make(chan struct{})
	a.addCommentFn = func(_ context.Context, _ int64, _ string) (*pixiv.Comment, error) {
		<-release
		return nil, errors.New("网络错误")
	}

	tt := ui.NewTester(a.View, 1280, 800)
	if err := tt.Click("发送"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	waitComments(t, st, "finish the failed send", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.sending
	})

	if st.draft != "没发出去" {
		t.Errorf("draft %q, want it kept", st.draft)
	}
	if st.replyTo == nil {
		t.Error("reply target dropped by a failed send")
	}
}

// TestCommentsSendKeepsTextTypedWhileSending checks text typed during the
// request is not cleared when the request finishes.
func TestCommentsSendKeepsTextTypedWhileSending(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.draft = "新评论"
	st.mu.Unlock()

	started := make(chan struct{})
	release := make(chan struct{})
	a.addCommentFn = func(_ context.Context, _ int64, _ string) (*pixiv.Comment, error) {
		close(started)
		<-release
		return &pixiv.Comment{ID: 3}, nil
	}

	tt := ui.NewTester(a.View, 1280, 800)
	if err := tt.Click("发送"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	<-started
	st.draft = "发送期间输入"
	close(release)
	waitComments(t, st, "finish sending", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.sending
	})

	if st.draft != "发送期间输入" {
		t.Errorf("draft %q, want the text typed during the send", st.draft)
	}
}

// TestCommentsOldPageDroppedAfterWorkChange checks a page still in flight for
// the previous work does not land in the next work's panel.
func TestCommentsOldPageDroppedAfterWorkChange(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments

	gate := make(chan struct{})
	a.fetchCommentPage = func(_ context.Context, _ int64, _ int) (*pixiv.Page[pixiv.Comment], error) {
		<-gate
		return &pixiv.Page[pixiv.Comment]{Items: []pixiv.Comment{commentFixture(1, "作者甲", "旧作品的评论")}}, nil
	}
	a.loadComments(st, false)

	next := viewerWork()
	next.ID = 43
	a.openViewer(&next, 0)
	close(gate)
	waitComments(t, st, "finish for the old work", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.loading
	})

	if a.comments == st {
		t.Fatal("the new work kept the old comment panel")
	}
	a.comments.mu.Lock()
	items := a.comments.items
	a.comments.mu.Unlock()
	if len(items) != 0 {
		t.Fatalf("new panel got %d comments from the old work", len(items))
	}
}

// TestCloseViewerDropsComments checks closing the viewer releases the panel.
func TestCloseViewerDropsComments(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	a.closeViewer()

	if a.comments != nil || a.commentsOpen {
		t.Fatalf("comments %v, open %v; want the panel released", a.comments, a.commentsOpen)
	}
}

// TestCommentsReplyPostsAuthorPrefix locks the reply payload: the API takes
// only a comment field, so a reply is the author's name in front of the text.
func TestCommentsReplyPostsAuthorPrefix(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.items = []pixiv.Comment{commentFixture(1, "作者甲", "第一条评论")}
	st.mu.Unlock()

	var mu sync.Mutex
	var posted string
	release := make(chan struct{})
	a.addCommentFn = func(_ context.Context, _ int64, text string) (*pixiv.Comment, error) {
		<-release
		mu.Lock()
		posted = text
		mu.Unlock()
		return &pixiv.Comment{ID: 2}, nil
	}

	tt := ui.NewTester(a.View, 1280, 800)
	if err := tt.Click("回复"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	st.draft = "文本"
	if err := tt.Click("发送"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	close(release)
	waitComments(t, st, "send the reply", func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return !st.sending
	})

	mu.Lock()
	got := posted
	mu.Unlock()
	if got != "@作者甲 文本" {
		t.Errorf("posted %q, want %q", got, "@作者甲 文本")
	}
}

// TestCommentsReplyShowsReplyTo checks the reply button marks the comment the
// next draft answers.
func TestCommentsReplyShowsReplyTo(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	w := viewerWork()
	a.openViewer(&w, 0)
	a.commentsOpen = true
	st := a.comments
	st.mu.Lock()
	st.started = true
	st.items = []pixiv.Comment{commentFixture(1, "作者甲", "第一条评论")}
	st.mu.Unlock()

	tt := ui.NewTester(a.View, 1280, 800)
	if err := tt.Click("回复"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if st.replyTo == nil {
		t.Fatal("reply target not set")
	}
	tt.Frame()
	if !tt.HasText("回复 @作者甲") {
		t.Errorf("reply target not shown: %q", tt.Texts())
	}
}
