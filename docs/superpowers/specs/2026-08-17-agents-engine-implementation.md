# The `agents` engine — implementation spec

**Date:** 2026-08-17

**Status:** implementation spec. Implements the storage and engine decisions of
[`2026-08-17-agent-chat-primary-surface-design.md`](./2026-08-17-agent-chat-primary-surface-design.md)
(hereafter *the design spec*) and **corrects its §4.1**.

**Reference codebase:** `quiver.core@develop`. Every structural rule in §3 is taken from there,
with the file that demonstrates it. Where this document departs from quiver, §12 says so and why.

**Scope:** the `engine/agents` decomposition and the `agentactivity` aggregate. Delivery strategy
(design spec §7), lazy start (§3) and the frontend (§10) are unchanged by this document and
proceed against the interface defined in §5.

---

## 1. Correction: asynx stores patches, not state

The design spec's §4.1 argued against putting tool payloads in the aggregate on the grounds that
"`events` is `(aggregate_id, version, data BLOB)` and every version is retained", implying each
event retains a copy of the whole aggregate. **That is wrong.** From
`asynx@v0.8.0/internal/eventstore/models/event.go`:

```go
// InternalEvent is the storage representation of an event.
// Patches holds a JSON-encoded RFC 6902 patch array (old → new state).
// The full aggregate state is never stored — it is reconstructed by
// replaying patches on top of the seed state (or latest snapshot).
```

The corrected mechanics, all verified against the v0.8.0 source:

| Property | Reality | Source |
| --- | --- | --- |
| Event size | O(delta), an RFC-6902 patch | `models.InternalEvent.Patches` |
| Snapshot | one upserted row holding full state; O(state) per write, constant storage | `models.SnapshotBlob` |
| Warm read | latest snapshot + deltas since | `reader.Load` |
| Event pruning | **never**, except `Forget` → `Delete` | `eventstore.go:136` |
| `Replay` | per-event, version order, from v1 always, with `Aggregate` **and** `PreviousAggregate` | `replayer.go:Replay` |

**The conclusion survives; the reasoning changes.** Payloads still must not enter the aggregate,
because a snapshot rewrites the entire state on every snapshotting command and the cold path
materialises all of it. But the fear that the *events table* grows quadratically with turn count
was unfounded, and the three-layer split in the design spec §4.1 stands on the snapshot and
cold-load costs alone.

Two consequences that this document depends on:

1. **An accumulating read model is rebuildable.** `Replay` delivers every event in order with
   both states, so a projection that appends rows can be reconstructed from the log. The existing
   `agentchat` store already enumerates aggregate ids and replays each one
   (`agentchat/internal/store/store.go:rebuild`); only the fold function differs.
2. **Events are never pruned**, so the event log — not the read model — is the durable record.

## 2. The constraint that shapes everything below

An asynx command contributes exactly two things to an event: its `EventName` and the patch
between `PreviousAggregate` and `Aggregate`. There is **no command-payload record**.

> **In asynx, the aggregate is the only payload channel.**

Anything a projection must observe has to pass through aggregate state, if only for one version.
This is not a limitation to work around; it is the contract, and naming it prevents the
recurring mistake of expecting a projection to read a command.

## 3. Decision: chat-level aggregate, open state only

Aggregate id = `chatID`. One stream per chat. State holds **what is currently open, plus the one
item this event just closed**.

```go
// domain/agent_activity.go — pure, no I/O, no internal imports.
type AgentActivity struct {
    ChatID string
    Seq    int64            // monotonic; orders items within the chat

    Open   *OpenTurn        // nil when the chat is idle
    Closed *ClosedItem      // what THIS event closed; overwritten every command
}

type OpenTurn struct {
    ID, Role, ProviderID, RunnerID, SessionID string
    StartedAt time.Time
    Text      string
    Effort    string

    Tools         map[string]OpenToolCall      // in-flight only
    Subagents     map[string]OpenSubagent      // in-flight only
    Interruptions map[string]OpenInterruption  // unresolved only
}
```

`Closed` is deliberately a single slot, not a list. It carries the closed item's full metadata —
status, `durationMs`, `resultRef` — and is overwritten by the next command. State is therefore
**O(concurrent activity + 1)**, independent of conversation length, so snapshot writes and cold
loads stay flat over a chat's whole life. The closed item's data lives forever in the patch that
wrote it, which is what makes the event log the durable record.

