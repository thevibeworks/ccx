package web

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestIsSafeURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://EXAMPLE.COM", true},
		{"ftp://example.com", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<h1>hi</h1>", false},
		{"", false},
		{"file:///etc/passwd", false},
	}
	for _, tt := range tests {
		got := isSafeURL(tt.url)
		if got != tt.want {
			t.Errorf("isSafeURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestParseProviderQuery(t *testing.T) {
	tests := []struct {
		input    string
		wantProv string
		wantQ    string
	}{
		{"cc: auth bug", "claude-code", "auth bug"},
		{"cx: codex stuff", "codex", "codex stuff"},
		{"claude-code: test", "claude-code", "test"},
		{"claude: search", "claude-code", "search"},
		{"codex: find", "codex", "find"},
		{"plain search", "", "plain search"},
		{"", "", ""},
		{"CC: uppercase", "claude-code", "uppercase"},
		{"CX: upper", "codex", "upper"},
	}
	for _, tt := range tests {
		prov, q := parseProviderQuery(tt.input)
		if prov != tt.wantProv || q != tt.wantQ {
			t.Errorf("parseProviderQuery(%q) = (%q, %q), want (%q, %q)", tt.input, prov, q, tt.wantProv, tt.wantQ)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "-"},
		{-1, "-"},
		{30, "30s"},
		{60, "1m"},
		{90, "1m 30s"},
		{3600, "1h 0m"},
		{3661, "1h 1m"},
		{7200, "2h 0m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.seconds)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{10000, "10.0k"},
		{999949, "999.9k"},
		{999950, "1.0M"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.n)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path   string
		maxLen int
		want   string
	}{
		{"/short", 20, "/short"},
		{"/very/long/path/to/file.go", 15, "...h/to/file.go"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncatePath(tt.path, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncatePath(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.want)
		}
	}
}

func TestIsActiveTool(t *testing.T) {
	active := []string{"Write", "Edit", "Bash", "Task", "TodoWrite", "Skill", "NotebookEdit", "KillShell", "AskUserQuestion"}
	passive := []string{"Read", "Grep", "Glob", "Agent", "WebSearch"}

	for _, name := range active {
		if !isActiveTool(name) {
			t.Errorf("isActiveTool(%q) = false, want true", name)
		}
	}
	for _, name := range passive {
		if isActiveTool(name) {
			t.Errorf("isActiveTool(%q) = true, want false", name)
		}
	}
}

func TestCompactToolPreview(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input any
		want  string
	}{
		{"read file", "Read", map[string]any{"file_path": "/tmp/foo.go"}, "/tmp/foo.go"},
		{"write file", "Write", map[string]any{"file_path": "/tmp/bar.go"}, "/tmp/bar.go"},
		{"edit file", "Edit", map[string]any{"file_path": "/tmp/baz.go"}, "/tmp/baz.go"},
		{"grep pattern", "Grep", map[string]any{"pattern": "TODO"}, "/TODO/"},
		{"glob pattern", "Glob", map[string]any{"pattern": "**/*.go"}, "**/*.go"},
		{"bash command", "Bash", map[string]any{"command": "ls -la"}, "$ ls -la"},
		{"non-map input", "Bash", "string-input", ""},
		{"nil input", "Bash", nil, ""},
		{"empty map fallback", "Unknown", map[string]any{"key": "val"}, "key=val"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactToolPreview(tt.tool, tt.input)
			if got != tt.want {
				t.Errorf("compactToolPreview(%q, %v) = %q, want %q", tt.tool, tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractSnippet(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		query  string
		maxLen int
		check  func(string) bool
	}{
		{"found", "the quick brown fox jumps over the lazy dog", "fox", 30, func(s string) bool { return s != "" }},
		{"not found", "hello world", "xyz", 30, func(s string) bool { return s == "" }},
		{"at start", "fox is here", "fox", 30, func(s string) bool { return s != "" && s[0] != '.' }},
		{"case insensitive", "The FOX runs", "fox", 30, func(s string) bool { return s != "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSnippet(tt.text, tt.query, tt.maxLen)
			if !tt.check(got) {
				t.Errorf("extractSnippet(%q, %q, %d) = %q", tt.text, tt.query, tt.maxLen, got)
			}
		})
	}
}

func TestSearchSessionContent(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				UUID:    "u1",
				Kind:    parser.KindUserPrompt,
				Type:    "user",
				Content: []parser.ContentBlock{{Type: "text", Text: "Fix the authentication bug"}},
			},
			{
				UUID:    "a1",
				Kind:    parser.KindAssistant,
				Type:    "assistant",
				Content: []parser.ContentBlock{{Type: "text", Text: "I found the issue in auth.go"}},
			},
		},
	}

	snippet, uuid := searchSessionContent(session, "authentication")
	if snippet == "" {
		t.Error("expected snippet for matching query")
	}
	if uuid != "u1" {
		t.Errorf("uuid = %q, want u1", uuid)
	}

	snippet, uuid = searchSessionContent(session, "nonexistent-xyz")
	if snippet != "" || uuid != "" {
		t.Error("expected empty for non-matching query")
	}
}

func TestSearchSessionContentToolUse(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				UUID: "a1",
				Kind: parser.KindAssistant,
				Type: "assistant",
				Content: []parser.ContentBlock{
					{Type: "tool_use", ToolName: "Bash", ToolInput: map[string]any{"command": "grep -r findme ."}},
				},
			},
		},
	}

	snippet, uuid := searchSessionContent(session, "findme")
	if snippet == "" || uuid != "a1" {
		t.Errorf("expected to find query in tool input, got snippet=%q uuid=%q", snippet, uuid)
	}
}

func TestSearchSessionContentToolResult(t *testing.T) {
	session := &parser.Session{
		RootMessages: []*parser.Message{
			{
				UUID: "tr1",
				Kind: parser.KindToolResult,
				Type: "user",
				Content: []parser.ContentBlock{
					{Type: "tool_result", ToolResult: "found matching line in config.go"},
				},
			},
		},
	}

	snippet, uuid := searchSessionContent(session, "config.go")
	if snippet == "" || uuid != "tr1" {
		t.Errorf("expected to find query in tool result, got snippet=%q uuid=%q", snippet, uuid)
	}
}

func TestGetFirstTextContent(t *testing.T) {
	msg := &parser.Message{
		Content: []parser.ContentBlock{
			{Type: "tool_use", ToolName: "Bash"},
			{Type: "text", Text: ""},
			{Type: "text", Text: "actual content"},
		},
	}
	if got := getFirstTextContent(msg); got != "actual content" {
		t.Errorf("got %q, want actual content", got)
	}

	empty := &parser.Message{Content: []parser.ContentBlock{{Type: "tool_use"}}}
	if got := getFirstTextContent(empty); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestGenerateExportFilename(t *testing.T) {
	tests := []struct {
		summary string
		want    string
	}{
		{"Fix the auth bug", "2026-01-15-fix-the-auth-bug.txt"},
		{"A B C D E F G H I", "2026-01-15-a-b-c-d-e-f.txt"},
		{"Special !@# chars $%^", "2026-01-15-special-chars.txt"},
		{"", "2026-01-15-session.txt"},
	}

	for _, tt := range tests {
		s := &parser.Session{
			StartTime: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			Summary:   tt.summary,
		}
		got := generateExportFilename(s)
		if got != tt.want {
			t.Errorf("generateExportFilename(%q) = %q, want %q", tt.summary, got, tt.want)
		}
	}
}

func TestExportTxt(t *testing.T) {
	session := &parser.Session{
		ID: "test-txt",
		RootMessages: []*parser.Message{
			{Kind: parser.KindUserPrompt, Type: "user", Content: []parser.ContentBlock{{Type: "text", Text: "hello"}}},
			{Kind: parser.KindAssistant, Type: "assistant", Content: []parser.ContentBlock{{Type: "text", Text: "hi there"}}},
			{Kind: parser.KindCompactSummary, IsCompacted: true},
		},
	}
	got := exportTxt(session)
	if got == "" {
		t.Fatal("empty output")
	}
	if !contains(got, "> hello") {
		t.Error("missing user prompt")
	}
	if !contains(got, "hi there") {
		t.Error("missing assistant response")
	}
	if !contains(got, "compacted") {
		t.Error("missing compaction marker")
	}
}

func TestExportMessageTxtCommand(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{Kind: parser.KindCommand, CommandName: "/help", CommandArgs: ""}
	exportMessageTxt(&b, msg)
	if !contains(b.String(), "/help") {
		t.Error("missing command name")
	}
}

func TestExportMessageTxtMeta(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{Kind: parser.KindMeta}
	exportMessageTxt(&b, msg)
	if b.String() != "" {
		t.Error("meta should produce no output in txt export")
	}
}

func TestExportMessageTxtToolResult(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{Kind: parser.KindToolResult}
	exportMessageTxt(&b, msg)
	if b.String() != "" {
		t.Error("standalone tool_result should produce no output")
	}
}

func TestExportMessageTxtAssistantWithTools(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		Kind: parser.KindAssistant,
		Type: "assistant",
		Content: []parser.ContentBlock{
			{Type: "text", Text: "running command"},
			{Type: "tool_use", ToolName: "Bash", ToolInput: map[string]any{"command": "ls -la"}},
		},
	}
	exportMessageTxt(&b, msg)
	got := b.String()
	if !contains(got, "Bash(ls -la)") {
		t.Errorf("expected tool preview, got %q", got)
	}
}

func TestFormatAge(t *testing.T) {
	if formatAge(time.Time{}) != "N/A" {
		t.Error("zero time should be N/A")
	}
	if got := formatAge(time.Now().Add(-30 * time.Second)); got != "just now" {
		t.Errorf("30s ago = %q, want just now", got)
	}
	if got := formatAge(time.Now().Add(-5 * time.Minute)); !contains(got, "m ago") {
		t.Errorf("5m ago = %q, want Xm ago", got)
	}
	if got := formatAge(time.Now().Add(-3 * time.Hour)); !contains(got, "h ago") {
		t.Errorf("3h ago = %q, want Xh ago", got)
	}
	if got := formatAge(time.Now().Add(-2 * 24 * time.Hour)); !contains(got, "d ago") {
		t.Errorf("2d ago = %q, want Xd ago", got)
	}
	if got := formatAge(time.Now().Add(-30 * 24 * time.Hour)); contains(got, "ago") {
		t.Errorf("30d ago = %q, want date format", got)
	}
}

func TestFormatRelativeTime(t *testing.T) {
	if formatRelativeTime(time.Time{}) != "" {
		t.Error("zero time should be empty")
	}
	if got := formatRelativeTime(time.Now().Add(-10 * time.Second)); got != "just now" {
		t.Errorf("10s ago = %q, want just now", got)
	}
	if got := formatRelativeTime(time.Now().Add(-1 * time.Minute)); got != "1 min ago" {
		t.Errorf("1m ago = %q, want 1 min ago", got)
	}
	if got := formatRelativeTime(time.Now().Add(-5 * time.Minute)); got != "5 mins ago" {
		t.Errorf("5m ago = %q, want 5 mins ago", got)
	}
	if got := formatRelativeTime(time.Now().Add(-1 * time.Hour)); got != "1 hour ago" {
		t.Errorf("1h ago = %q, want 1 hour ago", got)
	}
	if got := formatRelativeTime(time.Now().Add(-3 * time.Hour)); got != "3 hours ago" {
		t.Errorf("3h ago = %q, want 3 hours ago", got)
	}
	if got := formatRelativeTime(time.Now().Add(-24 * time.Hour)); got != "yesterday" {
		t.Errorf("1d ago = %q, want yesterday", got)
	}
	if got := formatRelativeTime(time.Now().Add(-3 * 24 * time.Hour)); got != "3 days ago" {
		t.Errorf("3d ago = %q, want 3 days ago", got)
	}
}

func TestSplitByCompactBoundaries(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "u1", Kind: parser.KindUserPrompt},
		{UUID: "a1", Kind: parser.KindAssistant},
		{UUID: "c1", Kind: parser.KindCompactSummary},
		{UUID: "u2", Kind: parser.KindUserPrompt},
		{UUID: "a2", Kind: parser.KindAssistant},
	}
	sections := splitByCompactBoundaries(msgs)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if len(sections[0]) != 2 {
		t.Errorf("first section: %d messages, want 2", len(sections[0]))
	}
	if sections[1][0].Kind != parser.KindCompactSummary {
		t.Error("second section should start with compact boundary")
	}
}

