package web

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// timelineTick represents one clickable marker on the time-machine rail.
//
// Position is by anchor INDEX, not wall-clock time. A session with an
// idle gap (user walked away for 3 hours and resumed) no longer
// compresses the actual work into a tiny sliver of the rail — every
// turn gets an even slot. The original Timestamp and Offset are still
// kept for the tooltip so the user can read "when did this happen"
// without losing the even layout.
type timelineTick struct {
	UUID       string        // Message UUID, used as #msg-<uuid> anchor
	Timestamp  time.Time     // Wall-clock timestamp of the anchor message
	PercentTop float64       // Vertical position on the rail (0-100), index-based
	Index      int           // 1-based ordinal of this tick within the session
	Kind       tickKind      // user prompt, command, compact boundary
	Snippet    string        // One-line preview shown in hover tooltip
	Offset     time.Duration // Elapsed time from session start (for tooltip)

	// Enrichment fields populated from the matching TurnStats (if any).
	// The rail uses these to paint visual weight: bigger/brighter dots
	// where the turn cost more. Zero for compact-boundary ticks (they're
	// system markers, not billable work).
	CostUSD           float64 // Sum of per-message cost for this tick's turn
	TotalTokens       int     // Sum of all token categories for the turn
	Heat              float64 // 0-1 normalized weight (highest turn = 1.0)
	CumulativeCostUSD float64 // Running total of session cost up to and incl. this tick
}

type tickKind int

const (
	tickUser    tickKind = iota // Normal user prompt — major rail notch
	tickCommand                 // Slash command — distinct notch, command color
	tickCompact                 // Context compaction boundary — widest notch
	tickAgent                   // Task tool dispatch (sub-agent) — minor notch
	tickSkill                   // Skill tool invocation — minor notch
)

func (k tickKind) class() string {
	switch k {
	case tickCompact:
		return "tick-compact"
	case tickCommand:
		return "tick-command"
	case tickAgent:
		return "tick-agent"
	case tickSkill:
		return "tick-skill"
	default:
		return "tick-user"
	}
}

func (k tickKind) attr() string {
	switch k {
	case tickCompact:
		return "compact"
	case tickCommand:
		return "command"
	case tickAgent:
		return "agent"
	case tickSkill:
		return "skill"
	default:
		return "user"
	}
}

// timelineGridline is a ruler-tick on the rail showing a round ordinal
// ("turn 10", "turn 20") at a uniform interval. Gridlines give the eye
// a scale so the rail's density is legible. Non-interactive.
type timelineGridline struct {
	PercentTop float64
	Label      string // "10" / "20" / "30" — tick ordinal
	IsMajor    bool   // Every 5th gridline is bolder
}

// chooseGridStep picks a round step for index-based gridlines, aiming
// for 4-10 lines across the rail. Steps follow a 1-2-5 progression
// (5, 10, 20, 25, 50, 100, 200, 500...) so labels stay readable.
// Sessions under 20 turns don't get gridlines — with so few ticks the
// rail is short enough that the user can scan everything directly.
func chooseGridStep(tickCount int) int {
	if tickCount < 20 {
		return 0
	}
	candidates := []int{5, 10, 20, 25, 50, 100, 200, 250, 500, 1000}
	for _, c := range candidates {
		n := tickCount / c
		if n >= 4 && n <= 10 {
			return c
		}
	}
	// Fallback: aim for ~8 lines
	step := tickCount / 8
	if step < 1 {
		step = 1
	}
	return step
}

// computeTimelineGridlines returns evenly-spaced ordinal gridlines for
// a given tick count. Returns nil when the session is too small to
// need a ruler.
func computeTimelineGridlines(tickCount int) []timelineGridline {
	step := chooseGridStep(tickCount)
	if step <= 0 || tickCount <= 1 {
		return nil
	}

	var lines []timelineGridline
	for i := step; i < tickCount; i += step {
		percent := float64(i) / float64(tickCount-1) * 100.0
		if percent < 0 || percent > 100 {
			continue
		}
		// Major ruler every 5 steps (every 50/100/500 turns depending on step)
		isMajor := (i/step)%5 == 0
		lines = append(lines, timelineGridline{
			PercentTop: percent,
			Label:      fmt.Sprintf("%d", i),
			IsMajor:    isMajor,
		})
	}
	return lines
}

