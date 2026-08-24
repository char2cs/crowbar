# The chat usecase — decomposition

**Date:** 2026-08-24

**Status:** design spec, awaiting approval. Nothing implemented.

**Scope:** `api/internal/app/usecases/chat` only. The engine is done and is not touched.

---

## 1. What is wrong

`usecases/chat` has **37 production files, 6,616 lines, flat at the top level**, and two
public subpackages (`tools/`, `tree/`) sitting beside the package's own face. A reader
opening the directory sees `spawn.go`, `turns.go`, `gate.go` and `observation.go` at the
same depth as the package's public API, with nothing saying which is which.

Three specific violations of the structure rule:

1. **The top level is not the public face.** It is 37 files of ports, orchestration and
   machinery mixed together.
2. **`tools/` and `tree/` are public.** They were moved *into* the directory but not
   *under* `internal/`, which misses the point: anything that is not the package's face
   belongs under `internal/`.
3. **`usecases/chatlineage` is a separate top-level usecase** with exactly two
   consumers, both of them chat. It answers "what chats is this a thread of" — a chat
   question, in a chat tree, living outside the chat package. (It predates this work,
   which is not a defence for leaving it.)

---

## 2. The rule this restores

Every package is:

```
<name>.go        the package's public face
types.go         shared types, when there are any
errors.go        sentinels, when there are any
*_test.go        one per production file
internal/        everything else, each submodule the same shape, recursively
```

This is the shape for a package with internals. The FACE of this feature ends up
one step further along — its ports and sentinels sit in the responsibility files
rather than in a shared `types.go` and `errors.go` — because a 500-line ceiling was
added later and grouping by technical kind was the wrong way to meet it. §9.3.

And one addition, because it is what makes the decomposition real rather than
cosmetic:

> **No `internal/` package imports a sibling `internal/` package.**

Components are leaves. Anything that needs two of them is orchestration, and
orchestration lives in the public face. This is checkable, so §6 adds a guard test for
it — the same shape as the engine's cross-import guard.

---

## 3. The orchestrator

One type, `Usecase`, whose methods are split across five files by responsibility. Not
five types: **one type, five sites of method exposure.**

```
usecases/chat/
  chat.go        the type, New, and the chat-record methods
  turn.go        the turn and activity methods
  runner.go      the CLI lifecycle methods
  answers.go     the answer-channel methods
  providers.go   the provider-table methods
  aliases.go     the door: what the COMPOSITION ROOT builds itself
```

Each responsibility file opens with the port it satisfies and closes over the
sentinels its own methods return; see §9.3 for why that replaced a shared
`types.go` and `errors.go`.

Each with its own `_test.go`, named for the source it covers: `chat_test.go`,
`turn_test.go`, `runner_test.go`, `answers_test.go`, `providers_test.go`,
`aliases_test.go`. A test file sits beside its source and takes its name; a test
named for a behaviour instead leaves the reader hunting for what covers what.

### 3.1 Method placement

**`chat.go`** — the type, `New`, and the chat record (13):
`MintChat` · `RenameChat` · `RenameByRunner` · `PurgeChat` · `ListChats` ·
`ListChatsByWorkspace` · `GetChat` · `SetChatSelection` · `ReadChatLog` ·
`ReadMessages` · `NoteThreadLineage` · `AssembleHandoff` · **`Ancestors`**

**`turn.go`** — what the agent did, and what it is blocked on (9):
`IngestHook` · `IngestHookDelivery` · `ReadActivity` · `ReadPendingChoices` ·
`ReadToolPayload` · `Telemetry` · `OpenWork` · `MatchTerminalPrompt` ·
`MatchTerminalNotice`

**`runner.go`** — the vendor CLI (13):
`SpawnChat` · `StartRunner` · `ResumeChat` · `StopChat` · `SwitchProvider` ·
`SubmitPrompt` · `SlashCatalog` · `LiveRunnerForChat` · `ConversationsForChat` ·
`ReconcileRunnersOnBoot` · `Compact` · `TerminalWait` · `StartTerminalWaitSweep`

**`answers.go`** — the human in the loop (5):
`PendingAnswer` · `AwaitAnswer` · `AbandonAnswer` · `AnswerChoice` ·
`AnswerableChoiceIDs`

**`providers.go`** — the global provider table (3):
`ResolveProviders` · `ReplaceProviderPreferences` · `DispatchMCP`

43 methods. That is the chat surface; splitting the type would not make it smaller,
only harder to sequence.

### 3.2 What the handlers take

The five interfaces stay declared and `*Usecase` satisfies all five. They ended up
not in a shared `types.go` but each at the head of its own responsibility file —
see §9.3.
A handler still takes only the port it uses, so no route gains reach it did not have.
The change is that one value now backs all of them, which also collapses the
container's five fields to one.

### 3.3 Why one type dissolves the hard problem

The five concern types call each other today: `chat↔runner`, `turn↔chat`,
`turn↔runner`, `runner→provider` — a cycle that only compiles because they share a
package. Splitting them into packages would need ~20 interfaces to break.

With one orchestrator there is nothing to break. Those calls become ordinary method
calls on the same receiver, and the components underneath never learn about each other.

---

## 4. The components

