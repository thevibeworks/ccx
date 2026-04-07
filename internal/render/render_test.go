package render

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestTruncateID(t *testing.T) {
	tests := []struct {
		id   string
		n    int
		want string
	}{
		{"abcdef12-3456-7890", 8, "abcdef12"},
		{"short", 10, "short"},
		{"exact8ch", 8, "exact8ch"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		if got := truncateID(tt.id, tt.n); got != tt.want {
			t.Errorf("truncateID(%q, %d) = %q, want %q", tt.id, tt.n, got, tt.want)
		}
		if got := truncateIDMD(tt.id, tt.n); got != tt.want {
			t.Errorf("truncateIDMD(%q, %d) = %q, want %q", tt.id, tt.n, got, tt.want)
		}
		if got := truncateIDOrg(tt.id, tt.n); got != tt.want {
			t.Errorf("truncateIDOrg(%q, %d) = %q, want %q", tt.id, tt.n, got, tt.want)
		}
	}
}

func TestFormatToolInput(t *testing.T) {
	tests := []struct {
		name  string
		input any
		check func(string) bool
	}{
		{"string", "ls -la", func(s string) bool { return s == "ls -la" }},
		{"map", map[string]any{"cmd": "pwd"}, func(s string) bool { return strings.Contains(s, "cmd: pwd") }},
		{"int", 42, func(s string) bool { return s == "42" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolInput(tt.input)
			if !tt.check(got) {
				t.Errorf("formatToolInput(%v) = %q", tt.input, got)
			}
		})
	}
}

func TestFormatToolResult(t *testing.T) {
	t.Run("short string", func(t *testing.T) {
		got := formatToolResult("output")
		if got != "output" {
			t.Errorf("got %q, want output", got)
		}
	})

	t.Run("long string truncated", func(t *testing.T) {
		long := strings.Repeat("x", 6000)
		got := formatToolResult(long)
		if len(got) != 5000 {
			t.Errorf("len = %d, want 5000", len(got))
		}
		if !strings.HasSuffix(got, "...") {
			t.Error("expected ... suffix")
		}
	})

	t.Run("non-string", func(t *testing.T) {
		got := formatToolResult([]int{1, 2, 3})
		if got != "[1 2 3]" {
			t.Errorf("got %q", got)
		}
	})
}

func TestGuessLang(t *testing.T) {
	tests := []struct {
		tool  string
		input string
		want  string
	}{
		{"Bash", "ls", "bash"},
		{"Read", "file_path: main.go", "go"},
		{"Write", "file_path: app.py", "python"},
		{"Edit", "file_path: index.ts", "javascript"},
		{"Read", "file_path: lib.rs", "rust"},
		{"Read", "file_path: data.csv", ""},
		{"Read", "no path", ""},
		{"Agent", "anything", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.input, func(t *testing.T) {
			got := guessLang(tt.tool, tt.input)
			if got != tt.want {
				t.Errorf("guessLang(%q, %q) = %q, want %q", tt.tool, tt.input, got, tt.want)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		check func(string) bool
	}{
		{"short line unchanged", "hello world", 80, func(s string) bool { return s == "hello world" }},
		{"wraps at width", "one two three four five", 10, func(s string) bool { return strings.Contains(s, "\n") }},
		{"preserves newlines", "line1\nline2", 80, func(s string) bool { return strings.Count(s, "\n") >= 1 }},
		{"zero width defaults to 80", "test", 0, func(s string) bool { return s == "test" }},
		{"negative width defaults to 80", "test", -5, func(s string) bool { return s == "test" }},
		{"empty string", "", 80, func(s string) bool { return s == "" }},
		{"blank lines", "a\n\nb", 80, func(s string) bool { return strings.Contains(s, "\n\n") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.width)
			if !tt.check(got) {
				t.Errorf("wrapText(%q, %d) = %q", tt.text, tt.width, got)
			}
		})
	}
}

func TestRenderHTMLBlockText(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "text", Text: "Hello\n\nWorld"}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "<p>Hello</p>") {
		t.Errorf("expected paragraph, got %q", got)
	}
	if !strings.Contains(got, "<p>World</p>") {
		t.Errorf("expected second paragraph, got %q", got)
	}
}

func TestRenderHTMLBlockTextEmpty(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "text", Text: ""}, ExportOptions{})
	if b.String() != "" {
		t.Errorf("empty text should produce no output, got %q", b.String())
	}
}

