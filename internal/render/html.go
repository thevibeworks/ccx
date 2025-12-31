package render

import (
	"fmt"
	"html"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

func truncateID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func exportHTML(session *parser.Session, opts ExportOptions) (string, error) {
	var b strings.Builder

	isDark := opts.Theme == "dark"
	css := htmlCSS(isDark)

	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"UTF-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	b.WriteString(fmt.Sprintf("<title>Session: %s</title>\n", html.EscapeString(truncateID(session.ID, 8))))
	b.WriteString("<style>\n")
	b.WriteString(css)
	b.WriteString("</style>\n")
	b.WriteString("</head>\n<body>\n")

	b.WriteString("<div class=\"container\">\n")
	b.WriteString("<header>\n")
	b.WriteString(fmt.Sprintf("<h1>Session: %s</h1>\n", html.EscapeString(truncateID(session.ID, 8))))
	b.WriteString(fmt.Sprintf("<p class=\"meta\">Started: %s | Messages: %d | Tools: %d</p>\n",
		session.StartTime.Format("2006-01-02 15:04"),
		session.Stats.MessageCount, session.Stats.ToolCalls))
	b.WriteString("</header>\n")

	b.WriteString("<div class=\"messages\">\n")
	for _, msg := range session.RootMessages {
		renderHTMLMessage(&b, msg, 0, opts)
	}
	b.WriteString("</div>\n")

	b.WriteString("</div>\n")

	b.WriteString("<script>\n")
	b.WriteString(htmlJS())
	b.WriteString("</script>\n")
	b.WriteString("</body>\n</html>")

	return b.String(), nil
}

func renderHTMLMessage(b *strings.Builder, msg *parser.Message, depth int, opts ExportOptions) {
	if msg.IsSidechain && !opts.IncludeAgents {
		return
	}

	if msg.IsCompacted {
		b.WriteString("<div class=\"message compacted\">\n")
		b.WriteString("<div class=\"message-header compacted-header\">═══ COMPACTED ═══</div>\n")
		b.WriteString("<div class=\"message-content\">\n")
		for _, block := range msg.Content {
			if block.Type == "text" {
				b.WriteString(fmt.Sprintf("<p class=\"dim\">%s</p>\n", html.EscapeString(block.Text)))
			}
		}
		b.WriteString("</div>\n</div>\n")
		return
	}

	class := "message " + msg.Type
	if msg.IsSidechain {
		class += " sidechain"
	}

	b.WriteString(fmt.Sprintf("<div class=\"%s\">\n", class))
	b.WriteString(fmt.Sprintf("<div class=\"message-header %s-header\">[%s] %s</div>\n",
		msg.Type, strings.ToUpper(msg.Type), msg.Timestamp.Format("15:04:05")))
	b.WriteString("<div class=\"message-content\">\n")

	for _, block := range msg.Content {
		renderHTMLBlock(b, block, opts)
	}

	b.WriteString("</div>\n</div>\n")

	for _, child := range msg.Children {
		renderHTMLMessage(b, child, depth+1, opts)
	}
}

func renderHTMLBlock(b *strings.Builder, block parser.ContentBlock, opts ExportOptions) {
	switch block.Type {
	case "text":
		if block.Text != "" {
			paragraphs := strings.Split(block.Text, "\n\n")
			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p != "" {
					b.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(p)))
				}
			}
		}

	case "thinking":
		if opts.IncludeThinking && block.Text != "" {
			b.WriteString("<details class=\"thinking\">\n")
			b.WriteString("<summary>Thinking</summary>\n")
			b.WriteString(fmt.Sprintf("<div class=\"thinking-content\">%s</div>\n", html.EscapeString(block.Text)))
			b.WriteString("</details>\n")
		}

	case "tool_use":
		b.WriteString("<div class=\"tool-use\">\n")
		b.WriteString(fmt.Sprintf("<div class=\"tool-header\">Tool: %s</div>\n", html.EscapeString(block.ToolName)))
		if block.ToolInput != nil {
			b.WriteString("<pre class=\"tool-input\">")
			b.WriteString(html.EscapeString(formatToolInput(block.ToolInput)))
			b.WriteString("</pre>\n")
		}
		b.WriteString("</div>\n")

	case "tool_result":
		class := "tool-result"
		if block.IsError {
			class += " error"
		}
		b.WriteString(fmt.Sprintf("<div class=\"%s\">\n", class))
		if block.ToolResult != nil {
			b.WriteString("<pre>")
			b.WriteString(html.EscapeString(formatToolResult(block.ToolResult)))
			b.WriteString("</pre>\n")
		}
		b.WriteString("</div>\n")

	case "image":
		if block.ImageData != "" {
			b.WriteString(fmt.Sprintf("<img src=\"data:%s;base64,%s\" class=\"inline-image\">\n",
				html.EscapeString(block.MediaType), html.EscapeString(block.ImageData)))
		}
	}
}

