package catalog

import (
	"errors"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

func TestApplySessionQueryScopesToWorkspace(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	projects := []*parser.Project{
		{
			Name: "repo",
			Path: "/tmp/repo",
			Sessions: []*parser.Session{
				{ID: "current-cx", Provider: "codex", CWD: "/tmp/repo", EndTime: now},
				{ID: "current-cc", Provider: "claude-code", CWD: "/tmp/repo", EndTime: now.Add(-time.Minute)},
			},
		},
		{
			Name: "other",
			Path: "/tmp/other",
			Sessions: []*parser.Session{
				{ID: "other", Provider: "codex", CWD: "/tmp/other", EndTime: now.Add(time.Minute)},
			},
		},
	}

	got := ApplySessionQuery(projects, SessionQuery{
		Scope:         ScopeWorkspace,
		WorkspacePath: "/tmp/repo",
		Sort:          SortTime,
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "current-cx" || got[1].ID != "current-cc" {
		t.Fatalf("got IDs %q, %q; want current-cx, current-cc", got[0].ID, got[1].ID)
	}
}

func TestApplySessionQueryAllPreservesGlobalCandidateSet(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	projects := []*parser.Project{
		{Name: "repo", Sessions: []*parser.Session{{ID: "repo", EndTime: now}}},
		{Name: "other", Sessions: []*parser.Session{{ID: "other", EndTime: now.Add(time.Minute)}}},
	}

	got := ApplySessionQuery(projects, SessionQuery{Scope: ScopeAll, Sort: SortTime})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "other" || got[1].ID != "repo" {
		t.Fatalf("global ordering = [%s %s], want [other repo]", got[0].ID, got[1].ID)
	}
}

func TestApplySessionQueryProjectScopeIsExplicit(t *testing.T) {
	projects := []*parser.Project{
		{Name: "repo", EncodedName: "tmp-repo", Path: "/tmp/repo", Sessions: []*parser.Session{{ID: "repo"}}},
		{Name: "other", EncodedName: "tmp-other", Path: "/tmp/other", Sessions: []*parser.Session{{ID: "other"}}},
	}

	got := ApplySessionQuery(projects, SessionQuery{
		Scope:       ScopeProject,
		ProjectName: "tmp-other",
	})
	if len(got) != 1 || got[0].ID != "other" {
		t.Fatalf("got %+v, want only other project session", got)
	}
}

func TestApplySessionQueryFiltersSortsAndLimits(t *testing.T) {
	base := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	projects := []*parser.Project{{
		Name: "repo",
		Sessions: []*parser.Session{
			{ID: "cc-big", Provider: "claude-code", Summary: "auth refactor", Model: "sonnet", StartTime: base, EndTime: base.Add(time.Minute), Stats: parser.SessionStats{MessageCount: 20}},
			{ID: "cx-small", Provider: "codex", Summary: "auth refactor", Model: "gpt-5", StartTime: base, EndTime: base.Add(2 * time.Minute), Stats: parser.SessionStats{MessageCount: 4}},
			{ID: "cx-big", Provider: "codex", Summary: "billing", Model: "gpt-5", StartTime: base, EndTime: base.Add(3 * time.Minute), Stats: parser.SessionStats{MessageCount: 40}},
		},
	}}

	got := ApplySessionQuery(projects, SessionQuery{
		Scope: ScopeAll,
		Filter: config.SessionFilter{
			Provider: "codex",
			Model:    "gpt",
		},
		Sort:  SortMessages,
		Limit: 1,
	})
	if len(got) != 1 || got[0].ID != "cx-big" {
		t.Fatalf("got %+v, want cx-big", got)
	}
}

func TestFindSessionByIDReportsAmbiguousPrefix(t *testing.T) {
	sessions := []*parser.Session{
		{ID: "abcdef-one"},
		{ID: "abcdef-two"},
	}

	got, err := FindSessionByID(sessions, "abcdef")
	if got != nil {
		t.Fatalf("session = %+v, want nil on ambiguity", got)
	}
	var ambiguous *AmbiguousSessionError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want AmbiguousSessionError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(ambiguous.Matches))
	}
}

func TestFindSessionByIDPrefersExactBeforePrefixAmbiguity(t *testing.T) {
	sessions := []*parser.Session{
		{ID: "abc"},
		{ID: "abcdef"},
	}

	got, err := FindSessionByID(sessions, "abc")
	if err != nil {
		t.Fatalf("FindSessionByID error: %v", err)
	}
	if got == nil || got.ID != "abc" {
		t.Fatalf("got %+v, want exact abc", got)
	}
}

func TestValidateSessionSortRejectsUnknownSort(t *testing.T) {
	if err := ValidateSessionSort("bogus"); err == nil {
		t.Fatal("ValidateSessionSort(bogus) returned nil, want error")
	}
}

func TestSortSessionsTokensRanksByInputPlusOutput(t *testing.T) {
	sessions := []*parser.Session{
		{ID: "small", Stats: parser.SessionStats{InputTokens: 100, OutputTokens: 50}},
		{ID: "big", Stats: parser.SessionStats{InputTokens: 5000, OutputTokens: 2000}},
		{ID: "cache-heavy", Stats: parser.SessionStats{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 999999}},
	}

	SortSessions(sessions, SortTokens)

	if sessions[0].ID != "big" || sessions[1].ID != "small" || sessions[2].ID != "cache-heavy" {
		t.Fatalf("token sort = [%s %s %s], want [big small cache-heavy]",
			sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}
	if err := ValidateSessionSort(SortTokens); err != nil {
		t.Fatalf("ValidateSessionSort(tokens) = %v, want nil", err)
	}
}
