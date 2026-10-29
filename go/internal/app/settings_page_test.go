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
	return a, ui.NewTester(a.View, 1000, 1500)
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

// TestSettingsPageNetworkAdvanced checks the network card keeps the proxy and
// fronting switch in reach and reveals the resolver and mirror fields from the
// advanced button.
func TestSettingsPageNetworkAdvanced(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 1800)
	if tt.HasText(a.t("settings.appAPIResolver")) {
		t.Fatal("advanced fields shown before opening")
	}
	if !tt.HasText(a.t("settings.proxy")) || !tt.HasText(a.t("settings.domainFronting")) {
		t.Fatalf("network controls missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("settings.networkAdvanced")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	for _, want := range []string{
		a.t("settings.frontingType"),
		a.t("settings.appAPIResolver"),
		a.t("settings.imageResolver"),
		a.t("settings.oauthResolver"),
		a.t("settings.imageMirror"),
	} {
		if !tt.HasText(want) {
			t.Errorf("%q missing after opening: %q", want, tt.Texts())
		}
	}
}

// TestSettingsPageGroupTitles checks every card is titled, and that a card's
// title reads as a heading for its rows: smaller than their labels, above
// them, and inside the window.
func TestSettingsPageGroupTitles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	// Tall enough to lay out every card at once.
	tt := ui.NewTester(a.View, 1000, 2600)
	tt.Frame()
	const width = 1000
	for _, key := range []string{
		"settings.groupAppearance",
		"settings.groupDownloads",
		"settings.groupNetwork",
		"settings.groupCache",
		"settings.groupContent",
		"settings.groupAbout",
		"settings.groupSubscriptions",
		"block.user",
		"block.tag",
		"settings.groupMCP",
		"settings.groupData",
	} {
		r, ok := tt.Find(a.t(key))
		if !ok || r.W == 0 || r.H == 0 {
			t.Errorf("%s is not titled: %q", key, tt.Texts())
			continue
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%s at %v overflows the %d wide window", key, r, width)
		}
	}
	// A title names its rows without competing with them: its text is
	// smaller than a row label, not the same 15 as the rows below.
	tok := tokensFor(false)
	title, ok := tt.Find(a.t("settings.groupAppearance"))
	label, labeled := tt.Find(a.t("settings.theme"))
	if !ok || !labeled {
		t.Fatalf("title or row label missing: %q", tt.Texts())
	}
	if got, want := title.H-(tok.SpaceL+tok.SpaceS), label.H; got >= want {
		t.Errorf("title text is %.0f high, want less than the row label's %.0f", got, want)
	}
	if title.Y >= label.Y {
		t.Errorf("title at y=%v, want it above its first row at y=%v", title.Y, label.Y)
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
	tt := ui.NewTester(a.View, 1000, 2100)
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

// dataClearRows pairs each record the data card clears with the label of its row.
var dataClearRows = []struct {
	label   string
	button  string
	confirm string
	action  string
}{
	{"settings.browseHistory", "settings.clearBrowseHistory", "settings.confirmBrowseHistory", "settings.confirmClearBrowseHistory"},
	{"settings.downloadHistory", "settings.clearDownloadHistory", "settings.confirmDownloadHistory", "settings.confirmClearDownloadHistory"},
	{"settings.searchHistory", "settings.clearSearchHistory", "settings.confirmSearchHistory", "settings.confirmClearSearchHistory"},
	{"settings.novelReading", "settings.clearNovelProgress", "settings.confirmNovelProgress", "settings.confirmClearNovelProgress"},
}

// TestSettingsPageDataCard checks the data card offers every record it can
// clear, names each row, and says a reset keeps the rest.
func TestSettingsPageDataCard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	// Tall enough that the data card's rows are laid out, not only positioned.
	tt := ui.NewTester(a.View, 1000, 2600)
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
	// Every row names the record its button empties, so a clear action is
	// never an unnamed button. The buttons share one width and sit at the
	// card's end, so their labels share one center instead of each following
	// its own length.
	var actionCenter float32
	for _, pair := range dataClearRows {
		label, ok := tt.Find(a.t(pair.label))
		if !ok || label.W == 0 {
			t.Errorf("%s label is not laid out: %q", pair.label, tt.Texts())
			continue
		}
		button, ok := tt.Find(a.t(pair.button))
		if !ok || button.W == 0 {
			t.Errorf("%s button is not laid out: %q", pair.button, tt.Texts())
			continue
		}
		if label.X >= button.X || label.Y+label.H <= button.Y || label.Y >= button.Y+button.H {
			t.Errorf("%s at %v is not left of %s at %v on the same row",
				pair.label, label, pair.button, button)
		}
		center := button.X + button.W/2
		if actionCenter == 0 {
			actionCenter = center
		} else if abs32(center-actionCenter) > 0.5 {
			t.Errorf("%s button center is %v, want the row's %v", pair.button, center, actionCenter)
		}
	}
	reset, ok := tt.Find(a.t("settings.reset"))
	if !ok || reset.W == 0 {
		t.Errorf("the reset button is not laid out: %q", tt.Texts())
	} else if center := reset.X + reset.W/2; abs32(center-actionCenter) > 0.5 {
		t.Errorf("reset button center is %v, want the clear buttons' %v", center, actionCenter)
	}
}

