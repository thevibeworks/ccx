package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectsGroupsCodexSessionsByCWD(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "24", "rollout-20260324T100000-thread-1.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-24T10:00:00Z","type":"session_meta","payload":{"id":"thread-1","timestamp":"2026-03-24T10:00:00Z","cwd":"/tmp/work/project-alpha","originator":"codex","cli_version":"0.1.0","source":"chat","model_provider":"openai","git":{"branch":"main"}}}
{"timestamp":"2026-03-24T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"## My request for Codex: inspect rollout support","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-24T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"working on it"}}
`)

	writeRollout(t, filepath.Join(home, "session_index.jsonl"), `{"id":"thread-1","thread_name":"named thread","updated_at":"2026-03-24T11:00:00Z"}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	project := projects[0]
	if project.EncodedName != "tmp-work-project-alpha" {
		t.Fatalf("project.EncodedName = %q, want %q", project.EncodedName, "tmp-work-project-alpha")
	}
	if project.Path != "/tmp/work/project-alpha" {
		t.Fatalf("project.Path = %q, want %q", project.Path, "/tmp/work/project-alpha")
	}
	if len(project.Sessions) != 1 {
		t.Fatalf("len(project.Sessions) = %d, want 1", len(project.Sessions))
	}

	session := project.Sessions[0]
	if session.ID != "thread-1" {
		t.Fatalf("session.ID = %q, want %q", session.ID, "thread-1")
	}
	if session.Summary != "named thread" {
		t.Fatalf("session.Summary = %q, want %q", session.Summary, "named thread")
	}
}

