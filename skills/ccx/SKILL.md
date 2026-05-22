---
name: ccx
description: >
  Session viewer for Claude Code and Codex. Use this skill when working with
  agent session files, browsing conversation history, exporting transcripts,
  inspecting memory/instruction files, or analyzing AI interactions.
  Triggers on: session management, conversation export, transcript viewing,
  memory inspection, Claude Code debugging, Codex session viewing.
---

# ccx - Session Viewer for Agent CLIs

Supports Claude Code (~/.claude) and OpenAI Codex (~/.codex) sessions.

```
ccx
├── projects                      # List all projects
├── sessions [project]            # List sessions
│   └── --provider cc|cx          # Filter by provider
│   └── --search QUERY            # Search summaries
│   └── --after DATE              # After date (YYYY-MM-DD)
│   └── --sort time|messages      # Sort order
├── view [session]                # View session in terminal
│   └── --brief                   # Conversation only
├── export [session]              # Export session
│   └── --format html|md|org|json
│   └── --brief                   # Conversation only
├── search [query]                # Search across sessions
│   └── --provider cc|cx          # Filter by provider
│   └── --after / --before DATE   # Date range
│   └── --model MODEL             # Filter by model
├── insight [scope]               # Summarize scoped session intelligence
│   └── --tz ZONE                 # IANA timezone, UTC, local, or offset like +8
│   └── --json                    # Machine-readable insight bundle
├── web                           # Start web UI
│   └── --port --host --no-open
├── fork <session-id>             # Fork session to current project
│   └── --to /path                # Fork to specific directory
├── config                        # Show / init config
└── doctor                        # Check setup
```

## Quick Start

```bash
ccx projects              # List all projects
ccx sessions              # List recent sessions
ccx sessions -p cx        # Codex sessions only
ccx view abc123           # View by session ID (prefix match)
ccx export -f html        # Export to HTML
ccx sessions --scope yesterday --tz +8 --all --json
ccx insight yesterday --tz +8 # Summarize session intelligence
ccx web                   # Start web UI at localhost:8080
```

## Multi-Provider

```bash
ccx sessions --provider=cc        # Claude Code only
ccx sessions --provider=cx        # Codex only
ccx search --provider=cx "query"  # Search codex sessions
```

Override provider homes:
```bash
ccx --claude-home /path view
ccx --codex-home /path view
```

## Export

```bash
ccx export -f html              # Rich HTML
ccx export -f md                # Markdown
ccx export -f org               # Org-mode
ccx export -f json              # Raw JSON
ccx export -f html --brief      # Conversation only (no tool details)
```

## Web UI

```bash
ccx web                   # localhost:8080
ccx web -p 3000           # Custom port
ccx web --no-open         # Don't open browser
```

Features:
- Project/session browser with multi-provider merge
- Provider filter dropdown (All / Claude Code / Codex)
- Memory file inspector (per-project, expandable with fmt/raw/copy)
- Collapsible thinking/tool blocks
- In-session search with filter chips
- Brief export (conversation-only)
- Dark/light theme toggle
- Keyboard navigation (j/k, /, gg/G, d for theme)
- Global search (sessions + memory files)
- Session stats (tokens, messages, tools)
- Settings page (provider status, config inspection)

## Search

```bash
ccx search "error handling"              # All providers
ccx search --provider=cc "auth bug"      # Claude Code only
ccx search --after=2026-03-01 "deploy"   # Date filtered
```

Web search supports provider prefixes: `cc: auth bug`, `cx: codex query`

## Fork Session

Resume any session from any project directory:

```bash
ccx fork abc12345                 # Fork to current directory
ccx fork abc12345 --to /new/path  # Fork to specific directory
# Then: claude --resume <new-uuid>
```

Rewrites sessionId and CWD, drops file-history snapshots, clears worktree state.
Original session is never modified.

## Memory Inspection

Web UI shows memory files on the project page:
- Global instructions (CLAUDE.md, AGENTS.md, instructions.md)
- Per-project memory (MEMORY.md + topic files)
- Expandable with formatted/raw view + copy

Cross-project memory overview at `/memory` page.

## Configuration

```yaml
# ~/.config/ccx/config.yaml
theme: dark
show_thinking: collapsed
default_format: html
port: 8080
providers:
  claude-code:
    enabled: true
    accent_color: "#da7756"
  codex:
    enabled: true
    accent_color: "#3b82f6"
```

Data locations:
- Claude Code: `~/.claude/projects/` (read-only)
- Codex: `~/.codex/sessions/` (read-only)
- Config: `~/.config/ccx/config.yaml`
- Data: `~/.local/share/ccx/` (stars)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/thevibeworks/ccx/main/install.sh | bash
```

Or via Go:
```bash
go install github.com/thevibeworks/ccx/cmd/ccx@latest
```

ccx treats all agent session data as read-only. It never modifies session files.
