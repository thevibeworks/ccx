package insight

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestAnalyzeBuildsScopedSummary(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, loc)
	sessions := []*parser.Session{
		{
			ID:          "s-new",
			Provider:    "codex",
			ProjectName: "repo",
			Summary:     "Fixed failing trace tests and completed installer review",
			StartTime:   now.Add(-2 * time.Hour),
			EndTime:     now.Add(-90 * time.Minute),
			Model:       "gpt-5.4",
			Stats: parser.SessionStats{
				MessageCount: 10,
				UserPrompts:  3,
				ToolCalls:    8,
				InputTokens:  1000,
				OutputTokens: 500,
				CostUSD:      0.25,
			},
		},
		{
			ID:          "s-open",
			Provider:    "claude-code",
			ProjectName: "repo",
			Summary:     "WIP needs follow-up on web insight page",
			StartTime:   now.Add(-time.Hour),
			EndTime:     now.Add(-30 * time.Minute),
			Stats:       parser.SessionStats{MessageCount: 6, ToolCalls: 7},
		},
		{
			ID:          "old",
			ProjectName: "repo",
			Summary:     "outside scope",
			StartTime:   now.AddDate(0, 0, -2),
			EndTime:     now.AddDate(0, 0, -2),
		},
	}

	result := Analyze(sessions, Options{Scope: ScopeToday, Location: loc, Now: now, Limit: 5})
	if result.Kind != Kind {
		t.Fatalf("kind = %q", result.Kind)
	}
	if result.Metrics.Sessions != 2 {
		t.Fatalf("sessions = %d, want 2", result.Metrics.Sessions)
	}
	if result.Metrics.Projects != 1 {
		t.Fatalf("projects = %d, want 1", result.Metrics.Projects)
	}
	if len(result.OpenLoops) == 0 {
		t.Fatal("expected open loop signal")
	}
	if len(result.Completed) == 0 || result.Completed[0].ID != "s-new" {
		t.Fatalf("completed = %+v", result.Completed)
	}
	if len(result.Patterns) == 0 {
		t.Fatal("expected pattern signals")
	}
}

func TestAnalyzeTruncatesLongNarrativeFields(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, loc)
	longSummary := strings.Repeat("needs follow-up on installer coverage ", 30)
	result := Analyze([]*parser.Session{
		{
			ID:          "long-session",
			Provider:    "codex",
			ProjectName: "repo",
			Summary:     longSummary,
			StartTime:   now.Add(-time.Hour),
			EndTime:     now.Add(-30 * time.Minute),
		},
	}, Options{Scope: ScopeToday, Location: loc, Now: now, Limit: 5})

	if len(result.Current) != 1 {
		t.Fatalf("current = %+v", result.Current)
	}
	if got := len([]rune(result.Current[0].Summary)); got > 503 {
		t.Fatalf("current summary length = %d, want <= 503", got)
	}
	if len(result.OpenLoops) != 1 {
		t.Fatalf("open loops = %+v", result.OpenLoops)
	}
	if got := len([]rune(result.OpenLoops[0].Summary)); got > 223 {
		t.Fatalf("open loop summary length = %d, want <= 223", got)
	}
}
