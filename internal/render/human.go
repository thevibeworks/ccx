package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

// exportHuman renders only the human's actual turns: every
// KindUserPrompt message in wire order, nothing else. Compaction
// replays re-emit identical user turns at different lines with the
// same timestamp, so turns are deduplicated on (timestamp, text).
//
// Each turn carries a stable anchor (turn ordinal + message UUID
// prefix) so downstream notes can cite "<session>#<uuid8>" and
// resolve it back to the transcript.
func exportHuman(session *parser.Session, opts ExportOptions) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}
	switch strings.ToLower(opts.Format) {
	case "", "md", "markdown":
	default:
		return "", fmt.Errorf("unsupported format for shape=human: %s (use md)", opts.Format)
	}

	flat := parser.FlattenSessionMessages(session)

	type turn struct {
		msg  *parser.Message
		text string
	}
	type dedupKey struct {
		ts   int64
		text string
	}
	seen := make(map[dedupKey]bool)
	var turns []turn
	raw, dropped := 0, 0

	for _, m := range flat {
		if m == nil || m.IsSidechain || m.Kind != parser.KindUserPrompt {
			continue
		}
		text := humanTurnText(m)
		if text == "" {
			continue
		}
		raw++
		k := dedupKey{ts: m.Timestamp.UnixNano(), text: text}
		if seen[k] {
			dropped++
			continue
		}
		seen[k] = true
		turns = append(turns, turn{msg: m, text: text})
	}

	var md strings.Builder
	md.WriteString("# Human turns: " + truncateID(session.ID, 8) + "\n\n")
	if session.ProjectName != "" {
		md.WriteString("Project: " + session.ProjectName + "\n")
	}
	if !session.StartTime.IsZero() {
		md.WriteString("Started: " + session.StartTime.Format("2006-01-02 15:04") + "\n")
	}
	md.WriteString(fmt.Sprintf("Turns: %d", len(turns)))
	if dropped > 0 {
		md.WriteString(fmt.Sprintf(" (%d raw, %d replay duplicates dropped)", raw, dropped))
	}
	md.WriteString("\n\n---\n\n")

	for i, t := range turns {
		md.WriteString(fmt.Sprintf("## %d", i+1))
		if !t.msg.Timestamp.IsZero() {
			md.WriteString("  ·  " + t.msg.Timestamp.Format("2006-01-02 15:04:05"))
		}
		if t.msg.UUID != "" {
			md.WriteString("  ·  " + truncateID(t.msg.UUID, 8))
		}
		md.WriteString("\n\n")
		md.WriteString(t.text)
		md.WriteString("\n\n---\n\n")
	}

	return md.String(), nil
}

// humanTurnText joins a user message's text blocks verbatim. Image
// blocks become a placeholder so image-only turns still show up as
// turns instead of vanishing.
func humanTurnText(m *parser.Message) string {
	var parts []string
	images := 0
	for _, block := range m.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		case "image":
			images++
		}
	}
	if len(parts) == 0 && images > 0 {
		return fmt.Sprintf("[%d image(s)]", images)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
