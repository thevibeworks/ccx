package parser

import (
	"time"
)

// MessageKind classifies the purpose/nature of a message
type MessageKind string

const (
	KindUserPrompt     MessageKind = "user_prompt"     // Actual user input
	KindToolResult     MessageKind = "tool_result"     // Tool execution result
	KindCommand        MessageKind = "command"         // Slash command (/init, /compact, etc)
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
	Sessions     []*Session
	LastModified time.Time
}

type Session struct {
	ID           string
	FilePath     string
	ProjectName  string
	Summary      string
	StartTime    time.Time
	EndTime      time.Time
	RootMessages []*Message
	Stats        SessionStats
	Branches     []Branch // For resume view tree structure

	// Metadata from session messages
	Slug      string // Human-readable name like "melodic-cooking-piglet"
	Version   string // Claude Code version used
	GitBranch string // Git branch at session start
	CWD       string // Working directory
}

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

	// Duration
	DurationSeconds float64
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

	raw rawMessage
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
	Type             string         `json:"type"`
	Subtype          string         `json:"subtype"` // compact_boundary, local_command
	Timestamp        string         `json:"timestamp"`
	UUID             string         `json:"uuid"`
	ParentUUID       string         `json:"parentUuid"`
	LogicalParentUUID string        `json:"logicalParentUuid"` // True parent for compact_boundary
	SessionID        string         `json:"sessionId"`
	IsCompactSummary bool           `json:"isCompactSummary"`
	IsSidechain      bool           `json:"isSidechain"`
	IsMeta           bool           `json:"isMeta"`
	AgentID          string         `json:"agentId"`
	Message          messagePayload `json:"message"`
	Content          string         `json:"content"` // For system messages
	Summary          string         `json:"summary"` // For summary type
	LeafUUID         string         `json:"leafUuid"` // For summary type
	Usage            *usageData     `json:"usage"`    // Token usage from API

	// Session metadata (extracted from first messages)
	Slug      string `json:"slug"`      // Human-readable name like "melodic-cooking-piglet"
	Version   string `json:"version"`   // Claude Code version
	GitBranch string `json:"gitBranch"` // Git branch at session start
	CWD       string `json:"cwd"`       // Working directory
}

type usageData struct {
	InputTokens             int `json:"input_tokens"`
	OutputTokens            int `json:"output_tokens"`
	CacheReadInputTokens    int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type messagePayload struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
	Model   string `json:"model"`   // Model ID for assistant messages
}

