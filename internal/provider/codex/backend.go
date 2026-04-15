package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

const (
	ProviderID                  = "codex"
	sessionIndexFile            = "session_index.jsonl"
	sessionsSubdir              = "sessions"
	archivedSessionsSubdir      = "archived_sessions"
	userMessagePrefix           = "## My request for Codex:"
	imageOnlyMessagePlaceholder = "[Image]"
	unknownProjectName          = "(unknown cwd)"
	unknownProjectEncodedName   = "unknown"
	maxScannerBufferBytes       = 10 * 1024 * 1024
)

type Backend struct {
	home        string
	sessionsDir string
	archivedDir string
}

type rolloutLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type rolloutEventHeader struct {
	Type string `json:"type"`
}

type responseItemHeader struct {
	Type string `json:"type"`
}

type sessionMetaPayload struct {
	ID            string        `json:"id"`
	ForkedFromID  *string       `json:"forked_from_id"`
	Timestamp     string        `json:"timestamp"`
	CWD           string        `json:"cwd"`
	CLIKeyVersion string        `json:"cli_version"`
	ModelProvider *string       `json:"model_provider"`
	AgentNickname *string       `json:"agent_nickname"`
	AgentRole     *string       `json:"agent_role"`
	Git           *gitInfoValue `json:"git"`
}

type gitInfoValue struct {
	CommitHash    *string `json:"commit_hash"`
	Branch        *string `json:"branch"`
	RepositoryURL *string `json:"repository_url"`
}

type turnContextPayload struct {
	TurnID *string `json:"turn_id"`
	CWD    string  `json:"cwd"`
	Model  string  `json:"model"`
}

type userMessagePayload struct {
	Message     string   `json:"message"`
	Images      []string `json:"images"`
	LocalImages []string `json:"local_images"`
}

type agentMessagePayload struct {
	Message string `json:"message"`
}

type agentReasoningPayload struct {
	Text string `json:"text"`
}

type tokenCountPayload struct {
	Info *tokenUsageInfo `json:"info"`
}

type tokenUsageInfo struct {
	TotalTokenUsage tokenUsageTotals `json:"total_token_usage"`
}

type tokenUsageTotals struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

type execCommandEndPayload struct {
	CallID           string   `json:"call_id"`
	TurnID           string   `json:"turn_id"`
	Command          []string `json:"command"`
	CWD              string   `json:"cwd"`
	Source           string   `json:"source"`
	InteractionInput *string  `json:"interaction_input"`
	Stdout           string   `json:"stdout"`
	Stderr           string   `json:"stderr"`
	AggregatedOutput string   `json:"aggregated_output"`
	ExitCode         int      `json:"exit_code"`
	Duration         string   `json:"duration"`
	FormattedOutput  string   `json:"formatted_output"`
	Status           string   `json:"status"`
}

type patchApplyEndPayload struct {
	CallID  string         `json:"call_id"`
	TurnID  string         `json:"turn_id"`
	Stdout  string         `json:"stdout"`
	Stderr  string         `json:"stderr"`
	Success bool           `json:"success"`
	Status  string         `json:"status"`
	Changes map[string]any `json:"changes"`
}

type webSearchEndPayload struct {
	CallID string `json:"call_id"`
	Query  string `json:"query"`
	Action any    `json:"action"`
}

type mcpInvocation struct {
	Server    string `json:"server"`
	Tool      string `json:"tool"`
	Arguments any    `json:"arguments"`
}

type mcpToolCallEndPayload struct {
	CallID     string        `json:"call_id"`
	Invocation mcpInvocation `json:"invocation"`
	Result     any           `json:"result"`
	Duration   string        `json:"duration"`
}

type dynamicToolCallResponsePayload struct {
	CallID       string `json:"call_id"`
	Tool         string `json:"tool"`
	Arguments    any    `json:"arguments"`
	ContentItems any    `json:"content_items"`
	Success      bool   `json:"success"`
	Error        string `json:"error"`
	Duration     string `json:"duration"`
}

type viewImageToolCallPayload struct {
	CallID string `json:"call_id"`
	Path   string `json:"path"`
}

type threadNameUpdatedPayload struct {
	ThreadName *string `json:"thread_name"`
}

type compactedPayload struct {
	Message string `json:"message"`
}

type functionCallPayload struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
}

type functionCallOutputPayload struct {
	CallID string `json:"call_id"`
	Output any    `json:"output"`
}

type customToolCallPayload struct {
	Name   string `json:"name"`
	Input  any    `json:"input"`
	CallID string `json:"call_id"`
}

type customToolCallOutputPayload struct {
	CallID string `json:"call_id"`
	Output any    `json:"output"`
}

type webSearchCallPayload struct {
	CallID string           `json:"call_id"`
	Status string           `json:"status"`
	Action *webSearchAction `json:"action"`
}

type webSearchAction struct {
	Type    string   `json:"type"`
	Query   string   `json:"query"`
	Queries []string `json:"queries"`
}

type reasoningPayload struct {
	Summary []any `json:"summary"`
}

type pendingToolCall struct {
	Name  string
	Input any
}

