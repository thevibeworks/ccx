package provider

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func writeCacheFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionCache_HitSkipsLoader(t *testing.T) {
	path := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"hi"}}`+"\n")
	cache := newSessionCache(8)

	var calls int32
	loader := func() (*parser.Session, error) {
		atomic.AddInt32(&calls, 1)
		return parser.ParseSession(path)
	}

	first, err := cache.getOrLoad(path, loader)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("first load returned nil session")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("first load should call loader once, got %d", calls)
	}

	second, err := cache.getOrLoad(path, loader)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("second load should hit cache, loader calls = %d, want 1", calls)
	}
	// Same pointer means same in-memory tree (no re-parse)
	if first != second {
		t.Error("cache hit returned a different session pointer than first load")
	}
}

func TestSessionCache_InvalidatesOnMtimeChange(t *testing.T) {
	path := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"original"}}`+"\n")
	cache := newSessionCache(8)

	var calls int32
	loader := func() (*parser.Session, error) {
		atomic.AddInt32(&calls, 1)
		return parser.ParseSession(path)
	}

	if _, err := cache.getOrLoad(path, loader); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("first load calls = %d, want 1", calls)
	}

	// Rewrite with different content AND bump mtime by 2 seconds
	if err := os.WriteFile(path, []byte(`{"type":"user","uuid":"u2","timestamp":"2026-04-01T10:00:01Z","message":{"content":"modified"}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	if _, err := cache.getOrLoad(path, loader); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected loader to be called again after mtime change, calls = %d", calls)
	}
}

func TestSessionCache_LRUEvictsOldest(t *testing.T) {
	cache := newSessionCache(3)

	paths := make([]string, 5)
	for i := range paths {
		paths[i] = writeCacheFixture(t, `{"type":"user","uuid":"u","timestamp":"2026-04-01T10:00:00Z","message":{"content":"x"}}`+"\n")
	}

	loader := func(p string) func() (*parser.Session, error) {
		return func() (*parser.Session, error) { return parser.ParseSession(p) }
	}

	// Fill cache with 3 entries
	for i := 0; i < 3; i++ {
		if _, err := cache.getOrLoad(paths[i], loader(paths[i])); err != nil {
			t.Fatal(err)
		}
	}
	if cache.size() != 3 {
		t.Errorf("cache size after 3 loads = %d, want 3", cache.size())
	}

	// Touch path[0] so it becomes most-recent
	if _, err := cache.getOrLoad(paths[0], loader(paths[0])); err != nil {
		t.Fatal(err)
	}

	// Load 2 more — should evict paths[1] and paths[2] (oldest), keeping paths[0]
	for i := 3; i < 5; i++ {
		if _, err := cache.getOrLoad(paths[i], loader(paths[i])); err != nil {
			t.Fatal(err)
		}
	}
	if cache.size() != 3 {
		t.Errorf("cache size after eviction = %d, want 3", cache.size())
	}

	// paths[0] should still be cached — load should not increment a counter
	var postCalls int32
	counted := func() (*parser.Session, error) {
		atomic.AddInt32(&postCalls, 1)
		return parser.ParseSession(paths[0])
	}
	if _, err := cache.getOrLoad(paths[0], counted); err != nil {
		t.Fatal(err)
	}
	if postCalls != 0 {
		t.Errorf("paths[0] should still be cached after eviction (touched most recently); loader called %d times", postCalls)
	}
}

func TestSessionCache_ConcurrentLoadsSafe(t *testing.T) {
	path := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"x"}}`+"\n")
	cache := newSessionCache(8)

	loader := func() (*parser.Session, error) {
		return parser.ParseSession(path)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.getOrLoad(path, loader); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if cache.size() != 1 {
		t.Errorf("cache size after concurrent loads = %d, want 1", cache.size())
	}
}

func TestSessionCache_ClearDropsEverything(t *testing.T) {
	cache := newSessionCache(8)
	path := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"x"}}`+"\n")
	if _, err := cache.getOrLoad(path, func() (*parser.Session, error) { return parser.ParseSession(path) }); err != nil {
		t.Fatal(err)
	}
	if cache.size() == 0 {
		t.Fatal("cache should not be empty after load")
	}
	cache.clear()
	if cache.size() != 0 {
		t.Errorf("cache size after clear = %d, want 0", cache.size())
	}
}

// writeLargeSessionForBench writes a realistic ~1500-message fixture.
// Mirrors writeLargeFixture in parser/session_test.go but lives here so
// the provider package has its own benchmark baseline.
func writeLargeSessionForBench(b *testing.B, msgCount int) string {
	b.Helper()
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.jsonl")

	// Reuse the synthetic JSONL shape from the parser benchmarks. The
	// exact content shape isn't the point — the point is that a session
	// this size takes ~19 ms to cold-parse, so the cached path should
	// be dramatically faster.
	var buf []byte
	longText := "the quick brown fox jumps over the lazy dog. the quick brown fox jumps over the lazy dog. the quick brown fox jumps over the lazy dog. the quick brown fox jumps over the lazy dog. the quick brown fox jumps over the lazy dog."
	for i := 0; i < msgCount; i++ {
		switch i % 2 {
		case 0:
			buf = append(buf, []byte(`{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"m-`)...)
			buf = append(buf, []byte(intToStr(i))...)
			buf = append(buf, []byte(`","message":{"content":"`+longText+`"}}`+"\n")...)
		case 1:
			buf = append(buf, []byte(`{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"m-`)...)
			buf = append(buf, []byte(intToStr(i))...)
			buf = append(buf, []byte(`","parentUuid":"m-`)...)
			buf = append(buf, []byte(intToStr(i-1))...)
			buf = append(buf, []byte(`","message":{"role":"assistant","model":"claude-sonnet-4-5","content":"`+longText+`","usage":{"input_tokens":1000,"output_tokens":400,"cache_read_input_tokens":5000,"cache_creation_input_tokens":200}}}`+"\n")...)
		}
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		b.Fatal(err)
	}
	return path
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + intToStr(-i)
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func BenchmarkSessionCache_ColdVsWarm(b *testing.B) {
	path := writeLargeSessionForBench(b, 1500)
	loader := func() (*parser.Session, error) { return parser.ParseSession(path) }

	b.Run("Cold (no cache)", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := parser.ParseSession(path)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Warm (cache hit)", func(b *testing.B) {
		cache := newSessionCache(8)
		// Prime
		if _, err := cache.getOrLoad(path, loader); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := cache.getOrLoad(path, loader)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
