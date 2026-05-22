package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/sessionlog"
)

func TestPrintLogTableUsesRequestedTimezoneAndProjectColumn(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	old := logLocation
	logLocation = loc
	t.Cleanup(func() { logLocation = old })

	bundle := &sessionlog.Bundle{
		Scope: sessionlog.ScopeSummary{
			Label:    "Yesterday",
			TimeZone: "+08:00",
			Start:    time.Date(2026, 5, 21, 0, 0, 0, 0, loc),
			End:      time.Date(2026, 5, 22, 0, 0, 0, 0, loc),
		},
		Metrics: sessionlog.Metrics{Sessions: 1, Records: 1, RecordsReturned: 1},
		Records: []sessionlog.Record{
			{
				Timestamp: time.Date(2026, 5, 21, 2, 3, 0, 0, time.UTC),
				Provider:  "codex",
				Kind:      "user_prompt",
				SessionID: "019e440f-long",
				Project:   "deploydock",
				Text:      "list accounts",
			},
		},
	}

	out := captureStdout(t, func() {
		if err := printLogTable(bundle); err != nil {
			t.Fatalf("printLogTable() error: %v", err)
		}
	})
	if !strings.Contains(out, "May 21 10:03") {
		t.Fatalf("timestamp not rendered in requested timezone:\n%s", out)
	}
	if !strings.Contains(out, "PROJECT") {
		t.Fatalf("table header should say PROJECT:\n%s", out)
	}
}