func New(home string) *Backend {
	return NewWithDirs(
		home,
		filepath.Join(home, sessionsSubdir),
		filepath.Join(home, archivedSessionsSubdir),
	)
}

func NewWithDirs(home, sessionsDir, archivedDir string) *Backend {
	return &Backend{
		home:        filepath.Clean(home),
		sessionsDir: filepath.Clean(sessionsDir),
		archivedDir: filepath.Clean(archivedDir),
	}
}

func (b *Backend) ID() string { return ProviderID }

func (b *Backend) Homes() []string { return []string{b.home} }

func (b *Backend) DiscoverProjects() ([]*parser.Project, error) {
	threadNames, err := b.readThreadNames()
	if err != nil {
		return nil, err
	}

	sessions, err := b.discoverSessions(threadNames)
	if err != nil {
		return nil, err
	}

	projectsByKey := make(map[string]*parser.Project)
	for _, session := range sessions {
		encodedName, displayName, projectPath := projectInfoForCWD(session.CWD)
		session.ProjectName = encodedName

		project := projectsByKey[encodedName]
		if project == nil {
			project = &parser.Project{
				Name:        displayName,
				EncodedName: encodedName,
				Path:        projectPath,
			}
			projectsByKey[encodedName] = project
		}

		session.Provider = ProviderID
		project.Sessions = append(project.Sessions, session)
		if project.LastModified.IsZero() || session.EndTime.After(project.LastModified) {
			project.LastModified = session.EndTime
		}
	}

	projects := make([]*parser.Project, 0, len(projectsByKey))
	for _, project := range projectsByKey {
		project.Provider = ProviderID
		sort.Slice(project.Sessions, func(i, j int) bool {
			return project.Sessions[i].EndTime.After(project.Sessions[j].EndTime)
		})
		if project.LastModified.IsZero() && len(project.Sessions) > 0 {
			project.LastModified = project.Sessions[0].EndTime
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified.After(projects[j].LastModified)
	})

	return projects, nil
}

func (b *Backend) FindProject(name string) (*parser.Project, error) {
	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(name))
	for _, project := range projects {
		if strings.ToLower(project.Name) == query || strings.ToLower(project.EncodedName) == query {
			return project, nil
		}
		if strings.Contains(strings.ToLower(project.Name), query) || strings.Contains(strings.ToLower(project.Path), query) {
			return project, nil
		}
	}

	return nil, nil
}

func (b *Backend) FindSession(projectName, sessionID string) (*parser.Session, error) {
	if projectName != "" {
		project, err := b.FindProject(projectName)
		if err != nil || project == nil {
			return nil, err
		}
		for _, session := range project.Sessions {
			if matchSession(session, sessionID) {
				session.Provider = ProviderID
				return session, nil
			}
		}
		return nil, nil
	}

	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		for _, session := range project.Sessions {
			if matchSession(session, sessionID) {
				session.Provider = ProviderID
				return session, nil
			}
		}
	}

	return nil, nil
}

func (b *Backend) ParseSession(filePath string) (*parser.Session, error) {
	threadNames, err := b.readThreadNames()
	if err != nil {
		return nil, err
	}
	s, err := b.parseSession(filePath, threadNames)
	if err != nil || s == nil {
		return s, err
	}
	s.Provider = ProviderID
	return s, nil
}

func (b *Backend) discoverSessions(threadNames map[string]string) ([]*parser.Session, error) {
	files, err := b.rolloutFiles()
	if err != nil {
		return nil, err
	}

	sessionsByID := make(map[string]*parser.Session)
	for _, filePath := range files {
		session, err := b.quickParseSession(filePath, threadNames)
		if err != nil || session == nil {
			continue
		}

		key := session.ID
		if key == "" {
			key = filePath
		}

		existing := sessionsByID[key]
		if shouldReplaceSession(existing, session) {
			sessionsByID[key] = session
		}
	}

	sessions := make([]*parser.Session, 0, len(sessionsByID))
	for _, session := range sessionsByID {
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})

	return sessions, nil
}

func (b *Backend) rolloutFiles() ([]string, error) {
	var files []string
	for _, root := range []string{b.sessionsDir, b.archivedDir} {
		rootFiles, err := collectRolloutFiles(root)
		if err != nil {
			return nil, err
		}
		files = append(files, rootFiles...)
	}
	sort.Strings(files)
	return files, nil
}

func collectRolloutFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

