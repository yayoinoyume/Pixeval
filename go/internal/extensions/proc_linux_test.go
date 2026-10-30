//go:build linux

package extensions

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHostKillsProcessGroup checks an extension that starts a child is stopped
// with its whole process group, so the child does not outlive it.
func TestHostKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	installFakeExtensionArgs(t, root, "demo", "spawnchild", nil, "--pidfile="+pidFile)
	h := openTestHost(t, root)
	h.timeout = 500 * time.Millisecond
	if err := h.SetEnabled("demo", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if got := h.TransformDownloadName("7_001.jpg", DownloadContext{}); got != "7_001.jpg" {
		t.Errorf("transform %q, want the original name", got)
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("child pid %q: %v", data, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if !processAlive(pid) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("child %d survived the process group kill", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processAlive reports whether pid is still running. A zombie counts as gone,
// since it can no longer do any work.
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// The state follows the parenthesized command name.
	if end := strings.LastIndexByte(string(stat), ')'); end >= 0 {
		fields := strings.Fields(string(stat)[end+1:])
		if len(fields) > 0 && fields[0] == "Z" {
			return false
		}
	}
	return true
}
