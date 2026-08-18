package parser

import (
	"encoding/json"
	"time"
)

// MessageKind classifies the purpose/nature of a message
type MessageKind string

const (
	KindUserPrompt     MessageKind = "user_prompt"     // Actual user input
	KindToolResult     MessageKind = "tool_result"     // Tool execution result
	KindCommand        MessageKind = "command"         // Slash command (/init, /compact, etc)
	KindCommandOutput  MessageKind = "command_output"  // <local-command-stdout/stderr/caveat> harness echo
	KindNotification   MessageKind = "notification"    // <task-notification> background-task event
	KindMeta           MessageKind = "meta"            // Meta/system instruction
	KindCompactSummary MessageKind = "compact_summary" // Compacted context carrier
	KindAssistant      MessageKind = "assistant"       // Assistant response
	KindSystem         MessageKind = "system"          // System event
	KindUnknown        MessageKind = "unknown"         // Fallback
)

type Project struct {
	Name         string
	EncodedName  string
	Path         string
	Provider     string // "claude-code", "codex", or "" for merged
	Sessions     []*Session
	LastModified time.Time
}

// CacheFormatVersion stamps every persisted parse cache (the session
// disk cache and the discovery metadata cache) and invalidates them
// all when it changes. Bump it on ANY change to parsing,
// classification, or the Session/Message structs: gob decodes across
// struct versions silently, so without this stamp an upgraded binary
// keeps serving parses produced by the old code until the source
// session file itself happens to change.
const CacheFormatVersion = 4

type Session struct {
	ID           string
	FilePath     string
	ProjectName  string
	Provider     string // "claude-code" or "codex"
	Summary      string
	StartTime    time.Time
	EndTime      time.Time
	RootMessages []*Message
	Stats        SessionStats
	Branches     []Branch // For resume view tree structure

	Slug      string
	Title     string // AI-generated session title (aiTitle from JSONL)
	Version   string
	GitBranch string
	CWD       string
	Model     string // Primary model used (from first assistant message)
}

func (s *Session) GetProvider() string     { return s.Provider }
func (s *Session) GetStartTime() time.Time { return s.StartTime }
func (s *Session) GetEndTime() time.Time   { return s.EndTime }
func (s *Session) GetSummary() string      { return s.Summary }
func (s *Session) GetModel() string        { return s.Model }
func (s *Session) GetMessageCount() int    { return s.Stats.MessageCount }

// Branch represents a conversation branch (for resume tree view)
type Branch struct {
	LeafUUID string
	Summary  string
	Messages int
}

type SessionStats struct {
	MessageCount    int
	UserPrompts     int
	ToolCalls       int
	Continuations   int
	AgentSidechains int

	// Token usage (from API responses)
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int

	// Cost (USD). Zero when no priced model was seen.
	CostUSD float64

	// Duration
	DurationSeconds float64
}

// MessageUsage holds per-assistant-message API token counts plus the
// computed cost (USD, using the embedded pricing table). Populated on
// full parse; nil when the message has no usage field.
//
// Covers both Claude Code shape (input/output/cache_read/cache_create)
// and Codex shape (input/output/cached_input/reasoning_output). For
// Codex sessions, CacheReadTokens carries cached_input_tokens and
// ReasoningTokens carries reasoning_output_tokens. CacheCreateTokens
// stays zero for Codex (OpenAI doesn't expose 5m/1h cache creation
// the way Anthropic does).
//
// CONTRACT (Anthropic-style exclusive semantics, both providers):
// InputTokens excludes cache reads — the Codex backend subtracts
// cached_input_tokens, which upstream includes in input_tokens.
// OutputTokens includes reasoning; ReasoningTokens is the informational
// subset and is never billed separately (see ComputeCost).
type MessageUsage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheCreateTokens int
	ReasoningTokens   int     // Codex-only (GPT-5 reasoning); 0 for Claude
	CostUSD           float64 // 0 if model is unknown to the pricing table
}

// Total returns the sum of all token categories.
func (u *MessageUsage) Total() int {
	if u == nil {
		return 0
	}
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheCreateTokens + u.ReasoningTokens
}