func TestSplitByCompactBoundariesEmpty(t *testing.T) {
	sections := splitByCompactBoundaries(nil)
	if len(sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(sections))
	}
}

func TestSplitByCompactBoundariesNoCompaction(t *testing.T) {
	msgs := []*parser.Message{
		{UUID: "u1", Kind: parser.KindUserPrompt},
		{UUID: "a1", Kind: parser.KindAssistant},
	}
	sections := splitByCompactBoundaries(msgs)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
}

func TestSplitByUserPrompts(t *testing.T) {
	msgs := make([]*parser.Message, 0)
	for i := 0; i < 15; i++ {
		kind := parser.KindAssistant
		if i%3 == 0 {
			kind = parser.KindUserPrompt
		}
		msgs = append(msgs, &parser.Message{Kind: kind})
	}
	sections := splitByUserPrompts(msgs, 5)
	if len(sections) < 2 {
		t.Fatalf("expected at least 2 sections for 15 messages with chunk=5, got %d", len(sections))
	}
	for i, s := range sections {
		if len(s) == 0 {
			t.Errorf("section %d is empty", i)
		}
	}
}

func TestSplitByUserPromptsSmall(t *testing.T) {
	msgs := []*parser.Message{
		{Kind: parser.KindUserPrompt},
		{Kind: parser.KindAssistant},
	}
	sections := splitByUserPrompts(msgs, 100)
	if len(sections) != 1 {
		t.Fatalf("small message list should be 1 section, got %d", len(sections))
	}
}

