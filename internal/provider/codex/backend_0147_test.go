package codex

import (
	"path/filepath"
	"testing"
)

func TestQuickParseSessionCodex0147ItemCompletedMessages(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	rolloutPath := filepath.Join(sessionsDir, "2026", "08", "17", "rollout-0147.jsonl")
	writeRollout(t, rolloutPath, codex0147Rollout)

	backend := NewWithDirs(home, sessionsDir, filepath.Join(home, "archived_sessions"))
	projects, err := backend.DiscoverProjects()
	if err != nil {
		t.Fatalf("DiscoverProjects() error = %v", err)
	}
	if len(projects) != 1 || len(projects[0].Sessions) != 1 {
		t.Fatalf("discovered projects/sessions = %d/%d, want 1/1", len(projects), len(projects[0].Sessions))
	}

	session := projects[0].Sessions[0]
	if session.Summary != "actual user prompt" {
		t.Fatalf("Summary = %q, want actual user prompt", session.Summary)
	}
	if session.Stats.MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", session.Stats.MessageCount)
	}
	if session.Stats.UserPrompts != 1 {
		t.Fatalf("UserPrompts = %d, want 1", session.Stats.UserPrompts)
	}
	if session.Model != "gpt-5.6-sol" {
		t.Fatalf("Model = %q, want gpt-5.6-sol", session.Model)
	}
}

func TestParseSessionCodex0147UsesCanonicalTurnItems(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	rolloutPath := filepath.Join(sessionsDir, "2026", "08", "17", "rollout-0147.jsonl")
	writeRollout(t, rolloutPath, codex0147Rollout)

	backend := NewWithDirs(home, sessionsDir, filepath.Join(home, "archived_sessions"))
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error = %v", err)
	}

	if session.Stats.MessageCount != 2 || session.Stats.UserPrompts != 1 {
		t.Fatalf("message stats = %d/%d, want 2 messages and 1 prompt",
			session.Stats.MessageCount, session.Stats.UserPrompts)
	}
	if len(session.RootMessages) != 1 {
		t.Fatalf("RootMessages = %d, want 1", len(session.RootMessages))
	}
	root := session.RootMessages[0]
	if root.UUID != "user-item-1" {
		t.Fatalf("user UUID = %q, want stable item id", root.UUID)
	}
	if got := root.Content[0].Text; got != "actual user prompt" {
		t.Fatalf("user text = %q, want actual user prompt", got)
	}
	if len(root.Children) != 1 {
		t.Fatalf("user children = %d, want 1", len(root.Children))
	}
	assistant := root.Children[0]
	if assistant.UUID != "assistant-item-1" {
		t.Fatalf("assistant UUID = %q, want stable item id", assistant.UUID)
	}
	if got := assistant.Content[0].Text; got != "actual assistant reply" {
		t.Fatalf("assistant text = %q, want actual assistant reply", got)
	}
}

// Codex 0.147 persists provider request/response messages and canonical
// item_completed turn items together. The response_item user record includes
// injected instruction envelopes, so it must never become the visible prompt.
const codex0147Rollout = `{"timestamp":"2026-08-18T04:45:37.600Z","type":"session_meta","payload":{"id":"thread-0147","timestamp":"2026-08-18T04:45:37.600Z","cwd":"/tmp/work/project-0147","originator":"codex-tui","cli_version":"0.147.0","source":"cli","model_provider":"openai"},"ordinal":0}
{"timestamp":"2026-08-18T04:45:37.610Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"private injected instruction envelope"}]},"ordinal":1}
{"timestamp":"2026-08-18T04:45:37.620Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/work/project-0147","model":"gpt-5.6-sol"},"ordinal":2}
{"timestamp":"2026-08-18T04:45:37.630Z","type":"event_msg","payload":{"type":"user_message","message":"legacy duplicate prompt","images":[],"local_images":[]},"ordinal":3}
{"timestamp":"2026-08-18T04:45:37.640Z","type":"event_msg","payload":{"type":"item_completed","thread_id":"thread-0147","turn_id":"turn-1","item":{"type":"UserMessage","id":"user-item-1","content":[{"type":"text","text":"actual user prompt","text_elements":[]}]},"completed_at_ms":1787028337640},"ordinal":4}
{"timestamp":"2026-08-18T04:45:37.650Z","type":"event_msg","payload":{"type":"agent_message","message":"legacy duplicate reply"},"ordinal":5}
{"timestamp":"2026-08-18T04:45:37.660Z","type":"response_item","payload":{"type":"message","id":"response-assistant-1","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"actual assistant reply"}]},"ordinal":6}
{"timestamp":"2026-08-18T04:45:37.670Z","type":"event_msg","payload":{"type":"item_completed","thread_id":"thread-0147","turn_id":"turn-1","item":{"type":"AgentMessage","id":"assistant-item-1","content":[{"type":"Text","text":"actual assistant reply"}],"phase":"final_answer"},"completed_at_ms":1787028337670},"ordinal":7}
`
