package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// loginHTTP is the client of the login exchange. It has its own timeout so a
// stuck request cannot hold the login window, and it follows the saved proxy
// through loginProxy, which is swapped atomically.
var (
	loginProxy atomic.Value
	loginHTTP  = newLoginHTTP()
)

func newLoginHTTP() *http.Client {
	loginProxy.Store(http.ProxyFromEnvironment)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		return loginProxy.Load().(func(*http.Request) (*url.URL, error))(req)
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: tr}
}

// setLoginProxy points the login exchange at raw, or back at the environment's
// proxy when raw is empty. A URL the transport cannot dial is rejected and the
// current proxy stays.
func setLoginProxy(raw string) error {
	proxy := http.ProxyFromEnvironment
	if raw = strings.TrimSpace(raw); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("proxy: %w", err)
		}
		switch u.Scheme {
		case "http", "https", "socks5":
		default:
			return fmt.Errorf("proxy: unsupported scheme %q", u.Scheme)
		}
		if u.Host == "" {
			return errors.New("proxy: missing host")
		}
		proxy = http.ProxyURL(u)
	}
	loginProxy.Store(proxy)
	return nil
}

// webViewProxyEnv remembers the process's proxy variables before the login
// window first changes them, so clearing the proxy restores the user's values.
var webViewProxyEnv = struct {
	sync.Mutex
	saved  bool
	values map[string]proxyEnvValue
}{values: map[string]proxyEnvValue{}}

type proxyEnvValue struct {
	value string
	set   bool
}

// WebKitGTK reads the proxy from the process environment when it creates the
// login page's network session. Both cases are set because libsoup reads the
// lower case names. A window that is already open keeps the proxy it started
// with; a save applies to the next one.
var webViewProxyVars = []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}

// applyWebViewProxy sets the login window's proxy environment, or restores the
// values captured before the first change when raw is empty.
func applyWebViewProxy(raw string) {
	raw = strings.TrimSpace(raw)
	webViewProxyEnv.Lock()
	defer webViewProxyEnv.Unlock()
	if !webViewProxyEnv.saved {
		for _, key := range webViewProxyVars {
			value, set := os.LookupEnv(key)
			webViewProxyEnv.values[key] = proxyEnvValue{value: value, set: set}
		}
		webViewProxyEnv.saved = true
	}
	for _, key := range webViewProxyVars {
		if raw != "" {
			os.Setenv(key, raw)
			continue
		}
		value := webViewProxyEnv.values[key]
		if value.set {
			os.Setenv(key, value.value)
		} else {
			os.Unsetenv(key)
		}
	}
	if raw == "" {
		webViewProxyEnv.saved = false
		webViewProxyEnv.values = map[string]proxyEnvValue{}
	}
}

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
	applyWebViewProxy(a.settings.ProxyURL)
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
	if !a.webviewOK {
		// No system webview means the page cannot load: skip a window bound
		// to fail and offer the pasted refresh token at once.
		l.fail(errors.New(a.t("login.noWebView")))
		return
	}
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
	l.app.applyTheme(c)
	t := themeOf(c)
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
		ui.Column(c).Padding(t.SpaceM).Gap(t.SpaceS).Children(func() {
			if busy {
				ui.Row(c).Gap(t.SpaceS).Children(func() {
					ui.Spinner(c)
					ui.Text(c, l.app.t("login.busy"))
				})
			}
			if errMsg != "" {
				ui.Text(c, errMsg).FontSize(t.TextSmall).TextColor(t.Danger)
			}
			// A retry or a paste while a token is in flight would race it.
			if !busy {
				ui.Row(c).Gap(t.SpaceS).Children(func() {
					// Without a page a retry can only fail again: leave the paste.
					if l.web != nil && ui.Button(c, l.app.t("common.retry")).Clicked() {
						l.retry()
					}
					if ui.Button(c, l.app.t("login.refreshAction")).Clicked() {
						l.submitRefresh()
					}
				})
				ui.Text(c, l.app.t("login.refreshHint")).FontSize(t.TextSmall).TextColor(t.TextMuted)
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
	// The message is looked up here: the bundle changes on the main thread.
	saveFailed := l.app.t("login.saveFailed")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err := pixiv.ExchangeCode(ctx, loginHTTP, code, verifier)
		if err != nil {
			l.fail(err)
			return
		}
		l.finish(tok, saveFailed)
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
	saveFailed := l.app.t("login.saveFailed")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tok, err := pixiv.RefreshToken(ctx, loginHTTP, refresh)
		if err != nil {
			l.fail(err)
			return
		}
		l.finish(tok, saveFailed)
	}()
}

// finish saves the token, closes the window and tells the app.
func (l *loginWindow) finish(t *pixiv.Token, saveFailed string) {
	path, err := tokenPath()
	if err != nil {
		l.fail(fmt.Errorf("%s: %w", saveFailed, err))
		return
	}
	if err := pixiv.SaveToken(path, t); err != nil {
		l.fail(fmt.Errorf("%s: %w", saveFailed, err))
		return
	}
	// The token file is the session: a list that cannot be written must not
	// fail the sign-in.
	if err := l.app.rememberAccount(t); err != nil {
		log.Printf("accounts: %v", err)
	}
	// A window opened here always exists; the check keeps finish usable
	// without one, as tests that check what it saves need.
	if l.win != nil {
		l.win.Close()
	}
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
