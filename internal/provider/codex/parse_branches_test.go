package codex

import (
	"path/filepath"
	"testing"
)

func TestFindProjectCaseInsensitive(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T100000-thread-fp.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T10:00:00Z","type":"session_meta","payload":{"id":"thread-fp","timestamp":"2026-04-02T10:00:00Z","cwd":"/home/dev/MyProject","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test","images":[],"local_images":[],"text_elements":[]}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	p, err := backend.FindProject("myproject")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project case-insensitively")
	}
}

func TestFindProjectSubstring(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T110000-thread-sub.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T11:00:00Z","type":"session_meta","payload":{"id":"thread-sub","timestamp":"2026-04-02T11:00:00Z","cwd":"/home/dev/code/my-cool-project","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T11:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test","images":[],"local_images":[],"text_elements":[]}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	p, err := backend.FindProject("cool")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p == nil {
		t.Fatal("expected to find project by substring")
	}
}

func TestFindProjectNotFound(t *testing.T) {
	home := t.TempDir()
	backend := NewWithDirs(home, filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions"))

	p, err := backend.FindProject("nonexistent")
	if err != nil {
		t.Fatalf("FindProject() error: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil for nonexistent project")
	}
}

func TestFindSessionByID(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T120000-thread-fs.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T12:00:00Z","type":"session_meta","payload":{"id":"thread-fs","timestamp":"2026-04-02T12:00:00Z","cwd":"/home/dev/project","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T12:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test find session","images":[],"local_images":[],"text_elements":[]}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	s, err := backend.FindSession("", "thread-fs")
	if err != nil {
		t.Fatalf("FindSession() error: %v", err)
	}
	if s == nil {
		t.Fatal("expected to find session by ID")
	}
	if s.ID != "thread-fs" {
		t.Fatalf("session.ID = %q, want thread-fs", s.ID)
	}
}

func TestFindSessionByPrefix(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T130000-thread-prefix.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T13:00:00Z","type":"session_meta","payload":{"id":"thread-prefix","timestamp":"2026-04-02T13:00:00Z","cwd":"/home/dev/project","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T13:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test prefix","images":[],"local_images":[],"text_elements":[]}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	s, err := backend.FindSession("", "thread-pre")
	if err != nil {
		t.Fatalf("FindSession() error: %v", err)
	}
	if s == nil {
		t.Fatal("expected to find session by prefix")
	}
}

func TestFindSessionNotFound(t *testing.T) {
	home := t.TempDir()
	backend := NewWithDirs(home, filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions"))

	s, err := backend.FindSession("", "nonexistent")
	if err != nil {
		t.Fatalf("FindSession() error: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestParseSessionPatchApplyEnd(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T140000-thread-patch.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T14:00:00Z","type":"session_meta","payload":{"id":"thread-patch","timestamp":"2026-04-02T14:00:00Z","cwd":"/tmp/test-patch","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T14:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"apply a patch","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T14:00:02Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"call-p1","turn_id":"turn-1","patch":"*** Begin Patch\n*** End Patch","status":"completed","result":"Success","formatted_output":"Applied 1 change"}}
{"timestamp":"2026-04-02T14:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"patch applied"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
	root := session.RootMessages[0]
	found := false
	for _, child := range root.Children {
		for _, block := range child.Content {
			if block.Type == "tool_use" && block.ToolName == "ApplyPatch" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected ApplyPatch tool_use in children")
	}
}

func TestParseSessionWebSearchEnd(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T150000-thread-ws.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T15:00:00Z","type":"session_meta","payload":{"id":"thread-ws","timestamp":"2026-04-02T15:00:00Z","cwd":"/tmp/test-websearch","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T15:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"search the web","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T15:00:02Z","type":"event_msg","payload":{"type":"web_search_end","call_id":"call-ws1","turn_id":"turn-1","query":"golang testing","status":"completed","results":"found 5 results"}}
{"timestamp":"2026-04-02T15:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"found results"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
}

func TestParseSessionThreadNameUpdated(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T160000-thread-rename.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T16:00:00Z","type":"session_meta","payload":{"id":"thread-rename","timestamp":"2026-04-02T16:00:00Z","cwd":"/tmp/test-rename","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T16:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"do something","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T16:00:02Z","type":"event_msg","payload":{"type":"thread_name_updated","thread_name":"Renamed Thread"}}
{"timestamp":"2026-04-02T16:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	session := projects[0].Sessions[0]
	if session.Summary != "Renamed Thread" {
		t.Fatalf("Summary = %q, want Renamed Thread", session.Summary)
	}
}

func TestParseSessionCompacted(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T170000-thread-compact.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T17:00:00Z","type":"session_meta","payload":{"id":"thread-compact","timestamp":"2026-04-02T17:00:00Z","cwd":"/tmp/test-compact","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T17:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"long conversation","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T17:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"reply"}}
{"timestamp":"2026-04-02T17:00:03Z","type":"compacted","payload":{"message":"context was compacted here","replacement_history":[]}}
{"timestamp":"2026-04-02T17:00:04Z","type":"event_msg","payload":{"type":"user_message","message":"continue","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T17:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"ok"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if len(session.RootMessages) != 3 {
		t.Fatalf("expected 3 roots (user1, compacted, user2), got %d", len(session.RootMessages))
	}

	compact := session.RootMessages[1]
	if !compact.IsCompacted {
		t.Fatal("second root should be compacted")
	}
	if compact.Content[0].Text != "context was compacted here" {
		t.Fatalf("compact text = %q", compact.Content[0].Text)
	}
}

func TestParseSessionShellCommandNormalized(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T180000-thread-shell.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T18:00:00Z","type":"session_meta","payload":{"id":"thread-shell","timestamp":"2026-04-02T18:00:00Z","cwd":"/tmp/test-shell","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T18:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"run a shell command","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T18:00:02Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-sc1"}}
{"timestamp":"2026-04-02T18:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-sc1","output":"/tmp/test-shell"}}
{"timestamp":"2026-04-02T18:00:04Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	root := session.RootMessages[0]
	found := false
	for _, child := range root.Children {
		for _, block := range child.Content {
			if block.Type == "tool_use" && block.ToolName == "Bash" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("shell_command should be normalized to Bash")
	}
}

func TestParseSessionProviderSet(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T190000-thread-prov.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T19:00:00Z","type":"session_meta","payload":{"id":"thread-prov","timestamp":"2026-04-02T19:00:00Z","cwd":"/tmp/test-provider","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T19:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"provider test","images":[],"local_images":[],"text_elements":[]}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Provider != "codex" {
		t.Fatalf("Provider = %q, want codex", session.Provider)
	}
}

func TestParseSessionTokenCountFromEventMsg(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T200000-thread-tok.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T20:00:00Z","type":"session_meta","payload":{"id":"thread-tok","timestamp":"2026-04-02T20:00:00Z","cwd":"/tmp/test-tokens","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T20:00:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/test-tokens","approval_policy":"never","sandbox_policy":"danger_full_access","model":"gpt-5"}}
{"timestamp":"2026-04-02T20:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"token test","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T20:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":500,"cached_input_tokens":100,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":700},"last_token_usage":{"input_tokens":500,"cached_input_tokens":100,"output_tokens":200,"reasoning_output_tokens":50,"total_tokens":700},"model_context_window":128000},"rate_limits":null}}
{"timestamp":"2026-04-02T20:00:03Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	// input_tokens=500 includes cached_input_tokens=100; ccx carries
	// the non-cached 400 (Anthropic-style exclusive semantics).
	if session.Stats.InputTokens != 400 {
		t.Fatalf("InputTokens = %d, want 400", session.Stats.InputTokens)
	}
	if session.Stats.CacheReadTokens != 100 {
		t.Fatalf("CacheReadTokens = %d, want 100", session.Stats.CacheReadTokens)
	}
	if session.Stats.OutputTokens != 200 {
		t.Fatalf("OutputTokens = %d, want 200", session.Stats.OutputTokens)
	}
	if len(session.RootMessages) != 1 {
		t.Fatalf("len(session.RootMessages) = %d, want 1", len(session.RootMessages))
	}
	if len(session.RootMessages[0].Children) != 1 {
		t.Fatalf("len(root.Children) = %d, want 1", len(session.RootMessages[0].Children))
	}
	assistant := session.RootMessages[0].Children[0]
	if assistant.Usage == nil {
		t.Fatal("assistant.Usage is nil, want attributed token delta")
	}
	if assistant.Usage.InputTokens != 400 {
		t.Fatalf("assistant.Usage.InputTokens = %d, want 400", assistant.Usage.InputTokens)
	}
	if assistant.Usage.CacheReadTokens != 100 {
		t.Fatalf("assistant.Usage.CacheReadTokens = %d, want 100", assistant.Usage.CacheReadTokens)
	}
	if assistant.Usage.OutputTokens != 200 {
		t.Fatalf("assistant.Usage.OutputTokens = %d, want 200", assistant.Usage.OutputTokens)
	}
	if assistant.Usage.ReasoningTokens != 50 {
		t.Fatalf("assistant.Usage.ReasoningTokens = %d, want 50", assistant.Usage.ReasoningTokens)
	}
	// gpt-5 tier 10/80, cache read 5: 400*10 + 200*80 + 100*5 per 1M.
	// Cached input billed once (at the cache rate) and reasoning not
	// billed on top of output — the double-billing regression guard.
	wantCost := 0.0205
	if diff := assistant.Usage.CostUSD - wantCost; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("assistant.Usage.CostUSD = %v, want %v", assistant.Usage.CostUSD, wantCost)
	}
	if session.Stats.CostUSD != assistant.Usage.CostUSD {
		t.Fatalf("session.Stats.CostUSD = %v, want %v", session.Stats.CostUSD, assistant.Usage.CostUSD)
	}
}

func TestParseSessionDeduplicatesResponseItemAndEventMsg(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T210000-thread-dedup.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T21:00:00Z","type":"session_meta","payload":{"id":"thread-dedup","timestamp":"2026-04-02T21:00:00Z","cwd":"/tmp/test-dedup","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T21:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test dedup","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T21:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-dup1"}}
{"timestamp":"2026-04-02T21:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-dup1","output":"/tmp/test-dedup"}}
{"timestamp":"2026-04-02T21:00:04Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-dup1","turn_id":"turn-1","command":["pwd"],"cwd":"/tmp/test-dedup","stdout":"/tmp/test-dedup\n","stderr":"","exit_code":0,"status":"completed","duration":"PT0.01S"}}
{"timestamp":"2026-04-02T21:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d, want 1 (should not double-count response_item + event_msg)", session.Stats.ToolCalls)
	}

	root := session.RootMessages[0]
	toolUseCount := 0
	for _, child := range root.Children {
		for _, block := range child.Content {
			if block.Type == "tool_use" {
				toolUseCount++
			}
		}
	}
	if toolUseCount != 1 {
		t.Fatalf("tool_use messages = %d, want 1 (should not duplicate)", toolUseCount)
	}
}

