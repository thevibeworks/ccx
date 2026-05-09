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
	mainUser.Children = []*parser.Message{mainAssistant}

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
