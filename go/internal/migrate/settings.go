// Package migrate reads the settings of the old C# Pixeval, which kept them
// in settings.yaml. It imports what the Go app can use and nothing else.
package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Imported are the old settings worth keeping, in the new app's terms. A field
// the old file leaves out, or holds a value the new app has no use for, stays
// empty so the caller keeps its default.
type Imported struct {
	Theme               string
	Language            string
	DownloadDir         string
	DownloadConcurrency int
}

// legacySettings mirrors the parts of the old settings.yaml this app reads.
// The old file nests its preferences in groups; the rest is ignored.
type legacySettings struct {
	ApplicationSettings struct {
		Theme       string `yaml:"Theme"`
		CultureName string `yaml:"CultureName"`
	} `yaml:"ApplicationSettings"`
	DownloadSettings struct {
		DownloadPath string `yaml:"DownloadPath"`
		MaxTasks     int    `yaml:"MaxDownloadTaskConcurrencyLevel"`
	} `yaml:"DownloadSettings"`
}

// FindLegacy returns the path of the old Pixeval's settings.yaml when this
// machine has one.
func FindLegacy() (string, bool) {
	dir, err := legacyDataDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(dir, "Pixeval", "Settings", "settings.yaml")
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// legacyDataDir is where the old app kept its per-user data on this platform.
func legacyDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return dir, nil
		}
		return "", errors.New("LOCALAPPDATA is not set")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return dir, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}

// ImportSettings reads the old settings at path. A file that is not YAML is an
// error; a field it leaves out is left empty.
func ImportSettings(path string) (*Imported, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw legacySettings
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("settings.yaml: %w", err)
	}
	return &Imported{
		Theme:               legacyTheme(raw.ApplicationSettings.Theme),
		Language:            legacyLanguage(raw.ApplicationSettings.CultureName),
		DownloadDir:         strings.TrimSpace(raw.DownloadSettings.DownloadPath),
		DownloadConcurrency: legacyConcurrency(raw.DownloadSettings.MaxTasks),
	}, nil
}

// legacyTheme maps the old theme name to the one the new settings save, or ""
// for a name the new app does not know.
func legacyTheme(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "default":
		return "system"
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	return ""
}

// legacyLanguage maps the old culture name to one of the languages the new app
// ships, or "" for any other culture. Older files said "zh" or "en" rather
// than a full tag, so the language part decides.
func legacyLanguage(name string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "zh"):
		return "zh-Hans"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "en"):
		return "en-US"
	}
	return ""
}

// legacyConcurrency clamps the old task count to the range the new settings
// page accepts. A file without the field reads as 0, which stays 0 so the
// caller keeps its default.
func legacyConcurrency(n int) int {
	switch {
	case n < 1:
		return 0
	case n > 8:
		return 8
	}
	return n
}
