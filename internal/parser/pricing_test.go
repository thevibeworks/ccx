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

func TestLookupPricing_NoFuzzyMatch(t *testing.T) {
	// Guard against ccusage's #934 regression: "claude-sonnet-4-5-mini"
	// must NOT match "claude-sonnet-4-5" via fuzzy substring matching.
	if p := LookupPricing("claude-sonnet-4-5-mini"); p != nil {
		t.Errorf("expected nil (no fuzzy match) for -mini variant, got %+v", p)
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
