package fold

import "time"

const (
	TraceKind          = "ccx.trace.v1"
	TraceSchemaVersion = "1"
)

type TraceResult struct {
	Kind          string             `json:"kind"`
	SchemaVersion string             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Session       SessionMeta        `json:"session"`
	Exchanges     []ExchangeEvidence `json:"exchanges"`
	Sidechains    []Sidechain        `json:"sidechains,omitempty"`
	Git           GitCorrelation     `json:"git"`
	Workspace     WorkspaceContext   `json:"workspace_context"`
	Stats         TraceStats         `json:"stats"`
	Warnings      []TraceWarning     `json:"warnings,omitempty"`
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

type ExchangeEvidence struct {
	Index         int                `json:"index"`
	AnchorID      string             `json:"anchor_id"`
	Start         time.Time          `json:"start"`
	End           time.Time          `json:"end"`
	UserText      string             `json:"user_text"`
	AssistantText string             `json:"assistant_text"`
	FilesEdited   []string           `json:"files_edited,omitempty"`
	FilesRead     []string           `json:"files_read,omitempty"`
	FilesTouched  []string           `json:"files_touched,omitempty"`
	ToolsUsed     []string           `json:"tools_used,omitempty"`
	ToolCalls     []ToolCallEvidence `json:"tool_calls,omitempty"`
	Signals       []EvidenceSignal   `json:"signals,omitempty"`
	HasCorrection bool               `json:"has_correction"`
	HasThinking   bool               `json:"has_thinking"`
	IsCommand     bool               `json:"is_command"`
	CommandName   string             `json:"command_name,omitempty"`
	Sidechains    []Sidechain        `json:"sidechains,omitempty"`

	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`

	LinkedCommits []string `json:"linked_commits,omitempty"`
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

type EvidenceSignal struct {
	Kind       string   `json:"kind"`
	Confidence string   `json:"confidence"`
	Summary    string   `json:"summary"`
	Evidence   []string `json:"evidence,omitempty"`
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
	RepoRoot            string               `json:"repo_root,omitempty"`
	Branch              string               `json:"branch,omitempty"`
	Head                string               `json:"head,omitempty"`
	Dirty               bool                 `json:"dirty"`
	UncommittedFiles    []GitFileStatus      `json:"uncommitted_files,omitempty"`
	UncommittedStat     string               `json:"uncommitted_stat,omitempty"`
	Commits             []GitCommit          `json:"commits"`
	ExchangeCommitLinks []ExchangeCommitLink `json:"exchange_commit_links"`
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

type ExchangeCommitLink struct {
	ExchangeIndex int      `json:"exchange_index"`
	CommitSHA     string   `json:"commit_sha"`
	FileOverlap   []string `json:"file_overlap"`
	Confidence    string   `json:"confidence,omitempty"`
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
	ExchangeCount     int     `json:"exchange_count"`
	CorrectionSignals int     `json:"correction_signals"`
	FilesEdited       int     `json:"files_edited"`
	FilesRead         int     `json:"files_read"`
	ToolsUsed         int     `json:"tools_used"`
	WorkspaceDocs     int     `json:"workspace_docs"`
	KnowledgeEntries  int     `json:"knowledge_entries"`
	CommitsLinked     int     `json:"commits_linked"`
	UncommittedFiles  int     `json:"uncommitted_files"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	DurationSecs      float64 `json:"duration_seconds"`
	HasSidechains     bool    `json:"has_sidechains"`
}
