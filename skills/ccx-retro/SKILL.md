---
name: ccx-retro
description: >
  Where did the agent go wrong, what fixed it, and what should change
  so it doesn't happen again? Mine a coding-agent session for
  mistakes, corrections, and wasted loops, then propose durable rule
  changes (CLAUDE.md / AGENTS.md / skills / memory) with evidence.
  Use after a frustrating session, when the human asks "what went
  wrong", "why did it do that", "retro", or "turn this into a rule".
---

# ccx-retro

Recap tells you what happened. Retro makes the next run better. The
real output is not a report — it is a **proposed patch to the
instructions the agent already reads** (CLAUDE.md, AGENTS.md, a
skill, memory), each line justified by evidence from the session.

## Process

1. **Outline.** `ccx trace --json`. Failure signals are factual and
   already in it:
   - steps with `errors` (tool failures, attributed to the step that
     issued the call)
   - error streaks: consecutive steps failing at the same thing
   - turns where the user pushed back — read the actual `user_text`
     and judge with your own comprehension; there is no correction
     flag, because keyword matching lies
   - cost spikes without matching edits (spinning)
   - `warnings` (evidence gaps are findings too)
2. **Drill.** `ccx trace --turn N` on each suspect. Reconstruct:
   what did the agent believe, what was actually true, what evidence
   was available at the time, what finally corrected it (user, test,
   self-check)?
3. **Classify each finding.**
   - `mistake` — agent had the evidence and got it wrong
   - `friction` — environment/tooling failed the agent
   - `drift` — agent misread intent; user had to steer
   - `save` — something caught the problem (name what: a test, a
     verification habit, a user correction). Saves become rules too.
4. **Propose changes.** For each finding worth keeping, draft the
   exact edit: file, section, wording. A rule earns its place only if:
   - it would have prevented or shortened the observed failure, AND
   - the next agent cannot derive it from code, docs, or common sense
   Fewer, sharper rules beat a rulebook. One-off accidents are
   reported, not legislated.
5. **Human gate.** Present findings + proposed patches and STOP.
   Never write to CLAUDE.md / AGENTS.md / skills / memory without
   explicit approval. Rejected proposals are dropped, not stashed.

## Output shape

```
Retro — <session> | <n> findings

 #  Kind      Finding (cited)                          Proposed change
 1  mistake   Assumed X, evidence said Y [#12.3-12.7]  CLAUDE.md: "check X before Y"
 2  friction  3 loops on flaky sandbox error [#9.2]    none — one-off, monitor
 3  save      Probe test killed a rewrite [#133.1-3]   AGENTS.md: "profile before rewrite"

Patches for approval:
--- CLAUDE.md
+ <exact line(s)>
```

Every finding cites `#turn.step`. If nothing is worth changing, say
so — "no durable lessons this session" is a valid retro.

## Hard rules

- Findings without receipts don't ship.
- Judge corrections by reading the exchange, not by keywords.
- Propose the smallest rule that prevents the failure class.
- Do not create new knowledge stores; patch the ones the agent
  already loads.
- Preserve the user's exact wording when their correction becomes a
  rule — their phrasing carries the intent.
