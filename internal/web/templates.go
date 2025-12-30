package web

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/claude-code/ccx/internal/parser"
)

const maxIndentDepth = 3

func renderIndexPage(projects []*parser.Project, totalSessions int, search, sortBy string) string {
	var b strings.Builder

	b.WriteString(pageHeader("ccx - Claude Code Explorer", "light"))
	b.WriteString(renderTopNav("", ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("projects"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header">`)
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

func renderProjectPage(project *parser.Project, sessions []*parser.Session, search, sortBy string) string {
	var b strings.Builder

	b.WriteString(pageHeader(project.Name+" - ccx", "light"))
	b.WriteString(renderTopNav(project.EncodedName, ""))
	b.WriteString(`<div class="layout">`)
	b.WriteString(renderSidebar("sessions"))

	b.WriteString(`<main class="main-content">`)
	b.WriteString(`<div class="page-header">`)
	b.WriteString(fmt.Sprintf(`<div class="breadcrumb"><a href="/">Projects</a> <span class="sep">/</span> <span class="current">%s</span></div>`, html.EscapeString(project.Name)))
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
	</div>
</a>`, html.EscapeString(project.EncodedName), html.EscapeString(s.ID),
			html.EscapeString(truncate(s.ID, 8)),
			s.StartTime.Format("2006-01-02 15:04"),
			formatRelativeTime(s.StartTime),
			html.EscapeString(summary),
			s.Stats.MessageCount, s.Stats.ToolCalls))
	}
	b.WriteString(`</div>`)

	b.WriteString(`</main>`)
	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(indexJS())
	b.WriteString(pageFooter())

	return b.String()
}

func renderSessionPage(session *parser.Session, projectName string, showThinking, showTools bool, theme string) string {
	var b strings.Builder

	title := fmt.Sprintf("Session %s - ccx", session.ID[:8])
	b.WriteString(pageHeader(title, theme))
	b.WriteString(renderTopNav(projectName, session.ID))
	b.WriteString(`<div class="layout session-layout">`)

	// Nav sidebar (keep it!)
	b.WriteString(`<aside class="nav-sidebar" id="nav-sidebar">`)
	b.WriteString(`<div class="sidebar-header">`)
	b.WriteString(`<h3>Conversation</h3>`)
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
	renderMessages(&b, session.RootMessages, 0, showThinking, showTools)
	b.WriteString(`</div>`)

	// Tail spinner for watch mode
	b.WriteString(`<div class="tail-spinner"><span class="cli-spinner-char"></span> Watching for updates...</div>`)

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
	b.WriteString(fmt.Sprintf(`<button class="dock-btn toggle%s" id="tb-tools" title="Tools (o)"><span class="dock-icon">⚙</span><span class="dock-label">Tools</span></button>`, toolsActive))
	b.WriteString(`</div>`)
	b.WriteString(`<div class="dock-sep"></div>`)
	b.WriteString(`<div class="dock-group dock-live">`)
	b.WriteString(`<button class="dock-btn live-btn" id="tb-watch" title="Watch live (w)"><span class="dock-pulse"></span><span class="dock-icon">◉</span><span class="dock-label">Live</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="dock-sep"></div>`)
	b.WriteString(`<div class="dock-group dock-actions">`)
	b.WriteString(`<div class="dock-dropdown">`)
	b.WriteString(`<button class="dock-btn" id="tb-export" title="Export"><span class="dock-icon">↗</span><span class="dock-label">Export</span></button>`)
	b.WriteString(`<div class="dock-menu" id="toolbar-export-menu">`)
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=html">HTML</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=md">Markdown</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=org">Org</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=txt">Text</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<a href="/api/export/%s/%s?format=json">JSON</a>`, html.EscapeString(projectName), html.EscapeString(session.ID)))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<button class="dock-btn" id="tb-info" title="Info (i)"><span class="dock-icon">ⓘ</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	// Info panel (floating, hidden by default)
	projDisplay := parser.GetProjectDisplayName(projectName)
	b.WriteString(`<div class="info-panel" id="info-panel">`)
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Project</span><a href="/project/%s">%s</a></div>`,
		html.EscapeString(projectName), html.EscapeString(projDisplay)))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Session</span><code>%s</code></div>`, html.EscapeString(session.ID)))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Started</span>%s</div>`, session.StartTime.Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Messages</span>%d</div>`, session.Stats.MessageCount))
	b.WriteString(fmt.Sprintf(`<div class="info-row"><span class="info-label">Tools</span>%d</div>`, session.Stats.ToolCalls))
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	b.WriteString(renderFooter())
	b.WriteString(sessionJS(projectName, session.ID))
	b.WriteString(pageFooter())

	return b.String()
}

