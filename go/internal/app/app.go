package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/mcp"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/saucenao"
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
	// loginHTTP is the login exchange's client: OAuth token exchange and
	// refresh use the shared client's transport, so the saved proxy and domain
	// fronting apply to them too. Its own timeout bounds a stuck request.
	loginHTTP *http.Client
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
	// myPixivUsers is the MyPixiv-users page shown, nil until one opens.
	myPixivUsers *myPixivUsersViewState
	// recommendedUsers is the recommended-authors page's list.
	recommendedUsers *userListState
	// myFollowing, myFollowers and myPixivUsersList are the signed-in user's
	// own people lists, each kept apart from the same list on a user's page.
	myFollowing      *userListState
	myFollowers      *userListState
	myPixivUsersList *userListState
	// seriesView is the series page shown, nil until one opens.
	seriesView *seriesViewState
	// seriesFeeds are the series watchlist tabs' lists, one per kind, and
	// seriesTab is the kind shown.
	seriesFeeds [seriesTabCount]*seriesListState
	seriesTab   int
	// related is the related-works page shown, nil until one opens.
	related *relatedViewState
	// novelView is the novel reading page shown, nil until one opens, and
	// novelProgress the place each novel was left at.
	novelView     *novelViewState
	novelProgress *NovelProgress
	// homeCards are the home page's cards, in the order they draw, each with
	// the one page its preview loaded.
	homeCards []homeCard
	// feed is the full feed a card's "view all" opened, nil on the home page.
	feed *homeFeedView
	// newFeed is the site-wide new works feed, and myPixivFeed the feed of
	// works by the users the signed-in user follows. newKind and myPixivKind
	// are the kind each page shows, illust or manga; switching kind restarts
	// that page's feed, so the other kind's works never show in it.
	newFeed     *browseState
	myPixivFeed *browseState
	newKind     int
	myPixivKind int
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
	// searchTags are the tags the search page shows under its box, loaded
	// once its box first draws, and searchLinkErr says why a pasted Pixiv link
	// did not open. Both fill in off the main thread, so searchSideMu guards
	// them. trendingFn fetches the tags, wired when the window starts; tests
	// replace it.
	searchSideMu   sync.Mutex
	searchTags     []pixiv.TrendingTag
	searchLinkErr  string
	trendingFn     func(ctx context.Context) ([]pixiv.TrendingTag, error)
	trendingLoaded bool
	// sauceURL is the reverse image search's image URL, as typed; sauce holds
	// its results. sauceSearchFn runs the search off the main thread; tests
	// replace it to avoid the network.
	sauceURL      string
	sauce         *sauceState
	sauceSearchFn func(ctx context.Context, apiKey, imageURL string) (*saucenao.Result, error)
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
	// bookmarkWithTagsFn adds a work's bookmark with its visibility and tags
	// off the main thread. Start wires it when the window runs; tests replace
	// it to avoid the network.
	bookmarkWithTagsFn func(ctx context.Context, id int64, private bool, tags []string) error
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
	// fetchNovelCommentPage fetches one page of a novel's comments off the main
	// thread; tests replace it to avoid the network.
	fetchNovelCommentPage func(ctx context.Context, novelID int64, offset int) (*pixiv.Page[pixiv.Comment], error)
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
	// openPathFn opens a downloaded file or folder with the system's default
	// application; tests replace it for the same reason.
	openPathFn func(path string) error
	// fetchImageBytes downloads a picture's bytes off the main thread, and
	// clipboardWriteImage puts a PNG on the clipboard; tests replace both to
	// avoid the network and the system clipboard.
	fetchImageBytes     func(ctx context.Context, url string) ([]byte, error)
	clipboardWriteImage func(png []byte) error
	// clipboardWriteText puts text on the clipboard, as the MCP card's copy
	// button does; tests replace it to avoid the system clipboard.
	clipboardWriteText func(text string) error
	// notifyFn shows a desktop notification; tests replace it so a check never
	// reaches the system's notification center.
	notifyFn func(title, body string) error
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
	newWorks     []subscription.Fetched
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
	if err := applyNetworkSettings(client, settings); err != nil {
		log.Printf("settings: network: %v", err)
	}
	loginClient := client.NewHTTPClient(loginTimeout)
	subsStore := &subscription.Store{}
	if err := loadSubscriptions(subsStore); err != nil {
		log.Printf("subscriptions: %v", err)
	}
	subs := subscription.New(pixivFetcher{client: client})
	subs.Store = subsStore
	a := &App{
		client:           client,
		loginHTTP:        loginClient,
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
			return pixiv.RefreshToken(ctx, loginClient, refresh)
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
		fetchCommentPage:      client.Comments,
		addCommentFn:          client.AddComment,
		deleteCommentFn:       client.DeleteComment,
		novelContent:          client.NovelContent,
		fetchNovelCommentPage: client.NovelComments,
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
		openPathFn:       mygo.Shell.OpenPath,
		route:            routeHome,
		cardText:         60,
		homeCards:        loadHomeLayout(),
		newFeed:          &browseState{},
		myPixivFeed:      &browseState{},
		search:           &browseState{},
		searchNovels:     &novelFeedState{},
		searchUsers:      &userListState{},
		recommendedUsers: &userListState{},
		myFollowing:      &userListState{},
		myFollowers:      &userListState{},
		myPixivUsersList: &userListState{},
		seriesFeeds:      [seriesTabCount]*seriesListState{{}, {}},
		spotlight:        &spotlightState{},
		searchSort:       searchSortDateDesc,
		searchTarget:     searchTargetPartial,
		searchHistory:    loadSearchHistory(),
		sauce:            &sauceState{},
		sauceSearchFn:    saucenao.NewClient().Search,
		updater:          mygoUpdater{},
	}
	// The check probes the install directory, so the settings page asks once.
	a.updateSt.off = !a.updater.Enabled()
	// A viewer's copy and save take the picture over the network and put it on
	// the clipboard; tests replace both to avoid either.
	a.fetchImageBytes = client.GetImage
	a.clipboardWriteImage = mygo.Clipboard.WriteImage
	a.clipboardWriteText = func(text string) error {
		return mygo.Clipboard.Write(transfer.TextData(text))
	}
	// A notification needs the window's main loop, which a test binary has
	// none of, and Show would wait for one forever.
	a.notifyFn = func(title, body string) error {
		if a.win == nil {
			return nil
		}
		return mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body}).Show()
	}
	// The saved limit applies before the first image loads.
	a.images.SetDiskLimit(imageCacheLimitBytes(settings))
	// The check runs on the timer goroutine, so it hands the works to the main
	// thread; the subscription page toasts the result when it next draws.
	subs.OnNewFetched = a.onNewSubscriptions
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
	// The search page's trending tags are a shortcut, not a result: they load
	// once its box first draws. Wired here so a headless draw asks for nothing.
	a.trendingFn = a.client.TrendingTags
	a.bookmarkWithTagsFn = func(ctx context.Context, id int64, private bool, tags []string) error {
		return a.client.AddBookmarkWithTags(ctx, id, private, tags)
	}
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
	// A busy port or a failed listen leaves the app running without MCP. The
	// server takes the saved bearer token, generated on the first start.
	if token := a.ensureMCPToken(); token != "" {
		go func() {
			if err := mcp.ListenAndServe(mcpAddr, a, token); err != nil {
				log.Printf("mcp: %v", err)
			}
		}()
	}
	// The subscription check looks for new works every subscriptionInterval.
	a.subsRunning = true
	a.subs.Start(context.Background(), a.subscriptionInterval())
}

