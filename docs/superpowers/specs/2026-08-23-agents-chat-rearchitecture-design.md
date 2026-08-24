# Agents → Chats — subsystem re-architecture

**Date:** 2026-08-23

**Status:** IMPLEMENTED (2026-08-23), with four deviations recorded in §11. Design spec. Supersedes the structural decisions of
[`2026-08-17-agents-engine-implementation.md`](./2026-08-17-agents-engine-implementation.md)
§3 (package layout) and §5 (engine interface). Its §1 correction on asynx patch
storage still holds and is assumed here.

**Scope:** every package in `api/` with an agent relation — the engine, the usecase,
three repositories, one adapter store, and the HTTP endpoint group. 23,900 production
lines today.

**Not in scope:** `engine/mcp` and the asynx library itself. The frontend is out of scope
except for one required companion change: the route rename in stage 8 (§8) breaks every
agent URL the web client calls, and must land with it.

---

## 1. What is wrong today

Measured on `feature/chat-wrapping` at `d9f84e78`.

| package | prod LOC | what it is |
|---|---|---|
| `app/usecases/agent` | 6,503 | everything: spawn, hooks, turns, answers, switch, catalog |
| `engine/agents` | 4,522 | a descriptor reader, nothing more |
| `app/usecases/agenttools` | 3,205 | the MCP tool surface CLIs call back into |
| `app/repositories/agentactivity` | 2,368 | asynx aggregate |
| `api/v0/endpoints/agent` | 1,812 | 30 routes, 6 port interfaces |
| `app/repositories/agentrunner` | 1,712 | asynx aggregate |
| `app/repositories/agentchat` | 1,444 | asynx aggregate |
| `app/usecases/agentchatfolder` | 1,190 | the Chats tree |
| `adapter/store/agentjournal` | 1,144 | on-disk exactly-once journals |

### 1.1 The engine is not an engine

`agents.Agent` is a 26-method interface and every method is a pure function of a YAML
file: `Models()`, `SelectionSteps()`, `ParseHook()`, `RenderAnswer()`, `SpawnPlan()` —
a *plan*, not a spawn. The only child processes `engine/agents` starts are one-shot
probes (`internal/exec`, for the slash catalog and telemetry). It owns no PTY, no
conversation, no runner.

Consequence: every **verb** — spawn, inject, route a hook, hold an answer gate, detect
a terminal wait, switch provider — had nowhere to live but the usecase. That is why the
usecase is 6,503 lines.

### 1.2 The five concerns are a cycle, not a decomposition

`agent.New` takes 12 parameters, constructs 8 pieces of shared mutable machinery (spawn
gate, turn registry, work mirror, turn-start interlock, hook barrier, message streams,
answer desk, two journals), then binds `chat↔runner`, `turn↔chat`, `turn↔runner`,
`runner→providers` and `runner→termWait` *after* construction, because the graph has
cycles. No concern can be constructed, tested or reasoned about alone.

### 1.3 The same taxonomy is declared three times

Engine capability flags → five usecase concerns → five handler port interfaces
(`handlers/handlers.go`, 384 lines of interface). Adding one method to the chat surface
edits three files.

### 1.4 State has three tiers and only two are packages

asynx aggregates (5,524 LOC), disk journals (1,144), and ~8 in-memory maps inside the
usecase. The in-memory tier carries the hard invariants — *`work.set` must publish
before `turns.complete`*, *the spawn gate must never be taken on the hook-ingest path* —
and it is the tier with no package, no name and no test boundary.

### 1.5 The repository layer broadcasts to the frontend

`agentchat` and `agentrunner` both push WS deltas from inside their store-layer
projections. The usecase is not in that path at all.

### 1.6 The descriptor is organised by mechanism, not by event

One concept is split across blocks: `hooks.events.permission` parses the inbound
permission, `answer.permission` renders the reply, and *how it arrives* is implicit in
which top-level key it sits under. Four separate inbound channels (`hooks`,
`telemetry`, `terminal_prompts`, `terminal_notices`) each have their own parser.

Event names are Go constants (`spec.HookSessionStart`) and per-event required-field
rules are hardcoded in `descriptor/internal/rules/hook_vocabulary.go`, so a third
provider cannot be added without writing Go.

---

## 2. Principles

Three rules, applied recursively at every level.

