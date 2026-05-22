package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestPrintSessionsTableKeepsEachSessionOnOneLine(t *testing.T) {
	sessions := []*parser.Session{
		{
			ID:        "019e48cb-long-id",
			Provider:  "codex",
			Summary:   "initiatives & request: ```\n\n› we've built and released our ccx project\n```",
			StartTime: time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        "019e48dc-long-id",
			Provider:  "codex",
			Summary:   "   Second review pass for the scoped-session refac...\n  with details",
			StartTime: time.Date(2026, 5, 21, 10, 1, 0, 0, time.UTC),
		},
	}

	out := captureStdout(t, func() {
		if err := printSessionsTable(sessions, false); err != nil {
			t.Fatalf("printSessionsTable() error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want header + 2 sessions; output:\n%s", len(lines), out)
	}
	if strings.Contains(out, "\n\n") {
		t.Fatalf("table contains blank line from raw summary: %q", out)
	}
	if !strings.Contains(lines[1], "initiatives & request: ``` › we've built") {
		t.Fatalf("first row summary not normalized: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Second review pass for the scoped-session") {
		t.Fatalf("second row summary not normalized: %q", lines[2])
	}
}

func TestPrintSessionsJSONIncludesWorkspaceEndAndMetrics(t *testing.T) {
	start := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	sessions := []*parser.Session{
		{
			ID:          "session-1",
			Provider:    "codex",
			ProjectName: "repo",
			CWD:         "/tmp/repo",
			FilePath:    "/tmp/session.jsonl",
			Summary:     "worked on scoped sessions",
			StartTime:   start,
			EndTime:     end,
			Model:       "gpt-5.4",
			Stats: parser.SessionStats{
				MessageCount:      12,
				ToolCalls:         7,
				AgentSidechains:   2,
				InputTokens:       100,
				OutputTokens:      50,
				CacheReadTokens:   25,
				CacheCreateTokens: 5,
				CostUSD:           0.12,
			},
		},
	}

	out := captureStdout(t, func() {
		if err := printSessionsJSON(sessions); err != nil {
			t.Fatalf("printSessionsJSON() error: %v", err)
		}
	})

	var items []sessionJSON
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	item := items[0]
	if item.Workspace != "/tmp/repo" || item.EndTime == "" || item.FilePath == "" {
		t.Fatalf("missing rich session fields: %+v", item)
	}
	if item.Tokens != 180 || item.Messages != 12 || item.ToolCalls != 7 || item.Sidechains != 2 {
		t.Fatalf("metrics = %+v", item)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
