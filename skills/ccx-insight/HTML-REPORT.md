# ccx-insight HTML Report

The HTML report is an audit cockpit for a `ccx.log.v1` evidence bundle. It is
not a blog post, a dashboard toy, or a prettier version of raw transcripts.

The job is to help a human answer five questions quickly:

1. What was actually worked on in this time window?
2. What is proven, human-stated, inferred, or still unverified?
3. What remains open and deserves the next push?
4. Where are the raw records behind each claim?
5. What caveats in the logs could mislead the review?

## Output Contract

Write one self-contained HTML file that can be opened with `file://`.

- Write to the OS temp directory by default:
  `/tmp/ccx-insight-<scope>-<timestamp>.html`.
- Write into the repo, for example `reports/`, only when the human asks for a
  durable project artifact.
- Inline CSS only.
- Inline JavaScript is allowed for filtering, searching, copying evidence refs,
  and opening detail panels.
- No remote fonts, CDNs, images, build steps, or external assets.
- The report must remain useful with JavaScript disabled.
- Keep the raw transcript hidden by default. Show concise snippets and exact
  evidence references instead.
- Add a print stylesheet. Print output should preserve the claim ledger,
  workstreams, timeline, caveats, and evidence refs.

## Before Writing

Regenerate the evidence bundle in the current environment. Do not reuse an old
`/tmp` file unless the human explicitly asks you to.

```bash
ccx log --scope <scope> --tz <timezone> --all --json
```

For custom ranges:

```bash
ccx log --since <start> --until <exclusive-end> --tz <timezone> --all --json
```

Then inspect the shape of the bundle before narrating it:

- `scope`: label, timezone, start, end, generated time
- `metrics`: records, returned records, workspaces, source files, truncation
- `sessions[]`: provider, workspace, source file, record count, relation
- `records[]`: provider, kind, role, timestamp, source file, line, text
- `records[].is_subagent`: Claude Code sidechain/subagent evidence
- `records[].kind == "compaction"`: lossy context, not proof

Call `metrics.sessions` "source log files" in the report unless you derive a
separate logical-session model. Agent containers can run for months; a daily
report is a timestamp slice through records, not a clean list of sessions.

## Information Architecture

### 1. Scope Header

Use the title `Session Intelligence`.

Include a compact metadata strip:

- Scope label and exact start/end timestamps
- Timezone
- Project scope: `all projects` or the workspace path
- Generated timestamp
- Evidence command
- Bundle kind/schema if present
- Truncation status

The header should make the boundary of the report impossible to miss.

### 2. TL;DR Judgment Band

Use one short paragraph and a confidence chip.

This is the human-facing synthesis. It should state the most important pattern,
the strongest correction to a naive reading of the logs, and the next pressure
point. Do not make this section a list of everything.

### 3. Data Quality Panel

Put caveats near the top, before the workstream narrative.

Show:

- Source log files
- Records returned / total matched
- Long-running containers
- Workspace count
- Provider split
- Subagent / sidechain count
- Codex duplicate-looking record warning, when applicable
- Claude compaction or continuation warning, when applicable
- Truncation warning, when applicable

This panel exists because bad session boundaries create bad product decisions.

### 4. Metrics Strip

Use tight, scannable metric cells with tabular numbers:

- Source log files
- Records
- Workspaces
- User prompts
- Assistant messages
- Tool calls
- Tool results
- Reasoning records
- Sidechains/subagents

Prefer precise labels over impressive labels. "52 source log files" is better
than "52 sessions" if the data is file-based.

### 5. Workstream Board

The workstream board is the primary review surface.

Use four columns:

- `In motion`
- `Needs closure`
- `Done / achieved`
- `Watch`

Group cards by workspace/project. Each card should include:

- Workstream title
- Workspace path
- Provider mix
- Source log file count
- Record count or cited record count
- Status
- Confidence
- What happened
- Why it matters
- Next action
- Evidence refs

Cards may be visually separated, but avoid nested cards. The report is not an
editor; do not add drag/drop state unless the human asked for triage editing.

### 6. Timeline

Use a vertical timeline for curated events, not every record.

Each event should include:

- Local scoped time
- Provider badge
- Workspace/project
- Record kind
- Short event text
- Evidence ref

Good timeline events include human corrections, explicit requirements, tool
execution evidence, merges/commits/builds/tests, blockers, and completion
signals. Generic assistant narration should usually stay out of the timeline.

