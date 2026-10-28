// Package diag keeps a crash log next to the app's other cache files.
//
// Go cannot recover another goroutine's panic, so there is no global handler
// to register. main installs a deferred recover for the main goroutine only.
package diag

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is the version written into crash reports.
const Version = "0.1.0"

// Install creates dir and logs the previous crash, if any. It does not
// register a panic handler; see the package comment.
func Install(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if path, ok := LastCrash(dir); ok {
		log.Printf("上次崩溃日志: %s", path)
	}
	return nil
}

// Report writes a crash report to dir and returns its path. r is the recovered
// panic value and stack the goroutine stack, normally from debug.Stack.
func Report(dir string, r any, stack []byte) error {
	if dir == "" {
		return fmt.Errorf("diag: no log directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	now := time.Now()
	body := fmt.Sprintf("time: %s\nversion: %s\npanic: %v\n\n%s\n",
		now.Format(time.RFC3339), Version, r, stack)
	name := "crash-" + now.Format("20060102-150405") + ".log"
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
}

// LastCrash returns the newest crash report in dir. Report names sort by time,
// so the last name is the newest.
func LastCrash(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	newest := ""
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "crash-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		if name > newest {
			newest = name
		}
	}
	if newest == "" {
		return "", false
	}
	return filepath.Join(dir, newest), true
}