```
usecases/chat/internal/
  ports/          TerminalCommander, WorkspaceReader — the outbound seams
  inflight/       the spawn gate, the turn registry, the work mirror, pending hooks
    internal/gate/ internal/turnstate/ internal/pending/
  stream/         per-chat message assembly
  answerdesk/     the answer desk: relays parked on a human decision
  telemetry/      the per-chat telemetry store
  catalog/        the slash-catalog run limiter
  conversation/   chat-record reads and writes: title, selection, chatlog, handoff
  turn/           hook payload -> turn and activity records
  runner/         PTY lifecycle: spawn, switch, resume, stop, prompt delivery
  provider/       the provider table and MCP dispatch
  tree/           folders, placement, cascading delete
    internal/lineage/   the chat-ancestry walk
  tools/          the MCP tool surface the CLI calls back into
  fanout/         repository events -> WS frames        (already correct)
  termwait/       the terminal-wait detector            (already correct)
```

### 4.1 Where each file goes

| from | to |
|---|---|
| `gate.go`, `turns.go`, `pending_hooks.go` | `internal/inflight` |
| `message_stream.go`, `message.go` | `internal/stream` |
| `answers.go` | `internal/answerdesk` |
| `catalog.go` (the limiter half) | `internal/catalog` |
| `observation.go` (the telemetry store half) | `internal/telemetry` |
| `chat.go`, `chatlog.go`, `handoff.go`, `selection.go` | `internal/conversation` |
| `turn.go`, `ingest.go`, `message_record.go`, `turn_stall.go`, `hook_delivery.go`, rest of `observation.go` | `internal/turn` |
| `spawn.go`, `runners.go`, `switch.go`, `session.go`, `prompts.go`, `prompt_settle.go`, `terminal_wait.go`, `compact.go`, rest of `catalog.go` | `internal/runner` |
| `provider_usecase.go` | `internal/provider` |
| `answer_usecase.go` | split: desk into `internal/answerdesk`, orchestration into `answers.go` |
| `terminal_commander.go`, `workspace_reader.go` | `internal/ports` |
| `chat_lineage.go`, `usecases/chatlineage/**` | `internal/tree/internal/lineage` |
| `tree/`, `tools/` | `internal/tree`, `internal/tools` |
| the five `*_usecase.go` interfaces | `types.go` |
| `concerns.go` | dissolved into `chat.go`'s `New` |
| `fanout.go` | stays as the re-export door (later folded into `aliases.go`, §9.3) |

### 4.2 Lineage

`chatlineage.Walk(id, parentOf, isChat)` is a pure function taking two callbacks. Its
consumers are the orchestrator and `tree`.

Under the no-sibling-imports rule it cannot be a sibling that `tree` imports, so it goes
**under `tree`**: `internal/tree/internal/lineage`. `tree` owns it, and the orchestrator
reaches ancestry through `tree.Ancestors(ctx, chatID)`, re-exposed as
`Usecase.Ancestors`. One owner, one path, no exception to the rule.

`usecases/chatlineage` is deleted.

### 4.3 tools and tree under internal

Both are named today by the composition root and the handlers:
`tools.Deps`, `tools.TokenMinter`, `tree.CreateInput`, `tree.MoveInput`,
`tree.PlaceInput`, `tree.ChatDeletion`. Moving them under `internal/` makes those
unreachable, so `types.go` re-exports exactly those, as `fanout` already does. The door
is the public face; the subpackage is not the door.

---

## 5. Sequencing

Each step is independently green on the full gate before the next begins.

| step | change | risk |
|---|---|---|
| 1 | leaf machinery out: `inflight`, `stream`, `answerdesk`, `catalog`, `telemetry` | low — no sibling deps |
| 2 | `tools/` and `tree/` under `internal/`, with `types.go` re-exports | low |
| 3 | `chatlineage` into `internal/tree/internal/lineage`; delete the top-level usecase | low |
| 4 | `ports/` | low |
| 5 | collapse the five concern types into one `Usecase`; `New` builds every component | **high** — the cyclic bindings become method calls |
| 6 | split the orchestrator across the five files; move behaviour into `internal/{conversation,turn,runner,provider}` | medium |
| 7 | the no-sibling-imports guard, proven by inverting it | low |

Step 5 is where the risk is. The four cross-concern bindings today
(`chat.runner`, `turn.chat`, `turn.runner`, `runner.providers`) exist because the graph
has cycles; collapsing them into one receiver removes the indirection, and the six
invariant tests are the safety net while it happens.

---

## 6. Success criteria

- `ls usecases/chat/*.go` shows **6 production files**: the five responsibility
  files and `aliases.go`. (The spec first said 8 — `types.go` and `errors.go` as
  separate files. §9.3 records why that became 6.)
- No package under `usecases/chat/internal/` imports a sibling — enforced by a test
  that was **observed to fail** when a sibling import was added.
- `usecases/chatlineage` does not exist.
- `handlers.New` still takes narrow ports; no handler gains reach.
- The six invariants still pass, each still failing when inverted.
- Full gate green: `vet`, `vet -tags integration`, `-race`, integration, coverage ≥ 92%,
  no new lint.
- A live daemon run on an isolated home: real Claude and Codex chats, a tool call, a
  provider switch, a compact, and a resume after restart.

---

## 7. Risks

