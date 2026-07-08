package sessionlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectSlicesRecordsInsideScopeAcrossLongRunningClaudeSession(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".claude")
	projectDir := filepath.Join(home, "projects", "-tmp-workspace")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "session-1.jsonl")
	writeLines(t, file,
		`{"type":"user","sessionId":"session-1","cwd":"/tmp/workspace","timestamp":"2026-05-20T23:00:00+08:00","message":{"role":"user","content":"start yesterday prep"}}`,
		`{"type":"assistant","sessionId":"session-1","cwd":"/tmp/workspace","timestamp":"2026-05-21T09:00:00+08:00","message":{"role":"assistant","content":[{"type":"text","text":"worked on the actual target day"}]}}`,
		`{"type":"user","sessionId":"session-1","cwd":"/tmp/workspace","timestamp":"2026-05-22T01:00:00+08:00","message":{"role":"user","content":"after the window"}}`,
	)

	start := mustParseTime(t, "2026-05-21T00:00:00+08:00")
	end := mustParseTime(t, "2026-05-22T00:00:00+08:00")
	bundle, err := Collect([]Source{{Provider: "claude-code", Home: home}}, Options{
		Start:     start,
		End:       end,
		ScopeName: "yesterday",
		TimeZone:  "+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(bundle.Sessions))
	}
	session := bundle.Sessions[0]
	if session.ID != "session-1" || session.Records != 1 {
		t.Fatalf("session slice = %+v", session)
	}
	if !session.Relation.StartedBeforeScope || !session.Relation.EndedAfterScope {
		t.Fatalf("expected long-running relation flags, got %+v", session.Relation)
	}
	if len(bundle.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(bundle.Records))
	}
	record := bundle.Records[0]
	if record.Kind != "assistant_message" || record.Text != "worked on the actual target day" {
		t.Fatalf("record = %+v", record)
	}
}

func TestCollectNormalizesCodexRecordsAndWorkspaceFilter(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "05")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessionDir, "rollout.jsonl")
	writeLines(t, file,
		`{"timestamp":"2026-05-21T00:01:00+08:00","type":"session_meta","payload":{"id":"codex-1","cwd":"/tmp/repo","timestamp":"2026-05-21T00:01:00+08:00"}}`,
		`{"timestamp":"2026-05-21T00:02:00+08:00","type":"event_msg","payload":{"type":"user_message","message":"what did I do yesterday?"}}`,
		`{"timestamp":"2026-05-21T00:03:00+08:00","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call-1"}}`,
		`{"timestamp":"2026-05-21T00:04:00+08:00","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-1","command":["git","status"],"cwd":"/tmp/repo","status":"completed","stdout":"clean"}}`,
	)

	start := mustParseTime(t, "2026-05-21T00:00:00+08:00")
	end := mustParseTime(t, "2026-05-22T00:00:00+08:00")
	bundle, err := Collect([]Source{{Provider: "codex", Home: home}}, Options{
		Start:         start,
		End:           end,
		WorkspacePath: "/tmp/repo",
		IncludeRaw:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metrics.Sessions != 1 || bundle.Metrics.UserPrompts != 1 || bundle.Metrics.ToolCalls != 1 || bundle.Metrics.ToolResults != 1 {
		t.Fatalf("metrics = %+v", bundle.Metrics)
	}
	if len(bundle.Records) != 4 {
		t.Fatalf("records = %d, want 4", len(bundle.Records))
	}
	if bundle.Records[0].SessionID != "codex-1" || bundle.Records[0].Workspace != "/tmp/repo" {
		t.Fatalf("metadata not propagated: %+v", bundle.Records[0])
	}
	if len(bundle.Records[0].RawJSON) == 0 {
		t.Fatalf("raw json missing for --raw")
	}
}

func TestCollectBoundaryRelationUsesHalfOpenScope(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".claude")
	projectDir := filepath.Join(home, "projects", "-tmp-boundary")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "boundary.jsonl")
	writeLines(t, file,
		`{"type":"user","sessionId":"boundary","cwd":"/tmp/boundary","timestamp":"2026-05-21T00:00:00+08:00","message":{"role":"user","content":"at start"}}`,
	)

	start := mustParseTime(t, "2026-05-21T00:00:00+08:00")
	end := mustParseTime(t, "2026-05-22T00:00:00+08:00")
	bundle, err := Collect([]Source{{Provider: "claude-code", Home: home}}, Options{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sessions) != 1 || len(bundle.Records) != 1 {
		t.Fatalf("bundle = %+v", bundle)
	}
	relation := bundle.Sessions[0].Relation
	if !relation.OverlapsScope || !relation.StartedInScope || !relation.EndedInScope {
		t.Fatalf("unexpected relation at scope start: %+v", relation)
	}
	if relation.EndedAfterScope {
		t.Fatalf("EndedAfterScope should be false at start boundary: %+v", relation)
	}
}

