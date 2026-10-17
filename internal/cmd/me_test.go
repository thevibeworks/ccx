package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadPromptHistoryKeepsWorkspaceAndReportsSources(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "claude-history.jsonl")
	codex := filepath.Join(dir, "codex-history.jsonl")
	writeFile(t, claude, `{"display":"/rewind","project":"/w/src/deva","sessionId":"a1","timestamp":1790000000000}
not json at all
{"display":"","project":"/w/src/deva","timestamp":1790000001000}
{"display":"let's continue","project":"/w/src/deva","sessionId":"a1","timestamp":1790000060000}`)
	writeFile(t, codex, `{"session_id":"c1","ts":1790000100,"text":"review the diff"}`+"\n")

	loc := time.FixedZone("+08", 8*3600)
	files := map[string]string{"claude-code": claude, "codex": codex, "missing": filepath.Join(dir, "nope")}
	prompts, sources := readPromptHistory(files, "", loc)

	if len(prompts) != 3 {
		t.Fatalf("prompts = %d, want 3 (bad and empty lines dropped, last line without newline kept)", len(prompts))
	}
	if p := prompts[0]; p.Workspace != "/w/src/deva" || p.Provider != "claude-code" || p.SessionID != "a1" {
		t.Fatalf("claude prompt = %+v", p)
	}
	if p := prompts[2]; p.Workspace != "" || p.Provider != "codex" || !p.Time.Equal(time.Unix(1790000100, 0)) {
		t.Fatalf("codex prompt = %+v", p)
	}
	if len(sources) != 2 || sources[0].Prompts != 2 || sources[1].Prompts != 1 {
		t.Fatalf("sources = %+v", sources)
	}
	if sources[0].First == "" || sources[0].First == sources[0].Last {
		t.Fatalf("source window = %q..%q", sources[0].First, sources[0].Last)
	}

	only, sources := readPromptHistory(files, "codex", loc)
	if len(only) != 1 || len(sources) != 1 {
		t.Fatalf("provider filter: %d prompts, %d sources", len(only), len(sources))
	}

	// A provider that was never used is a source with nothing in it.
	none, sources := readPromptHistory(map[string]string{"codex": filepath.Join(dir, "absent.jsonl")}, "", loc)
	if len(none) != 0 || len(sources) != 1 || sources[0].Prompts != 0 {
		t.Fatalf("absent history: %d prompts, sources %+v", len(none), sources)
	}
}

func TestParseMeTerms(t *testing.T) {
	terms, err := parseMeTerms([]string{"think=ultrathink", `again=\bagain\b|still`})
	if err != nil || len(terms) != 2 {
		t.Fatalf("terms = %+v, err = %v", terms, err)
	}
	if !terms[0].Pattern.MatchString("be ULTRATHINK") {
		t.Errorf("terms match case-insensitively")
	}
	// The value may itself contain '=': only the first one splits.
	if eq, err := parseMeTerms([]string{"eq=a=b"}); err != nil || !eq[0].Pattern.MatchString("A=B") {
		t.Errorf("name=regex splits on the first '=': %+v, %v", eq, err)
	}
	for _, bad := range [][]string{{"noequals"}, {"=x"}, {"x="}, {"x=("}, {"x=a", "x=b"}} {
		if _, err := parseMeTerms(bad); err == nil {
			t.Errorf("parseMeTerms(%q) should fail", bad)
		}
	}
}

func TestLogWindowSinceAloneRunsUntilNow(t *testing.T) {
	defer func(scope, since, until string) { logScope, logSince, logUntil = scope, since, until }(logScope, logSince, logUntil)
	loc := time.FixedZone("+08", 8*3600)

	logScope, logSince, logUntil = "", "2026-01-01", ""
	before := time.Now()
	start, end, name, _, err := logWindow(loc)
	if err != nil {
		t.Fatalf("--since alone: %v", err)
	}
	if name != "custom" || !start.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, loc)) || end.Before(before) {
		t.Fatalf("window = %s..%s (%s)", start, end, name)
	}
	// The table prints the window end as-is: it must be in --tz, not
	// in the host's zone.
	if end.Location() != loc {
		t.Fatalf("window end is in %s, want the requested zone", end.Location())
	}

	logSince, logUntil = "", "2026-01-01"
	if _, _, _, _, err := logWindow(loc); err == nil {
		t.Fatalf("--until alone has no start and must fail")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
