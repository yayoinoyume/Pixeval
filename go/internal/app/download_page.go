package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
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
	return a.downloadWorkSource(w, download.SourceOrdinary)
}

// downloadWorkSource queues w's pages tagged with the source they came from,
// so the download page can group subscription updates apart from the user's
// own downloads. An animation is skipped: its zip needs a metadata request,
// which this synchronous path must not make on the calling thread.
func (a *App) downloadWorkSource(w pixiv.Illustration, source string) (queued int, ok bool) {
	if a.downloads == nil || a.downloadsDir == "" {
		return 0, false
	}
	if isUgoira(w) {
		return 0, true
	}
	tmpl := downloadTemplate(a.downloadMacro())
	n := pageCount(w)
	tasks := make([]download.Task, 0, n)
	for i := 0; i < n; i++ {
		ctx := downloadContext(w, i, downloadExt(w, i))
		tasks = append(tasks, download.Task{
			ID:       fmt.Sprintf("%d_%d", w.ID, i),
			IllustID: w.ID,
			Title:    w.Title,
			Artist:   w.User.Name,
			URL:      w.PageURL(i),
			Dest:     downloadDest(a.downloadsDir, tmpl, ctx),
			Page:     i,
			Source:   source,
		})
	}
	return a.downloads.Add(tasks), true
}

// downloadUgoiraWork queues the animation's zip as one task, fetching its URL
// from the metadata first. A failed fetch queues nothing and reports why; ok is
// false when saving is off.
func (a *App) downloadUgoiraWork(w pixiv.Illustration, source string) (queued int, ok bool, err error) {
	if a.downloads == nil || a.downloadsDir == "" {
		return 0, false, nil
	}
	if a.client == nil {
		return 0, true, errors.New("pixiv client unavailable")
	}
	metaCtx, cancel := context.WithTimeout(context.Background(), browseTimeout)
	defer cancel()
	meta, err := a.client.UgoiraMetadata(metaCtx, w.ID)
	if err != nil {
		return 0, true, err
	}
	if meta.ZipURL == "" {
		return 0, true, errors.New("ugoira metadata has no zip url")
	}
	ctx := downloadContext(w, 0, "zip")
	ctx.IsPicGif, ctx.IsPicSet, ctx.IsPicOne = true, false, true
	task := download.Task{
		ID:       fmt.Sprintf("%d_ugoira", w.ID),
		IllustID: w.ID,
		Title:    w.Title,
		Artist:   w.User.Name,
		URL:      meta.ZipURL,
		Dest:     downloadDest(a.downloadsDir, downloadTemplate(a.downloadMacro()), ctx),
		Source:   source,
	}
	return a.downloads.Add([]download.Task{task}), true, nil
}

// queueUgoiraDownload fetches the animation's zip off the main thread and
// leaves the outcome as the next frame's notice.
func (a *App) queueUgoiraDownload(w pixiv.Illustration) {
	go func() {
		queued, ok, err := a.downloadUgoiraWork(w, download.SourceOrdinary)
		a.update(func() {
			switch {
			case !ok:
				a.notice = a.t("viewer.noDownloadDir")
			case err != nil:
				a.notice = a.tf("common.loadFailed", err.Error())
			case queued == 0:
				a.notice = a.t("viewer.alreadyQueued")
			default:
				a.notice = a.t("download.ugoiraQueued")
			}
		})
	}()
}

// downloadLoadedButton draws the button that queues the works a list has
// already loaded; a list with no works shows none. works is the list the
// button acts on.
func (a *App) downloadLoadedButton(c *ui.Context, works []pixiv.Illustration) {
	if len(works) == 0 {
		return
	}
	if ui.Button(c, a.tf("download.allLoaded", len(works))).Clicked() {
		a.downloadLoaded(works)
	}
}

