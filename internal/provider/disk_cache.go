package provider

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// diskCache persists parsed sessions to the filesystem so cold-start
// `ccx web` loads don't re-pay the parse cost for sessions the user
// has already viewed in a previous process. Companion to sessionCache
// (in-memory LRU): memory wins for hot access, disk wins across
// process restarts.
//
// Storage: one gob file per source session, named by sha256(abs path).
// Each file holds a diskCacheEntry with mtime+size metadata so reads
// can cheaply check freshness before deserializing the whole tree.
//
// Failure modes are always graceful: a corrupted file, a write error,
// or a missing cache directory all fall back to live parsing. The
// cache is an optimization, never a correctness layer.
type diskCache struct {
	dir string
	mu  sync.Mutex // serializes writes to the same cache dir
}

// diskCacheEntry is the gob envelope around a persisted session.
// SourcePath/Mtime/Size are stored in-band so a reader can verify
// freshness without relying on the filename alone.
type diskCacheEntry struct {
	SourcePath string
	Mtime      time.Time
	Size       int64
	Session    *parser.Session
}

func init() {
	// parser.ContentBlock.ToolInput is `any` and commonly holds
	// map[string]any / []any shapes from JSON decode. gob needs these
	// concrete types registered before it'll encode interface values.
	gob.Register(map[string]any{})
	gob.Register([]any{})
}

// newDiskCache opens or creates a persistent session cache rooted at
// dir. Returns an error if dir can't be created or probed — callers
// should treat that as "no disk cache, fall through to memory only".
func newDiskCache(dir string) (*diskCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &diskCache{dir: dir}, nil
}

// cachePathFor returns the gob file path where sourcePath's parsed
// session lives on disk. Uses sha256 of the absolute source path so
// filenames are stable, fixed-length, and collision-resistant.
func (d *diskCache) cachePathFor(sourcePath string) string {
	sum := sha256.Sum256([]byte(sourcePath))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:])+".gob")
}

// get returns the cached session for sourcePath if the on-disk copy
// exists AND its stored mtime+size match the caller-supplied values
// (which should come from os.Stat on the live file). Stale entries
// are deleted on miss so the cache directory eventually sheds old
// content.
func (d *diskCache) get(sourcePath string, mtime time.Time, size int64) (*parser.Session, bool) {
	cp := d.cachePathFor(sourcePath)
	f, err := os.Open(cp)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var entry diskCacheEntry
	if err := gob.NewDecoder(f).Decode(&entry); err != nil {
		// Corrupt file or schema mismatch (e.g., parser struct changed).
		// Drop it so the next write replaces it with fresh content.
		f.Close()
		_ = os.Remove(cp)
		return nil, false
	}

	if !entry.Mtime.Equal(mtime) || entry.Size != size {
		f.Close()
		_ = os.Remove(cp)
		return nil, false
	}
	return entry.Session, true
}

// put persists a parsed session to disk. Uses a tmp-then-rename write
// so a crash mid-write can't leave a half-encoded file behind. Errors
// are swallowed — a failed write just means the next read will miss
// and re-parse (correct, if slower).
func (d *diskCache) put(sourcePath string, sess *parser.Session, mtime time.Time, size int64) {
	if sess == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	cp := d.cachePathFor(sourcePath)
	tmp := cp + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	entry := diskCacheEntry{
		SourcePath: sourcePath,
		Mtime:      mtime,
		Size:       size,
		Session:    sess,
	}
	if err := gob.NewEncoder(f).Encode(&entry); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	// Atomic replace — readers see either the old file or the new one,
	// never a partial write.
	_ = os.Rename(tmp, cp)
}

// clear removes every entry in the cache directory. Used by tests and
// the (future) `ccx cache clear` command.
func (d *diskCache) clear() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries, err := os.ReadDir(d.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(d.dir, e.Name()))
	}
	return nil
}

// size reports the current entry count. Used by tests.
func (d *diskCache) countEntries() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".gob" {
			n++
		}
	}
	return n
}