func (b *Backend) readThreadNames() (map[string]string, error) {
	path := filepath.Join(b.home, sessionIndexFile)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	type sessionIndexEntry struct {
		ID         string `json:"id"`
		ThreadName string `json:"thread_name"`
	}

	names := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry sessionIndexEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.ID == "" {
			continue
		}

		name := strings.TrimSpace(entry.ThreadName)
		if name != "" {
			names[entry.ID] = name
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return names, nil
}

func (b *Backend) quickParseSession(filePath string, threadNames map[string]string) (*parser.Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sessionID := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	threadName := strings.TrimSpace(threadNames[sessionID])
	summary := ""
	meta := parser.SessionMeta{}
	stats := parser.SessionStats{}
	var firstTime time.Time
	var lastTime time.Time
	seenToolCallIDs := make(map[string]bool)

	countToolCall := func(callID string) {
		callID = strings.TrimSpace(callID)
		if callID == "" {
			stats.ToolCalls++
			return
		}
		if seenToolCallIDs[callID] {
			return
		}
		seenToolCallIDs[callID] = true
		stats.ToolCalls++
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rollout rolloutLine
		if err := json.Unmarshal([]byte(line), &rollout); err != nil {
			continue
		}

		recordTimestampBounds(rollout.Timestamp, &firstTime, &lastTime)

		switch rollout.Type {
		case "session_meta":
			var payload sessionMetaPayload
			if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
				continue
			}
			if payload.ID != "" {
				sessionID = payload.ID
				if threadName == "" {
					threadName = strings.TrimSpace(threadNames[payload.ID])
				}
			}
			if meta.CWD == "" && payload.CWD != "" {
				meta.CWD = payload.CWD
			}
			if meta.Version == "" && payload.CLIKeyVersion != "" {
				meta.Version = payload.CLIKeyVersion
			}
			if meta.GitBranch == "" && payload.Git != nil && payload.Git.Branch != nil {
				meta.GitBranch = strings.TrimSpace(*payload.Git.Branch)
			}

		case "turn_context":
			if meta.Model == "" {
				var payload turnContextPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && payload.Model != "" {
					meta.Model = payload.Model
				}
			}

		case "event_msg":
			var header rolloutEventHeader
			if err := json.Unmarshal(rollout.Payload, &header); err != nil {
				continue
			}

			switch header.Type {
			case "user_message":
				stats.MessageCount++
				stats.UserPrompts++

				var payload userMessagePayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && summary == "" {
					summary = userMessagePreview(payload)
				}

			case "agent_message":
				stats.MessageCount++

			case "exec_command_end":
				var payload execCommandEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "patch_apply_end":
				var payload patchApplyEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "web_search_end":
				var payload webSearchEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "mcp_tool_call_end":
				var payload mcpToolCallEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "dynamic_tool_call_response":
				var payload dynamicToolCallResponsePayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "view_image_tool_call":
				var payload viewImageToolCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "token_count":
				var payload tokenCountPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && payload.Info != nil {
					stats.InputTokens = payload.Info.TotalTokenUsage.InputTokens
					stats.CacheReadTokens = payload.Info.TotalTokenUsage.CachedInputTokens
					stats.OutputTokens = payload.Info.TotalTokenUsage.OutputTokens
				}

			case "thread_name_updated":
				var payload threadNameUpdatedPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && payload.ThreadName != nil {
					threadName = strings.TrimSpace(*payload.ThreadName)
				}
			}

		case "response_item":
			var header responseItemHeader
			if err := json.Unmarshal(rollout.Payload, &header); err != nil {
				continue
			}
			switch header.Type {
			case "function_call":
				var payload functionCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "custom_tool_call":
				var payload customToolCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}

			case "web_search_call":
				var payload webSearchCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil {
					countToolCall(payload.CallID)
				} else {
					stats.ToolCalls++
				}
			case "reasoning":
				// encrypted content, nothing useful to count
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if firstTime.IsZero() || lastTime.IsZero() {
		if info, err := os.Stat(filePath); err == nil {
			if firstTime.IsZero() {
				firstTime = info.ModTime()
			}
			if lastTime.IsZero() {
				lastTime = info.ModTime()
			}
		}
	}
	stats.DurationSeconds = durationSeconds(firstTime, lastTime)

	projectName, _, _ := projectInfoForCWD(meta.CWD)
	return &parser.Session{
		ID:          sessionID,
		FilePath:    filePath,
		ProjectName: projectName,
		Summary:     chooseSummary(threadName, summary),
		StartTime:   firstTime,
		EndTime:     lastTime,
		Stats:       stats,
		Version:     meta.Version,
		GitBranch:   meta.GitBranch,
		CWD:         meta.CWD,
		Model:       meta.Model,
	}, nil
}

func (b *Backend) parseSession(filePath string, threadNames map[string]string) (*parser.Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sessionID := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	threadName := strings.TrimSpace(threadNames[sessionID])
	sessionVersion := ""
	sessionBranch := ""
	sessionCWD := ""
	currentModel := ""
	firstUserSummary := ""
	stats := parser.SessionStats{}
	var firstTime time.Time
	var lastTime time.Time
	var messages []*parser.Message
	pendingTools := make(map[string]pendingToolCall)
	handledCallIDs := make(map[string]bool)
	completedCallIDs := make(map[string]bool)
	// Tracks the running token totals seen in the most recent
	// token_count event AND the index into messages beyond which we
	// haven't yet attributed. On each token_count, we distribute the
	// delta-since-last-event across every untagged assistant in
	// messages[usageWatermark:], then advance the watermark. This
	// handles multi-assistant turns (reasoning + agent_message) and
	// turns that produce multiple agent_message events between
	// token_count events.
	var previousTotals tokenUsageTotals
	var pendingUsageDelta parser.MessageUsage
	usageWatermark := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerBufferBytes)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rollout rolloutLine
		if err := json.Unmarshal([]byte(line), &rollout); err != nil {
			continue
		}

		ts := parseTimestamp(rollout.Timestamp)
		recordTimestampValue(ts, &firstTime, &lastTime)
		msgCountBefore := len(messages)

		switch rollout.Type {
		case "session_meta":
			var payload sessionMetaPayload
			if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
				continue
			}
			if payload.ID != "" {
				sessionID = payload.ID
				if threadName == "" {
					threadName = strings.TrimSpace(threadNames[payload.ID])
				}
			}
			if sessionVersion == "" && payload.CLIKeyVersion != "" {
				sessionVersion = payload.CLIKeyVersion
			}
			if sessionCWD == "" && payload.CWD != "" {
				sessionCWD = payload.CWD
			}
			if sessionBranch == "" && payload.Git != nil && payload.Git.Branch != nil {
				sessionBranch = strings.TrimSpace(*payload.Git.Branch)
			}

		case "turn_context":
			var payload turnContextPayload
			if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
				continue
			}
			if sessionCWD == "" && payload.CWD != "" {
				sessionCWD = payload.CWD
			}
			if payload.Model != "" {
				currentModel = payload.Model
			}

		case "event_msg":
			var header rolloutEventHeader
			if err := json.Unmarshal(rollout.Payload, &header); err != nil {
				continue
			}

			switch header.Type {
			case "user_message":
				var payload userMessagePayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				text := userMessagePreview(payload)
				if text == "" {
					text = "(empty)"
				}
				if firstUserSummary == "" {
					firstUserSummary = text
				}
				stats.MessageCount++
				stats.UserPrompts++
				messages = append(messages, newMessage(
					fmt.Sprintf("codex-user-%d", lineNum),
					"user",
					parser.KindUserPrompt,
					ts,
					currentModel,
					parser.ContentBlock{Type: "text", Text: text},
				))

			case "agent_message":
				var payload agentMessagePayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				stats.MessageCount++
				messages = append(messages, newMessage(
					fmt.Sprintf("codex-assistant-%d", lineNum),
					"assistant",
					parser.KindAssistant,
					ts,
					currentModel,
					parser.ContentBlock{Type: "text", Text: payload.Message},
				))

			case "agent_reasoning", "agent_reasoning_raw_content":
				var payload agentReasoningPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				messages = append(messages, newMessage(
					fmt.Sprintf("codex-thinking-%d", lineNum),
					"assistant",
					parser.KindAssistant,
					ts,
					currentModel,
					parser.ContentBlock{Type: "thinking", Text: payload.Text},
				))

			case "exec_command_end":
				var payload execCommandEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					if tool, ok := pendingTools[payload.CallID]; ok {
						messages = append(messages, newToolResultMessage(
							lineNum,
							ts,
							currentModel,
							fallbackToolName(tool.Name),
							payload.CallID,
							map[string]any{
								"stdout":    payload.Stdout,
								"stderr":    payload.Stderr,
								"output":    chooseToolOutput(payload.FormattedOutput, payload.AggregatedOutput, payload.Stdout, payload.Stderr),
								"exit_code": payload.ExitCode,
								"status":    payload.Status,
								"duration":  payload.Duration,
							},
							payload.ExitCode != 0,
						))
						delete(pendingTools, payload.CallID)
						completedCallIDs[payload.CallID] = true
					}
					continue
				}
				stats.ToolCalls++
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					"Bash",
					payload.CallID,
					map[string]any{
						"argv":    payload.Command,
						"command": strings.Join(payload.Command, " "),
						"cwd":     payload.CWD,
						"source":  payload.Source,
					},
					map[string]any{
						"stdout":    payload.Stdout,
						"stderr":    payload.Stderr,
						"output":    chooseToolOutput(payload.FormattedOutput, payload.AggregatedOutput, payload.Stdout, payload.Stderr),
						"exit_code": payload.ExitCode,
						"status":    payload.Status,
						"duration":  payload.Duration,
					},
					payload.ExitCode != 0,
				)...)
				completedCallIDs[payload.CallID] = true

			case "patch_apply_end":
				var payload patchApplyEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					if tool, ok := pendingTools[payload.CallID]; ok {
						messages = append(messages, newToolResultMessage(
							lineNum,
							ts,
							currentModel,
							fallbackToolName(tool.Name),
							payload.CallID,
							map[string]any{
								"stdout":  payload.Stdout,
								"stderr":  payload.Stderr,
								"success": payload.Success,
								"status":  payload.Status,
								"changes": payload.Changes,
							},
							!payload.Success,
						))
						delete(pendingTools, payload.CallID)
						completedCallIDs[payload.CallID] = true
					}
					continue
				}
				stats.ToolCalls++
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					"ApplyPatch",
					payload.CallID,
					map[string]any{"changes": payload.Changes},
					map[string]any{
						"stdout":  payload.Stdout,
						"stderr":  payload.Stderr,
						"success": payload.Success,
						"status":  payload.Status,
						"changes": payload.Changes,
					},
					!payload.Success,
				)...)
				completedCallIDs[payload.CallID] = true

			case "web_search_end":
				var payload webSearchEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					if tool, ok := pendingTools[payload.CallID]; ok {
						messages = append(messages, newToolResultMessage(
							lineNum,
							ts,
							currentModel,
							fallbackToolName(tool.Name),
							payload.CallID,
							map[string]any{
								"query":  payload.Query,
								"action": payload.Action,
							},
							false,
						))
						delete(pendingTools, payload.CallID)
						completedCallIDs[payload.CallID] = true
					}
					continue
				}
				stats.ToolCalls++
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					"WebSearch",
					payload.CallID,
					map[string]any{"query": payload.Query},
					map[string]any{
						"query":  payload.Query,
						"action": payload.Action,
					},
					false,
				)...)
				completedCallIDs[payload.CallID] = true

			case "mcp_tool_call_end":
				var payload mcpToolCallEndPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					continue
				}
				stats.ToolCalls++
				toolName := strings.TrimSpace(payload.Invocation.Tool)
				if toolName == "" {
					toolName = "MCP"
				}
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					toolName,
					payload.CallID,
					map[string]any{
						"server":    payload.Invocation.Server,
						"tool":      payload.Invocation.Tool,
						"arguments": payload.Invocation.Arguments,
					},
					map[string]any{
						"server":   payload.Invocation.Server,
						"result":   payload.Result,
						"duration": payload.Duration,
					},
					false,
				)...)
				completedCallIDs[payload.CallID] = true

			case "dynamic_tool_call_response":
				var payload dynamicToolCallResponsePayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					continue
				}
				stats.ToolCalls++
				toolName := strings.TrimSpace(payload.Tool)
				if toolName == "" {
					toolName = "DynamicTool"
				}
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					toolName,
					payload.CallID,
					map[string]any{"arguments": payload.Arguments},
					map[string]any{
						"content_items": payload.ContentItems,
						"success":       payload.Success,
						"error":         payload.Error,
						"duration":      payload.Duration,
					},
					!payload.Success,
				)...)
				completedCallIDs[payload.CallID] = true

			case "view_image_tool_call":
				var payload viewImageToolCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if handledCallIDs[payload.CallID] {
					continue
				}
				stats.ToolCalls++
				messages = append(messages, toolPairMessages(
					lineNum,
					ts,
					currentModel,
					"ViewImage",
					payload.CallID,
					map[string]any{"path": payload.Path},
					map[string]any{"path": payload.Path},
					false,
				)...)
				completedCallIDs[payload.CallID] = true

			case "token_count":
				var payload tokenCountPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && payload.Info != nil {
					total := payload.Info.TotalTokenUsage
					if pendingUsageDelta.Total() > 0 && usageWatermark <= len(messages) {
						if hasUntaggedAssistant(messages[usageWatermark:]) {
							distributeCodexDelta(messages[usageWatermark:], pendingUsageDelta)
							usageWatermark = len(messages)
							pendingUsageDelta = parser.MessageUsage{}
						}
					}
					// Session-level aggregate (latest snapshot wins — Codex
					// always emits running totals, not deltas).
					stats.InputTokens = total.InputTokens
					stats.CacheReadTokens = total.CachedInputTokens
					stats.OutputTokens = total.OutputTokens

					// Per-message attribution: diff this total against the
					// running previousTotals and distribute the delta
					// evenly across every untagged assistant message since
					// the last token_count event.
					//
					// Even-split isn't perfectly accurate per-message for
					// multi-assistant turns (reasoning + agent_message +
					// another agent_message), but the per-TURN aggregation
					// the UI displays reassembles correctly because each
					// turn sums all its messages. Without this fix, only
					// the latest assistant got tokens and all earlier
					// ones in the burst showed $0.00.
					delta := parser.MessageUsage{
						InputTokens:     clampNonNegative(total.InputTokens - previousTotals.InputTokens),
						OutputTokens:    clampNonNegative(total.OutputTokens - previousTotals.OutputTokens),
						CacheReadTokens: clampNonNegative(total.CachedInputTokens - previousTotals.CachedInputTokens),
						ReasoningTokens: clampNonNegative(total.ReasoningOutputTokens - previousTotals.ReasoningOutputTokens),
					}
					if delta.Total() > 0 && usageWatermark <= len(messages) {
						if hasUntaggedAssistant(messages[usageWatermark:]) {
							distributeCodexDelta(messages[usageWatermark:], delta)
							usageWatermark = len(messages)
						} else {
							pendingUsageDelta = addUsage(pendingUsageDelta, delta)
						}
					}
					// Use max() as the new floor so a running-total
					// regression doesn't double-count on recovery:
					// 100→200→150→250 should emit deltas 100, 0, 50 — not
					// 100, 0, 100.
					previousTotals.InputTokens = maxInt(previousTotals.InputTokens, total.InputTokens)
					previousTotals.OutputTokens = maxInt(previousTotals.OutputTokens, total.OutputTokens)
					previousTotals.CachedInputTokens = maxInt(previousTotals.CachedInputTokens, total.CachedInputTokens)
					previousTotals.ReasoningOutputTokens = maxInt(previousTotals.ReasoningOutputTokens, total.ReasoningOutputTokens)
					previousTotals.TotalTokens = maxInt(previousTotals.TotalTokens, total.TotalTokens)
				}

			case "thread_name_updated":
				var payload threadNameUpdatedPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err == nil && payload.ThreadName != nil {
					threadName = strings.TrimSpace(*payload.ThreadName)
				}
			}

		case "response_item":
			var header responseItemHeader
			if err := json.Unmarshal(rollout.Payload, &header); err != nil {
				continue
			}

			switch header.Type {
			case "function_call":
				var payload functionCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				stats.ToolCalls++
				toolName := normalizeToolName(payload.Name)
				toolInput := parseJSONString(payload.Arguments)
				pendingTools[payload.CallID] = pendingToolCall{
					Name:  toolName,
					Input: toolInput,
				}
				handledCallIDs[payload.CallID] = true
				messages = append(messages, newToolUseMessage(
					lineNum,
					ts,
					currentModel,
					toolName,
					payload.CallID,
					toolInput,
				))

			case "function_call_output":
				var payload functionCallOutputPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if completedCallIDs[payload.CallID] {
					delete(pendingTools, payload.CallID)
					continue
				}
				tool := pendingTools[payload.CallID]
				messages = append(messages, newToolResultMessage(
					lineNum,
					ts,
					currentModel,
					fallbackToolName(tool.Name),
					payload.CallID,
					parseEmbeddedJSON(payload.Output),
					false,
				))
				delete(pendingTools, payload.CallID)
				completedCallIDs[payload.CallID] = true

			case "custom_tool_call":
				var payload customToolCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				stats.ToolCalls++
				toolName := normalizeToolName(payload.Name)
				toolInput := parseEmbeddedJSON(payload.Input)
				handledCallIDs[payload.CallID] = true
				pendingTools[payload.CallID] = pendingToolCall{
					Name:  toolName,
					Input: toolInput,
				}
				messages = append(messages, newToolUseMessage(
					lineNum,
					ts,
					currentModel,
					toolName,
					payload.CallID,
					toolInput,
				))

			case "custom_tool_call_output":
				var payload customToolCallOutputPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if completedCallIDs[payload.CallID] {
					delete(pendingTools, payload.CallID)
					continue
				}
				tool := pendingTools[payload.CallID]
				messages = append(messages, newToolResultMessage(
					lineNum,
					ts,
					currentModel,
					fallbackToolName(tool.Name),
					payload.CallID,
					parseEmbeddedJSON(payload.Output),
					false,
				))
				delete(pendingTools, payload.CallID)
				completedCallIDs[payload.CallID] = true

			case "web_search_call":
				var payload webSearchCallPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				stats.ToolCalls++
				query := ""
				if payload.Action != nil {
					query = payload.Action.Query
				}
				if payload.CallID != "" {
					handledCallIDs[payload.CallID] = true
					pendingTools[payload.CallID] = pendingToolCall{
						Name:  "WebSearch",
						Input: map[string]any{"query": query},
					}
				}
				messages = append(messages, newToolUseMessage(
					lineNum,
					ts,
					currentModel,
					"WebSearch",
					payload.CallID,
					map[string]any{"query": query},
				))

			case "reasoning":
				var payload reasoningPayload
				if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
					continue
				}
				if len(payload.Summary) > 0 {
					var summaryText string
					for _, item := range payload.Summary {
						if s, ok := item.(string); ok && s != "" {
							if summaryText != "" {
								summaryText += "\n"
							}
							summaryText += s
						}
					}
					if summaryText != "" {
						messages = append(messages, newMessage(
							fmt.Sprintf("codex-thinking-%d", lineNum),
							"assistant",
							parser.KindAssistant,
							ts,
							currentModel,
							parser.ContentBlock{Type: "thinking", Text: summaryText},
						))
					}
				}
			}

		case "compacted":
			var payload compactedPayload
			if err := json.Unmarshal(rollout.Payload, &payload); err != nil {
				continue
			}
			messages = append(messages, newCompactMessage(
				lineNum,
				ts,
				currentModel,
				payload.Message,
			))
		}

		for i := msgCountBefore; i < len(messages); i++ {
			messages[i].RawJSON = line
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if pendingUsageDelta.Total() > 0 && usageWatermark <= len(messages) {
		if hasUntaggedAssistant(messages[usageWatermark:]) {
			distributeCodexDelta(messages[usageWatermark:], pendingUsageDelta)
		}
	}
	stats.CostUSD = sumMessageCosts(messages)

	if firstTime.IsZero() || lastTime.IsZero() {
		if info, err := os.Stat(filePath); err == nil {
			if firstTime.IsZero() {
				firstTime = info.ModTime()
			}
			if lastTime.IsZero() {
				lastTime = info.ModTime()
			}
		}
	}
	stats.DurationSeconds = durationSeconds(firstTime, lastTime)

	projectName, _, _ := projectInfoForCWD(sessionCWD)
	return &parser.Session{
		ID:           sessionID,
		FilePath:     filePath,
		ProjectName:  projectName,
		Summary:      chooseSummary(threadName, firstUserSummary),
		StartTime:    firstTime,
		EndTime:      lastTime,
		RootMessages: buildMessageTree(messages),
		Stats:        stats,
		Version:      sessionVersion,
		GitBranch:    sessionBranch,
		CWD:          sessionCWD,
		Model:        currentModel,
	}, nil
}

