package parser

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The session-metadata cache makes discovery cheap. Without it, every
// DiscoverProjects pass re-reads and JSON-decodes every line of every
// session file (gigabytes for a real corpus) just to produce the tiny
// Session metadata rows used by lists — measured at ~10s per pass, and
// the web server runs several passes per request.
//
// The cache maps source path → quickParse result, keyed on mtime+size
// so changed files (including actively-written sessions) re-parse.
// A nil Session records "this file yields no listable session"
// (no summary, warmup, unparseable) so skipped files aren't re-scanned
// forever. Everything persists to a single gob file loaded lazily on
// first use; entries are small (metadata only, no message trees).
//
// Like the full-session disk cache, this is an optimization layer:
// any failure (unwritable dir, corrupt file, schema drift) silently
// degrades to live parsing.
type sessionMetaCache struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	dirty   bool
	entries map[string]metaCacheEntry
}

type metaCacheEntry struct {
	Mtime   time.Time
	Size    int64
	Session *Session // nil = known non-listable file
}

var metaCache = &sessionMetaCache{}

// InitMetaCache points the package-level metadata cache at a persistent
// gob file. Call once at startup (before any discovery). An empty path
// disables persistence; lookups and stores still work in-memory.
func InitMetaCache(path string) {
	metaCache.mu.Lock()
	defer metaCache.mu.Unlock()
	metaCache.path = path
	metaCache.loaded = false
	metaCache.entries = nil
}

// MetaLookup returns the cached discovery metadata for file if the
// cache holds an entry matching mtime+size. The second return reports
// a hit; a hit with a nil Session means the file is known to produce
// no listable session. Returned sessions are copies — callers may
// mutate them freely.
func MetaLookup(file string, mtime time.Time, size int64) (*Session, bool) {
	c := metaCache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()

	entry, ok := c.entries[file]
	if !ok || !entry.Mtime.Equal(mtime) || entry.Size != size {
		return nil, false
	}
	if entry.Session == nil {
		return nil, true
	}
	copied := *entry.Session
	return &copied, true
}

// MetaStore records the discovery metadata for file. Pass a nil
// session to record that the file yields no listable session.
func MetaStore(file string, mtime time.Time, size int64, session *Session) {
	c := metaCache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLoadedLocked()

	entry := metaCacheEntry{Mtime: mtime, Size: size}
	if session != nil {
		copied := *session
		entry.Session = &copied
	}
	c.entries[file] = entry
	c.dirty = true
}

// FlushMetaCache persists the cache to disk if anything changed since
// the last flush. Discovery passes call this once at the end so a
// cold pass over thousands of files costs one write, not thousands.
func FlushMetaCache() {
	c := metaCache
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.dirty || c.path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	if err := gob.NewEncoder(f).Encode(c.entries); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	// Atomic replace — readers see the old file or the new one, never
	// a partial write.
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return
	}
	c.dirty = false
}

func (c *sessionMetaCache) ensureLoadedLocked() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.entries = make(map[string]metaCacheEntry)

	if c.path == "" {
		return
	}
	f, err := os.Open(c.path)
	if err != nil {
		return
	}
	defer f.Close()

	var entries map[string]metaCacheEntry
	if err := gob.NewDecoder(f).Decode(&entries); err != nil {
		// Corrupt or schema-drifted cache: start fresh; the next flush
		// overwrites it.
		return
	}
	c.entries = entries
}
