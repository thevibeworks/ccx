package parser

import (
	"strings"
	"time"
)

// TurnStats holds token usage and cost aggregated across one user turn.
// A "turn" is a user prompt (or slash command) plus every assistant
// response, tool call, tool result, and sidechain message that follows
// it until the next user-initiated message.
//
// TurnStats is how ccx answers "where did my quota go" — each turn is
// a clickable row in the per-turn breakdown UI with a cost figure.
type TurnStats struct {
	Index    int       // 1-based position in session (oldest first)
	AnchorID string    // UUID of the user prompt / command that starts this turn
	Snippet  string    // First line of the user prompt (or command name)
	Model    string    // Primary model used for this turn's assistant response
	Start    time.Time // Anchor message timestamp
	End      time.Time // Last-in-turn message timestamp

	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	CostUSD           float64 // Sum of per-message cost across the turn

	MessageCount  int // Total messages in the turn (anchor + assistants + tools + sidechains)
	AssistantHits int // Number of assistant messages in the turn
	HasSidechain  bool
}

// TotalTokens returns the sum of all four token categories for the turn.
func (t *TurnStats) TotalTokens() int {
	if t == nil {
		return 0
	}
	return t.InputTokens + t.OutputTokens + t.CacheReadTokens + t.CacheCreateTokens
}

// ComputeTurnStats walks a flat, session-ordered slice of messages and
// segments them into turns anchored by user prompts or slash commands.
// Messages appearing before any anchor are silently dropped (they're
// session metadata, not part of a turn).
//
// The input slice is expected to be in wire order (the order messages
// were written to the JSONL file). Use flattenSessionMessages or walk
// tree roots to produce it.
//
// Returns turns in chronological order. Each turn's Usage is the sum
// of per-message usage for every message the turn contains — including
// sidechains. This matches Anthropic billing; ccx does not attempt
// sidechain cache-read dedup because the user is charged for those
// reads regardless.
func ComputeTurnStats(messages []*Message) []*TurnStats {
	if len(messages) == 0 {
		return nil
	}

	var turns []*TurnStats
	var current *TurnStats

	flush := func() {
		if current != nil {
			turns = append(turns, current)
		}
	}

	for _, msg := range messages {
		if msg == nil {
			continue
		}

		if isTurnAnchor(msg) {
			flush()
			current = &TurnStats{
				Index:    len(turns) + 1,
				AnchorID: msg.UUID,
				Snippet:  turnSnippet(msg),
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
		}
		if msg.Usage != nil {
			current.InputTokens += msg.Usage.InputTokens
			current.OutputTokens += msg.Usage.OutputTokens
			current.CacheReadTokens += msg.Usage.CacheReadTokens
			current.CacheCreateTokens += msg.Usage.CacheCreateTokens
			current.CostUSD += msg.Usage.CostUSD
		}
	}

	flush()

	return turns
}

// isTurnAnchor reports whether a message starts a new turn. User prompts
// and slash commands are anchors. Tool results, compact summaries, and
// meta messages are not — they belong to the surrounding turn.
func isTurnAnchor(msg *Message) bool {
	switch msg.Kind {
	case KindUserPrompt, KindCommand:
		return true
	}
	return false
}

// turnSnippet returns a one-line summary for a turn anchor. Prefers the
// first line of user text; for commands, returns "/name args".
func turnSnippet(msg *Message) string {
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
// descendant. Used as input to ComputeTurnStats.
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
