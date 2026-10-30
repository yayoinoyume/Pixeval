package extensions

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout bounds every request to an extension, including the
// initialize handshake and a download-name transform.
const DefaultTimeout = 2 * time.Second

// stopBudget bounds one extension's whole shutdown, request and wait, when
// the host stops several at once. Three unresponsive extensions then delay
// closing the window by about a second and a half, not by a full request
// timeout each.
const stopBudget = 500 * time.Millisecond

// Status is what the host knows about one extension.
type Status string

const (
	StatusDisabled Status = "disabled"
	StatusStopped  Status = "stopped"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Extension is one manifest as the page shows it.
type Extension struct {
	ID           string
	Name         string
	Version      string
	Description  string
	Capabilities []string
	Enabled      bool
	Status       Status
	Error        string
}

// InvalidExtension is a folder under the extensions root that does not hold a
// usable manifest. The page shows it, so a broken install is visible instead
// of silently missing.
type InvalidExtension struct {
	Folder string
	Error  string
}

// DownloadContext is what a download_name extension sees about the page it is
// naming.
type DownloadContext struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Page   int    `json:"page"`
	Ext    string `json:"ext"`
}

// Host owns the discovered extensions and the processes they run in. It is
// safe for the main thread to call while a background caller transforms a
// name: the mutex serializes both.
type Host struct {
	mu        sync.Mutex
	root      string
	statePath string
	timeout   time.Duration
	manifests []Manifest
	invalid   []InvalidExtension
	enabled   map[string]bool
	procs     map[string]*process
	errors    map[string]string
}

// DefaultDir is where the app keeps extensions: the user's config directory,
// like the settings.
func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "extensions"), nil
}

// OpenDefault opens the host under DefaultDir.
func OpenDefault() (*Host, error) {
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return Open(dir)
}

// Open creates root when it is missing, discovers its extensions and reads
// the enabled set. No extension process starts until a transform needs it.
func Open(root string) (*Host, error) {
	if root == "" {
		return nil, errors.New("extensions: empty root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	h := &Host{
		root:      root,
		statePath: filepath.Join(root, stateName),
		timeout:   DefaultTimeout,
		enabled:   map[string]bool{},
		procs:     map[string]*process{},
		errors:    map[string]string{},
	}
	if err := h.reloadLocked(); err != nil {
		return nil, err
	}
	return h, nil
}

// Dir is the extensions folder the page offers to open.
func (h *Host) Dir() string {
	if h == nil {
		return ""
	}
	return h.root
}

// Reload stops the running processes, then reads the manifests and the
// enabled set again. An extension removed from disk disappears; a new one
// appears disabled unless the saved set names it.
func (h *Host) Reload() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reloadLocked()
}

func (h *Host) reloadLocked() error {
	h.stopAllLocked()
	h.errors = map[string]string{}
	h.enabled = loadEnabled(h.statePath)
	manifests, invalid, err := discover(h.root)
	if err != nil {
		return err
	}
	h.manifests = manifests
	h.invalid = invalid
	known := make(map[string]bool, len(manifests))
	for _, m := range manifests {
		known[m.ID] = true
	}
	for id := range h.enabled {
		if !known[id] {
			delete(h.enabled, id)
		}
	}
	return nil
}

// Extensions is the page's snapshot, sorted by id.
func (h *Host) Extensions() []Extension {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Extension, 0, len(h.manifests))
	for _, m := range h.manifests {
		ext := Extension{
			ID:           m.ID,
			Name:         m.Name,
			Version:      m.Version,
			Description:  m.Description,
			Capabilities: append([]string(nil), m.Capabilities...),
			Enabled:      h.enabled[m.ID],
			Error:        h.errors[m.ID],
		}
		switch {
		case !ext.Enabled:
			ext.Status = StatusDisabled
		case ext.Error != "":
			ext.Status = StatusError
		case h.procs[m.ID] != nil:
			ext.Status = StatusRunning
		default:
			ext.Status = StatusStopped
		}
		out = append(out, ext)
	}
	return out
}

// InvalidExtensions is the page's snapshot of folders that failed to load,
// sorted by folder.
func (h *Host) InvalidExtensions() []InvalidExtension {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]InvalidExtension(nil), h.invalid...)
}

// ReloadExtension stops id's process, clears its runtime error and reads its
// folder again, leaving the other extensions alone. A manifest that is still
// broken stays in InvalidExtensions; a folder that is gone disappears.
func (h *Host) ReloadExtension(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return fmt.Errorf("extensions: invalid folder %q", id)
	}
	if p := h.procs[id]; p != nil {
		delete(h.procs, id)
		p.stop(stopBudget)
	}
	delete(h.errors, id)
	dir := filepath.Join(h.root, id)
	m, err := loadManifest(dir)
	if err != nil {
		h.removeManifestLocked(id)
		if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
			h.removeInvalidLocked(id)
			return nil
		}
		h.setInvalidLocked(id, err)
		return nil
	}
	h.removeInvalidLocked(id)
	h.upsertManifestLocked(m)
	return nil
}

// SetEnabled saves whether id runs on the next transform. Disabling stops a
// process already running; enabling starts nothing until it is needed. An
// unknown id is ignored.
func (h *Host) SetEnabled(id string, on bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.manifestLocked(id); !ok {
		return nil
	}
	if on == h.enabled[id] {
		return nil
	}
	if on {
		h.enabled[id] = true
		if err := saveEnabled(h.statePath, h.enabled); err != nil {
			delete(h.enabled, id)
			return err
		}
		delete(h.errors, id)
		return nil
	}
	delete(h.enabled, id)
	if err := saveEnabled(h.statePath, h.enabled); err != nil {
		h.enabled[id] = true
		return err
	}
	delete(h.errors, id)
	if p := h.procs[id]; p != nil {
		delete(h.procs, id)
		p.stop(h.timeout)
	}
	return nil
}

