package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/yayoinoyume/pixeval-go/internal/i18n"
	"github.com/yayoinoyume/pixeval-go/internal/migrate"
)

// migratedMarker next to settings.json records that the old Pixeval's settings
// were already imported, so deleting settings.json does not import them twice.
const migratedMarker = ".migrated"

// The theme values Settings.Theme saves.
const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

// The subscription check interval the settings page offers, in minutes.
const (
	subscriptionIntervalDefault = 15
	subscriptionIntervalMin     = 5
	subscriptionIntervalMax     = 1440
)

// Settings are the user's preferences, kept in settings.json under the user's
// config directory.
type Settings struct {
	Theme               string  `json:"theme"`
	Language            string  `json:"language"`
	DownloadDir         string  `json:"downloadDir"`
	DownloadConcurrency int     `json:"downloadConcurrency"`
	ShowR18             bool    `json:"showR18"`
	CardWidth           float64 `json:"cardWidth"`
	// AutoDownloadSubscriptions queues the works a background check finds.
	AutoDownloadSubscriptions bool `json:"autoDownloadSubscriptions"`
	// SubscriptionIntervalMinutes is how often the check runs.
	SubscriptionIntervalMinutes int `json:"subscriptionIntervalMinutes"`
}

// Default is the settings of a first run.
func Default() *Settings {
	dir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, "Pictures", "Pixeval Go")
	}
	return &Settings{
		Theme:               themeSystem,
		Language:            i18n.LangAuto,
		DownloadDir:         dir,
		DownloadConcurrency: downloadConcurrency,
		ShowR18:             false,
		CardWidth:           cardWidth,

		AutoDownloadSubscriptions:   false,
		SubscriptionIntervalMinutes: subscriptionIntervalDefault,
	}
}

// settingsPath is where the preferences live: the user's config directory.
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "settings.json"), nil
}

// loadSettings reads the saved preferences, or Default when there are none or
// the file cannot be read. A first run imports the old Pixeval's settings
// instead when this machine has them.
func loadSettings() *Settings {
	path, err := settingsPath()
	if err != nil {
		log.Printf("settings: %v", err)
		return Default()
	}
	s, err := Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return importLegacy(path)
		}
		log.Printf("settings: %v", err)
		return Default()
	}
	return s
}

// importLegacy builds the first run's settings from the old Pixeval's
// settings.yaml, saves them and leaves a marker so it happens once. Anything
// missing or unreadable keeps the default.
func importLegacy(path string) *Settings {
	s := Default()
	if _, err := os.Stat(path + migratedMarker); err == nil {
		return s
	}
	legacy, ok := migrate.FindLegacy()
	if !ok {
		return s
	}
	imp, err := migrate.ImportSettings(legacy)
	if err != nil {
		log.Printf("settings: import %s: %v", legacy, err)
		return s
	}
	applyImported(s, imp)
	if err := Save(path, s); err != nil {
		log.Printf("settings: save imported: %v", err)
		return s
	}
	if err := os.WriteFile(path+migratedMarker, nil, 0o600); err != nil {
		log.Printf("settings: migration marker: %v", err)
	}
	return s
}

// applyImported copies the old settings the new app can use onto s, keeping
// the default of every one the old file did not have.
func applyImported(s *Settings, imp *migrate.Imported) {
	if imp.Theme != "" {
		s.Theme = imp.Theme
	}
	if imp.Language != "" {
		s.Language = imp.Language
	}
	if imp.DownloadDir != "" {
		s.DownloadDir = imp.DownloadDir
	}
	if imp.DownloadConcurrency >= 1 && imp.DownloadConcurrency <= 8 {
		s.DownloadConcurrency = imp.DownloadConcurrency
	}
}

// Load reads the settings at path. A field the file leaves out keeps its
// default; one holding an unusable value falls back to it too.
func Load(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := Default()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	s.normalize()
	return s, nil
}

// normalize replaces a value the settings page cannot produce, so a hand
// edited file cannot break the app.
func (s *Settings) normalize() {
	switch s.Theme {
	case themeSystem, themeLight, themeDark:
	default:
		s.Theme = themeSystem
	}
	switch s.Language {
	case i18n.LangAuto, i18n.LangZhHans, i18n.LangEnUS:
	default:
		s.Language = i18n.LangAuto
	}
	if s.DownloadConcurrency < 1 || s.DownloadConcurrency > 8 {
		s.DownloadConcurrency = downloadConcurrency
	}
	if s.CardWidth < 180 || s.CardWidth > 360 {
		s.CardWidth = cardWidth
	}
	if s.SubscriptionIntervalMinutes < subscriptionIntervalMin ||
		s.SubscriptionIntervalMinutes > subscriptionIntervalMax {
		s.SubscriptionIntervalMinutes = subscriptionIntervalDefault
	}
}

// Save writes s to path as JSON only the user can read, replacing the file in
// one step so a crash cannot leave a half written one. A temporary file of its
// own keeps two saves at once from writing the same one.
func Save(path string, s *Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

// masonryWidth is the column width the settings ask for.
func (a *App) masonryWidth() float64 {
	if a.settings != nil && a.settings.CardWidth > 0 {
		return a.settings.CardWidth
	}
	return cardWidth
}

// settingsDownloadDir is where downloads go: the saved directory, or the
// default one when it is empty.
func settingsDownloadDir(s *Settings) string {
	if s != nil && s.DownloadDir != "" {
		return s.DownloadDir
	}
	return downloadDir()
}
