package trace

import (
	"fmt"
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

// activeGapCap bounds what one inter-message gap can contribute to
// active time. Wall-span lies about long sessions (a 35-day span can
// hold 4 days of work; one autonomous turn can bleed into an overnight
// gap): summing gaps capped at this value keeps continuous work
// counted fully while an idle stretch counts as at most one cap.
const activeGapCap = 5 * time.Minute

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
	turns := segmentTurns(allMsgs, sidechainSummaries)
	supersededCount := markSupersededTurns(turns, detectSupersededAnchors(allMsgs))

	allEdited := make(map[string]struct{})
	allRead := make(map[string]struct{})
	allTools := make(map[string]struct{})
	stepCount := 0
	toolErrors := 0
	interrupts := 0
	denials := 0
	var mainCost float64
	var activeSecs float64
	var inputTok, outputTok, cacheReadTok, cacheCreateTok, reasoningTok int

	for _, turn := range turns {
		for _, f := range turn.FilesEdited {
			allEdited[f] = struct{}{}
		}
		for _, f := range turn.FilesRead {
			allRead[f] = struct{}{}
		}
		for tool := range turn.ToolCounts {
			allTools[tool] = struct{}{}
		}
		stepCount += len(turn.Steps)
		toolErrors += turn.Errors
		interrupts += turn.Interrupts
		denials += turn.Denials
		mainCost += turn.CostUSD
		activeSecs += turn.ActiveSecs
		inputTok += turn.InputTokens
		outputTok += turn.OutputTokens
		cacheReadTok += turn.CacheReadTokens
		cacheCreateTok += turn.CacheCreateTokens
		reasoningTok += turn.ReasoningTokens
	}

	// Agent spend comes from the authoritative sidechain map, not the
	// per-turn attachments: a sidechain whose result never landed in a
	// turn still spent real money.
	var agentsCost float64
	for _, sc := range sidechainSummaries {
		agentsCost += sc.CostUSD
	}

	dur := session.EndTime.Sub(session.StartTime).Seconds()
	if dur < 0 {
		dur = 0
	}

	stats := TraceStats{
		TurnCount:         len(turns) - supersededCount,
		SupersededTurns:   supersededCount,
		StepCount:         stepCount,
		FilesEdited:       len(allEdited),
		FilesRead:         len(allRead),
		ToolsUsed:         len(allTools),
		ToolErrors:        toolErrors,
		Interrupts:        interrupts,
		Denials:           denials,
		InputTokens:       inputTok,
		OutputTokens:      outputTok,
		CacheReadTokens:   cacheReadTok,
		CacheCreateTokens: cacheCreateTok,
		ReasoningTokens:   reasoningTok,
		TotalCostUSD:      mainCost + agentsCost,
		AgentsCostUSD:     agentsCost,
		DurationSecs:      dur,
		ActiveSecs:        activeSecs,
		HasSidechains:     session.Stats.AgentSidechains > 0,
	}

	return &TraceResult{
		Kind:          TraceKind,
		SchemaVersion: TraceSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Session:       meta,
		Turns:         turns,
		Sidechains:    sortedSidechains(sidechainSummaries),
		Stats:         stats,
	}
}

func segmentTurns(messages []*parser.Message, sidechainSummaries map[string]Sidechain) []Turn {
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

	turns := make([]Turn, 0, len(blocks))
	for i, b := range blocks {
		if b.anchor == nil {
			continue
		}
		turn := buildTurn(i+1, b.anchor, b.messages, sidechainSummaries)
		if turn.UserText == "" && len(turn.Steps) == 0 {
			continue
		}
		turns = append(turns, turn)
	}
	return turns
}