// computeTimelineTicks walks the session tree, collects anchor events
// in wire order, and assigns each one an evenly-spaced slot on the rail.
// Anchors include:
//   - user prompts, slash commands, compact boundaries (major ticks)
//   - Task tool dispatches (sub-agent invocations — minor ticks)
//   - Skill tool invocations (minor ticks)
//
// Returns nil when there are no anchor events.
//
// Position is INDEX-BASED: if there are N anchors, the i-th anchor
// (0-indexed) lands at i/(N-1) * 100 percent. This means idle time
// between turns doesn't distort the layout; a 3-hour pause between
// turn 9 and turn 10 still leaves turn 10 exactly one slot below
// turn 9 on the rail.
//
// Sub-event ticks (Task/Skill) carry the enclosing assistant's UUID so
// clicking them jumps to that assistant message. They don't show per-
// tick cost because cost is already attributed to the enclosing user
// turn — double-counting would be confusing.
func computeTimelineTicks(session *parser.Session) []timelineTick {
	if session == nil {
		return nil
	}

	type anchor struct {
		msg     *parser.Message
		kind    tickKind
		snippet string // overridden when the anchor is a sub-event (Task/Skill)
	}
	var anchors []anchor

	for _, msg := range flattenMessages(session.RootMessages) {
		if msg == nil {
			continue
		}
		switch msg.Kind {
		case parser.KindUserPrompt:
			anchors = append(anchors, anchor{msg: msg, kind: tickUser})
		case parser.KindCommand:
			anchors = append(anchors, anchor{msg: msg, kind: tickCommand})
		case parser.KindCompactSummary:
			anchors = append(anchors, anchor{msg: msg, kind: tickCompact})
		case parser.KindAssistant:
			// Scan assistant's tool_use blocks for Task (sub-agent) and Skill
			// invocations, emit a minor tick for each. Position inherits the
			// assistant's wire order; snippet carries the agent/skill name.
			for _, block := range msg.Content {
				if block.Type != "tool_use" {
					continue
				}
				switch block.ToolName {
				case "Task":
					anchors = append(anchors, anchor{
						msg:     msg,
						kind:    tickAgent,
						snippet: subagentSnippet(block),
					})
				case "Skill":
					anchors = append(anchors, anchor{
						msg:     msg,
						kind:    tickSkill,
						snippet: skillSnippet(block),
					})
				}
			}
		}
	}
	if len(anchors) == 0 {
		return nil
	}

	ticks := make([]timelineTick, 0, len(anchors))
	total := len(anchors)
	for i, a := range anchors {
		var percent float64
		if total == 1 {
			percent = 50.0
		} else {
			percent = float64(i) / float64(total-1) * 100.0
		}

		var offset time.Duration
		if !a.msg.Timestamp.IsZero() && !session.StartTime.IsZero() {
			offset = a.msg.Timestamp.Sub(session.StartTime)
			if offset < 0 {
				offset = 0
			}
		}

		snippet := a.snippet
		if snippet == "" {
			snippet = tickSnippet(a.msg)
		}

		ticks = append(ticks, timelineTick{
			UUID:       a.msg.UUID,
			Timestamp:  a.msg.Timestamp,
			PercentTop: percent,
			Index:      i + 1,
			Kind:       a.kind,
			Snippet:    snippet,
			Offset:     offset,
		})
	}

	return ticks
}

// subagentSnippet extracts a readable description for a Task tool_use
// block: "[subagent-type] description" when both are present, falling
// back to whatever's available. Used as the rail tooltip's snippet for
// sub-agent ticks.
func subagentSnippet(block parser.ContentBlock) string {
	input, _ := block.ToolInput.(map[string]any)
	if input == nil {
		return "Task dispatch"
	}
	var agentType, description string
	if s, ok := input["subagent_type"].(string); ok {
		agentType = s
	}
	if s, ok := input["description"].(string); ok {
		description = s
	}
	switch {
	case agentType != "" && description != "":
		return "[" + agentType + "] " + description
	case agentType != "":
		return "Task: " + agentType
	case description != "":
		return "Task: " + description
	}
	return "Task dispatch"
}