1. **One public file per package.** `<name>.go` is the package's whole exported face.
   Optional `types.go` and `errors.go`. Everything else lives under that package's own
   `internal/`. Precedent: `engine/agents/internal/catalog/`, and `quiver.core`'s
   `repositories/arrow/`.
2. **Engines never cross-import.** A capability two or more engines need is promoted to
   `core/`. Only `engine/container.go` may import more than one engine.
3. **The usecase holds no machinery.** It wires the engine to asynx and to the frontend.
   If a type in the usecase has a mutex, it is in the wrong layer.

---

## 3. The boundary

> The usecase layer is the thing that wires the agent engine with asynx and with
> Crowbar's frontend.

**`engine/agents` owns:** reading descriptors; speaking a provider's protocol in both
directions; the lifecycle of a live CLI and the terminal attached to it; the durable
record of *which process is talking to which native conversation*.

**`app/usecases/chat` owns:** translating engine Facts into asynx commands, and asynx
deltas into frontend WS messages. Nothing else.

**`app/repositories/chat` owns:** Crowbar's model of a conversation — thread, title,
placement, selection, activity.

### 3.1 Why the runner aggregate moves into the engine

`domain.AgentRunner` has 13 fields. Eleven are engine-scoped: provider id, PTY session,
native conversation id, launch model, launch effort, resumability, conversation-open
timestamp, started/exited. Only `WorkspaceID` and `CurrentChatID` are Crowbar's.

It is the engine's state machine wearing app clothes. Moving it is a relocation, not a
duplication — no new asynx instance is created.

The type moves with it: `domain.AgentRunner` becomes `agents.Runner` in
`engine/agents/types.go`. `domain.Chat` stays in `domain/`, since the usecase, the
repository and the API all read it.

### 3.2 Two logs, and what keeps them consistent

The engine's runner log and Crowbar's chat log hold different facts:

| | engine runner log | Crowbar chat log |
|---|---|---|
| which process, which PTY | ✓ | |
| native conversation id | ✓ | |
| model/effort **launched** | ✓ | |
| model/effort **desired** | | ✓ |
| live turn | ✓ (non-durable) | |
| turn record, activity | | ✓ |
| title, placement | | ✓ |

Launched-vs-desired looks like duplication but is not: comparing them is exactly what
decides whether the next prompt must replace the process.

**The one edge to guard:** the runner points at the chat; the chat never points back.
Liveness is "a runner row exists pointing here" — stored once, on the runner side. This
is today's rule and it is retained verbatim.

**Skew:** the two logs synchronise only through the Fact stream, so a crash between the
engine's write and the usecase's translate leaves them apart. Resolution: the engine's
log is the source of truth for the conversation, and boot replays Facts forward into
chat. This replaces today's `ReconcileRunnersOnBoot` orchestration.

### 3.3 asynx instances

Unchanged in count. `asynx.Asynx[agents.Runner]` (was `axAgentRunner`) and
`asynx.Asynx[domain.Chat]` (replacing `axAgentChat` + `axAgentActivity`). Both are built
in `app/container.go` over the shared sqlite event store and **injected**, so `engine/`
never imports `adapter/eventstore/sqlite` and stays testable against an in-memory asynx.

---

## 4. The descriptor

### 4.1 The event is the unit

Every conversational fact is one block. `transport:` is a property of the event, not of
the provider, so hooks/api/mixed falls out with no top-level mode flag.

Three keys carry everything:

- **`in:`** — they tell us. Maps a wire event onto a canonical Fact.
- **`out:`** — we tell them. Maps a Crowbar Intent onto a provider call.
- **`ask:`** — they block on our reply. Both halves plus a correlation id.

`answer:`, `presentation.prompt_submit`, `terminal_prompts` and `terminal_notices`
disappear as top-level concepts; they become events with a different transport.

### 4.2 Codex — verified surface

`codex app-server --listen stdio://|unix://PATH|ws://…` is a JSON-RPC 2.0 duplex
channel. Verified on `codex-cli 0.146.0`: `initialize` returns
`{userAgent, codexHome, platformFamily, platformOs}` and the server pushes
notifications unprompted.

The published protocol (`codex app-server generate-json-schema --out DIR`) declares:

- **90 `ClientRequest` methods** — `thread/start`, `thread/resume`, `turn/start`,
  `turn/steer`, `turn/interrupt`, `model/list`, `skills/list`,
  `account/rateLimits/read`, `thread/inject_items`.
