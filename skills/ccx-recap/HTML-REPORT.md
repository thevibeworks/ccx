# ccx-recap HTML cockpit

The HTML cockpit is the human review surface for a time window: the
judgment layer written on top of `ccx insight --json` (ccx.insight.v1).
It is not a prettier transcript and not a dashboard toy. A reader must
answer five questions from it in a minute:

1. What was actually worked on in this window, across every workspace?
2. What is observed, human-stated, agent-claimed, inferred, unverified?
3. What is still open and deserves the next push?
4. Where is the evidence behind each claim (`session:line`, `#turn.step`)?
5. What in the data could mislead the review?

## Evidence first

Regenerate the evidence in the current environment; never reuse an old
file unless the human asks:

    ccx insight --scope <scope> --tz <tz> --all --json > /tmp/insight.json

`ccx insight --scope <scope> --all` (no `--json`) writes the facts-only
cockpit ccx renders itself into the insights dir; the recap cockpit is
the same data plus judgment. Read the digest whole before writing:
`metrics` (with `cost_coverage`), `workspaces[]`, `sessions[]`
(`prompts[]`, `final_answer`, `edits`, `commits`, `interrupts`,
`denials`, `cost_usd` + `cost_status`, `relation`), `models[]`, `days[]`.
Then `ccx trace <id>` the sessions that decide the story and cite
`#turn.step` for anything you call verified.

## Output contract

One self-contained HTML file that opens with `file://`: inline CSS, no
remote fonts, CDNs, images or build steps. Inline JavaScript only for
filtering, searching and expanding details; the page must stay useful
with JavaScript disabled. Add a print stylesheet that keeps the
workstream board, claim ledger, needs-closure list and caveats.

Destination: `$XDG_DATA_HOME/ccx/insights/<date>-<slug>.html` (usually
`~/.local/share/ccx/insights/`) so it shows at `/insights` in `ccx web`;
`ccx web` serves these files raw and rejects subpaths, so every asset
must be inline. Use the OS temp dir for a throwaway; write into a repo
only when the human asks for a durable project artifact, and never
with real session ids or costs into a public repo.

## Information architecture

1. **Scope header** — title `Session Intelligence`; the window's exact
   start/end, timezone, `all projects` or the workspace, generated
   time, the evidence command, the ccx version.
2. **TL;DR judgment band** — one paragraph with a confidence chip:
   the dominant pattern, the strongest correction to a naive reading
   of the logs, the next pressure point. Not a list of everything.
3. **Data quality** — sessions vs source files, long-running
   containers, provider split, sidechain share, **cost coverage**
   (which models are unpriced and how many tokens that is), truncation.
   Bad boundaries make bad decisions; put this before the narrative.
4. **Metrics strip** — sessions, workspaces, human prompts, tool calls,
   edits, commits, interventions, tokens, cost (with coverage).
5. **Workstream board** — the primary surface. Four columns: `In
   motion`, `Needs closure`, `Done / achieved`, `Watch`. Group by
   workstream (a workspace, or a theme that spans workspaces), not by
   session. Each card: title, workspace path, provider mix, session
   count, what happened, why it matters, next action, confidence,
   evidence refs. No nested cards, no drag/drop.
6. **Timeline** — curated events, not every record: human corrections
   and interrupts, explicit requirements, commits/merges/releases,
   blockers, completion signals. Each with local time, provider badge,
   workspace, `session:line`.
7. **Claim ledger** — every consequential claim: status (`observed`,
   `human-stated`, `agent-claimed`, `inferred`, `unverified`,
   `contradicted`), confidence, who made it, why it is classified so,
   evidence refs. `Done / achieved` admits only `observed` or cited
   `human-stated` items; an assistant's final message is never
   completion evidence on its own.
8. **Decisions and corrections** — what changed, who decided (human,
   agent, mixed), rationale, the rejected alternative when visible.
   Human corrections outrank any agent summary.
9. **Needs closure** — open loops with current state, why still open,
   the next command or review action, evidence refs, owner only when
   the logs make one clear.
10. **Evidence drawer** — compact cited records (`details`/`summary`
    for snippets): provider, session prefix, time, workspace, kind,
    source path, line. Never paste transcripts.
11. **Caveats** — missing homes, unpriced models, heuristic labels,
    ambiguous session boundaries, workspaces not mounted where the
    report was written.

## Rules

- Quote cost with its coverage; `$0` on an unpriced model is not free.
- Do not equate "latest" with "active", or completion words with
  completed work, or a month-long container with one day's work.
- Every number comes from the digest or a trace; none are estimated.
- The fleet is the subject: one workspace never stands in for it.
- Keep the conversational summary short; the cockpit carries the depth.
