# Design: Session connections

**Created**: 2026-08-18
**Status**: Accepted
**CLI**: `ccx related [session]`; `related` in `ccx trace --full`
**Follows**: docs/design/0005-evidence-citations-lessons-from-semantica.md
(principle 2: every claim carries a quote + location; principle 4:
supersedes vs derives-from)

## Problem

ccx treats sessions as islands. The evidence for what happened across a
piece of work — the handoff a later session picked up, the fork that
carried a conversation into a new session, the second agent working the
same files at the same time, the session that said "see 736a7bac" — is
all in the transcripts, but nothing joins it. Every "how did we get
here" question that spans sessions falls back to eyeballing timestamps.

The MineContext framing (capture → process → consume) puts this in the
process layer: capture is free (the JSONL is already there); the missing
processing is *connect*. Semantica's framing: a decision chain that
cannot cross a session boundary is truncated at exactly the point where
context was lost.

## What a connection is

A connection is a deterministic relation between two sessions, backed
by evidence a reader can walk to (message id, time, path, quote). No
LLM, no similarity scores. Relations are stated from the anchor
session's point of view:

| Relation | Signal | Evidence carried | Strength |
|---|---|---|---|
| `forked_from` / `fork_of` | the two transcripts share message UUIDs (Claude Code fork and `ccx fork` copy history verbatim); the earlier session is the origin | shared count, first shared uuid | strong |
| `mentions` / `mentioned_by` | conversation text contains ≥8 hex chars matching the other session's id prefix | message id, time, quote | strong |
| `handoff_from` / `handoff_to` | a baton file (`HANDOFF*.md`, `*/handoffs/*`, `*devlog*`, `PLAN.md`, `TODO.md`) written by one session and read by the other later | path, writer msg id + time, reader msg id + time | strong |
| `builds_on` / `built_on_by` | a workspace file edited by one session and read or edited by the other later | up to 5 paths with both anchors, total count | medium |
| `overlaps` | time windows intersect (concurrent agents) | overlap window | medium |
| `previous` / `next` | nearest earlier / later session in the same project | start time | weak |

Strength is a band, never a decimal (0005 principle 7). A pair can carry
several relations; the pair's strength is the strongest one.

Scope: sessions in the same project (workspace) across providers.
`--all` widens id resolution like `trace --all`; relation search stays
per project because file-based signals only mean something inside one
workspace.

## Surface

```
ccx related                 # latest workspace session
ccx related 736a7bac        # by id / prefix / @N
ccx related --json          # full evidence
ccx trace --full            # bundle gains "related"
```

Text output: one row per related session, strongest first, then by
time: STRENGTH, SESSION, RELATIONS, WHEN, EVIDENCE (one bounded line).
Silent-cap rule holds: `-n` limits rows with "showing N of M".

## Cost

Every session in the project is parsed once (parse cache makes repeats
cheap); scan runs on the same bounded worker pool as `search`. Progress
on stderr when it is a terminal.

## Non-goals

- Semantic similarity between sessions (skills' job).
- Cross-workspace file links (paths are only comparable inside one
  workspace).
- Persisting a graph. The relations are recomputed from the transcripts;
  the transcripts are the store.
