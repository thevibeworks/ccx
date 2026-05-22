---
name: ccx-insight
description: >
  Generate rich scoped intelligence summaries from ccx session data. Use when
  the human asks what happened today/this week/this month, what is active,
  what still needs closure, what was achieved, or what patterns are emerging
  across agent sessions.
---

# ccx-insight

`ccx insight` is deterministic session intelligence. This skill is the
interpretive layer: it turns the JSON into a crisp human briefing with
judgment, caveats, and next actions.

Use this skill when the human asks:

- "what are we working on?"
- "summarize today / this week / this month"
- "what still needs to be closed?"
- "what did the agents achieve?"
- "what patterns or blockers are emerging?"

## Modes

| Trigger | Command |
|---|---|
| `/ccx-insight` | `ccx insight --json` |
| `/ccx-insight week` | `ccx insight week --json` |
| `/ccx-insight month --tz Asia/Shanghai` | `ccx insight month --tz Asia/Shanghai --json` |
| "deep insight" | run `ccx insight --json`, then inspect selected sessions/traces |

## Process

1. Pick the scope:
   - default: `today`
   - accepted: `today`, `week`, `month`, `quarter`, `year`
2. Preserve timezone. If the user names one, pass `--tz <IANA name>`.
   If they do not, use `--tz local`.
3. Run:
   ```bash
   ccx insight <scope> --tz <timezone> --json
   ```
   Add `--all` for cross-project summaries or `--project <name>` for one
   project.
4. Read the JSON. Treat it as evidence, not the final story.
5. For deep mode, inspect the top current/open sessions:
   ```bash
   ccx trace <session-id> --output "$TMPDIR/ccx-trace-<id>.json"
   ```
6. Write a concise briefing.

## Briefing Shape

Use this structure:

```text
Scope: Today, America/Los_Angeles

Currently active
- ...

Needs closure
- ...

Completed / achieved
- ...

Emerging signals
- ...

Next move
- ...
```

## Rules

- Do not invent project status beyond the evidence.
- Separate deterministic metrics from judgment.
- Label weak signals as weak.
- Use exact time scope and timezone in the first line.
- If no sessions exist in scope, say that directly and suggest the nearest
  broader scope.
- Keep the final briefing short unless the human asked for deep detail.

## Mental Model

Insight is not a report card. It is a steering surface.

Agents are tiny taskmasters: nudging, prodding, and occasionally cracking the
whip when humans drift. The briefing should help the human decide what to do
next, not admire what the agents wrote.