// downloadLoaded queues the loaded works, and leaves a notice saying how many
// pages the queue took: it skips a page it already holds, and an animation,
// whose zip the viewer queues instead. Without a download directory saving is
// off, and the notice says so instead. The notice is handed to the next frame,
// so a click that asks for no frame of its own still shows it.
func (a *App) downloadLoaded(works []pixiv.Illustration) (queued int) {
	skipped := 0
	for _, w := range works {
		if isUgoira(w) {
			skipped++
			continue
		}
		n, ok := a.downloadWork(w)
		if !ok {
			a.notice = a.t("viewer.noDownloadDir")
			a.update(func() {})
			return 0
		}
		queued += n
	}
	switch {
	case queued == 0 && skipped > 0:
		a.notice = a.t("download.ugoiraUnsupported")
	case queued == 0:
		a.notice = a.t("download.none")
	case skipped > 0:
		a.notice = a.tf("download.allQueued", queued) + " " + a.t("download.ugoiraUnsupported")
	default:
		a.notice = a.tf("download.allQueued", queued)
	}
	a.update(func() {})
	return queued
}

// downloadMacro is the naming template the settings ask for, or the default
// one when there are no settings yet.
func (a *App) downloadMacro() string {
	if a.settings != nil && strings.TrimSpace(a.settings.DownloadMacro) != "" {
		return a.settings.DownloadMacro
	}
	return download.DefaultMacro
}

// downloadTemplate parses a macro; nil means the caller names the file itself.
func downloadTemplate(macro string) *download.Template {
	tmpl, err := download.Parse(macro)
	if err != nil {
		return nil
	}
	return tmpl
}

// downloadContext is the macro view of one page of w. The page number is one
// based, as the C# context counts it.
func downloadContext(w pixiv.Illustration, page int, ext string) download.Context {
	pages := pageCount(w)
	ctx := download.Context{
		ID:          w.ID,
		Title:       downloadContextName(w.Title),
		ArtistID:    w.User.ID,
		ArtistName:  downloadContextName(downloadArtistName(w.User.Name)),
		PublishTime: w.CreateDate,
		Ext:         ext,
		PicSetIndex: page + 1,
		IsPicSet:    pages > 1,
		IsPicOne:    pages <= 1,
		IsSeries:    w.Series != nil,
		IsR18:       w.XRestrict >= 1,
		IsR18G:      w.XRestrict >= 2,
		IsAI:        w.IllustAIType == 2,
	}
	if w.Series != nil {
		ctx.SeriesID, ctx.SeriesTitle = w.Series.ID, downloadContextName(w.Series.Title)
	}
	return ctx
}

// downloadContextName keeps a value from the API from making path segments of
// its own: only the template's own separators may.
func downloadContextName(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_").Replace(s)
}

// downloadArtistName keeps an unnamed work filed under a folder of its own,
// as the fixed layout did before the template.
func downloadArtistName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "unknown"
	}
	return name
}

// downloadExt is the extension of the page's image URL, jpg when it has none.
func downloadExt(w pixiv.Illustration, page int) string {
	u, err := url.Parse(w.PageURL(page))
	if err != nil {
		return "jpg"
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(u.Path)), ".")
	if ext == "" {
		return "jpg"
	}
	return ext
}

// downloadDest renders one page's template under dir. The rendered text is
// split on its separators so a template can make subdirectories, but every
// segment is made a safe name and empty or parent segments are dropped, so a
// template cannot write outside dir. A template that renders nothing usable,
// or a path that still leaves dir, falls back to the id and page name.
func downloadDest(dir string, tmpl *download.Template, ctx download.Context) string {
	text := ""
	if tmpl != nil {
		if rendered, err := tmpl.Render(ctx); err == nil {
			text = rendered
		}
	}
	dest := filepath.Join(dir, downloadSafeRel(text))
	if !downloadWithin(dir, dest) {
		dest = filepath.Join(dir, fmt.Sprintf("%d_%03d.%s", ctx.ID, ctx.PicSetIndex, ctx.Ext))
	}
	return dest
}

