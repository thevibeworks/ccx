# Agent Knowledge Archive

How ccx-context-fold writes, maintains, and queries the project knowledge
base.

## Directory structure

```
.ccx/knowledge/
  index.md                     # catalog of all entries
  log.md                       # append-only fold history
  traces/                      # optional durable trace manifests
  decisions/
    chose-sse-over-websocket.md
    raw-sql-only-no-orm.md
  discoveries/
    stripe-429-at-100-rpm.md
  corrections/
    no-mock-database-in-integration-tests.md
  patterns/
    embedded-database-single-binary.md
```

## Naming convention

Name for the knowledge, not the method or the date.

- GOOD: `chose-sse-over-websocket.md`
- GOOD: `stripe-429-at-100-rpm.md`
- BAD: `2026-05-20-streaming-decision.md`
- BAD: `session-abc123-findings.md`

The date lives in frontmatter. The filename is the knowledge slug.

Archive entries are not session notes. They are reusable constraints for
future agents.

**Collision handling**: if a file with the same slug already exists
from a prior fold, append `-2`, `-3`, etc. If the new entry covers
the same topic as the existing one, use `supersedes` instead of
creating a duplicate (see Superseding below).

## Entry format: decision

```markdown
---
type: decision
date: 2026-05-20
session: abc12345
provenance: joint
attention: high
tags: [arch, api]
commits: [abc1234, def5678]
files: [src/stream.go, src/handler.go]
confidence: high
trace:
  kind: ccx.trace.v1
  sha256: 5f70bf18...
supersedes: null
---

# Chose SSE over WebSocket for real-time updates

## Tension

Dashboard needed live updates. Two viable transport options.

## Decision

Server-Sent Events over a single HTTP connection. Unidirectional
(server to client) is sufficient — the dashboard only receives.

## Rejected

- **WebSocket**: bidirectional capability unused. Adds connection
  management complexity (reconnection, heartbeat, proxy traversal).
  Would require a WebSocket library dependency.
- **Polling**: simple but wastes bandwidth and adds latency. The
  dashboard shows sub-second updates.

## Tradeoff

SSE is HTTP/1.1 only on some browsers. Limited to ~6 concurrent
connections per domain in HTTP/1.1. Acceptable at current scale
(single dashboard tab). Revisit if multi-tab or mobile is needed.

## Source

Session abc12345, exchanges #8-11.
Trace hash: `5f70bf18...`.
```

## Entry format: correction

```markdown
---
type: correction
date: 2026-05-20
session: abc12345
tags: [test, data]
confidence: high
trace:
  kind: ccx.trace.v1
  sha256: 5f70bf18...
---

# Don't mock the database in integration tests

## What happened

Agent wrote integration tests with a mock database layer. User
overrode: prior incident where mock/prod schema divergence masked
a broken migration.

## Rule

Integration tests must hit a real database instance. In-memory
equivalents (PGLite, SQLite) are acceptable. Hand-rolled mocks
are not.

## Apply when

Writing tests that touch data persistence.

## Do not apply when

Unit tests for pure functions with no persistence dependency.
```

## Entry format: discovery

```markdown
---
type: discovery
date: 2026-05-20
session: abc12345
tags: [api, ops]
confidence: medium
trace:
  kind: ccx.trace.v1
  sha256: 5f70bf18...
---

# Stripe API rate-limits at 100 req/min on test keys

## Finding

Stripe returns HTTP 429 after 100 requests per minute on test-mode
API keys. Not documented in their public rate limit page.

## Implication

Batch operations against Stripe must include exponential backoff.
Sequential processing of >100 items hits the limit within 60s.

## Source

Session abc12345, exchange #14, tool result `tool:a1:Bash`.
```

## Entry format: pattern

Patterns emerge when 2+ independent decisions or discoveries share
a common rule. A single occurrence stays as its own entry. Patterns
require human approval before creation.

```markdown
---
type: pattern
date: 2026-05-20
status: candidate
source_entries: [chose-sse-over-websocket, chose-sse-for-log-tail]
tags: [arch, api]
---

# Prefer SSE over WebSocket for unidirectional streams

## Rule

When the data flow is server-to-client only, use SSE. Reserve
WebSocket for bidirectional communication.

## Apply when

Adding real-time updates to a read-only surface (dashboards,
log viewers, status pages).

## Do not apply when

The client needs to send structured messages to the server
(chat, collaborative editing, game state sync).
```

## Promotion rules

| Condition | Action |
|---|---|
| 1 occurrence | Keep as decision/discovery entry |
| 2 independent occurrences | Propose pattern promotion; require human approval |
| Human says "remember this" | Propose immediate promotion with source citation |

## Index format

Regenerated on every fold:

```markdown
# Knowledge Base

Last fold: 2026-05-20 | Entries: 14

## Decisions (7)

- [Chose SSE over WebSocket](decisions/chose-sse-over-websocket.md) -- joint, high, arch
- [Raw SQL only](decisions/raw-sql-only-no-orm.md) -- correction, high, data

## Discoveries (3)

- [Stripe 429 at 100 rpm](discoveries/stripe-429-at-100-rpm.md) -- api, ops

## Corrections (2)

- [No mock database](corrections/no-mock-database-in-integration-tests.md) -- test

## Patterns (2)

- [SSE for unidirectional](patterns/prefer-sse-unidirectional.md) -- arch, api
```

## Log format

Append-only, one line per fold:

```markdown
## [2026-05-20] context-fold | session abc12345 | trace ccx.trace.v1 | 5 decisions, 1 discovery, 1 correction
## [2026-05-19] context-fold | session def67890 | trace ccx.trace.v1 | 3 decisions, 0 discoveries, 0 corrections
```

## Superseding entries

When a new decision contradicts a prior entry on the same topic:

1. Set `supersedes: <prior-filename>` in the new entry's frontmatter
2. Add a note at the top of the old entry:
   `> Superseded by [<new>](<path>). Kept for historical context.`
3. Do NOT delete the old entry — it records what was true at the time

## Lint checklist

When invoked with lint mode:

1. **Contradictions**: entries with overlapping tags and conflicting
   conclusions. Present both to the user.
2. **Staleness**: entries older than 60 days referencing files no
   longer in the repo. Flag for review.
3. **Orphans**: entries with zero inbound references from other
   entries, CONTEXT.md, or ADRs.
4. **Failed deletion test**: entries where the conclusion is now
   obvious from code or conventions. Propose archival.
5. **Pattern candidates**: 2+ entries sharing tags and similar
   reasoning without a consolidated pattern.
6. **Vocabulary drift**: terms in entries that don't match CONTEXT.md
   definitions. Propose CONTEXT.md updates.

Output a report to conversation. Do not auto-fix.

## Write Policy

Default to a proposed patch. Write entries only when one of these is true:

- the user approved the fold plan
- the user explicitly said to remember or archive the item

Never overwrite an existing entry silently. Supersede instead.

## Trace Provenance

Do not store `/tmp/ccx-trace-...json` paths as durable provenance. Temp paths
expire. Store:

- session id
- exchange citations
- commit SHAs
- trace kind
- SHA-256 of the trace JSON used for folding

If the user asks for a durable evidence bundle, copy the trace JSON into
`.ccx/knowledge/traces/<session-id>-<hash>.json` and reference that relative
path. Otherwise the hash plus citations are the durable source.
