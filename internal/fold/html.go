package fold

import (
	"fmt"
	"html"
	"strings"
)

func RenderHTML(result *FoldResult) string {
	if result == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Fold: `)
	b.WriteString(html.EscapeString(result.Session.Summary))
	b.WriteString(`</title>
<style>`)
	b.WriteString(foldCSS())
	b.WriteString(`</style>
</head>
<body>
<div class="container">
`)

	writeHeader(&b, result)
	writeTurns(&b, result)
	writeGitSection(&b, result)
	writeStats(&b, result)

	b.WriteString(`</div>
<script>`)
	b.WriteString(foldJS())
	b.WriteString(`</script>
</body>
</html>`)

	return b.String()
}

func writeHeader(b *strings.Builder, r *FoldResult) {
	b.WriteString(`<header class="fold-header">`)
	b.WriteString(fmt.Sprintf(`<h1>Session Fold: %s</h1>`, html.EscapeString(r.Session.Summary)))
	b.WriteString(`<div class="meta">`)

	var parts []string
	if r.Session.ID != "" {
		id := r.Session.ID
		if len(id) > 8 {
			id = id[:8]
		}
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">ID: %s</span>`, id))
	}
	if !r.Session.Start.IsZero() {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%s</span>`, r.Session.Start.Format("2006-01-02 15:04")))
	}
	if r.Session.Model != "" {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%s</span>`, html.EscapeString(r.Session.Model)))
	}
	parts = append(parts, fmt.Sprintf(`<span class="meta-item">%d turns</span>`, r.Stats.TurnCount))
	if r.Stats.Corrections > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item correction-count">%d corrections</span>`, r.Stats.Corrections))
	}
	if r.Stats.CommitsLinked > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">%d commits</span>`, r.Stats.CommitsLinked))
	}
	if r.Stats.TotalCostUSD > 0 {
		parts = append(parts, fmt.Sprintf(`<span class="meta-item">$%.4f</span>`, r.Stats.TotalCostUSD))
	}
	b.WriteString(strings.Join(parts, " · "))
	b.WriteString(`</div>`)
	b.WriteString(`<button onclick="toggleTheme()" class="theme-toggle">Toggle theme</button>`)
	b.WriteString(`</header>`)
}

func writeTurns(b *strings.Builder, r *FoldResult) {
	b.WriteString(`<section class="turns">`)

	for _, t := range r.Turns {
		class := "turn"
		if t.HasCorrection {
			class += " correction"
		}
		if t.Sidechain != nil {
			class += " has-sidechain"
		}

		b.WriteString(fmt.Sprintf(`<article class="%s" id="turn-%d">`, class, t.Index))
		b.WriteString(fmt.Sprintf(`<header><span class="turn-num">#%d</span>`, t.Index))

		if t.HasCorrection {
			b.WriteString(`<span class="badge correction">correction</span>`)
		}
		if t.IsCommand {
			b.WriteString(fmt.Sprintf(`<span class="badge command">/%s</span>`, html.EscapeString(t.CommandName)))
		}
		if t.HasThinking {
			b.WriteString(`<span class="badge thinking">thinking</span>`)
		}
		if t.Sidechain != nil {
			b.WriteString(fmt.Sprintf(`<span class="badge sidechain">agent: %s</span>`, html.EscapeString(t.Sidechain.AgentType)))
		}
		if len(t.LinkedCommits) > 0 {
			for _, sha := range t.LinkedCommits {
				short := sha
				if len(short) > 7 {
					short = short[:7]
				}
				b.WriteString(fmt.Sprintf(` <code class="commit">%s</code>`, short))
			}
		}
		b.WriteString(`</header>`)

		if t.UserText != "" {
			b.WriteString(`<div class="user-text">`)
			userPreview := t.UserText
			if len(userPreview) > 200 {
				userPreview = userPreview[:200] + "..."
			}
			b.WriteString(html.EscapeString(userPreview))
			b.WriteString(`</div>`)
		}

		if len(t.FilesEdited) > 0 {
			b.WriteString(`<div class="files-edited">Edited: `)
			for i, f := range t.FilesEdited {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf(`<code>%s</code>`, html.EscapeString(f)))
			}
			b.WriteString(`</div>`)
		}

		if len(t.ToolsUsed) > 0 {
			b.WriteString(`<div class="tools">`)
			for _, tool := range t.ToolsUsed {
				b.WriteString(fmt.Sprintf(`<span class="tool-badge">%s</span>`, html.EscapeString(tool)))
			}
			b.WriteString(`</div>`)
		}

		if t.AssistantText != "" {
			b.WriteString(`<details class="assistant-text"><summary>Assistant response</summary><p>`)
			b.WriteString(html.EscapeString(t.AssistantText))
			b.WriteString(`</p></details>`)
		}

		b.WriteString(`</article>`)
	}

	b.WriteString(`</section>`)
}