// downloadSafeRel rebuilds a relative path from rendered text, dropping what
// names no file: empty segments, "." and "..".
func downloadSafeRel(text string) string {
	var segments []string
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg := downloadSafeSegment(part); seg != "" {
			segments = append(segments, seg)
		}
	}
	return filepath.Join(segments...)
}

// downloadSafeSegment makes one segment a name that is legal on every
// platform. The separators are gone by now, since they split the text.
func downloadSafeSegment(seg string) string {
	seg = strings.Map(func(r rune) rune {
		switch {
		case r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return '_'
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, seg)
	// Windows drops a trailing dot or space from a name.
	seg = strings.TrimRight(strings.TrimSpace(seg), ". ")
	if seg == "" || seg == "." || seg == ".." {
		return ""
	}
	return seg
}

// downloadWithin reports whether dest names a file under dir.
func downloadWithin(dir, dest string) bool {
	rel, err := filepath.Rel(dir, dest)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// downloadFinished reports whether a task has reached its last state.
func downloadFinished(s download.State) bool {
	return s == download.Done || s == download.Failed || s == download.Canceled
}

// The download page's groups, in the order the tab line shows them.
const (
	downloadFilterAll = iota
	downloadFilterOrdinary
	downloadFilterSubscription
	downloadFilterHistory
)

// downloadFilterTasks keeps the tasks of the chosen group. The all group
// keeps every task, in queue order; the history group has no tasks.
func downloadFilterTasks(tasks []download.Task, filter int) []download.Task {
	switch filter {
	case downloadFilterAll:
		return tasks
	case downloadFilterHistory:
		return nil
	}
	out := make([]download.Task, 0, len(tasks))
	for _, task := range tasks {
		subscription := task.Source == download.SourceSubscription
		if (filter == downloadFilterSubscription) == subscription {
			out = append(out, task)
		}
	}
	return out
}

func (a *App) downloadStateLabel(s download.State) string {
	switch s {
	case download.Running:
		return a.t("download.running")
	case download.Done:
		return a.t("download.done")
	case download.Failed:
		return a.t("download.failed")
	case download.Canceled:
		return a.t("download.canceled")
	default:
		return a.t("download.queued")
	}
}

func downloadStateColor(t tokens, s download.State) ui.Color {
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
// finished ones. A tab line groups the user's own downloads apart from the
// subscription updates and the completed history. The manager reports a
// paused task as queued, so the pause state is kept here, on the main thread.
func (a *App) downloadsPage(c *ui.Context) {
	t := themeOf(c)
	tasks := a.downloads.Snapshot()
	history := a.downloadHistoryEntries()
	if len(tasks) == 0 && len(history) == 0 {
		emptyState(c, iconDownload, a.t("download.empty"), a.t("download.emptyHint"), nil)
		return
	}
	shown := downloadFilterTasks(tasks, a.downloadFilter)
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(t.SpaceS).Padding(t.SpaceL, contentPadding, t.SpaceS, contentPadding).
			AlignItems(ui.Center).Children(func() {
			if a.downloadFilter == downloadFilterHistory {
				ui.Text(c, a.t("download.history")).FontSize(t.TextSmall).
					TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, a.t("download.clearHistory")).Disabled(len(history) == 0).Clicked() {
					a.clearDownloadHistory()
				}
				return
			}
			ui.Text(c, a.tf("download.count", len(tasks))).FontSize(t.TextSmall).
				TextColor(t.TextMuted).Grow(1)
			if ui.Button(c, a.t("download.clearDone")).Disabled(!anyDownloadDone(tasks)).Clicked() {
				a.clearDoneDownloads()
			}
		})
		ui.Row(c).Padding(t.SpaceS, contentPadding, 0, contentPadding).Children(func() {
			textTabs(c, t, &a.downloadFilter,
				a.t("download.filterAll"), a.t("download.filterOrdinary"),
				a.t("download.filterSubscription"), a.t("download.history"))
		})
		if a.downloadFilter == downloadFilterHistory {
			a.downloadHistoryList(c, history)
			return
		}
		if len(shown) == 0 {
			a.downloadGroupEmpty(c)
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(0, contentPadding, contentPadding, contentPadding).
				Gap(t.SpaceS).Children(func() {
				for _, task := range shown {
					a.downloadRow(c, task)
				}
			})
		})
	})
}

