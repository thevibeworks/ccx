package web

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	ccxconfig "github.com/thevibeworks/ccx/internal/config"
	"github.com/thevibeworks/ccx/internal/parser"
)

var idSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeID(s string) string {
	return idSanitizer.ReplaceAllString(s, "")
}

// isSafeURL returns true if url has http or https scheme
func isSafeURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func projectProviders(p *parser.Project) []string {
	seen := make(map[string]bool)
	for _, s := range p.Sessions {
		if s.Provider != "" {
			seen[s.Provider] = true
		}
	}
	var providers []string
	for _, id := range []string{"claude-code", "codex"} {
		if seen[id] {
			providers = append(providers, id)
		}
	}
	return providers
}

func projectProvider(p *parser.Project) string {
	providers := projectProviders(p)
	if len(providers) == 1 {
		return providers[0]
	}
	if len(providers) > 1 {
		return "multi"
	}
	return ""
}

func providerTag(id string) string {
	switch id {
	case "claude-code":
		return "CC"
	case "codex":
		return "CX"
	default:
		return ""
	}
}

func providerBadgeHTML(id string) string {
	tag := providerTag(id)
	if tag == "" {
		return ""
	}
	return fmt.Sprintf(`<span class="provider-badge provider-%s">%s</span>`, tag, tag)
}

func providerBadgesHTML(providers []string) string {
	var b strings.Builder
	for _, id := range providers {
		b.WriteString(providerBadgeHTML(id))
	}
	return b.String()
}

func parseProviderQuery(q string) (provider, query string) {
	q = strings.TrimSpace(q)
	for _, prefix := range []string{"cc:", "claude-code:", "claude:"} {
		if strings.HasPrefix(strings.ToLower(q), prefix) {
			return "claude-code", strings.TrimSpace(q[len(prefix):])
		}
	}
	for _, prefix := range []string{"cx:", "codex:"} {
		if strings.HasPrefix(strings.ToLower(q), prefix) {
			return "codex", strings.TrimSpace(q[len(prefix):])
		}
	}
	return "", q
}

func renderIndexPage(projects []*parser.Project, totalSessions int, search, sortBy string) string {
	var b strings.Builder

	b.WriteString(pageHeader("ccx", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("projects"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header page-header-projects">`)
	b.WriteString(`<span class="page-badge badge-project">P</span>`)
	b.WriteString(`<h1>Projects</h1>`)
	b.WriteString(fmt.Sprintf(`<div class="stats">%d projects / %d sessions</div>`, len(projects), totalSessions))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="controls">`)
	b.WriteString(`<div class="search-wrap">`)
	b.WriteString(fmt.Sprintf(`<input type="text" id="search" class="search-input" placeholder="Search projects... (press /)" value="%s">`, html.EscapeString(search)))
	b.WriteString(`<span class="search-spinner" id="search-spinner"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="sort-controls">`)
	b.WriteString(`<span class="sort-label">Sort:</span>`)
	b.WriteString(fmt.Sprintf(`<select id="sort" class="sort-select">
		<option value="time"%s>Recent</option>
		<option value="name"%s>Name</option>
		<option value="sessions"%s>Sessions</option>
	</select>`, selected(sortBy, "time"), selected(sortBy, "name"), selected(sortBy, "sessions")))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="card-grid" id="results">`)
	for _, p := range projects {
		sessionsLabel := "sessions"
		if len(p.Sessions) == 1 {
			sessionsLabel = "session"
		}
		displayName := parser.GetProjectDisplayName(p.EncodedName)
		badges := providerBadgesHTML(projectProviders(p))
		prov := projectProvider(p)
		provAttr := ""
		if prov != "" {
			provAttr = fmt.Sprintf(` data-provider="%s"`, prov)
		}
		b.WriteString(fmt.Sprintf(`
<a href="/project/%s" class="card project-card"%s>
	<div class="card-header">
		<span class="card-title">%s</span>
		<span class="card-providers">%s</span>
	</div>
	<div class="card-stats">
		<span class="stat">%d %s</span>
		<span class="stat-sep">&middot;</span>
		<span class="stat">%s</span>
	</div>
</a>`, html.EscapeString(p.EncodedName), provAttr, html.EscapeString(displayName), badges, len(p.Sessions), sessionsLabel, formatAge(p.LastModified)))
	}
	b.WriteString(`</div>`)

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(indexJS())
	b.WriteString(pageFooter())

	return b.String()
}

func renderProjectPage(project *parser.Project, sessions []*parser.Session, allProjects []*parser.Project, memFiles []MemoryFile, search, sortBy string) string {
	var b strings.Builder

	b.WriteString(pageHeader(project.Name+" - ccx", "light"))
	b.WriteString(renderTopNav(project.EncodedName, ""))
	b.WriteString(`<div class="layout two-panel">`)

	// Left panel: Projects list
	b.WriteString(`<aside class="panel-nav">`)
	b.WriteString(`<div class="panel-header"><a href="/">Projects</a></div>`)
	b.WriteString(`<div class="panel-list">`)
	for _, p := range allProjects {
		active := ""
		if p.EncodedName == project.EncodedName {
			active = " active"
		}
		displayName := parser.GetProjectDisplayName(p.EncodedName)
		badges := providerBadgesHTML(projectProviders(p))
		b.WriteString(fmt.Sprintf(`<a href="/project/%s" class="panel-item%s" title="%s"><span class="panel-item-name">%s</span> %s</a>`,
			html.EscapeString(p.EncodedName), active, html.EscapeString(displayName), html.EscapeString(truncate(displayName, 20)), badges))
	}
	b.WriteString(`</div>`)
	b.WriteString(`</aside>`)

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header page-header-sessions">`)
	b.WriteString(fmt.Sprintf(`<div class="breadcrumb"><a href="/">Projects</a> <span class="sep">/</span> <span class="current">%s</span></div>`, html.EscapeString(project.Name)))
	b.WriteString(`<span class="page-badge badge-session">S</span>`)
	b.WriteString(fmt.Sprintf(`<h1>%s</h1>`, html.EscapeString(project.Name)))
	b.WriteString(fmt.Sprintf(`<div class="stats">%d sessions</div>`, len(sessions)))
	b.WriteString(`</div>`)

	// Memory section (above session list)
	if len(memFiles) > 0 {
		b.WriteString(`<details class="mem-section" id="mem-section" open>`)
		b.WriteString(fmt.Sprintf(`<summary class="mem-section-header"><span class="mem-icon">◇</span> Memory <span class="mem-badge">%d</span></summary>`, len(memFiles)))
		b.WriteString(`<div class="mem-section-body">`)
		for i, f := range memFiles {
			provClass := "mem-file-cc"
			if f.Provider == "codex" {
				provClass = "mem-file-cx"
			}
			b.WriteString(fmt.Sprintf(`<details class="mem-file %s" data-path="%s" data-idx="%d">`, provClass, html.EscapeString(f.FilePath), i))
			b.WriteString(fmt.Sprintf(`<summary class="mem-file-row"><code class="mem-file-name">%s</code><span class="mem-file-path">%s</span><span class="expand-icon">▶</span></summary>`,
				html.EscapeString(f.Name), html.EscapeString(truncatePath(f.FilePath, 50))))
			b.WriteString(fmt.Sprintf(`<div class="file-viewer" id="mem-%d">`, i))
			b.WriteString(`<div class="file-toolbar"><button class="mode-btn" data-mode="fmt">fmt</button><button class="mode-btn active" data-mode="raw">raw</button><button class="copy-btn">copy</button></div>`)
			b.WriteString(`<div class="file-content"><div class="loading">Loading...</div></div>`)
			b.WriteString(`</div></details>`)
		}
		b.WriteString(`</div></details>`)
	}

	b.WriteString(`<div class="controls">`)
	b.WriteString(`<div class="search-wrap">`)
	b.WriteString(fmt.Sprintf(`<input type="text" id="search" class="search-input" placeholder="Search sessions... (press /)" value="%s">`, html.EscapeString(search)))
	b.WriteString(`<span class="search-spinner" id="search-spinner"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="sort-controls">`)
	b.WriteString(`<select id="provider-filter" class="sort-select" title="Filter by provider"><option value="all">All</option><option value="claude-code">Claude Code</option><option value="codex">Codex</option></select>`)
	b.WriteString(`<span class="sort-label">Sort:</span>`)
	b.WriteString(fmt.Sprintf(`<select id="sort" class="sort-select">
		<option value="time"%s>Recent</option>
		<option value="messages"%s>Messages</option>
	</select>`, selected(sortBy, "time"), selected(sortBy, "messages")))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="session-list" id="results">`)
	for _, s := range sessions {
		summary := s.Summary
		totalTokens := s.Stats.InputTokens + s.Stats.OutputTokens
		tokenDisplay := ""
		if totalTokens > 0 {
			tokenDisplay = fmt.Sprintf(`<span class="stat stat-tokens" title="Total tokens"><span class="stat-icon">⧫</span> %s</span>`, formatTokens(totalTokens))
		}
		badge := providerBadgeHTML(s.Provider)
		provAttr := ""
		if s.Provider != "" {
			provAttr = fmt.Sprintf(` data-provider="%s"`, s.Provider)
		}
		b.WriteString(fmt.Sprintf(`
<a href="/session/%s/%s" class="card session-card"%s>
	<div class="session-header">
		%s<code class="session-id">%s</code>
		<span class="session-time" title="%s">%s</span>
	</div>
	<div class="session-summary">%s</div>
	<div class="session-stats">
		<span class="stat"><span class="stat-icon">M</span> %d</span>
		<span class="stat"><span class="stat-icon">T</span> %d</span>
		%s
	</div>
</a>`, html.EscapeString(project.EncodedName), html.EscapeString(s.ID),
			provAttr, badge,
			html.EscapeString(truncate(s.ID, 8)),
			s.StartTime.Format("2006-01-02 15:04"),
			formatRelativeTime(s.StartTime),
			html.EscapeString(summary),
			s.Stats.MessageCount, s.Stats.ToolCalls, tokenDisplay))
	}
	b.WriteString(`</div>`)

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(indexJS())
	if len(memFiles) > 0 {
		b.WriteString(memSectionCSS())
		b.WriteString(fileCardJS())
	}
	b.WriteString(pageFooter())

	return b.String()
}

func renderSessionPage(session *parser.Session, projectName string, allSessions []*parser.Session, memCount int, showThinking, showTools, loadAll bool, theme string) string {
	var b strings.Builder

	idPrefix := session.ID
	if len(idPrefix) > 8 {
		idPrefix = idPrefix[:8]
	}
	title := fmt.Sprintf("Session %s - ccx", idPrefix)
	b.WriteString(pageHeader(title, theme))
	// Hint the current session's provider to the CSS layer so the
	// loading spinner can pick up a provider-specific accent (e.g.
	// green for Codex). Non-session pages never set this attribute.
	if provider := strings.TrimSpace(session.Provider); provider != "" {
		b.WriteString(fmt.Sprintf(`<script>document.body.dataset.ccxProvider=%q;</script>`, provider))
	}
	b.WriteString(renderTopNav(projectName, session.ID))
	b.WriteString(`<div class="layout session-layout">`)

	// Left panel: Sessions list (only if we have sessions to show)
	if len(allSessions) > 0 {
		b.WriteString(`<aside class="panel-nav session-nav">`)
		b.WriteString(fmt.Sprintf(`<div class="panel-header"><a href="/project/%s">Sessions</a></div>`, html.EscapeString(projectName)))
		b.WriteString(`<div class="panel-list">`)
		for _, s := range allSessions {
			active := ""
			if s.ID == session.ID {
				active = " active"
			}
			summary := truncate(s.Summary, 32)
			if summary == "" {
				summary = truncate(s.ID, 8)
			}
			badge := providerBadgeHTML(s.Provider)
			provAttr := ""
			if s.Provider != "" {
				provAttr = fmt.Sprintf(` data-provider="%s"`, s.Provider)
			}
			b.WriteString(fmt.Sprintf(`<a href="/session/%s/%s" class="panel-item%s"%s title="%s"><span class="panel-id-row"><span class="panel-id">%s</span>%s</span><span class="panel-summary">%s</span></a>`,
				html.EscapeString(projectName), html.EscapeString(s.ID), active, provAttr,
				html.EscapeString(s.Summary), html.EscapeString(truncate(s.ID, 6)), badge, html.EscapeString(summary)))
		}
		b.WriteString(`</div>`)
		b.WriteString(`</aside>`)
	}

	// Conversation nav sidebar
	b.WriteString(`<aside class="nav-sidebar" id="nav-sidebar">`)
	b.WriteString(`<div class="sidebar-header">`)
	b.WriteString(`<h3>Outline</h3>`)
	b.WriteString(`<button class="icon-btn" onclick="toggleSidebar()" title="Toggle sidebar">`)
	b.WriteString(`<span id="toggle-icon">◀</span>`)
	b.WriteString(`</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="nav-list" id="nav-list">`)
	renderConversationNav(&b, session.RootMessages)
	b.WriteString(`</div>`)
	b.WriteString(`</aside>`)

	b.WriteString(`<div class="live-indicator"></div>`)
	b.WriteString(`<main class="main-content session-main">`)

	// Hidden controls for JS
	thinkingChecked := ""
	if showThinking {
		thinkingChecked = "checked"
	}
	toolsChecked := "checked"
	if !showTools {
		toolsChecked = ""
	}
	b.WriteString(fmt.Sprintf(`<input type="checkbox" id="show-thinking" style="display:none" %s>`, thinkingChecked))
	b.WriteString(fmt.Sprintf(`<input type="checkbox" id="show-tools" style="display:none" %s>`, toolsChecked))

	b.WriteString(`<div class="messages" id="messages">`)
	renderMessages(&b, session.RootMessages, 0, showThinking, showTools, loadAll)
	b.WriteString(`</div>`)

	// Tail spinner for watch mode
	b.WriteString(`<div class="tail-spinner"><span class="cli-spinner-char"></span> Tailing session...</div>`)

	// Tail output container for watch mode
	b.WriteString(`<div class="tail-output" id="tail-output" style="display:none"></div>`)

	b.WriteString(`</main>`)

	// Timeline rail — right-edge time-axis scrubber. Narrow by default,
	// expands on hover with a floating tooltip that snaps to the nearest
	// tick. Click to jump. Hidden on narrow viewports.
	renderTimelineRail(&b, session)

	// Bottom dock toolbar - horizontal, modern UX
	b.WriteString(`<div class="dock-toolbar" id="dock-toolbar">`)
	b.WriteString(`<div class="dock-group dock-nav">`)
	b.WriteString(`<button class="dock-btn" id="tb-prev-user" title="Previous user (k)"><span class="dock-icon">↑</span><span class="dock-key">k</span></button>`)
	b.WriteString(`<button class="dock-btn" id="tb-next-user" title="Next user (j)"><span class="dock-icon">↓</span><span class="dock-key">j</span></button>`)
	b.WriteString(`<button class="dock-btn" id="tb-top" title="Top (g)"><span class="dock-icon">⤒</span></button>`)
	b.WriteString(`<button class="dock-btn" id="tb-bottom" title="Bottom (G)"><span class="dock-icon">⤓</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="dock-sep"></div>`)
	b.WriteString(`<div class="dock-group dock-view">`)
	thinkingActive := ""
	if showThinking {
		thinkingActive = " active"
	}
	toolsActive := ""
	if showTools {
		toolsActive = " active"
	}
	b.WriteString(fmt.Sprintf(`<button class="dock-btn toggle%s" id="tb-thinking" title="Thinking (t)"><span class="dock-icon">∴</span><span class="dock-label">Think</span></button>`, thinkingActive))
	b.WriteString(fmt.Sprintf(`<button class="dock-btn toggle%s" id="tb-tools" title="Tools (o)"><span class="dock-icon">◎</span><span class="dock-label">Tools</span></button>`, toolsActive))
	b.WriteString(`</div>`)
	b.WriteString(`<div class="dock-sep"></div>`)
	b.WriteString(`<div class="dock-group dock-live">`)
	b.WriteString(`<button class="dock-btn live-btn" id="tb-watch" title="Watch live (w)"><span class="dock-icon">◉</span><span class="dock-label">Live</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="dock-sep"></div>`)
	b.WriteString(`<div class="dock-group dock-actions">`)
	b.WriteString(`<div class="dock-dropdown">`)
	b.WriteString(`<button class="dock-btn" id="tb-export" title="Export"><span class="dock-icon"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/></svg></span><span class="dock-label">Export</span></button>`)
	b.WriteString(`<div class="dock-menu" id="toolbar-export-menu">`)
	ep := fmt.Sprintf("/api/export/%s/%s", html.EscapeString(projectName), html.EscapeString(session.ID))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=md">Markdown</a>`, ep))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=html">HTML</a>`, ep))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=org">Org</a>`, ep))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=json">JSON</a>`, ep))
	b.WriteString(`<div class="dock-menu-sep"></div>`)
	b.WriteString(fmt.Sprintf(`<a href="%s?format=md&brief=1">Brief (md)</a>`, ep))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=html&brief=1">Brief (html)</a>`, ep))
	b.WriteString(fmt.Sprintf(`<a href="%s?format=org&brief=1">Brief (org)</a>`, ep))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<button class="dock-btn" id="tb-search" title="Search (/ or f)"><span class="dock-icon"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg></span><span class="dock-label">Find</span></button>`)
	b.WriteString(`<button class="dock-btn" id="tb-refresh" title="Refresh (r)"><span class="dock-icon"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 4v6h-6M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/></svg></span></button>`)
	b.WriteString(`<button class="dock-btn" id="tb-info" title="Info (i)"><span class="dock-icon">ⓘ</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	// Floating session search bar (hidden by default)
	b.WriteString(`<div class="session-search" id="session-search">`)
	b.WriteString(`<div class="search-row">`)
	b.WriteString(`<input type="text" id="search-input" placeholder="Search in session...">`)
	b.WriteString(`<span class="search-info" id="search-info"></span>`)
	b.WriteString(`<button class="search-nav" id="search-prev" title="Previous (N)">↑</button>`)
	b.WriteString(`<button class="search-nav" id="search-next" title="Next (n)">↓</button>`)
	b.WriteString(`<button class="search-close" id="search-close" title="Close (Esc)">×</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="search-filters">`)
	b.WriteString(`<label class="search-chip"><input type="checkbox" id="filter-user" checked><span>User</span></label>`)
	b.WriteString(`<label class="search-chip"><input type="checkbox" id="filter-response" checked><span>Response</span></label>`)
	b.WriteString(`<label class="search-chip"><input type="checkbox" id="filter-tools"><span>Tools</span></label>`)
	b.WriteString(`<label class="search-chip"><input type="checkbox" id="filter-agents"><span>Agents</span></label>`)
	b.WriteString(`<label class="search-chip"><input type="checkbox" id="filter-thinking"><span>Thinking</span></label>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	// Info panel (floating, hidden by default)
	projDisplay := parser.GetProjectDisplayName(projectName)
	b.WriteString(`<div class="info-panel" id="info-panel">`)

	// Context section
	b.WriteString(`<div class="info-section">`)
	b.WriteString(`<div class="info-section-header">Context</div>`)
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Project</span><a href="/project/%s">%s</a></div>`,
		html.EscapeString(projectName), html.EscapeString(projDisplay)))
	if memCount > 0 {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Memory</span><a href="/project/%s#mem-section" class="mem-link">%d files</a></div>`,
			html.EscapeString(projectName), memCount))
	}
	if session.Provider != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Provider</span>%s</div>`, providerBadgeHTML(session.Provider)))
	}
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Session</span><code class="copyable">%s</code><button class="copy-btn-sm" data-copy="%s">⧉</button></div>`,
		html.EscapeString(session.ID), html.EscapeString(session.ID)))
	if session.Title != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Title</span><span class="info-value">%s</span></div>`, html.EscapeString(session.Title)))
	}
	if session.Slug != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Slug</span><code class="copyable">%s</code><button class="copy-btn-sm" data-copy="%s">⧉</button></div>`,
			html.EscapeString(session.Slug), html.EscapeString(session.Slug)))
	}
	if session.Model != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Model</span><span class="info-value">%s</span></div>`, html.EscapeString(session.Model)))
	}
	if session.Version != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Version</span><span class="info-value">%s</span></div>`, html.EscapeString(session.Version)))
	}
	if session.GitBranch != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Branch</span><code>%s</code></div>`, html.EscapeString(session.GitBranch)))
	}
	if session.CWD != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">CWD</span><code class="info-cwd" title="%s">%s</code></div>`,
			html.EscapeString(session.CWD), html.EscapeString(truncatePath(session.CWD, 40))))
	}
	b.WriteString(`</div>`)

	// Time section
	b.WriteString(`<div class="info-section">`)
	b.WriteString(`<div class="info-section-header">Time</div>`)
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Started</span><span class="info-value">%s</span></div>`, session.StartTime.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Duration</span><span class="info-value">%s</span></div>`, formatDuration(session.Stats.DurationSeconds)))
	b.WriteString(`</div>`)

	// Activity section
	b.WriteString(`<div class="info-section">`)
	b.WriteString(`<div class="info-section-header">Activity</div>`)
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Messages</span><span class="info-value">%d</span></div>`, session.Stats.MessageCount))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">User prompts</span><span class="info-value">%d</span></div>`, session.Stats.UserPrompts))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Tool calls</span><span class="info-value">%d</span></div>`, session.Stats.ToolCalls))
	if session.Stats.AgentSidechains > 0 {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Agent tasks</span><span class="info-value">%d</span></div>`, session.Stats.AgentSidechains))
	}
	b.WriteString(`</div>`)

	// Token usage section (if available)
	totalTokens := session.Stats.InputTokens + session.Stats.OutputTokens
	if totalTokens > 0 {
		b.WriteString(`<div class="info-section info-section-tokens">`)
		b.WriteString(`<div class="info-section-header">Tokens</div>`)
		b.WriteString(fmt.Sprintf(`<div class="info-row" title="Fresh tokens sent to API (not from cache)"><span class="info-label">Input</span><span class="info-value">%s</span></div>`, formatTokens(session.Stats.InputTokens)))
		b.WriteString(fmt.Sprintf(`<div class="info-row" title="Tokens generated by the assistant"><span class="info-label">Output</span><span class="info-value">%s</span></div>`, formatTokens(session.Stats.OutputTokens)))
		// Show cache stats if present
		if session.Stats.CacheReadTokens > 0 || session.Stats.CacheCreateTokens > 0 {
			if session.Stats.CacheReadTokens > 0 {
				b.WriteString(fmt.Sprintf(`<div class="info-row info-cache" title="Tokens read from prompt cache (90%% cheaper)"><span class="info-label">↩ Cache read</span><span class="info-value">%s</span></div>`, formatTokens(session.Stats.CacheReadTokens)))
			}
			if session.Stats.CacheCreateTokens > 0 {
				b.WriteString(fmt.Sprintf(`<div class="info-row info-cache" title="Tokens written to prompt cache"><span class="info-label">↪ Cache write</span><span class="info-value">%s</span></div>`, formatTokens(session.Stats.CacheCreateTokens)))
			}
		}
		b.WriteString(fmt.Sprintf(`<div class="info-row info-total" title="Input + Output tokens"><span class="info-label">Total</span><span class="info-value"><strong>%s</strong></span></div>`, formatTokens(totalTokens)))
		// Cost row, shown when pricing resolved for at least one model
		if session.Stats.CostUSD > 0 {
			b.WriteString(fmt.Sprintf(`<div class="info-row info-cost" title="Sum of per-message USD cost using pinned Anthropic list pricing"><span class="info-label">Cost</span><span class="info-value"><strong>%s</strong></span></div>`, formatCost(session.Stats.CostUSD)))
		}
		b.WriteString(`</div>`)
	}

	// Per-turn spend section — the crown jewel of v0.next.
	// Only render when we have at least one turn with billable usage.
	allMsgs := flattenMessages(session.RootMessages)
	turns := parser.ComputeExchanges(allMsgs)
	if hasBillableUsage(turns) {
		b.WriteString(renderSpendSection(turns, session.Stats.CostUSD))
	}

	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(sessionJS(projectName, session.ID))
	b.WriteString(pageFooter())

	return b.String()
}

