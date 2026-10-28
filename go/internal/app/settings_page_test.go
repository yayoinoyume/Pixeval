package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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

// newUpdatesApp is a settings page whose update check is f's.
func newUpdatesApp(t *testing.T, f *fakeUpdater) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.updater = f
	a.updateSt.off = !f.enabled
	a.route = routeSettings
	return a, ui.NewTester(a.View, 1000, 700)
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
