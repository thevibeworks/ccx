package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This test drops a fake claude-code source tree into a temp dir with
// deliberate drift from ccx's embedded pricing and verifies the tool
// detects it. Without this test, the verify tool could silently return
// "no drift" even if its parser was broken.

const fakeModelCostTS = `// @see https://platform.claude.com/docs/pricing
export const COST_TIER_3_15 = {
  inputTokens: 3,
  outputTokens: 15,
  promptCacheWriteTokens: 3.75,
  promptCacheReadTokens: 0.3,
  webSearchRequests: 0.01,
} as const satisfies ModelCosts

export const COST_TIER_WRONG = {
  inputTokens: 99,
  outputTokens: 999,
  promptCacheWriteTokens: 123.45,
  promptCacheReadTokens: 9.9,
  webSearchRequests: 0.01,
} as const satisfies ModelCosts

export const MODEL_COSTS: Record<ModelShortName, ModelCosts> = {
  [firstPartyNameToCanonical(CLAUDE_SONNET_4_5_CONFIG.firstParty)]:
    COST_TIER_3_15,
  [firstPartyNameToCanonical(CLAUDE_OPUS_4_6_CONFIG.firstParty)]:
    COST_TIER_WRONG,
}
`

const fakeConfigsTS = `export const CLAUDE_SONNET_4_5_CONFIG = {
  firstParty: 'claude-sonnet-4-5-20250929',
  streamingEnabled: true,
}

export const CLAUDE_OPUS_4_6_CONFIG = {
  firstParty: 'claude-opus-4-6',
  streamingEnabled: true,
}
`

func writeFakeSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	utilsDir := filepath.Join(dir, "src", "utils")
	modelDir := filepath.Join(utilsDir, "model")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(utilsDir, "modelCost.ts"), []byte(fakeModelCostTS), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "configs.ts"), []byte(fakeConfigsTS), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestParseModelCostTiers_ReadsValues(t *testing.T) {
	dir := writeFakeSource(t)
	tiers, err := parseModelCostTiers(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatalf("parseModelCostTiers: %v", err)
	}
	if len(tiers) != 2 {
		t.Fatalf("expected 2 tiers, got %d", len(tiers))
	}

	tier315, ok := tiers["COST_TIER_3_15"]
	if !ok {
		t.Fatal("missing COST_TIER_3_15")
	}
	if tier315.InputTokens != 3 {
		t.Errorf("COST_TIER_3_15.InputTokens = %v, want 3", tier315.InputTokens)
	}
	if tier315.OutputTokens != 15 {
		t.Errorf("COST_TIER_3_15.OutputTokens = %v, want 15", tier315.OutputTokens)
	}
	if tier315.PromptCacheR != 0.3 {
		t.Errorf("COST_TIER_3_15.PromptCacheR = %v, want 0.3", tier315.PromptCacheR)
	}
}

func TestParseModelCostsMap_ExtractsBindings(t *testing.T) {
	dir := writeFakeSource(t)
	m, err := parseModelCostsMap(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatalf("parseModelCostsMap: %v", err)
	}
	if m["CLAUDE_SONNET_4_5_CONFIG"] != "COST_TIER_3_15" {
		t.Errorf("SONNET_4_5 mapping = %q, want COST_TIER_3_15", m["CLAUDE_SONNET_4_5_CONFIG"])
	}
	if m["CLAUDE_OPUS_4_6_CONFIG"] != "COST_TIER_WRONG" {
		t.Errorf("OPUS_4_6 mapping = %q, want COST_TIER_WRONG", m["CLAUDE_OPUS_4_6_CONFIG"])
	}
}

func TestParseFirstPartyNames_ExtractsFirstParty(t *testing.T) {
	dir := writeFakeSource(t)
	configs, err := parseFirstPartyNames(filepath.Join(dir, "src", "utils", "model", "configs.ts"))
	if err != nil {
		t.Fatalf("parseFirstPartyNames: %v", err)
	}
	if configs["CLAUDE_SONNET_4_5_CONFIG"] != "claude-sonnet-4-5-20250929" {
		t.Errorf("SONNET_4_5 firstParty = %q", configs["CLAUDE_SONNET_4_5_CONFIG"])
	}
	if configs["CLAUDE_OPUS_4_6_CONFIG"] != "claude-opus-4-6" {
		t.Errorf("OPUS_4_6 firstParty = %q", configs["CLAUDE_OPUS_4_6_CONFIG"])
	}
}

func TestCompareAgainstCCX_DetectsDrift(t *testing.T) {
	dir := writeFakeSource(t)
	tiers, err := parseModelCostTiers(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseModelCostsMap(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatal(err)
	}
	configs, err := parseFirstPartyNames(filepath.Join(dir, "src", "utils", "model", "configs.ts"))
	if err != nil {
		t.Fatal(err)
	}

	drift := compareAgainstCCX(tiers, m, configs, false)

	// Expect drift on claude-opus-4-6 since the fake source says it
	// costs $99/$999 but ccx says $5/$25. The SONNET_4_5 mapping should
	// match (ccx has the correct $3/$15) and be absent from the drift list.
	foundOpus46 := false
	for _, d := range drift {
		if strings.Contains(d, "claude-opus-4-6") {
			foundOpus46 = true
			if !strings.Contains(d, "CC=99") {
				t.Errorf("drift for opus-4-6 should cite the fake source's inflated value, got: %s", d)
			}
		}
		if strings.Contains(d, "claude-sonnet-4-5") {
			t.Errorf("unexpected drift reported for sonnet-4-5 (ccx matches the fake source): %s", d)
		}
	}
	if !foundOpus46 {
		t.Errorf("expected drift on claude-opus-4-6, got no report. Full drift: %v", drift)
	}
}

func TestCompareAgainstCCX_IgnoresCodexOnlyRowsInStaleCheck(t *testing.T) {
	dir := writeFakeSource(t)
	tiers, err := parseModelCostTiers(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseModelCostsMap(filepath.Join(dir, "src", "utils", "modelCost.ts"))
	if err != nil {
		t.Fatal(err)
	}
	configs, err := parseFirstPartyNames(filepath.Join(dir, "src", "utils", "model", "configs.ts"))
	if err != nil {
		t.Fatal(err)
	}

	drift := compareAgainstCCX(tiers, m, configs, false)
	for _, d := range drift {
		if strings.Contains(d, "gpt-5") {
			t.Fatalf("Codex-only pricing rows should not be flagged as Claude drift, got: %s", d)
		}
	}
}

func TestCanonicalize_OrderMatters(t *testing.T) {
	// The canonicalize function must match more specific patterns
	// before less specific ones. Verifies the tool stays in sync with
	// ccx's LookupPricing match order.
	cases := []struct {
		in   string
		want string
	}{
		{"claude-opus-4-6", "claude-opus-4-6"},
		{"claude-opus-4-5-20251101", "claude-opus-4-5"},
		{"claude-opus-4-1-20250805", "claude-opus-4-1"},
		{"claude-opus-4-20250514", "claude-opus-4"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"},
		{"us.anthropic.claude-sonnet-4-5-20250929-v1:0", "claude-sonnet-4-5"},
		{"CLAUDE-HAIKU-4-5-20251001", "claude-haiku-4-5"},
	}
	for _, c := range cases {
		if got := canonicalize(c.in); got != c.want {
			t.Errorf("canonicalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
