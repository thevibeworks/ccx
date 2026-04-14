package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

// editTools are the tool names that count as "file edits" in the exec
// summary. Emitted when an assistant uses one of these, a line in
// Files touched is produced for the enclosing turn.
var editTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"Create":       true, // legacy / alternate naming
}

// ExecMarkdown renders a session as an executive-style turn-by-turn
// report: user request, files touched, final agent summary, and
// optional per-turn cost/token footprint. Useful for sharing "what did
// the agent do" without dragging every tool invocation along.
//
// Layout (per turn):
//
//	## Turn N
//
//	> user prompt (first line, quoted)
//
//	**Files touched:** path/a, path/b
//
//	agent's final text response
//
//	*cost: $0.0423 · 12.3k tokens*
//
// Turns with no user text, no edits, AND no summary are dropped — an
// empty turn is usually a meta/compact boundary and not interesting
// for an executive report.
func ExecMarkdown(session *parser.Session) string {
	if session == nil {
		return ""
	}

	var b strings.Builder

	// Header: id + summary + session totals
	b.WriteString(fmt.Sprintf("# Session %s\n\n", session.ID))
	if s := strings.TrimSpace(session.Summary); s != "" && s != "(no summary)" {
		b.WriteString("> " + s + "\n\n")
	}
	writeSessionHeaderFacts(&b, session)
	b.WriteString("---\n\n")

	// Flatten all messages into wire order so we can segment into turns
	// anchored by user prompts / commands. Ignoring compact summaries
	// as anchors — they're rendered separately between turns so readers
	// see where context got rolled over.
	allMsgs := flattenAllMessages(session.RootMessages)
	turnsStats := parser.ComputeTurnStats(allMsgs)
	costByAnchor := make(map[string]*parser.TurnStats, len(turnsStats))
	for _, t := range turnsStats {
		costByAnchor[t.AnchorID] = t
	}

	type turnBlock struct {
		anchor   *parser.Message
		messages []*parser.Message
	}
	var blocks []turnBlock
	var current *turnBlock
	for _, msg := range allMsgs {
		if msg == nil {
			continue
		}

		if msg.Kind == parser.KindCompactSummary {
			// Flush any in-progress turn, then emit the compact marker
			// in the output stream (handled below after loop). For now
			// we close the current turn and skip.
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			blocks = append(blocks, turnBlock{anchor: msg})
			continue
		}

		if msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &turnBlock{anchor: msg}
			continue
		}

		if current != nil {
			current.messages = append(current.messages, msg)
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}

	turnNum := 0
	for _, block := range blocks {
		if block.anchor == nil {
			continue
		}
		if block.anchor.Kind == parser.KindCompactSummary {
			b.WriteString("---\n\n*— context compaction —*\n\n---\n\n")
			continue
		}

		userText := firstTextBlock(block.anchor)
		edits := extractEditedFiles(block.messages)
		summary := lastAssistantText(block.messages)

		// Drop empty turns entirely
		if userText == "" && len(edits) == 0 && summary == "" {
			continue
		}

		turnNum++
		b.WriteString(fmt.Sprintf("## Turn %d\n\n", turnNum))

		if userText != "" {
			b.WriteString(quoteBlock(userText))
			b.WriteString("\n")
		}

		if len(edits) > 0 {
			b.WriteString("**Files touched:** ")
			for i, f := range edits {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("`" + f + "`")
			}
			b.WriteString("\n\n")
		}

		if summary != "" {
			b.WriteString(summary)
			b.WriteString("\n\n")
		}

		// Per-turn cost footer (if we know it)
		if ts, ok := costByAnchor[block.anchor.UUID]; ok && ts != nil {
			if ts.CostUSD > 0 || ts.TotalTokens() > 0 {
				b.WriteString("*")
				if ts.CostUSD > 0 {
					b.WriteString(fmt.Sprintf("cost: $%.4f", ts.CostUSD))
				}
				if ts.TotalTokens() > 0 {
					if ts.CostUSD > 0 {
						b.WriteString(" · ")
					}
					b.WriteString(fmt.Sprintf("%s tokens", formatTokenCountShort(ts.TotalTokens())))
				}
				b.WriteString("*\n\n")
			}
		}
	}

	return b.String()
}

