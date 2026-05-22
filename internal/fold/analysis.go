package fold

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

var mutatingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"Bash":         true,
	"ApplyPatch":   true,
	"Create":       true,
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
	"i said", "not what i",
}

func Analyze(session *parser.Session) *TraceResult {
	if session == nil {
		return &TraceResult{
			Kind:          TraceKind,
			SchemaVersion: TraceSchemaVersion,
			GeneratedAt:   time.Now().UTC(),
		}
	}

	meta := SessionMeta{
		ID:          session.ID,
		FilePath:    session.FilePath,
		Provider:    session.Provider,
		ProjectName: session.ProjectName,
		Summary:     session.Summary,
		Model:       session.Model,
		Start:       session.StartTime,
		End:         session.EndTime,
		CWD:         session.CWD,
		GitBranch:   session.GitBranch,
	}

	allMsgs := parser.FlattenSessionMessages(session)
	sidechainSummaries := collectSidechainEvidence(allMsgs)
	exchanges := segmentExchanges(allMsgs, sidechainSummaries)

	allEdited := make(map[string]struct{})
	allRead := make(map[string]struct{})
	allTools := make(map[string]struct{})
	corrections := 0
	var totalCost float64

	for _, exchange := range exchanges {
		for _, f := range exchange.FilesEdited {
			allEdited[f] = struct{}{}
		}
		for _, f := range exchange.FilesRead {
			allRead[f] = struct{}{}
		}
		for _, tool := range exchange.ToolsUsed {
			allTools[tool] = struct{}{}
		}
		if exchange.HasCorrection {
			corrections++
		}
		totalCost += exchange.CostUSD
	}

	dur := session.EndTime.Sub(session.StartTime).Seconds()
	if dur < 0 {
		dur = 0
	}

	stats := TraceStats{
		ExchangeCount:     len(exchanges),
		CorrectionSignals: corrections,
		FilesEdited:       len(allEdited),
		FilesRead:         len(allRead),
		ToolsUsed:         len(allTools),
		TotalCostUSD:      totalCost,
		DurationSecs:      dur,
		HasSidechains:     session.Stats.AgentSidechains > 0,
	}

	return &TraceResult{
		Kind:          TraceKind,
		SchemaVersion: TraceSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Session:       meta,
		Exchanges:     exchanges,
		Sidechains:    sortedSidechains(sidechainSummaries),
		Stats:         stats,
	}
}

