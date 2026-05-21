package cmd

import (
	"bytes"
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
