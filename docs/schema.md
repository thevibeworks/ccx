# ccx JSON schema contract

The JSON these commands emit is the product; the CLI surface is a
convenience over it. Skills and scripts are JSON consumers, so the
shapes below are versioned contracts, not incidental output.

## Kinds

Every top-level JSON document carries a `kind` string. That string is
the schema identity: consumers should dispatch on it and reject kinds
they do not know.

| Kind             | Emitted by                   | Contents |
|------------------|------------------------------|----------|
| `ccx.outline.v1` | `ccx trace --json`           | Session skeleton: every turn and step headline with rollups. Read this first; it always fits. |
| `ccx.turn.v1`    | `ccx trace --turn N`         | One turn with full step evidence, plus the sidechain entries that turn references, plus warnings. |
| `ccx.trace.v2`   | `ccx trace --full`           | Complete evidence bundle: all turns/steps, sidechains, git correlation, workspace context, `related` sessions, stats, warnings. Large. |
| `ccx.related.v1` | `ccx related --json`         | The anchor session's connections to the other sessions of its workspace: `related[]` of `{session_id, provider, summary, start, end, strength, relations[]}`, plus `total`/`shown`. Each relation is `{kind, count?, paths?, evidence[], truncated?}`; evidence items are `{session_id, message_id, time, path?, quote?}`. Kinds: `forked_from`/`fork_of`, `mentions`/`mentioned_by`, `handoff_from`/`handoff_to`, `builds_on`/`built_on_by`, `overlaps`, `previous`/`next`. Strength is `strong`/`medium`/`weak`. |
| `ccx.log.v1`     | `ccx log --json`, `ccx insight --json` | Time-scoped records across sessions with pre-computed `days[]` / `providers[]` / `workspaces[]` aggregates. |

## Versioning policy

- The version lives in the `kind` string (`.v1`, `.v2`). A consumer
  that matches on `kind` never silently reads a reshaped document.
- **Additive changes** (new optional fields) do NOT bump the version.
  Consumers must ignore unknown fields.
- **Breaking changes** (removing/renaming fields, changing semantics)
  bump the version and get a CHANGELOG entry naming every removed
  field. The v1 -> v2 trace migration (exchanges -> turns, correction
  signals deleted) predates this policy and was silent; that mistake
  is why this document exists (issue #19).
- Fields marked `omitempty` are absent when zero — consumers must
  treat absence as zero, not as an error.

## Semantics worth knowing

### Timestamps and timezones

All JSON timestamps are RFC 3339, almost always UTC. The *text*
outline renders times in the local timezone and says so in its header
(`times UTC+8`). When cross-referencing text output against JSON or
raw session JSONL, convert first.

### Duration vs active time

`stats.duration_seconds` is wall-span (session end minus start). A
35-day span can hold four days of work, so traces also carry
`active_seconds` (per turn and in stats): the sum of inter-message
gaps, each capped at 5 minutes. Continuous work counts fully; an
overnight gap counts as at most one cap. Use active time for "how
long did this actually take"; use wall-span for calendar placement.

### Facts, not judgment

Traces record what happened: text excerpts, tool calls, mutations,
errors, costs, git correlation. They deliberately carry no
correction/sentiment/importance flags — v1 shipped keyword-based
`correction_signals` and mislabeled background-task notifications as
user pushback. Interpretation belongs to the consumer reading
`user_text` with comprehension.

### Evidence budgets

Text fields are bounded (user text 2000 runes, narration 1200) with
head+tail excerpts and an explicit omission marker. `*_truncated`
flags report when content was cut; `anchor_id` / `message_id` point
at the full message in the session file for drill-down.

### Warnings are findings

`warnings[]` reports evidence gaps (no git repo found, correlation
failed, missing time window). Consumers should surface them, not
swallow them — a recap built on a trace with warnings inherits those
gaps.
