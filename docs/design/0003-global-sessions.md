# Global sessions page (/sessions)

Design spec for the cross-project session list. Continues the material
contract in 0002 (quiet terminal, terracotta accent, hairline surfaces,
13px mono, no side-stripes, 1024/700 breakpoints). Every choice is marked
**Decision** (made here, with rationale) or **Open** (implementer judgment).

Nothing below invents new material. New CSS is hairline + token only.

## 1. What the page is for

The session auditor's cockpit: *what ran recently, everywhere.* Today the
only session list is per-project (`/project/<name>`), so "what did I run
yesterday across all repos" is unanswerable without visiting 47 pages.

Primary task: scan a time-ordered stream of sessions, narrow it, open one.
Protected functions: the row label (summary), the four sort metrics, the
project affiliation, the provider badge, the row link target.

**Decision:** this is a *product* surface, not brand. Density and column
alignment beat visual charm. Anything that makes a row prettier and the
scan slower is rejected.

## 2. Information architecture

Route `/sessions`. Server-rendered, state entirely in the query string.

| Param | Values | Default | Notes |
|---|---|---|---|
| `q` | free text | "" | summary substring OR session-ID prefix |
| `provider` | `claude-code` \| `codex` \| `grok` | all | `config.NormalizeProvider` already accepts `cc`/`cx`/`gx` |
| `project` | encoded project name | all | same encoding as `/project/<name>` |
| `model` | substring | "" | matches `Session.Model`, case-insensitive |
| `after`/`before` | `YYYY-MM-DD` | none | `config.ParseDate` / `ParseBeforeDate` (local tz, before = end-of-day) |
| `group` | `none`\|`project`\|`day`\|`provider`\|`model` | `none` | |
| `sort` | `time`\|`messages`\|`prompts`\|`tokens` | `time` | always descending |
| `limit` | int, `0`=all | `100` | applied *after* sort, *before* grouping |

**Decision:** params at their default are omitted from generated URLs (JS
strips empty fields before submit). Shareable links stay readable.

**Decision:** no `offset`/pagination. The auditor narrows with filters, not
pages; `limit` + "show all" covers the tail. **Open:** revisit if scroll
depth turns out to be the common path.

**Decision:** no `cc:` / `cx:` prefix parsing in `q` (the index page does
this via `parseProviderQuery`). There is a provider dropdown two inches
away; silently reinterpreting typed text is the implicit magic CLAUDE.md
rejects.

Sidebar (`renderSidebar`, templates.go:3232): insert
`{"/sessions", "Sessions", "sessions"}` **second, after Projects**.
Mirrors the hierarchy projects -> sessions and is the second-most-used
destination. Active key: `"sessions"`.

Server: `handleSessions` follows `handleIndex`'s shape.
`sessionProvider.DiscoverProjects()` already returns quick-parsed sessions
(disk-cached), so the page costs one flatten + one sort over ~2,300
structs — no new parse cost. Carry `(project.EncodedName, *parser.Session)`
pairs; `Session.ProjectName` alone is not enough to build the row href.

Filtering: reuse `parseSessionFilter(r)` (server.go:1646) — it already
covers provider/after/before/q/model. **Trap:** `SessionFilter.Match`
matches `q` against summary only, and returns false as a whole. To get
"summary OR ID prefix", split it:

```go
base := parseSessionFilter(r); qStr := base.Query; base.Query = ""
if !base.Match(s) { continue }
if qStr != "" && !containsFold(s.Summary, qStr) && !hasPrefixFold(s.ID, qStr) { continue }
```

Do not widen `SessionFilter.Query` itself — the CLI shares it.

## 3. Control bar

One instrument, three lines, and the third only appears when it has
something to say.

```
+--------------------------------------------------------------------------+
| [ Filter sessions... (press /)      ] [All providers v] [All projects v]  |
| Group: [None v]  Sort: [Recent v]                                         |
| v More filters                                                            |
| q: recovery [x]  provider: Codex [x]  after: 2026-08-01 [x]   Clear all   |
+--------------------------------------------------------------------------+
```

Expanded disclosure:

```
| ^ More filters                                                            |
|   Model [ e.g. opus        ]  After [ 2026-08-01 ]  Before [ 2026-08-10 ] |
```

