package fold

import (
	"sort"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

var mutatingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"Bash":         true,
	"ApplyPatch":   true,
}

var readTools = map[string]bool{
	"Read": true,
	"Glob": true,
	"Grep": true,
}

var correctionSignals = []string{
	"no,", "no ", "don't", "dont", "do not",
	"actually,", "actually ", "wait,", "wait ",
	"not that", "revert", "undo", "wrong",
	"stop", "cancel", "instead,", "instead ",
	"I said", "i said", "not what I",
}

func Analyze(session *parser.Session) *FoldResult {
	if session == nil {
		return &FoldResult{}
	}

	meta := SessionMeta{
		ID:        session.ID,
		Summary:   session.Summary,
		Model:     session.Model,
		Start:     session.StartTime,
		End:       session.EndTime,
		CWD:       session.CWD,
		GitBranch: session.GitBranch,
	}

	allMsgs := parser.FlattenSessionMessages(session)
	turns := segmentTurns(allMsgs)

	allEdited := make(map[string]struct{})
	corrections := 0
	hasSidechains := false
	var totalCost float64

	for _, t := range turns {
		for _, f := range t.FilesEdited {
			allEdited[f] = struct{}{}
		}
		if t.HasCorrection {
			corrections++
		}
		if t.Sidechain != nil {
			hasSidechains = true
		}
		totalCost += t.CostUSD
	}

	dur := session.EndTime.Sub(session.StartTime).Seconds()
	if dur < 0 {
		dur = 0
	}

	stats := FoldStats{
		TurnCount:     len(turns),
		Corrections:   corrections,
		FilesEdited:   len(allEdited),
		TotalCostUSD:  totalCost,
		DurationSecs:  dur,
		HasSidechains: hasSidechains,
	}

	return &FoldResult{
		Session: meta,
		Turns:   turns,
		Stats:   stats,
	}
}

func segmentTurns(messages []*parser.Message) []Turn {
	type block struct {
		anchor   *parser.Message
		messages []*parser.Message
	}

	var blocks []block
	var current *block

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Kind == parser.KindCompactSummary {
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			continue
		}
		if msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &block{anchor: msg}
			continue
		}
		if current != nil {
			current.messages = append(current.messages, msg)
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}

	turns := make([]Turn, 0, len(blocks))
	for i, b := range blocks {
		if b.anchor == nil {
			continue
		}
		t := buildTurn(i+1, b.anchor, b.messages)
		if t.UserText == "" && len(t.FilesEdited) == 0 && t.AssistantText == "" {
			continue
		}
		turns = append(turns, t)
	}
	return turns
}

func buildTurn(index int, anchor *parser.Message, messages []*parser.Message) Turn {
	t := Turn{
		Index:    index,
		AnchorID: anchor.UUID,
		Start:    anchor.Timestamp,
		End:      anchor.Timestamp,
	}

	if anchor.IsCommand {
		t.IsCommand = true
		t.CommandName = anchor.CommandName
	}

	t.UserText = firstText(anchor)

	toolSet := make(map[string]struct{})
	editSet := make(map[string]struct{})
	readSet := make(map[string]struct{})

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if !msg.Timestamp.IsZero() && msg.Timestamp.After(t.End) {
			t.End = msg.Timestamp
		}
		if msg.Usage != nil {
			t.InputTokens += msg.Usage.InputTokens
			t.OutputTokens += msg.Usage.OutputTokens
			t.CostUSD += msg.Usage.CostUSD
		}

		for _, block := range msg.Content {
			if block.Type == "thinking" {
				t.HasThinking = true
			}
			if block.Type == "tool_use" && block.ToolName != "" {
				toolSet[block.ToolName] = struct{}{}
				path := extractPath(block.ToolInput)
				if path != "" {
					if mutatingTools[block.ToolName] {
						editSet[path] = struct{}{}
					} else if readTools[block.ToolName] {
						readSet[path] = struct{}{}
					}
				}
			}
		}

		if msg.IsSidechain && msg.AgentID != "" && t.Sidechain == nil {
			t.Sidechain = &Sidechain{
				AgentID: msg.AgentID,
				Summary: lastAssistantInSidechain(msg),
			}
		}
	}

	t.AssistantText = lastAssistantText(messages)
	t.FilesEdited = sortedKeys(editSet)
	t.FilesRead = sortedKeys(readSet)
	t.ToolsUsed = sortedKeys(toolSet)
	t.HasCorrection = detectCorrection(t.UserText)

	return t
}

func detectCorrection(userText string) bool {
	if userText == "" {
		return false
	}
	lower := strings.ToLower(userText)
	for _, signal := range correctionSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

func firstText(msg *parser.Message) string {
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

func lastAssistantText(messages []*parser.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m == nil || m.Kind != parser.KindAssistant || m.IsSidechain {
			continue
		}
		text := firstText(m)
		if text != "" {
			return text
		}
	}
	return ""
}

func lastAssistantInSidechain(root *parser.Message) string {
	if root == nil {
		return ""
	}
	var walk func([]*parser.Message) string
	walk = func(msgs []*parser.Message) string {
		for i := len(msgs) - 1; i >= 0; i-- {
			if result := walk(msgs[i].Children); result != "" {
				return result
			}
			if msgs[i].Kind == parser.KindAssistant {
				if t := firstText(msgs[i]); t != "" {
					return t
				}
			}
		}
		return ""
	}
	return walk(root.Children)
}

func extractPath(toolInput any) string {
	input, _ := toolInput.(map[string]any)
	if input == nil {
		return ""
	}
	for _, key := range []string{"file_path", "notebook_path", "path"} {
		if p, ok := input[key].(string); ok && p != "" {
			return p
		}
	}
	return ""
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
