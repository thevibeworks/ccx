package web

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

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
		b.WriteString(fmt.Sprintf(`
<a href="/project/%s" class="card project-card">
	<div class="card-header">
		<span class="card-title">%s</span>
	</div>
	<div class="card-stats">
		<span class="stat">◉ %d %s</span>
		<span class="stat-sep">•</span>
		<span class="stat">%s</span>
	</div>
</a>`, html.EscapeString(p.EncodedName), html.EscapeString(displayName), len(p.Sessions), sessionsLabel, formatAge(p.LastModified)))
	}
	b.WriteString(`</div>`)

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(indexJS())
	b.WriteString(pageFooter())

	return b.String()
}

func renderProjectPage(project *parser.Project, sessions []*parser.Session, allProjects []*parser.Project, search, sortBy string) string {
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
		b.WriteString(fmt.Sprintf(`<a href="/project/%s" class="panel-item%s" title="%s">%s</a>`,
			html.EscapeString(p.EncodedName), active, html.EscapeString(displayName), html.EscapeString(truncate(displayName, 24))))
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

	b.WriteString(`<div class="controls">`)
	b.WriteString(`<div class="search-wrap">`)
	b.WriteString(fmt.Sprintf(`<input type="text" id="search" class="search-input" placeholder="Search sessions... (press /)" value="%s">`, html.EscapeString(search)))
	b.WriteString(`<span class="search-spinner" id="search-spinner"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="sort-controls">`)
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
		b.WriteString(fmt.Sprintf(`
<a href="/session/%s/%s" class="card session-card">
	<div class="session-header">
		<code class="session-id">%s</code>
		<span class="session-time" title="%s">%s</span>
	</div>
	<div class="session-summary">%s</div>
	<div class="session-stats">
		<span class="stat"><span class="stat-icon">M</span> %d</span>
		<span class="stat"><span class="stat-icon">T</span> %d</span>
		%s
	</div>
</a>`, html.EscapeString(project.EncodedName), html.EscapeString(s.ID),
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
	b.WriteString(pageFooter())

	return b.String()
}

func renderSessionPage(session *parser.Session, projectName string, allSessions []*parser.Session, showThinking, showTools, loadAll bool, theme string) string {
	var b strings.Builder

	title := fmt.Sprintf("Session %s - ccx", session.ID[:8])
	b.WriteString(pageHeader(title, theme))
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
			b.WriteString(fmt.Sprintf(`<a href="/session/%s/%s" class="panel-item%s" title="%s"><span class="panel-id">%s</span><span class="panel-summary">%s</span></a>`,
				html.EscapeString(projectName), html.EscapeString(s.ID), active,
				html.EscapeString(s.Summary), html.EscapeString(truncate(s.ID, 6)), html.EscapeString(summary)))
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
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=html">HTML</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=md">Markdown</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=org">Org</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=txt">Text</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=json">JSON</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
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
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Session</span><code class="copyable">%s</code><button class="copy-btn-sm" data-copy="%s">⧉</button></div>`,
		html.EscapeString(session.ID), html.EscapeString(session.ID)))
	if session.Slug != "" {
		b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Slug</span><code class="copyable">%s</code><button class="copy-btn-sm" data-copy="%s">⧉</button></div>`,
			html.EscapeString(session.Slug), html.EscapeString(session.Slug)))
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
		b.WriteString(fmt.Sprintf(`<div class="info-row" title="Tokens generated by Claude"><span class="info-label">Output</span><span class="info-value">%s</span></div>`, formatTokens(session.Stats.OutputTokens)))
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
		b.WriteString(`</div>`)
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

	// Check if progressive loading is needed (unless loadAll is requested)
	if !loadAll && len(allMsgs) > progressiveLoadThreshold {
		renderMessagesProgressive(b, allMsgs, showThinking, showTools)
		return
	}

	// Build tool results map for inline rendering
	toolResults := buildToolResultsMap(allMsgs)

	// Group messages into threads anchored by USER prompts
	var currentThread []*parser.Message
	inThread := false

	for _, msg := range allMsgs {
		// Skip standalone tool_result messages - they'll be rendered inline
		if msg.Kind == parser.KindToolResult {
			continue
		}

		isAnchor := msg.Kind == parser.KindUserPrompt || msg.Kind == parser.KindCommand

		if isAnchor {
			// Close previous thread if any
			if inThread && len(currentThread) > 0 {
				renderThread(b, currentThread, showThinking, showTools, toolResults)
			}
			// Start new thread
			currentThread = []*parser.Message{msg}
			inThread = true
		} else if inThread {
			currentThread = append(currentThread, msg)
		} else {
			// Messages before first anchor - render directly
			renderTurnMessage(b, msg, showThinking, showTools, 0, toolResults)
		}
	}

	// Close final thread
	if inThread && len(currentThread) > 0 {
		renderThread(b, currentThread, showThinking, showTools, toolResults)
	}
}

