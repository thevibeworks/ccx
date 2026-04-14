package web

import (
	"strings"
	"testing"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

func TestComputeTimelineTicks_EmptySession(t *testing.T) {
	if ticks := computeTimelineTicks(nil); ticks != nil {
		t.Errorf("expected nil for nil session, got %+v", ticks)
	}
	if ticks := computeTimelineTicks(&parser.Session{}); ticks != nil {
		t.Errorf("expected nil for empty session, got %+v", ticks)
	}
}

func TestComputeTimelineTicks_SingleAnchorCentered(t *testing.T) {
	// A single anchor gets position 50% (no division by zero).
	ts := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	session := &parser.Session{
		StartTime: ts,
		EndTime:   ts,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: ts},
		},
	}
	ticks := computeTimelineTicks(session)
	if len(ticks) != 1 {
		t.Fatalf("expected 1 tick, got %d", len(ticks))
	}
	if ticks[0].PercentTop != 50 {
		t.Errorf("single-anchor PercentTop = %v, want 50", ticks[0].PercentTop)
	}
	if ticks[0].Index != 1 {
		t.Errorf("single-anchor Index = %d, want 1", ticks[0].Index)
	}
}

func TestComputeTimelineTicks_PositionByIndexNotTime(t *testing.T) {
	// Three anchors with highly uneven timestamps: t, t+1s, t+1h.
	// Time-based positioning would push ticks[1] to ~0.028% (clustered
	// near the start). Index-based should put it at exactly 50%.
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	session := &parser.Session{
		StartTime: start,
		EndTime:   start.Add(1 * time.Hour),
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start},
			{UUID: "u2", Kind: parser.KindUserPrompt, Timestamp: start.Add(1 * time.Second)},
			{UUID: "u3", Kind: parser.KindUserPrompt, Timestamp: start.Add(1 * time.Hour)},
		},
	}
	ticks := computeTimelineTicks(session)
	if len(ticks) != 3 {
		t.Fatalf("expected 3 ticks, got %d", len(ticks))
	}
	if ticks[0].PercentTop != 0 {
		t.Errorf("ticks[0].PercentTop = %v, want 0", ticks[0].PercentTop)
	}
	if ticks[1].PercentTop < 49.9 || ticks[1].PercentTop > 50.1 {
		t.Errorf("ticks[1].PercentTop = %v, want 50 (index-based, not time-based)", ticks[1].PercentTop)
	}
	if ticks[2].PercentTop != 100 {
		t.Errorf("ticks[2].PercentTop = %v, want 100", ticks[2].PercentTop)
	}
	// Index ordinals should be 1, 2, 3
	for i, want := range []int{1, 2, 3} {
		if ticks[i].Index != want {
			t.Errorf("ticks[%d].Index = %d, want %d", i, ticks[i].Index, want)
		}
	}
}

