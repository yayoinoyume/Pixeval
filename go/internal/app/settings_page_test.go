package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeUpdater answers the settings page's update check without a feed.
type fakeUpdater struct {
	enabled bool
	update  *mygo.Update
	err     error
	checks  atomic.Int32
}

func (f *fakeUpdater) Enabled() bool { return f.enabled }

func (f *fakeUpdater) Check(context.Context) (*mygo.Update, error) {
	f.checks.Add(1)
	return f.update, f.err
}

func (f *fakeUpdater) Install(context.Context, *mygo.Update) error { return nil }

// newUpdatesApp is a settings page whose update check is f's. The tester is
// tall enough to reach the update row, which a shorter one scrolls out of
// reach.
func newUpdatesApp(t *testing.T, f *fakeUpdater) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.updater = f
	a.updateSt.off = !f.enabled
	a.route = routeSettings
	return a, ui.NewTester(a.View, 1000, 940)
}

// waitUpdate waits for the check to land as want says.
func waitUpdate(t *testing.T, a *App, what string, want func(updateState) bool) updateState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		st := a.updateSnapshot()
		if want(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("update did not %s: %+v", what, st)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestSettingsPageUpdatesOff checks a build without an update source says so
// and ignores the check button.
func TestSettingsPageUpdatesOff(t *testing.T) {
	f := &fakeUpdater{enabled: false}
	_, tt := newUpdatesApp(t, f)
	if !tt.HasText("未配置更新源") {
		t.Fatalf("notice missing: %q", tt.Texts())
	}
	if err := tt.Click("检查更新"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if n := f.checks.Load(); n != 0 {
		t.Errorf("the check ran %d time(s) without an update source", n)
	}
}

// TestSettingsPageShowsAvailableUpdate checks a check that finds a version
// shows it, with the button that installs it.
func TestSettingsPageShowsAvailableUpdate(t *testing.T) {
	f := &fakeUpdater{enabled: true, update: &mygo.Update{Version: "1.2.3"}}
	a, tt := newUpdatesApp(t, f)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitUpdate(t, a, "find the update", func(st updateState) bool { return st.version != "" })
	tt.Frame()
	for _, want := range []string{"发现新版本 1.2.3", "安装并重启"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing: %q", want, tt.Texts())
		}
	}
}

// TestSettingsPageShowsLatest checks a check that finds nothing says the app
// is up to date.
func TestSettingsPageShowsLatest(t *testing.T) {
	f := &fakeUpdater{enabled: true}
	a, tt := newUpdatesApp(t, f)
	if err := tt.Click("检查更新"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitUpdate(t, a, "finish", func(st updateState) bool { return st.latest })
	tt.Frame()
	if !tt.HasText("已是最新") {
		t.Errorf("up-to-date text missing: %q", tt.Texts())
	}
}

// TestSettingsPageLanguageChoices checks the language select offers the five
// shipped choices and that each label maps back to its tag.
func TestSettingsPageLanguageChoices(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	want := []string{"跟随系统", "中文", "English", "Français", "Русский"}
	if got := a.languageChoices(); len(got) != len(want) {
		t.Fatalf("choices %q, want %q", got, want)
	}
	for i, label := range want {
		if got := a.languageChoices()[i]; got != label {
			t.Errorf("choice %d is %q, want %q", i, got, label)
		}
	}
	for _, tag := range []string{i18n.LangAuto, i18n.LangZhHans, i18n.LangEnUS, i18n.LangFrFR, i18n.LangRuRU} {
		if got := a.languageValue(a.languageLabel(tag)); got != tag {
			t.Errorf("label of %q maps back to %q", tag, got)
		}
	}
}

// TestSettingsControlsNamed checks every settings control keeps the name its
// row gives it, so a screen reader does not meet an anonymous select or input.
func TestSettingsControlsNamed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.client.SetToken(a.session)
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 700)
	for _, label := range []string{"主题", "语言", "下载目录", "下载并发", "显示 R18", "瀑布流列宽"} {
		tabToLabel(t, tt, label)
	}
}

// tabToLabel tabs forward until the element named label has the focus.
func tabToLabel(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		if tt.Focused(label) {
			return
		}
		tt.Key(0, ui.KeyTab)
	}
	t.Fatalf("tab did not reach %q: %q", label, tt.Texts())
}
