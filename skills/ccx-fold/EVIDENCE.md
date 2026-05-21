# Evidence Graph

How ccx-fold gathers context and links evidence to decisions.

## Input inventory

Read as much as exists. Missing inputs reduce accuracy but don't block
the fold — except the session JSONL itself, which is required.

### Session (required)

The session JSONL is the primary evidence source.

```bash
# Via ccx (preferred — handles path resolution and format normalization)
ccx export "$SESSION_ID" --format json

# Direct fallback (if ccx unavailable)
# Claude Code stores sessions at ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl
# The encoded CWD replaces / with - and prepends -
ENCODED=$(echo "$PWD" | sed 's|^/|-|; s|/|-|g')
SESSION_DIR="$HOME/.claude/projects/$ENCODED"
# Pick the most recent JSONL
SESSION_FILE=$(ls -t "$SESSION_DIR"/*.jsonl 2>/dev/null | head -1)
```

Parse into exchanges: each user prompt and the assistant response,
tool calls, and results that follow it form one exchange.

**If the session file is missing**: STOP. No fold without transcript.
**If the file is truncated**: warn, parse what's available, note the
gap in fold.html metadata.

### Git state

```bash
git log --after="$SESSION_START" --before="$SESSION_END" \
  --format="%H %ai %s" --all
git diff --stat HEAD
git branch --show-current
```

**If git history doesn't overlap the session window**: skip git
correlation. Note "no commits found in session window" in the fold
output. Decisions will be ungrounded (no linked commits).

### Workspace context

```bash
cat CLAUDE.md CONTEXT.md AGENTS.md 2>/dev/null
cat .claude/MEMORY.md 2>/dev/null
find docs/adr/ -name '*.md' 2>/dev/null
```

### Prior knowledge

```bash
cat .ccx/knowledge/index.md 2>/dev/null
ls .ccx/knowledge/{decisions,discoveries,corrections}/ 2>/dev/null
```

## Evidence nodes

| Type | Source | Example |
|---|---|---|
| `exchange` | Session JSONL | User prompt + assistant response cycle |
| `tool.call` | Session JSONL | Bash, Edit, Write, Read invocations |
| `tool.result` | Session JSONL | Command output, file contents, errors |
| `thinking` | Session JSONL | Assistant thinking/reasoning blocks |
| `sidechain` | Session JSONL | Sub-agent delegation and result |
| `git.commit` | Git log | Commit in the session time window |
| `doc.section` | Workspace | CLAUDE.md rule, ADR entry, CONTEXT.md term |
| `prior.entry` | Knowledge base | Existing decision or pattern entry |

## Evidence edges

Link nodes when the relationship is clear from timestamps, content,
or explicit references:

| Edge | Meaning |
|---|---|
| `caused` | This exchange caused that tool call |
| `produced` | This tool call produced that commit |
| `contradicted` | This decision contradicts that ADR or prior entry |
| `superseded` | This decision replaces that prior entry |
| `verified_by` | This claim was verified by that test/command result |
| `corrected` | This user message corrected that agent action |

## Citation format

Use stable, greppable references. The UUID is always the message-level
`uuid` field from the JSONL (the parent message that contains the
tool_use block), not the tool_use block's `id`.

```
session:<session-id>#<exchange-number>       # exchange by position
session:<session-id>#<message-uuid>          # message by UUID
tool:<message-uuid>:<tool-name>              # tool call within a message
git:<commit-sha>                             # git commit
file:<path>:<line>                           # source location
doc:<path>#<heading-slug>                    # documentation section
kb:<entry-filename>                          # knowledge base entry
```

When a citation is approximate (e.g., exchange boundary ambiguous after
compaction), mark it: `session:abc123#~14 (post-compaction)`.

## Compaction handling

Sessions with compaction boundaries have incomplete context before the
boundary. For decisions detected pre-compaction:

- Mark the card: "pre-compaction -- reasoning may be incomplete"
- Use git commits in the time window as supplementary evidence
- Do not fabricate reasoning absent from the transcript

## Sidechain handling

Claude Code stores sub-agent transcripts alongside the parent session.
The parent JSONL contains `isSidechain: true` messages with an `agentId`
field. The sub-agent's full transcript is in the same session directory
at `<session-uuid>/subagents/agent-<agentId>.jsonl` (relative to the
session's project folder under `~/.claude/projects/`).

For each sidechain:

- The delegation (TaskCreate prompt) is a decision by the initiator
- Internal sidechain decisions are attributed to `agent`
- Include the sidechain's summary result, not its full transcript
- File changes from sidechains appear in git correlation as
  agent-attributed commits
