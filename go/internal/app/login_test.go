package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestLoginViewStates checks the sign-in window shows the fallback only when
// the page cannot finish: a test window has no web view.
func TestLoginViewStates(t *testing.T) {
	const (
		retry  = "重试"
		paste  = "用 refresh token 登录"
		busy   = "正在登录…"
		errMsg = "登录失败"
	)
	cases := []struct {
		name    string
		win     *loginWindow
		want    []string
		notWant []string
	}{
		{"idle", &loginWindow{app: New()}, nil, []string{retry, paste, busy}},
		{"busy", &loginWindow{app: New(), busy: true}, []string{busy}, []string{retry, paste}},
		// Without a page a retry can only fail again, so only the paste is left.
		{"error", &loginWindow{app: New(), errMsg: errMsg}, []string{errMsg, paste}, []string{retry, busy}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tt := ui.NewTester(c.win.view, 480, 640)
			for _, s := range c.want {
				if !tt.HasText(s) {
					t.Errorf("missing %q in %q", s, tt.Texts())
				}
			}
			for _, s := range c.notWant {
				if tt.HasText(s) {
					t.Errorf("unexpected %q in %q", s, tt.Texts())
				}
			}
		})
	}
}

// TestLoginNoWebViewFallback checks the state ShowLogin leaves when the app
// started without a webview: the no-webview message, the pasted-token entry,
// and no retry that could only fail again.
func TestLoginNoWebViewFallback(t *testing.T) {
	a := New()
	a.webviewOK = false
	l := &loginWindow{app: a, errMsg: a.t("login.noWebView")}
	tt := ui.NewTester(l.view, 480, 640)
	if !tt.HasText(a.t("login.noWebView")) || !tt.HasText("用 refresh token 登录") {
		t.Errorf("token fallback missing: %q", tt.Texts())
	}
	if tt.HasText("重试") {
		t.Errorf("retry shown without a web view: %q", tt.Texts())
	}
}

// TestLoginPromptWithoutWebView checks the signed-out prompt says the token
// path works when the system has no webview, and stays quiet when it does.
func TestLoginPromptWithoutWebView(t *testing.T) {
	a := New()
	a.webviewOK = false
	tt := ui.NewTester(a.View, 1000, 700)
	if !tt.HasText(a.t("login.noWebView")) {
		t.Errorf("no-webview hint missing: %q", tt.Texts())
	}
	if _, ok := tt.Find("登录 Pixiv"); !ok {
		t.Errorf("login button missing: %q", tt.Texts())
	}
	a.webviewOK = true
	tt = ui.NewTester(a.View, 1000, 700)
	if tt.HasText(a.t("login.noWebView")) {
		t.Errorf("no-webview hint shown with a web view: %q", tt.Texts())
	}
	if _, ok := tt.Find("登录 Pixiv"); !ok {
		t.Errorf("login button missing: %q", tt.Texts())
	}
}

// TestLoginRetryWithoutWebView checks retry keeps the fallback when there is
// no page to reload.
func TestLoginRetryWithoutWebView(t *testing.T) {
	l := &loginWindow{app: New(), errMsg: "登录失败"}
	l.retry()
	if l.errMsg != "登录失败" {
		t.Errorf("errMsg %q, want the error kept", l.errMsg)
	}
}
