package render

import (
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

var delegationTools = map[string]bool{
	"Agent":       true,
	"Task":        true,
	"Dispatch":    true,
	"SubAgent":    true,
	"TodoRead":    true,
	"close_agent": true,
}

func BriefSession(session *parser.Session) *parser.Session {
	result := *session
	result.RootMessages = briefMessages(session.RootMessages)
	return &result
}

func briefMessages(messages []*parser.Message) []*parser.Message {
	var result []*parser.Message
	for _, m := range messages {
		filtered := briefMessage(m)
		if filtered != nil {
			result = append(result, filtered)
		}
	}
	return result
}

func briefMessage(m *parser.Message) *parser.Message {
	if m.IsCompacted || m.Kind == parser.KindCompactSummary {
		return m
	}

	if m.IsMeta || m.IsCommand || m.Kind == parser.KindMeta || m.Kind == parser.KindSystem {
		return nil
	}

	if m.Kind == parser.KindToolResult {
		return briefToolResult(m)
	}

	if m.IsSidechain {
		return briefSidechain(m)
	}

	blocks := textBlocks(m.Content)
	if len(blocks) == 0 {
		return nil
	}

	result := *m
	result.Content = blocks
	result.Children = briefMessages(m.Children)
	return &result
}

func briefToolResult(m *parser.Message) *parser.Message {
	for _, b := range m.Content {
		if b.Type != "tool_result" || !delegationTools[b.ToolName] {
			continue
		}
		summary := extractResultText(b.ToolResult)
		if summary == "" {
			continue
		}
		return &parser.Message{
			UUID:      m.UUID,
			Type:      "assistant",
			Kind:      parser.KindAssistant,
			Timestamp: m.Timestamp,
			Model:     m.Model,
			Content:   []parser.ContentBlock{{Type: "text", Text: summary}},
		}
	}
	return nil
}

func briefSidechain(m *parser.Message) *parser.Message {
	all := flattenTree(m)
	for i := len(all) - 1; i >= 0; i-- {
		msg := all[i]
		if msg.Type != "assistant" || msg.Kind != parser.KindAssistant {
			continue
		}
		blocks := textBlocks(msg.Content)
		if len(blocks) == 0 {
			continue
		}
		result := *msg
		result.Content = blocks
		result.Children = nil
		result.IsSidechain = false
		return &result
	}
	return nil
}

func textBlocks(blocks []parser.ContentBlock) []parser.ContentBlock {
	var result []parser.ContentBlock
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			result = append(result, b)
		}
	}
	return result
}

func extractResultText(result any) string {
	switch v := result.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"result", "output", "summary", "text", "message"} {
			if val, ok := v[key]; ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func flattenTree(root *parser.Message) []*parser.Message {
	var result []*parser.Message
	result = append(result, root)
	for _, child := range root.Children {
		result = append(result, flattenTree(child)...)
	}
	return result
}
