package launches

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeReceipts(t *testing.T, lines string) *Index {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "2026-07-27.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return LoadDir(dir)
}

func TestGoalForMatch(t *testing.T) {
	idx := writeReceipts(t, `{"ts":"2026-07-27T10:00:00Z","goal":"auth-fix","cwd":"/w/app"}
`)
	start := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", start); got != "auth-fix" {
		t.Fatalf("got %q, want auth-fix", got)
	}
}

func TestGoalForLatestWins(t *testing.T) {
	idx := writeReceipts(t, `{"ts":"2026-07-27T08:00:00Z","goal":"old-goal","cwd":"/w/app"}
{"ts":"2026-07-27T10:00:00Z","goal":"new-goal","cwd":"/w/app"}
`)
	start := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", start); got != "new-goal" {
		t.Fatalf("got %q, want new-goal", got)
	}
}

func TestGoalForExpiry(t *testing.T) {
	idx := writeReceipts(t, `{"ts":"2026-07-25T10:00:00Z","goal":"stale","cwd":"/w/app"}
`)
	start := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", start); got != "" {
		t.Fatalf("got %q, want empty (24h expiry)", got)
	}
}

func TestGoalForStartGrace(t *testing.T) {
	idx := writeReceipts(t, `{"ts":"2026-07-27T10:02:00Z","goal":"grace","cwd":"/w/app"}
`)
	inside := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", inside); got != "grace" {
		t.Fatalf("got %q, want grace (receipt 2m after start is within grace)", got)
	}
	outside := time.Date(2026, 7, 27, 9, 50, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", outside); got != "" {
		t.Fatalf("got %q, want empty (receipt 12m after start)", got)
	}
}

func TestGoalForWrongCWD(t *testing.T) {
	idx := writeReceipts(t, `{"ts":"2026-07-27T10:00:00Z","goal":"auth-fix","cwd":"/w/app"}
`)
	start := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/other", start); got != "" {
		t.Fatalf("got %q, want empty (different cwd)", got)
	}
}

func TestLoadSkipsMalformed(t *testing.T) {
	idx := writeReceipts(t, `not json at all
{"ts":"2026-07-27T10:00:00Z","cwd":"/w/app"}
{"ts":"2026-07-27T10:00:00Z","goal":"ok","cwd":"/w/app"}
`)
	start := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if got := idx.GoalFor("/w/app", start); got != "ok" {
		t.Fatalf("got %q, want ok (bad lines skipped)", got)
	}
}

func TestLoadMissingDir(t *testing.T) {
	idx := LoadDir(filepath.Join(t.TempDir(), "nope"))
	if !idx.Empty() {
		t.Fatal("missing dir must yield empty index")
	}
}
