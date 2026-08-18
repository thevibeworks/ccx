package codex

import (
	"path/filepath"
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

// event_msg.turn_aborted (reason interrupted) is the human stopping a
// Codex turn — Codex's "[Request interrupted by user]". It must parse
// as a KindInterrupt marker (never a prompt) so trace counts it, and
// other abort reasons must not.
func TestParseSessionCodexTurnAbortedIsInterrupt(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	rolloutPath := filepath.Join(sessionsDir, "2026", "08", "18", "rollout-abort.jsonl")
	writeRollout(t, rolloutPath, `{"timestamp":"2026-08-18T10:00:00Z","type":"session_meta","payload":{"id":"abort-1","cwd":"/tmp/repo","timestamp":"2026-08-18T10:00:00Z"}}
{"timestamp":"2026-08-18T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"run the migration"}}
{"timestamp":"2026-08-18T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"Starting."}}
{"timestamp":"2026-08-18T10:00:03Z","type":"event_msg","payload":{"type":"turn_aborted","turn_id":"t1","reason":"interrupted"}}
{"timestamp":"2026-08-18T10:00:04Z","type":"event_msg","payload":{"type":"turn_aborted","turn_id":"t2","reason":"replaced"}}
`)
	backend := NewWithDirs(home, sessionsDir, filepath.Join(home, "archived_sessions"))
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	interrupts := 0
	prompts := 0
	for _, m := range parser.FlattenSessionMessages(session) {
		switch m.Kind {
		case parser.KindInterrupt:
			interrupts++
			if m.Type != "user" || m.Content[0].Text != "[Turn interrupted by user]" {
				t.Fatalf("interrupt message shape: %+v", m)
			}
		case parser.KindUserPrompt:
			prompts++
		}
	}
	if interrupts != 1 {
		t.Fatalf("interrupts = %d, want 1 (only reason=interrupted counts)", interrupts)
	}
	if prompts != 1 {
		t.Fatalf("prompts = %d, want 1 (the abort marker is not a prompt)", prompts)
	}
}