func TestCollectClassifiesCodexDeveloperMessageAsInstruction(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "05")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessionDir, "developer.jsonl")
	writeLines(t, file,
		`{"timestamp":"2026-05-21T00:01:00+08:00","type":"session_meta","payload":{"id":"codex-dev","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-05-21T00:02:00+08:00","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer instruction"}]}}`,
	)

	start := mustParseTime(t, "2026-05-21T00:00:00+08:00")
	end := mustParseTime(t, "2026-05-22T00:00:00+08:00")
	bundle, err := Collect([]Source{{Provider: "codex", Home: home}}, Options{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(bundle.Records))
	}
	if bundle.Records[1].Kind != "instruction" || bundle.Records[1].Role != "developer" {
		t.Fatalf("developer message classified incorrectly: %+v", bundle.Records[1])
	}
	if bundle.Metrics.AssistantMessages != 0 {
		t.Fatalf("developer message counted as assistant: %+v", bundle.Metrics)
	}
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustParseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestAggregateRecordsBucketsByDayProviderWorkspace(t *testing.T) {
	loc := time.FixedZone("+08:00", 8*3600)
	records := []Record{
		// 2026-01-01 23:30 UTC = 2026-01-02 07:30 +08 — must land on Jan 2 in scope TZ.
		{Timestamp: mustParseTime(t, "2026-01-01T23:30:00Z"), Provider: "claude-code", SessionID: "s1", Workspace: "/w/a", Kind: "user_prompt"},
		{Timestamp: mustParseTime(t, "2026-01-02T01:00:00Z"), Provider: "claude-code", SessionID: "s1", Workspace: "/w/a", Kind: "tool_call"},
		{Timestamp: mustParseTime(t, "2026-01-02T02:00:00Z"), Provider: "codex", SessionID: "s2", Workspace: "/w/b", Kind: "assistant_message", IsSidechain: true},
		{Timestamp: mustParseTime(t, "2026-01-03T02:00:00Z"), Provider: "codex", SessionID: "s3", Project: "proj-c", Kind: "user_prompt"},
	}

	days, providers, workspaces := aggregateRecords(records, loc)

	if len(days) != 2 || days[0].Key != "2026-01-02" || days[1].Key != "2026-01-03" {
		t.Fatalf("days: %+v", days)
	}
	if days[0].Records != 3 || days[0].Sessions != 2 || days[0].UserPrompts != 1 || days[0].ToolCalls != 1 || days[0].Sidechains != 1 {
		t.Fatalf("day[0]: %+v", days[0])
	}

	if len(providers) != 2 || providers[0].Key != "claude-code" || providers[0].Records != 2 {
		t.Fatalf("providers: %+v", providers)
	}
	if providers[1].Key != "codex" || providers[1].Sessions != 2 {
		t.Fatalf("providers[1]: %+v", providers[1])
	}

	if len(workspaces) != 3 {
		t.Fatalf("workspaces: %+v", workspaces)
	}
	if workspaces[0].Key != "/w/a" || workspaces[0].Records != 2 {
		t.Fatalf("workspaces[0]: %+v", workspaces[0])
	}
	// Project name is the fallback key when workspace is absent.
	found := false
	for _, w := range workspaces {
		if w.Key == "proj-c" && w.Records == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing project-fallback workspace: %+v", workspaces)
	}
}