func TestParseSessionCompletesResponseItemToolFromEventMsgEnd(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T211500-thread-dedup-missing-output.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T21:15:00Z","type":"session_meta","payload":{"id":"thread-dedup-missing-output","timestamp":"2026-04-02T21:15:00Z","cwd":"/tmp/test-dedup-missing-output","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T21:15:01Z","type":"event_msg","payload":{"type":"user_message","message":"test missing function_call_output","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T21:15:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-missing-output-1"}}
{"timestamp":"2026-04-02T21:15:03Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-missing-output-1","turn_id":"turn-1","command":["pwd"],"cwd":"/tmp/test-dedup-missing-output","stdout":"/tmp/test-dedup-missing-output\n","stderr":"","exit_code":0,"status":"completed","duration":"PT0.01S"}}
{"timestamp":"2026-04-02T21:15:04Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}

	root := session.RootMessages[0]
	toolUseCount := 0
	toolResultCount := 0
	for _, child := range root.Children {
		for _, block := range child.Content {
			switch block.Type {
			case "tool_use":
				toolUseCount++
			case "tool_result":
				toolResultCount++
				if block.ToolName != "Bash" {
					t.Fatalf("tool_result.ToolName = %q, want Bash", block.ToolName)
				}
			}
		}
	}
	if toolUseCount != 1 {
		t.Fatalf("tool_use messages = %d, want 1", toolUseCount)
	}
	if toolResultCount != 1 {
		t.Fatalf("tool_result messages = %d, want 1", toolResultCount)
	}
}

