# Agent chat as Crowbar's primary surface

**Date:** 2026-08-17

**Status:** design; supersedes the POC framing of
[`2026-08-15-react-agent-chat-wrapper-demo-design.md`](./2026-08-15-react-agent-chat-wrapper-demo-design.md)

**Providers in the initial mapping:** Claude Code 2.1.233, Codex 0.146.0

The 2026-08-15 document specified a React *presentation* of a chat whose real home was the
terminal. This document changes that decision: **Chat becomes the primary way people work with
agents in Crowbar**, and the terminal becomes a peer surface for the things a provider only
exposes natively.

That is a product decision with architectural consequences, because a surface people live in
must explain what the agent is doing. The POC could not: it saw three hooks and rendered two
kinds of message. Everything else — tool activity, subagents, permission waits, rate limits,
context pressure — was invisible, and the only honest response was "open Terminal."

Every provider fact below was measured against the installed CLIs on 2026-08-16/17. §12 records
what was measured, what was inferred from binaries, and what remains untested.

---

## 1. What changes, and what does not

Unchanged from the 2026-08-15 contract, and still non-negotiable:

1. **Interactive provider only.** No `-p`/`--print`, no `codex exec`, no SDK, no app-server, no
   remote-control. Every hosted provider process is the ordinary interactive CLI in a real PTY.
2. **No keystroke injection.** Crowbar never writes into a TUI whose state it cannot see.
3. **No provider-owned file reads.** Not config, not skills, not plugins, not transcripts, not
   session directories.
4. **Provider-neutral layers.** Domain, usecases, REST DTOs and React never branch on `claude`
   or `codex`. Provider facts live in Crowbar-owned descriptors, read through generic adapters.
5. **Hooks are authoritative for conversation content.** A successful API call never manufactures
   a message.
6. **No fake streaming, no estimation.** Crowbar renders what a provider reported. Where a
   provider reports nothing, Crowbar shows nothing rather than a guess.
7. **Terminal is permanent.** It is a peer, not a deprecated path.

Changed:

- **Chat is the default presentation**, and a setting controls that (§11).
- **Tool inputs and results are in scope.** The 2026-08-15 non-goal excluding them was a POC-era
  constraint and is withdrawn. Full fidelity, mapped into Crowbar's domain (§4).
- **A chat does not own a process until it needs one** (§3).
- **Delivery is a declared strategy**, not a single mechanism (§7).

## 2. Why the observation surface is the centre of this work

The POC wired three hooks: `session_start`, `user_prompt`, `turn_stop`. The installed CLIs expose
far more — **Claude 2.1.233 has 31 hook events**, Codex 0.146.0 has 11.

That gap is the entire difference between a transcript viewer and a surface worth living in. The
three failures observed during live testing on 2026-08-16 were all legibility failures, not
delivery failures:

| Observed | What Chat showed | What was available |
| --- | --- | --- |
| Provider blocked on a trust prompt | nothing, then a generic hint | `Notification`, `PermissionRequest` |
| Codex hit its account usage limit mid-turn | an indefinite spinner | Claude: `rate_limits` via telemetry (§8.1) |
| Agent running tools for minutes | "working…" | `PreToolUse`, `PostToolUse` with `duration_ms` |

So the ordering in §13 puts observation before delivery optimisation. Making a working thing
faster is worth less than making an opaque thing legible.

## 3. Session lifecycle: a chat does not own a process

Today a chat spawns a CLI at creation. N chats means N idle provider processes, N trust prompts
and N native sessions nobody asked for.

**A chat is a Crowbar object. A runner is a resource it acquires when there is work.**

A provider process is started only when one of these happens:

1. the user submits the first prompt from Chat;
2. the user opens the Terminal presentation for that chat;
3. the user explicitly resumes a dormant chat.

Consequences that make this cheaper than it looks:

- **The first message is always a fresh spawn carrying the positional prompt.** That is already
  the `restart_tui` code path, so message #1 needs no new delivery mechanism on any provider,
  and the fresh-spawn path stays exercised everywhere.
- **Model and effort become free choices.** They are argv. Choosing them before the first message
  costs nothing, where today changing them requires replacing a running CLI.
- Trust prompts, MCP approvals and native session creation are deferred to the moment they are
  actually warranted.

The work is that "no runner" must become a *normal initial state*. Today a runnerless chat reads
as dormancy-after-death and the pane auto-revives it; the FIFO's dispatch guard is literally "a
live TUI exists". Both need to distinguish **never started** from **died**.

