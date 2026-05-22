package fold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestAnalyzeNilSession(t *testing.T) {
	result := Analyze(nil)
	if result == nil {
		t.Fatal("expected non-nil result for nil session")
	}
	if result.Stats.ExchangeCount != 0 {
		t.Errorf("expected 0 exchanges, got %d", result.Stats.ExchangeCount)
	}
}

func TestAnalyzeEmptySession(t *testing.T) {
	session := &parser.Session{}
	result := Analyze(session)
	if result.Stats.ExchangeCount != 0 {
		t.Errorf("expected 0 exchanges, got %d", result.Stats.ExchangeCount)
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
	if result.Stats.ExchangeCount != 1 {
		t.Errorf("exchanges: got %d, want 1", result.Stats.ExchangeCount)
	}
	if result.Stats.FilesEdited != 1 {
		t.Errorf("files edited: got %d, want 1", result.Stats.FilesEdited)
	}
	if result.Kind != TraceKind {
		t.Errorf("kind: got %q, want %q", result.Kind, TraceKind)
	}

	exchange := result.Exchanges[0]
	if exchange.UserText != "Add a health check endpoint" {
		t.Errorf("user text: got %q", exchange.UserText)
	}
	if len(exchange.FilesEdited) != 1 || exchange.FilesEdited[0] != "src/server.go" {
		t.Errorf("files edited: got %v", exchange.FilesEdited)
	}
}

func TestAnalyzeSkipsSidechainAnchors(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "sidechain-test",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		Stats:     parser.SessionStats{AgentSidechains: 1},
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "Review the implementation"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "I'll delegate review."}}},
			{UUID: "sc-u1", Kind: parser.KindUserPrompt, Type: "user", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Sub-agent prompt"}}},
			{UUID: "sc-a1", Kind: parser.KindAssistant, Type: "assistant", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "Sub-agent response"}}},
		},
	}

	result := Analyze(session)
	if result.Stats.ExchangeCount != 1 {
		t.Fatalf("exchange count: got %d, want 1", result.Stats.ExchangeCount)
	}
	if !result.Stats.HasSidechains {
		t.Fatal("expected sidechain stats to be preserved")
	}
}

func TestAnalyzeAttachesSidechainEvidence(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "sidechain-evidence",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		Stats:     parser.SessionStats{AgentSidechains: 1},
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "Review the implementation"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Agent", ToolID: "tool-1", ToolInput: map[string]any{"subagent_type": "Explore"}}}},
			{UUID: "tr1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content:        []parser.ContentBlock{{Type: "tool_result", ToolID: "tool-1"}},
				SubAgentResult: &parser.SubAgentResultData{AgentID: "agent-1", AgentType: "Explore", Status: "completed", TotalToolUseCount: 1}},
			{UUID: "sc-a1", Kind: parser.KindAssistant, Type: "assistant", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Read", ToolInput: map[string]any{"file_path": "internal/fold/types.go"}}}},
		},
	}

	result := Analyze(session)
	if result.Stats.ExchangeCount != 1 {
		t.Fatalf("exchange count: got %d, want 1", result.Stats.ExchangeCount)
	}
	sidechains := result.Exchanges[0].Sidechains
	if len(sidechains) != 1 {
		t.Fatalf("sidechains: got %d, want 1", len(sidechains))
	}
	if len(result.Sidechains) != 1 {
		t.Fatalf("top-level sidechains: got %d, want 1", len(result.Sidechains))
	}
	if len(sidechains[0].FilesRead) != 1 || sidechains[0].FilesRead[0] != "internal/fold/types.go" {
		t.Fatalf("sidechain read files: got %v", sidechains[0].FilesRead)
	}
	if !sidechains[0].TranscriptOmitted {
		t.Fatal("expected sidechain transcript to be marked omitted")
	}
}

func TestExtractPathsFromPatchAndBashRedirect(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: src/new.go
+package main
*** Update File: src/old.go
@@
-old
+new
*** Delete File: tmp/gone.txt
*** End Patch
`
	paths := extractPaths(patch)
	want := []string{"src/new.go", "src/old.go", "tmp/gone.txt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("patch paths: got %v, want %v", paths, want)
	}

	paths = extractPaths(map[string]any{"cmd": "printf hi > out.txt && echo no >/dev/null && echo x >> logs/run.log"})
	want = []string{"logs/run.log", "out.txt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("redirect paths: got %v, want %v", paths, want)
	}

	paths = extractPaths(map[string]any{"argv": []any{"sh", "-c", "printf hi > quoted.txt"}})
	want = []string{"quoted.txt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("argv redirect paths: got %v, want %v", paths, want)
	}

	paths = extractPaths(map[string]any{"cmd": "printf hi > out.txt", "workdir": "/repo/subdir"})
	want = []string{"/repo/subdir/out.txt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("workdir redirect paths: got %v, want %v", paths, want)
	}
}

func TestCollectWorkspaceContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agent Rules\n\nUse trace first."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ccx", "knowledge", "decisions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ccx", "knowledge", "decisions", "trace-first.md"), []byte("# Trace First\n\nFacts before judgment."), 0644); err != nil {
		t.Fatal(err)
	}

	result := &TraceResult{}
	if err := CollectWorkspaceContext(result, dir); err != nil {
		t.Fatal(err)
	}
	if result.Stats.WorkspaceDocs != 1 {
		t.Fatalf("workspace docs: got %d, want 1", result.Stats.WorkspaceDocs)
	}
	if result.Stats.KnowledgeEntries != 1 {
		t.Fatalf("knowledge entries: got %d, want 1", result.Stats.KnowledgeEntries)
	}
	if result.Workspace.Documents[0].Kind != "agent-instructions" {
		t.Fatalf("doc kind: got %q", result.Workspace.Documents[0].Kind)
	}
	if result.Workspace.Documents[0].Excerpt != "" {
		t.Fatalf("expected metadata-only context document, got excerpt %q", result.Workspace.Documents[0].Excerpt)
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
	if result.Stats.CorrectionSignals != 1 {
		t.Errorf("corrections: got %d, want 1", result.Stats.CorrectionSignals)
	}
}

func TestFirstExchangeInstructionIsNotCorrection(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "initial-instruction",
		StartTime: now,
		EndTime:   now.Add(time.Minute),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "Do not use echo redirects for edits"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Second),
				Content: []parser.ContentBlock{{Type: "text", Text: "Understood."}}},
		},
	}

	result := Analyze(session)
	if result.Stats.CorrectionSignals != 0 {
		t.Fatalf("corrections: got %d, want 0", result.Stats.CorrectionSignals)
	}
}

func TestRenderHTMLNotEmpty(t *testing.T) {
	result := &TraceResult{
		Kind:      TraceKind,
		Session:   SessionMeta{ID: "abc123", Summary: "Test"},
		Stats:     TraceStats{ExchangeCount: 1},
		Exchanges: []ExchangeEvidence{{Index: 1, UserText: "hello"}},
	}
	html := RenderHTML(result)
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}
	if len(html) < 100 {
		t.Errorf("HTML suspiciously short: %d bytes", len(html))
	}
}
