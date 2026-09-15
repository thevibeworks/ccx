package insight

import (
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
	"github.com/thevibeworks/ccx/internal/sessionlog"
)

// Kind is the schema tag of the insight report.
const Kind = "ccx.insight.v1"

// Report is what `ccx insight --json` emits: a digest of every session
// active in the window, small enough for a skill to read whole, with
// the raw records available on request (--records). It answers "what
// was worked on, where, at what cost, and what did the human say"
// without a consumer re-bucketing hundreds of thousands of records.
//
// Design rule (docs/design/0005): ccx = facts, skills = judgment. The
// digest carries prompts, final answers, edits, commits, interventions,
// tokens and cost per session — the evidence a recap needs — and no
// interpretation of any of it.
type Report struct {
	Kind        string                  `json:"kind"`
	Scope       sessionlog.ScopeSummary `json:"scope"`
	GeneratedAt time.Time               `json:"generated_at"`
	// Command re-runs the evidence collection exactly as it was run.
	Command    string              `json:"evidence_command,omitempty"`
	Metrics    Metrics             `json:"metrics"`
	Models     []ModelStats        `json:"models"`
	Days       []Bucket            `json:"days"`
	Providers  []Bucket            `json:"providers"`
	Workspaces []WorkspaceStats    `json:"workspaces"`
	Sessions   []SessionDigest     `json:"sessions"`
	Records    []sessionlog.Record `json:"records,omitempty"`
	Warnings   []string            `json:"warnings,omitempty"`
}

// Metrics are scope-wide counts. Sessions counts logical sessions (a
// Claude session plus its subagent files is one session); SourceFiles
// counts the JSONL files behind them.
type Metrics struct {
	Sessions            int `json:"sessions"`
	SourceFiles         int `json:"source_files"`
	LongRunningSessions int `json:"long_running_sessions"`
	Workspaces          int `json:"workspaces"`
	Records             int `json:"records"`
	UserPrompts         int `json:"user_prompts"`
	AssistantMessages   int `json:"assistant_messages"`
	ToolCalls           int `json:"tool_calls"`
	ToolResults         int `json:"tool_results"`
	Reasoning           int `json:"reasoning"`
	Sidechains          int `json:"sidechains"`
	Edits               int `json:"edits"`
	Commits             int `json:"commits"`
	Interrupts          int `json:"interrupts"`
	Denials             int `json:"denials"`
	// Cost is summed over the sessions active in the window (whole
	// sessions — a session that started before the window contributes
	// its full cost; Relation on the session says so). CostCoverage is
	// priced tokens / all tokens: 1.0 means every token had a pricing
	// row, 0.39 means the dollar figure covers 39% of the tokens.
	Tokens           int     `json:"tokens"`
	CostUSD          float64 `json:"cost_usd"`
	UnpricedTokens   int     `json:"unpriced_tokens"`
	CostCoverage     float64 `json:"cost_coverage"`
	PricedSessions   int     `json:"priced_sessions"`
	UnpricedSessions int     `json:"unpriced_sessions"`
	RecordsReturned  int     `json:"records_returned,omitempty"`
	Truncated        bool    `json:"truncated,omitempty"`
}

