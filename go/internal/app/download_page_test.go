package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	a.downloadHistory = download.NewHistory()
	a.downloadRecorded = map[string]bool{}
	a.route = routeDownloads
	return a
}

// failedImageTransport answers every image request with a permanent status, so
// a task fails without retrying or reaching the network.
type failedImageTransport struct{}

func (failedImageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
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

// TestDownloadsFilterGroups checks the tab line shows one group at a time, and
// the empty group says so without hiding the other one.
func TestDownloadsFilterGroups(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{
		{ID: "1_0", IllustID: 1, Title: "普通作品", Source: download.SourceOrdinary},
		{ID: "2_0", IllustID: 2, Title: "订阅作品", Source: download.SourceSubscription},
	})
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("普通作品") || !tt.HasText("订阅作品") {
		t.Fatalf("all group missing a task: %q", tt.Texts())
	}

	if err := tt.Click("订阅"); err != nil {
		t.Fatalf("Click subscription: %v", err)
	}
	if !tt.HasText("订阅作品") {
		t.Errorf("subscription task missing: %q", tt.Texts())
	}
	if tt.HasText("普通作品") {
		t.Errorf("ordinary task stayed in the subscription group: %q", tt.Texts())
	}

	if err := tt.Click("普通"); err != nil {
		t.Fatalf("Click ordinary: %v", err)
	}
	if !tt.HasText("普通作品") {
		t.Errorf("ordinary task missing: %q", tt.Texts())
	}
	if tt.HasText("订阅作品") {
		t.Errorf("subscription task stayed in the ordinary group: %q", tt.Texts())
	}
}

// TestDownloadsEmptyGroup checks a group with no task shows its own empty
// state, not the empty queue's.
func TestDownloadsEmptyGroup(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "普通作品"}})
	a.downloadFilter = downloadFilterSubscription
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("还没有订阅下载") {
		t.Errorf("subscription empty state missing: %q", tt.Texts())
	}
	if tt.HasText("还没有下载任务") {
		t.Errorf("empty queue state shown for one empty group: %q", tt.Texts())
	}
}

// TestDownloadWorkMarksOrdinarySource checks a manual download is grouped as
// an ordinary one.
func TestDownloadWorkMarksOrdinarySource(t *testing.T) {
	a := newDownloadApp(t)
	if _, ok := a.downloadWork(viewerWork()); !ok {
		t.Fatal("downloadWork reported saving off")
	}
	for _, task := range a.downloads.Snapshot() {
		if task.Source != download.SourceOrdinary {
			t.Errorf("task %q source %q, want ordinary", task.ID, task.Source)
		}
	}
}

// TestDownloadsRestoreSavedQueue checks the start brings back the unfinished
// queue the previous run saved.
func TestDownloadsRestoreSavedQueue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := download.Task{
		ID:       "1_0",
		IllustID: 1,
		Title:    "上次没下完",
		Artist:   "画师",
		URL:      "https://example.invalid/1.jpg",
		Dest:     filepath.Join(t.TempDir(), "1.jpg"),
	}
	if err := download.Save(filepath.Join(dir, "pixeval-go", "downloads.json"), []download.Task{want}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a := newDownloadApp(t)
	a.restoreDownloads()
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("restored %d tasks, want 1: %+v", len(tasks), tasks)
	}
	if tasks[0] != want {
		t.Fatalf("restored task %+v, want %+v", tasks[0], want)
	}
}

// TestDownloadsRestoreBrokenQueue checks a damaged file starts an empty queue
// instead of blocking the start.
func TestDownloadsRestoreBrokenQueue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "pixeval-go", "downloads.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	a := newDownloadApp(t)
	a.restoreDownloads()
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Fatalf("restored %d tasks from a broken file, want none", len(tasks))
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

// TestDownloadsHistoryTab checks the history tab shows a completed download's
// title, author, page, source and completion time.
func TestDownloadsHistoryTab(t *testing.T) {
	a := newDownloadApp(t)
	a.downloadHistory.Add(download.HistoryEntry{
		ID:          "1_1",
		IllustID:    1,
		Title:       "历史作品",
		Artist:      "历史画师",
		Page:        1,
		Source:      download.SourceSubscription,
		CompletedAt: time.Date(2026, 5, 1, 12, 30, 0, 0, time.Local),
	})
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("下载历史"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	for _, want := range []string{
		"历史作品", "历史画师", "第 2 页", "订阅",
		"完成于 2026-05-01 12:30", "清空历史",
	} {
		if !tt.HasText(want) {
			t.Errorf("%q not shown: %q", want, tt.Texts())
		}
	}
}

// TestDownloadsHistoryEmpty checks the history tab says it has nothing when
// there are no completed downloads.
func TestDownloadsHistoryEmpty(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "排队作品"}})
	a.downloadFilter = downloadFilterHistory
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText("还没有下载历史") {
		t.Errorf("history empty state missing: %q", tt.Texts())
	}
}

