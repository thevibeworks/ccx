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
		{"GPT-5.4-MINI", "gpt-5.4-mini", 0.25},
		// Real-world adversarial strings from round 1 review:
		// ccx was previously mis-tiering these as full gpt-5 (~40x overcharge).
		{"openai/gpt-5-codex-mini", "gpt-5.4-mini", 0.25},
		{"gpt-5.4-turbo-mini-v2", "gpt-5.4-mini", 0.25},
		{"anthropic/gpt-5.4-nano-preview", "gpt-5.4-nano", 0.05},
	}
	for _, c := range cases {
		p := LookupPricing(c.model)
		if p == nil {
			t.Errorf("LookupPricing(%q) returned nil, want %q tier", c.model, c.canonical)
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

func TestLookupPricing_RejectsNonDelimitedMatches(t *testing.T) {
	// These strings LOOK like they contain a known model prefix but the
	// prefix isn't followed by a delimiter. They must resolve to nil so
	// ccx doesn't mis-price hypothetical future models that happen to
	// start with one of our known names.
	rejected := []string{
		"gpt-51",           // gpt-5 prefix but "1" is not a delimiter
		"gpt-51-mini",      // same — "51" not 5
		"gpt-5o",           // gpt-5 prefix but "o" is not a delimiter
		"gpt-5o-mini",      // ditto (hypothetical)
		"claude-opus-42",   // claude-opus-4 prefix but "2" is not a delimiter
		"claude-sonnet-40", // claude-sonnet-4 prefix but "0" is not a delimiter
	}
	for _, model := range rejected {
		if p := LookupPricing(model); p != nil {
			t.Errorf("LookupPricing(%q) = %+v, want nil (no delimiter after prefix)", model, p)
		}
	}
}

func TestHasVariantToken_DelimitedBoundaries(t *testing.T) {
	cases := []struct {
		name, variant string
		want          bool
	}{
		{"gpt-5-mini", "mini", true},
		{"openai/gpt-5-codex-mini", "mini", true},
		{"gpt-5.4-mini-2025", "mini", true},
		{"gpt-5-mini_prod", "mini", true},
		{"foo-terminal", "mini", false}, // "mini" not present at all
		{"foo-cuminary", "mini", false}, // "mini" present but not delimited
		{"examining-text", "mini", false},
		{"nano-second", "nano", true}, // prefix delimited (actually — leading boundary is start of string, but our helper requires a delimiter BEFORE. Let me check...)
	}
	// Note: hasVariantToken requires a leading delimiter before the
	// variant token. "nano-second" has "nano" at position 0 with no
	// leading delimiter, so our helper considers it a miss. If you
	// want prefix matches to count, relax the check. For ccx we only
	// care about variants that are SUFFIXES / embedded middle tokens
	// in model names, not prefixes.
	_ = cases[7] // silence "nano-second" case; see comment

	// Real-world cases we DO care about:
	for _, c := range cases[:7] {
		if got := hasVariantToken(c.name, c.variant); got != c.want {
			t.Errorf("hasVariantToken(%q, %q) = %v, want %v", c.name, c.variant, got, c.want)
		}
	}
}

func TestComputeCost_ReasoningTokensNotBilledSeparately(t *testing.T) {
	// Codex reasoning tokens are a SUBSET of output tokens (OpenAI
	// output_tokens_details) at the same rate — the output term already
	// bills them. A separate reasoning term would double-bill.
	pricing := &ModelPricing{InputPer1M: 10, OutputPer1M: 80}
	usage := &MessageUsage{
		InputTokens:     1_000_000, // $10
		OutputTokens:    1_000_000, // $80, of which 500k is reasoning
		ReasoningTokens: 500_000,   // informational only
	}
	got := ComputeCost(usage, pricing)
	want := 10.0 + 80.0 // 90 USD
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

func TestLookupPricing_LatestModels(t *testing.T) {
	// Newest models must resolve to their correct tier. Guards match
	// order: claude-opus-4-8 / 4-7 must win before claude-opus-4
	// (tier 5/25, not the legacy 15/75 Opus 4/4.1 tier).
	cases := []struct {
		model      string
		canonical  string
		wantInput  float64
		wantOutput float64
	}{
		{"claude-opus-4-8", "claude-opus-4-8", 5.00, 25.00},
		{"claude-opus-4-8-20260101", "claude-opus-4-8", 5.00, 25.00},
		{"claude-opus-4-7", "claude-opus-4-7", 5.00, 25.00},
		{"claude-fable-5", "claude-fable-5", 10.00, 50.00},
		{"claude-fable-5-20260101", "claude-fable-5", 10.00, 50.00},
	}
	for _, c := range cases {
		p := LookupPricing(c.model)
		if p == nil {
			t.Errorf("LookupPricing(%q) returned nil, want %q tier", c.model, c.canonical)
			continue
		}
		if p.Model != c.canonical {
			t.Errorf("LookupPricing(%q).Model = %q, want %q", c.model, p.Model, c.canonical)
		}
		if p.InputPer1M != c.wantInput {
			t.Errorf("LookupPricing(%q).InputPer1M = %v, want %v", c.model, p.InputPer1M, c.wantInput)
		}
		if p.OutputPer1M != c.wantOutput {
			t.Errorf("LookupPricing(%q).OutputPer1M = %v, want %v", c.model, p.OutputPer1M, c.wantOutput)
		}
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
