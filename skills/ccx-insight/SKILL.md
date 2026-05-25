---
name: ccx-insight
description: >
  Generate scoped intelligence summaries and human audit reports from ccx
  time-sliced session logs. Use when the human asks what happened today,
  yesterday, this week, this month, what still needs closure, what was
  achieved, or what patterns are emerging across agent work.
---

# ccx-insight

Session intelligence is not a session list. Agent sessions can last days or
months, so a scope like "yesterday" must slice the JSONL records by timestamp.

Use `ccx log` as the evidence layer. Use this skill as the interpretation
layer.

- `ccx sessions` tells you which containers exist.
- `ccx log` tells you what happened inside a time window.
- `ccx trace` gives deeper evidence for a single session.
- This skill synthesizes, checks claims, and writes the human report.

## Modes

| Trigger | Evidence command |
|---|---|
| `/ccx-insight` | `ccx log --scope today --all --json` |
| `/ccx-insight yesterday --tz +8` | `ccx log --scope yesterday --tz +8 --all --json` |
| `/ccx-insight week --all` | `ccx log --scope week --all --json` |
| `/ccx-insight --since 2026-05-21 --until 2026-05-22 --tz +8` | `ccx log --since 2026-05-21 --until 2026-05-22 --tz +8 --all --json` |
| "deep insight" | run `ccx log --json`, then `ccx trace` key sessions |
| "HTML report" | write a standalone HTML audit cockpit from the evidence bundle |

Use `--all` by default for broad questions like "what did I do yesterday?".
Use current-workspace scope only when the human names or implies the current
repo. The report header must state either "all projects" or "current workspace".

## Process

1. Pick the exact scope and timezone.
   - Default scope: `today`
   - Accepted scopes: `today`, `yesterday`, `week`, `month`, `quarter`, `year`
   - Preserve user timezone. Offset forms like `+8`, `+08:00`, and `UTC` are valid.
2. Collect log evidence:
   ```bash
   ccx log --scope <scope> --tz <timezone> --all --json
   ```
   Drop `--all` only for an explicitly workspace-scoped request. Use a
   project/path argument for one workspace, or `--provider cc|cx` when needed.
3. For custom ranges:
   ```bash
   ccx log --since <start> --until <exclusive-end> --tz <timezone> --all --json
   ```
4. Inspect `sessions[].relation` first. If `started_before_scope` or
   `ended_after_scope` is true, treat that session as a long-running container,
   not as work that began or ended in the scope.
5. Read records by kind:
   - `user_prompt`: human intent, corrections, requirements
   - `assistant_message`: agent claims and proposed status
   - `tool_call` / `tool_result`: execution evidence
   - `compaction` / `summary`: lossy memory, not proof
   - `reasoning`: signal, not auditable evidence
6. Verify important claims with `ccx trace <session-id>` or direct repo/git
   inspection before calling something complete, blocked, published, merged,
   or still open.
7. Before writing HTML, inspect provider/data-quality shape: source log files,
   long-running containers, workspace split, Codex duplicate-looking records,
   Claude sidechains/subagents, compaction records, and truncation.
8. Produce the briefing or report.

## Claim Ledger

Every important claim should be tagged:

- `observed`: tool output, git state, or transcript record directly shows it
- `agent-claimed`: an assistant said it, but no independent evidence was checked
- `human-stated`: the human said it
- `inferred`: reasoned from multiple weak signals
- `unverified`: plausible but not checked
- `contradicted`: evidence conflicts

"Completed / achieved" may include only `observed` or clearly cited
`human-stated` items. Assistant statements alone are not completion evidence.

## Briefing Shape

```text
Scope: Yesterday (2026-05-21), +08:00
Evidence: <N> records across <M> source log files, <K> long-running containers

What was worked on
- ...

Completed / achieved
- ...

Needs closure
- ...

Signals
- ...

Next move
- ...

Caveats
- ...
```

## HTML Report

When the human asks for a report, HTML report, human review surface, or
something inspired by `reference/html-effectiveness`, create a self-contained
HTML file. Write to the OS temp directory by default, and write into the repo
only when the human asks for a durable artifact. Follow
[HTML-REPORT.md](HTML-REPORT.md).

The HTML report is an audit cockpit over `ccx.log.v1`, not a decorative summary.
It should help the human review workstreams, claims, evidence, caveats, and open
loops without reading raw JSONL.

Required sections:

- Scope header: exact range, timezone, project scope, generated time, evidence command
- TL;DR judgment band: one concise paragraph with confidence
- Data quality panel: truncation, long-running containers, provider caveats
- Metrics: source log files, records, workspaces, prompts, tool calls/results
- Workstream board: in motion, needs closure, done/achieved, watch
- Timeline: curated timestamped events, not every record
- Claim ledger: status, confidence, who made the claim, evidence refs
- Decisions / corrections: who decided, what changed, why it matters
- Evidence drawer: compact cited records with source paths and line numbers
- Needs closure: open loops with cited records and next action
- Caveats: missing logs, heuristic labels, ambiguous session boundaries

Evidence references should include provider, session prefix, source JSONL path,
line number, and timestamp. Do not paste full transcripts.

Use optional inline JavaScript only for filtering, searching, copying evidence
refs, and expanding details. The report must remain readable without JavaScript.
Do not use remote assets, CDNs, gradients, decorative blobs, marketing heroes,
or cards inside cards.

Call `metrics.sessions` "source log files" unless you have derived a separate
logical-session model. A time-scoped report is a slice through records; it is
not proof that those containers began or ended inside the scope.

## Rules

- Do not equate "latest" with "active".
- Do not equate completion keywords with completed work.
- Do not summarize a month-long session as one day's work.
- Do not hide evidence gaps. They are part of the answer.
- Separate facts, inferences, and recommendations.
- Attribute human corrections exactly.
- Keep the conversational briefing short unless the human asked for deep detail.

## Mental Model

Insight is a steering surface, not a report card.

The point is to help the human decide where to push next: what changed, what is
stuck, what can be trusted, and what needs another look.
