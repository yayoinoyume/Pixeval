package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestHost(t *testing.T, root string) *Host {
	t.Helper()
	h, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(h.StopAll)
	return h
}

// TestHostTransformDownloadName checks an enabled extension is started
// lazily, receives the download context and renames the file.
func TestHostTransformDownloadName(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "context", nil)
	h := openTestHost(t, root)

	exts := h.Extensions()
	if len(exts) != 1 || exts[0].Status != StatusDisabled || exts[0].Enabled {
		t.Fatalf("extensions before enabling: %+v", exts)
	}
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if exts := h.Extensions(); exts[0].Status != StatusStopped {
		t.Fatalf("status after enabling: %q, want %q", exts[0].Status, StatusStopped)
	}

	got := h.TransformDownloadName("7_001.jpg", DownloadContext{
		ID: 7, Title: "title", Artist: "artist", Page: 1, Ext: "jpg",
	})
	want := "x_7_001.jpg_7_title_artist_1_jpg"
	if got != want {
		t.Errorf("transform %q, want %q", got, want)
	}
	if exts := h.Extensions(); exts[0].Status != StatusRunning {
		t.Errorf("status after transform: %q, want %q", exts[0].Status, StatusRunning)
	}

	h.StopAll()
	if exts := h.Extensions(); exts[0].Status != StatusStopped {
		t.Errorf("status after StopAll: %q, want %q", exts[0].Status, StatusStopped)
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after StopAll", len(h.procs))
	}
}

// TestHostTransformsInOrder checks two enabled extensions run in id order, so
// the second sees the first one's name.
func TestHostTransformsInOrder(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "a", "normal", nil)
	installFakeExtension(t, root, "b", "normal", nil)
	h := openTestHost(t, root)
	for _, id := range []string{"a", "b"} {
		if err := h.SetEnabled(id, true); err != nil {
			t.Fatalf("SetEnabled %s: %v", id, err)
		}
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_x_name" {
		t.Errorf("transform %q, want x_x_name", got)
	}
}

// TestHostInitializeFailure checks a failed handshake leaves no process and
// falls back to the original name.
func TestHostInitializeFailure(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "badinit", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}
	if exts := h.Extensions(); exts[0].Status != StatusError || exts[0].Error == "" {
		t.Errorf("extension after a failed initialize: %+v", exts[0])
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after a failed initialize", len(h.procs))
	}
}

// TestHostTimeout checks a silent extension times out, falls back to the
// original name and is marked as an error with no process left.
func TestHostTimeout(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "timeout", nil)
	h := openTestHost(t, root)
	h.timeout = 100 * time.Millisecond
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	start := time.Now()
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("timeout took %s, want under 2s", elapsed)
	}
	exts := h.Extensions()
	if exts[0].Status != StatusError || exts[0].Error == "" {
		t.Errorf("extension after timeout: %+v", exts[0])
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after timeout", len(h.procs))
	}
}

// TestHostBrokenExtensionNotRestarted checks a broken extension is not started
// again on every transform: only the first call waits for the timeout, later
// calls fall back to the previous name immediately.
func TestHostBrokenExtensionNotRestarted(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "timeout", nil)
	h := openTestHost(t, root)
	h.timeout = 300 * time.Millisecond
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Fatalf("first transform %q, want the original name", got)
	}
	for i := 2; i <= 3; i++ {
		start := time.Now()
		if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
			t.Fatalf("transform %d %q, want the original name", i, got)
		}
		if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
			t.Errorf("transform %d took %s, want an immediate fallback", i, elapsed)
		}
	}
	if exts := h.Extensions(); exts[0].Status != StatusError || exts[0].Error == "" {
		t.Errorf("extension after repeated transforms: %+v", exts[0])
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after repeated transforms", len(h.procs))
	}

	// Re-enabling still clears the error, so the user can retry a fixed extension.
	if err := h.SetEnabled("demo", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if h.errors["demo"] != "" {
		t.Errorf("error %q still set after re-enabling", h.errors["demo"])
	}
}