// TestDownloadsClearHistoryKeepsQueue checks clearing the history leaves the
// queue alone.
func TestDownloadsClearHistoryKeepsQueue(t *testing.T) {
	a := newDownloadApp(t)
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "排队作品"}})
	a.downloadHistory.Add(download.HistoryEntry{
		ID: "2_0", IllustID: 2, Title: "历史作品", CompletedAt: time.Now(),
	})
	a.downloadFilter = downloadFilterHistory
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("清空历史"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if got := a.downloadHistory.Snapshot(); len(got) != 0 {
		t.Errorf("%d history entries after the clear, want 0", len(got))
	}
	if got := a.downloads.Snapshot(); len(got) != 1 {
		t.Errorf("%d queued tasks after the clear, want 1", len(got))
	}
}

// TestDownloadsRecordOnlyDone checks only a completed task is recorded, and a
// later scan does not record it again.
func TestDownloadsRecordOnlyDone(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := newDownloadApp(t)
	a.client.HTTP().Transport = failedImageTransport{}
	a.downloads = download.New(a.client, 1)
	a.downloads.OnChange(a.recordDoneDownloads)

	done := filepath.Join(a.downloadsDir, "done.jpg")
	if err := os.WriteFile(done, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.downloads.Add([]download.Task{
		{ID: "done", IllustID: 1, Title: "已完成", Artist: "画师", Dest: done, Page: 0},
		{ID: "failed", IllustID: 2, Title: "失败", URL: "https://example.invalid/fail.jpg", Dest: filepath.Join(a.downloadsDir, "fail.jpg")},
		{ID: "canceled", IllustID: 3, Title: "取消", URL: "https://example.invalid/cancel.jpg", Dest: filepath.Join(a.downloadsDir, "cancel.jpg")},
	})
	a.downloads.Cancel("canceled")
	a.downloads.Start(context.Background())
	a.downloads.Wait()

	got := a.downloadHistory.Snapshot()
	if len(got) != 1 || got[0].ID != "done" {
		t.Fatalf("history %+v, want only the completed task", got)
	}
	if got[0].Title != "已完成" || got[0].Artist != "画师" || got[0].Page != 0 || got[0].Dest != done {
		t.Errorf("recorded entry %+v, want the completed task's fields", got[0])
	}

	a.recordDoneDownloads()
	if again := a.downloadHistory.Snapshot(); len(again) != 1 {
		t.Errorf("%d history entries after a second scan, want 1", len(again))
	}
}

// TestDownloadsLoadSavedHistory checks the start reads the history an earlier
// run saved.
func TestDownloadsLoadSavedHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	h := download.NewHistory()
	h.Add(download.HistoryEntry{ID: "1_0", IllustID: 1, Title: "上次完成的", CompletedAt: time.Now()})
	if err := h.Save(filepath.Join(dir, "pixeval-go", "download_history.json")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a := New()
	got := a.downloadHistory.Snapshot()
	if len(got) != 1 || got[0].Title != "上次完成的" {
		t.Fatalf("loaded history %+v, want the saved entry", got)
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
		// viewerWork has no artist, so the name uses the fallback one, and the
		// default macro files the pages under the work's title.
		want := filepath.Join(a.downloadsDir, "[unknown] 查看器作品",
			fmt.Sprintf("[unknown] %dp%d.jpg", w.ID, i+1))
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
	// A single page work has no set folder in the default macro.
	want := filepath.Join(a.downloadsDir, "[unknown] 9.jpg")
	if tasks[0].Dest != want {
		t.Errorf("dest %q, want %q", tasks[0].Dest, want)
	}
}

// TestDownloadWorkUsesMacro checks a saved template names each page and keeps
// the subdirectories it spells out.
func TestDownloadWorkUsesMacro(t *testing.T) {
	a := newDownloadApp(t)
	a.settings = Default()
	a.settings.DownloadMacro = "@{artist_name}/@{title}/@{id}_@{pic_set_index}.@{ext}"
	w := viewerWork()
	w.User.Name = "画家A"
	if _, ok := a.downloadWork(w); !ok {
		t.Fatal("downloadWork reported saving off")
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != pageCount(w) {
		t.Fatalf("%d tasks queued, want %d", len(tasks), pageCount(w))
	}
	for i, task := range tasks {
		want := filepath.Join(a.downloadsDir, "画家A", "查看器作品", fmt.Sprintf("%d_%d.jpg", w.ID, i+1))
		if task.Dest != want {
			t.Errorf("task %d dest %q, want %q", i, task.Dest, want)
		}
	}
}

// TestDownloadWorkFallbacks checks a template the parser rejects, and one that
// names no file, still queue a page under the download directory.
func TestDownloadWorkFallbacks(t *testing.T) {
	w := viewerWork()
	for _, macro := range []string{"@{nope}", ".."} {
		a := newDownloadApp(t)
		a.settings = Default()
		a.settings.DownloadMacro = macro
		if _, ok := a.downloadWork(w); !ok {
			t.Fatalf("%q: downloadWork reported saving off", macro)
		}
		tasks := a.downloads.Snapshot()
		want := filepath.Join(a.downloadsDir, fmt.Sprintf("%d_001.jpg", w.ID))
		if tasks[0].Dest != want {
			t.Errorf("%q: dest %q, want %q", macro, tasks[0].Dest, want)
		}
	}
}

// TestDownloadDestStaysInsideDownloadDir checks a template cannot climb out of
// the download directory, and that unsafe characters are replaced.
func TestDownloadDestStaysInsideDownloadDir(t *testing.T) {
	dir := t.TempDir()
	ctx := download.Context{ID: 7, PicSetIndex: 1, Ext: "jpg", ArtistName: "a:b*c?", Title: "t<i>tle|x"}
	cases := []struct{ name, macro string }{
		{"parent", `../@{id}.@{ext}`},
		{"nested parent", `@{artist_name}/../../@{id}.@{ext}`},
		{"absolute", `/etc/@{id}.@{ext}`},
		{"windows parent", `..\\@{id}.@{ext}`},
		{"unsafe characters", `@{artist_name}/@{title}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := downloadDest(dir, downloadTemplate(tc.macro), ctx)
			if !downloadWithin(dir, dest) {
				t.Fatalf("dest %q leaves %q", dest, dir)
			}
			if strings.Contains(dest, "..") {
				t.Errorf("dest %q keeps a parent segment", dest)
			}
		})
	}

	dest := downloadDest(dir, downloadTemplate(`@{artist_name}/@{title}`), ctx)
	want := filepath.Join(dir, "a_b_c_", "t_i_tle_x")
	if dest != want {
		t.Errorf("dest %q, want %q", dest, want)
	}
}

// TestDownloadContextKeepsDataOutOfPathSegments checks a slash in the API's
// text does not make a directory of its own.
func TestDownloadContextKeepsDataOutOfPathSegments(t *testing.T) {
	w := pixiv.Illustration{
		ID: 7, Title: "中文/标题", User: pixiv.User{Name: `画\家`},
		Series: &pixiv.Series{ID: 3, Title: "系/列"},
	}
	ctx := downloadContext(w, 0, "jpg")
	for _, got := range []string{ctx.Title, ctx.ArtistName, ctx.SeriesTitle} {
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("%q keeps a path separator", got)
		}
	}
	if ctx.SeriesID != 3 || !ctx.IsSeries || !ctx.IsPicOne || ctx.PicSetIndex != 1 {
		t.Errorf("context %+v", ctx)
	}
}

// TestDownloadLoadedSkipsQueued checks a second call queues nothing new, and
// says so.
func TestDownloadLoadedSkipsQueued(t *testing.T) {
	a := newDownloadApp(t)
	works := fakeWorks(0, 2)
	if queued := a.downloadLoaded(works); queued != 2 {
		t.Fatalf("queued %d pages, want 2", queued)
	}
	if want := a.tf("download.allQueued", 2); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	if queued := a.downloadLoaded(works); queued != 0 {
		t.Errorf("queued %d pages again, want 0", queued)
	}
	if want := a.t("download.none"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
}

// TestDownloadLoadedWithoutDir checks saving off queues nothing and leaves a
// notice.
func TestDownloadLoadedWithoutDir(t *testing.T) {
	a := newDownloadApp(t)
	a.downloadsDir = ""
	if queued := a.downloadLoaded(fakeWorks(0, 2)); queued != 0 {
		t.Errorf("queued %d pages without a directory, want 0", queued)
	}
	if want := a.t("viewer.noDownloadDir"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Errorf("%d tasks queued without a directory, want 0", len(tasks))
	}
}

// TestDownloadUgoiraQueuesZip checks an animation is queued as its original
// zip, not as the still image the feed carries.
func TestDownloadUgoiraQueuesZip(t *testing.T) {
	tr := &ugoiraTransport{frames: 2, zipData: ugoiraZip(t, 2)}
	a := newViewerApp(tr)
	a.downloadsDir = t.TempDir()
	a.settings = Default()
	w := ugoiraWork()

	queued, ok, err := a.downloadUgoiraWork(w, download.SourceOrdinary)
	if err != nil || !ok || queued != 1 {
		t.Fatalf("queued %d ok %v err %v, want 1 true nil", queued, ok, err)
	}
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("%d tasks queued, want 1", len(tasks))
	}
	task := tasks[0]
	if task.URL != ugoiraZipURL {
		t.Errorf("url %q, want %q", task.URL, ugoiraZipURL)
	}
	if task.Source != download.SourceOrdinary {
		t.Errorf("source %q, want %q", task.Source, download.SourceOrdinary)
	}
	if task.Page != 0 {
		t.Errorf("page %d, want 0", task.Page)
	}
	want := filepath.Join(a.downloadsDir, "[unknown] 77.zip")
	if task.Dest != want {
		t.Errorf("dest %q, want %q", task.Dest, want)
	}
	// Queuing the animation must not download anything yet.
	if calls := tr.zipCount(); calls != 0 {
		t.Errorf("zip fetched %d times while queuing, want 0", calls)
	}
}

// TestDownloadUgoiraMetadataFailure checks a failed metadata fetch queues
// nothing and reports the error.
func TestDownloadUgoiraMetadataFailure(t *testing.T) {
	tr := &ugoiraTransport{frames: 2}
	tr.setMetaFail(true)
	a := newViewerApp(tr)
	a.downloadsDir = t.TempDir()
	a.settings = Default()

	queued, ok, err := a.downloadUgoiraWork(ugoiraWork(), download.SourceOrdinary)
	if err == nil {
		t.Fatal("metadata failure reported no error")
	}
	if !ok {
		t.Error("saving reported off")
	}
	if queued != 0 {
		t.Errorf("queued %d tasks on failure, want 0", queued)
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Errorf("%d tasks queued on failure, want 0", len(tasks))
	}
}

// TestDownloadWorkSkipsUgoira checks the synchronous queue path never asks an
// animation for its metadata, and queues nothing for it.
func TestDownloadWorkSkipsUgoira(t *testing.T) {
	tr := &ugoiraTransport{frames: 2}
	a := newViewerApp(tr)
	a.downloadsDir = t.TempDir()
	a.settings = Default()

	for _, source := range []string{download.SourceOrdinary, download.SourceSubscription} {
		queued, ok := a.downloadWorkSource(ugoiraWork(), source)
		if !ok {
			t.Fatalf("%s: saving reported off", source)
		}
		if queued != 0 {
			t.Errorf("%s: queued %d, want 0", source, queued)
		}
	}
	if tasks := a.downloads.Snapshot(); len(tasks) != 0 {
		t.Errorf("%d tasks queued, want 0", len(tasks))
	}
	if calls := tr.metaCount(); calls != 0 {
		t.Errorf("metadata fetched %d times, want 0", calls)
	}
}

// TestDownloadLoadedSkipsUgoira checks a batch skips animations without a
// metadata request, says so, and still queues the works beside them.
func TestDownloadLoadedSkipsUgoira(t *testing.T) {
	tr := &ugoiraTransport{frames: 2}
	a := newViewerApp(tr)
	a.downloadsDir = t.TempDir()
	a.settings = Default()

	if queued := a.downloadLoaded([]pixiv.Illustration{ugoiraWork()}); queued != 0 {
		t.Errorf("queued %d, want 0", queued)
	}
	if want := a.t("download.ugoiraUnsupported"); a.notice != want {
		t.Errorf("notice %q, want %q", a.notice, want)
	}

	if queued, want := a.downloadLoaded([]pixiv.Illustration{ugoiraWork(), viewerWork()}), pageCount(viewerWork()); queued != want {
		t.Errorf("queued %d, want %d", queued, want)
	}
	if calls := tr.metaCount(); calls != 0 {
		t.Errorf("metadata fetched %d times, want 0", calls)
	}
}
