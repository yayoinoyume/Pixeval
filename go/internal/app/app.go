package app

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

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
	// paused marks the tasks the user paused: a paused task reads as queued in
	// a snapshot, so the download page cannot tell on its own.
	paused map[string]bool
	// session is the signed-in token, nil while logged out.
	session *pixiv.Token
	route   route
	// backRoute is the page a user page or series page returns to.
	backRoute route
	// viewer is the full-screen illustration viewer, nil while it is closed.
	viewer *viewerState
	// comments is the open viewer's comment panel, and commentsOpen shows it.
	comments     *commentState
	commentsOpen bool
	// userView is the user page shown, nil until one opens.
	userView *userViewState
	// seriesView is the series page shown, nil until one opens.
	seriesView *seriesViewState
	// homeTab is the home feed shown: recommended, following or ranking.
	homeTab int
	// homeFeeds are the home tabs' feeds. Each keeps its own pages and place,
	// so switching tabs loses nothing.
	homeFeeds [homeTabCount]*browseState
	// search is the last search's results.
	search *browseState
	// settings are the user's preferences, loaded at start and saved from the
	// settings page.
	settings *Settings
	// lang is the language the bundle speaks, and bundle holds its messages.
	lang   string
	bundle *i18n.Bundle
	// settingsSaved carries a save's result back to the main thread, which
	// applies it in the view.
	settingsSaved chan error
	settingsErr   string
	// history is the works the user opened, newest first.
	history *History
	// historyFailed holds the history covers that did not load, so a broken
	// URL is not asked for again every frame.
	historyFailed map[string]bool
	// bookmarkFn sets or clears a work's bookmark off the main thread; tests
	// replace it to avoid the network.
	bookmarkFn func(ctx context.Context, id int64, on bool) error
	// followFn follows or unfollows a user off the main thread; tests replace
	// it to avoid the network.
	followFn func(ctx context.Context, id int64, on bool) error
	// fetchCommentPage and addCommentFn answer the comment panel off the main
	// thread; tests replace them to avoid the network.
	fetchCommentPage func(ctx context.Context, illustID int64, offset int) (*pixiv.Page[pixiv.Comment], error)
	addCommentFn     func(ctx context.Context, illustID int64, text string) (*pixiv.Comment, error)
	// notice is a message a background task left for the next frame to show.
	notice string
}

func New() *App {
	client := pixiv.NewClient()
	settings := loadSettings()
	a := &App{
		client:        client,
		images:        imagex.New(client, imageDir()),
		downloads:     download.New(client, settings.DownloadConcurrency),
		downloadsDir:  settingsDownloadDir(settings),
		settings:      settings,
		settingsSaved: make(chan error, 1),
		paused:        map[string]bool{},
		history:       loadHistory(),
		historyFailed: map[string]bool{},
		bookmarkFn: func(ctx context.Context, id int64, on bool) error {
			if on {
				return client.AddBookmark(ctx, id, false)
			}
			return client.RemoveBookmark(ctx, id)
		},
		followFn: func(ctx context.Context, id int64, on bool) error {
			if on {
				return client.Follow(ctx, id, false)
			}
			return client.Unfollow(ctx, id)
		},
		fetchCommentPage: client.Comments,
		addCommentFn:     client.AddComment,
		route:            routeHome,
		homeFeeds:        newHomeFeeds(),
		search:           &browseState{},
	}
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
	// The queue downloads on its own goroutines and redraws the download
	// page on every change.
	a.downloads.OnChange(func() { a.update(func() {}) })
	go a.downloads.Start(context.Background())
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

// applySession makes tok the session on the main thread.
func (a *App) applySession(tok *pixiv.Token) {
	a.update(func() {
		a.session = tok
		if a.client != nil {
			a.client.SetToken(tok)
		}
		a.setTitle()
		// A signed-out start never draws the feed, so the home tab it shows
		// is armed here before the next frame loads it.
		a.armHome(a.homeTab)
	})
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
			a.homeFeeds = newHomeFeeds()
			a.armHome(a.homeTab)
		})
	}()
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
