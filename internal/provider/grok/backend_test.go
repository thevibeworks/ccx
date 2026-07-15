package grok

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

// fixtureHome points at the committed, sanitized Grok home built from
// real grok 0.2.101 sessions (docs/devlog/2026-07-15-grok-session-format.org).
func fixtureHome(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../../testdata/fixtures/grok-home")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestDiscoverProjectsFromFixtures(t *testing.T) {
	b := New(fixtureHome(t))
	projects, err := b.DiscoverProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects: got %d, want 1", len(projects))
	}
	p := projects[0]
	if p.Name != "widget" {
		t.Errorf("project name: got %q, want widget", p.Name)
	}
	if p.Path != "/home/dev/widget" {
		t.Errorf("decoded path: got %q", p.Path)
	}
	if p.Provider != ProviderID {
		t.Errorf("provider: got %q", p.Provider)
	}
	if len(p.Sessions) != 3 {
		t.Fatalf("sessions: got %d, want 3", len(p.Sessions))
	}
	// Newest first by EndTime: snapshots (12:03) > toolcalls (11:05) > chat (10:01).
	if p.Sessions[0].ID != "01900000-0000-7000-8000-000000000003" {
		t.Errorf("session order: newest first, got %s", p.Sessions[0].ID)
	}
	for _, s := range p.Sessions {
		if s.Provider != ProviderID {
			t.Errorf("session %s provider: got %q", s.ID, s.Provider)
		}
		if s.Summary == "" {
			t.Errorf("session %s has no summary", s.ID)
		}
		if s.Model != "grok-4.5" {
			t.Errorf("session %s model: got %q", s.ID, s.Model)
		}
	}
}

func TestFindSessionByPrefix(t *testing.T) {
	b := New(fixtureHome(t))
	s, err := b.FindSession("", "01900000-0000-7000-8000-000000000002")
	if err != nil || s == nil {
		t.Fatalf("find: %v %v", err, s)
	}
	if s.Summary != "Fix greeting bug with tool calls" {
		t.Errorf("summary: %q", s.Summary)
	}
}

func TestParseSessionToolCalls(t *testing.T) {
	b := New(fixtureHome(t))
	s, err := b.FindSession("widget", "01900000-0000-7000-8000-000000000002")
	if err != nil || s == nil {
		t.Fatalf("find: %v %v", err, s)
	}
	full, err := b.ParseSession(s.FilePath)
	if err != nil {
		t.Fatal(err)
	}

	msgs := parser.FlattenSessionMessages(full)
	var (
		userTexts []string
		toolUses  []parser.ContentBlock
		results   int
		thinking  int
	)
	for _, m := range msgs {
		switch m.Kind {
		case parser.KindUserPrompt:
			for _, c := range m.Content {
				if c.Type == "text" {
					userTexts = append(userTexts, c.Text)
				}
			}
		case parser.KindToolResult:
			results++
		case parser.KindAssistant:
			for _, c := range m.Content {
				if c.Type == "tool_use" {
					toolUses = append(toolUses, c)
				}
				if c.Type == "thinking" {
					thinking++
				}
			}
		}
	}

	// The harness envelope is stripped: the user typed only the query.
	if len(userTexts) != 1 || userTexts[0] != "fix the greeting bug in widget.go" {
		t.Errorf("user texts: %q", userTexts)
	}
	if thinking != 1 {
		t.Errorf("thinking blocks (reasoning summary): got %d, want 1", thinking)
	}
	if results != 3 {
		t.Errorf("tool results: got %d, want 3", results)
	}
	if len(toolUses) != 3 {
		t.Fatalf("tool_use blocks: got %d, want 3", len(toolUses))
	}

	// Tool names normalized to the canonical dialect, arguments
	// decoded from the OpenAI-style JSON string.
	wantNames := map[string]bool{"Read": true, "Edit": true, "Bash": true}
	for _, tu := range toolUses {
		if !wantNames[tu.ToolName] {
			t.Errorf("unexpected tool name %q", tu.ToolName)
		}
		if tu.ToolInput == nil {
			t.Errorf("tool %s: arguments not decoded", tu.ToolName)
		}
	}
	edit := toolUses[1]
	input, ok := edit.ToolInput.(map[string]any)
	if !ok {
		t.Fatalf("edit input: %T", edit.ToolInput)
	}
	if input["path"] != "widget.go" {
		t.Errorf("edit path: %v", input["path"])
	}

	// Usage totals from updates.jsonl; cost stays zero by contract.
	if full.Stats.InputTokens != 5000 || full.Stats.OutputTokens != 180 {
		t.Errorf("usage: in=%d out=%d", full.Stats.InputTokens, full.Stats.OutputTokens)
	}
	if full.Stats.CacheReadTokens != 1200 {
		t.Errorf("cache read: %d", full.Stats.CacheReadTokens)
	}
	if full.Stats.CostUSD != 0 {
		t.Errorf("grok cost must be zero (unverified pricing), got %v", full.Stats.CostUSD)
	}
	if full.Stats.UserPrompts != 1 {
		t.Errorf("user prompts: %d", full.Stats.UserPrompts)
	}

	// User prompts carry real timestamps from prompt_history.jsonl.
	for _, m := range msgs {
		if m.Kind == parser.KindUserPrompt && m.Timestamp.IsZero() {
			t.Error("user prompt has zero timestamp")
		}
	}
}

func TestParseSessionPlainChat(t *testing.T) {
	b := New(fixtureHome(t))
	s, err := b.FindSession("widget", "01900000-0000-7000-8000-000000000001")
	if err != nil || s == nil {
		t.Fatalf("find: %v %v", err, s)
	}
	full, err := b.ParseSession(s.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if full.Stats.UserPrompts != 1 || full.Stats.ToolCalls != 0 {
		t.Errorf("stats: %+v", full.Stats)
	}
	if full.Summary != "Plain greeting chat" {
		t.Errorf("summary: %q", full.Summary)
	}
}

func TestChatFormatVersionGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "summary.json"),
		[]byte(`{"info":{"id":"x","cwd":"/w"},"chat_format_version":2,"created_at":"2026-07-01T10:00:00Z","updated_at":"2026-07-01T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := quickSession(dir); err == nil {
		t.Fatal("a future chat_format_version must fail loud, not parse garbage")
	}
}
