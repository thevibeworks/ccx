package insight

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/sessionlog"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// fixtureBundle is one Claude session with a subagent file, plus one
// Codex session, inside a one-day window.
func fixtureBundle(t *testing.T) *sessionlog.Bundle {
	t.Helper()
	start := mustTime(t, "2026-08-20T00:00:00Z")
	end := mustTime(t, "2026-08-21T00:00:00Z")
	main := "/home/u/.claude/projects/-tmp-ws/aaaa1111.jsonl"
	agent := "/home/u/.claude/projects/-tmp-ws/aaaa1111/subagents/agent-x.jsonl"
	codex := "/home/u/.codex/sessions/2026/08/20/rollout.jsonl"
	rec := func(ts, file, id, ws, kind, text string, line int) sessionlog.Record {
		provider := "claude-code"
		if strings.Contains(file, ".codex") {
			provider = "codex"
		}
		return sessionlog.Record{
			Timestamp: mustTime(t, ts), Provider: provider, SessionID: id, Workspace: ws, Project: "ws",
			SourceFile: file, Line: line, Kind: kind, Text: text,
			IsSidechain: strings.Contains(file, "/subagents/"), IsSubagent: strings.Contains(file, "/subagents/"),
		}
	}
	records := []sessionlog.Record{
		rec("2026-08-20T09:00:00Z", main, "aaaa1111", "/tmp/ws", "user_prompt", "build the digest", 1),
		rec("2026-08-20T09:01:00Z", main, "aaaa1111", "/tmp/ws", "tool_call", "Edit: /tmp/ws/internal/insight/digest.go", 2),
		rec("2026-08-20T09:02:00Z", main, "aaaa1111", "/tmp/ws", "tool_call", "Bash: go test ./... && git commit -m digest", 3),
		rec("2026-08-20T09:03:00Z", agent, "aaaa1111", "/tmp/ws", "user_prompt", "You are a subagent: review the diff", 1),
		rec("2026-08-20T09:04:00Z", main, "aaaa1111", "/tmp/ws", "interrupt", "[Request interrupted by user]", 4),
		rec("2026-08-20T09:05:00Z", main, "aaaa1111", "/tmp/ws", "user_prompt", "wait, tests first", 5),
		rec("2026-08-20T09:06:00Z", main, "aaaa1111", "/tmp/ws", "assistant_message", "Done: digest built and tested.", 6),
		rec("2026-08-20T10:00:00Z", codex, "019f0000", "/tmp/ws", "user_prompt", "codex: fix the flake", 1),
		rec("2026-08-20T10:01:00Z", codex, "019f0000", "/tmp/ws", "tool_call", "apply_patch: internal/x.go", 2),
	}
	records[1].Tool, records[1].Path = "Edit", "/tmp/ws/internal/insight/digest.go"
	records[2].Tool = "Bash"
	records[8].Tool, records[8].Path = "apply_patch", "internal/x.go"
	sessions := []sessionlog.SessionSlice{
		{ID: "aaaa1111", Provider: "claude-code", Project: "ws", Workspace: "/tmp/ws", SourceFile: main, Start: records[0].Timestamp, End: records[6].Timestamp, FirstRecord: records[0].Timestamp, LastRecord: records[6].Timestamp, Records: 6, Relation: sessionlog.ScopeRelation{StartedInScope: true, EndedInScope: true, OverlapsScope: true}},
		{ID: "aaaa1111", Provider: "claude-code", Project: "ws", Workspace: "/tmp/ws", SourceFile: agent, Start: records[3].Timestamp, End: records[3].Timestamp, FirstRecord: records[3].Timestamp, LastRecord: records[3].Timestamp, Records: 1, Relation: sessionlog.ScopeRelation{StartedInScope: true, EndedInScope: true, OverlapsScope: true}},
		{ID: "019f0000", Provider: "codex", Project: "ws", Workspace: "/tmp/ws", SourceFile: codex, Start: records[7].Timestamp, End: records[8].Timestamp, FirstRecord: records[7].Timestamp, LastRecord: records[8].Timestamp, Records: 2, Relation: sessionlog.ScopeRelation{StartedBeforeScope: true, EndedInScope: true, OverlapsScope: true}},
	}
	return &sessionlog.Bundle{
		Kind:        sessionlog.Kind,
		Scope:       sessionlog.ScopeSummary{Name: "custom", Label: "Aug 20", TimeZone: "UTC", Start: start, End: end},
		GeneratedAt: end,
		Metrics:     sessionlog.Metrics{Sessions: 3, SourceFiles: 3, Records: 9, RecordsReturned: 9, AssistantMessages: 1, ToolCalls: 3, Sidechains: 1},
		Sessions:    sessions,
		Records:     records,
	}
}

