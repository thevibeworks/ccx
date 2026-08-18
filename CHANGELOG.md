# Changelog

All notable changes to ccx are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [Unreleased]

### Added
- **`ccx related [session]`: which sessions connect to this one, and how.** Sessions were islands; the joins were in the transcripts but nothing computed them. `related` derives, deterministically and with evidence, the anchor session's connections to the other sessions of its workspace: `forked_from`/`fork_of` (the transcripts share message ids — Claude Code fork and `ccx fork` copy history verbatim), `mentions`/`mentioned_by` (a session id named in conversation text, quoted), `handoff_from`/`handoff_to` (a baton file — HANDOFF.md, handoffs/, devlog, PLAN.md — written by one and read by the other later), `builds_on`/`built_on_by` (a workspace file edited by one, then read or edited by the other), `overlaps` (concurrent), `previous`/`next`. Strength is a band (strong/medium/weak), never a score; path lists are capped with the count kept and `truncated` set; `--json` (`ccx.related.v1`) carries message id, time, path, and quote per relation. `ccx trace --full` gains the same list as `related`. Design: docs/design/0006-session-connections.md.
- **`ccx search -w/--word` matches whole words.** Matching was substring-only, so a term that prefixes a common word was unanswerable: `search --content semantica` returned 47 sessions, 46 of them "semantic*ally*", and ccx alone could not tell 0 real hits from 46 (docs/devlog/2026-08-18-search-word-boundary-dogfood.org). `-w` demands an ASCII word boundary on each side of the query that starts/ends with a word character (so "semantica-agi" and "(semantica)" still hit; CJK queries are unaffected) and applies to names, summaries, conversation text, and `--raw` lines alike.
- **`ccx search --hits` turns matches into citations.** One row per matching message — time, session, role, message id, quote — oldest first across sessions, `-n`-capped with a visible "showing N of M". The anchors are the same ones `trace` and `view` use, so a claim built on a search can point at its evidence (design: docs/design/0005-evidence-citations-lessons-from-semantica.md). Under `--raw` the unit is a transcript line, anchored by its own `uuid`/`type`/`timestamp`.
- **`ccx search --content` reports when: `FIRST` column, `first_hit` in `--json`, `--sort first|last|hits`.** "When did we first mention X" needs the earliest matching message and an oldest-first order; results only carried session end time and ranked by hit count. Each content hit now records the timestamp of its earliest matching message (parsed messages by default; the raw line's top-level `timestamp` under `--raw`), printed as `FIRST` and sortable with `--sort first`; `--sort last` orders by session activity; `--sort hits` is the old order and the default.

### Added
- **Human interventions are first-class in `trace` and `log`.** "[Request interrupted by user]" (the human pressed stop) and permission-prompt rejections ("The user doesn't want to proceed with this tool use") are the human in the loop, but ccx read the first as a *prompt* — it opened a fake turn `u: [Request interrupted by user for tool use]` — and the second as an ordinary tool error. New parser kind `interrupt` (harness marker, never an exchange anchor) and `parser.IsToolDenial`; `trace` counts `interrupts` and `denials` on turn, step, and stats, marks the rejected call `denied` (not an error, it did not run), and badges them in the outline header and rows (`1 interrupt, 2 denied`; step `[4t 1! 1d]`); `log` reports kinds `interrupt` and `tool_denied` (`ccx log --scope today --all --kind interrupt,tool_denied`). 650 interruptions across 321 sessions and 145 rejections in one real store were invisible before.

- **`GET /api/related/<project>/<session>` serves session connections as JSON** — the `ccx.related.v1` envelope (`related`, `total`, `shown`, `warnings`; `?limit=N`), computed by the same `trace.RelateWorkspace` the CLI uses, so a web panel or an agent reading the API sees exactly what `ccx related --json` prints. Fetched on demand: it costs a parse of every workspace session (cached after the first call).
- **`ccx search --content` scans prompt history.** Claude Code and Codex append every human prompt to `history.jsonl`, and those files outlive session cleanup (one real store: prompts back to 2025-09-28, sessions back to 2025-12-07). Matches surface as type `prompt` — only for prompts whose session file is no longer in the store, so a prompt never appears twice — with `FIRST`, a `[user]` quote, and under `--hits` a `history:line` anchor. "When did we first say X" now reaches past the session horizon.
- **`ccx view --at MESSAGE_ID [--context N]` walks from a citation to its context.** Search `--hits` and `trace` hand out message ids, but nothing in the CLI could open one; drill-down meant the web page or raw grep (open since docs/devlog/2026-08-03-content-search-noise.org finding 4). `--at` renders the cited message with N messages before and after it (wire order, flattened; the target survives `--brief`), and says where it sits: `message 1 of 763`. Prefixes resolve; ambiguous prefixes are an error, not a guess.

### Changed
- **`ccx trace` labels narration-less steps.** A step the agent never narrated (straight to tools) rendered as a bare badge row `1.   [4t]`; the outline now says what it ran: `(no narration) Bash x3, Read` (docs/devlog/2026-08-17-codex-0147-rollout-drift.org finding 3).
- **`ccx log --kind` and `--match PHRASE [-w]` turn the firehose into a timeline.** `log` emitted every record in scope (16k for one day) with no way to narrow; "what did the humans ask today, across every session" was not answerable. `--kind user_prompt,assistant_message` keeps only those kinds; `--match` keeps records whose raw transcript line contains the phrase (grep parity, `-w` for whole words) — the time-bounded complement of `search --hits`. Metrics stay honest: `records` is scope-wide, new `records_matched` is the narrowed count, `showing` is after `-n`.
- **`ccx search --content` is ~10x faster and shows progress.** The scan ran on one core and lowercased every transcript line: 6m18s cold / 1m14s warm over a 3.5 GB store, silent throughout. Sessions now scan on a bounded worker pool (up to 8), the raw prefilter matches case-insensitively without allocating and stops at the first hit, and a `scanning N/M sessions` line ticks on stderr when it is a terminal. Same store, warm: 7.6s.

### Fixed
- **`ccx log` now applies the same conversation rules as the parser.** Two classes of user-role lines were reported as `user_prompt`: Claude harness wrappers (slash-command markers, `<local-command-*>` echoes, task notifications), injected meta messages (skill bodies), and compaction carriers — 233 "prompts" today of which 79 were typed by a human; and Codex 0.147 raw `response_item` messages, including the injected AGENTS.md envelope, while the real `item_completed` UserMessage/AgentMessage rows rendered as bare `item_completed` with no text. `sessionlog` now reuses `parser.ClassifyUserText` and the Codex TurnItem decoder (`DecodeCompletedTurnMessage`, exported), demotes legacy/raw duplicates in a 0.147 rollout to `legacy_message`/`model_input`/`model_output` (records kept, counts fixed), and re-tallies session kinds and preview after demotion. `user_prompts`/`assistant_messages` metrics and `insight` reports built on them count the conversation once.
- **`ccx log` previews Claude tool results.** A `tool_result` block's payload lives under `content`; the preview only looked at `text`/`message`, so every Claude tool result row read as the literal word `tool_result`.
- **Heredoc bodies no longer count as shell redirects.** `extractRedirectPaths` scanned the whole Bash command, so a Go `if n > 0` or a markdown `> 2026-08-18` inside `python3 - <<'EOF'` / `cat > f <<'EOF'` became "edited files" (`.../0`, `.../2026-08-18`) in `trace` `files_edited` and in session connections. Heredoc bodies are stripped before the redirect scan.
- **`-n` is the `--limit` shorthand everywhere.** Only `search` had it; `sessions -n 2` failed with "unknown shorthand flag". `sessions`, `projects`, and `log` now accept `-n` too.
- **A session whose summary matched dropped its content evidence.** The summary hit short-circuited the content scan, so under `--content` the session where the term was actually discussed could be the one result with no hit count, previews, or first-hit time. Summary hits stay typed `session` but now carry the content fields.
- **Codex 0.147 conversations render again.** Codex moved its UI-facing user and assistant records from legacy `event_msg.user_message` / `agent_message` events to canonical `event_msg.item_completed` TurnItems. The Codex adapter now selects one conversation source per rollout, reads stable `UserMessage` / `AgentMessage` item IDs and content, keeps legacy rollouts working, and never mistakes raw `response_item` model input (which can contain injected instruction envelopes) for a human prompt. Discovery metadata, terminal view, web, export, search, and trace now agree; the parse-cache format is bumped so upgrades cannot serve blank cached sessions.

## [0.15.0] - 2026-08-11

### Added
- **Global sessions page: `/sessions` lists every session across every project.** The web UI could only list sessions per project, so "what ran yesterday, everywhere" meant visiting each project page. The new page is the cross-project cockpit: filter by text (summary substring or session-ID prefix), provider, project, model, and date range; group by project, day, provider, or model under sticky headers with per-group aggregates (count, projects, tokens); sort by recency, messages, prompts, or tokens. Dense two-line rows keep every sort key visible as an aligned column (design: docs/design/0003-global-sessions.md). State lives entirely in the URL so filtered views are shareable; active filters render as chips clearable per-param; the whole control bar is a real GET form that works without JavaScript. Default window is 100 sessions with an honest `Showing X of Y` footer and explicit show-more links.
- **`GET /api/sessions` (no project suffix) serves the same query as JSON** — filter/group/sort/limit params identical to the page, wrapped in an envelope with `total` and `shown` so truncation is visible. The per-project `/api/sessions/<project>` keeps its existing bare-array shape.
- **`--sort tokens` for `ccx sessions`.** The web sort landed in the shared catalog layer, so the CLI gains it too: ranks by input+output tokens (cache reads excluded — they would double-count long sessions).

### Fixed
- **Dark-mode provider badges were white-on-light (~2.1:1 contrast).** Dark-theme accent hues are light; badge text now uses the page ground color in dark mode.
- **`.empty-state` only existed inside the memory-section CSS**, so any other page rendering it got unstyled body text. Promoted to the shared stylesheet.
- **The advertised `d` theme shortcut now works on `/sessions`.** The top-nav button has said "Toggle theme (d)" since the shell redesign, but no page bound the key.

## [0.14.0] - 2026-08-03

### Changed
- **`ccx search` shows last-activity time, labeled `LAST`.** The old `TIME` column showed session start while `--after`/`--before` filter on end time, so a 7-day session could pass `--after 2026-07-19` yet display `2026-07-15` — reading as a broken filter until you traced the session. The column now shows the timestamp the filter matches and results sort by. (dogfood finding, docs/devlog/2026-08-03-trace-session-dogfood.org)
- **`ccx search --content` now ranks conversation, not boilerplate.** The v0.13.0 raw-line scan counted every transcript line, so injected noise dominated ranking: a Stop-hook line fired every turn put a 327-hit session first while the session where the topic was actually designed ranked #3 (docs/devlog/2026-08-03-content-search-noise.org). The default now parses each candidate session (all providers, sidechains included) and counts only conversation text — user prompts and assistant text/thinking; hook attachments, tool results, command echoes, and meta lines contribute zero. New `--raw` keeps the old behavior verbatim: grep parity over raw lines, no parse, misses nothing grep would find. A cheap raw-line prefilter keeps the default at par with `--raw` speed (only hit files pay the parse); queries whose bytes JSON-escaping could hide (`"`, `\`, non-ASCII) skip the prefilter for correctness.

### Added
- **`ccx sessions` recovers from near-miss lookups instead of dead-ending.** Sessions are keyed by exact session cwd, so a path one level below the real workspace root returned a bare "No sessions found". Three-part fix: project-name lookup slug-folds both sides (`260715_ccx-session-watch` finds slug `260715-ccx-session-watch` without guessing the mapping); on zero hits, path-like queries walk up parent directories and show the nearest ancestor workspace with a note; if still empty, the closest project slugs are suggested by token overlap. `ccx projects` gains a `PATH` column (table and `--json`) so the slug ↔ directory mapping is visible.
- **`ccx search` explains itself on zero results.** Help now states phrase semantics (multiple words match adjacent and in order, exit 0 either way); zero results print hints — try a single term, try `--content`. Previously a multi-word query silently over-narrowed to nothing with zero guidance.
- **Content results show what matched and where the file lives.** Each `--content` result now carries a role-labeled matched-text snippet (`[user]`/`[assistant]`/`[agent]`) in the table, and `--json` gains `path` (session/content results) plus up to 3 `previews` — noise is distinguishable from signal, and drill-down no longer needs `find` + `grep` outside ccx.

### Fixed
- **From-source builds stamp a real version.** `make build` defaulted VERSION to `dev`, making every source build indistinguishable when debugging "which ccx am I running". VERSION now defaults to `git describe --tags --dirty --always` (e.g. `v0.13.0-1-g3e3a235-dirty`); releases still override it.
- **One git-root warning per trace, not two.** Every `ccx trace` against an archived workspace printed both `session_git_root_missing` and `git_root_missing` — two lines saying one thing (the cwd is gone). The generic line now fires only when there was no session cwd to blame.
- **`-p/--provider` help lists `gx`** for `sessions`, `search`, and `insight` (grok worked but was undocumented in the flag help). `ccx log` keeps `cc, cx, all`: it truly has no grok source wired yet.
- **`make build` refreshes a stale root `./ccx`.** A bare `go build ./cmd/ccx` drops `./ccx` at the repo root where `.gitignore` hides it; `make build` writes `bin/ccx`, so the root copy silently went stale and shadowed fresh builds ("built unknown"). `make build` now overwrites the root copy when one exists.

## [0.13.0] - 2026-08-02

### Added
- goal attribution (#24): `ccx sessions` joins deva launch receipts
  (`$XDG_DATA_HOME/ccx/launches/*.jsonl`, written by `deva --goal`,
  thevibeworks/deva#499) to sessions by cwd + time. A receipt stamps
  sessions in the same cwd starting within [ts-5m, ts+24h]; latest
  qualifying receipt wins. `--json` output gains `goal`; new `--goal
  SLUG` filter. Receipts are advisory: missing dir or malformed lines
  never error, and session files stay untouched.
- **Mutation evidence carries the command it ran.** `ToolCallEvidence` gains a bounded one-line `summary` for command-carrying tools (Bash and the provider dialects normalized onto the `command` input). Paths answered "did this step touch the workspace" while hiding *how* — auditing a Bash mutation meant opening the raw JSONL. (#21)
- **`ccx trace --width N` controls outline headline truncation** (0 = untruncated; default stays 160), applied to text and JSON outlines alike. Previously a constant, and the only escape was `--full`'s entire bundle — which bit JSON skill/script consumers hardest. (#23)
- **`ccx search --content` scans transcript lines.** Search covered names, paths, and summaries only, so the main session-mining question — "what did we discuss about X" — fell back to raw `grep -r` over the store, losing session identity, provider abstraction, and date filters. `--content` streams every candidate session file plus its subagent files (grep parity by design: raw-line match with unbounded line reads, no parse, so every provider's format works and nothing grep finds is missed — including matches past lines too large for any fixed scanner budget), ranks results by hit count, and composes with `--after`/`--before`/`-p`/`--model`. Truncated result lists say so on stderr. It is a crawl (~15s over a multi-GB store) because no message index exists yet; a content index is the follow-up that would make it a query.

### Fixed
- **Git-root fallback is provenance, not a warning.** `session_git_root_missing` fired whenever the session's recorded cwd didn't resolve locally, even when the process-cwd fallback then found the right repo — the common case in containers, where every trace carried the warning despite correct correlation. A successful fallback now records `git.resolved_from` (`"session_cwd"` | `"process_cwd"`); the warning fires only when nothing resolves. (#22)
- **Codex cost no longer double-bills cached input and reasoning tokens.** Codex usage fields are subsets, not disjoint categories: `input_tokens` includes `cached_input_tokens` (upstream: `non_cached_input = input - cached`) and `output_tokens` includes reasoning (OpenAI `output_tokens_details`) — ccx billed every field separately, overstating a real 36-minute session 2.6x ($101.99 shown, $38.54 honest) and printing "6.6m in" for ~206k of uncached input while Claude's `in` excludes cache. The Codex backend now normalizes to the exclusive semantics the rest of ccx assumes; `ComputeCost` drops the reasoning term; cache format bumped so upgrades reparse. Found by the first cross-provider field eval. (#27)
- **Trace sidechain reports are whole in JSON evidence.** `--full` and `--turn` capped `sidechains[].summary` at 240 runes — for research sessions the subagent final report IS the value, so the "complete trace bundle" was incomplete exactly where it mattered. The top-level sidechains list now carries the untruncated (ANSI-stripped) report; step-level entries stay bounded light refs keyed by `agent_id`, as documented.
- **`ccx view` respects pipes: `--color=auto|always|never`.** ANSI codes were emitted unconditionally, so piped output read as binary to grep — silent false-negatives unless you knew to add `-a`. `auto` (the default) follows whether stdout is a terminal; detection is stdlib-only (`os.ModeCharDevice`), keeping the zero-dependency stance. Session content is also scrubbed: escapes and control bytes embedded in tool results (untrusted terminal input) no longer reach the terminal or pipes in any color mode.
- **`ccx view` no longer indents sequential messages ever-deeper.** Every message nested one level under its `parentUuid` predecessor, so a long linear conversation drifted right without bound (400+ columns by the end of an 8.5k-line session). Sequential messages are siblings; indentation now marks only real branch descent — main chain into sidechain, or one agent into another.

### Changed
- **Web UI wears terminal material now** — cctrace's design language ported onto ccx's markup (every selector and JS hook kept, values rewritten). 13px `ui-monospace` body replaces 17px system sans ('Courier New' led the old mono stack); warm-tinted neutrals with terracotta as the single accent plus a five-hue semantic set (green/red/amber/purple/blue) replace ~12 stray hues; pastel role bubbles become hairline surfaces with faint washes — user turns get the cctrace anchor mechanic (space above + accent-washed header row); thinking is muted italic. Chrome details: thin scrollbars, accent selection, visible focus, tinted shadows, one radius scale. Second side-stripe purge caught what 0.11 missed (tool blocks, outline active item, agent turns, doctor/memory cards). Devlog: `docs/devlog/2026-07-29-web-terminal-material.org`.
- **Long tool outputs clamp with an explicit "show all" expander** instead of an inner scrollbar — the mouse wheel never gets trapped inside a pane (cctrace's msg-clamp mechanic). Progressive enhancement: JS measures overflow and adds the fade mask + button; panes inside collapsed details clamp lazily on first open; without JS the old scroll behavior remains.

## [0.12.0] - 2026-07-24

Both changes answer the second field report — the first one produced by ccx tracing itself: a user audited a live session's trace and caught its two headline numbers lying. Findings 3-5 from the same report are tracked in issues #21-#23.

### Fixed
- **`ccx trace` no longer counts branch siblings as turns.** A user edit/resend appears in the log as two user records sharing one `parentUuid` — a branch point, not two turns — but the trace showed both as sequential turns with duplicate text and an inflated `turn_count` (first field report: a "7-turn" session was 6 turns plus one abandoned branch). Branches are detected through the message tree: the abandoned sibling (and any follow-up prompts inside its subtree) stays in the trace marked `superseded` / `superseded_by_turn` — an edited prompt is evidence of a course change, not noise — while `stats.turn_count` counts only active turns and `stats.superseded_turns` discloses the rest. The outline header reads `9 turns (+1 superseded)` and the abandoned turn is badged `(superseded by #7)`.

### Added
- **Per-turn token split — cost is now auditable at the outline level.** Turns previously carried only `input_tokens`/`output_tokens`, but cache traffic (absent entirely) is what dominates real cost, so a one-line reply billing $0.39 was unexplainable. Turns and the outline now carry `cache_read_tokens`, `cache_create_tokens`, and `reasoning_tokens` (Codex), turn badges show `88 in/29k out, cache 2.6m r/138k w` next to the dollar figure, and the header gains a session-level `tokens:` line. Stats carry the same totals.
- **Sidechain spend is in the total now.** Turn costs sum only main-loop messages, so agent-heavy sessions under-reported: `stats.total_cost_usd` is now all-in (main + every sidechain with usage in the log), with the agent share broken out as `stats.agents_cost_usd`, per-turn `agents_cost_usd`, and `$12.40 ($3.10 agents)` in the header. Sidechains whose transcripts live outside the session file still contribute nothing — absence, not a guess.

### Added
- **`ccx run <skill> --agent claude|codex|grok` — the runner bridge.** Launches an installed agent CLI headlessly (claude -p / codex exec / grok --single) with one of ccx's bundled skills as the prompt, plus an optional task. Deliberately a bridge, not an agent loop: the provider CLI owns permissions, sandboxing, streaming, and the session file — ccx passes no permission flags and never writes into provider homes. `--dry-run` prints the exact command, the payload, and the permission posture without executing. Every run retains a receipt in `~/.local/share/ccx/runs/` linking it to the provider-native session it produced, which `ccx trace` opens directly.

### Changed
- **Session view runs one navigation rail.** The hover-expanding session-switcher rail is gone; the outline (turn-level navigation — the Inspect instrument) is the rail. Session switching moved to a context bar under the header: breadcrumb plus a native session switcher. On viewports under 1024px the outline becomes an off-canvas drawer behind a visible toggle instead of disappearing — mobile navigation now exists.
- **Motion is mechanical and reduced-motion aware.** A global `prefers-reduced-motion` block (the file had none in 2,700 lines); the `?turn=N` deep-link flash animates outline color instead of box-shadow (compositor-friendly) and degrades to a static outline under reduced motion.
- **Side-stripe accents removed across the UI** (kiln permanent anti-pattern): provider identity lives in badges only, active states use terracotta-tinted backgrounds, state blocks (tool errors, diffs, task prompts) use tints and full hairline borders. Audit and token plan: `docs/design/0002-shell-redesign-audit.md`.

## [0.10.0] - 2026-07-15

### Added
- **Grok provider support** — ccx now reads Grok Build (grok CLI) sessions from `~/.grok/sessions/` alongside Claude Code and Codex. All five verbs work: `sessions`, `view`, `export`, `search`, `trace`, plus the web UI (GX badges, provider filter, `gx:` search prefix), `--provider gx`, `--grok-home`, `GROK_HOME`, `providers.grok` config, and `ccx doctor`. Discovery reads only each session's `summary.json` — no line scans, so Grok listing is the cheapest of the three providers. The parser consumes `chat_history.jsonl` (conversation), workspace `prompt_history.jsonl` (user-turn timestamps), and `updates.jsonl` (token totals), normalizes Grok's tool dialect onto the canonical names the trace analyzer understands (`run_terminal_command`→Bash, `edit_file`→Edit, ...), unwraps the `<user_query>` harness envelope, and renders reasoning summaries only (Grok thinking is encrypted; ccx never implies more exists). Parsing is gated on `chat_format_version: 1` — a future format bumps loud, not garbled. Format reference: `docs/devlog/2026-07-15-grok-session-format.org`, observed against grok 0.2.101 with committed sanitized fixtures under `testdata/fixtures/grok-home/`.
- **No Grok cost, by design.** Grok sessions parse token counts but never display dollar figures: real Grok tool results carry no error flags either (203/203 sampled), so Grok turns also report zero tool errors instead of guessing from content text. Remove-wrong over display-wrong, both times.

### Changed
- **Zero cost now renders as nothing, not `$0.00`.** The trace outline header omits its cost segment when no cost was computed — `$0.00` read as "measured zero" when the truth is "unpriced" (Grok by contract, unknown models on any provider).
- CLI `search` results show the backend's human-readable project name instead of pushing grok's URL-encoded directory names through claude-code decoding heuristics.

## [0.9.0] - 2026-07-15

Two feature sets: the review loop grows per-turn evidence panels in the web UI, and export grows the distillation primitive. Driven by the vault-mining pilot (2026-07-12): 18 sessions distilled with ad-hoc Python exposed that every distiller agent re-implemented the same "what did the human actually say" filter. That filter is now a ccx primitive.

### Added
- **Per-turn evidence panel with `?turn=N` deep links in the web UI.** Every user turn in the session view carries a review panel: steps, tool counts, edited files (linked to their inline diff blocks), spawned agents, failed calls, cost, and active time. Turn ordinals come from the same segmentation `ccx trace` uses, so `#54` on the page is the turn `ccx trace --turn 54` prints and the turn an audit report cites; `?turn=54` (and `?turn=54.10` for a step) deep-links into the page and expands the right thread.
- **`export --shape human` — the distillation primitive.** Emits only the human's actual turns, verbatim, numbered, each anchored with timestamp + message-UUID prefix so notes can cite `<session>#<uuid8>` and resolve it back to the transcript. Compaction replays (identical text + identical timestamp re-emitted after a context rewind) are deduplicated and the header reports the drop count (`Turns: 73 (98 raw, 25 replay duplicates dropped)` on a real 4,498-line session). Markdown-only by design; defaults `--format md` when unset.
- **`sessions --sort prompts`** — rank sessions by human-turn count. Raw message counts are inflated by tool results and harness noise; user prompts are the signal-density metric for deciding which sessions are worth reading (or sending to an LLM) at all.

### Fixed
- **Tool errors now surface on the calls that caused them.** The trace analyzer counts errors from `tool_result` blocks (where providers actually record them), but call evidence previously looked for errors on `tool_use` blocks — so the web evidence panel could claim "2 errors" while listing no failed calls. Result errors are now attributed back to the issuing call by tool ID, including calls from non-mutating tools (a failed `Read` now appears in the failed-calls list). The panel's error chip and its failed-calls list are guaranteed to agree, and both match `ccx trace`.
- **Upgrading ccx no longer serves stale parses from the on-disk caches.** The session disk cache and the discovery metadata cache invalidated on source mtime+size only; gob decodes across struct versions silently, so a new binary kept serving sessions parsed by the old code until the source file happened to change — parser fixes shipped blind. Both caches now carry a format version stamp and discard entries written under a different one.
- **Tag pushes can no longer publish from a red repo.** The release workflow now runs `go test -race` and golangci-lint before goreleaser, and the linter version is pinned (CI, release, and `make tools` all install the same one) instead of floating on `latest`.
- **Harness noise no longer classified as human turns.** `<local-command-stdout/stderr/caveat>` echoes and `<task-notification>` events arrive as `type:user` in the JSONL but were never typed by a human; they previously classified as `user_prompt` (the `<command-` prefix check missed `<local-command-`). New kinds `command_output` and `notification`. In a measured session, 33 raw user-typed turns were actually 9 human turns.
- **Session-list prompt counts now match full-parse counts.** Quick parse counted commands and harness wrappers as user prompts; it now applies the same classification as the full parser, so `sessions` listings and session view agree.

## [0.8.0] - 2026-07-09

All changes below respond to the field report in [#19](https://github.com/thevibeworks/ccx/issues/19) — a real-usage grilling of the trace v2 + skills flow.

### Added
- **`ccx skills` command — skills are now embedded in the binary** (#19.1). `ccx skills install [--scope user|project]` writes the bundled ccx/ccx-recap/ccx-retro skills so the installed skill text always matches the CLI surface of the running build; `ccx skills list` reports drift between installed copies and the binary. This kills the skills-version-with-repo vs binary-versions-with-tags drift class: an agent following a skill can no longer hit flags the binary doesn't have. `install.sh` now delegates its skill step to `ccx skills install` (the download-by-version ceremony and stale skill names are gone). A test guards that embedded skills stay byte-identical to the repo files.
- **Active time alongside wall-span** (#19.4). Traces and outlines now carry `active_seconds` per turn and in stats: the sum of inter-message gaps, each capped at 5 minutes. The outline header prints `active 4h07m` next to the wall-span, and turn badges show active time instead of the wall gap — an autonomous turn bleeding into an overnight gap no longer reads as an 18-hour turn.
- **JSON schema contract documented** (#19.3): `docs/schema.md` covers all emitted kinds (`ccx.outline.v1`, `ccx.turn.v1`, `ccx.trace.v2`, `ccx.log.v1`), the versioning policy (version lives in the `kind` string; additive changes don't bump; breaking changes bump and list removed fields), and semantics consumers must know (UTC in JSON, active vs wall time, evidence budgets, warnings-are-findings).

### Fixed
- **Session ID lookup no longer dead-ends outside the owning workspace** (#19.2). `ccx trace <id>` (and view/export) now widen a workspace-scoped miss to all projects automatically — session IDs are globally unique, so `ccx trace f6f02cc2` works from any directory. Misses now say what was searched: `session "x" not found in project "y"; try --all` for explicit `-p` scopes, `not found in any project` otherwise.
- **Outline states its timezone** (#19.5). The text outline renders local times; the header now says so (`times UTC-7`), so citations cross-reference cleanly against the UTC timestamps in JSON and raw JSONL.

### Changed
- **Skills land durable knowledge, not just chat replies** (#19 architecture comment: "digs are rent, not equity"). ccx-recap ends by offering to write the distillation into the user's existing durable store with `[ccx:<session-id> #turn.step]` citations — the session id is re-verifiable provenance, since ccx can always re-open the receipt. ccx-retro carries the citation into approved rule patches. Both descriptions now state explicit routing ("what happened" → recap; "what should change" → retro), and both declare the minimum ccx version they drive with the upgrade path when the binary is older.
- The bundled `ccx` skill's command tree caught up with the binary: `trace`/`insight`/`skills`/web deep-link flags were missing — the exact drift #19 documents.

## [0.8.0-rc.1] - 2026-07-07

### Added
- **ccx-recap and ccx-retro skills**: two user-question skills replace the mechanism-named ones. `/ccx-recap` answers "what did the agent actually do?" (session or today/week scope, receipts required, 60-second-readable output). `/ccx-retro` answers "where did it go wrong, what fixed it, what should change?" — its output is a proposed patch to CLAUDE.md/AGENTS.md/skills/memory with evidence citations and a mandatory human gate. `ccx-context-fold` and `ccx-insight` skills removed; the `.ccx/knowledge/` store concept is gone (retro patches the instruction files agents already read).
- **Insight/log bundles carry pre-computed aggregates**: `ccx insight --json` and `ccx log --json` now include `days[]` (bucketed in the scope timezone), `providers[]`, and `workspaces[]`, each with sessions/records/user_prompts/assistant_messages/tool_calls/sidechains. Consumers no longer re-bucket tens of thousands of records to answer "what happened when/where". Aggregates are computed before any `--limit` truncation.
- **Latest Claude models in pricing table**: `claude-opus-4-8`, `claude-opus-4-7` (both tier `$5/$25`, matching Opus 4.5/4.6), and `claude-fable-5` (new tier `$10/$50`, above Opus). `LookupPricing` match order updated so the newer Opus IDs resolve correctly instead of degrading to the legacy `$15/$75` Opus 4/4.1 tier. `cmd/ccx-verify-pricing` canonicalization kept in sync. **UNVERIFIED**: Fable 5 input/output rates are verified from the public model catalog, but its cache-read/cache-write rates are placeholders following Anthropic's standard 10%/125%-of-input ratio — confirm against Claude Code's `modelCost.ts` once the reference checkout carries a Fable 5 entry.

### Changed
- **Trace v2 (`ccx.trace.v2`): turns → steps, outline by default**. The old exchange model collapsed on autonomous sessions (one prompt = hours of work = one opaque row) and dumped 0.7-2.5MB of JSON. v2 segments each turn into say-then-do *steps* anchored on the agent's own narration — the decision trail as it was written. `ccx trace` now prints a terminal-readable outline (every turn + step headline with tools/edits/errors/cost); `--json` for the outline as JSON, `--turn N` for full single-turn evidence, `--full` for the complete bundle. Tool errors are now attributed to the step that issued the call via tool-result IDs (v1 always reported 0 errors). Deleted: the Go HTML renderer (corrupt UTF-8 via byte-slicing, unreadable output), `--html`, keyword-based `signals`/`has_correction` (it flagged "Good work!" as a correction — interpretation belongs to the skills, facts to the CLI). Text hygiene kept from the interim 1.1 work: bounded head+tail excerpts, ANSI stripping, command-XML condensing, mutation-only call itemization, single-copy sidechain evidence. Package renamed `internal/fold` → `internal/trace`.

### Fixed
- **Insight HTML report honors the scope timezone**: daily buckets, workspace day counts, and timeline times were formatted in the records' raw timezone (usually UTC) while the header claimed the scope TZ — evening work showed up on the next calendar day. All record timestamps now convert via the scope location before bucketing/display, matching the JSON `days[]` aggregates.
- **Session summary no longer picks up harness XML**: full-parse summary extraction now skips `<command-name>`/`<local-command-stdout>` wrapped messages (with their raw ANSI escapes), matching the quick-parse rule — session pages and traces show the first real user prompt instead.
- **Discovery performance: web index 15s → 0.08s, session page 44s → 0.4s (warm)**. Every project/session listing re-read and JSON-decoded the full session corpus (measured: 2.8GB across 4,200 files → ~15s per pass, and the web server ran several passes per request). New persistent session-metadata cache (`$XDG_DATA_HOME/ccx/meta-cache.gob`) keyed on file mtime+size: unchanged files serve list metadata from cache, changed/new files re-parse, skipped files (no summary, warmup) are negative-cached. Applies to both Claude Code and Codex backends; `ccx sessions --all` drops from ~16s to ~0.5s. First pass after upgrade pays the one-time cache build. Codex thread names resolve from the live name store on every pass (renames show up despite caching); a thread name deleted entirely from the store may linger on cached entries until the rollout file changes.

## [0.7.0] - 2026-05-25

### Added
- **ccx-insight skill**: Time-sliced session intelligence — uses `ccx log` to slice timestamped records across long-running sessions, then briefs what was worked on, achieved, blocked, or emerging across today/yesterday/week/month/quarter/year. Writes standalone HTML audit reports for human review.
- **ccx-context-fold skill**: Session decision extraction — folds a ccx trace into auditable decisions and durable project knowledge. Use after coding-agent sessions, before context compaction, or during PR review.
- **Workspace-scoped session listing**: `ccx ls` and `ccx sessions` now scope to the current workspace by default. `--all` flag for cross-project listing.
- **Calendar-scoped session listing**: Session list supports time-range filters for day/week/month/quarter/year slicing.

### Changed
- **Security: sanitized test fixtures**: Real Anthropic API `requestId` values in testdata JSONL replaced with synthetic IDs across entire git history via `git-filter-repo`. No API-traceable identifiers remain in the repository.

### Fixed
- **Devlog artifact scrub**: Removed local machine artifact references from published devlog entries.

## [0.6.0] - 2026-05-21

### Added
- **ccx fold command**: Session turn analysis with git correlation — `ccx fold` analyzes session turns, correlates with git history, and extracts decision chains.
- **Workspace-scoped sessions**: Session listing scoped to current workspace directory.
- **Agent nav entries inline**: Sub-agent entries in the outline sidebar now appear inside their parent exchange group, not bottom-appended.
- **Agent names and stats in sidechain headers**: Sidechain groups show agent type (Explore, linus-rants, etc.), description, token count, tool count, and lines changed from `toolUseResult`.
- **Semantic color tokens**: `--event-subagent`, `--event-skill`, etc. — single source of truth for event-kind colors across nav, timeline, and sidechain groups.
- **Adaptive timeline rail**: Spine height computed from tick count with vertical centering. Satellites enlarged to 5x5px.

### Changed
- **CSS extracted to embedded file**: All CSS moved from inline Go string to `internal/web/static/style.css` via `embed.FS`. Templates.go reduced from 7693 to 5094 lines.
- **Schema audit as dev tool**: `cmd/ccx-audit-schema` for detecting JSONL schema drift — separate dev binary, not integrated into ccx CLI.

### Fixed
- **All CI lint failures**: Resolved errcheck, gosimple, unused variable warnings across the codebase.
- **Timeline rail vertical centering**: Rail uses `display:flex; align-items:center` with computed spine height.
- **Session metadata parsing**: `aiTitle`, `toolUseResult` (handles string vs object), Model, and Provider now extracted from JSONL and displayed in info panel.

## [0.5.0] - 2026-04-29

### Added
- **Docker support**: Multi-stage Dockerfile (distroless nonroot runtime), docker-compose.yml with read-only agent config mounts, and GitHub Actions workflow for automated multi-arch (amd64/arm64) image publishing to `ghcr.io/thevibeworks/ccx`. Default host port 2299 (C=2, C=2, X=9 on phone keypad).
- **Exchange + Step data model**: Replaces the older TurnStats vocabulary. An Exchange is one user request plus everything the agent did in response — including sub-agent dispatches, skill invocations, and tool calls. Steps enumerate sub-events inside each Exchange so the timeline rail can paint satellite markers (sub-agents, skills, tools, compaction boundaries).
- **Workspace as first-class object**: Parser and provider layers now surface workspace metadata (project path, git branch, CWD, CLI version) extracted from session JSONL headers.
- **Export `--shape` flag**: Splits export shape (`trace`, `brief`, `exec`) from output format (`json`, `html`, `md`, `org`, `txt`). Previously `--format exec` conflated shape and format.
- **Sub-agent sidechain discovery**: Parser now finds and loads sub-agent transcripts from `<uuid>/subagents/agent-*.jsonl` files alongside the main session JSONL. Sidechain messages contribute to stats (AgentSidechains, MessageCount, Exchange.HasSidechain) and are available for cost computation.
- **Sub-agent tool classification**: TaskCreate and Agent tool_use blocks are now classified as StepSubagent (previously fell through to StepToolUse). Timeline rail sub-agent satellites and tooltip badge counts are now accurate.
- **Friendly 404 pages**: Missing projects, sessions, and routes now render a styled page with the failing URL and a pre-filled GitHub issue link for one-click bug reporting.
- **Real-session test fixtures**: Captured fixtures from Claude Code 2.1.104 and Codex sessions with comprehensive assertions for message counts, content block types, tool distribution, and sidechain discovery.

### Fixed
- **Sidechain messages polluting main conversation**: Sub-agent transcripts loaded from sidechain files were rendered as separate thread sections in the main view and appeared as spurious entries in the outline sidebar. Root cause: rendering code never filtered on IsSidechain — it only used the flag for CSS styling. Fix: `filterMainConversation()` strips sidechain messages before thread grouping in `renderMessages`, `renderMessagesProgressive`, and `renderConversationNav`. Search results also filter sidechains so scroll-to-message anchors are always present in the rendered HTML.
- **Codex web_search double-counting**: web_search_call and web_search_end events were both incrementing tool counts. Now deduped.
- **Codex session rendering**: Fixed floating fold-toggle design, spinner icon position pinned, Codex accent color applied to CX sessions.
- **Cross-agent merged URL resolution**: `Multi.FindSession` now resolves session IDs correctly when the same project exists in both Claude Code and Codex providers.
- **O(n^2) digest computation**: Fixed quadratic behavior in session digest. Also fixed merge-key collisions, sidechain step bleed into parent exchanges, markdown-to-HTML rendering edge cases, and APFS case-sensitivity issues.
- **Exchange-first outline**: Split outline sidebar now groups by Exchange boundaries with separate click targets for "jump to message" and "toggle children". Stable fold button replaces the conflicting details/summary approach.
- **Spinner verbs**: Loading spinner now cycles through contextual verbs instead of a static indicator.

### Changed
- **Default Docker port**: 2299 (was 3773). Derived from "ccx" on phone keypad: C=2, C=2, X=9.
- **Makefile**: Added `dev` and `devweb` targets for fast build+run web loop.

## [0.4.0] - 2026-04-14

### Added
- **Executive-style `exec` export format** (#7): New `ccx export --format exec` mode that renders a session as a turn-by-turn executive report — user request (quoted), files touched (from Edit/Write/MultiEdit/NotebookEdit/Create tool calls, deduped and sorted), and the agent's final summary per turn. Per-turn cost and token footprint at the bottom of each turn when pricing is available. Context-compaction boundaries render as `— context compaction —` dividers between turns. Empty turns (meta, markers) are dropped. Designed for sharing "what did the agent do" without dragging every tool invocation along — much shorter than `--brief` and focused on outcomes.
- **Per-turn spend breakdown in the info panel** (#2): The info panel now shows a Per-turn spend section with one clickable row per user turn, sorted by cost desc so expensive turns surface first. Each row links to `#msg-<anchor>` which composes with the load-earlier fix to deep-link into any turn regardless of progressive-load state. Answers "where did my quota go?" inside the session viewer, without leaving ccx.
- **Per-message token usage on `Message`**: The parser now persists `input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, and computed USD cost on each assistant `Message`. Previously only session-level aggregates were kept.
- **Embedded Claude pricing table**: Pinned list pricing for Claude Opus 4.x, Sonnet 4.x, Haiku 4.5, and legacy 3.5 Sonnet/Haiku. Offline-by-design: no runtime fetch, no LiteLLM dependency. Unknown models resolve to nil (no fuzzy match) — callers treat nil as "cost unavailable" rather than mis-attributing.
- **Session-level Cost row**: Info panel's Tokens section now shows a Cost line when at least one message has pricing-resolved usage, computed as the exact sum of per-message costs (single source of truth — `session.Stats.CostUSD == sum(message.Usage.CostUSD)` by construction).

### Performance
- **In-memory LRU session cache** (#4): `provider.Multi.ParseSession` now caches parsed session trees by `(path, mtime, size)` with an LRU cap of 16 entries. Cache hits skip the full parse entirely. Measured: cold parse of a 1500-message session ~19 ms, cache hit ~330 ns — a **~60_000× speedup** on repeat views. Profile and methodology in `docs/profiles/long-session.md`. Transparent to callers; invalidated automatically when the session file is rewritten (mtime or size change).
- **Persistent disk cache** (#9): companion to the in-memory LRU, survives process restart. Parsed sessions are gob-encoded to `$XDG_DATA_HOME/ccx/session-cache/<sha256>.gob` with in-band `(mtime, size)` metadata. Two-tier lookup: memory first, disk second, live parse third. First `ccx web` view in a fresh process hits the disk instead of re-parsing — so restarting the server no longer costs anything for sessions you've already opened. Stale entries are deleted on miss. Corrupt files (e.g., schema drift across ccx versions) are treated as misses and swept. `gob.Register(map[string]any{})` and `gob.Register([]any{})` are set up in `init()` so `ContentBlock.ToolInput` round-trips cleanly. Disk cache is opt-in only in the sense that a missing / unwriteable data dir degrades gracefully to memory-only; no new config surface.

### UI
- **Narrow auto-expand session nav sidebar** (#1): The session list on the left of the conversation page now collapses to a 56px compact column showing only session-id stubs + provider badges — with the "Sessions" header replaced by a circular `S` dot in the session-accent colour. Hovering the sidebar expands it to 260px via CSS overlay (the flex slot stays 56px so the outline sidebar and main content don't shift right — the expanded column floats on top with a soft shadow instead). All session summaries, full IDs, and the "Sessions" text label fade in on expansion. Click or keyboard tab still work the same; navigation semantics unchanged. Mobile (<600px) continues to hide the panel entirely.
- **Time-machine timeline rail** (#5): A semantic scrubber on the **right** edge of the session view — narrow and subtle by default (10px visual with a 24px invisible hit target that forgives slight mouse drift), expanding to 52px on hover. Sits 14px in from the browser scrollbar so it never fights with the page progress bar. Each tick marks a user prompt, slash command, or context-compaction boundary, **positioned by anchor index rather than wall-clock time** — a session with a 3-hour idle gap no longer compresses all the work into a tiny sliver; every turn gets an even slot on the rail. **Ticks are sized and saturated by per-turn cost** (merging #2's data): the most expensive turn is the biggest, brightest dot, cheaper turns fade toward the baseline. For unpriced models, heat falls back to token count. Index-based ruler gridlines label turn ordinals at round intervals (every 5/10/25/50/100 turns depending on session size) with labels that fade in on hover. **Interaction model**: moving the mouse along the rail activates a local fisheye zoom on the 5 ticks nearest the cursor (`zoom-0` 2.6×, `zoom-1` 1.9×, `zoom-2` 1.35×) and snaps a floating tooltip to the nearest tick with kind badge, elapsed offset (`+1m42s`), turn ordinal (`turn 42`), preview text, per-turn cost (`$0.0423`), cumulative spend up to that point (`∑ $2.30`), and token total (`12.3k`). Tooltip clamps to viewport at top/bottom edges. **Hysteresis** (1.5% of rail height) prevents tooltip flicker when the cursor sits between adjacent ticks. Mousemove is **rAF-throttled** so long sessions stay smooth. **Grace period** (120ms) on `mouseleave` so briefly slipping off the rail doesn't tear the interaction down. Clicking jumps via `#msg-<uuid>` — composes with the load-earlier hash-nav fix so ticks into collapsed history auto-reload with full content. A scroll-linked current-position marker tracks where you are. Keyboard `[` / `]` jumps to the previous / next tick relative to the viewport center. Hidden under 1024px viewports (the outline sidebar is the fallback nav). Works for both Claude Code and Codex sessions.

### Fixed
- **Deep-link navigation into collapsed history** (#3): Hash URLs (`#msg-<uuid>`) targeting messages inside progressively-loaded "Load earlier" sections now auto-reload with `?all=1` and scroll to the target instead of silently failing. Ancestor threads and `<details>` elements are unfolded on arrival so the target is actually visible. `hashchange` events also trigger the same jump logic, so in-app navigation via `#msg-` links works after initial load.
- **In-session search 0-match experience** (#3): When search returns no hits but the session has hidden history, the info line now shows a "search full history" link that reloads with full content; the pending query is preserved via `sessionStorage` and re-applied after reload so the user picks up where they left off.
- **Short session ID crash**: `renderSessionPage` no longer panics when the session ID is shorter than 8 characters (was `session.ID[:8]` unguarded).
- **Server-side markdown headings**: The Go `renderMarkdown` pipeline now recognises CommonMark ATX headings (`#` through `######`) and emits `div.md-h1`-`div.md-h6`. Previously these lines were wrapped in plain `<p>` with the `##` literally visible. Also handles simple `- item` / `* item` lists the same way as the JS-side renderer.
- **Outline drops assistant summary on long turns**: `renderConversationNav` used to hard-cap each user turn's children at 10 and drop everything past that — including the final assistant summary, which is usually at the end of tool-heavy turns. The truncation logic now shows the first 9 children, a `+N more` marker, and the last child (preserving the summary). Tests: `TestRenderConversationNav_PreservesSummaryOnTruncation`.

### Data layer / Codex (#8)
- **`MessageUsage.ReasoningTokens` field**: Codex GPT-5/5.4 sessions emit `reasoning_output_tokens` (the extended-thinking output counter). ccx's unified `MessageUsage` struct now carries this field so Codex sessions get their thinking tokens priced at output rate (matching OpenAI's billing). Zero for Claude sessions.
- **GPT-5 / GPT-5.4 pricing table**: `gpt-5`, `gpt-5-mini`, `gpt-5-nano`, `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.4-nano` now resolve via `LookupPricing` to pinned tiers. Match order handles `nano` before `mini` before plain `gpt-5` so names like `gpt-5-mini-2025-07-07` don't degrade to the more-expensive full-sized tier. **UNVERIFIED**: these rates are pinned from publicly known GPT-5 list pricing — there's no equivalent of Claude Code's `modelCost.ts` in the Codex reference checkout to automate drift detection yet. Update `internal/parser/pricing.go` if OpenAI rebases gpt-5.4.
- **Codex per-message usage attribution**: the Codex backend's `ParseSession` full-parse path now diffs successive `token_count` events and attributes the delta across every untagged assistant message in the pending burst via `usageWatermark` + `distributeCodexDelta`. This also preserves usage when `token_count` arrives before the assistant output and keeps session cost equal to the sum of attributed message costs. Result: the #2 per-turn spend breakdown and #5 timeline-rail cost heat work for Codex sessions instead of silently showing $0.
- **`ComputeCost` includes reasoning tokens**: reasoning tokens now contribute to cost at the output rate.
- **Extended `tokenUsageTotals` struct** to parse `reasoning_output_tokens`.
- **Turn accounting fixes**: sidechain prompts now stay inside the parent user turn, reasoning tokens are included in per-turn totals, and `export --format exec` uses the same sidechain-aware turn boundary as the spend panel.

### Pricing (#10)
- **Fixed: Opus 4.5/4.6 overcharge.** Previously these models were hardcoded at tier `$15 input / $75 output` (the Opus 4/4.1 rate). Claude Code's own source has them at tier `$5 input / $25 output` (a roughly 3x lower rate). ccx was overstating the cost of every Opus 4.5/4.6 turn by ~3x. Now fixed in `internal/parser/pricing.go`.
- **Added models**: `claude-opus-4`, `claude-opus-4-1`, `claude-sonnet-4`, `claude-3-7-sonnet` — previously missing from the embedded table and would return nil (no cost displayed) on hover.
- **Rewritten match logic**: `LookupPricing` now mirrors Claude Code's `firstPartyNameToCanonical` algorithm — case-insensitive substring match with more-specific patterns first (`claude-opus-4-6` beats `claude-opus-4`). Handles bare canonical names, dated IDs (`claude-sonnet-4-5-20250929`), and Bedrock ARNs (`us.anthropic.claude-sonnet-4-5-20250929-v1:0`) uniformly. Non-Claude models (`gpt-4`, `grok-2`, etc.) still return nil — no cross-family misattribution.
- **New tool: `ccx-verify-pricing`**. Parses `reference/claude-code-2188/src/utils/modelCost.ts` and diffs it against ccx's embedded pricing table. Exit non-zero on drift so CI can gate on it. Run via `make verify-pricing` (set `CLAUDE_SOURCE=<path>` if your checkout lives elsewhere). Includes unit tests that feed the tool a fake source with deliberate drift to prove the detector works.
- **Known limitation**: Opus 4.6 in fast mode is billed at tier `$30/$150` (4x the default). ccx currently returns the default tier regardless of the message's `usage.speed` field — tracked as a follow-up that requires plumbing `speed` through the parser.

### Accuracy notes
- **What we count**: Per-turn cost is the sum of every billable assistant message within the turn, priced at the pinned list rates. Sidechain (`isSidechain: true`) assistant messages ARE included because you ARE billed for them — ccx does not attempt sidechain cache-read dedup (which would produce numbers that don't match the invoice).
- **When a number is omitted**: If the model is unknown to the pricing table, the Cost line is omitted and the per-turn breakdown shows a "No pricing match" note with token-only rows. We would rather not show a number than show a wrong one.

## [0.2.5] - 2026-01-07

### Added
- **Progressive loading**: Large conversations (500+ messages) now load only the last 3 compacted sections initially
- **Load earlier button**: Click to load full conversation history on demand
- **Context accent colors**: Distinct visual identity for Projects (blue), Sessions (purple), Conversations (teal)
- **Session token counts**: Token usage displayed in session cards (⧫ icon) and info panel
- **Cache token stats**: Info panel shows cache read/write tokens with tooltips
- **Token tooltips**: Hover for explanation (e.g., "Fresh tokens sent to API")

### Changed
- **Toolbar icons**: Replaced Unicode symbols with SVG icons for Export, Find, Refresh (Info kept as ⓘ)
- **Toolbar position**: Raised 12px for better visibility (52px from bottom)
- **Search panel position**: Aligned with toolbar center offset
- **Find button toggle**: Click Find to toggle search panel open/close (matches info button behavior)
- **Search result badges**: Now use context-specific accent colors
- **Side panel accents**: Active items show context-appropriate border colors
- **Page headers**: Badge + left border indicate current context (P for Projects, S for Sessions)
- **Session cards**: Use session accent color for left border
- **Info panel redesign**: Grouped sections (Context, Time, Activity, Tokens) with headers
- **Info panel accent**: Conversation-colored left border for visual context

### Fixed
- **Token display**: Fixed token extraction from `message.usage` (was looking at wrong JSON path)
- **Token formatting**: Fixed boundary issue where 999,950 tokens would show as "1000.0k"
- **Progressive loading**: Now works for sessions without compact boundaries (falls back to chunking by user prompts)
- **Progressive loading chunks**: Break before user prompts so each chunk starts with user prompt + responses
- **Mobile responsiveness**: Search and info panels now adapt to small screens

### Removed
- **Cost estimation**: Removed inaccurate cost display (Claude Code calculates at runtime, not stored in JSONL)
- **Lines changed**: Removed inaccurate lines tracking (agent sidechains in separate files not aggregated)

## [0.2.4] - 2026-01-05

### Added
- **Header social icons**: GitHub and X/Twitter (@ericwang42) icons in top nav
- **Screenshots**: Added 5 screenshots to README (projects, live, session info, export, settings)

### Changed
- **Toolbar positioning**: Shifted right (+140px) and up (40px) to align with main content center
- **Panel nav width**: Reduced from 200px to 170px for more compact feel
- **Info panel**: Repositioned to right side, floating above info icon
- **README**: Added screenshots, credited Simon Willison's inspiration

## [0.2.3] - 2026-01-05

### Fixed
- **Live mode tool results**: Show tool name (e.g., "TodoWrite") instead of raw ID ("toolu_01")
- **Scrollspy**: Use sidebar viewport rect for out-of-view check (was using content rect)
- **Live nav ID mismatch**: Sanitize UUIDs consistently for DOM IDs and nav item data-msg
- **Mobile layout**: Hide panel-nav at 600px, shrink at 768px
- **CSS collision**: Renamed sidebar `.nav-item` to `.sidebar-link`
- **Markdown links**: Added `rel="noopener noreferrer"` to live mode markdown renderer
- **Summary click UX**: Click only toggles group (no jump), removed redundant dblclick handler
- **Live nav grouping**: New messages grouped under "● Live" section separator
- **Debug spam**: Removed console.log statements

### Changed
- Scrollspy throttled via requestAnimationFrame (was firing on every scroll)

## [0.2.2] - 2026-01-04

### Added
- **Two-panel navigation**: Project page shows Projects | Sessions, Session page shows Sessions | Conversation
- Master-detail pattern for quick context switching without losing place

## [0.2.1] - 2026-01-04

### Changed
- README rewritten with web UI as primary feature, ASCII diagram
- CLI help (`ccx --help`, `ccx web --help`) emphasizes web UI

### Added
- Site footer with GitHub link and thevibeworks branding

## [0.2.0] - 2026-01-04

### Added
- **Session search**: In-session search with floating search bar, keyboard shortcuts (`/`, `f`, `Esc`), navigation (`Enter`, `Shift+Enter`), and filter chips (User, Response, Tools, Agents, Thinking)
- **Tool rendering**: Specialized preview/output formatting for Task, Skill, WebSearch, WebFetch, AskUserQuestion, LSP, TaskOutput, KillShell
- **Refresh button**: Toolbar refresh button (`r` shortcut) for manual page reload
- **Auto-expand on search**: Automatically unfolds collapsed sections when jumping to search matches

### Security
- URL sanitization: Only allow http/https URLs in rendered output
- Tabnabbing prevention: Added `rel="noopener noreferrer"` to all external links
- Deterministic preview: Fixed nondeterministic map iteration in tool parameter preview

## [0.1.1] - 2025-12-31

### Changed
- **Pure Go migration**: Replaced mattn/go-sqlite3 (CGO) with modernc.org/sqlite (pure Go) for true cross-platform single-binary distribution
- Updated Go 1.22 → 1.24.0, cobra 1.8 → 1.10.2, viper 1.18 → 1.21.0

### Added
- GitHub Actions CI workflow (test, lint on push/PR)
- GitHub Actions Release workflow (cross-compile darwin/linux arm64/amd64)
- CONTRIBUTING.md with dev setup and release workflow
- Skill bundle packaging (ccx.skill)

### Fixed
- Cobra duplicate error output
- Dead code removal
- Error handling for json.Encode, file.Seek

## [0.1.0] - 2025-12-30

### Added
- Core CLI commands: projects, sessions, view, export, search, config, doctor
- JSONL parser with tree-aware message structure (parentUuid, isCompactSummary, isSidechain)
- Web UI with project/session browser, collapsible blocks, syntax highlighting
- Export formats: HTML, MD, Org-mode, JSON
- Realtime watch mode via Server-Sent Events
- SQLite star/favorite system
- Global search across projects and sessions
- Dark/light theme toggle with persistence
- Keyboard navigation (j/k scroll, gg/G jump, / search, t theme, z collapse)

[Unreleased]: https://github.com/thevibeworks/ccx/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/thevibeworks/ccx/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/thevibeworks/ccx/compare/v0.3.2...v0.4.0
[0.2.5]: https://github.com/thevibeworks/ccx/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/thevibeworks/ccx/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/thevibeworks/ccx/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/thevibeworks/ccx/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/thevibeworks/ccx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/thevibeworks/ccx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/thevibeworks/ccx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/thevibeworks/ccx/releases/tag/v0.1.0