func formatToolInput(input any) string {
	switch v := input.(type) {
	case map[string]any:
		var lines []string
		for key, val := range v {
			lines = append(lines, fmt.Sprintf("%s: %v", key, val))
		}
		return strings.Join(lines, "\n")
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatToolResult(result any) string {
	switch v := result.(type) {
	case string:
		if len(v) > 5000 {
			return v[:4997] + "..."
		}
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func htmlCSS(dark bool) string {
	var bg, text, cardBg, userBg, assistBg, toolBg, border, dim, accent string

	if dark {
		bg = "#1a1a2e"
		text = "#eaeaea"
		cardBg = "#16213e"
		userBg = "#1a365d"
		assistBg = "#1e3a2f"
		toolBg = "#2d1b4e"
		border = "#333"
		dim = "#888"
		accent = "#64b5f6"
	} else {
		bg = "#f5f5f5"
		text = "#212121"
		cardBg = "#ffffff"
		userBg = "#e3f2fd"
		assistBg = "#e8f5e9"
		toolBg = "#f3e5f5"
		border = "#ddd"
		dim = "#666"
		accent = "#1976d2"
	}

	return fmt.Sprintf(`
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: %s;
    color: %s;
    line-height: 1.6;
    padding: 20px;
}
.container { max-width: 900px; margin: 0 auto; }
header { margin-bottom: 24px; padding-bottom: 16px; border-bottom: 2px solid %s; }
header h1 { font-size: 1.5rem; margin-bottom: 8px; }
.meta { color: %s; font-size: 0.9rem; }
.messages { display: flex; flex-direction: column; gap: 16px; }
.message {
    background: %s;
    border-radius: 8px;
    padding: 16px;
    border-left: 4px solid %s;
}
.message.user { background: %s; border-color: %s; }
.message.assistant { background: %s; border-color: #4caf50; }
.message.compacted { background: %s; border-color: #ff9800; }
.message.sidechain { margin-left: 20px; opacity: 0.9; }
.message-header {
    font-weight: 600;
    font-size: 0.85rem;
    margin-bottom: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
}
.user-header { color: %s; }
.assistant-header { color: #4caf50; }
.compacted-header { color: #ff9800; text-align: center; }
.message-content p { margin-bottom: 12px; }
.message-content p:last-child { margin-bottom: 0; }
.dim { color: %s; }
.tool-use {
    background: %s;
    border-radius: 6px;
    padding: 12px;
    margin: 12px 0;
}
.tool-header { font-weight: 600; color: #9c27b0; margin-bottom: 8px; }
.tool-input, .tool-result pre {
    background: rgba(0,0,0,0.2);
    padding: 12px;
    border-radius: 4px;
    overflow-x: auto;
    font-size: 0.85rem;
    white-space: pre-wrap;
    word-wrap: break-word;
}
.tool-result.error { border-left: 3px solid #f44336; }
.thinking { margin: 12px 0; }
.thinking summary {
    cursor: pointer;
    color: #ffc107;
    font-weight: 500;
}
.thinking-content {
    padding: 12px;
    margin-top: 8px;
    background: rgba(255,193,7,0.1);
    border-radius: 4px;
    color: %s;
    font-size: 0.9rem;
}
.inline-image { max-width: 100%%; border-radius: 8px; margin: 12px 0; }
details[open] summary { margin-bottom: 8px; }
pre { font-family: 'SF Mono', Monaco, Consolas, monospace; }
`, bg, text, accent, dim, cardBg, border, userBg, accent, assistBg, cardBg, accent, dim, toolBg, dim)
}

func htmlJS() string {
	return `
document.querySelectorAll('.tool-result pre, .tool-input').forEach(function(el) {
    if (el.scrollHeight > 300) {
        el.style.maxHeight = '300px';
        el.style.overflow = 'auto';
    }
});
`
}