// renderMessagesProgressive renders large conversations with lazy loading
func renderMessagesProgressive(b *strings.Builder, allMsgs []*parser.Message, showThinking, showTools bool) {
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
				renderThread(b, currentThread, showThinking, showTools, toolResults)
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
		renderThread(b, currentThread, showThinking, showTools, toolResults)
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

// renderThread renders a conversation thread anchored by a USER message
func renderThread(b *strings.Builder, thread []*parser.Message, showThinking, showTools bool, toolResults map[string]parser.ContentBlock) {
	if len(thread) == 0 {
		return
	}

	anchor := thread[0]
	responses := thread[1:]

	// Thread container with visual line
	b.WriteString(`<div class="thread">`)

	// Render anchor (USER prompt or Command)
	b.WriteString(`<div class="thread-anchor">`)
	renderTurnMessage(b, anchor, showThinking, showTools, 0, toolResults)
	b.WriteString(`</div>`)

	// Render responses with indent
	if len(responses) > 0 {
		b.WriteString(`<div class="thread-responses">`)
		for _, msg := range responses {
			level := 1
			if msg.IsSidechain {
				level = 2
			}
			renderTurnMessage(b, msg, showThinking, showTools, level, toolResults)
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
	m, ok := input.(map[string]any)
	if !ok {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		b.WriteString(fmt.Sprintf(`<pre class="tool-input">%s</pre>`, html.EscapeString(string(inputJSON))))
		return
	}

	switch toolName {
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

// renderConversationNav renders a collapsible tree navigation
func renderConversationNav(b *strings.Builder, messages []*parser.Message) {
	allMsgs := flattenMessages(messages)

	// Group messages: each user prompt starts a new group
	type navGroup struct {
		user     *parser.Message
		children []*parser.Message
	}
	var groups []navGroup
	var currentGroup *navGroup

	for _, msg := range allMsgs {
		switch msg.Kind {
		case parser.KindUserPrompt, parser.KindCommand, parser.KindCompactSummary:
			// Start new group
			if currentGroup != nil {
				groups = append(groups, *currentGroup)
			}
			currentGroup = &navGroup{user: msg}
		default:
			// Add to current group
			if currentGroup != nil {
				currentGroup.children = append(currentGroup.children, msg)
			}
		}
	}
	if currentGroup != nil {
		groups = append(groups, *currentGroup)
	}

	// Render groups as collapsible tree
	for i, g := range groups {
		isLast := i >= len(groups)-3 // Expand last 3 groups

		// User/command/compact header
		switch g.user.Kind {
		case parser.KindCompactSummary:
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-compact" data-msg="%s">`,
				sanitizeID(g.user.UUID), html.EscapeString(sanitizeID(g.user.UUID))))
			b.WriteString(`<span class="nav-icon">◇</span><span class="nav-text">COMPACT</span></a>`)

		case parser.KindCommand:
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-command" data-msg="%s">`,
				sanitizeID(g.user.UUID), html.EscapeString(sanitizeID(g.user.UUID))))
			b.WriteString(fmt.Sprintf(`<span class="nav-icon">⌘</span><span class="nav-text">%s</span></a>`,
				html.EscapeString(g.user.CommandName)))

		case parser.KindUserPrompt:
			preview := getNavPreview(g.user)
			childCount := len(g.children)

			if childCount == 0 {
				// No children - simple link
				b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-user" data-msg="%s">`,
					sanitizeID(g.user.UUID), html.EscapeString(sanitizeID(g.user.UUID))))
				b.WriteString(fmt.Sprintf(`<span class="nav-icon">▶</span><span class="nav-text">%s</span></a>`,
					html.EscapeString(preview)))
			} else {
				// Has children - collapsible
				openAttr := ""
				if isLast {
					openAttr = " open"
				}
				b.WriteString(fmt.Sprintf(`<details class="nav-group"%s>`, openAttr))
				b.WriteString(fmt.Sprintf(`<summary class="nav-item nav-user" data-msg="%s">`, html.EscapeString(sanitizeID(g.user.UUID))))
				b.WriteString(fmt.Sprintf(`<span class="nav-icon">▶</span><span class="nav-text">%s</span>`,
					html.EscapeString(preview)))
				b.WriteString(fmt.Sprintf(`<span class="nav-count">%d</span>`, childCount))
				b.WriteString(`</summary>`)
				b.WriteString(`<div class="nav-children">`)

				// Render children (limit to avoid bloat)
				maxChildren := 10
				for j, child := range g.children {
					if j >= maxChildren {
						b.WriteString(fmt.Sprintf(`<span class="nav-more">+%d more</span>`, len(g.children)-maxChildren))
						break
					}
					renderNavChild(b, child)
				}
				b.WriteString(`</div></details>`)
			}
		}
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
    resultsDiv.innerHTML = '<p class="search-hint">Type to search across all projects and sessions...</p>';
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
    const url = r.type === 'project' ? '/project/' + r.project_encoded :
                '/session/' + r.project_encoded + '/' + r.session_id;
    html += '<a href="' + url + '" class="search-result">';
    html += badge;
    html += '<div class="result-body">';
    html += '<div class="result-title">' + escapeHtml(r.title || r.summary || 'Untitled') + '</div>';
    html += '<div class="result-meta">' + escapeHtml(r.project_name || '') + (r.time ? ' · ' + escapeHtml(r.time) : '') + '</div>';
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

func renderSettingsPage(settings *Settings, config *GlobalConfig, agents []AgentInfo, skills []SkillInfo) string {
	var b strings.Builder

	b.WriteString(pageHeader("Settings - ccx", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("settings"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<h1>Claude Code Settings</h1>`)

	// Global Configuration
	if config != nil {
		b.WriteString(`<section class="settings-section">`)
		b.WriteString(`<h2><span class="section-icon">●</span> Configuration</h2>`)
		b.WriteString(`<table class="settings-table">`)
		b.WriteString(fmt.Sprintf(`<tr><td>Theme</td><td><code>%s</code></td></tr>`, html.EscapeString(config.Theme)))
		b.WriteString(fmt.Sprintf(`<tr><td>Editor Mode</td><td><code>%s</code></td></tr>`, html.EscapeString(config.EditorMode)))
		b.WriteString(fmt.Sprintf(`<tr><td>Verbose</td><td><code>%v</code></td></tr>`, config.Verbose))
		b.WriteString(fmt.Sprintf(`<tr><td>Total Startups</td><td><code>%d</code></td></tr>`, config.NumStartups))
		b.WriteString(`</table>`)
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

func renderTopNav(projectName, sessionID string) string {
	var b strings.Builder
	b.WriteString(`<header class="top-nav">`)
	b.WriteString(`<div class="top-nav-inner">`)
	b.WriteString(`<div class="nav-left">`)
	b.WriteString(`<a href="/" class="brand"><span class="brand-cc">cc</span><span class="brand-x">x</span></a>`)
	b.WriteString(`<span class="brand-sub">for Claude Code</span>`)
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
		<span class="footer-brand"><span class="brand-cc">cc</span><span class="brand-x">x</span></span>
		<span class="footer-sep">·</span>
		<span class="footer-text">by thevibeworks</span>
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

func faviconLink() string {
	// Bold favicon: cc in white, x in coral
	return `<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='4' fill='%23111'/%3E%3Ctext x='3' y='23' font-family='ui-monospace,monospace' font-weight='800' font-size='14'%3E%3Ctspan fill='%23fff'%3Ecc%3C/tspan%3E%3Ctspan fill='%23da7756'%3Ex%3C/tspan%3E%3C/text%3E%3C/svg%3E">`
}

func pageFooter() string {
	return `
<div id="loading-overlay" class="loading-overlay">
  <div class="cli-spinner">
    <span class="cli-spinner-char"></span>
    <span>Loading...</span>
  </div>
</div>
<script>
// Global loading overlay control
const loadingOverlay = document.getElementById('loading-overlay');
window.showLoading = function() { loadingOverlay?.classList.add('active'); };
window.hideLoading = function() { loadingOverlay?.classList.remove('active'); };

// Show loading on navigation (skip downloads/API links)
document.querySelectorAll('a[href^="/"]').forEach(a => {
  a.addEventListener('click', function(e) {
    const href = this.getAttribute('href') || '';
    // Skip: modifier keys, API/export links, anchor links
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
	return `
:root {
  --bg: #ffffff;
  --bg-secondary: #f8f9fa;
  --bg-tertiary: #e9ecef;
  --text: #212529;
  --text-muted: #6c757d;
  --border: #dee2e6;
  --primary: #da7756;
  --primary-hover: #c5634a;
  /* Context accent colors */
  --accent-project: #3b82f6;
  --accent-session: #8b5cf6;
  --accent-conversation: #06b6d4;
  --user-bg: #fff8f5;
  --user-border: #da7756;
  --assistant-bg: #f5faf5;
  --assistant-border: #5a9;
  --tool-bg: #fffbf0;
  --tool-border: #d97;
  --error-bg: #fff0f0;
  --error-border: #d55;
  --compacted-bg: #fffde0;
  --compacted-border: #da0;
  --shadow: 0 1px 3px rgba(0,0,0,0.08);
  --radius: 6px;
  --font-mono: 'Courier New', Courier, 'SF Mono', 'Consolas', 'Liberation Mono', 'Menlo', monospace;
  --font-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
}

[data-theme="dark"] {
  --bg: #1a1a1f;
  --bg-secondary: #222228;
  --bg-tertiary: #2a2a32;
  --text: #e8e8e8;
  --text-muted: #888;
  --border: #3a3a42;
  --accent-project: #60a5fa;
  --accent-session: #a78bfa;
  --accent-conversation: #22d3ee;
  --user-bg: #2a2520;
  --assistant-bg: #202820;
  --tool-bg: #282520;
  --error-bg: #2a2020;
  --compacted-bg: #282820;
}

* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: var(--font-sans);
  background: var(--bg);
  color: var(--text);
  line-height: 1.7;
  font-size: 17px;
}

code, pre, .session-id, .model-badge {
  font-family: var(--font-mono);
}

.top-nav {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 48px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.top-nav-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  max-width: 1200px;
  padding: 0 24px;
}

.nav-left, .nav-right { display: flex; align-items: center; gap: 16px; }
.nav-center { flex: 1; display: flex; justify-content: center; max-width: 400px; margin: 0 24px; }

.global-search {
  width: 100%;
  position: relative;
}
.global-search-input {
  width: 100%;
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text);
  font-size: 14px;
}
.global-search-input:focus { outline: none; border-color: var(--primary); }
.search-results {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  max-height: 400px;
  overflow-y: auto;
  z-index: 200;
  display: none;
  box-shadow: 0 4px 12px rgba(0,0,0,0.15);
}
.search-results.active { display: block; }
.search-result {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border);
  text-decoration: none;
  color: var(--text);
}
.search-result:last-child { border-bottom: none; }
.search-result:hover { background: var(--bg-secondary); }
.search-result .result-body { flex: 1; min-width: 0; }
.search-result .result-title { font-weight: 500; font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.search-result .result-meta { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.search-result .result-snippet { font-size: 12px; color: var(--text-muted); margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.search-result .result-snippet mark { background: #ffe066; color: #000; padding: 0 2px; }
.result-badge {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  font-family: var(--font-mono);
  border-radius: 4px;
  color: white;
}
.badge-project { background: var(--accent-project); }
.badge-session { background: var(--accent-session); }
.badge-message { background: var(--accent-conversation); }
.search-loading, .search-empty {
  padding: 16px;
  color: var(--text-muted);
  font-size: 13px;
  text-align: center;
}
.search-loading { display: flex; align-items: center; justify-content: center; gap: 8px; }

.brand {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-weight: 800;
  font-size: 24px;
  text-decoration: none;
  display: flex;
  align-items: baseline;
  letter-spacing: -1px;
}
.brand-cc { color: var(--text); }
.brand-x { color: #da7756; }
.brand-sub {
  font-size: 13px;
  font-weight: 400;
  color: var(--text-muted);
  margin-left: 10px;
}

.nav-link {
  color: var(--text-muted);
  text-decoration: none;
  font-size: 13px;
}
.nav-link:hover { color: var(--text); }

.icon-btn {
  background: none;
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 4px 8px;
  cursor: pointer;
  color: var(--text-muted);
  font-family: var(--font-mono);
  font-size: 12px;
  text-decoration: none;
}
.icon-btn:hover { color: var(--text); border-color: var(--text-muted); }

.site-footer {
  padding: 16px 24px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border);
  text-align: center;
}
.footer-inner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-muted);
}
.footer-brand { font-weight: 700; font-size: 14px; }
.footer-sep { opacity: 0.5; }
.footer-text { font-size: 12px; }

/* Two-panel navigation */
.panel-nav {
  width: 170px;
  min-width: 170px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  height: calc(100vh - 48px);
  position: sticky;
  top: 48px;
}
.panel-header {
  padding: 12px 16px;
  font-weight: 600;
  font-size: 13px;
  border-bottom: 1px solid var(--border);
  color: var(--text-muted);
}
.panel-header a {
  color: var(--text-muted);
  text-decoration: none;
}
.panel-header a:hover { color: var(--text); }
.panel-nav:not(.session-nav) .panel-header a:hover { color: var(--accent-project); }
.session-nav .panel-header a:hover { color: var(--accent-session); }
.panel-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}
.panel-item {
  display: block;
  padding: 8px 16px;
  font-size: 13px;
  color: var(--text);
  text-decoration: none;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-left: 3px solid transparent;
}
.panel-item:hover {
  background: var(--bg-tertiary);
}
.panel-item.active {
  background: var(--bg-tertiary);
  border-left-color: var(--primary);
  font-weight: 500;
}
/* Context-specific panel accents */
.panel-nav:not(.session-nav) .panel-item.active { border-left-color: var(--accent-project); }
.session-nav .panel-item.active { border-left-color: var(--accent-session); }
.session-nav .panel-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.panel-id {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
}
.panel-summary {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.two-panel {
  display: flex;
}
.two-panel .main-content {
  flex: 1;
  min-width: 0;
}

.layout {
  display: flex;
  justify-content: center;
  min-height: 100vh;
  padding-top: 48px;
  max-width: 1200px;
  margin: 0 auto;
  position: relative;
}

.sidebar {
  width: 140px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  padding: 16px 8px;
  position: fixed;
  left: max(0px, calc((100vw - 1200px) / 2));
  top: 48px;
  height: calc(100vh - 48px);
  overflow-y: auto;
}

.sidebar-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sidebar-link {
  display: block;
  padding: 8px 12px;
  color: var(--text-muted);
  text-decoration: none;
  border-radius: 4px;
  font-size: 13px;
}
.sidebar-link:hover { background: var(--bg-tertiary); color: var(--text); }
.sidebar-link.active { background: var(--primary); color: white; }

.main-content {
  width: 100%;
  max-width: 800px;
  margin-left: 140px;
  padding: 24px 40px;
}

/* Bottom dock toolbar - modern horizontal bar */
.dock-toolbar {
  position: fixed;
  bottom: 52px;
  left: calc(50% + 140px);
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0,0,0,0.15);
  z-index: 100;
  backdrop-filter: blur(10px);
  max-width: calc(100vw - 280px);
}
.dock-group { display: flex; align-items: center; gap: 2px; }
.dock-sep { width: 1px; height: 24px; background: var(--border); margin: 0 6px; }
.dock-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px 10px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 12px;
  font-family: var(--font-sans);
  transition: all 0.15s;
  line-height: 1;
  vertical-align: middle;
}
.dock-btn:hover { background: var(--bg-tertiary); color: var(--text); }
.dock-btn.active, .dock-btn.toggle.active { background: var(--primary); color: white; }
.dock-icon { font-size: 14px; line-height: 1; display: inline-flex; align-items: center; }
.dock-icon svg { vertical-align: middle; }
.dock-label { font-size: 11px; font-weight: 500; line-height: 1; }
.dock-key { font-size: 9px; opacity: 0.5; font-family: var(--font-mono); line-height: 1; }
.dock-btn:hover .dock-key { opacity: 0.8; }

/* Live button pulse animation - animate the ◉ icon directly */
.live-btn.active { background: rgba(40,80,40,0.9); }
.live-btn.active .dock-icon {
  color: #4f4;
  animation: live-pulse 1.5s infinite;
}
.live-btn.active .dock-label { color: #8f8; }
@keyframes live-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.6; transform: scale(1.15); }
}

/* Export dropdown */
.dock-dropdown { position: relative; }
.dock-menu {
  display: none;
  position: absolute;
  bottom: 100%;
  left: 50%;
  transform: translateX(-50%);
  margin-bottom: 8px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0,0,0,0.15);
  min-width: 100px;
  overflow: hidden;
}
.dock-menu.show { display: block; }
.dock-menu a {
  display: block;
  padding: 8px 14px;
  color: var(--text);
  text-decoration: none;
  font-size: 12px;
  transition: background 0.1s;
}
.dock-menu a:hover { background: var(--bg-secondary); }

