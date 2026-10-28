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
		{"idle", &loginWindow{}, nil, []string{retry, paste, busy}},
		{"busy", &loginWindow{busy: true}, []string{busy, retry, paste}, nil},
		{"error", &loginWindow{errMsg: errMsg}, []string{errMsg, retry, paste}, []string{busy}},
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
