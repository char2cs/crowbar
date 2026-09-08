# Codex on the native chat UI — protocol gap report

Investigation date: 2026-09-08.
Branch: `worktree-agent-a7e53caf132134af8`.

## How this was investigated

Two independent sources of truth were used, so that no claim here rests on
reading Crowbar's own YAML and believing its field names:

1. **Codex's own generated protocol schema**, fetched from `openai/codex`:
   - `codex-rs/app-server-protocol/schema/json/ServerNotification.json` — the
     machine-generated JSON Schema for every notification the app-server emits,
     including the full `ThreadItem` sum type.
   - `codex-rs/app-server-protocol/src/protocol/v2/item.rs` and `turn.rs` — the
     Rust source those are generated from.
   - `sdk/python/src/openai_codex/generated/notification_registry.py` — the
     authoritative list of notification **method names**.
2. **The ACP reference** (`~/Projects/Cloned/athas/crates/ai/src/acp/`), used as
   a model of what a *complete* agent integration represents — thought chunks,
   tool-call status/kind/locations, plan updates, stop reasons, N-option
   permissions. ACP is a different transport from the app-server API Crowbar
   drives, so it was used to ask "what should be representable", not "what does
   codex send".

Every claim marked **CONFIRMED** below was reproduced by running Crowbar's own
Go code against a payload built from codex's generated schema.

## Headline

Codex's app-server emits **65** distinct notification methods.
`codex.yaml` consumes **6** of them.

That alone would be survivable. The reason the chat reads as "a mess that is
really unusable" is that three of the six it *does* consume are mapped to field
paths that **do not exist in the payload**, so they resolve to nothing and the
UI renders a blank where the substance should be.

---

## A. Confirmed bugs — mapped, but to paths that do not exist

### A1. Every codex tool call renders with no target, no output, no duration, no status

**CONFIRMED** by running `agent.ParseHook(HookToolPost, …)` against a
schema-shaped `commandExecution` and `fileChange` payload:

```
commandExecution: name="commandExecution" target="" result="" durationMS=0 status=""
fileChange:       name="fileChange"       target="" result="" durationMS=0 status=""
```

`codex.yaml` maps:

```yaml
tool_name:   "item.type || tool_name"
tool_result: "item.content || tool_response"
```

- `item.content` **does not exist on any codex tool item.** The real output
  fields are `aggregatedOutput` (commandExecution), `result` (mcpToolCall) and
  `changes` (fileChange).
- `tool_target` is **not mapped at all**, though the vocabulary offers it and
  `claude.yaml` maps it.
- `duration_ms` is **not mapped**, though every codex tool item carries
  `durationMs`.

The frontend renders a tool call as `describeTool`
(`web/src/features/agent/lib/agent-activity.ts:138`):

```ts
if (!call.target) return call.name
return `${call.name} · ${call.target}`
```

So a codex turn that ran five commands and edited two files renders as the
literal word list:

```
commandExecution
commandExecution
commandExecution
fileChange
fileChange
```

Claude, by contrast, renders `Bash · rg --files -g '*.go'`. **This is the single
biggest contributor to the "unusable" feeling** — the transcript tells you
nothing about what the agent actually did.

### A2. Codex telemetry is dead — the context gauge never renders

**CONFIRMED** by running `agent.ParseTelemetry` for codex:

```
codex err=agents: provider declares no telemetry
```

`codex.yaml` declares an `events.telemetry` block mapping
`thread/tokenUsage/updated`, and it is correct. But telemetry is not read
through the event map — `RecvTelemetry` → `telemetry.ParseCallback` reads
`d.Telemetry.Callback`, the **v2 top-level `telemetry:` block**, which codex.yaml
does not have (claude.yaml does, at line 313). The v3 loader never populates it
from `events.telemetry`.

Result: `ParseCallback` returns `ErrUnsupported`, `handleTelemetry` swallows it
at Debug level, `t.telemetry.Set` is never called, and
`AgentContextGauge` renders `null` because `usedPercent` is undefined
(`web/src/features/agent/controls/context-gauge.tsx:16`).

**A codex chat has never shown context-window usage.** The mapping has been
sitting there, validated at load, being ignored.

### A3. A failed codex turn is reported as a successful one, and the error text is discarded

`codex.yaml` maps `turn_stop` to `turn/completed` with **no `when:` filter**.

But `turn/completed` carries `turn.status` ∈
`{completed, interrupted, failed, inProgress}` and a `turn.error`
(`{message, additionalDetails, codexErrorInfo, misalignment}`).

So when a codex turn *fails* — model error, rate limit, sandbox refusal —
Crowbar records an ordinary successful stop whose `message` resolves to
`turn.items[type=agentMessage].text`, which on a failed turn is usually absent.
The user sees the spinner stop and **an empty assistant reply**, with
`turn.error.message` thrown away entirely.