// buildTurn segments one turn into say-then-do steps: every assistant
// narration block opens a step; tool activity attaches to the step it
// followed; tool-result errors are attributed back to the step that
// issued the call.
func buildTurn(index int, anchor *parser.Message, messages []*parser.Message, sidechainSummaries map[string]Sidechain) Turn {
	turn := Turn{
		Index:    index,
		AnchorID: anchor.UUID,
		Start:    anchor.Timestamp,
		End:      anchor.Timestamp,
	}

	if anchor.IsCommand {
		turn.IsCommand = true
		turn.CommandName = anchor.CommandName
	}

	turn.UserText, turn.UserTextTruncated = cleanBoundedText(firstText(anchor), maxUserTextRunes)

	editSet := make(map[string]struct{})
	readSet := make(map[string]struct{})

	var steps []Step
	stepByToolID := make(map[string]int)              // ToolID -> index into steps
	callByToolID := make(map[string]ToolCallEvidence) // every call, for late error attribution
	mutIdxByToolID := make(map[string][2]int)         // ToolID -> (step index, index into Mutations)

	currentStep := func() *Step {
		if len(steps) == 0 {
			steps = append(steps, Step{Index: 1})
		}
		return &steps[len(steps)-1]
	}

	var active time.Duration
	prev := anchor.Timestamp

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if !msg.Timestamp.IsZero() {
			if !prev.IsZero() {
				if gap := msg.Timestamp.Sub(prev); gap > 0 {
					active += min(gap, activeGapCap)
				}
			}
			prev = msg.Timestamp
		}
		if !msg.Timestamp.IsZero() && msg.Timestamp.After(turn.End) {
			turn.End = msg.Timestamp
		}
		if msg.Usage != nil {
			turn.InputTokens += msg.Usage.InputTokens
			turn.OutputTokens += msg.Usage.OutputTokens
			turn.CacheReadTokens += msg.Usage.CacheReadTokens
			turn.CacheCreateTokens += msg.Usage.CacheCreateTokens
			turn.ReasoningTokens += msg.Usage.ReasoningTokens
			turn.CostUSD += msg.Usage.CostUSD
		}

		if msg.Kind == parser.KindInterrupt {
			turn.Interrupts++
			if len(steps) > 0 {
				steps[len(steps)-1].Interrupts++
			}
		}

		if msg.Kind == parser.KindAssistant {
			if narration := firstText(msg); narration != "" {
				step := Step{
					Index:     len(steps) + 1,
					MessageID: msg.UUID,
					Timestamp: msg.Timestamp,
				}
				step.Narration, step.NarrationTruncated = cleanBoundedText(narration, maxNarrationRunes)
				steps = append(steps, step)
			}
			if msg.Usage != nil && len(steps) > 0 {
				steps[len(steps)-1].CostUSD += msg.Usage.CostUSD
			}
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
				summary.Summary = firstText(msg)
			}
			if msg.SubAgentResult.TotalToolUseCount > summary.ToolCalls {
				summary.ToolCalls = msg.SubAgentResult.TotalToolUseCount
			}
			// Light reference on the step; full evidence lives once in
			// the top-level sidechains list, keyed by agent_id.
			summary.Summary = summarizeEvidenceText(summary.Summary)
			summary.ToolCallEvidence = nil
			summary.FilesEdited = nil
			summary.FilesRead = nil
			if step := stepForResult(msg, steps, stepByToolID); step != nil {
				step.Sidechains = append(step.Sidechains, summary)
				turn.AgentsCostUSD += summary.CostUSD
			}
		}

		for _, cb := range msg.Content {
			switch cb.Type {
			case "tool_use":
				if cb.ToolName == "" {
					continue
				}
				step := currentStep()
				if step.ToolCounts == nil {
					step.ToolCounts = make(map[string]int)
				}
				step.ToolCounts[cb.ToolName]++
				if cb.ToolID != "" {
					stepByToolID[cb.ToolID] = len(steps) - 1
				}

				paths := extractPaths(cb.ToolInput)
				mutatesWorkspace := mutatingTools[cb.ToolName] && len(paths) > 0
				for _, path := range paths {
					if mutatesWorkspace {
						editSet[path] = struct{}{}
					} else if readTools[cb.ToolName] {
						readSet[path] = struct{}{}
					}
				}
				evidence := ToolCallEvidence{
					MessageID:        msg.UUID,
					ToolID:           cb.ToolID,
					Name:             cb.ToolName,
					Timestamp:        msg.Timestamp,
					Summary:          commandSummary(cb.ToolInput),
					Paths:            paths,
					MutationCapable:  mutatingTools[cb.ToolName],
					MutatesWorkspace: mutatesWorkspace,
					Reads:            readTools[cb.ToolName],
					IsError:          cb.IsError,
				}
				if cb.ToolID != "" {
					callByToolID[cb.ToolID] = evidence
				}
				if !mutatingTools[cb.ToolName] && !cb.IsError {
					continue
				}
				if mutatesWorkspace {
					step.FilesEdited = appendUnique(step.FilesEdited, paths...)
				}
				step.Mutations = append(step.Mutations, evidence)
				if cb.ToolID != "" {
					mutIdxByToolID[cb.ToolID] = [2]int{len(steps) - 1, len(step.Mutations) - 1}
				}
			case "tool_result":
				if denied := parser.IsToolDenial(toolResultText(cb.ToolResult)); denied {
					// A human rejection is an intervention, not an
					// error: count it on the turn/step and mark the
					// issuing call as denied (materialized like errors
					// so denied lists match the counts).
					turn.Denials++
					if step := stepForResult(msg, steps, stepByToolID); step != nil {
						step.Denials++
					}
					if cb.ToolID != "" {
						if loc, ok := mutIdxByToolID[cb.ToolID]; ok {
							steps[loc[0]].Mutations[loc[1]].Denied = true
						} else if ev, ok := callByToolID[cb.ToolID]; ok {
							ev.Denied = true
							if idx, ok := stepByToolID[cb.ToolID]; ok && idx >= 0 && idx < len(steps) {
								steps[idx].Mutations = append(steps[idx].Mutations, ev)
								mutIdxByToolID[cb.ToolID] = [2]int{idx, len(steps[idx].Mutations) - 1}
							}
						}
					}
					continue
				}
				if !cb.IsError {
					continue
				}
				turn.Errors++
				if step := stepForResult(msg, steps, stepByToolID); step != nil {
					step.Errors++
				}
				// Errors arrive on results, not calls: mark the issuing
				// call's evidence, materializing it for non-mutating
				// tools so failure lists match the error counts.
				if cb.ToolID == "" {
					continue
				}
				if loc, ok := mutIdxByToolID[cb.ToolID]; ok {
					steps[loc[0]].Mutations[loc[1]].IsError = true
				} else if ev, ok := callByToolID[cb.ToolID]; ok {
					ev.IsError = true
					if idx, ok := stepByToolID[cb.ToolID]; ok && idx >= 0 && idx < len(steps) {
						steps[idx].Mutations = append(steps[idx].Mutations, ev)
						mutIdxByToolID[cb.ToolID] = [2]int{idx, len(steps[idx].Mutations) - 1}
					}
				}
			}
		}
	}

	turn.Steps = steps
	turn.FilesEdited = sortedKeys(editSet)
	turn.FilesRead = sortedKeys(readSet)
	turn.ToolCounts = sumToolCounts(steps)
	turn.ActiveSecs = active.Seconds()

	return turn
}

