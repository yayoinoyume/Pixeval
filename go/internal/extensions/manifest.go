// Package extensions hosts Pixeval Go's local extensions: separate executables
// that speak newline-delimited JSON-RPC 2.0 over standard input and output.
//
// An extension is a folder under the extensions directory with an
// extension.json manifest. It is started on first use, not when enabled, and
// is never loaded through a shell. Extensions run with the user's privileges
// and are not sandboxed.
package extensions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// ProtocolVersion is the handshake version this host speaks.
	ProtocolVersion = 1
	// CapabilityDownloadName lets an extension rename a download.
	CapabilityDownloadName = "download_name"

	manifestName = "extension.json"
	stateName    = "extensions.json"
)

// idPattern keeps an extension id usable as a folder name and as a JSON-RPC
// peer name: lowercase letters, digits, dots, underscores and dashes.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Manifest is one extension's extension.json.
type Manifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Exec         string   `json:"exec"`
	Args         []string `json:"args,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// loadManifest reads and validates the manifest in dir.
func loadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if err := validateManifest(dir, m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// validateManifest checks that m can be run safely from dir: its id names the
// folder, its executable is a regular file inside that folder even after
// resolving symlinks, and the display fields are present.
func validateManifest(dir string, m Manifest) error {
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("manifest: invalid id %q", m.ID)
	}
	if m.ID != filepath.Base(dir) {
		return fmt.Errorf("manifest: id %q does not match folder %q", m.ID, filepath.Base(dir))
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("manifest: name is empty")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("manifest: version is empty")
	}
	if _, err := manifestExecPath(dir, m.Exec); err != nil {
		return err
	}
	return nil
}

// manifestExecPath resolves an exec field to a regular file inside dir. The
// resolved path is what the host starts, so a symlink cannot point outside.
func manifestExecPath(dir, exec string) (string, error) {
	if exec == "" {
		return "", errors.New("manifest: exec is empty")
	}
	if filepath.IsAbs(exec) || filepath.VolumeName(exec) != "" {
		return "", fmt.Errorf("manifest: exec %q is absolute", exec)
	}
	clean := filepath.Clean(exec)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest: exec %q leaves the extension folder", exec)
	}
	dirPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("manifest: %w", err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(dirPath, clean))
	if err != nil {
		return "", fmt.Errorf("manifest: exec: %w", err)
	}
	rel, err := filepath.Rel(dirPath, target)
	if err != nil {
		return "", fmt.Errorf("manifest: exec: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("manifest: exec %q leaves the extension folder", exec)
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("manifest: exec: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("manifest: exec %q is not a regular file", exec)
	}
	return target, nil
}

// discover reads the valid extensions under root, sorted by id, and reports
// every folder that has no usable manifest. A broken folder is listed rather
// than stopping the host or silently disappearing.
func discover(root string) ([]Manifest, []InvalidExtension, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}
	var manifests []Manifest
	var invalid []InvalidExtension
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		m, err := loadManifest(dir)
		if err != nil {
			invalid = append(invalid, InvalidExtension{Folder: entry.Name(), Error: err.Error()})
			continue
		}
		if m.ID != entry.Name() {
			invalid = append(invalid, InvalidExtension{
				Folder: entry.Name(),
				Error:  fmt.Sprintf("manifest: id %q does not match folder", m.ID),
			})
			continue
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	sort.Slice(invalid, func(i, j int) bool { return invalid[i].Folder < invalid[j].Folder })
	return manifests, invalid, nil
}

// hasCapability reports whether m declares capability.
func hasCapability(m Manifest, capability string) bool {
	for _, c := range m.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}
