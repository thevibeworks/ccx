# ccx

Session viewer for Claude Code. Browse, inspect, search, and export your conversations.

Inspired by [Simon Willison's claude-code-transcripts](https://github.com/simonw/claude-code-transcripts), rebuilt in Go with live tailing, web UI, and skill-ready architecture.

## Installation

Requires Go 1.24+ (pure Go, no C compiler needed).

```bash
go install github.com/thevibeworks/ccx/cmd/ccx@latest
```

Or build from source:
```bash
git clone https://github.com/thevibeworks/ccx
cd ccx
make build
./bin/ccx --help
```

## Quick Start

```bash
ccx projects              # List all projects
ccx sessions              # List recent sessions
ccx view                  # View session (interactive picker)
ccx export -f html        # Export to HTML
ccx doctor                # Check configuration
```

## Commands

### List Projects
```bash
ccx projects [--sort=name|time|sessions] [--limit N] [--json]
```

### List Sessions
```bash
ccx sessions [project] [--sort=time|messages] [--limit N] [--json]
```

### View Session
```bash
ccx view [session] [--project NAME] [--show-thinking] [--flat]
```

Session identifiers:
- Full UUID: `e38536a2-dbe6-442d-8b69-5bab525796ee`
- Short prefix: `e38536`
- With project: `myproject:e38536`

### Export Session
```bash
ccx export [session] --format=html|md|org [--output FILE] [--theme dark|light]
```

### Search
```bash
ccx search QUERY          # Search projects and sessions by name or summary
ccx search auth           # Find sessions about authentication
ccx search -t session     # Only search sessions
ccx search --json         # JSON output
```

### Shell Completion
```bash
ccx completion bash       # Generate bash completion
ccx completion zsh        # Generate zsh completion
ccx completion fish       # Generate fish completion
```

### Configuration
```bash
ccx config show           # Show current config
ccx config init           # Create default config
ccx config path           # Show config file location
```

## Configuration

Environment variables:
- `CLAUDE_CODE_HOME` - Override `~/.claude` location
- `CCX_CONFIG` - Explicit config path

Config file locations (priority order):
1. `$CCX_CONFIG`
2. `$XDG_CONFIG_HOME/ccx/config.yaml`
3. `~/.config/ccx/config.yaml`

Example config:
```yaml
theme: dark
rendering:
  syntax_highlight: true
  show_thinking: collapsed
  code_theme: monokai
export:
  default_format: html
```

## Data Safety

ccx treats `CLAUDE_CODE_HOME` as read-only. It never modifies Claude Code session JSONL files or Claude Code config.

ccx only writes:
- `$XDG_DATA_HOME/ccx/` or `~/.local/share/ccx/` (SQLite data if you run `ccx web`)
- `$XDG_CONFIG_HOME/ccx/` or `~/.config/ccx/` (config file)
- Export output files you explicitly request

## Session Tree Model

ccx understands Claude Code's session structure:

```
Session
├─ [USER] Initial prompt
├─ [ASSISTANT] Response with tools
│   ├─ [TOOL: Read] file.go
│   └─ [TOOL: Edit] file.go
├─ ═══ [COMPACTED] ═══
│   Context summarized...
├─ [USER] Continue
└─ [ASSISTANT] Building on previous...
```

Features:
- Compacted context markers
- Agent sidechain linking
- Tree-aware rendering

## Export Formats

### HTML
Standalone file with embedded CSS, dark/light theme, collapsible sections.

### Markdown
GFM-compatible with code blocks and details tags.

### Org-mode
Emacs-friendly with proper headings, source blocks, and timestamps.

## License

Apache 2.0
