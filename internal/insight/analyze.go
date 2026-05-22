package insight

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/parser"
)

var openLoopPattern = regexp.MustCompile(`(?i)\b(todo|wip|follow[- ]?up|next|remaining|blocked|blocker|fixme|not yet|need(s|ed)?|pending|review|failing|failed|error|bug|issue|broken|missing)\b`)
var completionPattern = regexp.MustCompile(`(?i)\b(done|completed|complete|finished|shipped|implemented|fixed|resolved|landed|released|merged|passed|working)\b`)

func Analyze(sessions []*parser.Session, opts Options) *Summary {
	scope := opts.Scope
	if scope == "" {
		scope = ScopeToday
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 8
	}

	start, end, label := ScopeWindow(scope, now, loc)
	filtered := filterSessions(sessions, start, end)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].End.After(filtered[j].End)
	})

	summary := &Summary{
		Kind:        Kind,
		GeneratedAt: now.In(loc),
		Scope: ScopeSummary{
			Name:      string(scope),
			Label:     label,
			TimeZone:  loc.String(),
			Provider:  opts.Provider,
			Project:   opts.Project,
			Start:     start,
			End:       end,
			Generated: now.In(loc).Format(time.RFC3339),
		},
		Metrics:      aggregateMetrics(filtered),
		Current:      latestSessions(filtered, limit),
		OpenLoops:    openLoopSignals(filtered, limit),
		Completed:    completedSessions(filtered, limit),
		Achievements: achievementSignals(filtered, limit),
		Patterns:     patternSignals(filtered, limit),
		Projects:     projectRows(filtered, limit),
		Providers:    metricRows(filtered, func(s SessionRef) string { return providerLabel(s.Provider) }),
		Models: metricRows(filtered, func(s SessionRef) string {
			if s.Model == "" {
				return "unknown"
			}
			return s.Model
		}),
	}
	return summary
}

func filterSessions(sessions []*parser.Session, start, end time.Time) []SessionRef {
	var refs []SessionRef
	for _, s := range sessions {
		if s == nil {
			continue
		}
		sessionEnd := s.EndTime
		if sessionEnd.IsZero() {
			sessionEnd = s.StartTime
		}
		if sessionEnd.Before(start) || !sessionEnd.Before(end) {
			continue
		}
		refs = append(refs, sessionRef(s))
	}
	return refs
}

func sessionRef(s *parser.Session) SessionRef {
	cacheTokens := s.Stats.CacheReadTokens + s.Stats.CacheCreateTokens
	tokens := s.Stats.InputTokens + s.Stats.OutputTokens + cacheTokens
	return SessionRef{
		ID:              s.ID,
		Provider:        s.Provider,
		Project:         projectLabel(s.ProjectName),
		ProjectPath:     s.CWD,
		Summary:         truncateText(cleanSummary(s.Summary), 500),
		Start:           s.StartTime,
		End:             s.EndTime,
		Model:           s.Model,
		Messages:        s.Stats.MessageCount,
		UserPrompts:     s.Stats.UserPrompts,
		ToolCalls:       s.Stats.ToolCalls,
		Sidechains:      s.Stats.AgentSidechains,
		InputTokens:     s.Stats.InputTokens,
		OutputTokens:    s.Stats.OutputTokens,
		CacheTokens:     cacheTokens,
		Tokens:          tokens,
		CostUSD:         s.Stats.CostUSD,
		DurationSeconds: s.Stats.DurationSeconds,
	}
}

func aggregateMetrics(sessions []SessionRef) Metrics {
	projects := make(map[string]struct{})
	var m Metrics
	for _, s := range sessions {
		m.Sessions++
		if s.Project != "" {
			projects[s.Project] = struct{}{}
		}
		m.Messages += s.Messages
		m.UserPrompts += s.UserPrompts
		m.ToolCalls += s.ToolCalls
		m.Sidechains += s.Sidechains
		m.InputTokens += s.InputTokens
		m.OutputTokens += s.OutputTokens
		m.CacheTokens += s.CacheTokens
		m.TotalTokens += s.Tokens
		m.CostUSD += s.CostUSD
		if s.DurationSeconds > 0 {
			m.DurationSeconds += s.DurationSeconds
		} else if !s.Start.IsZero() && !s.End.IsZero() && s.End.After(s.Start) {
			m.DurationSeconds += s.End.Sub(s.Start).Seconds()
		}
	}
	m.Projects = len(projects)
	return m
}

func latestSessions(sessions []SessionRef, limit int) []SessionRef {
	return takeSessions(sessions, limit)
}

func completedSessions(sessions []SessionRef, limit int) []SessionRef {
	var out []SessionRef
	for _, s := range sessions {
		if completionPattern.MatchString(s.Summary) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = takeSessions(sessions, min(limit, 5))
	}
	return takeSessions(out, limit)
}

func openLoopSignals(sessions []SessionRef, limit int) []Signal {
	var signals []Signal
	for _, s := range sessions {
		if !openLoopPattern.MatchString(s.Summary) {
			continue
		}
		signals = append(signals, Signal{
			Label:       "Needs closure",
			Summary:     truncateText(s.Summary, 220),
			Count:       1,
			Confidence:  "medium",
			EvidenceIDs: []string{shortID(s.ID)},
		})
	}
	if len(signals) == 0 && len(sessions) > 0 {
		signals = append(signals, Signal{
			Label:       "Review latest work",
			Summary:     "No explicit blocker language found. Review the newest sessions for implicit follow-up.",
			Confidence:  "low",
			EvidenceIDs: idsFor(takeSessions(sessions, min(3, limit))),
		})
	}
	return takeSignals(signals, limit)
}