func chooseSummary(threadName, firstUser string) string {
	if name := strings.TrimSpace(threadName); name != "" {
		return name
	}
	if preview := strings.TrimSpace(firstUser); preview != "" {
		return preview
	}
	return "(no summary)"
}

func projectInfoForCWD(cwd string) (encodedName, displayName, path string) {
	cleaned := strings.TrimSpace(cwd)
	if cleaned == "" {
		return unknownProjectEncodedName, unknownProjectName, ""
	}
	cleaned = filepath.Clean(cleaned)
	encoded := parser.EncodePath(cleaned)
	return encoded, parser.GetProjectDisplayName(encoded), cleaned
}

func matchSession(session *parser.Session, query string) bool {
	return session.ID == query || strings.HasPrefix(session.ID, query)
}

func shouldReplaceSession(existing, candidate *parser.Session) bool {
	if existing == nil {
		return true
	}

	existingArchived := isArchivedSession(existing.FilePath)
	candidateArchived := isArchivedSession(candidate.FilePath)
	if existingArchived != candidateArchived {
		return existingArchived && !candidateArchived
	}

	if candidate.EndTime.After(existing.EndTime) {
		return true
	}
	if existing.EndTime.Equal(candidate.EndTime) && candidate.StartTime.After(existing.StartTime) {
		return true
	}

	return false
}