func TestComputeTimelineTicks_LongIdleGapDoesntDistort(t *testing.T) {
	// Simulates a resumed session: 5 turns close together, 3-hour idle,
	// 5 more turns close together. Index-based positioning should give
	// all 10 turns evenly-spaced slots (0, 11.11, 22.22, ..., 100).
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	resume := start.Add(3 * time.Hour)
	msgs := []*parser.Message{
		{UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start},
		{UUID: "u2", Kind: parser.KindUserPrompt, Timestamp: start.Add(10 * time.Second)},
		{UUID: "u3", Kind: parser.KindUserPrompt, Timestamp: start.Add(20 * time.Second)},
		{UUID: "u4", Kind: parser.KindUserPrompt, Timestamp: start.Add(30 * time.Second)},
		{UUID: "u5", Kind: parser.KindUserPrompt, Timestamp: start.Add(40 * time.Second)},
		{UUID: "u6", Kind: parser.KindUserPrompt, Timestamp: resume},
		{UUID: "u7", Kind: parser.KindUserPrompt, Timestamp: resume.Add(10 * time.Second)},
		{UUID: "u8", Kind: parser.KindUserPrompt, Timestamp: resume.Add(20 * time.Second)},
		{UUID: "u9", Kind: parser.KindUserPrompt, Timestamp: resume.Add(30 * time.Second)},
		{UUID: "u10", Kind: parser.KindUserPrompt, Timestamp: resume.Add(40 * time.Second)},
	}
	session := &parser.Session{
		StartTime:    start,
		EndTime:      resume.Add(40 * time.Second),
		RootMessages: msgs,
	}
	ticks := computeTimelineTicks(session)
	if len(ticks) != 10 {
		t.Fatalf("expected 10 ticks, got %d", len(ticks))
	}

	// Positions should be evenly spaced: 0, 11.11, 22.22, ..., 100
	for i := 0; i < 10; i++ {
		want := float64(i) / 9.0 * 100.0
		if diff := ticks[i].PercentTop - want; diff < -0.1 || diff > 0.1 {
			t.Errorf("ticks[%d].PercentTop = %v, want %v (evenly spaced)", i, ticks[i].PercentTop, want)
		}
	}

	// The 3-hour gap between u5 and u6 should NOT compress u1-u5 into a
	// tiny sliver — u5 should be at ~44.4% (turn 5 of 10 zero-indexed).
	if ticks[4].PercentTop < 44.3 || ticks[4].PercentTop > 44.5 {
		t.Errorf("u5 PercentTop = %v, want ~44.44 (would be ~0.37 under time-based)", ticks[4].PercentTop)
	}
}

func TestComputeTimelineTicks_EmitsSubagentAndSkillTicks(t *testing.T) {
	// A single user turn whose assistant dispatches a Task (sub-agent)
	// and calls a Skill tool should produce 3 ticks: the user turn
	// (major) and two sub-event ticks (minor).
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	assistant := &parser.Message{
		UUID:      "a1",
		Kind:      parser.KindAssistant,
		Timestamp: start.Add(time.Second),
		Content: []parser.ContentBlock{
			{
				Type:     "tool_use",
				ToolName: "Task",
				ToolID:   "t1",
				ToolInput: map[string]any{
					"subagent_type": "Plan",
					"description":   "Design auth flow",
				},
			},
			{
				Type:     "tool_use",
				ToolName: "Skill",
				ToolID:   "t2",
				ToolInput: map[string]any{
					"skill": "commit",
				},
			},
		},
	}
	user := &parser.Message{
		UUID:      "u1",
		Kind:      parser.KindUserPrompt,
		Timestamp: start,
		Children:  []*parser.Message{assistant},
	}

	session := &parser.Session{
		StartTime:    start,
		EndTime:      start.Add(2 * time.Second),
		RootMessages: []*parser.Message{user},
	}

	ticks := computeTimelineTicks(session)
	if len(ticks) != 3 {
		t.Fatalf("expected 3 ticks (user + task + skill), got %d", len(ticks))
	}

	kinds := make([]tickKind, len(ticks))
	snippets := make([]string, len(ticks))
	for i, tk := range ticks {
		kinds[i] = tk.Kind
		snippets[i] = tk.Snippet
	}

	wantKinds := []tickKind{tickUser, tickAgent, tickSkill}
	for i, want := range wantKinds {
		if kinds[i] != want {
			t.Errorf("ticks[%d].Kind = %v, want %v", i, kinds[i], want)
		}
	}

	// Snippet for agent tick should contain the subagent_type label
	if !strings.Contains(snippets[1], "Plan") {
		t.Errorf("agent tick snippet = %q, want to contain 'Plan'", snippets[1])
	}
	// Snippet for skill tick should contain the skill name
	if !strings.Contains(snippets[2], "commit") {
		t.Errorf("skill tick snippet = %q, want to contain 'commit'", snippets[2])
	}

	// All three ticks should carry the parent assistant's UUID for
	// jump navigation (except user prompt which keeps its own)
	if ticks[0].UUID != "u1" {
		t.Errorf("user tick UUID = %q, want u1", ticks[0].UUID)
	}
	if ticks[1].UUID != "a1" {
		t.Errorf("agent tick UUID = %q, want a1 (parent assistant)", ticks[1].UUID)
	}
	if ticks[2].UUID != "a1" {
		t.Errorf("skill tick UUID = %q, want a1 (parent assistant)", ticks[2].UUID)
	}
}

