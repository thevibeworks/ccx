package parser

import "strings"

// ModelPricing holds the per-million-token USD price for each token category.
// Rates mirror Anthropic's public list pricing at the time of pinning. Cache
// reads are discounted (typically 10% of input); cache writes are priced
// above input (typically 125% for 5-min write TTL).
type ModelPricing struct {
	Model           string  // Canonical model name (exact match key)
	InputPer1M      float64 // USD per 1,000,000 input tokens
	OutputPer1M     float64 // USD per 1,000,000 output tokens
	CacheReadPer1M  float64 // USD per 1,000,000 cache-read input tokens
	CacheWritePer1M float64 // USD per 1,000,000 cache-creation (write) input tokens
}

// pricingTable is the embedded, versioned Claude pricing catalog. Always
// prefer exact model-name match; no fuzzy matching — that's how ccusage
// shipped 5x overcharge bugs ([ryoppippi/ccusage#934]). Add new rows when
// Anthropic ships new models; don't synthesize rates for unknown models.
//
// Rates in USD/1M tokens. Sources tracked in docs/devlog/pricing-pinning.org
// (add-only history — if a rate changes, append a dated row, don't edit).
var pricingTable = map[string]ModelPricing{
	// Claude 4.x Opus
	"claude-opus-4-5": {Model: "claude-opus-4-5", InputPer1M: 15.00, OutputPer1M: 75.00, CacheReadPer1M: 1.50, CacheWritePer1M: 18.75},
	"claude-opus-4-6": {Model: "claude-opus-4-6", InputPer1M: 15.00, OutputPer1M: 75.00, CacheReadPer1M: 1.50, CacheWritePer1M: 18.75},
	// Claude 4.x Sonnet
	"claude-sonnet-4-5": {Model: "claude-sonnet-4-5", InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
	"claude-sonnet-4-6": {Model: "claude-sonnet-4-6", InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
	// Claude 4.5 Haiku
	"claude-haiku-4-5": {Model: "claude-haiku-4-5", InputPer1M: 1.00, OutputPer1M: 5.00, CacheReadPer1M: 0.10, CacheWritePer1M: 1.25},
	// Claude 3.5 Sonnet / Haiku (legacy sessions)
	"claude-3-5-sonnet-latest": {Model: "claude-3-5-sonnet-latest", InputPer1M: 3.00, OutputPer1M: 15.00, CacheReadPer1M: 0.30, CacheWritePer1M: 3.75},
	"claude-3-5-haiku-latest":  {Model: "claude-3-5-haiku-latest", InputPer1M: 0.80, OutputPer1M: 4.00, CacheReadPer1M: 0.08, CacheWritePer1M: 1.00},
}

// LookupPricing returns pricing for a model name. Exact match on the
// canonical name; also matches dated model IDs by stripping a trailing
// "-YYYYMMDD" suffix (so "claude-sonnet-4-5-20250929" maps to "claude-sonnet-4-5").
// Returns nil when the model is unknown — callers should treat nil as
// "cost unavailable" rather than zero.
func LookupPricing(model string) *ModelPricing {
	if model == "" {
		return nil
	}

	if p, ok := pricingTable[model]; ok {
		return &p
	}

	// Strip trailing date suffix: "claude-sonnet-4-5-20250929" -> "claude-sonnet-4-5"
	if canon := stripDateSuffix(model); canon != model {
		if p, ok := pricingTable[canon]; ok {
			return &p
		}
	}

	return nil
}

// stripDateSuffix returns name with a trailing "-YYYYMMDD" removed. If there
// is no such suffix, name is returned unchanged.
func stripDateSuffix(name string) string {
	if len(name) < 9 {
		return name
	}
	if name[len(name)-9] != '-' {
		return name
	}
	tail := name[len(name)-8:]
	for i := 0; i < 8; i++ {
		if tail[i] < '0' || tail[i] > '9' {
			return name
		}
	}
	return name[:len(name)-9]
}

// ComputeCost returns USD for the given token usage at the given pricing.
// Returns 0 when pricing is nil. Cached reads and cached writes are billed
// at their discounted/surcharged rates respectively.
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
	return cost
}

// KnownModels returns the sorted list of model names with pinned pricing.
// Useful for diagnostics and doctor command.
func KnownModels() []string {
	names := make([]string, 0, len(pricingTable))
	for name := range pricingTable {
		names = append(names, name)
	}
	// Sort for deterministic output (Go map iteration is randomized)
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.Compare(names[j-1], names[j]) > 0; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}
