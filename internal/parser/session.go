package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

func ParseSession(filePath string) (*Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []*Message
	var summaryFromFile string
	var totalInputTokens, totalOutputTokens, totalCacheRead, totalCacheCreate int
	var sessionSlug, sessionVersion, sessionBranch, sessionCWD string
	logicalParents := make(map[string]string) // compact_boundary UUID -> logical parent UUID
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	lineNum := 0
	parseErrors := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw rawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			parseErrors++
			continue
		}

		// Accumulate usage stats from any message with usage data
		if raw.Usage != nil {
			totalInputTokens += raw.Usage.InputTokens
			totalOutputTokens += raw.Usage.OutputTokens
			totalCacheRead += raw.Usage.CacheReadInputTokens
			totalCacheCreate += raw.Usage.CacheCreationInputTokens
		}

		// Extract session metadata from first message that has it
		if sessionSlug == "" && raw.Slug != "" {
			sessionSlug = raw.Slug
		}
		if sessionVersion == "" && raw.Version != "" {
			sessionVersion = raw.Version
		}
		if sessionBranch == "" && raw.GitBranch != "" {
			sessionBranch = raw.GitBranch
		}
		if sessionCWD == "" && raw.CWD != "" {
			sessionCWD = raw.CWD
		}

		if raw.Type == "system" && raw.Subtype == "compact_boundary" && raw.UUID != "" {
			if parent := strings.TrimSpace(raw.LogicalParentUUID); parent != "" {
				logicalParents[raw.UUID] = parent
			}
			continue
		}

		if raw.Type == "summary" && summaryFromFile == "" {
			if s := strings.TrimSpace(raw.Summary); s != "" {
				summaryFromFile = s
			}
			continue
		}

		if raw.Type != "user" && raw.Type != "assistant" {
			continue
		}

		msg := parseMessage(raw)
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for _, msg := range messages {
		msg.ParentUUID = resolveLogicalParent(msg.ParentUUID, logicalParents)
	}

	rootMessages := buildMessageTree(messages)
	stats := computeStats(messages)

	// Add token usage stats
	stats.InputTokens = totalInputTokens
	stats.OutputTokens = totalOutputTokens
	stats.CacheReadTokens = totalCacheRead
	stats.CacheCreateTokens = totalCacheCreate

	var startTime, endTime time.Time
	if len(messages) > 0 {
		startTime = messages[0].Timestamp
		endTime = messages[len(messages)-1].Timestamp
		stats.DurationSeconds = endTime.Sub(startTime).Seconds()
	}

	summary := summaryFromFile
	if summary == "" {
		summary = extractSummary(messages)
	}

	session := &Session{
		ID:           extractSessionID(filePath),
		FilePath:     filePath,
		Summary:      summary,
		StartTime:    startTime,
		EndTime:      endTime,
		RootMessages: rootMessages,
		Stats:        stats,
		Slug:         sessionSlug,
		Version:      sessionVersion,
		GitBranch:    sessionBranch,
		CWD:          sessionCWD,
	}

	return session, nil
}

func resolveLogicalParent(parentUUID string, logicalParents map[string]string) string {
	const maxHops = 16

	cur := parentUUID
	for i := 0; i < maxHops; i++ {
		next, ok := logicalParents[cur]
		if !ok || next == "" || next == cur {
			return cur
		}
		cur = next
	}
	return cur
}

func parseMessage(raw rawMessage) *Message {
	ts, _ := time.Parse(time.RFC3339Nano, raw.Timestamp)

	msg := &Message{
		UUID:        raw.UUID,
		ParentUUID:  raw.ParentUUID,
		Type:        raw.Type,
		Timestamp:   ts,
		IsCompacted: raw.IsCompactSummary,
		IsSidechain: raw.IsSidechain,
		IsMeta:      raw.IsMeta,
		AgentID:     raw.AgentID,
		Model:       raw.Message.Model,
		Subtype:     raw.Subtype,
		raw:         raw,
	}

	msg.Content = parseContent(raw.Message.Content)
	msg.Kind = classifyMessage(msg, raw)

	return msg
}

