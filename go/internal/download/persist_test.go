package download

import (
	"os"
	"path/filepath"
	"testing"
)

// TestManagerSaveLoadUnfinished checks a queued task survives a restart with
// its fields, a queued state and a private file.
func TestManagerSaveLoadUnfinished(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	m := newManager(1)
	want := Task{
		ID:         "1_0",
		IllustID:   1,
		Title:      "作品",
		Artist:     "画师",
		URL:        "https://example.invalid/1.jpg",
		Dest:       filepath.Join(t.TempDir(), "1.jpg"),
		Page:       2,
		Overwrite:  true,
		Source:     SourceSubscription,
		SourceID:   42,
		SourceName: "画师",
	}
	if got := m.Add([]Task{want}); got != 1 {
		t.Fatalf("Add took %d tasks, want 1", got)
	}
	if err := m.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %o, want 600", perm)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded %d tasks, want 1: %+v", len(got), got)
	}
	if got[0] != want {
		t.Fatalf("loaded task %+v, want %+v", got[0], want)
	}
}

// TestLoadQueueWithoutSourceIsOrdinary checks a queue written before tasks
// carried a source loads as ordinary downloads.
func TestLoadQueueWithoutSourceIsOrdinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	if err := os.WriteFile(path, []byte(`{"tasks":[{"id":"old","illustId":3,"title":"旧任务"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("loaded %d tasks, want 1: %+v", len(tasks), tasks)
	}
	if tasks[0].Source != SourceOrdinary {
		t.Errorf("source %q for an old task, want the ordinary source", tasks[0].Source)
	}
	if tasks[0].SourceID != 0 || tasks[0].SourceName != "" {
		t.Errorf("old task source %d/%q, want no subscription", tasks[0].SourceID, tasks[0].SourceName)
	}
}

// TestLoadSubscriptionWithoutSourceName checks a subscription task saved
// before names were stored still loads as a subscription with no name, so the
// page can put it in the unknown group.
func TestLoadSubscriptionWithoutSourceName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	if err := os.WriteFile(path, []byte(`{"tasks":[{"id":"old","source":"subscription"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("loaded %d tasks, want 1: %+v", len(tasks), tasks)
	}
	if tasks[0].Source != SourceSubscription || tasks[0].SourceID != 0 || tasks[0].SourceName != "" {
		t.Errorf("task source %q/%d/%q, want a nameless subscription", tasks[0].Source, tasks[0].SourceID, tasks[0].SourceName)
	}
}

// TestSaveKeepsOnlyUnfinished checks done, failed and canceled tasks are not
// written, and a running one comes back queued.
func TestSaveKeepsOnlyUnfinished(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	if err := Save(path, []Task{
		{ID: "queued", State: Queued},
		{ID: "running", State: Running},
		{ID: "done", State: Done},
		{ID: "failed", State: Failed},
		{ID: "canceled", State: Canceled},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 || got[0].ID != "queued" || got[1].ID != "running" {
		t.Fatalf("loaded %+v, want queued and running", got)
	}
	for _, task := range got {
		if task.State != Queued {
			t.Errorf("task %q state = %d, want Queued", task.ID, task.State)
		}
	}
}

// TestLoadMissingQueueIsEmpty checks the first run starts with no tasks.
func TestLoadMissingQueueIsEmpty(t *testing.T) {
	tasks, err := Load(filepath.Join(t.TempDir(), "missing", "downloads.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("loaded %d tasks, want none", len(tasks))
	}
}

// TestLoadBrokenQueueIsEmpty checks a damaged file is reported without a
// panic, so the app can start empty.
func TestLoadBrokenQueueIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a broken file")
	}
	if len(tasks) != 0 {
		t.Fatalf("loaded %d tasks from a broken file, want none", len(tasks))
	}
}

// TestSaveSkipsProgressOnlyChange checks a changed byte count does not rewrite
// the file, since progress is not saved.
func TestSaveSkipsProgressOnlyChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	m := newManager(1)
	m.Add([]Task{{ID: "a"}})
	if err := m.Save(path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	e := m.findLocked("a")
	m.mu.Unlock()
	m.update(e, func(task *Task) { task.Bytes, task.Total, task.Attempts = 10, 20, 2 })

	if err := m.Save(path); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("Save rewrote the file for a progress-only change")
	}
}