const (
	progressiveLoadThreshold = 500 // Messages above this trigger progressive loading
	initialContextSections   = 3   // Number of compact sections to show initially
	maxProjectsInitial       = 50  // Max projects to show initially (future: load more)
	maxSessionsInitial       = 100 // Max sessions per project initially (future: load more)
)

// splitByCompactBoundaries splits messages into sections delimited by compact summaries
func splitByCompactBoundaries(messages []*parser.Message) [][]*parser.Message {
	var sections [][]*parser.Message
	var current []*parser.Message

	for _, msg := range messages {
		if msg.Kind == parser.KindCompactSummary {
			if len(current) > 0 {
				sections = append(sections, current)
			}
			current = []*parser.Message{msg}
		} else {
			current = append(current, msg)
		}
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

// splitByUserPrompts splits messages into chunks, breaking before user prompts when chunk reaches target size
func splitByUserPrompts(messages []*parser.Message, chunkSize int) [][]*parser.Message {
	var sections [][]*parser.Message
	var current []*parser.Message

	for _, msg := range messages {
		// Break before user prompt if we're at capacity (so chunks start with user prompt)
		if msg.Kind == parser.KindUserPrompt && len(current) >= chunkSize {
			sections = append(sections, current)
			current = nil
		}
		current = append(current, msg)
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

func renderMessages(b *strings.Builder, messages []*parser.Message, depth int, showThinking, showTools, loadAll bool) {
	allMsgs := flattenMessages(messages)
	mainMsgs := filterMainConversation(allMsgs)
	sidechainGroups := groupSidechainsByAgent(allMsgs)
	scMap := matchSidechainsToToolUse(mainMsgs, sidechainGroups)

	if !loadAll && len(mainMsgs) > progressiveLoadThreshold {
		renderMessagesProgressive(b, mainMsgs, showThinking, showTools, scMap)
		return
	}

	toolResults := buildToolResultsMap(allMsgs)

	var currentThread []*parser.Message
	inThread := false

	for _, msg := range mainMsgs {
		if msg.Kind == parser.KindToolResult {
			continue
		}

		isAnchor := msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand

		if isAnchor {
			if inThread && len(currentThread) > 0 {
				renderThread(b, currentThread, showThinking, showTools, toolResults, scMap)
			}
			currentThread = []*parser.Message{msg}
			inThread = true
		} else if inThread {
			currentThread = append(currentThread, msg)
		} else {
			renderTurnMessage(b, msg, showThinking, showTools, 0, toolResults)
		}
	}

	if inThread && len(currentThread) > 0 {
		renderThread(b, currentThread, showThinking, showTools, toolResults, scMap)
	}
}

// renderMessagesProgressive renders large conversations with lazy loading
func renderMessagesProgressive(b *strings.Builder, allMsgs []*parser.Message, showThinking, showTools bool, scMap map[string]sidechainGroup) {
	sections := splitByCompactBoundaries(allMsgs)

	// If no compact boundaries, fall back to splitting by user prompts
	if len(sections) <= 1 && len(allMsgs) > progressiveLoadThreshold {
		sections = splitByUserPrompts(allMsgs, 50) // ~50 messages per chunk
	}

	totalSections := len(sections)

	// Calculate which sections to render initially
	startSection := 0
	if totalSections > initialContextSections {
		startSection = totalSections - initialContextSections
	}

	hiddenMsgCount := 0
	for i := 0; i < startSection; i++ {
		hiddenMsgCount += len(sections[i])
	}

	// Add "Load earlier" button if there's hidden content
	if startSection > 0 {
		b.WriteString(fmt.Sprintf(`<div class="load-earlier" id="load-earlier" data-hidden-sections="%d">`, startSection))
		b.WriteString(`<button class="load-earlier-btn" onclick="loadEarlierMessages()">`)
		b.WriteString(fmt.Sprintf(`<span class="load-icon">↑</span> Load earlier context (%d sections, ~%d messages)`, startSection, hiddenMsgCount))
		b.WriteString(`</button></div>`)
	}

	// Collect messages from visible sections
	var visibleMsgs []*parser.Message
	for i := startSection; i < totalSections; i++ {
		visibleMsgs = append(visibleMsgs, sections[i]...)
	}

	// Build tool results map
	toolResults := buildToolResultsMap(allMsgs) // Need full list for tool result lookups

	// Render visible messages using standard thread logic
	var currentThread []*parser.Message
	inThread := false

	for _, msg := range visibleMsgs {
		if msg.Kind == parser.KindToolResult {
			continue
		}

		isAnchor := msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand

		if isAnchor {
			if inThread && len(currentThread) > 0 {
				renderThread(b, currentThread, showThinking, showTools, toolResults, scMap)
			}
			currentThread = []*parser.Message{msg}
			inThread = true
		} else if inThread {
			currentThread = append(currentThread, msg)
		} else {
			renderTurnMessage(b, msg, showThinking, showTools, 0, toolResults)
		}
	}

	if inThread && len(currentThread) > 0 {
		renderThread(b, currentThread, showThinking, showTools, toolResults, scMap)
	}
}

// buildToolResultsMap creates a map of toolID -> result content
func buildToolResultsMap(messages []*parser.Message) map[string]parser.ContentBlock {
	results := make(map[string]parser.ContentBlock)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "tool_result" && block.ToolID != "" {
				results[block.ToolID] = block
			}
		}
	}
	return results
}

type sidechainGroup struct {
	AgentID     string
	AgentType   string
	Description string
	Messages    []*parser.Message
	Result      *parser.SubAgentResultData // from toolUseResult on the tool_result message
}

func groupSidechainsByAgent(allMsgs []*parser.Message) []sidechainGroup {
	order := make([]string, 0)
	byAgent := make(map[string][]*parser.Message)
	for _, m := range allMsgs {
		if !m.IsSidechain || m.AgentID == "" {
			continue
		}
		if _, seen := byAgent[m.AgentID]; !seen {
			order = append(order, m.AgentID)
		}
		byAgent[m.AgentID] = append(byAgent[m.AgentID], m)
	}
	groups := make([]sidechainGroup, 0, len(order))
	for _, id := range order {
		groups = append(groups, sidechainGroup{AgentID: id, Messages: byAgent[id]})
	}
	return groups
}

var subagentToolSet = map[string]bool{
	"Task": true, "TaskCreate": true, "Agent": true,
}

type agentDispatch struct {
	ToolID      string
	AgentType   string
	Description string
}

func matchSidechainsToToolUse(mainMsgs []*parser.Message, groups []sidechainGroup) map[string]sidechainGroup {
	if len(groups) == 0 {
		return nil
	}

	var dispatches []agentDispatch
	resultByToolID := make(map[string]*parser.SubAgentResultData)

	for _, msg := range mainMsgs {
		for _, block := range msg.Content {
			if block.Type == "tool_use" && subagentToolSet[block.ToolName] {
				d := agentDispatch{ToolID: block.ToolID}
				if m, ok := block.ToolInput.(map[string]any); ok {
					d.AgentType, _ = m["subagent_type"].(string)
					d.Description, _ = m["description"].(string)
				}
				dispatches = append(dispatches, d)
			}
		}
		if msg.SubAgentResult != nil {
			for _, block := range msg.Content {
				if block.Type == "tool_result" {
					resultByToolID[block.ToolID] = msg.SubAgentResult
				}
			}
		}
	}

	out := make(map[string]sidechainGroup, len(groups))
	for i, d := range dispatches {
		if i < len(groups) {
			g := groups[i]
			g.AgentType = d.AgentType
			g.Description = d.Description
			g.Result = resultByToolID[d.ToolID]
			out[d.ToolID] = g
		}
	}
	return out
}

func renderInlineSidechain(b *strings.Builder, g sidechainGroup, showThinking, showTools bool) {
	label := "Agent"
	if g.AgentType != "" {
		label = g.AgentType
	}
	if g.Description != "" {
		label += ": " + g.Description
	}

	var meta strings.Builder
	if r := g.Result; r != nil {
		if r.TotalTokens > 0 {
			fmt.Fprintf(&meta, "%s tokens", formatTokens(r.TotalTokens))
		}
		if r.TotalToolUseCount > 0 {
			if meta.Len() > 0 {
				meta.WriteString(", ")
			}
			fmt.Fprintf(&meta, "%d tools", r.TotalToolUseCount)
		}
		if r.TotalDurationMs > 0 {
			if meta.Len() > 0 {
				meta.WriteString(", ")
			}
			fmt.Fprintf(&meta, "%s", formatDuration(float64(r.TotalDurationMs)/1000))
		}
		if r.ToolStats != nil && r.ToolStats.LinesAdded > 0 {
			if meta.Len() > 0 {
				meta.WriteString(", ")
			}
			fmt.Fprintf(&meta, "+%d lines", r.ToolStats.LinesAdded)
		}
	}
	if meta.Len() == 0 {
		fmt.Fprintf(&meta, "%d messages", len(g.Messages))
	}

	b.WriteString(fmt.Sprintf(`<details class="sidechain-group" id="sidechain-%s">`, html.EscapeString(g.AgentID)))
	b.WriteString(fmt.Sprintf(`<summary class="sidechain-header"><span class="sidechain-icon">◆</span> %s <span class="sidechain-meta">%s</span></summary>`,
		html.EscapeString(label), html.EscapeString(meta.String())))
	b.WriteString(`<div class="sidechain-body">`)

	toolResults := buildToolResultsMap(g.Messages)
	for _, msg := range g.Messages {
		if msg.Kind == parser.KindToolResult {
			continue
		}
		renderTurnMessage(b, msg, showThinking, showTools, 0, toolResults)
	}

	b.WriteString(`</div></details>`)
}

// renderThread renders a conversation thread anchored by a USER message
func renderThread(b *strings.Builder, thread []*parser.Message, showThinking, showTools bool, toolResults map[string]parser.ContentBlock, scMap map[string]sidechainGroup) {
	if len(thread) == 0 {
		return
	}

	anchor := thread[0]
	responses := thread[1:]

	b.WriteString(`<div class="thread">`)

	b.WriteString(`<div class="thread-anchor">`)
	renderTurnMessage(b, anchor, showThinking, showTools, 0, toolResults)
	b.WriteString(`</div>`)

	if len(responses) > 0 {
		b.WriteString(`<div class="thread-responses">`)
		for _, msg := range responses {
			level := 1
			if msg.IsSidechain {
				level = 2
			}
			renderTurnMessage(b, msg, showThinking, showTools, level, toolResults)
			for _, block := range msg.Content {
				if block.Type == "tool_use" && subagentToolSet[block.ToolName] {
					if sc, ok := scMap[block.ToolID]; ok {
						renderInlineSidechain(b, sc, showThinking, showTools)
					}
				}
			}
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
}

func renderTurnMessage(b *strings.Builder, msg *parser.Message, showThinking, showTools bool, level int, toolResults map[string]parser.ContentBlock) {
	// Level class for indentation
	levelClass := ""
	if level > 0 {
		levelClass = fmt.Sprintf(" level-%d", level)
	}

	// Handle different message kinds with proper styling
	switch msg.Kind {
	case parser.KindToolResult:
		// Tool results are now rendered inline with tool_use - skip standalone
		return

	case parser.KindCompactSummary:
		// Compacted context: collapsible summary
		b.WriteString(fmt.Sprintf(`<details class="turn turn-compacted%s">`, levelClass))
		b.WriteString(`<summary class="turn-header"><span class="turn-icon">◇</span> Context Compacted</summary>`)
		b.WriteString(`<div class="turn-body compacted-text">`)
		for _, block := range msg.Content {
			if block.Type == "text" {
				b.WriteString(`<pre class="compact-content">`)
				b.WriteString(html.EscapeString(block.Text))
				b.WriteString(`</pre>`)
			}
		}
		b.WriteString(`</div></details>`)
		return

	case parser.KindMeta:
		// Meta/system instructions: collapsible
		b.WriteString(fmt.Sprintf(`<details class="turn turn-meta%s">`, levelClass))
		b.WriteString(`<summary class="turn-header"><span class="turn-icon">▽</span> System Instructions</summary>`)
		b.WriteString(`<div class="turn-body">`)
		for _, block := range msg.Content {
			renderBlock(b, block, showThinking, showTools, toolResults)
		}
		b.WriteString(`</div></details>`)
		return

	case parser.KindCommand:
		// Command message: show command name and args
		cmdName := msg.CommandName
		if cmdName == "" {
			cmdName = "/command"
		}
		b.WriteString(fmt.Sprintf(`<div class="turn turn-command%s" id="msg-%s">`, levelClass, sanitizeID(msg.UUID)))
		b.WriteString(`<div class="turn-header">`)
		b.WriteString(`<span class="turn-icon">⌘</span>`)
		b.WriteString(fmt.Sprintf(`<span class="turn-role">%s</span>`, html.EscapeString(cmdName)))
		b.WriteString(fmt.Sprintf(`<span class="turn-time">%s</span>`, msg.Timestamp.Format("15:04:05")))
		b.WriteString(`</div>`)
		// Show command args if present
		if msg.CommandArgs != "" {
			b.WriteString(`<div class="turn-body command-args">`)
			b.WriteString(renderMarkdown(msg.CommandArgs))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		return
	}

	// Standard message types: USER, ASSISTANT, AGENT
	isAgent := msg.IsSidechain

	turnClass := "turn" + levelClass
	icon := "●"
	role := "ASSISTANT"

	switch msg.Kind {
	case parser.KindUserPrompt:
		turnClass += " turn-user"
		icon = "▶"
		role = "USER"
	case parser.KindAssistant:
		turnClass += " turn-assistant"
	default:
		turnClass += " turn-unknown"
	}

	if isAgent {
		turnClass += " turn-agent"
		icon = "◆"
		role = "AGENT"
	}

	// Store raw content for copy/raw toggle
	rawContent := getRawContentJSON(msg)

	// USER blocks are collapsible
	if msg.Kind == parser.KindUserPrompt {
		preview := getFirstTextPreview(msg, 60)
		b.WriteString(fmt.Sprintf(`<details class="%s" id="msg-%s" open>`, turnClass, sanitizeID(msg.UUID)))
		b.WriteString(`<summary class="turn-header">`)
		b.WriteString(fmt.Sprintf(`<span class="turn-icon">%s</span>`, icon))
		b.WriteString(fmt.Sprintf(`<span class="turn-role">%s</span>`, role))
		b.WriteString(fmt.Sprintf(`<span class="turn-preview">%s</span>`, html.EscapeString(preview)))
		b.WriteString(fmt.Sprintf(`<span class="turn-time">%s</span>`, msg.Timestamp.Format("15:04:05")))
		b.WriteString(`<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(event,this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(event,this)">copy</button></span>`)
		b.WriteString(`</summary>`)
		b.WriteString(fmt.Sprintf(`<div class="turn-body" data-raw="%s">`, html.EscapeString(rawContent)))
		for _, block := range msg.Content {
			renderBlock(b, block, showThinking, showTools, toolResults)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</details>`)
		return
	}

	b.WriteString(fmt.Sprintf(`<div class="%s" id="msg-%s">`, turnClass, sanitizeID(msg.UUID)))

	b.WriteString(`<div class="turn-header">`)
	b.WriteString(fmt.Sprintf(`<span class="turn-icon">%s</span>`, icon))
	b.WriteString(fmt.Sprintf(`<span class="turn-role">%s</span>`, role))
	b.WriteString(fmt.Sprintf(`<span class="turn-time">%s</span>`, msg.Timestamp.Format("15:04:05")))
	if msg.Model != "" {
		b.WriteString(fmt.Sprintf(`<span class="turn-model">%s</span>`, html.EscapeString(msg.Model)))
	}
	b.WriteString(`<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(event,this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(event,this)">copy</button></span>`)
	b.WriteString(`</div>`)

	b.WriteString(fmt.Sprintf(`<div class="turn-body" data-raw="%s">`, html.EscapeString(rawContent)))
	for _, block := range msg.Content {
		renderBlock(b, block, showThinking, showTools, toolResults)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
}

func renderBlock(b *strings.Builder, block parser.ContentBlock, showThinking, showTools bool, toolResults map[string]parser.ContentBlock) {
	switch block.Type {
	case "text":
		if block.Text != "" {
			b.WriteString(`<div class="block-text">`)
			b.WriteString(renderMarkdown(block.Text))
			b.WriteString(`</div>`)
		}

	case "thinking":
		openAttr := ""
		if showThinking {
			openAttr = " open"
		}
		b.WriteString(fmt.Sprintf(`<details class="block-thinking"%s>`, openAttr))
		b.WriteString(`<summary><span class="block-icon">∴</span> Thinking...</summary>`)
		b.WriteString(`<div class="block-content">`)
		b.WriteString(html.EscapeString(block.Text))
		b.WriteString(`</div></details>`)

	case "tool_use":
		// Smart defaults: active tools expanded, passive tools collapsed
		openAttr := ""
		if isActiveTool(block.ToolName) || showTools {
			openAttr = " open"
		}
		// Compact preview for common tools
		preview := compactToolPreview(block.ToolName, block.ToolInput)
		b.WriteString(fmt.Sprintf(`<details class="block-tool" id="tool-%s" data-tool-id="%s"%s>`, sanitizeID(block.ToolID), html.EscapeString(block.ToolID), openAttr))
		b.WriteString(fmt.Sprintf(`<summary><span class="block-icon">●</span> %s<span class="tool-preview">%s</span><span class="tool-actions"><button class="raw-toggle">raw</button><button class="copy-btn">copy</button></span></summary>`,
			html.EscapeString(block.ToolName), html.EscapeString(preview)))

		// Tool input section
		if block.ToolInput != nil {
			b.WriteString(`<div class="tool-section tool-input-section">`)
			b.WriteString(`<div class="section-label">input</div>`)
			renderToolInput(b, block.ToolName, block.ToolInput)
			b.WriteString(`</div>`)
		}

		// Tool output section (inline from toolResults map)
		if result, ok := toolResults[block.ToolID]; ok {
			resultClass := "tool-section tool-output-section"
			if result.IsError {
				resultClass += " tool-error"
			}
			b.WriteString(fmt.Sprintf(`<div class="%s">`, resultClass))
			b.WriteString(`<div class="section-label">output</div>`)
			if result.ToolResult != nil {
				resultStr := fmt.Sprintf("%v", result.ToolResult)
				if len(resultStr) > 2000 {
					// Long output: collapsible instead of truncated
					preview := resultStr[:200]
					if idx := strings.LastIndex(preview, "\n"); idx > 50 {
						preview = preview[:idx]
					}
					b.WriteString(`<details class="long-output">`)
					b.WriteString(fmt.Sprintf(`<summary><pre class="output-preview">%s...</pre><span class="expand-hint">(%d chars, click to expand)</span></summary>`, html.EscapeString(preview), len(resultStr)))
					b.WriteString(fmt.Sprintf(`<pre class="output-full">%s</pre>`, html.EscapeString(resultStr)))
					b.WriteString(`</details>`)
				} else {
					b.WriteString(fmt.Sprintf(`<pre>%s</pre>`, html.EscapeString(resultStr)))
				}
			}
			b.WriteString(`</div>`)
		}

		b.WriteString(`</details>`)

	case "tool_result":
		// Tool results are now rendered inline with tool_use - skip standalone
		return

	case "image":
		if block.ImageData != "" {
			b.WriteString(fmt.Sprintf(`<img src="data:%s;base64,%s" class="block-image">`,
				html.EscapeString(block.MediaType), html.EscapeString(block.ImageData)))
		}
	}
}

// compactToolPreview returns a short preview for the tool call
func compactToolPreview(toolName string, input any) string {
	m, ok := input.(map[string]any)
	if !ok {
		return ""
	}

	switch toolName {
	case "Read":
		if fp, ok := m["file_path"].(string); ok {
			return fp
		}
	case "Write":
		if fp, ok := m["file_path"].(string); ok {
			return fp
		}
	case "Edit":
		if fp, ok := m["file_path"].(string); ok {
			return fp
		}
	case "Grep":
		if p, ok := m["pattern"].(string); ok {
			return "/" + p + "/"
		}
	case "Glob":
		if p, ok := m["pattern"].(string); ok {
			return p
		}
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			if len(cmd) > 50 {
				return "$ " + cmd[:50] + "..."
			}
			return "$ " + cmd
		}
	case "Task":
		var parts []string
		if agent, ok := m["subagent_type"].(string); ok && agent != "" {
			parts = append(parts, "["+agent+"]")
		}
		if desc, ok := m["description"].(string); ok {
			parts = append(parts, desc)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	case "Skill":
		if skill, ok := m["skill"].(string); ok {
			return "/" + skill
		}
	case "WebSearch":
		if q, ok := m["query"].(string); ok {
			if len(q) > 50 {
				return q[:50] + "..."
			}
			return q
		}
	case "WebFetch":
		if url, ok := m["url"].(string); ok {
			return url
		}
	case "AskUserQuestion":
		if questions, ok := m["questions"].([]any); ok && len(questions) > 0 {
			if q, ok := questions[0].(map[string]any); ok {
				if header, ok := q["header"].(string); ok {
					return header
				}
			}
		}
	case "LSP":
		if op, ok := m["operation"].(string); ok {
			if fp, ok := m["filePath"].(string); ok {
				// Just show filename, not full path
				parts := strings.Split(fp, "/")
				return op + " " + parts[len(parts)-1]
			}
			return op
		}
	case "TaskOutput":
		if id, ok := m["task_id"].(string); ok {
			return id
		}
	case "KillShell":
		if id, ok := m["shell_id"].(string); ok {
			return id
		}
	}

	// Fallback: show first key=value (sorted for determinism)
	if len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		k := keys[0]
		return fmt.Sprintf("%s=%v", k, m[k])
	}
	return ""
}

// isActiveTool returns true for tools that modify state (should be expanded by default)
func isActiveTool(name string) bool {
	active := map[string]bool{
		"Write": true, "Edit": true, "Bash": true, "Task": true,
		"TodoWrite": true, "Skill": true, "NotebookEdit": true,
		"KillShell": true, "AskUserQuestion": true,
	}
	return active[name]
}

// renderToolInput renders tool input in a formatted way
func renderToolInput(b *strings.Builder, toolName string, input any) {
	// ApplyPatch handles a RAW STRING input (the patch body itself)
	// before the map-type check, because unlike every other tool its
	// input isn't a JSON object — it's the patch text directly.
	if toolName == "ApplyPatch" || toolName == "apply_patch" {
		if s, ok := input.(string); ok && s != "" {
			b.WriteString(`<pre class="tool-input apply-patch">`)
			b.WriteString(html.EscapeString(s))
			b.WriteString(`</pre>`)
			return
		}
		if m, ok := input.(map[string]any); ok {
			if changes, ok := m["changes"]; ok {
				inputJSON, _ := json.MarshalIndent(changes, "", "  ")
				b.WriteString(`<pre class="tool-input apply-patch">`)
				b.WriteString(html.EscapeString(string(inputJSON)))
				b.WriteString(`</pre>`)
				return
			}
		}
	}

	m, ok := input.(map[string]any)
	if !ok {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		b.WriteString(fmt.Sprintf(`<pre class="tool-input">%s</pre>`, html.EscapeString(string(inputJSON))))
		return
	}

	switch toolName {
	case "Bash":
		// Two supported shapes: Claude Code uses {command, description,
		// timeout}; Codex's exec_command (normalized → Bash) uses
		// {cmd, workdir, max_output_tokens}. Pick whichever is present.
		b.WriteString(`<div class="bash-call">`)
		cmd := ""
		if v, ok := m["command"].(string); ok && v != "" {
			cmd = v
		} else if v, ok := m["cmd"].(string); ok && v != "" {
			cmd = v
		} else if argv, ok := m["argv"].([]any); ok {
			parts := make([]string, 0, len(argv))
			for _, a := range argv {
				if s, ok := a.(string); ok {
					parts = append(parts, s)
				}
			}
			cmd = strings.Join(parts, " ")
		}
		if cmd != "" {
			b.WriteString(`<pre class="bash-cmd">$ `)
			b.WriteString(html.EscapeString(cmd))
			b.WriteString(`</pre>`)
		}
		if wd, ok := m["workdir"].(string); ok && wd != "" {
			b.WriteString(fmt.Sprintf(`<div class="bash-meta">cwd: <code>%s</code></div>`, html.EscapeString(wd)))
		} else if wd, ok := m["cwd"].(string); ok && wd != "" {
			b.WriteString(fmt.Sprintf(`<div class="bash-meta">cwd: <code>%s</code></div>`, html.EscapeString(wd)))
		}
		if desc, ok := m["description"].(string); ok && desc != "" {
			b.WriteString(fmt.Sprintf(`<div class="bash-desc">%s</div>`, html.EscapeString(desc)))
		}
		b.WriteString(`</div>`)
		return

	case "UpdatePlan", "update_plan":
		// Codex's plan tool. Input shape:
		//   {explanation: "...", plan: [{step: "...", status: "pending|in_progress|completed"}]}
		b.WriteString(`<div class="update-plan">`)
		if exp, ok := m["explanation"].(string); ok && exp != "" {
			b.WriteString(fmt.Sprintf(`<div class="plan-exp">%s</div>`, html.EscapeString(exp)))
		}
		if plan, ok := m["plan"].([]any); ok {
			b.WriteString(`<ul class="plan-steps">`)
			for _, item := range plan {
				pm, ok := item.(map[string]any)
				if !ok {
					continue
				}
				step, _ := pm["step"].(string)
				status, _ := pm["status"].(string)
				icon := "○"
				cls := "plan-pending"
				if status == "completed" {
					icon = "✓"
					cls = "plan-done"
				} else if status == "in_progress" {
					icon = "◐"
					cls = "plan-active"
				}
				b.WriteString(fmt.Sprintf(`<li class="%s"><span class="plan-icon">%s</span><span class="plan-text">%s</span></li>`,
					cls, icon, html.EscapeString(step)))
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</div>`)
		return

	case "WriteStdin", "write_stdin":
		b.WriteString(`<div class="stdin-call">`)
		if sid, ok := m["session_id"]; ok {
			b.WriteString(fmt.Sprintf(`<span class="stdin-sid">session %v</span>`, sid))
		}
		if chars, ok := m["chars"].(string); ok {
			if chars == "" {
				b.WriteString(`<span class="stdin-empty">(empty — poll)</span>`)
			} else {
				b.WriteString(fmt.Sprintf(`<pre class="stdin-chars">%s</pre>`, html.EscapeString(chars)))
			}
		}
		b.WriteString(`</div>`)
		return

	case "Edit":
		// Show diff-style view for Edit tool
		b.WriteString(`<div class="edit-diff">`)
		if fp, ok := m["file_path"].(string); ok {
			b.WriteString(fmt.Sprintf(`<div class="diff-file">%s</div>`, html.EscapeString(fp)))
		}
		if old, ok := m["old_string"].(string); ok {
			b.WriteString(`<pre class="diff-old">`)
			b.WriteString(html.EscapeString(old))
			b.WriteString(`</pre>`)
		}
		if newStr, ok := m["new_string"].(string); ok {
			b.WriteString(`<pre class="diff-new">`)
			b.WriteString(html.EscapeString(newStr))
			b.WriteString(`</pre>`)
		}
		b.WriteString(`</div>`)
		return

	case "Write":
		// Show file path and full content (collapsible if long)
		b.WriteString(`<div class="write-content">`)
		if fp, ok := m["file_path"].(string); ok {
			b.WriteString(fmt.Sprintf(`<div class="diff-file">%s</div>`, html.EscapeString(fp)))
		}
		if content, ok := m["content"].(string); ok {
			if len(content) > 2000 {
				// Collapsible for long content
				preview := content[:200]
				if idx := strings.LastIndex(preview, "\n"); idx > 50 {
					preview = preview[:idx]
				}
				b.WriteString(`<details class="long-output">`)
				b.WriteString(fmt.Sprintf(`<summary><pre class="output-preview">%s...</pre><span class="expand-hint">(%d chars, click to expand)</span></summary>`, html.EscapeString(preview), len(content)))
				b.WriteString(fmt.Sprintf(`<pre class="diff-new">%s</pre>`, html.EscapeString(content)))
				b.WriteString(`</details>`)
			} else {
				b.WriteString(`<pre class="diff-new">`)
				b.WriteString(html.EscapeString(content))
				b.WriteString(`</pre>`)
			}
		}
		b.WriteString(`</div>`)
		return

	case "TodoWrite":
		// Render todos as a checklist
		if todos, ok := m["todos"].([]any); ok {
			b.WriteString(`<ul class="todo-checklist">`)
			for _, item := range todos {
				if todo, ok := item.(map[string]any); ok {
					content, _ := todo["content"].(string)
					status, _ := todo["status"].(string)
					checked := ""
					statusClass := "todo-pending"
					icon := "○"
					if status == "completed" {
						checked = " checked disabled"
						statusClass = "todo-completed"
						icon = "✓"
					} else if status == "in_progress" {
						statusClass = "todo-progress"
						icon = "◐"
					}
					b.WriteString(fmt.Sprintf(`<li class="%s"><span class="todo-icon">%s</span><input type="checkbox"%s><span class="todo-text">%s</span></li>`,
						statusClass, icon, checked, html.EscapeString(content)))
				}
			}
			b.WriteString(`</ul>`)
			return
		}

	case "Task":
		// Show agent type, model, and prompt
		b.WriteString(`<div class="task-call">`)
		if agent, ok := m["subagent_type"].(string); ok && agent != "" {
			b.WriteString(fmt.Sprintf(`<span class="task-agent">[%s]</span>`, html.EscapeString(agent)))
		}
		if model, ok := m["model"].(string); ok && model != "" {
			b.WriteString(fmt.Sprintf(`<span class="task-model">%s</span>`, html.EscapeString(model)))
		}
		if prompt, ok := m["prompt"].(string); ok {
			b.WriteString(`<div class="task-prompt">`)
			b.WriteString(renderMarkdown(prompt))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		return

	case "Skill":
		// Show skill name and args
		b.WriteString(`<div class="skill-call">`)
		if skill, ok := m["skill"].(string); ok {
			b.WriteString(fmt.Sprintf(`<span class="skill-name">/%s</span>`, html.EscapeString(skill)))
		}
		if args, ok := m["args"].(string); ok && args != "" {
			b.WriteString(fmt.Sprintf(`<span class="skill-args">%s</span>`, html.EscapeString(args)))
		}
		b.WriteString(`</div>`)
		return

	case "WebSearch":
		b.WriteString(`<div class="websearch-call">`)
		if q, ok := m["query"].(string); ok {
			b.WriteString(fmt.Sprintf(`<span class="search-query">🔍 %s</span>`, html.EscapeString(q)))
		}
		b.WriteString(`</div>`)
		return

	case "WebFetch":
		b.WriteString(`<div class="webfetch-call">`)
		if url, ok := m["url"].(string); ok {
			escaped := html.EscapeString(url)
			if isSafeURL(url) {
				b.WriteString(fmt.Sprintf(`<a href="%s" class="fetch-url" target="_blank" rel="noopener noreferrer">%s</a>`, escaped, escaped))
			} else {
				b.WriteString(fmt.Sprintf(`<span class="fetch-url">%s</span>`, escaped))
			}
		}
		if prompt, ok := m["prompt"].(string); ok && prompt != "" {
			b.WriteString(fmt.Sprintf(`<div class="fetch-prompt">%s</div>`, html.EscapeString(prompt)))
		}
		b.WriteString(`</div>`)
		return

	case "AskUserQuestion":
		if questions, ok := m["questions"].([]any); ok {
			b.WriteString(`<div class="ask-questions">`)
			for _, item := range questions {
				if q, ok := item.(map[string]any); ok {
					header, _ := q["header"].(string)
					question, _ := q["question"].(string)
					b.WriteString(`<div class="ask-question">`)
					if header != "" {
						b.WriteString(fmt.Sprintf(`<span class="ask-header">%s</span>`, html.EscapeString(header)))
					}
					b.WriteString(fmt.Sprintf(`<div class="ask-text">%s</div>`, html.EscapeString(question)))
					if options, ok := q["options"].([]any); ok {
						b.WriteString(`<ul class="ask-options">`)
						for _, opt := range options {
							if o, ok := opt.(map[string]any); ok {
								label, _ := o["label"].(string)
								desc, _ := o["description"].(string)
								b.WriteString(fmt.Sprintf(`<li><strong>%s</strong>`, html.EscapeString(label)))
								if desc != "" {
									b.WriteString(fmt.Sprintf(` - %s`, html.EscapeString(desc)))
								}
								b.WriteString(`</li>`)
							}
						}
						b.WriteString(`</ul>`)
					}
					b.WriteString(`</div>`)
				}
			}
			b.WriteString(`</div>`)
			return
		}

	case "LSP":
		b.WriteString(`<div class="lsp-call">`)
		if op, ok := m["operation"].(string); ok {
			b.WriteString(fmt.Sprintf(`<span class="lsp-op">%s</span>`, html.EscapeString(op)))
		}
		if fp, ok := m["filePath"].(string); ok {
			line, _ := m["line"].(float64)
			char, _ := m["character"].(float64)
			b.WriteString(fmt.Sprintf(`<span class="lsp-loc">%s:%d:%d</span>`, html.EscapeString(fp), int(line), int(char)))
		}
		b.WriteString(`</div>`)
		return

	case "TaskOutput":
		b.WriteString(`<div class="taskoutput-call">`)
		if id, ok := m["task_id"].(string); ok {
			b.WriteString(fmt.Sprintf(`<span class="task-id">%s</span>`, html.EscapeString(id)))
		}
		if block, ok := m["block"].(bool); ok {
			mode := "async"
			if block {
				mode = "blocking"
			}
			b.WriteString(fmt.Sprintf(`<span class="task-mode">%s</span>`, mode))
		}
		b.WriteString(`</div>`)
		return

	case "KillShell":
		b.WriteString(`<div class="killshell-call">`)
		if id, ok := m["shell_id"].(string); ok {
			b.WriteString(fmt.Sprintf(`<span class="shell-id">⊗ %s</span>`, html.EscapeString(id)))
		}
		b.WriteString(`</div>`)
		return
	}

	// Default: show as JSON
	inputJSON, _ := json.MarshalIndent(input, "", "  ")
	b.WriteString(fmt.Sprintf(`<pre class="tool-input">%s</pre>`, html.EscapeString(string(inputJSON))))
}

func renderMarkdown(text string) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	inCodeBlock := false
	codeBlockLang := ""
	var codeLines []string
	inTable := false
	var tableRows []string

	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				b.WriteString(fmt.Sprintf(`<pre class="code-block"><code class="lang-%s">%s</code></pre>`,
					html.EscapeString(codeBlockLang), html.EscapeString(strings.Join(codeLines, "\n"))))
				codeLines = nil
				inCodeBlock = false
			} else {
				inCodeBlock = true
				codeBlockLang = strings.TrimPrefix(line, "```")
				if codeBlockLang == "" {
					codeBlockLang = "text"
				}
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		// Table detection: line starts with | and contains |
		isTableLine := strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, "|")
		isSeparatorLine := isTableLine && strings.Contains(line, "---")

		if isTableLine {
			if !inTable {
				inTable = true
				tableRows = nil
			}
			if !isSeparatorLine {
				tableRows = append(tableRows, line)
			}
			// Check if next line is not a table line
			if i+1 >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "|") {
				// End of table, render it
				b.WriteString(renderTable(tableRows))
				inTable = false
				tableRows = nil
			}
			continue
		}

		if strings.TrimSpace(line) == "" {
			b.WriteString(`<br>`)
			continue
		}

		// Heading detection: 1-6 leading '#' followed by a space and text.
		// Matches CommonMark ATX headings. Anything else falls through to
		// the paragraph renderer below.
		if level, headingText, ok := parseATXHeading(line); ok {
			escaped := html.EscapeString(headingText)
			escaped = processInlineCode(escaped)
			escaped = processBold(escaped)
			b.WriteString(fmt.Sprintf(`<div class="md-h%d">%s</div>`, level, escaped))
			continue
		}

		// List detection: "- item" / "* item" / "N. item". Render as an
		// unnumbered line (the bullet is kept literal) rather than a flat
		// paragraph so the structure is visually preserved in the feed.
		trimmedLine := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmedLine, "- ") || strings.HasPrefix(trimmedLine, "* ") {
			text := trimmedLine[2:]
			escaped := html.EscapeString(text)
			escaped = processInlineCode(escaped)
			escaped = processBold(escaped)
			b.WriteString(`<div class="md-li">• ` + escaped + `</div>`)
			continue
		}

		// Process inline formatting
		escaped := html.EscapeString(line)
		escaped = processInlineCode(escaped)
		escaped = processBold(escaped)

		b.WriteString(`<p>` + escaped + `</p>`)
	}

	if inCodeBlock {
		b.WriteString(fmt.Sprintf(`<pre class="code-block"><code class="lang-%s">%s</code></pre>`,
			html.EscapeString(codeBlockLang), html.EscapeString(strings.Join(codeLines, "\n"))))
	}

	return b.String()
}

// renderTable converts markdown table rows to HTML table
func renderTable(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<table class="md-table">`)

	for i, row := range rows {
		cells := parseTableRow(row)
		if i == 0 {
			b.WriteString(`<thead><tr>`)
			for _, cell := range cells {
				escaped := html.EscapeString(strings.TrimSpace(cell))
				escaped = processInlineCode(escaped)
				escaped = processBold(escaped)
				b.WriteString(`<th>` + escaped + `</th>`)
			}
			b.WriteString(`</tr></thead><tbody>`)
		} else {
			b.WriteString(`<tr>`)
			for _, cell := range cells {
				escaped := html.EscapeString(strings.TrimSpace(cell))
				escaped = processInlineCode(escaped)
				escaped = processBold(escaped)
				b.WriteString(`<td>` + escaped + `</td>`)
			}
			b.WriteString(`</tr>`)
		}
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

// parseTableRow splits a markdown table row into cells
func parseTableRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.Trim(row, "|")
	return strings.Split(row, "|")
}

// parseATXHeading recognizes a CommonMark-style ATX heading ("# title"
// through "###### title"). Returns the heading level (1-6), the inner
// text with heading syntax stripped, and ok=true on match.
//
// Rejected: more than 6 leading #s, no space after the #s, empty inner
// text (we prefer to render those as plain paragraphs so the rail has
// nothing but real headings to latch onto), and any leading whitespace
// (CommonMark allows up to 3 spaces but ccx's pipeline trims lines
// before reaching here).
func parseATXHeading(line string) (int, string, bool) {
	i := 0
	for i < 6 && i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i >= len(line) {
		return 0, "", false
	}
	// Must be followed by a space or tab (CommonMark requirement)
	if line[i] != ' ' && line[i] != '\t' {
		return 0, "", false
	}
	text := strings.TrimSpace(line[i+1:])
	// Strip trailing #s per CommonMark (e.g. "## heading ##")
	text = strings.TrimRight(text, " #")
	if text == "" {
		return 0, "", false
	}
	return i, text, true
}

// processInlineCode converts `code` to <code>code</code>
func processInlineCode(s string) string {
	var result strings.Builder
	inCode := false
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			if inCode {
				result.WriteString("</code>")
			} else {
				result.WriteString("<code>")
			}
			inCode = !inCode
		} else {
			result.WriteByte(s[i])
		}
	}
	// Close unclosed code tag
	if inCode {
		result.WriteString("</code>")
	}
	return result.String()
}

// processBold converts **text** to <strong>text</strong>
func processBold(s string) string {
	var result strings.Builder
	inBold := false
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '*' && s[i+1] == '*' {
			if inBold {
				result.WriteString("</strong>")
			} else {
				result.WriteString("<strong>")
			}
			inBold = !inBold
			i++ // skip second *
		} else {
			result.WriteByte(s[i])
		}
	}
	// Close unclosed bold tag
	if inBold {
		result.WriteString("</strong>")
	}
	return result.String()
}

// getFirstLine returns first line of text (no truncation)
func getFirstLine(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, "\n"); idx > 0 {
		return text[:idx]
	}
	return text
}

// getFirstTextPreview returns first line of text content (no truncation)
func getFirstTextPreview(msg *parser.Message, _ int) string {
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			return getFirstLine(block.Text)
		}
	}
	return ""
}

func getRawContentJSON(msg *parser.Message) string {
	if msg.RawJSON != "" {
		return msg.RawJSON
	}
	data, _ := json.Marshal(msg.Content)
	return string(data)
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2")
	}
}

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return "-"
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s > 0 {
			return fmt.Sprintf("%dm %ds", m, s)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Show end of path (most relevant part)
	return "..." + path[len(path)-maxLen+3:]
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		k := float64(n) / 1000
		if k >= 999.95 {
			return fmt.Sprintf("%.1fM", float64(n)/1000000)
		}
		return fmt.Sprintf("%.1fk", k)
	}
	return fmt.Sprintf("%d", n)
}

// formatCost returns a USD string with adaptive precision:
//
//	< $0.01      -> "<$0.01"
//	< $1         -> "$0.0123"
//	>= $1        -> "$1.23"
//	>= $100      -> "$123"
//	>= $10,000   -> "$12.3k"
//
// No rounding games, no locale. Cost is the user's whole reason for
// looking — show what's billed.
func formatCost(usd float64) string {
	if usd <= 0 {
		return "$0.00"
	}
	if usd < 0.01 {
		return "<$0.01"
	}
	if usd < 1 {
		return fmt.Sprintf("$%.4f", usd)
	}
	if usd < 100 {
		return fmt.Sprintf("$%.2f", usd)
	}
	if usd < 10_000 {
		return fmt.Sprintf("$%.0f", usd)
	}
	return fmt.Sprintf("$%.1fk", usd/1000)
}

// hasBillableUsage reports whether any turn in the slice has non-zero
// tokens. Used to decide whether to render the spend section at all.
func hasBillableUsage(turns []*parser.Exchange) bool {
	for _, t := range turns {
		if t != nil && t.TotalTokens() > 0 {
			return true
		}
	}
	return false
}

// renderSpendSection renders the per-turn breakdown inside the info panel.
// Turns are sorted by cost desc (most expensive first) so quota hogs
// surface immediately. Rows link to #msg-<anchor> which composes with
// the load-earlier hash-nav fix shipped in #3.
func renderSpendSection(turns []*parser.Exchange, sessionTotal float64) string {
	var b strings.Builder

	// Sort by cost desc (stable — ties break by original turn index).
	// We don't sort the underlying slice; work on a copy so callers get
	// chronological order back.
	sorted := make([]*parser.Exchange, len(turns))
	copy(sorted, turns)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].CostUSD != sorted[j].CostUSD {
			return sorted[i].CostUSD > sorted[j].CostUSD
		}
		return sorted[i].Index < sorted[j].Index
	})

	b.WriteString(`<div class="info-section info-section-spend">`)
	b.WriteString(`<div class="info-section-header">Per-turn spend</div>`)

	priced := 0
	for _, t := range sorted {
		if t.CostUSD > 0 {
			priced++
		}
	}
	if priced == 0 {
		// Tokens present but no pricing matched any model (unknown model).
		// Still useful to show token-only breakdown; label the limitation.
		b.WriteString(`<div class="spend-note">No pricing match for model — showing token totals.</div>`)
	}

	b.WriteString(`<div class="spend-list">`)
	for _, t := range sorted {
		// Skip turns with literally zero activity (no tokens, no cost)
		if t.TotalTokens() == 0 && t.CostUSD == 0 {
			continue
		}
		snippet := t.Snippet
		if len(snippet) > 42 {
			snippet = snippet[:39] + "..."
		}
		label := fmt.Sprintf("%d. %s", t.Index, snippet)
		tokensLabel := formatTokens(t.TotalTokens())

		b.WriteString(fmt.Sprintf(
			`<a class="spend-row" href="#msg-%s" title="Jump to turn %d — %s tokens, %s"><span class="spend-label">%s</span><span class="spend-cost">%s</span></a>`,
			html.EscapeString(sanitizeID(t.AnchorID)),
			t.Index,
			tokensLabel,
			formatCost(t.CostUSD),
			html.EscapeString(label),
			formatCost(t.CostUSD),
		))
	}
	b.WriteString(`</div>`)

	// Footer: total cost summary + known-models link
	if sessionTotal > 0 {
		b.WriteString(fmt.Sprintf(
			`<div class="spend-footer">Session total: <strong>%s</strong></div>`,
			formatCost(sessionTotal),
		))
	}

	b.WriteString(`</div>`)
	return b.String()
}

// renderConversationNav emits the outline sidebar.
//
// Each Exchange group is a <div class="nav-group"> with two distinct
// clickable targets:
//
//	<a  class="nav-title"  href="#msg-…">prompt preview … 14:03</a>
//	<button class="nav-expand" aria-expanded="…">▶</button>
//
// Clicking the title jumps to the message. Clicking the expand button
// (and only the expand button) toggles the group's children. There is
// no <details>/<summary> overlap between "toggle" and "jump": one
// element, one action.
//
// The last three exchanges default to expanded; earlier ones collapse.
// Group children are rendered in a <div class="nav-children"> that the
// JS can hide/show via the aria-expanded attribute on the group root.
func renderConversationNav(b *strings.Builder, messages []*parser.Message) {
	flat := flattenMessages(messages)
	allMsgs := filterMainConversation(flat)
	scGroups := groupSidechainsByAgent(flat)
	scMap := matchSidechainsToToolUse(allMsgs, scGroups)

	type navGroup struct {
		user     *parser.Message
		children []*parser.Message
	}
	var groups []navGroup
	var currentGroup *navGroup

	for _, msg := range allMsgs {
		switch msg.Kind {
		case parser.KindUserPrompt, parser.KindCommand, parser.KindCompactSummary:
			if currentGroup != nil {
				groups = append(groups, *currentGroup)
			}
			currentGroup = &navGroup{user: msg}
		default:
			if currentGroup != nil {
				currentGroup.children = append(currentGroup.children, msg)
			}
		}
	}
	if currentGroup != nil {
		groups = append(groups, *currentGroup)
	}

	for i, g := range groups {
		isLast := i >= len(groups)-3 // default-expand last 3 exchanges
		uuid := sanitizeID(g.user.UUID)
		timeAttr := ""
		if !g.user.Timestamp.IsZero() {
			timeAttr = g.user.Timestamp.Local().Format("15:04")
		}

		switch g.user.Kind {
		case parser.KindCompactSummary:
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-compact" data-msg="%s">`,
				uuid, html.EscapeString(uuid)))
			b.WriteString(`<span class="nav-icon" aria-hidden="true">◇</span><span class="nav-text">COMPACT</span>`)
			if timeAttr != "" {
				b.WriteString(fmt.Sprintf(`<span class="nav-time">%s</span>`, html.EscapeString(timeAttr)))
			}
			b.WriteString(`</a>`)

		case parser.KindCommand:
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-command" data-msg="%s">`,
				uuid, html.EscapeString(uuid)))
			b.WriteString(fmt.Sprintf(`<span class="nav-icon" aria-hidden="true">⌘</span><span class="nav-text">%s</span>`,
				html.EscapeString(g.user.CommandName)))
			if timeAttr != "" {
				b.WriteString(fmt.Sprintf(`<span class="nav-time">%s</span>`, html.EscapeString(timeAttr)))
			}
			b.WriteString(`</a>`)

		case parser.KindUserPrompt:
			preview := getNavPreview(g.user)
			childCount := len(g.children)

			if childCount == 0 {
				b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-user" data-msg="%s">`,
					uuid, html.EscapeString(uuid)))
				b.WriteString(fmt.Sprintf(`<span class="nav-icon" aria-hidden="true">▶</span><span class="nav-text">%s</span>`,
					html.EscapeString(preview)))
				if timeAttr != "" {
					b.WriteString(fmt.Sprintf(`<span class="nav-time">%s</span>`, html.EscapeString(timeAttr)))
				}
				b.WriteString(`</a>`)
				continue
			}

			expandedAttr := "false"
			if isLast {
				expandedAttr = "true"
			}
			b.WriteString(fmt.Sprintf(`<div class="nav-group" data-expanded="%s">`, expandedAttr))
			b.WriteString(`<div class="nav-row">`)
			b.WriteString(fmt.Sprintf(`<button type="button" class="nav-expand" aria-expanded="%s" aria-label="Toggle exchange" data-target="%s"></button>`,
				expandedAttr, html.EscapeString(uuid)))
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-title nav-user" data-msg="%s">`,
				uuid, html.EscapeString(uuid)))
			b.WriteString(fmt.Sprintf(`<span class="nav-text">%s</span>`, html.EscapeString(preview)))
			b.WriteString(fmt.Sprintf(`<span class="nav-count">%d</span>`, childCount))
			if timeAttr != "" {
				b.WriteString(fmt.Sprintf(`<span class="nav-time">%s</span>`, html.EscapeString(timeAttr)))
			}
			b.WriteString(`</a>`)
			b.WriteString(`</div>`) // .nav-row

			b.WriteString(`<div class="nav-children">`)
			const maxChildren = 10
			total := len(g.children)
			if total <= maxChildren {
				for _, child := range g.children {
					renderNavChild(b, child)
					renderNavSidechainEntries(b, child, scMap)
				}
			} else {
				head := maxChildren - 1
				for _, child := range g.children[:head] {
					renderNavChild(b, child)
					renderNavSidechainEntries(b, child, scMap)
				}
				hidden := total - head - 1
				if hidden > 0 {
					b.WriteString(fmt.Sprintf(`<span class="nav-more">+%d more</span>`, hidden))
				}
				renderNavChild(b, g.children[total-1])
				renderNavSidechainEntries(b, g.children[total-1], scMap)
			}
			b.WriteString(`</div>`) // .nav-children
			b.WriteString(`</div>`) // .nav-group
		}
	}

}