func isArchivedSession(path string) bool {
	return strings.Contains(filepath.Clean(path), string(filepath.Separator)+archivedSessionsSubdir+string(filepath.Separator))
}

func recordTimestampBounds(value string, first, last *time.Time) {
	recordTimestampValue(parseTimestamp(value), first, last)
}

func recordTimestampValue(ts time.Time, first, last *time.Time) {
	if ts.IsZero() {
		return
	}
	if first.IsZero() || ts.Before(*first) {
		*first = ts
	}
	if last.IsZero() || ts.After(*last) {
		*last = ts
	}
}

func parseTimestamp(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func durationSeconds(start, end time.Time) float64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}

func stripUserMessagePrefix(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, userMessagePrefix) {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, userMessagePrefix))
	}
	return trimmed
}

func userMessagePreview(payload userMessagePayload) string {
	text := stripUserMessagePrefix(payload.Message)
	if text != "" {
		return text
	}
	if len(payload.Images) > 0 || len(payload.LocalImages) > 0 {
		return imageOnlyMessagePlaceholder
	}
	return ""
}

func newMessage(uuid, role string, kind parser.MessageKind, ts time.Time, model string, blocks ...parser.ContentBlock) *parser.Message {
	return &parser.Message{
		UUID:      uuid,
		Type:      role,
		Kind:      kind,
		Timestamp: ts,
		Content:   blocks,
		Model:     model,
	}
}

