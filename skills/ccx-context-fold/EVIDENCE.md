# Evidence Graph

Context Folding starts from evidence, not narrative. `ccx trace` produces
the base bundle; the skill interprets it.

## Required Input

Run:

```bash
TRACE_JSON="${TMPDIR:-/tmp}/ccx-trace-$(date +%Y%m%d-%H%M%S).json"
ccx trace ${SESSION_ID:-} --output "$TRACE_JSON"
```

The JSON must include:

- `kind: ccx.trace.v1`
- `session`: id, provider, file path, cwd, model, timestamps
- `exchanges`: user anchors, assistant text, tools, files, signals
- `git`: repo root, dirty files, commits in session window
- `workspace_context`: metadata for docs and prior `.ccx/knowledge`
- `warnings`: missing evidence or partial correlation

If the trace command fails, stop. Do not fold a session without transcript
evidence.

## Evidence Status

Treat evidence by strength:

| Status | Meaning |
|---|---|
| `observed` | Directly present in trace JSON, git, or file contents |
| `correlated` | Connected by timestamp, file overlap, or adjacent exchange |
| `inferred` | Reasoned from evidence but not explicitly stated |
| `missing` | Expected source absent; appears in `warnings` or `workspace_context.missing` |

Fold output must label inferred or missing evidence. Do not write inferred
claims into the knowledge archive without human review.

## Node Types

| Type | Source | Use |
|---|---|---|
| `exchange` | `exchanges[]` | Conversation unit for decisions and corrections |
| `tool.call` | `exchanges[].tool_calls[]` | Mutation/read/action evidence |
| `signal` | `exchanges[].signals[]` | Hints: correction, mutation, reasoning, sidechain |
| `git.commit` | `git.commits[]` | Shipped change in session time window |
| `git.uncommitted` | `git.uncommitted_files[]` | Working tree evidence not yet committed |
| `doc.section` | `workspace_context.documents[]` | Local instructions, ADRs, design docs |
| `prior.entry` | `workspace_context.knowledge[]` | Existing durable knowledge |
| `warning` | `warnings[]` | Evidence gap to expose in HTML |

## Edge Types

| Edge | Meaning |
|---|---|
| `caused` | Exchange led to tool call or mutation |
| `produced` | Mutation produced commit or dirty file |
| `corrected` | User redirected a prior agent action or assumption |
| `verified_by` | Test/command/tool result supports a claim |
| `contradicted` | New evidence conflicts with docs or prior knowledge |
| `superseded` | New approved decision replaces old knowledge |

## Citation Format

Use stable, greppable citations:

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

For uncertain boundaries, mark uncertainty:

```text
session:abc123#~14 (post-compaction)
```

## Compaction

If the trace includes compaction or missing context warnings:

- put the warning in fold.html metadata
- avoid reconstructing unstated reasoning
- use git/file evidence only as support, not as proof of motivation
- mark affected decisions as low confidence unless the user explicitly
  restated the rationale after compaction

## Sidechains

Sidechain evidence is agent-produced unless the parent exchange contains
explicit human direction. A sub-agent dispatch itself can be a decision by
the initiator; internal sidechain choices are agent decisions.

Use summaries and tool/file evidence. Do not paste full sidechain transcripts
into fold.html or knowledge entries.