func renderMessages(b *strings.Builder, messages []*parser.Message, depth int, showThinking, showTools bool) {
	allMsgs := flattenMessages(messages)

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
		b.WriteString(fmt.Sprintf(`<div class="turn turn-command%s" id="msg-%s">`, levelClass, msg.UUID))
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
		b.WriteString(fmt.Sprintf(`<details class="%s" id="msg-%s" open>`, turnClass, msg.UUID))
		b.WriteString(`<summary class="turn-header">`)
		b.WriteString(fmt.Sprintf(`<span class="turn-icon">%s</span>`, icon))
		b.WriteString(fmt.Sprintf(`<span class="turn-role">%s</span>`, role))
		b.WriteString(fmt.Sprintf(`<span class="turn-preview">%s</span>`, html.EscapeString(preview)))
		b.WriteString(fmt.Sprintf(`<span class="turn-time">%s</span>`, msg.Timestamp.Format("15:04:05")))
		b.WriteString(fmt.Sprintf(`<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(this)">copy</button></span>`))
		b.WriteString(`</summary>`)
		b.WriteString(fmt.Sprintf(`<div class="turn-body" data-raw="%s">`, html.EscapeString(rawContent)))
		for _, block := range msg.Content {
			renderBlock(b, block, showThinking, showTools, toolResults)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</details>`)
		return
	}

	b.WriteString(fmt.Sprintf(`<div class="%s" id="msg-%s">`, turnClass, msg.UUID))

	b.WriteString(`<div class="turn-header">`)
	b.WriteString(fmt.Sprintf(`<span class="turn-icon">%s</span>`, icon))
	b.WriteString(fmt.Sprintf(`<span class="turn-role">%s</span>`, role))
	b.WriteString(fmt.Sprintf(`<span class="turn-time">%s</span>`, msg.Timestamp.Format("15:04:05")))
	if msg.Model != "" {
		b.WriteString(fmt.Sprintf(`<span class="turn-model">%s</span>`, html.EscapeString(msg.Model)))
	}
	b.WriteString(fmt.Sprintf(`<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(this)">copy</button></span>`))
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
		b.WriteString(fmt.Sprintf(`<details class="block-tool" id="tool-%s"%s>`, block.ToolID, openAttr))
		b.WriteString(fmt.Sprintf(`<summary><span class="block-icon">●</span> %s<span class="tool-preview">%s</span><span class="tool-actions"><button class="raw-toggle" onclick="toggleRaw(event, '%s')">raw</button><button class="copy-btn" onclick="copyBlock(event)">copy</button></span></summary>`,
			html.EscapeString(block.ToolName), html.EscapeString(preview), block.ToolID))

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
			return "$ " + cmd
		}
	case "Task":
		if desc, ok := m["description"].(string); ok {
			return desc
		}
	}

	// Fallback: show first key=value
	for k, v := range m {
		return fmt.Sprintf("%s=%v", k, v)
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

func renderContentBlock(b *strings.Builder, block parser.ContentBlock, showThinking, showTools bool) {
	switch block.Type {
	case "text":
		if block.Text != "" {
			paragraphs := strings.Split(block.Text, "\n\n")
			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p != "" {
					b.WriteString(fmt.Sprintf(`<p>%s</p>`, html.EscapeString(p)))
				}
			}
		}

	case "thinking":
		display := "none"
		if showThinking {
			display = "block"
		}
		b.WriteString(fmt.Sprintf(`<details class="thinking-block" style="display:%s">`, display))
		b.WriteString(`<summary>Thinking...</summary>`)
		b.WriteString(fmt.Sprintf(`<div class="thinking-content">%s</div>`, html.EscapeString(block.Text)))
		b.WriteString(`</details>`)

	case "tool_use":
		display := "block"
		if !showTools {
			display = "none"
		}
		b.WriteString(fmt.Sprintf(`<div class="tool-use" id="tool-%s" style="display:%s">`, block.ToolID, display))
		b.WriteString(fmt.Sprintf(`<div class="tool-header"><span class="tool-icon">$</span> %s</div>`, html.EscapeString(block.ToolName)))
		if block.ToolInput != nil {
			inputJSON, _ := json.MarshalIndent(block.ToolInput, "", "  ")
			b.WriteString(fmt.Sprintf(`<pre class="tool-input">%s</pre>`, html.EscapeString(string(inputJSON))))
		}
		b.WriteString(`</div>`)

	case "tool_result":
		display := "block"
		if !showTools {
			display = "none"
		}
		class := "tool-result"
		if block.IsError {
			class += " tool-error"
		}
		b.WriteString(fmt.Sprintf(`<div class="%s" style="display:%s">`, class, display))
		if block.ToolResult != nil {
			result := fmt.Sprintf("%v", block.ToolResult)
			if len(result) > 2000 {
				result = result[:1997] + "..."
			}
			b.WriteString(fmt.Sprintf(`<pre>%s</pre>`, html.EscapeString(result)))
		}
		b.WriteString(`</div>`)

	case "image":
		if block.ImageData != "" {
			b.WriteString(fmt.Sprintf(`<img src="data:%s;base64,%s" class="inline-image">`,
				html.EscapeString(block.MediaType), html.EscapeString(block.ImageData)))
		}
	}
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
				g.user.UUID, g.user.UUID))
			b.WriteString(`<span class="nav-icon">◇</span><span class="nav-text">COMPACT</span></a>`)

		case parser.KindCommand:
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-command" data-msg="%s">`,
				g.user.UUID, g.user.UUID))
			b.WriteString(fmt.Sprintf(`<span class="nav-icon">⌘</span><span class="nav-text">%s</span></a>`,
				html.EscapeString(g.user.CommandName)))

		case parser.KindUserPrompt:
			preview := getNavPreview(g.user)
			childCount := len(g.children)

			if childCount == 0 {
				// No children - simple link
				b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-user" data-msg="%s">`,
					g.user.UUID, g.user.UUID))
				b.WriteString(fmt.Sprintf(`<span class="nav-icon">▶</span><span class="nav-text">%s</span></a>`,
					html.EscapeString(preview)))
			} else {
				// Has children - collapsible
				openAttr := ""
				if isLast {
					openAttr = " open"
				}
				b.WriteString(fmt.Sprintf(`<details class="nav-group"%s>`, openAttr))
				b.WriteString(fmt.Sprintf(`<summary class="nav-item nav-user" data-msg="%s">`, g.user.UUID))
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
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				hasTool = true
				toolName = block.ToolName
				break
			}
		}
		if hasTool {
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-tool" data-msg="%s">`,
				msg.UUID, msg.UUID))
			b.WriteString(fmt.Sprintf(`<span class="nav-icon">●</span><span class="nav-text">%s</span></a>`,
				html.EscapeString(toolName)))
		} else {
			b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-response" data-msg="%s">`,
				msg.UUID, msg.UUID))
			b.WriteString(`<span class="nav-icon">○</span><span class="nav-text">response</span></a>`)
		}
	case parser.KindMeta:
		b.WriteString(fmt.Sprintf(`<a href="#msg-%s" class="nav-item nav-meta" data-msg="%s">`,
			msg.UUID, msg.UUID))
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
    if (r.type === 'project') {
      html += '<a href="/project/' + r.project_encoded + '" class="search-item search-project">';
      html += '<span class="search-type">[P]</span>';
      html += '<span class="search-title">' + escapeHtml(r.title) + '</span>';
      html += '<span class="search-snippet">' + escapeHtml(r.snippet) + '</span>';
      html += '</a>';
    } else {
      html += '<a href="/session/' + r.project_encoded + '/' + r.session_id + '" class="search-item search-session">';
      html += '<span class="search-type">[S]</span>';
      html += '<span class="search-title">' + escapeHtml(r.title) + '</span>';
      html += '<span class="search-snippet">' + escapeHtml(r.project_name) + ' | ' + escapeHtml(r.snippet) + '</span>';
      html += '</a>';
    }
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
.search-item {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  text-decoration: none;
  color: inherit;
}
.search-item:hover { border-color: var(--primary); }
.search-type { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); }
.search-title { flex: 1; font-weight: 500; }
.search-snippet { font-size: 12px; color: var(--text-muted); }
.search-project { border-left: 3px solid var(--primary); }
.search-session { border-left: 3px solid var(--assistant-border); }
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
		b.WriteString(`<h2>⚙ Configuration</h2>`)
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
		b.WriteString(`<h2>🔐 Permissions</h2>`)
		b.WriteString(`<table class="settings-table">`)
		for k, v := range settings.Permissions {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><code>%s</code></td></tr>`, html.EscapeString(k), html.EscapeString(v)))
		}
		b.WriteString(`</table>`)
		b.WriteString(`</section>`)

		if len(settings.EnabledPlugins) > 0 {
			b.WriteString(`<section class="settings-section">`)
			b.WriteString(fmt.Sprintf(`<h2>🔌 Enabled Plugins <span class="count">(%d)</span></h2>`, len(settings.EnabledPlugins)))
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
			b.WriteString(`<h2>🌐 Environment</h2>`)
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
		b.WriteString(fmt.Sprintf(`<h2>◆ Agents <span class="count">(%d)</span></h2>`, len(agents)))
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
		b.WriteString(fmt.Sprintf(`<h2>✦ Skills <span class="count">(%d)</span></h2>`, len(skills)))
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
  el.innerHTML = '<pre class="source-raw">' + escapeHtml(raw) + '</pre>';
}
function escapeHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
function renderMarkdownFull(s) {
  const BT = '` + "`" + `';
  // First extract code blocks to protect them
  const codeBlocks = [];
  s = s.replace(new RegExp(BT+BT+BT+'(\\w*)\\n([\\s\\S]*?)'+BT+BT+BT, 'g'), (m, lang, code) => {
    codeBlocks.push('<pre class="code-block"><code class="lang-'+lang+'">' + escapeHtml(code) + '</code></pre>');
    return '%%CODE' + (codeBlocks.length-1) + '%%';
  });
  // Process markdown
  s = s
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
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')
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
	b.WriteString(`<a href="/" class="brand"><span class="brand-icon">◈</span> ccx</a>`)
	b.WriteString(`<span class="brand-sub">Claude Code eXplorer</span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="nav-center">`)
	b.WriteString(`<div class="global-search">`)
	b.WriteString(`<input type="text" id="global-search" class="global-search-input" placeholder="Search... (/)" autocomplete="off">`)
	b.WriteString(`<div id="search-results" class="search-results"></div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="nav-right">`)
	b.WriteString(`<button class="icon-btn" id="theme-toggle" title="Toggle theme">◐</button>`)
	b.WriteString(`<a href="/settings" class="icon-btn" title="Settings">⚙</a>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</header>`)
	return b.String()
}

func renderFooter() string {
	return ""
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
		class := "nav-item"
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
	return `<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='6' fill='%23da7756'/%3E%3Ctext x='16' y='22' text-anchor='middle' font-family='monospace' font-weight='bold' font-size='14' fill='white'%3Eccx%3C/text%3E%3C/svg%3E">`
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

// Show loading on navigation
document.querySelectorAll('a[href^="/"]').forEach(a => {
  a.addEventListener('click', function(e) {
    // Skip if modifier key or external link
    if (e.metaKey || e.ctrlKey || e.shiftKey) return;
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
.badge-project { background: var(--primary); }
.badge-session { background: var(--assistant-border); }
.badge-message { background: var(--tool-border); }
.search-loading, .search-empty {
  padding: 16px;
  color: var(--text-muted);
  font-size: 13px;
  text-align: center;
}
.search-loading { display: flex; align-items: center; justify-content: center; gap: 8px; }

.brand {
  font-weight: 700;
  font-size: 18px;
  color: var(--primary);
  text-decoration: none;
  display: flex;
  align-items: center;
  gap: 4px;
}
.brand-icon { font-size: 20px; }
.brand-sub {
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 400;
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

.nav-item {
  display: block;
  padding: 8px 12px;
  color: var(--text-muted);
  text-decoration: none;
  border-radius: 4px;
  font-size: 13px;
}
.nav-item:hover { background: var(--bg-tertiary); color: var(--text); }
.nav-item.active { background: var(--primary); color: white; }

.main-content {
  width: 100%;
  max-width: 800px;
  margin-left: 140px;
  padding: 24px 40px;
}

/* Bottom dock toolbar - modern horizontal bar */
.dock-toolbar {
  position: fixed;
  bottom: 20px;
  left: 50%;
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
.dock-label { font-size: 11px; font-weight: 500; line-height: 1; }
.dock-key { font-size: 9px; opacity: 0.5; font-family: var(--font-mono); line-height: 1; }
.dock-btn:hover .dock-key { opacity: 0.8; }

/* Live button pulse animation */
.dock-pulse {
  display: none;
  width: 6px;
  height: 6px;
  background: #4f4;
  border-radius: 50%;
  animation: pulse 1.5s infinite;
}
.dock-btn.active .dock-pulse { display: block; }
@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(1.3); }
}
.dock-btn.active .dock-icon { color: #4f4; }
.live-btn.active { background: rgba(40,80,40,0.9); }
.live-btn.active .dock-label { color: #8f8; }

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

.page-header { margin-bottom: 20px; }
.page-header h1 { font-size: 1.5rem; margin-bottom: 4px; }

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

.session-card { border-left: 3px solid var(--primary); }

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
.stat { display: flex; align-items: center; gap: 3px; }
.stat-icon { font-weight: 600; }

/* Info panel (floating above dock) */
.info-panel {
  position: fixed;
  bottom: 80px;
  left: 50%;
  transform: translateX(-50%);
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0,0,0,0.15);
  padding: 16px;
  z-index: 200;
  display: none;
  min-width: 300px;
  max-width: 400px;
}
.info-panel.show { display: block; }
.info-row { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
.info-row:last-child { border-bottom: none; }
.info-label { color: var(--text-muted); }
.info-row code { font-size: 11px; }
.info-row a { color: var(--primary); text-decoration: none; }
.info-row a:hover { text-decoration: underline; }

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
.nav-item.active { background: var(--bg-tertiary); color: var(--text); border-left: 2px solid var(--accent); }

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
  padding-left: 16px;
  margin-left: 4px;
  border-left: 1px solid var(--border);
}
.thread-responses::before { display: none; }

/* Thread folding - hide middle responses, show only last */
.thread.folded .thread-responses .turn:not(:last-child) {
  display: none;
}
.thread.folded .thread-responses .turn:last-child {
  border-left: 2px solid var(--assistant-border);
  padding-left: 8px;
  margin-left: -8px;
}
.thread.folded .fold-indicator::after { content: ' [+' attr(data-hidden) ' hidden]'; }
.thread:not(.folded) .fold-indicator::after { content: ' [-]'; }

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

/* User prompt - CLI style with > prefix and separator */
.turn-user {
  margin: 20px 0 4px 0;
  padding-top: 16px;
  border-top: 2px solid var(--user-border);
  position: relative;
}
.turn-user::before {
  content: '';
  position: absolute;
  top: -2px;
  left: 0;
  width: 60px;
  height: 2px;
  background: linear-gradient(90deg, var(--user-border), transparent);
}
.turn-user .turn-header {
  padding: 4px 0;
  cursor: pointer;
}
.turn-user .turn-icon { color: var(--user-border); font-size: 14px; }
.turn-user .turn-preview { font-weight: 500; }
/* Thread fold toggle */
.turn-user .fold-indicator {
  font-size: 10px;
  color: var(--text-muted);
  margin-left: 8px;
  opacity: 0.6;
}
.turn-user:hover .fold-indicator { opacity: 1; }

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
  padding: 20px;
  text-align: center;
  color: var(--assistant-border);
  font-size: 14px;
}
.tail-spinner .cli-spinner-char { color: var(--assistant-border); font-size: 16px; }
body.watching .tail-spinner { display: flex; align-items: center; justify-content: center; gap: 8px; }

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
.block-text p { margin: 0 0 8px 0; line-height: 1.6; }
.block-text p:last-child { margin-bottom: 0; }
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

/* Settings page */
.settings-section { margin-bottom: 24px; }
.settings-section h2 {
  font-size: 1rem;
  margin-bottom: 12px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border);
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
@media (max-width: 600px) {
  .nav-sidebar { display: none; }
  .top-nav { padding: 0 10px; }
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
            let html = '<a href="' + r.url + '" class="search-result">';
            html += '<div class="result-title">' + escapeHtml(r.summary) + '</div>';
            html += '<div class="result-meta">' + escapeHtml(r.project || '') + (r.time ? ' • ' + escapeHtml(r.time) : '') + '</div>';
            if (r.snippet) {
              html += '<div class="result-snippet">' + escapeHtml(r.snippet) + '</div>';
            }
            html += '</a>';
            return html;
          }).join('');
          searchResults.classList.add('active');
        } else {
          searchResults.innerHTML = '<div class="search-result"><div class="result-title">No results</div></div>';
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
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
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
            let html = '<a href="' + r.url + '" class="search-result">';
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
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
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

function toggleRaw(e, toolId) {
  e.stopPropagation();
  const tool = document.getElementById('tool-' + toolId);
  if (!tool) return;
  const inputSection = tool.querySelector('.tool-input-section');
  if (!inputSection) return;

  const btn = e.target;
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

function toggleTurnRaw(btn) {
  event.stopPropagation();
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
    const rawData = body.dataset.raw || '[]';
    body.innerHTML = '<pre class="raw-content">' + escapeHtml(rawData) + '</pre>';
    body.classList.add('raw-mode');
    btn.textContent = 'fmt';
    btn.classList.add('active');
  }
}

function copyTurn(btn) {
  event.stopPropagation();
  const turn = btn.closest('.turn') || btn.closest('details.turn-user');
  if (!turn) return;
  const body = turn.querySelector('.turn-body');
  if (!body) return;

  let text;
  if (body.classList.contains('raw-mode')) {
    // Raw mode: copy JSON
    text = body.dataset.raw || '';
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
    // If this is a summary inside a details, let native toggle work
    const isSummary = this.tagName === 'SUMMARY' || this.closest('summary');
    if (!isSummary) {
      e.preventDefault();
    }
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
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

// Make summary elements in nav-groups clickable for jump (separate from toggle)
document.querySelectorAll('.nav-group > summary').forEach(summary => {
  summary.addEventListener('dblclick', function(e) {
    // Double-click to jump to the message
    const msgId = this.dataset.msg;
    if (msgId) {
      const msgEl = document.getElementById('msg-' + msgId);
      if (msgEl) {
        msgEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
        msgEl.style.animation = 'flash 0.4s';
      }
    }
  });
});

// Scrollspy - highlight nav item matching visible message
const navSidebar = document.getElementById('nav-sidebar');
let lastActiveId = null;

function updateScrollspy() {
  const viewTop = window.scrollY + 100;
  let currentId = null;

  // Find message in view by checking each element's position
  document.querySelectorAll('[id^="msg-"]').forEach(el => {
    if (el.getBoundingClientRect().top + window.scrollY <= viewTop) {
      currentId = el.id.replace('msg-', '');
    }
  });

  if (currentId && currentId !== lastActiveId) {
    lastActiveId = currentId;

    // Update active nav item
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    const activeNav = document.querySelector('.nav-item[data-msg="' + currentId + '"]');
    if (activeNav) {
      activeNav.classList.add('active');
      // Scroll sidebar to show it
      activeNav.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }
}

window.addEventListener('scroll', updateScrollspy, { passive: true });
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
  if (data.type === 'user') {
    // Check if it's tool_result (first block is tool_result)
    if (Array.isArray(content) && content[0]?.type === 'tool_result') {
      return; // Tool results rendered inline with tool_use
    }
  }

  // Build HTML matching Go renderTurnMessage
  let html = '';
  const rawJSON = JSON.stringify(content || []);

  if (kind === 'user') {
    const preview = getTextPreview(content, 60);
    html = '<details class="turn turn-user" id="msg-' + uuid + '" open>' +
      '<summary class="turn-header">' +
        '<span class="turn-icon">▶</span>' +
        '<span class="turn-role">USER</span>' +
        '<span class="turn-preview">' + escapeHtml(preview) + '</span>' +
        '<span class="turn-time">' + timestamp + '</span>' +
        '<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(this)">copy</button></span>' +
      '</summary>' +
      '<div class="turn-body" data-raw="' + escapeHtml(rawJSON) + '">' +
        renderContentBlocks(content) +
      '</div>' +
    '</details>';
  } else {
    let turnClass = 'turn turn-assistant';
    let icon = '●';
    let role = 'ASSISTANT';
    if (isSidechain) {
      turnClass += ' turn-agent';
      icon = '◆';
      role = 'AGENT';
    }
    html = '<div class="' + turnClass + '" id="msg-' + uuid + '">' +
      '<div class="turn-header">' +
        '<span class="turn-icon">' + icon + '</span>' +
        '<span class="turn-role">' + role + '</span>' +
        '<span class="turn-time">' + timestamp + '</span>' +
        (model ? '<span class="turn-model">' + escapeHtml(model) + '</span>' : '') +
        '<span class="turn-actions"><button class="turn-raw-btn" onclick="toggleTurnRaw(this)">raw</button><button class="turn-copy-btn" onclick="copyTurn(this)">copy</button></span>' +
      '</div>' +
      '<div class="turn-body" data-raw="' + escapeHtml(rawJSON) + '">' +
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
  // In tail/watch mode: expand everything for verbose real-time view
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
        const toolOpen = showTools ? ' open' : '';
        const toolName = block.name || 'tool';
        const toolId = block.id || 'tool-' + Date.now();
        const inputPreview = compactToolPreviewJS(toolName, block.input);
        html += '<details class="block-tool" id="tool-' + toolId + '"' + toolOpen + '>' +
          '<summary><span class="block-icon">●</span> ' + escapeHtml(toolName) +
          '<span class="tool-preview">' + escapeHtml(inputPreview) + '</span></summary>' +
          '<div class="tool-section tool-input-section">' +
            '<div class="section-label">input</div>' +
            '<pre>' + escapeHtml(JSON.stringify(block.input, null, 2)) + '</pre>' +
          '</div>' +
        '</details>';
        break;
    }
  }
  return html;
}

function renderMarkdownJS(text) {
  // Simple markdown: code blocks, inline code, bold, italic, links
  const BT = '`+"`"+`';
  const codeBlockRe = new RegExp(BT+BT+BT+'(\\w*)\\n([\\s\\S]*?)'+BT+BT+BT, 'g');
  const inlineCodeRe = new RegExp(BT+'([^'+BT+']+)'+BT, 'g');
  return text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(codeBlockRe, '<pre class="code-block"><code>$2</code></pre>')
    .replace(inlineCodeRe, '<code>$1</code>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
    .replace(/\n/g, '<br>');
}

function compactToolPreviewJS(name, input) {
  if (!input) return '';
  switch (name) {
    case 'Read': return input.file_path || '';
    case 'Edit': return input.file_path || '';
    case 'Write': return input.file_path || '';
    case 'Bash': return (input.command || '').slice(0, 50);
    case 'Glob': return input.pattern || '';
    case 'Grep': return input.pattern || '';
    default: return '';
  }
}

function updateNavForMessage(uuid, kind, content, time) {
  const navList = document.getElementById('nav-list');
  if (!navList) return;

  let icon = kind === 'user' ? '▶' : '●';
  let cls = kind === 'user' ? 'nav-user' : 'nav-response';
  let text = kind === 'user' ? getTextPreview(content, 40) : 'Response';

  const item = document.createElement('a');
  item.href = '#msg-' + uuid;
  item.className = 'nav-item ' + cls;
  item.dataset.msg = uuid;
  item.innerHTML = '<span class="nav-icon">' + icon + '</span><span class="nav-text">' + escapeHtml(text || kind) + '</span>';
  item.addEventListener('click', function(e) {
    e.preventDefault();
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    this.classList.add('active');
    document.getElementById('msg-' + uuid)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  });
  navList.appendChild(item);
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

  // Add fold indicator
  const indicator = document.createElement('span');
  indicator.className = 'fold-indicator';
  indicator.dataset.hidden = turns.length - 1;
  userHeader.appendChild(indicator);

  // Start folded by default for threads with many responses
  if (turns.length > 2) {
    thread.classList.add('folded');
  }

  userHeader.addEventListener('click', function(e) {
    // Don't interfere with raw/copy buttons
    if (e.target.closest('.turn-actions')) return;
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
  console.log('Found', userBlocks.length, 'user blocks');
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
  if (userBlocks.length === 0) { console.log('No user blocks found'); return; }
  const cur = findCurrentUserIdx();
  scrollToUser(cur > 0 ? cur - 1 : 0);
});

document.getElementById('tb-next-user')?.addEventListener('click', () => {
  initUserNav();
  if (userBlocks.length === 0) { console.log('No user blocks found'); return; }
  const cur = findCurrentUserIdx();
  scrollToUser(cur < userBlocks.length - 1 ? cur + 1 : userBlocks.length - 1);
});

document.getElementById('tb-top')?.addEventListener('click', () => {
  window.scrollTo({ top: 0, behavior: 'smooth' });
});

document.getElementById('tb-bottom')?.addEventListener('click', () => {
  window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
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
</script>
<style>
@keyframes flash {
  0%%, 100%% { background: transparent; box-shadow: none; }
  25%%, 75%% { background: rgba(218, 119, 86, 0.15); box-shadow: 0 0 0 2px var(--primary); }
}
</style>
`, projectName, sessionID)
}
