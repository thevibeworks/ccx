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

func TestMaxInt(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{1, 2, 2},
		{10, 5, 10},
		{0, 0, 0},
		{-5, 3, 3},
	}
	for _, c := range cases {
		if got := maxInt(c.a, c.b); got != c.want {
			t.Errorf("maxInt(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestDistributeCodexDelta_EvenSplitAcrossUntagged(t *testing.T) {
	// Multi-assistant turn (reasoning + agent_message): both should
	// receive part of the delta, not just the latest one. This is the
	// regression guard for the round 1 review finding that
	// latestAssistantWithoutUsage dropped tokens on multi-assistant
	// turns.
	m1 := &parser.Message{UUID: "r1", Kind: parser.KindAssistant, Model: "gpt-5"}
	m2 := &parser.Message{UUID: "a1", Kind: parser.KindAssistant, Model: "gpt-5"}
	messages := []*parser.Message{m1, m2}

	delta := parser.MessageUsage{
		InputTokens:     1000,
		OutputTokens:    600,
		CacheReadTokens: 200,
		ReasoningTokens: 400,
	}
	distributeCodexDelta(messages, delta)

	if m1.Usage == nil || m2.Usage == nil {
		t.Fatal("both assistants should have Usage set")
	}
	// Each should get half (integer division, first absorbs remainder)
	if m1.Usage.InputTokens != 500 || m2.Usage.InputTokens != 500 {
		t.Errorf("InputTokens split = %d/%d, want 500/500", m1.Usage.InputTokens, m2.Usage.InputTokens)
	}
	if m1.Usage.OutputTokens != 300 || m2.Usage.OutputTokens != 300 {
		t.Errorf("OutputTokens split = %d/%d, want 300/300", m1.Usage.OutputTokens, m2.Usage.OutputTokens)
	}
	if m1.Usage.ReasoningTokens != 200 || m2.Usage.ReasoningTokens != 200 {
		t.Errorf("ReasoningTokens split = %d/%d, want 200/200", m1.Usage.ReasoningTokens, m2.Usage.ReasoningTokens)
	}

	// Cost is computed (non-zero) because both have a known model
	if m1.Usage.CostUSD == 0 || m2.Usage.CostUSD == 0 {
		t.Errorf("expected non-zero CostUSD for both (model gpt-5), got %v / %v",
			m1.Usage.CostUSD, m2.Usage.CostUSD)
	}
}

func TestDistributeCodexDelta_RemainderGoesToFirst(t *testing.T) {
	// 3 messages, delta of 10 input tokens → each gets 3, first gets +1
	// remainder. Sum must equal the original delta.
	m1 := &parser.Message{UUID: "a1", Kind: parser.KindAssistant, Model: "gpt-5"}
	m2 := &parser.Message{UUID: "a2", Kind: parser.KindAssistant, Model: "gpt-5"}
	m3 := &parser.Message{UUID: "a3", Kind: parser.KindAssistant, Model: "gpt-5"}
	msgs := []*parser.Message{m1, m2, m3}

	distributeCodexDelta(msgs, parser.MessageUsage{InputTokens: 10})

	if m1.Usage.InputTokens != 4 {
		t.Errorf("m1 input = %d, want 4 (3 + 1 remainder)", m1.Usage.InputTokens)
	}
	if m2.Usage.InputTokens != 3 {
		t.Errorf("m2 input = %d, want 3", m2.Usage.InputTokens)
	}
	if m3.Usage.InputTokens != 3 {
		t.Errorf("m3 input = %d, want 3", m3.Usage.InputTokens)
	}
	// Sum must equal original
	total := m1.Usage.InputTokens + m2.Usage.InputTokens + m3.Usage.InputTokens
	if total != 10 {
		t.Errorf("sum = %d, want 10 (delta must be preserved)", total)
	}
}

func TestDistributeCodexDelta_SkipsTaggedMessages(t *testing.T) {
	// Already-tagged assistants (from a previous token_count event)
	// should NOT be re-attributed on the next event.
	alreadyTagged := &parser.Message{
		UUID:  "a1",
		Kind:  parser.KindAssistant,
		Model: "gpt-5",
		Usage: &parser.MessageUsage{InputTokens: 100},
	}
	untagged := &parser.Message{UUID: "a2", Kind: parser.KindAssistant, Model: "gpt-5"}
	msgs := []*parser.Message{alreadyTagged, untagged}

	distributeCodexDelta(msgs, parser.MessageUsage{InputTokens: 500})

	if alreadyTagged.Usage.InputTokens != 100 {
		t.Errorf("tagged message should keep its original usage, got %d", alreadyTagged.Usage.InputTokens)
	}
	if untagged.Usage == nil || untagged.Usage.InputTokens != 500 {
		t.Errorf("untagged should get the full delta, got %v", untagged.Usage)
	}
}

func TestDistributeCodexDelta_SilentlyDropsWithNoUntaggedTargets(t *testing.T) {
	// Every assistant is tagged — delta has nowhere to go. Should not
	// panic and should not overwrite any existing Usage.
	tagged := &parser.Message{
		UUID:  "a1",
		Kind:  parser.KindAssistant,
		Model: "gpt-5",
		Usage: &parser.MessageUsage{InputTokens: 42},
	}
	msgs := []*parser.Message{tagged}

	distributeCodexDelta(msgs, parser.MessageUsage{InputTokens: 9999})

	if tagged.Usage.InputTokens != 42 {
		t.Errorf("tagged usage should be untouched, got %d", tagged.Usage.InputTokens)
	}
}