func TestComputeTimelineTicks_FiltersNonAnchorKinds(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)

	session := &parser.Session{
		StartTime: start,
		EndTime:   end,
		RootMessages: []*parser.Message{
			{UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start},
			{UUID: "a1", Kind: parser.KindAssistant, Timestamp: start.Add(time.Second)},
			{UUID: "t1", Kind: parser.KindToolResult, Timestamp: start.Add(2 * time.Second)},
			{UUID: "c1", Kind: parser.KindCompactSummary, Timestamp: start.Add(5 * time.Minute)},
			{UUID: "cmd1", Kind: parser.KindCommand, IsCommand: true, CommandName: "init", Timestamp: start.Add(6 * time.Minute)},
			{UUID: "u2", Kind: parser.KindUserPrompt, Timestamp: end},
		},
	}
	ticks := computeTimelineTicks(session)

	// Expect: u1, c1, cmd1, u2 = 4 ticks
	if len(ticks) != 4 {
		t.Fatalf("expected 4 anchor ticks, got %d", len(ticks))
	}

	gotKinds := make(map[string]tickKind)
	for _, tk := range ticks {
		gotKinds[tk.UUID] = tk.Kind
	}
	if gotKinds["u1"] != tickUser {
		t.Errorf("u1 kind = %v, want tickUser", gotKinds["u1"])
	}
	if gotKinds["c1"] != tickCompact {
		t.Errorf("c1 kind = %v, want tickCompact", gotKinds["c1"])
	}
	if gotKinds["cmd1"] != tickCommand {
		t.Errorf("cmd1 kind = %v, want tickCommand", gotKinds["cmd1"])
	}
}

