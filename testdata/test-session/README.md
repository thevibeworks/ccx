# test-session fixtures

Real captured sessions for both providers, used as the ground-truth input
for ccx's end-to-end parser and renderer tests. Generated on 2026-04-15 by
running the prompts from
[`2026-04-15-agent-loop-vocabulary-survey.org`](../../docs/devlog/2026-04-15-agent-loop-vocabulary-survey.org)
(section **Test Prompts for Fixture Generation**) against real CLIs.

These files are **committed verbatim** — no scrubbing, no anonymisation.
The sessions were run in a sandboxed scratchpad (`/tmp/ccx-fixture/{cc,cx}`)
against a throwaway task, so nothing sensitive is inside.

If you regenerate either fixture, **bump the filename** rather than
overwriting. Old fixtures keep their value as regression evidence that a
previous CLI version produced a specific rollout shape.

---

## File manifest

```
testdata/test-session/
├── README.md                                             (this file)
├── claude/
│   └── projects/test-session/
│       ├── de4b5d69-0744-4202-93f0-4d329a3dac3b.jsonl    (main session, 134 lines, 244 KB)
│       └── de4b5d69-0744-4202-93f0-4d329a3dac3b/
│           └── subagents/
│               ├── agent-a2b0aaeb88f277707.jsonl         (sub-agent sidechain, 49 lines)
│               └── agent-a2b0aaeb88f277707.meta.json     ({"agentType":"Explore",…})
└── codex/
    └── sessions/2026/04/15/
        └── rollout-2026-04-15T01-44-47-019d9050-…2cd93b.jsonl   (96 lines, 208 KB)
```

## Provenance

| Fixture      | Captured         | CLI version | Source prompt                     |
|--------------|------------------|-------------|-----------------------------------|
| Claude Code  | 2026-04-15 08:48Z| 2.1.104     | Prompt A in the devlog's §Test Prompts |
| CC sub-agent | same             | same        | TaskCreate dispatch inside Prompt A step 7 |
| Codex        | 2026-04-15 08:47Z| 0.120.0     | Prompt B in the devlog's §Test Prompts |

Both sessions ran with `permissionMode=bypassPermissions` so every tool
call actually executed. The Claude Code custom-title `update-readme-frequency-highlights`
is the agent's auto-generated session name from that run.

---

## Claude Code fixture coverage

### Top-level JSONL line types (from `jq -r '[.type, .subtype // empty] | join("/")' | sort | uniq -c`)

| Count | Line type                | ccx parser handles |
|-------|--------------------------|--------------------|
| 63    | `assistant`              | YES                |
| 41    | `user`                   | YES (includes tool_result carriers) |
| 12    | `attachment`             | **NO** — silently dropped |
|  6    | `queue-operation`        | **NO** — silently dropped |
|  5    | `file-history-snapshot`  | **NO** — silently dropped |
|  1    | `system/turn_duration`   | partial — treated as KindSystem |
|  1    | `system/stop_hook_summary` | partial          |
|  1    | `system/bridge_status`   | partial            |
|  1    | `system/away_summary`    | partial            |
|  1    | `permission-mode`        | **NO**             |
|  1    | `custom-title`           | **NO**             |
|  1    | `agent-name`             | **NO**             |

### Attachment subtypes (12 unique; all in `attachment.attachment.type`)

```
command_permissions
deferred_tools_delta
edited_text_file
hook_additional_context
hook_success              (e.g. SessionStart:startup)
mcp_instructions_delta
plan_mode
plan_mode_exit
queued_command (×3)
skill_listing
```

**None of these currently render in ccx.** They are silently filtered during
parse. The `plan_mode` / `plan_mode_exit` attachments are notable — they're
the actual state carriers for EnterPlanMode / ExitPlanMode, not just the
tool_use blocks.

### System subtypes observed

`bridge_status` (100-block content), `stop_hook_summary`, `turn_duration`,
`away_summary` (170-char content — the agent's "we're idle, run /compact"
note).

