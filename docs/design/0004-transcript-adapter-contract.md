# Transcript adapter contract

ccx currently supports Claude Code, Codex, and Grok. The next target set
also includes cctrace, Gemini CLI, Kimi Code, opencode, pi, dsh, and
Cursor. That is not a request for seven more ad hoc parsers. Some sources
are conversations, some may be lineage/orchestration records, and some
may be derived evidence. Each source needs a discovery spike before ccx
classifies it.

The Codex 0.147 break exposed the architectural rule: provider wire
formats churn; renderers must depend on a stable ccx transcript contract,
and format drift must be visible rather than becoming an empty page.

## Layers

```text
provider homes / indexes / files
        |
        v
provider-native reader + versioned wire adapter
        |
        v
ccx Transcript + Lineage + Diagnostics
        |
        +--> legacy Message-tree projection (migration only)
        +--> turn/step analysis
        +--> terminal, web, export, search
```

Provider-native structs stay inside `internal/provider/<id>`. Do not add
one cross-provider union of every upstream event type. Shared code begins
after native records have been reduced to ccx semantics.

## Stable domain

The provider-neutral transcript needs these concepts explicitly:

- `SessionRef`: identity, provider, source path, time bounds, cwd, model,
  source format/version, archive state.
- `Transcript`: ordered turns/blocks plus a declared topology.
- `Topology`: linear turns, message tree, or linked child sessions. Do
  not force Claude branches and Codex child threads into one tree.
- `Block`: human text, assistant text, reasoning summary, tool call,
  tool result, compaction, system/meta, attachment, unknown.
- `Lineage`: parent/child session edges separate from transcript nesting.
- `Capabilities`: whether tokens, cost, errors, files, reasoning,
  branching, and resume metadata are actually reported. Missing is not
  zero.
- `Diagnostics`: malformed records, unknown variants, unsupported format,
  dropped records, and lossy projections. Diagnostics are findings and
  must reach CLI/web consumers.

`parser.Session` / `parser.Message` is currently the renderer contract and
is still Claude-shaped. Keep it as a compatibility projection while the
neutral domain lands; do not make it the native model for the next
provider.

## Adapter rules

1. Select an authoritative upstream source for each semantic class.
   Model request/response history, UI events, hooks, and tool lifecycle
   records are not interchangeable.
2. Record the observed source format and producer version. Prefer a real
   format discriminant; otherwise use explicit feature detection.
3. Unknown variants survive as diagnostics or `unknown` blocks. Never
   silently turn a non-empty source into an empty transcript.
4. Deduplicate by upstream stable ID. Timestamp/text heuristics are a
   documented legacy fallback only.
5. Preserve raw source anchors without exposing raw private envelopes in
   rendered conversation text.
6. Keep discovery quick parse and full parse semantically identical.
7. Do not infer cost, error state, or token semantics when the provider
   does not report them.
8. Provider homes remain read-only.

## Provider acceptance gate

Support is not complete until one sanitized fixture set and one shared
contract suite prove all applicable surfaces:

- format reference names producer version and observed artifacts;
- fixtures cover plain chat, tools, compaction/resume, and lineage where
  the provider has them;
- list metadata equals full-parse metadata;
- terminal view, web, export, search, and trace use the same transcript;
- unknown format/variant tests produce visible diagnostics;
- raw instruction, auth, and environment envelopes do not render as
  human turns;
- no unsupported metric is displayed as measured zero;
- a read-only live smoke passes before release.

The schema audit command must dispatch through the same provider adapter
registry. A separate hand-maintained Claude field list cannot enforce this
gate.

## Sequence

1. Land the Codex 0.147 adapter fix and regression fixture.
2. Introduce `Transcript`, `Topology`, `Capabilities`, and `Diagnostics`,
   plus a projection back to the current message tree.
3. Move Claude Code, Codex, and Grok behind the contract without changing
   their rendered output; add the shared acceptance suite.
4. Replace the Claude-only schema audit with provider-dispatched audits.
5. Spike each remaining target against real local artifacts, write its
   format reference, sanitize fixtures, then implement one adapter at a
   time. Do not promise verbs before the fixture proves the data exists.
6. Model dsh/cctrace orchestration or derived evidence as lineage/source
   layers if their artifacts are not native conversations; do not fake
   them into chat messages.

Target CLI versions observed for this planning snapshot (2026-08-17) are
inputs to the spikes, not compatibility promises: Claude Code 2.1.234,
cctrace 0.40.0, Codex 0.147.0, Gemini CLI 0.55.1, Grok CLI 1.0.5,
Kimi Code 0.36.1, opencode 1.18.18, pi 0.84.2, dsh 0.1.0-rc.7, and
Cursor 2026.08.11-e8db854.