func renderNavSidechainEntries(b *strings.Builder, msg *parser.Message, scMap map[string]sidechainGroup) {
	if msg == nil || len(scMap) == 0 {
		return
	}
	for _, block := range msg.Content {
		if block.Type != "tool_use" || !subagentToolSet[block.ToolName] {
			continue
		}
		g, ok := scMap[block.ToolID]
		if !ok {
			continue
		}
		label := g.AgentType
		if label == "" {
			label = "Agent"
		}
		if g.Description != "" {
			label += ": " + g.Description
		}
		if len(label) > 35 {
			label = label[:32] + "..."
		}
		b.WriteString(fmt.Sprintf(`<a href="#sidechain-%s" class="nav-item nav-sidechain" data-msg="sidechain-%s">`,
			html.EscapeString(g.AgentID), html.EscapeString(g.AgentID)))
		b.WriteString(fmt.Sprintf(`<span class="nav-icon" aria-hidden="true">◆</span><span class="nav-text">%s</span>`,
			html.EscapeString(label)))
		b.WriteString(fmt.Sprintf(`<span class="nav-count">%d</span>`, len(g.Messages)))
		b.WriteString(`</a>`)
	}
}

func getNavPreview(msg *parser.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			line := getFirstLine(block.Text)
			if len(line) > 40 {
				return line[:40] + "..."
			}
			return line
		}
	}
	return "(empty)"
}