// ensureMCPToken is the bearer token the local MCP server requires: the saved
// one, or a fresh one saved for the next start. It is never logged: the
// settings page is where the user reads it. An empty result means generating
// or saving failed, and the server stays off rather than open.
func (a *App) ensureMCPToken() string {
	if a.settings == nil {
		return ""
	}
	if a.settings.MCPToken != "" {
		return a.settings.MCPToken
	}
	token, err := newMCPToken()
	if err != nil {
		log.Printf("mcp: generate token: %v", err)
		return ""
	}
	a.settings.MCPToken = token
	path, err := settingsPath()
	if err != nil {
		log.Printf("mcp: %v", err)
		return ""
	}
	if err := Save(path, a.settings); err != nil {
		log.Printf("mcp: save token: %v", err)
		return ""
	}
	return token
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
		tok, err = pixiv.RefreshToken(ctx, a.loginHTTP, tok.RefreshToken)
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
	// The layout is the person's, not the account's: keep the cards and drop
	// the works the account before them loaded.
	for i := range a.homeCards {
		a.homeCards[i].feed = &browseState{}
	}
	a.feed = nil
	if a.session != nil {
		// The home page arms its cards as it draws them; arming here lets the
		// next frame load without a blank one in between.
		a.armHomeCards()
	}
	a.bookmarks = &browseState{}
	a.newFeed = &browseState{}
	a.myPixivFeed = &browseState{}
	a.newKind = feedIllust
	a.myPixivKind = feedIllust
	a.search = &browseState{}
	a.searchNovels = &novelFeedState{}
	a.searchUsers = &userListState{}
	a.myFollowing = &userListState{}
	a.myFollowers = &userListState{}
	a.myPixivUsersList = &userListState{}
	a.userView = nil
	a.relatedUsers = nil
	a.myPixivUsers = nil
	a.seriesView = nil
	a.seriesFeeds = [seriesTabCount]*seriesListState{{}, {}}
	a.related = nil
	a.novelView = nil
	switch a.route {
	case routeUser, routeUserRelated, routeUserMyPixiv, routeSeries, routeRelated, routeNovel, routeFeed:
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

// SearchUsers returns the users matching word; the MCP user search tool.
func (a *App) SearchUsers(ctx context.Context, word string) ([]pixiv.UserPreview, error) {
	page, err := a.client.SearchUsers(ctx, word)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// UserDetail returns one user's profile; the MCP user detail tool.
func (a *App) UserDetail(ctx context.Context, id int64) (*pixiv.UserDetail, error) {
	return a.client.UserDetail(ctx, id)
}

// UserIllusts returns a user's works of kind; the MCP user works tool.
func (a *App) UserIllusts(ctx context.Context, id int64, kind string) ([]pixiv.Illustration, error) {
	page, err := a.client.UserIllusts(ctx, id, kind)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// UserBookmarks returns the signed-in user's bookmarks; the MCP bookmarks
// tool. A zero userID means the signed-in account.
func (a *App) UserBookmarks(ctx context.Context, userID int64, private bool) ([]pixiv.Illustration, error) {
	if userID <= 0 {
		if tok := a.client.Token(); tok != nil {
			userID = int64(tok.User.ID)
		}
	}
	if userID <= 0 {
		return nil, errors.New("not signed in")
	}
	page, err := a.client.BookmarkedIllustrations(ctx, userID, private, 0)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// SearchNovels returns the novels matching word; the MCP novel search tool.
func (a *App) SearchNovels(ctx context.Context, word string) ([]pixiv.Novel, error) {
	page, err := a.client.SearchNovels(ctx, pixiv.SearchOptions{Word: word})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// NovelComments returns the first page of comments on a novel; the MCP novel
// comments tool.
func (a *App) NovelComments(ctx context.Context, novelID int64) ([]pixiv.Comment, error) {
	if a.fetchNovelCommentPage == nil {
		return nil, errors.New("novel comments are unavailable")
	}
	page, err := a.fetchNovelCommentPage(ctx, novelID, 0)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// WatchLater returns the works saved for later; the MCP watch later tool.
func (a *App) WatchLater(_ context.Context) ([]mcp.SavedWork, error) {
	if a.watchLater == nil {
		return nil, nil
	}
	entries := a.watchLater.Snapshot()
	out := make([]mcp.SavedWork, len(entries))
	for i, e := range entries {
		out[i] = mcp.SavedWork{ID: e.ID, Title: e.Title, Artist: e.Artist, SavedAt: e.At}
	}
	return out, nil
}

// DownloadHistory returns the completed downloads; the MCP download history
// tool.
func (a *App) DownloadHistory(_ context.Context) ([]mcp.DownloadedWork, error) {
	if a.downloadHistory == nil {
		return nil, nil
	}
	entries := a.downloadHistory.Snapshot()
	out := make([]mcp.DownloadedWork, len(entries))
	for i, e := range entries {
		out[i] = mcp.DownloadedWork{
			IllustID:    e.IllustID,
			Title:       e.Title,
			Artist:      e.Artist,
			Dest:        e.Dest,
			Page:        e.Page,
			CompletedAt: e.CompletedAt,
		}
	}
	return out, nil
}

// Subscriptions returns the followed users, tags and series; the MCP
// subscriptions tool.
func (a *App) Subscriptions(_ context.Context) ([]mcp.SubscriptionInfo, error) {
	if a.subsStore == nil {
		return nil, nil
	}
	subs := a.subsStore.List()
	out := make([]mcp.SubscriptionInfo, len(subs))
	for i, s := range subs {
		out[i] = mcp.SubscriptionInfo{
			Kind:        string(s.Kind),
			TargetID:    s.TargetID,
			TargetName:  s.TargetName,
			LastChecked: s.LastChecked,
		}
	}
	return out, nil
}

// RelatedIllusts returns the works Pixiv relates to id; the MCP related
// works tool.
func (a *App) RelatedIllusts(ctx context.Context, id int64) ([]pixiv.Illustration, error) {
	page, err := a.client.RelatedIllustrations(ctx, id)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// Spotlight returns the Pixivision articles Pixiv features; the MCP spotlight
// tool.
func (a *App) Spotlight(ctx context.Context) ([]pixiv.SpotlightArticle, error) {
	page, err := a.client.SpotlightArticles(ctx)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// NewWorks returns the site-wide newest works of kind; the MCP new works tool.
func (a *App) NewWorks(ctx context.Context, kind string) ([]pixiv.Illustration, error) {
	page, err := a.client.NewIllustrations(ctx, pixiv.WorkKind(kind))
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// FollowingWorks returns the public works of followed users; the MCP
// following works tool.
func (a *App) FollowingWorks(ctx context.Context) ([]pixiv.Illustration, error) {
	page, err := a.client.Following(ctx)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// MyPixivWorks returns works by the account's MyPixiv users; the MCP MyPixiv
// works tool.
func (a *App) MyPixivWorks(ctx context.Context, kind string) ([]pixiv.Illustration, error) {
	page, err := a.client.MyPixivIllustrations(ctx, pixiv.WorkKind(kind))
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// signedInUserID is the signed-in account's Pixiv id, or 0 when logged out.
func (a *App) signedInUserID() int64 {
	if tok := a.client.Token(); tok != nil {
		return int64(tok.User.ID)
	}
	return 0
}

// FollowingUsers returns the users userID follows publicly; a zero userID
// means the signed-in account. The MCP following users tool.
func (a *App) FollowingUsers(ctx context.Context, userID int64) ([]pixiv.UserPreview, error) {
	if userID <= 0 {
		userID = a.signedInUserID()
	}
	if userID <= 0 {
		return nil, errors.New("not signed in")
	}
	page, err := a.client.UserFollowing(ctx, userID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// Followers returns the users following the signed-in account; the MCP
// followers tool.
func (a *App) Followers(ctx context.Context) ([]pixiv.UserPreview, error) {
	userID := a.signedInUserID()
	if userID <= 0 {
		return nil, errors.New("not signed in")
	}
	page, err := a.client.UserFollowers(ctx, userID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// MyPixivUsers returns the users userID added to MyPixiv; the MCP MyPixiv
// users tool.
func (a *App) MyPixivUsers(ctx context.Context, userID int64) ([]pixiv.UserPreview, error) {
	page, err := a.client.UserMyPixiv(ctx, userID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// RelatedUsers returns the users Pixiv lists beside userID; the MCP related
// users tool.
func (a *App) RelatedUsers(ctx context.Context, userID int64) ([]pixiv.UserPreview, error) {
	page, err := a.client.RelatedUsers(ctx, userID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// SeriesWatchlist returns the followed series of kind; the MCP series
// watchlist tool.
func (a *App) SeriesWatchlist(ctx context.Context, kind string) ([]pixiv.SeriesSummary, error) {
	page, err := a.client.SeriesWatchlist(ctx, pixiv.SeriesKind(kind))
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// TrendingTags returns Pixiv's trending illustration tags; the MCP trending
// tags tool.
func (a *App) TrendingTags(ctx context.Context) ([]pixiv.TrendingTag, error) {
	return a.client.TrendingTags(ctx)
}

// SettingsSummary describes the saved preferences for the MCP settings tool.
// Secrets, the proxy URL and resolver lists stay out of it.
func (a *App) SettingsSummary(context.Context) (string, error) {
	s := a.settings
	if s == nil {
		return "", errors.New("settings are unavailable")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "theme: %s\nlanguage: %s\n", s.Theme, s.Language)
	fmt.Fprintf(&b, "download dir: %s\nconcurrency: %d\nmacro: %s\n",
		s.DownloadDir, s.DownloadConcurrency, s.DownloadMacro)
	fmt.Fprintf(&b, "show R-18: %t\ncard width: %g\n", s.ShowR18, s.CardWidth)
	fmt.Fprintf(&b, "auto-download subscriptions: %t\nnotifications: %t\ncheck every %d min\n",
		s.AutoDownloadSubscriptions, s.NotificationsEnabled, s.SubscriptionIntervalMinutes)
	fmt.Fprintf(&b, "novel font %d, line height %g, width %g\n",
		s.NovelFontSize, s.NovelLineHeight, s.NovelWidth)
	fmt.Fprintf(&b, "image cache limit: %d MB", s.ImageCacheLimitMB)
	return b.String(), nil
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