/* Responsive: hide labels on small screens */
@media (max-width: 768px) {
  .dock-label { display: none; }
  .dock-btn { padding: 8px; }
}

.page-header { margin-bottom: 20px; position: relative; }
.page-header h1 { font-size: 1.5rem; margin-bottom: 4px; display: inline; }
.page-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  font-size: 12px;
  font-weight: 700;
  font-family: var(--font-mono);
  border-radius: 4px;
  color: white;
  margin-right: 10px;
  vertical-align: middle;
}
.page-header-projects { border-left: 3px solid var(--accent-project); padding-left: 16px; }
.page-header-sessions { border-left: 3px solid var(--accent-session); padding-left: 16px; }

.breadcrumb {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 8px;
}
.breadcrumb a { color: var(--primary); text-decoration: none; }
.breadcrumb a:hover { text-decoration: underline; }
.breadcrumb .sep { margin: 0 6px; color: var(--border); }
.breadcrumb .current { color: var(--text); }

.stats { color: var(--text-muted); font-size: 12px; }

.controls {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
  align-items: center;
}

.search-wrap {
  flex: 1;
  position: relative;
  max-width: 400px;
}

.search-input {
  width: 100%;
  padding: 8px 12px;
  padding-right: 32px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text);
  font-size: 13px;
}
.search-input:focus { outline: none; border-color: var(--primary); }