**Decision:** everything lives in one `<form method="get" action="/sessions">`.
Enter in the text field submits natively; that is the zero-JS path.

**Decision:** high-frequency controls (q, provider, project, group, sort)
stay visible; low-frequency ones (model, after, before) go behind a native
`<details>` labelled "More filters". Five visible controls is an
instrument; eight is a junk drawer.

**Decision:** the hidden-state risk of a disclosure is paid for by the chip
row — any set filter, visible or collapsed, renders as a chip. If a filter
is on, you can see it without opening anything.

**Decision:** filters that have a closed value set are `<select>`
(provider, project, group, sort). Filters with an open value space are text
(`q`, `model`) or `<input type="date">` (after, before). Native date input
gets a picker for free and degrades to a text field with the right format
hint.

**Decision:** the project `<select>` lists every project alphabetically by
display name with its session count — `ccx-codex (312)`. Alphabetical
because you are looking for a name you already know; the count because it
tells you whether the filter is worth applying. Native select typeahead
makes 47 options survivable.

Exact copy:

- search placeholder: `Filter sessions... (press /)`
- provider options: `All providers` / `Claude Code` / `Codex` / `Grok`
- project options: `All projects` / `<display name> (N)`
- `Group:` -> `None` `Project` `Day` `Provider` `Model`
- `Sort:` -> `Recent` `Messages` `Prompts` `Tokens`
- disclosure summary: `More filters`
- field labels: `Model` `After` `Before`
- chips: `q: recovery`, `provider: Codex`, `project: ccx-codex`,
  `model: opus`, `after: 2026-08-01`, `before: 2026-08-10`
- clear-all: `Clear all`

**Decision:** each chip's remove control is an ASCII `x` link to the same
URL minus that one param, separated from the value by a hairline. `Clear
all` links to bare `/sessions`. Both work with JS off — clearing a filter
must never depend on script.

**Decision:** no sort-direction toggle. Recent-first / most-first is the
only ordering an auditor wants; ascending has no use case. **Open:** if it
appears, `sort=time-asc` rather than a second control.

**Decision:** the control bar is **not** sticky. Group headers are, and two
sticky layers stack into a growing header. Filtering is set-and-scan, not
continuous.

## 4. List item: dense row, not card

**Decision: rows, not cards.** A `.session-card` costs ~92px with its gap;
a row costs ~44px. At a project's 10-40 sessions the card box usefully says
"one unit"; across 2,300 it just halves the sessions per screen and
destroys column alignment. The global list is a scan, so it gets
table-like rows with fixed-width cells that line up down the page.

Width check (no new layout class needed): `.main-content` is 800px max with
40px padding = 720px usable ~= 92ch at 13px mono. The row needs 81ch. It
fits inside the existing shell — **Decision:** no wide-main variant, the
app keeps one page width.

```
+--------------------------------------------------------------------------+
| CC  Fix session lookup recovery for encoded project names       2h ago    |
|     ccx-codex           opus-4-6      184 msg   12 prompts    1.2M tok    |
+--------------------------------------------------------------------------+
| CX  Add content-signal ranking to search                        5h ago    |
|     mining-pipeline     gpt-5.4        92 msg    7 prompts    412k tok    |
+--------------------------------------------------------------------------+
| GX  Sweep the vault index                                       yesterday |
|     vault               grok-4          31 msg        -           -       |
+--------------------------------------------------------------------------+
```

Line 1: provider badge, label, relative time (right, fixed 10ch).
Line 2: project (22ch), model (20ch), then three stat cells (11ch each,
right-aligned, `tabular-nums`).

**Decision: every sort key is a visible column, and nothing sorts by an
invisible number.** `sort` offers time/messages/prompts/tokens, so the row
shows exactly those four. Tool calls (correlated with messages) and
duration (wall-clock includes idle, so it misleads) are dropped from the
row and moved into the tooltip.

