package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/mcp"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// mcpAddr is the loopback address the local MCP server listens on.
const mcpAddr = "127.0.0.1:52163"

// App holds the state of the window. Views are rebuilt from it on the main
// thread, so state lives here and is changed from goroutines only through
// Window.Update.
type App struct {
	win    *mygo.Window
	client *pixiv.Client
	// images is reserved for the M1 browse page: thumbnails and originals.
	images *imagex.Loader
	// downloads runs the download queue, saving under downloadsDir.
	downloads    *download.Manager
	downloadsDir string
	// downloadHistory is the completed downloads, newest first, and
	// downloadRecorded the tasks already added to it, so a change callback does
	// not record the same completion again.
	downloadHistory    *download.History
	downloadRecorded   map[string]bool
	downloadRecordedMu sync.Mutex
	// paused marks the tasks the user paused: a paused task reads as queued in
	// a snapshot, so the download page cannot tell on its own.
	paused map[string]bool
	// downloadFilter is which queue group the download page shows: all,
	// ordinary or subscription.
	downloadFilter int
	// session is the signed-in token, nil while logged out.
	session *pixiv.Token
	// accounts are the saved sign-ins, and refreshTokenFn renews an expired
	// account's token when a switch adopts it; tests replace the latter to
	// avoid the network.
	accounts       *Accounts
	refreshTokenFn func(ctx context.Context, refresh string) (*pixiv.Token, error)
	// switchMu makes account switches run one at a time, and switchSeq is the
	// newest request, so one whose refresh finished late drops itself instead
	// of overwriting the newer account.
	switchMu  sync.Mutex
	switchSeq atomic.Uint64
	// webviewOK reports whether the system webview is there; without it the
	// login window takes a pasted refresh token instead of Pixiv's page.
	webviewOK bool
	route     route
	// backRoute is the page a user page or series page returns to.
	backRoute route
	// viewer is the full-screen illustration viewer, nil while it is closed.
	viewer *viewerState
	// comments is the open viewer's comment panel, and commentsOpen shows it.
	comments     *commentState
	commentsOpen bool
	// userView is the user page shown, nil until one opens.
	userView *userViewState
	// relatedUsers is the related-users page shown, nil until one opens.
	relatedUsers *relatedUsersViewState
	// recommendedUsers is the recommended-authors page's list.
	recommendedUsers *userListState
	// seriesView is the series page shown, nil until one opens.
	seriesView *seriesViewState
	// related is the related-works page shown, nil until one opens.
	related *relatedViewState
	// novelView is the novel reading page shown, nil until one opens, and
	// novelProgress the place each novel was left at.
	novelView     *novelViewState
	novelProgress *NovelProgress
	// homeTab is the home feed shown: recommended, following or ranking.
	homeTab int
	// homeFeeds are the home tabs' feeds. Each keeps its own pages and place,
	// so switching tabs loses nothing.
	homeFeeds [homeTabCount]*browseState
	// rankMode is the ranking feed's period and audience, chosen on the home
	// page's ranking tab.
	rankMode pixiv.RankMode
	// newFeed is the site-wide new works feed, and myPixivFeed the feed of
	// works by the users the signed-in user follows.
	newFeed     *browseState
	myPixivFeed *browseState
	// search is the last illustration search's results, searchNovels the last
	// novel search's and searchUsers the last user search's. searchKind is which
	// of the three the search page shows.
	search       *browseState
	searchNovels *novelFeedState
	searchUsers  *userListState
	searchKind   int
	// spotlight is the Pixivision article list the spotlight page shows.
	spotlight *spotlightState
	// searchSort and searchTarget are the illustration search's order and
	// match, chosen on the search page. The date and size fields are the
	// illustration search's advanced filters, as typed, and searchErr says
	// why the typed ones cannot be used.
	searchSort      string
	searchTarget    string
	searchStartDate string
	searchEndDate   string
	searchMinWidth  string
	searchMaxWidth  string
	searchMinHeight string
	searchMaxHeight string
	searchErr       string
	// searchHistory is the words the user searched for, newest first.
	searchHistory *SearchHistory
	// settings are the user's preferences, loaded at start and saved from the
	// settings page. shortcutsOpen shows the shortcut hint in its About card.
	settings      *Settings
	shortcutsOpen bool
	// cacheUsage is the image cache size the settings page shows, measured at
	// cacheUsageAt so a redraw every frame does not scan the cache directory.
	cacheUsage   int64
	cacheUsageAt time.Time
	// cardText is the card title-and-author height for the current text scale,
	// read on the main thread before a load goroutine uses it.
	cardText float64
	// lang is the language the bundle speaks, and bundle holds its messages.
	lang   string
	bundle *i18n.Bundle
	// settingsSaved carries a save's result back to the main thread, which
	// applies it in the view.
	settingsSaved chan error
	settingsErr   string
	// settingsResetPending makes the save that follows a reset report the
	// reset rather than a plain save.
	settingsResetPending bool
	// history is the works the user opened, newest first.
	history *History
	// historyFailed holds the history covers that did not load, so a broken
	// URL is not asked for again every frame.
	historyFailed map[string]bool
	// watchLater is the works the user saved for later, newest first, and
	// watchLaterFailed the covers of it that did not load.
	watchLater       *WatchLater
	watchLaterFailed map[string]bool
	// blocked is the users whose works and lists are hidden, and blockedTags
	// the illustration tags whose works are hidden.
	blocked     *BlockedUsers
	blockedTags *BlockedTags
	// bookmarkFn sets or clears a work's bookmark off the main thread; tests
	// replace it to avoid the network.
	bookmarkFn func(ctx context.Context, id int64, on bool) error
	// bookmarksTab is the bookmarks page's tab, public or private, and
	// bookmarks is its feed. Switching tabs restarts the feed, so works of
	// the other tab never show in it.
	bookmarksTab int
	bookmarks    *browseState
	// bookmarksPageFn fetches one page of the signed-in user's bookmarks off
	// the main thread; tests replace it to avoid the network. An empty next
	// asks for the tab's first page.
	bookmarksPageFn func(ctx context.Context, private bool, next string) (*pixiv.Page[pixiv.Illustration], error)
	// followFn follows or unfollows a user off the main thread; tests replace
	// it to avoid the network.
	followFn func(ctx context.Context, id int64, on bool) error
	// followPrivateFn privately follows a user off the main thread, so the
	// follow stays off the user's profile; tests replace it to avoid the
	// network.
	followPrivateFn func(ctx context.Context, id int64) error
	// fetchCommentPage, addCommentFn and deleteCommentFn answer the comment
	// panel off the main thread; tests replace them to avoid the network.
	fetchCommentPage func(ctx context.Context, illustID int64, offset int) (*pixiv.Page[pixiv.Comment], error)
	addCommentFn     func(ctx context.Context, illustID int64, text string) (*pixiv.Comment, error)
	deleteCommentFn  func(ctx context.Context, commentID int64) error
	// novelContent fetches a novel body off the main thread; tests replace it
	// to avoid the network.
	novelContent func(ctx context.Context, novelID int64) (string, error)
	// novelBookmarkFn sets or clears a novel's bookmark off the main thread;
	// tests replace it to avoid the network.
	novelBookmarkFn func(ctx context.Context, id int64, on bool) error
	// illustrationFn fetches one work's detail, which carries a novel body
	// image's URL; tests replace it to avoid the network.
	illustrationFn func(ctx context.Context, id int64) (*pixiv.Illustration, error)
	// relatedFn fetches one page of a work's related list off the main thread;
	// tests replace it to avoid the network.
	relatedFn func(ctx context.Context, illustID int64) (*pixiv.Page[pixiv.Illustration], error)
	// spotlightFn fetches one page of Pixivision articles off the main thread;
	// tests replace it to avoid the network.
	spotlightFn func(ctx context.Context, next string) (*pixiv.Page[pixiv.SpotlightArticle], error)
	// openExternalFn opens a URL in the default browser; tests replace it so a
	// click does not leave the app.
	openExternalFn func(url string) error
	// subsStore holds the followed users and series, loaded at start, and subs
	// checks them on demand and every subscriptionInterval.
	subsStore *subscription.Store
	subs      *subscription.Service
	// subsRunning marks the check loop as started, so a settings save restarts
	// only a running one. subsRestartMu keeps two restarts from overlapping.
	subsRunning   bool
	subsRestartMu sync.Mutex
	// subsMu guards newWorks, subsChecking and subsMsg, which a check fills in
	// off the main thread for the subscription page.
	subsMu       sync.Mutex
	newWorks     []pixiv.Illustration
	subsChecking bool
	// subsMsg is a check's result, which the subscription page toasts once.
	subsMsg string
	// notice is a message a background task left for the next frame to show.
	notice string
	// updater checks for and installs new versions; tests replace it.
	updater updater
	// updateMu guards updateSt, which a check fills in off the main thread.
	updateMu sync.Mutex
	updateSt updateState
}