.search-spinner {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  width: 14px;
  height: 14px;
  border: 2px solid var(--border);
  border-top-color: var(--primary);
  border-radius: 50%;
  display: none;
}
.search-spinner.loading {
  display: block;
  animation: spin 0.6s linear infinite;
}
@keyframes spin { to { transform: translateY(-50%) rotate(360deg); } }

/* CLI-style loading spinner */
.cli-spinner {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--primary);
  font-family: var(--font-mono);
  font-size: 14px;
}
.cli-spinner-char {
  display: inline-block;
  width: 16px;
  text-align: center;
  animation: cli-spin 0.72s steps(6) infinite;
}
@keyframes cli-spin {
  0% { content: '·'; }
  16% { content: '✢'; }
  33% { content: '✳'; }
  50% { content: '✶'; }
  66% { content: '✻'; }
  83% { content: '✽'; }
}
.cli-spinner-char::before {
  content: '·';
  animation: cli-frames 0.72s steps(1) infinite;
}
@keyframes cli-frames {
  0% { content: '·'; }
  14.3% { content: '✢'; }
  28.6% { content: '✳'; }
  42.9% { content: '✶'; }
  57.1% { content: '✻'; }
  71.4% { content: '✽'; }
  85.7% { content: '✻'; }
  100% { content: '·'; }
}
.loading-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  opacity: 0;
  visibility: hidden;
  transition: opacity 0.15s, visibility 0.15s;
}
[data-theme="dark"] .loading-overlay { background: rgba(26, 26, 31, 0.9); }
.loading-overlay.active { opacity: 1; visibility: visible; }

.sort-controls { display: flex; align-items: center; gap: 6px; }
.sort-label { font-size: 12px; color: var(--text-muted); }
.sort-select {
  padding: 6px 8px;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--bg);
  color: var(--text);
  font-size: 12px;
  cursor: pointer;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}

.card {
  display: block;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 14px;
  text-decoration: none;
  color: inherit;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.card:hover { border-color: var(--primary); box-shadow: var(--shadow); }

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 6px;
}

.card-title { font-weight: 600; font-size: 14px; word-break: break-word; }
.card-badge {
  background: var(--primary);
  color: white;
  padding: 2px 6px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
}
.card-meta { color: var(--text-muted); font-size: 12px; }
.card-stats {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-muted);
  font-size: 13px;
}
.card-stats .stat { display: flex; align-items: center; gap: 4px; }
.card-stats .stat-sep { opacity: 0.4; }

.session-list { display: flex; flex-direction: column; gap: 8px; }

.session-card { border-left: 3px solid var(--accent-session); }

.session-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.session-id {
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 11px;
}
.session-time { color: var(--text-muted); font-size: 11px; }
.session-summary { font-size: 13px; margin-bottom: 6px; }
.session-stats {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: var(--text-muted);
}
.session-stats .stat-tokens { color: var(--accent-conversation); }
.stat { display: flex; align-items: center; gap: 3px; }
.stat-icon { font-weight: 600; }

/* Session search bar (floating above toolbar) */
.session-search {
  position: fixed;
  bottom: 90px;
  left: calc(50% + 140px);
  transform: translateX(-50%);
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0,0,0,0.15);
  padding: 12px 16px;
  z-index: 300;
  display: none;
  flex-direction: column;
  gap: 10px;
  min-width: 400px;
}
.session-search.show { display: flex; }
.search-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.session-search input[type="text"] {
  border: none;
  background: var(--bg-secondary);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 14px;
  flex: 1;
  color: var(--text);
}
.session-search input[type="text"]:focus { outline: 2px solid var(--primary); }
.session-search input[type="text"]::placeholder { color: var(--text-muted); }
.search-info {
  font-size: 12px;
  color: var(--text-muted);
  min-width: 50px;
  text-align: center;
}
.search-nav {
  background: none;
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 5px 8px;
  cursor: pointer;
  color: var(--text-muted);
  font-size: 13px;
}
.search-nav:hover { background: var(--bg-secondary); color: var(--text); }
.search-close {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-muted);
  font-size: 18px;
  padding: 2px 6px;
}
.search-close:hover { color: var(--text); }

/* Search filter chips */
.search-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.search-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: 12px;
  font-size: 11px;
  cursor: pointer;
  color: var(--text-muted);
  transition: all 0.15s;
}
.search-chip:hover { border-color: var(--text-muted); }
.search-chip input { display: none; }
.search-chip:has(input:checked) {
  background: var(--primary);
  border-color: var(--primary);
  color: white;
}

/* Search highlight */
.search-match { background: rgba(255,220,0,0.3); }
.search-current { background: rgba(255,180,0,0.5); outline: 2px solid var(--primary); }

/* Info panel (floating above info icon) */
.info-panel {
  position: fixed;
  bottom: 90px;
  right: 40px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-left: 3px solid var(--accent-conversation);
  border-radius: 8px;
  box-shadow: 0 8px 32px rgba(0,0,0,0.12);
  padding: 0;
  z-index: 200;
  display: none;
  min-width: 260px;
  max-width: 320px;
  overflow: hidden;
}
.info-panel.show { display: block; }
.info-section {
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
}
.info-section:last-child { border-bottom: none; }
.info-section-header {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  margin-bottom: 8px;
}
.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
  gap: 12px;
}
.info-value { text-align: right; color: var(--text); font-weight: 500; }

