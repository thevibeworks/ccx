package trace

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const outlineHeadlineRunes = 160

// BuildOutline reduces a full trace to its always-fits skeleton:
// every turn and step headline plus rollup numbers. This is what
// consumers read FIRST; they drill into specific turns afterwards.
func BuildOutline(result *TraceResult) *Outline {
	if result == nil {
		return &Outline{Kind: OutlineKind}
	}
	outline := &Outline{
		Kind:        OutlineKind,
		GeneratedAt: result.GeneratedAt,
		Session:     result.Session,
		Stats:       result.Stats,
		Warnings:    result.Warnings,
	}
	for _, turn := range result.Turns {
		ot := OutlineTurn{
			Index:             turn.Index,
			Start:             turn.Start,
			UserText:          headline(turn.UserText),
			IsCommand:         turn.IsCommand,
			CommandName:       turn.CommandName,
			Superseded:        turn.Superseded,
			SupersededByTurn:  turn.SupersededByTurn,
			Edits:             len(turn.FilesEdited),
			Errors:            turn.Errors,
			InputTokens:       turn.InputTokens,
			OutputTokens:      turn.OutputTokens,
			CacheReadTokens:   turn.CacheReadTokens,
			CacheCreateTokens: turn.CacheCreateTokens,
			ReasoningTokens:   turn.ReasoningTokens,
			CostUSD:           turn.CostUSD,
			AgentsCostUSD:     turn.AgentsCostUSD,
			LinkedCommits:     shortSHAs(turn.LinkedCommits),
		}
		if d := turn.End.Sub(turn.Start).Seconds(); d > 0 {
			ot.DurationSecs = d
		}
		ot.ActiveSecs = turn.ActiveSecs
		for _, n := range turn.ToolCounts {
			ot.Tools += n
		}
		for _, step := range turn.Steps {
			os := OutlineStep{
				Index:    step.Index,
				Headline: headline(step.Narration),
				Edits:    len(step.FilesEdited),
				Errors:   step.Errors,
				Agents:   len(step.Sidechains),
			}
			for _, n := range step.ToolCounts {
				os.Tools += n
			}
			ot.Agents += len(step.Sidechains)
			ot.Steps = append(ot.Steps, os)
		}
		outline.Turns = append(outline.Turns, ot)
	}
	return outline
}

