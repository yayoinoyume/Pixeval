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
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// loginHTTP is the client of the login exchange. It has its own timeout so a
// stuck request cannot hold the login window.
var loginHTTP = &http.Client{Timeout: 30 * time.Second}

// tokenPath is where the session token lives: the user's config directory.
func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "token.json"), nil
}

// loginWindow is the Pixiv sign-in window: the login page in a web view, and
// what the user falls back to when the page cannot finish.
type loginWindow struct {
	app *App
	win *mygo.Window
	web *mygo.WebView

	// verifier and challenge are the PKCE pair the window opened with;
	// they are set before any goroutine starts and never change.
	verifier  string
	challenge string

	// mu guards the state goroutines change: the exchange goroutine writes
	// the error and clears busy, the view reads them.
	mu     sync.Mutex
	busy   bool
	errMsg string

	// refresh is the pasted refresh token, touched only by the view, which
	// runs on the main thread.
	refresh string
}

// ShowLogin opens the Pixiv sign-in window. It must run on the main thread.
func (a *App) ShowLogin() {
	verifier, challenge, err := pixiv.GeneratePKCE()
	if err != nil {
		// No PKCE pair, no login: crypto/rand failing leaves nothing to retry.
		log.Printf("pixiv login: %v", err)
		return
	}
	l := &loginWindow{app: a, verifier: verifier, challenge: challenge}
	l.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     a.t("login.title"),
		Width:     520,
		Height:    760,
		MinWidth:  420,
		MinHeight: 560,
		Content:   ui.View(l.view),
	})
	l.web, err = l.win.NewWebView(mygo.WebViewOptions{URL: pixiv.LoginURL(challenge)})
	if err != nil {
		// The window keeps the fallback: retry, or a pasted refresh token.
		l.fail(fmt.Errorf("%s: %w", a.t("login.pageFailed"), err))
		return
	}
	l.web.Page().OnWillNavigate(l.navigated)
	l.web.Page().OnDidFailLoad(func(e *mygo.LoadError) {
		l.fail(errors.New(e.Description))
	})
	l.win.Invalidate()
}

// view builds the sign-in window: the login page, and the fallback of a
// failed sign-in under it.
func (l *loginWindow) view(c *ui.Context) {
	t := c.Theme()
	l.mu.Lock()
	busy, errMsg := l.busy, l.errMsg
	l.mu.Unlock()

	ui.Column(c).Fill().Children(func() {
		if l.web != nil {
			ui.WebView(c, l.web).Grow(1)
		}
		if !busy && errMsg == "" {
			return
		}
		ui.Column(c).Padding(12).Gap(8).Children(func() {
			if busy {
				ui.Row(c).Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, l.app.t("login.busy"))
				})
			}
			if errMsg != "" {
				ui.Text(c, errMsg).TextColor(t.Danger)
			}
			// A retry or a paste while a token is in flight would race it.
			if !busy {
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, l.app.t("common.retry")).Clicked() {
						l.retry()
					}
					if ui.Button(c, l.app.t("login.refreshAction")).Clicked() {
						l.submitRefresh()
					}
				})
				ui.Text(c, l.app.t("login.refreshHint")).TextColor(t.TextMuted)
				ui.TextInput(c, &l.refresh)
			}
		})
	})
}

// navigated intercepts the redirect the login page makes when it succeeds.
func (l *loginWindow) navigated(e *mygo.NavigateEvent) {
	if !strings.HasPrefix(e.URL, "pixiv://") {
		return
	}
	e.PreventDefault()
	code, err := pixiv.ParseCallback(e.URL)
	if err != nil {
		l.fail(err)
		return
	}
	l.exchange(code)
}

// retry reloads the login page. The verifier stays the same, so a code from
// the reloaded page still exchanges.
func (l *loginWindow) retry() {
	if l.web == nil {
		// No page to reload: keep the error and the pasted-token fallback.
		return
	}
	l.mu.Lock()
	if l.busy {
		// An exchange is in flight: reloading would race its SaveToken.
		l.mu.Unlock()
		return
	}
	l.busy, l.errMsg = false, ""
	l.mu.Unlock()
	l.web.Page().LoadURL(pixiv.LoginURL(l.challenge))
	l.win.Invalidate()
}

// exchange turns the authorization code into a token, off the main thread.
func (l *loginWindow) exchange(code string) {
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return
	}
	l.busy, l.errMsg = true, ""
	verifier := l.verifier
	l.mu.Unlock()
	l.win.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err := pixiv.ExchangeCode(ctx, loginHTTP, code, verifier)
		if err != nil {
			l.fail(err)
			return
		}
		l.finish(tok)
	}()
}

// submitRefresh signs in with a refresh token the user pasted, for when the
// login page cannot complete.
func (l *loginWindow) submitRefresh() {
	refresh := strings.TrimSpace(l.refresh)
	if refresh == "" {
		l.fail(errors.New(l.app.t("login.refreshEmpty")))
		return
	}
	l.mu.Lock()
	if l.busy {
		l.mu.Unlock()
		return
	}
	l.busy, l.errMsg = true, ""
	l.mu.Unlock()
	l.win.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err := pixiv.RefreshToken(ctx, loginHTTP, refresh)
		if err != nil {
			l.fail(err)
			return
		}
		l.finish(tok)
	}()
}

// finish saves the token, closes the window and tells the app.
func (l *loginWindow) finish(t *pixiv.Token) {
	path, err := tokenPath()
	if err == nil {
		err = pixiv.SaveToken(path, t)
	}
	if err != nil {
		l.fail(fmt.Errorf("%s: %w", l.app.t("login.saveFailed"), err))
		return
	}
	l.win.Close()
	l.app.onLoggedIn()
}

// fail shows the error in the window and lets the user try again.
func (l *loginWindow) fail(err error) {
	l.mu.Lock()
	l.busy, l.errMsg = false, err.Error()
	l.mu.Unlock()
	l.win.Invalidate()
}

// onLoggedIn runs once a token is saved. The token file is the session; the
// shell redraws so it can show the signed-in user.
func (a *App) onLoggedIn() {
	a.syncSession()
}