// downloadHistoryEntries is the completed downloads, newest first, or none
// when the page has no history yet.
func (a *App) downloadHistoryEntries() []download.HistoryEntry {
	if a.downloadHistory == nil {
		return nil
	}
	return a.downloadHistory.Snapshot()
}

// downloadHistoryList draws the completed downloads, newest first.
func (a *App) downloadHistoryList(c *ui.Context, entries []download.HistoryEntry) {
	if len(entries) == 0 {
		emptyState(c, iconHistory, a.t("download.historyEmpty"), "", nil)
		return
	}
	t := themeOf(c)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(0, contentPadding, contentPadding, contentPadding).
			Gap(t.SpaceS).Children(func() {
			for _, entry := range entries {
				a.downloadHistoryRow(c, entry)
			}
		})
	})
}

// downloadHistoryRow draws one completed download: what it was, its page and
// source, and when it finished.
func (a *App) downloadHistoryRow(c *ui.Context, entry download.HistoryEntry) {
	t := themeOf(c)
	source := a.t("download.filterOrdinary")
	if entry.Source == download.SourceSubscription {
		source = a.t("download.filterSubscription")
	}
	ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, t.SpaceL).AlignItems(ui.Center).
		Background(t.Surface).Radius(t.RadiusM).Border(1, t.Border).
		Label(a.tf("a11y.download", entry.Title)).Children(func() {
		ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
			ui.Text(c, entry.Title).FontSize(t.TextBody).TextColor(t.Text).MaxLines(1)
			ui.Text(c, entry.Artist).FontSize(t.TextTiny).TextColor(t.TextMuted).MaxLines(1)
		})
		ui.Text(c, a.tf("download.page", entry.Page+1)).FontSize(t.TextSmall).TextColor(t.TextMuted)
		ui.Text(c, source).FontSize(t.TextSmall).TextColor(t.TextMuted)
		ui.Text(c, a.tf("download.completedAt", downloadCompletedAt(entry.CompletedAt))).
			FontSize(t.TextSmall).TextColor(t.TextMuted)
		if ui.Button(c, a.t("download.openFile")).Clicked() {
			a.openDownloadPath(entry.Dest)
		}
		if ui.Button(c, a.t("download.openFolder")).Clicked() {
			a.openDownloadPath(downloadFolder(entry.Dest))
		}
	})
}

// openDownloadPath opens a completed download or its folder with the system's
// default application, and leaves a notice when the path is gone or the
// system cannot open it.
func (a *App) openDownloadPath(p string) {
	var err error
	if p == "" {
		err = os.ErrNotExist
	} else if _, err = os.Stat(p); err == nil {
		open := a.openPathFn
		if open == nil {
			open = mygo.Shell.OpenPath
		}
		err = open(p)
	}
	if err != nil {
		a.notice = a.tf("download.openFailed", err.Error())
		a.update(func() {})
	}
}

// downloadFolder is the folder a completed download sits in, or "" when the
// entry has no destination.
func downloadFolder(dest string) string {
	if dest == "" {
		return ""
	}
	return filepath.Dir(dest)
}

// downloadCompletedAt is the local time a finished download shows.
func downloadCompletedAt(at time.Time) string {
	return at.Local().Format("2006-01-02 15:04")
}

