package fold

import (
	"fmt"
	"html"
	"strings"
)

func RenderHTML(result *TraceResult) string {
	if result == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Trace: `)
	b.WriteString(html.EscapeString(result.Session.Summary))
	b.WriteString(`</title>
<style>`)
	b.WriteString(traceCSS())
	b.WriteString(`</style>
</head>
<body>
<div class="container">
`)

	writeHeader(&b, result)
	writeWarnings(&b, result)
	writeEvidenceSummary(&b, result)
	writeExchanges(&b, result)
	writeSidechains(&b, result)
	writeGitSection(&b, result)
	writeWorkspaceSection(&b, result)
	writeStats(&b, result)

	b.WriteString(`</div>
<script>`)
	b.WriteString(traceJS())
	b.WriteString(`</script>
</body>
</html>`)

	return b.String()
}

func writeHeader(b *strings.Builder, r *TraceResult) {
	b.WriteString(`<header class="trace-header">`)
	b.WriteString(fmt.Sprintf(`<h1>Context Trace: %s</h1>`, html.EscapeString(r.Session.Summary)))
	b.WriteString(`<div class="meta">`)

	var parts []string
	if r.Session.ID != "" {
		id := r.Session.ID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">ID: %s</span>`, html.EscapeString(id)))
	}
	if r.Session.Provider != "" {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%s</span>`, html.EscapeString(r.Session.Provider)))
	}
	if !r.Session.Start.IsZero() {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%s</span>`, r.Session.Start.Format("2006-01-02 15:04")))
	}
	if r.Session.Model != "" {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%s</span>`, html.EscapeString(r.Session.Model)))
	}
	parts = append(parts, fmt.Sprintf(`<span class="meta-item">%d exchanges</span>`, r.Stats.ExchangeCount))
	if r.Stats.CorrectionSignals > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item correction-count">%d correction signals</span>`, r.Stats.CorrectionSignals))
	}
	if r.Stats.CommitsLinked > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%d linked commits</span>`, r.Stats.CommitsLinked))
	}
	if r.Stats.TotalCostUSD > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">$%.4f</span>`, r.Stats.TotalCostUSD))
	}
	b.WriteString(strings.Join(parts, " · "))
	b.WriteString(`</div>`)
	b.WriteString(`<button onclick="toggleTheme()" class="theme-toggle">Toggle theme</button>`)
	b.WriteString(`</header>`)
}

