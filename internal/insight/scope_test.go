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

func TestScopeWindowYesterdayWithFixedOffset(t *testing.T) {
	loc, err := LoadLocation("+8")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 22, 2, 0, 0, 0, time.UTC) // 10:00 +08

	start, end, label := ScopeWindow(ScopeYesterday, now, loc)
	if label != "Yesterday" {
		t.Fatalf("label = %q, want Yesterday", label)
	}
	if got := start.Format(time.RFC3339); got != "2026-05-21T00:00:00+08:00" {
		t.Fatalf("start = %s", got)
	}
	if got := end.Format(time.RFC3339); got != "2026-05-22T00:00:00+08:00" {
		t.Fatalf("end = %s", got)
	}
}

func TestLoadLocationParsesOffsetForms(t *testing.T) {
	for _, raw := range []string{"+8", "+08", "+0800", "+08:00"} {
		loc, err := LoadLocation(raw)
		if err != nil {
			t.Fatalf("LoadLocation(%q) error: %v", raw, err)
		}
		_, offset := time.Date(2026, 1, 1, 0, 0, 0, 0, loc).Zone()
		if offset != 8*60*60 {
			t.Fatalf("LoadLocation(%q) offset = %d, want +8h", raw, offset)
		}
	}
}