### 7. Claim Ledger

Every consequential claim belongs in a ledger.

Columns:

- Claim
- Status
- Confidence
- Made by: human, agent, mixed, observed
- Why it is classified that way
- Evidence refs

Allowed statuses:

- `observed`: tool output, git state, or transcript record directly shows it
- `human-stated`: the human said it
- `agent-claimed`: an assistant said it, but no independent evidence was checked
- `inferred`: reasoned from multiple weak signals
- `unverified`: plausible but not checked
- `contradicted`: evidence conflicts

"Done / achieved" may include only `observed` or clearly cited `human-stated`
items. Assistant statements alone are never completion evidence.

### 8. Decisions And Corrections

Extract the thinking turns, not just the task turns.

For each decision or correction, show:

- What changed
- Who made the decision: human, agent, or mixed
- Rationale
- Rejected or corrected alternative, if visible
- Evidence refs

Human corrections deserve special weight. They often reveal the real product
boundary better than any agent summary.

### 9. Evidence Drawer

Add an evidence drawer or evidence table after the main analysis.

Each row should include:

- Evidence ref
- Provider
- Session/source prefix
- Timestamp
- Workspace
- Kind
- Source file path
- Line number
- Snippet

Use `details` / `summary` for expandable raw snippets. Never paste giant
transcript blocks into the default view.

### 10. Needs Closure

Open loops should be explicit and actionable.

For each item:

- Closure title
- Current state
- Why it is still open
- Suggested next command or review action
- Evidence refs
- Owner if the logs make one clear

Do not invent owners.

## Interaction Model

Use optional JavaScript for review acceleration only:

- Search across workstream cards and evidence rows
- Filter by status, provider, workspace, and confidence
- Toggle "show only unverified / contradicted claims"
- Copy all evidence refs for a card or claim
- Collapse/expand evidence details

Core reading must work without JavaScript. HTML-native `details` elements are
preferred for drawers because they degrade well.

Accessibility requirements:

- Use real buttons with `type="button"`
- Reflect active filter state with `aria-pressed`
- Preserve visible keyboard focus
- Do not encode status by color alone
- Keep contrast strong enough for long review sessions
- Make tables horizontally scrollable on narrow screens

## Visual Direction

When available, use `reference/html-effectiveness` as the local taste reference:

- `11-status-report.html`: quiet editorial density and metric rhythm
- `12-incident-report.html`: strong TL;DR band and timeline cadence
- `14-research-feature-explainer.html`: sticky navigation, details blocks,
  and research-style evidence hierarchy
- `18-editor-triage-board.html`: compact interactive board behavior
- `20-editor-prompt-tuner.html`: sticky controls and side-by-side review flow

Design language:

- Minimal, modern, and text-first
- Restrained palette: paper, ink, muted clay/olive/slate accents
- Thin rules, generous whitespace, tight cards, tabular numerals
- Sticky review controls are useful; marketing heroes are not
- Use small badges for provider, status, and confidence
- No gradients, decorative blobs, bokeh, fake glass, or oversized hero art
- No cards inside cards
- No remote icon libraries; text badges are enough

The report should feel like a serious instrument: calm, exact, and slightly
beautiful because the structure is right.

## Evidence References

Use this citation format:

```text
<provider>:<session-prefix> <local-time> <kind> <source-file>:<line>
```

Example:

```text
cx:019e440f 2026-05-21 23:41 tool_result /path/to/rollout.jsonl:2135
```

For space-constrained cards, use a short form and put the full path in the
evidence drawer:

```text
cx:019e440f:2135
```

## Real Data Traps

The report must account for provider-specific log behavior.

- Codex can emit duplicate-looking user and assistant records from both
  `event_msg` and `response_item`. Deduplicate meaning, not record counts.
- Claude Code can store sidechain/subagent JSONL files under a parent session.
  Treat those as evidence lanes, not independent human workstreams by default.
- `compaction` and continuation summaries preserve context but are lossy. They
  can explain direction; they cannot prove completion.
- `reasoning` and `usage` records are signal, not product evidence.
- Tool results, git output, file diffs, tests, builds, and explicit human
  statements carry more weight than assistant prose.
- Long-running containers may have started weeks or months earlier. Scope claims
  to records inside the selected time window.
