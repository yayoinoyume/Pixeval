package migrate

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// legacyYAML is shaped like the old Pixeval's settings.yaml, which nests its
// preferences in groups.
const legacyYAML = `ApplicationSettings:
  Theme: Dark
  CultureName: zh
  AppFontFamily: Microsoft YaHei
DownloadSettings:
  DownloadPath: D:\Pixeval\Downloads
  MaxDownloadTaskConcurrencyLevel: 5
BrowsingExperienceSettings:
  IsFilterEnabled: true
NetworkSettings:
  Proxy: ""
`

func writeLegacy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportSettings(t *testing.T) {
	got, err := ImportSettings(writeLegacy(t, legacyYAML))
	if err != nil {
		t.Fatalf("ImportSettings: %v", err)
	}
	want := Imported{
		Theme:               "dark",
		Language:            "zh-Hans",
		DownloadDir:         `D:\Pixeval\Downloads`,
		DownloadConcurrency: 5,
	}
	if *got != want {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

// TestImportSettingsThemes checks every theme name the old app could save.
func TestImportSettingsThemes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"Default", "system"},
		{"Light", "light"},
		{"Dark", "dark"},
		{"Sepia", ""},
	} {
		path := writeLegacy(t, "ApplicationSettings:\n  Theme: "+tc.in+"\n")
		got, err := ImportSettings(path)
		if err != nil {
			t.Fatalf("ImportSettings(%q): %v", tc.in, err)
		}
		if got.Theme != tc.want {
			t.Errorf("theme %q: got %q, want %q", tc.in, got.Theme, tc.want)
		}
	}
}

// TestImportSettingsLanguages checks full tags and the older short ones, and
// that an unknown culture is left for the caller to default.
func TestImportSettingsLanguages(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"zh-Hans", "zh-Hans"},
		{"zh-CN", "zh-Hans"},
		{"en-US", "en-US"},
		{"ja-JP", ""},
		{"", ""},
	} {
		path := writeLegacy(t, "ApplicationSettings:\n  CultureName: \""+tc.in+"\"\n")
		got, err := ImportSettings(path)
		if err != nil {
			t.Fatalf("ImportSettings(%q): %v", tc.in, err)
		}
		if got.Language != tc.want {
			t.Errorf("culture %q: got %q, want %q", tc.in, got.Language, tc.want)
		}
	}
}

// TestImportSettingsClampsConcurrency checks the old count lands in the range
// the new settings page accepts.
func TestImportSettingsClampsConcurrency(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
	}{
		{1, 1},
		{5, 5},
		{8, 8},
		{32, 8},
		{0, 0},
	} {
		path := writeLegacy(t, "DownloadSettings:\n  MaxDownloadTaskConcurrencyLevel: "+strconv.Itoa(tc.in)+"\n")
		got, err := ImportSettings(path)
		if err != nil {
			t.Fatalf("ImportSettings(%d): %v", tc.in, err)
		}
		if got.DownloadConcurrency != tc.want {
			t.Errorf("concurrency %d: got %d, want %d", tc.in, got.DownloadConcurrency, tc.want)
		}
	}
}

// TestImportSettingsMissingFields checks a file that only holds some groups
// leaves the rest empty instead of failing.
func TestImportSettingsMissingFields(t *testing.T) {
	got, err := ImportSettings(writeLegacy(t, "ApplicationSettings:\n  Theme: Light\n"))
	if err != nil {
		t.Fatalf("ImportSettings: %v", err)
	}
	want := Imported{Theme: "light"}
	if *got != want {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

// TestImportSettingsBadYAML checks a file the old app could not have written
// is an error rather than a panic or a silent empty result.
func TestImportSettingsBadYAML(t *testing.T) {
	if _, err := ImportSettings(writeLegacy(t, "ApplicationSettings: [\n")); err == nil {
		t.Fatal("ImportSettings accepted a broken file")
	}
}

func TestImportSettingsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")
	if _, err := ImportSettings(path); err == nil {
		t.Fatal("ImportSettings accepted a missing file")
	}
}

// TestFindLegacy checks the old file is found where this platform keeps it,
// and only when it exists.
func TestFindLegacy(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("LOCALAPPDATA", base)

	if path, ok := FindLegacy(); ok {
		t.Errorf("FindLegacy found %q before it was written", path)
	}
	path := filepath.Join(base, "Pixeval", "Settings", "settings.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(legacyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := FindLegacy()
	if !ok {
		t.Fatal("FindLegacy did not find the written settings.yaml")
	}
	if got != path {
		t.Errorf("FindLegacy got %q, want %q", got, path)
	}
}