**Decision:** label = `Title` if non-empty, else `Summary`, else
`(no summary)`. **Fact worth knowing before you build it:** `Session.Title`
is populated only on the *full* parse path (`internal/parser/session.go:162`).
No list path sets it — `quickParseListableSession`
(`internal/parser/project.go:120`) has no Title field, `SessionMeta`
(session.go:507) has no Title, and the grok backend folds its title into
`Summary` (`internal/provider/grok/backend.go:260`). So today the label is
always Summary. Write the precedence anyway (it is free and future-proof),
but design no affordance that assumes a Title exists. Lighting it up is a
two-file change: add `Title` to `SessionMeta`, set it in `quickParseSession`,
copy it in `quickParseListableSession`.

**Decision:** the row's `title` attribute carries what the line cannot:
`2026-08-11 14:02 -> 15:47 (1h 45m) | 47 tool calls | <full summary>`.
Uses the existing `formatDuration`.

**Decision:** missing stats render as a dim ASCII `-`, not a blank or a
zero. Grok reports `MessageCount` only (backend.go:264-275); a blank breaks
column alignment and a `0` is a lie. Cell gets
`title="not reported by this provider"`.

**Decision:** project affiliation is plain text inside the row link, never a
nested link (invalid HTML, and it steals the click target). Filtering by
project is the dropdown's job.

**Decision:** when `group=project`, the project cell is dropped from line 2
— the group header already says it. Same for `group=model` and the model
cell, `group=provider` and the badge (badge stays; it is 4ch and identifies
the row when scrolled past the header — **Open:** drop it if it reads as
noise).

**Decision:** cost is not on the row. `Stats.CostUSD` is zero for any
unpriced model, and a column that is blank for two of three providers earns
nothing. **Open:** cost as a group aggregate, shown only when > 0 and
labelled as an estimate.

Row href: `/session/<encodedProject>/<sessionID>`. Time shown and sorted is
`EndTime` (last activity) — matches `handleProject`'s sort.

## 5. Group headers

```
ccx-codex                                    312 sessions  4.1M tok  2h ago
---------------------------------------------------------------------------
| CC  Fix session lookup recovery ...                             2h ago   |
```

**Decision:** sticky at `top: 48px` (under the fixed top-nav) in every group
mode. The value of grouping is knowing where you are seven screens down.
Opaque `background: var(--bg)` so rows do not bleed through.

Anatomy: label (left, 600 weight, 12px) + meta (right, 11px, muted). Meta
is separated by ` · ` in `--text-faint`.

| `group` | Label | Meta | Group order |
|---|---|---|---|
| `project` | display name | `N sessions · <tok> tok · <last activity>` | most recent session first |
| `day` | `Mon 2026-08-11`, with ` · today` / ` · yesterday` appended | `N sessions · N projects · <tok> tok` | newest day first |
| `provider` | badge + `Claude Code` | `N sessions · N projects · <tok> tok` | fixed CC, CX, GX |
| `model` | full model id, or `(no model recorded)` | `N sessions · N projects · <tok> tok` | most recent session first |

**Decision:** one ordering rule for all modes — *groups are ordered by their
most recent session; rows inside a group follow `sort`.* Predictable beats
clever. Day groups therefore descend by date for free. **Open:** ordering
groups by the sort metric's aggregate (e.g. biggest-token project first) if
users ask.

**Decision:** the model label is the full id (`claude-opus-4-6-20260514`),
not a prettified short name. It is the string you type into the model
filter; shortening it breaks the loop between the two.

**Decision:** group counts describe the *shown* set, not the matching set.
The page footer carries the honesty about truncation (section 6), so
headers stay short.

## 6. Limit and "show more"

**Decision: default `limit=100`.** ~7 screens of rows: enough to feel like
the whole recent record, ~40KB of HTML, instant. All 2,314 rows is ~900KB
and a visible layout stall — that has to be opt-in.

Footer, always rendered:

```
Showing 100 of 312 matching sessions      Show 500      Show all 312 (slower)
```

Unfiltered: `Showing 100 of 2,314 sessions`.
Not truncated: `Showing all 312 matching sessions` and no links.

**Decision:** "show more" is two plain links (`?limit=500`, `?limit=0`), not
a button and not infinite scroll. Links are shareable, keyboard-reachable,
and work with JS off. The `(slower)` suffix is the honest cost label.

**Decision:** `limit` applies to the sorted list *before* grouping. Groups
form from the limited set. Grouping is a view of what you are looking at,
not a second query.