func TestParseSessionDoesNotDuplicateToolResultWhenEventMsgEndPrecedesFunctionCallOutput(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T212000-thread-dedup-order.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T21:20:00Z","type":"session_meta","payload":{"id":"thread-dedup-order","timestamp":"2026-04-02T21:20:00Z","cwd":"/tmp/test-dedup-order","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T21:20:01Z","type":"event_msg","payload":{"type":"user_message","message":"test tool result order","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T21:20:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-order-1"}}
{"timestamp":"2026-04-02T21:20:03Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-order-1","turn_id":"turn-1","command":["pwd"],"cwd":"/tmp/test-dedup-order","stdout":"/tmp/test-dedup-order\n","stderr":"","exit_code":0,"status":"completed","duration":"PT0.01S"}}
{"timestamp":"2026-04-02T21:20:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-order-1","output":"/tmp/test-dedup-order"}}
{"timestamp":"2026-04-02T21:20:05Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}

	root := session.RootMessages[0]
	toolUseCount := 0
	toolResultCount := 0
	for _, child := range root.Children {
		for _, block := range child.Content {
			switch block.Type {
			case "tool_use":
				toolUseCount++
			case "tool_result":
				toolResultCount++
				if block.ToolName != "Bash" {
					t.Fatalf("tool_result.ToolName = %q, want Bash", block.ToolName)
				}
			}
		}
	}
	if toolUseCount != 1 {
		t.Fatalf("tool_use messages = %d, want 1", toolUseCount)
	}
	if toolResultCount != 1 {
		t.Fatalf("tool_result messages = %d, want 1", toolResultCount)
	}
}

