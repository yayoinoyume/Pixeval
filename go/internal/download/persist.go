package download

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// persistedTask is the part of a Task that survives a restart. Progress and
// the terminal states are not saved: an unfinished task starts over.
type persistedTask struct {
	ID         string `json:"id"`
	IllustID   int64  `json:"illustId"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	URL        string `json:"url"`
	Dest       string `json:"dest"`
	Page       int    `json:"page"`
	Overwrite  bool   `json:"overwrite"`
	Source     string `json:"source,omitempty"`
	SourceID   int64  `json:"sourceId,omitempty"`
	SourceName string `json:"sourceName,omitempty"`
}

// downloadsFile is the on-disk queue.
type downloadsFile struct {
	Tasks []persistedTask `json:"tasks"`
}

// Load reads the unfinished queue saved at path. A missing file is an empty
// queue; a broken one is reported so the caller can start empty.
func Load(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var file downloadsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(file.Tasks))
	for _, p := range file.Tasks {
		tasks = append(tasks, Task{
			ID:         p.ID,
			IllustID:   p.IllustID,
			Title:      p.Title,
			Artist:     p.Artist,
			URL:        p.URL,
			Dest:       p.Dest,
			Page:       p.Page,
			Overwrite:  p.Overwrite,
			Source:     p.Source,
			SourceID:   p.SourceID,
			SourceName: p.SourceName,
			State:      Queued,
		})
	}
	return tasks, nil
}

// Save writes the unfinished tasks to path as JSON only the user can read,
// replacing the file in one step so a crash cannot leave a half written one.
func Save(path string, tasks []Task) error {
	data, err := encodeTasks(tasks)
	if err != nil {
		return err
	}
	return writeDownloads(path, data)
}

// Save writes the manager's unfinished tasks to path, skipping the write when
// neither the queue nor their saved fields changed. Progress updates therefore
// do not touch the disk.
func (m *Manager) Save(path string) error {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	data, err := encodeTasks(m.Snapshot())
	if err != nil {
		return err
	}
	if path == m.savedPath && bytes.Equal(data, m.savedData) {
		return nil
	}
	if err := writeDownloads(path, data); err != nil {
		return err
	}
	m.savedPath, m.savedData = path, data
	return nil
}

// encodeTasks keeps only the tasks that can still run, with a running one
// stored as queued so a restart picks it up again.
func encodeTasks(tasks []Task) ([]byte, error) {
	out := make([]persistedTask, 0, len(tasks))
	for _, t := range tasks {
		if finished(t.State) {
			continue
		}
		out = append(out, persistedTask{
			ID:         t.ID,
			IllustID:   t.IllustID,
			Title:      t.Title,
			Artist:     t.Artist,
			URL:        t.URL,
			Dest:       t.Dest,
			Page:       t.Page,
			Overwrite:  t.Overwrite,
			Source:     t.Source,
			SourceID:   t.SourceID,
			SourceName: t.SourceName,
		})
	}
	return json.Marshal(downloadsFile{Tasks: out})
}

// writeDownloads writes data to path through a temporary file, so readers see
// either the old queue or the new one.
func writeDownloads(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
