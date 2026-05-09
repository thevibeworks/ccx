package web

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func buildSessionWithSidechains() *parser.Session {
	t0 := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)

	mainUser := &parser.Message{
		UUID:      "u1",
		Kind:      parser.KindUserPrompt,
		Timestamp: t0,
		Content:   []parser.ContentBlock{{Type: "text", Text: "fix the auth bug"}},
	}
	mainAssistant := &parser.Message{
		UUID:      "a1",
		Kind:      parser.KindAssistant,
		Timestamp: t0.Add(time.Second),
		Content: []parser.ContentBlock{
			{Type: "text", Text: "I'll investigate the auth module."},
			{Type: "tool_use", ToolName: "Task", ToolID: "t1", ToolInput: map[string]any{
				"subagent_type": "Explore",
				"description":   "find auth callers",
			}},
		},
	}
	mainToolResult := &parser.Message{
		UUID:      "tr1",
		Kind:      parser.KindToolResult,
		Timestamp: t0.Add(5 * time.Second),
		Content:   []parser.ContentBlock{{Type: "tool_result", ToolID: "t1", ToolResult: "done"}},
		SubAgentResult: &parser.SubAgentResultData{
			AgentID:           "agent-abc",
			AgentType:         "Explore",
			TotalTokens:       22351,
			TotalToolUseCount: 12,
			TotalDurationMs:   144629,
			ToolStats:         &parser.ToolStats{BashCount: 8, EditFileCount: 4, LinesAdded: 484},
		},
	}
	mainUser.Children = []*parser.Message{mainAssistant, mainToolResult}

	scUser := &parser.Message{
		UUID:        "sc-u1",
		Kind:        parser.KindUserPrompt,
		IsSidechain: true,
		AgentID:     "agent-abc",
		Timestamp:   t0.Add(2 * time.Second),
		Content:     []parser.ContentBlock{{Type: "text", Text: "SIDECHAIN_UNIQUE_USER_PROMPT_TEXT"}},
	}
	scAssistant := &parser.Message{
		UUID:        "sc-a1",
		Kind:        parser.KindAssistant,
		IsSidechain: true,
		AgentID:     "agent-abc",
		Timestamp:   t0.Add(3 * time.Second),
		Content: []parser.ContentBlock{
			{Type: "text", Text: "SIDECHAIN_UNIQUE_ASSISTANT_RESPONSE"},
			{Type: "tool_use", ToolName: "Grep", ToolID: "sc-t1"},
		},
	}
	scToolResult := &parser.Message{
		UUID:        "sc-tr1",
		Kind:        parser.KindToolResult,
		IsSidechain: true,
		AgentID:     "agent-abc",
		Timestamp:   t0.Add(4 * time.Second),
		Content:     []parser.ContentBlock{{Type: "tool_result", ToolID: "sc-t1", ToolResult: "auth.go:42"}},
	}
	scUser.Children = []*parser.Message{scAssistant, scToolResult}

	return &parser.Session{
		ID:        "test-sidechain-session",
		Provider:  "claude-code",
		StartTime: t0,
		EndTime:   t0.Add(5 * time.Second),
		RootMessages: []*parser.Message{
			mainUser,
			scUser,
		},
		Stats: parser.SessionStats{
			MessageCount:    5,
			AgentSidechains: 3,
		},
	}
}

func TestFilterMainConversation_RemovesSidechains(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "m1", IsSidechain: false},
		{UUID: "sc1", IsSidechain: true},
		{UUID: "m2", IsSidechain: false},
		{UUID: "sc2", IsSidechain: true},
		{UUID: "sc3", IsSidechain: true},
		{UUID: "m3", IsSidechain: false},
	}
	got := filterMainConversation(msgs)
	if len(got) != 3 {
		t.Fatalf("want 3 main messages, got %d", len(got))
	}
	for _, m := range got {
		if m.IsSidechain {
			t.Errorf("sidechain message %q leaked through filter", m.UUID)
		}
	}
}

func TestFilterMainConversation_EmptyInput(t *testing.T) {
	got := filterMainConversation(nil)
	if len(got) != 0 {
		t.Errorf("want 0, got %d", len(got))
	}
}

func TestFilterMainConversation_AllSidechain(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "sc1", IsSidechain: true},
		{UUID: "sc2", IsSidechain: true},
	}
	got := filterMainConversation(msgs)
	if len(got) != 0 {
		t.Errorf("all-sidechain input should return empty, got %d", len(got))
	}
}

