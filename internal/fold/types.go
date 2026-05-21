package fold

import "time"

type FoldResult struct {
	Session  SessionMeta  `json:"session"`
	Turns    []Turn       `json:"turns"`
	Git      GitCorrelation `json:"git"`
	Stats    FoldStats    `json:"stats"`
}

type SessionMeta struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	Model     string    `json:"model"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	CWD       string    `json:"cwd"`
	GitBranch string    `json:"git_branch"`
}

type Turn struct {
	Index         int       `json:"index"`
	AnchorID      string    `json:"anchor_id"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	UserText      string    `json:"user_text"`
	AssistantText string    `json:"assistant_text"`
	FilesEdited   []string  `json:"files_edited,omitempty"`
	FilesRead     []string  `json:"files_read,omitempty"`
	ToolsUsed     []string  `json:"tools_used,omitempty"`
	HasCorrection bool      `json:"has_correction"`
	HasThinking   bool      `json:"has_thinking"`
	IsCommand     bool      `json:"is_command"`
	CommandName   string    `json:"command_name,omitempty"`
	Sidechain     *Sidechain `json:"sidechain,omitempty"`

	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`

	LinkedCommits []string `json:"linked_commits,omitempty"`
}

type Sidechain struct {
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
	Summary   string `json:"summary"`
	ToolCalls int    `json:"tool_calls"`
}

type GitCorrelation struct {
	Commits        []GitCommit     `json:"commits"`
	TurnCommitLinks []TurnCommitLink `json:"turn_commit_links"`
}

type GitCommit struct {
	SHA       string   `json:"sha"`
	Timestamp string   `json:"timestamp"`
	Subject   string   `json:"subject"`
	Files     []string `json:"files"`
}

type TurnCommitLink struct {
	TurnIndex   int      `json:"turn_index"`
	CommitSHA   string   `json:"commit_sha"`
	FileOverlap []string `json:"file_overlap"`
}

type FoldStats struct {
	TurnCount     int     `json:"turn_count"`
	Corrections   int     `json:"corrections"`
	FilesEdited   int     `json:"files_edited"`
	CommitsLinked int     `json:"commits_linked"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	DurationSecs  float64 `json:"duration_seconds"`
	HasSidechains bool    `json:"has_sidechains"`
}
