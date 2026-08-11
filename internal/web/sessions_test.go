package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// setupMultiProjectDir builds two projects with one session each:
// -proj-alpha (newer, 2024-02-01) and -proj-beta (older, 2024-01-01).
func setupMultiProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(project, session, ts, text string) {
		projectDir := filepath.Join(dir, "projects", project)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatal(err)
		}
		content := `{"type":"user","timestamp":"` + ts + `","uuid":"u1","message":{"content":"` + text + `"}}
{"type":"assistant","timestamp":"` + ts + `","uuid":"a1","parentUuid":"u1","message":{"content":"ok"}}
`
		if err := os.WriteFile(filepath.Join(projectDir, session+".jsonl"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("-proj-alpha", "alpha-session-111", "2024-02-01T10:00:00Z", "alpha work")
	write("-proj-beta", "beta-session-222", "2024-01-01T10:00:00Z", "beta work")
	return dir
}

func TestHandleSessionsPage_ListsAcrossProjects(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"alpha-se", "beta-ses", "/session/-proj-alpha/", "/session/-proj-beta/"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// Newer session must render before the older one (sort=time default).
	if strings.Index(body, "alpha-se") > strings.Index(body, "beta-ses") {
		t.Error("alpha (newer) should render before beta (older)")
	}
}

func TestHandleSessionsPage_RejectsOtherPaths(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions/nope", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	if w.Code != 404 {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleSessionsPage_GroupByProject(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?group=project", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "sgroup-head") {
		t.Fatal("grouped view missing group headers")
	}
	// Project group headers link to the project page.
	if !strings.Contains(body, `href="/project/-proj-alpha"`) {
		t.Error("project group header should link to /project/-proj-alpha")
	}
	// Grouping by project drops the redundant project cell from rows.
	if strings.Contains(body, `class="srow-project"`) {
		t.Error("project-grouped rows should not repeat the project cell")
	}
}

func TestHandleSessionsPage_GroupByDay(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?group=day", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	body := w.Body.String()
	// Two sessions on different days: two group headers.
	if strings.Count(body, "sgroup-head\"") < 2 {
		t.Errorf("expected 2 day groups, body has %d sgroup-head", strings.Count(body, "sgroup-head\""))
	}
}

func TestHandleSessionsPage_ProjectFilter(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?project=-proj-alpha", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "/session/-proj-alpha/") {
		t.Error("filtered view missing alpha session")
	}
	if strings.Contains(body, "/session/-proj-beta/") {
		t.Error("project filter leaked beta session into results")
	}
}

func TestHandleSessionsPage_EmptyState(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?q=nomatchxyz", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	if !strings.Contains(w.Body.String(), "No sessions match") {
		t.Error("zero-result view missing empty state")
	}
}

func TestHandleSessionsPage_InvalidParamsFallBack(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions?sort=bogus&group=bogus&limit=-5", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (invalid params fall back to defaults)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/session/-proj-alpha/") {
		t.Error("fallback view missing sessions")
	}
}

func TestHandleAPISessionsGlobal_Envelope(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	handleAPISessions(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Sessions []struct {
			ID      string `json:"id"`
			Project string `json:"project"`
		} `json:"sessions"`
		Total int `json:"total"`
		Shown int `json:"shown"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 || resp.Shown != 2 || len(resp.Sessions) != 2 {
		t.Fatalf("total=%d shown=%d len=%d, want 2/2/2", resp.Total, resp.Shown, len(resp.Sessions))
	}
	if resp.Sessions[0].Project == "" {
		t.Error("global session response missing project encoded name")
	}
}

func TestHandleAPISessionsGlobal_LimitReportsTotal(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions?limit=1", nil)
	w := httptest.NewRecorder()
	handleAPISessions(w, req)

	var resp struct {
		Total int `json:"total"`
		Shown int `json:"shown"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 || resp.Shown != 1 {
		t.Fatalf("total=%d shown=%d, want total=2 shown=1", resp.Total, resp.Shown)
	}
}

func TestHandleAPISessions_PerProjectStillWorks(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/api/sessions/-proj-alpha", nil)
	w := httptest.NewRecorder()
	handleAPISessions(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), "[") {
		t.Error("per-project endpoint should keep its bare-array shape")
	}
	if strings.Contains(body, "beta-session") {
		t.Error("per-project endpoint leaked other project's session")
	}
}

func TestGroupSessions_ProviderModeAndAggregates(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	entries := []sessionEntry{
		{Session: &parser.Session{ID: "a", Provider: "claude-code", EndTime: now,
			Stats: parser.SessionStats{InputTokens: 100, OutputTokens: 20, MessageCount: 5}}},
		{Session: &parser.Session{ID: "b", Provider: "codex", EndTime: now.Add(-time.Hour),
			Stats: parser.SessionStats{InputTokens: 10, OutputTokens: 5, MessageCount: 2}}},
		{Session: &parser.Session{ID: "c", Provider: "claude-code", EndTime: now.Add(-2 * time.Hour),
			Stats: parser.SessionStats{InputTokens: 1, OutputTokens: 1, MessageCount: 1}}},
	}

	groups := groupSessions(entries, "provider")
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	// First-encounter order: claude-code seen first.
	if groups[0].Key != "claude-code" || groups[1].Key != "codex" {
		t.Fatalf("group order = [%s %s], want [claude-code codex]", groups[0].Key, groups[1].Key)
	}
	if groups[0].Label != "Claude Code" {
		t.Errorf("label = %q, want Claude Code", groups[0].Label)
	}
	if len(groups[0].Entries) != 2 || groups[0].Tokens != 122 {
		t.Errorf("claude group entries=%d tokens=%d, want 2/122", len(groups[0].Entries), groups[0].Tokens)
	}
	if !groups[0].Latest.Equal(now) {
		t.Errorf("claude group latest = %v, want %v", groups[0].Latest, now)
	}
}

