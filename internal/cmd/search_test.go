package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountMatchingLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	content := `{"type":"user","text":"tell me about Pi-Agent"}
{"type":"assistant","text":"nothing relevant"}
{"type":"assistant","text":"pi-agent uses ACP"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countMatchingLines(path, "pi-agent"); got != 2 {
		t.Fatalf("case-insensitive matches: got %d, want 2", got)
	}
	if got := countMatchingLines(path, "absent-term"); got != 0 {
		t.Fatalf("no-match count: got %d, want 0", got)
	}
	if got := countMatchingLines(filepath.Join(dir, "missing.jsonl"), "x"); got != 0 {
		t.Fatalf("missing file must count 0, got %d", got)
	}
}

// contentMatches must also cover subagent transcripts, which live in
// <id>/subagents/agent-*.jsonl beside the main <id>.jsonl.
func TestContentMatchesIncludesSubagents(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	if err := os.WriteFile(main, []byte(`{"text":"goose in main"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := `{"text":"goose in sidechain"}
{"text":"more goose here"}
`
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(sub), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-jsonl files in subagents/ (meta.json) must be skipped.
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.meta.json"), []byte(`{"text":"goose meta"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := contentMatches(main, "goose"); got != 3 {
		t.Fatalf("main+subagent matches: got %d, want 3", got)
	}
	if got := contentMatches("", "goose"); got != 0 {
		t.Fatalf("empty path must count 0, got %d", got)
	}
}
