# Changelog

All notable changes to ccx are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [Unreleased]

### Added
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

[0.2.5]: https://github.com/thevibeworks/ccx/compare/v0.2.4...v0.2.5
[0.2.4]: https://github.com/thevibeworks/ccx/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/thevibeworks/ccx/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/thevibeworks/ccx/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/thevibeworks/ccx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/thevibeworks/ccx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/thevibeworks/ccx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/thevibeworks/ccx/releases/tag/v0.1.0
