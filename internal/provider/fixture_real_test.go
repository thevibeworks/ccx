package provider

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/provider/claude"
	"github.com/thevibeworks/ccx/internal/provider/codex"
)

// Tests in this file parse the REAL captured fixtures under
// testdata/test-session/ and lock in the CURRENT parser behaviour —
// including the two known bugs documented in testdata/test-session/
// README.md (Codex web_search double-count, Claude Code sidechain
// subagents not discovered).
//
// When we fix a bug, the corresponding test must be updated with the
// NEW expected count — that's the signal a behavior change happened
// and was intentional. Tests that lock in "wrong" numbers are noted
// with a // BUG: comment and a reference to the README section.
//
// Fixture provenance:
//   Claude Code  — CLI 2.1.104, captured 2026-04-15 08:48Z via Prompt A
//   Codex        — CLI  0.120.0, captured 2026-04-15 08:47Z via Prompt B
// Both rollouts are from the same scratchpad task: build a tiny word-
// frequency index from scratch and generate full tool coverage.

// fixtureRoot locates testdata/test-session/ relative to this test
// file so tests run from any working directory.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// this file is at internal/provider/fixture_real_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "test-session")
}

// -----------------------------------------------------------------
// Claude Code real fixture
// -----------------------------------------------------------------

func TestRealFixture_ClaudeCode_ParsesCleanly(t *testing.T) {
	root := fixtureRoot(t)
	home := filepath.Join(root, "claude")
	backend := claude.NewWithProjectsDir(home, filepath.Join(home, "projects"))
	sessionPath := filepath.Join(home, "projects", "test-session", "de4b5d69-0744-4202-93f0-4d329a3dac3b.jsonl")

	session, err := backend.ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if session == nil {
		t.Fatal("nil session")
	}

	if session.ID != "de4b5d69-0744-4202-93f0-4d329a3dac3b" {
		t.Errorf("session.ID = %q", session.ID)
	}
	if session.Provider != "claude-code" {
		t.Errorf("Provider = %q, want claude-code", session.Provider)
	}
	if session.CWD == "" {
		t.Error("CWD empty — parser lost session metadata")
	}
	if session.Version != "2.1.104" {
		t.Errorf("Version = %q, want 2.1.104 (fixture provenance)", session.Version)
	}
}

func TestRealFixture_ClaudeCode_MessageCounts(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)

	// Flat count locks in the current tree-walk result. If the
	// parser starts discovering additional line types (attachments,
	// queue-operations, etc.), this number will rise and the test
	// will force an intentional update.
	if got, want := len(flat), 104; got != want {
		t.Errorf("flat message count = %d, want %d (see testdata/test-session/README.md for non-transcript line types currently filtered)", got, want)
	}

	// Kind distribution — one user prompt, many assistants, many
	// tool_results, one meta. This is the shape a single-prompt
	// fixture produces.
	kinds := map[parser.MessageKind]int{}
	for _, m := range flat {
		kinds[m.Kind]++
	}
	assertCount(t, "KindUserPrompt", kinds[parser.KindUserPrompt], 1)
	assertCount(t, "KindAssistant", kinds[parser.KindAssistant], 63)
	assertCount(t, "KindToolResult", kinds[parser.KindToolResult], 39)
	assertCount(t, "KindMeta", kinds[parser.KindMeta], 1)
}

func TestRealFixture_ClaudeCode_ContentBlockCounts(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)

	blocks := map[string]int{}
	for _, m := range flat {
		for _, c := range m.Content {
			blocks[c.Type]++
		}
	}
	assertCount(t, "text blocks", blocks["text"], 19)
	assertCount(t, "thinking blocks", blocks["thinking"], 7)
	assertCount(t, "tool_use blocks", blocks["tool_use"], 39)
	assertCount(t, "tool_result blocks", blocks["tool_result"], 39)
}

func TestRealFixture_ClaudeCode_ToolDistribution(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)

	tools := map[string]int{}
	for _, m := range flat {
		for _, c := range m.Content {
			if c.Type == "tool_use" && c.ToolName != "" {
				tools[c.ToolName]++
			}
		}
	}

	// Lock in the 16-tool distribution from the real capture. Any
	// change here flags either a parser regression or a fixture
	// regen.
	wantTools := map[string]int{
		"TaskUpdate":      8,
		"Write":           4,
		"ToolSearch":      4,
		"TaskCreate":      4,
		"Bash":            4,
		"Skill":           2,
		"Read":            2,
		"Edit":            2,
		"AskUserQuestion": 2,
		"WebFetch":        1,
		"Monitor":         1, // first Monitor tool_use ever captured — 2.1.104
		"Grep":            1,
		"Glob":            1,
		"ExitPlanMode":    1,
		"EnterPlanMode":   1,
		"Agent":           1,
	}
	for name, want := range wantTools {
		assertCount(t, "tool "+name, tools[name], want)
	}
	if got, want := len(tools), len(wantTools); got != want {
		// Flag unexpected tools so a CLI update that introduces a
		// brand-new tool name is a visible test failure.
		names := make([]string, 0, len(tools))
		for n := range tools {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("unique tool names = %d, want %d. got: %v", got, want, names)
	}
}

