// Package grok reads Grok Build (grok CLI) sessions from ~/.grok.
//
// Layout (see docs/devlog/2026-07-15-grok-session-format.org, observed
// against grok 0.2.101, chat_format_version 1):
//
//	~/.grok/sessions/<url-encoded-cwd>/<session-uuid>/
//	  summary.json         session metadata (quick-parse source)
//	  chat_history.jsonl   the conversation (full-parse source of truth)
//	  updates.jsonl        ACP update stream (usage totals)
//	  prompt_history.jsonl (workspace level) per-prompt timestamps
//
// Read-only by contract. Cost is never computed for Grok sessions:
// the files carry token counts but pricing is unverified — accuracy
// over features.
package grok

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/catalog"
	"github.com/thevibeworks/ccx/internal/parser"
)

const ProviderID = "grok"

// chatFormatVersion is the only chat_history format this parser
// understands. The format is young; a bump should fail loud (skip the
// session with an error), not garble silently.
const chatFormatVersion = 1

type Backend struct {
	home        string
	sessionsDir string
}

func New(home string) *Backend {
	return &Backend{
		home:        filepath.Clean(home),
		sessionsDir: filepath.Join(filepath.Clean(home), "sessions"),
	}
}

func (b *Backend) ID() string { return ProviderID }

func (b *Backend) Homes() []string { return []string{b.home} }

func (b *Backend) DiscoverProjects() ([]*parser.Project, error) {
	entries, err := os.ReadDir(b.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []*parser.Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workspace, ok := decodeWorkspaceDir(entry.Name())
		if !ok {
			continue
		}
		sessions := b.discoverWorkspaceSessions(filepath.Join(b.sessionsDir, entry.Name()))
		if len(sessions) == 0 {
			continue
		}
		project := &parser.Project{
			Name:         projectDisplayName(workspace),
			EncodedName:  entry.Name(),
			Path:         workspace,
			Provider:     ProviderID,
			Sessions:     sessions,
			LastModified: sessions[0].EndTime,
		}
		for _, s := range sessions {
			s.ProjectName = project.Name
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified.After(projects[j].LastModified)
	})
	return projects, nil
}

// discoverWorkspaceSessions lists the sessions of one workspace dir,
// newest first. Metadata comes entirely from summary.json — no line
// scan, which makes Grok discovery cheaper than either other provider.
func (b *Backend) discoverWorkspaceSessions(dir string) []*parser.Session {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var sessions []*parser.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := quickSession(filepath.Join(dir, entry.Name()))
		if err != nil || session == nil {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})
	return sessions
}

func (b *Backend) ListSessions(query catalog.SessionQuery) ([]*parser.Session, error) {
	if query.Scope == catalog.ScopeWorkspace && query.WorkspacePath != "" {
		projects, err := b.DiscoverProjects()
		if err != nil {
			return nil, err
		}
		want := filepath.Clean(query.WorkspacePath)
		var matched []*parser.Project
		for _, p := range projects {
			if filepath.Clean(p.Path) == want {
				matched = append(matched, p)
			}
		}
		return catalog.ApplySessionQuery(matched, query), nil
	}

	if query.Scope == catalog.ScopeProject && query.ProjectName != "" {
		project, err := b.FindProject(query.ProjectName)
		if err != nil || project == nil {
			return nil, err
		}
		return catalog.ApplySessionQuery([]*parser.Project{project}, query), nil
	}

	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	return catalog.ApplySessionQuery(projects, query), nil
}

func (b *Backend) FindProject(name string) (*parser.Project, error) {
	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Name == name || p.EncodedName == name {
			return p, nil
		}
	}
	return nil, nil
}

func (b *Backend) FindSession(projectName, sessionID string) (*parser.Session, error) {
	projects, err := b.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if projectName != "" && p.Name != projectName && p.EncodedName != projectName {
			continue
		}
		for _, s := range p.Sessions {
			if s.ID == sessionID || strings.HasPrefix(s.ID, sessionID) {
				return s, nil
			}
		}
	}
	return nil, nil
}

