package app

import (
	"context"
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
	if st.reply() == nil {
		t.Fatal("reply target not set")
	}
	tt.Frame()
	if !tt.HasText("回复 @作者甲") {
		t.Errorf("reply target not shown: %q", tt.Texts())
	}
}