func renderNavChild(b *strings.Builder, msg *parser.Message) {
	switch msg.Kind {
	case parser.KindAssistant:
		hasTool := false
		toolName := ""
		toolPreview := ""
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				hasTool = true
				toolName = block.ToolName
				toolPreview = compactToolPreview(block.ToolName, block.ToolInput)
				break
			}
		}
		if hasTool {
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-tool" data-msg="%s" title="%s">`,
				sanitizeID(msg.UUID), html.EscapeString(sanitizeID(msg.UUID)), html.EscapeString(toolPreview)))
			navText := toolName
			if toolPreview != "" && len(toolPreview) < 30 {
				navText = fmt.Sprintf("%s(%s)", toolName, toolPreview)
			}
			b.WriteString(fmt.Sprintf(`<span class="nav-icon">●</span><span class="nav-text">%s</span></a>`,
				html.EscapeString(navText)))
		} else {
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-response" data-msg="%s">`,
				sanitizeID(msg.UUID), html.EscapeString(sanitizeID(msg.UUID))))
			b.WriteString(`<span class="nav-icon">○</span><span class="nav-text">response</span></a>`)
		}
	case parser.KindMeta:
		b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-meta" data-msg="%s">`,
			sanitizeID(msg.UUID), html.EscapeString(sanitizeID(msg.UUID))))
		b.WriteString(`<span class="nav-icon">▽</span><span class="nav-text">system</span></a>`)
	}
}