// writeSessionHeaderFacts emits the header meta line: duration,
// messages, tools, total cost if priced.
func writeSessionHeaderFacts(b *strings.Builder, session *parser.Session) {
	stats := session.Stats
	var parts []string
	if stats.DurationSeconds > 0 {
		parts = append(parts, "**Duration:** "+formatDurationShort(stats.DurationSeconds))
	}
	if stats.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("**Messages:** %d", stats.MessageCount))
	}
	if stats.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("**Tool calls:** %d", stats.ToolCalls))
	}
	if stats.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("**Total cost:** $%.4f", stats.CostUSD))
	}
	if session.Model != "" {
		parts = append(parts, "**Model:** `"+session.Model+"`")
	}
	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " · "))
		b.WriteString("\n\n")
	}
}

// flattenAllMessages walks the tree depth-first and returns every
// message in wire order. Shared with web rendering conceptually, but
// copied here to avoid a cross-package dependency from render → web.
func flattenAllMessages(roots []*parser.Message) []*parser.Message {
	var out []*parser.Message
	var walk func([]*parser.Message)
	walk = func(msgs []*parser.Message) {
		for _, m := range msgs {
			if m == nil {
				continue
			}
			out = append(out, m)
			walk(m.Children)
		}
	}
	walk(roots)
	return out
}

// firstTextBlock returns the first non-empty text block from a
// message's content — used to extract the "user request" line from a
// user prompt message.
func firstTextBlock(msg *parser.Message) string {
	if msg == nil {
		return ""
	}
	for _, block := range msg.Content {
		if block.Type == "text" {
			text := strings.TrimSpace(block.Text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

// lastAssistantText walks the messages following a user turn and
// returns the text of the LAST assistant message that has text
// content. That's typically the agent's final summary — the thing a
// reader of the exec report actually wants to read.
func lastAssistantText(messages []*parser.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m == nil || m.Kind != parser.KindAssistant {
			continue
		}
		text := firstTextBlock(m)
		if text != "" {
			return text
		}
	}
	return ""
}

// extractEditedFiles walks the assistant messages in a turn and
// collects any file paths referenced by Edit / Write / MultiEdit /
// NotebookEdit tool calls. Deduplicates and sorts for deterministic
// output. Empty slice when the turn touched no files.
func extractEditedFiles(messages []*parser.Message) []string {
	seen := make(map[string]struct{})
	for _, m := range messages {
		if m == nil || m.Kind != parser.KindAssistant {
			continue
		}
		for _, block := range m.Content {
			if block.Type != "tool_use" {
				continue
			}
			if !editTools[block.ToolName] {
				continue
			}
			path := extractFilePath(block.ToolInput)
			if path != "" {
				seen[path] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// extractFilePath pulls a file path out of a tool_use block's input
// map. Handles the common shapes used by Edit/Write/NotebookEdit.
func extractFilePath(toolInput any) string {
	input, _ := toolInput.(map[string]any)
	if input == nil {
		return ""
	}
	// Edit/Write/MultiEdit use "file_path"
	if p, ok := input["file_path"].(string); ok && p != "" {
		return p
	}
	// NotebookEdit uses "notebook_path"
	if p, ok := input["notebook_path"].(string); ok && p != "" {
		return p
	}
	// Some Create-style tools use "path"
	if p, ok := input["path"].(string); ok && p != "" {
		return p
	}
	return ""
}

// quoteBlock wraps text in a markdown blockquote, handling multi-line
// text by prefixing every line with "> ".
func quoteBlock(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// formatDurationShort returns a concise duration string — used in the
// session header for readability. Mirrors the pattern from web's
// formatDuration but lives here to keep render self-contained.
func formatDurationShort(seconds float64) string {
	s := int(seconds)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
}

// formatTokenCountShort returns "1.2k" / "45" style token counts for
// compact display in the exec report.
func formatTokenCountShort(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
