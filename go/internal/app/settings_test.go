package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "settings.json")
	want := &Settings{
		Theme:               themeDark,
		Language:            i18n.LangZhHans,
		DownloadDir:         filepath.Join(t.TempDir(), "downloads"),
		DownloadConcurrency: 5,
		DownloadMacro:       "@{artist_name}/@{id}_@{pic_set_index}.@{ext}",
		ShowR18:             true,
		CardWidth:           300,

		AutoDownloadSubscriptions:   true,
		SubscriptionIntervalMinutes: 30,

		NovelFontSize:   novelFontSizeMax,
		NovelLineHeight: 1.6,
		NovelWidth:      900,

		ProxyURL: "socks5://127.0.0.1:1080",

		SauceNAOAPIKey: "secret-key",

		ImageCacheLimitMB: 1024,
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Errorf("got %+v, want %+v", *got, *want)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode %o, want 600", perm)
	}
}

func TestDefaultSettings(t *testing.T) {
	s := Default()
	if s.Theme != themeSystem {
		t.Errorf("theme %q, want %q", s.Theme, themeSystem)
	}
	if s.Language != i18n.LangAuto {
		t.Errorf("language %q, want %q", s.Language, i18n.LangAuto)
	}
	if s.DownloadConcurrency != 3 {
		t.Errorf("concurrency %d, want 3", s.DownloadConcurrency)
	}
	if s.ShowR18 {
		t.Error("ShowR18 on by default")
	}
	if s.CardWidth != 240 {
		t.Errorf("card width %v, want 240", s.CardWidth)
	}
	if s.AutoDownloadSubscriptions {
		t.Error("auto download on by default")
	}
	if s.SubscriptionIntervalMinutes != subscriptionIntervalDefault {
		t.Errorf("interval %d, want %d", s.SubscriptionIntervalMinutes, subscriptionIntervalDefault)
	}
	if s.DownloadMacro != download.DefaultMacro {
		t.Errorf("macro %q, want the default one", s.DownloadMacro)
	}
	if !strings.HasSuffix(s.DownloadDir, filepath.Join("Pictures", "Pixeval Go")) {
		t.Errorf("download dir %q", s.DownloadDir)
	}
	if s.NovelFontSize != novelFontSizeDefault || s.NovelLineHeight != novelLineHeightDefault || s.NovelWidth != novelWidthDefault {
		t.Errorf("novel typography %d/%v/%v, want the defaults", s.NovelFontSize, s.NovelLineHeight, s.NovelWidth)
	}
	if s.ImageCacheLimitMB != imageCacheLimitDefault {
		t.Errorf("image cache limit %d, want %d", s.ImageCacheLimitMB, imageCacheLimitDefault)
	}
}

func TestLoadBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted bad JSON")
	}
}

func TestLoadFillsMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Theme != themeDark {
		t.Errorf("theme %q", s.Theme)
	}
	if s.Language != i18n.LangAuto {
		t.Errorf("language %q, want %q", s.Language, i18n.LangAuto)
	}
	if s.DownloadConcurrency != 3 || s.CardWidth != 240 {
		t.Errorf("missing fields not defaulted: %+v", *s)
	}
	if s.AutoDownloadSubscriptions {
		t.Error("missing auto download not defaulted to off")
	}
	if s.SubscriptionIntervalMinutes != subscriptionIntervalDefault {
		t.Errorf("missing interval %d, want %d", s.SubscriptionIntervalMinutes, subscriptionIntervalDefault)
	}
	if s.DownloadMacro != download.DefaultMacro {
		t.Errorf("macro %q, want the default one", s.DownloadMacro)
	}
	if s.NovelFontSize != novelFontSizeDefault || s.NovelLineHeight != novelLineHeightDefault || s.NovelWidth != novelWidthDefault {
		t.Errorf("missing novel typography not defaulted: %+v", *s)
	}
	if s.ImageCacheLimitMB != imageCacheLimitDefault {
		t.Errorf("missing image cache limit not defaulted: %d", s.ImageCacheLimitMB)
	}
}

