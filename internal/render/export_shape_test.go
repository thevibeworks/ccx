package render

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func makeSessionWithOneExchange(t *testing.T) *parser.Session {
	t.Helper()
	start := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	user := &parser.Message{
		UUID:      "u1",
		Kind:      parser.KindUserPrompt,
		Type:      "user",
		Timestamp: start,
		Content:   []parser.ContentBlock{{Type: "text", Text: "build me a timeline rail"}},
	}
	assistant := &parser.Message{
		UUID:       "a1",
		Kind:       parser.KindAssistant,
		Type:       "assistant",
		Timestamp:  start.Add(30 * time.Second),
		ParentUUID: "u1",
		Content: []parser.ContentBlock{
			{Type: "text", Text: "sure, here's my plan"},
		},
	}
	user.Children = []*parser.Message{assistant}
	return &parser.Session{
		ID:           "abcdef1234",
		StartTime:    start,
		EndTime:      start.Add(30 * time.Second),
		RootMessages: []*parser.Message{user},
	}
}

func TestExport_ShapeExchangeReturnsDigest(t *testing.T) {
	session := makeSessionWithOneExchange(t)
	out, err := Export(session, ExportOptions{
		Format: "md",
		Shape:  ShapeExchange,
	})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if !strings.Contains(out, "# Session digest") {
		t.Errorf("shape=exchange should produce a digest header, got: %s", out)
	}
	if !strings.Contains(out, "exchange 1") {
		t.Errorf("shape=exchange should number exchanges, got: %s", out)
	}
	if !strings.Contains(out, "build me a timeline rail") {
		t.Errorf("shape=exchange should include the prompt snippet, got: %s", out)
	}
	if !strings.Contains(out, "sure, here's my plan") {
		t.Errorf("shape=exchange should include the final reply, got: %s", out)
	}
}

func TestExport_HTMLFragmentEnvelopeOmitsDoctype(t *testing.T) {
	session := makeSessionWithOneExchange(t)
	out, err := Export(session, ExportOptions{
		Format:   "html",
		Envelope: EnvelopeFragment,
	})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if strings.Contains(out, "<!DOCTYPE html>") {
		t.Errorf("fragment envelope should not emit DOCTYPE, got: %s", out)
	}
	if strings.Contains(out, "<html") {
		t.Errorf("fragment envelope should not emit <html>, got: %s", out)
	}
	if !strings.Contains(out, "messages") {
		t.Errorf("fragment envelope should still emit the messages container, got: %s", out)
	}
}

func TestExport_HTMLStandaloneEnvelopeEmitsDoctype(t *testing.T) {
	session := makeSessionWithOneExchange(t)
	out, err := Export(session, ExportOptions{
		Format:   "html",
		Envelope: EnvelopeStandalone,
	})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if !strings.Contains(out, "<!DOCTYPE html>") {
		t.Errorf("standalone envelope should emit DOCTYPE, got: %s", out)
	}
}

func TestExport_ShapeBriefStillWorksViaLegacyFlag(t *testing.T) {
	// Back-compat: the legacy Brief bool must still produce a brief
	// session for callers that haven't migrated to Shape yet.
	session := makeSessionWithOneExchange(t)
	out, err := Export(session, ExportOptions{
		Format: "md",
		Brief:  true,
	})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if out == "" {
		t.Error("legacy Brief flag produced empty output")
	}
}

func TestExport_ShapeInvalidFallsBackToFull(t *testing.T) {
	// Empty Shape should behave like full. Unknown shapes never get
	// past the cmd.go validator, so we don't need a dedicated branch
	// — empty is the same as full.
	session := makeSessionWithOneExchange(t)
	outFull, err1 := Export(session, ExportOptions{Format: "md", Shape: ShapeFull})
	outEmpty, err2 := Export(session, ExportOptions{Format: "md"})
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v / %v", err1, err2)
	}
	if outFull != outEmpty {
		t.Errorf("empty shape should equal ShapeFull")
	}
}

func TestExport_ShapeTraceDropsSidechains(t *testing.T) {
	start := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	sidechainAssistant := &parser.Message{
		UUID:        "sc1",
		Kind:        parser.KindAssistant,
		Type:        "assistant",
		Timestamp:   start,
		IsSidechain: true,
		Content:     []parser.ContentBlock{{Type: "text", Text: "sidechain secret"}},
	}
	assistant := &parser.Message{
		UUID:      "a1",
		Kind:      parser.KindAssistant,
		Type:      "assistant",
		Timestamp: start,
		Content: []parser.ContentBlock{
			{Type: "text", Text: "main reply"},
		},
	}
	user := &parser.Message{
		UUID:      "u1",
		Kind:      parser.KindUserPrompt,
		Type:      "user",
		Timestamp: start,
		Content:   []parser.ContentBlock{{Type: "text", Text: "prompt"}},
		Children:  []*parser.Message{assistant, sidechainAssistant},
	}
	session := &parser.Session{ID: "t", RootMessages: []*parser.Message{user}}

	out, err := Export(session, ExportOptions{Format: "md", Shape: ShapeTrace})
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if strings.Contains(out, "sidechain secret") {
		t.Errorf("trace shape should drop sidechain bodies, got: %s", out)
	}
	if !strings.Contains(out, "main reply") {
		t.Errorf("trace shape should keep main reply, got: %s", out)
	}
}
