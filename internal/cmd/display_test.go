package cmd

import (
	"strings"
	"testing"
)

func TestSessionSummaryPreviewCollapsesMarkdownAndNewlines(t *testing.T) {
	input := "initiatives & request: ```\n\n› we've built and released our ccx project\n\n```"
	got := sessionSummaryPreview(input, 80)

	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("preview contains table-breaking whitespace: %q", got)
	}
	if got != "initiatives & request: ``` › we've built and released our ccx project" {
		t.Fatalf("preview = %q", got)
	}
}

func TestSessionSummaryPreviewNormalizesIndentedReviewPrompt(t *testing.T) {
	input := "   Second review pass for the scoped-session refac...\n  with details"
	got := sessionSummaryPreview(input, 64)

	if got != "Second review pass for the scoped-session refac... with details" {
		t.Fatalf("preview = %q", got)
	}
}

func TestSessionSummaryPreviewDropsControlBytes(t *testing.T) {
	input := "fix\x00 auth\x1b[31m bug\nnow"
	got := sessionSummaryPreview(input, 64)

	if strings.Contains(got, "\x00") || strings.Contains(got, "\x1b") || strings.Contains(got, "\n") {
		t.Fatalf("preview retained control bytes: %q", got)
	}
	if got != "fix auth bug now" {
		t.Fatalf("preview = %q", got)
	}
}

func TestSessionSummaryPreviewTruncatesAfterNormalization(t *testing.T) {
	input := "hello\nworld from ccx sessions list"
	got := sessionSummaryPreview(input, 18)

	if got != "hello world fro..." {
		t.Fatalf("preview = %q", got)
	}
}

func TestSessionSummaryPreviewFallback(t *testing.T) {
	got := sessionSummaryPreview("\n\t  ", 20)
	if got != "(no summary)" {
		t.Fatalf("preview = %q", got)
	}
}
