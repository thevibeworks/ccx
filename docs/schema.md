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
| `ccx.related.v1` | `ccx related --json`, `GET /api/related/<project>/<session>` | The anchor session's connections to the other sessions of its workspace: `related[]` of `{session_id, provider, summary, start, end, strength, relations[]}`, plus `total`/`shown`. Each relation is `{kind, count?, paths?, evidence[], truncated?}`; evidence items are `{session_id, message_id, time, path?, quote?}`. Kinds: `forked_from`/`fork_of`, `mentions`/`mentioned_by`, `handoff_from`/`handoff_to`, `builds_on`/`built_on_by`, `overlaps`, `previous`/`next`. Strength is `strong`/`medium`/`weak`. |
| `ccx.log.v1`     | `ccx log --json`             | Time-scoped records across sessions with pre-computed `days[]` / `providers[]` / `workspaces[]` aggregates. Record `kind` is provider-normalized: `user_prompt` and `assistant_message` are the visible conversation only (a subagent's brief — user-role text on a sidechain or in a `subagents/` file, written by the parent agent — is `agent_prompt`; Claude command markers/echoes/notifications are `command`/`command_output`/`notification`, injected meta is `meta`, compaction carriers `compact_summary`; Codex 0.147 raw `response_item` messages are `model_input`/`model_output` and duplicated legacy events `legacy_message`). `tool_call` records carry `tool`, `path` (when the call named one) and a `Tool: argument` `text`. With `--kind`/`--match`, `metrics.records` stays scope-wide, `metrics.records_matched` reports the narrowed count before any limit, and `truncated` is measured against the matched count. `-C N` adds up to N neighbors per match from the same transcript, flagged `"context": true`; they are returned but never counted in `records_matched` or the aggregates. |
| `ccx.insight.v1` | `ccx insight --json`         | The window digest: `sessions[]` (one per logical session — a Claude session and its subagent files fold together) with `prompts[]` (`{time, line, text}`, first 6 / last 2, `prompts_omitted`), `final_answer`, `edits` + `edited_paths[]` (capped, `edited_paths_truncated`), `commits`, `interrupts`, `denials`, `tokens`, `cost_usd`, `unpriced_tokens`, `cost_status`, `model`, `summary`, `relation`; rollups `workspaces[]` (with `session_ids[]`), `days[]`, `providers[]`, `models[]`; `metrics` including `cost_coverage` (priced tokens / all tokens), `priced_sessions`, `unpriced_sessions`. `records[]` only with `--records`. Cost and tokens are whole-session figures from the provider's accounting, never sliced to the window. |
| `ccx.me.v1`      | `ccx me --json`              | The human's profile, counted from prompt history alone (no session files, no model): `sources[]` (the history files read), `span` (first/last, prompts, `calendar_days`, `active_days`, `providers[]`), `tempo` (`hours[24]` by clock hour, weekday/weekend prompts per active day, `median_day_start`/`median_day_end`, `longest_streak`, `breaks[]` of 2+ idle days), `months[]` (prompts, active days, workspaces, `new_workspaces`, `median_workspaces_per_day`, `median_chars`, `cjk_share`/`question_share` over prose, `command_share`/`late_share` over all prompts, `median_day_end`, `terms{}`), `commands[]`, `repeats[]` (short prompts typed at least twice), `openers[]` (first word), `rewinds` (`count`, `paired`, `resent`, `resent_longer`/`resent_shorter`, `median_seconds_before`), `resends`, `workspaces` (`total`, `lifespans` by active days, `switches`, `top[]`), `terms[]` (one per `--term`). Shares are fractions 0-1. A day runs 05:00-05:00 (`tempo.day_start_hour`), so clock strings past `24:00` are after midnight. Workspace figures cover Claude Code only: Codex history records no workspace. |

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
  treat absence as zero, not as an error. Exceptions that are always
  present because scripts index them: `turns[].steps` (`[]`),
  `turns[].tool_counts` / `steps[].tool_counts` (`{}`), `cost_usd` on
  turns and steps, and every `sessions[]` field of `ccx.insight.v1`
  except the `omitempty`-marked optionals.
- A cost of 0 is never "free" on its own: check `cost_status`
  (`priced` / `partial` / `unpriced`) or `unpriced_tokens` wherever a
  cost is emitted. `unpriced` means ccx has no pricing row for the
  model (or, for Grok, declines to guess by contract).

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

### Human interventions

`interrupts` (the "[Request interrupted by user]" marker: the human
pressed stop) and `denials` (a tool call rejected at the permission
prompt) are counted on turns, steps, and `stats`; a denied call carries
`denied: true` on its evidence and is not counted as an error (it did
not run). Neither is a prompt: an interrupt never opens a turn.

### Facts, not judgment

Traces record what happened: text excerpts, tool calls, mutations,
errors, costs, git correlation. They deliberately carry no
correction/sentiment/importance flags — v1 shipped keyword-based
`correction_signals` and mislabeled background-task notifications as
user pushback. Interpretation belongs to the consumer reading
`user_text` with comprehension.

### Two records of the human

Session files and prompt history are different records with different
lifetimes. `log` and `insight` read session files. Claude Code deletes
those after `cleanupPeriodDays` (default 30, by file mtime) and never
touches `history.jsonl`; on one store that ran on the default until May,
January has 1,953 prompts on record and a transcript for 332 of them.
`me` reads history only, so its counts are whole where the
session-derived ones have holes. `ccx doctor` lists the holes. Do not
reconcile the two records by adding them; a prompt in a surviving
session is in both.

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