func renderSearchPage(projectsDir, query string) string {
	var b strings.Builder

	b.WriteString(pageHeader("Search - ccx", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("search"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header">`)
	b.WriteString(`<h1>Global Search</h1>`)
	b.WriteString(`<p class="stats">Search across all projects and sessions</p>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="controls">`)
	b.WriteString(`<div class="search-wrap" style="max-width:600px">`)
	b.WriteString(fmt.Sprintf(`<input type="text" id="global-search" class="search-input" placeholder="Search projects, sessions, summaries..." value="%s" autofocus>`, html.EscapeString(query)))
	b.WriteString(`<span class="search-spinner" id="search-spinner"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div id="search-results" class="search-results"></div>`)

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(searchJS(query))
	b.WriteString(pageFooter())

	return b.String()
}

func searchJS(initialQuery string) string {
	return fmt.Sprintf(`
<script>
const searchInput = document.getElementById('global-search');
const spinner = document.getElementById('search-spinner');
const resultsDiv = document.getElementById('search-results');
let searchTimeout;

async function doSearch(query) {
  if (!query) {
    resultsDiv.innerHTML = '<p class="search-hint">Type to search across all projects and sessions...<br><span style="font-size:11px;color:var(--text-muted)">Prefix with <code>cc:</code> or <code>cx:</code> to filter by provider</span></p>';
    return;
  }
  spinner.classList.add('loading');
  try {
    const resp = await fetch('/api/search?q=' + encodeURIComponent(query));
    const results = await resp.json();
    renderResults(results);
  } catch (e) {
    resultsDiv.innerHTML = '<p class="search-error">Search failed</p>';
  }
  spinner.classList.remove('loading');
}

function providerBadge(p) {
  if (p === 'claude-code') return '<span class="provider-badge provider-CC">CC</span>';
  if (p === 'codex') return '<span class="provider-badge provider-CX">CX</span>';
  return '';
}

function renderResults(results) {
  if (results.length === 0) {
    resultsDiv.innerHTML = '<p class="search-empty">No results found</p>';
    return;
  }
  let html = '<div class="search-list">';
  for (const r of results) {
    const badge = r.type === 'project' ? '<span class="result-badge badge-project">P</span>' :
                  r.type === 'session' ? '<span class="result-badge badge-session">S</span>' :
                  '<span class="result-badge badge-message">M</span>';
    const pb = r.provider ? ' ' + providerBadge(r.provider) : '';
    html += '<a href="' + escapeHtml(r.url) + '" class="search-result">';
    html += badge;
    html += '<div class="result-body">';
    html += '<div class="result-title">' + escapeHtml(r.summary || 'Untitled') + pb + '</div>';
    html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' &middot; ' + escapeHtml(r.time) : '') + '</div>';
    if (r.snippet) {
      html += '<div class="result-snippet">' + escapeHtml(r.snippet) + '</div>';
    }
    html += '</div></a>';
  }
  html += '</div>';
  resultsDiv.innerHTML = html;
}

function escapeHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

searchInput.addEventListener('input', function(e) {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => doSearch(e.target.value), 300);
});

const themeToggle = document.getElementById('theme-toggle');
if (themeToggle) {
  themeToggle.addEventListener('click', function() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    html.setAttribute('data-theme', current === 'dark' ? 'light' : 'dark');
    localStorage.setItem('ccx-theme', html.getAttribute('data-theme'));
  });
  const saved = localStorage.getItem('ccx-theme');
  if (saved) document.documentElement.setAttribute('data-theme', saved);
}

const backTop = document.getElementById('back-to-top');
if (backTop) {
  window.addEventListener('scroll', function() {
    backTop.classList.toggle('show', window.scrollY > 300);
  });
  backTop.addEventListener('click', function() {
    window.scrollTo({ top: 0, behavior: 'smooth' });
  });
}

if (%q) doSearch(%q);
</script>
<style>
.search-results { margin-top: 20px; }
.search-hint, .search-empty, .search-error { color: var(--text-muted); font-size: 13px; }
.search-error { color: var(--error-border); }
.search-list { display: flex; flex-direction: column; gap: 8px; }
.search-list .search-result {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  text-decoration: none;
  color: inherit;
  position: static;
}
.search-list .search-result:hover { border-color: var(--primary); background: var(--bg-tertiary); }
</style>
`, initialQuery, initialQuery)
}

func renderSettingsPage(settings *Settings, config *GlobalConfig, configFiles []ConfigFileInfo, agents []AgentInfo, skills []SkillInfo) string {
	var b strings.Builder

	b.WriteString(pageHeader("Settings - ccx", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("settings"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<h1>Settings</h1>`)

	// ccx Provider Status
	ccxSettings := ccxconfig.Load()
	b.WriteString(`<section class="settings-section">`)
	b.WriteString(`<h2><span class="section-icon">◉</span> Providers</h2>`)
	b.WriteString(`<div class="provider-status-list">`)
	type providerInfo struct {
		id, home string
		sessions int
	}
	providers := []providerInfo{
		{"claude-code", ccxSettings.ClaudeHome, 0},
		{"codex", ccxSettings.CodexHome, 0},
	}
	allProjects, _ := sessionProvider.DiscoverProjects()
	for _, p := range allProjects {
		for _, s := range p.Sessions {
			for i := range providers {
				if s.Provider == providers[i].id {
					providers[i].sessions++
				}
			}
		}
	}
	for _, prov := range providers {
		pc := ccxSettings.Providers[prov.id]
		accentColor := ccxSettings.ProviderAccent(prov.id, "dark")
		_, statErr := os.Stat(prov.home)
		status := "active"
		if statErr != nil {
			status = "missing"
		}
		if !pc.Enabled {
			status = "disabled"
		}
		b.WriteString(fmt.Sprintf(`<div class="provider-status-card" style="border-left: 3px solid %s">`, accentColor))
		b.WriteString(fmt.Sprintf(`<div class="prov-header"><strong>%s</strong>`, html.EscapeString(pc.DisplayName)))
		b.WriteString(fmt.Sprintf(`<span class="prov-badge prov-%s">%s</span></div>`, status, status))
		b.WriteString(fmt.Sprintf(`<div class="prov-detail"><code>%s</code></div>`, html.EscapeString(prov.home)))
		if status == "active" {
			b.WriteString(fmt.Sprintf(`<div class="prov-detail">%d sessions</div>`, prov.sessions))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`</section>`)

	// ccx Configuration
	b.WriteString(`<section class="settings-section">`)
	b.WriteString(`<h2><span class="section-icon">●</span> ccx Configuration</h2>`)
	b.WriteString(`<table class="settings-table">`)
	b.WriteString(fmt.Sprintf(`<tr><td>theme</td><td><code>%s</code></td></tr>`, html.EscapeString(ccxSettings.Theme)))
	b.WriteString(fmt.Sprintf(`<tr><td>show_thinking</td><td><code>%s</code></td></tr>`, html.EscapeString(ccxSettings.ShowThinking)))
	b.WriteString(fmt.Sprintf(`<tr><td>default_format</td><td><code>%s</code></td></tr>`, html.EscapeString(ccxSettings.DefaultFormat)))
	b.WriteString(fmt.Sprintf(`<tr><td>port</td><td><code>%d</code></td></tr>`, ccxSettings.Port))
	b.WriteString(fmt.Sprintf(`<tr><td>host</td><td><code>%s</code></td></tr>`, html.EscapeString(ccxSettings.Host)))
	b.WriteString(fmt.Sprintf(`<tr><td>syntax_highlight</td><td><code>%v</code></td></tr>`, ccxSettings.SyntaxHighlight))
	b.WriteString(fmt.Sprintf(`<tr><td>code_theme</td><td><code>%s</code></td></tr>`, html.EscapeString(ccxSettings.CodeTheme)))
	b.WriteString(`</table>`)
	b.WriteString(`</section>`)

	// Claude Code global config
	if config != nil {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(`<h2><span class="section-icon">●</span> Claude Code Global</h2>`)
		b.WriteString(`<table class="settings-table">`)
		b.WriteString(fmt.Sprintf(`<tr><td>Theme</td><td><code>%s</code></td></tr>`, html.EscapeString(config.Theme)))
		b.WriteString(fmt.Sprintf(`<tr><td>Editor Mode</td><td><code>%s</code></td></tr>`, html.EscapeString(config.EditorMode)))
		b.WriteString(fmt.Sprintf(`<tr><td>Verbose</td><td><code>%v</code></td></tr>`, config.Verbose))
		b.WriteString(fmt.Sprintf(`<tr><td>Total Startups</td><td><code>%d</code></td></tr>`, config.NumStartups))
		b.WriteString(`</table>`)
		b.WriteString(`</section>`)
	}

	if len(configFiles) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">▣</span> Config Files <span class="count">(%d)</span></h2>`, len(configFiles)))
		b.WriteString(`<div class="file-card-list">`)
		for i, file := range configFiles {
			b.WriteString(fmt.Sprintf(`<details class="file-card config-card" data-path="%s" data-idx="%d">`, html.EscapeString(file.FilePath), i))
			b.WriteString(fmt.Sprintf(`<summary><code>%s</code><span class="file-path">%s</span><span class="expand-icon">▶</span></summary>`, html.EscapeString(file.Name), html.EscapeString(file.FilePath)))
			b.WriteString(`<div class="file-viewer" id="config-` + fmt.Sprint(i) + `">`)
			b.WriteString(`<div class="file-toolbar"><button class="mode-btn" data-mode="fmt">fmt</button><button class="mode-btn active" data-mode="raw">raw</button><button class="copy-btn">copy</button></div>`)
			b.WriteString(`<div class="file-content"><div class="loading">Loading...</div></div>`)
			b.WriteString(`</div></details>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</section>`)
	}

	// Permissions
	if settings != nil {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(`<h2><span class="section-icon">◐</span> Permissions</h2>`)
		b.WriteString(`<table class="settings-table">`)
		for k, v := range settings.Permissions {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><code>%s</code></td></tr>`, html.EscapeString(k), html.EscapeString(v)))
		}
		b.WriteString(`</table>`)
		b.WriteString(`</section>`)

		if len(settings.EnabledPlugins) > 0 {
			b.WriteString(`<section class="settings-section">`)
			b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◎</span> Enabled Plugins <span class="count">(%d)</span></h2>`, len(settings.EnabledPlugins)))
			b.WriteString(`<ul class="plugin-list">`)
			for plugin, enabled := range settings.EnabledPlugins {
				if enabled {
					b.WriteString(fmt.Sprintf(`<li><code>%s</code></li>`, html.EscapeString(plugin)))
				}
			}
			b.WriteString(`</ul>`)
			b.WriteString(`</section>`)
		}

		if len(settings.Env) > 0 {
			b.WriteString(`<section class="settings-section">`)
			b.WriteString(`<h2><span class="section-icon">◇</span> Environment</h2>`)
			b.WriteString(`<table class="settings-table">`)
			for k, v := range settings.Env {
				b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><code>%s</code></td></tr>`, html.EscapeString(k), html.EscapeString(v)))
			}
			b.WriteString(`</table>`)
			b.WriteString(`</section>`)
		}
	}

	// Agents - expandable with file content viewer
	if len(agents) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◆</span> Agents <span class="count">(%d)</span></h2>`, len(agents)))
		b.WriteString(`<div class="file-card-list">`)
		for i, agent := range agents {
			b.WriteString(fmt.Sprintf(`<details class="file-card agent-card" data-path="%s" data-idx="%d">`, html.EscapeString(agent.FilePath), i))
			b.WriteString(fmt.Sprintf(`<summary><code>%s</code><span class="file-path">%s</span><span class="expand-icon">▶</span></summary>`, html.EscapeString(agent.Name), html.EscapeString(agent.FilePath)))
			b.WriteString(`<div class="file-viewer" id="agent-` + fmt.Sprint(i) + `">`)
			b.WriteString(`<div class="file-toolbar"><button class="mode-btn" data-mode="fmt">fmt</button><button class="mode-btn active" data-mode="raw">raw</button><button class="copy-btn">copy</button></div>`)
			b.WriteString(`<div class="file-content"><div class="loading">Loading...</div></div>`)
			b.WriteString(`</div></details>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</section>`)
	}

	// Skills - expandable with file content viewer
	if len(skills) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◈</span> Skills <span class="count">(%d)</span></h2>`, len(skills)))
		b.WriteString(`<div class="file-card-list">`)
		for i, skill := range skills {
			// For skills, show skill.md inside the directory
			skillFile := skill.Path + "/skill.md"
			b.WriteString(fmt.Sprintf(`<details class="file-card skill-card" data-path="%s" data-idx="%d">`, html.EscapeString(skillFile), i))
			b.WriteString(fmt.Sprintf(`<summary><code>%s</code><span class="file-path">%s</span><span class="expand-icon">▶</span></summary>`, html.EscapeString(skill.Name), html.EscapeString(skill.Path)))
			b.WriteString(`<div class="file-viewer" id="skill-` + fmt.Sprint(i) + `">`)
			b.WriteString(`<div class="file-toolbar"><button class="mode-btn" data-mode="fmt">fmt</button><button class="mode-btn active" data-mode="raw">raw</button><button class="copy-btn">copy</button></div>`)
			b.WriteString(`<div class="file-content"><div class="loading">Loading...</div></div>`)
			b.WriteString(`</div></details>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</section>`)
	}

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(settingsPageCSS())
	b.WriteString(pageFooter())

	return b.String()
}

func settingsPageCSS() string {
	return `<style>
.provider-status-list { display: flex; gap: 12px; flex-wrap: wrap; }
.provider-status-card {
  background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius);
  padding: 12px 16px; min-width: 220px; flex: 1;
}
.prov-header { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.prov-badge { font-size: 11px; padding: 1px 6px; border-radius: 3px; }
.prov-active { background: #16a34a22; color: #16a34a; }
.prov-missing { background: #eab30822; color: #eab308; }
.prov-disabled { background: #64748b22; color: #64748b; }
.prov-detail { font-size: 13px; color: var(--text-muted); }
.prov-detail code { font-size: 12px; }
.count { color: var(--text-muted); font-weight: normal; font-size: 12px; }
.file-card-list { display: flex; flex-direction: column; gap: 8px; }
.file-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.file-card summary {
  padding: 10px 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 12px;
}
.file-card summary code { font-size: 13px; font-weight: 600; }
.file-card .file-path { flex: 1; font-size: 11px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-card summary:hover { background: var(--bg-tertiary); }
.expand-icon { font-size: 10px; color: var(--text-muted); transition: transform 0.2s; flex-shrink: 0; }
.file-card[open] .expand-icon { transform: rotate(90deg); }
.file-viewer { border-top: 1px solid var(--border); }
.file-toolbar {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
  background: var(--bg-tertiary);
  border-bottom: 1px solid var(--border);
}
.file-toolbar .mode-btn, .file-toolbar .copy-btn {
  padding: 4px 10px;
  font-size: 11px;
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--bg);
  color: var(--text-muted);
  cursor: pointer;
}
.file-toolbar .mode-btn:hover, .file-toolbar .copy-btn:hover { background: var(--bg-secondary); color: var(--text); }
.file-toolbar .mode-btn.active { background: var(--primary); color: white; border-color: var(--primary); }
.file-toolbar .copy-btn { margin-left: auto; }
.file-content { padding: 16px; max-height: 600px; overflow: auto; scrollbar-width: thin; }
.file-content .loading { color: var(--text-muted); font-style: italic; font-size: 13px; }
.file-content .source-raw {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', 'Consolas', var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text);
  tab-size: 2;
}
.file-content .fmt {
  font-size: 14px;
  line-height: 1.7;
  color: var(--text);
}
.file-content .fmt h1 { font-size: 1.4em; margin: 20px 0 12px; padding-bottom: 6px; border-bottom: 1px solid var(--border); }
.file-content .fmt h2 { font-size: 1.2em; margin: 18px 0 10px; }
.file-content .fmt h3 { font-size: 1.1em; margin: 14px 0 8px; color: var(--text-muted); }
.file-content .fmt h4 { font-size: 1em; margin: 12px 0 6px; font-weight: 600; }
.file-content .fmt p { margin: 10px 0; }
.file-content .fmt ul { margin: 8px 0; padding-left: 24px; }
.file-content .fmt li { margin: 4px 0; }
.file-content .fmt code {
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'JetBrains Mono', 'Fira Code', var(--font-mono);
  font-size: 0.9em;
}
.file-content .fmt .code-block {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 12px 16px;
  margin: 12px 0;
  overflow-x: auto;
}
.file-content .fmt .code-block code {
  background: none;
  padding: 0;
  font-size: 12px;
  line-height: 1.5;
  display: block;
  white-space: pre;
}
.file-content .fmt a { color: var(--primary); text-decoration: none; }
.file-content .fmt a:hover { text-decoration: underline; }
.file-content .fmt strong { font-weight: 600; }
.agent-card { border-left: 3px solid #86c; }
.skill-card { border-left: 3px solid var(--primary); }
</style>
<script>
document.querySelectorAll('.file-card').forEach(card => {
  card.addEventListener('toggle', async function() {
    if (!this.open) return;
    const viewer = this.querySelector('.file-viewer');
    const content = viewer.querySelector('.file-content');
    if (content.dataset.loaded) return;

    const path = this.dataset.path;
    try {
      const resp = await fetch('/api/file?path=' + encodeURIComponent(path));
      if (!resp.ok) throw new Error('Failed to load');
      const data = await resp.json();
      content.dataset.raw = data.content;
      content.dataset.loaded = '1';
      showRaw(content, data.content); // Default to raw view
    } catch (e) {
      content.innerHTML = '<div class="error">Failed to load file</div>';
    }
  });
});

function showFormatted(el, raw) {
  el.innerHTML = '<div class="fmt">' + renderMarkdownFull(raw) + '</div>';
}
function showRaw(el, raw) {
  el.innerHTML = '<pre class="source-raw">' + escapeHtmlSettings(raw) + '</pre>';
}
function escapeHtmlSettings(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}
function sanitizeLang(lang) {
  return lang ? lang.replace(/[^a-zA-Z0-9_-]/g, '') : 'text';
}
function renderMarkdownFull(s) {
  const BT = '` + "`" + `';
  // Extract code blocks first
  const codeBlocks = [];
  s = s.replace(new RegExp(BT+BT+BT+'(\\w*)\\n([\\s\\S]*?)'+BT+BT+BT, 'g'), (m, lang, code) => {
    codeBlocks.push('<pre class="code-block"><code class="lang-'+sanitizeLang(lang)+'">' + escapeHtmlSettings(code) + '</code></pre>');
    return '%%CODE' + (codeBlocks.length-1) + '%%';
  });
  // Escape first, then apply formatting
  s = escapeHtmlSettings(s)
    .replace(/^#### (.+)$/gm, '<h4>$1</h4>')
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    .replace(/^\- (.+)$/gm, '<li>$1</li>')
    .replace(/^\* (.+)$/gm, '<li>$1</li>')
    .replace(/(<li>.*<\/li>\n?)+/g, '<ul>$&</ul>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(new RegExp(BT+'([^'+BT+']+)'+BT, 'g'), '<code>$1</code>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(m, text, url) {
      if (/^https?:\/\//i.test(url)) {
        return '<a href="' + url + '" target="_blank" rel="noopener noreferrer">' + text + '</a>';
      }
      if (/^mailto:/i.test(url)) {
        return '<a href="' + url + '">' + text + '</a>';
      }
      return text + ' (' + url + ')';
    })
    .replace(/\n\n+/g, '</p><p>')
    .replace(/\n/g, '<br>');
  // Restore code blocks
  codeBlocks.forEach((block, i) => {
    s = s.replace('%%CODE'+i+'%%', block);
  });
  return '<p>' + s + '</p>';
}

document.querySelectorAll('.file-toolbar .mode-btn').forEach(btn => {
  btn.addEventListener('click', function() {
    const viewer = this.closest('.file-viewer');
    const content = viewer.querySelector('.file-content');
    viewer.querySelectorAll('.mode-btn').forEach(b => b.classList.remove('active'));
    this.classList.add('active');
    const raw = content.dataset.raw || '';
    if (this.dataset.mode === 'raw') showRaw(content, raw);
    else showFormatted(content, raw);
  });
});

document.querySelectorAll('.file-toolbar .copy-btn').forEach(btn => {
  btn.addEventListener('click', function() {
    const viewer = this.closest('.file-viewer');
    const content = viewer.querySelector('.file-content');
    const activeMode = viewer.querySelector('.mode-btn.active')?.dataset.mode;
    let text;
    if (activeMode === 'raw') {
      text = content.dataset.raw || '';
    } else {
      text = content.innerText || content.textContent || '';
    }
    navigator.clipboard.writeText(text);
    this.textContent = 'copied!';
    setTimeout(() => this.textContent = 'copy', 1500);
  });
});
</script>`
}

func memSectionCSS() string {
	return `<style>
.mem-section {
  margin-bottom: 16px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-secondary);
  border-left: 3px solid #eab308;
}
.mem-section-header {
  padding: 10px 14px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  user-select: none;
}
.mem-section-header:hover { background: var(--bg-tertiary); border-radius: var(--radius); }
.mem-icon { color: #eab308; font-size: 12px; }
.mem-badge {
  background: #eab30818; color: #eab308;
  font-size: 11px; font-weight: 600;
  padding: 1px 7px; border-radius: 10px;
  margin-left: auto;
}
.mem-section-body { padding: 0 10px 10px; }
.mem-file {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  margin-bottom: 4px;
}
.mem-file:last-child { margin-bottom: 0; }
.mem-file-row {
  padding: 6px 10px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.mem-file-row:hover { background: var(--bg-tertiary); }
.mem-file-name { font-size: 12px; font-weight: 600; color: var(--text); }
.mem-file-path {
  flex: 1; font-size: 11px; color: var(--text-muted);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  text-align: right;
}
.mem-file .expand-icon { font-size: 9px; color: var(--text-muted); transition: transform 0.15s; flex-shrink: 0; }
.mem-file[open] .expand-icon { transform: rotate(90deg); }
.mem-file-cc { border-left: 2px solid #da7756; }
.mem-file-cx { border-left: 2px solid #3b82f6; }
.mem-file .file-viewer { border-top: 1px solid var(--border); }
.mem-file .file-toolbar {
  display: flex; gap: 4px; padding: 6px 10px;
  background: var(--bg-tertiary); border-bottom: 1px solid var(--border);
}
.mem-file .file-toolbar .mode-btn,
.mem-file .file-toolbar .copy-btn {
  padding: 2px 8px; font-size: 11px;
  border: 1px solid var(--border); border-radius: 3px;
  background: var(--bg); color: var(--text-muted); cursor: pointer;
}
.mem-file .file-toolbar .mode-btn:hover,
.mem-file .file-toolbar .copy-btn:hover { background: var(--bg-secondary); color: var(--text); }
.mem-file .file-toolbar .mode-btn.active { background: var(--primary); color: white; border-color: var(--primary); }
.mem-file .file-toolbar .copy-btn { margin-left: auto; }
.mem-file .file-content {
  padding: 12px; max-height: 400px; overflow: auto; scrollbar-width: thin;
}
.mem-file .file-content .loading { color: var(--text-muted); font-style: italic; font-size: 12px; }
.mem-file .file-content .source-raw {
  margin: 0; white-space: pre-wrap; word-break: break-word;
  font-family: var(--font-mono); font-size: 12px; line-height: 1.6; color: var(--text);
}
.mem-file .file-content .fmt { font-size: 13px; line-height: 1.6; }
.mem-file .file-content .fmt h1 { font-size: 1.2em; margin: 12px 0 6px; }
.mem-file .file-content .fmt h2 { font-size: 1.1em; margin: 10px 0 4px; }
.mem-file .file-content .fmt h3 { font-size: 1.05em; margin: 8px 0 4px; }
.mem-file .file-content .fmt p { margin: 4px 0; }
.mem-file .file-content .fmt code { background: var(--bg-tertiary); padding: 1px 4px; border-radius: 3px; font-size: 0.9em; }
.mem-file .file-content .fmt pre.code-block { background: var(--bg-tertiary); padding: 8px; border-radius: 4px; overflow-x: auto; margin: 6px 0; }
.mem-file .file-content .fmt ul { padding-left: 20px; margin: 4px 0; }
.mem-file .file-content .fmt li { margin: 2px 0; }
</style>`
}

func fileCardJS() string {
	return `<script>
document.querySelectorAll('.mem-file, .file-card').forEach(card => {
  card.addEventListener('toggle', async function() {
    if (!this.open) return;
    const viewer = this.querySelector('.file-viewer');
    if (!viewer) return;
    const content = viewer.querySelector('.file-content');
    if (!content || content.dataset.loaded) return;
    const path = this.dataset.path;
    try {
      const resp = await fetch('/api/file?path=' + encodeURIComponent(path));
      if (!resp.ok) throw new Error('Failed to load');
      const data = await resp.json();
      content.dataset.raw = data.content;
      content.dataset.loaded = '1';
      content.innerHTML = '<pre class="source-raw">' + escapeHtmlMem(data.content) + '</pre>';
    } catch (e) {
      content.innerHTML = '<div style="color:var(--text-muted);font-style:italic">Failed to load file</div>';
    }
  });
});

function escapeHtmlMem(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
function showFmtMem(el, raw) {
  el.innerHTML = '<div class="fmt">' + renderMdMem(raw) + '</div>';
}
function showRawMem(el, raw) {
  el.innerHTML = '<pre class="source-raw">' + escapeHtmlMem(raw) + '</pre>';
}
function renderMdMem(s) {
  const BT = '` + "`" + `';
  const blocks = [];
  s = s.replace(new RegExp(BT+BT+BT+'(\\w*)\\n([\\s\\S]*?)'+BT+BT+BT,'g'), (m,lang,code) => {
    blocks.push('<pre class="code-block"><code>' + escapeHtmlMem(code) + '</code></pre>');
    return '%%CB'+blocks.length+'%%';
  });
  s = escapeHtmlMem(s)
    .replace(/^#### (.+)$/gm,'<h4>$1</h4>')
    .replace(/^### (.+)$/gm,'<h3>$1</h3>')
    .replace(/^## (.+)$/gm,'<h2>$1</h2>')
    .replace(/^# (.+)$/gm,'<h1>$1</h1>')
    .replace(/^\- (.+)$/gm,'<li>$1</li>')
    .replace(/^\* (.+)$/gm,'<li>$1</li>')
    .replace(/(<li>.*<\/li>\n?)+/g,'<ul>$&</ul>')
    .replace(/\*\*(.+?)\*\*/g,'<strong>$1</strong>')
    .replace(/\*(.+?)\*/g,'<em>$1</em>')
    .replace(new RegExp(BT+'([^'+BT+']+)'+BT,'g'),'<code>$1</code>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(m, text, url) {
      if (/^https?:\/\//i.test(url)) return '<a href="'+url+'" target="_blank" rel="noopener noreferrer">'+text+'</a>';
      return text+' ('+url+')';
    })
    .replace(/\n\n+/g,'</p><p>')
    .replace(/\n/g,'<br>');
  blocks.forEach((b,i) => { s = s.replace('%%CB'+(i+1)+'%%', b); });
  return '<p>' + s + '</p>';
}

document.querySelectorAll('.mem-file .file-toolbar .mode-btn, .file-card .file-toolbar .mode-btn').forEach(btn => {
  btn.addEventListener('click', function() {
    const viewer = this.closest('.file-viewer');
    const content = viewer.querySelector('.file-content');
    viewer.querySelectorAll('.mode-btn').forEach(b => b.classList.remove('active'));
    this.classList.add('active');
    const raw = content.dataset.raw || '';
    if (this.dataset.mode === 'raw') showRawMem(content, raw);
    else showFmtMem(content, raw);
  });
});

document.querySelectorAll('.mem-file .file-toolbar .copy-btn, .file-card .file-toolbar .copy-btn').forEach(btn => {
  btn.addEventListener('click', function() {
    const viewer = this.closest('.file-viewer');
    const content = viewer.querySelector('.file-content');
    const raw = content.dataset.raw || content.innerText || '';
    navigator.clipboard.writeText(raw);
    this.textContent = 'copied!';
    setTimeout(() => this.textContent = 'copy', 1500);
  });
});
</script>`
}

func renderMemoryPage(data *MemoryData) string {
	var b strings.Builder

	b.WriteString(pageHeader("Memory - ccx", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("memory"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(fmt.Sprintf(`<h1>Memory <span class="mem-count">(%d files)</span></h1>`, data.TotalFiles))

	idx := 0

	// Section 1: Global Instructions
	if len(data.Global) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◇</span> Global Instructions <span class="count">(%d)</span></h2>`, len(data.Global)))
		b.WriteString(`<div class="file-card-list">`)
		for _, f := range data.Global {
			renderMemoryFileCard(&b, f, idx)
			idx++
		}
		b.WriteString(`</div></section>`)
	}

	// Section 2: User Rules
	if len(data.Rules) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◆</span> User Rules <span class="count">(%d)</span></h2>`, len(data.Rules)))
		b.WriteString(`<div class="file-card-list">`)
		for _, f := range data.Rules {
			renderMemoryFileCard(&b, f, idx)
			idx++
		}
		b.WriteString(`</div></section>`)
	}

	// Section 3: Per-Project Memory
	if len(data.Projects) > 0 {
		totalProjectFiles := 0
		for _, p := range data.Projects {
			totalProjectFiles += len(p.Files)
		}
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◈</span> Project Memory <span class="count">(%d projects, %d files)</span></h2>`, len(data.Projects), totalProjectFiles))

		for _, proj := range data.Projects {
			badge := `<span class="prov-pill prov-pill-cc">CC</span>`
			if proj.Provider == "codex" {
				badge = `<span class="prov-pill prov-pill-cx">CX</span>`
			}
			b.WriteString(`<div class="mem-project-group">`)
			b.WriteString(fmt.Sprintf(`<div class="mem-project-header"><strong>%s</strong> %s <code class="mem-project-path">%s</code></div>`,
				html.EscapeString(proj.Name), badge, html.EscapeString(proj.Path)))
			b.WriteString(`<div class="file-card-list">`)
			for _, f := range proj.Files {
				renderMemoryFileCard(&b, f, idx)
				idx++
			}
			b.WriteString(`</div></div>`)
		}
		b.WriteString(`</section>`)
	}

	// Section 4: Codex Memories
	if len(data.CodexMem) > 0 {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(fmt.Sprintf(`<h2><span class="section-icon">◌</span> Codex Memories <span class="count">(%d)</span></h2>`, len(data.CodexMem)))
		b.WriteString(`<div class="file-card-list">`)
		for _, f := range data.CodexMem {
			renderMemoryFileCard(&b, f, idx)
			idx++
		}
		b.WriteString(`</div></section>`)
	}

	if data.TotalFiles == 0 {
		b.WriteString(`<div class="empty-state">No memory or instruction files found.</div>`)
	}

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(indexJS())
	b.WriteString(memoryPageCSS())
	b.WriteString(pageFooter())

	return b.String()
}

func renderMemoryFileCard(b *strings.Builder, f MemoryFile, idx int) {
	provClass := "mem-card-cc"
	if f.Provider == "codex" {
		provClass = "mem-card-cx"
	}
	b.WriteString(fmt.Sprintf(`<details class="file-card %s" data-path="%s" data-idx="%d">`, provClass, html.EscapeString(f.FilePath), idx))
	b.WriteString(fmt.Sprintf(`<summary><code>%s</code><span class="file-path">%s</span><span class="expand-icon">▶</span></summary>`,
		html.EscapeString(f.Name), html.EscapeString(f.FilePath)))
	b.WriteString(fmt.Sprintf(`<div class="file-viewer" id="mem-%d">`, idx))
	b.WriteString(`<div class="file-toolbar"><button class="mode-btn" data-mode="fmt">fmt</button><button class="mode-btn active" data-mode="raw">raw</button><button class="copy-btn">copy</button></div>`)
	b.WriteString(`<div class="file-content"><div class="loading">Loading...</div></div>`)
	b.WriteString(`</div></details>`)
}

func memoryPageCSS() string {
	return settingsPageCSS() + `<style>
.mem-count { color: var(--text-muted); font-weight: normal; font-size: 14px; }
.mem-project-group {
  background: var(--bg-secondary); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 12px; margin-bottom: 12px;
}
.mem-project-header {
  display: flex; align-items: center; gap: 8px;
  margin-bottom: 8px; padding-bottom: 8px; border-bottom: 1px solid var(--border);
}
.mem-project-path { font-size: 11px; color: var(--text-muted); margin-left: auto; }
.prov-pill { font-size: 10px; padding: 1px 6px; border-radius: 3px; font-weight: 600; }
.prov-pill-cc { background: #da775622; color: #da7756; }
.prov-pill-cx { background: #3b82f622; color: #3b82f6; }
.mem-card-cc { border-left: 3px solid #da7756; }
.mem-card-cx { border-left: 3px solid #3b82f6; }
.empty-state { color: var(--text-muted); font-style: italic; padding: 24px 0; }
</style>`
}

func renderTopNav(projectName, sessionID string) string {
	var b strings.Builder
	b.WriteString(`<header class="top-nav">`)
	b.WriteString(`<div class="top-nav-inner">`)
	b.WriteString(`<div class="nav-left">`)
	b.WriteString(`<a href="/" class="brand"><span class="brand-cc">cc</span><span class="brand-x">x</span></a>`)
	b.WriteString(`<span class="brand-sub">for agent sessions</span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="nav-center">`)
	b.WriteString(`<div class="global-search">`)
	b.WriteString(`<input type="text" id="global-search" class="global-search-input" placeholder="Search all... (press /)" autocomplete="off">`)
	b.WriteString(`<div id="search-results" class="search-results"></div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="nav-right">`)
	b.WriteString(`<a href="https://x.com/ericwang42" target="_blank" rel="noopener noreferrer" class="icon-btn" title="@ericwang42"><svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/></svg></a>`)
	b.WriteString(`<a href="https://github.com/thevibeworks/ccx" target="_blank" rel="noopener noreferrer" class="icon-btn" title="GitHub"><svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg></a>`)
	b.WriteString(`<button class="icon-btn" id="theme-toggle" title="Toggle theme (d)">◐</button>`)
	b.WriteString(`<a href="/settings" class="icon-btn" title="Settings">◎</a>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</header>`)
	return b.String()
}

func renderFooter() string {
	return `<footer class="site-footer">
	<div class="footer-inner">
		<a href="https://github.com/thevibeworks/ccx" target="_blank" rel="noopener noreferrer" class="footer-brand"><span class="brand-cc">cc</span><span class="brand-x">x</span></a>
		<span class="footer-sep">·</span>
		<a href="https://github.com/thevibeworks" target="_blank" rel="noopener noreferrer" class="footer-text">by thevibeworks</a>
	</div>
</footer>`
}

func renderSidebar(active string) string {
	var b strings.Builder

	b.WriteString(`<aside class="sidebar">`)
	b.WriteString(`<nav class="sidebar-nav">`)

	items := []struct {
		href, label, key string
	}{
		{"/", "Projects", "projects"},
		{"/search", "Search", "search"},
		{"/insights", "Insights", "insights"},
		{"/memory", "Memory", "memory"},
		{"/settings", "Settings", "settings"},
	}

	for _, item := range items {
		class := "sidebar-link"
		if item.key == active {
			class += " active"
		}
		b.WriteString(fmt.Sprintf(`<a href="%s" class="%s">%s</a>`, item.href, class, item.label))
	}

	b.WriteString(`</nav>`)
	b.WriteString(`</aside>`)

	return b.String()
}

func selected(current, value string) string {
	if current == value {
		return " selected"
	}
	return ""
}

func pageHeader(title, theme string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en" data-theme="%s">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<script>(function(){var t=localStorage.getItem('ccx-theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
<title>%s</title>
%s
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4"></script>
<style type="text/tailwindcss">
@theme {
  --color-ccx: #da7756;
  --color-ccx-dark: #c5634a;
}
@utility scrollbar-thin {
  scrollbar-width: thin;
}
</style>
<style>
%s
</style>
</head>
<body>
`, theme, html.EscapeString(title), faviconLink(), cssStyles())
}

// renderNotFoundPage emits a styled 404 page with a pre-filled
// "report this bug" link to the ccx GitHub issues page.
//
// kind is a one-word category ("session", "project", "page",
// "export") used in the headline. detail is an optional one-line
// explanation that helps the user understand what went wrong
// (e.g. "couldn't resolve session 019d8f43... in project foo").
//
// The bug report link captures the failing URL, user agent, and
// timestamp — everything a maintainer needs to reproduce without
// asking follow-up questions. The issue body is a Markdown template
// the user can edit before submitting.
func renderNotFoundPage(w http.ResponseWriter, r *http.Request, kind, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)

	failingURL := r.URL.String()
	userAgent := r.UserAgent()
	timestamp := time.Now().Format(time.RFC3339)

	// Pre-filled issue title + body. The user can (and should) edit
	// them before submitting; we just take the boring parts off
	// their plate.
	title := "404: " + kind + " not found"
	if detail != "" {
		title = "404: " + detail
	}
	bodyTemplate := "### What I tried to access\n\n" +
		"```\n" + failingURL + "\n```\n\n" +
		"### What I expected\n\n" +
		"_Replace this with what you expected to see._\n\n" +
		"### What actually happened\n\n" +
		"ccx web returned a 404 page.\n\n"
	if detail != "" {
		bodyTemplate += "Detail from the 404 page: `" + detail + "`\n\n"
	}
	bodyTemplate += "### Environment\n\n" +
		"- Timestamp: " + timestamp + "\n" +
		"- User-Agent: `" + userAgent + "`\n" +
		"- ccx version: _(paste output of `ccx --version` here)_\n"

	issueURL := "https://github.com/thevibeworks/ccx/issues/new?" +
		"labels=bug" +
		"&title=" + url.QueryEscape(title) +
		"&body=" + url.QueryEscape(bodyTemplate)

	var b strings.Builder
	b.WriteString(pageHeader("ccx — not found", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<main class="nf-main">`)
	b.WriteString(`<div class="nf-box">`)
	b.WriteString(`<div class="nf-glyph" aria-hidden="true">404</div>`)
	b.WriteString(`<h1 class="nf-title">Can't find this ` + html.EscapeString(kind) + `.</h1>`)
	if detail != "" {
		b.WriteString(`<p class="nf-detail">` + html.EscapeString(detail) + `</p>`)
	}
	b.WriteString(`<pre class="nf-url">` + html.EscapeString(failingURL) + `</pre>`)
	b.WriteString(`<p class="nf-note">`)
	b.WriteString(`This can happen if the session was deleted, if ccx's project index is stale, `)
	b.WriteString(`or — more interestingly — if we broke something. If you didn't expect this, `)
	b.WriteString(`please tell us:`)
	b.WriteString(`</p>`)
	b.WriteString(`<div class="nf-actions">`)
	b.WriteString(`<a class="nf-btn nf-primary" href="` + html.EscapeString(issueURL) + `" target="_blank" rel="noopener noreferrer">`)
	b.WriteString(`<svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" style="vertical-align:middle;margin-right:6px"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>`)
	b.WriteString(`Report this bug`)
	b.WriteString(`</a>`)
	b.WriteString(`<a class="nf-btn" href="/">← All projects</a>`)
	b.WriteString(`<a class="nf-btn" href="/search">Search sessions</a>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</main>`)
	b.WriteString(renderFooter())
	b.WriteString("</body></html>")

	fmt.Fprint(w, b.String())
}

func faviconLink() string {
	// Bold favicon: cc in white, x in coral
	return `<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='4' fill='%23111'/%3E%3Ctext x='3' y='23' font-family='ui-monospace,monospace' font-weight='800' font-size='14'%3E%3Ctspan fill='%23fff'%3Ecc%3C/tspan%3E%3Ctspan fill='%23da7756'%3Ex%3C/tspan%3E%3C/text%3E%3C/svg%3E">`
}

func pageFooter() string {
	return `
<div id="loading-overlay" class="loading-overlay">
  <div class="cli-spinner">
    <span class="cli-spinner-char"></span>
    <span id="spinner-verb">Loading</span>
  </div>
</div>
<script>
// Global loading overlay control
const loadingOverlay = document.getElementById('loading-overlay');
const spinnerVerbEl = document.getElementById('spinner-verb');

// ccx-flavored gerund list. Pure decoration — the label the spinner
// shows has no correlation with what ccx is actually doing. See
// internal/web/spinner_verbs.go for the Go-side source of truth.
const SPINNER_VERBS = ` + spinnerVerbsJSArray() + `;
let spinnerTimer = null;
function pickSpinnerVerb() {
  return SPINNER_VERBS[Math.floor(Math.random() * SPINNER_VERBS.length)] + '...';
}
function startSpinnerRotation() {
  if (spinnerVerbEl) spinnerVerbEl.textContent = pickSpinnerVerb();
  if (spinnerTimer) return;
  spinnerTimer = setInterval(() => {
    if (spinnerVerbEl) spinnerVerbEl.textContent = pickSpinnerVerb();
  }, 1400);
}
function stopSpinnerRotation() {
  if (spinnerTimer) { clearInterval(spinnerTimer); spinnerTimer = null; }
}

window.showLoading = function() {
  loadingOverlay?.classList.add('active');
  startSpinnerRotation();
};
window.hideLoading = function() {
  loadingOverlay?.classList.remove('active');
  stopSpinnerRotation();
};

// Show loading on navigation (skip downloads/API links)
document.querySelectorAll('a[href^="/"]').forEach(a => {
  a.addEventListener('click', function(e) {
    const href = this.getAttribute('href') || '';
    if (e.metaKey || e.ctrlKey || e.shiftKey) return;
    if (href.startsWith('/api/')) return;
    if (href.startsWith('#')) return;
    window.showLoading();
  });
});
window.addEventListener('pageshow', function() { window.hideLoading(); });
</script>
</body></html>`
}

func cssStyles() string {
	data, _ := staticFS.ReadFile("static/style.css")
	return string(data)
}

func indexJS() string {
	return `
<script>
let searchTimeout;
const searchInput = document.getElementById('search');
const spinner = document.getElementById('search-spinner');
const sortSelect = document.getElementById('sort');

if (searchInput) {
  searchInput.addEventListener('input', function(e) {
    clearTimeout(searchTimeout);
    spinner.classList.add('loading');
    searchTimeout = setTimeout(() => {
      const url = new URL(window.location);
      if (e.target.value) {
        url.searchParams.set('q', e.target.value);
      } else {
        url.searchParams.delete('q');
      }
      window.location = url;
    }, 400);
  });
}

if (sortSelect) {
  sortSelect.addEventListener('change', function(e) {
    const url = new URL(window.location);
    url.searchParams.set('sort', e.target.value);
    window.location = url;
  });
}

const providerFilter = document.getElementById('provider-filter');
if (providerFilter) {
  providerFilter.addEventListener('change', function(e) {
    const val = e.target.value;
    document.querySelectorAll('.session-card').forEach(function(card) {
      if (val === 'all') {
        card.style.display = '';
      } else {
        card.style.display = card.dataset.provider === val ? '' : 'none';
      }
    });
  });
}

document.addEventListener('keydown', function(e) {
  if (e.key === '/' && !e.target.matches('input, textarea')) {
    e.preventDefault();
    const globalSearch = document.getElementById('global-search');
    if (globalSearch) {
      globalSearch.focus();
    } else if (searchInput) {
      searchInput.focus();
    }
  }
  if (e.key === 'Escape') {
    document.getElementById('search-results')?.classList.remove('active');
    document.getElementById('global-search')?.blur();
  }
});

// Global search
const globalSearchInput = document.getElementById('global-search');
const searchResults = document.getElementById('search-results');
let globalSearchTimeout;

if (globalSearchInput && searchResults) {
  globalSearchInput.addEventListener('input', function(e) {
    clearTimeout(globalSearchTimeout);
    const query = e.target.value.trim();
    if (!query) {
      searchResults.classList.remove('active');
      return;
    }
    globalSearchTimeout = setTimeout(async () => {
      try {
        const res = await fetch('/api/search?q=' + encodeURIComponent(query));
        const data = await res.json();
        if (data.results && data.results.length > 0) {
          searchResults.innerHTML = data.results.map(r => {
            const badge = r.type === 'project' ? '<span class="result-badge badge-project">P</span>' :
                          r.type === 'session' ? '<span class="result-badge badge-session">S</span>' :
                          '<span class="result-badge badge-message">M</span>';
            const pb = r.provider === 'claude-code' ? ' <span class="provider-badge provider-CC">CC</span>' :
                       r.provider === 'codex' ? ' <span class="provider-badge provider-CX">CX</span>' : '';
            const safeUrl = (r.url && r.url[0] === '/' && r.url[1] !== '/') ? escapeHtml(r.url) : '#';
            let html = '<a href="' + safeUrl + '" class="search-result">';
            html += badge;
            html += '<div class="result-body">';
            html += '<div class="result-title">' + escapeHtml(r.summary || 'Untitled') + pb + '</div>';
            html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' &middot; ' + escapeHtml(r.time) : '') + '</div>';
            if (r.snippet) {
              html += '<div class="result-snippet">' + escapeHtml(r.snippet) + '</div>';
            }
            html += '</div></a>';
            return html;
          }).join('');
          searchResults.classList.add('active');
        } else {
          searchResults.innerHTML = '<div class="search-result"><span class="result-badge badge-message">?</span><div class="result-body"><div class="result-title">No results</div></div></div>';
          searchResults.classList.add('active');
        }
      } catch (err) {
        console.error('Search error:', err);
      }
    }, 200);
  });

  globalSearchInput.addEventListener('blur', function() {
    setTimeout(() => searchResults.classList.remove('active'), 150);
  });
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

const themeToggle = document.getElementById('theme-toggle');
if (themeToggle) {
  themeToggle.addEventListener('click', function() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    html.setAttribute('data-theme', current === 'dark' ? 'light' : 'dark');
    localStorage.setItem('ccx-theme', html.getAttribute('data-theme'));
  });
  const saved = localStorage.getItem('ccx-theme');
  if (saved) document.documentElement.setAttribute('data-theme', saved);
}

const backTop = document.getElementById('back-to-top');
if (backTop) {
  window.addEventListener('scroll', function() {
    backTop.classList.toggle('show', window.scrollY > 300);
  });
  backTop.addEventListener('click', function() {
    window.scrollTo({ top: 0, behavior: 'smooth' });
  });
}
</script>
`
}

func sessionJS(projectName, sessionID string) string {
	return fmt.Sprintf(`
<script>
const projectName = %q;
const sessionID = %q;
let eventSource = null;
let autoScroll = false;

// Progressive loading - load all earlier messages
function loadEarlierMessages() {
  const btn = document.querySelector('.load-earlier');
  if (btn) {
    btn.classList.add('loading');
    btn.querySelector('.load-earlier-btn').innerHTML = '<span class="load-icon">↻</span> Loading...';
  }
  // Persist active search across the reload, so "search full history" picks up where it was
  const pendingQuery = document.getElementById('search-input')?.value;
  if (pendingQuery && pendingQuery.trim().length >= 2) {
    try { sessionStorage.setItem('ccx-pending-search', pendingQuery); } catch (_) {}
  }
  // Reload page with all=1 parameter to load full content
  const url = new URL(window.location.href);
  url.searchParams.set('all', '1');
  window.location.href = url.toString();
}

// Delegated handler for copy buttons with data-copy attribute
document.addEventListener('click', function(e) {
  if (e.target.classList.contains('copy-btn-sm') && e.target.dataset.copy) {
    navigator.clipboard.writeText(e.target.dataset.copy).then(() => {
      const orig = e.target.textContent;
      e.target.textContent = '✓';
      setTimeout(() => e.target.textContent = orig, 1000);
    });
  }
});

document.getElementById('show-thinking')?.addEventListener('change', function() {
  document.querySelectorAll('.block-thinking').forEach(el => {
    if (this.checked) el.setAttribute('open', '');
    else el.removeAttribute('open');
  });
});

document.getElementById('show-tools')?.addEventListener('change', function() {
  document.querySelectorAll('.block-tool').forEach(el => {
    if (this.checked) el.setAttribute('open', '');
    else el.removeAttribute('open');
  });
  document.querySelectorAll('.block-result').forEach(el => {
    el.style.display = this.checked ? 'block' : 'none';
  });
});

const themeToggle = document.getElementById('theme-toggle');
if (themeToggle) {
  themeToggle.addEventListener('click', function() {
    const html = document.documentElement;
    const current = html.getAttribute('data-theme');
    html.setAttribute('data-theme', current === 'dark' ? 'light' : 'dark');
    localStorage.setItem('ccx-theme', html.getAttribute('data-theme'));
  });
  const saved = localStorage.getItem('ccx-theme');
  if (saved) document.documentElement.setAttribute('data-theme', saved);
}

// Global search with debounce and request cancellation
const globalSearchInput = document.getElementById('global-search');
const searchResults = document.getElementById('search-results');
let globalSearchTimeout;
let searchAbort = null;

if (globalSearchInput && searchResults) {
  globalSearchInput.addEventListener('input', function(e) {
    clearTimeout(globalSearchTimeout);
    if (searchAbort) { searchAbort.abort(); searchAbort = null; }

    const query = e.target.value.trim();
    if (!query) {
      searchResults.classList.remove('active');
      return;
    }

    // Show loading state
    searchResults.innerHTML = '<div class="search-loading"><span class="cli-spinner-char"></span> Searching...</div>';
    searchResults.classList.add('active');

    globalSearchTimeout = setTimeout(async () => {
      searchAbort = new AbortController();
      try {
        const res = await fetch('/api/search?q=' + encodeURIComponent(query), { signal: searchAbort.signal });
        const data = await res.json();
        if (data.results && data.results.length > 0) {
          searchResults.innerHTML = data.results.map(r => {
            const badge = r.type === 'project' ? '<span class="result-badge badge-project">P</span>' :
                          r.type === 'session' ? '<span class="result-badge badge-session">S</span>' :
                          '<span class="result-badge badge-message">M</span>';
            const pb = r.provider === 'claude-code' ? ' <span class="provider-badge provider-CC">CC</span>' :
                       r.provider === 'codex' ? ' <span class="provider-badge provider-CX">CX</span>' : '';
            const safeUrl = (r.url && r.url[0] === '/' && r.url[1] !== '/') ? escapeHtml(r.url) : '#';
            let html = '<a href="' + safeUrl + '" class="search-result">';
            html += badge;
            html += '<div class="result-body">';
            html += '<div class="result-title">' + escapeHtml(r.summary || 'Untitled') + pb + '</div>';
            html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' &middot; ' + escapeHtml(r.time) : '') + '</div>';
            if (r.snippet) {
              html += '<div class="result-snippet">' + escapeHtml(r.snippet) + '</div>';
            }
            html += '</div></a>';
            return html;
          }).join('');
          searchResults.classList.add('active');
        } else {
          searchResults.innerHTML = '<div class="search-empty">No results for "' + escapeHtml(query) + '"</div>';
          searchResults.classList.add('active');
        }
      } catch (err) {
        if (err.name !== 'AbortError') console.error('Search:', err);
      }
    }, 300); // 300ms debounce
  });

  globalSearchInput.addEventListener('blur', function() {
    setTimeout(() => searchResults.classList.remove('active'), 200);
  });
}

document.addEventListener('keydown', function(e) {
  if (e.key === '/' && !e.target.matches('input, textarea')) {
    e.preventDefault();
    globalSearchInput?.focus();
  }
  if (e.key === 'Escape') {
    searchResults?.classList.remove('active');
    globalSearchInput?.blur();
  }
});

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function sanitizeCodeLang(lang) {
  if (!lang) return 'text';
  const clean = lang.replace(/[^a-zA-Z0-9_-]/g, '');
  return clean || 'text';
}

function sanitizeMediaType(mt) {
  const allowed = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml'];
  return allowed.includes(mt) ? mt : 'image/png';
}

function isSafeURL(url) {
  if (!url) return false;
  const lower = url.toLowerCase();
  return lower.startsWith('http://') || lower.startsWith('https://');
}

function sanitizeID(s) {
  return s ? s.replace(/[^a-zA-Z0-9_-]/g, '') : '';
}

function toggleSidebar() {
  const layout = document.querySelector('.session-layout');
  const icon = document.getElementById('toggle-icon');
  layout?.classList.toggle('sidebar-collapsed');
  const collapsed = layout?.classList.contains('sidebar-collapsed');
  icon.textContent = collapsed ? '▶' : '◀';
  localStorage.setItem('ccx-sidebar', collapsed ? 'collapsed' : 'expanded');
}
// Restore sidebar state
if (localStorage.getItem('ccx-sidebar') === 'collapsed') {
  document.querySelector('.session-layout')?.classList.add('sidebar-collapsed');
  document.getElementById('toggle-icon').textContent = '▶';
}

function scrollToBottom() {
  window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
}

function copyBlock(e) {
  e.stopPropagation();
  const btn = e.target;
  const tool = btn.closest('.block-tool');
  if (!tool) return;
  const pres = tool.querySelectorAll('pre');
  let text = '';
  pres.forEach(pre => { text += pre.textContent + '\n'; });
  navigator.clipboard.writeText(text.trim()).then(() => {
    btn.textContent = 'copied!';
    btn.classList.add('copied');
    setTimeout(() => { btn.textContent = 'copy'; btn.classList.remove('copied'); }, 1500);
  });
}

function toggleRaw(e) {
  e.stopPropagation();
  const btn = e.target;
  const tool = btn.closest('.block-tool');
  if (!tool) return;
  const inputSection = tool.querySelector('.tool-input-section');
  if (!inputSection) return;

  if (inputSection.dataset.showRaw === 'true') {
    if (inputSection.dataset.original) {
      inputSection.innerHTML = inputSection.dataset.original;
    }
    inputSection.dataset.showRaw = 'false';
    btn.textContent = 'raw';
  } else {
    inputSection.dataset.original = inputSection.innerHTML;
    const pre = inputSection.querySelector('pre');
    if (pre) {
      const rawText = pre.textContent || pre.innerText;
      inputSection.innerHTML = '<div class="section-label">input (raw)</div><pre style="white-space:pre-wrap">' + escapeHtml(rawText) + '</pre>';
    }
    inputSection.dataset.showRaw = 'true';
    btn.textContent = 'fmt';
  }
}

// Bind tool action buttons via event delegation
document.addEventListener('click', function(e) {
  if (e.target.classList.contains('raw-toggle')) {
    toggleRaw(e);
  } else if (e.target.classList.contains('copy-btn')) {
    copyBlock(e);
  }
});

function toggleTurnRaw(e, btn) {
  e.stopPropagation();
  const turn = btn.closest('.turn') || btn.closest('details.turn-user');
  if (!turn) return;
  const body = turn.querySelector('.turn-body');
  if (!body) return;

  if (body.classList.contains('raw-mode')) {
    if (body.dataset.original) {
      body.innerHTML = body.dataset.original;
    }
    body.classList.remove('raw-mode');
    btn.textContent = 'raw';
    btn.classList.remove('active');
  } else {
    body.dataset.original = body.innerHTML;
    let rawData = body.dataset.raw || '[]';
    // Decode base64 if encoded (tailed content uses base64)
    if (body.dataset.rawb64) {
      try { rawData = decodeURIComponent(escape(atob(body.dataset.rawb64))); } catch(e) {}
    }
    // Pretty-print JSON
    try {
      const obj = JSON.parse(rawData);
      rawData = JSON.stringify(obj, null, 2);
    } catch(e) {}
    body.innerHTML = '<pre class="raw-content">' + escapeHtml(rawData) + '</pre>';
    body.classList.add('raw-mode');
    btn.textContent = 'fmt';
    btn.classList.add('active');
  }
}

function copyTurn(e, btn) {
  e.stopPropagation();
  const turn = btn.closest('.turn') || btn.closest('details.turn-user');
  if (!turn) return;
  const body = turn.querySelector('.turn-body');
  if (!body) return;

  let text;
  if (body.classList.contains('raw-mode')) {
    // Raw mode: copy JSON (check both data-rawb64 and data-raw)
    if (body.dataset.rawb64) {
      try { text = decodeURIComponent(escape(atob(body.dataset.rawb64))); } catch(x) { text = ''; }
    } else {
      text = body.dataset.raw || '';
    }
  } else {
    // Fmt mode: copy readable text
    text = body.innerText || body.textContent || '';
  }
  navigator.clipboard.writeText(text);
  btn.textContent = 'copied!';
  setTimeout(() => btn.textContent = 'copy', 1500);
}

// jumpToAnchor is the single entry point for "scroll this message into
// view with smooth behavior, open any closed ancestors, flash it to
// orient the eye." Used by outline clicks, rail ticks, load-earlier
// restore, hash navigation, and search result jumps. Any caller that
// wants "take me there" goes through here — one code path, one set of
// edge cases.
//
// opts.preserveFold (default false) — if true, do NOT auto-unfold a
// closed thread that contains the target. Used by the fold keybinding
// which wants to toggle without moving.
function jumpToAnchor(msgId, opts) {
  opts = opts || {};
  if (!msgId) return false;
  const msgEl = document.getElementById('msg-' + msgId);
  if (!msgEl) return false;

  if (!opts.preserveFold) {
    let node = msgEl;
    while (node) {
      if (node.tagName === 'DETAILS') {
        node.open = true;
        node.setAttribute('open', '');
      }
      if (node.classList && node.classList.contains('thread') && node.classList.contains('folded')) {
        node.classList.remove('folded');
      }
      node = node.parentElement;
    }
    msgEl.querySelectorAll('details').forEach(d => { d.open = true; d.setAttribute('open', ''); });
  }

  // Mark the matching nav item as active so the sidebar mirrors focus.
  document.querySelectorAll('.nav-item.active').forEach(el => el.classList.remove('active'));
  const navItem = document.querySelector('.nav-item[data-msg="' + msgId + '"]');
  if (navItem) {
    navItem.classList.add('active');
    // Also expand the owning group so the active child is visible.
    const group = navItem.closest('.nav-group');
    if (group && group.dataset.expanded === 'false') {
      group.dataset.expanded = 'true';
      const btn = group.querySelector('.nav-expand');
      if (btn) btn.setAttribute('aria-expanded', 'true');
    }
  }

  msgEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
  msgEl.style.animation = 'flash 0.8s';
  // Clear the animation so re-clicking the same target re-flashes.
  setTimeout(() => { msgEl.style.animation = ''; }, 900);
  return true;
}

// Outline title clicks jump; expand buttons toggle. Two targets, two
// actions, no overlap. The nav row intercepts neither — events reach
// the individual child elements.
document.querySelectorAll('.nav-expand').forEach(btn => {
  btn.addEventListener('click', function(e) {
    e.preventDefault();
    e.stopPropagation();
    const group = this.closest('.nav-group');
    if (!group) return;
    const expanded = group.dataset.expanded === 'true';
    group.dataset.expanded = expanded ? 'false' : 'true';
    this.setAttribute('aria-expanded', expanded ? 'false' : 'true');
  });
});

document.querySelectorAll('.nav-item').forEach(item => {
  item.addEventListener('click', function(e) {
    e.preventDefault();
    const msgId = this.dataset.msg;
    jumpToAnchor(msgId);
  });
});

// Scrollspy - highlight nav item matching visible message
const navSidebar = document.getElementById('nav-sidebar');
let lastActiveId = null;
let scrollspyScheduled = false;

function updateScrollspy() {
  scrollspyScheduled = false;
  const viewTop = window.scrollY + 100;
  let currentId = null;

  // Binary search would be better, but for now just walk through visible
  const messages = document.querySelectorAll('[id^="msg-"]');
  for (let i = messages.length - 1; i >= 0; i--) {
    const el = messages[i];
    if (el.getBoundingClientRect().top + window.scrollY <= viewTop) {
      currentId = el.id.replace('msg-', '');
      break;
    }
  }

  if (currentId && currentId !== lastActiveId) {
    lastActiveId = currentId;

    // Update active nav item
    document.querySelectorAll('.nav-item.active').forEach(el => el.classList.remove('active'));
    const activeNav = document.querySelector('.nav-item[data-msg="' + currentId + '"]');
    if (activeNav) {
      activeNav.classList.add('active');
      // Only scroll sidebar if item is out of view (use sidebar viewport, not content)
      if (navSidebar) {
        const rect = activeNav.getBoundingClientRect();
        const sidebarRect = navSidebar.getBoundingClientRect();
        if (rect.top < sidebarRect.top || rect.bottom > sidebarRect.bottom) {
          activeNav.scrollIntoView({ block: 'nearest' });
        }
      }
    }
  }
}

function scheduleScrollspy() {
  if (!scrollspyScheduled) {
    scrollspyScheduled = true;
    requestAnimationFrame(updateScrollspy);
  }
}

window.addEventListener('scroll', scheduleScrollspy, { passive: true });
setTimeout(updateScrollspy, 300);

const btnWatch = document.getElementById('btn-watch');
const tbWatch = document.getElementById('tb-watch');
const tailContainer = document.getElementById('tail-output');

function startWatch() {
  if (eventSource) return;
  eventSource = new EventSource('/api/watch/' + projectName + '/' + sessionID);
  autoScroll = true;
  document.body.classList.add('watching');
  updateWatchUI(true);

  // Auto-enable Think/Tools toggles for full visibility during tailing
  const thinkCb = document.getElementById('show-thinking');
  const toolsCb = document.getElementById('show-tools');
  if (thinkCb && !thinkCb.checked) { thinkCb.checked = true; toggleThinkingBlocks(); }
  if (toolsCb && !toolsCb.checked) { toolsCb.checked = true; toggleToolBlocks(); }
  updateToolbarState();

  scrollToBottom();

  eventSource.addEventListener('line', function(e) {
    try {
      const data = JSON.parse(e.data);
      appendTailMessage(data);
      if (autoScroll) scrollToBottom();
    } catch (err) {
      console.error('Parse error:', err);
    }
  });

  eventSource.addEventListener('error', function() {
    stopWatch();
  });
}

function appendTailMessage(data) {
  // Skip non-conversational types
  if (!['user', 'assistant'].includes(data.type)) return;

  const messagesEl = document.getElementById('messages');
  if (!messagesEl) return;

  const uuid = data.uuid || 'tail-' + Date.now();
  const timestamp = data.timestamp ? new Date(data.timestamp).toLocaleTimeString('en-US', {hour12: false}) : '';
  const content = data.message?.content;
  const model = data.message?.model || '';
  const isSidechain = data.isSidechain || false;
  const isMeta = data.isMeta || false;
  const isCompact = data.isCompactSummary || false;

  // Skip meta and compact summary in tail mode
  if (isMeta || isCompact) return;

  // Classify message kind
  let kind = data.type === 'assistant' ? 'assistant' : 'user';
  let isToolResult = false;
  if (data.type === 'user') {
    // Check if it's tool_result (first block is tool_result)
    if (Array.isArray(content) && content[0]?.type === 'tool_result') {
      isToolResult = true;
      kind = 'result';
    }
  }

  // Build HTML matching Go renderTurnMessage
  let html = '';
  const rawJSON = JSON.stringify(content || []);
  const rawB64 = btoa(unescape(encodeURIComponent(rawJSON)));

  if (kind === 'user') {
    const preview = getTextPreview(content, 60);
    html = '<details class="turn turn-user" id="msg-' + sanitizeID(uuid) + '" open>' +
      '<summary class="turn-header">' +
        '<span class="turn-icon">▶</span>' +
        '<span class="turn-role">USER</span>' +
        '<span class="turn-preview">' + escapeHtml(preview) + '</span>' +
        '<span class="turn-time">' + timestamp + '</span>' +
        '<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(event,this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(event,this)">copy</button></span>' +
      '</summary>' +
      '<div class="turn-body" data-rawb64="' + rawB64 + '">' +
        renderContentBlocks(content) +
      '</div>' +
    '</details>';
  } else if (kind === 'result') {
    // Get tool name from first tool_result block
    let resultToolName = 'result';
    if (Array.isArray(content) && content[0]?.type === 'tool_result') {
      const tid = content[0].tool_use_id;
      if (window.toolIdMap && window.toolIdMap[tid]) {
        resultToolName = window.toolIdMap[tid];
      }
    }
    html = '<div class="turn turn-result" id="msg-' + sanitizeID(uuid) + '">' +
      '<div class="turn-header">' +
        '<span class="turn-icon">○</span>' +
        '<span class="turn-role">' + escapeHtml(resultToolName) + '</span>' +
        '<span class="turn-time">' + timestamp + '</span>' +
        '<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(event,this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(event,this)">copy</button></span>' +
      '</div>' +
      '<div class="turn-body" data-rawb64="' + rawB64 + '">' +
        renderContentBlocks(content) +
      '</div>' +
    '</div>';
  } else {
    let turnClass = 'turn turn-assistant';
    let icon = '●';
    let role = 'ASSISTANT';
    if (isSidechain) {
      turnClass += ' turn-agent';
      icon = '◆';
      role = 'AGENT';
    }
    html = '<div class="' + turnClass + '" id="msg-' + sanitizeID(uuid) + '">' +
      '<div class="turn-header">' +
        '<span class="turn-icon">' + icon + '</span>' +
        '<span class="turn-role">' + role + '</span>' +
        '<span class="turn-time">' + timestamp + '</span>' +
        (model ? '<span class="turn-model">' + escapeHtml(model) + '</span>' : '') +
        '<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(event,this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(event,this)">copy</button></span>' +
      '</div>' +
      '<div class="turn-body" data-rawb64="' + rawB64 + '">' +
        renderContentBlocks(content) +
      '</div>' +
    '</div>';
  }

  messagesEl.insertAdjacentHTML('beforeend', html);

  // Update nav sidebar
  updateNavForMessage(uuid, kind, content, timestamp);
}

function getTextPreview(content, maxLen) {
  if (typeof content === 'string') return content.slice(0, maxLen);
  if (!Array.isArray(content)) return '';
  for (const block of content) {
    if (block.type === 'text' && block.text) {
      const firstLine = block.text.split('\n')[0];
      return firstLine.slice(0, maxLen);
    }
  }
  return '';
}

function renderContentBlocks(content, forceExpand) {
  if (!content) return '';
  if (typeof content === 'string') {
    return '<div class="block-text">' + renderMarkdownJS(content) + '</div>';
  }
  if (!Array.isArray(content)) return '';

  let html = '';
  const isWatching = !!eventSource;
  const expandAll = forceExpand || isWatching;
  const showThinking = expandAll || document.getElementById('show-thinking')?.checked;
  const showTools = expandAll || document.getElementById('show-tools')?.checked !== false;

  for (const block of content) {
    switch (block.type) {
      case 'text':
        if (block.text) {
          html += '<div class="block-text">' + renderMarkdownJS(block.text) + '</div>';
        }
        break;
      case 'thinking':
        const thinkOpen = showThinking ? ' open' : '';
        html += '<details class="block-thinking"' + thinkOpen + '>' +
          '<summary><span class="block-icon">∴</span> Thinking...</summary>' +
          '<div class="block-content">' + escapeHtml(block.thinking || block.text || '') + '</div>' +
        '</details>';
        break;
      case 'tool_use':
        const toolName = block.name || 'tool';
        const toolId = block.id || 'tool-' + Date.now();
        // Track tool ID -> name mapping for result lookup
        if (!window.toolIdMap) window.toolIdMap = {};
        window.toolIdMap[toolId] = toolName;
        const isActive = ['Write','Edit','Bash','Task','TodoWrite','Skill','NotebookEdit'].includes(toolName);
        const toolOpen = (isActive || showTools) ? ' open' : '';
        const inputPreview = compactToolPreviewJS(toolName, block.input);
        html += '<details class="block-tool" id="tool-' + sanitizeID(toolId) + '"' + toolOpen + '>' +
          '<summary><span class="block-icon">●</span> ' + escapeHtml(toolName) +
          '<span class="tool-preview">' + escapeHtml(inputPreview) + '</span>' +
          '<span class="tool-actions"><button class="raw-toggle">raw</button><button class="copy-btn">copy</button></span></summary>' +
          '<div class="tool-section tool-input-section">' +
            '<div class="section-label">input</div>' +
            renderToolInputJS(toolName, block.input) +
          '</div>' +
        '</details>';
        break;
      case 'image':
        if (block.source && block.source.data) {
          html += '<img src="data:' + sanitizeMediaType(block.source.media_type) + ';base64,' + block.source.data + '" class="block-image">';
        }
        break;
      case 'tool_result':
        const resId = block.tool_use_id || '';
        const resToolName = (window.toolIdMap && window.toolIdMap[resId]) || 'tool';
        let resContent = '';
        if (typeof block.content === 'string') {
          resContent = block.content;
        } else if (Array.isArray(block.content)) {
          for (const c of block.content) {
            if (c.type === 'text') resContent += c.text + '\n';
          }
        }
        const truncRes = resContent.length > 500 ? resContent.slice(0, 500) + '...' : resContent;
        // Inline result - no nested details, just output with tool name
        html += '<div class="block-result-inline">' +
          '<div class="result-header"><span class="block-icon">○</span> ' + escapeHtml(resToolName) + '</div>' +
          '<pre class="tool-output">' + escapeHtml(truncRes) + '</pre>' +
        '</div>';
        break;
    }
  }
  return html;
}

function renderToolInputJS(toolName, input) {
  if (!input) return '<pre>{}</pre>';

  switch (toolName) {
    case 'Edit':
      let editHtml = '<div class="edit-diff">';
      if (input.file_path) editHtml += '<div class="diff-file">' + escapeHtml(input.file_path) + '</div>';
      if (input.old_string) editHtml += '<pre class="diff-old">' + escapeHtml(input.old_string) + '</pre>';
      if (input.new_string) editHtml += '<pre class="diff-new">' + escapeHtml(input.new_string) + '</pre>';
      return editHtml + '</div>';

    case 'Write':
      let writeHtml = '<div class="write-content">';
      if (input.file_path) writeHtml += '<div class="diff-file">' + escapeHtml(input.file_path) + '</div>';
      if (input.content) {
        const c = input.content;
        if (c.length > 2000) {
          writeHtml += '<details class="long-output"><summary><pre class="output-preview">' + escapeHtml(c.slice(0,200)) + '...</pre><span class="expand-hint">(' + c.length + ' chars)</span></summary><pre class="diff-new">' + escapeHtml(c) + '</pre></details>';
        } else {
          writeHtml += '<pre class="diff-new">' + escapeHtml(c) + '</pre>';
        }
      }
      return writeHtml + '</div>';

    case 'TodoWrite':
      if (input.todos && Array.isArray(input.todos)) {
        let todoHtml = '<ul class="todo-checklist">';
        for (const todo of input.todos) {
          const status = todo.status || 'pending';
          const icon = status === 'completed' ? '✓' : status === 'in_progress' ? '◐' : '○';
          const cls = 'todo-' + (status === 'completed' ? 'completed' : status === 'in_progress' ? 'progress' : 'pending');
          const checked = status === 'completed' ? ' checked disabled' : '';
          todoHtml += '<li class="' + cls + '"><span class="todo-icon">' + icon + '</span><input type="checkbox"' + checked + '><span class="todo-text">' + escapeHtml(todo.content || '') + '</span></li>';
        }
        return todoHtml + '</ul>';
      }
      break;

    case 'Bash':
      if (input.command) {
        return '<pre class="tool-input">$ ' + escapeHtml(input.command) + '</pre>';
      }
      break;

    case 'Task':
      let taskHtml = '<div class="task-call">';
      if (input.subagent_type) taskHtml += '<span class="task-agent">[' + escapeHtml(input.subagent_type) + ']</span>';
      if (input.model) taskHtml += '<span class="task-model">' + escapeHtml(input.model) + '</span>';
      if (input.prompt) taskHtml += '<div class="task-prompt">' + renderMarkdownJS(input.prompt) + '</div>';
      return taskHtml + '</div>';

    case 'Skill':
      let skillHtml = '<div class="skill-call">';
      if (input.skill) skillHtml += '<span class="skill-name">/' + escapeHtml(input.skill) + '</span>';
      if (input.args) skillHtml += '<span class="skill-args">' + escapeHtml(input.args) + '</span>';
      return skillHtml + '</div>';

    case 'WebSearch':
      return '<div class="websearch-call"><span class="search-query">🔍 ' + escapeHtml(input.query || '') + '</span></div>';

    case 'WebFetch': {
      let fetchHtml = '<div class="webfetch-call">';
      if (input.url) {
        const escaped = escapeHtml(input.url);
        if (isSafeURL(input.url)) {
          fetchHtml += '<a href="' + escaped + '" class="fetch-url" target="_blank" rel="noopener noreferrer">' + escaped + '</a>';
        } else {
          fetchHtml += '<span class="fetch-url">' + escaped + '</span>';
        }
      }
      if (input.prompt) fetchHtml += '<div class="fetch-prompt">' + escapeHtml(input.prompt) + '</div>';
      return fetchHtml + '</div>';
    }

    case 'AskUserQuestion':
      if (input.questions && Array.isArray(input.questions)) {
        let askHtml = '<div class="ask-questions">';
        for (const q of input.questions) {
          askHtml += '<div class="ask-question">';
          if (q.header) askHtml += '<span class="ask-header">' + escapeHtml(q.header) + '</span>';
          if (q.question) askHtml += '<div class="ask-text">' + escapeHtml(q.question) + '</div>';
          if (q.options && Array.isArray(q.options)) {
            askHtml += '<ul class="ask-options">';
            for (const o of q.options) {
              askHtml += '<li><strong>' + escapeHtml(o.label || '') + '</strong>';
              if (o.description) askHtml += ' - ' + escapeHtml(o.description);
              askHtml += '</li>';
            }
            askHtml += '</ul>';
          }
          askHtml += '</div>';
        }
        return askHtml + '</div>';
      }
      break;

    case 'LSP': {
      let lspHtml = '<div class="lsp-call">';
      if (input.operation) lspHtml += '<span class="lsp-op">' + escapeHtml(input.operation) + '</span>';
      if (input.filePath) lspHtml += '<span class="lsp-loc">' + escapeHtml(input.filePath) + ':' + (input.line||0) + ':' + (input.character||0) + '</span>';
      return lspHtml + '</div>';
    }

    case 'TaskOutput': {
      let toHtml = '<div class="taskoutput-call">';
      if (input.task_id) toHtml += '<span class="task-id">' + escapeHtml(input.task_id) + '</span>';
      if (input.block !== undefined) toHtml += '<span class="task-mode">' + (input.block ? 'blocking' : 'async') + '</span>';
      return toHtml + '</div>';
    }

    case 'KillShell':
      return '<div class="killshell-call"><span class="shell-id">⊗ ' + escapeHtml(input.shell_id || '') + '</span></div>';
  }

  return '<pre class="tool-input">' + escapeHtml(JSON.stringify(input, null, 2)) + '</pre>';
}

function renderMarkdownJS(text) {
  // Full markdown: code blocks, tables, headers, lists, inline formatting
  const BT = '`+"`"+`';
  const lines = text.split('\n');
  let html = '';
  let inCodeBlock = false;
  let codeBlockLang = '';
  let codeLines = [];
  let inTable = false;
  let tableRows = [];

  for (let i = 0; i < lines.length; i++) {
    let line = lines[i];

    // Code blocks
    if (line.startsWith(BT+BT+BT)) {
      if (inCodeBlock) {
        html += '<pre class="code-block"><code class="lang-' + sanitizeCodeLang(codeBlockLang) + '">' + escapeHtml(codeLines.join('\n')) + '</code></pre>';
        codeLines = [];
        inCodeBlock = false;
      } else {
        inCodeBlock = true;
        codeBlockLang = line.slice(3);
      }
      continue;
    }
    if (inCodeBlock) { codeLines.push(line); continue; }

    // Tables
    const trimmed = line.trim();
    const isTableLine = trimmed.startsWith('|') && trimmed.includes('|');
    const isSeparator = isTableLine && trimmed.includes('---');

    if (isTableLine) {
      if (!inTable) { inTable = true; tableRows = []; }
      if (!isSeparator) { tableRows.push(line); }
      const nextLine = lines[i + 1];
      if (!nextLine || !nextLine.trim().startsWith('|')) {
        html += renderTableJS(tableRows);
        inTable = false; tableRows = [];
      }
      continue;
    }

    // Empty lines - collapse multiples
    if (trimmed === '') {
      if (!html.endsWith('<br>')) html += '<br>';
      continue;
    }

    // Headers
    if (line.startsWith('#### ')) { html += '<div class="md-h4">' + processInline(line.slice(5)) + '</div>'; continue; }
    if (line.startsWith('### ')) { html += '<div class="md-h3">' + processInline(line.slice(4)) + '</div>'; continue; }
    if (line.startsWith('## ')) { html += '<div class="md-h2">' + processInline(line.slice(3)) + '</div>'; continue; }
    if (line.startsWith('# ')) { html += '<div class="md-h1">' + processInline(line.slice(2)) + '</div>'; continue; }

    // Lists
    if (line.match(/^[\-\*] /)) { html += '<div class="md-li">• ' + processInline(line.slice(2)) + '</div>'; continue; }
    if (line.match(/^\d+\. /)) { html += '<div class="md-li">' + processInline(line) + '</div>'; continue; }

    // Regular text - no wrapping, just inline
    html += processInline(line) + '\n';
  }

  if (inCodeBlock) {
    html += '<pre class="code-block"><code class="lang-' + sanitizeCodeLang(codeBlockLang) + '">' + escapeHtml(codeLines.join('\n')) + '</code></pre>';
  }
  return html;

  function processInline(s) {
    const BT = '`+"`"+`';
    return escapeHtml(s)
      .replace(new RegExp(BT + '([^' + BT + ']+)' + BT, 'g'), '<code>$1</code>')
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(m, text, url) {
        if (/^https?:\/\//i.test(url)) {
          return '<a href="' + url + '" target="_blank" rel="noopener noreferrer">' + text + '</a>';
        }
        if (/^mailto:/i.test(url)) {
          return '<a href="' + url + '">' + text + '</a>';
        }
        return text + ' (' + url + ')';
      });
  }
}

function renderTableJS(rows) {
  if (!rows.length) return '';
  let html = '<table class="md-table">';
  rows.forEach((row, i) => {
    const cells = row.split('|').filter((c, idx, arr) => idx > 0 && idx < arr.length - 1);
    if (i === 0) {
      html += '<thead><tr>' + cells.map(c => '<th>' + escapeHtml(c.trim()) + '</th>').join('') + '</tr></thead><tbody>';
    } else {
      html += '<tr>' + cells.map(c => '<td>' + escapeHtml(c.trim()) + '</td>').join('') + '</tr>';
    }
  });
  return html + '</tbody></table>';
}

function compactToolPreviewJS(name, input) {
  if (!input) return '';
  switch (name) {
    case 'Read': return input.file_path || '';
    case 'Edit': return input.file_path || '';
    case 'Write': return input.file_path || '';
    case 'Bash': return (input.command || '').slice(0, 50);
    case 'Glob': return input.pattern || '';
    case 'Grep': return input.pattern ? '/' + input.pattern + '/' : '';
    case 'Task':
      const parts = [];
      if (input.subagent_type) parts.push('[' + input.subagent_type + ']');
      if (input.description) parts.push(input.description);
      return parts.join(' ');
    case 'Skill': return input.skill ? '/' + input.skill : '';
    case 'WebSearch': return input.query ? (input.query.length > 50 ? input.query.slice(0,50) + '...' : input.query) : '';
    case 'WebFetch': return input.url || '';
    case 'AskUserQuestion': return input.questions?.[0]?.header || '';
    case 'LSP': {
      const op = input.operation || '';
      const fp = input.filePath || '';
      const fname = fp.split('/').pop();
      return fname ? op + ' ' + fname : op;
    }
    case 'TaskOutput': return input.task_id || '';
    case 'KillShell': return input.shell_id || '';
    default: return '';
  }
}

function updateNavForMessage(uuid, kind, content, time) {
  const navList = document.getElementById('nav-list');
  if (!navList) return;

  // Sanitize ID to match DOM element IDs
  const safeId = sanitizeID(uuid);

  // Add "Live" section separator if not present
  let liveSection = navList.querySelector('.nav-live-section');
  if (!liveSection) {
    liveSection = document.createElement('div');
    liveSection.className = 'nav-live-section';
    liveSection.innerHTML = '<div class="nav-live-label">● Live</div>';
    navList.appendChild(liveSection);
  }

  let icon = kind === 'user' ? '▶' : '●';
  let cls = kind === 'user' ? 'nav-user' : 'nav-response';
  let text = kind === 'user' ? getTextPreview(content, 40) : 'Response';

  const item = document.createElement('a');
  item.href = '#msg-' + safeId;
  item.className = 'nav-item ' + cls;
  item.dataset.msg = safeId;
  item.innerHTML = '<span class="nav-icon">' + icon + '</span><span class="nav-text">' + escapeHtml(text || kind) + '</span>';
  item.addEventListener('click', function(e) {
    e.preventDefault();
    jumpToAnchor(safeId);
  });
  liveSection.appendChild(item);
}

function stopWatch() {
  eventSource?.close();
  eventSource = null;
  autoScroll = false;
  document.body.classList.remove('watching');
  updateWatchUI(false);
}

function updateWatchUI(active) {
  if (btnWatch) {
    btnWatch.textContent = active ? 'Stop' : 'Watch';
    btnWatch.classList.toggle('active', active);
  }
  if (tbWatch) {
    // Don't change textContent - just toggle class (preserves icon structure)
    tbWatch.classList.toggle('active', active);
    const label = tbWatch.querySelector('.dock-label');
    if (label) label.textContent = active ? 'Stop' : 'Live';
  }
}

function toggleWatch() {
  if (eventSource) stopWatch();
  else startWatch();
}

if (btnWatch) btnWatch.addEventListener('click', toggleWatch);
if (tbWatch) tbWatch.addEventListener('click', toggleWatch);

// Thread folding — one floating button per thread.
//
// The button is sticky-positioned over the thread's vertical line
// (see .fold-toggle CSS). Default state collapses threads that have
// ANY intermediate response (2+ turns in the responses column), so
// long sessions don't greet users with a wall of tool calls. The
// button reveals its full label on hover/focus; a one-shot pulse
// animation on first render draws the eye to it.
document.querySelectorAll('.thread').forEach(thread => {
  const responses = thread.querySelector('.thread-responses');
  if (!responses) return;
  const turns = responses.querySelectorAll('.turn');
  if (turns.length <= 1) return;

  const hiddenWhenFolded = turns.length - 1;

  const fold = document.createElement('button');
  fold.type = 'button';
  fold.className = 'fold-toggle';
  fold.title = 'Toggle thread (z)';
  fold.innerHTML =
    '<span class="fold-chevron" aria-hidden="true">▸</span>' +
    '<span class="fold-label">expand</span>' +
    '<span class="fold-summary">' + hiddenWhenFolded + ' steps</span>';

  const applyState = function(folded) {
    thread.classList.toggle('folded', folded);
    fold.setAttribute('aria-expanded', folded ? 'false' : 'true');
    const label = fold.querySelector('.fold-label');
    const chev  = fold.querySelector('.fold-chevron');
    if (label) label.textContent = folded ? 'expand' : 'collapse';
    if (chev)  chev.textContent  = folded ? '▸' : '▾';
  };

  fold.addEventListener('click', function(e) {
    e.preventDefault();
    applyState(!thread.classList.contains('folded'));
  });

  // Insert as the first child of .thread-responses so the button
  // sits at the TOP of the vertical line, overlapping its left edge.
  responses.insertBefore(fold, responses.firstChild);

  // Default: fold any thread with intermediate responses. A single
  // user→one-reply thread (turns.length === 1) doesn't even get a
  // fold button (handled by the early return above).
  applyState(turns.length >= 2);
});

// z keybinding toggles the thread fold under the current scroll position.
// Finds the visible thread whose top is nearest the viewport top and
// clicks its fold-toggle button.
function toggleNearestThreadFold() {
  const threads = document.querySelectorAll('.thread');
  if (threads.length === 0) return;
  const viewTop = window.scrollY + 80;
  let best = null;
  let bestDist = Infinity;
  threads.forEach(t => {
    const rect = t.getBoundingClientRect();
    const top = rect.top + window.scrollY;
    if (top <= viewTop + 200) {
      const d = Math.abs(top - viewTop);
      if (d < bestDist) { bestDist = d; best = t; }
    }
  });
  if (!best) return;
  const btn = best.querySelector('.fold-toggle');
  if (btn) btn.click();
}

const btnExport = document.getElementById('btn-export');
const exportMenu = document.getElementById('export-menu');
if (btnExport && exportMenu) {
  btnExport.addEventListener('click', function(e) {
    e.stopPropagation();
    exportMenu.classList.toggle('show');
  });
  document.addEventListener('click', function() {
    exportMenu.classList.remove('show');
  });
}

// Auto-scroll: jump to hash target on load, reload with full history
// if the target is hidden inside a progressive-loaded section, or
// scroll to bottom when there's no hash.
function jumpToHashTarget() {
  const hash = window.location.hash;
  if (!hash || !hash.startsWith('#msg-')) {
    window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
    return;
  }

  const msgId = hash.replace('#msg-', '');
  if (jumpToAnchor(msgId)) return;

  // Target not in DOM — the load-earlier escape hatch is our last
  // resort: reload with ?all=1 so the full history renders, then the
  // browser re-runs this handler after load and the jumpToAnchor call
  // above will find the element.
  if (document.getElementById('load-earlier')) {
    const url = new URL(window.location.href);
    url.searchParams.set('all', '1');
    window.location.replace(url.toString());
  }
}
setTimeout(jumpToHashTarget, 150);

// Listen for manual hash changes (e.g. nav clicks after initial load)
window.addEventListener('hashchange', jumpToHashTarget);

// Timeline rail — semantic scrubber with hover-scrub, fisheye zoom,
// hysteresis, and rAF-throttled motion.
const timelineRail = document.getElementById('timeline-rail');
const timelineSpine = document.getElementById('timeline-spine');
const timelineCurrent = document.getElementById('timeline-current');
const timelinePlayhead = document.getElementById('timeline-playhead');
const timelineTooltip = document.getElementById('timeline-tooltip');
const timelineTicksRaw = timelineRail ? Array.from(timelineRail.querySelectorAll('.timeline-tick')) : [];

// Snapshot tick data once so hover-scrub is O(log n) and doesn't re-parse
// attributes on every mousemove. Cost / cumulative / token strings are
// preformatted server-side so the tooltip just displays them.
const timelineTicks = timelineTicksRaw.map(el => {
  const pct = parseFloat(el.style.top) || 0;
  return {
    el:         el,
    uuid:       el.dataset.uuid || '',
    offset:     el.dataset.offset || '',
    snippet:    el.dataset.snippet || '',
    kind:       el.dataset.kind || 'user',
    cost:       el.dataset.cost || '',
    cumulative: el.dataset.cumulative || '',
    tokens:     el.dataset.tokens || '',
    index:      el.dataset.index || '',
    clock:      el.dataset.clock || '',
    duration:   el.dataset.duration || '',
    subagents:  parseInt(el.dataset.subagents || '0', 10) || 0,
    skills:     parseInt(el.dataset.skills || '0', 10) || 0,
    tools:      parseInt(el.dataset.tools || '0', 10) || 0,
    pct:        pct,
  };
}).sort((a, b) => a.pct - b.pct);

// Interaction state
let currentNearest = null;         // Tick currently shown in the tooltip
let zoomedTicks = [];              // Ticks with active fisheye zoom classes
let rafHandle = null;              // Pending rAF id
let pendingClientY = null;         // Latest mouse Y, coalesced to next frame
let leaveTimer = null;             // mouseleave grace-period timer

// Hysteresis: cursor must move this much closer (as %% of rail height)
// to a new tick before we switch the tooltip away from the locked one.
// Prevents flicker between adjacent ticks when the cursor sits on the edge.
const TIMELINE_HYSTERESIS_PCT = 1.5;
const TIMELINE_LEAVE_GRACE_MS = 120;

function clearNearest() {
  if (currentNearest) currentNearest.el.classList.remove('tick-nearest');
  currentNearest = null;
}

function clearZoom() {
  for (const t of zoomedTicks) {
    t.el.classList.remove('zoom-0', 'zoom-1', 'zoom-2');
  }
  zoomedTicks = [];
}

// Binary-search helper: returns the index of the tick whose pct is
// closest to the cursor. Returns -1 when there are no ticks.
function nearestTickIndex(cursorPct) {
  if (timelineTicks.length === 0) return -1;
  let lo = 0, hi = timelineTicks.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (timelineTicks[mid].pct < cursorPct) lo = mid + 1;
    else hi = mid;
  }
  if (lo > 0 &&
      Math.abs(timelineTicks[lo - 1].pct - cursorPct) <
      Math.abs(timelineTicks[lo].pct - cursorPct)) {
    return lo - 1;
  }
  return lo;
}

// Apply fisheye zoom to the 5 ticks nearest centerIdx (±2 around it).
function applyFisheyeZoom(centerIdx) {
  clearZoom();
  if (centerIdx < 0) return;
  const radius = 2;
  const start = Math.max(0, centerIdx - radius);
  const end = Math.min(timelineTicks.length - 1, centerIdx + radius);
  for (let i = start; i <= end; i++) {
    const distance = Math.abs(i - centerIdx);
    timelineTicks[i].el.classList.add('zoom-' + distance);
    zoomedTicks.push(timelineTicks[i]);
  }
}

// Hysteresis: return the locked tick unless the cursor is meaningfully
// closer (by TIMELINE_HYSTERESIS_PCT) to a different tick. Prevents
// rapid flipping between adjacent ticks at their midpoint.
function selectWithHysteresis(cursorPct, candidateIdx) {
  if (candidateIdx < 0) return null;
  const candidate = timelineTicks[candidateIdx];
  if (!currentNearest) return candidate;
  if (currentNearest.el === candidate.el) return currentNearest;
  const distToCurrent = Math.abs(cursorPct - currentNearest.pct);
  const distToCandidate = Math.abs(cursorPct - candidate.pct);
  if (distToCandidate + TIMELINE_HYSTERESIS_PCT < distToCurrent) {
    return candidate;
  }
  return currentNearest;
}

function updateTimelineCurrent() {
  if (!timelineCurrent) return;
  const doc = document.documentElement;
  const scrollTop = window.scrollY || doc.scrollTop || 0;
  const scrollMax = (doc.scrollHeight - doc.clientHeight);
  if (scrollMax <= 0) {
    timelineCurrent.style.top = '0%%';
    return;
  }
  const pct = (scrollTop / scrollMax) * 100;
  timelineCurrent.style.top = pct.toFixed(2) + '%%';
}

function showTooltip(tick, clientY) {
  if (!timelineTooltip || !tick) return;

  const clockEl    = timelineTooltip.querySelector('.tt-clock');
  const offsetEl   = timelineTooltip.querySelector('.tt-offset');
  const indexEl    = timelineTooltip.querySelector('.tt-index');
  const durationEl = timelineTooltip.querySelector('.tt-duration');
  const snippetEl  = timelineTooltip.querySelector('.tt-snippet');
  const costEl     = timelineTooltip.querySelector('.tt-cost');
  const cumEl      = timelineTooltip.querySelector('.tt-cum');
  const tokensEl   = timelineTooltip.querySelector('.tt-tokens');
  const badgesEl   = timelineTooltip.querySelector('.tt-badges');

  if (clockEl)    clockEl.textContent    = tick.clock || '';
  if (indexEl)    indexEl.textContent    = tick.index ? 'exchange ' + tick.index : '';
  if (offsetEl)   offsetEl.textContent   = tick.offset || '';
  if (durationEl) durationEl.textContent = tick.duration || '';
  if (snippetEl)  snippetEl.textContent  = tick.snippet || '(no preview)';
  if (costEl)     costEl.textContent     = tick.cost || '';
  if (cumEl)      cumEl.textContent      = tick.cumulative ? 'so far ' + tick.cumulative : '';
  if (tokensEl)   tokensEl.textContent   = tick.tokens || '';

  if (badgesEl) {
    const parts = [];
    if (tick.subagents > 0) {
      parts.push('<span class="badge badge-subagent">⎇ ' + tick.subagents + ' ' + (tick.subagents === 1 ? 'subagent' : 'subagents') + '</span>');
    }
    if (tick.skills > 0) {
      parts.push('<span class="badge badge-skill">✦ ' + tick.skills + ' ' + (tick.skills === 1 ? 'skill' : 'skills') + '</span>');
    }
    if (tick.tools > 0) {
      parts.push('<span class="badge badge-tool">◈ ' + tick.tools + ' ' + (tick.tools === 1 ? 'tool' : 'tools') + '</span>');
    }
    badgesEl.innerHTML = parts.join('<span class="badge-sep"> · </span>');
  }

  timelineTooltip.className = 'timeline-tooltip kind-' + tick.kind + ' show';

  // Viewport clamp — keep the tooltip fully visible even near top/bottom edges.
  // Use the tooltip's actual height once it's laid out.
  const tooltipHeight = timelineTooltip.offsetHeight || 64;
  const margin = 12;
  const minY = tooltipHeight / 2 + margin;
  const maxY = window.innerHeight - tooltipHeight / 2 - margin;
  const clampedY = Math.max(minY, Math.min(maxY, clientY));
  timelineTooltip.style.top = clampedY + 'px';
}

function hideTooltip() {
  if (timelineTooltip) timelineTooltip.classList.remove('show');
  clearNearest();
  clearZoom();
  if (timelinePlayhead) timelinePlayhead.style.opacity = '0';
}

function processRailFrame() {
  rafHandle = null;
  if (pendingClientY === null || !timelineSpine || timelineTicks.length === 0) return;
  const rect = timelineSpine.getBoundingClientRect();
  if (rect.height <= 0) return;
  const cursorY = pendingClientY;
  pendingClientY = null;
  const pct = Math.max(0, Math.min(100, ((cursorY - rect.top) / rect.height) * 100));

  const candidateIdx = nearestTickIndex(pct);
  applyFisheyeZoom(candidateIdx);

  const tick = selectWithHysteresis(pct, candidateIdx);
  if (!tick) return;

  if (tick !== currentNearest) {
    clearNearest();
    tick.el.classList.add('tick-nearest');
    currentNearest = tick;
  }

  if (timelinePlayhead) {
    timelinePlayhead.style.top = tick.pct.toFixed(2) + '%%';
    timelinePlayhead.style.opacity = '0.9';
  }

  // Snap tooltip Y to the tick's screen position (stable, not floating with cursor)
  const tickScreenY = rect.top + (tick.pct / 100) * rect.height;
  showTooltip(tick, tickScreenY);
}

function handleRailMouse(e) {
  // Cancel any pending leave grace timer — user is still engaging
  if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }

  pendingClientY = e.clientY;
  if (rafHandle === null) {
    rafHandle = requestAnimationFrame(processRailFrame);
  }
}

function handleRailLeave() {
  // Grace period: user might have slipped slightly off the rail; wait
  // a moment before fading out so re-entering feels uninterrupted.
  if (leaveTimer !== null) clearTimeout(leaveTimer);
  leaveTimer = setTimeout(() => {
    leaveTimer = null;
    hideTooltip();
  }, TIMELINE_LEAVE_GRACE_MS);
}

function handleRailEnter() {
  if (leaveTimer !== null) { clearTimeout(leaveTimer); leaveTimer = null; }
}

function handleRailClick(e) {
  if (!currentNearest) return;
  e.preventDefault();
  // Update hash so the URL is shareable, but route the actual scroll
  // through jumpToAnchor so ancestor unfold + flash + sidebar active
  // state all happen consistently with outline clicks.
  const uuid = currentNearest.uuid;
  if (uuid) {
    history.replaceState(null, '', '#msg-' + uuid);
    jumpToAnchor(uuid);
  }
}

if (timelineRail) {
  updateTimelineCurrent();
  window.addEventListener('scroll', updateTimelineCurrent, { passive: true });
  window.addEventListener('resize', updateTimelineCurrent);

  if (timelineTicks.length > 0) {
    timelineRail.addEventListener('mouseenter', handleRailEnter);
    timelineRail.addEventListener('mousemove', handleRailMouse, { passive: true });
    timelineRail.addEventListener('mouseleave', handleRailLeave);
    timelineRail.addEventListener('click', handleRailClick);
  }
}

function jumpTickRelative(delta) {
  if (timelineTicks.length === 0) return;
  const doc = document.documentElement;
  const scrollTop = window.scrollY || doc.scrollTop || 0;
  const scrollMax = (doc.scrollHeight - doc.clientHeight);
  const currentPct = scrollMax > 0 ? (scrollTop / scrollMax) * 100 : 0;

  let idx;
  if (delta > 0) {
    idx = timelineTicks.findIndex(t => t.pct > currentPct + 0.5);
    if (idx === -1) idx = timelineTicks.length - 1;
  } else {
    idx = -1;
    for (let i = 0; i < timelineTicks.length; i++) {
      if (timelineTicks[i].pct < currentPct - 0.5) idx = i;
      else break;
    }
    if (idx === -1) idx = 0;
  }
  const target = timelineTicks[idx];
  if (target && target.uuid) {
    history.replaceState(null, '', '#msg-' + target.uuid);
    jumpToAnchor(target.uuid);
  }
}

document.addEventListener('keydown', function(e) {
  if (e.target.matches('input, textarea')) return;
  switch(e.key) {
    case 'j': document.getElementById('tb-next-user')?.click(); break;
    case 'k': document.getElementById('tb-prev-user')?.click(); break;
    case 'g': if (e.shiftKey) scrollToBottom(); else window.scrollTo(0, 0); break;
    case 't': document.getElementById('show-thinking')?.click(); break;
    case 'o': document.getElementById('show-tools')?.click(); break;
    case 'i': document.getElementById('tb-info')?.click(); break;
    case 'w': btnWatch?.click(); break;
    case 'r': document.getElementById('tb-refresh')?.click(); break;
    case '[': jumpTickRelative(-1); e.preventDefault(); break;
    case ']': jumpTickRelative(1); e.preventDefault(); break;
    case 'z': toggleNearestThreadFold(); e.preventDefault(); break;
  }
});

// Floating toolbar
document.getElementById('tb-info')?.addEventListener('click', (e) => {
  e.stopPropagation();
  document.getElementById('info-panel')?.classList.toggle('show');
});
document.getElementById('tb-thinking')?.addEventListener('click', () => {
  const cb = document.getElementById('show-thinking');
  if (cb) { cb.checked = !cb.checked; updateToolbarState(); toggleThinkingBlocks(); }
});
document.getElementById('tb-tools')?.addEventListener('click', () => {
  const cb = document.getElementById('show-tools');
  if (cb) { cb.checked = !cb.checked; updateToolbarState(); toggleToolBlocks(); }
});
document.getElementById('tb-export')?.addEventListener('click', (e) => {
  e.stopPropagation();
  document.getElementById('toolbar-export-menu')?.classList.toggle('show');
});
// User prompt navigation
const HEADER_OFFSET = 60; // 48px header + margin
let userBlocks = [];
let currentUserIdx = -1;

function initUserNav() {
  userBlocks = Array.from(document.querySelectorAll('.turn-user'));
}

function scrollToUser(idx) {
  if (idx < 0 || idx >= userBlocks.length) return;
  currentUserIdx = idx;
  const el = userBlocks[idx];
  const top = el.getBoundingClientRect().top + window.scrollY - HEADER_OFFSET;
  window.scrollTo({ top: top, behavior: 'smooth' });
  // Brief highlight
  el.style.outline = '2px solid var(--user-border)';
  setTimeout(() => { el.style.outline = ''; }, 800);
}

function findCurrentUserIdx() {
  const scrollY = window.scrollY + HEADER_OFFSET + 10;
  for (let i = userBlocks.length - 1; i >= 0; i--) {
    const rect = userBlocks[i].getBoundingClientRect();
    const elTop = rect.top + window.scrollY;
    if (elTop <= scrollY) return i;
  }
  return 0;
}

document.getElementById('tb-prev-user')?.addEventListener('click', () => {
  initUserNav();
  if (userBlocks.length === 0) return;
  const cur = findCurrentUserIdx();
  scrollToUser(cur > 0 ? cur - 1 : 0);
});

document.getElementById('tb-next-user')?.addEventListener('click', () => {
  initUserNav();
  if (userBlocks.length === 0) return;
  const cur = findCurrentUserIdx();
  scrollToUser(cur < userBlocks.length - 1 ? cur + 1 : userBlocks.length - 1);
});

document.getElementById('tb-top')?.addEventListener('click', () => {
  window.scrollTo({ top: 0, behavior: 'smooth' });
});

document.getElementById('tb-bottom')?.addEventListener('click', () => {
  window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
});

document.getElementById('tb-refresh')?.addEventListener('click', () => {
  location.reload();
});

document.addEventListener('click', () => {
  document.getElementById('toolbar-export-menu')?.classList.remove('show');
  document.getElementById('info-panel')?.classList.remove('show');
});

function toggleThinkingBlocks() {
  const show = document.getElementById('show-thinking')?.checked;
  document.querySelectorAll('.block-thinking').forEach(el => { el.open = show; });
}
function toggleToolBlocks() {
  const show = document.getElementById('show-tools')?.checked;
  document.querySelectorAll('.block-tool').forEach(el => { el.open = show; });
}

// Update toolbar button states
function updateToolbarState() {
  document.getElementById('tb-thinking')?.classList.toggle('active', document.getElementById('show-thinking')?.checked);
  document.getElementById('tb-tools')?.classList.toggle('active', document.getElementById('show-tools')?.checked);
}
updateToolbarState();

// Session search
const sessionSearch = document.getElementById('session-search');
const searchInput = document.getElementById('search-input');
const searchInfo = document.getElementById('search-info');
const filterUser = document.getElementById('filter-user');
const filterResponse = document.getElementById('filter-response');
const filterTools = document.getElementById('filter-tools');
const filterAgents = document.getElementById('filter-agents');
const filterThinking = document.getElementById('filter-thinking');
let searchMatches = [];
let searchIdx = -1;

function openSearch() {
  sessionSearch?.classList.add('show');
  searchInput?.focus();
  searchInput?.select();
}

function closeSearch() {
  sessionSearch?.classList.remove('show');
  clearHighlights();
  searchMatches = [];
  searchIdx = -1;
  if (searchInfo) searchInfo.textContent = '';
  if (searchInput) searchInput.value = '';
}

function clearHighlights() {
  document.querySelectorAll('.search-match, .search-current').forEach(el => {
    el.classList.remove('search-match', 'search-current');
  });
}

// Search scoring: returns score (0 = no match, higher = better)
function searchScore(text, query) {
  text = text.toLowerCase();
  query = query.trim().toLowerCase();

  const words = query.split(/\s+/).filter(w => w.length > 0);

  if (words.length === 0) return 0;

  // Multi-word: ALL words must appear as substrings
  if (words.length > 1) {
    let score = 0;
    for (const word of words) {
      const idx = text.indexOf(word);
      if (idx === -1) return 0; // word not found, no match
      // Score: earlier match = better, word boundary = bonus
      score += 10;
      if (idx < 100) score += (100 - idx) / 20;
      if (idx === 0 || /\W/.test(text[idx-1])) score += 5; // word boundary
    }
    return score;
  }

  // Single word: substring match (exact), position-based scoring
  const word = words[0];
  const idx = text.indexOf(word);
  if (idx === -1) return 0;

  let score = 10 + word.length; // longer match = better
  if (idx < 100) score += (100 - idx) / 10; // earlier = better
  if (idx === 0 || /\W/.test(text[idx-1])) score += 10; // word boundary bonus

  return score;
}

function doSearch(query) {
  clearHighlights();
  searchMatches = [];
  searchIdx = -1;

  if (!query || query.trim().length < 2) {
    if (searchInfo) searchInfo.textContent = '';
    return;
  }

  const results = [];
  const q = query.trim().toLowerCase();

  // Check which filters are active
  const showUser = filterUser?.checked;
  const showResponse = filterResponse?.checked;
  const showTools = filterTools?.checked;
  const showAgents = filterAgents?.checked;
  const showThinking = filterThinking?.checked;

  if (showUser) {
    // Search user messages
    document.querySelectorAll('.turn-user, details.turn-user').forEach(msg => {
      const text = msg.textContent;
      const score = searchScore(text, query);
      if (score > 0) {
        results.push({ el: msg, score: score + 50, type: 'user' });
      }
    });
  }

  if (showResponse) {
    // Search assistant response text (excluding thinking blocks)
    document.querySelectorAll('.turn:not(.turn-user)').forEach(msg => {
      // Get text excluding thinking blocks
      const clone = msg.cloneNode(true);
      clone.querySelectorAll('.block-thinking').forEach(t => t.remove());
      const text = clone.textContent;
      const score = searchScore(text, query);
      if (score > 0) {
        results.push({ el: msg, score: score, type: 'response' });
      }
    });
  }

  if (showThinking) {
    // Search thinking blocks
    document.querySelectorAll('.block-thinking').forEach(block => {
      const text = block.textContent;
      const score = searchScore(text, query);
      if (score > 0) {
        results.push({ el: block, score: score, type: 'thinking' });
      }
    });
  }

  if (showTools) {
    // Search tool names and inputs
    document.querySelectorAll('.block-tool').forEach(tool => {
      const summary = tool.querySelector('summary');
      if (!summary) return;
      const text = summary.textContent;
      if (text.toLowerCase().includes(q)) {
        results.push({ el: tool, score: 20, type: 'tool' });
      }
    });
  }

  if (showAgents) {
    // Search Task tool for subagent_type
    document.querySelectorAll('.block-tool').forEach(tool => {
      const summary = tool.querySelector('summary');
      if (!summary) return;
      const text = summary.textContent;
      if (text.includes('Task') && text.includes('[')) {
        const match = text.match(/\[([^\]]+)\]/);
        if (match && match[1].toLowerCase().includes(q)) {
          results.push({ el: tool, score: 25, type: 'agent' });
        }
      }
    });
  }

  // Sort by score (highest first)
  results.sort((a, b) => b.score - a.score);

  // Apply highlights
  results.forEach(r => {
    searchMatches.push(r.el);
    r.el.classList.add('search-match');
  });

  if (searchMatches.length > 0) {
    searchIdx = 0;
    highlightCurrent();
  }

  updateSearchInfo();
}

function expandDetails(el) {
  if (el && el.tagName === 'DETAILS') {
    el.open = true;
    el.setAttribute('open', '');
  }
}

function unfoldThread(el) {
  // Unfold any parent .thread that's folded
  const thread = el.closest('.thread.folded');
  if (thread) {
    thread.classList.remove('folded');
  }
}

function highlightCurrent() {
  document.querySelectorAll('.search-current').forEach(el => el.classList.remove('search-current'));
  if (searchIdx >= 0 && searchIdx < searchMatches.length) {
    const el = searchMatches[searchIdx];
    el.classList.add('search-current');

    // Unfold any folded thread containing this element
    unfoldThread(el);

    // Open this element if it's details
    expandDetails(el);

    // Open all nested details within the matched element
    el.querySelectorAll('details').forEach(expandDetails);

    // Open all ancestor details elements
    let node = el.parentElement;
    while (node) {
      expandDetails(node);
      // Also unfold threads as we go up
      if (node.classList?.contains('thread') && node.classList?.contains('folded')) {
        node.classList.remove('folded');
      }
      node = node.parentElement;
    }

    // Also check closest details (in case el is inside one)
    const closestDetails = el.closest('details');
    if (closestDetails) {
      expandDetails(closestDetails);
      let ancestor = closestDetails.parentElement;
      while (ancestor) {
        expandDetails(ancestor);
        ancestor = ancestor.parentElement;
      }
    }

    // Scroll after opening (delay for DOM update)
    setTimeout(() => {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 100);
  }
}

function updateSearchInfo() {
  if (!searchInfo) return;
  if (searchMatches.length === 0) {
    // If progressive loading hid some history, let the user reload into full history and keep searching
    const hiddenLoader = document.getElementById('load-earlier');
    const hasQuery = searchInput && searchInput.value && searchInput.value.trim().length >= 2;
    if (hiddenLoader && hasQuery) {
      searchInfo.innerHTML = 'No matches in visible range — <a href="#" id="search-load-all" class="search-load-all-link">search full history</a>';
      const link = document.getElementById('search-load-all');
      if (link) {
        link.addEventListener('click', (e) => {
          e.preventDefault();
          loadEarlierMessages();
        });
      }
    } else {
      searchInfo.textContent = 'No matches';
    }
  } else {
    searchInfo.textContent = (searchIdx + 1) + '/' + searchMatches.length;
  }
}

function nextMatch() {
  if (searchMatches.length === 0) return;
  searchIdx = (searchIdx + 1) %% searchMatches.length;
  highlightCurrent();
  updateSearchInfo();
}

function prevMatch() {
  if (searchMatches.length === 0) return;
  searchIdx = (searchIdx - 1 + searchMatches.length) %% searchMatches.length;
  highlightCurrent();
  updateSearchInfo();
}

// Event listeners
document.getElementById('tb-search')?.addEventListener('click', () => {
  sessionSearch?.classList.contains('show') ? closeSearch() : openSearch();
});
document.getElementById('search-close')?.addEventListener('click', closeSearch);
document.getElementById('search-prev')?.addEventListener('click', prevMatch);
document.getElementById('search-next')?.addEventListener('click', nextMatch);

searchInput?.addEventListener('input', (e) => {
  doSearch(e.target.value);
});

// Re-search when filters change
[filterUser, filterResponse, filterTools, filterAgents, filterThinking].forEach(cb => {
  cb?.addEventListener('change', () => doSearch(searchInput?.value || ''));
});

searchInput?.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') {
    e.shiftKey ? prevMatch() : nextMatch();
    e.preventDefault();
  }
  if (e.key === 'Escape') {
    closeSearch();
    e.preventDefault();
  }
});

// Restore pending search after a "search full history" reload
try {
  const pending = sessionStorage.getItem('ccx-pending-search');
  if (pending && searchInput) {
    sessionStorage.removeItem('ccx-pending-search');
    searchInput.value = pending;
    openSearch();
    doSearch(pending);
  }
} catch (_) {}

// Global keyboard shortcuts for search
document.addEventListener('keydown', function(e) {
  if (sessionSearch?.classList.contains('show')) {
    // Search is open
    if (e.key === 'n' && !e.target.matches('input, textarea')) {
      e.shiftKey ? prevMatch() : nextMatch();
      e.preventDefault();
    }
  } else {
    // Search is closed - open with / or f
    if ((e.key === '/' || e.key === 'f') && !e.target.matches('input, textarea')) {
      e.preventDefault();
      openSearch();
    }
  }
});
</script>
<style>
@keyframes flash {
  0%%, 100%% { background: transparent; box-shadow: none; }
  25%%, 75%% { background: rgba(218, 119, 86, 0.15); box-shadow: 0 0 0 2px var(--primary); }
}
</style>
`, projectName, sessionID)
}
