package sessionlog

import (
	"os"
	"path/filepath"
	"strings"
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

// Codex 0.147 rollouts carry the visible conversation as
// event_msg.item_completed UserMessage/AgentMessage TurnItems; the raw
// response_item messages are model I/O (with injected envelopes) and
// legacy user_message/agent_message events are duplicates in hybrid
// files. The log surface must apply the parser's one-source rule:
// item_completed rows become the prompts/replies with text, and the
// duplicates stay as records but stop counting (docs/design/0004).
func TestCollectCodex0147ItemCompletedIsTheConversation(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "08")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessionDir, "rollout-0147.jsonl")
	writeLines(t, file,
		`{"timestamp":"2026-08-18T04:45:00Z","type":"session_meta","payload":{"id":"codex-0147","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-08-18T04:45:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /tmp/repo (injected envelope)"}]}}`,
		`{"timestamp":"2026-08-18T04:45:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"id":"item-u1","type":"UserMessage","content":[{"type":"text","text":"hello from 0.147"}]}}}`,
		`{"timestamp":"2026-08-18T04:45:03Z","type":"event_msg","payload":{"type":"user_message","message":"hello from 0.147"}}`,
		`{"timestamp":"2026-08-18T04:45:04Z","type":"event_msg","payload":{"type":"item_completed","item":{"id":"item-a1","type":"AgentMessage","content":[{"type":"Text","text":"Received."}]}}}`,
		`{"timestamp":"2026-08-18T04:45:05Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Received."}]}}`,
		`{"timestamp":"2026-08-18T04:45:06Z","type":"event_msg","payload":{"type":"item_completed","item":{"id":"item-c1","type":"CommandExecution","command":"ls"}}}`,
	)

	start := mustParseTime(t, "2026-08-18T00:00:00Z")
	end := mustParseTime(t, "2026-08-19T00:00:00Z")
	bundle, err := Collect([]Source{{Provider: "codex", Home: home}}, Options{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Records) != 7 {
		t.Fatalf("records = %d, want 7 (nothing dropped)", len(bundle.Records))
	}
	byLine := map[int]Record{}
	for _, r := range bundle.Records {
		byLine[r.Line] = r
	}
	if r := byLine[3]; r.Kind != "user_prompt" || r.Role != "user" || r.Text != "hello from 0.147" || r.UUID != "item-u1" {
		t.Fatalf("item_completed UserMessage: %+v", r)
	}
	if r := byLine[5]; r.Kind != "assistant_message" || r.Text != "Received." || r.UUID != "item-a1" {
		t.Fatalf("item_completed AgentMessage: %+v", r)
	}
	if r := byLine[2]; r.Kind != "model_input" {
		t.Fatalf("raw response_item user message must be model_input, got %+v", r)
	}
	if r := byLine[4]; r.Kind != "legacy_message" {
		t.Fatalf("legacy user_message event must be demoted in a hybrid file, got %+v", r)
	}
	if r := byLine[6]; r.Kind != "model_output" {
		t.Fatalf("raw response_item assistant message must be model_output, got %+v", r)
	}
	if r := byLine[7]; r.Kind != "item_completed" {
		t.Fatalf("non-message TurnItem keeps its own kind, got %+v", r)
	}
	if bundle.Metrics.UserPrompts != 1 || bundle.Metrics.AssistantMessages != 1 {
		t.Fatalf("metrics must count the conversation once: %+v", bundle.Metrics)
	}
	if bundle.Sessions[0].Preview != "hello from 0.147" {
		t.Fatalf("preview must come from the visible prompt, got %q", bundle.Sessions[0].Preview)
	}
}

// A legacy (pre-0.147) rollout has no item_completed messages, so its
// user_message / agent_message events remain the conversation.
func TestCollectCodexLegacyRolloutUnchanged(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".codex")
	sessionDir := filepath.Join(home, "sessions", "2026", "05")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(sessionDir, "legacy.jsonl"),
		`{"timestamp":"2026-05-21T00:01:00Z","type":"session_meta","payload":{"id":"codex-legacy","cwd":"/tmp/repo"}}`,
		`{"timestamp":"2026-05-21T00:02:00Z","type":"event_msg","payload":{"type":"user_message","message":"legacy prompt"}}`,
		`{"timestamp":"2026-05-21T00:03:00Z","type":"event_msg","payload":{"type":"agent_message","message":"legacy reply"}}`,
	)
	bundle, err := Collect([]Source{{Provider: "codex", Home: home}}, Options{Start: mustParseTime(t, "2026-05-21T00:00:00Z"), End: mustParseTime(t, "2026-05-22T00:00:00Z")})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metrics.UserPrompts != 1 || bundle.Metrics.AssistantMessages != 1 {
		t.Fatalf("legacy rollout must keep counting: %+v", bundle.Metrics)
	}
}