- **70 `ServerNotification` methods** — `thread/started`, `turn/started`,
  `turn/completed`, `item/started`, `item/completed`, `item/agentMessage/delta`,
  `thread/tokenUsage/updated`.
- **10 `ServerRequest` methods** — the CLI asking *us* and blocking on the reply:
  `item/permissions/requestApproval`, `item/commandExecution/requestApproval`,
  `mcpServer/elicitation/request`, `item/tool/requestUserInput`.

That third group is Crowbar's *choice* concept, native. It is what `ask:` models.

`codex --remote unix://PATH` attaches the TUI to the same app-server, so one
conversation yields both the structured protocol and the terminal pane. Codex therefore
needs no screen scraping at all.

Real field names, from the generated v2 schemas:

| method | required params |
|---|---|
| `thread/start` | *(none)*; response requires `thread`, `model`, `cwd`, `sandbox` |
| `thread/resume` | `threadId` |
| `turn/start` | `input`, `threadId` |
| `turn/completed` | `threadId`, `turn` |
| `item/agentMessage/delta` | `delta`, `itemId`, `threadId`, `turnId` |
| `item/started` | `item`, `startedAtMs`, `threadId`, `turnId` |
| `thread/tokenUsage/updated` | `threadId`, `tokenUsage`, `turnId` |

**VERIFIED 2026-08-23 against recorded traffic** (13 frames from a real turn,
`scripts/capture-codex-fixtures.sh` → `protocol/testdata/fixtures/codex-api`). Four
paths written from the schema were wrong:

| written | actual |
|---|---|
| `turn.lastAgentMessage` | does not exist — `turn.items[type=agentMessage].text` |
| `item.output` | does not exist — `item.content` |
| delta `.sequence` | does not exist; codex numbers nothing |
| `tokenUsage.inputTokens` | nests: `tokenUsage.total.inputTokens`, window at `tokenUsage.modelContextWindow` |

Two consequences: the mapping grammar gained **array selection**
(`items[type=agentMessage].text`), without which codex's final message is unmappable;
and `message_delta.index` became optional, because codex supplies none.

Still uncaptured: `permission` and `elicitation`, which need the CLI to actually ask.
The replay test names each unverified event rather than passing silently.

### 4.3 codex.yaml

```yaml
id: codex
display_name: Codex
icon: '<svg …/>'                                  # stays inline; the descriptor is self-contained
protocol_version: { min: "0.140", max: "0.199" }

runtime:
  transport: api                                  # default for every event below
  api:
    protocol: jsonrpc2
    serve:  [codex, app-server, --listen, "unix://{socket}"]
    attach: [codex, --remote, "unix://{socket}"]  # TUI onto the same conversation
    handshake: { call: initialize, send: { clientInfo.name: crowbar } }
  spawn:
    cmd: codex
    forbid_flags: [--json]

session:
  start:  { call: thread/start,  send: { cwd: "{cwd}", model: "{model}" }, map: { session_id: thread.id } }
  resume: { call: thread/resume, send: { threadId: "{session_id}" } }

events:
  session_start:
    in: thread/started
    map: { session_id: thread.id, model: thread.model }

  user_submit:
    out: turn/start
    send: { threadId: "{session_id}", input: "{message}", model: "{model}", effort: "{effort}" }

  turn_stop:
    in: turn/completed
    map: { session_id: threadId, message: turn.lastAgentMessage }

  message_delta:
    in: item/agentMessage/delta
    map: { session_id: threadId, message_id: itemId, index: sequence, text: delta }

  tool_pre:
    in: item/started
    when: { item.type: commandExecution || fileChange || mcpToolCall }
    map: { session_id: threadId, tool_id: item.id, tool_name: item.type, tool_input: item }

  tool_post:
    in: item/completed
    when: { item.type: commandExecution || fileChange || mcpToolCall }
    map: { session_id: threadId, tool_id: item.id, tool_result: item.output }

  permission:
    ask: item/permissions/requestApproval
    timeout_seconds: 270
    map: { prompt_id: "$rpc.id", tool_name: tool, tool_input: params }
    reply:
      allow: '{"decision":"approved"}'
      deny:  '{"decision":"denied","message":{reason_json}}'

  elicitation:
    ask: mcpServer/elicitation/request
    timeout_seconds: 270
    map: { prompt_id: "$rpc.id", message: message, schema: requestedSchema }
    reply:
      accept:  '{"action":"accept","content":{content_json}}'
      decline: '{"action":"decline"}'
      cancel:  '{"action":"cancel"}'

  interrupt: { out: turn/interrupt, send: { threadId: "{session_id}" } }

  compact_start:                                  # Crowbar asks for compaction
    out: thread/compact/start
    send: { threadId: "{session_id}" }

  compact_pre:  { in: turn/started, when: { turn.kind: compact } }
  compact_post:
    in: thread/compacted
    map: { session_id: threadId, turn_id: turnId }

  telemetry:
    in: thread/tokenUsage/updated
    map: { input_tokens: tokenUsage.inputTokens, output_tokens: tokenUsage.outputTokens }

catalog:
  models:   { call: model/list }
  commands: { call: skills/list }
  limits:   { call: account/rateLimits/read }

inject:
  - at: mcp
    call: config/mcpServer/reload
  - at: context
    call: thread/inject_items
    send: { threadId: "{session_id}", items: ["{context}"] }
```

