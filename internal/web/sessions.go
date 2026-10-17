package web

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/catalog"
	ccxconfig "github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

// Global sessions page (/sessions). Spec: docs/design/0003-global-sessions.md

// defaultSessionsLimit bounds the initial render of the global
// sessions page. The full corpus can run to thousands of sessions;
// grouping and scanning stay useful on a bounded window, and the
// footer links to ?limit=0 for the full list.
const defaultSessionsLimit = 100

// sessionEntry pairs a session with its owning project so the
// renderer can link /session/<encoded>/<id> and show project
// affiliation without re-deriving the mapping.
type sessionEntry struct {
	Session *parser.Session
	Project *parser.Project
}

// sessionGroup is one rendered group on the sessions page. Key is
// the machine value (encoded project name, date, provider id,
// model); Label is what the header shows.
type sessionGroup struct {
	Key     string
	Label   string
	Href    string // optional header link (project groups)
	Entries []sessionEntry
	Tokens  int
	Latest  time.Time

	projectSet map[string]bool
}

func (g *sessionGroup) add(e sessionEntry) {
	s := e.Session
	g.Entries = append(g.Entries, e)
	g.Tokens += s.Stats.InputTokens + s.Stats.OutputTokens
	if s.EndTime.After(g.Latest) {
		g.Latest = s.EndTime
	}
	if e.Project != nil {
		if g.projectSet == nil {
			g.projectSet = make(map[string]bool)
		}
		g.projectSet[e.Project.EncodedName] = true
	}
}

func (g *sessionGroup) ProjectCount() int { return len(g.projectSet) }

// sessionsQuery is the parsed, validated state of the sessions page.
// Every field round-trips through URL params so views are shareable.
// Search is matched here (summary OR ID prefix), not through
// SessionFilter.Query, which the CLI shares and matches summary only.
type sessionsQuery struct {
	Filter  catalog.SessionQuery
	GroupBy string
	SortBy  string
	Limit   int
	Search  string
	Project string
	Model   string
	After   string // raw YYYY-MM-DD as typed, for inputs and chips
	Before  string
}

// Filtered reports whether any user filter narrows the corpus.
// limit is a window, not a filter.
func (sq sessionsQuery) Filtered() bool {
	return sq.Search != "" || sq.Project != "" || sq.Model != "" ||
		sq.After != "" || sq.Before != "" || sq.Filter.Filter.Provider != ""
}

func validGroupBy(g string) string {
	switch g {
	case "project", "day", "provider", "model":
		return g
	default:
		return ""
	}
}

func parseSessionsQuery(r *http.Request) sessionsQuery {
	q := r.URL.Query()

	sortBy := q.Get("sort")
	if catalog.ValidateSessionSort(catalog.SessionSort(sortBy)) != nil || sortBy == "" {
		sortBy = string(catalog.SortTime)
	}

	limit := defaultSessionsLimit
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			limit = n
		}
	}

	query := catalog.SessionQuery{
		Scope:  catalog.ScopeAll,
		Filter: parseSessionFilter(r),
		Sort:   catalog.SessionSort(sortBy),
	}
	search := strings.TrimSpace(query.Filter.Query)
	// The q param means "summary OR session-ID prefix" on this page;
	// matchesSearch applies it after the shared filter runs.
	query.Filter.Query = ""

	project := strings.TrimSpace(q.Get("project"))
	if project != "" {
		query.Scope = catalog.ScopeProject
		query.ProjectName = project
	}

	after, before := q.Get("after"), q.Get("before")
	if query.Filter.After.IsZero() {
		after = ""
	}
	if query.Filter.Before.IsZero() {
		before = ""
	}

	return sessionsQuery{
		Filter:  query,
		GroupBy: validGroupBy(q.Get("group")),
		SortBy:  sortBy,
		Limit:   limit,
		Search:  search,
		Project: project,
		Model:   strings.TrimSpace(q.Get("model")),
		After:   after,
		Before:  before,
	}
}

func matchesSearch(s *parser.Session, q string) bool {
	if q == "" {
		return true
	}
	ql := strings.ToLower(q)
	return strings.Contains(strings.ToLower(s.Summary), ql) ||
		strings.HasPrefix(strings.ToLower(s.ID), ql)
}