// ParseSession full-parses a session from its chat_history.jsonl path
// (the FilePath quickSession hands out).
func (b *Backend) ParseSession(filePath string) (*parser.Session, error) {
	sessionDir := filepath.Dir(filePath)
	session, err := quickSession(sessionDir)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("grok: no summary.json next to %s", filePath)
	}

	promptTimes := promptTimestamps(filepath.Join(filepath.Dir(sessionDir), "prompt_history.jsonl"), session.ID)
	messages, stats, err := parseChatHistory(filePath, session, promptTimes)
	if err != nil {
		return nil, err
	}

	usage := lastUsageUpdate(filepath.Join(sessionDir, "updates.jsonl"))
	if usage != nil {
		stats.InputTokens = usage.InputTokens
		stats.OutputTokens = usage.OutputTokens
		stats.CacheReadTokens = usage.CachedReadTokens
		// CostUSD stays 0 by contract: no Grok cost estimation until
		// pricing is verified against real billing evidence.
	}
	stats.DurationSeconds = session.EndTime.Sub(session.StartTime).Seconds()
	if stats.DurationSeconds < 0 {
		stats.DurationSeconds = 0
	}

	session.RootMessages = buildMessageTree(messages)
	session.Stats = stats
	return session, nil
}

// grokSummary mirrors summary.json.
type grokSummary struct {
	Info struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"info"`
	SessionSummary    string    `json:"session_summary"`
	GeneratedTitle    string    `json:"generated_title"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	LastActiveAt      time.Time `json:"last_active_at"`
	NumMessages       int       `json:"num_messages"`
	NumChatMessages   int       `json:"num_chat_messages"`
	CurrentModelID    string    `json:"current_model_id"`
	ChatFormatVersion int       `json:"chat_format_version"`
	GitRootDir        string    `json:"git_root_dir"`
	HeadBranch        string    `json:"head_branch"`
}

func quickSession(sessionDir string) (*parser.Session, error) {
	data, err := os.ReadFile(filepath.Join(sessionDir, "summary.json"))
	if err != nil {
		return nil, err
	}
	var summary grokSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	if summary.Info.ID == "" {
		return nil, nil
	}
	if summary.ChatFormatVersion != chatFormatVersion {
		return nil, fmt.Errorf("grok: session %s has chat_format_version %d, this ccx understands %d",
			summary.Info.ID, summary.ChatFormatVersion, chatFormatVersion)
	}

	end := summary.LastActiveAt
	if end.IsZero() {
		end = summary.UpdatedAt
	}
	title := summary.SessionSummary
	if title == "" {
		title = summary.GeneratedTitle
	}
	return &parser.Session{
		ID:        summary.Info.ID,
		FilePath:  filepath.Join(sessionDir, "chat_history.jsonl"),
		Provider:  ProviderID,
		Summary:   title,
		StartTime: summary.CreatedAt,
		EndTime:   end,
		Model:     summary.CurrentModelID,
		CWD:       summary.Info.CWD,
		GitBranch: summary.HeadBranch,
		Stats: parser.SessionStats{
			MessageCount: summary.NumChatMessages,
		},
	}, nil
}

// chatLine is one chat_history.jsonl record, discriminated by Type.
type chatLine struct {
	Type       string          `json:"type"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []grokToolCall  `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
	ModelID    string          `json:"model_id"`
	Summary    json.RawMessage `json:"summary"` // reasoning lines
}

type grokToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string (OpenAI dialect)
}

func parseChatHistory(filePath string, session *parser.Session, promptTimes []time.Time) ([]*parser.Message, parser.SessionStats, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, parser.SessionStats{}, err
	}
	defer f.Close()

	var (
		messages []*parser.Message
		stats    parser.SessionStats
		model    = session.Model
		// Chat lines carry no timestamps; user prompts get theirs from
		// the workspace prompt history, everything else inherits the
		// last known time so turn ordering and boundaries stay honest.
		currentTS = session.StartTime
		userSeen  = 0
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		var line chatLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.ModelID != "" {
			model = line.ModelID
		}

		switch line.Type {
		case "system":
			messages = append(messages, newMessage(
				fmt.Sprintf("grok-system-%d", lineNum), "system", parser.KindMeta, currentTS, model,
				parser.ContentBlock{Type: "text", Text: decodeContentText(line.Content)},
			))

		case "user":
			if userSeen < len(promptTimes) {
				currentTS = promptTimes[userSeen]
			}
			userSeen++
			text := extractUserQuery(decodeContentText(line.Content))
			stats.MessageCount++
			stats.UserPrompts++
			messages = append(messages, newMessage(
				fmt.Sprintf("grok-user-%d", lineNum), "user", parser.KindUserPrompt, currentTS, model,
				parser.ContentBlock{Type: "text", Text: text},
			))

		case "assistant":
			blocks := make([]parser.ContentBlock, 0, 1+len(line.ToolCalls))
			if text := decodeContentText(line.Content); text != "" {
				blocks = append(blocks, parser.ContentBlock{Type: "text", Text: text})
			}
			for _, call := range line.ToolCalls {
				stats.ToolCalls++
				blocks = append(blocks, parser.ContentBlock{
					Type:      "tool_use",
					ToolID:    call.ID,
					ToolName:  normalizeToolName(call.Name),
					ToolInput: decodeArguments(call.Arguments),
				})
			}
			if len(blocks) == 0 {
				continue
			}
			stats.MessageCount++
			messages = append(messages, newMessage(
				fmt.Sprintf("grok-assistant-%d", lineNum), "assistant", parser.KindAssistant, currentTS, model,
				blocks...,
			))

		case "reasoning":
			// Grok thinking is encrypted; only the summary is
			// renderable. Never surface more than actually exists.
			if summary := decodeContentText(line.Summary); summary != "" {
				messages = append(messages, newMessage(
					fmt.Sprintf("grok-thinking-%d", lineNum), "assistant", parser.KindAssistant, currentTS, model,
					parser.ContentBlock{Type: "thinking", Text: summary},
				))
			}

		case "tool_result":
			messages = append(messages, newMessage(
				fmt.Sprintf("grok-result-%d", lineNum), "user", parser.KindToolResult, currentTS, model,
				parser.ContentBlock{
					Type:   "tool_result",
					ToolID: line.ToolCallID,
					Text:   decodeContentText(line.Content),
				},
			))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, parser.SessionStats{}, err
	}
	return messages, stats, nil
}

