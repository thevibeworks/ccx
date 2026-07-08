// Command ccx-verify-pricing compares ccx's embedded pricing table
// against Claude Code's source of truth at
// reference/claude-code-2188/src/utils/modelCost.ts. Exits non-zero on
// drift so CI / Makefile can gate on it.
//
// Usage:
//
//	go run ./cmd/ccx-verify-pricing
//	go run ./cmd/ccx-verify-pricing --claude-source /path/to/claude-code
//
// The tool parses the TypeScript source with regex (no bun/node
// dependency) — it only looks at the cost tier constants and the
// canonical-name → tier mapping. That's the surface ccx needs to
// match; deeper TS parsing would add brittleness without value.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

const defaultClaudeSource = "reference/claude-code-2188"

func main() {
	source := flag.String("claude-source", defaultClaudeSource, "path to claude-code source checkout (must contain src/utils/modelCost.ts)")
	verbose := flag.Bool("v", false, "print all models, not just drift")
	flag.Parse()

	modelCostPath := filepath.Join(*source, "src", "utils", "modelCost.ts")
	configsPath := filepath.Join(*source, "src", "utils", "model", "configs.ts")

	tiers, err := parseModelCostTiers(modelCostPath)
	if err != nil {
		fatal("parsing %s: %v", modelCostPath, err)
	}

	modelToTier, err := parseModelCostsMap(modelCostPath)
	if err != nil {
		fatal("parsing MODEL_COSTS map in %s: %v", modelCostPath, err)
	}

	configs, err := parseFirstPartyNames(configsPath)
	if err != nil {
		fatal("parsing %s: %v", configsPath, err)
	}

	drift := compareAgainstCCX(tiers, modelToTier, configs, *verbose)

	if len(drift) == 0 {
		fmt.Println("ccx pricing table matches Claude Code source — no drift.")
		return
	}

	fmt.Fprintf(os.Stderr, "\n%d pricing discrepancies found:\n\n", len(drift))
	for _, d := range drift {
		fmt.Fprintln(os.Stderr, "  "+d)
	}
	fmt.Fprintf(os.Stderr, "\nFix: update internal/parser/pricing.go to match the Claude Code source above.\n")
	os.Exit(1)
}

// costTier is a parsed tier definition from modelCost.ts. Names mirror
// the TypeScript constant names so logs and diffs are obvious.
type costTier struct {
	Name         string
	InputTokens  float64
	OutputTokens float64
	PromptCacheW float64 // promptCacheWriteTokens
	PromptCacheR float64 // promptCacheReadTokens
	WebSearch    float64 // webSearchRequests
}

// tierBlockRe matches an exported cost tier constant like:
//
//	export const COST_TIER_3_15 = {
//	  inputTokens: 3,
//	  outputTokens: 15,
//	  ...
//	} as const satisfies ModelCosts
var tierBlockRe = regexp.MustCompile(`(?s)export const (COST_[A-Z_0-9]+) = \{([^}]+)\}`)

// parseModelCostTiers extracts every exported cost tier constant from
// modelCost.ts. Values are parsed as floats so they can be compared
// directly against ccx's ModelPricing struct.
func parseModelCostTiers(path string) (map[string]costTier, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := make(map[string]costTier)
	matches := tierBlockRe.FindAllStringSubmatch(string(content), -1)
	for _, m := range matches {
		name := m[1]
		body := m[2]
		tier := costTier{Name: name}

		for _, field := range []struct {
			key string
			ptr *float64
		}{
			{"inputTokens", &tier.InputTokens},
			{"outputTokens", &tier.OutputTokens},
			{"promptCacheWriteTokens", &tier.PromptCacheW},
			{"promptCacheReadTokens", &tier.PromptCacheR},
			{"webSearchRequests", &tier.WebSearch},
		} {
			v, ok := extractField(body, field.key)
			if !ok {
				return nil, fmt.Errorf("tier %s missing field %s", name, field.key)
			}
			*field.ptr = v
		}
		out[name] = tier
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no cost tier constants found (regex failed to match)")
	}
	return out, nil
}

// extractField pulls a numeric field value from a tier body, e.g.
// "inputTokens: 3," -> 3.0
func extractField(body, key string) (float64, bool) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\s*:\s*([0-9.]+)`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseModelCostsMap extracts the MODEL_COSTS mapping, returning each
// canonical model name paired with its tier constant name. Uses a
// line-oriented parser because the TypeScript uses computed property
// keys and spans multiple lines per entry.
//
// Expected shape:
//
//	export const MODEL_COSTS: Record<ModelShortName, ModelCosts> = {
//	  [firstPartyNameToCanonical(CLAUDE_SONNET_4_5_CONFIG.firstParty)]:
//	    COST_TIER_3_15,
//	  ...
//	}
func parseModelCostsMap(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	configRe := regexp.MustCompile(`firstPartyNameToCanonical\(([A-Z0-9_]+)\.firstParty\)`)
	tierRe := regexp.MustCompile(`(COST_[A-Z_0-9]+)`)

	out := make(map[string]string)
	inBlock := false
	var currentConfig string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "export const MODEL_COSTS") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "}") && currentConfig == "" {
			// End of block (closing brace on its own line, no pending key)
			break
		}

		// Look for a key assignment line: [firstPartyNameToCanonical(XXX.firstParty)]:
		if m := configRe.FindStringSubmatch(line); m != nil {
			currentConfig = m[1]
		}
		// Look for a tier constant (may be same line or next)
		if currentConfig != "" {
			if m := tierRe.FindStringSubmatch(line); m != nil && m[1] != "" && strings.HasPrefix(m[1], "COST_") {
				// Only accept if it's a tier ref, not the MODEL_COSTS declaration line
				if !strings.Contains(line, "MODEL_COSTS") {
					out[currentConfig] = m[1]
					currentConfig = ""
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("MODEL_COSTS map parsed but empty")
	}
	return out, nil
}

// parseFirstPartyNames reads configs.ts and extracts each model
// config's firstParty string. Returns a map of CONSTANT_NAME →
// firstParty (e.g., "CLAUDE_SONNET_4_5_CONFIG" → "claude-sonnet-4-5-20250929").
func parseFirstPartyNames(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Match `export const <NAME>_CONFIG = { firstParty: '<string>', ...`
	configRe := regexp.MustCompile(`(?s)export const ([A-Z0-9_]+_CONFIG) = \{[^}]*?firstParty: '([^']+)'`)
	matches := configRe.FindAllStringSubmatch(string(content), -1)

	out := make(map[string]string, len(matches))
	for _, m := range matches {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no model configs found")
	}
	return out, nil
}