func writeGitSection(b *strings.Builder, r *FoldResult) {
	if len(r.Git.Commits) == 0 {
		return
	}

	b.WriteString(`<section class="git-correlation">`)
	b.WriteString(`<h2>Git Commits in Session Window</h2>`)
	b.WriteString(`<table><thead><tr><th>SHA</th><th>Subject</th><th>Files</th><th>Linked Turn</th></tr></thead><tbody>`)

	commitToTurn := make(map[string]int)
	for _, link := range r.Git.TurnCommitLinks {
		commitToTurn[link.CommitSHA] = link.TurnIndex
	}

	for _, c := range r.Git.Commits {
		sha := c.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		turnLink := ""
		if idx, ok := commitToTurn[c.SHA]; ok {
			turnLink = fmt.Sprintf(`<a href="#turn-%d">#%d</a>`, idx, idx)
		}
		b.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			sha, html.EscapeString(c.Subject), len(c.Files), turnLink))
	}

	b.WriteString(`</tbody></table></section>`)
}

func writeStats(b *strings.Builder, r *FoldResult) {
	b.WriteString(`<footer class="fold-stats">`)
	b.WriteString(fmt.Sprintf(`<p>Session: %s`, html.EscapeString(r.Session.ID)))
	if r.Session.CWD != "" {
		b.WriteString(fmt.Sprintf(` · CWD: %s`, html.EscapeString(r.Session.CWD)))
	}
	if r.Session.GitBranch != "" {
		b.WriteString(fmt.Sprintf(` · Branch: %s`, html.EscapeString(r.Session.GitBranch)))
	}
	b.WriteString(`</p>`)
	b.WriteString(`<p class="generated">Generated by ccx fold</p>`)
	b.WriteString(`</footer>`)
}

func foldCSS() string {
	return `
:root, [data-theme="dark"] {
  --bg: #1a1a2e; --surface: #232340; --text: #e0e0e0; --dim: #888;
  --primary: #da7756; --correction: #ef4444; --sidechain: #8b5cf6;
  --commit: #10b981; --border: #333;
}
[data-theme="light"] {
  --bg: #f5f5f5; --surface: #fff; --text: #1a1a1a; --dim: #666;
  --primary: #c45a3c; --correction: #dc2626; --sidechain: #7c3aed;
  --commit: #059669; --border: #ddd;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font: 14px/1.6 -apple-system, system-ui, sans-serif; background: var(--bg); color: var(--text); }
.container { max-width: 900px; margin: 0 auto; padding: 24px; }
.fold-header { margin-bottom: 24px; border-bottom: 1px solid var(--border); padding-bottom: 16px; }
.fold-header h1 { font-size: 20px; color: var(--primary); }
.meta { margin-top: 8px; color: var(--dim); font-size: 13px; }
.meta-item { white-space: nowrap; }
.correction-count { color: var(--correction); font-weight: 600; }
.theme-toggle { float: right; background: var(--surface); border: 1px solid var(--border); color: var(--text); padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px; }
.turn { background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: 12px 16px; margin-bottom: 8px; }
.turn.correction { border-left: 3px solid var(--correction); }
.turn.has-sidechain { border-left: 3px solid var(--sidechain); }
.turn header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
.turn-num { font-weight: 700; color: var(--dim); font-size: 13px; }
.badge { font-size: 11px; padding: 1px 6px; border-radius: 3px; font-weight: 600; }
.badge.correction { background: var(--correction); color: #fff; }
.badge.command { background: var(--primary); color: #fff; }
.badge.thinking { background: #3b82f6; color: #fff; }
.badge.sidechain { background: var(--sidechain); color: #fff; }
.commit { font-size: 12px; color: var(--commit); }
.user-text { color: var(--text); margin-bottom: 6px; white-space: pre-wrap; }
.files-edited { font-size: 12px; color: var(--dim); margin-bottom: 4px; }
.files-edited code { color: var(--primary); }
.tools { display: flex; gap: 4px; flex-wrap: wrap; margin-bottom: 4px; }
.tool-badge { font-size: 11px; background: var(--surface); border: 1px solid var(--border); padding: 0 4px; border-radius: 2px; color: var(--dim); }
.assistant-text { font-size: 13px; color: var(--dim); }
.assistant-text summary { cursor: pointer; }
.assistant-text p { margin-top: 8px; white-space: pre-wrap; }
.git-correlation { margin-top: 24px; }
.git-correlation h2 { font-size: 16px; margin-bottom: 8px; }
.git-correlation table { width: 100%; border-collapse: collapse; font-size: 13px; }
.git-correlation th, .git-correlation td { text-align: left; padding: 4px 8px; border-bottom: 1px solid var(--border); }
.git-correlation th { color: var(--dim); font-weight: 600; }
.git-correlation a { color: var(--primary); }
.fold-stats { margin-top: 24px; padding-top: 16px; border-top: 1px solid var(--border); font-size: 12px; color: var(--dim); }
.generated { margin-top: 4px; font-style: italic; }
@media print { body { background: #fff; color: #000; } .theme-toggle { display: none; } details { display: block; } details[open] summary { display: none; } }
`
}

func foldJS() string {
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