func newCompactMessage(lineNum int, ts time.Time, model, text string) *parser.Message {
	message := newMessage(
		fmt.Sprintf("codex-compact-%d", lineNum),
		"system",
		parser.KindCompactSummary,
		ts,
		model,
		parser.ContentBlock{Type: "text", Text: compactMessageText(text)},
	)
	message.IsCompacted = true
	return message
}

func compactMessageText(text string) string {
	if strings.TrimSpace(text) == "" {
		return "Context compacted"
	}
	return text
}

func newToolUseMessage(lineNum int, ts time.Time, model, toolName, toolID string, input any) *parser.Message {
	if toolID == "" {
		toolID = fmt.Sprintf("codex-tool-%d", lineNum)
	}
	return newMessage(
		fmt.Sprintf("codex-tool-use-%d", lineNum),
		"assistant",
		parser.KindAssistant,
		ts,
		model,
		parser.ContentBlock{
			Type:      "tool_use",
			ToolName:  toolName,
			ToolID:    toolID,
			ToolInput: input,
		},
	)
}

func newToolResultMessage(lineNum int, ts time.Time, model, toolName, toolID string, result any, isError bool) *parser.Message {
	if toolID == "" {
		toolID = fmt.Sprintf("codex-tool-%d", lineNum)
	}
	return newMessage(
		fmt.Sprintf("codex-tool-result-%d", lineNum),
		"user",
		parser.KindToolResult,
		ts,
		model,
		parser.ContentBlock{
			Type:       "tool_result",
			ToolName:   toolName,
			ToolID:     toolID,
			ToolResult: result,
			IsError:    isError,
		},
	)
}