func writeWarnings(b *strings.Builder, r *TraceResult) {
	if len(r.Warnings) == 0 {
		return
	}
	b.WriteString(`<section class="warnings"><h2>Evidence Gaps</h2>`)
	for _, warning := range r.Warnings {
		b.WriteString(`<div class="warning">`)
		b.WriteString(fmt.Sprintf(`<strong>%s</strong>`, html.EscapeString(warning.Kind)))
		if warning.Message != "" {
			b.WriteString(fmt.Sprintf(`<p>%s</p>`, html.EscapeString(warning.Message)))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
}

func writeEvidenceSummary(b *strings.Builder, r *TraceResult) {
	b.WriteString(`<section class="summary-grid">`)
	writeMetric(b, "Exchanges", r.Stats.ExchangeCount)
	writeMetric(b, "Mutated Files", r.Stats.FilesEdited)
	writeMetric(b, "Read Files", r.Stats.FilesRead)
	writeMetric(b, "Tools", r.Stats.ToolsUsed)
	writeMetric(b, "Workspace Docs", r.Stats.WorkspaceDocs)
	writeMetric(b, "Knowledge", r.Stats.KnowledgeEntries)
	writeMetric(b, "Uncommitted", r.Stats.UncommittedFiles)
	writeMetric(b, "Commits", len(r.Git.Commits))
	b.WriteString(`</section>`)
}

func writeMetric(b *strings.Builder, label string, value int) {
	b.WriteString(`<div class="metric">`)
	b.WriteString(fmt.Sprintf(`<span class="metric-value">%d</span>`, value))
	b.WriteString(fmt.Sprintf(`<span class="metric-label">%s</span>`, html.EscapeString(label)))
	b.WriteString(`</div>`)
}

func writeExchanges(b *strings.Builder, r *TraceResult) {
	b.WriteString(`<section class="exchanges"><h2>Exchange Evidence</h2>`)

	for _, exchange := range r.Exchanges {
		class := "exchange"
		if exchange.HasCorrection {
			class += " correction"
		}
		if len(exchange.Sidechains) > 0 {
			class += " has-sidechain"
		}

		b.WriteString(fmt.Sprintf(`<article class="%s" id="exchange-%d">`, class, exchange.Index))
		b.WriteString(fmt.Sprintf(`<header><span class="exchange-num">#%d</span>`, exchange.Index))

		if exchange.HasCorrection {
			b.WriteString(`<span class="badge correction">correction signal</span>`)
		}
		if exchange.IsCommand {
			b.WriteString(fmt.Sprintf(`<span class="badge command">/%s</span>`, html.EscapeString(exchange.CommandName)))
		}
		if exchange.HasThinking {
			b.WriteString(`<span class="badge thinking">thinking</span>`)
		}
		if len(exchange.Sidechains) > 0 {
			b.WriteString(fmt.Sprintf(`<span class="badge sidechain">%d sidechain</span>`, len(exchange.Sidechains)))
		}
		if len(exchange.LinkedCommits) > 0 {
			for _, sha := range exchange.LinkedCommits {
				short := sha
				if len(short) > 7 {
					short = short[:7]
				}
				b.WriteString(fmt.Sprintf(` <code class="commit">%s</code>`, html.EscapeString(short)))
			}
		}
		b.WriteString(`</header>`)

		if exchange.UserText != "" {
			b.WriteString(`<div class="user-text">`)
			b.WriteString(html.EscapeString(preview(exchange.UserText, 260)))
			b.WriteString(`</div>`)
		}

		if len(exchange.FilesEdited) > 0 {
			writePathList(b, "Edited", exchange.FilesEdited)
		}
		if len(exchange.FilesRead) > 0 {
			writePathList(b, "Read", exchange.FilesRead)
		}

		if len(exchange.ToolsUsed) > 0 {
			b.WriteString(`<div class="tools">`)
			for _, tool := range exchange.ToolsUsed {
				b.WriteString(fmt.Sprintf(`<span class="tool-badge">%s</span>`, html.EscapeString(tool)))
			}
			b.WriteString(`</div>`)
		}

		if len(exchange.ToolCalls) > 0 {
			b.WriteString(`<details class="tool-calls"><summary>Tool calls</summary>`)
			for _, call := range exchange.ToolCalls {
				b.WriteString(`<div class="tool-call">`)
				b.WriteString(fmt.Sprintf(`<strong>%s</strong>`, html.EscapeString(call.Name)))
				if len(call.Paths) > 0 {
					b.WriteString(` `)
					for i, path := range call.Paths {
						if i > 0 {
							b.WriteString(`, `)
						}
						b.WriteString(fmt.Sprintf(`<code>%s</code>`, html.EscapeString(path)))
					}
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</details>`)
		}

		if exchange.AssistantText != "" {
			b.WriteString(`<details class="assistant-text"><summary>Assistant response</summary><p>`)
			b.WriteString(html.EscapeString(exchange.AssistantText))
			b.WriteString(`</p></details>`)
		}

		b.WriteString(`</article>`)
	}

	b.WriteString(`</section>`)
}

func writeSidechains(b *strings.Builder, r *TraceResult) {
	if len(r.Sidechains) == 0 {
		return
	}
	b.WriteString(`<section class="sidechains"><h2>Sidechain Evidence</h2>`)
	for _, sc := range r.Sidechains {
		label := sc.AgentID
		if sc.AgentType != "" {
			label = sc.AgentType + " · " + sc.AgentID
		}
		b.WriteString(`<article class="sidechain-card">`)
		b.WriteString(fmt.Sprintf(`<h3>%s</h3>`, html.EscapeString(label)))
		b.WriteString(fmt.Sprintf(`<p>%d messages · %d tools`, sc.MessageCount, sc.ToolCalls))
		if sc.TranscriptOmitted {
			b.WriteString(` · transcript omitted`)
		}
		b.WriteString(`</p>`)
		if len(sc.FilesEdited) > 0 {
			writePathList(b, "Edited", sc.FilesEdited)
		}
		if len(sc.FilesRead) > 0 {
			writePathList(b, "Read", sc.FilesRead)
		}
		b.WriteString(`</article>`)
	}
	b.WriteString(`</section>`)
}

func writePathList(b *strings.Builder, label string, paths []string) {
	b.WriteString(fmt.Sprintf(`<div class="path-list"><span>%s:</span> `, html.EscapeString(label)))
	for i, path := range paths {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf(`<code>%s</code>`, html.EscapeString(path)))
	}
	b.WriteString(`</div>`)
}

func writeGitSection(b *strings.Builder, r *TraceResult) {
	if len(r.Git.Commits) == 0 && len(r.Git.UncommittedFiles) == 0 {
		return
	}

	b.WriteString(`<section class="git-correlation">`)
	b.WriteString(`<h2>Git Evidence</h2>`)
	if r.Git.RepoRoot != "" {
		b.WriteString(fmt.Sprintf(`<p class="repo-root">%s</p>`, html.EscapeString(r.Git.RepoRoot)))
	}

	if len(r.Git.UncommittedFiles) > 0 {
		b.WriteString(`<h3>Uncommitted Files</h3><ul class="file-status">`)
		for _, file := range r.Git.UncommittedFiles {
			b.WriteString(fmt.Sprintf(`<li><span>%s</span> <code>%s</code></li>`,
				html.EscapeString(file.Status), html.EscapeString(file.Path)))
		}
		b.WriteString(`</ul>`)
	}

	if len(r.Git.Commits) > 0 {
		b.WriteString(`<h3>Commits in Session Window</h3>`)
		b.WriteString(`<table><thead><tr><th>SHA</th><th>Subject</th><th>Files</th><th>Linked Exchange</th></tr></thead><tbody>`)

		commitToExchange := make(map[string]int)
		for _, link := range r.Git.ExchangeCommitLinks {
			commitToExchange[link.CommitSHA] = link.ExchangeIndex
		}

		for _, c := range r.Git.Commits {
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			exchangeLink := ""
			if idx, ok := commitToExchange[c.SHA]; ok {
				exchangeLink = fmt.Sprintf(`<a href="#exchange-%d">#%d</a>`, idx, idx)
			}
			b.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%d</td><td>%s</td></tr>`,
				html.EscapeString(sha), html.EscapeString(c.Subject), len(c.Files), exchangeLink))
		}

		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`</section>`)
}

func writeWorkspaceSection(b *strings.Builder, r *TraceResult) {
	if len(r.Workspace.Documents) == 0 && len(r.Workspace.Knowledge) == 0 {
		return
	}
	b.WriteString(`<section class="workspace"><h2>Workspace Context</h2>`)
	writeContextDocs(b, "Documents", r.Workspace.Documents)
	writeContextDocs(b, "Knowledge Entries", r.Workspace.Knowledge)
	b.WriteString(`</section>`)
}

func writeContextDocs(b *strings.Builder, label string, docs []ContextDocument) {
	if len(docs) == 0 {
		return
	}
	b.WriteString(fmt.Sprintf(`<h3>%s</h3>`, html.EscapeString(label)))
	b.WriteString(`<div class="doc-list">`)
	for _, doc := range docs {
		b.WriteString(`<details class="context-doc">`)
		title := doc.Title
		if title == "" {
			title = doc.Path
		}
		b.WriteString(fmt.Sprintf(`<summary><code>%s</code> <span>%s</span></summary>`,
			html.EscapeString(doc.Path), html.EscapeString(title)))
		if doc.Excerpt != "" {
			b.WriteString(`<pre>`)
			b.WriteString(html.EscapeString(doc.Excerpt))
			b.WriteString(`</pre>`)
		} else {
			b.WriteString(fmt.Sprintf(`<p class="doc-meta">%d bytes · sha256 %s</p>`,
				doc.Bytes, html.EscapeString(shortHash(doc.SHA256))))
		}
		b.WriteString(`</details>`)
	}
	b.WriteString(`</div>`)
}

func writeStats(b *strings.Builder, r *TraceResult) {
	b.WriteString(`<footer class="trace-stats">`)
	b.WriteString(fmt.Sprintf(`<p>Session: %s`, html.EscapeString(r.Session.ID)))
	if r.Session.CWD != "" {
		b.WriteString(fmt.Sprintf(` · CWD: %s`, html.EscapeString(r.Session.CWD)))
	}
	if r.Session.GitBranch != "" {
		b.WriteString(fmt.Sprintf(` · Branch: %s`, html.EscapeString(r.Session.GitBranch)))
	}
	b.WriteString(`</p>`)
	b.WriteString(`<p class="generated">Generated by ccx trace. Use ccx-context-fold to interpret decisions.</p>`)
	b.WriteString(`</footer>`)
}

func preview(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func traceCSS() string {
	return `
:root, [data-theme="dark"] {
  --bg: #151516; --surface: #202124; --surface-2: #292b2f; --text: #e7e3dc; --dim: #9aa0a6;
  --primary: #da7756; --correction: #ef4444; --sidechain: #8b5cf6;
  --commit: #10b981; --warning: #f59e0b; --border: #3a3d42;
}
[data-theme="light"] {
  --bg: #f7f7f5; --surface: #fff; --surface-2: #f0f1f2; --text: #1f2328; --dim: #656d76;
  --primary: #b45135; --correction: #dc2626; --sidechain: #7c3aed;
  --commit: #047857; --warning: #b45309; --border: #d8dee4;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font: 14px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: var(--bg); color: var(--text); }
.container { max-width: 1100px; margin: 0 auto; padding: 24px; }
.trace-header { margin-bottom: 20px; border-bottom: 1px solid var(--border); padding-bottom: 16px; }
.trace-header h1 { font-size: 22px; color: var(--primary); font-weight: 650; }
.meta { margin-top: 8px; color: var(--dim); font-size: 13px; }
.meta-item { white-space: nowrap; }
.correction-count { color: var(--correction); font-weight: 600; }
.theme-toggle { float: right; background: var(--surface); border: 1px solid var(--border); color: var(--text); padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px; }
h2 { font-size: 17px; margin: 24px 0 10px; }
h3 { font-size: 14px; margin: 14px 0 8px; color: var(--dim); }
.summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 8px; margin: 16px 0 20px; }
.metric { background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: 10px; }
.metric-value { display: block; font-size: 20px; font-weight: 700; color: var(--primary); }
.metric-label { color: var(--dim); font-size: 12px; }
.warning { background: color-mix(in srgb, var(--warning) 16%, var(--surface)); border: 1px solid var(--warning); border-radius: 6px; padding: 10px 12px; margin-bottom: 8px; }
.warning strong { color: var(--warning); }
.exchange { background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: 12px 16px; margin-bottom: 8px; }
.exchange.correction { border-left: 3px solid var(--correction); }
.exchange.has-sidechain { border-left: 3px solid var(--sidechain); }
.exchange header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
.exchange-num { font-weight: 700; color: var(--dim); font-size: 13px; }
.badge { font-size: 11px; padding: 1px 6px; border-radius: 3px; font-weight: 600; background: var(--surface-2); color: var(--text); }
.badge.correction { background: var(--correction); color: #fff; }
.badge.command { background: var(--primary); color: #fff; }
.badge.thinking { background: #3b82f6; color: #fff; }
.badge.sidechain { background: var(--sidechain); color: #fff; }
.commit { font-size: 12px; color: var(--commit); }
.user-text { color: var(--text); margin-bottom: 8px; white-space: pre-wrap; }
.path-list { font-size: 12px; color: var(--dim); margin-bottom: 4px; }
.path-list code { color: var(--primary); }
.tools { display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 4px; }
.tool-badge { font-size: 11px; background: var(--surface-2); border: 1px solid var(--border); padding: 0 4px; border-radius: 2px; color: var(--dim); }
details { margin-top: 6px; }
summary { cursor: pointer; color: var(--dim); }
.tool-call { font-size: 12px; margin: 4px 0; }
.assistant-text { font-size: 13px; color: var(--dim); }
.assistant-text p { margin-top: 8px; white-space: pre-wrap; }
.sidechain-card { background: var(--surface); border: 1px solid var(--border); border-left: 3px solid var(--sidechain); border-radius: 6px; padding: 10px 12px; margin-bottom: 8px; }
.sidechain-card p { color: var(--dim); font-size: 12px; margin-bottom: 6px; }
.git-correlation table { width: 100%; border-collapse: collapse; font-size: 13px; }
.git-correlation th, .git-correlation td { text-align: left; padding: 5px 8px; border-bottom: 1px solid var(--border); }
.git-correlation th { color: var(--dim); font-weight: 600; }
.git-correlation a { color: var(--primary); }
.repo-root { color: var(--dim); font-size: 12px; margin-bottom: 8px; }
.file-status { list-style: none; margin-bottom: 10px; }
.file-status li { font-size: 12px; margin-bottom: 2px; }
.file-status span { display: inline-block; width: 34px; color: var(--warning); font-weight: 700; }
.context-doc { background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px; margin-bottom: 6px; }
.context-doc summary { display: flex; gap: 10px; align-items: center; }
.context-doc pre { margin-top: 8px; padding: 10px; overflow: auto; max-height: 260px; background: var(--surface-2); border-radius: 4px; white-space: pre-wrap; font-size: 12px; }
.doc-meta { margin-top: 6px; color: var(--dim); font-size: 12px; }
.trace-stats { margin-top: 24px; padding-top: 16px; border-top: 1px solid var(--border); font-size: 12px; color: var(--dim); }
.generated { margin-top: 4px; font-style: italic; }
@media print { body { background: #fff; color: #000; } .theme-toggle { display: none; } details { display: block; } details[open] summary { display: none; } }
`
}

func traceJS() string {
	return `
function toggleTheme() {
  var h = document.documentElement;
  h.setAttribute('data-theme', h.getAttribute('data-theme') === 'dark' ? 'light' : 'dark');
}
document.addEventListener('keydown', function(e) {
  if (e.key === 'd') toggleTheme();
});
`
}
