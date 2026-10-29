package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/imagex"
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
	return a, ui.NewTester(a.View, 1000, 1200)
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

// TestCacheSizeText checks a cache size reads in the unit a person expects.
func TestCacheSizeText(t *testing.T) {
	for _, tt := range []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{4 << 10, "4 KB"},
		{3 << 20, "3.0 MB"},
		{2 << 30, "2.0 GB"},
	} {
		if got := cacheSizeText(tt.bytes); got != tt.want {
			t.Errorf("cacheSizeText(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

// TestSettingsPageClearsImageCache checks the cache card shows the cached
// size and its button deletes the cached images, leaving other files alone.
func TestSettingsPageClearsImageCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	dir := t.TempDir()
	a.images = imagex.New(nil, dir)
	cached := filepath.Join(dir, "imagex-0123456789abcdef")
	if err := os.WriteFile(cached, bytes.Repeat([]byte("x"), 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1200)
	tt.Frame()
	if !tt.HasText("4 KB") {
		t.Fatalf("cache size missing: %q", tt.Texts())
	}

	if err := tt.Click("清理缓存"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if _, err := os.Stat(cached); !os.IsNotExist(err) {
		t.Errorf("cache file: err %v, want it deleted", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("other file: %v, want it left alone", err)
	}
	tt.Frame()
	if !tt.HasText("0 B") {
		t.Errorf("cache size not refreshed: %q", tt.Texts())
	}
	if !tt.HasText("缓存已清理") {
		t.Errorf("clear not reported: %q", tt.Texts())
	}
}

// TestSettingsPageBlockedTags checks the tags card counts and lists blocked
// tags, and its buttons unblock one or clear them all.
func TestSettingsPageBlockedTags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.blockedTags.Add(BlockedTag{Name: "オリジナル"})
	a.blockedTags.Add(BlockedTag{Name: "風景"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	if !tt.HasText(a.tf("block.tagCount", 2)) {
		t.Fatalf("count missing: %q", tt.Texts())
	}
	if !tt.HasText("風景") {
		t.Fatalf("blocked tag not listed: %q", tt.Texts())
	}
	if err := tt.Click(a.t("block.tagRemove")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if a.isTagBlocked("風景") {
		t.Error("tag still blocked after the remove button")
	}
	tt.Frame()
	if err := tt.Click(a.t("block.tagClear")); err != nil {
		t.Fatalf("Click clear: %v", err)
	}
	if got := a.blockedTagsSnapshot(); len(got) != 0 {
		t.Errorf("blocked tags %+v after clearing, want none", got)
	}
	tt.Frame()
	if !tt.HasText(a.t("block.tagEmpty")) {
		t.Errorf("empty text missing: %q", tt.Texts())
	}
}

// TestSettingsPageDataCard checks the data card offers every record it can
// clear, and says a reset keeps the rest.
func TestSettingsPageDataCard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	for _, key := range []string{
		"settings.groupData",
		"settings.clearBrowseHistory",
		"settings.clearDownloadHistory",
		"settings.clearSearchHistory",
		"settings.clearNovelProgress",
		"settings.resetHint",
		"settings.resetSettings",
	} {
		if !tt.HasText(a.t(key)) {
			t.Errorf("%s missing: %q", key, tt.Texts())
		}
	}
}

// TestSettingsPageClearButtons checks each button of the data card empties
// its own record and leaves the others alone.
func TestSettingsPageClearButtons(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.history.Add(HistoryEntry{ID: 1, Title: "作品"})
	a.downloadHistory.Add(download.HistoryEntry{ID: "1_0", IllustID: 1, Title: "作品"})
	a.searchHistory.Add("猫")
	a.novelProgress.Set(7, 0.5)
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	click := func(key string) {
		t.Helper()
		if err := tt.Click(a.t(key)); err != nil {
			t.Fatalf("Click %s: %v", key, err)
		}
		tt.Frame()
	}

	click("settings.clearBrowseHistory")
	if got := a.history.Snapshot(); len(got) != 0 {
		t.Errorf("history %+v after clearing, want none", got)
	}
	if !tt.HasText(a.t("settings.historyCleared")) {
		t.Errorf("clear not reported: %q", tt.Texts())
	}
	if got := a.searchHistory.Snapshot(); len(got) != 1 {
		t.Errorf("search history %q changed by the browse clear", got)
	}
	if got := a.novelProgress.Get(7); got != 0.5 {
		t.Errorf("novel place %v changed by the browse clear", got)
	}
	if got := a.downloadHistory.Snapshot(); len(got) != 1 {
		t.Errorf("download history %+v changed by the browse clear", got)
	}

	click("settings.clearDownloadHistory")
	if got := a.downloadHistory.Snapshot(); len(got) != 0 {
		t.Errorf("download history %+v after clearing, want none", got)
	}
	if !tt.HasText(a.t("download.historyCleared")) {
		t.Errorf("download clear not reported: %q", tt.Texts())
	}
	if got := a.searchHistory.Snapshot(); len(got) != 1 {
		t.Errorf("search history %q changed by the download clear", got)
	}

	click("settings.clearSearchHistory")
	if got := a.searchHistory.Snapshot(); len(got) != 0 {
		t.Errorf("search history %q after clearing, want none", got)
	}
	if got := a.novelProgress.Get(7); got != 0.5 {
		t.Errorf("novel place %v changed by the search clear", got)
	}

	click("settings.clearNovelProgress")
	if got := a.novelProgress.Get(7); got != 0 {
		t.Errorf("novel place %v after clearing, want none", got)
	}
}

// TestSettingsPageResetsSettings checks the reset button saves the defaults
// while the account, the queue and the records stay.
func TestSettingsPageResetsSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	a := New()
	a.settings.Theme = themeDark
	a.settings.CardWidth = 300
	a.settings.ShowR18 = true
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "排队中"}})
	a.history.Add(HistoryEntry{ID: 1, Title: "作品"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	if err := tt.Click(a.t("settings.resetSettings")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	waitSettingsSaved(t, a, tt)

	if want := *Default(); *a.settings != want {
		t.Errorf("settings %+v after reset, want the defaults %+v", *a.settings, want)
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatal(err)
	}
	var saved Settings
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if want := *Default(); saved != want {
		t.Errorf("saved settings %+v, want the defaults", saved)
	}
	if a.session == nil || a.session.User.Name != "测试用户" {
		t.Errorf("account %+v lost by the reset", a.session)
	}
	if got := a.downloads.Snapshot(); len(got) != 1 {
		t.Errorf("queue %+v changed by the reset", got)
	}
	if got := a.history.Snapshot(); len(got) != 1 {
		t.Errorf("history %+v changed by the reset", got)
	}
	tt.Frame()
	if !tt.HasText(a.t("settings.resetDone")) {
		t.Errorf("reset not reported: %q", tt.Texts())
	}
}

// waitSettingsSaved frames until the save a click started has landed and
// been applied.
func waitSettingsSaved(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for a.settingsResetPending {
		if time.Now().After(deadline) {
			t.Fatal("the reset was not saved")
		}
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
}