// updater checks for and installs new versions of the app. mygo's Install is
// a method of the update it installs, so the interface takes the update.
type updater interface {
	Enabled() bool
	Check(ctx context.Context) (*mygo.Update, error)
	Install(ctx context.Context, u *mygo.Update) error
}

// mygoUpdater is the updater the app ships with.
type mygoUpdater struct{}

func (mygoUpdater) Enabled() bool { return mygo.Updater.Enabled() }

func (mygoUpdater) Check(ctx context.Context) (*mygo.Update, error) {
	return mygo.Updater.Check(ctx)
}

func (mygoUpdater) Install(ctx context.Context, u *mygo.Update) error {
	return u.Install(ctx, nil)
}

func New() *App {
	client := pixiv.NewClient()
	settings := loadSettings()
	client.SetAllowR18(settings.ShowR18)
	if err := client.SetProxy(settings.ProxyURL); err != nil {
		log.Printf("settings: proxy: %v", err)
	}
	subsStore := &subscription.Store{}
	if err := loadSubscriptions(subsStore); err != nil {
		log.Printf("subscriptions: %v", err)
	}
	subs := subscription.New(pixivFetcher{client: client})
	subs.Store = subsStore
	a := &App{
		client:           client,
		accounts:         loadAccounts(),
		subsStore:        subsStore,
		subs:             subs,
		images:           imagex.New(client, imageDir()),
		downloads:        download.New(client, settings.DownloadConcurrency),
		downloadsDir:     settingsDownloadDir(settings),
		downloadHistory:  loadDownloadHistory(),
		downloadRecorded: map[string]bool{},
		settings:         settings,
		settingsSaved:    make(chan error, 1),
		paused:           map[string]bool{},
		history:          loadHistory(),
		historyFailed:    map[string]bool{},
		watchLater:       loadWatchLater(),
		watchLaterFailed: map[string]bool{},
		blocked:          loadBlockedUsers(),
		blockedTags:      loadBlockedTags(),
		novelProgress:    loadNovelProgress(),
		bookmarkFn: func(ctx context.Context, id int64, on bool) error {
			if on {
				return client.AddBookmark(ctx, id, false)
			}
			return client.RemoveBookmark(ctx, id)
		},
		bookmarks: &browseState{},
		refreshTokenFn: func(ctx context.Context, refresh string) (*pixiv.Token, error) {
			return pixiv.RefreshToken(ctx, loginHTTP, refresh)
		},
		bookmarksPageFn: func(ctx context.Context, private bool, next string) (*pixiv.Page[pixiv.Illustration], error) {
			if next != "" {
				return client.NextIllustrations(ctx, next)
			}
			var userID int64
			if tok := client.Token(); tok != nil {
				userID = int64(tok.User.ID)
			}
			return client.BookmarkedIllustrations(ctx, userID, private, 0)
		},
		followFn: func(ctx context.Context, id int64, on bool) error {
			if on {
				return client.Follow(ctx, id, false)
			}
			return client.Unfollow(ctx, id)
		},
		followPrivateFn: func(ctx context.Context, id int64) error {
			return client.Follow(ctx, id, true)
		},
		fetchCommentPage: client.Comments,
		addCommentFn:     client.AddComment,
		deleteCommentFn:  client.DeleteComment,
		novelContent:     client.NovelContent,
		novelBookmarkFn: func(ctx context.Context, id int64, on bool) error {
			if on {
				return client.AddNovelBookmark(ctx, id, false)
			}
			return client.RemoveNovelBookmark(ctx, id)
		},
		illustrationFn: client.Illustration,
		relatedFn:      client.RelatedIllustrations,
		spotlightFn: func(ctx context.Context, next string) (*pixiv.Page[pixiv.SpotlightArticle], error) {
			if next != "" {
				return client.NextSpotlightArticles(ctx, next)
			}
			return client.SpotlightArticles(ctx)
		},
		openExternalFn:   mygo.Shell.OpenExternal,
		route:            routeHome,
		cardText:         60,
		homeFeeds:        newHomeFeeds(),
		newFeed:          &browseState{},
		myPixivFeed:      &browseState{},
		rankMode:         pixiv.Day,
		search:           &browseState{},
		searchNovels:     &novelFeedState{},
		searchUsers:      &userListState{},
		recommendedUsers: &userListState{},
		spotlight:        &spotlightState{},
		searchSort:       searchSortDateDesc,
		searchTarget:     searchTargetPartial,
		searchHistory:    loadSearchHistory(),
		updater:          mygoUpdater{},
	}
	// The check probes the install directory, so the settings page asks once.
	a.updateSt.off = !a.updater.Enabled()
	// The saved limit applies before the first image loads.
	a.images.SetDiskLimit(imageCacheLimitBytes(settings))
	// The check runs on the timer goroutine, so it hands the works to the main
	// thread; the subscription page toasts the result when it next draws.
	subs.OnNew = a.onNewSubscriptions
	a.loadLanguage()
	return a
}

