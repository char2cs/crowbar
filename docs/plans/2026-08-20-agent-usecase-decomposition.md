# Agent usecase decomposition

Status: **DRAFT 3 — architecture, reviewed twice.** Draft 1 was reviewed by three
independent passes (quiver.core fidelity, Go feasibility, behavioural risk); its
target architecture did not survive. Every claim below was verified against the
source before being written down.

---

## 1. What is actually wrong

Not "the folder is big". Measured, `usecases/agent` is the one package in this
repository that departed from conventions the rest already keeps.

| convention | rest of the repo | `usecases/agent` |
| --- | --- | --- |
| usecase exposed as an interface | 11 sibling packages export `Usecase` | **the only concrete pointer** in `usecases.Container` |
| usecase touches the filesystem | never | **8 files**; 23 fs calls in `prompt_journal.go`, 7 in `hook_delivery.go` |
| usecase imports `api/v0/dto` | **zero** packages | **3 files** — and `container.go:307` states the rule out loud |

Plus the size, which is the symptom rather than the disease:

| metric | value |
| --- | --- |
| production files / lines | 29 / 6,255 (+1,682 in `internal/termwait`) |
| test lines | 10,913 |
| `Usecase` struct fields | 26 |
| `*Usecase` methods | 46 exported, 87 unexported |
| other types in the package | 32 — **all with zero references to `Usecase`** |

That last row is the good news: every machinery type is already decoupled from
`Usecase`. Nothing has to be redesigned to break a back-reference.

The bad news is finding B1 below: a `grep '*Usecase'` measures the wrong thing.

---

## 2. The reference, as measured (not as assumed)

`quiver.core`, read directly. Corrections to widely-held assumptions marked ⚠.

- **One usecase per aggregate.** `ArrowUsecase` 10 methods, `CollectionUsecase`
  7, `RuntimeUsecase` 6, `DiscoveryUsecase` 4, `ConfigUsecase` 2,
  `SearchUsecase` **1**. A one-method usecase is fine; a method count is not a
  target, the aggregate boundary is.
- **Usecases hold ports only.** `runtimeUsecase` = three repository interfaces.
- **Usecases never touch the filesystem.** Zero of them import `os` or
  `path/filepath`. Durable writes live in `adapter/`,
  `repositories/*/internal/store/`, or `engine/vault` — the atomic
  temp→fsync→rename writer is `engine/vault/manifest.go:196-219`, unexported.
- ⚠ **No `internal/` tier under usecases.** `app/usecases/` has exactly **one**
  subdirectory and it is `mocks`. Coordination state lives in the struct that
  owns the resource: `engine/vault/store.go:31-34` (a lazy keyed-mutex map — our
  `chatGate`), `repositories/runtime/runtime.go:131-133`, and
  `usecases/discovery.go:93-99`, which holds a keyed in-flight registry with
  cancel, sweep and TTL *in the usecase struct*, in one 275-line file.
- ⚠ **No embedded interfaces.** Zero, anywhere. When several usecases must be
  presented together it uses **named fields** (`usecases/container.go:14-23`);
  a route group needing two receives two parameters (`api/v0/routes.go:18`).
- ⚠ **Not one type per file.** `usecases/discovery.go` holds 7 types,
  `config.go` 5. The real rule is one *concern* per file, split by role
  (`search.go` orchestration + `ranking.go` pure scoring).
- **Cross-component dependencies are consumer-declared narrow interfaces.**
  `usecases/collection.go:20-30` declares a 2-method `arrowCache` over the
  24-method repository it is handed. Named `Fn` types in `deps.go` are a
  constructor-readability device; the closures are always supplied by the
  **composition root** over a *different* component
  (`repositories/container.go:109-139`), never sibling-to-sibling.

This repo already has the pattern, and draft 1 walked past it:
`agent/internal/termwait/termwait.go:20-88` declares **nine** one- and two-method
consumer-side interfaces plus a `Deps` struct.

### The one conflict, flagged rather than silently resolved

The `go-style` skill says **one type per file, named after the type**.
quiver.core does not practice it (7 types in `discovery.go`); neither does this
repo's best-structured code (`termwait.go` holds 18). And 18 of our 29
production files hold *zero* types — they are verb-files.

**Adopted:** rule 4 is scoped to **data types**; behaviour files are exempt, and
a type keeps its tightly-bound satellites (`chatGate` with `gateEntry`,
`messageStreams` with `messageBuffer`). Say so if you want it read literally
instead.

---

## 3. Behavioural findings that constrain the design

These came out of the risk review and were verified. They are the reason the
architecture below is shaped the way it is.