// ModelStats is one model's share of the window.
type ModelStats struct {
	Model    string  `json:"model"`
	Sessions int     `json:"sessions"`
	Tokens   int     `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
	Priced   bool    `json:"priced"`
}

// Bucket is one day (in the scope's timezone), one provider, or the
// base of a workspace. Record counts are per record; Sessions and
// CostUSD are per session, and a session's cost lands on the day of
// its first record in the window so days sum to the total.
type Bucket struct {
	Key               string  `json:"key"`
	Sessions          int     `json:"sessions"`
	Records           int     `json:"records"`
	UserPrompts       int     `json:"user_prompts"`
	AssistantMessages int     `json:"assistant_messages"`
	ToolCalls         int     `json:"tool_calls"`
	Sidechains        int     `json:"sidechains"`
	Edits             int     `json:"edits"`
	Commits           int     `json:"commits"`
	Interrupts        int     `json:"interrupts"`
	Denials           int     `json:"denials"`
	CostUSD           float64 `json:"cost_usd"`
	UnpricedTokens    int     `json:"unpriced_tokens,omitempty"`
}

// WorkspaceStats is one workspace's share of the window plus the ids of
// its sessions (sorted by first record) so a consumer can walk from
// the board to the digests without a second pass.
type WorkspaceStats struct {
	Bucket
	Path       string   `json:"path"`
	Providers  []string `json:"providers"`
	Days       int      `json:"days"`
	SessionIDs []string `json:"session_ids"`
}

// Prompt is a cited piece of conversation text: when, which line of
// which file (the SourceFile of the session), and the bounded text.
type Prompt struct {
	Time time.Time `json:"time"`
	Line int       `json:"line"`
	Text string    `json:"text"`
}

// SessionDigest is one logical session's evidence for the window.
type SessionDigest struct {
	ID                string                   `json:"id"`
	Provider          string                   `json:"provider"`
	Project           string                   `json:"project,omitempty"`
	Workspace         string                   `json:"workspace,omitempty"`
	SourceFile        string                   `json:"source_file"`
	Model             string                   `json:"model,omitempty"`
	Summary           string                   `json:"summary,omitempty"`
	Start             time.Time                `json:"start"`
	End               time.Time                `json:"end"`
	FirstRecord       time.Time                `json:"first_record"`
	LastRecord        time.Time                `json:"last_record"`
	Relation          sessionlog.ScopeRelation `json:"relation"`
	SourceFiles       int                      `json:"source_files"`
	Records           int                      `json:"records"`
	UserPrompts       int                      `json:"user_prompts"`
	AssistantMessages int                      `json:"assistant_messages"`
	ToolCalls         int                      `json:"tool_calls"`
	Sidechains        int                      `json:"sidechains"`
	Edits             int                      `json:"edits"`
	Commits           int                      `json:"commits"`
	Interrupts        int                      `json:"interrupts"`
	Denials           int                      `json:"denials"`
	// Prompts are the human's words in the window (main thread only,
	// never a subagent's instruction): the first promptHead and the
	// last promptTail, PromptsOmitted saying how many sit between.
	// FinalAnswer is the last assistant text in the window — the
	// agent's own claim of where things stand, which the recap treats
	// as input, not truth.
	Prompts        []Prompt `json:"prompts"`
	PromptsOmitted int      `json:"prompts_omitted,omitempty"`
	FinalAnswer    *Prompt  `json:"final_answer,omitempty"`
	// EditedPaths are the distinct workspace paths the agent's Edit /
	// Write / apply_patch calls targeted, capped; Truncated keeps the
	// count honest.
	EditedPaths          []string `json:"edited_paths,omitempty"`
	EditedPathsTruncated bool     `json:"edited_paths_truncated,omitempty"`
	// Whole-session usage from the provider's own accounting (not
	// sliced to the window: token counts are per message and cost is
	// per session; Relation says whether the session spills over).
	Tokens         int     `json:"tokens"`
	CostUSD        float64 `json:"cost_usd"`
	UnpricedTokens int     `json:"unpriced_tokens,omitempty"`
	CostStatus     string  `json:"cost_status,omitempty"`
}

const (
	promptHead     = 6
	promptTail     = 2
	promptTextMax  = 280
	answerTextMax  = 400
	editedPathsMax = 12
)

// SessionLookup returns the provider's parsed session (model, summary,
// usage) for an id, or nil when the id is unknown.
type SessionLookup func(id string) *parser.Session

// BuildReport digests a sessionlog bundle. lookup may be nil, in which
// case model/summary/cost stay empty and CostStatus is "" (not
// "unpriced": absence of accounting is not the same as unpriced usage).
func BuildReport(bundle *sessionlog.Bundle, lookup SessionLookup, includeRecords bool, recordLimit int) *Report {
	loc := scopeLocation(bundle)
	report := &Report{
		Kind:        Kind,
		Scope:       bundle.Scope,
		GeneratedAt: bundle.GeneratedAt,
	}

	// Group source files into logical sessions by id: a Claude session
	// and its subagent transcripts share the session id.
	order := []string{}
	digests := map[string]*SessionDigest{}
	for _, s := range bundle.Sessions {
		d, ok := digests[s.ID]
		if !ok {
			d = &SessionDigest{
				ID:          s.ID,
				Provider:    s.Provider,
				Project:     s.Project,
				Workspace:   s.Workspace,
				SourceFile:  s.SourceFile,
				Start:       s.Start,
				End:         s.End,
				FirstRecord: s.FirstRecord,
				LastRecord:  s.LastRecord,
				Relation:    s.Relation,
				Prompts:     []Prompt{},
			}
			digests[s.ID] = d
			order = append(order, s.ID)
		}
		d.SourceFiles++
		d.Records += s.Records
		if isSidechainFile(s.SourceFile) {
			d.Sidechains += s.Records
		} else {
			// The main transcript wins for identity fields when a
			// subagent file was scanned first.
			d.SourceFile = s.SourceFile
			if d.Workspace == "" {
				d.Workspace = s.Workspace
				d.Project = s.Project
			}
		}
		if s.Start.Before(d.Start) || d.Start.IsZero() {
			d.Start = s.Start
		}
		if s.End.After(d.End) {
			d.End = s.End
		}
		if s.FirstRecord.Before(d.FirstRecord) || d.FirstRecord.IsZero() {
			d.FirstRecord = s.FirstRecord
		}
		if s.LastRecord.After(d.LastRecord) {
			d.LastRecord = s.LastRecord
		}
		d.Relation = mergeRelation(d.Relation, s.Relation)
	}

	// Walk the records once: prompts, answers, edits, commits,
	// interventions per session.
	prompts := map[string][]Prompt{}
	editedPaths := map[string]map[string]struct{}{}
	for i := range bundle.Records {
		r := &bundle.Records[i]
		d := digests[r.SessionID]
		if d == nil {
			continue
		}
		mainThread := !r.IsSidechain && !r.IsSubagent && !isSidechainFile(r.SourceFile)
		switch r.Kind {
		case "user_prompt":
			if mainThread {
				d.UserPrompts++
				prompts[r.SessionID] = append(prompts[r.SessionID], Prompt{Time: r.Timestamp, Line: r.Line, Text: truncate(r.Text, promptTextMax)})
			}
		case "assistant_message":
			if mainThread {
				d.AssistantMessages++
				if r.Text != "" {
					d.FinalAnswer = &Prompt{Time: r.Timestamp, Line: r.Line, Text: truncate(r.Text, answerTextMax)}
				}
			}
		case "tool_call":
			d.ToolCalls++
			if isEditTool(r.Tool) {
				d.Edits++
				if r.Path != "" {
					set := editedPaths[r.SessionID]
					if set == nil {
						set = map[string]struct{}{}
						editedPaths[r.SessionID] = set
					}
					set[r.Path] = struct{}{}
				}
			}
			if isCommit(r) {
				d.Commits++
			}
		case "interrupt":
			d.Interrupts++
		case "tool_denied":
			d.Denials++
		}
	}
	for id, list := range prompts {
		d := digests[id]
		if len(list) <= promptHead+promptTail {
			d.Prompts = list
			continue
		}
		d.Prompts = append(append([]Prompt{}, list[:promptHead]...), list[len(list)-promptTail:]...)
		d.PromptsOmitted = len(list) - promptHead - promptTail
	}
	for id, set := range editedPaths {
		d := digests[id]
		paths := make([]string, 0, len(set))
		for p := range set {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		if len(paths) > editedPathsMax {
			paths = paths[:editedPathsMax]
			d.EditedPathsTruncated = true
		}
		d.EditedPaths = paths
	}

	// Provider accounting: model, summary, tokens, cost.
	models := map[string]*ModelStats{}
	for _, id := range order {
		d := digests[id]
		if lookup == nil {
			continue
		}
		s := lookup(id)
		if s == nil {
			continue
		}
		d.Model = s.Model
		d.Summary = s.Summary
		d.Tokens = s.Stats.TotalTokens()
		d.CostUSD = s.Stats.CostUSD
		d.UnpricedTokens = s.Stats.UnpricedTokens
		d.CostStatus = s.Stats.CostStatus()
		if d.Workspace == "" && s.CWD != "" {
			d.Workspace = s.CWD
			d.Project = s.ProjectName
		}
		key := s.Model
		if key == "" {
			key = "(unknown)"
		}
		m, ok := models[key]
		if !ok {
			m = &ModelStats{Model: key, Priced: true}
			models[key] = m
		}
		m.Sessions++
		m.Tokens += d.Tokens
		m.CostUSD += d.CostUSD
		if d.CostStatus == "unpriced" || d.CostStatus == "partial" {
			m.Priced = false
		}
	}

	// Assemble: sessions in first-record order, buckets, metrics.
	sort.SliceStable(order, func(i, j int) bool {
		return digests[order[i]].FirstRecord.Before(digests[order[j]].FirstRecord)
	})
	sessionsByWorkspace := map[string][]string{}
	for _, id := range order {
		d := digests[id]
		report.Sessions = append(report.Sessions, *d)
		sessionsByWorkspace[d.Workspace] = append(sessionsByWorkspace[d.Workspace], id)
	}
	report.Days, report.Providers, report.Workspaces = buckets(bundle, digests, loc)
	for i := range report.Workspaces {
		report.Workspaces[i].SessionIDs = sessionsByWorkspace[report.Workspaces[i].Path]
	}
	for _, m := range models {
		report.Models = append(report.Models, *m)
	}
	sort.Slice(report.Models, func(i, j int) bool {
		if report.Models[i].Tokens != report.Models[j].Tokens {
			return report.Models[i].Tokens > report.Models[j].Tokens
		}
		return report.Models[i].Model < report.Models[j].Model
	})
	report.Metrics = metrics(bundle, report)
	if includeRecords {
		report.Records = bundle.Records
		if recordLimit > 0 && len(report.Records) > recordLimit {
			report.Records = report.Records[:recordLimit]
		}
		report.Metrics.RecordsReturned = len(report.Records)
		report.Metrics.Truncated = bundle.Metrics.Truncated || len(report.Records) < len(bundle.Records)
	}
	if lookup == nil {
		report.Warnings = append(report.Warnings, "no provider accounting joined: model, summary, and cost are empty")
	}
	return report
}

func metrics(bundle *sessionlog.Bundle, report *Report) Metrics {
	m := Metrics{
		Sessions:            len(report.Sessions),
		SourceFiles:         bundle.Metrics.SourceFiles,
		LongRunningSessions: 0,
		Workspaces:          len(report.Workspaces),
		Records:             bundle.Metrics.Records,
		UserPrompts:         0,
		AssistantMessages:   bundle.Metrics.AssistantMessages,
		ToolCalls:           bundle.Metrics.ToolCalls,
		ToolResults:         bundle.Metrics.ToolResults,
		Reasoning:           bundle.Metrics.Reasoning,
		Sidechains:          bundle.Metrics.Sidechains,
	}
	var pricedTokens int
	for _, d := range report.Sessions {
		if d.Relation.StartedBeforeScope || d.Relation.EndedAfterScope || d.Relation.SpansWholeScope {
			m.LongRunningSessions++
		}
		m.UserPrompts += d.UserPrompts
		m.Edits += d.Edits
		m.Commits += d.Commits
		m.Interrupts += d.Interrupts
		m.Denials += d.Denials
		m.Tokens += d.Tokens
		m.CostUSD += d.CostUSD
		m.UnpricedTokens += d.UnpricedTokens
		pricedTokens += d.Tokens - d.UnpricedTokens
		switch d.CostStatus {
		case "priced", "partial":
			m.PricedSessions++
		case "unpriced":
			m.UnpricedSessions++
		}
	}
	if m.Tokens > 0 {
		m.CostCoverage = float64(pricedTokens) / float64(m.Tokens)
	}
	return m
}

func buckets(bundle *sessionlog.Bundle, digests map[string]*SessionDigest, loc *time.Location) (days, providers []Bucket, workspaces []WorkspaceStats) {
	dayMap := map[string]*Bucket{}
	provMap := map[string]*Bucket{}
	wsMap := map[string]*WorkspaceStats{}
	daySessions := map[string]map[string]struct{}{}
	wsDays := map[string]map[string]struct{}{}
	wsProviders := map[string]map[string]struct{}{}

	bump := func(b *Bucket, r *sessionlog.Record) {
		b.Records++
		switch r.Kind {
		case "user_prompt":
			if !r.IsSidechain && !r.IsSubagent && !isSidechainFile(r.SourceFile) {
				b.UserPrompts++
			}
		case "assistant_message":
			b.AssistantMessages++
		case "tool_call":
			b.ToolCalls++
			if isEditTool(r.Tool) {
				b.Edits++
			}
			if isCommit(r) {
				b.Commits++
			}
		case "interrupt":
			b.Interrupts++
		case "tool_denied":
			b.Denials++
		}
		if isSidechainFile(r.SourceFile) {
			b.Sidechains++
		}
	}
	for i := range bundle.Records {
		r := &bundle.Records[i]
		d := digests[r.SessionID]
		ws := r.Workspace
		if d != nil && d.Workspace != "" {
			ws = d.Workspace
		}
		day := r.Timestamp.In(loc).Format("2006-01-02")
		if _, ok := dayMap[day]; !ok {
			dayMap[day] = &Bucket{Key: day}
			daySessions[day] = map[string]struct{}{}
		}
		bump(dayMap[day], r)
		daySessions[day][r.SessionID] = struct{}{}

		if _, ok := provMap[r.Provider]; !ok {
			provMap[r.Provider] = &Bucket{Key: r.Provider}
		}
		bump(provMap[r.Provider], r)

		if _, ok := wsMap[ws]; !ok {
			wsMap[ws] = &WorkspaceStats{Bucket: Bucket{Key: projectKey(ws, r.Project)}, Path: ws}
			wsDays[ws] = map[string]struct{}{}
			wsProviders[ws] = map[string]struct{}{}
		}
		bump(&wsMap[ws].Bucket, r)
		wsDays[ws][day] = struct{}{}
		wsProviders[ws][r.Provider] = struct{}{}
	}
	// Session counts and whole-session cost attach to the buckets a
	// session belongs to: its workspace and provider always, and — so
	// the day table sums to the total instead of counting a multi-day
	// session on every day — the day of its first record in the window.
	provSessions := map[string]int{}
	for _, d := range digests {
		if b := provMap[d.Provider]; b != nil {
			provSessions[d.Provider]++
			b.CostUSD += d.CostUSD
			b.UnpricedTokens += d.UnpricedTokens
		}
		if w := wsMap[d.Workspace]; w != nil {
			w.Sessions++
			w.CostUSD += d.CostUSD
			w.UnpricedTokens += d.UnpricedTokens
		}
		if b := dayMap[d.FirstRecord.In(loc).Format("2006-01-02")]; b != nil {
			b.CostUSD += d.CostUSD
			b.UnpricedTokens += d.UnpricedTokens
		}
	}
	for key, b := range provMap {
		b.Sessions = provSessions[key]
	}
	for day, b := range dayMap {
		b.Sessions = len(daySessions[day])
		days = append(days, *b)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Key < days[j].Key })
	for _, b := range provMap {
		providers = append(providers, *b)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Key < providers[j].Key })
	for path, w := range wsMap {
		w.Days = len(wsDays[path])
		for p := range wsProviders[path] {
			w.Providers = append(w.Providers, p)
		}
		sort.Strings(w.Providers)
		workspaces = append(workspaces, *w)
	}
	sort.Slice(workspaces, func(i, j int) bool {
		if workspaces[i].Records != workspaces[j].Records {
			return workspaces[i].Records > workspaces[j].Records
		}
		return workspaces[i].Path < workspaces[j].Path
	})
	return days, providers, workspaces
}

func projectKey(workspace, project string) string {
	if project != "" {
		return project
	}
	if workspace == "" {
		return "(unknown)"
	}
	if idx := strings.LastIndex(workspace, "/"); idx >= 0 && idx < len(workspace)-1 {
		return workspace[idx+1:]
	}
	return workspace
}

func mergeRelation(a, b sessionlog.ScopeRelation) sessionlog.ScopeRelation {
	return sessionlog.ScopeRelation{
		OverlapsScope:      a.OverlapsScope || b.OverlapsScope,
		StartedBeforeScope: a.StartedBeforeScope || b.StartedBeforeScope,
		StartedInScope:     a.StartedInScope || b.StartedInScope,
		EndedInScope:       a.EndedInScope || b.EndedInScope,
		EndedAfterScope:    a.EndedAfterScope || b.EndedAfterScope,
		SpansWholeScope:    a.SpansWholeScope || b.SpansWholeScope,
	}
}

func isSidechainFile(path string) bool {
	return strings.Contains(path, "/subagents/")
}

// isEditTool names the tools that change workspace files on each
// provider: Claude's Edit family and Codex's apply_patch.
func isEditTool(tool string) bool {
	switch tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "apply_patch":
		return true
	}
	return false
}

// isCommit is a shell tool call that ran git commit.
func isCommit(r *sessionlog.Record) bool {
	switch r.Tool {
	case "Bash", "exec_command", "shell", "container.exec", "local_shell":
	default:
		return false
	}
	return strings.Contains(r.Text, "git commit")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "..."
}
