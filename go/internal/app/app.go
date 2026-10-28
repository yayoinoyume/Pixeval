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
	// viewer is the full-screen illustration viewer, nil while it is closed.
	viewer *viewerState
	// home is the recommended feed, search the last search's results.
	home   *browseState
	search *browseState
}

func New() *App {
	client := pixiv.NewClient()
	return &App{
		client:       client,
		images:       imagex.New(client, imageDir()),
		downloads:    download.New(client, downloadConcurrency),
		downloadsDir: downloadDir(),
		paused:       map[string]bool{},
		route:        routeHome,
		home:         newHomeState(client),
		search:       &browseState{},
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
	// The feeds load once the window runs, not before it can draw them.
	a.home.armed = true
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
