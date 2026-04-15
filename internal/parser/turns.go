package parser

import (
	"strings"
	"time"
)

// Exchange is one complete request/response unit: a user prompt (or
// slash command) plus every assistant emission, tool call, tool result,
// sub-agent dispatch, and sidechain message it triggered, up to (but
// not including) the next user-initiated message.
//
// Exchange is how ccx answers "where did my quota go" — each Exchange
// is a clickable row in the per-Exchange breakdown UI with a cost
// figure, and each notch on the timeline rail maps to one Exchange.
//
// The type name deliberately replaces the older "turn" vocabulary:
// from a user's POV, the unit of interest is one request they made
// and the one response they got back, including everything the agent
// did in between.
type Exchange struct {
	Index    int       // 1-based position in session (oldest first)
	AnchorID string    // UUID of the user prompt / command that starts this exchange
	Snippet  string    // First line of the user prompt (or command name)
	Model    string    // Primary model used for this exchange's assistant response
	Start    time.Time // Anchor message timestamp
	End      time.Time // Last-in-exchange message timestamp

	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	ReasoningTokens   int
	CostUSD           float64 // Sum of per-message cost across the exchange

	MessageCount  int // Total messages in the exchange (anchor + assistants + tools + sidechains)
	AssistantHits int // Number of assistant messages in the exchange
	HasSidechain  bool

	// Steps enumerates notable sub-events inside this exchange in wire
	// order: tool calls, sub-agent dispatches, skill invocations, and
	// plugin calls. The timeline rail reads this to paint satellite
	// markers on the owning exchange notch.
	Steps []Step
}

// TurnStats is a back-compat alias so older callers (tests, render,
// web) compile during the rename. New code should use Exchange.
//
// Deprecated: use Exchange.
type TurnStats = Exchange

// Duration returns the wall-clock span of this exchange. Zero when
// either timestamp is unset.
func (e *Exchange) Duration() time.Duration {
	if e == nil || e.Start.IsZero() || e.End.IsZero() {
		return 0
	}
	if e.End.Before(e.Start) {
		return 0
	}
	return e.End.Sub(e.Start)
}

// TotalTokens returns the sum of all token categories for the exchange.
func (e *Exchange) TotalTokens() int {
	if e == nil {
		return 0
	}
	return e.InputTokens + e.OutputTokens + e.CacheReadTokens + e.CacheCreateTokens + e.ReasoningTokens
}

// CountSteps returns how many steps of a given kind live in this
// exchange. Used by tooltip badge rendering.
func (e *Exchange) CountSteps(kind StepKind) int {
	if e == nil {
		return 0
	}
	n := 0
	for _, s := range e.Steps {
		if s.Kind == kind {
			n++
		}
	}
	return n
}

// StepKind classifies sub-events inside an Exchange. Tool calls,
// sub-agent dispatches, skill invocations, and plugin/command calls
// are all Steps — the thing a user intuitively means by "what did the
// agent actually do in this exchange."
type StepKind int

const (
	StepToolUse    StepKind = iota // Plain tool call (Read, Edit, Bash, ...)
	StepSubagent                   // Task tool — sub-agent dispatch
	StepSkill                      // Skill tool invocation
	StepPlugin                     // Slash command (aka plugin command) embedded mid-exchange
	StepCompaction                 // Context compaction boundary inside the exchange
)

// Step is one notable sub-event inside an Exchange. The UUID points at
// the assistant message that emitted it, so rail markers and outline
// entries can jump to the right anchor.
type Step struct {
	Kind      StepKind
	UUID      string // assistant message UUID that emitted this step
	Name      string // tool name, subagent_type, skill name, command name
	Label     string // longer human-readable preview
	Timestamp time.Time
}

// ComputeExchanges walks a flat, session-ordered slice of messages and
// segments them into Exchanges anchored by user prompts or slash
// commands. Messages appearing before any anchor are silently dropped
// (they're session metadata, not part of an exchange).
//
// The input slice is expected to be in wire order (the order messages
// were written to the JSONL file). Use FlattenSessionMessages or walk
// tree roots to produce it.
//
// Returns exchanges in chronological order. Each exchange's tokens/
// cost are the sum of per-message usage for every message the
// exchange contains — including sidechains. This matches Anthropic
// billing; ccx does not attempt sidechain cache-read dedup because
// the user is charged for those reads regardless.
func ComputeExchanges(messages []*Message) []*Exchange {
	if len(messages) == 0 {
		return nil
	}

	var exchanges []*Exchange
	var current *Exchange

	flush := func() {
		if current != nil {
			exchanges = append(exchanges, current)
		}
	}

	for _, msg := range messages {
		if msg == nil {
			continue
		}

		if isExchangeAnchor(msg) {
			flush()
			current = &Exchange{
				Index:    len(exchanges) + 1,
				AnchorID: msg.UUID,
				Snippet:  exchangeSnippet(msg),
				Start:    msg.Timestamp,
				End:      msg.Timestamp,
			}
		}

		if current == nil {
			continue
		}

		current.MessageCount++
		if !msg.Timestamp.IsZero() && msg.Timestamp.After(current.End) {
			current.End = msg.Timestamp
		}
		if msg.IsSidechain {
			current.HasSidechain = true
		}
		if msg.Kind == KindAssistant {
			current.AssistantHits++
			if current.Model == "" && msg.Model != "" {
				current.Model = msg.Model
			}
			// Only collect steps from MAIN-TREE assistant messages. A
			// sub-agent's tool calls are interesting for the sub-agent
			// itself, not for the enclosing user turn: bubbling them up
			// would inflate the parent exchange's tool/badge counts and
			// spam the rail with 20+ satellite markers for a single
			// sub-agent dispatch. The subagent is already represented
			// by the parent's Task tool_use, which DOES count as a
			// StepSubagent on the parent.
			if !msg.IsSidechain {
				current.Steps = append(current.Steps, extractSteps(msg)...)
			}
		}
		if msg.Kind == KindCompactSummary && msg.UUID != current.AnchorID {
			current.Steps = append(current.Steps, Step{
				Kind:      StepCompaction,
				UUID:      msg.UUID,
				Name:      "compact",
				Label:     "Context compaction",
				Timestamp: msg.Timestamp,
			})
		}
		if msg.Usage != nil {
			current.InputTokens += msg.Usage.InputTokens
			current.OutputTokens += msg.Usage.OutputTokens
			current.CacheReadTokens += msg.Usage.CacheReadTokens
			current.CacheCreateTokens += msg.Usage.CacheCreateTokens
			current.ReasoningTokens += msg.Usage.ReasoningTokens
			current.CostUSD += msg.Usage.CostUSD
		}
	}

	flush()

	return exchanges
}