// TestLoadNormalizesNovelTypography checks a hand edited reading preference
// outside its range falls back to the default, while the range's ends stay.
func TestLoadNormalizesNovelTypography(t *testing.T) {
	load := func(t *testing.T, data string) *Settings {
		t.Helper()
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		return s
	}
	t.Run("font size", func(t *testing.T) {
		for _, tt := range []struct{ in, want int }{
			{novelFontSizeMin - 1, novelFontSizeDefault},
			{novelFontSizeMax + 1, novelFontSizeDefault},
			{0, novelFontSizeDefault},
			{novelFontSizeMin, novelFontSizeMin},
			{novelFontSizeMax, novelFontSizeMax},
		} {
			s := load(t, `{"novelFontSize":`+strconv.Itoa(tt.in)+`}`)
			if s.NovelFontSize != tt.want {
				t.Errorf("font size %d, want %d", s.NovelFontSize, tt.want)
			}
		}
	})
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	t.Run("line height", func(t *testing.T) {
		for _, tt := range []struct{ in, want float64 }{
			{novelLineHeightMin - 0.1, novelLineHeightDefault},
			{novelLineHeightMax + 0.1, novelLineHeightDefault},
			{0, novelLineHeightDefault},
			{novelLineHeightMin, novelLineHeightMin},
			{novelLineHeightMax, novelLineHeightMax},
		} {
			s := load(t, `{"novelLineHeight":`+format(tt.in)+`}`)
			if s.NovelLineHeight != tt.want {
				t.Errorf("line height %v, want %v", s.NovelLineHeight, tt.want)
			}
		}
	})
	t.Run("width", func(t *testing.T) {
		for _, tt := range []struct{ in, want float64 }{
			{novelWidthMin - novelWidthStep, novelWidthDefault},
			{novelWidthMax + novelWidthStep, novelWidthDefault},
			{0, novelWidthDefault},
			{novelWidthMin, novelWidthMin},
			{novelWidthMax, novelWidthMax},
		} {
			s := load(t, `{"novelWidth":`+format(tt.in)+`}`)
			if s.NovelWidth != tt.want {
				t.Errorf("width %v, want %v", s.NovelWidth, tt.want)
			}
		}
	})
}

// TestLoadNormalizesSubscriptionInterval checks a hand edited interval outside
// the settings page's range falls back to the default.
func TestLoadNormalizesSubscriptionInterval(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   int
		want int
	}{
		{"zero", 0, subscriptionIntervalDefault},
		{"below", subscriptionIntervalMin - 1, subscriptionIntervalDefault},
		{"above", subscriptionIntervalMax + 1, subscriptionIntervalDefault},
		{"lowest", subscriptionIntervalMin, subscriptionIntervalMin},
		{"highest", subscriptionIntervalMax, subscriptionIntervalMax},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			data := []byte(`{"subscriptionIntervalMinutes":` + strconv.Itoa(tt.in) + `}`)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.SubscriptionIntervalMinutes != tt.want {
				t.Errorf("interval %d, want %d", s.SubscriptionIntervalMinutes, tt.want)
			}
		})
	}
}

// TestLoadRejectsInvalidMacro checks a hand edited template the parser rejects
// falls back to the default one, so downloads keep working.
func TestLoadRejectsInvalidMacro(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"downloadMacro":"@{nope}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.DownloadMacro != download.DefaultMacro {
		t.Errorf("macro %q, want the default one", s.DownloadMacro)
	}
}

// TestLoadKeepsCustomMacro checks a template the parser accepts is kept.
func TestLoadKeepsCustomMacro(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"downloadMacro":"@{id}.@{ext}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.DownloadMacro != "@{id}.@{ext}" {
		t.Errorf("macro %q, want the saved one", s.DownloadMacro)
	}
}

// TestLoadRejectsUnknownLanguage checks a hand edited language falls back to
// following the system.
func TestLoadRejectsUnknownLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"language":"klingon"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Language != i18n.LangAuto {
		t.Errorf("language %q, want %q", s.Language, i18n.LangAuto)
	}
}

// TestAppFallsBackToDefaults checks a broken settings file starts the app on
// the defaults instead of failing.
func TestAppFallsBackToDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New()
	if a.settings.Theme != themeSystem {
		t.Errorf("theme %q, want %q", a.settings.Theme, themeSystem)
	}
	if a.settings.DownloadConcurrency != 3 {
		t.Errorf("concurrency %d, want 3", a.settings.DownloadConcurrency)
	}
}