// loadLanguage makes the messages the ones the settings ask for: the saved
// language, or the system's when the setting follows it.
func (a *App) loadLanguage() {
	lang := i18n.LangAuto
	if a.settings != nil {
		lang = a.settings.Language
	}
	b, err := i18n.Load(lang)
	if err != nil {
		log.Printf("i18n: %v", err)
		return
	}
	a.bundle = b
	a.lang = b.Lang()
}

// t is the message for key in the current language.
func (a *App) t(key string) string {
	if a == nil {
		return key
	}
	return a.bundle.T(key)
}

// tf is t with the format's arguments filled in.
func (a *App) tf(key string, args ...any) string {
	if a == nil {
		return key
	}
	return a.bundle.Tf(key, args...)
}

// downloadsPath is where the unfinished download queue lives: the user's
// config directory, like the settings.
func downloadsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "downloads.json"), nil
}

// downloadHistoryPath is where the completed download records live.
func downloadHistoryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "download_history.json"), nil
}

// loadDownloadHistory reads the completed downloads an earlier run saved. A
// missing or broken file starts empty instead of blocking the start.
func loadDownloadHistory() *download.History {
	path, err := downloadHistoryPath()
	if err != nil {
		log.Printf("download history: %v", err)
		return download.NewHistory()
	}
	h, err := download.LoadHistory(path)
	if err != nil {
		log.Printf("download history: %v", err)
	}
	return h
}

