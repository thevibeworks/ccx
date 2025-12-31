package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

type TerminalOptions struct {
	ShowThinking bool
	ShowAgents   bool
	FlatMode     bool
	Theme        string
}

const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorUser    = "\033[34m" // Blue
	colorAssist  = "\033[32m" // Green
	colorTool    = "\033[35m" // Magenta
	colorThink   = "\033[33m" // Yellow
	colorError   = "\033[31m" // Red
	colorCompact = "\033[36m" // Cyan
)

func Terminal(session *parser.Session, opts TerminalOptions) error {
	fmt.Printf("%s%sSession: %s%s\n", colorBold, colorUser, session.ID, colorReset)
	fmt.Printf("%sStarted: %s | Messages: %d | Tools: %d%s\n\n",
		colorDim, session.StartTime.Format("2006-01-02 15:04"),
		session.Stats.MessageCount, session.Stats.ToolCalls, colorReset)

	printSeparator()

	for _, msg := range session.RootMessages {
		printMessage(msg, 0, opts)
	}

	return nil
}

func printMessage(msg *parser.Message, depth int, opts TerminalOptions) {
	indent := strings.Repeat("  ", depth)

	if msg.IsCompacted {
		printCompacted(msg, indent)
		return
	}

	if msg.IsSidechain && !opts.ShowAgents {
		return
	}

	ts := msg.Timestamp.Format("15:04:05")

	switch msg.Type {
	case "user":
		fmt.Printf("\n%s%s%s[USER] %s%s\n", indent, colorBold, colorUser, ts, colorReset)
	case "assistant":
		fmt.Printf("\n%s%s%s[ASSISTANT] %s%s\n", indent, colorBold, colorAssist, ts, colorReset)
	}

	for _, block := range msg.Content {
		printContentBlock(block, indent, opts)
	}

	if !opts.FlatMode {
		for _, child := range msg.Children {
			printMessage(child, depth+1, opts)
		}
	}
}

func printContentBlock(block parser.ContentBlock, indent string, opts TerminalOptions) {
	switch block.Type {
	case "text":
		if block.Text != "" {
			text := wrapText(block.Text, 80-len(indent))
			for _, line := range strings.Split(text, "\n") {
				fmt.Printf("%s%s\n", indent, line)
			}
		}

	case "thinking":
		if opts.ShowThinking && block.Text != "" {
			fmt.Printf("\n%s%s[THINKING]%s\n", indent, colorThink, colorReset)
			text := wrapText(block.Text, 80-len(indent))
			for _, line := range strings.Split(text, "\n") {
				fmt.Printf("%s%s%s%s\n", indent, colorDim, line, colorReset)
			}
		} else if block.Text != "" {
			fmt.Printf("%s%s[thinking collapsed...]%s\n", indent, colorDim, colorReset)
		}

	case "tool_use":
		fmt.Printf("\n%s%s[TOOL: %s]%s\n", indent, colorTool, block.ToolName, colorReset)
		if block.ToolInput != nil {
			printToolInput(block.ToolInput, indent+"  ")
		}

	case "tool_result":
		label := "[RESULT]"
		color := colorDim
		if block.IsError {
			label = "[ERROR]"
			color = colorError
		}
		fmt.Printf("%s%s%s%s\n", indent, color, label, colorReset)
		if block.ToolResult != nil {
			printToolResult(block.ToolResult, indent+"  ")
		}

	case "image":
		fmt.Printf("%s%s[IMAGE: %s]%s\n", indent, colorDim, block.MediaType, colorReset)
	}
}

func printCompacted(msg *parser.Message, indent string) {
	fmt.Printf("\n%s%s═══ [COMPACTED] ═══%s\n", indent, colorCompact, colorReset)
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			summary := block.Text
			if len(summary) > 200 {
				summary = summary[:197] + "..."
			}
			fmt.Printf("%s%s%s%s\n", indent, colorDim, summary, colorReset)
		}
	}
	fmt.Printf("%s%s═══════════════════%s\n", indent, colorCompact, colorReset)
}

func printToolInput(input any, indent string) {
	switch v := input.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "content" || key == "input" {
				continue
			}
			valStr := fmt.Sprintf("%v", val)
			if len(valStr) > 60 {
				valStr = valStr[:57] + "..."
			}
			fmt.Printf("%s%s: %s\n", indent, key, valStr)
		}
	case string:
		if len(v) > 100 {
			v = v[:97] + "..."
		}
		fmt.Printf("%s%s\n", indent, v)
	}
}

func printToolResult(result any, indent string) {
	switch v := result.(type) {
	case string:
		lines := strings.Split(v, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			for _, line := range lines[:maxLines] {
				if len(line) > 80 {
					line = line[:77] + "..."
				}
				fmt.Printf("%s%s%s%s\n", indent, colorDim, line, colorReset)
			}
			fmt.Printf("%s%s... (%d more lines)%s\n", indent, colorDim, len(lines)-maxLines, colorReset)
		} else {
			for _, line := range lines {
				if len(line) > 80 {
					line = line[:77] + "..."
				}
				fmt.Printf("%s%s%s%s\n", indent, colorDim, line, colorReset)
			}
		}
	default:
		fmt.Printf("%s%s%v%s\n", indent, colorDim, v, colorReset)
	}
}

func printSeparator() {
	fmt.Println(strings.Repeat("─", 60))
}

func wrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}

		lineLen := 0
		for j, word := range words {
			if j > 0 && lineLen+len(word)+1 > width {
				result.WriteString("\n")
				lineLen = 0
			} else if j > 0 {
				result.WriteString(" ")
				lineLen++
			}
			result.WriteString(word)
			lineLen += len(word)
		}
	}

	return result.String()
}