// decodeContentText flattens grok content shapes: a plain string, or
// an array of {type:"text",text} blocks.
func decodeContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, blk := range blocks {
			if blk.Text != "" {
				parts = append(parts, blk.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// extractUserQuery unwraps the harness envelope around the first user
// turn: grok wraps the typed prompt in <user_query> and prepends
// <user_info>/<git_status> blocks. The typed prompt is the message;
// the envelope is provider noise.
func extractUserQuery(text string) string {
	start := strings.Index(text, "<user_query>")
	if start < 0 {
		return text
	}
	rest := text[start+len("<user_query>"):]
	end := strings.Index(rest, "</user_query>")
	if end < 0 {
		return text
	}
	return strings.TrimSpace(rest[:end])
}

// decodeArguments parses the OpenAI-dialect arguments string (JSON
// encoded as a string) into a structured value for path extraction.
func decodeArguments(arguments string) any {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}
	return arguments
}

// normalizeToolName maps grok's tool dialect onto the canonical names
// the trace analyzer understands (same precedent as the codex
// backend's exec_command -> Bash mapping).
func normalizeToolName(name string) string {
	switch strings.TrimSpace(name) {
	case "":
		return "Tool"
	case "run_terminal_command", "terminal", "bash":
		return "Bash"
	case "read_file":
		return "Read"
	case "edit_file":
		return "Edit"
	case "create_file", "write_file":
		return "Write"
	case "todo_write":
		return "TodoWrite"
	default:
		return name
	}
}

// promptTimestamps returns the timestamps of this session's prompts,
// in order, from the workspace-level prompt_history.jsonl.
func promptTimestamps(path, sessionID string) []time.Time {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var times []time.Time
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Timestamp time.Time `json:"timestamp"`
			SessionID string    `json:"session_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.SessionID == sessionID {
			times = append(times, entry.Timestamp)
		}
	}
	return times
}

// grokUsage mirrors the camelCase usage object in updates.jsonl.
type grokUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CachedReadTokens int `json:"cachedReadTokens"`
	ReasoningTokens  int `json:"reasoningTokens"`
}

// lastUsageUpdate scans updates.jsonl for the final cumulative usage
// snapshot. Only totals are trusted; per-message attribution needs
// stream correlation that v1 parity doesn't do.
func lastUsageUpdate(path string) *grokUsage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var last *grokUsage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var update struct {
			Params struct {
				Update struct {
					Usage *grokUsage `json:"usage"`
				} `json:"update"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &update); err != nil {
			continue
		}
		if update.Params.Update.Usage != nil {
			last = update.Params.Update.Usage
		}
	}
	return last
}

// decodeWorkspaceDir url-decodes a sessions/ entry name back to the
// absolute workspace path. Non-path entries (sqlite indexes, stray
// files) report ok=false.
func decodeWorkspaceDir(name string) (string, bool) {
	decoded, err := url.QueryUnescape(name)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(decoded, "/") {
		return "", false
	}
	return decoded, true
}

func projectDisplayName(workspace string) string {
	return filepath.Base(filepath.Clean(workspace))
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

// buildMessageTree anchors responses under their user prompt, the
// same shape the codex backend produces for linear transcripts.
func buildMessageTree(messages []*parser.Message) []*parser.Message {
	var roots []*parser.Message
	var currentAnchor *parser.Message

	for _, message := range messages {
		if message.Kind == parser.KindUserPrompt || message.Kind == parser.KindCommand {
			roots = append(roots, message)
			currentAnchor = message
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
