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
	b.WriteString(fmt.Sprintf("**Started:** %s\n", session.StartTime.Format("2006-01-02 15:04")))
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
		b.WriteString("## ═══ Context Compacted ═══\n\n")
		for _, block := range msg.Content {
			if block.Type == "text" {
				b.WriteString(fmt.Sprintf("*%s*\n\n", block.Text))
			}
		}
		b.WriteString("---\n\n")
		return
	}

	ts := msg.Timestamp.Format("15:04:05")

	switch msg.Type {
	case "user":
		b.WriteString(fmt.Sprintf("## User (%s)\n\n", ts))
	case "assistant":
		b.WriteString(fmt.Sprintf("## Assistant (%s)\n\n", ts))
	}

	for _, block := range msg.Content {
		renderMarkdownBlock(b, block, opts)
	}

	b.WriteString("---\n\n")

	for _, child := range msg.Children {
		renderMarkdownMessage(b, child, opts)
	}
}

func renderMarkdownBlock(b *strings.Builder, block parser.ContentBlock, opts ExportOptions) {
	switch block.Type {
	case "text":
		if block.Text != "" {
			b.WriteString(block.Text)
			b.WriteString("\n\n")
		}

	case "thinking":
		if opts.IncludeThinking && block.Text != "" {
			b.WriteString("<details>\n<summary>Thinking</summary>\n\n")
			b.WriteString(block.Text)
			b.WriteString("\n\n</details>\n\n")
		}

	case "tool_use":
		b.WriteString(fmt.Sprintf("### Tool: %s\n\n", block.ToolName))
		if block.ToolInput != nil {
			input := formatToolInput(block.ToolInput)
			if strings.Contains(input, "\n") || len(input) > 80 {
				b.WriteString("```\n")
				b.WriteString(input)
				b.WriteString("\n```\n\n")
			} else {
				b.WriteString(fmt.Sprintf("`%s`\n\n", input))
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
			b.WriteString("```\n")
			b.WriteString(result)
			b.WriteString("\n```\n\n")
		}

	case "image":
		b.WriteString(fmt.Sprintf("![Image](%s)\n\n", block.MediaType))
	}
}