**B1 — "Usecase coupling: 0" measures the wrong thing.** Every machinery type
has zero `*Usecase` references and every one of them has a *caller-side*
contract that no grep sees:

| type | the real contract |
| --- | --- |
| `chatGate` | non-reentrant, and **must never be taken on the hook path** (`gate.go:23-25`) |
| `turnWaits` | a publish **order**: `turn.go:180` is `defer turns.complete`, `:192` is `work.set` — the defer runs LAST, so `work.set` publishes first |
| `messageStreams` | an **external** lock makes its escaped pointers safe |
| `promptJournal` | one **process-global** mutex, held across fsync, taken on the hook path |
| `pendingRunnerHooks` | `finish` runs under `spawns` and calls back into hook ingest |

A census must record, per type: every caller, every lock held at each call site,
and every ordering dependency. A file-level coupling count is not a census.

**B2 — there are THREE `chatGate` instances, not two.** `spawns` (chat-keyed),
`turnStarts` (chat-keyed), and `hookDeliveryJournal.gates` (**runner**-keyed,
`hook_delivery.go:46`). Three instances, two key spaces, never merged.

**B3 — a real data race, in shipped code.** `messageStreams.observe` returns
`*messageBuffer` after releasing the mutex (`message_stream.go:101`). Seven
sites in `message_record.go` then read `buffer.Text()` — which ranges
`b.chunks` — and `buffer.recordedText` with **no lock**, while `observe` and
`markRecorded` write them under it. Two concurrent `message_delta` hooks for one
chat is a concurrent map read/write: a fatal throw `safego.Recover` cannot catch.
It does not fire today only because every hook carries a `delivery_id` and is
serialised per runner by `hookDeliveryJournal.gates` — a lock in a different
file. **Fix this before moving anything**: return snapshots, not pointers.

**B4 — an unbounded map.** `hookDeliveryJournal.completed` (`hook_delivery.go:41`)
is written at `:106` and never deleted — one entry per hook delivery for the
life of the daemon, at ~13 hooks/turn.

**B5 — the staging gate compiles away every invariant.** All **37** files in
`api/tests/` are `//go:build integration`. `go test -tags noEmbed ./...` does not
compile a single one. Integration must run after **every** stage.

**B6 — `agenttools.Deps` is held by value and `ToolAccess == nil` fails OPEN**
(`toolset.go:77-80`): a lost assignment silently re-enables tools for a provider
the user switched off. Hold `*Deps`, assemble once.

**B7 — locked-inner-body pairs.** `ResumeChat` calls `switchProviderLocked`, not
`SwitchProvider`; `discardSpawnedChat` calls `purgeChatLocked`, not `PurgeChat`
— because `chatGate` is non-reentrant. Wiring either to the exported method
compiles and deadlocks the goroutine on its own gate **forever**.

**B8 — `StartTerminalWaitSweep` assigns two callbacks** (`promptSettled`,
`messageDelta`) that belong to different concerns. Any composition where one is
silently left nil kills **live message streaming to the chat**, and *nothing in
the repo tests it* — the invariant test asserts via the REST ledger, and both
unit tests pass nil.

**B9 — `hookDeliveryContextKey` has four consumers across three would-be
destinations.** If it is ever duplicated rather than shared, `turnID(ctx)` falls
back to a random UUID; the `rowKey(chatID, turnID)` upsert that provides
at-most-once then stops deduplicating, producing **duplicate user turns and
duplicate assistant messages** in production. Existing tests cannot see it.

**B10 — `onRunnerExit`** is a closure captured at `spawn.go:298` and held by the
**terminal engine** for the whole life of the PTY, invoked on the engine's
goroutine with `context.Background()`. It reaches five different concerns'
state.

---

## 4. Target architecture

Draft 2 proposed five sibling **packages**. A second review killed it, correctly,
and the decisive objection is a compile error rather than a matter of taste.

### 4.0 Why not sibling packages — three verified blockers

1. **It does not compile.** `usecases/agent/internal/termwait` is imported by
   eight files in `usecases/agent/`, and `prompt_settle.go` returns
   `termwait.Delivery`. Go's internal rule makes that package importable only
   from within `app/usecases/agent/…`. A *sibling* `app/usecases/agentrunner` is
   not in that tree. The split is impossible without also relocating `termwait`,
   which draft 2 explicitly marked "unchanged".
2. **It is not what the reference does.** All of quiver.core's usecase files are
   `package usecases` — six usecase **types in one package**, wired by named
   fields in `container.go:14-23`. Draft 2 claimed the package split "matches
   quiver.core". It does not.
