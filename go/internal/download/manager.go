// Package download runs the app's download queue: a bounded number of image
// downloads at a time, atomic files, retries, and pause/cancel per task.
package download

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// partSuffix marks the in-progress file. A finished download is renamed over
// Dest, so a reader never sees a half-written image.
const partSuffix = ".part"

// defaultConcurrency is used when New gets a non-positive limit.
const defaultConcurrency = 3

// State is where a task sits in its lifecycle.
type State int

const (
	Queued State = iota
	Running
	Done
	Failed
	Canceled
)

// Task is one download. Bytes and Total stay zero until the body has arrived,
// because pixiv.Client.GetImage returns the whole image at once.
type Task struct {
	ID        string
	IllustID  int64
	Title     string
	Artist    string
	URL       string
	Dest      string
	Page      int
	State     State
	Bytes     int64
	Total     int64
	Err       string
	Attempts  int
	Overwrite bool // write even when Dest already exists
	Skipped   bool // Dest existed and Overwrite was false
}

// Manager runs the queue. All methods are safe for concurrent use.
type Manager struct {
	client  *pixiv.Client
	sem     chan struct{} // one slot per allowed concurrent download
	backoff []time.Duration

	mu       sync.Mutex
	entries  []*entry // join order, which Snapshot keeps
	started  bool
	ctx      context.Context
	onChange func()
	wg       sync.WaitGroup
}

// entry is the mutable side of a task that Snapshot does not expose.
type entry struct {
	task    Task
	cancel  context.CancelFunc // stops the attempt in flight
	paused  bool
	running bool // a worker goroutine owns this entry
}

// New builds a manager. concurrency below 1 falls back to the default.
func New(client *pixiv.Client, concurrency int) *Manager {
	if concurrency < 1 {
		concurrency = defaultConcurrency
	}
	return &Manager{
		client:  client,
		sem:     make(chan struct{}, concurrency),
		backoff: []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second},
		ctx:     context.Background(),
	}
}

// Add appends tasks as Queued and skips IDs that are already present,
// reporting how many it took.
func (m *Manager) Add(tasks []Task) int {
	var added []*entry
	m.mu.Lock()
	for _, t := range tasks {
		if m.findLocked(t.ID) != nil {
			continue
		}
		t.State = Queued
		t.Err = ""
		t.Attempts = 0
		t.Skipped = false
		e := &entry{task: t}
		m.entries = append(m.entries, e)
		added = append(added, e)
	}
	m.mu.Unlock()
	for _, e := range added {
		m.launch(e)
	}
	return len(added)
}

// Start begins downloading; later calls do nothing.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.ctx = ctx
	queued := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		if e.task.State == Queued && !e.paused {
			queued = append(queued, e)
		}
	}
	m.mu.Unlock()
	for _, e := range queued {
		m.launch(e)
	}
}

// Pause stops a queued or running task; it stays Queued until Resume.
func (m *Manager) Pause(id string) {
	m.mu.Lock()
	e := m.findLocked(id)
	if e == nil || finished(e.task.State) {
		m.mu.Unlock()
		return
	}
	e.paused = true
	cancel := e.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel() // the worker requeues the task when it sees paused
	}
}

// Resume requeues a paused task.
func (m *Manager) Resume(id string) {
	m.mu.Lock()
	e := m.findLocked(id)
	if e == nil || !e.paused {
		m.mu.Unlock()
		return
	}
	e.paused = false
	m.mu.Unlock()
	m.launch(e)
}

// Cancel abandons a task and drops its partial file.
func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	e := m.findLocked(id)
	if e == nil || finished(e.task.State) {
		m.mu.Unlock()
		return
	}
	e.paused = false
	e.task.State = Canceled
	cancel, dest := e.cancel, e.task.Dest
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	os.Remove(dest + partSuffix)
	m.changed()
}

// Remove drops a task from the snapshot, stopping it first.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	e := m.findLocked(id)
	if e == nil {
		m.mu.Unlock()
		return
	}
	for i, x := range m.entries {
		if x == e {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			break
		}
	}
	e.paused = true // keeps the worker from requeueing a dropped task
	cancel, dest := e.cancel, e.task.Dest
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	os.Remove(dest + partSuffix)
	m.changed()
}

// Snapshot copies the tasks in the order they were added.
func (m *Manager) Snapshot() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, len(m.entries))
	for i, e := range m.entries {
		out[i] = e.task
	}
	return out
}

// OnChange registers a callback fired after any state change, on the worker
// goroutine, so the caller must hop to its own thread.
func (m *Manager) OnChange(f func()) {
	m.mu.Lock()
	m.onChange = f
	m.mu.Unlock()
}

