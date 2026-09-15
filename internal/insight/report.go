package insight

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/sessionlog"
)

// RenderHTML writes the evidence cockpit for a Report: one
// self-contained page (inline CSS, optional inline JS for filtering,
// print stylesheet) that a human can review without reading JSONL.
//
// It renders facts only — scope, data quality, workspace board with
// every session's prompts / final answer / edits / commits / cost, a
// timeline of human prompts and interventions, and the model table.
// Judgment (TL;DR, what is done vs claimed, what needs closure) is the
// recap skill's layer; it starts from `ccx insight --json`.
func RenderHTML(report *Report) []byte {
	loc := reportLocation(report)
	var b strings.Builder

	title := "Session Intelligence — " + report.Scope.Label
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<meta name=\"ccx-report-kind\" content=\"%s\">\n", Kind)
	fmt.Fprintf(&b, "<meta name=\"ccx-report-scope\" content=\"%s\">\n", html.EscapeString(report.Scope.Label))
	fmt.Fprintf(&b, "<meta name=\"ccx-report-generated\" content=\"%s\">\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString("<style>\n" + reportCSS + "</style>\n</head>\n<body>\n")

	// 1. Scope header
	b.WriteString("<h1>Session Intelligence</h1>\n")
	scopeKind := "all projects"
	switch {
	case report.Scope.Project != "":
		scopeKind = "project " + report.Scope.Project
	case report.Scope.Workspace != "":
		scopeKind = "workspace " + report.Scope.Workspace
	}
	b.WriteString(`<div class="scope">`)
	fmt.Fprintf(&b, `<span><b>%s</b></span>`, html.EscapeString(report.Scope.Label))
	fmt.Fprintf(&b, `<span>%s → %s</span>`, report.Scope.Start.In(loc).Format("2006-01-02 15:04"), report.Scope.End.In(loc).Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, `<span>TZ %s</span>`, html.EscapeString(report.Scope.TimeZone))
	fmt.Fprintf(&b, `<span>%s</span>`, html.EscapeString(scopeKind))
	fmt.Fprintf(&b, `<span>generated %s</span>`, report.GeneratedAt.In(loc).Format("2006-01-02 15:04"))
	if report.Command != "" {
		fmt.Fprintf(&b, `<span>evidence <code>%s</code></span>`, html.EscapeString(report.Command))
	}
	b.WriteString("</div>\n")

	// 2. Data quality
	m := report.Metrics
	b.WriteString(`<div class="dq"><b>Data quality.</b> `)
	fmt.Fprintf(&b, "%d sessions across %d source log files (%d long-running: started before or ended after the window). ", m.Sessions, m.SourceFiles, m.LongRunningSessions)
	fmt.Fprintf(&b, "%s records; %s inside subagent transcripts. ", formatInt(m.Records), formatInt(m.Sidechains))
	fmt.Fprintf(&b, "Providers: %s. ", html.EscapeString(providerSplit(report.Providers)))
	switch {
	case m.Tokens == 0:
		b.WriteString("No token usage recorded. ")
	case m.UnpricedTokens == 0:
		b.WriteString("Cost covers every token (all models priced). ")
	default:
		fmt.Fprintf(&b, "<b>Cost covers %.0f%% of tokens</b>: %d sessions on models without a pricing row (%s tokens) are not in the dollar figures. ", m.CostCoverage*100, m.UnpricedSessions, formatInt(m.UnpricedTokens))
	}
	if m.Truncated {
		fmt.Fprintf(&b, "Records truncated: %s of %s returned. ", formatInt(m.RecordsReturned), formatInt(m.Records))
	}
	b.WriteString("Session cost and tokens are whole-session figures from the provider's accounting, not sliced to the window.")
	for _, w := range report.Warnings {
		fmt.Fprintf(&b, " Warning: %s.", html.EscapeString(w))
	}
	b.WriteString("</div>\n")

	// 3. Metrics strip
	b.WriteString(`<div class="metrics">`)
	writeMetric(&b, formatInt(m.Sessions), "Sessions")
	writeMetric(&b, formatInt(m.Workspaces), "Workspaces")
	writeMetric(&b, formatInt(m.UserPrompts), "Human prompts")
	writeMetric(&b, formatInt(m.ToolCalls), "Tool calls")
	writeMetric(&b, formatInt(m.Edits), "File edits")
	writeMetric(&b, formatInt(m.Commits), "Commits")
	writeMetric(&b, formatInt(m.Interrupts+m.Denials), "Interventions")
	writeMetric(&b, formatTokensShort(m.Tokens), "Tokens")
	costLabel := "Cost"
	if m.Tokens > 0 && m.UnpricedTokens > 0 {
		costLabel = fmt.Sprintf("Cost (%.0f%% priced)", m.CostCoverage*100)
	}
	writeMetric(&b, formatUSD(m.CostUSD), costLabel)
	b.WriteString("</div>\n")

	// 4. Workspace board
	b.WriteString("<h2>Workspaces</h2>\n")
	b.WriteString(`<input type="text" class="search no-print" placeholder="Filter workspaces and sessions..." oninput="filterRows(this.value)">` + "\n")
	b.WriteString(`<table class="board"><thead><tr><th>Workspace</th><th>Prov</th><th class="n">Sessions</th><th class="n">Prompts</th><th class="n">Tools</th><th class="n">Edits</th><th class="n">Commits</th><th class="n">Interv.</th><th class="n">Days</th><th class="n">Cost</th></tr></thead><tbody>` + "\n")
	digests := map[string]*SessionDigest{}
	for i := range report.Sessions {
		digests[report.Sessions[i].ID] = &report.Sessions[i]
	}
	for _, ws := range report.Workspaces {
		key := strings.ToLower(ws.Key + " " + ws.Path)
		fmt.Fprintf(&b, `<tr class="ws" data-t="%s"><td><details><summary><b>%s</b> <span class="muted">%s</span></summary>`,
			html.EscapeString(key), html.EscapeString(ws.Key), html.EscapeString(ws.Path))
		b.WriteString(`<div class="sessions">`)
		for _, id := range ws.SessionIDs {
			d := digests[id]
			if d == nil {
				continue
			}
			writeSession(&b, d, loc)
		}
		b.WriteString(`</div></details></td>`)
		fmt.Fprintf(&b, `<td>%s</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%s</td></tr>`+"\n",
			providerBadges(ws.Providers), ws.Sessions, ws.UserPrompts, ws.ToolCalls, ws.Edits, ws.Commits, ws.Interrupts+ws.Denials, ws.Days, costCell(ws.CostUSD, ws.UnpricedTokens))
	}
	b.WriteString("</tbody></table>\n")

	// 5. Daily activity
	b.WriteString("<h2>Days</h2>\n")
	b.WriteString(`<table><thead><tr><th>Day</th><th class="n">Sessions</th><th class="n">Prompts</th><th class="n">Tools</th><th class="n">Edits</th><th class="n">Commits</th><th class="n">Interv.</th></tr></thead><tbody>` + "\n")
	for _, d := range report.Days {
		dow := ""
		if t, err := time.ParseInLocation("2006-01-02", d.Key, loc); err == nil {
			dow = t.Format("Mon") + " "
		}
		fmt.Fprintf(&b, `<tr><td>%s%s</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td></tr>`+"\n",
			dow, d.Key, d.Sessions, d.UserPrompts, d.ToolCalls, d.Edits, d.Commits, d.Interrupts+d.Denials)
	}
	b.WriteString("</tbody></table>\n")

	// 6. Models
	if len(report.Models) > 0 {
		b.WriteString("<h2>Models</h2>\n")
		b.WriteString(`<table><thead><tr><th>Model</th><th class="n">Sessions</th><th class="n">Tokens</th><th class="n">Cost</th></tr></thead><tbody>` + "\n")
		for _, mo := range report.Models {
			cost := formatUSD(mo.CostUSD)
			if !mo.Priced {
				cost = `<span class="warn">n/a (unpriced)</span>`
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td class="n">%d</td><td class="n">%s</td><td class="n">%s</td></tr>`+"\n",
				html.EscapeString(mo.Model), mo.Sessions, formatTokensShort(mo.Tokens), cost)
		}
		b.WriteString("</tbody></table>\n")
	}

	// 7. Timeline: human prompts and interventions, in order.
	b.WriteString("<h2>Timeline: the humans in the loop</h2>\n")
	b.WriteString(`<p class="muted">Every human prompt in the window, plus interrupts and denials. Subagent instructions are excluded. Each entry cites <code>session:line</code>; open with <code>ccx view &lt;session&gt;</code> or <code>ccx trace &lt;session&gt;</code>.</p>` + "\n")
	b.WriteString(`<input type="text" class="search no-print" placeholder="Filter timeline..." oninput="filterTL(this.value)">` + "\n")
	b.WriteString(`<div class="tl" id="tl">` + "\n")
	events := timelineEvents(report)
	lastDay := ""
	for _, ev := range events {
		day := ev.Time.In(loc).Format("2006-01-02 Mon")
		if day != lastDay {
			fmt.Fprintf(&b, `<div class="day">%s</div>`+"\n", day)
			lastDay = day
		}
		fmt.Fprintf(&b, `<div class="ev %s" data-t="%s"><div class="et">%s <span class="badge %s">%s</span> %s · <code>%s:%d</code></div><div class="ex">%s</div></div>`+"\n",
			ev.Class, html.EscapeString(strings.ToLower(ev.Project+" "+ev.Text)),
			ev.Time.In(loc).Format("15:04"), badgeClass(ev.Provider), badgeTag(ev.Provider),
			html.EscapeString(ev.Project), html.EscapeString(shortID(ev.SessionID)), ev.Line,
			html.EscapeString(ev.Text))
	}
	b.WriteString("</div>\n")

	b.WriteString("<script>" + reportJS + "</script>\n</body></html>\n")
	return []byte(b.String())
}

func writeSession(b *strings.Builder, d *SessionDigest, loc *time.Location) {
	key := strings.ToLower(d.ID + " " + d.Model + " " + d.Summary)
	for _, p := range d.Prompts {
		key += " " + strings.ToLower(p.Text)
	}
	fmt.Fprintf(b, `<div class="sess" data-t="%s">`, html.EscapeString(key))
	span := d.FirstRecord.In(loc).Format("01-02 15:04") + " → " + d.LastRecord.In(loc).Format("01-02 15:04")
	rel := ""
	if d.Relation.StartedBeforeScope {
		rel += " started before window"
	}
	if d.Relation.EndedAfterScope {
		rel += " ended after window"
	}
	fmt.Fprintf(b, `<div class="sh"><span class="badge %s">%s</span> <code>%s</code> <span class="muted">%s%s</span> · %s`,
		badgeClass(d.Provider), badgeTag(d.Provider), html.EscapeString(shortID(d.ID)), span, html.EscapeString(rel), html.EscapeString(orDash(d.Model)))
	fmt.Fprintf(b, ` · %d prompts · %d tools · %d edits · %d commits`, d.UserPrompts, d.ToolCalls, d.Edits, d.Commits)
	if d.Interrupts+d.Denials > 0 {
		fmt.Fprintf(b, ` · <span class="warn">%d interrupts, %d denials</span>`, d.Interrupts, d.Denials)
	}
	fmt.Fprintf(b, ` · %s</div>`, costCell(d.CostUSD, d.UnpricedTokens))
	if len(d.Prompts) > 0 {
		b.WriteString(`<div class="prompts">`)
		for i, p := range d.Prompts {
			if d.PromptsOmitted > 0 && i == len(d.Prompts)-promptTail {
				fmt.Fprintf(b, `<div class="muted">… %d more prompts …</div>`, d.PromptsOmitted)
			}
			fmt.Fprintf(b, `<div class="p"><span class="muted">%s</span> %s</div>`, p.Time.In(loc).Format("15:04"), html.EscapeString(p.Text))
		}
		b.WriteString(`</div>`)
	}
	if d.FinalAnswer != nil {
		fmt.Fprintf(b, `<details class="fa"><summary>Final answer <span class="muted">(agent-claimed, %s)</span></summary><div>%s</div></details>`,
			d.FinalAnswer.Time.In(loc).Format("01-02 15:04"), html.EscapeString(d.FinalAnswer.Text))
	}
	if len(d.EditedPaths) > 0 {
		extra := ""
		if d.EditedPathsTruncated {
			extra = " …"
		}
		fmt.Fprintf(b, `<div class="paths muted">edited: %s%s</div>`, html.EscapeString(strings.Join(shortPaths(d.EditedPaths, d.Workspace), ", ")), extra)
	}
	fmt.Fprintf(b, `<div class="ref muted">ccx trace %s · ccx related %s</div>`, html.EscapeString(shortID(d.ID)), html.EscapeString(shortID(d.ID)))
	b.WriteString(`</div>` + "\n")
}

type timelineEvent struct {
	Time      time.Time
	Provider  string
	Project   string
	SessionID string
	Line      int
	Text      string
	Class     string
}

// timelineEvents lists the human's prompts (bounded per session by the
// digest) and interventions in time order. Interventions are counted
// on the digest but have no text there; they show as markers.
func timelineEvents(report *Report) []timelineEvent {
	var events []timelineEvent
	for _, d := range report.Sessions {
		for _, p := range d.Prompts {
			events = append(events, timelineEvent{Time: p.Time, Provider: d.Provider, Project: d.Project, SessionID: d.ID, Line: p.Line, Text: p.Text, Class: "prompt"})
		}
	}
	for _, r := range report.Records {
		switch r.Kind {
		case "interrupt", "tool_denied":
			events = append(events, timelineEvent{Time: r.Timestamp, Provider: r.Provider, Project: r.Project, SessionID: r.SessionID, Line: r.Line, Text: r.Kind + ": " + r.Text, Class: "interv"})
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events
}

func writeMetric(b *strings.Builder, val, label string) {
	fmt.Fprintf(b, `<div class="metric"><span class="v">%s</span><span class="l">%s</span></div>`, val, html.EscapeString(label))
}

func providerSplit(providers []Bucket) string {
	parts := make([]string, 0, len(providers))
	for _, p := range providers {
		parts = append(parts, fmt.Sprintf("%s %d sessions / %d records", p.Key, p.Sessions, p.Records))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

func providerBadges(providers []string) string {
	parts := make([]string, 0, len(providers))
	for _, p := range providers {
		parts = append(parts, fmt.Sprintf(`<span class="badge %s">%s</span>`, badgeClass(p), badgeTag(p)))
	}
	return strings.Join(parts, " ")
}

func badgeClass(provider string) string {
	switch provider {
	case "claude-code":
		return "b-cc"
	case "codex":
		return "b-cx"
	case "grok":
		return "b-gx"
	}
	return "b-xx"
}

func badgeTag(provider string) string {
	switch provider {
	case "claude-code":
		return "CC"
	case "codex":
		return "CX"
	case "grok":
		return "GX"
	}
	if provider == "" {
		return "??"
	}
	return strings.ToUpper(provider[:min(2, len(provider))])
}

func costCell(cost float64, unpriced int) string {
	switch {
	case cost <= 0 && unpriced > 0:
		return `<span class="warn">n/a</span>`
	case cost <= 0:
		return "—"
	case unpriced > 0:
		return fmt.Sprintf(`%s<span class="warn">+</span>`, formatUSD(cost))
	}
	return formatUSD(cost)
}

func formatUSD(v float64) string {
	switch {
	case v <= 0:
		return "$0"
	case v >= 1000:
		return fmt.Sprintf("$%s", formatInt(int(v+0.5)))
	case v >= 10:
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func formatTokensShort(n int) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func shortPaths(paths []string, workspace string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if workspace != "" && strings.HasPrefix(p, workspace+"/") {
			p = p[len(workspace)+1:]
		}
		out = append(out, p)
	}
	return out
}

// reportLocation resolves the timezone the report claims in its header
// (Scope.TimeZone) so day bucketing and displayed times agree with it.
func reportLocation(report *Report) *time.Location {
	if report != nil && report.Scope.TimeZone != "" {
		if loc, err := LoadLocation(report.Scope.TimeZone); err == nil {
			return loc
		}
	}
	if report != nil && !report.GeneratedAt.IsZero() {
		return report.GeneratedAt.Location()
	}
	return time.Local
}

// scopeLocation is the bundle-side equivalent, used while digesting.
func scopeLocation(bundle *sessionlog.Bundle) *time.Location {
	if bundle != nil && bundle.Scope.TimeZone != "" {
		if loc, err := LoadLocation(bundle.Scope.TimeZone); err == nil {
			return loc
		}
	}
	if bundle != nil && !bundle.GeneratedAt.IsZero() {
		return bundle.GeneratedAt.Location()
	}
	return time.Local
}

const reportCSS = `*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--ink:#1a1a1a;--paper:#fafaf8;--muted:#6b7280;--rule:#e5e5e0;--accent:#da7756;--warn:#b45309;
--mono:ui-monospace,"SF Mono","Cascadia Code",Consolas,monospace;
--sans:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif}
body{font:14px/1.55 var(--sans);color:var(--ink);background:var(--paper);max-width:1180px;margin:0 auto;padding:24px 16px}
h1{font-size:20px;font-weight:600;margin-bottom:4px}
h2{font-size:13px;font-weight:600;margin:28px 0 10px;padding-bottom:5px;border-bottom:1px solid var(--rule);color:var(--muted);text-transform:uppercase;letter-spacing:.5px}
code{font:12px var(--mono)}
.scope{font:12px var(--mono);color:var(--muted);margin-bottom:16px;line-height:1.9}
.scope span{margin-right:14px}
.dq{background:#fffbeb;border:1px solid #fde68a;border-radius:4px;padding:10px 14px;margin-bottom:16px;font-size:13px}
.metrics{display:grid;grid-template-columns:repeat(auto-fill,minmax(112px,1fr));gap:7px;margin-bottom:8px}
.metric{text-align:center;padding:10px 6px;background:#fff;border:1px solid var(--rule);border-radius:4px}
.metric .v{font:600 19px var(--mono);display:block;font-variant-numeric:tabular-nums}
.metric .l{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.3px}
table{width:100%;border-collapse:collapse;font-size:13px;margin-bottom:14px}
th{text-align:left;font-weight:600;padding:5px 7px;border-bottom:2px solid var(--rule);font-size:11px;text-transform:uppercase;letter-spacing:.3px;color:var(--muted)}
td{padding:5px 7px;border-bottom:1px solid var(--rule);vertical-align:top}
.n{font-family:var(--mono);text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
.muted{color:var(--muted)}
.warn{color:var(--warn)}
.badge{display:inline-block;font:600 10px var(--mono);padding:1px 5px;border-radius:3px;text-transform:uppercase;letter-spacing:.3px}
.b-cc{background:#dbeafe;color:#1e40af}.b-cx{background:#fce7f3;color:#9d174d}.b-gx{background:#ede9fe;color:#5b21b6}.b-xx{background:#e5e7eb;color:#374151}
.board summary{cursor:pointer;list-style:none}
.board summary::-webkit-details-marker{display:none}
.board summary::before{content:"▸ ";color:var(--muted)}
.board details[open] summary::before{content:"▾ "}
.sessions{margin:8px 0 4px 14px}
.sess{border-left:2px solid var(--rule);padding:6px 10px;margin-bottom:10px}
.sh{font-size:12px}
.prompts{margin:6px 0 4px}
.p{font-size:13px;margin:2px 0}
.fa{font-size:13px;margin:4px 0}
.fa summary{cursor:pointer;color:var(--muted)}
.fa div{white-space:pre-wrap;padding:6px 0 2px 12px;border-left:2px solid var(--rule);margin-top:4px}
.paths,.ref{font:12px var(--mono);margin-top:4px;word-break:break-all}
.search{width:100%;padding:7px 10px;border:1px solid var(--rule);border-radius:4px;font-size:13px;margin-bottom:12px;background:#fff}
.search:focus{outline:2px solid var(--accent);border-color:transparent}
.tl{border-left:2px solid var(--rule);margin-left:8px;padding-left:14px}
.day{font:600 12px var(--mono);color:var(--muted);margin:14px 0 6px}
.ev{margin-bottom:8px;position:relative}
.ev::before{content:"";position:absolute;left:-19px;top:6px;width:7px;height:7px;border-radius:50%;background:var(--muted);border:2px solid var(--paper)}
.ev.interv::before{background:var(--warn)}
.et{font:11px var(--mono);color:var(--muted)}
.ex{font-size:13px;margin-top:1px;white-space:pre-wrap}
@media print{.search,.no-print{display:none}body{font-size:11px}.board details{open:true}.fa div{display:block}}
`

const reportJS = `function filterRows(q){q=q.toLowerCase();document.querySelectorAll('tr.ws').forEach(function(r){var hit=!q||r.dataset.t.includes(q);var any=false;r.querySelectorAll('.sess').forEach(function(s){var h=!q||s.dataset.t.includes(q);s.style.display=h?'':'none';any=any||h});r.style.display=(hit||any)?'':'none';if(q&&any&&!hit){var d=r.querySelector('details');if(d)d.open=true}})}
function filterTL(q){q=q.toLowerCase();document.querySelectorAll('.ev').forEach(function(e){e.style.display=(!q||e.dataset.t.includes(q))?'':'none'})}`