func toolPairMessages(lineNum int, ts time.Time, model, toolName, toolID string, input, result any, isError bool) []*parser.Message {
	return []*parser.Message{
		newToolUseMessage(lineNum, ts, model, toolName, toolID, input),
		newToolResultMessage(lineNum, ts, model, toolName, toolID, result, isError),
	}
}

func chooseToolOutput(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseJSONString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}

	return value
}

func parseEmbeddedJSON(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return text
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}

	return value
}

func normalizeToolName(name string) string {
	switch strings.TrimSpace(name) {
	case "":
		return "Tool"
	case "exec_command", "shell_command", "shell":
		return "Bash"
	case "apply_patch":
		return "ApplyPatch"
	case "write_stdin":
		return "WriteStdin"
	case "update_plan":
		return "UpdatePlan"
	default:
		return name
	}
}

func fallbackToolName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Tool"
	}
	return name
}

func buildMessageTree(messages []*parser.Message) []*parser.Message {
	var roots []*parser.Message
	var currentAnchor *parser.Message

	for _, message := range messages {
		if message.Kind == parser.KindUserPrompt || message.Kind == parser.KindCommand {
			roots = append(roots, message)
			currentAnchor = message
			continue
		}
		if message.Kind == parser.KindCompactSummary {
			roots = append(roots, message)
			continue
		}

		if currentAnchor == nil {
			roots = append(roots, message)
			continue
		}

		message.ParentUUID = currentAnchor.UUID
		currentAnchor.Children = append(currentAnchor.Children, message)
	}

	return roots
}

