package app

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
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

// TestViewerDownloadQueuesPages checks the viewer's download button queues
// every page of the work it shows, under the artist's folder.
func TestViewerDownloadQueuesPages(t *testing.T) {
	a := newViewerApp(newImageTransport(nil))
	a.downloadsDir = t.TempDir()
	w := viewerWork()
	a.openViewer(&w, 0)
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("下载"); err != nil {
		t.Fatalf("Click: %v", err)
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
		want := filepath.Join(a.downloadsDir, "unknown", fmt.Sprintf("%d_%d.jpg", w.ID, i))
		if task.Dest != want {
			t.Errorf("task %d dest %q, want %q", i, task.Dest, want)
		}
		if task.State != download.Queued {
			t.Errorf("task %d state %v, want Queued", i, task.State)
		}
	}
}
