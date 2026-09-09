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


### Second pass — the capability gaps in section B

Everything in section B was then closed too, driven by **real captured traffic**
rather than the schema. A harness (`scratchpad/capture.py`) drove a real
`codex app-server` over stdio and recorded every frame; the recordings replaced
the schema-derived fixtures and are what the mappings below were written against.

- **B / reasoning** — `reasoning_delta`, a new canonical event. Same shape as
  `message_delta` and it rides the **same live channel**, distinguished by a
  `kind` on the frame rather than by a parallel fan-out. Live-only: nothing
  durable is written, because recording it would put the model's thinking into
  the transcript as though it had said it. Rendered in the working line, trimmed
  to its tail, markdown emphasis stripped, clamped to two lines.
- **B / command output deltas** — `tool_output_delta`, the second kind on that
  same channel. Rendered under the running tool row it belongs to.
- **B / plan updates** — `plan_update`, the first canonical event to need a
  **structured extra** (`steps:`, beside `map:`, as `rate_limits:` already is).
  Its `status_map` translates a provider's own status words into Crowbar's
  pending/active/done, so Go never learns that codex spells one "inProgress".
- **B / `thread/status/changed`** — `idle`. See the correction below.

### A design in this report that live capture proved WRONG

The first pass proposed mapping `thread/status/changed` as a turn-closer.
Capturing a real session showed `{"type":"idle"}` arriving **immediately before**
`turn/completed` on a perfectly healthy turn — sub-millisecond. Implementing it
as proposed would have abandoned *every* turn microseconds before it ended
properly, salvaging and closing work that was about to be recorded correctly.

It is instead a **latch**: the report arms it, the turn's own close disarms it,
and a latch still armed when the 2s sweep next looks is a turn nothing is going
to close, on a provider that has said so itself. That covers the case nothing
else could reach — the stall detector needs a declared notice on a PTY for 120s
and the abandoned-message detector needs a half-written message idle for 30s, and
a codex turn that only reasoned produces neither.

### A second thing only live verification could find

Mapping `reasoning_delta` was not enough: **codex emits no reasoning summaries
unless asked**. Measured by running the identical reasoning-heavy prompt twice —
zero `item/reasoning/*` frames with the default configuration, a full stream with
`model_reasoning_summary` set, isolated to that one knob. Without the matching
`config_injection` line the whole feature was inert in production while every
unit test passed. It was found by driving the real app and watching nothing
appear.

## E. Verification status

### Automated

- Go: the **whole** `./internal/...` suite, green.
- Frontend: `bun tsc --noEmit` clean, and the full `features/agent` +
  `features/workspace` suites — **2028 tests, green**. (The first pass could only
  run a subset because this worktree had no `node_modules`; they are installed now.)
- The B2.1 race was **measured**, not asserted: 6/20 and 8/20 failing on the
  unmodified tree, 0/25 after.

### Live, in the real app

`make dev-desktop` in this worktree (its own `CROWBAR_HOME`, its own derived
origin `localhost:5515`, its own MCP bridge on 9225 — three other worktrees'
dev apps were running and were **not** touched), seeded with `make seed`, driving
a real `codex` 0.149.1 chat. Console evidence captured via `read_logs`.

Confirmed working end to end:

- **Tool target and duration** — rows render
  `set_chat_title · crowbar 7ms` and
  `commandExecution · /bin/zsh -lc "…" 8.0s`.
  Before the fix both were bare type names with no target and no duration.
- **Telemetry** — the context gauge renders `36% context`,
  title `93,876 of 258,400 tokens`. A codex chat had never shown this.
- **Reasoning** — the working line showed the model's own thought,
  `"Searching for crowbar tool"`, during the stretch where codex emits nothing else.
- **Tool output** — streamed live under its own tool row, one line per second:
  `tick 2` → `tick 2 tick 3` → … → `tick 2 … tick 10`.
- **Plan** — a three-step task drove the checklist through every state, captured
  as four distinct renders:
  `active,pending,pending` → `done,active,pending` → `done,done,active` →
  `done,done,done`. Those are CROWBAR'S status words, translated from codex's
  `inProgress`/`completed` by the descriptor's `status_map` — the frontend never
  saw a codex spelling.