// saveDownloadHistory writes the completed download records.
func (a *App) saveDownloadHistory() {
	if a.downloadHistory == nil {
		return
	}
	path, err := downloadHistoryPath()
	if err != nil {
		log.Printf("download history: %v", err)
		return
	}
	if err := a.downloadHistory.Save(path); err != nil {
		log.Printf("download history: save: %v", err)
	}
}

// restoreDownloads brings back the queue an earlier run left unfinished. A
// missing or broken file starts empty instead of blocking the start.
func (a *App) restoreDownloads() {
	path, err := downloadsPath()
	if err != nil {
		log.Printf("downloads: %v", err)
		return
	}
	tasks, err := download.Load(path)
	if err != nil {
		log.Printf("downloads: %v", err)
		return
	}
	a.downloads.Restore(tasks)
}

// saveDownloads writes the unfinished queue; the manager skips the write when
// only progress changed.
func (a *App) saveDownloads() {
	path, err := downloadsPath()
	if err != nil {
		log.Printf("downloads: %v", err)
		return
	}
	if err := a.downloads.Save(path); err != nil {
		log.Printf("downloads: save: %v", err)
	}
}

// imageDir is the image cache under the user's cache directory, created when
// it is missing. An empty result turns the disk cache off.
func imageDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		log.Printf("image cache: %v", err)
		return ""
	}
	dir := filepath.Join(base, "pixeval-go", "images")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("image cache: %v", err)
		return ""
	}
	return dir
}

