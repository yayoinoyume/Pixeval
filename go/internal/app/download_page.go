package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// downloadConcurrency is how many pages the queue fetches at once.
const downloadConcurrency = 3

// downloadDir is where the pages are saved: Pictures/Pixeval Go under the home
// directory, created when it is missing. An empty result turns saving off.
func downloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("download dir: %v", err)
		return ""
	}
	dir := filepath.Join(home, "Pictures", "Pixeval Go")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("download dir: %v", err)
		return ""
	}
	return dir
}

// downloadWork queues every page of w and reports how many pages the queue
// took, and whether saving is on at all; the queue skips a page it already
// holds.
func (a *App) downloadWork(w pixiv.Illustration) (queued int, ok bool) {
	if a.downloads == nil || a.downloadsDir == "" {
		return 0, false
	}
	n := pageCount(w)
	artist := downloadSafeName(w.User.Name)
	tasks := make([]download.Task, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, download.Task{
			ID:       fmt.Sprintf("%d_%d", w.ID, i),
			IllustID: w.ID,
			Title:    w.Title,
			Artist:   w.User.Name,
			URL:      w.PageURL(i),
			// The page number is one based and padded, so a folder sorts.
			Dest: filepath.Join(a.downloadsDir, artist, fmt.Sprintf("%d_%03d.jpg", w.ID, i+1)),
			Page: i,
		})
	}
	return a.downloads.Add(tasks), true
}

// downloadSafeName keeps an artist name from naming a directory outside the
// download directory, since the name comes from the API.
func downloadSafeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == 0 {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "unknown"
	}
	return name
}

// downloadFinished reports whether a task has reached its last state.
func downloadFinished(s download.State) bool {
	return s == download.Done || s == download.Failed || s == download.Canceled
}

func downloadStateLabel(s download.State) string {
	switch s {
	case download.Running:
		return "下载中"
	case download.Done:
		return "完成"
	case download.Failed:
		return "失败"
	case download.Canceled:
		return "取消"
	default:
		return "排队"
	}
}

func downloadStateColor(t *ui.Theme, s download.State) ui.Color {
	switch s {
	case download.Failed:
		return t.Danger
	case download.Done:
		return t.Success
	case download.Running:
		return t.Text
	}
	return t.TextMuted
}

// downloadsPage draws the queue: a row per task, and the button clearing the
// finished ones. The manager reports a paused task as queued, so the pause
// state is kept here, on the main thread.
func (a *App) downloadsPage(c *ui.Context) {
	t := c.Theme()
	tasks := a.downloads.Snapshot()
	if len(tasks) == 0 {
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "还没有下载任务").TextColor(t.TextMuted)
			ui.Text(c, "在查看器里点「下载」把作品加入队列").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 个任务", len(tasks))).Grow(1).TextColor(t.TextMuted)
			if ui.Button(c, "清空已完成").Disabled(!anyDownloadDone(tasks)).Clicked() {
				a.clearDoneDownloads()
			}
		})
		ui.Scroll(c).Grow(1).Children(func() {
			for _, task := range tasks {
				a.downloadRow(c, task)
			}
		})
	})
}

// downloadRow draws one task: what it is, how far it got, and its buttons.
func (a *App) downloadRow(c *ui.Context, task download.Task) {
	t := c.Theme()
	// A paused task reads as queued in a snapshot; only this page knows.
	paused := task.State == download.Queued && a.paused[task.ID]
	label := downloadStateLabel(task.State)
	if paused {
		label = "已暂停"
	}
	ui.Row(c).Gap(12).Padding(10, 12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, task.Title).MaxLines(1)
			ui.Text(c, fmt.Sprintf("第 %d 页", task.Page+1)).FontSize(12).TextColor(t.TextMuted)
			if task.State == download.Failed && task.Err != "" {
				ui.Text(c, task.Err).FontSize(12).TextColor(t.Danger).MaxLines(1)
			}
		})
		ui.Text(c, label).TextColor(downloadStateColor(t, task.State))
		ui.Column(c).Width(160).AlignItems(ui.Center).Children(func() {
			switch {
			case task.Total > 0:
				ui.Progress(c, float64(task.Bytes)/float64(task.Total)).Width(160)
			case task.State == download.Running:
				ui.Spinner(c)
			}
		})
		if task.State == download.Failed {
			if ui.Button(c, "重试").Clicked() {
				a.retryDownload(task)
			}
		}
		if !downloadFinished(task.State) {
			if paused {
				if ui.Button(c, "恢复").Clicked() {
					a.resumeDownload(task.ID)
				}
			} else if ui.Button(c, "暂停").Clicked() {
				a.pauseDownload(task.ID)
			}
			if ui.Button(c, "取消").Clicked() {
				a.cancelDownload(task.ID)
			}
		}
		if ui.Button(c, "移除").Clicked() {
			a.removeDownload(task.ID)
		}
	})
}

// anyDownloadDone reports whether the clear button has anything to clear.
func anyDownloadDone(tasks []download.Task) bool {
	for _, task := range tasks {
		if task.State == download.Done {
			return true
		}
	}
	return false
}

func (a *App) pauseDownload(id string) {
	a.paused[id] = true
	a.downloads.Pause(id)
	a.update(func() {})
}

func (a *App) resumeDownload(id string) {
	delete(a.paused, id)
	a.downloads.Resume(id)
	a.update(func() {})
}

func (a *App) cancelDownload(id string) {
	delete(a.paused, id)
	a.downloads.Cancel(id)
	a.update(func() {})
}

func (a *App) removeDownload(id string) {
	delete(a.paused, id)
	a.downloads.Remove(id)
	a.update(func() {})
}

// clearDoneDownloads drops the finished tasks from the queue, redrawing once
// for the lot.
func (a *App) clearDoneDownloads() {
	var ids []string
	for _, task := range a.downloads.Snapshot() {
		if task.State == download.Done {
			ids = append(ids, task.ID)
		}
	}
	for _, id := range ids {
		delete(a.paused, id)
		a.downloads.Remove(id)
	}
	a.update(func() {})
}

// retryDownload queues a failed task again: the manager only takes a task it
// does not hold, so the failed one goes first.
func (a *App) retryDownload(task download.Task) {
	delete(a.paused, task.ID)
	a.downloads.Remove(task.ID)
	a.downloads.Add([]download.Task{task})
	a.update(func() {})
}