func achievementSignals(sessions []SessionRef, limit int) []Signal {
	projectCounts := make(map[string]int)
	for _, s := range sessions {
		if completionPattern.MatchString(s.Summary) {
			projectCounts[s.Project]++
		}
	}
	var signals []Signal
	for project, count := range projectCounts {
		signals = append(signals, Signal{
			Label:      "Completed work",
			Summary:    project,
			Count:      count,
			Confidence: "medium",
		})
	}
	sort.SliceStable(signals, func(i, j int) bool { return signals[i].Count > signals[j].Count })
	return takeSignals(signals, limit)
}

func patternSignals(sessions []SessionRef, limit int) []Signal {
	var signals []Signal
	if len(sessions) == 0 {
		return signals
	}
	if hot := topProjectSignal(sessions); hot != nil {
		signals = append(signals, *hot)
	}
	if tools := toolIntensitySignal(sessions); tools != nil {
		signals = append(signals, *tools)
	}
	if providers := providerMixSignal(sessions); providers != nil {
		signals = append(signals, *providers)
	}
	if models := modelSignal(sessions); models != nil {
		signals = append(signals, *models)
	}
	return takeSignals(signals, limit)
}

func topProjectSignal(sessions []SessionRef) *Signal {
	counts := make(map[string]int)
	for _, s := range sessions {
		counts[s.Project]++
	}
	var top string
	var n int
	for project, count := range counts {
		if count > n {
			top, n = project, count
		}
	}
	if top == "" || n < 2 {
		return nil
	}
	return &Signal{Label: "Focused project", Summary: top, Count: n, Confidence: "high"}
}

func toolIntensitySignal(sessions []SessionRef) *Signal {
	var tools, messages int
	for _, s := range sessions {
		tools += s.ToolCalls
		messages += s.Messages
	}
	if tools == 0 || messages == 0 {
		return nil
	}
	ratio := float64(tools) / float64(messages)
	if ratio < 0.8 {
		return nil
	}
	return &Signal{Label: "Tool-heavy work", Summary: "Sessions are execution-heavy; verify outputs and close test loops.", Count: tools, Confidence: "medium"}
}

func providerMixSignal(sessions []SessionRef) *Signal {
	seen := make(map[string]struct{})
	for _, s := range sessions {
		if s.Provider != "" {
			seen[s.Provider] = struct{}{}
		}
	}
	if len(seen) < 2 {
		return nil
	}
	return &Signal{Label: "Multi-agent workflow", Summary: "Claude Code and Codex both contributed in this scope.", Count: len(seen), Confidence: "high"}
}

func modelSignal(sessions []SessionRef) *Signal {
	counts := make(map[string]int)
	for _, s := range sessions {
		if s.Model != "" {
			counts[s.Model]++
		}
	}
	var top string
	var n int
	for model, count := range counts {
		if count > n {
			top, n = model, count
		}
	}
	if top == "" {
		return nil
	}
	return &Signal{Label: "Dominant model", Summary: top, Count: n, Confidence: "high"}
}

func projectRows(sessions []SessionRef, limit int) []ProjectRow {
	rows := make(map[string]ProjectRow)
	for _, s := range sessions {
		key := s.Project
		row := rows[key]
		row.Name = s.Project
		row.Path = s.ProjectPath
		row.Sessions++
		row.Messages += s.Messages
		row.Tools += s.ToolCalls
		row.Tokens += s.Tokens
		row.CostUSD += s.CostUSD
		if row.Latest == "" || s.End.After(parseTime(row.Latest)) {
			row.Latest = s.End.Format(time.RFC3339)
		}
		rows[key] = row
	}
	out := make([]ProjectRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sessions != out[j].Sessions {
			return out[i].Sessions > out[j].Sessions
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func metricRows(sessions []SessionRef, nameFor func(SessionRef) string) []MetricRow {
	rows := make(map[string]MetricRow)
	for _, s := range sessions {
		name := nameFor(s)
		if strings.TrimSpace(name) == "" {
			name = "unknown"
		}
		row := rows[name]
		row.Name = name
		row.Sessions++
		row.Messages += s.Messages
		row.Tools += s.ToolCalls
		row.Tokens += s.Tokens
		row.CostUSD += s.CostUSD
		rows[name] = row
	}
	out := make([]MetricRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sessions != out[j].Sessions {
			return out[i].Sessions > out[j].Sessions
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func takeSessions(sessions []SessionRef, limit int) []SessionRef {
	if limit > 0 && len(sessions) > limit {
		return append([]SessionRef(nil), sessions[:limit]...)
	}
	return append([]SessionRef(nil), sessions...)
}

func takeSignals(signals []Signal, limit int) []Signal {
	if limit > 0 && len(signals) > limit {
		return append([]Signal(nil), signals[:limit]...)
	}
	return append([]Signal(nil), signals...)
}

func idsFor(sessions []SessionRef) []string {
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, shortID(s.ID))
	}
	return ids
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func providerLabel(provider string) string {
	switch provider {
	case "claude-code":
		return "Claude Code"
	case "codex":
		return "Codex"
	default:
		return "unknown"
	}
}

func projectLabel(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return "unknown"
	}
	return parser.GetProjectDisplayName(project)
}

func cleanSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	summary = strings.Join(strings.Fields(summary), " ")
	if summary == "" {
		return "(no summary)"
	}
	return summary
}

func truncateText(text string, max int) string {
	if max <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func parseTime(raw string) time.Time {
	t, _ := time.Parse(time.RFC3339, raw)
	return t
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
