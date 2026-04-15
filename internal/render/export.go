package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

// Shape selects how much of the session ends up in the export.
//
// full      — everything: user prompts, assistant text, thinking
//
//	blocks, tool calls, tool results, sidechain messages
//
// brief     — conversation only: user, assistant text, compactions.
//
//	Strips thinking, tool use, tool results, meta messages.
//
// trace     — agent trace view: same as full but drops sidechain
//
//	conversation bodies, keeping just their Task dispatch
//	markers for context. Useful for tracking "what did the
//	agent actually do" without the sub-agent chatter.
//
// exchange  — one block per Exchange with the user prompt, the final
//
//	assistant reply, and a compact stats footer. Nothing
//	else. Produces a reviewable digest of the whole session.
type Shape string

const (
	ShapeFull     Shape = "full"
	ShapeBrief    Shape = "brief"
	ShapeTrace    Shape = "trace"
	ShapeExchange Shape = "exchange"
)

// Envelope controls how much chrome wraps the exported session. Only
// applies to HTML today; other formats ignore it.
//
// standalone — full <!DOCTYPE html> document with <head>, CSS, JS
// fragment   — just the session content div, no wrapper. Useful for
//
//	embedding in a larger document.
type Envelope string

const (
	EnvelopeStandalone Envelope = "standalone"
	EnvelopeFragment   Envelope = "fragment"
)

type ExportOptions struct {
	Format          string
	Theme           string
	IncludeThinking bool
	IncludeAgents   bool

	// Shape defaults to ShapeFull when unset. The legacy Brief flag
	// forwards to ShapeBrief when true.
	Shape Shape
	Brief bool // deprecated: use Shape = ShapeBrief

	// Envelope defaults to EnvelopeStandalone when unset. Ignored by
	// Markdown/Org which are always "fragmentish" by nature.
	Envelope Envelope

	TemplatePath string
}

func Export(session *parser.Session, opts ExportOptions) (string, error) {
	// exec is a turn-by-turn executive summary (user request, files
	// touched, final agent response). It bypasses the normal shape
	// pipeline because it's structurally different: a report, not a
	// rendered conversation transcript.
	if strings.EqualFold(opts.Format, "exec") || strings.EqualFold(opts.Format, "exec-md") {
		return ExecMarkdown(session), nil
	}

	shape := opts.Shape
	if shape == "" {
		if opts.Brief {
			shape = ShapeBrief
		} else {
			shape = ShapeFull
		}
	}

	switch shape {
	case ShapeBrief:
		session = BriefSession(session)
	case ShapeTrace:
		session = TraceSession(session)
	case ShapeExchange:
		return exportExchange(session, opts)
	}

	switch strings.ToLower(opts.Format) {
	case "html":
		return exportHTML(session, opts)
	case "md", "markdown":
		return exportMarkdown(session, opts)
	case "org":
		return exportOrg(session, opts)
	default:
		return "", fmt.Errorf("unsupported format: %s", opts.Format)
	}
}

// TraceSession returns a copy of session with sidechain bodies dropped.
// The parent assistant messages that dispatched those sidechains stay
// in place so the reader can still see "here's where the agent spun
// up a Plan subagent" without reading the subagent's conversation.
func TraceSession(session *parser.Session) *parser.Session {
	if session == nil {
		return nil
	}
	cp := *session
	cp.RootMessages = dropSidechains(session.RootMessages)
	return &cp
}

func dropSidechains(msgs []*parser.Message) []*parser.Message {
	out := make([]*parser.Message, 0, len(msgs))
	for _, m := range msgs {
		if m == nil || m.IsSidechain {
			continue
		}
		cp := *m
		cp.Children = dropSidechains(m.Children)
		out = append(out, &cp)
	}
	return out
}