### 3.1 Projection contract

The projection is **delta-driven and idempotent**:

- It switches on `evt.EventName` (quiver's pattern —
  `netbridge/internal/projections/port_events.go`), not on inference from state.
- Inserts and updates are **upserts keyed by the item's own id** (turn id, tool-call id). Replay
  re-delivering an already-projected event is therefore a no-op, so rebuild needs no watermark.
- `evt.Aggregate.Closed` is read on close events; `evt.Aggregate.Open` on open events;
  `evt.PreviousAggregate` is used only where the delta is a removal.

**This is a different projection style from the one Crowbar has today.** `agentchat`'s projector
saves `evt.Aggregate` wholesale into a single JSON column (`agentChatRow.Data []byte`). That is
correct for a small aggregate and wrong here — it would rewrite the whole conversation per event,
which is §4.1's problem at the read-model layer. `agentchat` is not changed by this document; the
new projector simply does not copy it.

### 3.2 What this costs

The aggregate can no longer answer "what did this chat say"; only the projection can. That is
accepted: the projection is durable, is rebuilt by replay when lost, and is what every reader —
UI, `assembleConversation`, the MCP tools (design spec §9) — was always going to query.

---

## 4. House rules, adopted from quiver.core

These are not aspirations; §11 gates on them.

| # | Rule | Demonstrated by |
| --- | --- | --- |
| R1 | Package root file holds the doc comment, the interface, the unexported impl struct and the constructor. Nothing else is exported from the root. | `engine/netbridge/netbridge.go` |
| R2 | Everything else lives under `internal/`, recursively — a child with children gets its own `internal/`. | `engine/wizard/internal/runtime/internal/{models,process}` |
| R3 | Public value types live in an internal package and are re-exported at the root as type aliases. | `engine/wizard/wizard.go` `type ( Event = models.Event; … )` |
| R4 | Sentinel errors live in the package's `errors.go`. Never returned bare; always wrapped with context. | `engine/{deptree,netbridge,vault,provider}/errors.go` |
| R5 | `context.Context` is the first parameter of every function that does I/O. Never stored in a struct. | quiver AGENTS.md §8.4 |
| R6 | Constructor returns the interface, never the struct. `New(...) (T, error)`. | `netbridge.newNetbridge` |
| R7 | One asynx command per file under `internal/commands/`. | `repositories/arrow/internal/commands/*.go` |
| R8 | Read model splits into `internal/store/internal/{storage,projections}` — GORM rows and view structs in one, event handlers in the other. | `repositories/arrow/internal/store/internal/` |
| R9 | Engines are independent: no engine imports another. | quiver AGENTS.md §2 |
| R10 | Test doubles are hand-written stubs in `internal/mocks/`. No generation tools. | quiver AGENTS.md §9.4 |
| R11 | Tests named `TestType_Method_Description`; table-driven with `testCases`; `require` for fatal, `assert` for the rest. | quiver AGENTS.md §9.2–9.3 |
| R12 | Guard clauses over nesting; happy path last. | `revive: early-return`, `nestif: 2` |

Crowbar's `.golangci.yml` is **already byte-identical to quiver's** on every setting that matters
(`funlen 100/50`, `gocyclo 15`, `nestif 2`, `revive early-return`, `gofumpt extra-rules`). The
rules above are therefore already the declared standard; they are simply not met by the package
being replaced (§10).

---

## 5. `internal/engine/agents`

### 5.1 Tree

```text
internal/engine/agents/
  agents.go            R1 — package doc, Agents + Agent interfaces, service, New
  aliases.go           R3 — re-exports of internal/models
  errors.go            R4 — every sentinel
  descriptors/         claude.yaml, codex.yaml (embedded)
  internal/
    spec/              parsed-YAML types. PURE: no sibling imports, no I/O.
    models/            engine output types. PURE. Re-exported by aliases.go.
    descriptor/        load, embedded+disk merge, validate
      internal/rules/  one validation rule per file
    template/          Expand, TemplateCtx
    spawn/             spec.InjectStep[] → models.SpawnPlan
      internal/verbs/  one file per injection verb (pass_arg, write_file, …)
    hooks/             canonical event mapping; ownership guard
      internal/payload/ dotted-path extraction, typed leaves
    telemetry/         ContextUsage / RateLimits / SessionCost / ModelIdentity
    exec/              bounded subprocess: budget, buffers, pgroup kill, timeout
    catalog/           slash + model + effort catalogues, over exec
      internal/adapters/  json_inventory_text_detail, json_section, text_section
      internal/normalize/ dedupe, cap, redact, sanitise
    move/              the Decide reducer
    registry/          injected-context echo suppression
    mocks/             hand-written stubs
```