func TestRealFixture_ClaudeCode_MonitorToolInputShape(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)

	// The first-ever captured Monitor call. Its input shape is
	// locked here so future parser changes can't silently drop
	// fields. See testdata/test-session/README.md § Monitor tool_use.
	var found bool
	for _, m := range flat {
		for _, c := range m.Content {
			if c.Type != "tool_use" || c.ToolName != "Monitor" {
				continue
			}
			found = true
			input, ok := c.ToolInput.(map[string]any)
			if !ok {
				t.Fatalf("Monitor ToolInput type = %T, want map[string]any", c.ToolInput)
			}
			if _, ok := input["description"].(string); !ok {
				t.Error("Monitor.description missing or wrong type")
			}
			if _, ok := input["command"].(string); !ok {
				t.Error("Monitor.command missing or wrong type")
			}
			if _, ok := input["persistent"].(bool); !ok {
				t.Error("Monitor.persistent missing or wrong type")
			}
			if _, ok := input["timeout_ms"].(float64); !ok {
				t.Error("Monitor.timeout_ms missing or wrong type")
			}
			// The fixture command watches a signal file for READY
			if cmd := input["command"].(string); !stringContains(cmd, "signal.log") {
				t.Errorf("Monitor.command unexpected: %q", cmd)
			}
		}
	}
	if !found {
		t.Fatal("no Monitor tool_use found in fixture — regen broke coverage")
	}
}

// TestRealFixture_ClaudeCode_SidechainNotDiscovered is a CHARACTERIZATION
// test: the sub-agent sidechain file at
// claude/projects/test-session/de4b5d69-.../subagents/agent-*.jsonl
// exists and contains 49 real messages, but ccx's backend does NOT
// discover it today. This test locks in the CURRENT (broken) behaviour
// of zero sidechain messages. When the sidechain-discovery bug is
// fixed, this test MUST be updated with the new expected count.
//
// Bug: testdata/test-session/README.md § Bug #2
func TestRealFixture_ClaudeCode_SidechainNotDiscovered(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)

	sidechain := 0
	for _, m := range flat {
		if m.IsSidechain {
			sidechain++
		}
	}
	if sidechain != 0 {
		t.Errorf("sidechain count = %d, want 0 (current broken behaviour). If you fixed the subagents/ discovery bug, update this test to the new expected count — the sidechain file has 49 messages (26 assistant + 23 user).", sidechain)
	}

	// Session stats mirror the zero-sidechain observation
	if session.Stats.AgentSidechains != 0 {
		t.Errorf("Stats.AgentSidechains = %d, want 0 (current broken behaviour)", session.Stats.AgentSidechains)
	}
}

func TestRealFixture_ClaudeCode_Exchanges(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)
	exchanges := parser.ComputeExchanges(flat)

	// One user prompt, one exchange. The whole fixture is one
	// massive agent loop.
	if len(exchanges) != 1 {
		t.Fatalf("exchanges = %d, want 1", len(exchanges))
	}
	ex := exchanges[0]
	if ex.AnchorID == "" {
		t.Error("exchange anchor UUID empty")
	}
	if len(ex.Steps) == 0 {
		t.Error("exchange has no steps (expected 39 tool_use blocks)")
	}
}

// TestRealFixture_ClaudeCode_SubagentStepsClassified verifies the
// extractSteps fix for Bug #3. The fixture invokes TaskCreate × 4
// and Agent × 1 to dispatch sub-agents, all 5 of which must now
// classify as StepSubagent instead of falling through to StepToolUse.
//
// Before the fix: StepSubagent = 0 (broken — only ToolName=="Task"
// was recognised).
// After the fix:  StepSubagent = 5.
//
// Bug: testdata/test-session/README.md § Bug #3.
func TestRealFixture_ClaudeCode_SubagentStepsClassified(t *testing.T) {
	session := parseClaudeFixture(t)
	flat := parser.FlattenSessionMessages(session)
	exchanges := parser.ComputeExchanges(flat)
	if len(exchanges) != 1 {
		t.Fatalf("exchanges = %d, want 1", len(exchanges))
	}
	ex := exchanges[0]

	// 4 TaskCreate + 1 Agent = 5 sub-agent dispatches.
	if got := ex.CountSteps(parser.StepSubagent); got != 5 {
		t.Errorf("StepSubagent = %d, want 5 (4 TaskCreate + 1 Agent)", got)
	}
}

// -----------------------------------------------------------------
// Codex real fixture
// -----------------------------------------------------------------

func TestRealFixture_Codex_ParsesCleanly(t *testing.T) {
	session := parseCodexFixture(t)

	if session.ID != "019d9050-7dbf-7e02-a2ac-7c3adc2cd93b" {
		t.Errorf("session.ID = %q", session.ID)
	}
	if session.Provider != "codex" {
		t.Errorf("Provider = %q, want codex", session.Provider)
	}
	if session.Version != "0.120.0" {
		t.Errorf("Version = %q, want 0.120.0 (fixture provenance)", session.Version)
	}
	if session.CWD == "" {
		t.Error("CWD empty")
	}
}

