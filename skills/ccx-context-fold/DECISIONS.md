# Decision Extraction

How ccx-context-fold detects, classifies, and filters decisions from a
`ccx trace` evidence bundle.

## Detection heuristics

Not every exchange contains a decision. Most exchanges are mechanical
execution. A decision exists when direction changed, a choice was made
between alternatives, or an agent silently chose an approach that future
agents would otherwise repeat or undo.

### Human direction

User message contains an imperative or explicit choice:
"use X", "switch to Y", "don't do Z", "go with A over B".

Provenance: **human**.

### Agent proposal accepted

Agent says "I'll do X" or "I recommend Y". User's next message is
acceptance (explicit: "yes", "go ahead"; implicit: proceeds without
objection).

Provenance: **joint**.

### Agent autonomous action

Agent modifies files (`tool_calls[].mutates_workspace == true`) without
prior explicit instruction for that specific choice. The trace `mutation`
signal is only a hint; classify as a decision only when there was a real
choice, not every edit.

Provenance: **agent**. These are the highest-value decisions to
surface — invisible without the fold.

### Human correction

User contradicts, redirects, or negates the previous agent action:
"no", "don't", "actually", "wait", "not that way", "revert".

Provenance: **correction**. Always high attention.

The trace `correction` signal is lexical. Verify the surrounding exchange
before treating it as a rule.

### Design discussion

Extended back-and-forth (3+ exchanges on the same topic) about
approach or tradeoffs. Often contains "should we", "what about",
"tradeoff", "option A vs B".

Provenance: **joint**.

## Discovery detection

A discovery is a fact uncovered during the session — not a choice, but a
constraint or behavior learned. Signals:

- Tool result contains an error that changed the approach
- Assistant text after an error: "turns out", "the issue was",
  "this means", "found that", "gotcha"
- A measurement (benchmark, test, API response) that informed a
  subsequent decision
- A `warnings[]` gap that changed confidence or scope
- Dirty git state that shows work not captured by commits

## Provenance labels

| Label | Meaning |
|---|---|
| `human` | User explicitly directed the choice |
| `agent` | Agent chose without prior instruction |
| `joint` | Proposal + acceptance, or iterative discussion |
| `correction` | Human overrode agent behavior |
| `inferred` | Unclear from transcript; mark confidence low |

## Confidence Labels

| Label | Meaning |
|---|---|
| `high` | Direct user statement, explicit agent proposal, or verified tool output |
| `medium` | Supported by adjacent evidence but not stated directly |
| `low` | Plausible inference; needs human confirmation |

## Attention tiers

| Tier | Criteria | In fold.html | In KB |
|---|---|---|---|
| high | Architecture, data model, security, public API, irreversible | Full card with scene | Entry (if passes three-gate) |
| mid | Library choice, design pattern, approach, error handling | Compact card | Entry (if passes three-gate) |
| low | Formatting, imports, variable names, routine | Collapsed list | Skip |
| correction | Any human override of agent | Warning card | Entry (always — corrections are inherently surprising and hard to re-derive) |
| discovery | Constraint, bug, or behavior learned | Callout | Entry (if non-obvious from code/docs) |

## Three-gate bar

Before writing ANY knowledge base entry (regardless of attention
tier), all three must hold:

1. **Hard to reverse** — changing this later is expensive (schema,
   API, architecture, data migration). If the fix is editing one line,
   skip the entry.

2. **Surprising without context** — a future reader would wonder
   "why?" If the choice is obvious from conventions or the code, skip.

3. **Real tradeoff** — genuine alternatives existed and were rejected.
   If there was only one sensible option, skip.

Corrections always pass gate 2 (they record a non-obvious constraint
the agent violated). They pass gate 3 (the agent chose the rejected
alternative). Gate 1 is the only real filter for corrections — if the
correction is "fix the typo," it fails gate 1 and gets skipped.

Decisions that fail any gate still appear in fold.html for human
review. They just don't enter the knowledge base.

Evidence gaps lower confidence. A high-attention decision with missing
evidence can appear in fold.html, but do not archive it until the human
confirms the rationale.

## Deletion test

After drafting a KB entry, ask: "If this entry didn't exist, would
the next agent make a worse decision?" If the conclusion is derivable
from code, conventions, or common sense — delete the draft. Only
persist knowledge that prevents dead-end re-exploration.

## Scene reconstruction

For high-attention decisions, reconstruct the moment using the
reflect model. Do not just log the fact; write the scene:

- **Tension**: What was broken, stuck, or wrong?
- **Observation**: What was actually seen (error, benchmark, code)?
- **Decision**: What was chosen AND what was rejected, with reasons.
- **Tradeoff**: What was given up. What pressure will build.
- **Next**: The live wire — what's still open.

Mid-attention decisions get Decision + Rejected only (no full scene).

## Tags

| Tag | When |
|---|---|
| `arch` | Module boundaries, data model, system shape |
| `security` | Auth, authz, crypto, secrets |
| `data` | Schema, migration, storage, query pattern |
| `api` | Public interface, protocol, contract |
| `perf` | Performance, caching, optimization |
| `ux` | User-facing behavior, UI, CLI |
| `test` | Testing strategy, coverage, fixtures |
| `ops` | Deploy, infra, monitoring, CI/CD |
| `process` | Workflow, conventions, tooling |