// TestLoadNormalizesImageCacheLimit checks a hand edited limit outside the
// settings page's range falls back to the default, and the range's ends stay.
func TestLoadNormalizesImageCacheLimit(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   int
		want int
	}{
		{"zero", 0, imageCacheLimitDefault},
		{"below", imageCacheLimitMin - 1, imageCacheLimitDefault},
		{"above", imageCacheLimitMax + 1, imageCacheLimitDefault},
		{"lowest", imageCacheLimitMin, imageCacheLimitMin},
		{"highest", imageCacheLimitMax, imageCacheLimitMax},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			data := []byte(`{"imageCacheLimitMB":` + strconv.Itoa(tt.in) + `}`)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.ImageCacheLimitMB != tt.want {
				t.Errorf("limit %d, want %d", s.ImageCacheLimitMB, tt.want)
			}
		})
	}
}

// TestLoadTrimsSauceNAOKey checks a saved key loses the space around it.
func TestLoadTrimsSauceNAOKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := `{"sauceNaoApiKey":"  secret-key  "}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.SauceNAOAPIKey != "secret-key" {
		t.Errorf("key %q, want secret-key", s.SauceNAOAPIKey)
	}
}

// TestLoadNormalizesProxy checks a saved proxy keeps an accepted URL, trims
// surrounding space and drops one the transport cannot dial.
func TestLoadNormalizesProxy(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"http://127.0.0.1:7890", "http://127.0.0.1:7890"},
		{"https://proxy.example:8080", "https://proxy.example:8080"},
		{"socks5://127.0.0.1:1080", "socks5://127.0.0.1:1080"},
		{"  http://127.0.0.1:7890  ", "http://127.0.0.1:7890"},
		{"", ""},
		{"127.0.0.1:7890", ""},
		{"ftp://127.0.0.1:21", ""},
		{"http://", ""},
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		data := `{"proxyUrl":` + strconv.Quote(tt.in) + `}`
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.ProxyURL != tt.want {
			t.Errorf("proxy %q, want %q", s.ProxyURL, tt.want)
		}
	}
}

// TestAppliesSavedProxy checks starting the app with a saved proxy points the
// shared client at it.
func TestAppliesSavedProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	s := Default()
	s.ProxyURL = "http://127.0.0.1:7890"
	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a := New()
	tr, ok := a.client.HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.Proxy(req)
	if err != nil || got == nil || got.String() != s.ProxyURL {
		t.Errorf("proxy = %v, %v, want %s", got, err, s.ProxyURL)
	}
}

// TestAppliesSavedImageCacheLimit checks starting the app with a saved limit
// gives it to the image loader.
func TestAppliesSavedImageCacheLimit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	s := Default()
	s.ImageCacheLimitMB = 1024
	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	a := New()
	if got, want := a.images.DiskLimit(), int64(1024)<<20; got != want {
		t.Errorf("disk limit %d, want %d", got, want)
	}
}

// TestSettingsPageSavesImageCacheLimit checks a save gives the new limit to
// the image loader and writes it back.
func TestSettingsPageSavesImageCacheLimit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.ImageCacheLimitMB = 256
	tt := ui.NewTester(a.View, 1000, 1200)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil && got.ImageCacheLimitMB == 256 && a.images.DiskLimit() == int64(256)<<20 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("limit not applied and saved: settings %d, loader %d, err %v",
				got.ImageCacheLimitMB, a.images.DiskLimit(), err)
		}
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSettingsPageShowsFields checks the page draws all five preferences.
func TestSettingsPageShowsFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 700)
	for _, label := range []string{"主题", "语言", "下载目录", "下载并发", "命名模板", "自动下载新作品", "检查间隔（分钟）", "显示 R18", "瀑布流列宽", "网络", "代理地址", "SauceNAO API Key",
		"缓存", "图片缓存上限（MB）", "当前缓存大小", "清理缓存"} {
		if !tt.HasText(label) {
			t.Errorf("label %q missing: %q", label, tt.Texts())
		}
	}
	if !tt.HasText("保存") {
		t.Errorf("save button missing: %q", tt.Texts())
	}
}

// TestCardWidthFollowsSettings checks a wider setting lays the feed out in
// wider columns.
func TestCardWidthFollowsSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := &browseState{fetch: func(context.Context) (*pixiv.Page[pixiv.Illustration], error) {
		return &pixiv.Page[pixiv.Illustration]{Items: fakeWorks(0, 20)}, nil
	}}
	a, tt := newBrowseTest(st)
	waitBrowse(t, st, "load", func(v browseView) bool { return len(v.items) == 20 })
	tt.Frame()
	narrow, ok := tt.Find("作品 1")
	if !ok {
		t.Fatalf("second card missing: %q", tt.Texts())
	}

	a.settings.CardWidth = 360
	tt.Frame()
	wide, ok := tt.Find("作品 1")
	if !ok {
		t.Fatalf("second card missing after resize: %q", tt.Texts())
	}
	if wide.X <= narrow.X+50 {
		t.Errorf("second card X = %v at width 360, want well right of %v at 240", wide.X, narrow.X)
	}
}

// TestSaveConcurrent checks saves racing on one path still leave a readable
// file.
func TestSaveConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "settings.json")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := Default()
			s.DownloadConcurrency = 1 + i%8
			if err := Save(path, s); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if _, err := Load(path); err != nil {
		t.Fatalf("Load after concurrent saves: %v", err)
	}
}

// TestSettingsPageCreatesDownloadDir checks a saved directory is created.
func TestSettingsPageCreatesDownloadDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	dir := filepath.Join(t.TempDir(), "downloads", "nested")
	a.settings.DownloadDir = dir
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(dir); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("download directory %s not created", dir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSettingsPageSavesTheme checks a save writes the edited theme, and a
// later load reads it back.
func TestSettingsPageSavesTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 700)
	a.settings.Theme = themeDark
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil {
			if got.Theme != themeDark {
				t.Fatalf("saved theme %q, want %q", got.Theme, themeDark)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("settings not saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSettingsPageRejectsInvalidMacro checks a template the parser rejects is
// reported and nothing is written.
func TestSettingsPageRejectsInvalidMacro(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.DownloadMacro = "@{nope}"
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if !tt.HasText("命名模板不合法，未保存") {
		t.Errorf("invalid macro not reported: %q", tt.Texts())
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("settings saved with an invalid macro")
	}
}

// TestSettingsPageSavesSauceNAOKey checks a save writes the trimmed key, so
// the reverse image search can use it after a restart.
func TestSettingsPageSavesSauceNAOKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.SauceNAOAPIKey = "  secret-key  "
	tt := ui.NewTester(a.View, 1000, 820)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil && got.SauceNAOAPIKey == "secret-key" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("key not saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSettingsPageSavesProxy checks a save writes the proxy, points the
// running client at it, and a later load reads it back.
func TestSettingsPageSavesProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.ProxyURL = "socks5://127.0.0.1:1080"
	tt := ui.NewTester(a.View, 1000, 820)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil && got.ProxyURL == a.settings.ProxyURL {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("settings not saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	tr, ok := a.client.HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	req, err := http.NewRequest(http.MethodGet, "https://www.pixiv.net", nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		got, err := tr.Proxy(req)
		if err == nil && got != nil && got.String() == a.settings.ProxyURL {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("client proxy = %v, %v, want %s", got, err, a.settings.ProxyURL)
		}
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSettingsPageRejectsInvalidProxy checks a proxy the transport cannot
// dial is reported and nothing is written.
func TestSettingsPageRejectsInvalidProxy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.ProxyURL = "ftp://127.0.0.1:21"
	tt := ui.NewTester(a.View, 1000, 820)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if !tt.HasText("代理地址无效，未保存") {
		t.Errorf("invalid proxy not reported: %q", tt.Texts())
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("settings saved with an invalid proxy")
	}
}

// TestSettingsPageSavesMacro checks a save writes the edited template, and a
// later load reads it back.
func TestSettingsPageSavesMacro(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	a.settings.DownloadMacro = "@{artist_name}/@{id}.@{ext}"
	tt := ui.NewTester(a.View, 1000, 700)
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := Load(path)
		if err == nil {
			if got.DownloadMacro != "@{artist_name}/@{id}.@{ext}" {
				t.Fatalf("saved macro %q", got.DownloadMacro)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("settings not saved: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