// ComputeTurnStats is a deprecated alias that forwards to
// ComputeExchanges. Kept during the rename so call sites migrate
// incrementally.
//
// Deprecated: use ComputeExchanges.
func ComputeTurnStats(messages []*Message) []*Exchange {
	return ComputeExchanges(messages)
}

// extractSteps inspects an assistant message's tool_use blocks and
// returns one Step per interesting call. Task and Skill get their own
// kinds so the rail can paint them distinctly; ordinary tool calls are
// captured as StepToolUse so they contribute to the "N tools" badge
// without adding visual clutter.
func extractSteps(msg *Message) []Step {
	if msg == nil || msg.Kind != KindAssistant {
		return nil
	}
	var steps []Step
	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}
		switch block.ToolName {
		case "Task":
			steps = append(steps, Step{
				Kind:      StepSubagent,
				UUID:      msg.UUID,
				Name:      subagentName(block),
				Label:     subagentLabel(block),
				Timestamp: msg.Timestamp,
			})
		case "Skill":
			steps = append(steps, Step{
				Kind:      StepSkill,
				UUID:      msg.UUID,
				Name:      skillName(block),
				Label:     skillLabel(block),
				Timestamp: msg.Timestamp,
			})
		default:
			steps = append(steps, Step{
				Kind:      StepToolUse,
				UUID:      msg.UUID,
				Name:      block.ToolName,
				Timestamp: msg.Timestamp,
			})
		}
	}
	return steps
}

func subagentName(block ContentBlock) string {
	input, _ := block.ToolInput.(map[string]any)
	if input == nil {
		return "Task"
	}
	if s, ok := input["subagent_type"].(string); ok && s != "" {
		return s
	}
	return "Task"
}

func subagentLabel(block ContentBlock) string {
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

func skillName(block ContentBlock) string {
	input, _ := block.ToolInput.(map[string]any)
	if input == nil {
		return "Skill"
	}
	if s, ok := input["skill"].(string); ok && s != "" {
		return s
	}
	return "Skill"
}

func skillLabel(block ContentBlock) string {
	input, _ := block.ToolInput.(map[string]any)
	if input == nil {
		return "Skill"
	}
	name, _ := input["skill"].(string)
	args, _ := input["args"].(string)
	switch {
	case name != "" && args != "":
		return "/" + name + " " + args
	case name != "":
		return "/" + name
	}
	return "Skill"
}

// isExchangeAnchor reports whether a message starts a new exchange.
// User prompts and slash commands are anchors. Tool results, compact
// summaries, sidechain children, and meta messages are not — they
// belong to the surrounding exchange.
func isExchangeAnchor(msg *Message) bool {
	if msg == nil || msg.IsSidechain {
		return false
	}
	switch msg.Kind {
	case KindUserPrompt, KindCommand:
		return true
	}
	return false
}

// exchangeSnippet returns a one-line summary for an exchange anchor.
// Prefers the first line of user text; for commands, returns "/name args".
func exchangeSnippet(msg *Message) string {
	if msg == nil {
		return ""
	}
	if msg.IsCommand && msg.CommandName != "" {
		if msg.CommandArgs != "" {
			return "/" + msg.CommandName + " " + msg.CommandArgs
		}
		return "/" + msg.CommandName
	}
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			if idx := strings.Index(text, "\n"); idx > 0 {
				text = text[:idx]
			}
			return text
		}
	}
	return "(no text)"
}

// FlattenSessionMessages returns a flat, wire-order slice of messages
// for a parsed session. Walks tree roots depth-first and collects every
// descendant. Used as input to ComputeExchanges.
func FlattenSessionMessages(session *Session) []*Message {
	if session == nil {
		return nil
	}
	var out []*Message
	var walk func([]*Message)
	walk = func(nodes []*Message) {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			out = append(out, n)
			walk(n.Children)
		}
	}
	walk(session.RootMessages)
	return out
}