- **No stuck spinner, and live state cleared** — at turn end the working line was
  gone (`workingLine: false`) and the plan with it (`planStillShown: false`),
  which is the turn-edge clear working on both sides.

### Still not verified live

- **`idle`** is covered by unit tests (the latch, and the sweep detector's three
  branches) and its `when:` routing is replayed against a live-captured fixture of
  the real `thread/status/changed` frame — but its RECONCILE was not observed
  firing in the running app. By construction it only fires on a turn whose close
  never arrives, which is not something a healthy codex will do on demand. Every
  healthy turn exercised above disarmed the latch correctly, which is the other
  half of the contract and is what a wrong implementation would have broken.
- **`tool_fail` / `turn_failed`** — mapped and unit-tested, but no failing codex
  turn was provoked live.
- The **B1** hooks-routed events (subagent/compaction/session-end on a
  disconnected companion PTY) were untouched and a known gap as of this
  section — **see section F below**: compact_pre/compact_post are now moved
  and live-verified (including in the real dev-desktop app), session_end was
  investigated and confirmed to need no change, and subagent_pre/subagent_post
  remain hooks-transport with a much better-informed follow-up spec, plus a
  real, live-verified partial fix (collabAgentToolCall as an ordinary tool
  call) landed alongside them.

---

## F. Section B1 — the disconnected-PTY gap, closed for two of five events

Investigation date: 2026-09-08. Branch: this one, on top of `337a59437`.

Same discipline as the rest of this report: codex's own generated protocol
schema (`codex app-server generate-json-schema`, codex-cli 0.149.1 — the exact
version already captured against elsewhere in this file) was read first for a
hypothesis, and every mapping below was then driven against a REAL `codex
app-server` process over its actual stdio JSON-RPC transport before being
trusted. The capture harness lives at `scratchpad/capture.py` in this
worktree (a fresh one — the prior one did not survive into a new worktree, per
this doc's own note; `git log --all --diff-filter=A -- '**/capture.py'`
confirmed there was nothing to recover).

### F1. `compact_pre` / `compact_post` — moved onto the live connection

**CONFIRMED live.** `thread/compacted` really is deprecated in favour of a
`contextCompaction` ThreadItem, exactly as its schema description says. Driving
`thread/compact/start` against a real app-server produces:

```
turn/started   (a NEW turn, itemsView: notLoaded)
item/started   {type: contextCompaction, id: "..."}      <- compact_pre
item/completed {type: contextCompaction, id: "..."}      <- compact_post
turn/completed (status: completed, items: [], itemsView: notLoaded)
```

`codex.yaml`'s `compact_pre`/`compact_post` now declare `in: item/started` /
`in: item/completed`, `when: { item.type: contextCompaction }` — the exact
same sum-type pattern `tool_pre`/`tool_post` already use, gated by `item.type`
instead of a dedicated notification. The `contextCompaction` item itself
carries only `{id, type}` — no trigger, no summary — so `trigger:` is left
unmapped rather than pointed at a path that would never resolve, the same
call `plan_update`'s own `explanation` field already made.

**The trap only live capture found.** `thread/compact/start`'s own
`turn/started..turn/completed` wrapper rides the EXACT SAME wire event
`turn_stop`/`turn_failed` already consume unconditionally. The obvious
guess — gate on the wrapper's empty `items`/`itemsView: notLoaded` shape — is
**wrong**, proven by driving two more scenarios against the same live
app-server:

| Scenario | `turn.status` | `turn.items` | `turn.itemsView` |
|---|---|---|---|
| Compaction's own wrapper | `completed` | `[]` | `notLoaded` |
| A genuinely **interrupted** real turn | `interrupted` | `[]` | `notLoaded` |
| A real turn that ran a tool and sent **no final message** | `completed` | `[]` | `notLoaded` |

All three are byte-for-byte the same shape. Gating `turn_stop` on itemsView or
an empty items list — the first idea — would have suppressed the close of a
real turn the user actually interrupted, or one that legitimately ended with
only tool calls: the identical class of "designed from the schema, wrong
against real traffic" mistake this report's own `thread/status/changed`
section already documents once.