func TestParseSessionModelFromTurnContext(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T220000-thread-model.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T22:00:00Z","type":"session_meta","payload":{"id":"thread-model","timestamp":"2026-04-02T22:00:00Z","cwd":"/tmp/test-model","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T22:00:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/test-model","model":"gpt-5.4"}}
{"timestamp":"2026-04-02T22:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"model test","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T22:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)

	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}
	if session.Model != "gpt-5.4" {
		t.Fatalf("full parse Model = %q, want gpt-5.4", session.Model)
	}

	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error: %v", err)
	}
	if len(projects) != 1 || len(projects[0].Sessions) != 1 {
		t.Fatal("expected 1 project with 1 session")
	}
	if projects[0].Sessions[0].Model != "gpt-5.4" {
		t.Fatalf("quick parse Model = %q, want gpt-5.4", projects[0].Sessions[0].Model)
	}
}

func TestIDAndHomes(t *testing.T) {
	home := "/tmp/test-codex"
	b := New(home)
	if b.ID() != "codex" {
		t.Fatalf("ID() = %q, want codex", b.ID())
	}
	homes := b.Homes()
	if len(homes) != 1 || homes[0] != home {
		t.Fatalf("Homes() = %v, want [%s]", homes, home)
	}
}

func TestProjectInfoForCWD(t *testing.T) {
	tests := []struct {
		cwd         string
		wantEncoded string
		wantDisplay string
		wantPath    string
	}{
		{"/home/dev/project", "home-dev-project", "project", "/home/dev/project"},
		{"", "unknown", "(unknown cwd)", ""},
	}

	for _, tt := range tests {
		encoded, display, path := projectInfoForCWD(tt.cwd)
		if encoded != tt.wantEncoded {
			t.Errorf("projectInfoForCWD(%q) encoded = %q, want %q", tt.cwd, encoded, tt.wantEncoded)
		}
		if display != tt.wantDisplay {
			t.Errorf("projectInfoForCWD(%q) display = %q, want %q", tt.cwd, display, tt.wantDisplay)
		}
		if path != tt.wantPath {
			t.Errorf("projectInfoForCWD(%q) path = %q, want %q", tt.cwd, path, tt.wantPath)
		}
	}
}