The vocabulary already has `turn_failed` (`reason` required, `message`/`detail`
optional) and the Go side already renders it as a `TurnRoleNotice` row — claude
uses it via `StopFailure`. Codex simply never declared it.

There is even a test asserting this is correct:

```go
// codex still declares no failure hook at all — turn_failed has no wire event
// on either transport.
func TestAgent_CodexDeclaresNoFailureHook(t *testing.T)
```

That comment is **factually wrong** against codex's schema. `turn/completed`
*is* the failure signal; it is a sum type on `turn.status`.

### A4. Failed and declined tool calls are never closed

`tool_post` fires on `item/completed` regardless of `item.status`, which is one
of `inProgress | completed | failed | declined`. A failed or user-declined tool
is recorded as `ToolStatusOK`. `tool_fail` — which exists in the vocabulary and
which claude declares — is not declared by codex, so `tool_error`
(`item.error.message` on an mcpToolCall) is never captured.

### A5. Web searches are invisible

`tool_pre`/`tool_post` gate on
`when: { item.type: commandExecution || fileChange || mcpToolCall }`.

Codex's `ThreadItem` union also contains `webSearch` (with a `query`), plus
`dynamicToolCall`, `functionCallOutput`, `imageView`, `imageGeneration` and
`sleep`. A codex turn that searches the web shows **no activity at all** for
that step — dead air under a spinner.

### A6. Why the fixture-replay test never caught A1

`descriptor/fixture_test.go` is a good harness — it replays recorded provider
traffic against every mapped field path and fails on any that resolves to
nothing. It is explicitly credited with catching four wrong paths.

It did not catch A1 because the only recorded `item/started` fixture is a
**`userMessage`** variant. `mapping.Match` therefore fails for `tool_pre`'s
`when:` clause, the test hits `continue` (fixture_test.go:66), and
**`tool_pre`/`tool_post` are never checked against anything.**

The harness silently covers only whichever sum-type variant happens to have been
captured. That is the systemic defect behind A1, and it will hide the next such
bug too.

---

## B. Capability gaps — things codex reports that Crowbar has no vocabulary for

These are not mis-mappings; Crowbar's canonical vocabulary
(`descriptor/internal/schema/vocabulary.yaml`, deliberately **closed**, 20
events) has no name for them, so no descriptor could express them.

| Codex notification | ACP equivalent | Crowbar | Consequence in the UI |
|---|---|---|---|
| `item/reasoning/summaryTextDelta`, `item/reasoning/textDelta`, and the `reasoning` item | `ThoughtChunk` | **no event** | The largest one. Codex is a reasoning model; on a hard prompt it reasons for tens of seconds emitting nothing else. Crowbar shows a spinner and a rotating flavour verb. Claude Code and every ACP client stream the thinking. This is most of the perceived "dead air". |
| `turn/plan/updated`, `item/plan/delta`, `plan` item | `PlanUpdate` | **no event** | No plan/todo panel. Codex's own plan tool output is invisible. |
| `item/commandExecution/outputDelta` | tool `ToolUpdate` | **no event** | A 90-second build streams its output to nothing; the tool row is static until it completes. |
| `item/mcpToolCall/progress` | `ToolUpdate` | **no event** | Same for MCP tools. |
| `turn/diff/updated` | — | **no event** | No cumulative turn diff, though `FileUpdateChange` even carries a per-file `diff` string. |
| `thread/name/updated` | `SessionInfoUpdate` | **no event** | Codex names the thread; Crowbar keeps its own first-prompt-derived title. |
| `thread/compacted` | — | routed via **hooks** | See C1. |
| `subAgentActivity` item, `collabAgentToolCall` | — | routed via **hooks** | See C1. |
| `account/rateLimits/updated` | — | not mapped | `telemetry.extras: [rate_limits]` exists in the vocabulary and the UI has a rate-limit tooltip; codex never feeds it. A fixture for this event was even captured and left unwired. |
| tool `kind` (read/edit/execute/search/fetch) and `locations` (path + line) | `AcpToolKind`, `AcpToolCallLocation` | **no field** | No per-tool iconography or click-to-open. `AgentToolCall.target` is one opaque string. |
| stop reason on a *successful* turn (`max_tokens`, `refusal`, …) | `StopReason` | **no field** | A turn truncated on token limit is indistinguishable from one that finished. |

### B1. Five events ride a disconnected PTY

`codex.yaml` routes `subagent_pre`, `subagent_post`, `compact_pre`,
`compact_post` and `session_end` over `transport: hooks`. Its own comment admits
what this means:

> The api transport carries none of these five. `transport: hooks` means SOME
> codex process still fires them, but with no attach it is a separate,
> disconnected PTY spawnRunner still forks — driving its own unrelated
> conversation. Known gap, tracked as follow-up work.