A dormant chat with queued text follows the same rule: acquire the runner when the prompt is
dispatched, not while the user is still typing.

## 4. Domain model

Crowbar records its own concepts, mapped from provider payloads by the descriptor. Nothing
provider-shaped reaches the domain.

```text
Turn
  id, chatId, role, providerId, runnerId, startedAt, endedAt
  text                         (hook-confirmed; the conversation record)
  effort                       (when reported)

ToolCall
  id, turnId, name, target
  requestRef, resultRef        (content refs, never inline payloads)
  status, durationMs, startedAt, endedAt

Subagent
  id, turnId, agentType, startedAt, endedAt

Interruption
  id, turnId, kind             (permission | notification | elicitation | compaction)
  detail, at, resolvedAt
```

### 4.1 Payloads live beside the event log, not inside it

> **Corrected 2026-08-17.** The mechanism described below is wrong in one respect: asynx
> stores RFC-6902 **patches**, not full aggregate state, so an event is O(delta) and the
> events table does not grow with total state. The conclusion stands — snapshot writes and
> cold loads are both O(state) — but see
> [`2026-08-17-agents-engine-implementation.md`](./2026-08-17-agents-engine-implementation.md) §1
> for the measured mechanics and their consequences.

Tool inputs and results are stored in **full**. They are not stored *in the aggregate*.

This is a storage-layout decision forced by how asynx behaves, not a fidelity compromise. From
its own spec: `events` is `(aggregate_id, version, data BLOB)` and **every version is retained**;
a snapshot is **the entire aggregate state upserted as one row**; and the cold path *"must replay
all events from the beginning of the stream before returning state"*.

So an aggregate whose state contains tool payloads has three mechanical problems: every snapshot
rewrites every payload in that chat as one BLOB, every cold load materialises all of them in
memory, and both scale with total tool output rather than conversation length. A coding agent
produces hundreds of KB per turn; the longest and most valuable chats pay the largest bill.

Therefore:

| Layer | Holds | Properties |
| --- | --- | --- |
| **Event log** (asynx) | domain facts: `TurnStarted`, `ToolInvoked{name, target, requestRef}`, `ToolCompleted{status, durationMs, resultRef}`, `SubagentStarted/Stopped`, `Interrupted`, `TurnEnded` | small, bounded per event, cheap to snapshot and replay |
| **Content store** | full tool payloads, addressed by content hash | immutable, never replayed, deduplicated, retention is a policy |
| **Projection** (`view.db`) | queryable read model: tools by name, files touched, agent, chat, time | what the UI and the MCP tools read |

Content addressing is not incidental: agents re-read the same files constantly, so the same
200 KB file read forty times stores once.

## 5. Storage: retiring the flat-file ledger

The 2026-08-15 ledger is append-only text files per chat. It cannot represent a tool call, a
subagent or an interruption, so it is a blocker for §2 rather than a component to extend.

It is replaced by the three layers in §4.1. Two properties must survive the move:

1. **Durability.** The current design fsyncs the entry, atomically renames, and fsyncs the parent
   directory, with a bounded hook-delivery spool in front. An acknowledged hook must still
   survive an OS crash.
2. **Handoff assembly.** `assembleConversation` reads the ledger to build provider handoffs on
   switch. That must keep working, now sourced from the projection.

Crowbar is pre-production, so there is no migration path: the old ledger is dropped, not
converted.

## 6. Observation: which hooks, and what they carry

All fields below were captured from live payloads on 2026-08-17 unless marked.

| Crowbar concept | Claude | Codex |
| --- | --- | --- |
| conversation text | `UserPromptSubmit{prompt}`, `Stop{last_assistant_message}` | same |
| session identity + model | `SessionStart{source, model}` | `SessionStart{model, source, permission_mode}` |
| tool activity | `PreToolUse{tool_name, tool_input, tool_use_id}`, `PostToolUse{+tool_response, duration_ms}` | same events; payloads inferred from binary, not captured (§12) |
| subagents | `SubagentStart/Stop{agent_id, agent_type}` | `SubagentStart/Stop` |
| blocked on a human | `Notification{message, notification_type}`, `PermissionRequest`, `PermissionDenied` | `PermissionRequest`; **no `Notification`** |
| context pressure | `PreCompact{trigger}`, `PostCompact{trigger, compact_summary}` | same |
| session end | `SessionEnd{reason}` | `SessionEnd{reason}` |

