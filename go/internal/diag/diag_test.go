package diag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportWritesPanicAndStack(t *testing.T) {
	dir := t.TempDir()
	stack := []byte("goroutine 1 [running]:\nmain.main()")
	if err := Report(dir, "boom", stack); err != nil {
		t.Fatalf("Report: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "crash-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("crash files = %v, want one", matches)
	}
	path := matches[0]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"time:", "version: " + Version, "panic: boom", "main.main"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("report missing %q:\n%s", want, b)
		}
	}
}

func TestLastCrashNewest(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "crash-20240101-000000.log")
	newest := filepath.Join(dir, "crash-20240102-000000.log")
	for _, path := range []string{newest, old} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := LastCrash(dir)
	if !ok {
		t.Fatal("LastCrash found nothing")
	}
	if got != newest {
		t.Errorf("LastCrash = %q, want %q", got, newest)
	}
}

func TestLastCrashEmpty(t *testing.T) {
	if path, ok := LastCrash(t.TempDir()); ok {
		t.Errorf("LastCrash = %q, want none", path)
	}
	if path, ok := LastCrash(filepath.Join(t.TempDir(), "missing")); ok {
		t.Errorf("LastCrash on missing dir = %q, want none", path)
	}
}
