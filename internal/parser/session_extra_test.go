package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveLogicalParentDirect(t *testing.T) {
	parents := map[string]string{
		"b1": "u1",
	}
	result := resolveLogicalParent("b1", parents)
	if result != "u1" {
		t.Fatalf("resolveLogicalParent() = %q, want %q", result, "u1")
	}
}

func TestResolveLogicalParentChain(t *testing.T) {
	parents := map[string]string{
		"b3": "b2",
		"b2": "b1",
		"b1": "u1",
	}
	result := resolveLogicalParent("b3", parents)
	if result != "u1" {
		t.Fatalf("resolveLogicalParent(chain) = %q, want %q", result, "u1")
	}
}

func TestResolveLogicalParentCycleSafety(t *testing.T) {
	parents := map[string]string{
		"a": "b",
		"b": "c",
		"c": "a",
	}
	result := resolveLogicalParent("a", parents)
	// Should terminate within maxHops and not infinite-loop
	if result == "" {
		t.Fatal("expected non-empty result from cycle")
	}
}

func TestResolveLogicalParentSelfReference(t *testing.T) {
	parents := map[string]string{
		"a": "a",
	}
	result := resolveLogicalParent("a", parents)
	if result != "a" {
		t.Fatalf("self-reference should return self, got %q", result)
	}
}

func TestResolveLogicalParentNoMapping(t *testing.T) {
	parents := map[string]string{}
	result := resolveLogicalParent("u1", parents)
	if result != "u1" {
		t.Fatalf("no mapping should return original, got %q", result)
	}
}

func TestResolveLogicalParentEmptyTarget(t *testing.T) {
	parents := map[string]string{
		"a": "",
	}
	result := resolveLogicalParent("a", parents)
	if result != "a" {
		t.Fatalf("empty target should return original, got %q", result)
	}
}

func TestParseContentImage(t *testing.T) {
	content := []any{
		map[string]any{
			"type": "image",
			"source": map[string]any{
				"media_type": "image/png",
				"data":       "iVBORw0KGgo=",
			},
		},
	}
	result := parseContent(content)
	if len(result) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result))
	}
	if result[0].Type != "image" {
		t.Fatalf("expected image type, got %q", result[0].Type)
	}
	if result[0].MediaType != "image/png" {
		t.Fatalf("MediaType = %q, want image/png", result[0].MediaType)
	}
	if result[0].ImageData != "iVBORw0KGgo=" {
		t.Fatalf("ImageData not preserved")
	}
}

func TestParseContentNonArray(t *testing.T) {
	result := parseContent(42)
	if result != nil {
		t.Fatalf("expected nil for non-array/non-string, got %v", result)
	}
}

func TestParseContentMixedBlocks(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "intro"},
		map[string]any{"type": "thinking", "thinking": "hmm"},
		map[string]any{"type": "tool_use", "name": "Bash", "id": "t1", "input": map[string]any{"command": "ls"}},
		"not a map",
	}
	result := parseContent(content)
	if len(result) != 3 {
		t.Fatalf("expected 3 blocks (non-map skipped), got %d", len(result))
	}
}

func TestParseSessionRawJSONPreserved(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","timestamp":"2026-01-01T10:00:00.000Z","uuid":"u1","parentUuid":"","message":{"role":"user","content":"test prompt"},"sessionId":"sess-1","version":"2.1.0"}
{"type":"assistant","timestamp":"2026-01-01T10:00:01.000Z","uuid":"a1","parentUuid":"u1","requestId":"req_test123","message":{"role":"assistant","model":"claude-test","id":"msg_test456","content":[{"type":"text","text":"response"}]}}
`
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if len(session.RootMessages) != 1 {
		t.Fatalf("expected 1 root, got %d", len(session.RootMessages))
	}
	root := session.RootMessages[0]
	if root.RawJSON == "" {
		t.Fatal("RawJSON should be populated on user message")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.RawJSON == "" {
		t.Fatal("RawJSON should be populated on assistant message")
	}
	// Verify the raw JSON contains fields not in the parsed struct
	if !contains(child.RawJSON, "req_test123") {
		t.Fatal("RawJSON should contain requestId")
	}
	if !contains(child.RawJSON, "msg_test456") {
		t.Fatal("RawJSON should contain message.id")
	}
}

func TestParseSessionSessionMetadata(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","timestamp":"2026-02-01T09:00:00.000Z","uuid":"u1","message":{"role":"user","content":"init"},"slug":"melodic-cooking-piglet","version":"2.1.88","gitBranch":"feature/tests","cwd":"/home/dev/project"}
{"type":"assistant","timestamp":"2026-02-01T09:00:01.000Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"ok"}}
`
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Slug != "melodic-cooking-piglet" {
		t.Fatalf("Slug = %q, want melodic-cooking-piglet", session.Slug)
	}
	if session.Version != "2.1.88" {
		t.Fatalf("Version = %q, want 2.1.88", session.Version)
	}
	if session.GitBranch != "feature/tests" {
		t.Fatalf("GitBranch = %q, want feature/tests", session.GitBranch)
	}
	if session.CWD != "/home/dev/project" {
		t.Fatalf("CWD = %q, want /home/dev/project", session.CWD)
	}
}