// toolResultText returns the text of a tool result payload, which
// arrives as a string or as a list of content blocks.
func toolResultText(result any) string {
	switch v := result.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return text
		}
	}
	return ""
}

// stepForResult finds the step that issued the call a result belongs
// to (via ToolID), falling back to the latest step. Results can land
// after later narration — background agents especially — so ToolID
// attribution matters for error and sidechain placement. Returns nil
// when no step exists yet (malformed or truncated logs).
func stepForResult(msg *parser.Message, steps []Step, stepByToolID map[string]int) *Step {
	if len(steps) == 0 {
		return nil
	}
	for _, cb := range msg.Content {
		if cb.Type == "tool_result" && cb.ToolID != "" {
			if idx, ok := stepByToolID[cb.ToolID]; ok && idx < len(steps) {
				return &steps[idx]
			}
		}
	}
	return &steps[len(steps)-1]
}

func sumToolCounts(steps []Step) map[string]int {
	var out map[string]int
	for _, s := range steps {
		for name, n := range s.ToolCounts {
			if out == nil {
				out = make(map[string]int)
			}
			out[name] += n
		}
	}
	return out
}

func appendUnique(list []string, values ...string) []string {
	for _, v := range values {
		found := false
		for _, existing := range list {
			if existing == v {
				found = true
				break
			}
		}
		if !found {
			list = append(list, v)
		}
	}
	return list
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
		// The final report is often the sidechain's whole value (research
		// agents); keep it untruncated here, ANSI-stripped only. Step-level
		// refs bound it on attach.
		if text := stripANSI(firstText(msg)); text != "" {
			summary.Summary = text
		}

		// Sidechains are subagent internals: the trace keeps their
		// mutation/error calls, file lists, and counts, but not every
		// read — per-call read evidence made sidechain-heavy traces
		// several times larger with no decision value.
		editSet := makeStringSet(summary.FilesEdited)
		readSet := makeStringSet(summary.FilesRead)
		for _, block := range msg.Content {
			if block.Type != "tool_use" || block.ToolName == "" {
				continue
			}
			paths := extractPaths(block.ToolInput)
			mutatesWorkspace := mutatingTools[block.ToolName] && len(paths) > 0
			if mutatingTools[block.ToolName] || block.IsError {
				summary.ToolCallEvidence = append(summary.ToolCallEvidence, ToolCallEvidence{
					MessageID:        msg.UUID,
					ToolID:           block.ToolID,
					Name:             block.ToolName,
					Timestamp:        msg.Timestamp,
					Summary:          commandSummary(block.ToolInput),
					Paths:            paths,
					MutationCapable:  mutatingTools[block.ToolName],
					MutatesWorkspace: mutatesWorkspace,
					Reads:            readTools[block.ToolName],
					IsError:          block.IsError,
				})
			}
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
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, path)
		}
		add(path)
	}

	switch input := toolInput.(type) {
	case map[string]any:
		workdir := toolWorkdir(input)
		for _, key := range []string{"file_path", "path", "notebook_path", "filePath"} {
			if v, ok := input[key].(string); ok {
				addFromBase(v, workdir)
			}
		}
		if v, ok := input["edits"].([]any); ok {
			for _, e := range v {
				if em, ok := e.(map[string]any); ok {
					if p, ok := em["file_path"].(string); ok {
						addFromBase(p, workdir)
					}
				}
			}
		}
		if patch, ok := input["patch"].(string); ok {
			for _, p := range extractPatchPaths(patch) {
				addFromBase(p, workdir)
			}
		}
		if input["patch"] == nil {
			if patch, ok := input["input"].(string); ok && strings.Contains(patch, "*** Begin Patch") {
				for _, p := range extractPatchPaths(patch) {
					addFromBase(p, workdir)
				}
			}
		}
		if cmd, ok := input["command"].(string); ok {
			for _, p := range extractRedirectPaths(cmd) {
				addFromBase(p, workdir)
			}
		}
	case string:
		if strings.Contains(input, "*** Begin Patch") {
			for _, p := range extractPatchPaths(input) {
				add(p)
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func toolWorkdir(input map[string]any) string {
	for _, key := range []string{"workdir", "cwd", "working_dir", "directory"} {
		if v, ok := input[key].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				return v
			}
		}
	}
	return ""
}

func makeStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

var patchFilePattern = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

func extractPatchPaths(patch string) []string {
	matches := patchFilePattern.FindAllStringSubmatch(patch, -1)
	paths := make([]string, 0, len(matches))
	for _, m := range matches {
		if p := cleanEvidencePath(m[1]); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

var redirectPattern = regexp.MustCompile(`(?:>>?|\btee(?:\s+-a)?)\s+([^\s;|&<>]+)`)

// heredocStart matches a heredoc operator and captures its delimiter:
// <<EOF, <<-EOF, <<'EOF', <<"EOF".
var heredocStart = regexp.MustCompile(`<<-?\s*(['"]?)([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// stripHeredocs removes heredoc bodies from a shell command so that
// code or markdown fed through them (a Go `if n > 0`, a blockquote
// `> 2026-08-18`) is not read as an output redirect. The body starts
// after the line carrying the operator and ends at the line equal to
// the delimiter (leading tabs allowed for <<-).
func stripHeredocs(command string) string {
	if !strings.Contains(command, "<<") {
		return command
	}
	lines := strings.Split(command, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		m := heredocStart.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		delim := m[2]
		for i+1 < len(lines) {
			i++
			if strings.TrimLeft(lines[i], "\t") == delim {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

func extractRedirectPaths(command string) []string {
	if !strings.ContainsAny(command, ">") && !strings.Contains(command, "tee") {
		return nil
	}
	command = stripHeredocs(command)
	matches := redirectPattern.FindAllStringSubmatch(command, -1)
	var paths []string
	for _, m := range matches {
		p := cleanEvidencePath(m[1])
		if p == "" || strings.HasPrefix(p, "-") || strings.HasPrefix(p, "&") {
			continue
		}
		paths = append(paths, p)
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

// evidenceCommandRunes bounds the one-line command excerpt carried on
// tool-call evidence. Short by design: the summary answers "what did
// this step run", the raw JSONL (via message_id) holds the rest.
const evidenceCommandRunes = 200

// commandSummary extracts what a command-carrying tool call ran, as
// one bounded line. Providers normalize their shell dialects onto a
// "command" string input (Bash, exec_command, run_terminal_command),
// so that key is the contract.
func commandSummary(toolInput any) string {
	input, ok := toolInput.(map[string]any)
	if !ok {
		return ""
	}
	cmd, ok := input["command"].(string)
	if !ok {
		return ""
	}
	cmd = strings.Join(strings.Fields(cmd), " ")
	runes := []rune(cmd)
	if len(runes) <= evidenceCommandRunes {
		return cmd
	}
	return strings.TrimSpace(string(runes[:evidenceCommandRunes])) + "..."
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

// Evidence text budgets. A trace is an evidence bundle for an LLM
// skill, not a transcript: unbounded text made real traces balloon
// past what any consumer can hold. Head+tail excerpts keep the intent
// and the outcome of long messages; message IDs remain the drill-down
// pointers to full content.
const (
	maxUserTextRunes  = 2000
	maxNarrationRunes = 1200
	omittedTailFrac   = 5 // keep 1/5 of the budget as tail
)

var ansiEscapePattern = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[A-Za-z]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

// commandXMLTags are Claude Code harness wrappers around slash-command
// anchors and their local output. The tag soup is noise for evidence
// consumers; condenseCommandText reduces it to a readable form.
var (
	commandNamePattern   = regexp.MustCompile(`<command-name>([^<]*)</command-name>`)
	commandArgsPattern   = regexp.MustCompile(`<command-args>([^<]*)</command-args>`)
	commandStdoutPattern = regexp.MustCompile(`(?s)<local-command-stdout>(.*?)</local-command-stdout>`)
	commandNoisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?s)<command-message>.*?</command-message>`),
		regexp.MustCompile(`(?s)<local-command-caveat>.*?</local-command-caveat>`),
	}
)

func stripANSI(text string) string {
	return strings.TrimSpace(ansiEscapePattern.ReplaceAllString(text, ""))
}

// cleanBoundedText normalizes one evidence text field: ANSI escapes
// stripped, command XML condensed, and length bounded to maxRunes.
// The bool reports whether content was omitted.
func cleanBoundedText(text string, maxRunes int) (string, bool) {
	text = condenseCommandText(stripANSI(text))
	return boundText(text, maxRunes)
}

func condenseCommandText(text string) string {
	if !strings.Contains(text, "<command-name>") && !strings.Contains(text, "<local-command-stdout>") {
		return text
	}
	var parts []string
	if m := commandNamePattern.FindStringSubmatch(text); m != nil {
		command := strings.TrimSpace(m[1])
		if a := commandArgsPattern.FindStringSubmatch(text); a != nil && strings.TrimSpace(a[1]) != "" {
			command += " " + strings.TrimSpace(a[1])
		}
		parts = append(parts, "command: "+command)
	}
	if m := commandStdoutPattern.FindStringSubmatch(text); m != nil {
		if out := strings.TrimSpace(m[1]); out != "" {
			parts = append(parts, "stdout: "+out)
		}
	}
	rest := commandNamePattern.ReplaceAllString(text, "")
	rest = commandArgsPattern.ReplaceAllString(rest, "")
	rest = commandStdoutPattern.ReplaceAllString(rest, "")
	for _, p := range commandNoisePatterns {
		rest = p.ReplaceAllString(rest, "")
	}
	if rest = strings.TrimSpace(rest); rest != "" {
		parts = append(parts, rest)
	}
	return strings.Join(parts, "\n")
}

func boundText(text string, maxRunes int) (string, bool) {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text, false
	}
	tail := maxRunes / omittedTailFrac
	head := maxRunes - tail
	omitted := len(runes) - maxRunes
	var b strings.Builder
	b.WriteString(strings.TrimSpace(string(runes[:head])))
	fmt.Fprintf(&b, "\n[... %d chars omitted, see anchor message for full text ...]\n", omitted)
	b.WriteString(strings.TrimSpace(string(runes[len(runes)-tail:])))
	return b.String(), true
}

func sortedSidechains(m map[string]Sidechain) []Sidechain {
	out := make([]Sidechain, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AgentID < out[j].AgentID
	})
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