func TestRealFixture_Codex_MessageCounts(t *testing.T) {
	session := parseCodexFixture(t)
	flat := parser.FlattenSessionMessages(session)

	// Lock in the current parser output. 46 flat messages from 96
	// rollout lines (non-message events don't become Messages).
	if got, want := len(flat), 46; got != want {
		t.Errorf("flat message count = %d, want %d", got, want)
	}

	kinds := map[parser.MessageKind]int{}
	for _, m := range flat {
		kinds[m.Kind]++
	}
	assertCount(t, "KindUserPrompt", kinds[parser.KindUserPrompt], 1)
	assertCount(t, "KindAssistant", kinds[parser.KindAssistant], 29)
	assertCount(t, "KindToolResult", kinds[parser.KindToolResult], 16)
}

// TestRealFixture_Codex_ToolDistributionLocksInBug captures the
// CURRENT (buggy) web_search double-count. The real session has 3
// web_search calls but ccx produces 6 WebSearch tool_use blocks
// because response_item/web_search_call.id doesn't match
// event_msg/web_search_end.call_id. See README.md § Bug #1.
//
// When the bug is fixed, the expected WebSearch count MUST drop
// from 6 to 3 and the total unique tools number stays the same.
func TestRealFixture_Codex_ToolDistributionLocksInBug(t *testing.T) {
	session := parseCodexFixture(t)
	flat := parser.FlattenSessionMessages(session)

	tools := map[string]int{}
	for _, m := range flat {
		for _, c := range m.Content {
			if c.Type == "tool_use" && c.ToolName != "" {
				tools[c.ToolName]++
			}
		}
	}
	wantTools := map[string]int{
		"Bash":       7, // exec_command × 7
		"WebSearch":  6, // BUG: should be 3 — locks in current double-count
		"ApplyPatch": 3, // custom_tool_call × 3
		"UpdatePlan": 2, // update_plan × 2
		"mcp__codex_apps__github_search_repositories": 1,
	}
	for name, want := range wantTools {
		assertCount(t, "tool "+name, tools[name], want)
	}
	if got, want := len(tools), len(wantTools); got != want {
		names := make([]string, 0, len(tools))
		for n := range tools {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("unique tool names = %d, want %d. got: %v", got, want, names)
	}
}

func TestRealFixture_Codex_Stats(t *testing.T) {
	session := parseCodexFixture(t)

	// Token accounting (from the trailing token_count event).
	// These locks let us catch any regression in the token-count
	// delta-distribution logic.
	if session.Stats.InputTokens == 0 {
		t.Error("InputTokens zero — token_count parsing broken")
	}
	if session.Stats.OutputTokens == 0 {
		t.Error("OutputTokens zero")
	}
	if session.Stats.CostUSD <= 0 {
		t.Error("CostUSD zero — pricing lookup failed or cost computation broken")
	}
	// Sanity bounds: this specific capture should be in a recognizable
	// range. Wide tolerance so minor pricing-table tweaks don't break.
	if session.Stats.CostUSD < 1.0 || session.Stats.CostUSD > 50.0 {
		t.Errorf("CostUSD = %.4f, want in [1, 50] for this fixture", session.Stats.CostUSD)
	}
}

func TestRealFixture_Codex_Exchanges(t *testing.T) {
	session := parseCodexFixture(t)
	flat := parser.FlattenSessionMessages(session)
	exchanges := parser.ComputeExchanges(flat)

	if len(exchanges) != 1 {
		t.Fatalf("exchanges = %d, want 1", len(exchanges))
	}
	ex := exchanges[0]
	if ex.Snippet == "" {
		t.Error("exchange snippet empty")
	}
	// NOTE: the step count reflects the buggy web_search double-count.
	// When bug #1 is fixed, the tool step count for WebSearch should
	// drop from 6 to 3.
	if wsSteps := ex.CountSteps(parser.StepToolUse); wsSteps == 0 {
		t.Error("exchange has no tool-use steps")
	}
}

// -----------------------------------------------------------------
// helpers
// -----------------------------------------------------------------

func parseClaudeFixture(t *testing.T) *parser.Session {
	t.Helper()
	root := fixtureRoot(t)
	home := filepath.Join(root, "claude")
	backend := claude.NewWithProjectsDir(home, filepath.Join(home, "projects"))
	sessionPath := filepath.Join(home, "projects", "test-session", "de4b5d69-0744-4202-93f0-4d329a3dac3b.jsonl")
	session, err := backend.ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("claude ParseSession: %v", err)
	}
	return session
}

func parseCodexFixture(t *testing.T) *parser.Session {
	t.Helper()
	root := fixtureRoot(t)
	home := filepath.Join(root, "codex")
	backend := codex.New(home)
	sessionPath := filepath.Join(home, "sessions", "2026", "04", "15", "rollout-2026-04-15T01-44-47-019d9050-7dbf-7e02-a2ac-7c3adc2cd93b.jsonl")
	session, err := backend.ParseSession(sessionPath)
	if err != nil {
		t.Fatalf("codex ParseSession: %v", err)
	}
	return session
}

func assertCount(t *testing.T, label string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", label, got, want)
	}
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