func TestQuickParseSessionMetadata(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","timestamp":"2026-03-01T12:00:00.000Z","uuid":"u1","message":{"role":"user","content":"quick parse test"},"slug":"test-slug","version":"3.0.0","gitBranch":"main","cwd":"/opt/app"}
{"type":"assistant","timestamp":"2026-03-01T12:00:05.000Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"response","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}}
`
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	summary, start, end, stats, meta := quickParseSession(sessionPath)
	if summary != "quick parse test" {
		t.Fatalf("summary = %q, want quick parse test", summary)
	}
	if start.IsZero() || end.IsZero() {
		t.Fatal("timestamps should not be zero")
	}
	if end.Sub(start) != 5*time.Second {
		t.Fatalf("duration = %v, want 5s", end.Sub(start))
	}
	if stats.InputTokens != 100 {
		t.Fatalf("InputTokens = %d, want 100", stats.InputTokens)
	}
	if stats.OutputTokens != 50 {
		t.Fatalf("OutputTokens = %d, want 50", stats.OutputTokens)
	}
	if stats.CacheReadTokens != 20 {
		t.Fatalf("CacheReadTokens = %d, want 20", stats.CacheReadTokens)
	}
	if stats.CacheCreateTokens != 10 {
		t.Fatalf("CacheCreateTokens = %d, want 10", stats.CacheCreateTokens)
	}
	if meta.Slug != "test-slug" {
		t.Fatalf("meta.Slug = %q, want test-slug", meta.Slug)
	}
	if meta.Version != "3.0.0" {
		t.Fatalf("meta.Version = %q, want 3.0.0", meta.Version)
	}
	if meta.GitBranch != "main" {
		t.Fatalf("meta.GitBranch = %q, want main", meta.GitBranch)
	}
	if meta.CWD != "/opt/app" {
		t.Fatalf("meta.CWD = %q, want /opt/app", meta.CWD)
	}
}

func TestQuickParseSessionSkipsXMLCommands(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","timestamp":"2026-01-01T10:00:00.000Z","uuid":"u1","message":{"role":"user","content":"<command-name>/help</command-name>"}}
{"type":"user","timestamp":"2026-01-01T10:00:01.000Z","uuid":"u2","message":{"role":"user","content":"actual user prompt"}}
`
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	summary, _, _, _, _ := quickParseSession(sessionPath)
	if summary != "actual user prompt" {
		t.Fatalf("summary = %q, want actual user prompt (should skip XML command)", summary)
	}
}