func fixtureLookup(id string) *parser.Session {
	switch id {
	case "aaaa1111":
		return &parser.Session{ID: id, Model: "claude-opus-5", Summary: "build the digest", CWD: "/tmp/ws",
			Stats: parser.SessionStats{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 8500, CostUSD: 1.25}}
	case "019f0000":
		return &parser.Session{ID: id, Model: "gpt-9-unknown", CWD: "/tmp/ws",
			Stats: parser.SessionStats{InputTokens: 3000, OutputTokens: 2000, UnpricedTokens: 5000}}
	}
	return nil
}

func TestBuildReportFoldsSubagentFilesIntoOneSessionAndKeepsHumanPromptsOnly(t *testing.T) {
	report := BuildReport(fixtureBundle(t), fixtureLookup, false, 0)
	if report.Kind != Kind {
		t.Fatalf("kind = %q", report.Kind)
	}
	if len(report.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2 (subagent file folded into its session)", len(report.Sessions))
	}
	claude := report.Sessions[0]
	if claude.ID != "aaaa1111" || claude.SourceFiles != 2 || claude.Records != 7 || claude.Sidechains != 1 {
		t.Fatalf("claude digest = %+v", claude)
	}
	if claude.UserPrompts != 2 || len(claude.Prompts) != 2 || claude.Prompts[0].Text != "build the digest" || claude.Prompts[1].Line != 5 {
		t.Fatalf("prompts = %d %+v (subagent instruction must not count as a human prompt)", claude.UserPrompts, claude.Prompts)
	}
	if claude.FinalAnswer == nil || claude.FinalAnswer.Text != "Done: digest built and tested." || claude.FinalAnswer.Line != 6 {
		t.Fatalf("final answer = %+v", claude.FinalAnswer)
	}
	if claude.Edits != 1 || claude.Commits != 1 || claude.Interrupts != 1 || claude.ToolCalls != 2 {
		t.Fatalf("edits/commits/interrupts/tools = %d/%d/%d/%d", claude.Edits, claude.Commits, claude.Interrupts, claude.ToolCalls)
	}
	if len(claude.EditedPaths) != 1 || claude.EditedPaths[0] != "/tmp/ws/internal/insight/digest.go" {
		t.Fatalf("edited paths = %v", claude.EditedPaths)
	}
	if claude.Model != "claude-opus-5" || claude.Tokens != 10000 || claude.CostUSD != 1.25 || claude.CostStatus != "priced" {
		t.Fatalf("accounting = model %q tokens %d cost %v status %q", claude.Model, claude.Tokens, claude.CostUSD, claude.CostStatus)
	}
	codex := report.Sessions[1]
	if codex.CostStatus != "unpriced" || codex.UnpricedTokens != 5000 || codex.Edits != 1 {
		t.Fatalf("codex digest = %+v", codex)
	}
	if !codex.Relation.StartedBeforeScope {
		t.Fatalf("relation must carry over: %+v", codex.Relation)
	}
}

func TestBuildReportMetricsReportCostCoverageNotZeroCost(t *testing.T) {
	report := BuildReport(fixtureBundle(t), fixtureLookup, false, 0)
	m := report.Metrics
	if m.Sessions != 2 || m.SourceFiles != 3 || m.Workspaces != 1 || m.LongRunningSessions != 1 {
		t.Fatalf("metrics = %+v", m)
	}
	if m.UserPrompts != 3 || m.Edits != 2 || m.Commits != 1 || m.Interrupts != 1 {
		t.Fatalf("metrics = %+v", m)
	}
	if m.Tokens != 15000 || m.UnpricedTokens != 5000 || m.CostUSD != 1.25 || m.PricedSessions != 1 || m.UnpricedSessions != 1 {
		t.Fatalf("cost metrics = %+v", m)
	}
	if got := m.CostCoverage; got < 0.66 || got > 0.67 {
		t.Fatalf("cost coverage = %v, want 10000/15000", got)
	}
	if len(report.Models) != 2 || report.Models[0].Model != "claude-opus-5" || report.Models[1].Priced {
		t.Fatalf("models = %+v", report.Models)
	}
	if len(report.Workspaces) != 1 || report.Workspaces[0].Sessions != 2 || report.Workspaces[0].Commits != 1 || len(report.Workspaces[0].SessionIDs) != 2 {
		t.Fatalf("workspaces = %+v", report.Workspaces)
	}
	if len(report.Days) != 1 || report.Days[0].Key != "2026-08-20" || report.Days[0].Sessions != 2 || report.Days[0].UserPrompts != 3 {
		t.Fatalf("days = %+v", report.Days)
	}
	if len(report.Records) != 0 {
		t.Fatalf("records must be omitted unless requested, got %d", len(report.Records))
	}
}

func TestBuildReportWithoutLookupWarnsInsteadOfClaimingUnpriced(t *testing.T) {
	report := BuildReport(fixtureBundle(t), nil, true, 0)
	if report.Sessions[0].CostStatus != "" || report.Sessions[0].Model != "" {
		t.Fatalf("no accounting joined, digest = %+v", report.Sessions[0])
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "accounting") {
		t.Fatalf("warnings = %v", report.Warnings)
	}
	if len(report.Records) != 9 || report.Metrics.RecordsReturned != 9 {
		t.Fatalf("records requested but got %d (returned %d)", len(report.Records), report.Metrics.RecordsReturned)
	}
}

