package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	ext "github.com/yayoinoyume/pixeval-go/internal/extensions"
)

// The app's test binary doubles as a fake extension. The parent sets the flag
// for its children; a child runs the helper before TestMain, so no test or
// config setup runs in it.
func init() {
	if os.Getenv("PIXEVAL_APP_EXTENSION_HELPER") == "1" {
		runAppExtensionHelper()
		os.Exit(0)
	}
	os.Setenv("PIXEVAL_APP_EXTENSION_HELPER", "1")
}

// runAppExtensionHelper speaks the extension protocol on stdin and stdout.
func runAppExtensionHelper() {
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
			reply(req.ID, map[string]any{
				"name":         "Fake extension",
				"version":      "1.2.3",
				"capabilities": []string{ext.CapabilityDownloadName},
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
			case "fail":
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -1, "message": "transform failed"},
				})
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
				Name    string              `json:"name"`
				Context ext.DownloadContext `json:"context"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
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
		}
	}
}

// installAppFakeExtension writes an executable fake extension under root. The
// executable is a hard link (or copy) of this test binary.
func installAppFakeExtension(t *testing.T, root, id, mode string) {
	t.Helper()
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
		in, err := os.Open(exe)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			in.Close()
			t.Fatalf("Create: %v", err)
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			in.Close()
			t.Fatalf("Copy: %v", err)
		}
		out.Close()
		in.Close()
	}
	manifest := ext.Manifest{
		ID:           id,
		Name:         "Fake " + id,
		Version:      "1.2.3",
		Description:  "A fake extension",
		Exec:         "ext",
		Args:         []string{"--mode=" + mode},
		Capabilities: []string{ext.CapabilityDownloadName},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extension.json"), data, 0o644); err != nil {
		t.Fatalf("Write manifest: %v", err)
	}
}

// newExtensionsApp is a settings page tall enough to show the Extensions row
// at the bottom of the About card.
func newExtensionsApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New()
	if a.extensions != nil {
		t.Cleanup(a.extensions.StopAll)
	}
	a.route = routeSettings
	return a, ui.NewTester(a.View, 1000, 2400)
}

// newExtensionsPage is an app on the extensions page, with root as its
// extension folder and an optional fake extension installed before start.
func newExtensionsPage(t *testing.T, install bool, mode string) (*App, *ui.Tester, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root, err := ext.DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if install {
		installAppFakeExtension(t, root, "demo", mode)
	}
	a := New()
	if a.extensions != nil {
		t.Cleanup(a.extensions.StopAll)
	}
	a.route = routeExtensions
	return a, ui.NewTester(a.View, 1000, 900), root
}

// TestSettingsOpensExtensions checks the settings row opens the extension
// page, which shows the empty state and real folder actions, and returns to
// the settings page.
func TestSettingsOpensExtensions(t *testing.T) {
	a, tt := newExtensionsApp(t)
	tt.Frame()
	if _, ok := tt.Find(a.t("ext.settingsHint")); !ok {
		t.Fatalf("the extensions row is missing: %q", tt.Texts())
	}
	if err := tt.Click(a.t("ext.settingsHint")); err != nil {
		t.Fatalf("Click extensions: %v", err)
	}
	if a.route != routeExtensions {
		t.Fatalf("route %q after extensions, want %q", a.route, routeExtensions)
	}
	tt.Frame()
	for _, want := range []string{
		a.t("ext.empty"),
		a.t("ext.emptyHint"),
		a.t("ext.openFolder"),
		a.t("ext.reload"),
		a.t("ext.protocol"),
		a.t("ext.security"),
	} {
		if !tt.HasText(want) {
			t.Errorf("%q missing from extensions: %q", want, tt.Texts())
		}
	}
	for _, fake := range []string{
		"添加扩展", "卸载扩展", "add extension", "uninstall extension",
		"ajouter une extension", "désinstaller", "добавить расширение", "удалить расширение",
	} {
		if _, ok := tt.Find(fake); ok {
			t.Errorf("extensions page offers a fake action %q", fake)
		}
	}
	if err := tt.Click(a.t("common.back")); err != nil {
		t.Fatalf("Click back from extensions: %v", err)
	}
	if a.route != routeSettings {
		t.Errorf("route %q after back, want %q", a.route, routeSettings)
	}
}

// TestExtensionsPageEscapeReturnsToSettings checks Esc leaves the extension
// page for the settings page it came from.
func TestExtensionsPageEscapeReturnsToSettings(t *testing.T) {
	a, tt := newExtensionsApp(t)
	tt.Frame()
	if err := tt.Click(a.t("ext.settingsHint")); err != nil {
		t.Fatalf("Click extensions: %v", err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.route != routeSettings {
		t.Errorf("route %q after Esc, want %q", a.route, routeSettings)
	}
}

// TestExtensionsPageOpensFolder checks the open-folder button hands the
// extensions directory to the system opener.
func TestExtensionsPageOpensFolder(t *testing.T) {
	a, tt, root := newExtensionsPage(t, false, "normal")
	var opened string
	a.openPathFn = func(path string) error {
		opened = path
		return nil
	}
	tt.Frame()
	if err := tt.Click(a.t("ext.openFolder")); err != nil {
		t.Fatalf("Click open folder: %v", err)
	}
	if opened != root {
		t.Errorf("opened %q, want %q", opened, root)
	}
}

// TestExtensionsPageListsInstalled checks an installed extension shows its
// name, version, description, capability and state, and its switch saves the
// enabled state.
func TestExtensionsPageListsInstalled(t *testing.T) {
	a, tt, root := newExtensionsPage(t, true, "normal")
	tt.Frame()
	for _, want := range []string{
		"Fake demo",
		a.tf("ext.version", "1.2.3"),
		"A fake extension",
		a.tf("ext.capabilities", ext.CapabilityDownloadName),
		a.t("ext.statusDisabled"),
	} {
		if !tt.HasText(want) {
			t.Errorf("%q missing from extensions: %q", want, tt.Texts())
		}
	}
	if err := tt.Click(a.tf("ext.toggle", "Fake demo")); err != nil {
		t.Fatalf("Click the extension switch: %v", err)
	}
	tt.Frame()
	exts := a.extensions.Extensions()
	if len(exts) != 1 || !exts[0].Enabled || exts[0].Status != ext.StatusStopped {
		t.Fatalf("extension after enabling: %+v", exts)
	}
	if !tt.HasText(a.t("ext.statusStopped")) {
		t.Errorf("stopped state missing after enabling: %q", tt.Texts())
	}
	info, err := os.Stat(filepath.Join(root, "extensions.json"))
	if err != nil {
		t.Fatalf("Stat extensions.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("extensions.json mode %04o, want 0600", perm)
	}
}

// TestExtensionsPageReloadPicksUpNew checks Reload reads a folder changed
// after the app started.
func TestExtensionsPageReloadPicksUpNew(t *testing.T) {
	a, tt, root := newExtensionsPage(t, false, "normal")
	tt.Frame()
	if !tt.HasText(a.t("ext.empty")) {
		t.Fatalf("empty state missing: %q", tt.Texts())
	}
	installAppFakeExtension(t, root, "late", "normal")
	if err := tt.Click(a.t("ext.reload")); err != nil {
		t.Fatalf("Click reload: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Fake late") {
		t.Errorf("the new extension is missing after Reload: %q", tt.Texts())
	}
	if exts := a.extensions.Extensions(); len(exts) != 1 || exts[0].Enabled {
		t.Errorf("new extension state: %+v", exts)
	}
}

// TestExtensionsPageShowsInvalidManifest checks a folder with a broken
// manifest is visible, and reloading just that folder picks up the fix.
func TestExtensionsPageShowsInvalidManifest(t *testing.T) {
	a, tt, root := newExtensionsPage(t, false, "normal")
	badDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "extension.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(a.t("ext.reload")); err != nil {
		t.Fatalf("Click reload: %v", err)
	}
	tt.Frame()
	if !tt.HasText(a.tf("ext.invalid", "bad")) {
		t.Errorf("invalid extension row missing: %q", tt.Texts())
	}
	if !tt.HasText(a.tf("ext.reloadOne", "bad")) {
		t.Errorf("reload button missing for the invalid row: %q", tt.Texts())
	}
	if tt.HasText(a.t("ext.empty")) {
		t.Errorf("empty state shown despite an invalid extension: %q", tt.Texts())
	}

	installAppFakeExtension(t, root, "bad", "normal")
	if err := tt.Click(a.tf("ext.reloadOne", "bad")); err != nil {
		t.Fatalf("Click reload one: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("Fake bad"); !ok {
		t.Errorf("fixed extension missing after reloading it: %q", tt.Texts())
	}
	if tt.HasText(a.tf("ext.invalid", "bad")) {
		t.Errorf("invalid row still shown after fixing it: %q", tt.Texts())
	}
}

// TestExtensionsPageReloadsOneExtension checks the card button reloads only
// its own folder, not the whole extensions folder.
func TestExtensionsPageReloadsOneExtension(t *testing.T) {
	a, tt, root := newExtensionsPage(t, true, "normal")
	tt.Frame()
	installAppFakeExtension(t, root, "late", "normal")
	if err := tt.Click(a.tf("ext.reloadOne", "Fake demo")); err != nil {
		t.Fatalf("Click reload one: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("Fake late"); ok {
		t.Errorf("reloading one extension discovered another: %q", tt.Texts())
	}
	if exts := a.extensions.Extensions(); len(exts) != 1 || exts[0].ID != "demo" {
		t.Errorf("extensions after reloading one: %+v", exts)
	}

	if err := os.RemoveAll(filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(a.tf("ext.reloadOne", "Fake demo")); err != nil {
		t.Fatalf("Click reload one after removing: %v", err)
	}
	tt.Frame()
	if _, ok := tt.Find("Fake demo"); ok {
		t.Errorf("removed extension still shown: %q", tt.Texts())
	}
}

// TestExtensionsPageFitsNarrowWindow checks the extension page in a 1024 by
// 720 window: its actions, cards and texts stay inside the width instead of
// running off the side.
func TestExtensionsPageFitsNarrowWindow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root, err := ext.DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	installAppFakeExtension(t, root, "demo", "normal")
	a := New()
	if a.extensions != nil {
		t.Cleanup(a.extensions.StopAll)
	}
	a.route = routeExtensions
	const width = 1024
	tt := ui.NewTester(a.View, width, 720)
	tt.Frame()

	for _, label := range []string{
		a.t("nav.extensions"),
		a.t("ext.openFolder"),
		a.t("ext.reload"),
		"Fake demo",
		a.tf("ext.version", "1.2.3"),
		"A fake extension",
		a.tf("ext.capabilities", ext.CapabilityDownloadName),
		a.t("ext.statusDisabled"),
		a.tf("ext.reloadOne", "Fake demo"),
	} {
		r, ok := tt.Find(label)
		if !ok || r.W == 0 {
			t.Fatalf("%q is not laid out: %q", label, tt.Texts())
		}
		if r.X < 0 || r.X+r.W > width {
			t.Errorf("%q at %v overflows the %d wide window", label, r, width)
		}
	}
}
