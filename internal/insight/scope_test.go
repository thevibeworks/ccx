package insight

import (
	"testing"
	"time"
)

func TestScopeWindowUsesProvidedTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 22, 7, 30, 0, 0, time.UTC) // 00:30 PDT

	start, end, label := ScopeWindow(ScopeToday, now, loc)
	if label != "Today" {
		t.Fatalf("label = %q, want Today", label)
	}
	if got := start.Format(time.RFC3339); got != "2026-05-22T00:00:00-07:00" {
		t.Fatalf("start = %s", got)
	}
	if got := end.Format(time.RFC3339); got != "2026-05-23T00:00:00-07:00" {
		t.Fatalf("end = %s", got)
	}
}

func TestScopeWindowQuarter(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, loc)
	start, end, label := ScopeWindow(ScopeQuarter, now, loc)
	if label != "Q2 2026" {
		t.Fatalf("label = %q", label)
	}
	if start.Month() != time.April || end.Month() != time.July {
		t.Fatalf("quarter window = %s to %s", start, end)
	}
}