type Message struct {
	UUID       string
	ParentUUID string
	Type       string      // "user" | "assistant" | "system"
	Kind       MessageKind // Semantic classification
	Timestamp  time.Time
	Content    []ContentBlock
	Children   []*Message

	IsCompacted bool   // isCompactSummary - carries compacted context
	IsSidechain bool   // Agent sidechain
	IsMeta      bool   // Meta message (system instructions)
	IsCommand   bool   // Slash command message
	CommandName string // /init, /compact, /resume, etc.
	CommandArgs string // Command arguments (extra instructions)
	AgentID     string
	Model       string // Model ID (e.g., claude-sonnet-4-5-20250929)
	Subtype     string // For system messages: compact_boundary, local_command

	Usage          *MessageUsage       // Per-message API token usage + cost. nil if absent.
	SubAgentResult *SubAgentResultData // Sub-agent summary from toolUseResult. nil for non-agent tool results.

	RawJSON string // Original JSONL line verbatim
	raw     rawMessage
}

type ContentBlock struct {
	Type       string // text | tool_use | tool_result | thinking | image
	Text       string
	ToolName   string
	ToolID     string
	ToolInput  any
	ToolResult any
	IsError    bool
	ImageData  string
	MediaType  string
}

type rawMessage struct {
	Type              string         `json:"type"`
	Subtype           string         `json:"subtype"` // compact_boundary, local_command
	Timestamp         string         `json:"timestamp"`
	UUID              string         `json:"uuid"`
	ParentUUID        string         `json:"parentUuid"`
	LogicalParentUUID string         `json:"logicalParentUuid"` // True parent for compact_boundary
	SessionID         string         `json:"sessionId"`
	IsCompactSummary  bool           `json:"isCompactSummary"`
	IsSidechain       bool           `json:"isSidechain"`
	IsMeta            bool           `json:"isMeta"`
	AgentID           string         `json:"agentId"`
	Message           messagePayload `json:"message"`
	Content           string         `json:"content"`  // For system messages
	Summary           string         `json:"summary"`  // For summary type
	LeafUUID          string         `json:"leafUuid"` // For summary type
	Usage             *usageData     `json:"usage"`    // Token usage from API

	// Session metadata (extracted from first messages)
	Slug      string `json:"slug"`      // Human-readable name like "melodic-cooking-piglet"
	Version   string `json:"version"`   // Claude Code version
	GitBranch string `json:"gitBranch"` // Git branch at session start
	CWD       string `json:"cwd"`       // Working directory

	// Fields added in Claude Code 2.1.137+
	AITitle       string          `json:"aiTitle"`       // AI-generated session title
	Entrypoint    string          `json:"entrypoint"`    // "cli" or "web"
	UserType      string          `json:"userType"`      // "external"
	StopReason    string          `json:"stopReason"`    // Why the turn ended
	DurationMs    int             `json:"durationMs"`    // Turn duration in milliseconds
	URL           string          `json:"url"`           // Bridge/remote URL
	ToolUseResult json.RawMessage `json:"toolUseResult"` // Sub-agent result metadata (string or object)
}

type SubAgentResultData struct {
	AgentID           string     `json:"agentId"`
	AgentType         string     `json:"agentType"`
	Status            string     `json:"status"`
	TotalTokens       int        `json:"totalTokens"`
	TotalToolUseCount int        `json:"totalToolUseCount"`
	TotalDurationMs   int        `json:"totalDurationMs"`
	Usage             *usageData `json:"usage"`
	ToolStats         *ToolStats `json:"toolStats"`
}

type ToolStats struct {
	ReadCount     int `json:"readCount"`
	SearchCount   int `json:"searchCount"`
	BashCount     int `json:"bashCount"`
	EditFileCount int `json:"editFileCount"`
	LinesAdded    int `json:"linesAdded"`
	LinesRemoved  int `json:"linesRemoved"`
}

type usageData struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type messagePayload struct {
	Role    string     `json:"role"`
	Content any        `json:"content"` // string or []contentBlock
	Model   string     `json:"model"`   // Model ID for assistant messages
	Usage   *usageData `json:"usage"`   // Token usage (inside message for API responses)
}