Claude additionally exposes `PostToolUseFailure`, `TaskCreated/Completed`, `Elicitation`,
`FileChanged`, `CwdChanged`, `MessageDisplay`, `InstructionsLoaded`, `WorktreeCreate/Remove` and
others — 31 in total. Wiring more of them is descriptor work, not engine work.

**No hook on either provider carries token, context, usage or cost data.** This was established
by capturing every reachable event and by extracting the complete hook-input schema union from
the Claude binary. Context telemetry therefore needs a different channel (§8.1).

## 7. Delivery

Prompt delivery becomes a **declared strategy** selected by the descriptor. The usecase owns
validation, the durable request journal, the idle gate and outcome classification; the strategy
owns everything transport-specific.

```yaml
presentation:
  prompt_submit:
    strategy: restart_tui | rewake_hook
```

Rules:

- **Message #1 always spawns** with the positional prompt (§3), on every provider.
- From message #2, the descriptor's declared strategy applies.
- **`restart_tui` is the portable floor.** It must always work; `rewake_hook` is an optimisation
  that can be disabled by descriptor change, without a code change, if a provider surface moves.
- **A replacement is only ever triggered by a prompt submitted from Chat.** Terminal typing goes
  straight to the PTY and never causes one.

### 7.1 `restart_tui`

Gracefully replaces the CLI; the prompt is the final argv element after an end-of-options `--`.
Owns displacement, the turn-start interlock, fresh-vs-resume selection and the startup hook
barrier. Destructive, so an ambiguous outcome must block the queue rather than risk a duplicate.

### 7.2 `rewake_hook` (Claude)

Verified working on 2026-08-16. A hook registered with `asyncRewake: true` runs in the background
and wakes the model when it exits 2, with its stdout delivered to the session. Crowbar registers
`crowbar hook await-prompt`, which blocks until the daemon has a queued prompt for that runner's
current chat, prints it, and exits 2.

This is a **pull** model and that is the point: the daemon holds the queue and the provider comes
to collect, so an ambiguous HTTP outcome is resolved by asking rather than guessing. It removes
the startup barrier, displacement, resume selection and the destructive-retry class entirely.

Measured properties:

- `UserPromptSubmit` **does** fire, so the ledger stays hook-authoritative and any organisation
  hooks registered on that event run normally.
- The payload is wrapped, and both wrapper parts are Crowbar-controlled:
  `<task-notification><summary>{rewakeSummary}</summary></task-notification>` plus
  `<system-reminder>{rewakeMessage} {text}</system-reminder>`. The descriptor strips it, and the
  `rewakeMessage` sentinel doubles as the discriminator for "Crowbar delivered this" — necessary
  because, unlike restart, the same runner emits both native and injected prompts.
- The resulting `Stop` carries `stop_hook_active: true`.

Constraints:

- Requires an armed hook, which requires a completed turn — consistent with §3.
- `await-prompt` **reads** the user's prompt, unlike the write-only hooks, so it needs a
  credential rather than a bare segment id; argv is world-readable.
- It parks prompt plaintext in the daemon until collection. The 2026-08-15 journal deliberately
  stored only a SHA-256. This is a deliberate change to that property.
- `asyncRewake` is marked `@internal` in Claude's schema. Hence the fallback requirement above.

Codex has no equivalent: its hooks only run while it is already doing something, so an idle
session cannot be woken. Codex stays on `restart_tui`.

## 8. Provider capability surfaces

Every capability below is **optional per provider**, declared in the descriptor, and absent
capabilities render as absent UI. This is the existing `presentation` pattern generalised.

### 8.1 Telemetry — context, cost, rate limits, model identity

Model the **facts, not the transport**. Claude bundles four unrelated concepts into one payload;
another provider will split them differently.

```text
ContextUsage    capacityTokens, usedTokens, usedPercent, remainingPercent
RateLimits      windows[]: id, usedPercent, resetsAt
SessionCost     totalUsd, apiDurationMs
ModelIdentity   id, displayName
```

Each is independently optional, each carries `observedAt` and `source`, and **nothing is derived
that was not reported** — a percentage is computed only when capacity and usage are both known.

Two ingress transports, both descriptor-declared:

- **`callback`** — Crowbar registers a command via `config_injection`; the provider invokes it
  with JSON on stdin. Same channel and scoping as hooks: `--segment {segid}`, runner-scoped,
  resolved to the chat at ingestion, write-only.
- **`probe`** — Crowbar runs a deterministic subcommand on a cadence and maps the output. Reuses
  the slash-catalog engine wholesale: bounded output, timeout, process-group kill, shared budget.

