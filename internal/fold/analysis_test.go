package fold

import (
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestAnalyzeNilSession(t *testing.T) {
	result := Analyze(nil)
	if result == nil {
		t.Fatal("expected non-nil result for nil session")
	}
	if result.Stats.TurnCount != 0 {
		t.Errorf("expected 0 turns, got %d", result.Stats.TurnCount)
	}
}

func TestAnalyzeEmptySession(t *testing.T) {
	session := &parser.Session{}
	result := Analyze(session)
	if result.Stats.TurnCount != 0 {
		t.Errorf("expected 0 turns, got %d", result.Stats.TurnCount)
	}
}

func TestAnalyzeBasicSession(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "test-session-123",
		Summary:   "Test session",
		Model:     "claude-opus-4-6",
		StartTime: now,
		EndTime:   now.Add(30 * time.Minute),
		RootMessages: []*parser.Message{
			{
				UUID:      "msg-1",
				Kind:      parser.KindUserPrompt,
				Type:      "user",
				Timestamp: now,
				Content:   []parser.ContentBlock{{Type: "text", Text: "Add a health check endpoint"}},
			},
			{
				UUID:      "msg-2",
				Kind:      parser.KindAssistant,
				Type:      "assistant",
				Timestamp: now.Add(1 * time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "I'll add a /health endpoint."},
					{Type: "tool_use", ToolName: "Edit", ToolInput: map[string]any{"file_path": "src/server.go"}},
				},
			},
		},
	}

	result := Analyze(session)

	if result.Session.ID != "test-session-123" {
		t.Errorf("session ID: got %q, want %q", result.Session.ID, "test-session-123")
	}
	if result.Stats.TurnCount != 1 {
		t.Errorf("turns: got %d, want 1", result.Stats.TurnCount)
	}
	if result.Stats.FilesEdited != 1 {
		t.Errorf("files edited: got %d, want 1", result.Stats.FilesEdited)
	}

	turn := result.Turns[0]
	if turn.UserText != "Add a health check endpoint" {
		t.Errorf("user text: got %q", turn.UserText)
	}
	if len(turn.FilesEdited) != 1 || turn.FilesEdited[0] != "src/server.go" {
		t.Errorf("files edited: got %v", turn.FilesEdited)
	}
}

func TestDetectCorrection(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Add a health check", false},
		{"No, don't use that approach", true},
		{"Actually, let's use SSE instead", true},
		{"Wait, that's wrong", true},
		{"Revert the last change", true},
		{"Sounds good, continue", false},
		{"I said use DuckDB not Postgres", true},
		{"Can you explain that?", false},
		{"Stop doing that", true},
	}

	for _, tt := range tests {
		got := detectCorrection(tt.text)
		if got != tt.want {
			t.Errorf("detectCorrection(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestAnalyzeCorrectionCounting(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "correction-test",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "Build auth middleware"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(1 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "I'll add JWT auth."}}},
			{UUID: "u2", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "No, don't use JWT. Use session cookies instead."}}},
			{UUID: "a2", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Switching to session cookies."}}},
		},
	}

	result := Analyze(session)
	if result.Stats.Corrections != 1 {
		t.Errorf("corrections: got %d, want 1", result.Stats.Corrections)
	}
}

func TestRenderHTMLNotEmpty(t *testing.T) {
	result := &FoldResult{
		Session: SessionMeta{ID: "abc123", Summary: "Test"},
		Stats:   FoldStats{TurnCount: 1},
		Turns:   []Turn{{Index: 1, UserText: "hello"}},
	}
	html := RenderHTML(result)
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}
	if len(html) < 100 {
		t.Errorf("HTML suspiciously short: %d bytes", len(html))
	}
}
