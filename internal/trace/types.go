package trace

import "time"

const (
	TraceKind          = "ccx.trace.v2"
	TraceSchemaVersion = "2"
)

// TraceResult is the full evidence bundle for one session. It is a
// factual record — parsing, slicing, correlation — with zero
// interpretation. Judgment (what mattered, what went wrong, what to
// learn) belongs to the skills that consume this.
type TraceResult struct {
	Kind          string           `json:"kind"`
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   time.Time        `json:"generated_at"`
	Session       SessionMeta      `json:"session"`
	Turns         []Turn           `json:"turns"`
	Sidechains    []Sidechain      `json:"sidechains,omitempty"`
	Git           GitCorrelation   `json:"git"`
	Workspace     WorkspaceContext `json:"workspace_context"`
	Stats         TraceStats       `json:"stats"`
	Warnings      []TraceWarning   `json:"warnings,omitempty"`
}

type SessionMeta struct {
	ID          string    `json:"id"`
	FilePath    string    `json:"file_path,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	ProjectName string    `json:"project_name,omitempty"`
	Summary     string    `json:"summary"`
	Model       string    `json:"model"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	CWD         string    `json:"cwd"`
	GitBranch   string    `json:"git_branch"`
}

// Turn is one unit of user intent: a user prompt (or command) and
// everything the agent did in response, broken into Steps.
type Turn struct {
	Index    int       `json:"index"`
	AnchorID string    `json:"anchor_id"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	// UserText is a bounded evidence excerpt: ANSI-stripped, command
	// XML condensed. AnchorID points at the full message.
	UserText          string `json:"user_text"`
	UserTextTruncated bool   `json:"user_text_truncated,omitempty"`
	IsCommand         bool   `json:"is_command,omitempty"`
	CommandName       string `json:"command_name,omitempty"`

	Steps []Step `json:"steps,omitempty"`

	// Turn-level rollups across all steps.
	FilesEdited   []string       `json:"files_edited,omitempty"`
	FilesRead     []string       `json:"files_read,omitempty"`
	ToolCounts    map[string]int `json:"tool_counts,omitempty"`
	Errors        int            `json:"errors,omitempty"`
	InputTokens   int            `json:"input_tokens,omitempty"`
	OutputTokens  int            `json:"output_tokens,omitempty"`
	CostUSD       float64        `json:"cost_usd,omitempty"`
	LinkedCommits []string       `json:"linked_commits,omitempty"`
}

// Step is the agent's say-then-do unit: one narration block (the
// agent explaining what it is doing and why, at the moment it decides)
// followed by the actions taken before the next narration. This is
// where the decision trail lives in autonomous sessions — a single
// turn can span hours and hundreds of tool calls, and the narration
// sequence is the only faithful account of how the work unfolded.
type Step struct {
	Index     int       `json:"index"`
	MessageID string    `json:"message_id,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`

	// Narration is the bounded assistant text that opened this step.
	Narration          string `json:"narration"`
	NarrationTruncated bool   `json:"narration_truncated,omitempty"`

	ToolCounts  map[string]int `json:"tool_counts,omitempty"`
	FilesEdited []string       `json:"files_edited,omitempty"`
	// Mutations itemizes workspace-changing and failed calls; reads
	// are summarized by ToolCounts and the turn-level file lists.
	Mutations  []ToolCallEvidence `json:"mutations,omitempty"`
	Errors     int                `json:"errors,omitempty"`
	Sidechains []Sidechain        `json:"sidechains,omitempty"`
	CostUSD    float64            `json:"cost_usd,omitempty"`
}

type ToolCallEvidence struct {
	MessageID        string    `json:"message_id,omitempty"`
	ToolID           string    `json:"tool_id,omitempty"`
	Name             string    `json:"name"`
	Timestamp        time.Time `json:"timestamp,omitempty"`
	Paths            []string  `json:"paths,omitempty"`
	MutationCapable  bool      `json:"mutation_capable"`
	MutatesWorkspace bool      `json:"mutates_workspace"`
	Reads            bool      `json:"reads"`
	IsError          bool      `json:"is_error,omitempty"`
}

type Sidechain struct {
	AgentID           string             `json:"agent_id"`
	AgentType         string             `json:"agent_type"`
	Summary           string             `json:"summary"`
	ToolCalls         int                `json:"tool_calls"`
	ToolCallEvidence  []ToolCallEvidence `json:"tool_call_evidence,omitempty"`
	FilesEdited       []string           `json:"files_edited,omitempty"`
	FilesRead         []string           `json:"files_read,omitempty"`
	Status            string             `json:"status,omitempty"`
	TranscriptOmitted bool               `json:"transcript_omitted"`
	MessageCount      int                `json:"message_count,omitempty"`
	InputTokens       int                `json:"input_tokens,omitempty"`
	OutputTokens      int                `json:"output_tokens,omitempty"`
	CostUSD           float64            `json:"cost_usd,omitempty"`
}