3. **This repo already has the right precedent, and draft 2 missed it.**
   `usecases/project/` exports **three** usecase interfaces — `Usecase`,
   `DeleteUsecase`, `ImportUsecase` — from one package, as three container
   fields (`usecases/container.go:44-46`).

Sibling packages also manufactured problems that simply do not exist inside one
package: ~10 cross-package seams instead of draft 2's claimed 5, homeless shared
types (`WorkspaceReader`, `TerminalCommander`, `ChatLineage`, `PendingAnswer`,
`HookAnswer`, `ChatActivity`), a `chatGate` that must be shared across a
boundary, and a duplication hazard on `hookDeliveryContextKey` whose consequence
is duplicate user turns in production (B9).

### 4.1 The shape: one package, five usecase types

```
usecases/agent/                       package agent
  agent.go        New() — builds shared state ONCE, returns the five
  chat.go         chatUsecase      + ChatUsecase interface
  runner.go       runnerUsecase    + RunnerUsecase   (spawn/switch/resume/prompt)
  turn.go         turnUsecase      + TurnUsecase     (turns, tools, messages, observations)
  answer.go       answerUsecase    + AnswerUsecase
  provider.go     providerUsecase  + ProviderUsecase
  …role-split behaviour files, coordination state as files
  internal/termwait/                  unchanged, still reachable
```

Five fields in `usecases.Container`, replacing the single `Agent *agent.Usecase`
that is today the only concrete pointer in that struct. Each is an interface, so
§1's first violation is fixed.

Cross-type calls are ordinary intra-package method calls. No `deps.go`, no
narrow-interface ceremony, no register of instances that must not be duplicated
— they cannot be duplicated, because `New` constructs the shared state once and
hands the same values to each type. Draft 2's §4.3, §4.4 and §4.5 are deleted
wholesale as artifacts of the wrong boundary.

**`answerUsecase` is its own type** (draft 2 folded it into `agentturn`): it
blocks a relay up to 15 minutes (`answers.go:21,280-307`), keys on delivery and
choice rather than the activity aggregate, and is released on *runner* exit
(`runners.go:147`).

**There is no `agentread`.** Draft 2's "query-only" package was a dumping ground
and was not even query-only: `chatlog.go:17-37` `recordTurn` is a **write**, and
`chatlog.go:39-44` needs `hookDeliveryContextKey`. Reads live on the type that
owns the aggregate they read, which is what quiver.core does.

### 4.2 Durability: move the IO down, leave the lock where it is

Draft 2 sent both journals to `app/repositories/agentdelivery`. Corrected on
three counts:

- **`hookDeliveryJournal.gates` must not go with it.** The gate is held across
  `ingestHookNow` (`hook_delivery.go:195-215`) — the entire hook machine, not a
  store critical section. A repository exporting `Lock(runnerID)` that the
  usecase holds across orchestration is a worse edge than the one it removes.
- **A repository cannot resolve its own directory.** `dir` comes from
  `u.ws.AgentChatsDir(...)`, and `repositories/container.go:61-79` documents
  that this reader does not exist until `repositories.New` returns.
- **The cited precedent is `adapter/store/`, not `app/repositories/`.** Every
  `app/repositories/*` package is an asynx event-sourced aggregate with
  `internal/commands/`. A flat-file JSON journal is not that shape.

So: `adapter/store/agentjournal/`, two types, each taking its directory **per
call**, exposed as interfaces. The one shared temp→fsync→rename writer lives
there, unexported — that is the deduplication of §C3.1. `gates` stays in the
usecase. This fixes §1's filesystem violation without inventing a construction
cycle.

### 4.3 The DTO violation

`SubmitPrompt`, the prompt record, `ResolveProviders` and
`ReplaceProviderPreferences` all return `api/v0/dto` types
(`prompt_journal.go:44`, `providers.go:28,80`). All four return domain types
instead; the DTO mapping moves into the handlers, where every other endpoint in
the repo already does it. This must be done for **providers too**, not just the
prompt record — otherwise the provider extraction creates a new violation of the
very law being fixed.

---

## 5. Staging

| stage | content | risk |
| --- | --- | --- |
| **S0** | live defects + dead code (§6, already in progress) | low |
| **S1** | `adapter/store/agentjournal`: both journals' IO moves down, one shared writer, sentinels move with them and are re-exported by alias to preserve `errors.Is` identity. Gates stay put. | medium |
| **S2** | DTOs out of the usecase — prompt record, `SubmitPrompt`, `ResolveProviders`, `ReplaceProviderPreferences`; mapping moves to handlers. | medium |
| **S3** | Split `Usecase` into the five types behind five interfaces; five container fields; `handlers.AgentUsecase` splits per concern. | **high** |
| **S4** | Style: rule 1 (79 funcs), rule 4 scoped to data types, package doc comments. | low |