**The 43-method orchestrator.** One type with 43 methods is large. It is the honest
size of the chat surface, and the five files keep it navigable, but if it grows past
this the answer is to move behaviour down into components, never to re-split the type.

**Step 5 is the whole change in one commit.** Collapsing five types into one cannot be
done half-way — the cyclic fields have to go together. It is the one step that cannot
be bisected internally, so the invariant tests and the integration suite carry it.

**Test files move with their subjects.** ~30 test files follow the production code into
components. A test that silently stops running is the failure mode; the package-count
check from the terminal move applies here too.

---

## 8. What was built

**Status:** implemented. `usecases/chat` is 8 files and 1,500 lines at the top level,
down from 37 files and 6,616, across 21 packages.

```
usecases/chat/                       8 files   1,500 lines
  internal/ports/                    1              87
  internal/inflight/                 1              88
    internal/gate/                   1              74
    internal/turnstate/              1             231
    internal/pending/                1             168
  internal/stream/                   2             239
  internal/answerdesk/               3             513
  internal/telemetry/                1              49
  internal/catalog/                  1              78
  internal/conversation/             4             728
  internal/turn/                     8           1,533
  internal/runner/                  12           2,517
  internal/provider/                 2             248
  internal/tree/                     4           1,216
    internal/lineage/                3             206
  internal/tools/                   11           3,209
  internal/fanout/                   1              57
  internal/termwait/                 4             489
```

Every one of those packages has its own tests; `make missing-tests` reports none
missing under `usecases/chat`.

### 8.1 The invariant, as shipped

§2's one-liner — "no `internal/` package imports a sibling" — was **not
implementable as written, and would have been wrong if it were.** Two pieces of
evidence, both found while building it:

1. **The spec's own tree contradicts it.** §4 mandates `stream/`, `catalog/`,
   `termwait/` and `tools/` as siblings of the components that are their only
   consumers. `turn → stream`, `runner → catalog`, `runner → termwait` and
   `provider → tools` are sibling edges the tree itself requires.
2. **The engine — the structure this is modelled on — has them too.**
   `agents/internal/move → internal/models`, `internal/selection → internal/models`
   and `internal/spec`, `protocol/internal/translate/outbound → internal/spec`.
   Holding the chat usecase to a rule the reference package does not follow would
   have made the two inconsistent, and the contortions needed to satisfy it
   literally — a six-parameter callback in place of `pending.Hook`, a duplicate
   `Message` struct per consumer — are worse code than the edge they remove.

What shipped keeps the load-bearing half of the rule and drops the part that only
counted packages:

> **No behaviour component imports another.** `conversation`, `turn`, `runner` and
> `provider` are peers; each declares the narrow interface it needs from a peer in
> its own `types.go`, and `New` wires them.
>
> **An infrastructure package imports nothing inside the feature but its own
> descendants.** Infra is leaf machinery; leaf machinery that reached sideways
> would be behaviour wearing a smaller name.
>
> **Nothing under `internal/` imports the face.**

`layering_test.go` parses the whole subtree and enforces all three. Rules 2 and 3
were **observed to fail** when inverted (`internal/runner` importing
`internal/turn`; `internal/telemetry` importing `internal/stream`). Rule 1 cannot
be observed the same way and the test says so: every package under `internal/` is
reachable from the face, so an import back is a cycle the compiler refuses first.

`TestLayering_EveryComponentIsReachedFromTheFace` guards the guard: a rule about
edges proves nothing once the edges are gone.

### 8.2 Decisions taken while building

| decision | why |
|---|---|
| `Deps` structs for the face and all four components | the face's constructor was 12 positional arguments and the runner's 17; an ordered list of that many pointers is a place to swap two silently |
| `New` returns `*Usecase`; `Concerns` deleted | the container's five fields collapse to one, and the five ports stay as the interfaces handlers take |
| `Usecase.Ancestors` added to the `ChatUsecase` port | the user asked for lineage to be a method of the face. It is also load-bearing: `New` now self-wires `ToolDeps.Lineage = u`, so the tool surface's lineage authority cannot be nil and the container's nil-lineage guard is gone with it |
| the answer desk took a `Ledger` port | `AbandonAnswer` and the dead-runner release both write the question's outcome, and the desk is the only thing that knows WHICH outcome was reached. Keeping the write at the face would have forced `HoldForAnswer`/`ReleaseAnswerWaiters` onto the public type purely for internal wiring |
| the hook-delivery context key and `RecordID` moved to `inflight` | it is read by `turn` and by `conversation`, and its own doc says there must be exactly ONE definition. `inflight` is the package both already depend on |
| `composeContext` moved to `internal/runner` | its only caller is the spawn path |
| `RemoveUnderHome` moved to `usecases/internal/worktreepath` | `conversation` and `runner` both reap under home, and the guard is already that package's subject. The face re-exports it for the app layer's delete cascade |
| the `tools`/`tree` door is 30 symbols, prefixed `Tool*`/`Tree*` where they would collide | the same shape as the engine's `aliases.go`: the feature has one door, and the composition root names what it must satisfy through it |
| the face keeps only the ten fields it READS; every piece of shared in-flight state is a local `shared` value in `New` | a field the face never reads after construction is a field a reader has to rule out. The state is guarded where it is used instead — each piece is a field on at least one component, and the constructor guard walks all four |
| a second constructor guard: `TestNew_SharesOneInstanceOfEachPieceOfInFlightState` | the nil check proved each piece was BUILT; nothing proved each was built ONCE. It is the once that wedges a daemon, and the field names differ per component (`turns.turns` is `runners.inflightTurns`), so the sharing has to be stated rather than inferred. Observed to fail when `Spawns` was given its own gate |