// collectSessions applies the query and returns the visible entries,
// the total match count, and the distinct-project count of the match
// set (both computed before the limit window).
func collectSessions(projects []*parser.Project, sq sessionsQuery) (entries []sessionEntry, total, matchProjects int) {
	owner := make(map[*parser.Session]*parser.Project)
	for _, p := range projects {
		for _, s := range p.Sessions {
			owner[s] = p
		}
	}

	var matched []sessionEntry
	projSeen := make(map[*parser.Project]bool)
	for _, s := range catalog.ApplySessionQuery(projects, sq.Filter) {
		if !matchesSearch(s, sq.Search) {
			continue
		}
		e := sessionEntry{Session: s, Project: owner[s]}
		matched = append(matched, e)
		if e.Project != nil {
			projSeen[e.Project] = true
		}
	}

	total = len(matched)
	if sq.Limit > 0 && len(matched) > sq.Limit {
		matched = matched[:sq.Limit]
	}
	return matched, total, len(projSeen)
}

func providerDisplayName(id string) string {
	switch id {
	case "claude-code":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "grok":
		return "Grok"
	case "":
		return "Unknown"
	default:
		return id
	}
}

// groupSessions splits entries into groups keyed by mode. Rows inside
// a group keep the active sort; groups themselves are ordered by
// their most recent session (so day groups descend by date), except
// provider mode which uses the fixed CC, CX, GX order.
func groupSessions(entries []sessionEntry, mode string) []*sessionGroup {
	if mode == "" {
		g := &sessionGroup{}
		for _, e := range entries {
			g.add(e)
		}
		return []*sessionGroup{g}
	}

	var groups []*sessionGroup
	index := make(map[string]*sessionGroup)
	for _, e := range entries {
		key, label, href := groupKey(e, mode)
		g, ok := index[key]
		if !ok {
			g = &sessionGroup{Key: key, Label: label, Href: href}
			index[key] = g
			groups = append(groups, g)
		}
		g.add(e)
	}

	if mode == "provider" {
		rank := map[string]int{"claude-code": 0, "codex": 1, "grok": 2}
		sort.SliceStable(groups, func(i, j int) bool {
			ri, iOK := rank[groups[i].Key]
			rj, jOK := rank[groups[j].Key]
			if iOK != jOK {
				return iOK
			}
			if iOK {
				return ri < rj
			}
			return groups[i].Latest.After(groups[j].Latest)
		})
	} else {
		sort.SliceStable(groups, func(i, j int) bool {
			return groups[i].Latest.After(groups[j].Latest)
		})
	}
	return groups
}

func groupKey(e sessionEntry, mode string) (key, label, href string) {
	s := e.Session
	switch mode {
	case "project":
		if e.Project == nil {
			return "unknown", "unknown project", ""
		}
		enc := e.Project.EncodedName
		return enc, parser.GetProjectDisplayName(enc), "/project/" + enc
	case "day":
		if s.EndTime.IsZero() {
			return "undated", "undated", ""
		}
		day := s.EndTime.Local()
		return day.Format("2006-01-02"), day.Format("Mon 2006-01-02"), ""
	case "provider":
		return s.Provider, providerDisplayName(s.Provider), ""
	case "model":
		if s.Model == "" {
			return "unknown", "(no model recorded)", ""
		}
		return s.Model, s.Model, ""
	}
	return "", "", ""
}

// handleAPISessionsGlobal serves GET /api/sessions: the cross-project
// session list with the same filter/sort/limit params as /sessions.
// Unlike the per-project array endpoint it wraps results in an
// envelope so callers can see how many matches the limit hid.
func handleAPISessionsGlobal(w http.ResponseWriter, r *http.Request) {
	projects, err := sessionProvider.DiscoverProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sq := parseSessionsQuery(r)
	entries, total, _ := collectSessions(projects, sq)

	type sessionResp struct {
		ID          string  `json:"id"`
		Provider    string  `json:"provider"`
		Project     string  `json:"project"`
		ProjectName string  `json:"project_name"`
		Summary     string  `json:"summary"`
		StartTime   string  `json:"start_time"`
		EndTime     string  `json:"end_time"`
		Messages    int     `json:"messages"`
		Tokens      int     `json:"tokens"`
		CostUSD     float64 `json:"cost_usd,omitempty"`
		Model       string  `json:"model,omitempty"`
	}

	resp := make([]sessionResp, 0, len(entries))
	for _, e := range entries {
		s := e.Session
		item := sessionResp{
			ID:        s.ID,
			Provider:  s.Provider,
			Summary:   s.Summary,
			StartTime: s.StartTime.Format(time.RFC3339),
			EndTime:   s.EndTime.Format(time.RFC3339),
			Messages:  s.Stats.MessageCount,
			Tokens:    s.Stats.InputTokens + s.Stats.OutputTokens,
			CostUSD:   s.Stats.CostUSD,
			Model:     s.Model,
		}
		if e.Project != nil {
			item.Project = e.Project.EncodedName
			item.ProjectName = parser.GetProjectDisplayName(e.Project.EncodedName)
		}
		resp = append(resp, item)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessions": resp,
		"total":    total,
		"shown":    len(resp),
	})
}

func handleSessionsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sessions" {
		renderNotFoundPage(w, r, "page", "no ccx route matches "+r.URL.Path)
		return
	}

	projects, err := sessionProvider.DiscoverProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sq := parseSessionsQuery(r)
	entries, total, matchProjects := collectSessions(projects, sq)
	groups := groupSessions(entries, sq.GroupBy)

	corpus := 0
	for _, p := range projects {
		corpus += len(p.Sessions)
	}

	// Alphabetical by display name for the project select: you are
	// looking for a name you already know, and native typeahead
	// needs a stable order.
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(parser.GetProjectDisplayName(projects[i].EncodedName)) <
			strings.ToLower(parser.GetProjectDisplayName(projects[j].EncodedName))
	})

	view := sessionsView{
		Query:          sq,
		Groups:         groups,
		Shown:          len(entries),
		MatchTotal:     total,
		MatchProjects:  matchProjects,
		CorpusSessions: corpus,
		CorpusProjects: len(projects),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, locFrom(r).renderSessionsPage(projects, view))
}

type sessionsView struct {
	Query          sessionsQuery
	Groups         []*sessionGroup
	Shown          int
	MatchTotal     int // matches before the limit window
	MatchProjects  int // distinct projects in the match set
	CorpusSessions int // all sessions, unfiltered
	CorpusProjects int
}

