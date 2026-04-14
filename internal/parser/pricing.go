package parser

import "strings"

// ModelPricing holds the per-million-token USD price for each token category.
// Rates mirror Anthropic's public list pricing and are pinned from Claude
// Code's own cost tiers (`src/utils/modelCost.ts` in the claude-code source).
//
// Cache reads are typically 10% of input; cache writes are typically
// 125% of input for the 5-minute-TTL variant. Different models follow
// different tiers — see pricingTable below.
//
// Pricing is verified against Claude Code source via `cmd/ccx-verify-pricing`.
// Run `go run ./cmd/ccx-verify-pricing` (or `make verify-pricing`) after
// bumping either this file or updating the reference/claude-code-2188
// checkout to catch drift.
type ModelPricing struct {
	Model           string  // Canonical short model name
	InputPer1M      float64 // USD per 1,000,000 input tokens
	OutputPer1M     float64 // USD per 1,000,000 output tokens
	CacheReadPer1M  float64 // USD per 1,000,000 cache-read input tokens
	CacheWritePer1M float64 // USD per 1,000,000 cache-creation input tokens
}

// Named Claude cost tiers — mirror the constants in Claude Code's
// modelCost.ts so drift detection can compare tier-by-tier.
//
// Source: reference/claude-code-2188/src/utils/modelCost.ts
var (
	costTier_3_15 = ModelPricing{
		InputPer1M: 3.00, OutputPer1M: 15.00,
		CacheReadPer1M: 0.30, CacheWritePer1M: 3.75,
	}
	costTier_15_75 = ModelPricing{
		InputPer1M: 15.00, OutputPer1M: 75.00,
		CacheReadPer1M: 1.50, CacheWritePer1M: 18.75,
	}
	costTier_5_25 = ModelPricing{
		InputPer1M: 5.00, OutputPer1M: 25.00,
		CacheReadPer1M: 0.50, CacheWritePer1M: 6.25,
	}
	costTier_30_150 = ModelPricing{
		InputPer1M: 30.00, OutputPer1M: 150.00,
		CacheReadPer1M: 3.00, CacheWritePer1M: 37.50,
	}
	costHaiku_35 = ModelPricing{
		InputPer1M: 0.80, OutputPer1M: 4.00,
		CacheReadPer1M: 0.08, CacheWritePer1M: 1.00,
	}
	costHaiku_45 = ModelPricing{
		InputPer1M: 1.00, OutputPer1M: 5.00,
		CacheReadPer1M: 0.10, CacheWritePer1M: 1.25,
	}

	// Codex / GPT-5.x tiers.
	//
	// UNVERIFIED: these rates are pinned from OpenAI's published
	// gpt-5 pricing circa late 2025 (list pricing, no enterprise
	// discount). Cache reads are billed at ~50% of input per OpenAI's
	// cached-prompt pricing — OpenAI does NOT split cache into 5m/1h
	// creation tiers the way Anthropic does, so CacheWritePer1M stays 0.
	// Reasoning output tokens are priced at the same rate as regular
	// output tokens.
	//
	// If OpenAI rebases gpt-5.4 rates these constants MUST be updated.
	// No automated verification tool yet (there's no equivalent of
	// Claude Code's modelCost.ts in the Codex reference checkout).
	costGPT54_10_80 = ModelPricing{
		InputPer1M: 10.00, OutputPer1M: 80.00,
		CacheReadPer1M: 5.00, CacheWritePer1M: 0,
	}
	costGPT54Mini_025_2 = ModelPricing{
		InputPer1M: 0.25, OutputPer1M: 2.00,
		CacheReadPer1M: 0.125, CacheWritePer1M: 0,
	}
	costGPT54Nano_005_04 = ModelPricing{
		InputPer1M: 0.05, OutputPer1M: 0.40,
		CacheReadPer1M: 0.025, CacheWritePer1M: 0,
	}
)

// pricingTable maps canonical model names to their pricing tier. Keys
// match Claude Code's `MODEL_COSTS` map. The canonical names are the
// ShortNames produced by `firstPartyNameToCanonical` in Claude Code's
// model.ts — matching is case-insensitive substring, with more specific
// names checked first.
//
// IMPORTANT: when a new Claude model ships, add its entry HERE AND
// update the match order in LookupPricing. Then run the verify tool.
var pricingTable = map[string]ModelPricing{
	// Haiku
	"claude-3-5-haiku": costHaiku_35,
	"claude-haiku-4-5": costHaiku_45,

	// Sonnet (all Sonnet 3.x and 4.x on tier 3/15)
	"claude-3-5-sonnet": costTier_3_15,
	"claude-3-7-sonnet": costTier_3_15,
	"claude-sonnet-4":   costTier_3_15,
	"claude-sonnet-4-5": costTier_3_15,
	"claude-sonnet-4-6": costTier_3_15,

	// Opus 4 / 4.1 on tier 15/75; Opus 4.5 / 4.6 dropped to tier 5/25
	"claude-opus-4":   costTier_15_75,
	"claude-opus-4-1": costTier_15_75,
	"claude-opus-4-5": costTier_5_25,
	"claude-opus-4-6": costTier_5_25, // Default (non-fast-mode)

	// Codex / GPT-5.x models — see tier constants above for caveats.
	"gpt-5":        costGPT54_10_80,
	"gpt-5-mini":   costGPT54Mini_025_2,
	"gpt-5-nano":   costGPT54Nano_005_04,
	"gpt-5.4":      costGPT54_10_80,
	"gpt-5.4-mini": costGPT54Mini_025_2,
	"gpt-5.4-nano": costGPT54Nano_005_04,
}

