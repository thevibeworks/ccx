---
name: ccx-recap
description: >
  What did the agent actually do? Recap a coding-agent session or a
  time window from ccx evidence: the story, the decisions, what was
  verified vs merely claimed, mistakes and cost. Use after stepping
  away from an autonomous run, at end of day, or whenever the human
  asks "what happened", "what did it do", "recap", "summarize the
  session", or "what did I work on today/this week". Routing: this
  skill answers "what happened"; for "what went wrong / what should
  change", use ccx-retro.
---

# ccx-recap

One question: **what did the agent actually do?** Answer it with
receipts, not vibes. The agent's own final message is a claim, not
evidence — the recap is built from the trace.

## Scope

| Trigger | Evidence source |
|---|---|
| `/ccx-recap` | Latest session here: `ccx trace` |
| `/ccx-recap <session-id>` | That session: `ccx trace <id>` |
| `/ccx-recap today` / `week` | Fleet: `ccx insight --scope <s> --json`, then `ccx trace` per notable session |

Timezone: preserve the user's. Warn when a `--tz` offset makes "today"
differ from their local day (e.g. `--tz +8` from a US machine).

This skill describes ccx >= 0.8. If a command or flag here is missing
(`unknown flag: --turn`), the binary is older than the skill: upgrade
ccx, then run `ccx skills install` — the binary embeds skills matching
its own CLI surface.

## Process

1. **Outline first.** `ccx trace --json` (or `ccx trace` text) — every
   turn and step headline with tools/edits/errors/cost. This always
   fits; read it whole. Steps are the agent's own narration at the
   moment it acted — that sequence IS the story skeleton.
2. **Pick what matters.** Turns with edits, errors, linked commits,
   high cost, or user pushback. Ignore command noise (`is_command`).
3. **Drill only there.** `ccx trace --turn N` for full narration,
   mutations, and error attribution. Raw text when needed: the
   `anchor_id` / `message_id` point into the session file.
4. **Verify before claiming.** "Done" requires evidence: a passing
   test step, a linked commit, a verification narration. Tag every
   important claim:
   - `observed` — tool output / git / trace shows it
   - `agent-claimed` — the agent said it; nothing independent checked
   - `unverified` — plausible, not checked
5. **Write the recap.** Conversation reply first; offer an HTML
   artifact for sessions worth keeping.
6. **Land it.** A recap that ends at a chat reply evaporates — the
   next session re-pays the full archaeology. Offer to write the
   distillation into whatever durable store the user already runs
   (devlog, memory file, wiki/vault page; never invent a new store).
   Keep the claims inline and cite the session as provenance:
   `[ccx:<session-id> #turn.step]` — anyone can re-open the receipt
   with `ccx trace <session-id> --turn N`. If the user declines or
   has no store, the reply is the deliverable.

## The 60-second test

A reader must know, within a minute: what was asked, what the agent
did about it, the 3-5 decisions that shaped the work, what is verified
vs claimed, what it cost, and what is still open. If the recap can't
produce that paragraph, it is not done.

Format (conversation or HTML):

```
Recap — <session/scope> | <duration> | $<cost> | <files> files | <errors> tool errors

What happened      3-8 sentences, chronological, cited [#turn.step]
Decisions          the real ones only, with the rejected alternative
Verified vs claimed anything important that lacks evidence, flagged
Still open         unfinished work, unverified claims, warnings
Numbers            small table: turns, steps, edits, errors, cost
```

Citations are `#turn.step` (e.g. `#133.9`). Copy trace `warnings`
into the recap — evidence gaps are part of the answer.

## Multi-session (today/week)

Use insight's pre-computed `days[]`, `providers[]`, `workspaces[]` for
tempo and where-the-work-went; then outline the 3-5 sessions that
dominated (records, cost, or the user's stated interest) and recap
each in 1-3 sentences. Never re-bucket raw records yourself.

## Hard rules

- No judgment without a citation. No citation, say "unverified".
- Do not paste transcripts; the recap is the distillation.
- Numbers come from the trace/insight, never estimated.
- The final assistant message is input, not truth: check it against
  the evidence like any other claim.