### Content blocks inside `message.content`

| Count | Block type    |
|-------|---------------|
| 39    | `tool_use`    |
| 39    | `tool_result` |
| 18    | `text`        |
| 7     | `thinking`    |

### Tool calls (16 unique tools, 39 invocations)

| Tool              | Count | ccx has specialised renderer? |
|-------------------|-------|-------------------------------|
| TaskUpdate        | 8     | NO — JSON fallback            |
| Write             | 4     | YES                           |
| ToolSearch        | 4     | NO                            |
| TaskCreate        | 4     | **NO** (legacy `Task` case exists, but name is `TaskCreate`) |
| Bash              | 4     | YES                           |
| Skill             | 2     | YES                           |
| Read              | 2     | NO                            |
| Edit              | 2     | YES                           |
| AskUserQuestion   | 2     | YES                           |
| WebFetch          | 1     | YES                           |
| **Monitor**       | 1     | **NO — new in 2.1.109, first ever captured** |
| Grep              | 1     | NO                            |
| Glob              | 1     | NO                            |
| ExitPlanMode      | 1     | NO                            |
| EnterPlanMode     | 1     | NO                            |
| Agent             | 1     | partial (legacy name)         |

**Coverage gap: 10/16 tools fall through to raw-JSON fallback.**

### The Monitor tool_use — first captured input shape

```json
{"type":"tool_use","id":"toolu_01HUYWnDjAS23ApRNHoUnKMJ","name":"Monitor",
 "input":{"description":"watching signal.log for READY",
          "command":"tail -F /tmp/ccx-fixture/cc/signal.log 2>/dev/null | grep --line-buffered READY",
          "persistent":false,
          "timeout_ms":60000},
 "caller":{"type":"direct"}}
```

`caller.type` is a new field I haven't seen in other tools — worth checking
whether it appears on other tool_use blocks in newer sessions (could be a
namespace hint for "direct vs sub-agent-proxy-invoked" tools).

### Sub-agent sidechain

The sub-agent transcript lives in a sibling subdirectory:
`de4b5d69-…/subagents/agent-a2b0aaeb88f277707.jsonl` — **49 lines** (26
assistant + 23 user/tool_result), every message carries `isSidechain:true`
and `agentId` pointing at the Task tool_use id. The companion
`.meta.json` carries `{"agentType":"Explore","description":"Verify high-frequency tokens"}`.

**Sub-agent tools exercised:** Grep×20, Read×2. That's a narrow surface
but it's enough to verify the sidechain-discovery path once ccx starts
loading these files.

**ccx's Claude Code backend does not currently discover sub-agent files**
because `parser/project.go:74` explicitly skips any `*.jsonl` under
`projects/<encoded>/` that starts with `agent-`, AND it doesn't recurse
into subdirectories. So the 49 messages are invisible to the parser
today.

### Slash commands & compact boundaries

The fixture contains `<command-name>` substrings **only inside the pasted
prompt's text body** (the prompt itself mentions `/compact` and
`<command-name>` as examples) — there is **no real slash command firing
and no compact_boundary system message** in this rollout. The `away_summary`
system line explicitly says "Next action: you run /compact yourself so
the rollout captures the compact_boundary, then confirm we're finished,"
which means the agent noticed the missing coverage and wanted the user to
trigger it. The user did not.

**Gap:** slash-command path and compact-boundary path are unexercised in
this fixture. A second-pass capture should rerun Prompt A and actually
hit step 9's `/compact`.

---

## Codex fixture coverage

### Top-level rollout items

| Count | Type            | ccx handles |
|-------|-----------------|-------------|
| 54    | `response_item` | YES         |
| 40    | `event_msg`     | YES (16 of 54 known variants) |
| 1     | `session_meta`  | YES         |
| 1     | `turn_context`  | YES         |

v2 rollout format confirmed by the presence of `turn_context`. No
`compacted` top-level item in this fixture (the prompt didn't trigger a
`/compact`).