// abs32 is the distance between two coordinates.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
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
	tt := ui.NewTester(a.View, 1000, 2500)
	click := func(key, action string) {
		t.Helper()
		if err := tt.Click(a.t(key)); err != nil {
			t.Fatalf("Click %s: %v", key, err)
		}
		tt.Frame()
		if err := tt.Click(a.t(action)); err != nil {
			t.Fatalf("Confirm %s: %v", key, err)
		}
		tt.Frame()
	}

	click("settings.clearBrowseHistory", "settings.confirmClearBrowseHistory")
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

	click("settings.clearDownloadHistory", "settings.confirmClearDownloadHistory")
	if got := a.downloadHistory.Snapshot(); len(got) != 0 {
		t.Errorf("download history %+v after clearing, want none", got)
	}
	if !tt.HasText(a.t("download.historyCleared")) {
		t.Errorf("download clear not reported: %q", tt.Texts())
	}
	if got := a.searchHistory.Snapshot(); len(got) != 1 {
		t.Errorf("search history %q changed by the download clear", got)
	}

	click("settings.clearSearchHistory", "settings.confirmClearSearchHistory")
	if got := a.searchHistory.Snapshot(); len(got) != 0 {
		t.Errorf("search history %q after clearing, want none", got)
	}
	if got := a.novelProgress.Get(7); got != 0.5 {
		t.Errorf("novel place %v changed by the search clear", got)
	}

	click("settings.clearNovelProgress", "settings.confirmClearNovelProgress")
	if got := a.novelProgress.Get(7); got != 0 {
		t.Errorf("novel place %v after clearing, want none", got)
	}
}

// TestSettingsPageClearCancel checks each clear asks first, and Cancel leaves
// every record alone.
func TestSettingsPageClearCancel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.history.Add(HistoryEntry{ID: 1, Title: "作品"})
	a.downloadHistory.Add(download.HistoryEntry{ID: "1_0", IllustID: 1, Title: "作品"})
	a.searchHistory.Add("猫")
	a.novelProgress.Set(7, 0.5)
	a.route = routeSettings
	settingsConfirmPending = settingsConfirmNone
	tt := ui.NewTester(a.View, 1000, 2600)
	tt.Frame()
	unchanged := func() bool {
		return len(a.history.Snapshot()) == 1 &&
			len(a.downloadHistory.Snapshot()) == 1 &&
			len(a.searchHistory.Snapshot()) == 1 &&
			a.novelProgress.Get(7) == 0.5
	}
	for _, pair := range dataClearRows {
		if err := tt.Click(a.t(pair.button)); err != nil {
			t.Fatalf("Click %s: %v", pair.button, err)
		}
		tt.Frame()
		if !tt.HasText(a.t(pair.confirm)) {
			t.Errorf("%s did not ask for confirmation", pair.button)
		}
		// The confirm button names the action; a bare Confirm could belong to
		// anything.
		if !tt.HasText(a.t(pair.action)) {
			t.Errorf("%s confirm button does not name the action", pair.button)
		}
		if tt.HasText(a.t("common.confirm")) {
			t.Errorf("%s still offers a generic Confirm", pair.button)
		}
		if !unchanged() {
			t.Fatalf("%s ran before confirmation", pair.button)
		}
		if err := tt.Click(a.t("common.cancel")); err != nil {
			t.Fatalf("Cancel %s: %v", pair.button, err)
		}
		tt.Frame()
		if tt.HasText(a.t(pair.confirm)) {
			t.Errorf("%s stayed open after Cancel", pair.button)
		}
		if !unchanged() {
			t.Fatalf("%s ran after Cancel", pair.button)
		}
	}
}