type GitCorrelation struct {
	RepoRoot         string           `json:"repo_root,omitempty"`
	Branch           string           `json:"branch,omitempty"`
	Head             string           `json:"head,omitempty"`
	Dirty            bool             `json:"dirty"`
	UncommittedFiles []GitFileStatus  `json:"uncommitted_files,omitempty"`
	UncommittedStat  string           `json:"uncommitted_stat,omitempty"`
	Commits          []GitCommit      `json:"commits"`
	TurnCommitLinks  []TurnCommitLink `json:"turn_commit_links"`
}

type GitCommit struct {
	SHA       string   `json:"sha"`
	Timestamp string   `json:"timestamp"`
	Subject   string   `json:"subject"`
	Files     []string `json:"files"`
}

type GitFileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type TurnCommitLink struct {
	TurnIndex   int      `json:"turn_index"`
	CommitSHA   string   `json:"commit_sha"`
	FileOverlap []string `json:"file_overlap"`
	Confidence  string   `json:"confidence,omitempty"`
}

type WorkspaceContext struct {
	RepoRoot     string            `json:"repo_root,omitempty"`
	Documents    []ContextDocument `json:"documents,omitempty"`
	Knowledge    []ContextDocument `json:"knowledge,omitempty"`
	Missing      []string          `json:"missing,omitempty"`
	Truncated    bool              `json:"truncated"`
	MaxBytes     int               `json:"max_bytes_per_document,omitempty"`
	MaxDocuments int               `json:"max_documents,omitempty"`
}

type ContextDocument struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Title     string `json:"title,omitempty"`
	Bytes     int    `json:"bytes"`
	SHA256    string `json:"sha256"`
	Excerpt   string `json:"excerpt,omitempty"`
	Truncated bool   `json:"truncated"`
}

type TraceWarning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type TraceStats struct {
	TurnCount        int     `json:"turn_count"`
	StepCount        int     `json:"step_count"`
	FilesEdited      int     `json:"files_edited"`
	FilesRead        int     `json:"files_read"`
	ToolsUsed        int     `json:"tools_used"`
	ToolErrors       int     `json:"tool_errors"`
	WorkspaceDocs    int     `json:"workspace_docs"`
	KnowledgeEntries int     `json:"knowledge_entries"`
	CommitsLinked    int     `json:"commits_linked"`
	UncommittedFiles int     `json:"uncommitted_files"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	DurationSecs     float64 `json:"duration_seconds"`
	HasSidechains    bool    `json:"has_sidechains"`
}

// Outline is the always-fits skeleton of a session: every turn and
// step headline with rollup numbers, small enough to hold whole even
// for monster sessions. Consumers read the outline first, then drill
// into specific turns with `ccx trace --turn N`.
type Outline struct {
	Kind        string         `json:"kind"`
	GeneratedAt time.Time      `json:"generated_at"`
	Session     SessionMeta    `json:"session"`
	Stats       TraceStats     `json:"stats"`
	Turns       []OutlineTurn  `json:"turns"`
	Warnings    []TraceWarning `json:"warnings,omitempty"`
}

const OutlineKind = "ccx.outline.v1"

type OutlineTurn struct {
	Index         int           `json:"index"`
	Start         time.Time     `json:"start"`
	DurationSecs  float64       `json:"duration_seconds,omitempty"`
	UserText      string        `json:"user_text"`
	IsCommand     bool          `json:"is_command,omitempty"`
	CommandName   string        `json:"command_name,omitempty"`
	Steps         []OutlineStep `json:"steps,omitempty"`
	Edits         int           `json:"edits,omitempty"`
	Tools         int           `json:"tools,omitempty"`
	Errors        int           `json:"errors,omitempty"`
	Agents        int           `json:"agents,omitempty"`
	CostUSD       float64       `json:"cost_usd,omitempty"`
	LinkedCommits []string      `json:"linked_commits,omitempty"`
}

type OutlineStep struct {
	Index    int    `json:"index"`
	Headline string `json:"headline"`
	Tools    int    `json:"tools,omitempty"`
	Edits    int    `json:"edits,omitempty"`
	Errors   int    `json:"errors,omitempty"`
	Agents   int    `json:"agents,omitempty"`
}