// TestHostCrash checks an extension that exits mid-request falls back and is
// marked as an error.
func TestHostCrash(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "crash", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}
	if exts := h.Extensions(); exts[0].Status != StatusError || exts[0].Error == "" {
		t.Errorf("extension after crash: %+v", exts[0])
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after crash", len(h.procs))
	}
}

// TestHostProtocolError checks a malformed response marks the extension as an
// error and leaves the name alone.
func TestHostProtocolError(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "garbage", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}
	if exts := h.Extensions(); exts[0].Status != StatusError {
		t.Errorf("extension after a protocol error: %+v", exts[0])
	}
}

// TestHostHealthFailure checks the health response is part of the handshake.
func TestHostHealthFailure(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "badhealth", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}
	if exts := h.Extensions(); exts[0].Status != StatusError || exts[0].Error == "" {
		t.Errorf("extension after a failed health check: %+v", exts[0])
	}
}

// TestHostRejectsBadNames checks empty, absolute and parent names from an
// extension are refused and the extension is marked as an error.
func TestHostRejectsBadNames(t *testing.T) {
	for _, mode := range []string{"badname", "absname", "empty"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			installFakeExtension(t, root, "demo", mode, nil)
			h := openTestHost(t, root)
			if err := h.SetEnabled("demo", true); err != nil {
				t.Fatalf("SetEnabled: %v", err)
			}
			if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
				t.Errorf("transform %q, want the original name", got)
			}
			if exts := h.Extensions(); exts[0].Status != StatusError {
				t.Errorf("extension after %q: %+v", mode, exts[0])
			}
		})
	}
}

// TestHostEnabledPersists checks the enabled set is saved 0600 and read back
// by the next host.
func TestHostEnabledPersists(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	path := filepath.Join(root, stateName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode %04o, want 0600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("state JSON: %v", err)
	}
	if len(state.Enabled) != 1 || state.Enabled[0] != "demo" {
		t.Errorf("enabled %v, want [demo]", state.Enabled)
	}

	again := openTestHost(t, root)
	if exts := again.Extensions(); len(exts) != 1 || !exts[0].Enabled {
		t.Fatalf("enabled set not read back: %+v", exts)
	}
	if err := again.SetEnabled("demo", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
		t.Fatalf("state after disable: %q, %v", data, err)
	}
}

