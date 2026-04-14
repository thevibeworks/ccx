package parser

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeSession writes the given JSONL content to a temp file and returns
// its path. The caller should invoke ParseSession on the returned path.
func writeSession(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoTurnSession = `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"hello world"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"hi","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":2000,"cache_creation_input_tokens":500}}}
{"type":"user","timestamp":"2026-04-01T10:01:00Z","uuid":"u2","parentUuid":"a1","message":{"content":"second question"}}
{"type":"assistant","timestamp":"2026-04-01T10:01:02Z","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"answer","model":"claude-sonnet-4-5","usage":{"input_tokens":200,"output_tokens":80,"cache_read_input_tokens":2600,"cache_creation_input_tokens":0}}}
`

func TestParseSession_PopulatesPerMessageUsage(t *testing.T) {
	path := writeSession(t, twoTurnSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	msgs := FlattenSessionMessages(session)
	var assistantWithUsage int
	for _, m := range msgs {
		if m.Type == "assistant" {
			if m.Usage == nil {
				t.Errorf("assistant message %q has nil Usage", m.UUID)
				continue
			}
			assistantWithUsage++
		}
	}
	if assistantWithUsage != 2 {
		t.Errorf("expected 2 assistants with usage, got %d", assistantWithUsage)
	}
}

func TestParseSession_MessageCostUsesPricingTable(t *testing.T) {
	path := writeSession(t, twoTurnSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	pricing := LookupPricing("claude-sonnet-4-5")
	if pricing == nil {
		t.Fatal("pricing missing for claude-sonnet-4-5")
	}

	// First assistant: 100 input, 50 output, 2000 cache_read, 500 cache_create
	wantA1 := 100*pricing.InputPer1M/1_000_000.0 +
		50*pricing.OutputPer1M/1_000_000.0 +
		2000*pricing.CacheReadPer1M/1_000_000.0 +
		500*pricing.CacheWritePer1M/1_000_000.0

	msgs := FlattenSessionMessages(session)
	var found bool
	for _, m := range msgs {
		if m.UUID == "a1" && m.Usage != nil {
			if math.Abs(m.Usage.CostUSD-wantA1) > 1e-12 {
				t.Errorf("a1 CostUSD = %v, want %v", m.Usage.CostUSD, wantA1)
			}
			found = true
		}
	}
	if !found {
		t.Error("message a1 not found in flattened session")
	}
}

func TestParseSession_SessionCostEqualsSumOfMessageCosts(t *testing.T) {
	path := writeSession(t, twoTurnSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	var sumPerMessage float64
	for _, m := range FlattenSessionMessages(session) {
		if m.Usage != nil {
			sumPerMessage += m.Usage.CostUSD
		}
	}

	if math.Abs(session.Stats.CostUSD-sumPerMessage) > 1e-12 {
		t.Errorf("session.Stats.CostUSD = %v, sum per-message = %v (must be equal)", session.Stats.CostUSD, sumPerMessage)
	}
	if session.Stats.CostUSD <= 0 {
		t.Errorf("session CostUSD = %v, want > 0 for a priced session", session.Stats.CostUSD)
	}
}

func TestComputeTurnStats_TwoTurns(t *testing.T) {
	path := writeSession(t, twoTurnSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	turns := ComputeTurnStats(FlattenSessionMessages(session))
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}

	if turns[0].AnchorID != "u1" {
		t.Errorf("turn[0].AnchorID = %q, want u1", turns[0].AnchorID)
	}
	if turns[1].AnchorID != "u2" {
		t.Errorf("turn[1].AnchorID = %q, want u2", turns[1].AnchorID)
	}
	if turns[0].Snippet != "hello world" {
		t.Errorf("turn[0].Snippet = %q, want %q", turns[0].Snippet, "hello world")
	}
	if turns[0].Model != "claude-sonnet-4-5" {
		t.Errorf("turn[0].Model = %q, want claude-sonnet-4-5", turns[0].Model)
	}

	// Turn 1 should have the usage from a1
	if turns[0].InputTokens != 100 {
		t.Errorf("turn[0].InputTokens = %d, want 100", turns[0].InputTokens)
	}
	if turns[0].OutputTokens != 50 {
		t.Errorf("turn[0].OutputTokens = %d, want 50", turns[0].OutputTokens)
	}
	if turns[0].CacheReadTokens != 2000 {
		t.Errorf("turn[0].CacheReadTokens = %d, want 2000", turns[0].CacheReadTokens)
	}
}

func TestComputeTurnStats_SumMatchesSessionCost(t *testing.T) {
	path := writeSession(t, twoTurnSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	turns := ComputeTurnStats(FlattenSessionMessages(session))
	var sumTurnCost float64
	for _, tr := range turns {
		sumTurnCost += tr.CostUSD
	}

	if math.Abs(sumTurnCost-session.Stats.CostUSD) > 1e-12 {
		t.Errorf("sum of turn costs = %v, session cost = %v (must be equal)", sumTurnCost, session.Stats.CostUSD)
	}
}

const sidechainSession = `{"type":"user","timestamp":"2026-04-01T10:00:00Z","uuid":"u1","message":{"content":"dispatch an agent"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:01Z","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"launching","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"user","timestamp":"2026-04-01T10:00:02Z","uuid":"s1","parentUuid":"a1","isSidechain":true,"message":{"content":"subagent task"}}
{"type":"assistant","timestamp":"2026-04-01T10:00:03Z","uuid":"s2","parentUuid":"s1","isSidechain":true,"message":{"role":"assistant","content":"subagent done","model":"claude-sonnet-4-5","usage":{"input_tokens":200,"output_tokens":100,"cache_read_input_tokens":500,"cache_creation_input_tokens":0}}}
`

func TestComputeTurnStats_SidechainInclusion(t *testing.T) {
	path := writeSession(t, sidechainSession)
	session, err := ParseSession(path)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	turns := ComputeTurnStats(FlattenSessionMessages(session))
	if len(turns) == 0 {
		t.Fatal("expected at least 1 turn")
	}

	var sumCost float64
	var sumIn int
	var foundSidechainAnchor bool
	for _, tr := range turns {
		sumCost += tr.CostUSD
		sumIn += tr.InputTokens
		if tr.HasSidechain {
			foundSidechainAnchor = true
		}
	}

	if math.Abs(sumCost-session.Stats.CostUSD) > 1e-12 {
		t.Errorf("sum turn cost = %v, session cost = %v", sumCost, session.Stats.CostUSD)
	}
	// Sum of inputs across all turns: a1(100) + s2(200) = 300
	if sumIn != 300 {
		t.Errorf("sum input = %d, want 300", sumIn)
	}
	if !foundSidechainAnchor {
		t.Error("expected at least one turn to have HasSidechain=true")
	}
}