// recordDoneDownloads adds each task that reached Done since the last scan to
// the history. The recorded set keeps a later change callback from writing the
// same completion again.
func (a *App) recordDoneDownloads() {
	if a.downloadHistory == nil || a.downloads == nil {
		return
	}
	a.downloadRecordedMu.Lock()
	defer a.downloadRecordedMu.Unlock()
	if a.downloadRecorded == nil {
		a.downloadRecorded = map[string]bool{}
	}
	added := false
	for _, task := range a.downloads.Snapshot() {
		if task.State != download.Done || a.downloadRecorded[task.ID] {
			continue
		}
		a.downloadHistory.Add(download.HistoryEntry{
			ID:          task.ID,
			IllustID:    task.IllustID,
			Title:       task.Title,
			Artist:      task.Artist,
			Dest:        task.Dest,
			Page:        task.Page,
			Source:      task.Source,
			CompletedAt: time.Now(),
		})
		a.downloadRecorded[task.ID] = true
		added = true
	}
	if added {
		a.saveDownloadHistory()
	}
}

// clearDownloadHistory empties the history without touching the queue.
func (a *App) clearDownloadHistory() {
	if a.downloadHistory == nil {
		return
	}
	a.downloadHistory.Clear()
	a.saveDownloadHistory()
	a.notice = a.t("download.historyCleared")
	a.update(func() {})
}

// downloadGroupEmpty says the chosen group has nothing, without pretending the
// rest of the queue is gone.
func (a *App) downloadGroupEmpty(c *ui.Context) {
	title := a.t("download.ordinaryEmpty")
	if a.downloadFilter == downloadFilterSubscription {
		title = a.t("download.subscriptionEmpty")
	}
	emptyState(c, iconDownload, title, "", nil)
}

// downloadRow draws one task: what it is, how far it got, and its buttons.
func (a *App) downloadRow(c *ui.Context, task download.Task) {
	t := themeOf(c)
	// A paused task reads as queued in a snapshot; only this page knows.
	paused := task.State == download.Queued && a.paused[task.ID]
	label := a.downloadStateLabel(task.State)
	if paused {
		label = a.t("download.paused")
	}
	ui.Row(c).Gap(t.SpaceM).Padding(t.SpaceM, t.SpaceL).AlignItems(ui.Center).
		Background(t.Surface).Radius(t.RadiusM).Border(1, t.Border).
		Label(a.tf("a11y.download", task.Title)).Children(func() {
		ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
			ui.Text(c, task.Title).FontSize(t.TextBody).TextColor(t.Text).MaxLines(1)
			ui.Text(c, a.tf("download.page", task.Page+1)).FontSize(t.TextTiny).TextColor(t.TextMuted)
			if task.State == download.Failed && task.Err != "" {
				ui.Text(c, task.Err).FontSize(t.TextTiny).TextColor(t.Danger).MaxLines(1)
			}
		})
		ui.Text(c, label).FontSize(t.TextSmall).TextColor(downloadStateColor(t, task.State))
		if task.Total > 0 || task.State == download.Running {
			ui.Column(c).Width(160).AlignItems(ui.Center).Children(func() {
				switch {
				case task.Total > 0:
					ui.Progress(c, float64(task.Bytes)/float64(task.Total)).Width(160)
				default:
					ui.Spinner(c)
				}
			})
		}
		if task.State == download.Failed {
			if ui.Button(c, a.t("common.retry")).Clicked() {
				a.retryDownload(task)
			}
		}
		if task.State == download.Done {
			if ui.Button(c, a.t("download.openFile")).Clicked() {
				a.openDownloadPath(task.Dest)
			}
			if ui.Button(c, a.t("download.openFolder")).Clicked() {
				a.openDownloadPath(downloadFolder(task.Dest))
			}
		}
		if !downloadFinished(task.State) {
			if paused {
				if ui.Button(c, a.t("download.resume")).Clicked() {
					a.resumeDownload(task.ID)
				}
			} else if ui.Button(c, a.t("download.pause")).Clicked() {
				a.pauseDownload(task.ID)
			}
			if ui.Button(c, a.t("download.cancel")).Clicked() {
				a.cancelDownload(task.ID)
			}
		}
		if ui.Button(c, a.t("download.remove")).Clicked() {
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