func TestQuickParseSessionEmptyFile(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(sessionPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	summary, _, _, _, _ := quickParseSession(sessionPath)
	if summary != "(no summary)" {
		t.Fatalf("summary = %q, want (no summary)", summary)
	}
}

func TestQuickParseSessionMissingFile(t *testing.T) {
	summary, _, _, _, _ := quickParseSession("/nonexistent/path.jsonl")
	if summary != "(no summary)" {
		t.Fatalf("summary = %q, want (no summary)", summary)
	}
}

func TestBuildMessageTreeOrphanBecomesRoot(t *testing.T) {
	messages := []*Message{
		{UUID: "orphan", ParentUUID: "nonexistent-parent"},
	}
	roots := buildMessageTree(messages)
	if len(roots) != 1 {
		t.Fatalf("orphan should become root, got %d roots", len(roots))
	}
	if roots[0].UUID != "orphan" {
		t.Fatalf("root UUID = %q, want orphan", roots[0].UUID)
	}
}

func TestBuildMessageTreeEmptyUUID(t *testing.T) {
	messages := []*Message{
		{UUID: "", ParentUUID: ""},
	}
	roots := buildMessageTree(messages)
	if len(roots) != 1 {
		t.Fatalf("empty UUID should still work, got %d roots", len(roots))
	}
}

func TestClassifyMessageCommandInRawContent(t *testing.T) {
	msg := &Message{
		Type:    "user",
		Content: []ContentBlock{{Type: "text", Text: "just text"}},
	}
	raw := rawMessage{
		Type:    "user",
		Message: messagePayload{Content: "<command-name>/compact</command-name>"},
	}
	kind := classifyMessage(msg, raw)
	if kind != KindCommand {
		t.Fatalf("expected KindCommand from raw string content, got %s", kind)
	}
	if !msg.IsCommand {
		t.Fatal("msg.IsCommand should be true")
	}
	if msg.CommandName != "/compact" {
		t.Fatalf("CommandName = %q, want /compact", msg.CommandName)
	}
}

func TestSessionFilterableInterface(t *testing.T) {
	ts := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	s := &Session{
		Provider:  "claude-code",
		StartTime: ts,
		EndTime:   ts.Add(time.Hour),
		Summary:   "Fix auth bug",
		Model:     "claude-opus-4-6",
		Stats:     SessionStats{MessageCount: 42},
	}
	if s.GetProvider() != "claude-code" {
		t.Error("GetProvider")
	}
	if !s.GetStartTime().Equal(ts) {
		t.Error("GetStartTime")
	}
	if !s.GetEndTime().Equal(ts.Add(time.Hour)) {
		t.Error("GetEndTime")
	}
	if s.GetSummary() != "Fix auth bug" {
		t.Error("GetSummary")
	}
	if s.GetModel() != "claude-opus-4-6" {
		t.Error("GetModel")
	}
	if s.GetMessageCount() != 42 {
		t.Error("GetMessageCount")
	}
}

func TestExtractModel(t *testing.T) {
	messages := []*Message{
		{Type: "user", Model: ""},
		{Type: "assistant", Model: "claude-opus-4-6"},
		{Type: "assistant", Model: "claude-sonnet-4-6"},
	}
	got := extractModel(messages)
	if got != "claude-opus-4-6" {
		t.Errorf("extractModel() = %q, want claude-opus-4-6 (first assistant)", got)
	}
}

func TestExtractModelEmpty(t *testing.T) {
	got := extractModel([]*Message{})
	if got != "" {
		t.Errorf("extractModel() = %q, want empty", got)
	}
}

func TestExtractModelSkipsUserMessages(t *testing.T) {
	messages := []*Message{
		{Type: "user", Model: "user-model"},
	}
	got := extractModel(messages)
	if got != "" {
		t.Errorf("extractModel() = %q, want empty (user messages skipped)", got)
	}
}

func TestParseSessionTokenUsageAccumulation(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test.jsonl")

	content := `{"type":"user","timestamp":"2026-01-01T10:00:00.000Z","uuid":"u1","message":{"role":"user","content":"msg1"}}
{"type":"assistant","timestamp":"2026-01-01T10:00:01.000Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"r1","usage":{"input_tokens":50,"output_tokens":25,"cache_read_input_tokens":10,"cache_creation_input_tokens":5}}}
{"type":"user","timestamp":"2026-01-01T10:00:02.000Z","uuid":"u2","message":{"role":"user","content":"msg2"}}
{"type":"assistant","timestamp":"2026-01-01T10:00:03.000Z","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"r2","usage":{"input_tokens":60,"output_tokens":30,"cache_read_input_tokens":15,"cache_creation_input_tokens":8}}}
`
	if err := os.WriteFile(sessionPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("ParseSession() error: %v", err)
	}

	if session.Stats.InputTokens != 110 {
		t.Fatalf("InputTokens = %d, want 110", session.Stats.InputTokens)
	}
	if session.Stats.OutputTokens != 55 {
		t.Fatalf("OutputTokens = %d, want 55", session.Stats.OutputTokens)
	}
	if session.Stats.CacheReadTokens != 25 {
		t.Fatalf("CacheReadTokens = %d, want 25", session.Stats.CacheReadTokens)
	}
	if session.Stats.CacheCreateTokens != 13 {
		t.Fatalf("CacheCreateTokens = %d, want 13", session.Stats.CacheCreateTokens)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