// headline reduces evidence text to its first meaningful line,
// capped for outline display.
func headline(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.IndexByte(text, '\n'); idx > 0 {
		text = text[:idx]
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > outlineHeadlineRunes {
		return strings.TrimSpace(string(runes[:outlineHeadlineRunes])) + "..."
	}
	return string(runes)
}

func shortSHAs(shas []string) []string {
	if len(shas) == 0 {
		return nil
	}
	out := make([]string, len(shas))
	for i, sha := range shas {
		if len(sha) > 7 {
			out[i] = sha[:7]
		} else {
			out[i] = sha
		}
	}
	return out
}

// RenderOutlineText renders the outline for a terminal: one line per
// turn, indented step headlines, plain ASCII.
func RenderOutlineText(outline *Outline) string {
	if outline == nil {
		return ""
	}
	var b strings.Builder

	s := outline.Session
	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	project := s.ProjectName
	if project == "" && s.CWD != "" {
		project = filepath.Base(s.CWD)
	}
	fmt.Fprintf(&b, "session %s | %s | %s | %s\n", id, s.Provider, project, s.Model)
	turnsSeg := fmt.Sprintf("%d turns", outline.Stats.TurnCount)
	if outline.Stats.SupersededTurns > 0 {
		turnsSeg = fmt.Sprintf("%d turns (+%d superseded)", outline.Stats.TurnCount, outline.Stats.SupersededTurns)
	}
	// Wall-span misleads on long sessions; print active time next to it
	// and say which timezone the rendered times are in (JSON carries
	// them in UTC — silent localization breaks cross-referencing).
	fmt.Fprintf(&b, "%s -> %s (times %s) | active %s | %s | %d steps | %d files edited | %d tool errors%s\n",
		formatOutlineTime(s.Start), formatOutlineTime(s.End), outlineZone(s.Start),
		formatActive(outline.Stats.ActiveSecs),
		turnsSeg, outline.Stats.StepCount,
		outline.Stats.FilesEdited, outline.Stats.ToolErrors,
		headerCost(outline.Stats.TotalCostUSD, outline.Stats.AgentsCostUSD))
	if tok := tokenSplit(outline.Stats.InputTokens, outline.Stats.OutputTokens,
		outline.Stats.CacheReadTokens, outline.Stats.CacheCreateTokens,
		outline.Stats.ReasoningTokens); tok != "" {
		fmt.Fprintf(&b, "tokens: %s\n", tok)
	}
	for _, w := range outline.Warnings {
		fmt.Fprintf(&b, "warning: %s: %s\n", w.Kind, w.Message)
	}
	b.WriteString("\n")

	for _, turn := range outline.Turns {
		fmt.Fprintf(&b, "#%d %s%s%s\n",
			turn.Index,
			formatOutlineTime(turn.Start),
			turnBadges(turn),
			commitBadge(turn.LinkedCommits))
		if turn.UserText != "" {
			fmt.Fprintf(&b, "  u: %s\n", turn.UserText)
		}
		for _, step := range turn.Steps {
			if step.Headline == "" && step.Tools == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %2d. %s%s\n", step.Index, step.Headline, stepBadges(step))
		}
	}
	return b.String()
}

func turnBadges(turn OutlineTurn) string {
	var parts []string
	if turn.Superseded {
		if turn.SupersededByTurn > 0 {
			parts = append(parts, fmt.Sprintf("superseded by #%d", turn.SupersededByTurn))
		} else {
			parts = append(parts, "superseded")
		}
	}
	// Active time, not wall gap: an autonomous turn bleeding into an
	// overnight gap would otherwise badge as an 18-hour turn.
	if turn.ActiveSecs >= 60 {
		parts = append(parts, formatActive(turn.ActiveSecs))
	}
	if turn.Tools > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", turn.Tools))
	}
	if turn.Edits > 0 {
		parts = append(parts, fmt.Sprintf("%d edits", turn.Edits))
	}
	if turn.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", turn.Errors))
	}
	if turn.Agents > 0 {
		parts = append(parts, fmt.Sprintf("%d agents", turn.Agents))
	}
	if tok := tokenSplit(turn.InputTokens, turn.OutputTokens,
		turn.CacheReadTokens, turn.CacheCreateTokens, turn.ReasoningTokens); tok != "" {
		parts = append(parts, tok)
	}
	if turn.CostUSD >= 0.005 {
		parts = append(parts, fmt.Sprintf("$%.2f", turn.CostUSD))
	}
	if turn.AgentsCostUSD >= 0.005 {
		parts = append(parts, fmt.Sprintf("$%.2f agents", turn.AgentsCostUSD))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// tokenSplit renders the token components that explain a cost figure.
// Cache tokens get their own segment: cache writes bill at 1.25x input
// rate and routinely dwarf everything else, so a turn that "looks
// cheap" (2 tools, one short reply) can only be audited when the cache
// traffic is visible.
func tokenSplit(input, output, cacheRead, cacheCreate, reasoning int) string {
	var parts []string
	if input > 0 || output > 0 {
		parts = append(parts, fmt.Sprintf("%s in/%s out", formatTokens(input), formatTokens(output)))
	}
	if reasoning > 0 {
		parts = append(parts, fmt.Sprintf("%s reason", formatTokens(reasoning)))
	}
	switch {
	case cacheCreate > 0:
		parts = append(parts, fmt.Sprintf("cache %s r/%s w", formatTokens(cacheRead), formatTokens(cacheCreate)))
	case cacheRead > 0:
		parts = append(parts, fmt.Sprintf("cache %s r", formatTokens(cacheRead)))
	}
	return strings.Join(parts, ", ")
}

// formatTokens renders a token count compactly: 512, 1.2k, 45k, 3.2m.
func formatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	case n < 10_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1e6)
	default:
		return fmt.Sprintf("%dm", n/1_000_000)
	}
}

func stepBadges(step OutlineStep) string {
	var parts []string
	if step.Tools > 0 {
		parts = append(parts, fmt.Sprintf("%dt", step.Tools))
	}
	if step.Edits > 0 {
		parts = append(parts, fmt.Sprintf("%de", step.Edits))
	}
	if step.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%dx", step.Errors))
	}
	if step.Agents > 0 {
		parts = append(parts, fmt.Sprintf("%da", step.Agents))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  [" + strings.Join(parts, " ") + "]"
}

func commitBadge(shas []string) string {
	if len(shas) == 0 {
		return ""
	}
	return " -> commit " + strings.Join(shas, ",")
}

func formatOutlineTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// outlineZone names the timezone rendered times are in, as a UTC
// offset (evaluated at the session's date, so DST resolves correctly).
func outlineZone(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	_, offset := t.Local().Zone()
	if offset == 0 {
		return "UTC"
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	h := offset / 3600
	m := (offset % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, m)
}

// formatActive renders a duration in seconds compactly: 45s, 12m, 4h07m.
func formatActive(secs float64) string {
	d := time.Duration(secs) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%02dm", h, m)
	}
}

// headerCost renders the header's cost segment, or nothing when no
// cost was computed: $0.00 would read as "measured zero" when the
// truth is "unpriced" (Grok sessions by contract, unknown models on
// any provider) — remove-wrong over display-wrong. The total is
// all-in; the agents share is broken out when nonzero so the sum of
// turn costs remains reconcilable.
func headerCost(cost, agentsCost float64) string {
	if cost <= 0 {
		return ""
	}
	if agentsCost >= 0.005 {
		return fmt.Sprintf(" | $%.2f ($%.2f agents)", cost, agentsCost)
	}
	return fmt.Sprintf(" | $%.2f", cost)
}
