package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestAnalyzeNilSession(t *testing.T) {
	result := Analyze(nil)
	if result.Kind != TraceKind {
		t.Fatalf("kind: got %q", result.Kind)
	}
	if result.Stats.TurnCount != 0 {
		t.Fatalf("turns: got %d, want 0", result.Stats.TurnCount)
	}
}

// TestAnalyzeSegmentsSaySteps is the core contract of trace v2: an
// autonomous turn (one prompt, many narration+action cycles) must
// come back as one turn with one step per narration block, tools
// attributed to the step they followed.
func TestAnalyzeSegmentsSaySteps(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "steps",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "improve the web, too slow"}}},
			// Step 1: narrate, then read.
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Let me profile the render path first.\nMore detail here."},
					{Type: "tool_use", ToolName: "Read", ToolID: "t1", ToolInput: map[string]any{"file_path": "/w/server.go"}},
				}},
			{UUID: "r1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t1"}}},
			// Step 2: narrate the finding, then edit; the edit fails.
			{UUID: "a2", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Root cause found: discovery re-parses everything. Fixing."},
					{Type: "tool_use", ToolName: "Edit", ToolID: "t2", ToolInput: map[string]any{"file_path": "/w/project.go"}},
				}},
			{UUID: "r2", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(4 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t2", IsError: true}}},
		},
	}

	result := Analyze(session)
	if result.Stats.TurnCount != 1 {
		t.Fatalf("turns: got %d, want 1", result.Stats.TurnCount)
	}
	turn := result.Turns[0]
	if len(turn.Steps) != 2 {
		t.Fatalf("steps: got %d, want 2: %+v", len(turn.Steps), turn.Steps)
	}

	s1, s2 := turn.Steps[0], turn.Steps[1]
	if !strings.HasPrefix(s1.Narration, "Let me profile") {
		t.Fatalf("step1 narration: %q", s1.Narration)
	}
	if s1.ToolCounts["Read"] != 1 {
		t.Fatalf("step1 tools: %+v", s1.ToolCounts)
	}
	if !strings.HasPrefix(s2.Narration, "Root cause found") {
		t.Fatalf("step2 narration: %q", s2.Narration)
	}
	if s2.ToolCounts["Edit"] != 1 {
		t.Fatalf("step2 tools: %+v", s2.ToolCounts)
	}
	// The failed result for t2 must be attributed to step 2.
	if s1.Errors != 0 || s2.Errors != 1 {
		t.Fatalf("error attribution: s1=%d s2=%d", s1.Errors, s2.Errors)
	}
	if turn.Errors != 1 || result.Stats.ToolErrors != 1 {
		t.Fatalf("turn errors: %d, stats: %d", turn.Errors, result.Stats.ToolErrors)
	}
	// Rollups.
	if len(turn.FilesEdited) != 1 || turn.FilesEdited[0] != "/w/project.go" {
		t.Fatalf("files edited: %v", turn.FilesEdited)
	}
	if len(turn.FilesRead) != 1 || turn.FilesRead[0] != "/w/server.go" {
		t.Fatalf("files read: %v", turn.FilesRead)
	}
	if turn.ToolCounts["Read"] != 1 || turn.ToolCounts["Edit"] != 1 {
		t.Fatalf("turn tool counts: %+v", turn.ToolCounts)
	}
	if result.Stats.StepCount != 2 {
		t.Fatalf("step count: %d", result.Stats.StepCount)
	}
}

