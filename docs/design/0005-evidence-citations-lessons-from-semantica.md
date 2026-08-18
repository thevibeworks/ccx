# Design: Evidence citations — what ccx takes from Semantica

**Created**: 2026-08-18
**Status**: Accepted (principles + shipped items); Proposed (roadmap)
**CLI**: `ccx search --hits`, `ccx search -w / --sort / FIRST`
**Source studied**: `reference/semantica-agi/semantica` @ 5c2901a (cloned
2026-08-18, 8.7k stars, "Graph-Native Infrastructure for Context and
Accountable AI Systems") and the org profile README.

## Why we looked

Semantica and ccx answer the same question for different subjects:
*what was decided, why, and what evidence supports it*. Semantica does
it for enterprise AI decisions (a Python graph library: context graph,
decision records, W3C PROV-O provenance, causal chains, temporal
queries). ccx does it for coding-agent sessions (a Go CLI over the
transcripts Claude Code / Codex / Grok leave behind). The overlap is
the methodology, not the code. This note records which of their
principles we adopt, how each maps onto ccx, and what we reject.

## Principles adopted

| # | Semantica principle (where) | ccx mapping | Status |
|---|---|---|---|
| 1 | Explain the observable trail, not the model's cognition. "Semantica explains what's *outside* the model: the context fed in, the decision produced, its provenance, the execution trail" (`docs/concepts.md:20`). | Already ccx's stance: `trace` is "a factual record with zero interpretation; judgment belongs to the skills" (`internal/trace/types.go:10`). We keep the line hard: CLI = facts, skills = judgment. | held |
| 2 | Every fact carries a **verbatim quote plus a location** (`ProvenanceEntry.source_quote`, `source_location`, `semantica/provenance/schemas.py`). A decision without a quotable span is an assertion, not a finding. | Search results were counts per session. `--hits` makes each match a citation: time, session, role, message id, quote (`ccx search --content -w --hits X`). `trace` already anchors steps to `message_id`/`tool_id`. Rule: any ccx output that names a fact carries (session, message id, time). | shipped |
| 3 | Decision = **scenario / reasoning / outcome** as separate fields, plus who decided (`decision_models.py:87`). | Trace already carries the triple implicitly: `Turn.user_text` (scenario), `Step.narration` (reasoning at the moment of decision), `Step.mutations` + linked commits (outcome), `session.model` (decision maker). Make the mapping explicit in the recap/retro skill prompts; no schema change. | doc |
| 4 | **Supersedes ≠ derives-from** (`previous_version_id` vs `derived_from_id`); retraction is a tombstone, not a delete (`prov:Invalidation`). | `Turn.superseded` / `superseded_by_turn` already keeps the edited-away prompt as evidence. Missing: cross-session lineage (fork/resume) as a derives-from edge. | partial |
| 5 | **Bi-temporal**: when a fact was true vs when it was recorded (`kg/temporal_model.py`). | `search` now reports `FIRST` (when a term entered the record) beside `LAST` (session activity); `--sort first` answers "when did we first say X". `log --scope` slices by record time. | shipped |
| 6 | Truncation is explicit, never silent (`trace_decision_causality` appends `{"truncated": true, ...}`). | Already a ccx rule ("showing N of M (raise with -n)"); `--hits` follows it. | held |
| 7 | Discrete strength bands + one-sentence interpretation from **one shared threshold function** (`utils/helpers.py:586`), not invented decimals. | For skills: state claim strength as verified / claimed / inferred (ccx-recap already does); never emit a confidence float the data cannot support. | held |
| 8 | Symptom-named regression tests as readability contracts (`test_add_decision_scenario_stored_as_content`: humans got an opaque id where prose was expected). | Adopt the naming: e.g. `TestSessionSearcherSummaryHitKeepsContent` (this session) names the symptom, not the function. | adopted |

## Rejected (for ccx)

- Graph database / embeddings / vector precedent search. ccx is a
  single static binary over files; lexical search with word
  boundaries answers "was X ever discussed" exactly, and skills do
  the semantic step.
- PROV-O / RDF export, policy engine, SHACL. Compliance theatre for our
  use; the transcript file *is* the primary evidence and ccx is
  read-only over it.
- Hash-chained records. Tamper evidence matters when the store is
  yours; ccx does not own the store.
- God objects and alias APIs (three public names per behavior). Keep
  one name per behavior; name by the user's question.
- Uncalibrated confidence floats multiplied through "decay". Ordering
  is useful; the decimals are not.

## Shipped in this pass (2026-08-18)

- `search -w/--word`; `FIRST` column / `first_hit` / `--sort hits|first|last`;
  `--hits` citations; summary hits keep content evidence; parallel scan
  (~10x) with progress; `-n` shorthand on `sessions`/`projects`/`log`.
  Devlog: `docs/devlog/2026-08-18-search-word-boundary-dogfood.org`.

## Roadmap (small, ordered)

1. `ccx log --match QUERY [-w]`: records containing a term inside a time
   window — the bi-temporal slice (`search --hits` is unbounded in
   time; `log` is bounded but cannot filter by term). Reuse
   `textMatcher`.
2. `ccx view <session> --at <message_id>` (or `--grep`): walk from a
   citation to its surrounding context without leaving ccx (open since
   `docs/devlog/2026-08-03-content-search-noise.org` finding 4).
3. `~/.claude/history.jsonl` as a `type: prompt` search source: the
   longest-lived evidence (prompts back to 2025-09) for "when did we
   first say X" once session files have been cleaned up.
4. Session lineage: `fork`/`resume` parents as derives-from edges in
   `sessions --json` and `trace`, so a decision chain can cross
   session boundaries.
5. Readability contract test for `trace`: every step carries
   human-readable narration or a mutation summary, never only ids.