func (a *App) Start() {
	// The shell draws native UI and needs no webview; the login window does,
	// so ask once before it can open.
	a.webviewOK = mygo.WebViewAvailable()
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     a.title(),
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		Content:   ui.View(a.View),
	})
	// A refresh can take a network round trip: the window opens first and
	// applySession fills in the session when it lands.
	go a.restoreSession()
	// The queue an earlier run left unfinished comes back as queued tasks,
	// then downloads on its own goroutines and redraws the page on change.
	a.restoreDownloads()
	a.downloads.OnChange(func() {
		a.recordDoneDownloads()
		a.saveDownloads()
		a.update(func() {})
	})
	go a.downloads.Start(context.Background())
	// A busy port or a failed listen leaves the app running without MCP.
	go func() {
		if err := mcp.ListenAndServe(mcpAddr, a); err != nil {
			log.Printf("mcp: %v", err)
		}
	}()
	// The subscription check looks for new works every subscriptionInterval.
	a.subsRunning = true
	a.subs.Start(context.Background(), a.subscriptionInterval())
}

// restoreSession adopts the saved token, refreshing it when expired. It runs
// off the main thread once the window is open, so a failure leaves the app
// logged out instead of blocking the start.
func (a *App) restoreSession() {
	path, err := tokenPath()
	if err != nil {
		log.Printf("pixiv session: %v", err)
		return
	}
	tok, err := pixiv.LoadToken(path)
	if err != nil {
		// No saved token is the normal logged-out start.
		return
	}
	if tok.Expired() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err = pixiv.RefreshToken(ctx, loginHTTP, tok.RefreshToken)
		if err != nil {
			log.Printf("pixiv session: refresh: %v", err)
			return
		}
		// Pixiv rotates the refresh token, so the new one must be saved.
		if err := pixiv.SaveToken(path, tok); err != nil {
			log.Printf("pixiv session: save: %v", err)
		}
	}
	// The token file is the session in use: keep the account list in step with
	// it, including a login saved by a version that only knew token.json.
	if err := a.rememberAccount(tok); err != nil {
		log.Printf("accounts: %v", err)
	}
	a.applySession(tok)
}

// syncSession adopts the token the login window just saved. onLoggedIn calls
// it from the login goroutine.
func (a *App) syncSession() {
	path, err := tokenPath()
	if err != nil {
		log.Printf("pixiv session: %v", err)
		return
	}
	tok, err := pixiv.LoadToken(path)
	if err != nil {
		log.Printf("pixiv session: %v", err)
		return
	}
	a.applySession(tok)
}

// applySession makes tok the session on the main thread. The pages hold the
// account before it, so they start empty.
func (a *App) applySession(tok *pixiv.Token) {
	a.update(func() {
		a.session = tok
		if a.client != nil {
			a.client.SetToken(tok)
		}
		a.setTitle()
		a.resetAccountPages()
	})
}

// resetAccountPages drops the pages that held the signed-in account's works,
// so the next account never shows them.
func (a *App) resetAccountPages() {
	a.homeFeeds = newHomeFeeds()
	// A signed-out start never draws the feed, so the home tab it shows is
	// armed here before the next frame loads it.
	a.armHome(a.homeTab)
	a.bookmarks = &browseState{}
	a.newFeed = &browseState{}
	a.myPixivFeed = &browseState{}
	a.search = &browseState{}
	a.searchNovels = &novelFeedState{}
	a.searchUsers = &userListState{}
	a.userView = nil
	a.relatedUsers = nil
	a.seriesView = nil
	a.related = nil
	a.novelView = nil
	switch a.route {
	case routeUser, routeUserRelated, routeSeries, routeRelated, routeNovel:
		a.route = routeHome
	}
}

