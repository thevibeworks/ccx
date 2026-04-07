package render

import (
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestBriefSessionPreservesCompacted(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{Kind: parser.KindCompactSummary, IsCompacted: true, Content: []parser.ContentBlock{{Type: "text", Text: "context compacted"}}},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.RootMessages))
	}
	if result.RootMessages[0].Kind != parser.KindCompactSummary {
		t.Fatalf("expected KindCompactSummary, got %s", result.RootMessages[0].Kind)
	}
}

func TestBriefSessionFiltersMeta(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{Kind: parser.KindMeta, IsMeta: true, Content: []parser.ContentBlock{{Type: "text", Text: "system instructions"}}},
			{Kind: parser.KindSystem, Content: []parser.ContentBlock{{Type: "text", Text: "system event"}}},
			{Kind: parser.KindCommand, IsCommand: true, Content: []parser.ContentBlock{{Type: "text", Text: "<command-name>/help</command-name>"}}},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result.RootMessages))
	}
}

func TestBriefSessionKeepsUserAndAssistantText(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				Kind:    parser.KindUserPrompt,
				Type:    "user",
				Content: []parser.ContentBlock{{Type: "text", Text: "How do I fix this?"}},
			},
			{
				Kind:    parser.KindAssistant,
				Type:    "assistant",
				Content: []parser.ContentBlock{{Type: "text", Text: "Here's the fix."}},
			},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.RootMessages))
	}
}

func TestBriefSessionStripsThinkingAndToolUseBlocks(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				Kind: parser.KindAssistant,
				Type: "assistant",
				Content: []parser.ContentBlock{
					{Type: "thinking", Text: "internal reasoning"},
					{Type: "text", Text: "visible response"},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t1"},
				},
			},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.RootMessages))
	}
	if len(result.RootMessages[0].Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.RootMessages[0].Content))
	}
	if result.RootMessages[0].Content[0].Text != "visible response" {
		t.Fatalf("expected visible response, got %q", result.RootMessages[0].Content[0].Text)
	}
}

func TestBriefSessionDropsAssistantWithNoText(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				Kind:    parser.KindAssistant,
				Type:    "assistant",
				Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Read"}},
			},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result.RootMessages))
	}
}

func TestBriefToolResultExtractsDelegationTool(t *testing.T) {
	for _, tool := range []string{"Agent", "Task", "Dispatch", "SubAgent", "TodoRead", "close_agent"} {
		t.Run(tool, func(t *testing.T) {
			msg := &parser.Message{
				UUID: "tr1",
				Kind: parser.KindToolResult,
				Type: "user",
				Content: []parser.ContentBlock{
					{Type: "tool_result", ToolName: tool, ToolResult: "agent concluded with summary"},
				},
			}
			result := briefToolResult(msg)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.Content[0].Text != "agent concluded with summary" {
				t.Fatalf("expected extracted text, got %q", result.Content[0].Text)
			}
			if result.Type != "assistant" {
				t.Fatalf("expected assistant type, got %q", result.Type)
			}
		})
	}
}

func TestBriefToolResultSkipsNonDelegationTool(t *testing.T) {
	msg := &parser.Message{
		Kind: parser.KindToolResult,
		Type: "user",
		Content: []parser.ContentBlock{
			{Type: "tool_result", ToolName: "Bash", ToolResult: "file listing"},
		},
	}
	result := briefToolResult(msg)
	if result != nil {
		t.Fatal("expected nil for non-delegation tool")
	}
}