// TestHostBadStateStartsEmpty checks a broken extensions.json means nothing is
// enabled instead of blocking the host.
func TestHostBadStateStartsEmpty(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	if err := os.WriteFile(filepath.Join(root, stateName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := openTestHost(t, root)
	if exts := h.Extensions(); len(exts) != 1 || exts[0].Enabled {
		t.Fatalf("extensions from a broken state: %+v", exts)
	}
}

// TestHostUnknownStateIDIgnored checks an id that is not installed is dropped,
// including one the page tries to enable.
func TestHostUnknownStateIDIgnored(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	data := []byte(`{"enabled":["ghost","demo"]}`)
	if err := os.WriteFile(filepath.Join(root, stateName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h := openTestHost(t, root)
	exts := h.Extensions()
	if len(exts) != 1 || !exts[0].Enabled {
		t.Fatalf("extensions: %+v", exts)
	}
	if err := h.SetEnabled("ghost", true); err != nil {
		t.Fatalf("SetEnabled ghost: %v", err)
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_name" {
		t.Errorf("transform %q, want x_name", got)
	}
}

// TestHostDisableStopsProcess checks disabling an extension shuts its process
// down instead of leaving it running.
func TestHostDisableStopsProcess(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_name" {
		t.Fatalf("transform %q", got)
	}
	p := h.procs["demo"]
	if p == nil {
		t.Fatal("no process after a transform")
	}
	if err := h.SetEnabled("demo", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !p.exited() {
		t.Error("process still running after disabling the extension")
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after disabling", len(h.procs))
	}
	if exts := h.Extensions(); exts[0].Status != StatusDisabled {
		t.Errorf("status %q, want %q", exts[0].Status, StatusDisabled)
	}
}

// TestHostReloadPicksUpChanges checks Reload stops the old process and reads a
// newly installed extension.
func TestHostReloadPicksUpChanges(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_name" {
		t.Fatalf("transform %q", got)
	}
	p := h.procs["demo"]
	installFakeExtension(t, root, "late", "normal", nil)
	if err := h.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if p == nil || !p.exited() {
		t.Error("Reload did not stop the old process")
	}
	exts := h.Extensions()
	if len(exts) != 2 || exts[0].ID != "demo" || exts[1].ID != "late" {
		t.Fatalf("extensions after Reload: %+v", exts)
	}
	if !exts[0].Enabled || exts[0].Status != StatusStopped {
		t.Errorf("demo after Reload: %+v", exts[0])
	}
	if exts[1].Enabled {
		t.Errorf("late starts enabled: %+v", exts[1])
	}
}

// TestHostStopAllIsIdempotent checks StopAll waits for the process and a
// second call is harmless.
func TestHostStopAllIsIdempotent(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "normal", nil)
	h := openTestHost(t, root)
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_name" {
		t.Fatalf("transform %q", got)
	}
	p := h.procs["demo"]
	h.StopAll()
	if p == nil || !p.exited() {
		t.Error("StopAll did not wait for the process")
	}
	h.StopAll()
}

// TestOpenCreatesRoot checks Open creates the extensions directory private to
// the user.
func TestOpenCreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "extensions")
	h := openTestHost(t, root)
	if h.Dir() != root {
		t.Errorf("Dir %q, want %q", h.Dir(), root)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("root mode %04o, want 0700", perm)
	}
}

// TestHostInvalidManifests checks a folder without a usable manifest is shown
// as an invalid extension instead of disappearing silently.
func TestHostInvalidManifests(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "ok", "normal", nil)
	if err := os.MkdirAll(filepath.Join(root, "missing"), 0o755); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, badDir, Manifest{ID: "bad", Name: "n", Version: "v", Exec: "../escape"})

	h := openTestHost(t, root)
	if exts := h.Extensions(); len(exts) != 1 || exts[0].ID != "ok" {
		t.Fatalf("extensions: %+v, want only ok", exts)
	}
	invalid := h.InvalidExtensions()
	if len(invalid) != 2 || invalid[0].Folder != "bad" || invalid[1].Folder != "missing" {
		t.Fatalf("invalid: %+v, want bad and missing", invalid)
	}
	for _, item := range invalid {
		if item.Error == "" {
			t.Errorf("invalid folder %q has no error", item.Folder)
		}
	}
}

// TestHostReloadExtension checks reloading one extension stops only its
// process, clears its error and reads its new manifest.
func TestHostReloadExtension(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "demo", "badhealth", nil)
	installFakeExtension(t, root, "keep", "normal", nil)
	h := openTestHost(t, root)
	for _, id := range []string{"demo", "keep"} {
		if err := h.SetEnabled(id, true); err != nil {
			t.Fatalf("SetEnabled %s: %v", id, err)
		}
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_name" {
		t.Fatalf("transform %q, want x_name", got)
	}
	demo := h.Extensions()[0]
	if demo.ID != "demo" || demo.Status != StatusError || demo.Error == "" {
		t.Fatalf("demo after timeout: %+v", demo)
	}
	keepProc := h.procs["keep"]
	if keepProc == nil {
		t.Fatal("keep is not running")
	}

	dir := filepath.Join(root, "demo")
	writeManifest(t, dir, Manifest{
		ID: "demo", Name: "Demo demo", Version: "1.2.3", Description: "A fake extension",
		Exec: "ext", Args: []string{"--mode=normal"}, Capabilities: []string{CapabilityDownloadName},
	})
	if err := h.ReloadExtension("demo"); err != nil {
		t.Fatalf("ReloadExtension: %v", err)
	}
	if h.errors["demo"] != "" {
		t.Errorf("error %q still set after reloading one extension", h.errors["demo"])
	}
	exts := h.Extensions()
	if len(exts) != 2 || exts[0].ID != "demo" || exts[0].Status != StatusStopped {
		t.Fatalf("extensions after ReloadExtension: %+v", exts)
	}
	if h.procs["keep"] != keepProc || keepProc.exited() {
		t.Error("reloading demo stopped or replaced keep")
	}

	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_x_name" {
		t.Fatalf("transform after reloading demo %q, want x_x_name", got)
	}
	demoProc := h.procs["demo"]
	if demoProc == nil {
		t.Fatal("demo did not start after it was fixed")
	}
	if err := h.ReloadExtension("demo"); err != nil {
		t.Fatalf("ReloadExtension a running demo: %v", err)
	}
	if !demoProc.exited() {
		t.Error("ReloadExtension did not stop demo's process")
	}
	if h.procs["keep"] != keepProc || keepProc.exited() {
		t.Error("reloading demo stopped or replaced keep")
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_x_name" {
		t.Errorf("transform after reloading a running demo %q, want x_x_name", got)
	}
}