// skillSnippet extracts a readable description for a Skill tool_use block.
func skillSnippet(block parser.ContentBlock) string {
	input, _ := block.ToolInput.(map[string]any)
	if input == nil {
		return "Skill"
	}
	if name, ok := input["skill"].(string); ok && name != "" {
		if args, ok := input["args"].(string); ok && args != "" {
			return "/" + name + " " + args
		}
		return "/" + name
	}
	return "Skill"
}

// tickSnippet returns a one-line preview for a tick's hover tooltip.
// Commands show "/name args", user prompts show the first line of text.
func tickSnippet(msg *parser.Message) string {
	if msg == nil {
		return ""
	}
	if msg.IsCommand && msg.CommandName != "" {
		if msg.CommandArgs != "" {
			return "/" + msg.CommandName + " " + msg.CommandArgs
		}
		return "/" + msg.CommandName
	}
	if msg.Kind == parser.KindCompactSummary {
		return "Context compaction"
	}
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			text := strings.TrimSpace(block.Text)
			if idx := strings.Index(text, "\n"); idx > 0 {
				text = text[:idx]
			}
			if len(text) > 80 {
				text = text[:77] + "..."
			}
			return text
		}
	}
	return "(no text)"
}

// formatOffset returns a short human-readable elapsed time, e.g. "1m42s"
// or "2h03m". Used for tick tooltips so the user knows how deep into
// the session a tick sits.
func formatOffset(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// enrichTicksWithCost walks the session's TurnStats, maps each turn's
// cost + token count back onto its corresponding USER-TURN tick by
// AnchorID, then normalizes to a 0-1 "heat" scale using the highest
// turn as 100%. Also computes a running cumulative cost across all
// ticks in session order.
//
// Sub-event ticks (tickAgent, tickSkill) and system ticks (tickCompact)
// don't get their own cost — cost lives on the enclosing user turn.
// But the cumulative running total DOES advance past them so the
// tooltip's "so far" reads accurately as the mouse scrubs.
//
// When no turn has cost data (unknown models), falls back to token-
// based heat so unpriced sessions still paint a useful heatmap.
func enrichTicksWithCost(ticks []timelineTick, session *parser.Session) []timelineTick {
	if len(ticks) == 0 || session == nil {
		return ticks
	}

	turns := parser.ComputeTurnStats(flattenMessages(session.RootMessages))
	if len(turns) == 0 {
		return ticks
	}

	byAnchor := make(map[string]*parser.TurnStats, len(turns))
	for _, t := range turns {
		byAnchor[t.AnchorID] = t
	}

	var maxCost float64
	var maxTokens int
	for i := range ticks {
		// Only primary anchors (user/command) get cost data wired through.
		// Compact/agent/skill are decoration or sub-events.
		if ticks[i].Kind != tickUser && ticks[i].Kind != tickCommand {
			continue
		}
		if turn, ok := byAnchor[ticks[i].UUID]; ok {
			ticks[i].CostUSD = turn.CostUSD
			ticks[i].TotalTokens = turn.TotalTokens()
			if turn.CostUSD > maxCost {
				maxCost = turn.CostUSD
			}
			if tt := turn.TotalTokens(); tt > maxTokens {
				maxTokens = tt
			}
		}
	}

	// Prefer cost-based heat; fall back to token counts for unpriced.
	usesCost := maxCost > 0
	for i := range ticks {
		if ticks[i].Kind != tickUser && ticks[i].Kind != tickCommand {
			continue
		}
		switch {
		case usesCost && ticks[i].CostUSD > 0:
			ticks[i].Heat = ticks[i].CostUSD / maxCost
		case !usesCost && maxTokens > 0 && ticks[i].TotalTokens > 0:
			ticks[i].Heat = float64(ticks[i].TotalTokens) / float64(maxTokens)
		}
	}

	// Cumulative pass — walk in session order and accumulate. Sub-event
	// ticks contribute 0 each, so "so far" only advances when a priced
	// user turn is crossed.
	var running float64
	for i := range ticks {
		running += ticks[i].CostUSD
		ticks[i].CumulativeCostUSD = running
	}

	return ticks
}

// renderTimelineRail writes the HTML for the time-machine rail plus its
// floating tooltip sibling. Rail is empty (but still present) when the
// session has no usable ticks, so CSS layout stays consistent.
//
// Ticks are rendered as spans (not anchors) because the rail itself
// handles click-to-nearest via JS — individual tick pointer events are
// disabled in CSS. Each tick carries data-uuid / data-offset /
// data-snippet / data-kind / data-cost / data-tokens / data-cumulative
// / data-index for the hover-scrub handler to read.
func renderTimelineRail(b *strings.Builder, session *parser.Session) {
	ticks := enrichTicksWithCost(computeTimelineTicks(session), session)

	if len(ticks) == 0 {
		b.WriteString(`<aside class="timeline-rail has-no-ticks" id="timeline-rail" aria-label="Session timeline">`)
		b.WriteString(`<div class="timeline-spine timeline-empty">turns</div>`)
		b.WriteString(`</aside>`)
		return
	}

	b.WriteString(`<aside class="timeline-rail" id="timeline-rail" aria-label="Session timeline">`)
	b.WriteString(`<div class="timeline-spine" id="timeline-spine">`)

	// Index-based ruler gridlines — labels are turn ordinals ("10",
	// "20", ...). Visible subtly when collapsed, labels fade in on
	// hover. Non-interactive.
	for _, line := range computeTimelineGridlines(len(ticks)) {
		class := "timeline-gridline"
		if line.IsMajor {
			class += " gridline-major"
		}
		b.WriteString(fmt.Sprintf(
			`<div class="%s" style="top:%.2f%%"><span class="gridline-label">%s</span></div>`,
			class,
			line.PercentTop,
			html.EscapeString(line.Label),
		))
	}

	// Current-position indicator (updated via JS as the user scrolls)
	b.WriteString(`<div class="timeline-current" id="timeline-current" style="top:0%"></div>`)
	// Dashed playhead — horizontal guide that snaps to the nearest tick
	// while the user mouses along the rail.
	b.WriteString(`<div class="timeline-playhead" id="timeline-playhead" style="top:0%"></div>`)

	for _, tick := range ticks {
		class := "timeline-tick " + tick.Kind.class()
		kindAttr := tick.Kind.attr()

		// Build inline style: index position + heat custom property.
		// CSS reads --heat to scale tick size and opacity so cost is
		// visually encoded without extra DOM.
		style := fmt.Sprintf("top:%.2f%%;--heat:%.3f", tick.PercentTop, tick.Heat)

		// Data attributes used by the JS hover-scrub handler. Cost,
		// tokens, and cumulative are preformatted strings so the tooltip
		// just displays them verbatim without re-running format logic.
		costAttr := ""
		if tick.CostUSD > 0 {
			costAttr = fmt.Sprintf(` data-cost="%s"`, formatCost(tick.CostUSD))
		}
		tokensAttr := ""
		if tick.TotalTokens > 0 {
			tokensAttr = fmt.Sprintf(` data-tokens="%s"`, formatTokens(tick.TotalTokens))
		}
		cumAttr := ""
		if tick.CumulativeCostUSD > 0 {
			cumAttr = fmt.Sprintf(` data-cumulative="%s"`, formatCost(tick.CumulativeCostUSD))
		}
		indexAttr := fmt.Sprintf(` data-index="%d"`, tick.Index)

		b.WriteString(fmt.Sprintf(
			`<span class="%s" style="%s" data-uuid="%s" data-offset="%s" data-kind="%s" data-snippet="%s"%s%s%s%s></span>`,
			class,
			style,
			html.EscapeString(sanitizeID(tick.UUID)),
			html.EscapeString(formatOffset(tick.Offset)),
			kindAttr,
			html.EscapeString(tick.Snippet),
			costAttr,
			tokensAttr,
			cumAttr,
			indexAttr,
		))
	}

	b.WriteString(`</div>`) // timeline-spine
	b.WriteString(`</aside>`)

	// Tooltip as a sibling — positioned by JS, hidden by default.
	// Layout:
	//   [kind-badge] +offset · turn N
	//   first line of the message
	//   cost $X.XX     so far $Y.YY     N tokens
	b.WriteString(`<div class="timeline-tooltip" id="timeline-tooltip" aria-hidden="true">`)
	b.WriteString(`<span class="tt-head"><span class="tt-kind"></span><span class="tt-offset"></span><span class="tt-index"></span></span>`)
	b.WriteString(`<span class="tt-snippet"></span>`)
	b.WriteString(`<span class="tt-meta"><span class="tt-cost"></span><span class="tt-cum"></span><span class="tt-tokens"></span></span>`)
	b.WriteString(`</div>`)
}
