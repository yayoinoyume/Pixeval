package extensions

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadManifest reads a valid manifest from a folder named for its id.
func TestLoadManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ext"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{
		ID: "demo", Name: "Demo", Version: "1.0.0", Description: "d",
		Exec: "ext", Args: []string{"--mode=normal"}, Capabilities: []string{CapabilityDownloadName},
	})
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if m.ID != "demo" || m.Name != "Demo" || m.Version != "1.0.0" || m.Exec != "ext" {
		t.Errorf("manifest %+v", m)
	}
}

// TestManifestRejectsBadID checks ids that could not be a folder name or a
// JSON-RPC peer are refused.
func TestManifestRejectsBadID(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ext"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "Demo", "../demo", "demo/x", "-demo", ".demo"} {
		m := Manifest{ID: id, Name: "n", Version: "v", Exec: "ext"}
		if err := validateManifest(dir, m); err == nil {
			t.Errorf("validateManifest accepted id %q", id)
		}
	}
}

// TestManifestRejectsIDMismatch checks the manifest id must name its folder.
func TestManifestRejectsIDMismatch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ext"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	m := Manifest{ID: "other", Name: "n", Version: "v", Exec: "ext"}
	if err := validateManifest(dir, m); err == nil {
		t.Fatal("validateManifest accepted an id for another folder")
	}
}

// TestManifestRejectsExecEscape checks exec cannot be absolute or climb out of
// the extension folder.
func TestManifestRejectsExecEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, exec := range []string{"", "/bin/sh", "../evil", "a/../../evil", ".", "sub/../.."} {
		m := Manifest{ID: "demo", Name: "n", Version: "v", Exec: exec}
		if err := validateManifest(dir, m); err == nil {
			t.Errorf("validateManifest accepted exec %q", exec)
		}
	}
}

// TestManifestRejectsSymlinkEscape checks an executable symlink is resolved
// before the folder check, so a link to outside is refused.
func TestManifestRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "ext")); err != nil {
		t.Fatal(err)
	}
	m := Manifest{ID: "demo", Name: "n", Version: "v", Exec: "ext"}
	if err := validateManifest(dir, m); err == nil {
		t.Fatal("validateManifest accepted a symlink leaving the extension folder")
	}
}

// TestManifestAllowsSymlinkInside checks a symlink that stays inside the
// extension folder is still a valid executable path.
func TestManifestAllowsSymlinkInside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "ext")); err != nil {
		t.Fatal(err)
	}
	m := Manifest{ID: "demo", Name: "n", Version: "v", Exec: "ext"}
	if err := validateManifest(dir, m); err != nil {
		t.Fatalf("validateManifest: %v", err)
	}
}

// TestDiscoverSkipsInvalid checks discovery keeps valid folders sorted and
// skips a missing manifest, an invalid one and plain files.
func TestDiscoverSkipsInvalid(t *testing.T) {
	root := t.TempDir()
	installFakeExtension(t, root, "b", "normal", nil)
	installFakeExtension(t, root, "a", "normal", nil)
	if err := os.MkdirAll(filepath.Join(root, "missing"), 0o755); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, badDir, Manifest{ID: "bad", Name: "n", Version: "v", Exec: "../escape"})
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("discover %+v, want a and b", got)
	}
}

// TestValidDownloadName checks only a plain file name is accepted from an
// extension.
func TestValidDownloadName(t *testing.T) {
	for _, name := range []string{"7_001.jpg", "title with space.png", "作品.jpg"} {
		if !validDownloadName(name) {
			t.Errorf("validDownloadName(%q) = false", name)
		}
	}
	for _, name := range []string{"", ".", "..", "/tmp/x", `a/b`, `a\b`, "x\x00y"} {
		if validDownloadName(name) {
			t.Errorf("validDownloadName(%q) = true", name)
		}
	}
}