func TestDiscoverProjectsWithoutSessionIndexUsesResponseItemToolCounts(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "25", "rollout-20260325T100000-thread-2.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-25T10:00:00Z","type":"session_meta","payload":{"id":"thread-2","timestamp":"2026-03-25T10:00:00Z","cwd":"/tmp/work/project-beta","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-03-25T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"check current codex rollout support","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-25T10:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-1"}}
{"timestamp":"2026-03-25T10:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","status":"completed","name":"apply_patch","input":"*** Begin Patch\n*** End Patch\n","call_id":"call-2"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	session := projects[0].Sessions[0]
	if session.Summary != "check current codex rollout support" {
		t.Fatalf("session.Summary = %q, want %q", session.Summary, "check current codex rollout support")
	}
	if session.Stats.ToolCalls != 2 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 2", session.Stats.ToolCalls)
	}
}

func TestParseSessionBuildsUsableTranscript(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "24", "rollout-20260324T100000-thread-1.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-24T10:00:00Z","type":"session_meta","payload":{"id":"thread-1","timestamp":"2026-03-24T10:00:00Z","cwd":"/tmp/work/project-alpha","originator":"codex","cli_version":"0.1.0","source":"chat","model_provider":"openai","git":{"branch":"main"}}}
{"timestamp":"2026-03-24T10:00:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/work/project-alpha","approval_policy":"never","sandbox_policy":"danger_full_access","model":"gpt-5","summary":"auto"}}
{"timestamp":"2026-03-24T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"## My request for Codex: inspect rollout support","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-24T10:00:02Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"checking the transcript model"}}
{"timestamp":"2026-03-24T10:00:03Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-1","turn_id":"turn-1","command":["rg","rollout"],"cwd":"/tmp/work/project-alpha","parsed_cmd":[],"source":"agent","interaction_input":null,"stdout":"rollout support\n","stderr":"","aggregated_output":"rollout support\n","exit_code":0,"duration":"PT0.1S","formatted_output":"rollout support\n","status":"completed"}}
{"timestamp":"2026-03-24T10:00:04Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
{"timestamp":"2026-03-24T10:00:05Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":5,"reasoning_output_tokens":1,"total_tokens":15},"last_token_usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":5,"reasoning_output_tokens":1,"total_tokens":15},"model_context_window":128000},"rate_limits":null}}
`)

	writeRollout(t, filepath.Join(home, "session_index.jsonl"), `{"id":"thread-1","thread_name":"named thread","updated_at":"2026-03-24T11:00:00Z"}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error = %v", err)
	}

	if session.Summary != "named thread" {
		t.Fatalf("session.Summary = %q, want %q", session.Summary, "named thread")
	}
	if session.Version != "0.1.0" {
		t.Fatalf("session.Version = %q, want %q", session.Version, "0.1.0")
	}
	if session.GitBranch != "main" {
		t.Fatalf("session.GitBranch = %q, want %q", session.GitBranch, "main")
	}
	if session.CWD != "/tmp/work/project-alpha" {
		t.Fatalf("session.CWD = %q, want %q", session.CWD, "/tmp/work/project-alpha")
	}
	if session.Stats.UserPrompts != 1 {
		t.Fatalf("session.Stats.UserPrompts = %d, want 1", session.Stats.UserPrompts)
	}
	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
	if session.Stats.InputTokens != 10 || session.Stats.CacheReadTokens != 2 || session.Stats.OutputTokens != 5 {
		t.Fatalf("unexpected token stats: %+v", session.Stats)
	}
	if session.Stats.CostUSD <= 0 {
		t.Fatalf("session.Stats.CostUSD = %v, want > 0", session.Stats.CostUSD)
	}
	if len(session.RootMessages) != 1 {
		t.Fatalf("len(session.RootMessages) = %d, want 1", len(session.RootMessages))
	}

	root := session.RootMessages[0]
	if root.Kind != "user_prompt" {
		t.Fatalf("root.Kind = %q, want user_prompt", root.Kind)
	}
	if len(root.Children) != 4 {
		t.Fatalf("len(root.Children) = %d, want 4", len(root.Children))
	}
	if root.Children[0].Content[0].Type != "thinking" {
		t.Fatalf("root.Children[0].Content[0].Type = %q, want %q", root.Children[0].Content[0].Type, "thinking")
	}
	if root.Children[1].Content[0].Type != "tool_use" {
		t.Fatalf("root.Children[1].Content[0].Type = %q, want %q", root.Children[1].Content[0].Type, "tool_use")
	}
	if root.Children[2].Content[0].Type != "tool_result" {
		t.Fatalf("root.Children[2].Content[0].Type = %q, want %q", root.Children[2].Content[0].Type, "tool_result")
	}
	if root.Children[3].Content[0].Text != "done" {
		t.Fatalf("root.Children[3].Content[0].Text = %q, want %q", root.Children[3].Content[0].Text, "done")
	}
	if root.Children[3].Usage == nil {
		t.Fatal("assistant message missing Usage after trailing token_count")
	}
	if root.Children[3].Usage.CostUSD <= 0 {
		t.Fatalf("assistant message CostUSD = %v, want > 0", root.Children[3].Usage.CostUSD)
	}
	var sumCost float64
	for _, child := range root.Children {
		if child.Usage != nil {
			sumCost += child.Usage.CostUSD
		}
	}
	if session.Stats.CostUSD != sumCost {
		t.Fatalf("session.Stats.CostUSD = %v, want %v", session.Stats.CostUSD, sumCost)
	}
}

