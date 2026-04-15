package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

func truncateIDMD(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func exportMarkdown(session *parser.Session, opts ExportOptions) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Session: %s\n\n", truncateIDMD(session.ID, 8)))
	if !session.StartTime.IsZero() {
		b.WriteString(fmt.Sprintf("**Started:** %s\n", session.StartTime.Format("2006-01-02 15:04")))
	}
	b.WriteString(fmt.Sprintf("**Messages:** %d | **Tools:** %d\n\n",
		session.Stats.MessageCount, session.Stats.ToolCalls))
	b.WriteString("---\n\n")

	for _, msg := range session.RootMessages {
		renderMarkdownMessage(&b, msg, opts)
	}

	return b.String(), nil
}

func renderMarkdownMessage(b *strings.Builder, msg *parser.Message, opts ExportOptions) {
	if msg.IsSidechain && !opts.IncludeAgents {
		return
	}

	if msg.IsCompacted {
		b.WriteString("## Context Compacted\n\n")
		for _, block := range msg.Content {
			if block.Type == "text" {
				b.WriteString("> " + strings.ReplaceAll(block.Text, "\n", "\n> ") + "\n\n")
			}
		}
		b.WriteString("---\n\n")
		return
	}

	ts := ""
	if !msg.Timestamp.IsZero() {
		ts = msg.Timestamp.Format("15:04:05")
	}

	switch msg.Type {
	case "user":
		if ts != "" {
			b.WriteString(fmt.Sprintf("## User · %s\n\n", ts))
		} else {
			b.WriteString("## User\n\n")
		}
	case "assistant":
		if ts != "" {
			b.WriteString(fmt.Sprintf("## Assistant · %s\n\n", ts))
		} else {
			b.WriteString("## Assistant\n\n")
		}
	}

	for _, block := range msg.Content {
		renderMarkdownBlock(b, block, msg.Type, opts)
	}

	b.WriteString("---\n\n")

	for _, child := range msg.Children {
		renderMarkdownMessage(b, child, opts)
	}
}

// renderMarkdownBlock emits one content block as markdown.
//
// User text is rendered as a blockquote so stray `#` characters in the
// prompt don't hijack the outline, and so multi-paragraph prompts read
// visually distinct from the assistant's response. Assistant text is
// emitted verbatim because it's meant to be markdown.
//
// Tool input/output is always fenced. The fence length is chosen
// dynamically to be one backtick longer than the longest run of
// backticks inside the content, per CommonMark § 4.5 (fenced code
// blocks). A naive ``` fence breaks when the content itself contains
// ``` — e.g., when a tool printed a markdown snippet.
func renderMarkdownBlock(b *strings.Builder, block parser.ContentBlock, msgType string, opts ExportOptions) {
	switch block.Type {
	case "text":
		if block.Text == "" {
			return
		}
		if msgType == "user" {
			// Quote user prompts so embedded `#` / code fences don't
			// collide with the outer document structure.
			quoted := "> " + strings.ReplaceAll(block.Text, "\n", "\n> ")
			b.WriteString(quoted)
			b.WriteString("\n\n")
		} else {
			b.WriteString(block.Text)
			if !strings.HasSuffix(block.Text, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

	case "thinking":
		if opts.IncludeThinking && block.Text != "" {
			b.WriteString("<details>\n<summary>Thinking</summary>\n\n")
			b.WriteString(block.Text)
			if !strings.HasSuffix(block.Text, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n</details>\n\n")
		}

	case "tool_use":
		b.WriteString(fmt.Sprintf("### Tool: %s\n\n", escapeInlineMD(block.ToolName)))
		if block.ToolInput != nil {
			input := formatToolInput(block.ToolInput)
			if strings.Contains(input, "\n") || len(input) > 80 {
				fence := longestBacktickRun(input)
				b.WriteString(fence + "\n")
				b.WriteString(input)
				if !strings.HasSuffix(input, "\n") {
					b.WriteString("\n")
				}
				b.WriteString(fence + "\n\n")
			} else {
				// Inline code — use single or double backticks depending
				// on whether the input itself contains backticks.
				if strings.ContainsRune(input, '`') {
					b.WriteString("`` ")
					b.WriteString(input)
					b.WriteString(" ``\n\n")
				} else {
					b.WriteString(fmt.Sprintf("`%s`\n\n", input))
				}
			}
		}

	case "tool_result":
		label := "### Result"
		if block.IsError {
			label = "### Error"
		}
		b.WriteString(label + "\n\n")
		if block.ToolResult != nil {
			result := formatToolResult(block.ToolResult)
			fence := longestBacktickRun(result)
			b.WriteString(fence + "\n")
			b.WriteString(result)
			if !strings.HasSuffix(result, "\n") {
				b.WriteString("\n")
			}
			b.WriteString(fence + "\n\n")
		}

	case "image":
		b.WriteString(fmt.Sprintf("![Image](%s)\n\n", block.MediaType))
	}
}

// longestBacktickRun returns a code fence that is guaranteed to be
// longer than any backtick run inside s. Minimum length is 3. This
// matches CommonMark's fenced code block rule: the opening fence must
// be at least 3 backticks and must be longer than any ``` sequence
// inside the code it encloses.
func longestBacktickRun(s string) string {
	max := 0
	cur := 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 0
		}
	}
	n := max + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}

// escapeInlineMD escapes the small set of markdown characters that
// can break inline rendering when a tool name contains them. We don't
// try to full-escape every possible markdown construct — tool names
// are short and well-behaved in practice.
func escapeInlineMD(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
	)
	return r.Replace(s)
}