func TestAnalyzeMultipleTurnsAndCompactBoundary(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "turns",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "first ask"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "done one"}}},
			{UUID: "c1", Kind: parser.KindCompactSummary, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "compacted"}}},
			{UUID: "u2", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "second ask"}}},
			{UUID: "a2", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(4 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: "done two"}}},
		},
	}

	result := Analyze(session)
	if result.Stats.TurnCount != 2 {
		t.Fatalf("turns: got %d, want 2", result.Stats.TurnCount)
	}
	if result.Turns[0].UserText != "first ask" || result.Turns[1].UserText != "second ask" {
		t.Fatalf("turn texts: %q / %q", result.Turns[0].UserText, result.Turns[1].UserText)
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
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Spawning a reviewer."},
					{Type: "tool_use", ToolName: "Agent", ToolID: "tool-1", ToolInput: map[string]any{"subagent_type": "Explore"}},
				}},
			{UUID: "tr1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content:        []parser.ContentBlock{{Type: "tool_result", ToolID: "tool-1"}},
				SubAgentResult: &parser.SubAgentResultData{AgentID: "agent-1", AgentType: "Explore", Status: "completed", TotalToolUseCount: 1}},
			{UUID: "sc-a1", Kind: parser.KindAssistant, Type: "assistant", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Read", ToolInput: map[string]any{"file_path": "internal/fold/types.go"}}}},
		},
	}

	result := Analyze(session)
	if result.Stats.TurnCount != 1 {
		t.Fatalf("turn count: got %d, want 1", result.Stats.TurnCount)
	}
	if len(result.Sidechains) != 1 {
		t.Fatalf("top-level sidechains: got %d, want 1", len(result.Sidechains))
	}
	// Full per-call evidence lives once at the top level...
	top := result.Sidechains[0]
	if len(top.FilesRead) != 1 || top.FilesRead[0] != "internal/fold/types.go" {
		t.Fatalf("top-level sidechain read files: got %v", top.FilesRead)
	}
	// ...while the step carries a light reference keyed by agent_id,
	// attributed to the step that launched the agent.
	steps := result.Turns[0].Steps
	if len(steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(steps))
	}
	if len(steps[0].Sidechains) != 1 {
		t.Fatalf("step sidechains: got %d, want 1", len(steps[0].Sidechains))
	}
	ref := steps[0].Sidechains[0]
	if ref.AgentID != "agent-1" {
		t.Fatalf("sidechain ref agent id: got %q", ref.AgentID)
	}
	if len(ref.FilesRead) != 0 || len(ref.ToolCallEvidence) != 0 {
		t.Fatalf("step sidechain must be a light ref, got files=%v evidence=%d", ref.FilesRead, len(ref.ToolCallEvidence))
	}
	if !ref.TranscriptOmitted {
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
*** End Patch`
	paths := extractPaths(map[string]any{"patch": patch})
	if len(paths) != 3 {
		t.Fatalf("patch paths: %v", paths)
	}

	paths = extractPaths(map[string]any{"command": "go test ./... > /w/out.log 2>&1"})
	if len(paths) != 1 || paths[0] != "/w/out.log" {
		t.Fatalf("redirect paths: %v", paths)
	}
}

func TestCleanBoundedTextStripsANSI(t *testing.T) {
	got, truncated := cleanBoundedText("Set model to \x1b[1mFable 5\x1b[22m done", maxUserTextRunes)
	if truncated {
		t.Fatal("unexpected truncation")
	}
	if got != "Set model to Fable 5 done" {
		t.Fatalf("ANSI not stripped: %q", got)
	}
}

func TestCleanBoundedTextCondensesCommandXML(t *testing.T) {
	in := "<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args>claude-fable-5</command-args>\n<local-command-stdout>Set model to Fable 5</local-command-stdout>"
	got, _ := cleanBoundedText(in, maxUserTextRunes)
	want := "command: /model claude-fable-5\nstdout: Set model to Fable 5"
	if got != want {
		t.Fatalf("command XML not condensed:\n got: %q\nwant: %q", got, want)
	}
}

func TestCleanBoundedTextBoundsLongText(t *testing.T) {
	long := strings.Repeat("head ", 1000) + "TAIL-MARKER"
	got, truncated := cleanBoundedText(long, maxUserTextRunes)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if len([]rune(got)) > maxUserTextRunes+120 {
		t.Fatalf("bounded text too long: %d runes", len([]rune(got)))
	}
	if !strings.Contains(got, "chars omitted") {
		t.Fatal("missing omission marker")
	}
	if !strings.Contains(got, "TAIL-MARKER") {
		t.Fatal("tail lost — head+tail excerpt must keep the ending")
	}
}

func TestBuildOutlineAndRenderText(t *testing.T) {
	now := time.Date(2026, 7, 6, 16, 10, 0, 0, time.UTC)
	session := &parser.Session{
		ID:        "outline-test-session",
		Provider:  "claude-code",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "improve the web\nmore lines here"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Profiling first — rendering may not be the problem.\nDetails follow."},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t1", ToolInput: map[string]any{"command": "curl localhost"}},
				}},
		},
	}

	outline := BuildOutline(Analyze(session))
	if outline.Kind != OutlineKind {
		t.Fatalf("kind: %q", outline.Kind)
	}
	if len(outline.Turns) != 1 {
		t.Fatalf("outline turns: %d", len(outline.Turns))
	}
	ot := outline.Turns[0]
	if ot.UserText != "improve the web" {
		t.Fatalf("outline user text: %q", ot.UserText)
	}
	if len(ot.Steps) != 1 || !strings.HasPrefix(ot.Steps[0].Headline, "Profiling first") {
		t.Fatalf("outline steps: %+v", ot.Steps)
	}
	if strings.Contains(ot.Steps[0].Headline, "Details follow") {
		t.Fatal("headline must be first line only")
	}
	if ot.Tools != 1 {
		t.Fatalf("outline turn tools: %d", ot.Tools)
	}

	text := RenderOutlineText(outline)
	if !strings.Contains(text, "session outline-") {
		t.Fatalf("text missing session header:\n%s", text)
	}
	if !strings.Contains(text, "u: improve the web") {
		t.Fatalf("text missing user line:\n%s", text)
	}
	if !strings.Contains(text, "1. Profiling first") {
		t.Fatalf("text missing step line:\n%s", text)
	}
}