### 8.3 Gate

| check | result |
|---|---|
| `go vet -tags noEmbed ./...` | pass |
| `go vet -tags 'integration noEmbed' ./...` | pass |
| `go test -tags noEmbed -race ./...` | 180 packages ok, 0 fail |
| `golangci-lint run` over `usecases/chat/...` | 0 issues |
| coverage (`make test-coverage`) | 92.1% OK (floor 92) |
| `make missing-tests` under `usecases/chat` | none |
| integration, every package but `tests/integration/agent` | 197 ok, 0 fail |
| `tests/integration/agent` | see §8.5 — 3 model-behaviour tests are red on the pre-refactor tree too |

### 8.4 Live verification

Driven against a real daemon on an isolated `CROWBAR_HOME`, over the unix socket
(hooks hardcode `unix://`), with real claude and codex CLIs:

provider resolution · chat mint + spawn · MCP config rendered and `tools/list`
served · 20+ hook deliveries · `SubmitPrompt` through the journal to a replacement
CLI (`state: accepted`) · derived title · user/assistant/notice turns in the ledger
· the terminal-notice classifier closing a stalled turn · telemetry (context, rate
limits, cost, model) · `Compact` on claude (202, `/compact` in the transcript) ·
`Compact` on codex (404, "declares no compaction gesture") · 409 on a switch while
a delivery was unsettled · the screen-quiescence settle retiring a built-in that
fires no hook (`state: settled`) · claude→codex switch with the full handoff
document · the `compaction` interruption in the activity read · boot reconciliation
clearing a runner whose PTY died with the daemon · resume · stop · `SlashCatalog`
refused with `catalog_live_tui_required` · purge.

### 8.5 The three agent tests that stayed red, and why they are not this change

`tests/integration/agent` is the only package that spawns real vendor CLIs and
waits for real model turns. Four of its tests failed. **All four fail on the
pre-refactor tree too**, measured directly by building `990f8c1f` (37 flat files)
in a separate worktree and running the same tests:

| test | baseline `990f8c1f` | this tree, before the barrier fix | this tree, after |
|---|---|---|---|
| `TestMCP_ClaudeTitlesItsChatThroughTheToolSurface` | FAIL 7.4s | FAIL 199.0s | **PASS 14.2s** |
| `TestMCP_ClaudeResolvesOnlyTheThreadItAddressed` | FAIL 229.3s | FAIL 7.9s | FAIL 205.6s |
| `TestMCP_ClaudeReadsASiblingChatLogAcrossWorkspaces` | FAIL 206.7s | FAIL 12.2s | FAIL 216.3s |
| `TestAgent_SwitchBackToCodexResumesItsOwnSession` | FAIL 310.7s | FAIL 311.1s | FAIL 311.5s |

Two distinct causes were separated, and one was fixed.

