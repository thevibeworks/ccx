package render

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestExtractEditedFiles_DedupesAndSorts(t *testing.T) {
	msgs := []*parser.Message{
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "Edit", ToolInput: map[string]any{"file_path": "/src/b.go"}},
				{Type: "tool_use", ToolName: "Write", ToolInput: map[string]any{"file_path": "/src/a.go"}},
			},
		},
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "Edit", ToolInput: map[string]any{"file_path": "/src/a.go"}},
			},
		},
	}
	got := extractEditedFiles(msgs)
	want := []string{"/src/a.go", "/src/b.go"}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestExtractEditedFiles_IgnoresNonEditTools(t *testing.T) {
	msgs := []*parser.Message{
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "Bash", ToolInput: map[string]any{"command": "ls"}},
				{Type: "tool_use", ToolName: "Read", ToolInput: map[string]any{"file_path": "/src/a.go"}},
				{Type: "tool_use", ToolName: "Glob", ToolInput: map[string]any{"pattern": "*.go"}},
			},
		},
	}
	if got := extractEditedFiles(msgs); len(got) != 0 {
		t.Errorf("expected no files for non-edit tools, got %v", got)
	}
}

func TestExtractEditedFiles_HandlesNotebookAndPathFields(t *testing.T) {
	msgs := []*parser.Message{
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "NotebookEdit", ToolInput: map[string]any{"notebook_path": "/nb/x.ipynb"}},
				{Type: "tool_use", ToolName: "Create", ToolInput: map[string]any{"path": "/src/new.go"}},
			},
		},
	}
	got := extractEditedFiles(msgs)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %v", len(got), got)
	}
}

func TestExtractEditedFiles_EmptyWhenNoAssistants(t *testing.T) {
	msgs := []*parser.Message{
		{Kind: parser.KindUserPrompt, Content: []parser.ContentBlock{{Type: "text", Text: "hi"}}},
	}
	if got := extractEditedFiles(msgs); got != nil {
		t.Errorf("got %v, want nil for user-only messages", got)
	}
}

func TestLastAssistantText_ReturnsFinalSummary(t *testing.T) {
	// Turn with tool calls in the middle, final text at the end
	msgs := []*parser.Message{
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "text", Text: "Let me check"},
				{Type: "tool_use", ToolName: "Bash"},
			},
		},
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "tool_use", ToolName: "Edit"},
			},
		},
		{
			Kind: parser.KindAssistant,
			Content: []parser.ContentBlock{
				{Type: "text", Text: "Done — updated the function."},
			},
		},
	}
	got := lastAssistantText(msgs)
	if got != "Done — updated the function." {
		t.Errorf("got %q, want final summary", got)
	}
}

func TestLastAssistantText_EmptyWhenNoTextBlocks(t *testing.T) {
	msgs := []*parser.Message{
		{
			Kind:    parser.KindAssistant,
			Content: []parser.ContentBlock{{Type: "tool_use", ToolName: "Bash"}},
		},
	}
	if got := lastAssistantText(msgs); got != "" {
		t.Errorf("got %q, want empty string for tool-only turn", got)
	}
}

func TestExecMarkdown_EmptySessionReturnsHeaderOnly(t *testing.T) {
	session := &parser.Session{ID: "abc123"}
	out := ExecMarkdown(session)
	if !strings.Contains(out, "# Session abc123") {
		t.Errorf("expected header with session id, got: %s", out)
	}
}

func TestExecMarkdown_RendersSingleTurn(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	user := &parser.Message{
		UUID:      "u1",
		Kind:      parser.KindUserPrompt,
		Timestamp: start,
		Content:   []parser.ContentBlock{{Type: "text", Text: "add a tests.go file"}},
		Children: []*parser.Message{
			{
				UUID:      "a1",
				Kind:      parser.KindAssistant,
				Timestamp: start.Add(time.Second),
				Content: []parser.ContentBlock{
					{Type: "tool_use", ToolName: "Write", ToolInput: map[string]any{"file_path": "/src/tests.go"}},
				},
			},
			{
				UUID:      "a2",
				Kind:      parser.KindAssistant,
				Timestamp: start.Add(2 * time.Second),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "Created tests.go with the table-driven template you asked for."},
				},
			},
		},
	}
	session := &parser.Session{
		ID:           "s1",
		StartTime:    start,
		EndTime:      start.Add(3 * time.Second),
		RootMessages: []*parser.Message{user},
	}

	out := ExecMarkdown(session)

	for _, want := range []string{
		"# Session s1",
		"## Turn 1",
		"> add a tests.go file",
		"**Files touched:**",
		"`/src/tests.go`",
		"Created tests.go with the table-driven template you asked for.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exec markdown missing %q\n---\n%s", want, out)
		}
	}
}

