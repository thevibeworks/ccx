package web

import (
	"strings"
	"testing"

	"html"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/trace"
)

// buildToolHeavyTurn constructs a user turn with N tool-using
// assistant messages followed by a final plain-text assistant
// "summary" message. Used to verify the outline preserves the
// summary even when the turn is truncated.
func buildToolHeavyTurn(numTools int) []*parser.Message {
	user := &parser.Message{
		UUID: "u1",
		Kind: parser.KindUserPrompt,
		Content: []parser.ContentBlock{
			{Type: "text", Text: "do a lot of work"},
		},
	}
	var children []*parser.Message
	for i := 0; i < numTools; i++ {
		tool := &parser.Message{
			UUID: "tool-" + string(rune('a'+i)),
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "Bash", ToolInput: map[string]any{"command": "ls"}},
			},
		}
		children = append(children, tool)
	}
	// Final summary message — pure text response, no tool_use. This is
	// the "last assistant response" that must survive truncation.
	summary := &parser.Message{
		UUID: "summary-msg",
		Kind: parser.KindAssistant,
		Content: []parser.ContentBlock{
			{Type: "text", Text: "All done with the work."},
		},
	}
	children = append(children, summary)

	user.Children = children
	return []*parser.Message{user}
}

func TestRenderConversationNav_PreservesSummaryOnTruncation(t *testing.T) {
	// 15 tool calls + 1 summary = 16 children in the turn, way above
	// the 10-child truncation threshold. The outline must still link
	// to the summary (summary-msg) — not drop it in the "+N more"
	// cutoff.
	roots := buildToolHeavyTurn(15)
	var b strings.Builder
	renderConversationNav(&b, roots, nil)
	out := b.String()

	if !strings.Contains(out, `data-msg="summary-msg"`) {
		t.Error("expected outline to preserve final summary message (summary-msg) after truncation")
	}
	// "+N more" marker must still appear
	if !strings.Contains(out, `nav-more`) {
		t.Error("expected '+N more' marker when children are truncated")
	}
}

func TestRenderConversationNav_ShortTurnsShowEverything(t *testing.T) {
	// 3 tool calls + 1 summary = 4 children, well under the 10-child
	// cap. Every child should render, and there should be no "+N more"
	// marker.
	roots := buildToolHeavyTurn(3)
	var b strings.Builder
	renderConversationNav(&b, roots, nil)
	out := b.String()

	if strings.Contains(out, `nav-more`) {
		t.Error("expected NO nav-more marker for short turns")
	}
	if !strings.Contains(out, `data-msg="summary-msg"`) {
		t.Error("expected outline to include the summary for short turns")
	}
	// All 3 tools should be linked
	for _, c := range []rune{'a', 'b', 'c'} {
		want := `data-msg="tool-` + string(c) + `"`
		if !strings.Contains(out, want) {
			t.Errorf("expected outline to link tool-%c", c)
		}
	}
}

func TestRenderConversationNav_HiddenCountExcludesSummary(t *testing.T) {
	// 15 tool calls + 1 summary, truncation at 10. The "+N more" marker
	// should count: total - (maxChildren-1) - 1 summary = 16 - 9 - 1 = 6.
	roots := buildToolHeavyTurn(15)
	var b strings.Builder
	renderConversationNav(&b, roots, nil)
	out := b.String()

	// Expect "+6 more" (not "+7 more" — the summary is broken out).
	if !strings.Contains(out, `+6 more`) {
		t.Errorf("expected '+6 more' marker (summary preserved separately), got: %s", out)
	}
}

func TestRenderConversationNav_SplitsTitleAndExpand(t *testing.T) {
	// The new DOM: each Exchange group has a .nav-title anchor and a
	// .nav-expand button. No <summary>, no <details>. Clicking the
	// title jumps; clicking the button toggles. Two distinct targets
	// mean no more conflicting click handlers.
	roots := buildToolHeavyTurn(3)
	var b strings.Builder
	renderConversationNav(&b, roots, nil)
	out := b.String()

	if !strings.Contains(out, `class="nav-group"`) {
		t.Error("expected .nav-group container")
	}
	if !strings.Contains(out, `data-expanded="true"`) {
		t.Error("expected data-expanded attribute on nav-group")
	}
	if !strings.Contains(out, `class="nav-expand"`) {
		t.Error("expected separate nav-expand button")
	}
	if !strings.Contains(out, `aria-expanded="true"`) {
		t.Error("expected aria-expanded on nav-expand button")
	}
	if !strings.Contains(out, `nav-item nav-title`) {
		t.Error("expected separate nav-title anchor")
	}
	// Crucially: no <summary> or <details>.
	if strings.Contains(out, `<summary`) || strings.Contains(out, `<details`) {
		t.Error("new nav should not use <details>/<summary> (conflicts with click handler)")
	}
}

// TestRenderConversationNav_UsesStepNarration is the regression guard for
// the outline's whole purpose. The rail used to print the kind of each
// message ("response", "Bash"), which tells an auditor nothing. When a
// message opened a trace step, the line must carry that step's narration
// — the agent's own stated intent — plus what the step actually did.
func TestRenderConversationNav_UsesStepNarration(t *testing.T) {
	roots := buildToolHeavyTurn(2)
	const narration = "Handoff read. ccx has an uncommitted v0.17.0 candidate on main."

	stepMap := map[string]*trace.Step{
		"tool-a": {
			Index:       1,
			MessageID:   "tool-a",
			Narration:   narration,
			ToolCounts:  map[string]int{"Bash": 4},
			FilesEdited: []string{"internal/web/templates.go"},
			Errors:      1,
		},
	}

	var b strings.Builder
	renderConversationNav(&b, roots, stepMap)
	out := b.String()

	if !strings.Contains(out, html.EscapeString(narration)) {
		t.Errorf("outline must carry the step narration; got:\n%s", out)
	}
	if !strings.Contains(out, `class="nav-headline`) {
		t.Error("narrated step must render as .nav-headline, not .nav-text")
	}
	// Counters make the expensive and the failing steps visible without
	// opening them: 4 tool calls, 1 edited file, 1 error.
	for _, want := range []string{"4t", "1e", "1x"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected step meta %q in outline; got:\n%s", want, out)
		}
	}

	// A message with no matching step keeps the old compact rendering.
	if !strings.Contains(out, `class="nav-text"`) {
		t.Error("un-narrated messages should still use the compact .nav-text line")
	}
}

// TestRenderConversationNav_NilStepMapIsSafe: trace data is optional
// (a session may have no outline), and the rail must degrade rather
// than panic.
func TestRenderConversationNav_NilStepMapIsSafe(t *testing.T) {
	roots := buildToolHeavyTurn(2)
	var b strings.Builder
	renderConversationNav(&b, roots, nil)
	if out := b.String(); !strings.Contains(out, "nav-text") {
		t.Errorf("expected compact outline with nil step map; got:\n%s", out)
	}
}