// Wait blocks until no worker is running.
func (m *Manager) Wait() { m.wg.Wait() }

func finished(s State) bool { return s == Done || s == Failed || s == Canceled }

func (m *Manager) findLocked(id string) *entry {
	for _, e := range m.entries {
		if e.task.ID == id {
			return e
		}
	}
	return nil
}

// launch starts a worker for e unless one already owns it.
func (m *Manager) launch(e *entry) {
	m.mu.Lock()
	if !m.started || e.running || e.paused || e.task.State != Queued {
		m.mu.Unlock()
		return
	}
	e.running = true
	m.wg.Add(1)
	m.mu.Unlock()
	go m.worker(e)
}

// worker owns one task from queue slot to terminal state.
func (m *Manager) worker(e *entry) {
	defer m.finish(e)

	m.mu.Lock()
	dest, overwrite, state := e.task.Dest, e.task.Overwrite, e.task.State
	m.mu.Unlock()
	if !overwrite && state == Queued {
		if _, err := os.Stat(dest); err == nil {
			m.update(e, func(t *Task) {
				if t.State == Queued {
					t.State, t.Skipped = Done, true
				}
			})
			return
		}
	}

	select {
	case m.sem <- struct{}{}:
	case <-m.ctx.Done():
		return
	}
	defer func() { <-m.sem }()

	m.mu.Lock()
	if e.paused || e.task.State != Queued {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	e.cancel = cancel
	e.task.State = Running
	m.mu.Unlock()
	m.changed()

	err := m.run(ctx, e)
	// The attempt's own cancel, read before cancel below, so a Resume that
	// undid the pause is still seen as one.
	canceled := ctx.Err() != nil
	cancel()

	m.mu.Lock()
	e.cancel = nil
	paused, state := e.paused, e.task.State
	m.mu.Unlock()

	switch {
	case state == Canceled:
		os.Remove(dest + partSuffix)
	case paused || canceled:
		// Pause interrupted the attempt, or Resume undid the pause before this
		// worker saw it: the canceled attempt must not fail the task, which
		// waits for the next launch instead.
		os.Remove(dest + partSuffix)
		m.update(e, func(t *Task) {
			if t.State != Canceled {
				t.State, t.Err = Queued, ""
			}
		})
	case err != nil:
		m.update(e, func(t *Task) {
			if t.State != Canceled {
				t.State, t.Err = Failed, err.Error()
			}
		})
	default:
		m.update(e, func(t *Task) {
			if t.State != Canceled {
				t.State = Done
			}
		})
	}
}

// finish clears the running mark and restarts the task if Resume arrived while
// the worker was still unwinding.
func (m *Manager) finish(e *entry) {
	m.mu.Lock()
	e.running = false
	// A queue that stopped does not relaunch, which would spin on a canceled
	// context.
	retry := m.started && !e.paused && e.task.State == Queued && m.ctx.Err() == nil
	m.mu.Unlock()
	if retry {
		m.launch(e) // wg.Add before Done, so Wait cannot slip through
	}
	m.wg.Done()
}

// run performs the attempts for one task, retrying transient failures.
func (m *Manager) run(ctx context.Context, e *entry) error {
	dest := e.task.Dest
	part := dest + partSuffix
	for attempt := 1; ; attempt++ {
		m.update(e, func(t *Task) { t.Attempts = attempt })
		data, err := m.client.GetImage(ctx, e.task.URL)
		if err == nil {
			if werr := writeFile(dest, part, data); werr != nil {
				return werr
			}
			m.update(e, func(t *Task) {
				t.Bytes, t.Total = int64(len(data)), int64(len(data))
			})
			return nil
		}
		delay, retry := m.retryDelay(attempt)
		if !retry || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
	}
}

// retryDelay reports the wait before the next attempt, if another is allowed.
func (m *Manager) retryDelay(attempt int) (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if attempt > len(m.backoff) {
		return 0, false
	}
	return m.backoff[attempt-1], true
}

// writeFile writes part and renames it over dest, creating parent directories
// first so a fresh download directory works.
func writeFile(dest, part string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(part, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(part, dest); err != nil {
		os.Remove(part)
		return err
	}
	return nil
}

// update mutates a task under the lock and then reports the change.
func (m *Manager) update(e *entry, mutate func(*Task)) {
	m.mu.Lock()
	mutate(&e.task)
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) changed() {
	m.mu.Lock()
	f := m.onChange
	m.mu.Unlock()
	if f != nil {
		f()
	}
}