## 7. Empty and degraded states

No match (filters are set):

```
No sessions match these filters.
Clear filters
```

Cold start (no sessions exist at all):

```
No sessions found. ccx reads ~/.claude, ~/.codex and ~/.grok —
start an agent session first.
```

**Decision:** both use `.empty-state`, which currently lives inside
`memSectionCSS()` (templates.go:3187) and is therefore unavailable on any
page without memory files. Promote it to `style.css` and delete the
duplicate in the same commit — a contract that lies about where a class
lives is worse than no contract.

Zero-JS behavior, end to end:

| Control | Without JS |
|---|---|
| `q`, `model`, dates | Enter submits the form |
| selects | `<noscript>` renders `Apply` (styled as `.sort-select`) |
| chips `x`, `Clear all` | links, always work |
| `Show 500` / `Show all` | links, always work |
| `j`/`k`, `/`, `d` | absent; Tab + Enter reach every row |

**Decision:** the `Apply` button is inside `<noscript>` rather than always
visible. With JS it is redundant (selects auto-submit); a permanently dead
button is worse than an absent one.

## 8. CSS plan

Reused as-is — no changes: `.layout`, `.sidebar`, `.sidebar-link`,
`.main-content`, `.page-header`, `.page-badge`, `.badge-session`, `.stats`,
`.controls`, `.search-wrap`, `.search-input`, `.search-spinner`,
`.sort-controls`, `.sort-label`, `.sort-select`, `.provider-badge` +
`.provider-CC/CX/GX` (via `providerBadgeHTML`), `.site-footer`, the global
`:focus-visible` outline, and every token.

Page header block: `.page-header` + `<span class="page-badge badge-session">S</span>`
+ `<h1>Sessions</h1>` + `.stats` reading `2,314 sessions across 47 projects`
(or `312 of 2,314 sessions · 12 projects` when filtered).

New classes (key properties only; all values are tokens):

| Class | Key properties |
|---|---|
| `.controls-wrap` | modifier on `.controls`: `flex-wrap: wrap; row-gap: 8px` |
| `.filter-more` | `flex-basis: 100%; font-size: 12px` (native `<details>`, native marker) |
| `.filter-more > summary` | `cursor: pointer; color: var(--text-muted)` |
| `.filter-more-body` | `display: flex; gap: 12px; align-items: center; padding-top: 8px` |
| `.filter-field` | `display: flex; align-items: center; gap: 6px` (label reuses `.sort-label`) |
| `.filter-input` | same box as `.sort-select`: `padding: 6px 8px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg); color: var(--text); font-size: 12px` |
| `.filter-chips` | `display: flex; flex-wrap: wrap; gap: 6px; margin: -8px 0 16px` |
| `.filter-chip` | `display: inline-flex; align-items: center; gap: 6px; padding: 2px 6px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--bg-secondary); font-size: 12px; color: var(--text-muted)`; inner `<b>` = `color: var(--text); font-weight: 600` |
| `.filter-chip-x` | `border-left: 1px solid var(--border); padding-left: 6px; color: var(--text-faint)`; hover `color: var(--primary)` |
| `.filter-clear` | `color: var(--primary); text-decoration: none; font-size: 12px` |
| `.sgroup` | `margin-bottom: 16px` |
| `.sgroup-head` | `position: sticky; top: 48px; z-index: 5; display: flex; align-items: baseline; gap: 8px; padding: 6px 0; background: var(--bg); border-bottom: 1px solid var(--border)` |
| `.sgroup-label` | `font-size: 12px; font-weight: 600` |
| `.sgroup-meta` | `margin-left: auto; font-size: 11px; color: var(--text-muted)` |
| `.session-rows` | `display: flex; flex-direction: column; border-top: 1px solid var(--border)` |
| `.srow` | `display: block; padding: 7px 8px; border-bottom: 1px solid var(--border); text-decoration: none; color: inherit`; `:hover`/`:focus` -> `background: var(--hover)` |
| `.srow-top` | `display: flex; align-items: baseline; gap: 8px` |
| `.srow-label` | `flex: 1; min-width: 0; font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis` |
| `.srow-time` | `flex-shrink: 0; width: 10ch; text-align: right; font-size: 11px; color: var(--text-muted); font-variant-numeric: tabular-nums` |
| `.srow-meta` | `display: flex; gap: 12px; margin-top: 2px; font-size: 11px; color: var(--text-muted)` |
| `.srow-project` | `width: 22ch; flex-shrink: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis` |
| `.srow-model` | as `.srow-project` at `width: 20ch` |
| `.srow-stat` | `width: 11ch; flex-shrink: 0; text-align: right; font-variant-numeric: tabular-nums` |
| `.srow-none` | `color: var(--text-faint)` (the `-` placeholder) |
| `.list-footer` | `display: flex; align-items: center; gap: 12px; padding: 12px 0; font-size: 12px; color: var(--text-muted)` |
| `.empty-state` | promoted from `memSectionCSS()` unchanged |