S0–S2 land all three objective violations of §1 and are safe to stop at. S3 is
the only stage with real behavioural risk.

**Gate after every stage** — matching CI (`ci.yml:103-106`), which draft 2's gate
did not:

```
go build -tags noEmbed ./...
go test  -tags noEmbed -race ./...
go test  -tags "integration noEmbed" ./tests ./internal/api/...
golangci-lint run
```
plus the ≥92% coverage step. The `./internal/api/...` half matters: it holds the
route audit, which is exactly what an `handlers.AgentUsecase` split can break.

`tests/integration/agent` — the only suite that spawns real claude/codex — is
never run in CI and must be run by hand after S3.

### Two hazards to carry into S3

- `handlers/hooks.go:29-38` discovers the journalled ingress by a **runtime type
  assertion** (`, ok` form). If it ends up pointed at a type that does not carry
  `IngestHookDelivery`, it fails silently, every hook takes the un-journalled
  path, and every relay retry applies its effects twice. Nothing in CI catches
  it. Pin it with an explicit compile-time assertion.
- B7's locked-inner-body pairs (`switchProviderLocked`, `purgeChatLocked`) become
  calls between two types. Wiring either to the exported sibling compiles and
  deadlocks forever. Keep the `…Locked` naming and document the held gate.

### Invariants

`api/tests/` pins them and **no test assertion may change**. Relocation and
package-clause edits are expected; a changed *expectation* is the red flag.

---

## 6. S0 — live defects (in progress)

Independent of the architecture; worth doing whatever happens to the rest. A
review of the first S0 draft found four of its five parts wrong; this is the
corrected version.

### S0.1 — the `messageBuffer` data race — RUNNING

**Corrected cause.** The first draft said the race is masked by the per-runner
delivery gate. That covers hook-vs-hook only. `AbandonMessage` and
`UnfinishedSince` are called from the **termwait sweep goroutine**
(`internal/termwait/run.go:11` ticker → `evaluate.go:161,178`), which takes no
gate. It reads `buffer.Text()` — ranging `b.chunks` — at `message_record.go:192`
while a hook goroutine writes `buffer.chunks[index]` under `s.mu`. **Live,
unmasked `concurrent map read and map write`.**

Fix: `observe`/`openMessages`/`unfinished` return a value snapshot computed
inside the lock. Field census confirms `{ID, TurnID, Text, RecordedText, Final,
Complete, LastAt}` is exactly sufficient.

Also fixes a second live bug: `message_record.go:46` and `:47` call
`buffer.Text()` **separately**, so a buffer that grows between them stores a
`recordedText` newer than what was persisted — `closeAssistantTurn:103` then
matches, skips re-recording, and **permanently loses the tail of the message**.

Also: `UnfinishedSince` keeps the **newest** `LastAt` in a local named `oldest`
(`message_record.go:171-177`). Behaviour is right; the name inverts it on a path
that destroys a turn. Rename only.

Race test drives `AbandonMessage` against concurrent `observe`, under `-race`.
Not without `-race`: a concurrent map read/write is a fatal throw that kills the
whole test binary.

### S0.2 + S0.4-2 — bound the delivery journal — ONE WORKER

Worse than first stated: the directory is **per runner**
(`hook_delivery.go:192`), so pruning records still leaves unbounded empty
`.hook-deliveries/<runnerID>/` dirs. Nothing in the repo touches that tree — not
`PurgeChat`, not the workspace-delete cascade, not `reapCrashOrphanRunnerTmp`.

**The first draft's pruning instruction was backwards.** `prunePromptRecords`
keeps in-flight states and deletes terminal ones. Deliveries need the opposite:
`completed` is what provides deduplication and must be **kept**. Corrected:
prune `completed` only, by age (30d — `cmd/crowbar/hook_spool.go` has *no*
expiry, so a hook spooled during an outage returns with its original id), never
`pending`; reap the per-runner directory on the same rule; amortise the scan;
FIFO-evict the in-memory map only for entries whose disk record says
`completed`; and move `j.completed[…] = hash` to **after** the durable write
(`hook_delivery.go:106` currently precedes `:116`).

Eviction and the replay test ship together — eviction is what opens the window
the test guards. The test needs a fault seam that does not exist: add
`syncDir func(string) error` to `hookDeliveryJournal`, thread it through
`writeHookDelivery` (hard-wired at `:163`), expose `SetHookDeliveryDirSync`,
mirroring `promptJournal`. Assert `Seq`/ordering, not row count — the upsert
guarantees one row, but a replay **relocates the message to the end of the log**.