// formatCount renders 2314 as "2,314"; row-level numbers stay bare.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func (l loc) renderSessionsPage(projects []*parser.Project, v sessionsView) string {
	sq := v.Query
	var b strings.Builder

	b.WriteString(l.pageHeader(l.T("title.sessions"), ccxconfig.Theme()))
	b.WriteString(l.renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(l.renderSidebar("sessions"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header page-header-sessions">`)
	b.WriteString(`<span class="page-badge badge-session">S</span>`)
	b.WriteString(`<h1>` + html.EscapeString(l.T("sessions.heading")) + `</h1>`)
	if sq.Filtered() {
		b.WriteString(fmt.Sprintf(`<div class="stats">%s</div>`,
			html.EscapeString(l.T("sessions.stats_filtered", formatCount(v.MatchTotal), formatCount(v.CorpusSessions), v.MatchProjects))))
	} else {
		b.WriteString(fmt.Sprintf(`<div class="stats">%s</div>`,
			html.EscapeString(l.T("sessions.stats_all", formatCount(v.CorpusSessions), v.CorpusProjects))))
	}
	b.WriteString(`</div>`)

	b.WriteString(l.renderSessionsControls(projects, sq))
	b.WriteString(l.renderFilterChips(projects, sq))

	if v.Shown == 0 {
		if sq.Filtered() {
			b.WriteString(`<div class="empty-state">` + html.EscapeString(l.T("sessions.empty_filtered")) + `<br><a href="/sessions">` + html.EscapeString(l.T("sessions.clear_filters")) + `</a></div>`)
		} else {
			b.WriteString(`<div class="empty-state">` + html.EscapeString(l.T("sessions.empty")) + `</div>`)
		}
	} else {
		for _, g := range v.Groups {
			b.WriteString(`<section class="sgroup">`)
			if g.Label != "" {
				l.renderGroupHead(&b, g, sq.GroupBy)
			}
			b.WriteString(`<div class="session-rows">`)
			for _, e := range g.Entries {
				l.renderSessionRow(&b, e, sq.GroupBy)
			}
			b.WriteString(`</div></section>`)
		}
	}

	b.WriteString(l.renderSessionsFooter(sq, v))

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(l.indexJS())
	b.WriteString(sessionsJS())
	b.WriteString(l.pageFooter())

	return b.String()
}

func (l loc) renderSessionsControls(projects []*parser.Project, sq sessionsQuery) string {
	var b strings.Builder
	prov := sq.Filter.Filter.Provider

	b.WriteString(`<form method="get" action="/sessions" id="s-form" class="controls controls-wrap">`)
	b.WriteString(`<div class="search-wrap">`)
	b.WriteString(fmt.Sprintf(`<input type="text" id="s-q" name="q" class="search-input" placeholder="%s" value="%s">`, html.EscapeString(l.T("sessions.filter")), html.EscapeString(sq.Search)))
	b.WriteString(`<span class="search-spinner" id="search-spinner"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="sort-controls">`)

	b.WriteString(fmt.Sprintf(`<select id="s-provider" name="provider" class="sort-select" title="%s">
		<option value="">%s</option>
		<option value="claude-code"%s>Claude Code</option>
		<option value="codex"%s>Codex</option>
		<option value="grok"%s>Grok</option>
	</select>`, html.EscapeString(l.T("project.filter_provider")), html.EscapeString(l.T("sessions.all_providers")), selected(prov, "claude-code"), selected(prov, "codex"), selected(prov, "grok")))

	b.WriteString(fmt.Sprintf(`<select id="s-project" name="project" class="sort-select" title="%s"><option value="">%s</option>`, html.EscapeString(l.T("sessions.group_project")), html.EscapeString(l.T("sessions.all_projects"))))
	for _, p := range projects {
		b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s (%d)</option>`,
			html.EscapeString(p.EncodedName), selected(sq.Project, p.EncodedName),
			html.EscapeString(parser.GetProjectDisplayName(p.EncodedName)), len(p.Sessions)))
	}
	b.WriteString(`</select>`)

	b.WriteString(`<span class="sort-label">` + html.EscapeString(l.T("sessions.group")) + `</span>`)
	b.WriteString(fmt.Sprintf(`<select id="s-group" name="group" class="sort-select">
		<option value="">%s</option>
		<option value="project"%s>%s</option>
		<option value="day"%s>%s</option>
		<option value="provider"%s>%s</option>
		<option value="model"%s>%s</option>
	</select>`, html.EscapeString(l.T("sessions.group_none")), selected(sq.GroupBy, "project"), html.EscapeString(l.T("sessions.group_project")), selected(sq.GroupBy, "day"), html.EscapeString(l.T("sessions.group_day")), selected(sq.GroupBy, "provider"), html.EscapeString(l.T("sessions.group_provider")), selected(sq.GroupBy, "model"), html.EscapeString(l.T("sessions.group_model"))))

	b.WriteString(`<span class="sort-label">` + html.EscapeString(l.T("projects.sort")) + `</span>`)
	b.WriteString(fmt.Sprintf(`<select id="s-sort" name="sort" class="sort-select">
		<option value="">%s</option>
		<option value="messages"%s>%s</option>
		<option value="prompts"%s>%s</option>
		<option value="tokens"%s>%s</option>
	</select>`, html.EscapeString(l.T("projects.sort_recent")), selected(sq.SortBy, "messages"), html.EscapeString(l.T("project.sort_messages")), selected(sq.SortBy, "prompts"), html.EscapeString(l.T("sessions.sort_prompts")), selected(sq.SortBy, "tokens"), html.EscapeString(l.T("sessions.sort_tokens"))))

	b.WriteString(`<noscript><button type="submit" class="sort-select">` + html.EscapeString(l.T("sessions.apply")) + `</button></noscript>`)
	b.WriteString(`</div>`)

	open := ""
	if sq.Model != "" || sq.After != "" || sq.Before != "" {
		open = " open"
	}
	b.WriteString(fmt.Sprintf(`<details class="filter-more"%s><summary>%s</summary><div class="filter-more-body">`, open, html.EscapeString(l.T("sessions.more_filters"))))
	b.WriteString(fmt.Sprintf(`<label class="filter-field"><span class="sort-label">%s</span><input type="text" id="s-model" name="model" class="filter-input" placeholder="%s" value="%s"></label>`, html.EscapeString(l.T("sessions.model")), html.EscapeString(l.T("sessions.model_ph")), html.EscapeString(sq.Model)))
	b.WriteString(fmt.Sprintf(`<label class="filter-field"><span class="sort-label">%s</span><input type="date" id="s-after" name="after" class="filter-input" value="%s"></label>`, html.EscapeString(l.T("sessions.after")), html.EscapeString(sq.After)))
	b.WriteString(fmt.Sprintf(`<label class="filter-field"><span class="sort-label">%s</span><input type="date" id="s-before" name="before" class="filter-input" value="%s"></label>`, html.EscapeString(l.T("sessions.before")), html.EscapeString(sq.Before)))
	b.WriteString(`</div></details>`)

	b.WriteString(`</form>`)
	return b.String()
}

// renderFilterChips makes every active filter visible and clearable
// without JS — each x is a link to the same URL minus that param.
func (l loc) renderFilterChips(projects []*parser.Project, sq sessionsQuery) string {
	type chip struct{ param, label, value string }
	var chips []chip
	if sq.Search != "" {
		chips = append(chips, chip{"q", "q", sq.Search})
	}
	if p := sq.Filter.Filter.Provider; p != "" {
		chips = append(chips, chip{"provider", "provider", providerDisplayName(p)})
	}
	if sq.Project != "" {
		display := sq.Project
		for _, p := range projects {
			if p.EncodedName == sq.Project {
				display = parser.GetProjectDisplayName(p.EncodedName)
				break
			}
		}
		chips = append(chips, chip{"project", "project", display})
	}
	if sq.Model != "" {
		chips = append(chips, chip{"model", "model", sq.Model})
	}
	if sq.After != "" {
		chips = append(chips, chip{"after", "after", sq.After})
	}
	if sq.Before != "" {
		chips = append(chips, chip{"before", "before", sq.Before})
	}
	if len(chips) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="filter-chips">`)
	for _, c := range chips {
		b.WriteString(fmt.Sprintf(`<span class="filter-chip">%s: <b>%s</b><a class="filter-chip-x" href="%s" aria-label="%s">x</a></span>`,
			c.label, html.EscapeString(c.value),
			html.EscapeString(sessionsPageURL(sq, c.param, "")), html.EscapeString(l.T("sessions.remove_filter", c.label))))
	}
	b.WriteString(`<a class="filter-clear" href="/sessions">` + html.EscapeString(l.T("sessions.clear_all")) + `</a>`)
	b.WriteString(`</div>`)
	return b.String()
}

func (l loc) renderGroupHead(b *strings.Builder, g *sessionGroup, mode string) {
	b.WriteString(`<div class="sgroup-head">`)
	if mode == "provider" {
		b.WriteString(providerBadgeHTML(g.Key))
	}
	label := html.EscapeString(g.Label)
	if mode == "day" {
		today := time.Now().Local().Format("2006-01-02")
		yesterday := time.Now().Local().AddDate(0, 0, -1).Format("2006-01-02")
		switch g.Key {
		case today:
			label += `<span class="sgroup-day-hint"> · ` + html.EscapeString(l.T("sessions.today")) + `</span>`
		case yesterday:
			label += `<span class="sgroup-day-hint"> · ` + html.EscapeString(l.T("sessions.yesterday")) + `</span>`
		}
	}
	if g.Href != "" {
		b.WriteString(fmt.Sprintf(`<a href="%s" class="sgroup-label">%s</a>`, html.EscapeString(g.Href), label))
	} else {
		b.WriteString(fmt.Sprintf(`<span class="sgroup-label">%s</span>`, label))
	}

	count := l.T("sessions.session_one")
	if n := len(g.Entries); n != 1 {
		count = l.T("sessions.session_other", formatCount(n))
	}
	// Everything past the count collapses at the 700px breakpoint.
	var extra []string
	if mode != "project" && g.ProjectCount() > 0 {
		if g.ProjectCount() == 1 {
			extra = append(extra, l.T("sessions.project_one"))
		} else {
			extra = append(extra, l.T("sessions.project_other", g.ProjectCount()))
		}
	}
	if g.Tokens > 0 {
		extra = append(extra, formatTokens(g.Tokens)+" "+l.T("sessions.tok"))
	}
	if mode == "project" && !g.Latest.IsZero() {
		extra = append(extra, l.age(g.Latest))
	}
	b.WriteString(`<span class="sgroup-meta">` + count)
	if len(extra) > 0 {
		b.WriteString(fmt.Sprintf(`<span class="sgroup-meta-extra"> · %s</span>`, strings.Join(extra, " · ")))
	}
	b.WriteString(`</span></div>`)
}

// sessionRowLabel: Title beats Summary. Title is only populated on
// the full parse path today, so this is Summary in practice; the
// precedence is written so quick-parse Title lights it up for free.
func sessionRowLabel(s *parser.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.Summary != "" {
		return s.Summary
	}
	return ""
}

func (l loc) sessionRowLabel(s *parser.Session) string {
	if label := sessionRowLabel(s); label != "" {
		return label
	}
	return l.T("sessions.no_summary")
}

func (l loc) renderSessionRow(b *strings.Builder, e sessionEntry, groupBy string) {
	s := e.Session
	enc := ""
	if e.Project != nil {
		enc = e.Project.EncodedName
	}

	// The tooltip carries what the two lines cannot: absolute times,
	// duration, tool calls, and the untruncated summary.
	dur := s.Stats.DurationSeconds
	if dur <= 0 {
		dur = s.EndTime.Sub(s.StartTime).Seconds()
	}
	tip := fmt.Sprintf("%s -> %s (%s)",
		s.StartTime.Format("2006-01-02 15:04"),
		s.EndTime.Format("15:04"),
		formatDuration(dur))
	if s.Stats.ToolCalls > 0 {
		tip += " | " + l.T("sessions.tool_calls", s.Stats.ToolCalls)
	}
	tip += " | " + l.sessionRowLabel(s)

	stat := func(n int, unit, extraClass string) string {
		if n <= 0 {
			return fmt.Sprintf(`<span class="srow-stat srow-none%s" title="%s">-</span>`, extraClass, html.EscapeString(l.T("sessions.not_reported")))
		}
		return fmt.Sprintf(`<span class="srow-stat%s">%s %s</span>`, extraClass, formatCount(n), unit)
	}
	tokCell := fmt.Sprintf(`<span class="srow-stat srow-none" title="%s">-</span>`, html.EscapeString(l.T("sessions.not_reported")))
	if tok := s.Stats.InputTokens + s.Stats.OutputTokens; tok > 0 {
		tokCell = fmt.Sprintf(`<span class="srow-stat">%s %s</span>`, formatTokens(tok), html.EscapeString(l.T("sessions.tok")))
	}

	var meta strings.Builder
	if groupBy != "project" {
		name := ""
		if e.Project != nil {
			name = parser.GetProjectDisplayName(e.Project.EncodedName)
		}
		meta.WriteString(fmt.Sprintf(`<span class="srow-project">%s</span>`, html.EscapeString(name)))
	}
	if groupBy != "model" {
		model := s.Model
		if model == "" {
			model = "-"
		}
		meta.WriteString(fmt.Sprintf(`<span class="srow-model">%s</span>`, html.EscapeString(model)))
	}
	meta.WriteString(stat(s.Stats.MessageCount, l.T("sessions.msg"), ""))
	promptUnit := l.T("sessions.prompt_other")
	if s.Stats.UserPrompts == 1 {
		promptUnit = l.T("sessions.prompt_one")
	}
	meta.WriteString(stat(s.Stats.UserPrompts, promptUnit, " srow-stat-prompts"))
	meta.WriteString(tokCell)

	fmt.Fprintf(b, `
<a href="/session/%s/%s" class="srow" title="%s">
	<div class="srow-top">%s<span class="srow-label">%s</span><span class="srow-time" title="%s">%s</span></div>
	<div class="srow-meta">%s</div>
</a>`,
		html.EscapeString(enc), html.EscapeString(s.ID),
		html.EscapeString(tip),
		providerBadgeHTML(s.Provider),
		html.EscapeString(l.sessionRowLabel(s)),
		s.EndTime.Format("2006-01-02 15:04"),
		l.relTime(s.EndTime),
		meta.String())
}

// renderSessionsFooter states the truncation honestly: what is
// shown, what matched, and the links to widen the window.
func renderSessionsFooter(sq sessionsQuery, v sessionsView) string {
	return defaultLoc().renderSessionsFooter(sq, v)
}

func (l loc) renderSessionsFooter(sq sessionsQuery, v sessionsView) string {
	matching := ""
	if sq.Filtered() {
		matching = l.T("sessions.matching")
	}

	var b strings.Builder
	b.WriteString(`<div class="list-footer">`)
	if v.Shown < v.MatchTotal {
		b.WriteString(fmt.Sprintf(`<span>%s</span>`,
			html.EscapeString(l.T("sessions.showing_of", formatCount(v.Shown), formatCount(v.MatchTotal), matching))))
		if v.MatchTotal > 500 && sq.Limit < 500 {
			b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(sessionsPageURL(sq, "limit", "500")), html.EscapeString(l.T("sessions.show_500"))))
		}
		b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`,
			html.EscapeString(sessionsPageURL(sq, "limit", "0")), html.EscapeString(l.T("sessions.show_all", formatCount(v.MatchTotal)))))
	} else if v.Shown > 0 {
		b.WriteString(fmt.Sprintf(`<span>%s</span>`, html.EscapeString(l.T("sessions.showing_all", formatCount(v.Shown), matching))))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// sessionsPageURL rebuilds the page URL from the current query with
// one param overridden (value "" removes it), so controls compose
// instead of resetting each other. Params at their defaults are
// omitted to keep shared links readable.
func sessionsPageURL(sq sessionsQuery, key, value string) string {
	params := url.Values{}
	set := func(k, v string) {
		if v != "" {
			params.Set(k, v)
		}
	}
	set("q", sq.Search)
	set("project", sq.Project)
	set("provider", sq.Filter.Filter.Provider)
	set("model", sq.Model)
	set("after", sq.After)
	set("before", sq.Before)
	set("group", sq.GroupBy)
	if sq.SortBy != string(catalog.SortTime) {
		set("sort", sq.SortBy)
	}
	if sq.Limit != defaultSessionsLimit {
		params.Set("limit", strconv.Itoa(sq.Limit))
	}
	if value == "" {
		params.Del(key)
	} else {
		params.Set(key, value)
	}
	if enc := params.Encode(); enc != "" {
		return "/sessions?" + enc
	}
	return "/sessions"
}

// sessionsJS wires the filter form and this page's keyboard map.
// indexJS is also emitted for the shared theme button and top-nav
// global search; the s- id prefix keeps this form out of its way.
func sessionsJS() string {
	return `
<script>
(function() {
  const form = document.getElementById('s-form');
  if (!form) return;
  let t;
  // One debounced path for typing AND selects: Firefox fires change
  // while arrow-keying through a closed select, so instant submit
  // would navigate away mid-choice.
  // Empty fields would submit as ?q=&model= — strip them.
  function stripEmpty() {
    form.querySelectorAll('input, select').forEach(el => { if (!el.value) el.disabled = true; });
  }
  function submitSoon() {
    clearTimeout(t);
    document.getElementById('search-spinner')?.classList.add('loading');
    t = setTimeout(() => { stripEmpty(); form.submit(); }, 400);
  }
  // Native submit (Enter in a text field) must strip too.
  form.addEventListener('submit', stripEmpty);
  form.querySelectorAll('select').forEach(el => el.addEventListener('change', submitSoon));
  form.querySelectorAll('input[type="date"]').forEach(el => el.addEventListener('change', submitSoon));
  form.querySelectorAll('input[type="text"]').forEach(el => el.addEventListener('input', submitSoon));

  const rows = Array.from(document.querySelectorAll('.srow'));
  document.addEventListener('keydown', function(e) {
    if (e.target.matches('input, textarea, select')) {
      if (e.key === 'Escape') e.target.blur();
      return;
    }
    if (e.key === '/') {
      // Registered after indexJS's handler so this wins the focus:
      // on this page the filter is the instrument, not global search.
      e.preventDefault();
      document.getElementById('s-q')?.focus();
    } else if (e.key === 'j' || e.key === 'k') {
      const cur = rows.indexOf(document.activeElement);
      const next = e.key === 'j' ? Math.min(cur + 1, rows.length - 1) : Math.max(cur - 1, 0);
      if (rows[next]) {
        rows[next].focus();
        rows[next].scrollIntoView({ block: 'nearest' });
      }
    } else if (e.key === 'd') {
      const html = document.documentElement;
      const next = html.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
      html.setAttribute('data-theme', next);
      localStorage.setItem('ccx-theme', next);
    }
  });
})();
</script>
`
}