**Decision:** row hover/focus feedback is a background change only. No
`box-shadow` (0002 finding 6 caps shadows at dropdowns and pressed/hover
elevation; a 44px hairline row is not elevated), no transform, no border
recolor that would fight the hairline grid.

**Decision:** no `data-provider` attribute on rows. Provider filtering is
server-side now; the index page's client-side hide-the-cards filter
(`indexJS`, templates.go:3466) is not reused here.

## 9. Interaction spec

New `sessionsJS()` (~45 lines). **Decision:** do not reuse `indexJS()` for
the controls — it binds `#search`/`#sort`/`#provider-filter` by id with
different param semantics and would fight the form. Do keep the shared
theme-toggle and global-search bindings by including `indexJS()` as well,
and give every form control a `s-`-prefixed id (`s-q`, `s-provider`,
`s-project`, `s-group`, `s-sort`) so nothing collides.

| Key | Action | Guard |
|---|---|---|
| `/` | focus `#s-q` | not already in an input |
| `Esc` | blur the filter input | |
| `j` | focus next `.srow` | `!e.target.matches('input, textarea, select')` |
| `k` | focus previous `.srow` | same |
| `Enter` | open the focused row | native `<a>` behavior |
| `d` | toggle theme | same guard |

**Decision:** `j`/`k` move real DOM focus (`el.focus()` +
`scrollIntoView({block:'nearest'})`) rather than maintaining a separate
`.active` class. Free focus ring from the global `:focus-visible`, free
Enter-to-open, free screen-reader position, zero new CSS.

**Decision:** `/` focuses *this page's* filter, not the top-nav global
search. On this page the filter is the instrument. This diverges from
index/project, where `indexJS` prefers the nav search even though the
placeholder says `(press /)` — **Open:** align those pages afterwards; the
current behavior there contradicts its own copy.

**Decision:** implement `d` here. The top-nav button has advertised
`Toggle theme (d)` since the redesign (templates.go:3208) and no page binds
the key — grep for a `d` handler returns nothing. Four lines fixes a
documented-but-absent shortcut.

**Decision:** every control submits through a 400ms debounce, the same one
`indexJS` uses for search. This is not only for typing: Firefox fires
`change` while arrow-keying through a closed `<select>`, and an
un-debounced auto-submit makes keyboard selection impossible. One code
path solves both.

Before submit, JS disables empty-valued fields so they are omitted from the
query string. Without JS you get `?q=&model=` — ugly, still correct.

## 10. Responsive, dark, reduced motion

Breakpoints per 0002: 1024px and 700px only.

- **<= 1024px:** hide `.srow-model` (the least load-bearing cell). Control
  bar already wraps.
- **<= 700px:** `.search-wrap { max-width: none; flex-basis: 100% }`;
  `.srow-project { width: auto; flex: 1 }`; hide the prompts stat (keep
  msg + tok); `.sgroup-meta` shows the session count only. Row stays two
  lines — it must not become three.

**Flag (pre-existing, do not fix here):** the shell still hides `.sidebar`
at 900px (style.css:2548) and has 768/600 queries, which contradicts the
0002 two-breakpoint rule. Moving 900 -> 1024 touches every page; file it
separately.

Dark mode rides the tokens — every new class uses `--bg`, `--border`,
`--text-muted`, `--text-faint`, `--hover`. Two things to flag:

