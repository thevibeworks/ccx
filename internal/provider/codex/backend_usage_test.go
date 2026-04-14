package codex

import (
	"testing"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestLatestAssistantWithoutUsage_SkipsTaggedMessages(t *testing.T) {
	tagged := &parser.Message{
		UUID:  "a1",
		Kind:  parser.KindAssistant,
		Usage: &parser.MessageUsage{InputTokens: 100},
	}
	untagged := &parser.Message{
		UUID: "a2",
		Kind: parser.KindAssistant,
	}
	user := &parser.Message{
		UUID: "u1",
		Kind: parser.KindUserPrompt,
	}
	messages := []*parser.Message{user, tagged, untagged}

	got := latestAssistantWithoutUsage(messages)
	if got == nil {
		t.Fatal("expected to find untagged assistant, got nil")
	}
	if got.UUID != "a2" {
		t.Errorf("got %q, want a2 (the untagged one)", got.UUID)
	}
}

func TestLatestAssistantWithoutUsage_ReturnsNilWhenAllTagged(t *testing.T) {
	messages := []*parser.Message{
		{
			UUID:  "a1",
			Kind:  parser.KindAssistant,
			Usage: &parser.MessageUsage{InputTokens: 100},
		},
		{
			UUID:  "a2",
			Kind:  parser.KindAssistant,
			Usage: &parser.MessageUsage{InputTokens: 200},
		},
	}
	if got := latestAssistantWithoutUsage(messages); got != nil {
		t.Errorf("got %v, want nil (every assistant tagged)", got)
	}
}

func TestLatestAssistantWithoutUsage_ReturnsNilWhenNoAssistants(t *testing.T) {
	messages := []*parser.Message{
		{Kind: parser.KindUserPrompt, UUID: "u1"},
		{Kind: parser.KindToolResult, UUID: "t1"},
	}
	if got := latestAssistantWithoutUsage(messages); got != nil {
		t.Errorf("got %v, want nil (no assistants)", got)
	}
}

func TestLatestAssistantWithoutUsage_PicksLatestNotFirst(t *testing.T) {
	// Multiple untagged assistants in wire order — we want the LATEST
	// untagged one (closest to the token_count event that's attributing).
	messages := []*parser.Message{
		{Kind: parser.KindAssistant, UUID: "a1"},
		{Kind: parser.KindUserPrompt, UUID: "u2"},
		{Kind: parser.KindAssistant, UUID: "a2"},
	}
	got := latestAssistantWithoutUsage(messages)
	if got == nil || got.UUID != "a2" {
		t.Errorf("got %v, want a2 (latest untagged)", got)
	}
}

func TestClampNonNegative(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{100, 100},
		{0, 0},
		{-1, 0},
		{-1000, 0},
	}
	for _, c := range cases {
		if got := clampNonNegative(c.in); got != c.want {
			t.Errorf("clampNonNegative(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
