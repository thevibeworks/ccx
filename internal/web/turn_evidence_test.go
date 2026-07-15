package web

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/trace"
)

func evidenceTurns() []trace.Turn {
	return []trace.Turn{
		{
			Index:    3,
			AnchorID: "anchor-uuid-3",
			Steps: []trace.Step{
				{Index: 1, MessageID: "step-msg-31"},
				{Index: 2, MessageID: "step-msg-32", Mutations: []trace.ToolCallEvidence{
					{ToolID: "toolu_edit1", Name: "Edit", MutatesWorkspace: true, MutationCapable: true, Paths: []string{"/repo/a.go"}},
					{ToolID: "toolu_bad", Name: "Bash", MutationCapable: true, IsError: true},
				}},
			},
			FilesEdited: []string{"/repo/a.go"},
			ToolCounts:  map[string]int{"Edit": 1, "Bash": 3},
			Errors:      1,
			CostUSD:     1.23,
		},
		{Index: 5, AnchorID: "anchor-uuid-5", UserText: "next"},
	}
}

func TestResolveTurnTarget(t *testing.T) {
	turns := evidenceTurns()

	cases := []struct {
		param string
		want  string
	}{
		{"3", "anchor-uuid-3"},     // turn -> user anchor
		{"3.2", "step-msg-32"},     // turn.step -> narration message
		{"3.9", "anchor-uuid-3"},   // unknown step falls back to anchor
		{"5", "anchor-uuid-5"},     // sparse indexes resolve by Index, not position
		{"4", ""},                  // dropped/unknown turn
		{"nope", ""},               // garbage
		{" 3 . 2 ", "step-msg-32"}, // tolerant of stray spaces
	}
	for _, c := range cases {
		if got := resolveTurnTarget(turns, strings.TrimSpace(c.param)); got != c.want {
			// retry without inner spaces for the tolerant case
			if got2 := resolveTurnTarget(turns, c.param); got2 != c.want {
				t.Errorf("resolveTurnTarget(%q) = %q, want %q", c.param, got2, c.want)
			}
		}
	}
}

// TestRenderSessionPage_TurnEvidence verifies the per-turn review
// panel against real analyzer output — not fabricated turns. The
// session has one successful edit and two failed calls (one mutating,
// one read-only), so it checks the invariant the panel exists to
// uphold: the error chip and the failed-calls list describe the same
// events the trace outline counts.
func TestRenderSessionPage_TurnEvidence(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "sess-1",
		StartTime: now,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: "please fix"}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Fixing the bug now."},
					{Type: "tool_use", ToolName: "Edit", ToolID: "t_edit1", ToolInput: map[string]any{"file_path": "/repo/a.go"}},
					{Type: "tool_use", ToolName: "Bash", ToolID: "t_bad", ToolInput: map[string]any{"command": "make test"}},
					{Type: "tool_use", ToolName: "Read", ToolID: "t_read", ToolInput: map[string]any{"file_path": "/repo/b.go"}},
				}},
			{UUID: "r1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_edit1"}}},
			{UUID: "r2", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(3 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_bad", IsError: true}}},
			{UUID: "r3", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(4 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t_read", IsError: true}}},
		},
	}

	turns := trace.Analyze(session).Turns
	if len(turns) != 1 {
		t.Fatalf("turns: got %d, want 1", len(turns))
	}
	if turns[0].Errors != 2 {
		t.Fatalf("analyzer errors: got %d, want 2", turns[0].Errors)
	}

	html := renderSessionPage(session, "p", nil, 0, false, false, true, "light", turns, "msg-u1")

	for _, want := range []string{
		`id="turn-1"`,                     // evidence panel exists for the turn
		`href="?turn=1"`,                  // permalink uses trace ordinal
		`>#1</a>`,                         // visible ordinal badge
		`href="#tool-t_edit1"`,            // edit links to the inline diff block
		`1 files`,                         // edited-files chip
		`2 errors`,                        // error chip equals analyzer turn.Errors
		`href="#tool-t_bad"`,              // failed mutating call links to its block
		`href="#tool-t_read"`,             // failed read call is materialized and linked
		`document.body.dataset.ccxTarget`, // deep-link target hint emitted
	} {
		if !strings.Contains(html, want) {
			t.Errorf("session page missing %q", want)
		}
	}

	// The failed-calls list must carry exactly as many rows as the
	// error chip claims — the mismatch this test exists to prevent.
	if got := strings.Count(html, `class="te-fail"`); got != turns[0].Errors {
		t.Errorf("failed-call rows: got %d, want %d (must match error chip)", got, turns[0].Errors)
	}

	// A turn whose anchor thread is not in the DOM (filtered or
	// truncated views) must not render a panel.
	orphaned := append(turns, trace.Turn{Index: 99, AnchorID: "not-in-dom"})
	html = renderSessionPage(session, "p", nil, 0, false, false, true, "light", orphaned, "")
	if strings.Contains(html, `id="turn-99"`) {
		t.Error("turn 99 has no thread in this session; its panel should not render")
	}
}

func TestTurnEvidence_ChipsOmitZeroes(t *testing.T) {
	turn := &trace.Turn{Index: 7, AnchorID: "a7"}
	var b strings.Builder
	renderTurnEvidence(&b, turn, nil, nil)
	out := b.String()
	for _, absent := range []string{"steps", "tools", "files", "agents", "errors"} {
		if strings.Contains(out, absent) {
			t.Errorf("zero-value chip %q should be omitted, got: %s", absent, out)
		}
	}
	if !strings.Contains(out, `>#7</a>`) {
		t.Error("permalink badge must render even for empty turns")
	}
}

// TestExportSafePermalinks verifies a downloaded HTML export carries
// no ?turn= links (they only resolve through the server) while the
// panels stay reachable via their in-document #turn-N anchors.
func TestExportSafePermalinks(t *testing.T) {
	now := time.Now()
	session := &parser.Session{
		ID:        "sess-exp",
		StartTime: now,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Type: "user", Timestamp: now,
				Content: []parser.ContentBlock{{Type: "text", Text: `see href="?turn=9" in body text`}}},
			{UUID: "a1", Kind: parser.KindAssistant, Type: "assistant", Timestamp: now.Add(time.Minute),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Working."},
					{Type: "tool_use", ToolName: "Edit", ToolID: "t1", ToolInput: map[string]any{"file_path": "/repo/a.go"}},
				}},
			{UUID: "r1", Kind: parser.KindToolResult, Type: "user", Timestamp: now.Add(2 * time.Minute),
				Content: []parser.ContentBlock{{Type: "tool_result", ToolID: "t1"}}},
		},
	}
	turns := trace.Analyze(session).Turns
	page := renderSessionPage(session, "p", nil, 0, true, true, true, "light", turns, "")
	if !strings.Contains(page, `class="te-permalink" href="?turn=1"`) {
		t.Fatal("server page must keep ?turn= permalinks")
	}

	export := exportSafePermalinks(page)
	if strings.Contains(export, `class="te-permalink" href="?turn=`) {
		t.Error("export must not carry server-only ?turn= permalinks")
	}
	if !strings.Contains(export, `class="te-permalink" href="#turn-1"`) {
		t.Error("export permalink must target the in-document anchor")
	}
	// User content mentioning ?turn= is escaped, untouched, and preserved.
	if !strings.Contains(export, `href=&#34;?turn=9&#34;`) {
		t.Error("escaped user content must survive the rewrite")
	}
}