// LookupPricing returns pricing for a model name. Matching mirrors
// Claude Code's `firstPartyNameToCanonical`: case-insensitive substring
// with more-specific patterns checked first (so "claude-opus-4-6" takes
// precedence over "claude-opus-4"). Handles Bedrock ARNs, dated model
// IDs, and bare canonical names uniformly.
//
// Returns nil when the model is unknown — callers should treat nil as
// "cost unavailable" rather than mis-attributing.
//
// KNOWN LIMITATION: Opus 4.6 in fast mode is billed at tier 30/150, not
// tier 5/25. ccx currently returns the default tier for Opus 4.6
// regardless of speed. Fast-mode support would require piping the
// message's `usage.speed` field through cost computation — tracked as
// a follow-up.
func LookupPricing(model string) *ModelPricing {
	if model == "" {
		return nil
	}
	name := strings.ToLower(model)

	// Order matters: more specific versions first (4-6 before 4-5 before 4-1 before 4)
	switch {
	case strings.Contains(name, "claude-opus-4-6"):
		p := pricingTable["claude-opus-4-6"]
		p.Model = "claude-opus-4-6"
		return &p
	case strings.Contains(name, "claude-opus-4-5"):
		p := pricingTable["claude-opus-4-5"]
		p.Model = "claude-opus-4-5"
		return &p
	case strings.Contains(name, "claude-opus-4-1"):
		p := pricingTable["claude-opus-4-1"]
		p.Model = "claude-opus-4-1"
		return &p
	case strings.Contains(name, "claude-opus-4"):
		p := pricingTable["claude-opus-4"]
		p.Model = "claude-opus-4"
		return &p
	case strings.Contains(name, "claude-sonnet-4-6"):
		p := pricingTable["claude-sonnet-4-6"]
		p.Model = "claude-sonnet-4-6"
		return &p
	case strings.Contains(name, "claude-sonnet-4-5"):
		p := pricingTable["claude-sonnet-4-5"]
		p.Model = "claude-sonnet-4-5"
		return &p
	case strings.Contains(name, "claude-sonnet-4"):
		p := pricingTable["claude-sonnet-4"]
		p.Model = "claude-sonnet-4"
		return &p
	case strings.Contains(name, "claude-haiku-4-5"):
		p := pricingTable["claude-haiku-4-5"]
		p.Model = "claude-haiku-4-5"
		return &p
	case strings.Contains(name, "claude-3-7-sonnet"):
		p := pricingTable["claude-3-7-sonnet"]
		p.Model = "claude-3-7-sonnet"
		return &p
	case strings.Contains(name, "claude-3-5-sonnet"):
		p := pricingTable["claude-3-5-sonnet"]
		p.Model = "claude-3-5-sonnet"
		return &p
	case strings.Contains(name, "claude-3-5-haiku"):
		p := pricingTable["claude-3-5-haiku"]
		p.Model = "claude-3-5-haiku"
		return &p
	// Codex / GPT-5.x family. Order matters: nano / mini checked before
	// the plain "gpt-5" substring match so "gpt-5-mini" doesn't
	// degrade to "gpt-5" rates.
	case strings.Contains(name, "gpt-5.4-nano"), strings.Contains(name, "gpt-5-nano"):
		p := pricingTable["gpt-5.4-nano"]
		p.Model = "gpt-5.4-nano"
		return &p
	case strings.Contains(name, "gpt-5.4-mini"), strings.Contains(name, "gpt-5-mini"):
		p := pricingTable["gpt-5.4-mini"]
		p.Model = "gpt-5.4-mini"
		return &p
	case strings.Contains(name, "gpt-5.4"), strings.Contains(name, "gpt-5"):
		p := pricingTable["gpt-5.4"]
		p.Model = "gpt-5.4"
		return &p
	}
	return nil
}

// ComputeCost returns USD for the given token usage at the given pricing.
// Returns 0 when pricing is nil.
//
// Cached reads and cached writes are billed at their discounted/
// surcharged rates respectively. Reasoning tokens (Codex-only) are
// billed at the output rate — OpenAI treats extended-thinking output
// as regular output for pricing purposes.
func ComputeCost(u *MessageUsage, p *ModelPricing) float64 {
	if u == nil || p == nil {
		return 0
	}
	const perMillion = 1_000_000.0
	cost := 0.0
	cost += float64(u.InputTokens) * p.InputPer1M / perMillion
	cost += float64(u.OutputTokens) * p.OutputPer1M / perMillion
	cost += float64(u.CacheReadTokens) * p.CacheReadPer1M / perMillion
	cost += float64(u.CacheCreateTokens) * p.CacheWritePer1M / perMillion
	cost += float64(u.ReasoningTokens) * p.OutputPer1M / perMillion
	return cost
}

// KnownModels returns the sorted list of canonical model names with
// pinned pricing. Useful for diagnostics and the doctor command.
func KnownModels() []string {
	names := make([]string, 0, len(pricingTable))
	for name := range pricingTable {
		names = append(names, name)
	}
	// Sort for deterministic output
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.Compare(names[j-1], names[j]) > 0; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}

// PricingTableCopy returns a defensive copy of the pricing table. Used
// by the ccx-verify-pricing tool to compare against Claude Code source.
func PricingTableCopy() map[string]ModelPricing {
	out := make(map[string]ModelPricing, len(pricingTable))
	for k, v := range pricingTable {
		v.Model = k
		out[k] = v
	}
	return out
}