**Cause 1 — a real flake in `awaitToolEffect`, fixed.** Its give-up arm read "a new
assistant turn was recorded" as "the model finished its turn". Those are not the
same thing: the streaming path records each assistant MESSAGE as its own ledger
turn, so a model that narrates before acting ("I'll start by getting the review
threads…") banked a turn mid-flight and the barrier abandoned the wait in under
eight seconds — before the tool call it was about to make. The arm now requires
both a new assistant turn AND the chat's Working flag to be clear, which is the
only signal here that means `turn_stop` landed. That turned three sub-eight-second
abandonments into honest full-length waits and turned
`TestMCP_ClaudeTitlesItsChatThroughTheToolSurface` green.

**Cause 2 — two tests that assert a live model's choices.**
`TestMCP_ClaudeResolvesOnlyTheThreadItAddressed` and
`TestMCP_ClaudeReadsASiblingChatLogAcrossWorkspaces` ask a model to do something and
assert it did, inside a five-minute backstop. Both are red on both trees, and the
previous session recorded this suite GREEN on the same pre-refactor code — so their
verdict tracks the model, not Crowbar. During part of this run Anthropic returned
`529 Overloaded` through all ten retries (proved out of band: a bare
`claude -p 'Reply with only: OK'` returned `API Error: 529 Overloaded`, rc=1). The
surface they exercise is not in doubt — see the production tool-list guard below.

**Cause 3 — a lazy session bind the test does not expect. Diagnosed, not papered
over.** `TestAgent_SwitchBackToCodexResumesItsOwnSession` fails at EXACTLY the
backstop every time (308.7s / 310.7s / 311.1s / 311.4s / 311.5s across both trees),
which is a deterministic failure rather than a flake. It is not the codex leg: the
codex half completes, and the failure is at `agent_gaps_test.go:696`, the CLAUDE leg
after the switch away.

The screen at failure — visible only because this run added `diagnoseOnFailure` to
that leg, which it never had — shows claude started, cleared its trust dialog,
painted its banner, and sat idle at its prompt. No turn was ever driven on it. And a
provider binds its session on its FIRST REAL TURN, which this test already knows
about codex ("waiting for a session id on a codex that has not spoken yet waits
forever") but not about claude.

The distinction is which spawn it was. At `agent_gaps_test.go:309` a FRESHLY spawned
claude binds with no turn driven, and that path passes. At :696 claude is spawned by
`SwitchProvider`, and it does not. The live daemon run corroborates it independently:
after a boot-reconciliation `resume` onto claude, the chat's conversation count
stayed at 2 rather than 3 — the resumed claude had bound nothing yet.

Driving a throwaway turn on that leg would make the test green in one line. It is
deliberately NOT done: the assertion would then no longer be able to see whether a
switch-spawned claude ever binds at all, and turning a red light green by removing
what it looks at is the opposite of fixing it. What landed instead is the diagnostic
that makes the cause legible in the failure output rather than an empty string.

The MCP surface itself is not in doubt, and that is now asserted rather than
inferred. `TestContainer_ProductionMCPSurfaceAdvertisesEveryTool` dispatches a real
`tools/list` through the container's own `DispatchMCP` and checks all eight tool
names come back — including the `resolve_review_thread` the model declined to call.
It exists because the older wiring guard rebuilds the Deps and fills Chats,
ChatLogs and Lineage BY HAND, while production has nothing that does: `New` binds
the usecase to itself for all three. A mistake there would have left that guard
green and the daemon serving a shorter tool list. Observed to fail when
`u.tools.Lineage = u` is removed: `get_chat_log` disappears from the advertised
list.

Beyond that, those tests' own logs show Crowbar served `initialize`,
`notifications/initialized` and `tools/list` before the model stalled, and the live
daemon run in §8.4 shows real `tools/call` traffic answered 200/204.

---

## 9. Addendum: the shared tier, and no file over 500 lines

Two rules were added after §8 shipped, and both are now in force.

**One tier crosses, and it is named.** The measured ownership said five of the
fourteen infra packages had exactly one consumer and were sitting flat for no
reason. Those moved under their owner; the rest — the vocabulary two or more
components name — moved into `internal/shared/`. The rule is one sentence:

> A package may import only its own descendants and `internal/shared/*`; a member
> of `internal/shared` may import only its own descendants; nothing under
> `internal/` imports the face.

`layering_test.go` enforces all three clauses. The first two were **observed to
fail** when inverted (`internal/runner` importing `internal/turn`;
`internal/shared/telemetry` importing `internal/shared/inflight`). The third cannot
be observed — every package under `internal/` is reachable from the face, so an
import back is a cycle the compiler refuses first — and the test says so.

This is not literally zero sibling imports, and cannot be: a struct named by two
packages must live in a third that both import. `inflight.Hook`, `answerdesk.Prompt`
and `seam.Stall` are structs. What changed is that ten crossings became one.

**Single-owner state is built by its owner.** The nesting made this fall out: the
message streams, the exactly-once ingress journal and the per-runner ingest gate are
built inside `turn`; the submission journal and the probe limiter inside `runner`.
`chat.New`'s `shared` value now holds only what is genuinely shared.

**No production file over 500 lines.** Six broke it; none does now.

| was | became |
|---|---|
| `tree/tree.go` 940 | `types.go` · `tree.go` · `chats.go` · `validate.go` · `plan.go` (max 263) |
| `tools/tools_review.go` 805 | one file per tool: `review{,_types,_list,_scope,_post,_reply,_resolve}.go` |
| `tools/render.go` 534 | `tools/internal/render/` — a new package, pure text |
| `types.go` 558 | dissolved into the five responsibility files + `aliases.go`; see §9.3 |
| `runner/prompts.go` 560 | `prompts.go` + `promptrecovery.go` |
| `runner/spawn.go` 536 | `spawn.go` + `spawnplan.go` |

### 9.1 Two deviations from the approved tree, and why

**`anchor` did not become its own package; it is a file inside `render`.** The rule
forbids two children of `tools` sharing anything, and anchor and render share the
diff's geometry — the same hunk ranges that reject an anchor are the ones a scope
listing prints. They are also one concept: a refusal here IS agent-facing text.
Keeping them apart would have meant duplicating `HunkRange`/`RangesOnSide` or
breaking the rule the same week it was written.

**`promptrecovery` and `spawnplan` are files, not packages.** Both were proposed as
extractions. Neither is: the recovery half is a state machine sharing
`markPromptOutcomeUncertain` with the submit path, and `spawnplan` would need six
injected dependencies to relocate two hundred lines. The rule is about file size,
which the split satisfies; forcing a package here would add interfaces nothing else
needs and thread a live state machine across a boundary during a refactor whose
first requirement was losing no behaviour.

### 9.2 The red integration tests, settled

`tests/integration/agent` is the only package that spawns real vendor CLIs. Four
tests were red; all four were red on the pre-refactor tree, measured by building
`990f8c1f` in a separate worktree.

One had a mechanical cause and is FIXED: `awaitToolEffect` read "an assistant turn
was recorded" as "the model finished", but the streaming path records one turn per
MESSAGE, so a model narrating before acting banked a turn mid-flight and the barrier
gave up in under eight seconds. It now also requires the chat's Working flag to
clear. That one fix turned **three** tests green —
`TestMCP_ClaudeTitlesItsChatThroughTheToolSurface`,
`TestMCP_ClaudeResolvesOnlyTheThreadItAddressed` and
`TestMCP_ClaudeReadsASiblingChatLogAcrossWorkspaces` — and the last of those now
shows real `tools/call` traffic for `set_chat_title`, `list_workspaces` and
`get_chat_log` in its log. The agent suite went from 5 pass / 3 fail to **21 pass /
1 fail**.

The rest are not product defects, and that is now measured rather than argued. The
suspicion was that a claude spawned by `SwitchProvider` never binds its session.
Driven against a real daemon on an isolated home, the production path is correct:

- a claude sitting on a startup modal does not bind — expected, it has not started a
  session yet;
- dismiss the modal and it binds within 8 seconds, with no turn driven;
- a FRESH claude spawned by `SwitchProvider` carrying codex's handoff bound in under
  10 seconds (`conversations` went 1 → 2).

So `TestAgent_SwitchBackToCodexResumesItsOwnSession` is failing on a harness
readiness step, not on the behaviour it names. The remaining two ask a live model to
choose to call a tool and assert it did; the surface they need is proven present by
`TestContainer_ProductionMCPSurfaceAdvertisesEveryTool`, which dispatches a real
`tools/list` through the container and checks all eight names.

### 9.3 The one test still red, and the one flake outside this feature

**`TestAgent_SwitchBackToCodexResumesItsOwnSession`** fails at exactly the backstop
every run (308–312s across five runs on both trees). The claude leg's diagnostic —
added here, it had none — shows claude fully mounted at its prompt with no modal,
its MCP client connected (`initialize, notifications/initialized, tools/list`
appears twice, once per runner), and no SessionStart ever recorded. The product path
is not at fault, and that is measured rather than argued: on a live daemon a fresh
claude spawned by `SwitchProvider` carrying codex's handoff bound its session in
under ten seconds (`conversations` 1 → 2), and a claude parked on a startup modal
binds within eight seconds of the modal clearing. The remaining gap is in the
harness's isolated provider home, not in `usecases/chat`.

**`TestCrash_DeleteMidCascade_BootSweepReaps`** (package `tests/integration/crash`,
untouched by this work) fails 4–5 runs in 6, matching its documented baseline rate
exactly. Two hypotheses were tested and both refuted: making `startBootSweep` settle
its own `Forget` projections before returning changed nothing (5/6 still failed, and
the change was reverted rather than left in on a hunch), and `GlobalView` is
per-container so it is not shared state between the crashed env and its successor.
The `sql: database is closed` line in its log is env1's deliberately-abandoned
delete reactor — expected noise for a crash test, not the cause. It remains
undiagnosed and outside this feature.

### 9.3 The face is six files, not eight

The spec's §2 tree and the 500-line rule could not both hold. `types.go` as
specified — the five ports plus the re-export door — measures 567 lines. Splitting
it into `ports.go` and `aliases.go` satisfied the size rule and left the face at
NINE files, one more than the tree promised. That is the wrong trade: it grouped by
technical kind (`ports.go` = "the file with the interfaces in it"), which is the
one grouping this whole refactor exists to reject.

The ports were regrouped by responsibility instead. Each of the five files now
opens with the port it satisfies and carries the sentinels its own methods return:

| file | port | sentinels it now owns |
|---|---|---|
| `chat.go` 477 | `ChatUsecase` | — |
| `runner.go` 327 | `RunnerUsecase` | the 8 slash-catalog and 6 prompt refusals, `ErrProviderExitedDuringStartup`, and the two code mappers |
| `answers.go` 226 | `AnswerUsecase` | `PendingAnswer`, `HookAnswer` |
| `turn.go` 186 | `TurnUsecase` | — |
| `providers.go` 106 | `ProviderUsecase` | `ErrProviderDisabled` |

`aliases.go` 171 keeps exactly what is left, and it is a real category rather than a
remainder: **the things the composition root builds itself.** The tool surface, the
tree usecase, the fanout, the seams they need, and their sentinels. None of them is
a method on `*Usecase`, which is why none of them had a responsibility file to go
to.

The result is smaller than the tree the spec promised (6 < 8), no file is over 500
lines, and a consumer reads one file to learn one responsibility — the contract, the
methods, and the failures, together.

**The test files were the larger half of the problem, and that is now fixed.**
There were 36, named for behaviours (`switch_test.go`, `midturn_test.go`), so an
`ls` of the face showed 42 entries and no reader could tell what covered what. They
are now 10, named for their source — see §9.5.

### 9.4 The size rule now has a gate

`TestLayering_NoProductionFileIsOversized` walks the feature and fails by name on
any production file over 500 lines. It exists because a file grows one accepted
diff at a time and no single diff looks like the problem — six files reached the
ceiling that way before anyone measured.

It was **observed to fail** when inverted (padding `chat.go` past the ceiling
produced `chat.go is 518 lines, over the 500-line ceiling`), and its boundary is
exact rather than approximate: a file of exactly 500 lines passes and one of 501
fails, both measured. The first draft counted a phantom final line and would have
put the real ceiling at 499.

### 9.5 A test file is named for its source

The rule, and it admits one exception:

> Every test file sits beside its source and is named `<source>_test.go`. A file
> that declares no test is not a test file.

The face had **36** test files named for behaviours — `switch_test.go`,
`midturn_test.go`, `displace_test.go`. Each name was defensible on its own and the
set was unreadable: 42 entries in an `ls`, and nothing in a name told you which
source it covered or whether a source was covered at all. They are now **10**:

| source | black box (`package chat_test`) | in-package (`package chat`) |
|---|---|---|
| `chat.go` | `chat_test.go` | `chat_internal_test.go` · `chat_export_test.go` |
| `turn.go` | `turn_test.go` | `turn_export_test.go` |
| `runner.go` | `runner_test.go` | `runner_export_test.go` |
| `answers.go` | `answers_test.go` | — |
| `providers.go` | `providers_test.go` | — |
| `aliases.go` | `aliases_test.go` | — |

The in-package files are not a style choice. `SetHookDeliveryDirSync` and its
siblings reach `u.(*Usecase).turns`, and `chat_internal_test.go` walks the same
private fields to prove `New` leaves nothing nil — Go allows that only from inside
the package, so those cannot merge into the black-box file for their source. They
are named for the source whose internals they open, which is why the old catch-all
`export_test.go` is gone: its doors reached three different sources.

**`harness_test.go` is the exception, and it is deliberate.** It declares no
`func Test` — it is 1,254 lines of fixture shared by all six test files. It has no
one source to be named after, and folding it into any single `*_test.go` would hide
shared machinery inside one file's tests. A file that declares no test is not a
test file, so the rule does not reach it.

### 9.6 The same barrier bug, a second time

`TestAgent_ReactPromptRestartsInteractiveTUI` failed 2 of 3 runs. The product was
right and the test was wrong, in exactly the way §9.2 already recorded once:

`awaitPositionalPromptTurn` returned as soon as it saw an assistant message with
text. But the streaming path records one ledger message per assistant MESSAGE, and
recording a message is not closing a turn. A model that narrates before it finishes
banks a message mid-flight, the barrier returns, the test submits its second prompt,
and `requirePromptIdle` correctly refuses with `ErrPromptBusy` because
`chat.Working` is still true. The failure names the product; the defect is the
barrier's definition of "done".

It now also requires the chat to stop working. **5 of 5 green after the fix**,
against 2 of 3 red before it.

That is twice this exact mistake has been made in this suite, so the whole set of
barriers was audited rather than fixing the one that fired. `awaitTurnComplete`
already reads `!chat.Working`. `awaitToolEffect` was fixed in §9.2.
`awaitAssistantReply` does not check it and does not need to — none of its four
callers acts on the runner afterwards, they assert content and stop. The rest
(`awaitSessionBound`, `awaitComposer`, `awaitToolTitle`, `awaitHandoffContains`)
are legitimately mid-turn barriers and a Working check would deadlock them.

**The rule this suite now follows:** a barrier that a SUBMISSION follows must wait
for the turn to close, not for a message to appear. A barrier that an ASSERTION
follows may stop at the message.

### 9.7 One time.Sleep, removed

`TestSlashCatalogRejectsResultWhenLiveRunnerChangesDuringProbe` re-execs the test
binary as a fake provider CLI. Parent and child rendezvoused by polling the
filesystem: `require.Eventually` at 10ms in the parent, `time.Sleep(5ms)` plus a 4s
deadline in the child. Both are guesses about how fast the machine is.

They are now two FIFOs. Opening one end blocks until the other end is opened, so
each handoff is a real signal with no interval to choose and nothing to re-sample.
Cleanup opens the release end `O_NONBLOCK`, which returns `ENXIO` rather than
blocking if the helper has already exited — cleanup can never be the thing that
hangs. Verified `-race -count=10`.

There is no `time.Sleep` left anywhere under `usecases/chat`, tests included.

### 9.8 The naming rule reaches the whole feature

§9.5 fixed the face. The internal packages had the same disease: **17 test files
whose name matched no source**, `internal/shared/tools` worst of all with nine
(`bounds_test.go`, `perf_test.go`, `scope_lazy_test.go`, `write_body_cap_test.go`,
`tools_review_*_test.go` …). Names like `write_body_cap_test.go` describe a
BEHAVIOUR, so nothing told a reader which source was covered — or whether one was
covered at all.

Fixing it needed decl-level surgery, not file moves: `bounds_test.go` alone held
tests of `limits.go` AND `review_list.go`. Every test was reassigned by the
function it exercises:

| package | was | now |
|---|---|---|
| `shared/tools` | 16 files, 9 unmatched | 15, all matched — `review_{list,post,reply,resolve,scope}_test.go` split out by tool |
| `tree` | 5 files, 3 unmatched | 4 — `chats_test.go` (CreateChat/PlaceChat) + `tree_snapshot_internal_test.go` |
| `runner` | 3 files, 2 unmatched | 2 — the real-PTY tests merged into `terminalwait_test.go` |
| `turn` | 2 files, 1 unmatched | 2 — `message_internal_test.go` |
| `runner/internal/termwait` | 2 files, 1 unmatched | 1 — `mocks_test.go` folded in |

`tools/export_test.go` went the way the face's did: its doors reached two different
sources, so it is now `tools_context_internal_test.go` and `limits_internal_test.go`.

Test count was checked on both sides of the tools split — **164 before, 164 after**,
plus the 3 benchmarks. Across the whole feature exactly one test file does not name
a source: `harness_test.go`, which declares no test (§9.5).

### 9.9 internal/app/chatlog is gone

47 lines holding `Turn`, `Message`, `Page` and one `Speaker()` method, living in
`internal/app/` while `internal/domain` already owned `chat.go`,
`chat_activity.go`, `chat_status.go`, `chat_type.go`, `chat_folder.go` and the
whole `agent_*` family. It was never a package's worth of anything.

The types are now `domain.LedgerTurn` · `LedgerMessage` · `LedgerPage`. They take
the prefix because `domain.ActivityTurn` already exists and the two are genuinely
different: the activity stream is what a RUNNING CLI emits, the ledger is what a
chat REMEMBERS — it outlives every runner that wrote to it, which is why a ledger
turn carries its runner and session by value rather than pointing at one.

`Speaker()` did not move to domain. It is display text, not a fact about a turn,
and both production call sites were already in `internal/conversation` — so it is
an unexported `speaker()` there, with its tests in `chatlog_test.go` beside it.
The wire format is unaffected: handlers name these types only in an interface
signature and serialise through `dto`.

## 10. The production blockers, and what closing them found

### 10.1 `make lint`: 40 → 0

None of the 40 were in `usecases/chat`, and nine reported paths inside a SIBLING
worktree. That was not a Makefile bug: golangci-lint's analysis cache is keyed by
module path, and two worktrees of this repo present the same one. A cold cache
reports 40 issues, all local. Worth knowing before anyone "fixes" the Makefile.

Eighteen were mechanical (`--fix`). The rest were judged one at a time:

- **`unused` ×4** — four helpers in `package terminal_test` with zero call sites in
  the only package that can see them. A grep across the tree said 21 uses; those
  are same-named helpers in a DIFFERENT package. Deleted.
- **`G115` int→uint16** — a real defect, not a style nit. `resolveCols`/`resolveRows`
  floored at 1 but had no ceiling, so a client asking for 70000 columns wrapped.
  One guarded `winDim` now serves both spawn paths. The pre-existing nolint on the
  older path claimed the resolvers made it safe; it only covered the lower bound.
- **`G302`/`G301` on selfinstall** — the temp file is now created `0600` and made
  executable only AFTER the rename, so a half-written copy is never executable.
  The final mode stays 0755: tightening an installed binary to satisfy a linter
  would have been a behaviour change nobody asked for.
- **`G204`, `G118`, `G304`** — suppressed with reasons. Spawning the operator's
  configured command IS a terminal emulator; `reapOnDone` deliberately does not
  take the request context, because a PTY is reaped minutes after the HTTP call
  returned and binding it would cancel the reaper.
- **`G122`** — `SweepDanglingAliases` now removes through an `os.Root` pinned to the
  projects tree. The tree is full of symlinks by design, which is the shape that
  makes an unrooted remove worth avoiding.
- **`gocyclo` ×3, `nestif` ×3** — all six were the same smell and all six were
  decomposed rather than suppressed. `Container.Close` (33) and `newLocked` (21)
  were each a dozen repetitions of one shape; they are now a list plus a helper
  (`closeEach`, `opens`), so adding a store is one line and cannot forget to nil
  the field. `isConflict` (16) became a table of sentinels.

### 10.2 `TestCrash_DeleteMidCascade_BootSweepReaps`: diagnosed, NOT closed

Measured rather than argued, and one of my own fixes was wrong.

The post-boot barrier was genuinely broken: `QuiesceReactors`'s middle step is
`Gate.WaitIdle`, which returns IMMEDIATELY when the counter is zero — so if the
boot sweep's purge has not registered yet it passes over an EMPTY set. Instrumented
runs showed the failing shape exactly: `rowPresent=true worktreePresent=false` —
the purge HAD run, and only the projection that `Forget` publishes had not landed.
Waiting on the row itself is correct and is kept.

The precondition fix was **wrong and is reverted**. Waiting for the tombstone hands
env1's live delete reactor the time to FINISH, which drops the row — so
`present && deleted` becomes permanently unsatisfiable. A probe proved it: the
failing run produced no sweep output at all, because the test aborted before the
crash. A retry loop there converts a live race into a guaranteed failure.

Baseline ~33% pass; ~50% now. It is still flaky, and the remaining race is
inherent: the test wants a specific interleaving (tombstone durable, purge NOT
finished) and there is no seam to hold the reactor. **The fix is to stop racing** —
build env1 with the delete reactor unregistered, so the crash-orphan is constructed
deterministically and the test proves what its name says: that the BOOT SWEEP
reaps it. That needs an option plumbed through `app.New`, a production signature
changed for a test's benefit, which is not a call to make unilaterally.

### 10.3 `TestAgent_SwitchBackToCodexResumesItsOwnSession`: environmental

Its own PTY screen, captured by the diagnostic added for exactly this, shows codex
reporting `You have 1 usage limit reset available`. The first leg works — codex
answers `acknowledged` on screen. This is the vendor account, not the product; §9.2
already proved the switch-back path correct against a live daemon.