func TestBriefToolResultExtractsMapResult(t *testing.T) {
	tests := []struct {
		name   string
		result any
		want   string
	}{
		{"result key", map[string]any{"result": "done"}, "done"},
		{"output key", map[string]any{"output": "completed"}, "completed"},
		{"summary key", map[string]any{"summary": "findings"}, "findings"},
		{"text key", map[string]any{"text": "content"}, "content"},
		{"message key", map[string]any{"message": "report"}, "report"},
		{"empty map", map[string]any{"other": "stuff"}, ""},
		{"empty string value", map[string]any{"result": ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractResultText(tt.result)
			if got != tt.want {
				t.Fatalf("extractResultText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBriefSidechainExtractsLastAssistantText(t *testing.T) {
	msg := &parser.Message{
		IsSidechain: true,
		Type:        "assistant",
		Kind:        parser.KindAssistant,
		Content:     []parser.ContentBlock{{Type: "tool_use", ToolName: "Read"}},
		Children: []*parser.Message{
			{
				Type:    "user",
				Kind:    parser.KindToolResult,
				Content: []parser.ContentBlock{{Type: "tool_result", ToolResult: "data"}},
				Children: []*parser.Message{
					{
						Type:    "assistant",
						Kind:    parser.KindAssistant,
						Content: []parser.ContentBlock{{Type: "text", Text: "final conclusion"}},
					},
				},
			},
		},
	}
	result := briefSidechain(msg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Content[0].Text != "final conclusion" {
		t.Fatalf("expected final conclusion, got %q", result.Content[0].Text)
	}
	if result.IsSidechain {
		t.Fatal("result should not be marked as sidechain")
	}
}

func TestBriefSidechainReturnsNilWhenNoAssistantText(t *testing.T) {
	msg := &parser.Message{
		IsSidechain: true,
		Type:        "assistant",
		Kind:        parser.KindAssistant,
		Content:     []parser.ContentBlock{{Type: "tool_use", ToolName: "Bash"}},
	}
	result := briefSidechain(msg)
	if result != nil {
		t.Fatal("expected nil when no assistant text in sidechain")
	}
}

func TestTextBlocksFiltersEmpty(t *testing.T) {
	blocks := []parser.ContentBlock{
		{Type: "text", Text: ""},
		{Type: "text", Text: "   "},
		{Type: "text", Text: "valid"},
		{Type: "tool_use", ToolName: "Bash"},
		{Type: "text", Text: "also valid"},
	}
	result := textBlocks(blocks)
	if len(result) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result))
	}
	if result[0].Text != "valid" || result[1].Text != "also valid" {
		t.Fatalf("unexpected blocks: %v", result)
	}
}

func TestFlattenTree(t *testing.T) {
	root := &parser.Message{
		UUID: "r",
		Children: []*parser.Message{
			{UUID: "c1", Children: []*parser.Message{
				{UUID: "gc1"},
			}},
			{UUID: "c2"},
		},
	}
	result := flattenTree(root)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	ids := make([]string, len(result))
	for i, m := range result {
		ids[i] = m.UUID
	}
	want := []string{"r", "c1", "gc1", "c2"}
	for i, id := range ids {
		if id != want[i] {
			t.Fatalf("index %d: got %q, want %q", i, id, want[i])
		}
	}
}

func TestBriefSessionRecursesChildren(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				Kind:    parser.KindUserPrompt,
				Type:    "user",
				Content: []parser.ContentBlock{{Type: "text", Text: "question"}},
				Children: []*parser.Message{
					{Kind: parser.KindMeta, IsMeta: true, Content: []parser.ContentBlock{{Type: "text", Text: "meta"}}},
					{Kind: parser.KindAssistant, Type: "assistant", Content: []parser.ContentBlock{{Type: "text", Text: "answer"}}},
				},
			},
		},
	}
	result := BriefSession(session)
	if len(result.RootMessages) != 1 {
		t.Fatalf("expected 1 root, got %d", len(result.RootMessages))
	}
	if len(result.RootMessages[0].Children) != 1 {
		t.Fatalf("expected 1 child (meta filtered), got %d", len(result.RootMessages[0].Children))
	}
	if result.RootMessages[0].Children[0].Content[0].Text != "answer" {
		t.Fatalf("expected answer child, got %q", result.RootMessages[0].Children[0].Content[0].Text)
	}
}

func TestBriefSessionDoesNotMutateOriginal(t *testing.T) {
	original := &parser.Session{
		ID: "test-session",
		RootMessages: []*parser.Message{
			{Kind: parser.KindMeta, IsMeta: true, Content: []parser.ContentBlock{{Type: "text", Text: "meta"}}},
			{Kind: parser.KindUserPrompt, Type: "user", Content: []parser.ContentBlock{{Type: "text", Text: "hi"}}},
		},
	}
	result := BriefSession(original)
	if len(original.RootMessages) != 2 {
		t.Fatal("original was mutated")
	}
	if len(result.RootMessages) != 1 {
		t.Fatalf("expected 1 in brief, got %d", len(result.RootMessages))
	}
	if result.ID != original.ID {
		t.Fatal("session ID not preserved")
	}
}