/* Progressive loading - Load earlier button */
.load-earlier {
  text-align: center;
  padding: 16px;
  margin-bottom: 24px;
  border-bottom: 1px dashed var(--border);
}
.load-earlier-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-muted);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s;
}
.load-earlier-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text);
  border-color: var(--primary);
}
.load-earlier-btn .load-icon {
  font-size: 16px;
}
.load-earlier.loading .load-earlier-btn {
  opacity: 0.6;
  pointer-events: none;
}
.info-label { color: var(--text-muted); font-weight: 400; flex-shrink: 0; }
.info-row code { font-size: 10px; background: var(--bg-secondary); padding: 2px 6px; border-radius: 3px; }
.info-row a { color: var(--accent-session); text-decoration: none; font-weight: 500; }
.info-row a:hover { text-decoration: underline; }
.info-cache { font-size: 11px; }
.info-cache .info-label { font-size: 11px; }
.info-cache .info-value { color: var(--text-muted); }
.info-total { padding-top: 6px; margin-top: 4px; border-top: 1px dashed var(--border); }
.info-total .info-label { color: var(--text); }
.info-row[title] { cursor: help; }
.info-cwd { max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.copy-btn-sm {
  background: none; border: none; color: var(--text-muted);
  cursor: pointer; padding: 0 4px; font-size: 12px; opacity: 0.6;
  margin-left: 4px; vertical-align: middle;
}
.copy-btn-sm:hover { opacity: 1; color: var(--primary); }

/* Session page */
.session-layout {
  display: flex;
  max-width: 1200px;
  margin: 0 auto;
  position: relative;
}

.nav-sidebar {
  width: 220px;
  flex-shrink: 0;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  position: sticky;
  top: 48px;
  height: calc(100vh - 48px);
  overflow-y: auto;
  transition: width 0.2s;
}
.session-layout.sidebar-collapsed .nav-sidebar { width: 48px; overflow: hidden; }
.session-layout.sidebar-collapsed .nav-sidebar .nav-list { overflow: hidden; }
.session-layout.sidebar-collapsed .nav-sidebar .nav-text { display: none; }
.session-layout.sidebar-collapsed .nav-sidebar .sidebar-header h3 { display: none; }
.session-layout.sidebar-collapsed .nav-sidebar .nav-item { justify-content: center; padding: 8px 4px; }
.session-layout.sidebar-collapsed .nav-sidebar .nav-icon { margin: 0; }

.sidebar-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  background: var(--bg-secondary);
  z-index: 1;
}
.sidebar-header h3 { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; }

.nav-list { padding: 4px; }

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  color: var(--text-muted);
  text-decoration: none;
  font-size: 10px;
  border-radius: 3px;
  margin-bottom: 1px;
  overflow: hidden;
}
.nav-item:hover { background: var(--bg-tertiary); color: var(--text); }
.nav-item.active { background: var(--bg-tertiary); color: var(--text); border-left: 2px solid var(--accent-conversation); }

