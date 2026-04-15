package web

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

// timelineTick is one Exchange notch on the time-machine rail.
//
// The rail represents Exchanges (not individual events): major notches
// are one-per-Exchange, and all sub-activity (tool calls, sub-agents,
// skills, compaction) lives as Satellites attached to the owning notch.
// That keeps the rail legible at any session size while still letting
// users SEE that "exchange 42 spun up two sub-agents and made three
// tool calls before replying."
//
// Position is by Exchange INDEX, not wall-clock time. A session with
// an idle gap (user walked away for 3 hours and resumed) no longer
// compresses the actual work into a sliver of the rail. The original
// Timestamp is still kept for the tooltip so the user can read "when
// did this happen" without losing the even layout.
type timelineTick struct {
	UUID       string    // Anchor message UUID, used as #msg-<uuid>
	Timestamp  time.Time // Wall-clock timestamp of the anchor (Local for display)
	PercentTop float64   // Vertical position on the rail (0-100), index-based
	Index      int       // 1-based ordinal within the session
	Kind       tickKind  // user prompt, command, compact boundary
	Snippet    string    // One-line preview shown in hover tooltip

	// Offset from session start (for "how deep into the session" readout).
	Offset time.Duration
	// Duration is the wall-clock span from anchor to last message of
	// the exchange — "how long did this exchange take." Zero for
	// system-only ticks like compact boundaries.
	Duration time.Duration

	// Enrichment populated from the matching Exchange (if any).
	CostUSD           float64 // Sum of per-message cost for this exchange
	TotalTokens       int     // Sum of all token categories
	Heat              float64 // 0-1 normalized weight (highest exchange = 1.0)
	CumulativeCostUSD float64 // Running total up to and including this tick

	// Satellite markers are decorations on the primary notch: sub-agent
	// dispatches, skill invocations, tool calls, and embedded compaction
	// boundaries that happened inside this exchange. Rendered as small
	// dots fanning off the main notch.
	Satellites []satelliteMarker

	// Pre-computed badge counts so the tooltip renders without walking
	// Satellites again on every hover.
	SubagentCount int
	SkillCount    int
	ToolCount     int
}

// satelliteMarker is one sub-event attached to the owning exchange
// notch. Kind drives its color; Name feeds the hover label.
type satelliteMarker struct {
	Kind  satelliteKind
	Name  string
	Label string
}

type satelliteKind int

const (
	satSubagent satelliteKind = iota
	satSkill
	satTool
	satCompact
)

func (s satelliteKind) class() string {
	switch s {
	case satSubagent:
		return "sat-subagent"
	case satSkill:
		return "sat-skill"
	case satCompact:
		return "sat-compact"
	default:
		return "sat-tool"
	}
}

type tickKind int

const (
	tickUser    tickKind = iota // Normal user prompt — primary rail notch
	tickCommand                 // Slash command — primary notch, command color
	tickCompact                 // Context compaction boundary — widest notch
)

func (k tickKind) class() string {
	switch k {
	case tickCompact:
		return "tick-compact"
	case tickCommand:
		return "tick-command"
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
	default:
		return "user"
	}
}

// timelineGridline is a ruler-tick on the rail showing a round ordinal
// ("exchange 10", "exchange 20") at a uniform interval. Non-interactive.
type timelineGridline struct {
	PercentTop float64
	Label      string // "10" / "20" / "30" — exchange ordinal
	IsMajor    bool   // Every 5th gridline is bolder
}

// chooseGridStep picks a round step for index-based gridlines, aiming
// for 4-10 lines across the rail. Sessions under 20 exchanges don't
// get gridlines — the rail is short enough to scan directly.
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
	step := tickCount / 8
	if step < 1 {
		step = 1
	}
	return step
}

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
		isMajor := (i/step)%5 == 0
		lines = append(lines, timelineGridline{
			PercentTop: percent,
			Label:      fmt.Sprintf("%d", i),
			IsMajor:    isMajor,
		})
	}
	return lines
}