**The fix**: `turn_id`. `compact_pre`'s `item/started` envelope carries
`turnId`; the wrapper's own `turn/completed` carries `turn.id` — the SAME
value. `turn_stop` and `turn_failed` now map `turn_id: turn.id`
(`vocabulary.yaml` grew an optional `turn_id` field on both, plus on
`compact_pre` — `compact_post` already had one). A new latch,
`api/internal/app/usecases/chat/internal/turn/compaction.go`
(`compactionTurns`, one armed id per chat, the same shape `idle.go`'s
`idleLatch` already uses), is armed by `compact_pre` and consulted+consumed by
`closeTurnFromStop`/`closeTurnFromFailure` before either does anything else. A
miss leaves whatever is genuinely armed untouched — the first version deleted
on any lookup, match or not, which would have cleared a real compaction's own
latch out from under it if an unrelated turn's stop ever landed first; caught
by `TestCompactionTurns_AnUnrelatedTurnIDIsNotConsumed` before it shipped.

Two `TestRegression_*` tests in `api/internal/app/usecases/chat/turn_test.go`
drive this end to end through the real usecase and the real descriptor
(`TestRegression_CodexCompactionTurnNeverStopsTheChat`,
`TestRegression_CodexFailedCompactionTurnRecordsNoFailureNotice`), and both
were confirmed to actually fail without the guard (temporarily reverted,
rerun, restored) before being trusted — the guard against a guard test that
passes vacuously.