// exportExchange renders one block per Exchange: user prompt, last
// assistant text, stats footer. Works across all formats via a simple
// plaintext/markdown pass; HTML wraps it in a minimal container.
func exportExchange(session *parser.Session, opts ExportOptions) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}
	exchanges := parser.ComputeExchanges(parser.FlattenSessionMessages(session))

	var md strings.Builder
	md.WriteString("# Session digest: " + truncateID(session.ID, 8) + "\n\n")
	if !session.StartTime.IsZero() {
		md.WriteString("Started: " + session.StartTime.Format("2006-01-02 15:04") + "\n")
	}
	md.WriteString(fmt.Sprintf("Exchanges: %d\n\n", len(exchanges)))
	md.WriteString("---\n\n")

	for _, ex := range exchanges {
		if ex == nil {
			continue
		}
		md.WriteString(fmt.Sprintf("## exchange %d", ex.Index))
		if !ex.Start.IsZero() {
			md.WriteString("  ·  " + ex.Start.Format("15:04:05"))
		}
		if d := ex.Duration(); d > 0 {
			md.WriteString("  ·  " + formatShortDuration(d))
		}
		md.WriteString("\n\n")

		md.WriteString("> " + indentLines(ex.Snippet, "> ") + "\n\n")

		if reply := finalAssistantReply(session, ex.AnchorID); reply != "" {
			md.WriteString(reply + "\n\n")
		}

		stats := []string{}
		if ex.CostUSD > 0 {
			stats = append(stats, fmt.Sprintf("$%.4f", ex.CostUSD))
		}
		if tot := ex.TotalTokens(); tot > 0 {
			stats = append(stats, fmt.Sprintf("%s tok", humanCount(tot)))
		}
		if n := ex.CountSteps(parser.StepSubagent); n > 0 {
			stats = append(stats, fmt.Sprintf("%d subagents", n))
		}
		if n := ex.CountSteps(parser.StepSkill); n > 0 {
			stats = append(stats, fmt.Sprintf("%d skills", n))
		}
		if n := ex.CountSteps(parser.StepToolUse); n > 0 {
			stats = append(stats, fmt.Sprintf("%d tools", n))
		}
		if len(stats) > 0 {
			md.WriteString("_" + strings.Join(stats, " · ") + "_\n\n")
		}
		md.WriteString("---\n\n")
	}

	body := md.String()

	switch strings.ToLower(opts.Format) {
	case "md", "markdown", "":
		return body, nil
	case "org":
		// Lightweight MD→Org conversion: swap ## to * and the rest
		// is close enough for a digest.
		orgBody := strings.ReplaceAll(body, "## ", "** ")
		orgBody = strings.ReplaceAll(orgBody, "# ", "* ")
		return orgBody, nil
	case "html":
		return wrapExchangeHTML(session, body, opts), nil
	default:
		return "", fmt.Errorf("unsupported format for shape=exchange: %s", opts.Format)
	}
}

func wrapExchangeHTML(session *parser.Session, body string, opts ExportOptions) string {
	if opts.Envelope == EnvelopeFragment {
		return `<article class="ccx-digest"><pre>` + escapeHTMLText(body) + `</pre></article>`
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"UTF-8\"><title>Digest: ")
	b.WriteString(escapeHTMLText(truncateID(session.ID, 8)))
	b.WriteString("</title><style>body{font-family:system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 20px;color:#222;line-height:1.55}pre{white-space:pre-wrap;font-family:inherit}</style></head><body><pre>")
	b.WriteString(escapeHTMLText(body))
	b.WriteString("</pre></body></html>")
	return b.String()
}

// finalAssistantReply returns the last assistant text block in the
// exchange anchored at anchorID. Used by shape=exchange.
func finalAssistantReply(session *parser.Session, anchorID string) string {
	msgs := parser.FlattenSessionMessages(session)
	var started bool
	var reply string
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if m.UUID == anchorID {
			started = true
			continue
		}
		if !started {
			continue
		}
		// Stop at the next exchange anchor.
		if (m.Kind == parser.KindUserPrompt || m.Kind == parser.KindCommand) && !m.IsSidechain {
			break
		}
		if m.Kind == parser.KindAssistant && !m.IsSidechain {
			for _, block := range m.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					reply = block.Text
				}
			}
		}
	}
	return reply
}

func indentLines(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatShortDuration(d interface{ Seconds() float64 }) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s%60)
	}
	return fmt.Sprintf("%dh%02dm", m/60, m%60)
}

func escapeHTMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