`spec` and `models` are the engine's `domain/`: pure types, no I/O, importable by every sibling,
importing none of them. Every other internal package depends downward only. There are no cycles
by construction, and that is checkable (§11 G4).

### 5.2 The public face

```go
// Package agents maps provider-owned CLI facts into Crowbar-neutral values.
// It owns no chat, no runner and no process lifetime: it renders spawn plans,
// interprets hook and telemetry payloads, and runs bounded capability probes.
package agents

type Agents interface {
    // List enumerates every known agent: the embedded descriptors, overlaid by
    // any on-disk override under homeDir.
    List(ctx context.Context, homeDir string) ([]Agent, error)

    // Get resolves one agent by id. Returns ErrUnknownAgent if none matches.
    Get(ctx context.Context, homeDir, id string) (Agent, error)
}

// Agent is one configured provider CLI. Its ID is the domain's ProviderID.
type Agent interface {
    ID() string
    Display() Display
    Installed() bool

    // Capabilities reports which optional surfaces this agent declares.
    // An absent capability is absent UI, never a disabled control.
    Capabilities() Capabilities

    SpawnPlan(ctx TemplateCtx, baseEnv []string, extra ...InjectStep) (*SpawnPlan, error)
    PromptSteps(resume bool) ([]InjectStep, error)
    DeliveryStrategy() DeliveryStrategy

    // ParseHook decodes, ownership-checks and maps a raw hook payload in one
    // call. Returns ErrForeignConversation (carrying the field that gave it
    // away) when the payload is not this CLI's own conversation.
    ParseHook(canonical string, raw []byte) (CanonicalEvent, error)

    // ParseTelemetry maps a telemetry callback payload onto whichever facts
    // the provider reported. Absent facts stay absent; nothing is derived.
    ParseTelemetry(raw []byte) (Telemetry, error)

    SlashCatalog(ctx context.Context, opts ProbeOptions) (SlashCatalog, error)
    ModelCatalog(ctx context.Context, opts ProbeOptions) (ModelCatalog, error)
    EffortCatalog(ctx context.Context, opts ProbeOptions) (EffortCatalog, error)
}
```

Three deliberate changes from today's surface:

- **`Descriptor` stops being public.** Today it is an exported struct with **19 exported fields**,
  and callers read them directly. Behind `Agent`, the YAML shape is free to change without
  touching a caller.
- **`ParseHook` fuses `ParsePayload` → `OwnsConversation` → `MapHook`.** Those are three separate
  public calls today and the middle one is the guard that stops a provider's internal session
  hijacking a chat. A guard that a caller can forget to call is a guard that will eventually be
  forgotten; fusing makes it unskippable.
- **`ctx` first on every I/O call** (R5). `ResolveDescriptor`/`AllDescriptors` read disk today
  with no context.

### 5.3 Naming

`engine/provider` already exists and is the **Git** provider engine (GitHub/GitLab protected
branches and PR state) — unrelated. No file imports both today, and the new engine must not make
that a trap. The per-vendor handle is therefore `agents.Agent`, not `agents.Provider`.

The design spec §11 warns that "agent" must not propagate into *domain* names. It does not here:
the domain keeps `ProviderID`, and `Agent.ID()` returns exactly that. `Agent` is an engine-local
noun for "one configured provider CLI", and it is the term the product itself now uses.

### 5.4 Migration map

Every current symbol lands somewhere. Nothing is dropped silently.