// classifyMessage determines the semantic kind of a message
func classifyMessage(msg *Message, raw rawMessage) MessageKind {
	switch msg.Type {
	case "assistant":
		return KindAssistant

	case "system":
		return KindSystem

	case "user":
		// Check for compact summary
		if raw.IsCompactSummary {
			return KindCompactSummary
		}

		// Check for meta message (system instructions)
		if raw.IsMeta {
			return KindMeta
		}

		// Check for tool results
		if len(msg.Content) > 0 && msg.Content[0].Type == "tool_result" {
			return KindToolResult
		}

		// Check for command message
		if len(msg.Content) > 0 && msg.Content[0].Type == "text" {
			text := msg.Content[0].Text
			if strings.HasPrefix(text, "<command-") {
				msg.IsCommand = true
				msg.CommandName = extractCommandName(text)
				msg.CommandArgs = extractCommandArgs(text)
				return KindCommand
			}
		}

		// Check raw content for string commands
		if str, ok := raw.Message.Content.(string); ok {
			if strings.HasPrefix(str, "<command-") {
				msg.IsCommand = true
				msg.CommandName = extractCommandName(str)
				msg.CommandArgs = extractCommandArgs(str)
				return KindCommand
			}
		}

		// Actual user prompt
		return KindUserPrompt
	}

	return KindUnknown
}

// extractCommandName extracts command name from <command-name>/foo</command-name>
func extractCommandName(text string) string {
	start := strings.Index(text, "<command-name>")
	if start == -1 {
		return ""
	}
	start += len("<command-name>")
	end := strings.Index(text[start:], "</command-name>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(text[start : start+end])
}

// extractCommandArgs extracts command args from <command-args>...</command-args>
func extractCommandArgs(text string) string {
	start := strings.Index(text, "<command-args>")
	if start == -1 {
		return ""
	}
	start += len("<command-args>")
	end := strings.Index(text[start:], "</command-args>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(text[start : start+end])
}

func parseContent(content any) []ContentBlock {
	if content == nil {
		return nil
	}

	if str, ok := content.(string); ok {
		return []ContentBlock{{Type: "text", Text: str}}
	}

	arr, ok := content.([]any)
	if !ok {
		return nil
	}

	var blocks []ContentBlock
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		block := ContentBlock{}
		if t, ok := m["type"].(string); ok {
			block.Type = t
		}

		switch block.Type {
		case "text":
			if text, ok := m["text"].(string); ok {
				block.Text = text
			}
		case "thinking":
			if thinking, ok := m["thinking"].(string); ok {
				block.Text = thinking
			}
		case "tool_use":
			if name, ok := m["name"].(string); ok {
				block.ToolName = name
			}
			if id, ok := m["id"].(string); ok {
				block.ToolID = id
			}
			block.ToolInput = m["input"]
		case "tool_result":
			block.ToolResult = m["content"]
			if isErr, ok := m["is_error"].(bool); ok {
				block.IsError = isErr
			}
			if id, ok := m["tool_use_id"].(string); ok {
				block.ToolID = id
			}
		case "image":
			if source, ok := m["source"].(map[string]any); ok {
				if mt, ok := source["media_type"].(string); ok {
					block.MediaType = mt
				}
				if data, ok := source["data"].(string); ok {
					block.ImageData = data
				}
			}
		}

		blocks = append(blocks, block)
	}

	return blocks
}

func buildMessageTree(messages []*Message) []*Message {
	byUUID := make(map[string]*Message)
	for _, msg := range messages {
		if msg.UUID != "" {
			byUUID[msg.UUID] = msg
		}
	}

	var roots []*Message
	for _, msg := range messages {
		if msg.ParentUUID == "" {
			roots = append(roots, msg)
		} else if parent, ok := byUUID[msg.ParentUUID]; ok {
			parent.Children = append(parent.Children, msg)
		} else {
			roots = append(roots, msg)
		}
	}

	return roots
}

func computeStats(messages []*Message) SessionStats {
	var stats SessionStats

	for _, msg := range messages {
		// Count meaningful conversation turns:
		// - Actual user prompts (not commands, meta, tool results, compact summaries)
		// - Assistant responses
		switch msg.Kind {
		case KindUserPrompt:
			stats.MessageCount++
			stats.UserPrompts++
		case KindAssistant:
			stats.MessageCount++
		}
		if msg.IsCompacted {
			stats.Continuations++
		}
		if msg.IsSidechain {
			stats.AgentSidechains++
		}
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				stats.ToolCalls++
			}
		}
	}

	return stats
}