### 4.4 claude.yaml — same schema, different transport

```yaml
runtime:
  transport: hooks
  hooks: { format: json, delivery: http }

events:
  session_start: { in: SessionStart,      map: { session_id: session_id, model: model } }
  user_submit:   { in: UserPromptSubmit,  map: { message: prompt } }
  permission:
    ask: PermissionRequest
    timeout_seconds: 270
    map: { prompt_id: prompt_id, tool_name: tool_name, tool_input: tool_input }
    reply:
      allow: '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'
      deny:  '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":{reason_json}}}}'
  telemetry:     { in: statusline,        map: { input_tokens: cost.total_tokens } }

  compact_pre:   { in: PreCompact,  map: { session_id: session_id, trigger: trigger } }
  compact_post:  { in: PostCompact, map: { session_id: session_id, trigger: trigger } }
  compact_start:                          # no API to trigger it — inject the slash command
    out: prompt
    transport: hooks
    send: { text: "/compact" }
```

A provider may set `transport:` per event, so a mixed provider — API for turns, hooks
for permissions — needs no new concept.

### 4.5 Context compaction — promoted to a Crowbar state

Both providers compact context, both expose it, and Crowbar currently sees only half of
it. `compact_pre`/`compact_post` are already in the vocabulary
(`spec/hooks.go:13-14`), both descriptors map them, and the single consumer
(`usecases/agent/observation.go:177`) records them as an `InterruptEvent` with
`Kind: compaction`.

Three things are wrong with that and all three are fixed here.

**It is inbound-only.** Crowbar cannot ask for compaction. Codex exposes
`thread/compact/start` (params `{threadId}`, empty response); Claude exposes no API, so
the trigger is the `/compact` slash command injected through the prompt transport. The
vocabulary therefore gains one outbound event:

| canonical | direction | codex | claude |
|---|---|---|---|
| `compact_start` | **out** | `thread/compact/start` | inject `/compact` |
| `compact_pre` | in | `turn/started` where `turn.kind: compact` | `PreCompact` hook |
| `compact_post` | in | `thread/compacted` → `{threadId, turnId}` | `PostCompact` hook |

**It is modelled as an interrupt, not a state.** An interrupt is a thing that happened
to a turn. Compaction is a *condition the chat is in* — the CLI is busy, prompts must
queue, and the UI needs to say so. `compact_pre` therefore sets a chat work-state
(`compacting`) that `compact_post` clears, alongside the existing `working` state, and
it is published on the same WS channel.

**It leaves no mark on the ledger.** After compaction the conversation above the
boundary is no longer in the model's context, which is exactly the thing a user needs to
see. `compact_post` writes a ledger marker carrying the trigger (`manual` | `auto`) so
the transcript can render a divider, and so `AssembleHandoff` knows not to replay
pre-boundary turns as if they were live context.

Capability is key-presence, as everywhere else: a provider that declares no
`compact_start` gets no compact control in the UI.

### 4.6 What must change in the interpreter

The interpreter stays and is the right investment: it is what lets Crowbar support
virtually any provider. Four changes make "a new provider needs zero Go" actually true.

1. **The vocabulary becomes data.** `descriptor/vocabulary.yaml` declares Crowbar's
   canonical events and each one's required fields, replacing the `spec.Hook*` constants
   and `rules/hook_vocabulary.go`. The vocabulary is **Crowbar-owned and closed**: a
   provider maps *into* it and cannot extend it. Only a new *capability* needs Go.
