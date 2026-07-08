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
			Index:         turn.Index,
			Start:         turn.Start,
			UserText:      headline(turn.UserText),
			IsCommand:     turn.IsCommand,
			CommandName:   turn.CommandName,
			Edits:         len(turn.FilesEdited),
			Errors:        turn.Errors,
			CostUSD:       turn.CostUSD,
			LinkedCommits: shortSHAs(turn.LinkedCommits),
		}
		if d := turn.End.Sub(turn.Start).Seconds(); d > 0 {
			ot.DurationSecs = d
		}
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
	fmt.Fprintf(&b, "%s -> %s | %d turns | %d steps | %d files edited | %d tool errors | $%.2f\n",
		formatOutlineTime(s.Start), formatOutlineTime(s.End),
		outline.Stats.TurnCount, outline.Stats.StepCount,
		outline.Stats.FilesEdited, outline.Stats.ToolErrors,
		outline.Stats.TotalCostUSD)
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
	if turn.DurationSecs >= 60 {
		parts = append(parts, fmt.Sprintf("%dm", int(turn.DurationSecs/60)))
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
	if turn.CostUSD >= 0.005 {
		parts = append(parts, fmt.Sprintf("$%.2f", turn.CostUSD))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
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