func TestExportOrg(t *testing.T) {
	session := &parser.Session{
		ID:        "test-org-export",
		StartTime: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		RootMessages: []*parser.Message{
			{Kind: parser.KindUserPrompt, Type: "user", Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), Content: []parser.ContentBlock{{Type: "text", Text: "question"}}},
			{Kind: parser.KindAssistant, Type: "assistant", Timestamp: time.Date(2026, 1, 15, 10, 0, 1, 0, time.UTC), Content: []parser.ContentBlock{{Type: "text", Text: "answer"}}},
			{Kind: parser.KindCompactSummary, IsCompacted: true},
		},
	}
	got := exportOrg(session)
	if !contains(got, "#+TITLE:") {
		t.Error("missing org title")
	}
	if !contains(got, "USER") {
		t.Error("missing USER heading")
	}
	if !contains(got, "ASSISTANT") {
		t.Error("missing ASSISTANT heading")
	}
	if !contains(got, "COMPACTED") {
		t.Error("missing COMPACTED marker")
	}
}

func TestExportMessageOrgAllKinds(t *testing.T) {
	kinds := []struct {
		kind    parser.MessageKind
		content []parser.ContentBlock
		check   string
	}{
		{parser.KindUserPrompt, []parser.ContentBlock{{Type: "text", Text: "q"}}, "USER"},
		{parser.KindAssistant, []parser.ContentBlock{{Type: "text", Text: "a"}}, "ASSISTANT"},
		{parser.KindCompactSummary, nil, "COMPACTED"},
		{parser.KindCommand, nil, ""},
		{parser.KindMeta, nil, "System Instructions"},
	}
	for _, tt := range kinds {
		var b strings.Builder
		msg := &parser.Message{Kind: tt.kind, CommandName: "/test", Timestamp: time.Now(), Content: tt.content}
		exportMessageOrg(&b, msg)
		if tt.check != "" && !contains(b.String(), tt.check) {
			t.Errorf("kind %s: missing %q in output %q", tt.kind, tt.check, b.String())
		}
	}
}

func TestExportMessageOrgToolBlocks(t *testing.T) {
	var b strings.Builder
	msg := &parser.Message{
		Kind:      parser.KindAssistant,
		Type:      "assistant",
		Timestamp: time.Now(),
		Content: []parser.ContentBlock{
			{Type: "thinking", Text: "hmm"},
			{Type: "tool_use", ToolName: "Bash", ToolInput: map[string]any{"command": "ls"}},
			{Type: "tool_result", ToolResult: "output"},
		},
	}
	exportMessageOrg(&b, msg)
	got := b.String()
	if !contains(got, "Thinking") {
		t.Error("missing thinking marker")
	}
	if !contains(got, "Bash") {
		t.Error("missing tool name")
	}
	if !contains(got, "#+BEGIN_SRC") {
		t.Error("missing src block")
	}
	if !contains(got, "#+BEGIN_EXAMPLE") {
		t.Error("missing example block")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
