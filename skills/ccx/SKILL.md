---
name: ccx
description: >
  Session viewer for Claude Code, Codex, and Grok. Use this skill when working
  with agent session files, browsing conversation history, exporting
  transcripts, inspecting memory/instruction files, or analyzing AI
  interactions. Triggers on: session management, conversation export,
  transcript viewing, memory inspection, Claude Code debugging, Codex session
  viewing, Grok session viewing.
---

# ccx - Session Viewer for Agent CLIs

Supports Claude Code (~/.claude), OpenAI Codex (~/.codex), and Grok
Build (~/.grok) sessions.

```
ccx
├── projects                      # List all projects
├── sessions [project]            # List sessions
│   └── --provider cc|cx|gx       # Filter by provider
│   └── --search QUERY            # Search summaries
│   └── --after DATE              # After date (YYYY-MM-DD)
│   └── --sort time|messages|prompts  # Sort order (prompts = human-turn count)
│   └── --goal SLUG               # Filter by launch-receipt goal (deva --goal)
├── view [session]                # View session in terminal
│   └── --brief                   # Conversation only
├── export [session]              # Export session
│   └── --format html|md|org|exec
│   └── --shape full|brief|trace|exchange|human
│   └── --brief                   # (deprecated: use --shape brief)
├── search [query]                # Search across sessions
│   └── --provider cc|cx|gx       # Filter by provider
│   └── --after / --before DATE   # Date range
│   └── --model MODEL             # Filter by model
├── trace [session]               # What the agent did: turn/step outline
│   └── --json                    # Outline as JSON (ccx.outline.v1)
│   └── --turn N                  # Full evidence for one turn (ccx.turn.v1)
│   └── --full                    # Complete trace bundle (ccx.trace.v2, large; includes related)
├── related [session]             # Which sessions connect to this one, and how
│   └── --json                    # Every relation with evidence (ccx.related.v1)
├── log [project]                 # Slice raw session logs by time scope
│   └── --scope today|yesterday|week|month|quarter|year
│   └── --since / --until TIME    # RFC3339 or YYYY-MM-DD
│   └── --tz ZONE                 # IANA timezone, UTC, local, or offset like +8
│   └── --json --raw              # Evidence bundle, optional raw JSONL
├── insight [project]             # HTML/JSON data report from session logs
│   └── --scope --tz --since --until --all
│   └── --json                    # Aggregates: days[]/providers[]/workspaces[]
├── web                           # Start web UI
│   └── --port --host --no-open
│   └── --project [path] --session ID --latest  # Deep-link into a view
├── fork <session-id>             # Fork session to current project
│   └── --to /path                # Fork to specific directory
├── run <skill> [task]            # Run a bundled skill via an agent CLI
│   └── --agent claude|codex|grok # Which installed CLI executes it
│   └── --dry-run                 # Show command + payload + permissions, no exec
├── skills                        # Manage bundled agent skills
│   └── install --scope user|project  # Install skills matching this binary
├── config                        # Show / init config
└── doctor                        # Check setup
```

## Quick Start

```bash
ccx projects              # List all projects
ccx sessions              # List recent sessions
ccx sessions -p cx        # Codex sessions only
ccx sessions -p gx        # Grok sessions only
ccx view abc123           # View by session ID (prefix match)
ccx export -f html        # Export to HTML
ccx sessions --scope yesterday --tz +8 --all --json # Session containers by end time
ccx log --scope yesterday --tz +8 --all --json
ccx trace                 # Outline of the latest workspace session
ccx trace abc123 --turn 5 # Full evidence for one turn
ccx related abc123        # Sessions connected to this one: fork, handoff, mentions, shared files
ccx web                   # Start web UI at localhost:8080
```

## Trace: what the agent did

