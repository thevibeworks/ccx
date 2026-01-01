---
name: ccx
description: >
  Session viewer for Claude Code. Use this skill when working with Claude Code
  session files, browsing conversation history, exporting transcripts, or
  analyzing AI interactions. Triggers on: session management, conversation
  export, transcript viewing, Claude Code debugging.
---

# ccx - Session Viewer for Claude Code

```
ccx
├── projects                      # List all projects
├── sessions [project]            # List sessions (interactive picker)
├── view [session]                # View session in terminal
├── export [session]              # Export session
│   └── --format html|md|org|json
├── web                           # Start web UI
│   └── --port --host --no-open
├── search [query]                # Search across sessions
├── config                        # Show config
└── doctor                        # Check setup
```

## Quick Start

```bash
ccx projects              # List all projects
ccx sessions              # Interactive session picker
ccx view                  # View session (interactive)
ccx export -f html        # Export to HTML
ccx web                   # Start web UI at localhost:8080
```

## Session Viewing

```bash
ccx view abc123           # View by session ID
ccx view                  # Interactive picker
ccx view --project foo    # Filter by project
```

## Export Formats

```bash
ccx export -f html        # Rich HTML with syntax highlighting
ccx export -f md          # Markdown
ccx export -f org         # Org-mode
ccx export -f json        # Raw JSON
ccx export -o out.html    # Output to file
```

## Web UI

```bash
ccx web                   # Start on localhost:8080
ccx web -p 3000           # Custom port
ccx web --no-open         # Don't open browser
```

Features:
- Project/session browser with tree navigation
- Collapsible thinking/tool blocks
- Syntax highlighting
- Dark/light theme toggle
- Keyboard navigation (j/k, /, gg/G)
- Global search
- Session stats (tokens, tool usage)

## Search

```bash
ccx search "error handling"   # Search across all sessions
ccx search --project foo bar  # Search within project
```

## Configuration

Data locations:
- Sessions: `~/.claude/projects/` (read-only)
- Config: `~/.config/ccx/config.yaml`
- Data: `~/.local/share/ccx/` (stars, cache)

Override Claude Code home:
```bash
ccx --claude-home /path/to/claude view
CCX_CLAUDE_HOME=/path/to/claude ccx view
```

## Pitfalls

```bash
# Session IDs are UUIDs, not slugs
ccx view abc123-def456    # RIGHT (UUID)
ccx view my-session       # WRONG (slug not supported in CLI)

# Web UI shows both ID and slug
ccx web                   # Use web for human-friendly names
```

ccx treats Claude Code data as read-only. It never modifies session files.
