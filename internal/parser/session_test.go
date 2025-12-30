package parser

import (
	"testing"
)

func TestClassifyMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		raw      rawMessage
		expected MessageKind
	}{
		{
			name:     "assistant message",
			msg:      &Message{Type: "assistant"},
			raw:      rawMessage{Type: "assistant"},
			expected: KindAssistant,
		},
		{
			name:     "system message",
			msg:      &Message{Type: "system"},
			raw:      rawMessage{Type: "system"},
			expected: KindSystem,
		},
		{
			name:     "compact summary",
			msg:      &Message{Type: "user"},
			raw:      rawMessage{Type: "user", IsCompactSummary: true},
			expected: KindCompactSummary,
		},
		{
			name:     "meta message",
			msg:      &Message{Type: "user"},
			raw:      rawMessage{Type: "user", IsMeta: true},
			expected: KindMeta,
		},
		{
			name: "tool result",
			msg: &Message{
				Type:    "user",
				Content: []ContentBlock{{Type: "tool_result"}},
			},
			raw:      rawMessage{Type: "user"},
			expected: KindToolResult,
		},
		{
			name: "user prompt",
			msg: &Message{
				Type:    "user",
				Content: []ContentBlock{{Type: "text", Text: "Hello"}},
			},
			raw:      rawMessage{Type: "user"},
			expected: KindUserPrompt,
		},
		{
			name: "command message",
			msg: &Message{
				Type:    "user",
				Content: []ContentBlock{{Type: "text", Text: "<command-name>/help</command-name>"}},
			},
			raw:      rawMessage{Type: "user"},
			expected: KindCommand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyMessage(tt.msg, tt.raw)
			if result != tt.expected {
				t.Errorf("classifyMessage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractCommandName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<command-name>/help</command-name>", "/help"},
		{"<command-name>/commit</command-name><command-args>-m fix</command-args>", "/commit"},
		{"no command here", ""},
		{"<command-name>", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractCommandName(tt.input)
		if result != tt.expected {
			t.Errorf("extractCommandName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractCommandArgs(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<command-args>-m fix bug</command-args>", "-m fix bug"},
		{"<command-name>/commit</command-name><command-args>message</command-args>", "message"},
		{"no args here", ""},
		{"<command-args>", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractCommandArgs(tt.input)
		if result != tt.expected {
			t.Errorf("extractCommandArgs(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestComputeStats(t *testing.T) {
	messages := []*Message{
		{Kind: KindUserPrompt},
		{Kind: KindAssistant, Content: []ContentBlock{{Type: "tool_use"}, {Type: "tool_use"}}},
		{Kind: KindUserPrompt},
		{Kind: KindAssistant},
		{Kind: KindToolResult},
		{Kind: KindCompactSummary, IsCompacted: true},
		{Kind: KindMeta},
		{Kind: KindAssistant, IsSidechain: true},
	}

	stats := computeStats(messages)

	if stats.UserPrompts != 2 {
		t.Errorf("UserPrompts = %d, want 2", stats.UserPrompts)
	}
	if stats.MessageCount != 5 { // 2 user + 3 assistant
		t.Errorf("MessageCount = %d, want 5", stats.MessageCount)
	}
	if stats.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", stats.ToolCalls)
	}
	if stats.Continuations != 1 {
		t.Errorf("Continuations = %d, want 1", stats.Continuations)
	}
	if stats.AgentSidechains != 1 {
		t.Errorf("AgentSidechains = %d, want 1", stats.AgentSidechains)
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name     string
		messages []*Message
		expected string
	}{
		{
			name:     "empty messages",
			messages: []*Message{},
			expected: "(no summary)",
		},
		{
			name: "first user prompt",
			messages: []*Message{
				{Kind: KindUserPrompt, Content: []ContentBlock{{Type: "text", Text: "Hello world"}}},
			},
			expected: "Hello world",
		},
		{
			name: "multiline - returns first line",
			messages: []*Message{
				{Kind: KindUserPrompt, Content: []ContentBlock{{Type: "text", Text: "First line\nSecond line"}}},
			},
			expected: "First line",
		},
		{
			name: "skips non-user messages",
			messages: []*Message{
				{Kind: KindAssistant, Content: []ContentBlock{{Type: "text", Text: "Response"}}},
				{Kind: KindUserPrompt, Content: []ContentBlock{{Type: "text", Text: "Actual prompt"}}},
			},
			expected: "Actual prompt",
		},
		{
			name: "skips tool results",
			messages: []*Message{
				{Kind: KindToolResult, Content: []ContentBlock{{Type: "text", Text: "Tool output"}}},
				{Kind: KindUserPrompt, Content: []ContentBlock{{Type: "text", Text: "User question"}}},
			},
			expected: "User question",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSummary(tt.messages)
			if result != tt.expected {
				t.Errorf("extractSummary() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildMessageTree(t *testing.T) {
	messages := []*Message{
		{UUID: "1", ParentUUID: ""},
		{UUID: "2", ParentUUID: "1"},
		{UUID: "3", ParentUUID: "1"},
		{UUID: "4", ParentUUID: "2"},
		{UUID: "5", ParentUUID: ""},
	}

	roots := buildMessageTree(messages)

	if len(roots) != 2 {
		t.Errorf("len(roots) = %d, want 2", len(roots))
	}

	// Check first root has 2 children
	if len(roots[0].Children) != 2 {
		t.Errorf("roots[0].Children = %d, want 2", len(roots[0].Children))
	}

	// Check nested child
	if len(roots[0].Children[0].Children) != 1 {
		t.Errorf("roots[0].Children[0].Children = %d, want 1", len(roots[0].Children[0].Children))
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/path/to/sessions/abc123.jsonl", "abc123"},
		{"abc123.jsonl", "abc123"},
		{"/a/b/c/uuid-here.jsonl", "uuid-here"},
	}

	for _, tt := range tests {
		result := extractSessionID(tt.input)
		if result != tt.expected {
			t.Errorf("extractSessionID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseContent(t *testing.T) {
	t.Run("nil content", func(t *testing.T) {
		result := parseContent(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("string content", func(t *testing.T) {
		result := parseContent("hello")
		if len(result) != 1 || result[0].Type != "text" || result[0].Text != "hello" {
			t.Errorf("unexpected result: %v", result)
		}
	})

	t.Run("array content with text", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "hello"},
		}
		result := parseContent(content)
		if len(result) != 1 || result[0].Text != "hello" {
			t.Errorf("unexpected result: %v", result)
		}
	})

	t.Run("array content with thinking", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "thinking", "thinking": "let me think..."},
		}
		result := parseContent(content)
		if len(result) != 1 || result[0].Text != "let me think..." {
			t.Errorf("unexpected result: %v", result)
		}
	})

	t.Run("array content with tool_use", func(t *testing.T) {
		content := []any{
			map[string]any{
				"type":  "tool_use",
				"name":  "read_file",
				"id":    "tool_123",
				"input": map[string]any{"path": "/tmp/test"},
			},
		}
		result := parseContent(content)
		if len(result) != 1 {
			t.Fatalf("expected 1 block, got %d", len(result))
		}
		if result[0].ToolName != "read_file" {
			t.Errorf("ToolName = %q, want read_file", result[0].ToolName)
		}
		if result[0].ToolID != "tool_123" {
			t.Errorf("ToolID = %q, want tool_123", result[0].ToolID)
		}
	})

	t.Run("array content with tool_result", func(t *testing.T) {
		content := []any{
			map[string]any{
				"type":        "tool_result",
				"tool_use_id": "tool_123",
				"content":     "file contents here",
				"is_error":    true,
			},
		}
		result := parseContent(content)
		if len(result) != 1 {
			t.Fatalf("expected 1 block, got %d", len(result))
		}
		if result[0].ToolID != "tool_123" {
			t.Errorf("ToolID = %q, want tool_123", result[0].ToolID)
		}
		if !result[0].IsError {
			t.Error("IsError should be true")
		}
	})
}

func TestCountToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected int
	}{
		{"nil", nil, 0},
		{"string", "hello", 0},
		{"empty array", []any{}, 0},
		{
			"one tool_use",
			[]any{map[string]any{"type": "tool_use"}},
			1,
		},
		{
			"mixed content",
			[]any{
				map[string]any{"type": "text"},
				map[string]any{"type": "tool_use"},
				map[string]any{"type": "tool_use"},
				map[string]any{"type": "thinking"},
			},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countToolCalls(tt.content)
			if result != tt.expected {
				t.Errorf("countToolCalls() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestExtractTextFromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "  hello  ", "hello"},
		{"empty array", []any{}, ""},
		{
			"single text block",
			[]any{map[string]any{"type": "text", "text": "hello"}},
			"hello",
		},
		{
			"multiple text blocks",
			[]any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "text", "text": "world"},
			},
			"hello world",
		},
		{
			"mixed with non-text",
			[]any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "tool_use"},
				map[string]any{"type": "text", "text": "world"},
			},
			"hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextFromContent(tt.content)
			if result != tt.expected {
				t.Errorf("extractTextFromContent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkComputeStats(b *testing.B) {
	messages := make([]*Message, 1000)
	for i := range messages {
		kind := KindUserPrompt
		if i%2 == 1 {
			kind = KindAssistant
		}
		messages[i] = &Message{
			Kind:    kind,
			Content: []ContentBlock{{Type: "tool_use"}, {Type: "text"}},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeStats(messages)
	}
}

func BenchmarkBuildMessageTree(b *testing.B) {
	messages := make([]*Message, 100)
	for i := range messages {
		parentUUID := ""
		if i > 0 {
			parentUUID = messages[i-1].UUID
		}
		messages[i] = &Message{
			UUID:       string(rune('a' + i)),
			ParentUUID: parentUUID,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildMessageTree(messages)
	}
}
