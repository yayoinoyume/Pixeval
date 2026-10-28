package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pixeval-go", "settings.json")
	want := &Settings{
		Theme:               themeDark,
		DownloadDir:         filepath.Join(t.TempDir(), "downloads"),
		DownloadConcurrency: 5,
		ShowR18:             true,
		CardWidth:           300,
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
	if s.DownloadConcurrency != 3 {
		t.Errorf("concurrency %d, want 3", s.DownloadConcurrency)
	}
	if s.ShowR18 {
		t.Error("ShowR18 on by default")
	}
	if s.CardWidth != 240 {
		t.Errorf("card width %v, want 240", s.CardWidth)
	}
	if !strings.HasSuffix(s.DownloadDir, filepath.Join("Pictures", "Pixeval Go")) {
		t.Errorf("download dir %q", s.DownloadDir)
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
	if s.DownloadConcurrency != 3 || s.CardWidth != 240 {
		t.Errorf("missing fields not defaulted: %+v", *s)
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

// TestSettingsPageShowsFields checks the page draws all five preferences.
func TestSettingsPageShowsFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	a.route = routeSettings
	tt := ui.NewTester(a.View, 1000, 700)
	for _, label := range []string{"主题", "下载目录", "下载并发", "显示 R18", "瀑布流列宽"} {
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
