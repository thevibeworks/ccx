package render

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func makeSessionWithNoise(t *testing.T) *parser.Session {
	t.Helper()
	start := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	mk := func(uuid string, kind parser.MessageKind, ts time.Time, text string) *parser.Message {
		return &parser.Message{
			UUID:      uuid,
			Type:      "user",
			Kind:      kind,
			Timestamp: ts,
			Content:   []parser.ContentBlock{{Type: "text", Text: text}},
		}
	}
	human1 := mk("u1", parser.KindUserPrompt, start, "fix the login bug")
	cmdOut := mk("c1", parser.KindCommandOutput, start.Add(time.Minute), "<local-command-stdout>Set model</local-command-stdout>")
	notif := mk("n1", parser.KindNotification, start.Add(2*time.Minute), "<task-notification>agent done</task-notification>")
	human2 := mk("u2", parser.KindUserPrompt, start.Add(3*time.Minute), "now add a test for it")
	// Compaction replay: same text AND same timestamp as human2, later in wire order.
	replay := mk("u2r", parser.KindUserPrompt, start.Add(3*time.Minute), "now add a test for it")
	sidechain := mk("s1", parser.KindUserPrompt, start.Add(4*time.Minute), "sidechain prompt")
	sidechain.IsSidechain = true
	assistant := &parser.Message{
		UUID: "a1", Type: "assistant", Kind: parser.KindAssistant,
		Timestamp: start.Add(5 * time.Minute),
		Content:   []parser.ContentBlock{{Type: "text", Text: "done"}},
	}
	return &parser.Session{
		ID:           "deadbeef99",
		StartTime:    start,
		RootMessages: []*parser.Message{human1, cmdOut, notif, human2, replay, sidechain, assistant},
	}
}

func TestExport_ShapeHumanOnlyHumanTurns(t *testing.T) {
	session := makeSessionWithNoise(t)
	out, err := Export(session, ExportOptions{Format: "md", Shape: ShapeHuman})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if !strings.Contains(out, "fix the login bug") || !strings.Contains(out, "now add a test for it") {
		t.Errorf("shape=human should include human turns, got: %s", out)
	}
	for _, noise := range []string{"local-command-stdout", "task-notification", "sidechain prompt", "done"} {
		if strings.Contains(out, noise) {
			t.Errorf("shape=human should exclude %q, got: %s", noise, out)
		}
	}
}

func TestExport_ShapeHumanDedupsCompactionReplays(t *testing.T) {
	session := makeSessionWithNoise(t)
	out, err := Export(session, ExportOptions{Format: "md", Shape: ShapeHuman})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if got := strings.Count(out, "now add a test for it"); got != 1 {
		t.Errorf("replayed turn should appear once, appeared %d times", got)
	}
	if !strings.Contains(out, "Turns: 2 (3 raw, 1 replay duplicates dropped)") {
		t.Errorf("header should report dedup, got: %s", out)
	}
}

func TestExport_ShapeHumanNumbersAndAnchorsTurns(t *testing.T) {
	session := makeSessionWithNoise(t)
	out, err := Export(session, ExportOptions{Format: "md", Shape: ShapeHuman})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if !strings.Contains(out, "## 1") || !strings.Contains(out, "## 2") {
		t.Errorf("turns should be numbered, got: %s", out)
	}
	if !strings.Contains(out, "u1") || !strings.Contains(out, "u2") {
		t.Errorf("turns should carry uuid anchors, got: %s", out)
	}
}

func TestExport_ShapeHumanRejectsNonMarkdown(t *testing.T) {
	session := makeSessionWithNoise(t)
	if _, err := Export(session, ExportOptions{Format: "html", Shape: ShapeHuman}); err == nil {
		t.Error("shape=human with html format should error")
	}
}