// TestHostReloadExtensionFixesInvalid checks a fixed manifest becomes a normal
// extension and a deleted folder stops being listed.
func TestHostReloadExtensionFixesInvalid(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{ID: "demo", Name: "Demo", Version: "1.0.0", Exec: "missing"})
	h := openTestHost(t, root)
	if invalid := h.InvalidExtensions(); len(invalid) != 1 || invalid[0].Folder != "demo" {
		t.Fatalf("invalid before fixing: %+v", invalid)
	}

	installFakeExtension(t, root, "demo", "normal", nil)
	if err := h.ReloadExtension("demo"); err != nil {
		t.Fatalf("ReloadExtension after fixing: %v", err)
	}
	if invalid := h.InvalidExtensions(); len(invalid) != 0 {
		t.Fatalf("invalid after fixing: %+v", invalid)
	}
	if exts := h.Extensions(); len(exts) != 1 || exts[0].ID != "demo" || exts[0].Enabled {
		t.Fatalf("extensions after fixing: %+v", exts)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := h.ReloadExtension("demo"); err != nil {
		t.Fatalf("ReloadExtension after deleting: %v", err)
	}
	if exts := h.Extensions(); len(exts) != 0 {
		t.Errorf("extensions after deleting: %+v", exts)
	}
	if invalid := h.InvalidExtensions(); len(invalid) != 0 {
		t.Errorf("invalid after deleting: %+v", invalid)
	}
}

// TestHostStopAllBudget checks StopAll caps each extension's shutdown, so a
// few unresponsive extensions do not hold closing the window for seconds.
func TestHostStopAllBudget(t *testing.T) {
	root := t.TempDir()
	ids := []string{"a", "b", "c"}
	for _, id := range ids {
		installFakeExtension(t, root, id, "hangshutdown", nil)
	}
	h := openTestHost(t, root)
	for _, id := range ids {
		if err := h.SetEnabled(id, true); err != nil {
			t.Fatalf("SetEnabled %s: %v", id, err)
		}
	}
	if got := h.TransformDownloadName("name", DownloadContext{}); got != "x_x_x_name" {
		t.Fatalf("transform %q, want x_x_x_name", got)
	}
	procs := make([]*process, 0, len(ids))
	for _, id := range ids {
		p := h.procs[id]
		if p == nil {
			t.Fatalf("%s did not start", id)
		}
		procs = append(procs, p)
	}

	start := time.Now()
	h.StopAll()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("StopAll took %s, want under 3s", elapsed)
	}
	if len(h.procs) != 0 {
		t.Errorf("%d processes left after StopAll", len(h.procs))
	}
	for _, p := range procs {
		if !p.exited() {
			t.Error("a process was not exited after StopAll")
		}
	}
}