func TestRenderHTMLBlockThinking(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "thinking", Text: "reasoning"}, ExportOptions{IncludeThinking: true})
	got := b.String()
	if !strings.Contains(got, "details") {
		t.Error("thinking should render as details")
	}
	if !strings.Contains(got, "reasoning") {
		t.Error("thinking text missing")
	}
}

func TestRenderHTMLBlockThinkingHidden(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "thinking", Text: "reasoning"}, ExportOptions{IncludeThinking: false})
	if b.String() != "" {
		t.Error("thinking should be hidden when IncludeThinking=false")
	}
}

func TestRenderHTMLBlockToolUse(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "tool_use", ToolName: "Bash", ToolInput: "ls -la"}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "Bash") {
		t.Error("tool name missing")
	}
	if !strings.Contains(got, "ls -la") {
		t.Error("tool input missing")
	}
}

func TestRenderHTMLBlockToolResult(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "tool_result", ToolResult: "file.txt", IsError: true}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "error") {
		t.Error("error class missing")
	}
	if !strings.Contains(got, "file.txt") {
		t.Error("result missing")
	}
}

func TestRenderHTMLBlockImage(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "image", MediaType: "image/png", ImageData: "iVBORw0="}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "data:image/png;base64,iVBORw0=") {
		t.Errorf("image src missing, got %q", got)
	}
}

func TestRenderHTMLBlockImageEmpty(t *testing.T) {
	var b strings.Builder
	renderHTMLBlock(&b, parser.ContentBlock{Type: "image", MediaType: "image/png", ImageData: ""}, ExportOptions{})
	if b.String() != "" {
		t.Error("empty image should produce no output")
	}
}

func TestRenderMarkdownBlockText(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "text", Text: "hello"}, ExportOptions{})
	if !strings.Contains(b.String(), "hello") {
		t.Error("text missing")
	}
}

func TestRenderMarkdownBlockThinking(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "thinking", Text: "hmm"}, ExportOptions{IncludeThinking: true})
	got := b.String()
	if !strings.Contains(got, "<details>") {
		t.Error("details tag missing")
	}
	if !strings.Contains(got, "hmm") {
		t.Error("thinking text missing")
	}
}

func TestRenderMarkdownBlockToolUseShort(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "tool_use", ToolName: "Bash", ToolInput: "ls"}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "`ls`") {
		t.Errorf("short input should be inline code, got %q", got)
	}
}

func TestRenderMarkdownBlockToolUseLong(t *testing.T) {
	var b strings.Builder
	longInput := strings.Repeat("x", 100)
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "tool_use", ToolName: "Bash", ToolInput: longInput}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "```") {
		t.Error("long input should use code fence")
	}
}

func TestRenderMarkdownBlockToolUseMultiline(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "tool_use", ToolName: "Bash", ToolInput: "line1\nline2"}, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "```") {
		t.Error("multiline input should use code fence")
	}
}

func TestRenderMarkdownBlockToolResult(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "tool_result", ToolResult: "output", IsError: false}, ExportOptions{})
	if !strings.Contains(b.String(), "### Result") {
		t.Error("result header missing")
	}
}

func TestRenderMarkdownBlockToolResultError(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "tool_result", ToolResult: "fail", IsError: true}, ExportOptions{})
	if !strings.Contains(b.String(), "### Error") {
		t.Error("error header missing")
	}
}

func TestRenderMarkdownBlockImage(t *testing.T) {
	var b strings.Builder
	renderMarkdownBlock(&b, parser.ContentBlock{Type: "image", MediaType: "image/png"}, ExportOptions{})
	if !strings.Contains(b.String(), "![Image]") {
		t.Error("image markdown missing")
	}
}

func TestRenderOrgBlockText(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "text", Text: "hello"}, 2, ExportOptions{})
	if !strings.Contains(b.String(), "hello") {
		t.Error("text missing")
	}
}

func TestRenderOrgBlockThinking(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "thinking", Text: "hmm"}, 2, ExportOptions{IncludeThinking: true})
	got := b.String()
	if !strings.Contains(got, "THINKING") {
		t.Error("thinking header missing")
	}
	if !strings.Contains(got, ":PROPERTIES:") {
		t.Error("properties drawer missing")
	}
}