**A related dead-code fix, necessary to make any of this reachable from the
UI**: `Runners.Compact` (`internal/app/usecases/chat/internal/runner/compact.go`)
refused outright whenever a provider's `compact_start` declared anything other
than `wire == "prompt"` — "the jsonrpc transport... does not exist yet". It
did, since `interruptTurn` already drives `turn/interrupt` the identical way
(`conn.driver.Send`, confirmed by `APIConn.Send`'s own doc comment: "drives a
plain outbound canonical event (interrupt, **compact_start**)"). Codex's own
compact button was calling this function and getting `ErrUnavailable` every
time, silently. `Compact` now drives `conn.driver.Send(ctx, "compact_start",
nil)` over the chat's live api connection for any non-prompt wire, mirroring
`interruptTurn` exactly. Without this fix, `compact_pre`/`compact_post`'s new
live mapping would have nothing to observe except codex's own automatic
compaction, which cannot be provoked on demand.

New fixtures, both live captures:
`item_started.contextCompaction.json`, `item_completed.contextCompaction.json`.

### F2. `subagent_pre` / `subagent_post` — the model's own multi-agent tool, mapped as a tool call; the nested-thread redesign left as a scoped follow-up

**Do not guess how this behaves** was the instruction, so it was driven live
rather than designed from the schema. Two independent full runs
(`-c features.collab_agents=true`, a prompt asking the model to spawn a
sub-agent, wait for it, and relay its answer) produced the **exact same**
sequence both times:

```
PARENT thread, ordinary turn:
  item/started   {type: collabAgentToolCall, tool: spawnAgent, receiverThreadIds: []}
  item/completed {type: collabAgentToolCall, tool: spawnAgent,
                   receiverThreadIds: [childId], agentsStates: {childId: {status: pendingInit}}}

CHILD thread — its OWN complete, independent turn/started..item/*..turn/completed
cycle, pushed over the SAME connection despite the client never calling
thread/start or thread/resume on it:
  turn/started -> item/started(userMessage) -> item/started(reasoning) ->
  item/started(agentMessage) -> delta -> item/completed -> turn/completed

PARENT thread, continuing:
  item/started   {type: collabAgentToolCall, tool: wait, receiverThreadIds: [childId]}
  item/completed {type: collabAgentToolCall, tool: wait,
                   agentsStates: {childId: {status: completed, message: "done"}}}
  item/started   {type: collabAgentToolCall, tool: closeAgent, receiverThreadIds: [childId]}
  item/completed {type: collabAgentToolCall, tool: closeAgent, ...}
  (parent's own agentMessage relaying "done", then turn/completed)
```

**`subAgentActivity` — the item type the first pass's schema reading guessed
this would ride, with its `started|interacted|interrupted` kinds — NEVER
appeared, in either full capture.** It is presumably reserved for codex's OWN
internally-spawned subagents (`SubAgentSource`'s bare `review | compact |
memory_consolidation` variants — an auto-review or auto-compaction subagent
Crowbar never asked for), not for an explicit model-driven collab tool call.
That remains **unconfirmed** — no live capture of it exists.

`review/start` with `delivery: detached` was also tried as a more
deterministic trigger (it does exist, and does spawn a real independent
thread, returned as `reviewThreadId`) — it turned out to be an unrelated
mechanism entirely, nothing to do with `collabAgentToolCall`/`subAgentActivity`,
and is not part of this pass's design.

**What shipped**: `collabAgentToolCall` was added to `tool_pre`/`tool_post`/
`tool_fail`'s existing `item.type` gate, alongside `commandExecution` etc. —
not as a new `subagent_pre`/`subagent_post` pair, but as an ordinary tool
call, because that is structurally what it is: one `item/started`..
`item/completed` pair per collab action, with an `id`, a `status`
(`inProgress|completed|failed` — no `declined`, so it can only ever match
`tool_fail`'s `failed` branch), and (for `spawnAgent`) a human-readable
`prompt`. `tool_target`'s alternation grew `item.prompt || item.receiverThreadIds[0]`
(the latter an INDEX selector — the grammar has no way to select a value out
of `agentsStates`, which is keyed by a thread id it cannot know in advance,
so that map is exposed via `tool_result`'s alternation whole, as JSON, rather
than picked apart). Live-verified: a real spawn/wait/closeAgent sequence now
renders as three ordinary, correctly-targeted-and-timed tool rows instead of
nothing at all — the actual current state of things before this pass, since
neither `tool_pre`/`tool_post` (item.type not in the gate) nor the
hooks-transport `subagent_pre`/`subagent_post` (fired from the disconnected
companion PTY's own unrelated conversation) ever saw it.

**What is deliberately left as a follow-up**: a real `StartSubagent`/
`StopSubagent`-shaped `subagent_pre`/`subagent_post` — the activity-ledger
entries the subagent shelf reads (`subagent-shelf.tsx`). That needs Crowbar's
own activity ledger to grow a model it does not have: a subagent is a WHOLE
SECOND THREAD with its own turn/item stream, not a flat id with a start and a
stop. Landing that without either a broken mapping or a redesign of
`domain.ActivitySubagent` itself was judged out of scope for this pass — per
the task's own priority order, tool-call visibility for the *existing*
mechanism first, a correct nested model later, rather than a hurried and
unverified one now. `subagent_pre`/`subagent_post` remain hooks-transport,
unchanged, still reading the disconnected companion PTY's own conversation —
codex.yaml's comment on them now records this investigation's findings in
full for whoever picks this up next.

New fixtures, both live captures:
`item_started.collabAgentToolCall.json` (spawnAgent),
`item_completed.collabAgentToolCall.json` (spawnAgent, completed),
`item_completed.collabAgentToolCall.wait.json` (wait, completed — the
variant whose `prompt` is null, proving the `tool_target` fallback to
`receiverThreadIds[0]` is real rather than merely written).

### F3. `session_end` — investigated, confirmed no new mapping needed

`HookSessionEnd`'s dispatch (`turn/ingest.go`) was already, deliberately, a
no-op on EITHER transport — its own comment says why: "A session ending is
already observed authoritatively by the PTY exit reconcile... Acting on it
here as well would close a turn twice." For the api transport specifically,
`onAPIConnLost` (`runner/connloss.go`) is that authority: it forgets the
connection's registry entry and reconciles the turn the instant the driver's
`Events()` channel closes, whether that is a clean exit or the process dying.

Codex's api schema has no dedicated "session is ending, and here is why"
notification. The closest candidate, `thread/closed` (`{threadId}`, no
reason field), was tried live against the one client-reachable action that
looked like it might precede it — `thread/unsubscribe` — and it did not fire.
No other `ClientRequest` looked like a plausible trigger for it either. Absent
a reachable trigger, mapping it would have been exactly the kind of
ships-but-never-fires change this report's own methodology exists to catch
(see section D's `thread/status/changed` correction). Nothing was added;
`session_end` stays on hooks, unchanged, with a comment recording this
investigation so it is not repeated blind.

### F4. Verification

**Automated**: `go build`/`go vet` clean on every touched package (`gofmt -l`
clean too). Targeted suites green:
`internal/app/usecases/chat/...`, `internal/engine/agents/...`
(includes the fixture-replay harness — `codex/compact_pre` and
`codex/compact_post` now resolve against live-captured traffic instead of
logging "event unverified"). Per this repo's own rule, the full `go test
./...` was NOT run — only the packages this pass touched or that import them.
Both new `TestRegression_*` tests, and all six new `compactionTurns` unit
tests, were confirmed to actually catch the bug they guard (guard temporarily
reverted, test rerun to see it fail, guard restored) before being trusted —
not just asserted to pass.

**Live, in the real app**: `make dev-desktop` in this worktree, own
`CROWBAR_HOME`, own derived origin/MCP-bridge port 9225 (checked `ps aux`
first; other worktrees' dev instances on ports 9223/9224 and vite 5699/5719
were running and were **not** touched), seeded with `make seed`, driving a
real `codex` 0.149.1 chat via the Tauri MCP bridge.

A real ordinary turn confirmed the baseline is unharmed: a fresh codex chat
replied normally (`ready`), the context gauge rendered `7% context`, and the
turn closed cleanly (`working: false`).

**A real /compact, driven and observed live.** The frontend gap the live app
surfaced first: `compactChat()` (`web/src/features/agent/api/agent-api.ts`)
is wired to the backend but **no component in `web/src` currently calls
it** — there is no button, menu item, or shortcut in the running UI to press.
That is a pre-existing frontend gap, separate from this pass's backend scope
(confirmed by a dedicated search of the whole frontend tree; the `/compact`
text some chats accept is an unrelated path — a literal prompt string a CLI's
own built-in slash-command parser may or may not honor, which for an
api-transport codex turn does nothing but ask the model to talk about
compacting). Rather than leave `Compact` unverified, it was driven the way
any other real client of this same daemon would — a `POST` on its own unix
socket, the exact route `compactChat()` itself calls
(`.../projects/:id/home/chats/:id/compact`), against the SAME live app,
SAME live codex connection, SAME running frontend watching the same
WebSocket:

```
$ curl -s -X POST --unix-socket <daemon socket> \
    http://localhost/v0/projects/.../home/chats/<id>/compact
{"success":true,"data":{"id":"2a01b8b0-6544-4f24-ae74-d4463bf289b2"}}   (202 Accepted, 2.6ms)
```

Confirmed in the running webview, immediately after:

- The working line rendered **`Compacting…`** and the composer showed
  **`Compacting… your message will be queued`** — the live push
  (`compaction_started`) this pass's `Compact` fix made reachable for codex
  for the first time.
- Both cleared on their own moments later (`compaction_stopped`), composer
  back to `Message the agent…` — no stuck indicator.
- The ledger (`GET .../messages`) shows **exactly the same two turns** as
  before the compaction — the user prompt and the `ready` reply, nothing
  else — confirming the turn_id-armed latch did its job: the compaction's own
  turn/completed produced **no** spurious ledger entry.
- `GET .../chats/:id` shows `working: false` throughout, and the daemon's own
  access log (`.crowbar/logs/daemon.log`) shows the request and every
  telemetry poll around it clean — `200`/`202`, no warnings, no errors.

This is the exact live proof the "ships but never fires" trap this report's
own methodology exists to catch — the frontend button doesn't exist yet, but
everything this pass actually owns (the wire mapping, the turn_id guard, the
RPC-transport `Compact` fix) was driven for real and behaved correctly.

**`collabAgentToolCall` (tool_pre/tool_post/tool_fail)** was live-verified
against a real codex app-server twice over (Section F2's capture, not
dev-desktop — it needs `-c features.collab_agents=true`, a flag Crowbar's own
spawn config does not set, and adding it purely to re-run this one check in
the full app was judged not worth changing production spawn args for). The
frontend rendering of a tool_pre/tool_post pair as a transcript row is
already the mechanism this branch's own A1 fix verified live, unchanged by
this addition — only the `item.type` gate grew a new accepted value.
