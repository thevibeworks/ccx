package insight

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/thevibeworks/ccx/internal/sessionlog"
)

type WorkspaceStats struct {
	Name      string
	Records   int
	Prompts   int
	Tools     int
	Providers map[string]bool
	Days      map[string]bool
}

type DailyStats struct {
	Date    string
	Records int
	Prompts int
	Tools   int
	WS      int
	CC      int
	CX      int
}

type PromptEvent struct {
	Timestamp string
	Provider  string
	Project   string
	Text      string
}

func GenerateHTMLReport(bundle *sessionlog.Bundle) []byte {
	workspaces := aggregateWorkspaces(bundle)
	daily := aggregateDaily(bundle)
	prompts := collectPrompts(bundle)
	kinds := countKinds(bundle)
	provCC, provCX := countProviders(bundle)
	longRunning := countLongRunning(bundle)
	compactions := kinds["compacted"] + kinds["context_compacted"]

	var b strings.Builder

	b.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="ccx-report-scope" content="%s">
<meta name="ccx-report-generated" content="%s">
<title>Session Intelligence — %s</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--ink:#1a1a1a;--paper:#fafaf8;--muted:#6b7280;--rule:#e5e5e0;
--accent:#da7756;--card-bg:#fff;--card-border:#e5e5e0;
--mono:"SF Mono","Cascadia Code",Consolas,monospace;
--sans:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif}
body{font:14px/1.6 var(--sans);color:var(--ink);background:var(--paper);
max-width:1100px;margin:0 auto;padding:24px 16px}
h1{font-size:20px;font-weight:600;margin-bottom:4px}
h2{font-size:14px;font-weight:600;margin:28px 0 10px;padding-bottom:5px;
border-bottom:1px solid var(--rule);color:var(--muted);text-transform:uppercase;letter-spacing:.5px}
.scope{font:12px var(--mono);color:var(--muted);margin-bottom:20px;line-height:1.8}
.scope span{margin-right:14px}
.dq{background:#fffbeb;border:1px solid #fde68a;border-radius:5px;padding:10px 14px;margin-bottom:20px;font-size:13px}
.metrics{display:grid;grid-template-columns:repeat(auto-fill,minmax(110px,1fr));gap:7px;margin-bottom:20px}
.metric{text-align:center;padding:10px 6px;background:var(--card-bg);border:1px solid var(--card-border);border-radius:4px}
.metric .v{font:600 20px var(--mono);display:block}
.metric .l{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.3px}
table{width:100%%;border-collapse:collapse;font-size:13px;margin-bottom:14px}
th{text-align:left;font-weight:600;padding:5px 7px;border-bottom:2px solid var(--rule);
font-size:11px;text-transform:uppercase;letter-spacing:.3px;color:var(--muted)}
td{padding:5px 7px;border-bottom:1px solid var(--rule);vertical-align:top}
tr:hover{background:#f8f8f5}
.n{font-family:var(--mono);text-align:right;font-variant-numeric:tabular-nums}
.badge{display:inline-block;font:600 10px var(--mono);padding:1px 5px;border-radius:3px;text-transform:uppercase;letter-spacing:.3px}
.b-cc{background:#dbeafe;color:#1e40af}.b-cx{background:#fce7f3;color:#9d174d}
.tl{border-left:2px solid var(--rule);margin-left:8px;padding-left:14px}
.ev{margin-bottom:10px;position:relative}
.ev::before{content:"";position:absolute;left:-19px;top:6px;width:7px;height:7px;border-radius:50%%;background:var(--muted);border:2px solid var(--paper)}
.et{font:11px var(--mono);color:var(--muted)}
.ex{font-size:13px;margin-top:2px}
.search{width:100%%;padding:7px 10px;border:1px solid var(--rule);border-radius:4px;font-size:13px;margin-bottom:14px}
.search:focus{outline:2px solid var(--accent);border-color:transparent}
@media print{.search,.no-print{display:none}body{font-size:11px}}
</style>
</head>
<body>
<h1>Session Intelligence</h1>
`,
		html.EscapeString(bundle.Scope.Label),
		bundle.GeneratedAt.Format(time.RFC3339),
		html.EscapeString(bundle.Scope.Label),
	))

	// Scope strip
	b.WriteString(`<div class="scope">`)
	b.WriteString(fmt.Sprintf(`<span>%s</span>`, html.EscapeString(bundle.Scope.Label)))
	b.WriteString(fmt.Sprintf(`<span>%s → %s</span>`,
		bundle.Scope.Start.Format("2006-01-02"),
		bundle.Scope.End.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf(`<span>TZ: %s</span>`, html.EscapeString(bundle.Scope.TimeZone)))
	b.WriteString(`<span>All projects</span>`)
	b.WriteString(fmt.Sprintf(`<span>Generated: %s</span>`, bundle.GeneratedAt.Format("2006-01-02 15:04")))
	b.WriteString(`</div>`)

	// Data quality
	b.WriteString(fmt.Sprintf(`<div class="dq"><strong>Data quality:</strong> %d source log files, %d records. %d long-running containers. %d compaction records. CC %d / CX %d.</div>`,
		bundle.Metrics.Sessions, len(bundle.Records), longRunning, compactions, provCC, provCX))

	// Metrics
	b.WriteString(`<div class="metrics">`)
	writeMetric(&b, fmt.Sprintf("%d", bundle.Metrics.Sessions), "Source log files")
	writeMetric(&b, fmt.Sprintf("%d", len(bundle.Records)), "Records")
	writeMetric(&b, fmt.Sprintf("%d", bundle.Metrics.Workspaces), "Workspaces")
	writeMetric(&b, fmt.Sprintf("%d", kinds["user_prompt"]), "User prompts")
	writeMetric(&b, fmt.Sprintf("%d", kinds["tool_call"]), "Tool calls")
	writeMetric(&b, fmt.Sprintf("%d", kinds["tool_result"]), "Tool results")
	writeMetric(&b, fmt.Sprintf("%d", kinds["assistant_message"]), "Asst messages")
	writeMetric(&b, fmt.Sprintf("%d", kinds["reasoning"]), "Reasoning")
	b.WriteString(`</div>`)

	// Daily
	b.WriteString(`<h2>Daily Activity</h2>`)
	b.WriteString(`<table><tr><th>Date</th><th class="n">Records</th><th class="n">Prompts</th><th class="n">Tools</th><th class="n">WS</th><th class="n">CC</th><th class="n">CX</th></tr>`)
	for _, d := range daily {
		dow := ""
		if t, err := time.Parse("2006-01-02", d.Date); err == nil {
			dow = t.Format("Mon") + " "
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s%s</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td></tr>`,
			dow, d.Date, d.Records, d.Prompts, d.Tools, d.WS, d.CC, d.CX))
	}
	b.WriteString(`</table>`)

	// Workspaces
	b.WriteString(`<h2>Workspaces</h2>`)
	b.WriteString(`<table><tr><th>Workspace</th><th>Providers</th><th class="n">Records</th><th class="n">Prompts</th><th class="n">Tools</th><th class="n">Days</th><th class="n">Ratio</th></tr>`)
	for _, ws := range workspaces[:min(15, len(workspaces))] {
		prov := providerBadges(ws.Providers)
		ratio := "—"
		if ws.Prompts > 0 {
			ratio = fmt.Sprintf("%d:1", ws.Tools/ws.Prompts)
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%d</td><td class="n">%s</td></tr>`,
			html.EscapeString(ws.Name), prov, ws.Records, ws.Prompts, ws.Tools, len(ws.Days), ratio))
	}
	b.WriteString(`</table>`)

	// Timeline
	b.WriteString(`<h2>Timeline</h2>`)
	b.WriteString(`<input type="text" class="search no-print" placeholder="Filter..." oninput="filterTL(this.value)">`)
	b.WriteString(`<div class="tl" id="tl">`)
	seen := make(map[string]bool)
	for _, p := range prompts {
		key := p.Text[:min(50, len(p.Text))]
		if seen[key] {
			continue
		}
		if strings.HasPrefix(p.Text, "<") || strings.HasPrefix(p.Text, "#") {
			continue
		}
		seen[key] = true
		badge := "b-cc"
		tag := "CC"
		if p.Provider == "codex" {
			badge = "b-cx"
			tag = "CX"
		}
		text := p.Text
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		b.WriteString(fmt.Sprintf(`<div class="ev" data-t="%s"><div class="et"><span class="badge %s">%s</span> %s · %s</div><div class="ex">%s</div></div>`,
			html.EscapeString(strings.ToLower(text)),
			badge, tag,
			html.EscapeString(p.Timestamp),
			html.EscapeString(p.Project),
			html.EscapeString(text)))
	}
	b.WriteString(`</div>`)

	// Footer
	b.WriteString(`<script>function filterTL(q){q=q.toLowerCase();document.querySelectorAll('.ev').forEach(e=>{e.style.display=(!q||e.dataset.t.includes(q))?'':'none'})}</script>`)
	b.WriteString(`</body></html>`)

	return []byte(b.String())
}

func writeMetric(b *strings.Builder, val, label string) {
	b.WriteString(fmt.Sprintf(`<div class="metric"><span class="v">%s</span><span class="l">%s</span></div>`, val, label))
}

func providerBadges(providers map[string]bool) string {
	var parts []string
	if providers["claude-code"] {
		parts = append(parts, `<span class="badge b-cc">CC</span>`)
	}
	if providers["codex"] {
		parts = append(parts, `<span class="badge b-cx">CX</span>`)
	}
	return strings.Join(parts, " ")
}

func aggregateWorkspaces(bundle *sessionlog.Bundle) []WorkspaceStats {
	m := make(map[string]*WorkspaceStats)
	for _, r := range bundle.Records {
		ws, ok := m[r.Project]
		if !ok {
			ws = &WorkspaceStats{
				Name:      r.Project,
				Providers: make(map[string]bool),
				Days:      make(map[string]bool),
			}
			m[r.Project] = ws
		}
		ws.Records++
		ws.Providers[r.Provider] = true
		ws.Days[r.Timestamp.Format("2006-01-02")] = true
		switch r.Kind {
		case "user_prompt":
			ws.Prompts++
		case "tool_call":
			ws.Tools++
		}
	}
	result := make([]WorkspaceStats, 0, len(m))
	for _, ws := range m {
		result = append(result, *ws)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Records > result[j].Records })
	return result
}

func aggregateDaily(bundle *sessionlog.Bundle) []DailyStats {
	m := make(map[string]*DailyStats)
	wsPerDay := make(map[string]map[string]bool)
	for _, r := range bundle.Records {
		date := r.Timestamp.Format("2006-01-02")
		d, ok := m[date]
		if !ok {
			d = &DailyStats{Date: date}
			m[date] = d
			wsPerDay[date] = make(map[string]bool)
		}
		d.Records++
		wsPerDay[date][r.Project] = true
		if r.Provider == "claude-code" {
			d.CC++
		} else {
			d.CX++
		}
		switch r.Kind {
		case "user_prompt":
			d.Prompts++
		case "tool_call":
			d.Tools++
		}
	}
	result := make([]DailyStats, 0, len(m))
	for date, d := range m {
		d.WS = len(wsPerDay[date])
		result = append(result, *d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result
}

func collectPrompts(bundle *sessionlog.Bundle) []PromptEvent {
	var prompts []PromptEvent
	for _, r := range bundle.Records {
		if r.Kind != "user_prompt" || r.Text == "" {
			continue
		}
		prompts = append(prompts, PromptEvent{
			Timestamp: r.Timestamp.Format("2006-01-02 15:04"),
			Provider:  r.Provider,
			Project:   r.Project,
			Text:      r.Text,
		})
	}
	return prompts
}

func countKinds(bundle *sessionlog.Bundle) map[string]int {
	m := make(map[string]int)
	for _, r := range bundle.Records {
		m[r.Kind]++
	}
	return m
}

func countProviders(bundle *sessionlog.Bundle) (cc, cx int) {
	for _, r := range bundle.Records {
		if r.Provider == "claude-code" {
			cc++
		} else {
			cx++
		}
	}
	return
}

func countLongRunning(bundle *sessionlog.Bundle) int {
	count := 0
	for _, s := range bundle.Sessions {
		if s.Relation.StartedBeforeScope {
			count++
		}
	}
	return count
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
