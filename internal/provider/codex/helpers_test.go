package codex

import (
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestNormalizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"exec_command", "Bash"},
		{"shell_command", "Bash"},
		{"shell", "Bash"},
		{"apply_patch", "ApplyPatch"},
		{"write_stdin", "WriteStdin"},
		{"update_plan", "UpdatePlan"},
		{"", "Tool"},
		{"  ", "Tool"},
		{"custom_tool", "custom_tool"},
		{"read_file", "read_file"},
		{"spawn_agent", "spawn_agent"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeToolName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeToolName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFallbackToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "Tool"},
		{"  ", "Tool"},
		{"MyTool", "MyTool"},
	}

	for _, tt := range tests {
		result := fallbackToolName(tt.input)
		if result != tt.expected {
			t.Errorf("fallbackToolName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		isNil    bool
		isString bool
	}{
		{"empty", "", true, false},
		{"whitespace", "   ", true, false},
		{"plain string", "hello world", false, true},
		{"json object", `{"key":"value"}`, false, false},
		{"json array", `[1,2,3]`, false, false},
		{"invalid json", `{broken`, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseJSONString(tt.input)
			if tt.isNil && result != nil {
				t.Fatalf("expected nil, got %v", result)
			}
			if tt.isString {
				if _, ok := result.(string); !ok {
					t.Fatalf("expected string, got %T", result)
				}
			}
			if !tt.isNil && !tt.isString {
				if _, ok := result.(string); ok {
					t.Fatal("expected parsed JSON, got string")
				}
			}
		})
	}
}

func TestParseEmbeddedJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		isString bool
	}{
		{"non-string passthrough", 42, false},
		{"empty string", "", true},
		{"plain string", "hello", true},
		{"json object", `{"key":"value"}`, false},
		{"json array", `[1,2]`, false},
		{"invalid json with brace", `{broken`, true},
		{"no json prefix", "plain text no braces", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEmbeddedJSON(tt.input)
			if tt.isString {
				if _, ok := result.(string); !ok {
					t.Fatalf("expected string result, got %T", result)
				}
			}
		})
	}
}