### session_meta fields present

```
base_instructions, cli_version, cwd, git, id, model_provider,
originator, source, timestamp
```

**Missing from the v2 inventory:** `forked_from_id`, `agent_nickname`,
`agent_role`, `agent_path`, `dynamic_tools`, `memory_mode`. These would
only appear on a forked sub-agent session, which the default
`collaboration_mode=default` run doesn't produce.

### turn_context fields present

```
approval_policy, collaboration_mode, current_date, cwd, effort, model,
personality, realtime_active, sandbox_policy, summary, timezone,
truncation_policy, turn_id, user_instructions
```

Rich field set — the devlog catalogue had `cwd, approval_policy,
sandbox_policy, model, reasoning_effort, user_instructions,
developer_instructions, final_output_json_schema, truncation_policy,
timezone, collaboration_mode`. Extras observed: **`current_date`,
`personality`, `effort` (vs catalogue's `reasoning_effort`),
`realtime_active`, `summary`, `turn_id`**. Missing from catalogue's list:
`developer_instructions`, `final_output_json_schema`. Worth updating the
devlog.

### event_msg payload types (9 unique, 40 instances)

| Count | Payload type       | ccx handles |
|-------|--------------------|-------------|
| 13    | `token_count`      | YES         |
| 10    | `agent_message`    | YES         |
|  7    | `exec_command_end` | YES         |
|  3    | `web_search_end`   | YES (double-counts — see bug #1) |
|  3    | `patch_apply_end`  | YES         |
|  1    | `user_message`     | YES         |
|  1    | `task_started`     | YES         |
|  1    | `task_complete`    | YES         |
|  1    | `mcp_tool_call_end`| YES         |

**Coverage gap — 45 of 54 known event_msg variants are NOT exercised:**
all `*_begin` variants, all `*_delta` streaming events, `agent_reasoning`
(this fixture has reasoning only via `response_item/reasoning`),
`agent_reasoning_raw_content`, `agent_reasoning_section_break`,
`context_compacted`, `thread_rolled_back`, `thread_name_updated`,
`model_reroute`, `session_configured`, `skills_update_available`,
`plan_update`, `undo_*`, `turn_aborted`, `shutdown_complete`,
`stream_error`, `entered_review_mode`/`exited_review_mode`, all
`collab_*` events, `raw_response_item`, `item_started`/`item_completed`,
`hook_started`/`hook_completed`, `realtime_*`, `error`/`warning`,
`deprecation_notice`, `turn_diff`, `get_history_entry_response`,
`mcp_list_tools_response`, `list_skills_response`, `background_event`,
`guardian_assessment`, `dynamic_tool_call_request`, `view_image_tool_call`,
`elicitation_request`, `request_user_input`, `request_permissions`,
`exec_approval_request`, `apply_patch_approval_request`,
`exec_command_output_delta`.

### response_item payload types (7 unique, 54 instances)

| Count | Payload type              | ccx handles |
|-------|---------------------------|-------------|
| 13    | `message`                 | INTENTIONALLY DROPPED (avoids duplication with event_msg) |
| 12    | `reasoning`               | YES         |
| 10    | `function_call_output`    | YES         |
| 10    | `function_call`           | YES         |
|  3    | `web_search_call`         | YES (double-counts — see bug #1) |
|  3    | `custom_tool_call_output` | YES         |
|  3    | `custom_tool_call`        | YES         |

**Missing from catalogue:** `LocalShellCall` (deprecated, so OK),
`ImageGenerationCall`, `ToolSearchCall`, `ToolSearchOutput`, `Compaction`,
`GhostSnapshot`.

### function_call.name registry observed

```
exec_command                                   7×
update_plan                                    2×
mcp__codex_apps__github_search_repositories    1×
```

Plus 3 `custom_tool_call.name = apply_patch`.

### token_count shape

```json
{"total_token_usage": {...}, "last_token_usage": {...}, "model_context_window": 258400}
```

Note: the first `token_count` event in this session has
`info: null` (jq `null has no keys` error above) — ccx must treat
`info` as optional and skip when null. This is a real edge case that's
worth a unit test.

### task_started payload

```json
{"type":"task_started",
 "turn_id":"019d9053-5edb-7151-bd80-71f5b20ce8ce",
 "started_at":1776242876,
 "model_context_window":258400,
 "collaboration_mode_kind":"default"}
```

`started_at` is a **unix timestamp (seconds)**, not an ISO string.
ccx's parser needs to accept both shapes — worth a test.

---

## Is the fixture set "enough"?

**Enough to be valuable:** yes. Between the two sessions we exercise
16 Claude Code tools (including Monitor, which nobody has tested before),
every v2 Codex top-level rollout type, 9 event_msg variants, 7
response_item variants, both apply_patch + exec_command + web_search +
MCP tool shapes, and a complete sub-agent sidechain file structure.
That's dramatically more coverage than any synthetic fixture we could
hand-write.

**Enough for permanent reference:** no — there are specific gaps worth
filling in a second-pass capture when the opportunity arises:

### Known gaps to capture in a follow-up fixture

1. **Real `/compact` firing in a CC session** — both the slash-command
   `<command-name>compact</command-name>` XML and the resulting
   `system/compact_boundary` message. Rerun Prompt A and actually
   execute step 9.
2. **Codex `Compacted` rollout item** — same thing on the Codex side.
   Rerun Prompt B and trigger `/compact` at step 10.
3. **Codex streaming deltas** — Prompt B ran in a mode that doesn't
   preserve `AgentMessageDelta`/`AgentReasoningDelta`. Rerun with the
   delta-preserving flag (whatever that is) to capture them.
4. **Codex Collab* sub-agent events** — requires
   `collaboration_mode_kind != default`. Run with `codex --collab-mode`
   (flag name TBD).
5. **CC Notebook / PowerShell / CronCreate / EnterWorktree /
   SendMessage / Config / LSP / MCP / MonitorWithRealEventStream** —
   individually hard to trigger naturally. Build a second fixture that
   deliberately exercises each.
6. **Image content blocks** — neither fixture has an image in the
   transcript. A single screenshot attachment would cover it.
7. **Review mode** — `entered_review_mode`/`exited_review_mode` on
   Codex. Run one session that asks for a review of the previous.

These gaps are documented here, not treated as ship blockers. The current
fixtures are committed first; follow-up fixtures go in as
`testdata/test-session-<topic>/` alongside this one.

### What we explicitly do NOT need more of

- More Bash calls (have 4 CC + 7 Codex, plenty)
- More Edit/Write (have enough)
- More tokens (have 4.2M cache-read tokens on CC, 483k input on Codex)
- More agent_message/reasoning (10 + 12 on Codex is enough)

---

## Bugs surfaced by the real fixtures

These are documented here so the fixture tests lock in current behaviour
and future fixes produce visible diffs. Each bug has a tracking comment
in the test file at `internal/provider/fixture_test.go`.

### Bug #1 — Codex web_search_call deduplication fails

**Symptom:** Codex session with 3 `web_search_call` response_items + 3
`web_search_end` event_msgs produces **6 WebSearch tool_use blocks**
in the parsed tree instead of 3.

**Root cause:** The two events carry different ID fields.
`response_item/web_search_call.payload.id = "ws_0ae9f1..."` but
`event_msg/web_search_end.payload.call_id = null`. ccx's dedupe map
at `internal/provider/codex/backend.go` keys on `call_id`, which is
null for the response_item path, so the merge never happens.

**Fix:** In the `response_item/web_search_call` case, read `payload.id`
when `payload.call_id` is empty, and populate `handledCallIDs[payload.id]`
so the subsequent `event_msg/web_search_end` knows the call is already
represented.

### Bug #2 — Claude Code sub-agent sidechain not discovered

**Symptom:** CC session with TaskCreate/Agent tool_use calls parses
successfully, but `session.Stats.AgentSidechains = 0` and flat message
list contains 0 `isSidechain:true` messages, even though the sub-agent's
`agent-a2b0aaeb88f277707.jsonl` file exists under
`projects/<encoded>/<session-uuid>/subagents/`.

**Root cause:** `internal/parser/project.go:74` skips any file starting
with `agent-`, and the parser doesn't recurse into the
`<session-uuid>/subagents/` subdirectory at all.

**Fix:** When parsing `<uuid>.jsonl`, also check for a sibling directory
`<uuid>/subagents/` and load any `agent-*.jsonl` files inside it. Tag
every message with `isSidechain:true` + `agentId` from the `.meta.json`,
and graft them under the correct parent TaskCreate tool_use by matching
task IDs.

### Bug #3 — Sub-agent dispatches misclassified as generic tool calls

**Symptom:** `parser.ComputeExchanges` walks the flat message list and
classifies tool_use blocks into `StepSubagent` / `StepSkill` /
`StepToolUse`. In the real fixture, 4 × `TaskCreate` and 1 × `Agent`
tool_use blocks should count as `StepSubagent = 5`, but the actual
count is **0** — they fall through to `StepToolUse`.

**Root cause:** `parser/turns.go`'s `extractSteps` only recognises
`ToolName == "Task"` as a sub-agent. The Claude Code 2.1.88 →
2.1.104 tool-name migration introduced `TaskCreate` (plus the
still-valid legacy `Agent`), and the classifier was never updated
to match.

**User-visible impact:** timeline rail sub-agent satellites count 0
instead of 5 on modern CC sessions; tooltip badge row shows
`⎇ 0 subagents` when sub-agents were genuinely dispatched; outline
hierarchy doesn't distinguish sub-agent dispatch from a regular
tool call.

**Fix:** Add `"TaskCreate"` and `"Agent"` to the switch in
`extractSteps`. Optionally centralize the sub-agent name set as a
package constant so future renamings touch one place.

**Locked by:** `TestRealFixture_ClaudeCode_SubagentStepsMisclassified`
in `internal/provider/fixture_real_test.go`.

### Observation #4 — `CurrentModel` vs `effort` vs `personality`

The Codex `turn_context` exposes `effort` (not `reasoning_effort`) and
a new `personality` field that wasn't in the devlog catalogue. Update
the devlog. Also `current_date`, `realtime_active`, `summary`, `turn_id`
are new finds. None are currently read by ccx.

### Observation #5 — token_count `info` field can be null

First `token_count` event on Codex startup has `payload.info = null`.
ccx's current code does `if payload.Info != nil` so it's safe, but
this is the kind of edge case a fixture test should lock in.

---

## How to regenerate this fixture set

```bash
# Claude Code side
mkdir -p /tmp/ccx-fixture/cc && cd /tmp/ccx-fixture/cc
claude                         # interactive
# paste Prompt A from the devlog's §Test Prompts
# answer EnterPlanMode checkpoint with "go"
# confirm the Monitor step
# confirm ship/hold on the AskUserQuestion
# manually run /compact at step 9     <-- do not skip this next time

# locate the fresh rollout
ls ~/.claude/projects/-tmp-ccx-fixture-cc/*.jsonl

# Codex side
mkdir -p /tmp/ccx-fixture/cx && cd /tmp/ccx-fixture/cx
codex                          # interactive
# paste Prompt B from the devlog's §Test Prompts
# the prompt already includes an explicit /compact at step 10

# locate the fresh rollout
find ~/.codex/sessions -name "rollout-*.jsonl" -newer /tmp/ccx-fixture/cx
```

Copy the resulting files (and any `<uuid>/subagents/` sidechain
subdirectory from the CC side) into this directory, bump the filename
if replacing an older capture, and update this README's **Provenance**
table with the new CLI version and timestamp.
