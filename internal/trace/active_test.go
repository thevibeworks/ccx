package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// TestActiveTimeCapsGaps is the honest-duration contract from issue
// #19: a turn spanning an overnight gap must report active time near
// the worked intervals, not the wall span.
func TestActiveTimeCapsGaps(t *testing.T) {
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	session := &parser.Session{
		ID:        "active",
		StartTime: now,
		EndTime:   now.Add(10*time.Hour + 4*time.Minute),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "run the migration overnight"}}},
			// Two minutes of work...
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Starting."}}},
			// ...then a 10-hour overnight gap...
			{UUID: "a2", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(10*time.Hour + 2*time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Continuing after the run."}}},
			// ...then two more minutes.
			{UUID: "a3", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(10*time.Hour + 4*time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Done."}}},
		},
	}

	result := Analyze(session)
	turn := result.Turns[0]

	// Wall span is ~10h04m; active must be 2m + cap(5m) + 2m = 9m.
	wantActive := (2*time.Minute + activeGapCap + 2*time.Minute).Seconds()
	if turn.ActiveSecs != wantActive {
		t.Fatalf("turn active = %vs, want %vs", turn.ActiveSecs, wantActive)
	}
	if result.Stats.ActiveSecs != wantActive {
		t.Fatalf("stats active = %vs, want %vs", result.Stats.ActiveSecs, wantActive)
	}
	if result.Stats.DurationSecs <= result.Stats.ActiveSecs {
		t.Fatal("wall span must remain reported alongside active time")
	}

	outline := BuildOutline(result, DefaultHeadlineWidth)
	if outline.Turns[0].ActiveSecs != wantActive {
		t.Fatalf("outline turn active = %vs, want %vs", outline.Turns[0].ActiveSecs, wantActive)
	}
	// The turn badge must show active (9m), not the 604m wall gap.
	text := RenderOutlineText(outline)
	if !strings.Contains(text, "(9m") {
		t.Fatalf("turn badge must show active minutes, got:\n%s", text)
	}
	if strings.Contains(text, "604m") {
		t.Fatalf("turn badge must not show wall-gap minutes:\n%s", text)
	}
}

func TestRenderOutlineHeaderStatesActiveAndTimezone(t *testing.T) {
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	session := &parser.Session{
		ID:        "header",
		StartTime: now,
		EndTime:   now.Add(3 * time.Minute),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "quick fix"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Fixed."}}},
		},
	}

	text := RenderOutlineText(BuildOutline(Analyze(session), DefaultHeadlineWidth))
	if !strings.Contains(text, "| active 3m |") {
		t.Fatalf("header must state active time, got:\n%s", text)
	}
	if !strings.Contains(text, "(times UTC") {
		t.Fatalf("header must state the rendered timezone, got:\n%s", text)
	}
}

func TestOutlineZoneFormats(t *testing.T) {
	if got := formatActive(45); got != "45s" {
		t.Fatalf("formatActive(45) = %q", got)
	}
	if got := formatActive(12 * 60); got != "12m" {
		t.Fatalf("formatActive(12m) = %q", got)
	}
	if got := formatActive(4*3600 + 7*60); got != "4h07m" {
		t.Fatalf("formatActive(4h07m) = %q", got)
	}
}
