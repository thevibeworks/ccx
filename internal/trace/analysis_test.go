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

// TestAnalyzeSidechainReportStaysWhole guards the evidence contract for
// research sessions: the subagent's final report is often the whole
// value, so the top-level sidechain entry must carry it untruncated
// (ANSI-stripped), while the step-level light ref stays bounded.
func TestAnalyzeSidechainReportStaysWhole(t *testing.T) {
	now := time.Now()
	report := "\x1b[1mFindings:\x1b[0m " + strings.Repeat("evidence sentence. ", 200) // ~3800 runes
	session := &parser.Session{
		ID:        "sidechain-report",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		Stats:     parser.SessionStats{AgentSidechains: 1},
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "Research the topic"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Spawning a researcher."},
					{Type: "tool_use", ToolName: "Agent", ToolID: "tool-1", ToolInput: map[string]any{"subagent_type": "Explore"}},
				}},
			{UUID: "tr1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content:        []parser.ContentBlock{{Type: "tool_result", ToolID: "tool-1"}},
				SubAgentResult: &parser.SubAgentResultData{AgentID: "agent-1", AgentType: "Explore", Status: "completed"}},
			{UUID: "sc-a1", Kind: parser.KindAssistant, Type: "assistant", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: report}}},
		},
	}

	result := Analyze(session)
	if len(result.Sidechains) != 1 {
		t.Fatalf("top-level sidechains: got %d, want 1", len(result.Sidechains))
	}
	top := result.Sidechains[0]
	if len([]rune(top.Summary)) < 3000 {
		t.Fatalf("top-level summary truncated: %d runes", len([]rune(top.Summary)))
	}
	if strings.Contains(top.Summary, "\x1b") {
		t.Fatal("top-level summary must be ANSI-stripped")
	}
	ref := result.Turns[0].Steps[0].Sidechains[0]
	if got := len([]rune(ref.Summary)); got > 250 {
		t.Fatalf("step ref summary must stay bounded, got %d runes", got)
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

	outline := BuildOutline(Analyze(session), DefaultHeadlineWidth)
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

// TestBuildOutlineWidthControl: the headline cap is a parameter, not a
// constant — 0 disables truncation (the JSON consumers' escape hatch
// that previously required --full), and the cap applies identically to
// turn user text and step headlines.
func TestBuildOutlineWidthControl(t *testing.T) {
	now := time.Now()
	long := strings.TrimSpace(strings.Repeat("alpha beta ", 30)) // 329 runes
	session := &parser.Session{
		ID:        "width",
		StartTime: now,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: long}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{{Type: "text", Text: long}}},
		},
	}
	result := Analyze(session)

	def := BuildOutline(result, DefaultHeadlineWidth).Turns[0]
	if !strings.HasSuffix(def.UserText, "...") || len([]rune(def.UserText)) > DefaultHeadlineWidth+3 {
		t.Fatalf("default width must truncate: %d runes", len([]rune(def.UserText)))
	}

	full := BuildOutline(result, 0).Turns[0]
	if full.UserText != long || full.Steps[0].Headline != long {
		t.Fatalf("width 0 must not truncate: user=%d step=%d runes",
			len([]rune(full.UserText)), len([]rune(full.Steps[0].Headline)))
	}

	narrow := BuildOutline(result, 20).Turns[0]
	if len([]rune(narrow.UserText)) > 23 || len([]rune(narrow.Steps[0].Headline)) > 23 {
		t.Fatalf("width 20 must cap both headline kinds: user=%q step=%q",
			narrow.UserText, narrow.Steps[0].Headline)
	}
}

// TestAnalyzeTokenSplitAndAgentCost is the cost-auditability contract:
// cache tokens dominate real spend, so every turn must carry the full
// split, and sidechain spend must land in the total instead of
// vanishing — a $5 turn the outline can't explain is a $5 turn nobody
// trusts.
func TestAnalyzeTokenSplitAndAgentCost(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "token-split",
		StartTime: now,
		EndTime:   now.Add(10 * time.Minute),
		Stats:     parser.SessionStats{AgentSidechains: 1},
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "one-line ask"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Usage: &parser.MessageUsage{InputTokens: 200, OutputTokens: 500,
					CacheReadTokens: 90_000, CacheCreateTokens: 45_000, CostUSD: 1.10},
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Spawning an agent."},
					{Type: "tool_use", ToolName: "Agent", ToolID: "t1", ToolInput: map[string]any{"subagent_type": "Explore"}},
				}},
			{UUID: "sc1", Kind: parser.KindAssistant, Type: "assistant", IsSidechain: true, AgentID: "agent-1", Timestamp: now.Add(2 * time.Minute),
				Usage:   &parser.MessageUsage{InputTokens: 1000, OutputTokens: 300, CostUSD: 0.40},
				Content: []parser.ContentBlock{{Type: "text", Text: "agent working"}}},
			{UUID: "tr1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(3 * time.Minute),
				Content:        []parser.ContentBlock{{Type: "tool_result", ToolID: "t1"}},
				SubAgentResult: &parser.SubAgentResultData{AgentID: "agent-1", AgentType: "Explore", Status: "completed"}},
		},
	}

	result := Analyze(session)
	turn := result.Turns[0]
	if turn.CacheReadTokens != 90_000 || turn.CacheCreateTokens != 45_000 {
		t.Fatalf("turn cache tokens: r=%d w=%d", turn.CacheReadTokens, turn.CacheCreateTokens)
	}
	if turn.CostUSD != 1.10 {
		t.Fatalf("turn main cost: %v", turn.CostUSD)
	}
	if turn.AgentsCostUSD != 0.40 {
		t.Fatalf("turn agents cost: %v", turn.AgentsCostUSD)
	}
	if got := result.Stats.TotalCostUSD; got != 1.50 {
		t.Fatalf("total cost must be all-in (main+agents): %v", got)
	}
	if result.Stats.AgentsCostUSD != 0.40 {
		t.Fatalf("stats agents cost: %v", result.Stats.AgentsCostUSD)
	}
	if result.Stats.CacheReadTokens != 90_000 || result.Stats.CacheCreateTokens != 45_000 {
		t.Fatalf("stats cache tokens: r=%d w=%d",
			result.Stats.CacheReadTokens, result.Stats.CacheCreateTokens)
	}

	outline := BuildOutline(result, DefaultHeadlineWidth)
	ot := outline.Turns[0]
	if ot.CacheReadTokens != 90_000 || ot.CacheCreateTokens != 45_000 || ot.AgentsCostUSD != 0.40 {
		t.Fatalf("outline turn split lost: %+v", ot)
	}
	text := RenderOutlineText(outline)
	if !strings.Contains(text, "cache 90k r/45k w") {
		t.Fatalf("turn badge must show the cache split:\n%s", text)
	}
	if !strings.Contains(text, "$1.50 ($0.40 agents)") {
		t.Fatalf("header cost must be all-in with the agents share:\n%s", text)
	}
	if !strings.Contains(text, "tokens: 200 in/500 out, cache 90k r/45k w") {
		t.Fatalf("header must carry the session token split:\n%s", text)
	}
}