// latestAssistantWithoutUsage walks messages in reverse and returns
// the first assistant message without Usage. Kept for tests that
// already rely on it; new code should prefer distributeCodexDelta.
func latestAssistantWithoutUsage(messages []*parser.Message) *parser.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m == nil || m.Kind != parser.KindAssistant {
			continue
		}
		if m.Usage == nil {
			return m
		}
	}
	return nil
}

func hasUntaggedAssistant(messages []*parser.Message) bool {
	for _, m := range messages {
		if m == nil || m.Kind != parser.KindAssistant {
			continue
		}
		if m.Usage == nil {
			return true
		}
	}
	return false
}

func addUsage(a, b parser.MessageUsage) parser.MessageUsage {
	return parser.MessageUsage{
		InputTokens:       a.InputTokens + b.InputTokens,
		OutputTokens:      a.OutputTokens + b.OutputTokens,
		CacheReadTokens:   a.CacheReadTokens + b.CacheReadTokens,
		CacheCreateTokens: a.CacheCreateTokens + b.CacheCreateTokens,
		ReasoningTokens:   a.ReasoningTokens + b.ReasoningTokens,
	}
}

func sumMessageCosts(messages []*parser.Message) float64 {
	var total float64
	for _, m := range messages {
		if m == nil || m.Usage == nil {
			continue
		}
		total += m.Usage.CostUSD
	}
	return total
}

// distributeCodexDelta evenly splits a token delta across every
// assistant message in `recent` that doesn't already carry Usage.
// Computes per-message cost via LookupPricing on the target's model.
//
// The distribution is even by count, not proportional to individual
// message output size, because Codex's wire format doesn't expose
// per-message counts — only running totals at checkpoints. The
// per-TURN aggregation the UI displays reassembles correctly since
// each turn sums all its messages, so the approximation is invisible
// at the turn level.
//
// If no untagged assistants exist in `recent`, the delta is silently
// dropped. This is correct: the delta is already reflected in the
// session-level stats, and there's no message to attach it to.
func distributeCodexDelta(recent []*parser.Message, delta parser.MessageUsage) {
	untagged := make([]*parser.Message, 0, len(recent))
	for _, m := range recent {
		if m == nil || m.Kind != parser.KindAssistant {
			continue
		}
		if m.Usage == nil {
			untagged = append(untagged, m)
		}
	}
	n := len(untagged)
	if n == 0 {
		return
	}

	share := parser.MessageUsage{
		InputTokens:       delta.InputTokens / n,
		OutputTokens:      delta.OutputTokens / n,
		CacheReadTokens:   delta.CacheReadTokens / n,
		CacheCreateTokens: delta.CacheCreateTokens / n,
		ReasoningTokens:   delta.ReasoningTokens / n,
	}
	// First message absorbs any integer-division remainder so the sum
	// of per-message usage equals the delta exactly.
	remainder := parser.MessageUsage{
		InputTokens:       delta.InputTokens - share.InputTokens*n,
		OutputTokens:      delta.OutputTokens - share.OutputTokens*n,
		CacheReadTokens:   delta.CacheReadTokens - share.CacheReadTokens*n,
		CacheCreateTokens: delta.CacheCreateTokens - share.CacheCreateTokens*n,
		ReasoningTokens:   delta.ReasoningTokens - share.ReasoningTokens*n,
	}
	for i, m := range untagged {
		per := share
		if i == 0 {
			per.InputTokens += remainder.InputTokens
			per.OutputTokens += remainder.OutputTokens
			per.CacheReadTokens += remainder.CacheReadTokens
			per.CacheCreateTokens += remainder.CacheCreateTokens
			per.ReasoningTokens += remainder.ReasoningTokens
		}
		per.CostUSD = parser.ComputeCost(&per, parser.LookupPricing(m.Model))
		m.Usage = &per
	}
}

// clampNonNegative returns v if v >= 0, else 0. Used when diffing
// Codex token totals — a running-total event that reset or regressed
// would otherwise produce negative deltas.
func clampNonNegative(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// maxInt returns the larger of a and b. Go stdlib adds this in 1.21
// but the codebase targets a lower floor. Tiny helper, kept local.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