// user-role lines that no human typed — command markers, local command
// echoes, task notifications, injected meta (skill bodies), compaction
// carriers — must not be user_prompt in the log, exactly as in the
// parser; --kind user_prompt is "the humans in the loop".
func TestCollectClaudeUserRoleNoiseIsNotAPrompt(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".claude")
	projDir := filepath.Join(home, "projects", "-tmp-repo")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(projDir, "s1.jsonl"),
		`{"type":"user","uuid":"u1","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"real prompt"}]}}`,
		`{"type":"user","uuid":"u2","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:01Z","message":{"role":"user","content":"<command-name>/model</command-name><command-message>model</command-message>"}}`,
		`{"type":"user","uuid":"u3","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:02Z","message":{"role":"user","content":[{"type":"text","text":"<local-command-stdout>Set model</local-command-stdout>"}]}}`,
		`{"type":"user","uuid":"u4","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:03Z","message":{"role":"user","content":[{"type":"text","text":"<task-notification>done</task-notification>"}]}}`,
		`{"type":"user","uuid":"u5","sessionId":"s1","cwd":"/tmp/repo","isMeta":true,"timestamp":"2026-08-18T10:00:04Z","message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: /x"}]}}`,
		`{"type":"user","uuid":"u6","sessionId":"s1","cwd":"/tmp/repo","isCompactSummary":true,"timestamp":"2026-08-18T10:00:05Z","message":{"role":"user","content":[{"type":"text","text":"summary of earlier context"}]}}`,
		`{"type":"user","uuid":"u7","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:06Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"user","uuid":"u8","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:07Z","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"type":"user","uuid":"u9","sessionId":"s1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:08Z","message":{"role":"user","content":[{"type":"tool_result","content":"The user doesn't want to proceed with this tool use. The tool use was rejected.","is_error":true}]}}`,
	)
	bundle, err := Collect([]Source{{Provider: "claude-code", Home: home}}, Options{Start: mustParseTime(t, "2026-08-18T00:00:00Z"), End: mustParseTime(t, "2026-08-19T00:00:00Z")})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"u1": "user_prompt", "u2": "command", "u3": "command_output", "u4": "notification", "u5": "meta", "u6": "compact_summary", "u7": "tool_result", "u8": "interrupt", "u9": "tool_denied"}
	for _, r := range bundle.Records {
		if want[r.UUID] != r.Kind {
			t.Errorf("%s: kind %q, want %q", r.UUID, r.Kind, want[r.UUID])
		}
	}
	if bundle.Metrics.UserPrompts != 1 {
		t.Fatalf("user prompts: %+v", bundle.Metrics)
	}

	// --kind and --match narrow the records list, keep scope-wide
	// metrics, and report the matched count; Match sees the raw line.
	bundle, err = Collect([]Source{{Provider: "claude-code", Home: home}}, Options{
		Start: mustParseTime(t, "2026-08-18T00:00:00Z"), End: mustParseTime(t, "2026-08-19T00:00:00Z"),
		Kinds: []string{"user_prompt", "meta"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Records) != 2 || bundle.Metrics.Records != 9 || bundle.Metrics.RecordsMatched != 2 || bundle.Metrics.RecordsReturned != 2 {
		t.Fatalf("kind filter: %d records, metrics %+v", len(bundle.Records), bundle.Metrics)
	}
	bundle, err = Collect([]Source{{Provider: "claude-code", Home: home}}, Options{
		Start: mustParseTime(t, "2026-08-18T00:00:00Z"), End: mustParseTime(t, "2026-08-19T00:00:00Z"),
		Match: func(line string) bool { return strings.Contains(line, "isMeta") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Records) != 1 || bundle.Records[0].UUID != "u5" {
		t.Fatalf("match filter on raw line: %+v", bundle.Records)
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
