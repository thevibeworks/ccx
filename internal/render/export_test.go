package render

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func makeTestSession() *parser.Session {
	return &parser.Session{
		ID:        "abc12345-test-session",
		Summary:   "Test export session",
		StartTime: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Stats:     parser.SessionStats{MessageCount: 2, UserPrompts: 1},
		RootMessages: []*parser.Message{
			{
				UUID:      "u1",
				Kind:      parser.KindUserPrompt,
				Type:      "user",
				Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
				Content:   []parser.ContentBlock{{Type: "text", Text: "Explain concurrency"}},
			},
			{
				UUID:      "a1",
				Kind:      parser.KindAssistant,
				Type:      "assistant",
				Timestamp: time.Date(2026, 1, 15, 10, 0, 5, 0, time.UTC),
				Content:   []parser.ContentBlock{{Type: "text", Text: "Concurrency allows multiple tasks to progress simultaneously."}},
			},
		},
	}
}

func TestExportHTML(t *testing.T) {
	result, err := Export(makeTestSession(), ExportOptions{Format: "html", Theme: "dark"})
	if err != nil {
		t.Fatalf("Export(html) error: %v", err)
	}
	if !strings.Contains(result, "<html") {
		t.Fatal("expected HTML output")
	}
	if !strings.Contains(result, "Explain concurrency") {
		t.Fatal("expected user message in output")
	}
}

func TestExportMarkdown(t *testing.T) {
	result, err := Export(makeTestSession(), ExportOptions{Format: "md"})
	if err != nil {
		t.Fatalf("Export(md) error: %v", err)
	}
	if !strings.Contains(result, "Explain concurrency") {
		t.Fatal("expected user message in output")
	}
	if !strings.Contains(result, "## ") {
		t.Fatal("expected markdown headers")
	}
}

func TestExportMarkdownAlias(t *testing.T) {
	result, err := Export(makeTestSession(), ExportOptions{Format: "markdown"})
	if err != nil {
		t.Fatalf("Export(markdown) error: %v", err)
	}
	if !strings.Contains(result, "Explain concurrency") {
		t.Fatal("expected user message in output")
	}
}

func TestExportOrg(t *testing.T) {
	result, err := Export(makeTestSession(), ExportOptions{Format: "org"})
	if err != nil {
		t.Fatalf("Export(org) error: %v", err)
	}
	if !strings.Contains(result, "Explain concurrency") {
		t.Fatal("expected user message in output")
	}
	if !strings.Contains(result, "* ") {
		t.Fatal("expected org-mode headers")
	}
}

func TestExportUnsupportedFormat(t *testing.T) {
	_, err := Export(makeTestSession(), ExportOptions{Format: "pdf"})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportFormatCaseInsensitive(t *testing.T) {
	_, err := Export(makeTestSession(), ExportOptions{Format: "HTML"})
	if err != nil {
		t.Fatalf("Export(HTML) should work case-insensitively: %v", err)
	}
}

func TestExportBriefFiltersMeta(t *testing.T) {
	session := &parser.Session{
		ID: "brief-test",
		RootMessages: []*parser.Message{
			{Kind: parser.KindMeta, IsMeta: true, Type: "user", Content: []parser.ContentBlock{{Type: "text", Text: "system instructions"}}},
			{Kind: parser.KindUserPrompt, Type: "user", Content: []parser.ContentBlock{{Type: "text", Text: "visible question"}}},
			{Kind: parser.KindAssistant, Type: "assistant", Content: []parser.ContentBlock{{Type: "text", Text: "visible answer"}}},
		},
	}
	result, err := Export(session, ExportOptions{Format: "md", Brief: true})
	if err != nil {
		t.Fatalf("Export(brief) error: %v", err)
	}
	if strings.Contains(result, "system instructions") {
		t.Fatal("brief should filter out meta messages")
	}
	if !strings.Contains(result, "visible question") {
		t.Fatal("brief should keep user prompts")
	}
	if !strings.Contains(result, "visible answer") {
		t.Fatal("brief should keep assistant responses")
	}
}

func TestExportHTMLLightTheme(t *testing.T) {
	result, err := Export(makeTestSession(), ExportOptions{Format: "html", Theme: "light"})
	if err != nil {
		t.Fatalf("Export(html,light) error: %v", err)
	}
	if !strings.Contains(result, "<html") {
		t.Fatal("expected HTML output")
	}
}

func TestExportOrgIncludesStats(t *testing.T) {
	session := makeTestSession()
	session.Stats.Continuations = 2
	result, err := Export(session, ExportOptions{Format: "org"})
	if err != nil {
		t.Fatalf("Export(org) error: %v", err)
	}
	if !strings.Contains(result, "2") {
		t.Fatal("expected continuations in org output")
	}
}
