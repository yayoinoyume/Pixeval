package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// newDownloadApp is an app on the download page, saving under a temporary
// directory, whose queue has not been started so nothing reaches the network.
func newDownloadApp(t *testing.T) *App {
	t.Helper()
	a := New()
	a.downloadsDir = t.TempDir()
	a.route = routeDownloads
	return a
}

// TestDownloadsEmpty checks an empty queue says how to fill it.
func TestDownloadsEmpty(t *testing.T) {
	a := newDownloadApp(t)
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("还没有下载任务") {
		t.Errorf("empty state not shown: %q", tt.Texts())
	}
}

// TestDownloadsList checks the queue lists each task's title and page, and
// offers the button clearing the finished ones.
func TestDownloadsList(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{
		{ID: "1_0", IllustID: 1, Title: "第一个作品", Page: 0},
		{ID: "2_1", IllustID: 2, Title: "第二个作品", Page: 1},
	})
	tt := ui.NewTester(a.View, 1000, 700)
	for _, want := range []string{"第一个作品", "第二个作品", "第 1 页", "第 2 页", "清空已完成"} {
		if !tt.HasText(want) {
			t.Errorf("%q not shown: %q", want, tt.Texts())
		}
	}
	if !tt.HasText("排队") {
		t.Errorf("queued state not shown: %q", tt.Texts())
	}
}

// TestDownloadsPauseResume checks the pause button marks the task paused in
// the list, and the resume button takes the mark away.
func TestDownloadsPauseResume(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "待暂停", Page: 0}})
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("暂停"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if !tt.HasText("已暂停") {
		t.Errorf("paused state not shown: %q", tt.Texts())
	}
	if !a.paused["1_0"] {
		t.Errorf("task not marked paused: %v", a.paused)
	}
	if err := tt.Click("恢复"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if tt.HasText("已暂停") {
		t.Errorf("still paused after the resume: %q", tt.Texts())
	}
}

// TestDownloadsCancel checks the cancel button cancels the task.
func TestDownloadsCancel(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "待取消", Page: 0}})
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("%d tasks after the cancel, want 1", len(tasks))
	}
	if got := tasks[0].State; got != download.Canceled {
		t.Errorf("state %v after the cancel, want Canceled", got)
	}
}

// TestDownloadsRetryFailed checks a failed row shows its error, and its retry
// button queues the task again.
func TestDownloadsRetryFailed(t *testing.T) {
	a := newDownloadApp(t)
	task := download.Task{
		ID: "1_0", IllustID: 1, Title: "失败作品", Page: 0,
		State: download.Failed, Err: "连接超时",
	}
	tt := ui.NewTester(func(c *ui.Context) { a.downloadRow(c, task) }, 900, 240)
	if !tt.HasText("失败") {
		t.Errorf("failed state not shown: %q", tt.Texts())
	}
	if !tt.HasText("连接超时") {
		t.Errorf("failure reason not shown: %q", tt.Texts())
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("%d tasks after the retry, want 1", len(tasks))
	}
	if got := tasks[0]; got.State != download.Queued || got.Err != "" {
		t.Errorf("task after the retry %+v, want a clean queued one", got)
	}
}

// TestDownloadsClearDone checks the clear button drops the finished tasks.
func TestDownloadsClearDone(t *testing.T) {
	a := newDownloadApp(t)
	client := pixiv.NewClient()
	client.HTTP().Transport = newImageTransport(nil)
	a.downloads = download.New(client, 1)
	// A destination already on disk finishes without a request.
	done := filepath.Join(a.downloadsDir, "1_001.jpg")
	if err := os.WriteFile(done, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.downloads.Add([]download.Task{
		{ID: "1_0", IllustID: 1, Title: "已完成的", URL: "https://example.invalid/0.jpg", Dest: done},
		{ID: "2_0", IllustID: 2, Title: "另一个", URL: "https://example.invalid/1.jpg", Dest: filepath.Join(a.downloadsDir, "2_001.jpg")},
	})
	a.downloads.Start(context.Background())
	a.downloads.Wait()

	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("清空已完成") {
		t.Fatalf("clear button not shown: %q", tt.Texts())
	}
	if err := tt.Click("清空已完成"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Errorf("%d tasks after the clear, want 0", len(tasks))
	}
}

// TestViewerDownloadQueuesPages checks the viewer's download button queues
// every page of the work it shows, under the artist's folder, once.
func TestViewerDownloadQueuesPages(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.downloadsDir = t.TempDir()
	w := viewerWork()
	a.openViewer(&w, 0)
	tt := ui.NewTester(a.View, 1000, 700)
	for range 2 { // the second click queues nothing new
		if err := tt.Click("下载"); err != nil {
			t.Fatalf("Click: %v", err)
		}
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != pageCount(w) {
		t.Fatalf("%d tasks queued, want %d", len(tasks), pageCount(w))
	}
	for i, task := range tasks {
		if task.URL != w.PageURL(i) {
			t.Errorf("task %d url %q, want %q", i, task.URL, w.PageURL(i))
		}
		// viewerWork has no artist, so the folder is the fallback one.
		want := filepath.Join(a.downloadsDir, "unknown", fmt.Sprintf("%d_%03d.jpg", w.ID, i+1))
		if task.Dest != want {
			t.Errorf("task %d dest %q, want %q", i, task.Dest, want)
		}
		if task.State != download.Queued {
			t.Errorf("task %d state %v, want Queued", i, task.State)
		}
	}
}

// TestViewerDownloadSinglePage checks a work without a page count queues its
// one page.
func TestViewerDownloadSinglePage(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.downloadsDir = t.TempDir()
	w := pixiv.Illustration{
		ID: 9, Title: "单页", Width: 100, Height: 100,
		MetaSinglePage: pixiv.MetaSinglePage{OriginalImageURL: "https://example.invalid/9.jpg"},
	}
	queued, ok := a.downloadWork(w)
	if !ok || queued != 1 {
		t.Fatalf("queued %d ok %v, want 1 true", queued, ok)
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("%d tasks queued, want 1", len(tasks))
	}
	if tasks[0].URL != w.PageURL(0) {
		t.Errorf("url %q, want %q", tasks[0].URL, w.PageURL(0))
	}
}