func TestAnalyzeAttributesResultErrorsToCallEvidence(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "err-attr",
		StartTime: now,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "run and read"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Editing, running, reading."},
					{Type: "tool_use", ToolName: "Edit", ToolID: "t_edit", ToolInput: map[string]any{"file_path": "/w/a.go"}},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t_bash", ToolInput: map[string]any{"command": "make test"}},
					{Type: "tool_use", ToolName: "Read", ToolID: "t_read", ToolInput: map[string]any{"file_path": "/w/b.go"}},
				}},
			{UUID: "r1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_edit"}}},
			{UUID: "r2", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_bash", IsError: true}}},
			{UUID: "r3", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(4 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_read", IsError: true}}},
		},
	}

	turn := Analyze(session).Turns[0]
	if turn.Errors != 2 {
		t.Fatalf("turn errors: got %d, want 2", turn.Errors)
	}

	// Errors live on results in the log; the analyzer must surface
	// them on the issuing call's evidence so failure lists agree with
	// error counts. The failed Read is not a mutating tool, so its
	// evidence is materialized on error.
	failed := map[string]bool{}
	for _, step := range turn.Steps {
		for _, m := range step.Mutations {
			if m.IsError {
				failed[m.ToolID] = true
			}
			if m.ToolID == "t_edit" && m.IsError {
				t.Error("successful edit must not be marked as error")
			}
		}
	}
	if !failed["t_bash"] || !failed["t_read"] {
		t.Fatalf("failed evidence: got %v, want t_bash and t_read", failed)
	}
	if len(failed) != turn.Errors {
		t.Fatalf("failed evidence count %d must equal turn.Errors %d", len(failed), turn.Errors)
	}
}

// TestAnalyzeMutationEvidenceCarriesCommandSummary: paths answer "did
// this step touch the workspace", the summary answers "how" — without
// the command, auditing a Bash mutation means opening the raw JSONL.
func TestAnalyzeMutationEvidenceCarriesCommandSummary(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "cmd-summary",
		StartTime: now,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "build it"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Building and editing."},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t_bash", ToolInput: map[string]any{"command": "go test ./... > /w/out.log 2>&1"}},
					{Type: "tool_use", ToolName: "Edit", ToolID: "t_edit", ToolInput: map[string]any{"file_path": "/w/a.go"}},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t_fail", ToolInput: map[string]any{"command": "make lint"}},
				}},
			{UUID: "r1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_fail", IsError: true}}},
		},
	}

	turn := Analyze(session).Turns[0]
	summaries := map[string]string{}
	for _, step := range turn.Steps {
		for _, m := range step.Mutations {
			summaries[m.ToolID] = m.Summary
		}
	}
	if summaries["t_bash"] != "go test ./... > /w/out.log 2>&1" {
		t.Fatalf("bash mutation summary: %q", summaries["t_bash"])
	}
	if summaries["t_fail"] != "make lint" {
		t.Fatalf("failed bash summary: %q", summaries["t_fail"])
	}
	if summaries["t_edit"] != "" {
		t.Fatalf("edit carries no command, summary must be empty: %q", summaries["t_edit"])
	}
}

func TestCommandSummaryCollapsesAndBounds(t *testing.T) {
	if got := commandSummary(map[string]any{"command": "git commit -m \"$(cat <<'EOF'\nsubject line\n\nbody\nEOF\n)\""}); strings.ContainsAny(got, "\n\t") {
		t.Fatalf("summary must be one line: %q", got)
	}
	long := strings.Repeat("x", 5000)
	got := commandSummary(map[string]any{"command": long})
	if len([]rune(got)) > evidenceCommandRunes+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("summary must be bounded with ellipsis, got %d runes", len([]rune(got)))
	}
	if commandSummary(map[string]any{"file_path": "/w/a.go"}) != "" {
		t.Fatal("non-command input must yield empty summary")
	}
	if commandSummary(nil) != "" {
		t.Fatal("nil input must yield empty summary")
	}
}