// TransformDownloadName runs name through every enabled extension that
// declares download_name, in id order. A failure marks that extension as
// broken and leaves the name as the previous extension left it, so a broken
// extension never loses the original file name. A broken extension is skipped
// without another start attempt until it is re-enabled or the host is reloaded.
func (h *Host) TransformDownloadName(name string, ctx DownloadContext) string {
	if h == nil {
		return name
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.manifests {
		if !h.enabled[m.ID] || !hasCapability(m, CapabilityDownloadName) {
			continue
		}
		if h.errors[m.ID] != "" {
			continue
		}
		p, err := h.ensureLocked(m)
		if err != nil {
			h.errors[m.ID] = err.Error()
			continue
		}
		params := struct {
			Name    string          `json:"name"`
			Context DownloadContext `json:"context"`
		}{Name: name, Context: ctx}
		raw, err := p.call("transform_download_name", params, h.timeout)
		if err != nil {
			h.failLocked(m.ID, err)
			continue
		}
		var result struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			h.failLocked(m.ID, fmt.Errorf("extensions: %s: transform result: %w", m.ID, err))
			continue
		}
		if !validDownloadName(result.Name) {
			h.failLocked(m.ID, fmt.Errorf("extensions: %s: rejected download name %q", m.ID, result.Name))
			continue
		}
		name = result.Name
	}
	return name
}

// StopAll shuts every extension down and waits for it. The window calls it
// when it closes.
func (h *Host) StopAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopAllLocked()
}

func (h *Host) stopAllLocked() {
	ids := make([]string, 0, len(h.procs))
	for id := range h.procs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := h.procs[id]
		delete(h.procs, id)
		p.stop(stopBudget)
	}
}

func (h *Host) manifestLocked(id string) (Manifest, bool) {
	for _, m := range h.manifests {
		if m.ID == id {
			return m, true
		}
	}
	return Manifest{}, false
}

func (h *Host) upsertManifestLocked(m Manifest) {
	for i := range h.manifests {
		if h.manifests[i].ID == m.ID {
			h.manifests[i] = m
			return
		}
	}
	h.manifests = append(h.manifests, m)
	sort.Slice(h.manifests, func(i, j int) bool { return h.manifests[i].ID < h.manifests[j].ID })
}

func (h *Host) removeManifestLocked(id string) {
	out := h.manifests[:0]
	for _, m := range h.manifests {
		if m.ID != id {
			out = append(out, m)
		}
	}
	h.manifests = out
}

func (h *Host) setInvalidLocked(folder string, err error) {
	for i := range h.invalid {
		if h.invalid[i].Folder == folder {
			h.invalid[i].Error = err.Error()
			return
		}
	}
	h.invalid = append(h.invalid, InvalidExtension{Folder: folder, Error: err.Error()})
	sort.Slice(h.invalid, func(i, j int) bool { return h.invalid[i].Folder < h.invalid[j].Folder })
}

func (h *Host) removeInvalidLocked(folder string) {
	out := h.invalid[:0]
	for _, item := range h.invalid {
		if item.Folder != folder {
			out = append(out, item)
		}
	}
	h.invalid = out
}

func (h *Host) ensureLocked(m Manifest) (*process, error) {
	if p := h.procs[m.ID]; p != nil {
		return p, nil
	}
	p, err := h.startLocked(m)
	if err != nil {
		return nil, err
	}
	h.procs[m.ID] = p
	delete(h.errors, m.ID)
	return p, nil
}

// startLocked starts m and performs the initialize and health handshakes. A
// failed handshake leaves no process behind.
func (h *Host) startLocked(m Manifest) (*process, error) {
	dir := filepath.Join(h.root, m.ID)
	execPath, err := manifestExecPath(dir, m.Exec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(execPath, m.Args...)
	prepareProcess(cmd)
	cmd.Dir = dir
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("extensions: start %s: %w", m.ID, err)
	}
	p := &process{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), nextID: 1}
	initParams := struct {
		ProtocolVersion int      `json:"protocolVersion"`
		Capabilities    []string `json:"capabilities"`
	}{ProtocolVersion: ProtocolVersion, Capabilities: m.Capabilities}
	raw, err := p.call("initialize", initParams, h.timeout)
	if err != nil {
		p.stop(h.timeout)
		return nil, fmt.Errorf("extensions: %s: initialize: %w", m.ID, err)
	}
	var init struct {
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &init); err != nil {
		p.stop(h.timeout)
		return nil, fmt.Errorf("extensions: %s: initialize result: %w", m.ID, err)
	}
	if strings.TrimSpace(init.Name) == "" || strings.TrimSpace(init.Version) == "" {
		p.stop(h.timeout)
		return nil, fmt.Errorf("extensions: %s: initialize result has no name or version", m.ID)
	}
	raw, err = p.call("health", nil, h.timeout)
	if err != nil {
		p.stop(h.timeout)
		return nil, fmt.Errorf("extensions: %s: health: %w", m.ID, err)
	}
	var health struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &health); err != nil || !health.OK {
		p.stop(h.timeout)
		return nil, fmt.Errorf("extensions: %s: health check failed", m.ID)
	}
	return p, nil
}

// failLocked marks id as broken and stops its process, so a crashed or
// misbehaving extension cannot keep answering later calls.
func (h *Host) failLocked(id string, err error) {
	if p := h.procs[id]; p != nil {
		delete(h.procs, id)
		p.stop(h.timeout)
	}
	h.errors[id] = err.Error()
}

// validDownloadName accepts only a plain file name: no directory part, no
// parent segment and no empty name.
func validDownloadName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return false
	}
	return true
}
