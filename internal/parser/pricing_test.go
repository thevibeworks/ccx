package parser

import (
	"math"
	"testing"
)

func TestLookupPricing_ExactMatch(t *testing.T) {
	p := LookupPricing("claude-sonnet-4-5")
	if p == nil {
		t.Fatal("expected pricing for claude-sonnet-4-5, got nil")
	}
	if p.InputPer1M != 3.00 {
		t.Errorf("InputPer1M = %v, want 3.00", p.InputPer1M)
	}
	if p.OutputPer1M != 15.00 {
		t.Errorf("OutputPer1M = %v, want 15.00", p.OutputPer1M)
	}
}

func TestLookupPricing_DateSuffixStripped(t *testing.T) {
	p := LookupPricing("claude-sonnet-4-5-20250929")
	if p == nil {
		t.Fatal("expected pricing for dated sonnet, got nil")
	}
	if p.Model != "claude-sonnet-4-5" {
		t.Errorf("Model = %q, want %q", p.Model, "claude-sonnet-4-5")
	}
}

func TestLookupPricing_UnknownReturnsNil(t *testing.T) {
	if p := LookupPricing("gpt-4"); p != nil {
		t.Errorf("expected nil for unknown model, got %+v", p)
	}
}

func TestLookupPricing_EmptyReturnsNil(t *testing.T) {
	if p := LookupPricing(""); p != nil {
		t.Errorf("expected nil for empty model, got %+v", p)
	}
}

func TestLookupPricing_UnknownFamilyReturnsNil(t *testing.T) {
	// Guard against cross-family mismatches: any model name that's
	// not in a family we explicitly price (Claude, GPT-5/5.4) must
	// return nil. Keeps us from silently pricing Grok or Llama
	// requests at Claude rates.
	unknown := []string{
		"gpt-4",
		"gpt-4-turbo",
		"grok-2",
		"llama-3-70b",
		"mistral-large",
		"",
	}
	for _, model := range unknown {
		if p := LookupPricing(model); p != nil {
			t.Errorf("LookupPricing(%q) = %+v, want nil (unknown family)", model, p)
		}
	}
}

func TestLookupPricing_GPT5Family(t *testing.T) {
	cases := []struct {
		model     string
		canonical string
		wantInput float64
	}{
		{"gpt-5", "gpt-5.4", 10.00},
		{"gpt-5.4", "gpt-5.4", 10.00},
		{"gpt-5-mini", "gpt-5.4-mini", 0.25},
		{"gpt-5.4-mini", "gpt-5.4-mini", 0.25},
		{"gpt-5-nano", "gpt-5.4-nano", 0.05},
		{"gpt-5.4-nano", "gpt-5.4-nano", 0.05},
		// mini/nano must win before the plain gpt-5 substring
		{"GPT-5.4-MINI", "gpt-5.4-mini", 0.25},
	}
	for _, c := range cases {
		p := LookupPricing(c.model)
		if p == nil {
			t.Errorf("LookupPricing(%q) returned nil", c.model)
			continue
		}
		if p.Model != c.canonical {
			t.Errorf("LookupPricing(%q).Model = %q, want %q", c.model, p.Model, c.canonical)
		}
		if p.InputPer1M != c.wantInput {
			t.Errorf("LookupPricing(%q).InputPer1M = %v, want %v", c.model, p.InputPer1M, c.wantInput)
		}
	}
}

func TestComputeCost_ReasoningTokensBilledAsOutput(t *testing.T) {
	// Codex / GPT-5 reasoning tokens should be billed at the same
	// rate as regular output tokens.
	pricing := &ModelPricing{InputPer1M: 10, OutputPer1M: 80}
	usage := &MessageUsage{
		InputTokens:     1_000_000, // $10
		OutputTokens:    1_000_000, // $80
		ReasoningTokens: 500_000,   // $40
	}
	got := ComputeCost(usage, pricing)
	want := 10.0 + 80.0 + 40.0 // 130 USD
	if got != want {
		t.Errorf("ComputeCost = %v, want %v", got, want)
	}
}

