# Shell redesign: kiln audit and token plan

Phase 2 of docs/2026-q3-goal.md. Audit of the web UI as of v0.10.0
(style.css @ 2,702 lines), by kiln layer. Each finding is marked
permanent (structural rule) or judgment (design decision this
redesign makes and records).

## Findings

### L0 principles

1. **Figure-ground: three competing rails.** The session view runs a
   session-switcher panel (`.session-nav`), an outline sidebar
   (`.nav-sidebar`), and the main thread — three vertical regions of
   similar visual weight. The eye has to resolve which rail is
   navigation, which is content, before reading anything.
   *Decision (judgment): the outline is the one rail.* It is the
   Inspect instrument of the Observe → Inspect → Verify → Export
   workflow — turn-level navigation is what a session auditor uses
   constantly. Session switching is Observe-level and moves into the
   breadcrumb as a compact switcher; the project page remains the
   real session list.
2. **Working memory: header + dock + breadcrumb + two rails** all
   carry navigation affordances on one screen. Consolidate to:
   header (global destinations), breadcrumb (location + switcher),
   one rail (in-session outline).

### Permanent anti-patterns (kiln)

3. **Side-stripe accents** — `border-left: 3px solid` on page
   headers, project cards, session cards, active panel items.
   Replaced with background tints and full 1px borders; provider
   identity moves entirely to the badge (the stripe duplicated it).
4. **No `prefers-reduced-motion` handling** anywhere in 2,702 lines.
   Added as a global reduce block (per kiln adaptation: instant
   cuts, keep color/opacity state changes).
5. **Animated box-shadow** — `ccx-target-flash` keyframes animate
   `box-shadow` (paint-heavy, and the exact instance flagged in the
   review). Replaced with an `outline` + opacity flash: same signal
   (deep-link landing), compositor-friendly, honors reduced motion.
6. **Shadow inflation risk** — 23 `box-shadow` uses; several on
   static containers where elevation communicates nothing. Reduced
   to: dropdowns/menus (real elevation), pressed/hover feedback.

### L2 materials

7. **Color tokens are inconsistent between config and CSS** (config
   said codex=blue while CSS said codex=emerald until v0.10.0) and
   provider accents leak into non-semantic surfaces (page-header
   stripes colored by *page type*, not by state or provider).
   Token plan below is the single source of truth.
8. **Breakpoints scattered** — 600/768/900/1024px media queries with
   different regions collapsing at each. Two breakpoints: 1024px
   (rail collapses to drawer) and 700px (compact type/spacing,
   mobile nav).

## Token plan

Kit: terminal density (0.85, monospace numerics) on warm-ground
surfaces, terracotta as the only identity accent. Provider and
state colors are semantic-only.

| Token | Role | Rule |
|---|---|---|
| `--primary` (terracotta) | Identity accent | Interactive affordances, active states, focus. <= 10% coverage |
| `--accent-cc/cx/gx` | Provider identity | Badges and provider filter chips ONLY |
| `--ok/--warn/--err` | State | Status text/chips ONLY (e.g. te-error) |
| `--bg/--surface/--raised` | Neutral ground | Tinted toward terracotta hue (chroma ~0.005), never pure gray |
| `--border` | Structure | 1px rules; whitespace groups first, borders second |

Motion: mechanical (kiln vocabulary) — micro 50ms, transition 100ms,
no entrance choreography; the deep-link flash is the one deliberate
attention cue, and reduced-motion collapses it to a static outline.

## Status

Applied in this pass: findings 1 (session view), 3, 4, 5, and the
8 → 2 breakpoint consolidation for the session view. Remaining
surfaces (index, project, search, insights, settings pages) adopt
the same tokens page by page; tracked in the Q3 contract.