Codex's app-server *does* carry equivalents natively: `thread/compacted` and the
`contextCompaction` / `subAgentActivity` items. So these five could move onto
the real connection.

---

## C. Frontend defects (provider-agnostic, but codex hits them hardest)

### C1. An unresolved interruption blanks the working line and renders nothing in its place

`web/src/features/agent/activity/working-line.tsx:89`:

```ts
const blocked = pendingChoices(activity).length > 0 || (interruption !== null && !compacting)
if ((!working && !compacting) || blocked) return null
```

Meanwhile `toDividerTag` (`chat/agent-chat-view.tsx:156-171`) maps only five of
the eight interruption kinds to a visible pill. The three it returns `null` for
are exactly **`permission`, `notification`, `elicitation`**.

So an unresolved permission or elicitation interruption **kills the spinner and
draws nothing at all** — the chat looks frozen and dead, with no explanation on
screen. There is even a `describeInterruption` helper in
`lib/agent-activity.ts:145-160` with the right human copy for precisely those
three kinds, and it has **zero call sites**.

Codex hits this far harder than claude because of C2.

### C2. Codex permission choices get a fresh random id every time

`promptCorrelationKey` (`turn/observation.go:168`) returns `""` when
`PromptID` is empty — and codex's permission payload carries no prompt id
(`vocabulary.yaml:121` documents this). So `choiceID` falls to
`"choice-" + fallbackID()`, a fresh timestamp-derived id per event.

The existing code is careful to mint it **once** and thread it to both the
choice and the interruption (there is a regression test for that). But it means
a codex permission can never be de-duplicated or re-correlated across a redelivery.

### C3. Only the first pending choice is ever shown

`resolveComposerState` (`composer/lib/composer-state.ts:130`) takes
`pendingChoices(activity)[0]`. `pendingChoices` deliberately returns all of them.
Choices 2..N are invisible **and** each of them keeps the working line blanked
via C1.

### C4. Other silent drops (lower severity)

- `AgentChatMessage.effort` is fetched and never rendered.
- `hasRequest`/`hasResult` and `getToolPayload` exist on the API client and are
  never called — the tool request/result payloads are captured by the backend and
  unreachable from the UI.
- Ended subagents are never shown; `subagent-shelf.tsx:33` filters to running only.
- Resolved choices are never rendered — no record of what was approved or denied.
- Finished tool calls are capped at 6 per turn, running ones at 3.
- `AgentToolCall.status` is written to a `data-status` attribute for which
  **no CSS rule exists anywhere in the repo** — `ok`, `error` and `abandoned`
  are visually identical.

---

## D. What was fixed in this branch

See the commits on `worktree-agent-a7e53caf132134af8`.

**Fixed**

- **A1** — `tool_target`, `tool_result`, `duration_ms`, a better `tool_name`, and
  `tool_status` mapped for every codex tool item variant.
- **A3** — `turn_failed` declared on `turn/completed` gated by `turn.status`;
  `turn_stop` gated to non-failure statuses.
- **A4** — `tool_fail` declared, gated on `item.status: failed || declined`,
  carrying `tool_error`.
- **A5** — `webSearch` and `dynamicToolCall` added to the tool `when:` gate.
- **A6** — fixtures added for every codex `ThreadItem` variant Crowbar maps, and
  the replay harness changed so an unexercised sum-type variant is a **failure**
  rather than a silent skip.
- **A2** — fixed generically: `telemetry.ParseCallback` now falls back to the v3
  `events.telemetry` field map when no v2 `telemetry.callback` block is declared.
  This is a Crowbar-wide primitive; any v3 provider gets it.
- **C1** — the working line now renders the blocking interruption instead of
  returning `null`, using the already-written `describeInterruption` copy.

**Left open** (designed, not implemented — each needs a new vocabulary event plus
Go persistence plus frontend rendering plus live verification, which is more than
one reviewable change):

- **B / reasoning streaming** — the highest-value remaining item.
- **B / plan updates**, **command output deltas**, **tool progress**.
- **B1** — moving the five hooks-routed events onto the api transport.
- **C2**, **C3**, **C4**.

## E. Honest verification status

- Go: targeted `go test` runs on the touched packages, all green (see commits).
- Payload shapes: derived from codex's **generated schema**, not from live
  capture — codex is not installed on this machine. The new fixtures are marked
  as schema-derived in-file so nobody mistakes them for live recordings. This
  matters: the repo's own history records four paths written from a published
  schema that were wrong against real traffic. These are generated-from-source
  rather than hand-written docs, which is stronger, but it is **not** a live capture.
- Frontend: unit tests only.
- **No live Tauri verification was performed**, because reproducing any of this
  requires a working codex CLI and an authenticated account, neither of which is
  available in this worktree. Every UI-visible change here should be re-checked
  in `make dev-desktop` against a real codex chat before this is considered done.