Field mapping is declarative, in the same shape as the hook `map:` blocks, so a Claude
`context_window.used_percentage` and a future Codex `session.context.pct_used` both land on
`ContextUsage.usedPercent` with no code between them.

The descriptor section is named `telemetry`. It is **not** named after Claude's feature; the
moment the key is called `status_line`, the abstraction has failed.

**Claude today** fills all four via `statusLine`, measured 2026-08-17:

```json
{"context_window": {"context_window_size": 200000, "used_percentage": 19,
                    "remaining_percentage": 81, "total_input_tokens": 37117},
 "cost": {"total_cost_usd": 0.0649},
 "rate_limits": {"five_hour": {"used_percentage": 1, "resets_at": …},
                 "seven_day": {"used_percentage": 47, "resets_at": …}},
 "model": {"id": "claude-haiku-4-5-20251001", "display_name": "Haiku 4.5"}}
```

Measured behaviour: it fires with **no client attached to the PTY** (so it works under §3 with
Terminal closed); Crowbar can print nothing and leave the user's status line untouched; and it is
**change-driven, not per-frame** — 3 invocations across a 37s session with one turn, 2 across 60s
of idle. Usage is `null` until the first turn completes, so a fresh session has no gauge.

**Codex today** fills only `ContextUsage.capacityTokens`, from `debug models`
(`context_window`, `max_context_window`). Its live token data exists only on the app-server
stream, which §1 excludes. When Codex ships a hook or a deterministic subcommand carrying it,
this becomes a descriptor entry.

Telemetry is **current state, not history**. It does not enter the event log; thousands of
"19% used" events exist only to be superseded. Only notable transitions become domain events: a
compaction occurred, a rate-limit window crossed a threshold, capacity changed with the model.

### 8.2 Model catalogue

```yaml
model_catalog:
  completeness: live | none
```

- **Codex** — `debug models`, live and account-derived: `slug`, `display_name`, `description`,
  `visibility` (`hide` rows filtered), `priority` (ordering), `context_window`. `base_instructions`
  is ~114 KB of system prompt and is stripped in the mapper.
- **Claude** — no enumeration exists. Verified against all 54 flags, the complete command
  registry including hidden commands, `auth status --json`, `doctor`, and the hook surface. The
  catalogue is fetched from `/v1/models` with the client's own credentials. A shipping competitor
  with full access to Claude's programmatic surface hardcodes its list for this reason.

Where enumeration is absent, Crowbar offers free-text entry plus whatever it has observed, and
displays the resolved model from `SessionStart.model` / `ModelIdentity`. **No model names are
declared in any descriptor.**

Prohibited models fail gracefully, measured 2026-08-16:

| Path | Behaviour | Detectable |
| --- | --- | --- |
| org allowlist (`availableModels` + `enforceAvailableModels`) | substitutes and runs; warns in the TUI | **yes, before any tokens** — `SessionStart.model` reports the substitute |
| account entitlement | starts, then the first turn returns explanatory prose | only after a turn, and only from prose |

Neither is destructive, so offering a superset costs a substitution or one wasted turn — not a
broken session.

### 8.3 Effort

```yaml
effort_catalog:
  completeness: live | flag_enum
```

- **Codex** — per model from `debug models`: `supported_reasoning_levels` with descriptions and
  `default_reasoning_level`. Genuinely per-model (`gpt-5.6-sol` supports `ultra`, `gpt-5.6-luna`
  stops at `max`).
- **Claude** — a flat enum obtained by a sentinel probe: an unknown `--effort` value causes
  `Valid values: low, medium, high, xhigh, max.` on exit 1, with no session and no tokens.
  Claude's real per-model `supportedEffortLevels` and org `maxEffortLevel` exist internally and
  are not exposed, so an invalid combination surfaces at spawn rather than at selection.

Effort actually used is observable on both: Claude's `Stop` carries `effort: {level}`.

### 8.4 Slash catalogue

Unchanged from 2026-08-15, including the `completeness` labelling and the terminal fallback.

## 9. MCP: cross-agent awareness

Crowbar already hosts an MCP server with eight tools. With §4's projection, agents can be given
read access to what other agents in the workspace are doing — which chats are active, what a
sibling changed, which files are being touched.

This is a genuine differentiator and it is only possible because Crowbar now holds structured,
queryable activity rather than flat transcripts. Scoping, redaction and per-tool limits follow
the existing `agenttools` patterns.

## 10. Frontend