.nav-icon { font-size: 9px; flex-shrink: 0; width: 12px; text-align: center; }
.nav-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* Navigation item hierarchy */
.nav-compact {
  background: linear-gradient(90deg, rgba(255,180,50,0.15) 0%, transparent 100%);
  font-weight: 600;
  border-left: 3px solid #fa0;
  margin: 8px 0;
}
.nav-compact .nav-icon { color: #fa0; }

.nav-user { padding-left: 8px; font-weight: 500; }
.nav-user .nav-icon { color: var(--user-border); }

.nav-command { padding-left: 8px; }
.nav-command .nav-icon { color: #e9a; }

.nav-response { padding-left: 20px; opacity: 0.9; }
.nav-response .nav-icon { color: var(--assistant-border); }

.nav-thinking { padding-left: 20px; opacity: 0.6; }
.nav-thinking .nav-icon { color: #a80; }

.nav-tool { padding-left: 28px; opacity: 0.8; }
.nav-tool .nav-icon { color: var(--tool-border); }

.nav-meta { padding-left: 20px; opacity: 0.4; font-style: italic; }
.nav-meta .nav-icon { color: #999; }

/* Collapsible nav groups */
.nav-group { margin: 2px 0; }
.nav-group > summary { list-style: none; cursor: pointer; }
.nav-group > summary::-webkit-details-marker { display: none; }
.nav-group > summary .nav-icon { transition: transform 0.15s; }
.nav-group[open] > summary .nav-icon { transform: rotate(90deg); }
.nav-count {
  margin-left: auto;
  font-size: 9px;
  background: var(--bg-tertiary);
  padding: 1px 5px;
  border-radius: 8px;
  color: var(--text-muted);
}
.nav-children {
  padding-left: 12px;
  border-left: 1px solid var(--border);
  margin-left: 10px;
  margin-top: 2px;
}
.nav-children .nav-item { font-size: 9px; padding: 2px 6px; }
.nav-more {
  display: block;
  font-size: 9px;
  color: var(--text-muted);
  padding: 2px 6px;
  font-style: italic;
}
.nav-live-section {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}
.nav-live-label {
  font-size: 10px;
  color: var(--primary);
  font-weight: 500;
  padding: 4px 8px;
  opacity: 0.8;
}

.session-main { flex: 1; max-width: 900px; min-width: 0; margin-left: 0; padding: 24px 32px; }
.session-layout.sidebar-collapsed .nav-sidebar { width: 48px; }

.session-page-header {
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
}
.session-meta {
  display: flex;
  gap: 20px;
  color: var(--text-muted);
  font-size: 12px;
  margin-top: 8px;
}
.session-controls {
  display: flex;
  gap: 12px;
  margin-top: 12px;
  align-items: center;
  flex-wrap: wrap;
}
.toggle-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  cursor: pointer;
}

.btn {
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
  background: var(--bg);
  color: var(--text);
}
.btn:hover { border-color: var(--text-muted); }
.btn-watch { background: var(--primary); color: white; border-color: var(--primary); }
.btn-watch:hover { background: var(--primary-hover); }
.btn-watch.active { background: #d55; border-color: #d55; }

.export-dropdown { position: relative; }
.export-menu {
  position: absolute;
  top: 100%;
  left: 0;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 4px;
  box-shadow: var(--shadow);
  display: none;
  z-index: 10;
  min-width: 100px;
}
.export-menu.show { display: block; }
.export-item {
  display: block;
  padding: 8px 12px;
  color: var(--text);
  text-decoration: none;
  font-size: 12px;
}
.export-item:hover { background: var(--bg-tertiary); }

.messages { display: flex; flex-direction: column; gap: 0; padding-bottom: 60px; }

/* Thread structure - compact CLI style */
.thread {
  position: relative;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border);
}
.thread:last-child { border-bottom: none; }
.thread-anchor {
  position: relative;
}
.thread-responses {
  position: relative;
  padding-left: 12px;
  margin-left: 8px;
  border-left: 1px dotted var(--border);
}
.thread-responses::before { display: none; }

/* Thread folding - collapse middle responses (direct children only) */
.thread.folded > .thread-responses > .turn:not(:last-child) {
  max-height: 0;
  overflow: hidden;
  opacity: 0;
  margin: 0;
  padding: 0;
  transition: max-height 0.2s, opacity 0.2s, margin 0.2s;
}
.thread:not(.folded) > .thread-responses > .turn {
  max-height: none;
  opacity: 1;
  transition: opacity 0.2s;
}
/* Fold indicator in header */
.fold-indicator {
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  margin-left: 8px;
  padding: 2px 6px;
  background: var(--bg-secondary);
  border-radius: 3px;
  cursor: pointer;
}
.thread.folded .fold-indicator { background: var(--primary); color: white; }
.thread.folded .fold-indicator::after { content: '+' attr(data-hidden); }
.thread:not(.folded) .fold-indicator::after { content: '−'; }

/* Fold separator line in middle - clickable */
.fold-separator {
  display: none;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  margin: 8px 0;
  cursor: pointer;
  border-radius: 4px;
  transition: background 0.15s;
}
.fold-separator:hover { background: var(--bg-secondary); }
.thread.folded .fold-separator { display: flex; }
.fold-sep-line {
  flex: 1;
  height: 1px;
  background: var(--border);
}
.fold-sep-text {
  font-size: 11px;
  font-family: var(--font-mono);
  color: white;
  white-space: nowrap;
  padding: 4px 10px;
  background: var(--primary);
  border-radius: 12px;
}
.fold-separator:hover .fold-sep-text { background: var(--primary-hover); }

/* Level indentation */
.level-1 { margin-left: 0; }
.level-2 { margin-left: 16px; opacity: 0.95; }
.level-3 { margin-left: 32px; opacity: 0.9; }

/* Tool output styling */
.tool-output {
  padding: 4px 0;
}

/* Compact CLI-style turn design */
.turn {
  margin: 0;
  padding: 0;
  border: none;
  border-radius: 0;
  overflow: visible;
}

.turn-header {
  padding: 6px 0;
  font-size: 13px;
  display: flex;
  gap: 6px;
  align-items: center;
  background: transparent;
  border: none;
}

.turn-icon {
  font-size: 13px;
  font-family: var(--font-mono);
  flex-shrink: 0;
  width: 16px;
}
.turn-role {
  font-family: var(--font-mono);
  font-weight: 600;
  font-size: 11px;
  color: var(--text-muted);
}
.turn-time {
  color: var(--text-muted);
  font-size: 10px;
  opacity: 0.7;
}
.turn-preview {
  flex: 1;
  color: var(--text);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
details.turn-user summary { cursor: pointer; list-style: none; }
details.turn-user summary::-webkit-details-marker { display: none; }
details.turn-user[open] .turn-preview { display: none; }
details.turn-user[open] .turn-role { display: none; }
.turn-model {
  background: var(--bg-secondary);
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 9px;
  font-family: var(--font-mono);
  color: var(--text-muted);
}
.turn-actions {
  display: flex;
  gap: 4px;
  margin-left: auto;
  opacity: 0;
  transition: opacity 0.15s;
}
.turn:hover .turn-actions, details.turn-user:hover .turn-actions { opacity: 1; }
.turn-raw-btn, .turn-copy-btn {
  padding: 1px 6px;
  font-size: 9px;
  border: 1px solid var(--border);
  border-radius: 3px;
  background: var(--bg);
  color: var(--text-muted);
  cursor: pointer;
  font-family: var(--font-mono);
}
.turn-raw-btn:hover, .turn-copy-btn:hover { background: var(--bg-secondary); color: var(--text); }
.turn-raw-btn.active { background: var(--primary); color: white; border-color: var(--primary); }
.turn-body.raw-mode { background: var(--bg-tertiary); border-radius: var(--radius); }
.turn-body.raw-mode pre.raw-content { margin: 0; padding: 8px; font-size: 10px; line-height: 1.4; white-space: pre-wrap; word-break: break-word; }

/* User prompt - CLI style with subtle separator */
.turn-user {
  margin: 16px 0 4px 0;
  padding-top: 12px;
  border-top: 1px dashed var(--border);
}
.turn-user:first-child { border-top: none; padding-top: 0; }
.turn-user .turn-header {
  padding: 4px 0;
  cursor: pointer;
}
.turn-user .turn-icon { color: var(--user-border); font-size: 14px; }
.turn-user .turn-preview { font-weight: 500; }

/* Assistant response - CLI style with ● prefix */
.turn-assistant {
  margin: 0 0 8px 0;
  padding-left: 0;
}
.turn-assistant .turn-header {
  background: transparent;
  padding: 4px 0;
}
.turn-assistant .turn-icon { color: var(--assistant-border); }
.turn-assistant .turn-role { display: none; } /* Hide role label in compact mode */

/* Live tail animation for watch mode */
@keyframes tailSlideIn {
  from { opacity: 0; transform: translateX(-20px); }
  to { opacity: 1; transform: translateX(0); }
}
@keyframes tailPulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(79, 255, 79, 0); }
  50% { box-shadow: 0 0 0 4px rgba(79, 255, 79, 0.3); }
}

/* Live mode: visual feedback for new messages */
body.watching .turn:last-child {
  animation: tailSlideIn 0.3s ease-out, tailPulse 0.6s ease-out;
}
body.watching .turn:last-child .turn-header {
  background: linear-gradient(90deg, rgba(79,255,79,0.1) 0%, transparent 50%);
}

/* Live mode indicator bar */
.live-indicator {
  display: none;
  position: fixed;
  top: 48px;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(90deg, #4f4, #8f8, #4f4);
  background-size: 200% 100%;
  animation: liveGradient 2s linear infinite;
  z-index: 200;
}
body.watching .live-indicator { display: block; }
@keyframes liveGradient {
  0% { background-position: 0% 0%; }
  100% { background-position: 200% 0%; }
}

/* Tail spinner at bottom during watch mode */
.tail-spinner {
  display: none;
  padding: 12px 0 80px 0; /* bottom padding to clear toolbar */
  color: var(--assistant-border);
  font-size: 13px;
}
.tail-spinner .cli-spinner-char { color: var(--assistant-border); font-size: 14px; }
body.watching .tail-spinner { display: flex; align-items: center; gap: 8px; }

/* Agent (sidechain) - indented, purple accent */
.turn-agent {
  margin-left: 16px;
  padding-left: 12px;
  border-left: 2px solid #86c;
  opacity: 0.9;
}
.turn-agent .turn-header { background: transparent; }
.turn-agent .turn-icon { color: #86c; }
.turn-agent .turn-role { display: inline; color: #86c; }

/* Compacted context - minimal separator */
.turn-compacted {
  margin: 16px 0;
  padding: 4px 0;
  border-top: 1px dashed var(--compacted-border);
  border-bottom: 1px dashed var(--compacted-border);
  opacity: 0.7;
}
.turn-compacted .turn-header { cursor: pointer; padding: 4px 0; }
.turn-compacted .turn-icon { color: var(--compacted-border); }
.compacted-text { font-style: italic; color: var(--text-muted); font-size: 12px; }
.compact-content {
  font-size: 10px;
  max-height: 300px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--bg-secondary);
  padding: 8px;
  margin: 4px 0;
  border-radius: var(--radius);
}

/* Command - CLI style */
.turn-command { margin: 12px 0 4px 0; }
.turn-command .turn-header { padding: 4px 0; }
.turn-command .turn-icon { color: #e9a; }
.turn-command .turn-role { color: #e9a; font-family: var(--font-mono); font-size: 12px; }

/* Meta/system - dimmed */
.turn-meta { opacity: 0.5; margin: 8px 0; }
.turn-meta .turn-header { cursor: pointer; padding: 4px 0; }
.turn-meta .turn-icon { color: #999; }

/* Turn body - compact padding */
.turn-body { padding: 4px 0 8px 20px; font-size: 14px; line-height: 1.6; }

/* Block styling */
.block-text { white-space: pre-wrap; line-height: 1.5; }
.block-text p { margin: 0 0 4px 0; }
.block-text p:last-child { margin-bottom: 0; }
.block-text .md-h1, .block-text .md-h2, .block-text .md-h3, .block-text .md-h4 { font-weight: 600; margin: 8px 0 4px 0; }
.block-text .md-h1 { font-size: 1.2em; }
.block-text .md-h2 { font-size: 1.1em; }
.block-text .md-li { margin: 2px 0; }
.block-text code {
  background: var(--bg-tertiary);
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 12px;
  font-family: var(--font-mono);
}

.code-block {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 10px;
  margin: 8px 0;
  overflow-x: auto;
  font-size: 12px;
  font-family: var(--font-mono);
}

/* Markdown table */
.md-table {
  width: 100%;
  border-collapse: collapse;
  margin: 12px 0;
  font-size: 13px;
}
.md-table th, .md-table td {
  border: 1px solid var(--border);
  padding: 8px 12px;
  text-align: left;
}
.md-table th {
  background: var(--bg-secondary);
  font-weight: 600;
  font-size: 12px;
}
.md-table tr:nth-child(even) td { background: var(--bg-secondary); }
.md-table tr:hover td { background: var(--bg-tertiary); }

/* Thinking block - CLI style single-line */
.block-thinking {
  margin: 4px 0;
  border: none;
  border-radius: 0;
  background: transparent;
}
.block-thinking summary {
  padding: 2px 0;
  cursor: pointer;
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #a80;
  font-style: italic;
}
.block-thinking summary::-webkit-details-marker { display: none; }
.block-thinking summary::marker { display: none; }
.block-thinking .block-content {
  padding: 8px 0 8px 20px;
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.5;
  white-space: pre-wrap;
}

/* Tool block - inline CLI style */
.block-tool {
  margin: 4px 0;
  border: none;
  border-left: 2px solid var(--tool-border);
  border-radius: 0;
  background: transparent;
  padding-left: 8px;
}
.block-tool summary {
  padding: 2px 0;
  cursor: pointer;
  font-size: 12px;
  font-family: var(--font-mono);
  display: flex;
  align-items: center;
  gap: 4px;
}
.block-tool summary::-webkit-details-marker { display: none; }
.block-tool summary::marker { display: none; }

.block-icon { color: var(--text-muted); font-size: 11px; }
.block-thinking .block-icon { color: #a80; }
.block-tool .block-icon { color: var(--tool-border); }

.tool-preview {
  color: var(--text-muted);
  font-size: 11px;
  margin-left: 4px;
  flex: 1;
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tool-actions {
  margin-left: auto;
  display: flex;
  gap: 4px;
}

.block-content {
  padding: 10px;
  border-top: 1px solid var(--border);
  font-size: 12px;
  max-height: 300px;
  overflow: auto;
}

.block-thinking .block-content {
  background: #fffde0;
  white-space: pre-wrap;
  color: var(--text-muted);
}
[data-theme="dark"] .block-thinking .block-content { background: #282820; }

.tool-input {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 12px;
  white-space: pre-wrap;
  word-wrap: break-word;
}

/* Tool sections - compact */
.tool-section {
  border-top: none;
  padding: 4px 0 4px 8px;
  margin-top: 4px;
}
.section-label {
  font-size: 9px;
  text-transform: uppercase;
  color: var(--text-muted);
  margin-bottom: 2px;
  font-weight: 600;
  opacity: 0.7;
}
.tool-input-section pre, .tool-output-section pre {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-wrap: break-word;
  max-height: 250px;
  overflow: auto;
  background: var(--bg-secondary);
  padding: 6px 8px;
  border-radius: 4px;
}
.tool-output-section {
  background: var(--bg-tertiary);
}
.long-output { margin: 0; }
.long-output summary { cursor: pointer; display: flex; align-items: flex-start; gap: 8px; }
.long-output summary:hover { background: var(--bg-secondary); }
.long-output .output-preview { flex: 1; margin: 0; white-space: pre-wrap; }
.long-output .expand-hint { color: var(--text-muted); font-size: 11px; flex-shrink: 0; }
.long-output .output-full { max-height: 600px; overflow: auto; }
.tool-error {
  background: var(--error-bg);
  border-left: 3px solid var(--error-border);
}

.block-result {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  margin: 8px 0;
  max-height: 200px;
  overflow: auto;
}
.block-result pre {
  margin: 0;
  padding: 10px;
  font-size: 11px;
  font-family: var(--font-mono);
  white-space: pre-wrap;
}
.block-error { background: var(--error-bg); border-color: var(--error-border); }

/* Inline result for live mode */
.block-result-inline {
  margin: 0;
}
.block-result-inline .result-header {
  display: none; /* Hide since turn header already shows tool name */
}
.block-result-inline .tool-output {
  margin: 0;
  padding: 0;
  font-size: 13px;
  background: none;
  border: none;
}

.block-image { max-width: 100%; border-radius: var(--radius); margin: 8px 0; }

/* Command args */
.command-args {
  background: rgba(238,153,170,0.05);
  border-top: 1px dashed rgba(238,153,170,0.3);
  padding: 8px 12px;
  font-size: 12px;
}

/* Copy button */
.copy-btn {
  position: absolute;
  top: 4px;
  right: 4px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 3px;
  padding: 2px 6px;
  font-size: 10px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
  font-family: var(--font-mono);
  color: var(--text-muted);
}
.block-result:hover .copy-btn,
.block-content:hover .copy-btn,
.code-block:hover .copy-btn { opacity: 1; }
.copy-btn:hover { background: var(--bg-secondary); color: var(--text); }
.copy-btn.copied { color: #0a0; }

/* Raw toggle */
.raw-toggle {
  font-size: 10px;
  cursor: pointer;
  padding: 2px 6px;
  margin-left: auto;
  color: var(--text-muted);
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 3px;
}
.raw-toggle:hover { color: var(--text); }

/* Diff styling for Edit/Write tools */
.edit-diff, .write-content {
  font-family: var(--font-mono);
  font-size: 11px;
}
.diff-file {
  padding: 4px 8px;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  border-bottom: 1px solid var(--border);
  font-size: 10px;
}
.diff-old {
  background: rgba(255,100,100,0.1);
  border-left: 3px solid #c66;
  padding: 8px;
  margin: 0;
  white-space: pre-wrap;
  color: #a55;
}
.diff-new {
  background: rgba(100,255,100,0.1);
  border-left: 3px solid #6a6;
  padding: 8px;
  margin: 0;
  white-space: pre-wrap;
  color: #595;
}
[data-theme="dark"] .diff-old { background: rgba(255,100,100,0.15); color: #f88; }
[data-theme="dark"] .diff-new { background: rgba(100,255,100,0.15); color: #8f8; }

/* Todo checklist */
.todo-checklist {
  list-style: none;
  padding: 0;
  margin: 0;
}
.todo-checklist li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--border);
}
.todo-checklist li:last-child { border-bottom: none; }
.todo-icon { font-size: 14px; width: 18px; text-align: center; }
.todo-pending .todo-icon { color: var(--text-muted); }
.todo-progress .todo-icon { color: var(--primary); }
.todo-completed .todo-icon { color: #5a9; }
.todo-completed .todo-text { text-decoration: line-through; color: var(--text-muted); }
.todo-checklist input[type="checkbox"] { margin: 0; cursor: default; }

/* Task tool */
.task-call { font-family: var(--font-mono); font-size: 12px; }
.task-agent {
  display: inline-block;
  background: var(--primary);
  color: white;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;
  margin-right: 8px;
}
.task-model {
  display: inline-block;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 10px;
}
.task-prompt {
  margin-top: 8px;
  padding: 8px 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius);
  border-left: 3px solid var(--primary);
}

/* Skill tool */
.skill-call { font-family: var(--font-mono); font-size: 12px; }
.skill-name { color: var(--primary); font-weight: 600; }
.skill-args { margin-left: 8px; color: var(--text-muted); }

/* WebSearch tool */
.websearch-call { font-family: var(--font-mono); font-size: 12px; }
.search-query { color: var(--text); }

/* WebFetch tool */
.webfetch-call { font-family: var(--font-mono); font-size: 12px; }
.fetch-url { color: var(--primary); word-break: break-all; }
.fetch-prompt { margin-top: 6px; color: var(--text-muted); font-size: 11px; }

/* AskUserQuestion tool */
.ask-questions { font-size: 12px; }
.ask-question { margin-bottom: 12px; }
.ask-header {
  display: inline-block;
  background: var(--bg-tertiary);
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  margin-bottom: 4px;
}
.ask-text { margin: 6px 0; }
.ask-options { margin: 8px 0 0 16px; padding: 0; }
.ask-options li { margin: 4px 0; color: var(--text-muted); }
.ask-options li strong { color: var(--text); }

/* LSP tool */
.lsp-call { font-family: var(--font-mono); font-size: 12px; display: flex; gap: 8px; align-items: center; }
.lsp-op {
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 3px;
  font-weight: 600;
}
.lsp-loc { color: var(--text-muted); word-break: break-all; }

/* TaskOutput tool */
.taskoutput-call { font-family: var(--font-mono); font-size: 12px; display: flex; gap: 8px; align-items: center; }
.task-id { color: var(--primary); }
.task-mode { color: var(--text-muted); font-size: 10px; }

/* KillShell tool */
.killshell-call { font-family: var(--font-mono); font-size: 12px; }
.shell-id { color: #c55; }

/* Settings page */
.settings-section { margin-bottom: 24px; }
.settings-section h2 {
  font-size: 1rem;
  margin-bottom: 12px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-icon {
  font-size: 14px;
  color: var(--primary);
  width: 18px;
  text-align: center;
}

.settings-table { width: 100%; border-collapse: collapse; }
.settings-table td {
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}
.settings-table td:first-child { font-weight: 500; width: 180px; }
.settings-table code {
  background: var(--bg-tertiary);
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 12px;
}

.plugin-list { list-style: none; }
.plugin-list li {
  padding: 6px 10px;
  background: var(--bg-secondary);
  border-radius: 4px;
  margin-bottom: 4px;
}
.plugin-list code { font-size: 12px; }

.site-footer {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 40px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  color: var(--text-muted);
}

@media (max-width: 900px) {
  .sidebar { display: none; }
  .main-content { margin-left: 0; }
  .nav-sidebar { width: 180px; }
}
@media (max-width: 768px) {
  .panel-nav { width: 140px; min-width: 140px; }
  .dock-toolbar { left: 50%; max-width: calc(100vw - 40px); }
  .session-search { left: 50%; max-width: calc(100vw - 40px); min-width: auto; }
}
@media (max-width: 600px) {
  .nav-sidebar { display: none; }
  .panel-nav { display: none; }
  .top-nav { padding: 0 10px; }
  .dock-toolbar { bottom: 28px; }
  .session-search { bottom: 66px; }
  .info-panel { bottom: 66px; right: 16px; }
}
`
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
            const safeUrl = (r.url && r.url[0] === '/' && r.url[1] !== '/') ? escapeHtml(r.url) : '#';
            let html = '<a href="' + safeUrl + '" class="search-result">';
            html += badge;
            html += '<div class="result-body">';
            html += '<div class="result-title">' + escapeHtml(r.summary || r.title || 'Untitled') + '</div>';
            html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' · ' + escapeHtml(r.time) : '') + '</div>';
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
            const safeUrl = (r.url && r.url[0] === '/' && r.url[1] !== '/') ? escapeHtml(r.url) : '#';
            let html = '<a href="' + safeUrl + '" class="search-result">';
            html += badge;
            html += '<div class="result-body">';
            html += '<div class="result-title">' + escapeHtml(r.summary || r.title || 'Untitled') + '</div>';
            html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' · ' + escapeHtml(r.time) : '') + '</div>';
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

document.querySelectorAll('.nav-item').forEach(item => {
  item.addEventListener('click', function(e) {
    // Summary elements: just toggle group (native behavior), no jump
    const isSummary = this.tagName === 'SUMMARY' || this.closest('summary');
    if (isSummary) {
      // Update active state only, let native toggle handle open/close
      document.querySelectorAll('.nav-item.active').forEach(el => el.classList.remove('active'));
      this.classList.add('active');
      return;
    }

    // Regular nav items: prevent default anchor and scroll to message
    e.preventDefault();
    document.querySelectorAll('.nav-item.active').forEach(el => el.classList.remove('active'));
    this.classList.add('active');
    const msgId = this.dataset.msg;
    if (msgId) {
      const msgEl = document.getElementById('msg-' + msgId);
      if (msgEl) {
        const details = msgEl.querySelector('details');
        if (details) details.setAttribute('open', '');
        msgEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
        msgEl.style.animation = 'flash 0.4s';
      }
    }
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
    document.querySelectorAll('.nav-item.active').forEach(el => el.classList.remove('active'));
    this.classList.add('active');
    document.getElementById('msg-' + safeId)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
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

// Thread folding - click user block to fold/unfold middle responses
document.querySelectorAll('.thread').forEach(thread => {
  const userHeader = thread.querySelector('.turn-user .turn-header');
  const responses = thread.querySelector('.thread-responses');
  if (!userHeader || !responses) return;

  const turns = responses.querySelectorAll('.turn');
  if (turns.length <= 1) return; // no folding needed for single response

  // Add fold indicator in header
  const indicator = document.createElement('span');
  indicator.className = 'fold-indicator';
  indicator.dataset.hidden = turns.length - 1;
  userHeader.appendChild(indicator);

  // Add fold separator line between user and final response (clickable)
  const separator = document.createElement('div');
  separator.className = 'fold-separator';
  separator.innerHTML = '<span class="fold-sep-line"></span><span class="fold-sep-text">+' + (turns.length - 1) + ' ▶ ··· ○</span><span class="fold-sep-line"></span>';
  separator.title = 'Click to expand';
  separator.addEventListener('click', function() {
    thread.classList.remove('folded');
  });
  responses.insertBefore(separator, responses.firstChild);

  // Start folded by default for threads with many responses
  if (turns.length > 2) {
    thread.classList.add('folded');
  }

  userHeader.addEventListener('click', function(e) {
    // Don't interfere with raw/copy buttons
    if (e.target.closest('.turn-actions')) return;
    // Prevent the user details from toggling (we're hijacking the click for thread fold)
    e.preventDefault();
    thread.classList.toggle('folded');
  });
});

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

// Auto-scroll: jump to hash target or scroll to bottom
setTimeout(() => {
  const hash = window.location.hash;
  if (hash && hash.startsWith('#msg-')) {
    // Jump to specific message from search
    const msgEl = document.querySelector(hash);
    if (msgEl) {
      // Expand parent details if collapsed
      const details = msgEl.querySelector('details');
      if (details) details.setAttribute('open', '');
      msgEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
      msgEl.style.animation = 'flash 0.8s';
      // Highlight in nav
      const msgId = hash.replace('#msg-', '');
      const navItem = document.querySelector('.nav-item[data-msg="' + msgId + '"]');
      if (navItem) navItem.classList.add('active');
    }
  } else {
    // No hash - scroll to bottom (default behavior)
    window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
  }
}, 150);

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
    searchInfo.textContent = 'No matches';
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
