package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// stateFile is the on-disk enabled set.
type stateFile struct {
	Enabled []string `json:"enabled"`
}

// loadEnabled reads the enabled ids. A missing or broken file means no
// extension is enabled, so a bad edit cannot stop the app from starting.
func loadEnabled(path string) map[string]bool {
	enabled := map[string]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return enabled
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return enabled
	}
	for _, id := range state.Enabled {
		enabled[id] = true
	}
	return enabled
}

// saveEnabled writes the enabled ids as JSON only the user can read, replacing
// the file in one step so a crash cannot leave a half written one.
func saveEnabled(path string, enabled map[string]bool) error {
	ids := make([]string, 0, len(enabled))
	for id, on := range enabled {
		if on {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	data, err := json.MarshalIndent(stateFile{Enabled: ids}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".extensions-*.tmp")
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
	return os.Rename(tmp, path)
}