### S0.3 — delete `ListProviders` — RUNNING

Zero callers repo-wide including `web/`; in no declared interface.

### S0.4-1 — WS delta fan-out — needs BOTH halves

A unit test wiring its own callback cannot catch B8 — it would pass on a build
where `internal/app/container.go:446-448` was deleted. Unit half in
`message_record_test.go` (`fakeCommander` has no `Screen`, so the detector is nil
and no goroutine starts — deterministic); integration half in
`tests/regression_agent_message_stream_test.go`, which already has the
`streamstub` descriptor and `h.dial()`.

### S0.5 — replaced; the identity test is vacuous today

Every register entry is a field of one struct in one composite literal
(`usecase.go:167-191`); the assertion reduces to `u.spawns == u.spawns`. Under
draft 3 it never becomes meaningful — one package, one `New`, one construction.
**Deleted, along with draft 2's register.**

Written instead: a reflection test asserting `New` leaves no constructor-owned
field nil (catches a dropped initialiser during S1–S3), plus
`u.tools.Chats == u`, `u.tools.ChatLogs == u`, `u.tools.ToolAccess != nil` —
which closes **B6**, where `toolset.go:77-80` treats nil `ToolAccess` as *allow
all*, silently re-enabling tools for a provider the user switched off.

Also fold in: `u.messageDelta`/`u.promptSettled` are written at
`terminal_wait.go:24-25` and read from hook goroutines unsynchronised. Benign
only because `startTerminalWaitSweep` runs inside `app.New` before serving — an
edge nothing records. Pass them to `New()`.

### Order

`chatGate` is refcounted and deletes at `refs == 0` (`gate.go:62-71`) — not a
third unbounded map; that worry is dropped.

1. **S0.1** ∥ **S0.3** — disjoint, both running.
2. **S0.4-1** after S0.1 (shares `message_record.go`).
3. **S0.2 + S0.4-2** — one worker; shared files and semantically coupled.
4. **S0.5** last.

---

## 7. S0 outcome

| item | result |
| --- | --- |
| S0.1 `messageBuffer` race | **done** — value snapshots computed under the lock; `*messageBuffer` no longer leaves its file |
| S0.1b lost message tail | **done** — one frozen `Text` for both the persist and the `markRecorded` |
| S0.1c `UnfinishedSince` naming | **done** — `oldest` → `newest`; behaviour unchanged |
| S0.2 delivery journal bounded | **done** — FIFO in memory, record prune + runner-dir reap on disk, amortised |
| S0.2b marker ordering | **done** — `completed[id]` now set only after a successful durable write |
| S0.3 delete `ListProviders` | **done** — 62 lines |
| S0.4-1 WS delta fan-out | **done** — unit + integration halves |
| S0.4-2 replay after failed completion | **done** — asserts Seq/ordering, not row count |
| S0.5 constructor wiring guard | in progress |

### Two CI failures found and fixed, unrelated to the refactor

The branch was **red** and one failure was masking the other.

1. `internal/api/v0` did not compile under `-tags integration`: the `fileProbe`
   double was missing `PushAgentChatMessageDelta` and
   `PushAgentChatPromptSettled`, both added to `hub.Subscriber` by `46c6f923`.
   The gate run at that time covered `./tests` but not `./internal/api/...`.
2. With the package compiling again, `TestRouteAudit_AllSpecRoutesRegistered`
   surfaced **22 registered agent routes absent from the spec** — 11 shapes ×
   workspace and home scopes, all added by this branch. The audit exists to catch
   exactly that drift and had been unrunnable behind the build error.

This is B5 in the concrete: the invariants live behind `//go:build integration`,
so a green `go test -tags noEmbed` proves much less than it appears to.

### Notes for later stages

- Two of my specs were wrong and the workers corrected them, both verified:
  an fsync fault does **not** leave a `pending` record (`os.Rename` precedes
  `syncDir`, so it lands fully renamed as `completed`); and `closeAssistantTurn`
  never calls `markRecorded`, so the staleness concern there did not exist.
- Every new test was proven non-vacuous by breaking the mechanism, capturing the
  failure, and restoring with a sha256 check. The delivery-journal work carried
  six such mutations; mutation D showed the real damage of a replay — Seq 1→3,
  the message relocated to the end of the log, with the row count unchanged
  because the `rowKey` upsert holds.
- **CI runs no `golangci-lint` step for Go.** The 52 findings in this package are
  a local standard, not a merge gate. 20 are new from the comment strip and are
  cleaned before commit; `gocyclo`/`nestif` (4) are pre-existing and out of S0.
