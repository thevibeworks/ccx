# Design: ccx-fold — Session Decision Extraction

**Created**: 2026-05-20
**Status**: Accepted
**Skill**: `skills/ccx-fold/`

## Problem

Three systems capture different slices of "what happened and why" in
human-agent collaboration, and none capture the whole picture:

| System | Input | Captures | Misses |
|---|---|---|---|
| ccx | Session JSONL | Who said what, when | What actually shipped |
| dev-vibe-fold | Git log | What shipped | Why it was done that way |
| phase-act (PDCA) | process.jsonl | Structured decisions | Requires SCRUM lane discipline |

The raw session transcript — where thinking and deciding actually happen —
is treated as an audit trail nobody reads. Knowledge evaporates when the
context window compacts or the session ends.

## Core insight

A coding session is a decision stream buried in 1:20 signal-to-noise.
"Folding" collapses this into two artifacts:

1. **For humans**: a reviewable decision trail (fold.html)
2. **For agents**: a compounding knowledge base (.ccx/knowledge/)

The fold is the closing move of a session — raw transcript in,
structured knowledge out.

## Key design decisions

### Session-first, not git-first

Unlike dev-vibe-fold (starts from `git log`), ccx-fold starts from the
session transcript. This captures WHY. Git changes are correlated as
evidence, not as the primary source.

### Decision provenance is the product

The central question is not "what changed" but "who decided what and why."
Every decision is classified: human / agent / joint / correction.

### Three-gate quality bar (from Matt Pocock's ADR pattern)

Before any KB entry: (1) hard to reverse, (2) surprising without context,
(3) real tradeoff. All three required. Prevents the KB from becoming a
session diary.

### Deletion test

After drafting a KB entry: "would the next agent make a worse decision
without this?" If derivable from code or conventions, delete the draft.

### Scene reconstruction (from /reflect)

High-attention decisions use the reflect model: tension, observation,
decision, tradeoff, next. Don't log facts — reconstruct the moment.

### Dual-format output (from html-effectiveness research)

Markdown dies at ~100 lines for human review. HTML survives via spatial
hierarchy, collapsible sections, and visual badges. Agents need structured
markdown with frontmatter. Different audiences, different formats.

### Compounding KB (from Karpathy LLM Wiki)

Each fold adds to a persistent knowledge base. Index, log, cross-references,
superseding, and lint keep it healthy. The fold is an ingest operation, not
a disposable report.

## Alternatives considered

### Extend dev-vibe-fold to read sessions

Rejected: dev-vibe-fold is git-centric by design. Bolting session parsing
onto it would violate its architectural assumption (commits are the source
of truth). Better to keep them complementary — different primary sources,
compatible knowledge formats.

### Build into ccx Go binary

Partially accepted: the Go binary can do deterministic work (parsing,
correlation, HTML scaffolding). But decision classification requires LLM
reasoning. The skill calls the Go binary for parsing and adds its own
classification layer. Future: `ccx fold` CLI command for the deterministic
parts.

### Single output format (markdown only)

Rejected: html-effectiveness research shows structured HTML materially
improves human engagement at document lengths >100 lines. A fold with
9 decisions, excerpts, and diffs easily exceeds that. Agents need
structured markdown. Two audiences, two formats.

### Knowledge entries in CLAUDE.md memory

Rejected: CLAUDE.md memory is flat (no index, no lint, no superseding).
The knowledge base needs structure to compound. Entries could optionally
be copied to memory, but the KB is the source of truth.

## Influences

| Source | What we took |
|---|---|
| dev-vibe-fold | Fold as closing move; pattern harvest; retro note format |
| phase-act.md | 4-tier attention model (high/mid/low + metadata) |
| /reflect skill | Scene reconstruction: tension/observation/decision/tradeoff/next |
| dev-log-writer | Decision table format; tag system; "name for knowledge, not method" |
| Karpathy LLM Wiki | Compounding KB; index.md + log.md; ingest/query/lint operations |
| html-effectiveness | Self-contained HTML; spatial > sequential; ~100 line readability cliff |
| Matt Pocock skills | Three-gate ADR bar; CONTEXT.md shared language; write-a-skill structure |
| ccx | Session tree model; read-only philosophy; single-binary; CSS palette |

## File structure

```
skills/ccx-fold/
  SKILL.md           129 lines   Process, modes, error handling
  EVIDENCE.md        131 lines   Inputs, evidence graph, citations
  DECISIONS.md       137 lines   Detection, provenance, three-gate bar
  HTML-REPORT.md     145 lines   Page structure, card templates
  ARCHIVE.md         242 lines   KB format, naming, promotion, lint
```

## Future work

- `ccx fold` Go CLI command for deterministic parsing + HTML generation
- CONTEXT.md vocabulary drift detection during fold
- Cross-project knowledge linking
- Team-level KB aggregation
- Automated fold trigger via Claude Code hooks
