package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

func truncateIDOrg(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func exportOrg(session *parser.Session, opts ExportOptions) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("#+TITLE: Session %s\n", truncateIDOrg(session.ID, 8)))
	b.WriteString(fmt.Sprintf("#+DATE: %s\n", session.StartTime.Format("2006-01-02")))
	b.WriteString("#+OPTIONS: toc:2 num:nil\n\n")

	b.WriteString("* Session Metadata\n")
	b.WriteString(fmt.Sprintf("- Started: %s\n", session.StartTime.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf("- Messages: %d\n", session.Stats.MessageCount))
	b.WriteString(fmt.Sprintf("- Tool Calls: %d\n", session.Stats.ToolCalls))
	if session.Stats.Continuations > 0 {
		b.WriteString(fmt.Sprintf("- Continuations: %d\n", session.Stats.Continuations))
	}
	b.WriteString("\n")

	b.WriteString("* Conversation\n\n")

	for _, msg := range session.RootMessages {
		renderOrgMessage(&b, msg, 2, opts)
	}

	return b.String(), nil
}

func renderOrgMessage(b *strings.Builder, msg *parser.Message, level int, opts ExportOptions) {
	if msg.IsSidechain && !opts.IncludeAgents {
		return
	}

	stars := strings.Repeat("*", level)
	ts := msg.Timestamp.Format("[2006-01-02 Mon 15:04]")

	if msg.IsCompacted {
		b.WriteString(fmt.Sprintf("%s ═══ COMPACTED ═══\n", stars))
		for _, block := range msg.Content {
			if block.Type == "text" {
				b.WriteString(block.Text)
				b.WriteString("\n\n")
			}
		}
		return
	}

	switch msg.Type {
	case "user":
		b.WriteString(fmt.Sprintf("%s USER %s\n", stars, ts))
	case "assistant":
		b.WriteString(fmt.Sprintf("%s ASSISTANT %s\n", stars, ts))
	}

	for _, block := range msg.Content {
		renderOrgBlock(b, block, level, opts)
	}

	b.WriteString("\n")

	for _, child := range msg.Children {
		renderOrgMessage(b, child, level+1, opts)
	}
}

func renderOrgBlock(b *strings.Builder, block parser.ContentBlock, level int, opts ExportOptions) {
	stars := strings.Repeat("*", level+1)

	switch block.Type {
	case "text":
		if block.Text != "" {
			b.WriteString(block.Text)
			b.WriteString("\n\n")
		}

	case "thinking":
		if opts.IncludeThinking && block.Text != "" {
			b.WriteString(fmt.Sprintf("%s THINKING\n", stars))
			b.WriteString(":PROPERTIES:\n:VISIBILITY: folded\n:END:\n")
			b.WriteString(block.Text)
			b.WriteString("\n\n")
		}

	case "tool_use":
		b.WriteString(fmt.Sprintf("%s TOOL: %s\n", stars, block.ToolName))
		if block.ToolInput != nil {
			input := formatToolInput(block.ToolInput)
			lang := guessLang(block.ToolName, input)
			b.WriteString(fmt.Sprintf("#+BEGIN_SRC %s\n", lang))
			b.WriteString(input)
			b.WriteString("\n#+END_SRC\n\n")
		}

	case "tool_result":
		label := "RESULT"
		if block.IsError {
			label = "ERROR"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", stars, label))
		if block.ToolResult != nil {
			result := formatToolResult(block.ToolResult)
			b.WriteString("#+BEGIN_EXAMPLE\n")
			b.WriteString(result)
			b.WriteString("\n#+END_EXAMPLE\n\n")
		}

	case "image":
		b.WriteString(fmt.Sprintf("[[data:%s;base64,%s]]\n\n", block.MediaType, block.ImageData[:20]+"..."))
	}
}

func guessLang(toolName, input string) string {
	switch toolName {
	case "Bash":
		return "bash"
	case "Read", "Write", "Edit":
		if strings.Contains(input, "file_path") {
			if strings.Contains(input, ".go") {
				return "go"
			}
			if strings.Contains(input, ".py") {
				return "python"
			}
			if strings.Contains(input, ".js") || strings.Contains(input, ".ts") {
				return "javascript"
			}
			if strings.Contains(input, ".rs") {
				return "rust"
			}
		}
		return ""
	default:
		return ""
	}
}