// canonicalize mirrors Claude Code's firstPartyNameToCanonical in
// model.ts. Keep in sync manually — if Claude Code changes its
// canonicalization rules, so must we.
func canonicalize(name string) string {
	n := strings.ToLower(name)
	// More specific versions first
	patterns := []string{
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-opus-4-7",
		"claude-opus-4-6",
		"claude-opus-4-5",
		"claude-opus-4-1",
		"claude-opus-4",
		"claude-sonnet-4-6",
		"claude-sonnet-4-5",
		"claude-sonnet-4",
		"claude-haiku-4-5",
		"claude-3-7-sonnet",
		"claude-3-5-sonnet",
		"claude-3-5-haiku",
	}
	for _, p := range patterns {
		if strings.Contains(n, p) {
			return p
		}
	}
	return n
}

func compareAgainstCCX(
	tiers map[string]costTier,
	modelToTier map[string]string, // CLAUDE_SONNET_4_5_CONFIG → COST_TIER_3_15
	configs map[string]string, // CLAUDE_SONNET_4_5_CONFIG → claude-sonnet-4-5-20250929
	verbose bool,
) []string {
	var drift []string
	ccxTable := parser.PricingTableCopy()

	// Walk every model that Claude Code prices
	configNames := make([]string, 0, len(modelToTier))
	for k := range modelToTier {
		configNames = append(configNames, k)
	}
	sort.Strings(configNames)

	for _, configName := range configNames {
		tierName := modelToTier[configName]
		tier, ok := tiers[tierName]
		if !ok {
			drift = append(drift, fmt.Sprintf("%s: unknown tier %s", configName, tierName))
			continue
		}
		firstParty, ok := configs[configName]
		if !ok {
			drift = append(drift, fmt.Sprintf("%s: no firstParty found in configs.ts", configName))
			continue
		}
		canonical := canonicalize(firstParty)

		ccxPricing, ccxHas := ccxTable[canonical]
		if !ccxHas {
			drift = append(drift, fmt.Sprintf(
				"%s: ccx MISSING pricing for %s (CC says tier %s: input=%v output=%v)",
				configName, canonical, tierName, tier.InputTokens, tier.OutputTokens,
			))
			continue
		}

		mismatches := diffPricing(tier, ccxPricing)
		if len(mismatches) > 0 {
			drift = append(drift, fmt.Sprintf(
				"%s (%s) [tier %s]: %s",
				configName, canonical, tierName, strings.Join(mismatches, "; "),
			))
		} else if verbose {
			fmt.Printf("  OK  %s (%s) [tier %s]\n", configName, canonical, tierName)
		}
	}

	// Check for ccx models that Claude Code doesn't recognise — stale rows
	ccxByCanonical := make(map[string]bool)
	for _, configName := range configNames {
		firstParty := configs[configName]
		canonical := canonicalize(firstParty)
		ccxByCanonical[canonical] = true
	}
	ccxKeys := make([]string, 0, len(ccxTable))
	for k := range ccxTable {
		ccxKeys = append(ccxKeys, k)
	}
	sort.Strings(ccxKeys)
	for _, k := range ccxKeys {
		if !strings.HasPrefix(k, "claude-") {
			continue
		}
		if !ccxByCanonical[k] {
			drift = append(drift, fmt.Sprintf("ccx has %q but Claude Code source does not reference it", k))
		}
	}

	return drift
}

// diffPricing returns per-field mismatches between a Claude Code tier
// and a ccx ModelPricing struct. Empty slice means exact agreement.
func diffPricing(cc costTier, ccx parser.ModelPricing) []string {
	var diffs []string
	eq := func(a, b float64) bool {
		return math.Abs(a-b) < 1e-9
	}
	if !eq(cc.InputTokens, ccx.InputPer1M) {
		diffs = append(diffs, fmt.Sprintf("input CC=%v ccx=%v", cc.InputTokens, ccx.InputPer1M))
	}
	if !eq(cc.OutputTokens, ccx.OutputPer1M) {
		diffs = append(diffs, fmt.Sprintf("output CC=%v ccx=%v", cc.OutputTokens, ccx.OutputPer1M))
	}
	if !eq(cc.PromptCacheR, ccx.CacheReadPer1M) {
		diffs = append(diffs, fmt.Sprintf("cache-read CC=%v ccx=%v", cc.PromptCacheR, ccx.CacheReadPer1M))
	}
	if !eq(cc.PromptCacheW, ccx.CacheWritePer1M) {
		diffs = append(diffs, fmt.Sprintf("cache-write CC=%v ccx=%v", cc.PromptCacheW, ccx.CacheWritePer1M))
	}
	return diffs
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ccx-verify-pricing: "+format+"\n", args...)
	os.Exit(2)
}
