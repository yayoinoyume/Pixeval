// Package download runs the app's bounded image download queue.
package download

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// partSuffix marks the in-progress file, renamed over Dest when it is complete.
const partSuffix = ".part"

// defaultConcurrency is used when New or SetConcurrency gets a non-positive
// limit.
const defaultConcurrency = 3

// permanentStatuses will not change on a retry.
var permanentStatuses = []string{"400 Bad Request", "401 Unauthorized", "403 Forbidden", "404 Not Found", "410 Gone"}

// State is where a task sits in its lifecycle.
type State int

const (
	Queued State = iota
	Running
	Done
	Failed
	Canceled
)

// Task is one download. Bytes and Total stay zero until the body arrives,
// since pixiv.Client.GetImage returns the whole image at once.
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

	mu          sync.Mutex
	concurrency int
	entries     []*entry // join order, which Snapshot keeps
	started     bool
	ctx         context.Context
	onChange    func()
	wg          sync.WaitGroup
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
		client:      client,
		sem:         make(chan struct{}, concurrency),
		concurrency: concurrency,
		backoff:     []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second},
		ctx:         context.Background(),
	}
}

// SetConcurrency changes how many downloads run at once. Tasks already
// running keep the slots they took, so the total can exceed n until they
// finish; tasks started after the call use the new limit.
func (m *Manager) SetConcurrency(n int) {
	if n < 1 {
		n = defaultConcurrency
	}
	m.mu.Lock()
	if n != m.concurrency {
		m.concurrency = n
		m.sem = make(chan struct{}, n)
	}
	m.mu.Unlock()
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
	if len(added) > 0 {
		m.changed()
	}
	return len(added)
}

// Start begins downloading. A canceled run is restarted by the next call, which
// takes the new ctx and relaunches whatever is still Queued.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started && m.ctx.Err() == nil {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.ctx = ctx
	entries := append([]*entry(nil), m.entries...)
	m.mu.Unlock()
	for _, e := range entries {
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
	m.entries = slices.DeleteFunc(m.entries, func(x *entry) bool { return x == e })
	e.paused = true // keeps the worker from requeueing a dropped task
	cancel, dest := e.cancel, e.task.Dest
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	os.Remove(dest + partSuffix)
	m.changed()
}

// Snapshot copies the tasks in the order added; finished ones stay until
// Remove drops them.
func (m *Manager) Snapshot() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, len(m.entries))
	for i, e := range m.entries {
		out[i] = e.task
	}
	return out
}

// OnChange registers a callback fired after any change, on the goroutine that
// made it, so the caller must hop to its own thread.
func (m *Manager) OnChange(f func()) {
	m.mu.Lock()
	m.onChange = f
	m.mu.Unlock()
}

// Wait blocks until no worker is running.
func (m *Manager) Wait() { m.wg.Wait() }

func finished(s State) bool { return s == Done || s == Failed || s == Canceled }

func (m *Manager) findLocked(id string) *entry {
	i := slices.IndexFunc(m.entries, func(e *entry) bool { return e.task.ID == id })
	if i < 0 {
		return nil
	}
	return m.entries[i]
}

// launch starts a worker for e unless one already owns it or the run is dead.
func (m *Manager) launch(e *entry) {
	m.mu.Lock()
	if !m.started || m.ctx.Err() != nil || e.running || e.paused || e.task.State != Queued {
		m.mu.Unlock()
		return
	}
	e.running = true
	sem := m.sem
	m.wg.Add(1)
	m.mu.Unlock()
	go m.worker(e, sem)
}

// worker owns one task from queue slot to terminal state. sem is the limit the
// task was launched under, so a later SetConcurrency cannot swap it midway.
func (m *Manager) worker(e *entry, sem chan struct{}) {
	defer m.finish(e)

	m.mu.Lock()
	dest, overwrite, state, parent := e.task.Dest, e.task.Overwrite, e.task.State, m.ctx
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
	case sem <- struct{}{}:
	case <-parent.Done():
		m.update(e, func(t *Task) { t.State = Canceled })
		return
	}
	defer func() { <-sem }()

	m.mu.Lock()
	if e.paused || e.task.State != Queued {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	e.cancel = cancel // set with Running, so Cancel always reaches the attempt
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
	paused := e.paused
	m.mu.Unlock()

	switch {
	case parent.Err() != nil:
		// The queue stopped: the attempt is not a failure of the task.
		os.Remove(dest + partSuffix)
		m.update(e, func(t *Task) {
			if t.State != Canceled {
				t.State, t.Err = Canceled, ""
			}
		})
	case paused || canceled:
		// A pause, or a Resume that undid it: the canceled attempt stays
		// queued for the next launch instead of failing.
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

// finish clears the running mark and requeues a task whose Resume arrived
// while the worker was unwinding.
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

// run performs the attempts for one task, retrying only what can succeed later.
func (m *Manager) run(ctx context.Context, e *entry) error {
	dest := e.task.Dest
	part := dest + partSuffix
	m.mu.Lock()
	backoff := m.backoff
	m.mu.Unlock()
	for attempt := 1; ; attempt++ {
		m.update(e, func(t *Task) { t.Attempts = attempt })
		data, err := m.client.GetImage(ctx, e.task.URL)
		if err == nil {
			if err = writeFile(ctx, dest, part, data); err == nil {
				m.update(e, func(t *Task) {
					t.Bytes, t.Total = int64(len(data)), int64(len(data))
				})
				return nil
			}
		}
		if ctx.Err() != nil || !retryable(err) || attempt > len(backoff) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff[attempt-1]):
		}
	}
}

// retryable reports whether another attempt can help. pixiv.GetImage only says
// "image: <status>", so a status text is all there is to match on.
// shortcut: string matching, upgrade once pixiv returns a typed status error.
func retryable(err error) bool {
	if err == nil {
		return false
	}
	for _, s := range permanentStatuses {
		if strings.Contains(err.Error(), s) {
			return false
		}
	}
	return true
}

// writeFile writes part and renames it over dest, creating parent directories
// first. The ctx check before the rename keeps a Cancel that lands mid-write
// from leaving a finished file behind.
func writeFile(ctx context.Context, dest, part string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(part, data, 0o644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		os.Remove(part)
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
