---
name: ccx-fold
description: >
  Fold a coding session into auditable decisions and durable agent knowledge.
  Use after a session, when reviewing agent work, or when the human asks
  "what did we decide?" Triggers: /ccx-fold, fold session, session decisions,
  what did we decide, extract decisions, session debrief.
---

# ccx-fold

Fold a human-agent session into two outputs:

1. **fold.html** -- decision trail for human review (temp dir, not repo)
2. **knowledge entries** -- structured markdown for agent consumption (`.ccx/knowledge/`)

A fold is not a summary. It reconstructs decision provenance: who decided,
what was decided, why, what was rejected, and what pressure will build.

## Modes

| Trigger | Behavior |
|---|---|
| `/ccx-fold` | Fold the most recent session for this project |
| `/ccx-fold <session-id>` | Fold a specific session (prefix match) |
| `/ccx-fold --since <ref>` | Fold all sessions since a git ref or date (see Multi-session below) |
| "quick fold" or "just highlights" | Light mode: top decisions + open question, no files |
| "check the knowledge base" | Lint mode: audit KB for staleness and contradictions |
| `/ccx-fold --dry-run` | Print the decision plan; write nothing |

## Process

### Phase 0: Pre-flight

1. Verify git repo (`git rev-parse --show-toplevel`). If not a git repo, STOP.
2. Resolve session: if session-id given, find the matching JSONL; if empty,
   pick the most recent session for this project's working directory.
   If no session found, STOP with guidance.
3. Set output paths:
   ```
   TMPDIR="${TMPDIR:-/tmp}"
   FOLD_HTML="$TMPDIR/ccx-fold-$(date +%Y%m%d-%H%M%S)-<slug>.html"
   KB_DIR="<git-root>/.ccx/knowledge"
   ```
4. Create `$KB_DIR/{decisions,discoveries,corrections,patterns}` if first fold.
5. Read workspace context: CLAUDE.md, CONTEXT.md, docs/adr/, prior KB entries.
   See [EVIDENCE.md](EVIDENCE.md) for the full input inventory.

### Phase 1: Build evidence graph

Parse the session into exchanges. Correlate with git commits in the session
time window. See [EVIDENCE.md](EVIDENCE.md) for evidence types, edges, and
citation format.

### Phase 2: Extract and approve decisions

Detect decisions using [DECISIONS.md](DECISIONS.md) heuristics. For each,
attribute provenance (human / agent / joint / correction) and attention tier.

Present the full list to the user in conversation:

```
Fold Plan — session <slug> (<id>)
<date> | <duration> | <model> | <N> exchanges | <M> commits

 #  Attention  Provenance  Decision
 1  high       joint       Chose SSE over WebSocket for real-time updates
 2  high       correction  Don't use ORM — raw SQL only in this codebase
 3  mid        agent       Added retry with backoff on 429 responses
 4  low        agent       Sorted imports alphabetically
    ...

Approve all? [y] / Edit specific entries? [e] / Skip fold? [n]
```

If user edits: reclassify, add missing, or remove false positives.
Rejected decisions are excluded from both HTML and KB.

Apply the three-gate bar (hard to reverse + surprising + real tradeoff)
to every decision regardless of attention tier. Decisions that fail any
gate go into fold.html but NOT the knowledge base.

### Phase 3: Write outputs

1. Generate `$FOLD_HTML` — see [HTML-REPORT.md](HTML-REPORT.md).
   Open in browser. Tell user the path.
2. Write approved KB entries to `$KB_DIR/` — see [ARCHIVE.md](ARCHIVE.md).
3. Update `$KB_DIR/index.md` and append to `$KB_DIR/log.md`.
4. `git add $KB_DIR/ && git commit` with fold summary.

### Phase 4: Review queue

Print to conversation (not to a file):

- Decisions needing human confirmation (inferred provenance, low confidence)
- Agent decisions with high blast radius
- Vocabulary that drifted from CONTEXT.md
- The single most important open question (the live wire)

## Light mode

No evidence graph, no HTML, no KB writes. Print to conversation only:

- Top 3 decisions: one line each with provenance badge
- The live wire: what's unresolved
- Suggested next action

## Lint mode

Audit the existing KB. See [ARCHIVE.md](ARCHIVE.md) lint checklist.

## Multi-session (`--since`)

When folding multiple sessions: process each independently through
Phases 0-2, then merge decision lists. De-duplicate decisions on the
same topic across sessions (keep the latest). Generate one combined
fold.html and one set of KB entries. Cross-session decisions link via
the `supersedes` field.

## Error handling

| Situation | Behavior |
|---|---|
| No session found | STOP with: "No session found. Run `ccx sessions` to list available sessions." |
| Session JSONL corrupted/truncated | Warn, fold what's parseable, note gap in HTML metadata |
| Git history doesn't overlap session | Warn, skip git correlation, note "no commits in session window" |
| Zero decisions detected | Report "no decisions detected — session may have been mechanical." Do not create empty files. |
| KB entry filename collision | Append `-2`, `-3` suffix to the slug |
| `.ccx/knowledge/` doesn't exist | Create it (first fold) |