// TestSettingsPageConfirmDroppedOnLeave checks a data action's confirmation
// is dropped when the settings page is left, so returning to it by shortcut
// does not ask again. The dialog covers the window and takes the keys, so the
// pending action is set and the page left the way a page change does it.
func TestSettingsPageConfirmDroppedOnLeave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.history.Add(HistoryEntry{ID: 1, Title: "作品"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 2600)
	settingsConfirmPending = settingsConfirmBrowseHistory
	a.goRoute(routeHistory)
	tt.Frame()
	if settingsConfirmPending != settingsConfirmNone {
		t.Fatalf("pending action %v survived leaving the page", settingsConfirmPending)
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Frame()
	if a.route != routeSettings {
		t.Fatalf("route %q after the settings shortcut, want %q", a.route, routeSettings)
	}
	if tt.HasText(a.t("settings.confirmBrowseHistory")) {
		t.Errorf("the confirmation came back after leaving the page")
	}
	if got := len(a.history.Snapshot()); got != 1 {
		t.Errorf("history %d entries, want the record kept", got)
	}
}

// TestSettingsPageMCPCard checks the MCP card shows the local address, the
// token clients send and the button that replaces it.
func TestSettingsPageMCPCard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.MCPToken = "shown-token"
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 2000)
	// The token itself sits in a read-only field, whose text the headless
	// tester does not read; the label is what it shows here.
	for _, text := range []string{
		a.t("settings.groupMCP"),
		a.t("settings.mcpAddress"),
		mcpAddr,
		a.t("settings.mcpToken"),
		a.t("settings.mcpCopy"),
		a.t("settings.mcpRegenerate"),
	} {
		if !tt.HasText(text) {
			t.Errorf("%q missing: %q", text, tt.Texts())
		}
	}
}

// TestSettingsPageCopiesMCPToken checks the copy button puts exactly the saved
// token on the clipboard.
func TestSettingsPageCopiesMCPToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.MCPToken = "copy-me"
	var copied string
	a.clipboardWriteText = func(text string) error {
		copied = text
		return nil
	}
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 2400)
	if err := tt.Click(a.t("settings.mcpCopy")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if copied != "copy-me" {
		t.Errorf("clipboard got %q, want the token", copied)
	}
	if !tt.HasText(a.t("settings.mcpCopied")) {
		t.Errorf("copy not reported: %q", tt.Texts())
	}
}

