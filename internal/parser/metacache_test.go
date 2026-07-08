package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetMetaCache(t *testing.T, path string) {
	t.Helper()
	InitMetaCache(path)
	t.Cleanup(func() { InitMetaCache("") })
}

func TestMetaCacheLookupStoreRoundtrip(t *testing.T) {
	resetMetaCache(t, filepath.Join(t.TempDir(), "meta.gob"))

	mtime := time.Now()
	if _, hit := MetaLookup("/a.jsonl", mtime, 10); hit {
		t.Fatal("unexpected hit on empty cache")
	}

	MetaStore("/a.jsonl", mtime, 10, &Session{ID: "a", Summary: "hello"})

	got, hit := MetaLookup("/a.jsonl", mtime, 10)
	if !hit || got == nil || got.Summary != "hello" {
		t.Fatalf("expected cached session, got %+v hit=%v", got, hit)
	}

	// Returned session is a copy — caller mutation must not poison the cache.
	got.Summary = "mutated"
	again, _ := MetaLookup("/a.jsonl", mtime, 10)
	if again.Summary != "hello" {
		t.Fatalf("cache poisoned by caller mutation: %q", again.Summary)
	}
}

func TestMetaCacheInvalidatesOnMtimeOrSizeChange(t *testing.T) {
	resetMetaCache(t, filepath.Join(t.TempDir(), "meta.gob"))

	mtime := time.Now()
	MetaStore("/a.jsonl", mtime, 10, &Session{ID: "a"})

	if _, hit := MetaLookup("/a.jsonl", mtime.Add(time.Second), 10); hit {
		t.Fatal("expected miss on mtime change")
	}
	if _, hit := MetaLookup("/a.jsonl", mtime, 11); hit {
		t.Fatal("expected miss on size change")
	}
}

func TestMetaCacheNegativeEntry(t *testing.T) {
	resetMetaCache(t, filepath.Join(t.TempDir(), "meta.gob"))

	mtime := time.Now()
	MetaStore("/skip.jsonl", mtime, 5, nil)

	got, hit := MetaLookup("/skip.jsonl", mtime, 5)
	if !hit || got != nil {
		t.Fatalf("expected negative hit, got %+v hit=%v", got, hit)
	}
}

func TestMetaCachePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.gob")
	resetMetaCache(t, path)

	mtime := time.Now()
	MetaStore("/a.jsonl", mtime, 10, &Session{ID: "a", Summary: "persisted"})
	FlushMetaCache()

	// Simulate a new process.
	InitMetaCache(path)
	got, hit := MetaLookup("/a.jsonl", mtime, 10)
	if !hit || got == nil || got.Summary != "persisted" {
		t.Fatalf("expected persisted session after reload, got %+v hit=%v", got, hit)
	}
}

func TestMetaCacheDisabledPathStillWorksInMemory(t *testing.T) {
	resetMetaCache(t, "")

	mtime := time.Now()
	MetaStore("/a.jsonl", mtime, 10, &Session{ID: "a"})
	FlushMetaCache() // must not panic or write anywhere

	if _, hit := MetaLookup("/a.jsonl", mtime, 10); !hit {
		t.Fatal("expected in-memory hit with persistence disabled")
	}
}

// TestDiscoverProjectSessionsUsesMetaCache proves discovery serves
// unchanged files from the cache: after the first pass, the file
// content is replaced with same-size garbage and the original mtime
// restored — a second pass must still return the original metadata.
func TestDiscoverProjectSessionsUsesMetaCache(t *testing.T) {
	resetMetaCache(t, filepath.Join(t.TempDir(), "meta.gob"))

	projectDir := t.TempDir()
	sessionFile := filepath.Join(projectDir, "abc-123.jsonl")
	line := `{"type":"user","timestamp":"2026-01-02T03:04:05Z","message":{"role":"user","content":"real summary text"}}` + "\n"
	if err := os.WriteFile(sessionFile, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := DiscoverProjectSessions(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 session, got %d", len(first))
	}

	info, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	garbage := make([]byte, info.Size())
	for i := range garbage {
		garbage[i] = 'x'
	}
	if err := os.WriteFile(sessionFile, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(sessionFile, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	second, err := DiscoverProjectSessions(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Summary != first[0].Summary {
		t.Fatalf("expected cached metadata for unchanged mtime+size, got %+v", second)
	}

	// Now bump the mtime: discovery must re-parse and see the garbage
	// (which quick-parses to no listable session).
	later := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(sessionFile, later, later); err != nil {
		t.Fatal(err)
	}
	third, err := DiscoverProjectSessions(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 0 {
		t.Fatalf("expected re-parse after mtime change, got %+v", third)
	}
}
