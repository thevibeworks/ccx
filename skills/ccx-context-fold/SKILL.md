---
name: ccx-context-fold
description: >
  Fold a ccx trace into auditable decisions and durable project knowledge.
  Use after a coding-agent session, before context compaction, during PR
  review, or when the human asks "what did we decide?", "fold this session",
  "extract decisions", or "update project knowledge".
---

# ccx-context-fold

Context Folding turns a finite-context agent session into two reviewable
artifacts:

1. **fold.html** -- decision trail for humans, written to the temp dir
2. **knowledge patch** -- proposed `.ccx/knowledge/` entries for agents

The split matters:

- `ccx trace` is factual: transcript, tools, files, git, docs, gaps.
- `ccx-context-fold` is interpretive: decisions, provenance, tradeoffs,
  corrections, discoveries, and what should compound into knowledge.

A fold is not a summary. It reconstructs decision provenance: who decided,
what was decided, why, what was rejected, what evidence supports it, and
what pressure will build later.

## Modes

| Trigger | Behavior |
|---|---|
| `/ccx-context-fold` | Trace and fold the most recent session for this project |
| `/ccx-context-fold <session-id>` | Fold a specific session prefix |
| "quick fold" or "just highlights" | Light mode: top decisions + live wire, no files |
| "lint context knowledge" | Lint mode: audit `.ccx/knowledge/` for stale or contradictory entries |
| `/ccx-context-fold --dry-run` | Build the fold plan; write no files |

## Process

### Phase 0: Pre-flight

1. Verify a git repo:
   ```bash
   git rev-parse --show-toplevel
   ```
   If absent, stop. Context Folding needs a project boundary.
2. Build the evidence trace:
   ```bash
   TRACE_JSON="${TMPDIR:-/tmp}/ccx-trace-$(date +%Y%m%d-%H%M%S).json"
   ccx trace ${SESSION_ID:-} --output "$TRACE_JSON"
   ```
   Use `--all` or `--project` only when the current workspace lookup misses.
3. Inspect the trace JSON before interpreting:
   - `kind` must be `ccx.trace.v1`
   - `warnings` must be copied into fold.html metadata
   - `workspace_context.documents` and `.knowledge` are evidence, not truth
4. Set output paths:
   ```bash
   FOLD_HTML="${TMPDIR:-/tmp}/ccx-context-fold-$(date +%Y%m%d-%H%M%S)-<slug>.html"
   KB_DIR="<git-root>/.ccx/knowledge"
   ```
5. Create `$KB_DIR/{decisions,discoveries,corrections,patterns}` only after
   the user approves writing knowledge entries.

### Phase 1: Build The Evidence Graph

Use the trace as the base graph. Do not re-parse raw JSONL unless the trace
has an explicit gap you need to inspect.

Evidence nodes:

- `exchange`: user anchor plus following assistant/tool activity
- `tool.call`: each trace tool call, with paths and mutation/read flags
- `git.commit`: commits in the session window
- `git.uncommitted`: dirty files at fold time
- `doc.section`: AGENTS, CLAUDE, CONTEXT, README, docs/adr, docs/design
- `prior.entry`: existing `.ccx/knowledge` entry
- `warning`: missing or partial evidence

Evidence edges:

- `caused`: exchange caused tool call or file mutation
- `produced`: mutation likely produced commit or dirty file
- `corrected`: user correction redirected agent behavior
- `verified_by`: command/test/tool output supports a claim
- `contradicted`: decision conflicts with prior doc or knowledge
- `superseded`: new decision replaces prior knowledge

Citation format:

```text
session:<session-id>#<exchange-index>
session:<session-id>#<message-uuid>
tool:<message-uuid>:<tool-name>
git:<commit-sha>
file:<path>:<line>
doc:<path>#<heading-slug>
kb:<entry-filename>
warning:<kind>
```

### Phase 2: Extract Decisions

Classify only claims supported by evidence. Use [DECISIONS.md](DECISIONS.md).

Provenance:

- **human**: user explicitly directed the choice
- **agent**: agent chose without explicit instruction for that choice
- **joint**: agent proposal plus acceptance or iterative discussion
- **correction**: human redirected or overrode agent behavior
- **inferred**: unclear; must be low confidence and reviewed

Attention:

- **high**: architecture, data model, security, public API, irreversible
- **mid**: library, pattern, workflow, error handling, testing approach
- **low**: formatting, naming, routine local implementation
- **discovery**: fact or constraint learned, not a choice
- **correction**: human override; always review

Before writing any knowledge entry, apply the three-gate bar:

1. Hard to reverse
2. Surprising without this context
3. Real tradeoff existed

Then apply the deletion test: if the next agent can derive it from code,
docs, or common sense, do not archive it.

### Phase 3: Human Review Plan

Present the fold plan in conversation before writing knowledge:

```text
Fold Plan -- session <slug> (<id>)
<date> | <duration> | <model> | <N> exchanges | <M> commits | <K> warnings

 #  Attention  Provenance  Kind        Decision / Discovery
 1  high       human       decision    Use SSE, not WebSocket, for live updates
 2  high       correction  correction  Do not mock database integration tests
 3  mid        agent       discovery   API returns 429 after 100 test-key requests

Needs confirmation:
- #2: inferred rule scope
- #3: source is one failed command; confidence medium
```

If the user edits the plan, update provenance and attention before writing.
Rejected items may appear in fold.html as "not archived" but must not enter
`.ccx/knowledge/`.

### Phase 4: Write Outputs

1. Generate `fold.html` from [HTML-REPORT.md](HTML-REPORT.md).
   It must be self-contained, spatial, and scannable.
2. Open `fold.html` in the browser and report the absolute path.
3. Write approved knowledge entries using [ARCHIVE.md](ARCHIVE.md).
4. Regenerate `.ccx/knowledge/index.md`.
5. Append one line to `.ccx/knowledge/log.md`.
6. Do not commit automatically unless the user asked for commits.

### Phase 5: Review Queue

Print the unresolved audit queue:

- Decisions needing human confirmation
- Agent decisions with high blast radius
- Corrections that should become project rules
- Prior knowledge that may be contradicted
- The single most important live wire

## Light Mode

No files. Print only:

- Top 3 decisions with provenance
- Top discovery or correction
- The live wire
- Whether a full fold is worth doing

## Lint Mode

Audit `.ccx/knowledge/`:

- stale entries contradicted by code/docs
- duplicate or superseded entries
- entries failing the three-gate bar
- vague filenames or missing provenance
- missing index/log links

## Multi-session

Current release: multi-session folding is manual. Run `ccx sessions` to pick
session IDs, run `ccx trace <session-id>` for each one, then merge the
decision list. De-duplicate by topic, keep the latest decision, and link
older entries via `supersedes`.

Do not advertise `/ccx-context-fold --since` as available until `ccx trace`
has a matching `--since` command.

## Hard Rules

- Do not present trace facts as decisions.
- Do not hide warnings; evidence gaps are part of the output.
- Do not archive low-attention implementation details.
- Do not overwrite existing knowledge silently; supersede it.
- Do not write full transcripts into fold.html or knowledge entries.
- Attribute human corrections exactly and preserve their force.