func extractSummary(messages []*Message) string {
	// Look for actual user prompts only
	for _, msg := range messages {
		if msg.Kind == KindUserPrompt {
			for _, block := range msg.Content {
				if block.Type == "text" && block.Text != "" {
					text := strings.TrimSpace(block.Text)
					// Return first line only (no truncation)
					if idx := strings.Index(text, "\n"); idx > 0 {
						return text[:idx]
					}
					return text
				}
			}
		}
	}
	return "(no summary)"
}

func extractSessionID(filePath string) string {
	base := strings.TrimSuffix(filePath, ".jsonl")
	parts := strings.Split(base, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return base
}

// SessionMeta holds quick-parsed session metadata
type SessionMeta struct {
	Slug      string
	Version   string
	GitBranch string
	CWD       string
}

func quickParseSession(filePath string) (summary string, startTime, endTime time.Time, stats SessionStats, meta SessionMeta) {
	file, err := os.Open(filePath)
	if err != nil {
		return "(no summary)", time.Time{}, time.Time{}, SessionStats{}, SessionMeta{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var firstTime, lastTime time.Time
	foundSummary := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw struct {
			Type             string `json:"type"`
			Timestamp        string `json:"timestamp"`
			IsCompactSummary bool   `json:"isCompactSummary"`
			IsSidechain      bool   `json:"isSidechain"`
			IsMeta           bool   `json:"isMeta"`
			Summary          string `json:"summary"`
			Slug             string `json:"slug"`
			Version          string `json:"version"`
			GitBranch        string `json:"gitBranch"`
			CWD              string `json:"cwd"`
			Message          struct {
				Content any `json:"content"`
			} `json:"message"`
		}

		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		// Extract metadata from first message that has it
		if meta.Slug == "" && raw.Slug != "" {
			meta.Slug = raw.Slug
		}
		if meta.Version == "" && raw.Version != "" {
			meta.Version = raw.Version
		}
		if meta.GitBranch == "" && raw.GitBranch != "" {
			meta.GitBranch = raw.GitBranch
		}
		if meta.CWD == "" && raw.CWD != "" {
			meta.CWD = raw.CWD
		}

		if ts, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
			if firstTime.IsZero() {
				firstTime = ts
			}
			lastTime = ts
		}

		// Count only meaningful conversation turns (matching computeStats logic)
		// User: only actual prompts (not compact summaries, meta, or tool results)
		if raw.Type == "user" && !raw.IsCompactSummary && !raw.IsMeta {
			// Check if it's a tool result
			isToolResult := false
			if arr, ok := raw.Message.Content.([]any); ok && len(arr) > 0 {
				if m, ok := arr[0].(map[string]any); ok {
					if t, ok := m["type"].(string); ok && t == "tool_result" {
						isToolResult = true
					}
				}
			}
			if !isToolResult {
				stats.MessageCount++
				stats.UserPrompts++
			}
		} else if raw.Type == "assistant" {
			stats.MessageCount++
		}
		if raw.IsCompactSummary {
			stats.Continuations++
		}
		if raw.IsSidechain {
			stats.AgentSidechains++
		}
		if raw.Type == "user" || raw.Type == "assistant" {
			stats.ToolCalls += countToolCalls(raw.Message.Content)
		}

		if raw.Type == "summary" && raw.Summary != "" {
			summary = raw.Summary
			foundSummary = true
			continue
		}

		if !foundSummary && raw.Type == "user" && !raw.IsCompactSummary {
			text := extractTextFromContent(raw.Message.Content)
			if text != "" && !strings.HasPrefix(text, "<") {
				// First line only (no truncation)
				if idx := strings.Index(text, "\n"); idx > 0 {
					text = text[:idx]
				}
				summary = text
				foundSummary = true
			}
		}
	}

	if summary == "" {
		summary = "(no summary)"
	}

	return summary, firstTime, lastTime, stats, meta
}

func countToolCalls(content any) int {
	arr, ok := content.([]any)
	if !ok {
		return 0
	}
	count := 0
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if t, ok := m["type"].(string); ok && t == "tool_use" {
				count++
			}
		}
	}
	return count
}

func extractTextFromContent(content any) string {
	if str, ok := content.(string); ok {
		return strings.TrimSpace(str)
	}

	arr, ok := content.([]any)
	if !ok {
		return ""
	}

	var texts []string
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if t, ok := m["type"].(string); ok && t == "text" {
				if text, ok := m["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
	}

	return strings.TrimSpace(strings.Join(texts, " "))
}
