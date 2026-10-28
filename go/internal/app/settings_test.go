package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
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