| Today | Lands in | Public? |
| --- | --- | --- |
| `Descriptor` + 19 fields, `LoadDescriptor`, `Validate` | `internal/descriptor` over `internal/spec` | no — behind `Agent` |
| `ResolveDescriptor`, `AllDescriptors` | `Agents.Get` / `Agents.List` | yes, renamed |
| `ParsePayload`, `OwnsConversation`, `MapHook`, `CanonicalEvent` | `internal/hooks` | fused into `Agent.ParseHook` |
| `extract`, `countAt` | `internal/hooks/internal/payload` | no |
| `BuildSpawnPlan`, `SpawnPlan`, `InjectStep` | `internal/spawn`, `internal/models` | `Agent.SpawnPlan` |
| `Expand`, `TemplateCtx` | `internal/template` | `TemplateCtx` re-exported; `Expand` internal |
| `ProbeSlashCatalog`, `SlashCatalog*`, 6 catalogue sentinels | `internal/catalog` | `Agent.SlashCatalog` |
| `boundedCatalogExecutor`, `outputBudget`, `boundedBuffer`, `isolateProcessGroup`, `killProcessTree` | `internal/exec` | no — **shared by all three catalogues and by telemetry probes** |
| `Decide`, `Decision`, `MoveKind` + 4 consts | `internal/move` | re-exported (pure) |
| `Registry`, `NewRegistry` | `internal/registry` | re-exported |
| `Connected` | `internal/descriptor` | `Agent.Installed()` |
| `PresentationSpec`, `PromptSubmitSpec`, `CatalogPipelineSpec`, `CatalogItemMapping`, `SlashCatalogSpec`, `HookSpec`, `ArgSpec` | `internal/spec` | no |
| — new — | `internal/telemetry` | `Agent.ParseTelemetry` |

The bounded-executor extraction is the load-bearing one. It is private inside `slash_catalog.go`
today, and the design spec commits to reusing it "wholesale" for telemetry probes (§8.1), model
catalogues (§8.2) and effort catalogues (§8.3). Three future callers of a private helper is the
definition of a package boundary in the wrong place.

### 5.5 Wiring

`engine.Container` gains `Agents agents.Agents`, constructed in `engine.New`. Today the package
is imported ad hoc by `usecases/agent`, `usecases/container.go` and the API handlers, which is
the one rule quiver states about engines that Crowbar currently breaks: engines are received
through DI (R9, quiver AGENTS.md §10). The API handler's only use is the `SlashCatalog` return
type, which becomes `agents.SlashCatalog`; it keeps no engine dependency of its own.

---

## 6. `internal/app/repositories/agentactivity`

Domain aggregates live in `app/repositories/`, not in an engine. quiver's precedent is exact:
`netbridge` owns an asynx aggregate *inside* the engine because port allocations are engine-local
resource state, while Arrow, Collection and Runtime — the domain aggregates — live under
`app/repositories/`. Turns and tool calls are domain.

```text
internal/app/repositories/agentactivity/
  agentactivity.go     R1 — EventStore interface + New
  errors.go            R4
  internal/
    commands/          R7 — one per file:
                         start_turn.go      record_text.go
                         invoke_tool.go     complete_tool.go
                         start_subagent.go  stop_subagent.go
                         interrupt.go       resolve_interruption.go
                         end_turn.go        abandon_turn.go
    store/
      internal/storage/     R8 — rows + indexes: turns, tool_calls,
                                 subagents, interruptions
      internal/projections/ R8 — delta-driven handlers (§3.1)
      internal/content/     content-addressed payload store (§7)
    upcasters/         empty at schemaVersion 1; the hook exists so the first
                       shape change is a migration, not a rewrite
    mocks/
```

Event names follow the existing convention `agentactivity.<kind>.<chatID>`, so the hub projection
derives the frame kind by stripping the prefix exactly as `agentchat` does.

### 6.1 What `agentchat` keeps

`agentchat` is unchanged. It keeps title, tree placement, `Working`, `AsyncWork` and
`CurrentTurnStarted` — the facts the sidebar reads on every frame. Conversation content moves to
`agentactivity`. The two aggregates are joined by `chatID` and neither writes to the other, which
preserves the property `agentchat`'s own docs call out: a torn write across two aggregates with no
transaction is what previously bricked a chat.

`LedgerCursor` on `domain.AgentChat` is retired with the ledger.

---

## 7. Content store

Full tool inputs and results, addressed by SHA-256 of the content, under the workspace's chats
directory. Never in the aggregate, never in the projection — the projection holds `requestRef`
and `resultRef` only.

