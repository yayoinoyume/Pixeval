package imagex

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cachePrefix marks the files in the cache directory that the loader owns;
// eviction removes only these and never touches a file beside them.
const cachePrefix = "imagex-"

// diskPath is where an image's bytes are cached, named by their URL's hash.
func (l *Loader) diskPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(l.dir, cachePrefix+hex.EncodeToString(sum[:]))
}

// diskGet reads cached bytes and marks the file recently used, as eviction
// goes by last access.
func (l *Loader) diskGet(url string) ([]byte, bool) {
	if l.dir == "" {
		return nil, false
	}
	path := l.diskPath(url)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return data, true
}

// diskPut writes bytes under a temporary name and renames them into place, so
// a reader never sees a half-written file.
func (l *Loader) diskPut(url string, data []byte) {
	if l.dir == "" {
		return
	}
	l.diskMu.Lock()
	defer l.diskMu.Unlock()

	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return
	}
	path := l.diskPath(url)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return
	}
	l.evictDisk()
}

// evictDisk removes the least recently used files until the cache fits its
// limit. It also clears temporary files a crash left behind.
func (l *Loader) evictDisk() {
	l.mu.Lock()
	limit := l.diskLimit
	l.mu.Unlock()

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		at   time.Time
	}
	files := make([]file, 0, len(entries))
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.IsDir() {
			continue
		}
		path := filepath.Join(l.dir, e.Name())
		if !strings.HasPrefix(e.Name(), cachePrefix) {
			continue
		}
		if strings.HasSuffix(e.Name(), ".tmp") {
			if time.Since(info.ModTime()) > time.Hour {
				os.Remove(path)
			}
			continue
		}
		total += info.Size()
		files = append(files, file{path: path, size: info.Size(), at: info.ModTime()})
	}
	if total <= limit {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for _, f := range files {
		if total <= limit {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
