package provider

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// realParseBackend is a pointer-receiver Backend that parses real files
// from disk via parser.ParseSession and counts how many times it was
// invoked. The counter is what disk-cache tests check to prove a hit
// skipped the live parse.
type realParseBackend struct {
	homes      []string
	parseCount int32
}

func (b *realParseBackend) ID() string                          { return "real" }
func (b *realParseBackend) Homes() []string                     { return b.homes }
func (b *realParseBackend) DiscoverProjects() ([]*parser.Project, error) { return nil, nil }
func (b *realParseBackend) FindProject(string) (*parser.Project, error)  { return nil, nil }
func (b *realParseBackend) FindSession(string, string) (*parser.Session, error) {
	return nil, nil
}
func (b *realParseBackend) ParseSession(filePath string) (*parser.Session, error) {
	atomic.AddInt32(&b.parseCount, 1)
	return parser.ParseSession(filePath)
}

// realSession parses a small in-memory session so disk-cache round-trip
// tests exercise the actual parser.Session shape (nested messages,
// ContentBlock.ToolInput map, time.Time timestamps, nil pointers).
// Hand-constructed sessions are too sanitised — if gob can't encode a
// real parse tree we want to know from the test, not from a user bug
// report later.
func realSession(t *testing.T) *parser.Session {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"hello world"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"thinking","thinking":"Let me think..."},{"type":"text","text":"Here is my reply"},{"type":"tool_use","id":"t1","name":"Task","input":{"subagent_type":"Plan","description":"design auth"}}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":2000,"cache_creation_input_tokens":500}}}
{"type":"user","timestamp":"2026-04-01T10:00:02Z","uuid":"u2","parentUuid":"a1","message":{"content":"follow up"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:03Z","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","model":"claude-sonnet-4-5","content":"done","usage":{"input_tokens":200,"output_tokens":80,"cache_read_input_tokens":2500,"cache_creation_input_tokens":0}}}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	session, err := parser.ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestDiskCache_RoundTripsRealSession(t *testing.T) {
	// Prove gob can serialize a real parsed Session and deserialize
	// it with the same shape. Without this test the disk cache could
	// silently drop tool inputs, content blocks, or usage data and
	// nobody would notice until a user hit a stale cache file.
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	original := realSession(t)
	mtime := time.Now()
	dc.put("/fake/source.jsonl", original, mtime, 1234)

	got, ok := dc.get("/fake/source.jsonl", mtime, 1234)
	if !ok {
		t.Fatal("expected cache hit after put")
	}
	if got == nil {
		t.Fatal("cache returned nil session on hit")
	}
	if len(got.RootMessages) != len(original.RootMessages) {
		t.Errorf("root count: got %d, want %d", len(got.RootMessages), len(original.RootMessages))
	}
	if got.Stats.InputTokens != original.Stats.InputTokens {
		t.Errorf("stats.input lost: got %d, want %d", got.Stats.InputTokens, original.Stats.InputTokens)
	}
	if got.Stats.CostUSD != original.Stats.CostUSD {
		t.Errorf("stats.cost lost: got %v, want %v", got.Stats.CostUSD, original.Stats.CostUSD)
	}

	// Walk the tree and verify usage + content survive the round trip
	var roundTripped *parser.Message
	var walk func([]*parser.Message)
	walk = func(msgs []*parser.Message) {
		for _, m := range msgs {
			if m.UUID == "a1" {
				roundTripped = m
				return
			}
			walk(m.Children)
		}
	}
	walk(got.RootMessages)
	if roundTripped == nil {
		t.Fatal("message a1 missing after round trip")
	}
	if roundTripped.Usage == nil {
		t.Error("a1.Usage lost on round trip")
	} else if roundTripped.Usage.InputTokens != 100 {
		t.Errorf("a1.Usage.InputTokens = %d, want 100", roundTripped.Usage.InputTokens)
	}
	if len(roundTripped.Content) != 3 {
		t.Errorf("a1.Content: got %d blocks, want 3 (thinking + text + tool_use)", len(roundTripped.Content))
	}
}

func TestDiskCache_MissOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}

	original := realSession(t)
	t0 := time.Now()
	dc.put("/fake/path.jsonl", original, t0, 100)

	if _, ok := dc.get("/fake/path.jsonl", t0.Add(1*time.Second), 100); ok {
		t.Error("expected miss on mtime drift")
	}
	if dc.countEntries() != 0 {
		t.Errorf("stale entry should be removed, got %d entries", dc.countEntries())
	}
}

func TestDiskCache_MissOnSizeChange(t *testing.T) {
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	dc.put("/fake/path.jsonl", realSession(t), t0, 100)
	if _, ok := dc.get("/fake/path.jsonl", t0, 200); ok {
		t.Error("expected miss on size drift")
	}
}

func TestDiskCache_MissOnNonExistentPath(t *testing.T) {
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := dc.get("/never/stored.jsonl", time.Now(), 0); ok {
		t.Error("expected miss for never-cached path")
	}
}

