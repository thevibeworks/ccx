package insight

import "time"

type Scope string

const (
	ScopeToday     Scope = "today"
	ScopeYesterday Scope = "yesterday"
	ScopeWeek      Scope = "week"
	ScopeMonth     Scope = "month"
	ScopeQuarter   Scope = "quarter"
	ScopeYear      Scope = "year"
)

type Options struct {
	Scope    Scope
	Location *time.Location
	Now      time.Time
	Limit    int
	Provider string
	Project  string
}

type Summary struct {
	Kind         string       `json:"kind"`
	Scope        ScopeSummary `json:"scope"`
	GeneratedAt  time.Time    `json:"generated_at"`
	Metrics      Metrics      `json:"metrics"`
	Current      []SessionRef `json:"current_work"`
	OpenLoops    []Signal     `json:"open_loops"`
	Completed    []SessionRef `json:"completed"`
	Achievements []Signal     `json:"achievements"`
	Patterns     []Signal     `json:"patterns"`
	Projects     []ProjectRow `json:"projects"`
	Providers    []MetricRow  `json:"providers"`
	Models       []MetricRow  `json:"models"`
}

type ScopeSummary struct {
	Name      string    `json:"name"`
	Label     string    `json:"label"`
	TimeZone  string    `json:"time_zone"`
	Provider  string    `json:"provider,omitempty"`
	Project   string    `json:"project,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Generated string    `json:"generated"`
}

type Metrics struct {
	Sessions        int     `json:"sessions"`
	Projects        int     `json:"projects"`
	Messages        int     `json:"messages"`
	UserPrompts     int     `json:"user_prompts"`
	ToolCalls       int     `json:"tool_calls"`
	Sidechains      int     `json:"sidechains"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	CacheTokens     int     `json:"cache_tokens"`
	TotalTokens     int     `json:"total_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type SessionRef struct {
	ID              string    `json:"id"`
	Provider        string    `json:"provider"`
	Project         string    `json:"project"`
	ProjectPath     string    `json:"project_path,omitempty"`
	Summary         string    `json:"summary"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	Model           string    `json:"model,omitempty"`
	Messages        int       `json:"messages"`
	UserPrompts     int       `json:"user_prompts"`
	ToolCalls       int       `json:"tool_calls"`
	Sidechains      int       `json:"sidechains"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	CacheTokens     int       `json:"cache_tokens"`
	Tokens          int       `json:"tokens"`
	CostUSD         float64   `json:"cost_usd,omitempty"`
	DurationSeconds float64   `json:"duration_seconds"`
}

type Signal struct {
	Label       string   `json:"label"`
	Summary     string   `json:"summary"`
	Count       int      `json:"count,omitempty"`
	Confidence  string   `json:"confidence"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type ProjectRow struct {
	Name     string  `json:"name"`
	Path     string  `json:"path,omitempty"`
	Sessions int     `json:"sessions"`
	Messages int     `json:"messages"`
	Tools    int     `json:"tools"`
	Tokens   int     `json:"tokens"`
	CostUSD  float64 `json:"cost_usd,omitempty"`
	Latest   string  `json:"latest"`
}

type MetricRow struct {
	Name     string  `json:"name"`
	Sessions int     `json:"sessions"`
	Messages int     `json:"messages"`
	Tools    int     `json:"tools"`
	Tokens   int     `json:"tokens"`
	CostUSD  float64 `json:"cost_usd,omitempty"`
}
