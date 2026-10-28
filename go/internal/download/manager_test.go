package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// newManager builds a manager over a real client with tiny retry delays.
func newManager(concurrency int) *Manager {
	m := New(pixiv.NewClient(), concurrency)
	m.backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	return m
}

// waitState blocks until the task with id reaches want.
func waitState(t *testing.T, m *Manager, id string, want State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range m.Snapshot() {
			if task.ID == id && task.State == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %q never reached state %d: %+v", id, want, m.Snapshot())
}

func taskOf(t *testing.T, m *Manager, id string) Task {
	t.Helper()
	for _, task := range m.Snapshot() {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %q is missing", id)
	return Task{}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s should not exist, stat err = %v", path, err)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var active, peak atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			seen := peak.Load()
			if n <= seen || peak.CompareAndSwap(seen, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte("image"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := newManager(3)
	tasks := make([]Task, 8)
	for i := range tasks {
		tasks[i] = Task{
			ID:   fmt.Sprintf("t%d", i),
			URL:  srv.URL,
			Dest: filepath.Join(dir, fmt.Sprintf("%d.png", i)),
		}
	}
	m.Add(tasks)
	m.Start(context.Background())
	m.Wait()

	if got := peak.Load(); got > 3 {
		t.Fatalf("peak concurrent downloads = %d, want <= 3", got)
	} else if got < 2 {
		t.Fatalf("peak concurrent downloads = %d, expected the queue to run in parallel", got)
	}
	for _, task := range m.Snapshot() {
		if task.State != Done {
			t.Fatalf("task %q state = %d, want Done (err %q)", task.ID, task.State, task.Err)
		}
	}
}

func TestAtomicWrite(t *testing.T) {
	content := []byte("image-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "nested", "a.png")
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	waitState(t, m, "a", Running)

	mustNotExist(t, dest)

	m.Wait()
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("dest content = %q, want %q", got, content)
	}
	mustNotExist(t, dest+".part")
	if task := taskOf(t, m, "a"); task.Bytes != int64(len(content)) || task.Total != int64(len(content)) {
		t.Fatalf("progress = %d/%d, want %d", task.Bytes, task.Total, len(content))
	}
}

func TestRetryThenSuccess(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.png")
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	m.Wait()

	task := taskOf(t, m, "a")
	if task.State != Done {
		t.Fatalf("state = %d, want Done (err %q)", task.State, task.Err)
	}
	if task.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", task.Attempts)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "ok" {
		t.Fatalf("dest = %q, err = %v", got, err)
	}
}

func TestCancelRunning(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	dest := filepath.Join(t.TempDir(), "a.png")
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	waitState(t, m, "a", Running)

	m.Cancel("a")
	m.Wait()

	if task := taskOf(t, m, "a"); task.State != Canceled {
		t.Fatalf("state = %d, want Canceled (err %q)", task.State, task.Err)
	}
	mustNotExist(t, dest+".part")
	mustNotExist(t, dest)
}

func TestPauseAndResume(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			<-r.Context().Done() // the paused attempt never finishes
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.png")
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	waitState(t, m, "a", Running)

	m.Pause("a")
	waitState(t, m, "a", Queued)
	mustNotExist(t, dest)

	m.Resume("a")
	m.Wait()

	if task := taskOf(t, m, "a"); task.State != Done {
		t.Fatalf("state = %d, want Done (err %q)", task.State, task.Err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "ok" {
		t.Fatalf("dest = %q, err = %v", got, err)
	}
}

// TestResumeBeforePauseIsSeen checks a Resume that arrives before the paused
// worker unwinds requeues the task rather than failing it with the canceled
// attempt's error.
func TestResumeBeforePauseIsSeen(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			<-r.Context().Done() // the paused attempt never finishes
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.png")
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	waitState(t, m, "a", Running)

	m.Pause("a")
	m.Resume("a") // no wait for Queued
	m.Wait()

	if task := taskOf(t, m, "a"); task.State != Done {
		t.Fatalf("state = %d, want Done (err %q)", task.State, task.Err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "ok" {
		t.Fatalf("dest = %q, err = %v", got, err)
	}
}

func TestSkipExistingTarget(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("new"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newManager(1)
	m.Add([]Task{{ID: "a", URL: srv.URL, Dest: dest}})
	m.Start(context.Background())
	m.Wait()

	task := taskOf(t, m, "a")
	if task.State != Done || !task.Skipped {
		t.Fatalf("state = %d skipped = %v, want Done and skipped", task.State, task.Skipped)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests, want 0", n)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "old" {
		t.Fatalf("dest = %q, err = %v", got, err)
	}
}