1. **Contrast defect the row list will multiply:** `.provider-badge` sets
   `color: #fff` on `--accent-cc/cx/gx`, which in dark mode are *light*
   (`#e08662`, `#3fb950`, `#a371f7`). White on `#3fb950` is ~2.1:1. One
   badge per row makes this the most-repeated element on the page. Fix:
   `[data-theme="dark"] .provider-badge { color: var(--bg); }`.
2. `--text-faint` (`#97908a` on `#fcfbfa`, ~2.9:1) fails small-text
   contrast. Restricted here to the `-` placeholder and ` · ` separators,
   where low contrast is semantically correct. All readable meta uses
   `--text-muted` (~4.9:1).

Reduced motion: the page adds **no** animation. Hover is a color change,
which the global reduce block correctly preserves (0002 finding 4). The
only motion is the existing `.search-spinner`.

## 11. Inherited defects worth naming

- `pageHeader()` loads Tailwind from a CDN (templates.go:3272), which
  contradicts CLAUDE.md's "single binary, no CDN". The new page inherits
  it; it should be removed repo-wide, not worked around here.
- `.empty-state` living in `memSectionCSS()` (section 7).
- The `d` shortcut advertised but unbound (section 9).
- `/` focus inconsistency on index/project (section 9).

## 12. Tests

Follow `internal/web/server_test.go`'s table style: param parsing
(defaults, `limit=0`, bad dates), the q = summary-OR-ID-prefix split,
group bucketing and ordering for each mode, and limit-before-group.
Render assertions: chip row appears iff a filter is set, footer copy
switches between truncated and complete.

## 13. Reconciliation with the in-flight implementation

`internal/web/sessions.go` (untracked at time of writing, ~493 lines)
already implements a working v1: route, `sessionsQuery` parsing, the four
group modes, `defaultSessionsLimit = 100`, a JSON endpoint, and
`sessionsPageURL` — which composes params correctly and even decrements the
`before` end-of-day back to the date the user typed. **Keep all of that.**
This spec is the second pass over the surface, not a rewrite.

Deltas, in rough priority order:

| # | Area | Today | Spec | Why |
|---|---|---|---|---|
| 1 | List item | `.card .session-card`, 3 lines | `.srow`, 2 lines | halves rows per screen and no columns align (§4) |
| 2 | Row stats | messages, **tools**, tokens | messages, **prompts**, tokens | `sort=prompts` exists but the number is invisible; tools is visible but not sortable (§4) |
| 3 | `model` / `after` / `before` | parsed, no UI | "More filters" disclosure | three params reachable only by hand-editing the URL |
| 4 | Active filters | invisible | chip row with per-param `x` | with §3 landed, set filters must be visible and clearable |
| 5 | Zero-JS | selects are JS-only, no `<form>` | `<form method="get">` + `<noscript>` Apply | page is unusable without script today |
| 6 | Group header | `.group-header`, static | `.sgroup-head`, sticky `top: 48px` | you lose your place seven screens down |
| 7 | Group meta | `⧫ <tokens>` | `N sessions · <tok> tok · <last>` | CLAUDE.md: text labels over cryptic symbols. **Open:** `⧫` is an established idiom on the project page; changing it here diverges until that page follows |
| 8 | Select submit | immediate on `change` | 400ms debounce | Firefox fires `change` while arrow-keying a closed select — keyboard selection currently navigates away mid-choice |
| 9 | Project select | discovery order, no counts | alphabetical + `(N)` | 47 options in recency order is unsearchable |
| 10 | Footer | `Show all N sessions` | `Showing X of Y` + `Show 500` + `Show all (slower)` | truncation should be stated, not implied by the presence of a link |
| 11 | Keyboard | `/` only (via `indexJS`) | `/`, `j`, `k`, `d` | §9 |
| 12 | Label | `Summary` | `Title` else `Summary` | free, and pre-wires the quick-parse Title work (§4) |

**Live bug, independent of any of the above:** `sessions.go:282` renders
`<div class="empty-state">`, but `.empty-state` is defined only inside
`memSectionCSS()` (templates.go:3187), which this page never emits. The
empty state currently renders as unstyled body text. Promoting the class to
`style.css` (§7) fixes it.
