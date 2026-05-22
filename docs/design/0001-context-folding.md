# Design: Context Folding

**Created**: 2026-05-20
**Status**: Accepted
**CLI**: `ccx trace`
**Skill**: `skills/ccx-context-fold/`

## Problem

Human-agent coding sessions generate valuable project knowledge, but it
evaporates because each system captures only one layer:

| System | Captures | Misses |
|---|---|---|
| Session JSONL | who said what, tools, timestamps | what shipped, what should persist |
| Git | what changed | why it changed, what was rejected |
| Docs/ADRs | approved rationale | tacit corrections, discoveries, agent choices |
| Agent memory | reusable hints | provenance and auditability |

The hard problem is not summarization. It is reconstructing decisions under
limited context: what was messy, what was processed, who decided, why that
choice beat alternatives, and what future agents must not relearn.

## Naming

The umbrella practice is **Context Folding**.

Names by layer:

- **ccx trace**: deterministic evidence extraction
- **ccx-context-fold**: interpretive decision and knowledge folding skill
- **fold.html**: human decision trail
- **.ccx/knowledge/**: agent knowledge archive

The CLI owns trace evidence; the skill owns decision folding. Keeping the
names separate prevents the tool from implying that deterministic extraction
has already made judgment calls.

## Core Insight

A session is a decision stream buried inside execution noise. Folding is
the closing move:

```text
session JSONL + workspace docs + git + prior knowledge
  -> ccx trace
  -> ccx-context-fold
  -> fold.html + .ccx/knowledge patch
```

The trace is factual. The fold is interpretive. Mixing them makes the tool
overclaim and hides the epistemic boundary.

## Outputs

### Human Output: Decision Trail

`fold.html` is a self-contained, browser-reviewable audit surface:

- high-attention decisions as cards
- provenance badges: human, agent, joint, correction, inferred
- confidence and citations
- corrections and discoveries separated from choices
- evidence gaps visible, not buried
- rejected alternatives and tradeoffs
- open live wires

This follows the HTML-effectiveness lesson: long linear Markdown collapses
attention; spatial HTML with hierarchy, badges, and collapsible excerpts is
better for review.

### Agent Output: Knowledge Patch

`.ccx/knowledge/` is a curated project knowledge base:

- `decisions/`
- `discoveries/`
- `corrections/`
- `patterns/`
- `index.md`
- `log.md`

This follows the LLM wiki lesson: future agents need maintained, scoped,
curated knowledge, not raw transcript search. Entries must pass:

1. hard to reverse
2. surprising without context
3. real tradeoff

Then they must pass the deletion test: if the next agent can derive it from
code, docs, or common sense, delete the draft.

## Deterministic Layer: `ccx trace`

`ccx trace` emits `ccx.trace.v1` JSON:

- session metadata and provider
- exchanges anchored by user prompts or commands
- tool calls with read/mutation flags and paths
- correction/mutation/reasoning/sidechain signals
- git repo root, branch, head, dirty files, commits in session window
- workspace context inventory: AGENTS, CLAUDE, CONTEXT, README, docs, KB
- explicit warnings for missing or partial evidence

It does not decide what mattered.

## Interpretive Layer: `ccx-context-fold`

The skill consumes `ccx trace` and reconstructs:

- decisions
- discoveries
- corrections
- provenance
- confidence
- rejected alternatives
- tradeoffs
- archive eligibility

It presents a fold plan before writing knowledge unless the user explicitly
requested non-interactive archiving.

## Design Decisions

### Session-first, not git-first

Git shows what shipped. The session shows why it happened, including
corrections and rejected paths. Commits are correlated evidence, not the
source of truth.

### Evidence before judgment

The CLI produces evidence with warnings. The skill makes judgments with
confidence labels. This protects future agents from mistaking pattern-match
summaries for facts.

### Decision provenance is the product

The central question is "who decided what and why?" not "what changed?"
Agent decisions are especially important because they are often invisible
unless surfaced explicitly.

### HTML for humans, Markdown for agents

Humans review spatially. Agents retrieve text. The system produces both
instead of forcing one format to serve both audiences.

### Knowledge must compound

The archive is not a diary. It is a maintained knowledge base with index,
log, superseding, linting, and promotion from repeated decisions to patterns.

## Alternatives

### Make the CLI produce decisions directly

Rejected. Deterministic Go can extract evidence, but decision
classification depends on context, judgment, and human audit. Keeping it in
the skill is more honest.

### Store everything in CLAUDE.md

Rejected. Flat memory files do not preserve provenance, superseding, or
entry lifecycle. `.ccx/knowledge/` is structured and queryable.

### Markdown-only report

Rejected. Decision folds become too long for linear reading. HTML provides
scannability, hierarchy, and collapsible citations.

## Influences

| Source | What we took |
|---|---|
| ccx | session tree model, provider normalization, read-only stance |
| html-effectiveness | self-contained spatial HTML for human review |
| Karpathy LLM wiki | curated, maintained knowledge base for future agents |
| Matt Pocock skills | short entry skill, deeper docs split by concern |
| ADR pattern | three-gate bar for durable entries |
| reflect skill | tension, observation, decision, tradeoff, next |
| dev-log-writer | decision tables, tags, name knowledge by content |

## Future Work

- `ccx trace --since` for multi-session bundles
- trace schema JSON Schema file
- knowledge lint command in ccx
- optional local HTML generator for fold plans after skill produces JSON
- team-level `.ccx/knowledge` aggregation
