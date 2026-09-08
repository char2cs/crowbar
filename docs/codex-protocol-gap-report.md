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

## B2. "We lose track of whether codex is working" — three separate desyncs

Reported as the single most annoying bug. It is not one defect but three, and
codex hits all three hardest for the same structural reason: **codex reports no
async-work level of its own**, so Crowbar's own recount of the tool calls and
subagents in its activity ledger is the *only* thing that ever darkens a codex
chat once its top-level turn has ended with work still open. Claude restates its
own `background_tasks` level and is largely self-healing; codex has nothing.

### B2.1 The activity ledger was read back before it had been written

`OpenWork` — "is any tool call or subagent still open?" — reads the activity
**read model**. The four commands that write it (`InvokeTool`, `CompleteTool`,
`StartSubagent`, `StopSubagent`) were on asynx's **async** send path, and
`turn.go` calls `OpenWork` on the very next line after each of them.

So the recount routinely observed the state from *before* the write it was meant
to see. Both directions are a wrong spinner:

- a close not yet folded reads as still-open → the recount that would have
  stopped the spinner never happens → **stuck on**;
- an open not yet folded reads as idle at `turn_stop` → **the spinner darkens
  under a tool call that is genuinely still running**.

Measured: `TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking` failed
**6/20 and 8/20** on the unmodified tree. This was a *pre-existing* flake that
had been living in the suite; it is a real production race, not a test artifact.
After the fix: **0/25**.

### B2.2 The recount decided its own preconditions on projected state

`restateAsyncWork` asked both of its preconditions — that no turn is currently
open, and that the level actually changed — out in the caller, off `domain.Chat`
read back through `GetChat`. That read model is folded by an asynchronous
projection, so a `turn_stop` already durable in the log could still read as open:
the function took its early return and no recount was ever appended.

This is the *identical* mistake `stop_turn.go` already documents having fixed for
`AbandonTurn`:

> It used to be asked by the caller instead … and the read model is folded by an
> ASYNCHRONOUS projection … The caller took the early return, nothing ever closed
> the turn, and the chat spun forever. **A reconcile must not decide on projected
> state.**

### B2.3 A lost api connection was never noticed — the chat went permanently silent

The highest-severity one. `pumpAPIConn`'s loop simply **returned** when the
driver's `Events()` channel closed — no teardown of any kind:

```go
go func() {
    for ev := range conn.driver.Events() { ... }
}()   // no defer, no drop, no reconcile
```

When the `codex app-server` process died or the socket dropped:

1. the open turn was never closed → `working` stayed true forever;
2. the registry entry stayed → `HasLiveAPIConnection` kept answering **true**;
3. so `apiOwnsThisEvent` kept **discarding the companion PTY's hooks copy** of
   every api-owned event as a redundant duplicate of a transport that no longer
   existed — the chat fell **permanently silent**, not merely stuck-spinning;
4. nothing else could reach it: the companion PTY is still alive, so no
   runner-exit reconcile fires, and neither `termwait` sweep applies (one needs a
   declared `ends_turn` needle on screen for 120s, the other needs a half-streamed
   message idle for 30s — a codex turn that only *reasoned* produces neither,
   because reasoning deltas are unmapped, per section B).

This survives a daemon restart: the event log records the turn as open, and the
boot reconcile only closes turns whose PTY is also dead.

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
- **B2.1** — the four tool/subagent lifecycle commands use `sendWait`, exactly as
  `OpenChoice` already does for the same "a caller reads this back immediately"
  reason. 6–8/20 flake → 0/25.
- **B2.2** — both preconditions moved into `StopTurn.Validate` behind a new
  `Restate` flag, where asynx evaluates them against the authoritative fold.
- **B2.3** — a lost api connection now forgets its registry entry (so the hooks
  wire is honoured again instead of being suppressed) and reconciles its turn the
  same way a dead CLI does. A *deliberate* teardown is told apart by the
  connection's own cancelled ctx, since each of those already owns its turn.
- **C1** — the working line now renders the blocking interruption instead of
  returning `null`, using the already-written `describeInterruption` copy.

**Left open** (designed, not implemented — each needs a new vocabulary event plus
Go persistence plus frontend rendering plus live verification, which is more than
one reviewable change):

- **B / reasoning streaming** — the highest-value remaining item.
- **B / plan updates**, **command output deltas**, **tool progress**.
- **B1** — moving the five hooks-routed events onto the api transport.
- **C2**, **C3**, **C4**.

### The single highest-value item still open

`thread/status/changed` is codex's **authoritative** answer to "am I working":
`status.type` ∈ `{notLoaded, idle, systemError, active}`, plus `activeFlags` ∈
`{waitingOnApproval, waitingOnUserInput}`. Crowbar ignores it completely — there
is even a captured live fixture (`thread_status_changed.json`) sitting unused in
testdata.

Mapping it needs a new canonical event, because the vocabulary is closed and has
no name for a provider-reported idle state. The design that fits: an `idle`
inbound event (`required: []`, `optional: [session_id]`), which codex maps as
`in: thread/status/changed`, `when: {status.type: idle}`. Its Go handler
reconciles an orphaned open turn — the same `AbandonMessage` salvage-then-abandon
the quiet-screen sweep already performs, but driven by the provider's own word
instead of a 30–120s heuristic that a codex turn frequently never satisfies.

That would close the residual of B2.3 (a turn orphaned while the connection is
still *up*) and is a genuinely generic primitive — ACP models the same thing as
`SessionComplete` / `StatusChanged`. It was scoped out here because it needs
vocabulary + Go + persistence + tests, and the three fixes above address the
measured causes.

## E. Honest verification status

- Go: targeted `go test` runs on the touched packages, then the **whole**
  `./internal/...` suite, all green. The specific race in B2 was measured before
  and after (6–8/20 failing → 0/25), not asserted.
- Payload shapes: derived from codex's **generated schema**, not from live
  capture — codex is not installed on this machine. The new fixtures are marked
  as schema-derived in-file so nobody mistakes them for live recordings. This
  matters: the repo's own history records four paths written from a published
  schema that were wrong against real traffic. These are generated-from-source
  rather than hand-written docs, which is stronger, but it is **not** a live capture.
- Frontend: unit tests only, and only a subset could run at all. This worktree
  has no `node_modules`; the tests were run against the main worktree's install,
  which predates several of this branch's dependencies (`react-dnd`,
  `papaparse`, `platejs`). 26 test files fail to resolve imports there
  **regardless of these changes**. The files covering what was touched do run:
  `working-line`, `agent-activity`, `composer-choice`, `composer-state`,
  `use-agent-activity` — 114 tests, green.
- **No live Tauri verification was performed.** `codex` is not installed on this
  machine (`which codex` → not found), so a real codex chat cannot be exercised
  here at all. Every UI-visible change in this branch — the tool rows now showing
  a target and output, the context gauge appearing, the blocked-state line, the
  spinner actually stopping — must be re-checked in `make dev-desktop` against a
  real codex chat before this is considered done. Treat that as outstanding work,
  not a caveat.