func TestChooseToolOutput(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected string
	}{
		{"first non-empty", []string{"", "  ", "output"}, "output"},
		{"already first", []string{"first", "second"}, "first"},
		{"all empty", []string{"", "  ", ""}, ""},
		{"no values", []string{}, ""},
		{"single value", []string{"only"}, "only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chooseToolOutput(tt.values...)
			if result != tt.expected {
				t.Errorf("chooseToolOutput() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStripUserMessagePrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"plain message", "plain message"},
		{"  padded  ", "padded"},
		{userMessagePrefix + "actual message", "actual message"},
		{userMessagePrefix, ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripUserMessagePrefix(tt.input)
			if result != tt.expected {
				t.Errorf("stripUserMessagePrefix(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUserMessagePreview(t *testing.T) {
	tests := []struct {
		name     string
		payload  userMessagePayload
		expected string
	}{
		{"text message", userMessagePayload{Message: "hello"}, "hello"},
		{"empty with images", userMessagePayload{Message: "", Images: []string{"img1"}}, imageOnlyMessagePlaceholder},
		{"empty with local images", userMessagePayload{Message: "", LocalImages: []string{"/path/img.png"}}, imageOnlyMessagePlaceholder},
		{"completely empty", userMessagePayload{Message: ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := userMessagePreview(tt.payload)
			if result != tt.expected {
				t.Errorf("userMessagePreview() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCompactMessageText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"summary text", "summary text"},
		{"", "Context compacted"},
		{"   ", "Context compacted"},
	}

	for _, tt := range tests {
		result := compactMessageText(tt.input)
		if result != tt.expected {
			t.Errorf("compactMessageText(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestShouldReplaceSession(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name      string
		existing  *parser.Session
		candidate *parser.Session
		expected  bool
	}{
		{
			name:      "nil existing always replaced",
			existing:  nil,
			candidate: &parser.Session{},
			expected:  true,
		},
		{
			name:      "candidate newer end time",
			existing:  &parser.Session{EndTime: earlier, FilePath: "/sessions/s1.jsonl"},
			candidate: &parser.Session{EndTime: now, FilePath: "/sessions/s2.jsonl"},
			expected:  true,
		},
		{
			name:      "candidate older end time",
			existing:  &parser.Session{EndTime: now, FilePath: "/sessions/s1.jsonl"},
			candidate: &parser.Session{EndTime: earlier, FilePath: "/sessions/s2.jsonl"},
			expected:  false,
		},
		{
			name:      "same end time candidate newer start",
			existing:  &parser.Session{StartTime: earlier, EndTime: now, FilePath: "/sessions/s1.jsonl"},
			candidate: &parser.Session{StartTime: now, EndTime: now, FilePath: "/sessions/s2.jsonl"},
			expected:  true,
		},
		{
			name:      "archived existing replaced by active",
			existing:  &parser.Session{EndTime: now, FilePath: "/archived_sessions/s1.jsonl"},
			candidate: &parser.Session{EndTime: earlier, FilePath: "/sessions/s2.jsonl"},
			expected:  true,
		},
		{
			name:      "active existing not replaced by archived",
			existing:  &parser.Session{EndTime: earlier, FilePath: "/sessions/s1.jsonl"},
			candidate: &parser.Session{EndTime: now, FilePath: "/archived_sessions/s2.jsonl"},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldReplaceSession(tt.existing, tt.candidate)
			if result != tt.expected {
				t.Errorf("shouldReplaceSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMatchSession(t *testing.T) {
	session := &parser.Session{ID: "abc12345-full-uuid"}

	tests := []struct {
		query    string
		expected bool
	}{
		{"abc12345-full-uuid", true},
		{"abc12345", true},
		{"abc", true},
		{"xyz", false},
		{"12345-full-uuid", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := matchSession(session, tt.query)
			if result != tt.expected {
				t.Errorf("matchSession(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isZero  bool
	}{
		{"RFC3339", "2026-03-24T10:00:00Z", false},
		{"RFC3339Nano", "2026-03-24T10:00:00.123456789Z", false},
		{"invalid", "not-a-timestamp", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestamp(tt.input)
			if tt.isZero && !result.IsZero() {
				t.Fatalf("expected zero time, got %v", result)
			}
			if !tt.isZero && result.IsZero() {
				t.Fatal("expected non-zero time")
			}
		})
	}
}

func TestDurationSeconds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		expected float64
	}{
		{"normal", now, now.Add(10 * time.Second), 10},
		{"zero start", time.Time{}, now, 0},
		{"zero end", now, time.Time{}, 0},
		{"end before start", now, now.Add(-time.Second), 0},
		{"same time", now, now, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := durationSeconds(tt.start, tt.end)
			if result != tt.expected {
				t.Errorf("durationSeconds() = %f, want %f", result, tt.expected)
			}
		})
	}
}

func TestRecordTimestampValue(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var first, last time.Time

	recordTimestampValue(time.Time{}, &first, &last)
	if !first.IsZero() {
		t.Fatal("zero time should not update first")
	}

	recordTimestampValue(t2, &first, &last)
	if !first.Equal(t2) {
		t.Fatal("first should be set to t2")
	}
	if !last.Equal(t2) {
		t.Fatal("last should be set to t2")
	}

	recordTimestampValue(t1, &first, &last)
	if !first.Equal(t1) {
		t.Fatal("first should update to t1 (earlier than t2)")
	}
	if !last.Equal(t2) {
		t.Fatal("last should remain t2 (t1 is not later)")
	}

	recordTimestampValue(t3, &first, &last)
	if !last.Equal(t3) {
		t.Fatal("last should update to t3")
	}
}

func TestCodexBuildMessageTree(t *testing.T) {
	messages := []*parser.Message{
		{UUID: "u1", Kind: parser.KindUserPrompt},
		{UUID: "a1", Kind: parser.KindAssistant, Content: []parser.ContentBlock{{Type: "text", Text: "response"}}},
		{UUID: "t1", Kind: parser.KindAssistant, Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Bash"}}},
		{UUID: "tr1", Kind: parser.KindToolResult, Content: []parser.ContentBlock{{Type: "tool_result"}}},
		{UUID: "c1", Kind: parser.KindCompactSummary, IsCompacted: true},
		{UUID: "u2", Kind: parser.KindUserPrompt},
		{UUID: "a2", Kind: parser.KindAssistant},
	}

	roots := buildMessageTree(messages)

	if len(roots) != 3 {
		t.Fatalf("expected 3 roots (u1, compact, u2), got %d", len(roots))
	}
	if roots[0].UUID != "u1" {
		t.Fatalf("roots[0] = %q, want u1", roots[0].UUID)
	}
	if len(roots[0].Children) != 3 {
		t.Fatalf("u1 should have 3 children (a1, t1, tr1), got %d", len(roots[0].Children))
	}
	if roots[1].UUID != "c1" {
		t.Fatalf("roots[1] = %q, want c1 (compact)", roots[1].UUID)
	}
	if roots[2].UUID != "u2" {
		t.Fatalf("roots[2] = %q, want u2", roots[2].UUID)
	}
	if len(roots[2].Children) != 1 {
		t.Fatalf("u2 should have 1 child (a2), got %d", len(roots[2].Children))
	}
}

func TestCodexBuildMessageTreeNoAnchor(t *testing.T) {
	messages := []*parser.Message{
		{UUID: "a1", Kind: parser.KindAssistant},
		{UUID: "a2", Kind: parser.KindAssistant},
	}
	roots := buildMessageTree(messages)
	if len(roots) != 2 {
		t.Fatalf("messages without anchor should become roots, got %d", len(roots))
	}
}

func TestNewToolUseMessageFallbackID(t *testing.T) {
	msg := newToolUseMessage(42, time.Now(), "gpt-5", "Bash", "", nil)
	if msg.Content[0].ToolID != "codex-tool-42" {
		t.Fatalf("expected fallback tool ID, got %q", msg.Content[0].ToolID)
	}
}

func TestNewToolUseMessagePreservesID(t *testing.T) {
	msg := newToolUseMessage(1, time.Now(), "gpt-5", "Bash", "call-123", nil)
	if msg.Content[0].ToolID != "call-123" {
		t.Fatalf("expected preserved tool ID, got %q", msg.Content[0].ToolID)
	}
}

func TestNewToolResultMessageError(t *testing.T) {
	msg := newToolResultMessage(1, time.Now(), "gpt-5", "Bash", "call-1", "error output", true)
	if !msg.Content[0].IsError {
		t.Fatal("expected IsError = true")
	}
}

func TestToolPairMessages(t *testing.T) {
	pair := toolPairMessages(1, time.Now(), "gpt-5", "Bash", "call-1", "cmd", "output", false)
	if len(pair) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(pair))
	}
	if pair[0].Content[0].Type != "tool_use" {
		t.Fatal("first should be tool_use")
	}
	if pair[1].Content[0].Type != "tool_result" {
		t.Fatal("second should be tool_result")
	}
	if pair[0].Content[0].ToolID != pair[1].Content[0].ToolID {
		t.Fatal("tool IDs should match")
	}
}

func TestIsArchivedSession(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/home/user/.codex/sessions/2026/rollout.jsonl", false},
		{"/home/user/.codex/archived_sessions/2026/rollout.jsonl", true},
	}

	for _, tt := range tests {
		result := isArchivedSession(tt.path)
		if result != tt.expected {
			t.Errorf("isArchivedSession(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestParseSessionRawJSONOnCodexMessages(t *testing.T) {
	home := t.TempDir()
	sessionsDir := home + "/sessions"
	archivedDir := home + "/archived_sessions"

	rolloutPath := sessionsDir + "/2026/04/01/rollout-20260401T100000-thread-raw.jsonl"
	writeRollout(t, rolloutPath, `{"timestamp":"2026-04-01T10:00:00Z","type":"session_meta","payload":{"id":"thread-raw","timestamp":"2026-04-01T10:00:00Z","cwd":"/tmp/rawtest","originator":"codex","cli_version":"0.2.0","source":"chat","model_provider":"openai"}}
{"timestamp":"2026-04-01T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"test raw json","images":[],"local_images":[],"text_elements":[]}}
{"timestamp":"2026-04-01T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"got it"}}
`)

	backend := NewWithDirs(home, sessionsDir, archivedDir)
	session, err := backend.ParseSession(rolloutPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if len(session.RootMessages) != 1 {
		t.Fatalf("expected 1 root, got %d", len(session.RootMessages))
	}
	root := session.RootMessages[0]
	if root.RawJSON == "" {
		t.Fatal("user message RawJSON should be populated")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	if root.Children[0].RawJSON == "" {
		t.Fatal("agent message RawJSON should be populated")
	}
}

func TestChooseSummary(t *testing.T) {
	tests := []struct {
		name       string
		threadName string
		firstUser  string
		expected   string
	}{
		{"thread name wins", "named thread", "user prompt", "named thread"},
		{"fallback to user", "", "user prompt", "user prompt"},
		{"whitespace thread ignored", "   ", "user prompt", "user prompt"},
		{"both empty", "", "", "(no summary)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chooseSummary(tt.threadName, tt.firstUser)
			if result != tt.expected {
				t.Errorf("chooseSummary(%q, %q) = %q, want %q", tt.threadName, tt.firstUser, result, tt.expected)
			}
		})
	}
}