- Chat is the default presentation; the terminal attaches **only** when Terminal is selected
  (already true) and, under §3, selecting it may also start the provider.
- Chat renders: hook-confirmed messages, tool activity with durations, subagent fan-out,
  interruption states with the reason, the telemetry gauges where reported, and the durable FIFO.
- **Absent capability renders as absent UI**, never as a disabled control implying breakage.
- Every capability that can be stale shows its freshness rather than a confident stale number.

## 11. Settings

The settings section named **Provider** becomes **Agents**. The rename is user-facing only —
`usecases/agent`, `agentrunner` and `agentchat` already overload the word internally, and Claude
has its own `--agent`/subagent concepts, so the term must not propagate into domain names.

A toggle controls whether Chat is the default presentation. Default on; users who prefer the
terminal keep it as their landing surface without losing Chat.

## 12. Evidence record

Measured live against installed CLIs:

| Fact | Date | Method |
| --- | --- | --- |
| `asyncRewake` delivers to a live session; `UserPromptSubmit` fires; payload wrapper is Crowbar-controlled | 2026-08-16 | PTY harness, reproduced 3× |
| `statusLine` fires unattended, can print nothing, is change-driven (~1/turn) | 2026-08-17 | PTY harness, 37s turn session + 60s idle |
| No hook on either provider carries token/context/cost | 2026-08-17 | 35 captured payloads + Claude binary schema union |
| Claude has 31 hook events; Codex 11 | 2026-08-17 | binary schema extraction |
| Org-allowlist model → substitution, visible in `SessionStart.model` | 2026-08-16 | PTY harness with `enforceAvailableModels` |
| Entitlement-restricted model → failure as assistant prose at first turn | 2026-08-16 | PTY harness |
| `--effort` sentinel enumerates valid values | 2026-08-16 | direct invocation |
| Codex `debug models` shape, `visibility`, `priority`, 273 KB payload | 2026-08-16 | direct invocation |
| Claude has no model enumeration surface | 2026-08-17 | 54 flags, full command registry, `auth status --json`, hooks |

**Inferred from binaries, not captured live:** Codex `PreToolUse`/`PostToolUse`/`Stop`/
`SubagentStart`/`SubagentStop`/`PreCompact`/`PostCompact` payload fields. The account hit its
usage limit during probing and no Codex model call could complete. Re-run after 2026-08-22.

**Untested:** repeat `rewake` cycles beyond one; a rewake arriving while an unsent draft sits in
the TUI composer; ESC/interrupt during a rewake turn (Claude documents that interrupt fires no
hook at all); multi-window behaviour under load.

## 13. Build order

1. **Delivery seam** — extract `deliveryStrategy` with `restart_tui` as the only implementation.
   No behaviour change. Prerequisite for §7, and it prevents a forked `SubmitPrompt`.
2. **Lazy start** (§3) — self-contained, immediate resource win, unblocks pre-spawn model/effort.
3. **Storage** (§4, §5) — the aggregate, content store and projection. Prerequisite for §6.
4. **Observation** (§6) — wire the hooks that matter, starting with the interruption events that
   caused the observed legibility failures.
5. **Telemetry** (§8.1) — the `telemetry` capability with both transports; Claude fills it now,
   Codex when it can.
6. **MCP cross-agent surface** (§9).
7. **`rewake_hook`** (§7.2) — an optimisation on a proven seam, never the only path.
8. **Catalogues, settings, polish** (§8.2, §8.3, §11) — slot in anywhere.

Items 1–2 can proceed in parallel with 3. Item 4 cannot start before 3, because the current
ledger has no representation for anything but text.

## 14. Rejected alternatives

| Alternative | Reason |
| --- | --- |
| headless/SDK/app-server for richer data | violates §1; costs the real TUI and couples Crowbar to faster-moving API surfaces |
| keystroke injection for delivery | four native modals appeared in one live session on 2026-08-16; typed text would have selected menu items |
| reading provider transcripts for token usage | violates §1.3; the data is reachable through `telemetry` without it |
| estimating context from Crowbar's own record | tool payloads dominate context and a Crowbar-side estimate would understate badly; a wrong gauge is worse than none |
| declaring model names in descriptors | offers models the user may not have; goes stale on every provider release |
| tool payloads inside the event log | snapshot size and cold-load time would scale with total tool output (§4.1) |
| naming the telemetry capability after `statusLine` | bakes one provider's mechanism into a provider-neutral contract |
| making `rewake_hook` the only Claude path | it is an `@internal` surface; `restart_tui` must remain a config-level fallback |
