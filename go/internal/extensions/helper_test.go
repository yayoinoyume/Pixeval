package extensions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets this test binary act as a fake extension when the host starts
// it with PIXEVAL_EXTENSION_HELPER set. The parent sets the variable for its
// children; a child never runs the tests.
func TestMain(m *testing.M) {
	if os.Getenv("PIXEVAL_EXTENSION_HELPER") == "1" {
		runExtensionHelper()
		os.Exit(0)
	}
	os.Setenv("PIXEVAL_EXTENSION_HELPER", "1")
	os.Exit(m.Run())
}

// runExtensionHelper speaks the host protocol on stdin and stdout. The mode
// comes from --mode=... in the manifest's args.
func runExtensionHelper() {
	mode := "normal"
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "--mode=") {
			mode = strings.TrimPrefix(arg, "--mode=")
		}
	}
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	reply := func(id int64, result any) {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	fail := func(id int64, message string) {
		_ = enc.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": -1, "message": message},
		})
	}
	for {
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := dec.Decode(&req); err != nil {
			return
		}
		switch req.Method {
		case "initialize":
			if mode == "badinit" {
				fail(req.ID, "initialize failed")
				continue
			}
			reply(req.ID, map[string]any{
				"name":         "Fake extension",
				"version":      "1.0.0",
				"capabilities": []string{CapabilityDownloadName},
			})
		case "health":
			reply(req.ID, map[string]any{"ok": mode != "badhealth"})
		case "transform_download_name":
			switch mode {
			case "timeout":
				time.Sleep(10 * time.Second)
				return
			case "crash":
				os.Exit(3)
			case "garbage":
				fmt.Fprintln(os.Stdout, "not json")
				continue
			case "fail":
				fail(req.ID, "transform failed")
				continue
			case "badname":
				reply(req.ID, map[string]any{"name": "../evil"})
				continue
			case "absname":
				reply(req.ID, map[string]any{"name": "/tmp/evil"})
				continue
			case "empty":
				reply(req.ID, map[string]any{"name": ""})
				continue
			}
			var params struct {
				Name    string          `json:"name"`
				Context DownloadContext `json:"context"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				fail(req.ID, err.Error())
				continue
			}
			name := "x_" + params.Name
			if mode == "context" {
				name = fmt.Sprintf("%s_%d_%s_%s_%d_%s", name,
					params.Context.ID, params.Context.Title, params.Context.Artist,
					params.Context.Page, params.Context.Ext)
			}
			reply(req.ID, map[string]any{"name": name})
		case "shutdown":
			reply(req.ID, map[string]any{})
			return
		default:
			fail(req.ID, "unknown method "+req.Method)
		}
	}
}

// installFakeExtension writes an executable fake extension under root. The
// executable is a hard link (or copy) of this test binary, so no external
// binary or network is involved.
func installFakeExtension(t *testing.T, root, id, mode string, capabilities []string) string {
	t.Helper()
	if capabilities == nil {
		capabilities = []string{CapabilityDownloadName}
	}
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	dest := filepath.Join(dir, "ext")
	if err := os.Link(exe, dest); err != nil {
		copyExecutable(t, exe, dest)
	}
	writeManifest(t, dir, Manifest{
		ID:           id,
		Name:         "Demo " + id,
		Version:      "1.2.3",
		Description:  "A fake extension",
		Exec:         "ext",
		Args:         []string{"--mode=" + mode},
		Capabilities: capabilities,
	})
	return dir
}

func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("Open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("Create %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("Copy: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func writeManifest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), data, 0o644); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}
}