func TestExecMarkdown_MultipleTurnsNumberedSequentially(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	u1 := &parser.Message{
		UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start,
		Content: []parser.ContentBlock{{Type: "text", Text: "first"}},
		Children: []*parser.Message{
			{UUID: "a1", Kind: parser.KindAssistant, Timestamp: start.Add(time.Second),
				Content: []parser.ContentBlock{{Type: "text", Text: "reply 1"}}},
		},
	}
	u2 := &parser.Message{
		UUID: "u2", Kind: parser.KindUserPrompt, Timestamp: start.Add(10 * time.Second),
		ParentUUID: "a1",
		Content:    []parser.ContentBlock{{Type: "text", Text: "second"}},
		Children: []*parser.Message{
			{UUID: "a2", Kind: parser.KindAssistant, Timestamp: start.Add(11 * time.Second),
				Content: []parser.ContentBlock{{Type: "text", Text: "reply 2"}}},
		},
	}
	// Link u2 as child of a1 in the tree
	u1.Children = append(u1.Children, u2)

	session := &parser.Session{
		ID:           "s",
		StartTime:    start,
		EndTime:      start.Add(12 * time.Second),
		RootMessages: []*parser.Message{u1},
	}

	out := ExecMarkdown(session)
	if !strings.Contains(out, "## Turn 1") {
		t.Error("missing Turn 1 header")
	}
	if !strings.Contains(out, "## Turn 2") {
		t.Error("missing Turn 2 header")
	}
	// Turn 1 should contain its prompt; turn 2 should contain its prompt
	i1 := strings.Index(out, "## Turn 1")
	i2 := strings.Index(out, "## Turn 2")
	if i1 < 0 || i2 < 0 || i2 <= i1 {
		t.Errorf("turn order wrong: Turn 1 at %d, Turn 2 at %d", i1, i2)
	}
	if !strings.Contains(out[i1:i2], "first") {
		t.Error("Turn 1 should contain 'first' prompt")
	}
	if !strings.Contains(out[i2:], "second") {
		t.Error("Turn 2 should contain 'second' prompt")
	}
}

func TestExecMarkdown_DropsTurnsWithNoContent(t *testing.T) {
	// A turn with no text, no edits, no summary — just a meta/marker
	// prompt — should not appear in the exec output.
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	emptyTurn := &parser.Message{
		UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start,
		Content: []parser.ContentBlock{{Type: "text", Text: ""}},
	}
	session := &parser.Session{
		ID:           "s",
		StartTime:    start,
		EndTime:      start.Add(time.Second),
		RootMessages: []*parser.Message{emptyTurn},
	}
	out := ExecMarkdown(session)
	if strings.Contains(out, "## Turn 1") {
		t.Errorf("empty turn should not render, but got:\n%s", out)
	}
}

func TestExecMarkdown_EmitsPerTurnCostWhenPriced(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	user := &parser.Message{
		UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start,
		Content: []parser.ContentBlock{{Type: "text", Text: "do it"}},
		Children: []*parser.Message{
			{
				UUID: "a1", Kind: parser.KindAssistant, Model: "claude-sonnet-4-5",
				Timestamp: start.Add(time.Second),
				Content: []parser.ContentBlock{
					{Type: "text", Text: "done"},
				},
				Usage: &parser.MessageUsage{
					InputTokens: 1000, OutputTokens: 500, CostUSD: 0.0105,
				},
			},
		},
	}
	session := &parser.Session{
		ID:           "s",
		StartTime:    start,
		EndTime:      start.Add(2 * time.Second),
		RootMessages: []*parser.Message{user},
	}
	out := ExecMarkdown(session)
	if !strings.Contains(out, "$0.0105") {
		t.Errorf("expected per-turn cost in output, got:\n%s", out)
	}
	if !strings.Contains(out, "tokens") {
		t.Errorf("expected token count label in output, got:\n%s", out)
	}
}

func TestExport_FormatExecDispatchesToExecMarkdown(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	session := &parser.Session{
		ID:        "dispatch-test",
		StartTime: start,
		EndTime:   start.Add(time.Second),
		RootMessages: []*parser.Message{
			{
				UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start,
				Content: []parser.ContentBlock{{Type: "text", Text: "hi"}},
				Children: []*parser.Message{
					{UUID: "a1", Kind: parser.KindAssistant, Timestamp: start.Add(time.Second),
						Content: []parser.ContentBlock{{Type: "text", Text: "hello"}}},
				},
			},
		},
	}

	out, err := Export(session, ExportOptions{Format: "exec"})
	if err != nil {
		t.Fatalf("Export exec: %v", err)
	}
	if !strings.Contains(out, "# Session dispatch-test") {
		t.Errorf("expected exec header, got:\n%s", out)
	}

	// Also accept exec-md alias
	out2, err := Export(session, ExportOptions{Format: "exec-md"})
	if err != nil {
		t.Fatalf("Export exec-md: %v", err)
	}
	if out != out2 {
		t.Error("exec and exec-md aliases should produce identical output")
	}
}

func TestFormatDurationShort(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{5, "5s"},
		{90, "1m30s"},
		{3665, "1h01m"},
	}
	for _, c := range cases {
		if got := formatDurationShort(c.in); got != c.want {
			t.Errorf("formatDurationShort(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatTokenCountShort(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{42, "42"},
		{1500, "1.5k"},
		{2_500_000, "2.5M"},
	}
	for _, c := range cases {
		if got := formatTokenCountShort(c.in); got != c.want {
			t.Errorf("formatTokenCountShort(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
