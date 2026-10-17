package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/me"
)

func TestTranscriptCoverageCountsWhatFellOutOfTheStore(t *testing.T) {
	jan := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	may := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	var prompts []me.Prompt
	add := func(n int, at time.Time, provider, session string) {
		for i := 0; i < n; i++ {
			prompts = append(prompts, me.Prompt{Time: at, Provider: provider, SessionID: session, Text: "x"})
		}
	}
	add(30, jan, "claude-code", "GONE-1") // ids compare case-insensitively
	add(10, jan, "claude-code", "kept-1")
	add(5, jan, "claude-code", "") // older history rows name no session
	add(40, may, "claude-code", "kept-2")
	add(1, may, "claude-code", "never-written") // a lone /model: under the floor
	add(25, jan, "codex", "cx-kept")

	rows := transcriptCoverage(prompts, map[string]bool{"kept-1": true, "kept-2": true, "cx-kept": true}, time.UTC)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	janRow := rows[0]
	want := coverageRow{Provider: "claude-code", Month: "2026-01", Prompts: 40, PromptsOnDisk: 10, Sessions: 2, SessionsOnDisk: 1, Unattributed: 5}
	if janRow != want {
		t.Fatalf("january = %+v, want %+v", janRow, want)
	}
	if !janRow.gap() {
		t.Errorf("30 of 40 prompts without a transcript is a gap")
	}
	if mayRow := rows[1]; mayRow.gap() || mayRow.Prompts != 41 || mayRow.PromptsOnDisk != 40 {
		t.Errorf("one command-only session must not flag the month: %+v", mayRow)
	}
	if cx := rows[2]; cx.Provider != "codex" || cx.gap() {
		t.Errorf("codex = %+v", cx)
	}
	if small := (coverageRow{Prompts: 5, PromptsOnDisk: 0}); small.gap() {
		t.Errorf("a month with a handful of prompts is noise, not a gap")
	}
}

func TestTranscriptIDsFromBothHomes(t *testing.T) {
	root := t.TempDir()
	claude := filepath.Join(root, "claude")
	codex := filepath.Join(root, "codex")
	for path, body := range map[string]string{
		filepath.Join(claude, "projects", "-w-a", "AAAA1111-0000-4000-8000-000000000001.jsonl"):                                        "{}",
		filepath.Join(claude, "projects", "-w-a", "gone-parent", "subagents", "agent-x.jsonl"):                                         "{}",
		filepath.Join(codex, "sessions", "2026", "01", "02", "rollout-2026-01-02T03-04-05-019ba178-0e7d-7380-a58c-1d64d653c655.jsonl"): "{}",
		filepath.Join(codex, "archived_sessions", "rollout-2026-08-30T23-16-51-01a05676-a62e-7311-8bf9-db685e3cef67.jsonl"):            "{}",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, body)
	}
	ids := transcriptIDs(claude, codex)
	for _, id := range []string{
		"aaaa1111-0000-4000-8000-000000000001",
		"019ba178-0e7d-7380-a58c-1d64d653c655",
		"01a05676-a62e-7311-8bf9-db685e3cef67", // archived is still on disk
	} {
		if !ids[id] {
			t.Errorf("missing id %s in %v", id, ids)
		}
	}
	// A surviving subagent file is not the parent's transcript.
	if ids["gone-parent"] || len(ids) != 3 {
		t.Errorf("ids = %v", ids)
	}
	if got := transcriptIDs(filepath.Join(root, "absent"), filepath.Join(root, "absent")); len(got) != 0 {
		t.Errorf("absent homes hold no transcripts: %v", got)
	}
}

func TestClaudeRetention(t *testing.T) {
	home := t.TempDir()
	if days, set, err := claudeRetention(home); err != nil || set || days != 30 {
		t.Fatalf("no settings file: days=%d set=%v err=%v, want the 30-day default", days, set, err)
	}
	settings := filepath.Join(home, "settings.json")
	writeFile(t, settings, `{"model":"opus"}`)
	if days, set, _ := claudeRetention(home); set || days != 30 {
		t.Fatalf("key absent: days=%d set=%v", days, set)
	}
	writeFile(t, settings, `{"cleanupPeriodDays": 3650}`)
	if days, set, _ := claudeRetention(home); !set || days != 3650 {
		t.Fatalf("days=%d set=%v, want 3650 set", days, set)
	}
	writeFile(t, settings, `{"cleanupPeriodDays": 0}`)
	if days, set, _ := claudeRetention(home); !set || days != 0 {
		t.Fatalf("an explicit 0 is set, and means no transcripts: days=%d set=%v", days, set)
	}
	writeFile(t, settings, `{not json`)
	if _, _, err := claudeRetention(home); err == nil {
		t.Fatalf("unreadable settings must not read as the default")
	}
}
