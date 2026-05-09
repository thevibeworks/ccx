package provider

import (
	"container/list"
	"os"
	"sync"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// sessionCache is a bounded LRU cache over parsed sessions, keyed by
// absolute file path and invalidated by mtime+size. Holding parsed
// trees in memory across repeat views is the bulk of #4's measured
// win: cold parse of a 1500-message session is ~19ms and a 5000-message
// session is ~54ms; a cache hit is sub-microsecond.
//
// We cap the cache at a small number of entries (default 16) because
// parsed sessions are large — a 5000-message session holds ~50 MB of
// in-memory state. 16 entries × 50 MB = ~800 MB worst case; typical
// sessions are under 2000 messages and use ~10-20 MB each.
type sessionCache struct {
	mu   sync.Mutex
	cap  int
	lru  *list.List
	byID map[string]*list.Element
}

type cacheEntry struct {
	path    string
	mtime   time.Time
	size    int64
	session *parser.Session
}

func newSessionCache(cap int) *sessionCache {
	if cap <= 0 {
		cap = 16
	}
	return &sessionCache{
		cap:  cap,
		lru:  list.New(),
		byID: make(map[string]*list.Element),
	}
}

// getOrLoad returns the cached session for path if it exists and is
// still fresh (mtime+size match the on-disk file). Otherwise it calls
// loader, stores the result, and returns it. Concurrent calls for the
// same path may parse more than once on first access — that's fine;
// the last writer wins and subsequent reads hit the cache.
func (c *sessionCache) getOrLoad(path string, loader func() (*parser.Session, error)) (*parser.Session, error) {
	info, statErr := os.Stat(path)

	c.mu.Lock()
	if el, ok := c.byID[path]; ok {
		ent := el.Value.(*cacheEntry)
		if statErr == nil && ent.mtime.Equal(info.ModTime()) && ent.size == info.Size() {
			c.lru.MoveToFront(el)
			session := ent.session
			c.mu.Unlock()
			return session, nil
		}
		// Stale — drop the stale entry, fall through to reload
		c.lru.Remove(el)
		delete(c.byID, path)
	}
	c.mu.Unlock()

	session, err := loader()
	if err != nil || session == nil {
		return session, err
	}
	if statErr != nil {
		return session, nil // can't cache without stable metadata
	}

	c.mu.Lock()
	// Race check: another goroutine may have populated while we were loading
	if el, ok := c.byID[path]; ok {
		c.lru.Remove(el)
		delete(c.byID, path)
	}
	ent := &cacheEntry{
		path:    path,
		mtime:   info.ModTime(),
		size:    info.Size(),
		session: session,
	}
	el := c.lru.PushFront(ent)
	c.byID[path] = el
	for c.lru.Len() > c.cap {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		c.lru.Remove(oldest)
		delete(c.byID, oldest.Value.(*cacheEntry).path)
	}
	c.mu.Unlock()

	return session, nil
}

// clear drops all cached entries. Used by tests and for diagnostics.
func (c *sessionCache) clear() {
	c.mu.Lock()
	c.lru.Init()
	c.byID = make(map[string]*list.Element)
	c.mu.Unlock()
}

// size reports the current entry count. Used by tests.
func (c *sessionCache) size() int {
	c.mu.Lock()
	n := c.lru.Len()
	c.mu.Unlock()
	return n
}
