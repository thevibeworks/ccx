package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
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

// countContentMatches must also cover subagent transcripts, which live
// in <id>/subagents/agent-*.jsonl beside the main <id>.jsonl — and
// must count exactly the files view --show-agents renders.
func TestCountContentMatchesIncludesSubagents(t *testing.T) {
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
	// Non-jsonl files (meta.json) and jsonl without the agent- prefix
	// must be skipped, matching what the session parser loads.
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.meta.json"), []byte(`{"text":"goose meta"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "notes.jsonl"), []byte(`{"text":"goose notes"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := countContentMatches(main, "goose"); got != 3 {
		t.Fatalf("main+subagent matches: got %d, want 3", got)
	}
	if got := countContentMatches("", "goose"); got != 0 {
		t.Fatalf("empty path must count 0, got %d", got)
	}
}

type stubSessionParser struct{}

func (stubSessionParser) ParseSession(path string) (*parser.Session, error) {
	return parser.ParseSession(path)
}

// scanConversationText must count only what a human reads in the
// conversation. Hook attachments (isMeta) and tool-result lines are
// exactly the boilerplate that once outranked the real discussion
// 327 hits to 13 — they must contribute zero.
func TestScanConversationTextSignalOnly(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	lines := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2026-08-03T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"let's design the deadman auto-handoff timer"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-08-03T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"deadman fires after idle; the deadman then writes the handoff"}]}}`,
		`{"type":"user","uuid":"m1","parentUuid":"a1","isMeta":true,"timestamp":"2026-08-03T00:00:02Z","message":{"role":"user","content":[{"type":"text","text":"deadman armed: auto-handoff in 50m"}]}}`,
		`{"type":"user","uuid":"t1","parentUuid":"m1","timestamp":"2026-08-03T00:00:03Z","message":{"role":"user","content":[{"type":"tool_result","content":"deadman armed: auto-handoff stdout twin"}]}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"t1","timestamp":"2026-08-03T00:00:04Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"the deadman design needs a disarm path"}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(main, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	side := `{"type":"assistant","uuid":"s1","isSidechain":true,"timestamp":"2026-08-03T00:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"deadman in sidechain"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}

	// 6 raw lines match; only 5 occurrences live in conversation text
	// (1 user + 2 assistant + 1 thinking + 1 sidechain).
	if raw := countContentMatches(main, "deadman"); raw != 6 {
		t.Fatalf("raw line matches: got %d, want 6", raw)
	}
	n, previews := scanConversationText(stubSessionParser{}, main, "deadman")
	if n != 5 {
		t.Fatalf("signal matches: got %d, want 5 (noise counted?)", n)
	}
	if len(previews) != maxContentPreviews {
		t.Fatalf("previews: got %d, want %d", len(previews), maxContentPreviews)
	}
	if previews[0].Role != "user" || !strings.Contains(previews[0].Text, "deadman") {
		t.Fatalf("first preview should be the user prompt, got [%s] %q", previews[0].Role, previews[0].Text)
	}
	if previews[1].Role != "assistant" {
		t.Fatalf("second preview role: got %q, want assistant", previews[1].Role)
	}

	if n, _ := scanConversationText(stubSessionParser{}, main, "auto-handoff"); n != 1 {
		t.Fatalf("auto-handoff signal matches: got %d, want 1 (hook noise counted?)", n)
	}
}

func TestScanConversationTextSidechainRole(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "abc-123.jsonl")
	line := `{"type":"user","uuid":"u1","timestamp":"2026-08-03T00:00:00Z","message":{"role":"user","content":[{"type":"text","text":"kick off"}]}}` + "\n"
	if err := os.WriteFile(main, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "abc-123", "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	side := `{"type":"assistant","uuid":"s1","isSidechain":true,"timestamp":"2026-08-03T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"goose only lives here"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(subDir, "agent-1.jsonl"), []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}

	n, previews := scanConversationText(stubSessionParser{}, main, "goose")
	if n != 1 || len(previews) != 1 {
		t.Fatalf("sidechain match: got n=%d previews=%d, want 1/1", n, len(previews))
	}
	if previews[0].Role != "agent" {
		t.Fatalf("sidechain preview role: got %q, want agent", previews[0].Role)
	}
}

func TestMatchSnippet(t *testing.T) {
	long := strings.Repeat("a", 100) + " needle " + strings.Repeat("b", 100)
	got := matchSnippet(long, 101, len("needle"))
	if !strings.Contains(got, "needle") {
		t.Fatalf("snippet must contain the match, got %q", got)
	}
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "...") {
		t.Fatalf("mid-text snippet should be marked truncated on both ends, got %q", got)
	}
	if got := matchSnippet("short text", 0, 5); got != "short text" {
		t.Fatalf("untruncated snippet: got %q", got)
	}
	// Out-of-range indexes must clamp, not panic.
	_ = matchSnippet("tiny", 999, 4)
}

func TestRawPrefilterSafe(t *testing.T) {
	cases := map[string]bool{
		"deadman":    true,
		"fix bug":    true,
		`say "hi"`:   false,
		`back\slash`: false,
		"路径":         false,
		"tab\there":  false,
	}
	for q, want := range cases {
		if got := rawPrefilterSafe(q); got != want {
			t.Errorf("rawPrefilterSafe(%q) = %v, want %v", q, got, want)
		}
	}
}

// Grep parity must survive lines larger than any fixed scanner budget
// (transcript lines with embedded images run past 10MB).
func TestCountMatchingLinesOversizedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")
	huge := `{"pad":"` + strings.Repeat("x", 11*1024*1024) + `"}`
	content := huge + "\n" + `{"text":"needle after the giant line"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countMatchingLines(path, "needle"); got != 1 {
		t.Fatalf("match after oversized line: got %d, want 1", got)
	}
}