func TestFilterMainConversation_NoSidechain(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "m1"},
		{UUID: "m2"},
	}
	got := filterMainConversation(msgs)
	if len(got) != 2 {
		t.Errorf("no-sidechain input should pass through all, got %d", len(got))
	}
}

func TestRenderMessages_SidechainInlineWithDispatch(t *testing.T) {
	session := buildSessionWithSidechains()
	var b strings.Builder
	renderMessages(&b, session.RootMessages, 0, false, false, true)
	html := b.String()

	if !strings.Contains(html, "fix the auth bug") {
		t.Error("main user prompt text missing")
	}
	if !strings.Contains(html, "auth module") {
		t.Error("main assistant text missing")
	}
	if !strings.Contains(html, "SIDECHAIN_UNIQUE_USER_PROMPT_TEXT") {
		t.Error("sidechain content MUST be visible inline")
	}
	if !strings.Contains(html, "SIDECHAIN_UNIQUE_ASSISTANT_RESPONSE") {
		t.Error("sidechain assistant text MUST be visible inline")
	}
	if !strings.Contains(html, "sidechain-group") {
		t.Error("sidechain should be in a sidechain-group container")
	}

	taskIdx := strings.Index(html, `tool-t1`)
	scIdx := strings.Index(html, "sidechain-group")
	if taskIdx < 0 || scIdx < 0 {
		t.Fatal("expected both Task tool_use and sidechain-group in output")
	}
	if scIdx < taskIdx {
		t.Error("sidechain group should appear AFTER the dispatching Task tool_use, not before")
	}
}

func TestRenderMessages_SidechainNotAsOwnThread(t *testing.T) {
	session := buildSessionWithSidechains()
	var b strings.Builder
	renderMessages(&b, session.RootMessages, 0, false, false, true)
	html := b.String()

	threadCount := strings.Count(html, `class="thread"`)
	if threadCount != 1 {
		t.Errorf("expected 1 thread (main conversation only), got %d — sidechain must not create its own thread", threadCount)
	}
}

func TestRenderConversationNav_SidechainInNavAsSeparateEntries(t *testing.T) {
	session := buildSessionWithSidechains()
	var b strings.Builder
	renderConversationNav(&b, session.RootMessages)
	html := b.String()

	if !strings.Contains(html, `data-msg="u1"`) {
		t.Error("main user prompt should appear in nav outline")
	}
	if strings.Contains(html, `data-msg="sc-u1"`) {
		t.Error("sidechain user prompt should NOT create its own nav GROUP (it's not a main thread)")
	}
	if !strings.Contains(html, "nav-sidechain") {
		t.Error("sidechain should appear as a nav-sidechain entry in the outline")
	}
	if !strings.Contains(html, `sidechain-agent-abc`) {
		t.Error("sidechain nav entry should link to #sidechain-<agentId>")
	}
}

func TestRenderMessages_SidechainStatsPreserved(t *testing.T) {
	session := buildSessionWithSidechains()
	if session.Stats.AgentSidechains != 3 {
		t.Errorf("AgentSidechains = %d, want 3 (filtering is render-only, stats must be untouched)", session.Stats.AgentSidechains)
	}
	allMsgs := flattenMessages(session.RootMessages)
	sidechainCount := 0
	for _, m := range allMsgs {
		if m.IsSidechain {
			sidechainCount++
		}
	}
	if sidechainCount != 3 {
		t.Errorf("flattenMessages should still include %d sidechain messages (filter is applied separately), got %d", 3, sidechainCount)
	}
}

func TestRenderMessages_SidechainShowsAgentTypeAndStats(t *testing.T) {
	session := buildSessionWithSidechains()
	var b strings.Builder
	renderMessages(&b, session.RootMessages, 0, false, false, true)
	html := b.String()

	if !strings.Contains(html, "Explore") {
		t.Error("sidechain header should show agent type name 'Explore'")
	}
	if !strings.Contains(html, "find auth callers") {
		t.Error("sidechain header should show description")
	}
	if !strings.Contains(html, "22.4k tokens") || !strings.Contains(html, "12 tools") {
		t.Errorf("sidechain header should show stats from toolUseResult, got html containing sidechain-meta")
	}
	if !strings.Contains(html, "+484 lines") {
		t.Error("sidechain header should show lines added from toolStats")
	}
}