func TestParseSessionBuildsTranscriptFromResponseItems(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "25", "rollout-20260325T100000-thread-3.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-25T10:00:00Z","type":"session_meta","payload":{"id":"thread-3","timestamp":"2026-03-25T10:00:00Z","cwd":"/tmp/work/project-gamma","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-03-25T10:00:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/work/project-gamma","approval_policy":"never","sandbox_policy":"danger_full_access","model":"gpt-5.4"}}
{"timestamp":"2026-03-25T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"check response_item tools","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-25T10:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\",\"workdir\":\"/tmp/work/project-gamma\"}","call_id":"call-1"}}
{"timestamp":"2026-03-25T10:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"Command: pwd\nOutput: /tmp/work/project-gamma\n"}}
{"timestamp":"2026-03-25T10:00:04Z","type":"response_item","payload":{"type":"custom_tool_call","status":"completed","name":"apply_patch","input":"*** Begin Patch\n*** End Patch\n","call_id":"call-2"}}
{"timestamp":"2026-03-25T10:00:05Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call-2","output":"{\"output\":\"Success\",\"metadata\":{\"exit_code\":0}}"}}
{"timestamp":"2026-03-25T10:00:06Z","type":"compacted","payload":{"message":"older context summarized","replacement_history":[]}}
{"timestamp":"2026-03-25T10:00:07Z","type":"event_msg","payload":{"type":"agent_message","message":"done"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error = %v", err)
	}

	if session.Summary != "check response_item tools" {
		t.Fatalf("session.Summary = %q, want %q", session.Summary, "check response_item tools")
	}
	if session.Stats.UserPrompts != 1 {
		t.Fatalf("session.Stats.UserPrompts = %d, want 1", session.Stats.UserPrompts)
	}
	if session.Stats.ToolCalls != 2 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 2", session.Stats.ToolCalls)
	}
	if len(session.RootMessages) != 2 {
		t.Fatalf("len(session.RootMessages) = %d, want 2", len(session.RootMessages))
	}

	root := session.RootMessages[0]
	if root.Kind != "user_prompt" {
		t.Fatalf("root.Kind = %q, want user_prompt", root.Kind)
	}
	if len(root.Children) != 5 {
		t.Fatalf("len(root.Children) = %d, want 5", len(root.Children))
	}
	if root.Children[0].Content[0].Type != "tool_use" || root.Children[0].Content[0].ToolName != "Bash" {
		t.Fatalf("root.Children[0] = %+v, want Bash tool_use", root.Children[0].Content[0])
	}
	if root.Children[1].Content[0].Type != "tool_result" {
		t.Fatalf("root.Children[1].Content[0].Type = %q, want %q", root.Children[1].Content[0].Type, "tool_result")
	}
	if root.Children[2].Content[0].Type != "tool_use" || root.Children[2].Content[0].ToolName != "ApplyPatch" {
		t.Fatalf("root.Children[2] = %+v, want ApplyPatch tool_use", root.Children[2].Content[0])
	}
	if root.Children[3].Content[0].Type != "tool_result" {
		t.Fatalf("root.Children[3].Content[0].Type = %q, want %q", root.Children[3].Content[0].Type, "tool_result")
	}
	if root.Children[4].Content[0].Text != "done" {
		t.Fatalf("root.Children[4].Content[0].Text = %q, want %q", root.Children[4].Content[0].Text, "done")
	}

	compact := session.RootMessages[1]
	if !compact.IsCompacted {
		t.Fatalf("compact.IsCompacted = %v, want true", compact.IsCompacted)
	}
	if compact.Content[0].Text != "older context summarized" {
		t.Fatalf("compact.Content[0].Text = %q, want %q", compact.Content[0].Text, "older context summarized")
	}
}

func TestParseSessionHandlesWebSearchAndReasoningResponseItems(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "25", "rollout-20260325T120000-thread-msg.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-25T12:00:00Z","type":"session_meta","payload":{"id":"thread-msg","timestamp":"2026-03-25T12:00:00Z","cwd":"/tmp/work/project-msg","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-03-25T12:00:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/work/project-msg","approval_policy":"never","sandbox_policy":"danger_full_access","model":"gpt-5"}}
{"timestamp":"2026-03-25T12:00:01Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system prompt"}]}}
{"timestamp":"2026-03-25T12:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
{"timestamp":"2026-03-25T12:00:03Z","type":"event_msg","payload":{"type":"user_message","message":"hello","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-25T12:00:04Z","type":"response_item","payload":{"type":"reasoning","summary":["thinking about it"],"content":null,"encrypted_content":"gAAAA..."}}
{"timestamp":"2026-03-25T12:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"Hi there"}}
{"timestamp":"2026-03-25T12:00:06Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi there"}]}}
{"timestamp":"2026-03-25T12:00:07Z","type":"response_item","payload":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"test query","queries":["test query"]}}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error = %v", err)
	}

	// Stats: 1 user_message from event_msg, 1 agent_message from event_msg
	// response_item/message should NOT be double-counted
	if session.Stats.UserPrompts != 1 {
		t.Fatalf("session.Stats.UserPrompts = %d, want 1", session.Stats.UserPrompts)
	}
	if session.Stats.MessageCount != 2 {
		t.Fatalf("session.Stats.MessageCount = %d, want 2", session.Stats.MessageCount)
	}
	// 1 web_search_call
	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}

	// Tree: root[0] = user "hello"
	//   children: reasoning, assistant "Hi there", WebSearch tool_use
	if len(session.RootMessages) != 1 {
		t.Fatalf("len(session.RootMessages) = %d, want 1", len(session.RootMessages))
	}

	root := session.RootMessages[0]
	if root.Kind != "user_prompt" {
		t.Fatalf("root.Kind = %q, want user_prompt", root.Kind)
	}
	if len(root.Children) != 3 {
		t.Fatalf("len(root.Children) = %d, want 3", len(root.Children))
	}
	if root.Children[0].Content[0].Type != "thinking" {
		t.Fatalf("child[0] type = %q, want thinking", root.Children[0].Content[0].Type)
	}
	if root.Children[1].Content[0].Text != "Hi there" {
		t.Fatalf("child[1] text = %q, want %q", root.Children[1].Content[0].Text, "Hi there")
	}
	if root.Children[2].Content[0].ToolName != "WebSearch" {
		t.Fatalf("child[2] tool = %q, want WebSearch", root.Children[2].Content[0].ToolName)
	}
}