// logout forgets the session off the main thread: the token file goes, then
// the shell redraws logged out.
func (a *App) logout() {
	path, err := tokenPath()
	go func() {
		if err == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				// The token is still there: stay signed in rather than claim
				// a logout that did not happen.
				log.Printf("pixiv logout: %v", err)
				return
			}
		}
		a.update(func() {
			a.session = nil
			if a.client != nil {
				a.client.SetToken(nil)
			}
			a.setTitle()
			// The feeds hold the signed-out account's works: drop them so the
			// next login starts empty.
			a.resetAccountPages()
		})
	}()
}

// Search returns the works matching query; the MCP search tool.
func (a *App) Search(ctx context.Context, query string, limit int) ([]pixiv.Illustration, error) {
	page, err := a.client.SearchIllustrations(ctx, pixiv.SearchOptions{Word: query})
	if err != nil {
		return nil, err
	}
	return page.Items[:min(limit, len(page.Items))], nil
}

// Recommended returns Pixiv's feed for the user; the MCP feed tool.
func (a *App) Recommended(ctx context.Context, limit int) ([]pixiv.Illustration, error) {
	page, err := a.client.Recommended(ctx)
	if err != nil {
		return nil, err
	}
	return page.Items[:min(limit, len(page.Items))], nil
}

// Detail returns one work; the MCP detail tool.
func (a *App) Detail(ctx context.Context, id int64) (*pixiv.Illustration, error) {
	return a.client.Illustration(ctx, id)
}

// Ranking returns the top works of mode; the MCP ranking tool.
func (a *App) Ranking(ctx context.Context, mode string, limit int) ([]pixiv.Illustration, error) {
	page, err := a.client.Ranking(ctx, pixiv.RankMode(mode), "")
	if err != nil {
		return nil, err
	}
	return page.Items[:min(limit, len(page.Items))], nil
}

// SetBookmark adds or removes a work's bookmark; the MCP bookmark tool.
func (a *App) SetBookmark(ctx context.Context, id int64, on bool) error {
	if a.bookmarkFn == nil {
		return errors.New("bookmarks are unavailable")
	}
	return a.bookmarkFn(ctx, id, on)
}

// QueueDownload queues every page of a work; the MCP download tool.
func (a *App) QueueDownload(ctx context.Context, id int64) (int, error) {
	if a.illustrationFn == nil {
		return 0, errors.New("downloads are unavailable")
	}
	w, err := a.illustrationFn(ctx, id)
	if err != nil {
		return 0, err
	}
	n, ok := a.downloadWork(*w)
	if !ok {
		return 0, errors.New("downloads are off: no download directory")
	}
	return n, nil
}

// Status reports the download queue; the MCP status tool. It reads only the
// queue, whose snapshot is safe off the main thread.
func (a *App) Status() string {
	if a.downloads == nil {
		return "downloads: unavailable"
	}
	return fmt.Sprintf("downloads: %d task(s) in %s", len(a.downloads.Snapshot()), a.downloadsDir)
}

// update runs fn on the main thread, or at once before the window exists.
func (a *App) update(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// showNotice shows the message a background task left, once.
func (a *App) showNotice(c *ui.Context) {
	if a.notice == "" {
		return
	}
	c.Toast(a.notice)
	a.notice = ""
}

func (a *App) title() string {
	name := a.userName()
	if name == "" {
		return "Pixeval Go"
	}
	return "Pixeval Go — " + name
}

func (a *App) setTitle() {
	if a.win != nil {
		a.win.SetTitle(a.title())
	}
}

func (a *App) userName() string {
	if a.session == nil {
		return ""
	}
	if name := a.session.User.Name; name != "" {
		return name
	}
	return a.session.User.Account
}