It **inherits the ledger's durability discipline rather than discarding it**: write to a temp
file, fsync, atomically rename, fsync the parent directory. That sequence is the one property of
`app/ledger` worth keeping, and the design spec §5.1 requires it survive the move — an
acknowledged hook must still survive an OS crash.

Content addressing is not incidental. Agents re-read the same files constantly; the same 200 KB
file read forty times stores once. Retention is a policy over the store, not a property of the
event log, so payloads can be swept without touching history.

---

## 8. What leaves

`internal/app/ledger` (498 lines, 3 non-test importers) is deleted, not converted — Crowbar is
pre-production and there is no migration path. Its ~20 read methods have replacements:

| Ledger method | Replacement |
| --- | --- |
| `AppendTurn`, `AppendRunnerTurn`, `AppendSessionTurn`, `AppendDeliveredSessionTurn` | `agentactivity` commands |
| `Turns`, `Page` | projection queries |
| `RenderConversation`, `RenderConversationAfter`, `render` | `assembleConversation` over the projection |
| `LastEntryAt`, `LastTurnAt`, `LastTurnForSession`, `HasTurnAtOrAfter` | indexed projection queries |

`assembleConversation` is the one caller whose behaviour must be preserved exactly: it builds the
handoff document a spawning provider receives on switch, and design spec §5.2 makes that a
survival requirement, not a nice-to-have.

---

## 9. Build order

Each step ends green and shippable. Steps 1–3 are pure refactors with **no behaviour change**,
which is what makes them safe to do first and verifiable by the existing suite.

1. **`internal/spec` + `internal/models` extraction.** Move the YAML shapes and output types into
   pure packages. Root re-exports keep every current caller compiling unchanged.
2. **`internal/exec` extraction.** Lift the bounded runner out of `slash_catalog.go`. Existing
   slash-catalogue tests are the regression gate; they must pass untouched.
3. **Remaining decomposition + the `Agent` interface.** `descriptor`, `hooks`, `spawn`,
   `template`, `catalog`, `move`, `registry`. Callers move to `Agents`/`Agent`. `Descriptor` stops
   being exported. `engine.Container` gains `Agents`.
4. **`agentactivity` aggregate**: domain type, commands, storage rows, delta-driven projections,
   rebuild-by-replay. Written against the empty case; nothing reads it yet.
5. **Content store**, with the fsync/rename discipline ported from the ledger.
6. **Cutover.** Hook ingestion writes `agentactivity`; `assembleConversation` and the chat log
   read the projection; `app/ledger` is deleted.
7. **`internal/telemetry`** and the `telemetry` descriptor section, ingested through the existing
   `crowbar hook` channel (§10 note).
8. **Observation breadth** — the design spec §6 hooks, starting with the interruption events that
   caused the three observed legibility failures.

Steps 1–3 can proceed in parallel with 4. Step 8 cannot start before 6: until the ledger is gone
there is nowhere to put a `PreToolUse`.

**Telemetry needs no new ingress.** `crowbar hook <event> --segment X` already posts arbitrary
JSON over a unix socket and dispatch is `MapHook(kind, payload)` → switch on `ev.Kind`. Claude's
`statusLine` is the same channel shape. `hook telemetry` is a switch case and a descriptor block,
not a new route and not a new subcommand. Two things to check when step 7 starts: whether the
descriptor's `require_payload_fields` ownership guard passes on a `statusLine` payload, and
whether injecting our own `statusLine` via `--settings` overrides a user who already has one.

---

## 10. Starting condition

Measured 2026-08-17 with `golangci-lint run --build-tags noEmbed` on
`./internal/app/usecases/agent/... ./internal/engine/agent/...`:

**37 issues** — goimports 9, nestif 9, gocyclo 7, gosec 4, errcheck 2, gofumpt 2, staticcheck 2,
exhaustive 1.

The gocyclo findings, against a limit of 15:

| Function | Complexity |
| --- | --- |
| `(*Usecase).SubmitPrompt` | **50** |
| `(*Descriptor).validateSlashCatalog` | **39** |
| `(*Usecase).spawnRunner` | 22 |
| `probeJSONInventoryDetails` | 22 |
| `(*Usecase).SlashCatalog` | 20 |
| `(*Usecase).switchProviderLocked` | 18 |
| `(*promptJournal).begin` | 17 |