func TestGroupSessions_ProviderFixedOrderBeatsRecency(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	// grok has the most recent session but must still sort last.
	entries := []sessionEntry{
		{Session: &parser.Session{ID: "g", Provider: "grok", EndTime: now}},
		{Session: &parser.Session{ID: "x", Provider: "codex", EndTime: now.Add(-time.Hour)}},
		{Session: &parser.Session{ID: "c", Provider: "claude-code", EndTime: now.Add(-2 * time.Hour)}},
	}

	groups := groupSessions(entries, "provider")
	got := []string{groups[0].Key, groups[1].Key, groups[2].Key}
	want := []string{"claude-code", "codex", "grok"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("provider group order = %v, want %v", got, want)
		}
	}
}

func TestMatchesSearch_SummaryOrIDPrefix(t *testing.T) {
	s := &parser.Session{ID: "019F6528-abcd", Summary: "Fix session lookup recovery"}
	cases := []struct {
		q    string
		want bool
	}{
		{"", true},
		{"lookup", true}, // summary substring
		{"LOOKUP", true}, // case-folded
		{"019f65", true}, // ID prefix, case-folded
		{"f6528", false}, // ID substring but not prefix
		{"unrelated", false},
	}
	for _, c := range cases {
		if got := matchesSearch(s, c.q); got != c.want {
			t.Errorf("matchesSearch(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestHandleSessionsPage_ChipsAppearOnlyWhenFiltered(t *testing.T) {
	dir := setupMultiProjectDir(t)
	setTestBackend(dir)

	req := httptest.NewRequest("GET", "/sessions", nil)
	w := httptest.NewRecorder()
	handleSessionsPage(w, req)
	if strings.Contains(w.Body.String(), `class="filter-chip"`) {
		t.Error("unfiltered page should render no chips")
	}

	req = httptest.NewRequest("GET", "/sessions?q=alpha&provider=claude-code", nil)
	w = httptest.NewRecorder()
	handleSessionsPage(w, req)
	body := w.Body.String()
	if !strings.Contains(body, `class="filter-chip"`) {
		t.Fatal("filtered page missing chip row")
	}
	if !strings.Contains(body, "q: <b>alpha</b>") {
		t.Error("missing q chip")
	}
	if !strings.Contains(body, "provider: <b>Claude Code</b>") {
		t.Error("missing provider chip with display name")
	}
	if !strings.Contains(body, `href="/sessions">Clear all</a>`) {
		t.Error("missing Clear all link")
	}
	// Removing the q chip keeps the provider filter.
	if !strings.Contains(body, "provider=claude-code") {
		t.Error("chip removal URL should preserve the other filter")
	}
}

func TestRenderSessionsFooter_Copy(t *testing.T) {
	unfiltered := sessionsQuery{Limit: defaultSessionsLimit, SortBy: "time"}
	filtered := sessionsQuery{Limit: defaultSessionsLimit, SortBy: "time", Search: "x"}

	truncated := renderSessionsFooter(unfiltered, sessionsView{Shown: 100, MatchTotal: 2314})
	for _, want := range []string{"Showing 100 of 2,314 sessions", "Show 500", "Show all 2,314 (slower)"} {
		if !strings.Contains(truncated, want) {
			t.Errorf("truncated footer missing %q in %q", want, truncated)
		}
	}
	if strings.Contains(truncated, "matching") {
		t.Error("unfiltered footer should not say matching")
	}

	complete := renderSessionsFooter(filtered, sessionsView{Shown: 42, MatchTotal: 42})
	if !strings.Contains(complete, "Showing all 42 matching sessions") {
		t.Errorf("complete footer = %q", complete)
	}
	if strings.Contains(complete, "Show all") || strings.Contains(complete, "Show 500") {
		t.Error("complete footer should have no widen links")
	}

	small := renderSessionsFooter(filtered, sessionsView{Shown: 100, MatchTotal: 312})
	if strings.Contains(small, "Show 500") {
		t.Error("Show 500 is pointless when only 312 match")
	}
	if !strings.Contains(small, "Show all 312 (slower)") {
		t.Errorf("small footer missing show-all: %q", small)
	}
}

func TestFormatCount(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 999: "999", 1000: "1,000", 2314: "2,314", 1234567: "1,234,567"}
	for n, want := range cases {
		if got := formatCount(n); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSessionsPageURL_OverridesAndClears(t *testing.T) {
	sq := sessionsQuery{
		Search:  "fix",
		GroupBy: "day",
		SortBy:  "tokens",
		Limit:   defaultSessionsLimit,
	}
	got := sessionsPageURL(sq, "limit", "0")
	for _, want := range []string{"q=fix", "group=day", "sort=tokens", "limit=0"} {
		if !strings.Contains(got, want) {
			t.Errorf("url %q missing %q", got, want)
		}
	}

	cleared := sessionsPageURL(sq, "group", "")
	if strings.Contains(cleared, "group=") {
		t.Errorf("url %q should have cleared group", cleared)
	}
}