func TestQuickParseSessionDoesNotDoubleCountResponseItemMessage(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "03", "25", "rollout-20260325T130000-thread-qp.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-03-25T13:00:00Z","type":"session_meta","payload":{"id":"thread-qp","timestamp":"2026-03-25T13:00:00Z","cwd":"/tmp/work/project-qp","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-03-25T13:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-03-25T13:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"response"}}
{"timestamp":"2026-03-25T13:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"response"}]}}
{"timestamp":"2026-03-25T13:00:04Z","type":"response_item","payload":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"test query","queries":["test query"]}}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	session := projects[0].Sessions[0]
	// event_msg counts: 1 user + 1 agent = 2 messages
	// response_item/message should NOT add to count
	if session.Stats.MessageCount != 2 {
		t.Fatalf("session.Stats.MessageCount = %d, want 2", session.Stats.MessageCount)
	}
	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
}

func TestQuickParseSessionDeduplicatesResponseItemAndEventMsgToolCalls(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "02", "rollout-20260402T211000-thread-qp-dedup.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-02T21:10:00Z","type":"session_meta","payload":{"id":"thread-qp-dedup","timestamp":"2026-04-02T21:10:00Z","cwd":"/tmp/test-qp-dedup","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-02T21:10:01Z","type":"event_msg","payload":{"type":"user_message","message":"test quick parse dedup","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-02T21:10:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pwd\"}","call_id":"call-qp-dup1"}}
{"timestamp":"2026-04-02T21:10:03Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"call-qp-dup1","turn_id":"turn-1","command":["pwd"],"cwd":"/tmp/test-qp-dedup","stdout":"/tmp/test-qp-dedup\n","stderr":"","exit_code":0,"status":"completed","duration":"PT0.01S"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	session := projects[0].Sessions[0]
	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1 (should not double-count response_item + event_msg)", session.Stats.ToolCalls)
	}
}

func TestQuickParseSessionDeduplicatesWebSearchCallAndEnd(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "03", "rollout-20260403T101000-thread-qp-web.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-03T10:10:00Z","type":"session_meta","payload":{"id":"thread-qp-web","timestamp":"2026-04-03T10:10:00Z","cwd":"/tmp/test-qp-web","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-03T10:10:01Z","type":"event_msg","payload":{"type":"user_message","message":"search once","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-03T10:10:02Z","type":"response_item","payload":{"type":"web_search_call","call_id":"call-web-1","status":"completed","action":{"type":"search","query":"golang testing","queries":["golang testing"]}}}
{"timestamp":"2026-04-03T10:10:03Z","type":"event_msg","payload":{"type":"web_search_end","call_id":"call-web-1","turn_id":"turn-1","query":"golang testing","status":"completed","results":"found 5 results"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	session := projects[0].Sessions[0]
	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
}

func TestParseSessionDeduplicatesWebSearchCallAndEnd(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archivedDir := filepath.Join(home, "archived_sessions")

	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "03", "rollout-20260403T111000-thread-parse-web.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-03T11:10:00Z","type":"session_meta","payload":{"id":"thread-parse-web","timestamp":"2026-04-03T11:10:00Z","cwd":"/tmp/test-parse-web","originator":"codex_cli_rs","cli_version":"0.116.0","source":"cli","model_provider":"openai"}}
{"timestamp":"2026-04-03T11:10:00Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/test-parse-web","approval_policy":"never","sandbox_policy":"danger_full_access","model":"gpt-5"}}
{"timestamp":"2026-04-03T11:10:01Z","type":"event_msg","payload":{"type":"user_message","message":"search once","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-03T11:10:02Z","type":"response_item","payload":{"type":"web_search_call","call_id":"call-web-1","status":"completed","action":{"type":"search","query":"golang testing","queries":["golang testing"]}}}
{"timestamp":"2026-04-03T11:10:03Z","type":"event_msg","payload":{"type":"web_search_end","call_id":"call-web-1","turn_id":"turn-1","query":"golang testing","status":"completed","results":"found 5 results"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error = %v", err)
	}

	if session.Stats.ToolCalls != 1 {
		t.Fatalf("session.Stats.ToolCalls = %d, want 1", session.Stats.ToolCalls)
	}
	if len(session.RootMessages) != 1 {
		t.Fatalf("len(session.RootMessages) = %d, want 1", len(session.RootMessages))
	}

	root := session.RootMessages[0]
	toolUses := 0
	toolResults := 0
	for _, child := range root.Children {
		for _, block := range child.Content {
			if block.ToolName != "WebSearch" {
				continue
			}
			switch block.Type {
			case "tool_use":
				toolUses++
			case "tool_result":
				toolResults++
			}
		}
	}

	if toolUses != 1 {
		t.Fatalf("WebSearch tool uses = %d, want 1", toolUses)
	}
	if toolResults != 1 {
		t.Fatalf("WebSearch tool results = %d, want 1", toolResults)
	}
}

// TestParseSession_DedupesConsecutiveUserMessage guards against the
// Codex 0.120.0+ rollout quirk where the first user_message is
// emitted twice with byte-identical payload and timestamp. Without
// dedupe, the session page renders the same prompt twice at the top
// and users think the UI is broken.
func TestParseSession_DedupesConsecutiveUserMessage(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "14", "rollout-dupe.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-14T10:00:00Z","type":"session_meta","payload":{"id":"dupe-thread","timestamp":"2026-04-14T10:00:00Z","cwd":"/tmp/work","originator":"codex","cli_version":"0.120.0"}}
{"timestamp":"2026-04-14T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"please review PR #7","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-14T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"please review PR #7","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-14T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"ok"}}
{"timestamp":"2026-04-14T10:00:30Z","type":"event_msg","payload":{"type":"user_message","message":"please review PR #7","images":[],"local_images":[],"text_elements":[]}}
`)
	backend := NewWithDirs(home, sessionsDir, filepath.Join(home, "archived_sessions"))
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession error: %v", err)
	}
	// First two user_message events (identical + same-timestamp) collapse
	// to one. The third user_message 29s later with the same text does
	// NOT dedupe because it's outside the 2-second window — user
	// legitimately re-asked.
	if got := session.Stats.UserPrompts; got != 2 {
		t.Fatalf("UserPrompts = %d, want 2 (dedupe the instant replay, keep the real re-ask)", got)
	}
	if len(session.RootMessages) != 2 {
		t.Fatalf("RootMessages = %d, want 2", len(session.RootMessages))
	}
	if session.RootMessages[0].Content[0].Text != "please review PR #7" {
		t.Errorf("first root content wrong: %q", session.RootMessages[0].Content[0].Text)
	}
}

// TestParseSession_DoesNotDedupDistinctUserMessages guards the other
// direction: two same-second user_messages with DIFFERENT text must
// both survive.
func TestParseSession_DoesNotDedupDistinctUserMessages(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	rolloutPath := filepath.Join(sessionsDir, "2026", "04", "14", "rollout-distinct.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-14T10:00:00Z","type":"session_meta","payload":{"id":"distinct","cwd":"/tmp/work","cli_version":"0.120.0"}}
{"timestamp":"2026-04-14T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"first question","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-14T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"second question","images":[],"local_images":[],"text_elements":[]}}
`)
	backend := NewWithDirs(home, sessionsDir, filepath.Join(home, "archived_sessions"))
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession error: %v", err)
	}
	if got := session.Stats.UserPrompts; got != 2 {
		t.Fatalf("UserPrompts = %d, want 2 (different text should not dedupe)", got)
	}
}

func writeRollout(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