Two of the worst are in the engine being replaced, and `validateSlashCatalog` at 39 is precisely
what R1/R2 and a rule-per-file `internal/rules` package exist to prevent.

`staticcheck` also reports a dead branch in `SubmitPrompt` — `SA4006` on `existingAttempt`
followed by `SA9003` empty branch at `prompts.go:210`. That is a real hole in the durable request
journal's replay path, sitting inside the 50-complexity function, and it is the concrete argument
for the design spec's build-order item 1: extract the delivery seam before adding a second
strategy to it.

---

## 11. Acceptance gates

| # | Gate | How it is checked |
| --- | --- | --- |
| G1 | `engine/agents` and `agentactivity` produce **zero** golangci-lint issues | `golangci-lint run --build-tags noEmbed ./internal/engine/agents/... ./internal/app/repositories/agentactivity/...` |
| G2 | ≥95% unit coverage per new package (quiver AGENTS.md §9.6) | `go test -cover` per package |
| G3 | Nothing outside `engine/agents` imports below its root | `go list -deps` assertion test |
| G4 | No import cycles; `spec` and `models` import no sibling | compile-time + an architecture test |
| G5 | Aggregate state size is independent of conversation length | test: 500 turns × 6 tool calls, assert marshalled state stays bounded |
| G6 | Projection rebuild by replay reproduces the read model exactly | test: build model live, drop it, `Replay`, assert row-for-row equality |
| G7 | Projection is idempotent | test: replay twice, assert identical rows |
| G8 | Content store survives simulated crash between write and rename | test over a fault-injecting fs |
| G9 | `assembleConversation` output is byte-identical pre- and post-cutover | golden test captured **before** step 6 |
| G10 | Every regression test from the three fixed production bugs still passes | existing `TestRegression_*` |

G9 is the one that catches a silent handoff regression, and it must be captured before the
ledger is deleted — after is too late.

---

## 12. Deliberate departures from quiver.core

| quiver does | Here | Why |
| --- | --- | --- |
| Some engines are flat (`vault`, `manifold`, `deptree`, `provider`), with public sub-packages | `agents` hides everything under `internal/` | Explicit instruction, and the stricter form. Today's package leaks 19 types, 19 `Descriptor` fields and 9 functions; the leak is the problem being fixed. |
| `netbridge` owns an asynx aggregate inside the engine | `agentactivity` lives in `app/repositories/` | quiver's own split: engine-local resource state inside the engine, domain aggregates in `app/`. Turns are domain. |
| `ShouldSnapshot()` returns `true` unconditionally | Same | Correct given §1: a snapshot is one upserted row and state here is bounded by §3. |
| Store projection saves whole `evt.Aggregate` | Delta-driven, upsert-by-id | §3.1. Whole-state saves are right for small aggregates and wrong for an accumulating read model. |
| `internal/upcasters` carries real migrations | Present but empty | Pre-production, schemaVersion 1. The package exists so the first shape change is a migration rather than a rewrite. |

## 13. Evidence record

| Fact | Method | Date |
| --- | --- | --- |
| asynx events store RFC-6902 patches, not full state | read `asynx@v0.8.0/internal/eventstore/models/event.go` | 2026-08-17 |
| Events are never pruned except by `Forget`/`Delete` | read `eventstore.go:134-143` | 2026-08-17 |
| `Replay` is per-event, from v1, with `PreviousAggregate` | read `replayer.go:Replay` | 2026-08-17 |
| Crowbar's `.golangci.yml` already matches quiver's on funlen/gocyclo/nestif/revive/gofumpt | diffed both files | 2026-08-17 |
| 37 lint issues; `SubmitPrompt` gocyclo 50; `validateSlashCatalog` 39 | `golangci-lint run` on both packages | 2026-08-17 |
| `engine/agent` exports 19 types, 9 functions, 10 methods, 7 sentinels; `Descriptor` exposes 19 fields | `go doc -all` | 2026-08-17 |
| `engine/provider` is the **Git** provider engine; no file imports both | `go doc` + import scan | 2026-08-17 |
| `engine/agent` is not in `engine.Container`; it is imported ad hoc by 5 files | read `engine/container.go` + import scan | 2026-08-17 |

**Untested:** every claim about the *new* code, which does not exist yet. G1–G10 are how they
become evidence.