2. **One path grammar.** Merge the two resolvers (`payload/walk` and
   `catalog/internal/adapters/selectPath`) into `descriptor/internal/mapping/`.
   Alternation becomes explicit `a || b || c` instead of comma-overloading. `when:`
   becomes a first-class variant selector so provider sum types (Codex's `item`) stop
   being a special case.
3. **`protocol_version`.** `app-server` is flagged experimental; a renamed method must
   fail descriptor validation at load, not at runtime.
4. **Fixtures in the contract.** Each event names recorded provider traffic under
   `protocol/testdata/fixtures/`, replayed in CI. This is the only mechanism that
   catches a provider changing payload shape.

---

## 5. Target tree

```
core/terminal/                     PTY capability — promoted out of engine/, 8 consumers

engine/agents/
  agents.go                        public face: Engine + New
  types.go                         Agent, Runner, Fact, Intent, Decision, Capabilities
  errors.go
  internal/
    protocol/                      speaking one provider's language
      protocol.go                  Load(home, id) → Protocol — the only face
      internal/
        descriptor/                YAML → validated Spec
          descriptor.go
          vocabulary.yaml          Crowbar's canonical events + required fields — CLOSED
          descriptors/*.yaml       claude, codex — transport declared per event
          internal/schema/         validates a descriptor against the vocabulary
          internal/mapping/        the one path grammar: walk, `||`, `when:`
        transport/                 moves bytes, no semantics
          transport.go
          internal/hooks/          HTTP in, response body out
          internal/jsonrpc/        duplex over stdio/unix/ws
          internal/oneshot/        spawn, read stdout, exit
        translate/                 payload ↔ Fact — pure, no IO, no state
          translate.go
          internal/inbound/
          internal/outbound/
          internal/answer/
      testdata/fixtures/           recorded provider traffic, replayed in CI

    runner/                        owns one live CLI and its terminal
      runner.go
      internal/
        inflight/                  turn + answer correlation — non-durable, no asynx
        store/
          store.go                 the EventStore face
          internal/commands/       Spawn, Bind, Displace, Exit, Selection
          internal/projections/    read model + reconcile-on-open

    session/                       spawn, resume, switch, destroy

app/usecases/chat/
  chat.go                          public face: the ports handlers call — wiring only
  types.go
  internal/
    translate/                     engine Facts → asynx commands
    fanout/                        asynx deltas → frontend WS
    tree/                          folders + placement
    tools/                         MCP surface the CLI calls back into

app/repositories/chat/
  chat.go                          one aggregate: thread, title, placement, selection, activity
  internal/commands/
  internal/store/
  internal/content/                tool payload blobs, off the event stream

api/v0/endpoints/chat/
  chat.go                          Register + the 30 routes
  internal/handlers/               one thin file per resource
```

### 5.1 Deleted

`app/usecases/agent` (35 production files) · `app/usecases/agentchatfolder` ·
`app/usecases/agenttools` · `app/repositories/agentchat` ·
`app/repositories/agentactivity` · `app/repositories/agentrunner` ·
`adapter/store/agentjournal` · the five handler port interfaces · and 16 of the 17
packages under `engine/agents/internal` (`answers`, `catalog`, `env`, `exec`, `hooks`,
`models`, `move`, `payload`, `promptorigin`, `registry`, `selection`, `spawn`, `spec`,
`telemetry`, `template`, `termprompt`), folded into `protocol/`.

`probe/` is not created: a slash-catalog or telemetry probe is a third **transport**
(`oneshot`) beside `hooks` and `jsonrpc` — same descriptor entry, same translate path,
different byte-mover. `facts/` is not created either: `Fact` is a public type and
belongs in `types.go`.

---

## 6. Interfaces

### 6.1 `protocol.Protocol`

The runner holds one of these and never learns that descriptor, transport and translate
are separate packages.

```go
type Protocol interface {
    Capabilities() Capabilities
    Recv(wireEvent string, raw []byte) (Fact, error)
    Send(Intent) error
    Reply(correlationID string, Decision) error
}
```

### 6.2 Data flow

```
CLI ──raw──▶ transport ──▶ translate/inbound ──Fact──▶ runner ──▶ usecase/translate ──▶ asynx ──▶ fanout ──▶ WS
CLI ◀──call── transport ◀── translate/outbound ◀─Intent── runner ◀── usecase ◀── handler
```

The engine never names asynx-for-chat and never reaches the frontend. The usecase never
parses a provider payload.

### 6.3 Properties of `translate/`

- **Pure.** `(Spec, []byte) → Fact`. No clock, no network, no state. Every fixture is a
  table test.
- **One table, two directions.** The same `events:` entry serves both; `in:`/`out:`/`ask:`
  selects which half is used.
- **Unmapped means absent.** Key-presence is the capability check — no flags to drift.
- **It decides nothing.** No chat, no asynx, no frontend.

### 6.4 The `inflight` / `store` line

`inflight/` never imports asynx: gates, waits and correlation ids die with the process.
`store/` never holds a channel. Correlating a `ServerRequest` id to the human who
answers it is `inflight/`; recording that the answer happened is `store/` and, for the
user-visible record, the chat aggregate.

---

## 7. Invariants carried forward

These are load-bearing and were each discovered by a production failure. They survive
the move unchanged and every one needs a test that fails when it is violated.

1. **The spawn gate is never taken on the hook-ingest path.** Otherwise
   `SwitchProvider`'s untimed park deadlocks the CLI.
2. **Work-state publishes before turn-complete.** Otherwise a switch reads "no turn, not
   yet known" as idle and kills a CLI doing background work.
3. **The runner points at the chat; the chat never points back.** Liveness is row
   existence.
4. **`TerminateGraceful` is SIGTERM, never SIGKILL** — a well-behaved CLI flushes its
   native transcript on SIGTERM, and an evicted runner's conversation is about to be
   read by its successor.
5. **Hook delivery is exactly-once by delivery id.** The relay mints one id and reuses
   it on every retry; the ingress must collapse retries into one semantic hook. Today
   this lives in `agentjournal`; it moves to `runner/internal/inflight`.
6. **The hook ingress is a declared method, never a runtime type assertion.** A port
   that only *might* carry it is a port a mis-wire drops silently.

---

## 8. Sequencing

Each stage is independently green on the full CI gate — `go vet`, `-race` across all
packages, the integration suite, and the coverage floor — and committed before the next
begins.

| stage | change | why first |
|---|---|---|
| **0** | Lift WS broadcast out of the repository projections into `usecases/chat/internal/fanout`. | §1.5. Mandatory before the runner aggregate moves, or the engine ends up pushing WS deltas. |
| **1** | Promote `engine/terminal` → `core/terminal`. | Mechanical; unblocks the engine owning a PTY without a cross-engine import. |
| **2** | Descriptor v3: event-centric schema, `vocabulary.yaml`, one path grammar, `protocol_version`, fixtures. Both providers migrated. Adds the `compact_start` outbound event (§4.5). | Self-contained, testable against recorded traffic, no consumer changes. |
| **3** | Build `protocol/` from the 16 folded packages. Engine still stateless. | The new `Protocol` face lands before anything depends on it. |
| **4** | Move `repositories/agentrunner` → `engine/agents/internal/runner/internal/store`; `domain.AgentRunner` → `agents.Runner`. | §3.1. Depends on 0 and 1. |
| **5** | Move the in-flight tier (gates, turn registry, work mirror, answer desk, message streams, `agentjournal`) into `runner/internal/inflight`. Engine becomes stateful; `Fact` stream goes live. | The largest single step. Every §7 invariant gets its test here. |
| **6** | Merge `agentchat` + `agentactivity` → `repositories/chat`, payload blobs to `internal/content`. | Independent of 4–5; can run in parallel. |
| **7** | Collapse `usecases/agent` + `agentchatfolder` + `agenttools` → `usecases/chat`. Five handler ports → one. Compaction becomes a chat work-state and a ledger marker (§4.5), with a route to trigger it. | Only possible once the engine holds the machinery. |
| **8** | Rename `endpoints/agent` → `endpoints/chat`; routes become `/workspaces/:wsId/chats/...` and `/settings/chat/providers`. Frontend updated in the same commit. | Pre-production: rename outright, no aliases. |

### 8.1 Codex on the API transport

Stage 2 declares it; a follow-up stage switches Codex from PTY-scraping to
`app-server` + `--remote` TUI attach. It is deliberately **not** bundled with the
restructure — the descriptor change must be provable against fixtures before the
transport swap adds a second variable.

---

## 9. Success criteria

- No package under `engine/` imports another `engine/` package. Only
  `engine/container.go` does.
- `engine/` imports no `adapter/eventstore/*`.
- `usecases/chat` contains no type holding a mutex, and no provider payload parsing.
- A new provider is one YAML file plus fixtures — zero Go.
- Compaction is triggerable from Crowbar on both providers, shows as a chat state while
  it runs, and leaves a ledger marker that `AssembleHandoff` respects.
- Each of the six §7 invariants has a test that fails when the invariant is inverted.
- The full CI gate is green at every stage boundary, coverage at or above the current
  92% floor.

---

## 10. Open questions

1. ~~**Fixture capture.**~~ CLOSED: `scripts/capture-codex-fixtures.sh` drives a real
   turn and writes redacted recordings; the claude half needs the live-daemon hook
   capture, which happens in the final verification.
2. ~~**Nested leaf paths for Codex.**~~ CLOSED — see §4.2. Four were wrong.
3. **Codex on the API transport is a capability REDUCTION, not just a swap.** The
   app-server carries no subagent, compaction or session_end notifications, all of
   which the hook transport provides. The shipped codex descriptor therefore stays on
   hooks; `experimental/codex-api.yaml` holds the verified API mapping. Adopting it
   means either accepting the loss or finding those events elsewhere in the protocol.
4. **Codex permission prompts are observable but not answerable** (`answerable: false`).
   That was true before this work and invisible; it is now declared. Giving codex a real
   answer path needs a verified response template.
3. **`turn/steer`, `thread/fork`, `thread/compact/start`** are Codex capabilities with no
   Claude equivalent and no Crowbar concept. They are out of scope here; the closed
   vocabulary means adding them later is a vocabulary change plus Go, by design.


---

## 11. What changed during implementation

Four places where the code disagreed with this spec and the code was right.

**1. The chat and activity aggregates were NOT merged (§3.2, §5).**
`app/container.go:60` already documents why they are split: *"their write rates differ
by orders of magnitude: a chat emits a handful of events, its activity emits hundreds
per turn, and sharing one single-writer event log would put a sidebar repaint behind a
tool-call storm."* Merging the event logs would reintroduce a solved problem. The
PACKAGE is unified — `repositories/chat/` with `activity/` inside it — and the two
aggregates keep their own asynx instances.

**2. The five handler ports were NOT fused into one (§1.3, stage 7).**
Measured, they are NARROWER than the usecase interfaces they were said to duplicate:
ChatUsecase 7 methods against 12, TurnUsecase 6 against 9, RunnerUsecase 8 against 12.
They are consumer-side interfaces narrowed to what each route uses, which is the Go
idiom. Fusing them would make every handler depend on every capability.

**3. Nine packages did NOT move under `protocol/` (§5).**
catalog, spawn, selection, move, registry, promptorigin, env, exec and template are
neither descriptor-reading nor payload-translating — they are behaviours *driven by* a
descriptor. `protocol/` holds descriptor + translate; forcing the rest in would make
the name mean "everything".

**4. `transport/` was not created.** There is nothing to put in it: the hook wire is
the daemon's HTTP handler and the one-shot probe wire is `internal/exec`. A `jsonrpc`
transport becomes real only when codex adopts app-server, which §8.1 already stages
separately.

### Also settled by implementation

- **Codex's API transport is a capability REDUCTION**, not a swap: app-server carries
  no subagent, compaction or session_end notifications. The shipped codex descriptor
  stays on hooks; the verified API mapping lives in
  `descriptors-v3/experimental/codex-api.yaml`.
- **Codex permission prompts are observable but not answerable** (`answerable: false`),
  which was true before this work and invisible.
- **Four Codex leaf paths in §4.2 were wrong** and are corrected there.
- The runner aggregate moved to `engine/agents/runner`, not
  `engine/agents/internal/runner/internal/store` — the usecase still needs to reach it
  while the engine has no full facade.

### Not done

- **The outbound half of translate.** `out:`/`send:` are parsed and validated but no
  code consumes them yet, so `compact_start` is declared and unreachable. The
  compaction ROUTE therefore does not exist: §4.5's state and ledger marker are
  specified, not built.
- **Stage 5's relocation.** The six invariants have tests (each proven by inverting
  it) but the in-flight tier still lives in `usecases/chat`.