func TestRenderOrgBlockToolUse(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "tool_use", ToolName: "Bash", ToolInput: "pwd"}, 2, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "TOOL: Bash") {
		t.Error("tool header missing")
	}
	if !strings.Contains(got, "#+BEGIN_SRC bash") {
		t.Error("bash src block missing")
	}
}

func TestRenderOrgBlockToolResult(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "tool_result", ToolResult: "output"}, 2, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "RESULT") {
		t.Error("result header missing")
	}
	if !strings.Contains(got, "#+BEGIN_EXAMPLE") {
		t.Error("example block missing")
	}
}

func TestRenderOrgBlockToolResultError(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "tool_result", ToolResult: "fail", IsError: true}, 2, ExportOptions{})
	if !strings.Contains(b.String(), "ERROR") {
		t.Error("error label missing")
	}
}

func TestRenderOrgBlockImage(t *testing.T) {
	var b strings.Builder
	renderOrgBlock(&b, parser.ContentBlock{Type: "image", MediaType: "image/png", ImageData: "iVBORw0KGgoAAAANSUhEUg=="}, 2, ExportOptions{})
	if !strings.Contains(b.String(), "[[data:image/png") {
		t.Error("image link missing")
	}
}

func TestRenderHTMLMessageCompacted(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		IsCompacted: true,
		Content:     []parser.ContentBlock{{Type: "text", Text: "compacted context"}},
	}
	renderHTMLMessage(&b, msg, 0, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "COMPACTED") {
		t.Error("compacted header missing")
	}
	if !strings.Contains(got, "compacted context") {
		t.Error("compacted text missing")
	}
}

func TestRenderHTMLMessageSidechainFiltered(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{IsSidechain: true, Type: "assistant"}
	renderHTMLMessage(&b, msg, 0, ExportOptions{IncludeAgents: false})
	if b.String() != "" {
		t.Error("sidechain should be filtered")
	}
}

func TestRenderHTMLMessageSidechainIncluded(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		IsSidechain: true,
		Type:        "assistant",
		Timestamp:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Content:     []parser.ContentBlock{{Type: "text", Text: "agent output"}},
	}
	renderHTMLMessage(&b, msg, 0, ExportOptions{IncludeAgents: true})
	got := b.String()
	if !strings.Contains(got, "sidechain") {
		t.Error("sidechain class missing")
	}
	if !strings.Contains(got, "agent output") {
		t.Error("agent text missing")
	}
}

func TestRenderHTMLMessageRecurses(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		Type:      "user",
		Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Content:   []parser.ContentBlock{{Type: "text", Text: "parent"}},
		Children: []*parser.Message{
			{
				Type:      "assistant",
				Timestamp: time.Date(2026, 1, 1, 10, 0, 1, 0, time.UTC),
				Content:   []parser.ContentBlock{{Type: "text", Text: "child"}},
			},
		},
	}
	renderHTMLMessage(&b, msg, 0, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "parent") || !strings.Contains(got, "child") {
		t.Error("recursion failed")
	}
}

func TestRenderMarkdownMessageCompacted(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		IsCompacted: true,
		Content:     []parser.ContentBlock{{Type: "text", Text: "summary"}},
	}
	renderMarkdownMessage(&b, msg, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "Compacted") {
		t.Error("compacted header missing")
	}
}

func TestRenderOrgMessageCompacted(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		IsCompacted: true,
		Content:     []parser.ContentBlock{{Type: "text", Text: "summary"}},
	}
	renderOrgMessage(&b, msg, 2, ExportOptions{})
	got := b.String()
	if !strings.Contains(got, "COMPACTED") {
		t.Error("compacted header missing")
	}
}

func TestExportHTMLEscapesContent(t *testing.T) {
	session := &parser.Session{
		ID: "test-escape",
		RootMessages: []*parser.Message{
			{
				Type:      "user",
				Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
				Content:   []parser.ContentBlock{{Type: "text", Text: "<script>alert('xss')</script>"}},
			},
		},
	}
	result, err := Export(session, ExportOptions{Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "<script>alert") {
		t.Error("HTML should escape script tags")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("script tag should be escaped")
	}
}