func TestRecordLimitPreservesDigestEvidence(t *testing.T) {
	bundle := fixtureBundle(t)
	report := BuildReport(bundle, fixtureLookup, true, 1)
	if len(report.Records) != 1 || report.Metrics.RecordsReturned != 1 || !report.Metrics.Truncated {
		t.Fatalf("record limit not applied: %+v", report.Metrics)
	}
	if report.Metrics.UserPrompts != 3 || report.Metrics.Edits != 2 || len(report.Workspaces) != 1 {
		t.Fatalf("record limit changed digest totals: %+v", report.Metrics)
	}
	if report.Sessions[0].FinalAnswer == nil || report.Sessions[0].FinalAnswer.Line != 6 || report.Sessions[1].Edits != 1 {
		t.Fatalf("record limit dropped later evidence: %+v", report.Sessions)
	}
	withoutRecords := BuildReport(bundle, fixtureLookup, false, 1)
	if len(withoutRecords.Records) != 0 || withoutRecords.Metrics.Truncated || withoutRecords.Metrics.UserPrompts != 3 {
		t.Fatalf("limit must only affect included records: %+v", withoutRecords.Metrics)
	}
}

func TestModelWithPartialPricingIsNotFullyPriced(t *testing.T) {
	report := BuildReport(fixtureBundle(t), func(id string) *parser.Session {
		s := fixtureLookup(id)
		if id == "aaaa1111" {
			s.Stats.UnpricedTokens = 1000
		}
		return s
	}, false, 0)
	if report.Models[0].Priced || report.Sessions[0].CostStatus != "partial" {
		t.Fatalf("partial pricing reported as complete: %+v", report.Models)
	}
}

func TestBuildReportPromptWindowKeepsHeadAndTail(t *testing.T) {
	bundle := fixtureBundle(t)
	var extra []sessionlog.Record
	for i := 0; i < 20; i++ {
		extra = append(extra, sessionlog.Record{
			Timestamp: mustTime(t, "2026-08-20T11:00:00Z").Add(time.Duration(i) * time.Minute),
			Provider:  "claude-code", SessionID: "aaaa1111", Workspace: "/tmp/ws", Project: "ws",
			SourceFile: bundle.Sessions[0].SourceFile, Line: 100 + i, Kind: "user_prompt", Text: strings.Repeat("x", 300),
		})
	}
	bundle.Records = append(bundle.Records, extra...)
	report := BuildReport(bundle, fixtureLookup, false, 0)
	d := report.Sessions[0]
	if d.UserPrompts != 22 || len(d.Prompts) != promptHead+promptTail || d.PromptsOmitted != 22-promptHead-promptTail {
		t.Fatalf("prompts = %d shown %d omitted %d", d.UserPrompts, len(d.Prompts), d.PromptsOmitted)
	}
	if last := d.Prompts[len(d.Prompts)-1]; last.Line != 119 || len(last.Text) != promptTextMax+3 || !strings.HasSuffix(last.Text, "...") {
		t.Fatalf("tail prompt = line %d len %d", last.Line, len(last.Text))
	}
}

func TestRenderHTMLIsSelfContainedAndSaysUnpriced(t *testing.T) {
	report := BuildReport(fixtureBundle(t), fixtureLookup, true, 0)
	report.Command = "ccx insight --since 2026-08-20 --until 2026-08-21 --all --json"
	page := string(RenderHTML(report))
	for _, want := range []string{
		"<title>Session Intelligence — Aug 20</title>",
		"Cost covers 67% of tokens",
		"n/a (unpriced)",
		"build the digest",
		"Done: digest built and tested.",
		"ccx trace aaaa1111",
		"interrupt: [Request interrupted by user]",
		"ccx insight --since 2026-08-20",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, forbidden := range []string{"https://", "http://", "<script src", "@import"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("page must be self-contained, found %q", forbidden)
		}
	}
	if strings.Contains(page, "You are a subagent") {
		t.Errorf("subagent instruction rendered as a human prompt")
	}
}

func TestReportJSONShapeIsStableForScripts(t *testing.T) {
	report := BuildReport(fixtureBundle(t), fixtureLookup, false, 0)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "scope", "metrics", "models", "days", "providers", "workspaces", "sessions"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}
	if _, ok := decoded["records"]; ok {
		t.Errorf("records must be absent without --records")
	}
	sessions := decoded["sessions"].([]any)
	first := sessions[0].(map[string]any)
	if _, ok := first["prompts"]; !ok {
		t.Errorf("session digest must always carry prompts (empty list, never absent)")
	}
	if _, ok := first["cost_usd"]; !ok {
		t.Errorf("session digest must always carry cost_usd")
	}
}
