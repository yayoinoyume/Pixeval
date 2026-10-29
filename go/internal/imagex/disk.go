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

// cacheFile is one file in the cache directory that the loader owns.
type cacheFile struct {
	path string
	size int64
	at   time.Time
}

// ownedFiles lists the files the loader owns. A file beside them, which the
// user put there, is not one of them.
func (l *Loader) ownedFiles() []cacheFile {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil
	}
	files := make([]cacheFile, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.IsDir() || !strings.HasPrefix(e.Name(), cachePrefix) {
			continue
		}
		files = append(files, cacheFile{
			path: filepath.Join(l.dir, e.Name()),
			size: info.Size(),
			at:   info.ModTime(),
		})
	}
	return files
}

// evictDisk removes the least recently used files until the cache fits its
// limit. It also clears temporary files a crash left behind.
func (l *Loader) evictDisk() {
	l.mu.Lock()
	limit := l.diskLimit
	l.mu.Unlock()

	files := make([]cacheFile, 0, 16)
	var total int64
	for _, f := range l.ownedFiles() {
		if strings.HasSuffix(f.path, ".tmp") {
			if time.Since(f.at) > time.Hour {
				os.Remove(f.path)
			}
			continue
		}
		total += f.size
		files = append(files, f)
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

// ClearDisk deletes the cached bytes, leaving every other file in the cache
// directory alone. The decoded bitmaps stay in memory, so the pictures on
// screen do not blink out.
func (l *Loader) ClearDisk() {
	l.diskMu.Lock()
	defer l.diskMu.Unlock()
	for _, f := range l.ownedFiles() {
		os.Remove(f.path)
	}
}

// DiskUsage is the bytes the cache holds. It is approximate: it counts the
// files while they are there, and a load may add one meanwhile.
func (l *Loader) DiskUsage() int64 {
	l.diskMu.Lock()
	defer l.diskMu.Unlock()
	var total int64
	for _, f := range l.ownedFiles() {
		total += f.size
	}
	return total
}
