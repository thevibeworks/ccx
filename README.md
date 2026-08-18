# ccx

**Session viewer for coding agents** — Browse, search, and export conversations from Claude Code, Codex, and Grok.

```
ccx web
```

Opens a browser at `localhost:8080`. That's it.

## Screenshots

![Projects](screenshots/ccx-projects.jpeg)

![Live Mode](screenshots/ccx-live.jpeg)

<details>
<summary>More screenshots</summary>

![Session Info](screenshots/ccx-session-info.jpeg)

![Export](screenshots/ccx-export.jpg)

![Settings](screenshots/ccx-settings.jpeg)

</details>

## What it does

ccx reads session files from `~/.claude/`, `~/.codex/`, and `~/.grok/` and gives you a fast, keyboard-driven interface to browse them.

- **Multi-provider** — Claude Code + Codex + Grok sessions merged by project, with provider badges
- **Two-panel navigation** — Projects → Sessions → Conversation tree
- **Global sessions** — `/sessions` lists every session across projects; filter by text/provider/project/model/date, group by project/day/provider/model, sort by recency/messages/prompts/tokens
- **Live tail** — Watch active sessions update in real-time
- **In-session search** — Filter by User, Response, Tools, Agents, Thinking
- **Memory inspector** — View CLAUDE.md, MEMORY.md, AGENTS.md per project
- **Export shapes** — HTML, Markdown, Org-mode; `--shape brief` strips tool noise, `--shape human` emits only the human's numbered, citable turns
- **Turn evidence** — per-turn review panel in the web UI (steps, tools, edited files, failed calls, cost) numbered identically to `ccx trace`, with `?turn=N` deep links
- **Runner bridge** — `ccx run <skill> --agent claude|codex|grok` executes a bundled skill through your installed agent CLI, with `--dry-run` disclosure and a session receipt
- **Context trace** — `ccx trace` emits evidence for Context Folding
- **Time-sliced logs** — `ccx log` cuts through long-running session JSONL by timestamp
- **Provider filter** — `--provider cc`, `cx`, or `gx` on any command
- **Date filter** — `--after 2026-03-01 --before 2026-04-01`
- **Keyboard shortcuts** — `j/k` scroll, `/` search, `z` fold, `r` refresh, `d` theme

Single binary, zero dependencies, read-only. Never touches your session files.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/thevibeworks/ccx/main/install.sh | bash
```

Or via Go:
```bash
go install github.com/thevibeworks/ccx/cmd/ccx@latest
```

Or build from source:
```bash
git clone https://github.com/thevibeworks/ccx
cd ccx && make build
./bin/ccx web
```

## Usage

```bash
ccx web                          # Start web UI (recommended)
ccx projects                     # List all projects
ccx sessions                     # List recent sessions for this workspace
ccx sessions --all               # List recent sessions across all projects
ccx sessions --provider=cx       # Codex sessions in this workspace
ccx sessions --after=2026-03-01  # Date filtered
ccx sessions --scope yesterday --tz +8 --all --json # Session containers by end time
ccx view [session]               # View in terminal
ccx export --shape brief         # Export conversation-only HTML
ccx export --shape human         # Only the human's turns, citable
ccx trace [session] -o trace.json # Extract evidence for context folding
ccx log --scope yesterday --tz +8 --all --json # Time-sliced log evidence
ccx search "auth bug"            # Search across sessions + memory
ccx search --content -w --sort first goose # Whole-word content hits, earliest first
ccx search --content -w --hits goose      # Every mention, quoted + anchored (time, session, message id)
ccx run ccx-recap --agent claude # Run a bundled skill via an agent CLI
ccx fork abc123                  # Fork session to current project
ccx doctor                       # Check setup
```

## Configuration

```yaml
# ~/.config/ccx/config.yaml
theme: dark
show_thinking: collapsed
default_format: html
providers:
  claude-code:
    enabled: true
  codex:
    enabled: true
  grok:
    enabled: true
```

Grok sessions show token counts but never cost: pricing is
unverified, and a guessed dollar figure is worse than none.

Override provider homes:
```bash
ccx --claude-home /path web
ccx --codex-home /path web
ccx --grok-home /path web
```

## Data safety

ccx treats all agent data as **read-only**. Writes only to its own directories:
- `$XDG_CONFIG_HOME/ccx/` — config
- `$XDG_DATA_HOME/ccx/` — stars database

## Claude Code skills

ccx ships with Claude Code skills, embedded in the binary:

- **ccx** — Session viewer. Browse, search, export sessions from inside Claude Code.
- **ccx-recap** — What did the agent actually do? Recaps a session or time window from trace evidence: story, decisions, verified-vs-claimed, cost. Ends by landing the distillation in your durable store with `[ccx:<session-id> #turn.step]` citations.
- **ccx-retro** — Where did it go wrong and what should change? Mines a session for mistakes, corrections, and saves, then proposes evidence-cited patches to CLAUDE.md/AGENTS.md/skills/memory — behind a human gate.

Install them from the binary so the skill text always matches the CLI
surface of the build you are running (skills version with the repo,
binaries with tags — copying from a checkout invites drift):

```bash
ccx skills install                   # -> ~/.claude/skills/
ccx skills install --scope project   # -> ./.claude/skills/
ccx skills list                      # show drift between installed copies and this binary
```

Usage:
```bash
/ccx-recap                           # Recap the latest session here
/ccx-recap <session-id>              # Recap a specific session
/ccx-retro                           # What went wrong -> proposed rule patches
```

The JSON contracts these skills consume (`ccx.outline.v1`,
`ccx.turn.v1`, `ccx.trace.v2`, `ccx.log.v1`) are documented in
[docs/schema.md](docs/schema.md).

## Credits

Inspired by [Simon Willison's claude-code-transcripts](https://github.com/simonw/claude-code-transcripts). Rebuilt in Go with live tailing, multi-provider support, and a web UI.

## License

Apache 2.0

---

Built by [thevibeworks](https://github.com/thevibeworks) · [@ericwang42](https://x.com/ericwang42)