`ccx trace` prints a terminal-readable outline: every turn (user
intent) broken into steps (the agent's own narration), with tool,
edit, error, active-time, and cost rollups. Read the outline whole,
then drill: `--turn N` for one turn's full evidence, `--full` for the
complete bundle. JSON kinds and field semantics are documented in
`docs/schema.md`; header times are local (stated as `times UTC+X`),
JSON times are UTC. Session IDs resolve across all projects
automatically; `ccx trace <id>` works from any directory.

## Related: connections between sessions

`ccx related [session]` joins the islands: which other sessions of the
workspace connect to this one and how — `forked_from`/`fork_of`
(shared message ids), `mentions`/`mentioned_by` (a session id named
in text), `handoff_from`/`handoff_to` (a baton file such as HANDOFF.md,
handoffs/, devlog written by one and read by the other later),
`builds_on`/`built_on_by` (a file edited by one, then touched by the
other), `overlaps` (concurrent), `previous`/`next`. Strength is a band
(strong/medium/weak). `--json` carries the evidence per relation
(session, message id, time, path, quote) — cite from that when a
claim spans sessions. Also present as `related` in `ccx trace --full`.

## Multi-Provider

```bash
ccx sessions --provider=cc        # Claude Code only
ccx sessions --provider=cx        # Codex only
ccx sessions --provider=gx        # Grok only
ccx search --provider=cx "query"  # Search codex sessions
```

Override provider homes:
```bash
ccx --claude-home /path view
ccx --codex-home /path view
ccx --grok-home /path view
```

Grok sessions never display cost: token counts are parsed but
pricing is unverified — trust the counts, not a guessed dollar
figure.

## Export

```bash
ccx export -f html              # Rich HTML
ccx export -f md                # Markdown
ccx export -f org               # Org-mode
ccx export -f exec              # Executive summary
ccx export --shape brief        # Conversation only (no tool details)
ccx export --shape human        # Only the human's turns, verbatim,
                                # numbered, citable by #<uuid8>
```

`--shape` picks the content (full, brief, trace, exchange, human);
`--format` picks the encoding. `--shape human` is markdown-only and
defaults `--format md`; use it when distilling what the human
actually said (replay duplicates from compaction are deduplicated).
For machine-readable JSON evidence use `ccx trace --json`, not
export.

## Web UI

```bash
ccx web                   # localhost:8080
ccx web -p 3000           # Custom port
ccx web --no-open         # Don't open browser
```

Features:
- Project/session browser with multi-provider merge
- Provider filter dropdown (All / Claude Code / Codex / Grok)
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
ccx search --content "deploy"            # Also scan conversation text (user + assistant)
ccx search --content -w --sort first X   # Whole word only, earliest mention first (FIRST column)
ccx search --content -w --hits X         # Every mention as a citation: time, session, role, message id, quote
ccx search --raw "deploy"                # Grep parity over raw transcript lines
```

`-w` matters when the term prefixes a common word ("semantica" vs
"semantically"); `--json` carries `matches`, `previews`, `first_hit`.
Cite from `--hits --json` (`message_id`, `time`, `quote`) rather than
from a session-level count when a claim needs evidence.

Web search supports provider prefixes: `cc: auth bug`, `cx: codex query`, `gx: grok query`

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
  grok:
    enabled: true
    accent_color: "#8b5cf6"
```

Data locations:
- Claude Code: `~/.claude/projects/` (read-only)
- Codex: `~/.codex/sessions/` (read-only)
- Grok: `~/.grok/sessions/` (read-only)
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

Then install the agent skills that match the binary (this file and
ccx-recap/ccx-retro are embedded in the binary, so installed skills
never describe flags the binary doesn't have):
```bash
ccx skills install               # -> ~/.claude/skills/
ccx skills install --scope project  # -> ./.claude/skills/
```

## Runner bridge

```bash
ccx run ccx-recap --agent claude          # recap this workspace via claude
ccx run ccx-retro --agent codex "last session"
ccx run ccx-recap --agent grok --dry-run  # inspect before running
```

`ccx run` launches the INSTALLED agent CLI headlessly with a bundled
skill as the prompt. The agent CLI owns permissions and sandboxing —
ccx passes no permission flags. The run's session is written by the
provider and readable with `ccx trace`; a receipt linking run to
session lands in `~/.local/share/ccx/runs/`.

ccx treats all agent session data as read-only. It never modifies session files.