// computeTimelineTicks walks the session and produces one tick per
// Exchange (anchored by user prompt / command / compact summary).
// Returns nil when the session has no anchorable events.
//
// Position is INDEX-BASED: the i-th tick (0-indexed) lands at
// i/(N-1) * 100 percent, independent of wall-clock gaps.
func computeTimelineTicks(session *parser.Session) []timelineTick {
	if session == nil {
		return nil
	}

	type anchor struct {
		msg  *parser.Message
		kind tickKind
	}
	var anchors []anchor

	for _, msg := range flattenMessages(session.RootMessages) {
		if msg == nil || msg.IsSidechain {
			continue
		}
		switch msg.Kind {
		case parser.KindUserPrompt:
			anchors = append(anchors, anchor{msg: msg, kind: tickUser})
		case parser.KindCommand:
			anchors = append(anchors, anchor{msg: msg, kind: tickCommand})
		case parser.KindCompactSummary:
			anchors = append(anchors, anchor{msg: msg, kind: tickCompact})
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

		ticks = append(ticks, timelineTick{
			UUID:       a.msg.UUID,
			Timestamp:  a.msg.Timestamp,
			PercentTop: percent,
			Index:      i + 1,
			Kind:       a.kind,
			Snippet:    tickSnippet(a.msg),
			Offset:     offset,
		})
	}

	return ticks
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
// or "2h03m".
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

// formatLocalClock returns viewer-local 24h wall-clock time HH:MM.
// Empty string when the timestamp is zero.
func formatLocalClock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("15:04")
}

// enrichTicksWithCost maps each Exchange tick back to its matching
// parser.Exchange, copies cost / tokens / duration / step counts onto
// the tick, normalizes heat to 0-1, and walks in order to compute the
// cumulative running cost readout for the tooltip.
//
// Sub-event contribution: tool / sub-agent / skill steps become
// Satellites on the owning exchange's tick. They don't get their own
// rail notch — the rail is an Exchange-level ruler.
func enrichTicksWithCost(ticks []timelineTick, session *parser.Session) []timelineTick {
	if len(ticks) == 0 || session == nil {
		return ticks
	}

	exchanges := parser.ComputeExchanges(parser.FlattenSessionMessages(session))
	if len(exchanges) == 0 {
		return ticks
	}

	byAnchor := make(map[string]*parser.Exchange, len(exchanges))
	for _, e := range exchanges {
		byAnchor[e.AnchorID] = e
	}

	var maxCost float64
	var maxTokens int
	for i := range ticks {
		if ticks[i].Kind != tickUser && ticks[i].Kind != tickCommand {
			continue
		}
		ex, ok := byAnchor[ticks[i].UUID]
		if !ok {
			continue
		}
		ticks[i].CostUSD = ex.CostUSD
		ticks[i].TotalTokens = ex.TotalTokens()
		ticks[i].Duration = ex.Duration()
		ticks[i].Satellites = buildSatellites(ex)
		ticks[i].SubagentCount = ex.CountSteps(parser.StepSubagent)
		ticks[i].SkillCount = ex.CountSteps(parser.StepSkill)
		ticks[i].ToolCount = ex.CountSteps(parser.StepToolUse)
		if ex.CostUSD > maxCost {
			maxCost = ex.CostUSD
		}
		if tt := ex.TotalTokens(); tt > maxTokens {
			maxTokens = tt
		}
	}

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

	// Cumulative pass — accumulate running cost in session order so the
	// tooltip can show "so far $X.XX" as the mouse scrubs.
	var running float64
	for i := range ticks {
		running += ticks[i].CostUSD
		ticks[i].CumulativeCostUSD = running
	}

	return ticks
}

// buildSatellites turns Exchange.Steps into rail satellite markers,
// capped to a reasonable render budget so an exchange with 50 tool
// calls doesn't spam the rail with 50 dots. The cap keeps the primary
// notch legible; the tooltip still shows full counts via the badge row.
func buildSatellites(ex *parser.Exchange) []satelliteMarker {
	if ex == nil || len(ex.Steps) == 0 {
		return nil
	}
	const maxSatellites = 8
	out := make([]satelliteMarker, 0, len(ex.Steps))

	// Priority order: subagent > skill > compact > tool. Prioritising
	// ensures the few rendered dots are the interesting ones.
	add := func(kind parser.StepKind, satKind satelliteKind) {
		for _, s := range ex.Steps {
			if len(out) >= maxSatellites {
				return
			}
			if s.Kind != kind {
				continue
			}
			out = append(out, satelliteMarker{Kind: satKind, Name: s.Name, Label: s.Label})
		}
	}
	add(parser.StepSubagent, satSubagent)
	add(parser.StepSkill, satSkill)
	add(parser.StepCompaction, satCompact)
	add(parser.StepToolUse, satTool)
	return out
}

// renderTimelineRail writes the HTML for the time-machine rail plus
// its floating tooltip sibling.
//
// Each tick is a <span> with data attributes the JS hover-scrub reads.
// Satellites render as small <b> dots inside the tick, which CSS
// positions just to the left of the primary notch so they don't fight
// for the same pixel column.
func renderTimelineRail(b *strings.Builder, session *parser.Session) {
	ticks := enrichTicksWithCost(computeTimelineTicks(session), session)

	if len(ticks) == 0 {
		b.WriteString(`<aside class="timeline-rail has-no-ticks" id="timeline-rail" aria-label="Session timeline">`)
		b.WriteString(`<div class="timeline-spine timeline-empty">exchanges</div>`)
		b.WriteString(`</aside>`)
		return
	}

	b.WriteString(`<aside class="timeline-rail" id="timeline-rail" aria-label="Session timeline">`)
	b.WriteString(`<div class="timeline-spine" id="timeline-spine">`)

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

	b.WriteString(`<div class="timeline-current" id="timeline-current" style="top:0%"></div>`)
	b.WriteString(`<div class="timeline-playhead" id="timeline-playhead" style="top:0%"></div>`)

	for _, tick := range ticks {
		class := "timeline-tick " + tick.Kind.class()
		kindAttr := tick.Kind.attr()
		style := fmt.Sprintf("top:%.2f%%;--heat:%.3f", tick.PercentTop, tick.Heat)

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
		clockAttr := ""
		if clock := formatLocalClock(tick.Timestamp); clock != "" {
			clockAttr = fmt.Sprintf(` data-clock="%s"`, clock)
		}
		durAttr := ""
		if tick.Duration > 0 {
			durAttr = fmt.Sprintf(` data-duration="%s"`, formatOffset(tick.Duration))
		}
		subAttr := ""
		if tick.SubagentCount > 0 {
			subAttr = fmt.Sprintf(` data-subagents="%d"`, tick.SubagentCount)
		}
		skillAttr := ""
		if tick.SkillCount > 0 {
			skillAttr = fmt.Sprintf(` data-skills="%d"`, tick.SkillCount)
		}
		toolAttr := ""
		if tick.ToolCount > 0 {
			toolAttr = fmt.Sprintf(` data-tools="%d"`, tick.ToolCount)
		}
		indexAttr := fmt.Sprintf(` data-index="%d"`, tick.Index)

		b.WriteString(fmt.Sprintf(
			`<span class="%s" style="%s" data-uuid="%s" data-offset="%s" data-kind="%s" data-snippet="%s"%s%s%s%s%s%s%s%s%s>`,
			class,
			style,
			html.EscapeString(sanitizeID(tick.UUID)),
			html.EscapeString(formatOffset(tick.Offset)),
			kindAttr,
			html.EscapeString(tick.Snippet),
			costAttr,
			tokensAttr,
			cumAttr,
			clockAttr,
			durAttr,
			subAttr,
			skillAttr,
			toolAttr,
			indexAttr,
		))
		// Render satellites inside the tick so CSS can position them
		// relative to the parent notch.
		for _, sat := range tick.Satellites {
			b.WriteString(fmt.Sprintf(
				`<b class="sat %s" title="%s"></b>`,
				sat.Kind.class(),
				html.EscapeString(sat.Label),
			))
		}
		b.WriteString(`</span>`)
	}

	b.WriteString(`</div>`) // timeline-spine
	b.WriteString(`</aside>`)

	// Tooltip sibling — populated by JS on hover.
	//
	//   [clock]  exchange N  +offset · duration    ← head row
	//   first line of the prompt                   ← snippet
	//   $cost · so far $cumulative · N tok         ← spend row
	//   ⎇ 2 subagents · ✦ 1 skill · ◈ 3 tools      ← badge row (only if any)
	b.WriteString(`<div class="timeline-tooltip" id="timeline-tooltip" aria-hidden="true">`)
	b.WriteString(`<span class="tt-head"><span class="tt-clock"></span><span class="tt-index"></span><span class="tt-offset"></span><span class="tt-duration"></span></span>`)
	b.WriteString(`<span class="tt-snippet"></span>`)
	b.WriteString(`<span class="tt-meta"><span class="tt-cost"></span><span class="tt-cum"></span><span class="tt-tokens"></span></span>`)
	b.WriteString(`<span class="tt-badges"></span>`)
	b.WriteString(`</div>`)
}