func segmentExchanges(messages []*parser.Message, sidechainSummaries map[string]Sidechain) []ExchangeEvidence {
	type block struct {
		anchor   *parser.Message
		messages []*parser.Message
	}

	var blocks []block
	var current *block

	for _, msg := range messages {
		if msg == nil || msg.IsSidechain {
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

	exchanges := make([]ExchangeEvidence, 0, len(blocks))
	for i, b := range blocks {
		if b.anchor == nil {
			continue
		}
		exchange := buildTurn(i+1, b.anchor, b.messages, sidechainSummaries)
		if exchange.UserText == "" && len(exchange.FilesEdited) == 0 && exchange.AssistantText == "" {
			continue
		}
		exchanges = append(exchanges, exchange)
	}
	return exchanges
}

func buildTurn(index int, anchor *parser.Message, messages []*parser.Message, sidechainSummaries map[string]Sidechain) ExchangeEvidence {
	exchange := ExchangeEvidence{
		Index:    index,
		AnchorID: anchor.UUID,
		Start:    anchor.Timestamp,
		End:      anchor.Timestamp,
	}

	if anchor.IsCommand {
		exchange.IsCommand = true
		exchange.CommandName = anchor.CommandName
	}

	exchange.UserText = firstText(anchor)

	toolSet := make(map[string]struct{})
	editSet := make(map[string]struct{})
	readSet := make(map[string]struct{})
	touchSet := make(map[string]struct{})
	sidechains := make(map[string]Sidechain)

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if !msg.Timestamp.IsZero() && msg.Timestamp.After(exchange.End) {
			exchange.End = msg.Timestamp
		}
		if msg.Usage != nil {
			exchange.InputTokens += msg.Usage.InputTokens
			exchange.OutputTokens += msg.Usage.OutputTokens
			exchange.CostUSD += msg.Usage.CostUSD
		}

		if msg.SubAgentResult != nil && msg.SubAgentResult.AgentID != "" {
			summary := sidechainSummaries[msg.SubAgentResult.AgentID]
			summary.AgentID = msg.SubAgentResult.AgentID
			if summary.AgentType == "" {
				summary.AgentType = msg.SubAgentResult.AgentType
			}
			if summary.Status == "" {
				summary.Status = msg.SubAgentResult.Status
			}
			if summary.Summary == "" {
				summary.Summary = summarizeEvidenceText(firstText(msg))
			}
			if msg.SubAgentResult.TotalToolUseCount > summary.ToolCalls {
				summary.ToolCalls = msg.SubAgentResult.TotalToolUseCount
			}
			sidechains[msg.SubAgentResult.AgentID] = summary
		}

		for _, block := range msg.Content {
			if block.Type == "thinking" {
				exchange.HasThinking = true
			}
			if block.Type != "tool_use" || block.ToolName == "" {
				continue
			}

			toolSet[block.ToolName] = struct{}{}
			paths := extractPaths(block.ToolInput)
			mutatesWorkspace := mutatingTools[block.ToolName] && len(paths) > 0
			for _, path := range paths {
				touchSet[path] = struct{}{}
				if mutatesWorkspace {
					editSet[path] = struct{}{}
				} else if readTools[block.ToolName] {
					readSet[path] = struct{}{}
				}
			}
			exchange.ToolCalls = append(exchange.ToolCalls, ToolCallEvidence{
				MessageID:        msg.UUID,
				ToolID:           block.ToolID,
				Name:             block.ToolName,
				Timestamp:        msg.Timestamp,
				Paths:            paths,
				MutationCapable:  mutatingTools[block.ToolName],
				MutatesWorkspace: mutatesWorkspace,
				Reads:            readTools[block.ToolName],
				IsError:          block.IsError,
			})
		}
	}

	exchange.AssistantText = lastAssistantText(messages)
	exchange.FilesEdited = sortedKeys(editSet)
	exchange.FilesRead = sortedKeys(readSet)
	exchange.FilesTouched = sortedKeys(touchSet)
	exchange.ToolsUsed = sortedKeys(toolSet)
	exchange.Sidechains = sortedSidechains(sidechains)
	exchange.HasCorrection = index > 1 && detectCorrection(exchange.UserText)
	exchange.Signals = buildSignals(exchange)

	return exchange
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

func collectSidechainEvidence(messages []*parser.Message) map[string]Sidechain {
	out := make(map[string]Sidechain)
	for _, msg := range messages {
		if msg == nil || !msg.IsSidechain || msg.AgentID == "" {
			continue
		}
		summary := out[msg.AgentID]
		summary.AgentID = msg.AgentID
		summary.MessageCount++
		summary.TranscriptOmitted = true
		if msg.Usage != nil {
			summary.InputTokens += msg.Usage.InputTokens
			summary.OutputTokens += msg.Usage.OutputTokens
			summary.CostUSD += msg.Usage.CostUSD
		}
		if text := summarizeEvidenceText(firstText(msg)); text != "" {
			summary.Summary = text
		}

		editSet := makeStringSet(summary.FilesEdited)
		readSet := makeStringSet(summary.FilesRead)
		for _, block := range msg.Content {
			if block.Type != "tool_use" || block.ToolName == "" {
				continue
			}
			paths := extractPaths(block.ToolInput)
			mutatesWorkspace := mutatingTools[block.ToolName] && len(paths) > 0
			call := ToolCallEvidence{
				MessageID:        msg.UUID,
				ToolID:           block.ToolID,
				Name:             block.ToolName,
				Timestamp:        msg.Timestamp,
				Paths:            paths,
				MutationCapable:  mutatingTools[block.ToolName],
				MutatesWorkspace: mutatesWorkspace,
				Reads:            readTools[block.ToolName],
				IsError:          block.IsError,
			}
			summary.ToolCallEvidence = append(summary.ToolCallEvidence, call)
			summary.ToolCalls++
			for _, path := range paths {
				if mutatesWorkspace {
					editSet[path] = struct{}{}
				} else if readTools[block.ToolName] {
					readSet[path] = struct{}{}
				}
			}
		}
		summary.FilesEdited = sortedKeys(editSet)
		summary.FilesRead = sortedKeys(readSet)
		out[msg.AgentID] = summary
	}
	return out
}

func extractPaths(toolInput any) []string {
	seen := make(map[string]struct{})
	add := func(path string) {
		path = cleanEvidencePath(path)
		if path != "" {
			seen[path] = struct{}{}
		}
	}
	addFromBase := func(path, baseDir string) {
		path = cleanEvidencePath(path)
		if path == "" {
			return
		}
		if baseDir != "" && !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") && !strings.Contains(path, "$") {
			path = filepath.Join(baseDir, path)
		}
		add(path)
	}

	if raw, ok := toolInput.(string); ok {
		for _, path := range extractPatchPaths(raw) {
			add(path)
		}
		return sortedKeys(seen)
	}

	input, _ := toolInput.(map[string]any)
	if input == nil {
		return nil
	}
	baseDir := toolWorkdir(input)
	for _, key := range []string{"file_path", "notebook_path", "path"} {
		if p, ok := input[key].(string); ok {
			add(p)
		}
	}
	for _, key := range []string{"command", "cmd"} {
		command, ok := input[key].(string)
		if !ok {
			continue
		}
		for _, path := range extractRedirectPaths(command) {
			addFromBase(path, baseDir)
		}
	}
	if argv, ok := input["argv"].([]any); ok && len(argv) > 0 {
		var parts []string
		for _, item := range argv {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		for _, path := range extractRedirectPaths(strings.Join(parts, " ")) {
			addFromBase(path, baseDir)
		}
	}
	return sortedKeys(seen)
}

func toolWorkdir(input map[string]any) string {
	for _, key := range []string{"workdir", "cwd", "working_dir"} {
		if dir, ok := input[key].(string); ok {
			dir = cleanEvidencePath(dir)
			if dir != "" {
				return dir
			}
		}
	}
	return ""
}

func makeStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

var patchPathPattern = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

func extractPatchPaths(patch string) []string {
	var paths []string
	for _, match := range patchPathPattern.FindAllStringSubmatch(patch, -1) {
		if len(match) == 2 {
			paths = append(paths, match[1])
		}
	}
	return paths
}

func extractRedirectPaths(command string) []string {
	var paths []string
	for i := 0; i < len(command); i++ {
		if command[i] != '>' {
			continue
		}
		if i > 0 {
			prev := command[i-1]
			if prev == '2' || prev == '1' {
				continue
			}
		}
		for i+1 < len(command) && command[i+1] == '>' {
			i++
		}
		j := i + 1
		for j < len(command) && (command[j] == ' ' || command[j] == '\t') {
			j++
		}
		if j >= len(command) {
			break
		}
		var path string
		if command[j] == '\'' || command[j] == '"' {
			quote := command[j]
			start := j + 1
			j = start
			for j < len(command) && command[j] != quote {
				j++
			}
			path = command[start:j]
		} else {
			start := j
			for j < len(command) && !strings.ContainsRune(" \t\n;&|", rune(command[j])) {
				j++
			}
			path = command[start:j]
		}
		path = cleanEvidencePath(path)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func cleanEvidencePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	if path == "" || path == "/dev/null" {
		return ""
	}
	return path
}

func summarizeEvidenceText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const maxRunes = 240
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func buildSignals(exchange ExchangeEvidence) []EvidenceSignal {
	var signals []EvidenceSignal
	citation := exchangeCitation(exchange)
	if exchange.HasCorrection {
		signals = append(signals, EvidenceSignal{
			Kind:       "correction",
			Confidence: "medium",
			Summary:    "user text contains a correction or redirect signal",
			Evidence:   []string{citation},
		})
	}
	if len(exchange.FilesEdited) > 0 {
		signals = append(signals, EvidenceSignal{
			Kind:       "mutation",
			Confidence: "high",
			Summary:    "exchange contains mutating tool calls",
			Evidence:   []string{citation},
		})
	}
	if len(exchange.Sidechains) > 0 {
		signals = append(signals, EvidenceSignal{
			Kind:       "sidechain",
			Confidence: "high",
			Summary:    "exchange contains sub-agent result metadata",
			Evidence:   []string{citation},
		})
	}
	if exchange.HasThinking {
		signals = append(signals, EvidenceSignal{
			Kind:       "reasoning",
			Confidence: "medium",
			Summary:    "exchange includes assistant thinking blocks",
			Evidence:   []string{citation},
		})
	}
	return signals
}

func exchangeCitation(exchange ExchangeEvidence) string {
	if exchange.AnchorID == "" {
		return ""
	}
	return "session:#" + exchange.AnchorID
}

func sortedSidechains(m map[string]Sidechain) []Sidechain {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Sidechain, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
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