func TestGroupSidechainsByAgent(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "m1"},
		{UUID: "sc1", IsSidechain: true, AgentID: "a1"},
		{UUID: "sc2", IsSidechain: true, AgentID: "a1"},
		{UUID: "sc3", IsSidechain: true, AgentID: "a2"},
		{UUID: "m2"},
		{UUID: "sc4", IsSidechain: true, AgentID: "a2"},
		{UUID: "sc5", IsSidechain: true, AgentID: ""}, // no agentId — skip
	}
	groups := groupSidechainsByAgent(msgs)
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(groups))
	}
	if groups[0].AgentID != "a1" || len(groups[0].Messages) != 2 {
		t.Errorf("group 0: agentID=%q msgs=%d, want a1/2", groups[0].AgentID, len(groups[0].Messages))
	}
	if groups[1].AgentID != "a2" || len(groups[1].Messages) != 2 {
		t.Errorf("group 1: agentID=%q msgs=%d, want a2/2", groups[1].AgentID, len(groups[1].Messages))
	}
}

func TestMatchSidechainsToToolUse(t *testing.T) {
	mainMsgs := []*parser.Message{
		{UUID: "u1", Kind: parser.KindUserPrompt},
		{UUID: "a1", Kind: parser.KindAssistant, Content: []parser.ContentBlock{
			{Type: "tool_use", ToolName: "Agent", ToolID: "t1", ToolInput: map[string]any{
				"subagent_type": "Explore",
				"description":   "search codebase",
			}},
			{Type: "tool_use", ToolName: "Agent", ToolID: "t2", ToolInput: map[string]any{
				"subagent_type": "linus-rants",
				"description":   "review naming",
			}},
		}},
	}
	groups := []sidechainGroup{
		{AgentID: "agent-1", Messages: []*parser.Message{{UUID: "sc1"}}},
		{AgentID: "agent-2", Messages: []*parser.Message{{UUID: "sc2"}}},
	}
	scMap := matchSidechainsToToolUse(mainMsgs, groups)
	if len(scMap) != 2 {
		t.Fatalf("want 2 matched groups, got %d", len(scMap))
	}
	g1 := scMap["t1"]
	if g1.AgentType != "Explore" {
		t.Errorf("t1 AgentType = %q, want Explore", g1.AgentType)
	}
	if g1.Description != "search codebase" {
		t.Errorf("t1 Description = %q, want 'search codebase'", g1.Description)
	}
	g2 := scMap["t2"]
	if g2.AgentType != "linus-rants" {
		t.Errorf("t2 AgentType = %q, want linus-rants", g2.AgentType)
	}
}

func TestMatchSidechainsToToolUse_NoGroups(t *testing.T) {
	scMap := matchSidechainsToToolUse(nil, nil)
	if scMap != nil {
		t.Error("nil groups should return nil map")
	}
}

func TestMatchSidechainsToToolUse_MoreDispatchesThanGroups(t *testing.T) {
	mainMsgs := []*parser.Message{
		{UUID: "a1", Kind: parser.KindAssistant, Content: []parser.ContentBlock{
			{Type: "tool_use", ToolName: "Agent", ToolID: "t1"},
			{Type: "tool_use", ToolName: "Agent", ToolID: "t2"},
			{Type: "tool_use", ToolName: "Agent", ToolID: "t3"},
		}},
	}
	groups := []sidechainGroup{
		{AgentID: "a1", Messages: []*parser.Message{{UUID: "sc1"}}},
	}
	scMap := matchSidechainsToToolUse(mainMsgs, groups)
	if len(scMap) != 1 {
		t.Errorf("should match only 1 group (not panic), got %d", len(scMap))
	}
}

func TestRenderMessages_MainOnlySession(t *testing.T) {
	t0 := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	user := &parser.Message{
		UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: t0,
		Content: []parser.ContentBlock{{Type: "text", Text: "hello"}},
	}
	assistant := &parser.Message{
		UUID: "a1", Kind: parser.KindAssistant, Timestamp: t0.Add(time.Second),
		Content: []parser.ContentBlock{{Type: "text", Text: "hi there"}},
	}
	user.Children = []*parser.Message{assistant}
	session := &parser.Session{
		ID:           "no-sidechain",
		RootMessages: []*parser.Message{user},
	}

	var b strings.Builder
	renderMessages(&b, session.RootMessages, 0, false, false, true)
	html := b.String()

	if !strings.Contains(html, "hello") {
		t.Error("user text missing")
	}
	if !strings.Contains(html, "hi there") {
		t.Error("assistant text missing")
	}
}