func TestLookupPricing_DatedBedrockAndBareNamesAllMap(t *testing.T) {
	// Mirror Claude Code's firstPartyNameToCanonical behaviour: dated
	// IDs, Bedrock ARNs, and bare canonical names should all resolve
	// to the same pricing.
	variants := []string{
		"claude-sonnet-4-5",
		"claude-sonnet-4-5-20250929",
		"us.anthropic.claude-sonnet-4-5-20250929-v1:0",
	}
	for _, v := range variants {
		p := LookupPricing(v)
		if p == nil {
			t.Errorf("LookupPricing(%q) returned nil", v)
			continue
		}
		if p.InputPer1M != 3.00 {
			t.Errorf("LookupPricing(%q) InputPer1M = %v, want 3.00", v, p.InputPer1M)
		}
	}
}

func TestLookupPricing_OpusMoreSpecificBeforeLess(t *testing.T) {
	// Match order matters: "claude-opus-4-6" must win before
	// "claude-opus-4" because opus 4.6 is priced at tier 5/25,
	// not tier 15/75.
	p46 := LookupPricing("claude-opus-4-6")
	if p46 == nil || p46.InputPer1M != 5.00 {
		t.Errorf("claude-opus-4-6 InputPer1M = %v, want 5.00", p46)
	}

	p4 := LookupPricing("claude-opus-4-20250514")
	if p4 == nil || p4.InputPer1M != 15.00 {
		t.Errorf("claude-opus-4 (bare) InputPer1M = %v, want 15.00 (not 5.00)", p4)
	}

	p41 := LookupPricing("claude-opus-4-1-20250805")
	if p41 == nil || p41.InputPer1M != 15.00 {
		t.Errorf("claude-opus-4-1 InputPer1M = %v, want 15.00", p41)
	}
}

func TestComputeCost_AllCategories(t *testing.T) {
	pricing := &ModelPricing{
		InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75,
	}
	usage := &MessageUsage{
		InputTokens:       1_000_000,
		OutputTokens:      1_000_000,
		CacheReadTokens:   1_000_000,
		CacheCreateTokens: 1_000_000,
	}
	got := ComputeCost(usage, pricing)
	want := 3.00 + 15.00 + 0.30 + 3.75 // 22.05 USD
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("ComputeCost = %v, want %v", got, want)
	}
}

func TestComputeCost_SmallNumbersScaleCorrectly(t *testing.T) {
	pricing := &ModelPricing{InputPer1M: 3.00, OutputPer1M: 15.00}
	usage := &MessageUsage{InputTokens: 500, OutputTokens: 200}
	got := ComputeCost(usage, pricing)
	want := 500*3.00/1_000_000.0 + 200*15.00/1_000_000.0
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("ComputeCost = %v, want %v", got, want)
	}
}

func TestComputeCost_NilPricingReturnsZero(t *testing.T) {
	usage := &MessageUsage{InputTokens: 1000, OutputTokens: 1000}
	if got := ComputeCost(usage, nil); got != 0 {
		t.Errorf("ComputeCost(usage, nil) = %v, want 0", got)
	}
}

func TestComputeCost_NilUsageReturnsZero(t *testing.T) {
	pricing := &ModelPricing{InputPer1M: 3.00, OutputPer1M: 15.00}
	if got := ComputeCost(nil, pricing); got != 0 {
		t.Errorf("ComputeCost(nil, pricing) = %v, want 0", got)
	}
}

func TestKnownModels_DeterministicOrder(t *testing.T) {
	a := KnownModels()
	b := KnownModels()
	if len(a) != len(b) {
		t.Fatalf("KnownModels length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("KnownModels[%d]: %q vs %q (non-deterministic)", i, a[i], b[i])
		}
	}
}