func TestDiskCache_PutNilSessionIsNoop(t *testing.T) {
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	dc.put("/fake/nil.jsonl", nil, time.Now(), 0)
	if dc.countEntries() != 0 {
		t.Errorf("nil-session put should be noop, got %d entries", dc.countEntries())
	}
}

func TestDiskCache_ClearRemovesEverything(t *testing.T) {
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	dc.put("/a", realSession(t), time.Now(), 1)
	dc.put("/b", realSession(t), time.Now(), 2)
	dc.put("/c", realSession(t), time.Now(), 3)
	if dc.countEntries() != 3 {
		t.Fatalf("expected 3 entries, got %d", dc.countEntries())
	}
	if err := dc.clear(); err != nil {
		t.Fatal(err)
	}
	if dc.countEntries() != 0 {
		t.Errorf("after clear: %d entries, want 0", dc.countEntries())
	}
}

func TestDiskCache_CorruptFileGracefullyDropped(t *testing.T) {
	// Simulates a torn write or stale gob format (parser struct
	// changed between ccx versions). The reader should treat the
	// corrupt file as a miss and clean up.
	dir := t.TempDir()
	dc, err := newDiskCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Write nonsense into the file we'd expect to find for /fake/x
	cp := dc.cachePathFor("/fake/x.jsonl")
	if err := os.WriteFile(cp, []byte("not gob at all"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := dc.get("/fake/x.jsonl", time.Now(), 100); ok {
		t.Error("corrupt cache file should not produce a hit")
	}
	if _, err := os.Stat(cp); !os.IsNotExist(err) {
		t.Error("corrupt cache file should have been removed")
	}
}

func TestMulti_DiskCacheSurvivesMultiRebuild(t *testing.T) {
	// Simulates `ccx web` restart: build Multi, parse a session, tear
	// it down, build a fresh Multi pointing at the same disk dir, parse
	// the same session again. The backend's parseCount should NOT
	// increment the second time because the disk cache satisfies it.
	sessionPath := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"x"}}`+"\n")
	diskDir := t.TempDir()

	// First "process": parse once, populating both memory and disk.
	b1 := &realParseBackend{homes: []string{filepath.Dir(sessionPath)}}
	m1 := NewMultiWithDiskCache(diskDir, b1)
	if _, err := m1.ParseSession(sessionPath); err != nil {
		t.Fatalf("first parse: %v", err)
	}
	if atomic.LoadInt32(&b1.parseCount) != 1 {
		t.Errorf("first process: parseCount = %d, want 1", atomic.LoadInt32(&b1.parseCount))
	}

	// Second "process": new Multi (fresh memory cache) but same disk
	// dir. The second parse should hit the disk and NOT invoke the
	// backend's live parser.
	b2 := &realParseBackend{homes: []string{filepath.Dir(sessionPath)}}
	m2 := NewMultiWithDiskCache(diskDir, b2)
	if m2.cache.size() != 0 {
		t.Fatalf("fresh Multi should have empty memory cache, got %d", m2.cache.size())
	}
	if _, err := m2.ParseSession(sessionPath); err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if atomic.LoadInt32(&b2.parseCount) != 0 {
		t.Errorf("second process should hit disk cache, but backend parsed %d times", atomic.LoadInt32(&b2.parseCount))
	}

	// And memory cache should now hold the session again
	if m2.cache.size() != 1 {
		t.Errorf("expected 1 in-memory entry after disk-to-memory promotion, got %d", m2.cache.size())
	}
}

func TestMulti_DiskCacheInvalidatesOnFileMtimeChange(t *testing.T) {
	// Edit the session file after caching. Next access must re-parse
	// via the backend because (mtime, size) no longer match.
	sessionPath := writeCacheFixture(t, `{"type":"user","uuid":"u1","timestamp":"2026-04-01T10:00:00Z","message":{"content":"first"}}`+"\n")
	diskDir := t.TempDir()

	b := &realParseBackend{homes: []string{filepath.Dir(sessionPath)}}
	m := NewMultiWithDiskCache(diskDir, b)
	if _, err := m.ParseSession(sessionPath); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&b.parseCount); got != 1 {
		t.Fatalf("first parse count = %d, want 1", got)
	}

	// Rewrite with different content + bump mtime
	if err := os.WriteFile(sessionPath, []byte(`{"type":"user","uuid":"u2","timestamp":"2026-04-01T10:00:01Z","message":{"content":"second"}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(sessionPath, future, future); err != nil {
		t.Fatal(err)
	}

	// Drop in-memory cache so we fall through to disk (which should ALSO miss on mtime drift)
	m.cache.clear()

	if _, err := m.ParseSession(sessionPath); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&b.parseCount); got != 2 {
		t.Errorf("parse count after edit = %d, want 2 (disk must invalidate)", got)
	}
}
