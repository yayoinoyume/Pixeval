package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

// The theme values Settings.Theme saves.
const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

// Settings are the user's preferences, kept in settings.json under the user's
// config directory.
type Settings struct {
	Theme               string  `json:"theme"`
	DownloadDir         string  `json:"downloadDir"`
	DownloadConcurrency int     `json:"downloadConcurrency"`
	ShowR18             bool    `json:"showR18"`
	CardWidth           float64 `json:"cardWidth"`
}

// Default is the settings of a first run.
func Default() *Settings {
	dir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, "Pictures", "Pixeval Go")
	}
	return &Settings{
		Theme:               themeSystem,
		DownloadDir:         dir,
		DownloadConcurrency: downloadConcurrency,
		ShowR18:             false,
		CardWidth:           cardWidth,
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
// the file cannot be read.
func loadSettings() *Settings {
	path, err := settingsPath()
	if err != nil {
		log.Printf("settings: %v", err)
		return Default()
	}
	s, err := Load(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("settings: %v", err)
		}
		return Default()
	}
	return s
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
	if s.DownloadConcurrency < 1 || s.DownloadConcurrency > 8 {
		s.DownloadConcurrency = downloadConcurrency
	}
	if s.CardWidth < 180 || s.CardWidth > 360 {
		s.CardWidth = cardWidth
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