// TestSettingsPageRegeneratesMCPToken checks the button replaces the token,
// saves it and says a restart applies it.
func TestSettingsPageRegeneratesMCPToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.settings.MCPToken = "old-token"
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 2400)
	if err := tt.Click(a.t("settings.mcpRegenerate")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	token := a.settings.MCPToken
	if token == "" || token == "old-token" {
		t.Fatalf("token %q after regenerating, want a new one", token)
	}
	if !tt.HasText(a.t("settings.mcpRegenerated")) {
		t.Errorf("regeneration not reported: %q", tt.Texts())
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil && got.MCPToken == token {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("token not saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
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
	a.settings.MCPToken = "token-before-reset"
	a.session = &pixiv.Token{User: pixiv.TokenUser{Name: "测试用户"}}
	a.downloads.Add([]download.Task{{ID: "1_0", IllustID: 1, Title: "排队中"}})
	a.history.Add(HistoryEntry{ID: 1, Title: "作品"})
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 2600)
	if err := tt.Click(a.t("settings.reset")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if a.settings.Theme != themeDark {
		t.Fatal("reset ran before confirmation")
	}
	if !tt.HasText(a.t("settings.confirmResetAction")) {
		t.Fatalf("the confirm button does not name the reset: %q", tt.Texts())
	}
	if err := tt.Click(a.t("common.cancel")); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	tt.Frame()
	if a.settings.Theme != themeDark {
		t.Fatal("reset ran after Cancel")
	}
	if err := tt.Click(a.t("settings.reset")); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if err := tt.Click(a.t("settings.confirmResetAction")); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	waitSettingsSaved(t, a, tt)

	want := *Default()
	// The MCP token is a credential, so a reset keeps it.
	want.MCPToken = "token-before-reset"
	if *a.settings != want {
		t.Errorf("settings %+v after reset, want %+v", *a.settings, want)
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
	if saved != want {
		t.Errorf("saved settings %+v, want %+v", saved, want)
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

// TestSettingsPageFitsNarrowWindow checks the settings page in a 1024 by 720
// window: its rows stay inside the width, Save stays pinned at the bottom,
// and a scroll over the cards brings the last card into view.
func TestSettingsPageFitsNarrowWindow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	settingsConfirmPending = settingsConfirmNone
	a.route = routeSettings
	const width, height = 1024, 720
	var tok tokens
	tt := ui.NewTester(func(c *ui.Context) {
		tok = themeOf(c)
		a.View(c)
	}, width, height)
	tt.Frame()

	save, ok := tt.Find(a.t("common.save"))
	if !ok {
		t.Fatalf("save missing: %q", tt.Texts())
	}
	if save.W == 0 || save.H == 0 || save.Y < 0 || save.Y+save.H > height {
		t.Fatalf("save %v is not in the window", save)
	}
	title, ok := tt.Find(a.t("settings.groupAppearance"))
	if !ok {
		t.Fatalf("the first card is not titled: %q", tt.Texts())
	}
	theme, ok := tt.Find(a.t("settings.theme"))
	if !ok || title.W == 0 || title.X < 0 || title.X+title.W > width || title.Y >= theme.Y {
		t.Errorf("the first card title at %v is not shown above its rows", title)
	}
	for _, label := range []string{
		a.t("settings.theme"),
		a.t("settings.downloadDir"),
		a.t("settings.groupNetwork"),
		a.t("settings.networkAdvanced"),
	} {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
	}

	// The page is taller than the window: scrolling the cards, rather than
	// squeezing them, brings the last card into view, and Save stays put.
	tt.Scroll(width/2, height/2, 0, 2000)
	tt.Frame()
	data, ok := tt.Find(a.t("settings.groupData"))
	if !ok || data.W == 0 {
		t.Fatalf("the data card did not come into view: %q", tt.Texts())
	}
	if data.Y < 0 || data.Y+data.H > height || data.X+data.W > width {
		t.Errorf("the data card at %v is outside the window after scrolling", data)
	}
	// The rows' labels widen the card, so each must still fit the window.
	for _, pair := range dataClearRows {
		r, ok := tt.Find(a.t(pair.label))
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", pair.label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width || r.Y < 0 || r.Y+r.H > height {
			t.Errorf("the data card's %q at %v is outside the %d by %d window",
				pair.label, r, width, height)
		}
	}
	// The reset row names what it does and keeps the same button width, so it
	// reads as part of the card rather than a stray action.
	for _, key := range []string{"settings.resetSettings", "settings.resetHint", "settings.reset"} {
		r, ok := tt.Find(a.t(key))
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", key, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width || r.Y < 0 || r.Y+r.H > height {
			t.Errorf("%q at %v is outside the %d by %d window", key, r, width, height)
		}
	}
	if after, ok := tt.Find(a.t("common.save")); !ok || after.Y != save.Y {
		t.Errorf("save moved from y=%v to %v while scrolling", save.Y, after.Y)
	}

	// A clear asks first in a dialog that fits the same window, and its
	// confirm button is a filled danger button naming the action.
	if err := tt.Click(a.t("settings.clearBrowseHistory")); err != nil {
		t.Fatalf("Click the browse history clear: %v", err)
	}
	tt.Frame()
	for _, key := range []string{
		"settings.confirmBrowseHistory",
		"settings.confirmClearBrowseHistory",
		"common.cancel",
	} {
		r, ok := tt.Find(a.t(key))
		if !ok || r.W == 0 || r.H == 0 || r.X < 0 || r.Y < 0 || r.X+r.W > width || r.Y+r.H > height {
			t.Errorf("%s at %v is not in the %d by %d window", key, r, width, height)
		}
	}
	if tt.HasText(a.t("common.confirm")) {
		t.Errorf("the dialog still offers a generic Confirm: %q", tt.Texts())
	}
	// Cancel comes first, so it is where the keyboard starts and where a
	// stray Enter lands.
	if !tt.Focused(a.t("common.cancel")) {
		t.Errorf("Cancel does not hold the focus when the dialog opens")
	}
	confirm, ok := tt.Find(a.t("settings.confirmClearBrowseHistory"))
	if !ok || confirm.W == 0 {
		t.Fatalf("the confirm button is not laid out: %q", tt.Texts())
	}
	// Sample inside the button's left padding, away from its label.
	img := tt.Image()
	px, py := int(confirm.X)+8, int(confirm.Y+confirm.H/2)
	r8, g8, b8, _ := img.At(px, py).RGBA()
	got := ui.RGB(uint8(r8>>8), uint8(g8>>8), uint8(b8>>8))
	if got.R != tok.Danger.R || got.G != tok.Danger.G || got.B != tok.Danger.B {
		t.Errorf("confirm button background %v, want the danger fill %v", got, tok.Danger)
	}
	if err := tt.Click(a.t("common.cancel")); err != nil {
		t.Fatalf("Cancel the confirmation: %v", err)
	}
}