func TestFormatOffset(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{61 * time.Minute, "1h01m"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
	}
	for _, c := range cases {
		if got := formatOffset(c.in); got != c.want {
			t.Errorf("formatOffset(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnrichTicksWithCost_NormalizesHeat(t *testing.T) {
	// Build a session with two user turns: turn 1 cheap, turn 2 expensive.
	// Heat should be 0 < cheap < 1 and expensive == 1 (highest = 1.0).
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	mid := start.Add(30 * time.Second)
	session := &parser.Session{
		StartTime: start,
		EndTime:   mid.Add(30 * time.Second),
		RootMessages: []*parser.Message{
			{
				UUID:      "u1",
				Kind:      parser.KindUserPrompt,
				Timestamp: start,
				Children: []*parser.Message{
					{
						UUID:      "a1",
						Kind:      parser.KindAssistant,
						Model:     "claude-sonnet-4-5",
						Timestamp: start.Add(time.Second),
						Usage: &parser.MessageUsage{
							InputTokens: 100, OutputTokens: 50, CostUSD: 0.001,
						},
					},
				},
			},
			{
				UUID:      "u2",
				Kind:      parser.KindUserPrompt,
				Timestamp: mid,
				Children: []*parser.Message{
					{
						UUID:      "a2",
						Kind:      parser.KindAssistant,
						Model:     "claude-sonnet-4-5",
						Timestamp: mid.Add(time.Second),
						Usage: &parser.MessageUsage{
							InputTokens: 5000, OutputTokens: 3000, CostUSD: 0.060,
						},
					},
				},
			},
		},
	}

	ticks := enrichTicksWithCost(computeTimelineTicks(session), session)
	if len(ticks) != 2 {
		t.Fatalf("expected 2 ticks, got %d", len(ticks))
	}

	var cheap, expensive *timelineTick
	for i := range ticks {
		switch ticks[i].UUID {
		case "u1":
			cheap = &ticks[i]
		case "u2":
			expensive = &ticks[i]
		}
	}
	if cheap == nil || expensive == nil {
		t.Fatalf("ticks missing: cheap=%v expensive=%v", cheap, expensive)
	}
	if expensive.Heat != 1.0 {
		t.Errorf("expensive.Heat = %v, want 1.0 (highest cost = 100%%)", expensive.Heat)
	}
	if cheap.Heat <= 0 || cheap.Heat >= expensive.Heat {
		t.Errorf("cheap.Heat = %v, want 0 < cheap < expensive (%v)", cheap.Heat, expensive.Heat)
	}
	if cheap.CostUSD != 0.001 {
		t.Errorf("cheap.CostUSD = %v, want 0.001", cheap.CostUSD)
	}
	if expensive.CostUSD != 0.060 {
		t.Errorf("expensive.CostUSD = %v, want 0.060", expensive.CostUSD)
	}
}

func TestChooseGridStep(t *testing.T) {
	cases := []struct {
		tickCount int
		wantCount [2]int // acceptable gridline count range; [0,0] = no gridlines
	}{
		{tickCount: 5, wantCount: [2]int{0, 0}},    // too few, no ruler
		{tickCount: 15, wantCount: [2]int{0, 0}},   // still too few
		{tickCount: 30, wantCount: [2]int{4, 10}},  // step 5 → 5 lines
		{tickCount: 50, wantCount: [2]int{4, 10}},  // step 5 or 10
		{tickCount: 100, wantCount: [2]int{4, 10}}, // step 10 or 20
		{tickCount: 500, wantCount: [2]int{4, 10}},
		{tickCount: 2000, wantCount: [2]int{4, 10}},
	}
	for _, c := range cases {
		step := chooseGridStep(c.tickCount)
		if step == 0 {
			if c.wantCount[1] != 0 {
				t.Errorf("chooseGridStep(%d) = 0, want non-zero for %v-%v lines", c.tickCount, c.wantCount[0], c.wantCount[1])
			}
			continue
		}
		count := (c.tickCount - 1) / step
		if count < c.wantCount[0] || count > c.wantCount[1] {
			t.Errorf("chooseGridStep(%d)=%d yields %d gridlines, want %d-%d", c.tickCount, step, count, c.wantCount[0], c.wantCount[1])
		}
	}
}

func TestComputeTimelineGridlines_IndexBased(t *testing.T) {
	lines := computeTimelineGridlines(100)
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 gridlines for 100 ticks, got %d", len(lines))
	}

	// Lines must be strictly increasing in PercentTop, each within [0, 100]
	for i, line := range lines {
		if line.PercentTop < 0 || line.PercentTop > 100 {
			t.Errorf("lines[%d].PercentTop = %v, want 0-100", i, line.PercentTop)
		}
		if i > 0 && line.PercentTop <= lines[i-1].PercentTop {
			t.Errorf("gridlines not monotonically increasing at %d", i)
		}
		if line.Label == "" {
			t.Errorf("lines[%d].Label is empty", i)
		}
	}
}

func TestComputeTimelineGridlines_TooFewTicksReturnsNil(t *testing.T) {
	if lines := computeTimelineGridlines(3); lines != nil {
		t.Errorf("expected nil gridlines for 3-tick session, got %+v", lines)
	}
	if lines := computeTimelineGridlines(0); lines != nil {
		t.Errorf("expected nil gridlines for 0-tick session, got %+v", lines)
	}
}

func TestEnrichTicksWithCost_CumulativeMatchesSessionTotal(t *testing.T) {
	// Three turns, known per-turn costs. After enrichment, the last
	// tick's CumulativeCostUSD must equal the sum of all per-turn costs.
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	mkAssistant := func(uuid, parent string, offset time.Duration, cost float64, tokens int) *parser.Message {
		return &parser.Message{
			UUID:       uuid,
			ParentUUID: parent,
			Kind:       parser.KindAssistant,
			Model:      "claude-sonnet-4-5",
			Timestamp:  start.Add(offset),
			Usage: &parser.MessageUsage{
				InputTokens: tokens,
				CostUSD:     cost,
			},
		}
	}

	u1 := &parser.Message{UUID: "u1", Kind: parser.KindUserPrompt, Timestamp: start, Children: []*parser.Message{mkAssistant("a1", "u1", 1*time.Second, 0.002, 200)}}
	u2 := &parser.Message{UUID: "u2", Kind: parser.KindUserPrompt, Timestamp: start.Add(10 * time.Second), Children: []*parser.Message{mkAssistant("a2", "u2", 11*time.Second, 0.010, 1000)}}
	u3 := &parser.Message{UUID: "u3", Kind: parser.KindUserPrompt, Timestamp: start.Add(20 * time.Second), Children: []*parser.Message{mkAssistant("a3", "u3", 21*time.Second, 0.004, 400)}}

	session := &parser.Session{
		StartTime:    start,
		EndTime:      start.Add(25 * time.Second),
		RootMessages: []*parser.Message{u1, u2, u3},
	}

	ticks := enrichTicksWithCost(computeTimelineTicks(session), session)
	if len(ticks) != 3 {
		t.Fatalf("expected 3 ticks, got %d", len(ticks))
	}

	wantCum := []float64{0.002, 0.012, 0.016}
	for i, want := range wantCum {
		got := ticks[i].CumulativeCostUSD
		diff := got - want
		if diff < -1e-9 || diff > 1e-9 {
			t.Errorf("ticks[%d].CumulativeCostUSD = %v, want %v", i, got, want)
		}
	}

	// The last tick's cumulative must equal the total session cost.
	sessionTotal := 0.002 + 0.010 + 0.004
	diff := ticks[len(ticks)-1].CumulativeCostUSD - sessionTotal
	if diff < -1e-9 || diff > 1e-9 {
		t.Errorf("last cumulative = %v, want session total %v", ticks[len(ticks)-1].CumulativeCostUSD, sessionTotal)
	}
}

func TestEnrichTicksWithCost_TokenFallbackForUnpriced(t *testing.T) {
	// Session with unknown model → no cost → heat falls back to token count.
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	session := &parser.Session{
		StartTime: start,
		EndTime:   start.Add(60 * time.Second),
		RootMessages: []*parser.Message{
			{
				UUID:      "u1",
				Kind:      parser.KindUserPrompt,
				Timestamp: start,
				Children: []*parser.Message{
					{
						UUID:      "a1",
						Kind:      parser.KindAssistant,
						Model:     "gpt-4", // unknown to ccx pricing
						Timestamp: start.Add(time.Second),
						Usage:     &parser.MessageUsage{InputTokens: 1000, OutputTokens: 500, CostUSD: 0},
					},
				},
			},
			{
				UUID:      "u2",
				Kind:      parser.KindUserPrompt,
				Timestamp: start.Add(30 * time.Second),
				Children: []*parser.Message{
					{
						UUID:      "a2",
						Kind:      parser.KindAssistant,
						Model:     "gpt-4",
						Timestamp: start.Add(31 * time.Second),
						Usage:     &parser.MessageUsage{InputTokens: 500, OutputTokens: 100, CostUSD: 0},
					},
				},
			},
		},
	}

	ticks := enrichTicksWithCost(computeTimelineTicks(session), session)
	if len(ticks) != 2 {
		t.Fatalf("expected 2 ticks, got %d", len(ticks))
	}

	for _, tk := range ticks {
		if tk.CostUSD != 0 {
			t.Errorf("expected zero cost for unpriced model, got %v", tk.CostUSD)
		}
		if tk.TotalTokens == 0 {
			t.Errorf("expected non-zero token count for tick %s", tk.UUID)
		}
	}

	// The higher-token turn (u1 with 1500 tokens) should have heat 1.0
	// and the other (u2 with 600 tokens) should be strictly lower.
	var u1, u2 *timelineTick
	for i := range ticks {
		switch ticks[i].UUID {
		case "u1":
			u1 = &ticks[i]
		case "u2":
			u2 = &ticks[i]
		}
	}
	if u1 == nil || u2 == nil {
		t.Fatal("ticks missing")
	}
	if u1.Heat != 1.0 {
		t.Errorf("u1.Heat = %v, want 1.0 (highest token count = 100%%)", u1.Heat)
	}
	if u2.Heat >= u1.Heat {
		t.Errorf("u2.Heat = %v, want < u1.Heat (%v)", u2.Heat, u1.Heat)
	}
}
