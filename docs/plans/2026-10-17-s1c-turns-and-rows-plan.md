# S1c — Turns, one ingress, and the transcript as rows: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The turn is a ledger state machine with a Crowbar-minted id and "busy" is its fold (T5, symptom 4); one hook ingress resolves, records and only then replies, dead-lettering what it cannot place (T6); every transcript fact is a row and the activity aggregate is deleted (T7, symptom 3); rows carry the tool, MCP, subagent and plan detail the hooks already send (T8, T9).

**Architecture:** See the index, `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. Requires T1–T4: the ledger store and fold, prompts as rows, chat-owned conversations, the in-memory live table, `snapshot.LedgerFacts`/`ApplyLedger`.

**Tech Stack:** Go, SQLite, testify, `api/tests`, the descriptor grammar under `api/internal/engine/agents/internal/{spec,mapping,protocol}`.

**Spec:** architecture §3 (journal invariants), §4 (hook ingress paragraph, canonical vocabulary), §5 (turn diagram); report §2 rows "Mid-turn messages lost" and "Prompt sent, nothing renders", §4.3 ingress contract; additions design §3.1, §3.2, §4.C, §4.F.

---

### Task 5: The turn is a state machine in the ledger (symptom 4)

**Goal:** `turn_started` / `turn_ended` / `turn_failed` rows own the turn; `State.Busy()` is the only busy answer; every close names its turn; a Stop is recorded by Crowbar at once; a runner's exit and a daemon restart close the running turn. The chat aggregate's turn state, the in-flight turn registry and work mirror, the stall closer, the message-quiet/idle/stall timers, the 2 h staleness nets and the switch force-kill are deleted.

**Files:**
- Create: `api/internal/domain/ledger/turn.go` and `turn_test.go`
- Modify: `api/internal/domain/ledger/{row.go,state.go,state_test.go}` — kinds `KindTurnEnded`, `KindTurnFailed`; turn fold; `Busy`; `HasOpenWork` includes a running turn
- Modify: `api/internal/app/usecases/chat/internal/turn/{turn.go,turn_open.go,turns.go,types.go,ingest.go,observation.go,message.go,choices.go}` — lifecycle decisions write rows (below); the activity record keeps receiving its turn commands until T7
- Modify: `api/internal/app/usecases/chat/internal/runner/{prompts.go,steer.go,stop.go,switch.go,promptswitch.go,lifecycle.go,runner.go,types.go,terminalwait.go,errors.go}`
- Modify: `api/internal/app/usecases/chat/internal/runner/internal/termwait/{termwait.go,evaluate.go,sweep.go,run.go,termwait_test.go}` — keep only terminal-prompt and terminal-notice detection; a notice with `ends_turn` reports to a callback that appends `turn_failed{reason: <notice kind>}`
- Modify: `api/internal/app/usecases/chat/internal/shared/snapshot/snapshot.go` — `LedgerFacts.Busy` drives the snapshot's `working`; `api/internal/api/v0/dto/agent.go` — `AgentChatDTO.Working` from the snapshot, not `domain.Chat`
- Modify: `api/internal/app/repositories/chat/{event_store.go,internal/store/hub.go,internal/store/store.go,internal/store/storage.go}` — drop the turn commands and the `turn_started`/`turn_stopped` event kinds
- Modify: `api/internal/domain/chat.go` — delete `Working`, `CurrentTurnStarted`, `LastActivityAt`, `AsyncWork`, `LedgerCursor`
- Modify: `api/internal/app/repositories/container.go` — delete `registerAgentWorkingProjection`; export `SetAgentWorking(ctx context.Context, wsID, chatID string, working bool)` (the body of `setAgentTurn` + rebroadcast); `api/internal/app/usecases/container.go` passes it to chat `Deps.AgentWorking` (constructor argument)
- Modify: `api/internal/api/v0/endpoints/home/handlers/home.go`, `api/internal/api/v0/dto/{chat_worktree.go,workspace.go}`, `api/internal/api/v0/container.go`, `api/internal/app/usecases/workspace/internal/hierarchy/worktree.go` — every reader of `domain.Chat.Working` reads the chat usecase's `ChatBusy(ctx, chatID) (bool, error)` (from the snapshot) instead
- Modify: `api/internal/app/container.go` — boot pass: for each chat in `Store.OpenChats` whose state has a running turn and no live runner (after T4's `runner_exited{daemon_restart}`), append `turn_ended{interrupted, reason: daemon_restart}` (normally already written by the runner-exit decision below)
- Delete: `api/internal/app/repositories/chat/internal/commands/{start_turn.go,stop_turn.go,working.go}` (+ their test cases); `api/internal/app/usecases/chat/internal/turn/{stall.go,idle.go,subagent_staleness.go,tool_staleness.go}` and `{idle,subagent_staleness,tool_staleness}_internal_test.go`; `api/internal/app/usecases/chat/internal/runner/switchwait.go`; `api/internal/app/usecases/chat/internal/shared/inflight/internal/turnstate/`; the `Turns`/`Work`/`Steered` exports of `inflight.go`; `api/internal/app/usecases/chat/internal/shared/seam/seam.go` `Stall`; `interruptBound`/`turnEndedAfterInterrupt`/`AwaitTurnComplete`; `SetSwitchAwaitTimeout`, `SetMessageAwaitTimeout` (test seams in production files); termwait `DefaultStallQuiet`, `DefaultMessageQuiet`, `DefaultIdleQuiet`, `Messages`, `Idle`, `Work`, `Stalled` and their tests; `api/internal/app/usecases/chat/{idle_export_test.go,idle_notification_test.go,stop_invariants_test.go}` cases for deleted timers
- Modify (spawn gate): move `inflight.Gate`'s per-chat serialisation of user-initiated spawns into `runner/live.go` as `func (t *liveTable) lockChat(chatID string) (unlock func())`; delete `inflight/internal/gate` if nothing else uses it after T6
- Test (black box): create `api/tests/regression_chat_turn_test.go`

**Interfaces:**
- Consumes: T2 `user_message`/`turn_started`/`prompt_settled` and `PendingPrompts`; T4 live table, `runner_exited`; T3 `ApplyLedger`.
- Produces, `package ledger`:

```go
type TurnStatus string
const (TurnPending TurnStatus = "pending"; TurnRunning = "running"; TurnDone = "done"
	TurnFailed = "failed"; TurnInterrupted = "interrupted"; TurnRetracted = "retracted")
// Next: pending→running|retracted, running→done|failed|interrupted; nothing else.
func (s TurnStatus) Next(to TurnStatus) (TurnStatus, bool)

type TurnEnded struct {
	Status    string `json:"status"` // "done" | "interrupted"
	Reason    string `json:"reason,omitempty"`
	AsyncWork int    `json:"asyncWork,omitempty"`
	Effort    string `json:"effort,omitempty"`
}
type TurnFailed struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}
type Turn struct {
	ID, ProviderTurnID, RunnerID, RequestID string
	Status                                 TurnStatus
	StartedAt                              time.Time
}
func (s State) RunningTurn() (Turn, bool)
func (s State) Busy() bool // R11
```

Turn-end reasons (constants): `ReasonStopped = "stopped"`, `ReasonProviderIdle = "provider_idle"`, `ReasonRunnerExited = "runner_exited"` (exists), `ReasonDaemonRestart` (exists), `ReasonSuperseded = "superseded_by_prompt"`.

**Decisions** (each is one `Store.Do`; the activity record is told the same facts as today until T7):

| Trigger | Rows |
|---|---|
| `user_prompt` hook (T2 decision) while a different turn is running | first `turn_ended{done, reason: superseded_by_prompt}` for the running turn, then T2's rows |
| `turn_stop` hook, running turn T, event `providerTurnId` empty or equal to T's | `turn_ended{done, asyncWork: ev.AsyncWork, effort: ev.Effort}` for T; if a steer prompt is pending: also `turn_started{requestId}` for it and the hook reply is `PromptSteer(text)` |
| `turn_stop` naming another provider turn id, or with no running turn | nothing (late) |
| `turn_failed` hook, running turn T | `turn_failed{reason, detail}` for T; a pending steer prompt → `prompt_settled{consumed:false, reason: turn_failed}` |
| idle hook, running turn T | `turn_ended{done, reason: provider_idle}` for T (R2); with no running turn, T2's not-acknowledged settle |
| terminal notice with `ends_turn`, running turn T | `turn_failed{reason: <notice kind>, detail: <captured line>}` |
| user Stop (`StopChat`) | for a running turn T: `turn_ended{interrupted, reason: stopped}`; for every pending Crowbar prompt (steer or not): `prompt_settled{consumed:false, reason: stopped}`; **then** signal the provider (api interrupt call, else graceful terminate) and return — no wait |
| runner exit (T4 decision) | add `turn_ended{interrupted, reason: runner_exited}` for a running turn in the same `Do` |
| switch / selection change / prompt while `Busy()` | refused with `ErrPromptBusy` (R10); steer-eligible prompts park as T2 does |

Publication (`assembly.go` subscriber): a `turn_started` row → `snapshots.Announce(ctx, chatID, "turn_started")`; `turn_ended`/`turn_failed` → `Announce(…, "turn_stopped")`; every batch → `ApplyLedger(…{Busy: st.Busy()})` and, when busy flipped, `AgentWorking(ctx, wsID, chatID, busy)`.

- [ ] **Step 1: Domain tests**

```go
func TestTurnStatus_Next(t *testing.T) // all 36 pairs; exactly the 5 allowed transitions succeed

func TestFold_TurnLifecycle(t *testing.T)
// user_message T1, turn_started T1 → RunningTurn T1, Busy
// turn_ended{done} T1 → no RunningTurn, !Busy
// turn_ended{done} T1 again (late) → unchanged
// turn_started T2, turn_failed T2 → T2 failed, !Busy

func TestFold_AsyncWorkKeepsTheChatBusyOnlyWhileARunnerLives(t *testing.T)
// runner_started r1, turn_started T1, turn_ended{done, asyncWork:1} → Busy
// runner_exited r1 → !Busy
```

- [ ] **Step 2: Symptom 4 black-box tests** — `api/tests/regression_chat_turn_test.go`. For the provider-turn-id case install a stub fake (`Base: ""`) whose `EventsYAML` maps `turn_id: turn_id` on `user_prompt` and `turn_stop`.

```go
func TestRegression_Symptom4_ARunnerExitClosesItsTurnAndTheNextPromptGoesThrough(t *testing.T)
// prompt, user_prompt hook → working true; l.Kill → readUntil turn_stopped frame;
// GET chat working false; POST a new prompt → 2xx and a new launch is recorded

func TestRegression_Symptom4_StopClosesTheTurnAtOnce(t *testing.T)
// running turn; POST …/stop returns; the very next GET shows working false;
// the chat feed carried turn_stopped before the fake's process was signalled to exit
// (assert order of frames: turn_stopped precedes the runner "exited" frame)

func TestRegression_Symptom4_ProviderIdleClosesARunningTurn(t *testing.T)
// running turn; post event "idle" → working false; POST prompt → accepted

func TestRegression_Symptom4_ALateStopNeverClosesTheNextTurn(t *testing.T)
// stub fake: user_prompt{turn_id:A} → Stop chat (A interrupted) → user_prompt{turn_id:B}
// typed at the terminal (provider origin) → turn_stop{turn_id:A} → working still true;
// turn_stop{turn_id:B} → working false

func TestRegression_Symptom4_CrashMidTurnLeavesTheChatUsable(t *testing.T)
// running turn; h.crash(); h2 := newHarnessAt(t, home) → chat working false,
// session.exitReason "daemon_restart"; POST prompt → 2xx, new launch recorded

func TestRegression_Symptom4_ASwitchWhileATurnRunsIsRefusedNotForced(t *testing.T)
// running turn; POST …/switch to another provider → 409; the running launch's
// process is still alive (syscall.Kill(pid, 0) == nil)
```

`TestRegression_ChatStoppedMidTurn_SpunTheWorkspaceForever`, `TestRegression_GracefulShutdown_MidTurn_ClosesAbandonedTurn`, `TestRegression_WorkspaceWorkingReflectsAgentTurn`, `TestRegression_WorkspaceWorkingOverlappingChatsBroadcastOnce`, `TestRegression_HomeWorkspaceWorkingReflectsAgentTurn` and every `regression_steered_prompt_test.go` test must stay green; delete `TestRegression_WorkspaceWorkingClearsWhenClearAbandonsATurn` (the `/clear` move it relies on was deleted in T3).

- [ ] **Step 3: Run; the stop, idle, late-stop and crash cases FAIL today**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom4_' ./tests/`
Expected: FAIL (Stop waits for the 10 s bound on api chats and does not record first; idle arms a 5 s latch; a late stop closes the open turn; a switch force-kills).

- [ ] **Step 4: Implement the fold and the decision table; route every busy reader to the snapshot; delete everything listed; bring each edited file over 500 lines under it**

- [ ] **Step 5: Run to green**

Run: `cd api && go test -tags noEmbed ./internal/domain/ledger/... ./internal/app/... ./internal/api/...` → PASS.
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(Symptom4_|Symptom2_|ChatStoppedMidTurn|GracefulShutdown|WorkspaceWorking|HomeWorkspaceWorking|PromptSentMidTurn|SteeredPrompt|CommandSentMidTurn|PromptSentWhileIdle|StopRefusesAParkedPrompt|AgentStop|AgentClose)|TestFakeProvider_' ./tests/` → PASS.
Run: `cd api && make lint deadcode && go vet -tags noEmbed ./...`; `grep -rn 'DefaultStallQuiet\|DefaultMessageQuiet\|DefaultIdleQuiet\|CloseStalledTurn\|AwaitTurnComplete' api/internal` → nothing.

- [ ] **Step 6: Commit**

```bash
git add -A api
git commit -m "fix(api): the turn is a ledger state machine; busy is its fold

Every close names its turn. Delete the stall closer, quiet/idle timers,
staleness nets, switch force-kill and the chat aggregate's turn state.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** all six `TestRegression_Symptom4_*` green; no `time.After`/`time.NewTimer`/ticker in `turn/` or `runner/` decides a turn; `domain.Chat` carries no turn state.

---

### Task 6: One hook ingress

**Goal:** A single `ingress` package receives every hook (HTTP route, relay route, codex api pump): it resolves the runner through the live table, parses through the descriptor, holds the chat's ingest lock for the whole hook, and replies only after its rows committed. Anything it cannot place is answered non-2xx and dead-lettered (R14). The per-runner hook gate, the startup barrier, the delivery-id dedup and the 3 s step-out are deleted.

**Files:**
- Create: `api/internal/app/usecases/chat/internal/ingress/ingress.go`, `deadletter.go` and `ingress_internal_test.go`
- Modify: `api/internal/adapter/store/ledger/{schema.go,store.go,store_test.go}` — dead-letter table and `DeadLetter`, `DeadLetterCount`
- Modify: `api/internal/app/usecases/chat/internal/turn/{ingest.go,turns.go,types.go}` — `ingestResolvedHook`'s dispatch is called by `ingress`; `IngestHook`, `IngestHookDelivery`, `TakeHookReply` removed from `TurnUsecase`
- Modify: `api/internal/api/v0/endpoints/chat/handlers/{hooks.go,handlers.go}` — `HTTPHook` and `Hooks` call `Ingress.Hook`; `api/internal/app/usecases/chat/internal/runner/apiconn.go` — the pump calls `Ingress.Hook` with the api channel and keeps reading after an error (it is already recorded)
- Modify: `api/internal/api/v0/endpoints/health/` handler and its test — `chatDeadLetters` (count) on the health response
- Modify: `api/internal/app/usecases/chat/{assembly.go,chat.go}` — `PurgeChat` removes the chat's live entry from the table before terminating its CLI, so later hooks cannot resolve to the deleted chat
- Delete: `api/internal/app/usecases/chat/internal/turn/delivery.go`, `api/internal/app/usecases/chat/internal/turn/internal/dedup/`, `api/internal/app/usecases/chat/internal/shared/inflight/` (the remaining `pending` barrier and `gate`; the package is gone after this task), the `AwaitOpen` 3 s wait and gate step-out in `turn/message.go`, `api/tests/integration/agent/barriers_test.go`, `api/internal/app/usecases/chat/hook_ingest_bench_test.go` cases that exercise the barrier
- Test (black box): extend `api/tests/agent_home_hook_test.go`; create cases in `api/tests/regression_chat_turn_test.go`

**Interfaces:**
- Consumes: live table (`RunnerByID`), `engineagents.Agent.ParseHook`, T2–T5 decisions.
- Produces:

```go
package ingress
var (
	ErrUnknownRunner = fmt.Errorf("ingress: unknown runner: %w", apperr.ErrNotFound)      // 404
	ErrUnmappable    = fmt.Errorf("ingress: unmappable hook: %w", apperr.ErrUnprocessable) // 422, already mapped in api/libs/status.go
)
type Reply struct {
	Body []byte // stdout the CLI receives (a steer); nil = empty 200
	Ask  string // non-empty: the response is held for this ask (answer desk), as today
}
func (i *Ingress) Hook(ctx context.Context, runnerID, provider, event string,
	raw []byte, channel engineagents.Channel) (Reply, error)

// adapter/store/ledger
type DeadLetter struct{ ChatID, RunnerID, Provider, Event, Reason string; Payload []byte }
func (s *Store) DeadLetter(ctx context.Context, d DeadLetter) error // payload truncated at 65536 bytes, truncated=1
func (s *Store) DeadLetterCount(ctx context.Context) (int, error)
```

```sql
CREATE TABLE ledger_dead_letters (
  id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, chat_id TEXT NOT NULL DEFAULT '',
  runner_id TEXT NOT NULL, provider TEXT NOT NULL, event TEXT NOT NULL, reason TEXT NOT NULL,
  payload BLOB NOT NULL, truncated INTEGER NOT NULL);
```

**Ingress contract** (report §4.3, in this order):
1. `RunnerByID` miss → `DeadLetter{reason: "unknown_runner"}`, `ErrUnknownRunner`. A bad token stays 401 in the handler (no dead letter: an unauthenticated body is not a fact).
2. Resolve the descriptor; `ParseHook`. A variant mismatch (`inbound.VariantMismatchError`: a `when:` that selects another variant of the same wire hook, e.g. every Notification is posted to both `/notification` and `/idle`) is a declared filter: `Reply{}` (200), no record. `ErrForeignConversation` → `DeadLetter{reason: "foreign_conversation"}`, `Reply{}` (200). `ErrHookUndeclared`, `ErrRequiredFieldMissing`, any other parse error → in one `Store.Append` a `marker{dead_letter, detail: "<event>: <error>"}` row, then `DeadLetter{reason: "unmappable"}`, `ErrUnmappable`.
3. Take the chat's ingest lock (a per-chat `sync.Mutex` map in `ingress`; lock order: ingest lock → ledger writer → snapshot), run the existing dispatch (`session_start`, turn lifecycle, observations), release.
4. Return only after step 3's rows committed. Asks keep today's held response: `Hook` returns `Reply{Ask}` and the handler holds the HTTP response after `Hook` returned — never under the ingest lock, so a held permission never blocks the chat's other hooks.
Surface-gated events stay a declared filter (R14) and return `Reply{}`.

- [ ] **Step 1: Tests**

```go
// adapter/store/ledger/store_test.go
func TestStore_DeadLettersKeepThePayloadTruncatedAt64KiB(t *testing.T)
// 70000-byte payload → stored length 65536, truncated; count == 1

// ingress_internal_test.go (fakes for the live table and the descriptor through the
// package's constructor arguments)
func TestHook_UnknownRunnerIsNotFoundAndDeadLettered(t *testing.T)
func TestHook_UnmappablePayloadRecordsAMarkerAndIsRefused(t *testing.T)
func TestHook_ForeignConversationIsAcknowledgedAndDeadLettered(t *testing.T)

// api/tests/regression_chat_turn_test.go
func TestRegression_ConcurrentHooksForOneChatAllLand(t *testing.T)
// 20 goroutines each post tool_pre then tool_post for their own tool id → all 200;
// GET …/activity has 20 tool calls, all status "ok"
func TestRegression_HooksAfterChatDeleteAreRefusedNotResurrected(t *testing.T)
// l := AwaitLaunch; DELETE the chat; l.PostHook(tool_pre) → 404; GET /health
// chatDeadLetters ≥ 1; GET …/chats/<id> → 404
func TestRegression_AnUnmappableHookIsRefusedVisibly(t *testing.T)
// post user_prompt {"session_id":"s"} with no prompt (claude.yaml requires
// [session_id, message]) → 422; health chatDeadLetters +1
func TestRegression_AHookRepliesOnlyAfterItsRowsAreReadable(t *testing.T)
// post user_prompt; on its response, GET …/messages already contains it
```

`TestRegression_AgentHomeCallbacksReachDaemon`, `TestRegression_InternalSessionHooksAreDroppedNotFailed` (now: acknowledged and dead-lettered), `TestRegression_HooksDeliveryOfAChannelSplit*` must stay green.

- [ ] **Step 2: Run; the concurrency, delete and 422 cases FAIL today** (`go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(ConcurrentHooksForOneChat|HooksAfterChatDelete|AnUnmappableHook|AHookRepliesOnlyAfter)' ./tests/`; today an unmappable hook is a silent 200 and a deleted chat's runner is displaced, not unknown)

- [ ] **Step 3: Implement `ingress`, the dead-letter table, the handler and pump wiring; delete the gate, barrier, dedup and step-out**

- [ ] **Step 4: Run to green** — the new tests, the three must-stay-green families above, `TestRegression_Symptom(2|4)_`, `TestFakeProvider_`; `go test -tags noEmbed ./internal/app/... ./internal/adapter/... ./internal/api/...`; `make lint deadcode`; `test ! -d api/internal/app/usecases/chat/internal/shared/inflight`.

- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "fix(api): one hook ingress that records before it replies and dead-letters what it cannot place

Delete the per-runner hook gate, startup barrier, inert delivery dedup and step-out.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** no code path answers a hook 2xx without either committed rows or a dead-letter entry; the `inflight` package is gone.

---

### Task 7: The transcript is the ledger (symptom 3)

**Goal:** Every content fact a hook carries becomes a row through one pure mapper; the old read endpoints (`/messages`, `/activity`, `/activity/:toolId/payload`, `/choices`, the `get_chat_log` MCP tool, the handoff) and the old live frames (`message_delta`, `plan`, `choice`, `compaction_*`) are projections of those rows. A subagent's own tool calls are recorded under its scope instead of dropped. The activity aggregate and the `turn` package are deleted.

**Files:**
- Create: `api/internal/domain/ledger/{message.go,tool.go,ask.go,subagent.go,plan.go}` and their `_test.go`; `marker.go` gains the T7 marker kinds
- Create: `api/internal/app/usecases/chat/internal/ingress/{rows.go,rows_test.go,live.go,live_internal_test.go,steer.go}` — the mapper, the ephemeral live-text buffer, steer (moved from `turn/steer.go`)
- Create: `api/internal/app/usecases/chat/internal/transcript/{transcript.go,messages.go,activity.go,choices.go}` and `transcript_test.go`, `messages_test.go`, `activity_test.go`
- Move: `api/internal/app/repositories/chat/activity/internal/store/internal/content/content.go` → `api/internal/adapter/store/ledger/blobs.go` (same on-disk directory `state/content/`, same sha256 refs), with its test
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema/vocabulary.yaml` — new canonical in-event `message_complete` {`session_id`, `turn_id`, `message_id`, `text`, `phase`}; `codex.yaml` api channel maps it from `item/completed` when `item.type == agentMessage` (`message_id: item.id`, `text: item.text`, `phase: item.phase`); `claude.yaml` lists it in `not_emitted`; `models.CanonicalEvent` gains `Complete *MessageComplete`
- Modify: `api/internal/app/usecases/chat/{assembly.go,chat.go,turn.go,answers.go,aliases.go,runner.go}`, `internal/conversation/{chatlog.go,handoff.go,conversation.go}`, `internal/shared/answerdesk/{answerdesk.go,types.go}` (its `Ledger` port becomes an ask resolver that appends `ask_resolved`), `internal/shared/tools/` readers of activity, `internal/runner/{stop.go,switch.go,promptswitch.go,compact.go,apiconn.go}` (markers instead of activity interruptions)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/{activity.go,activity_components.go,answers.go,chats.go}` — read through the transcript projection; response shapes unchanged
- Modify: `api/internal/app/repositories/container.go`, `api/internal/app/container.go`, `api/internal/adapter/container.go`, `api/internal/app/upgrade.go`, `api/internal/app/usecases/container.go` — drop `AgentActivity`, `axAgentActivity`, the `agent_activity` event/snapshot/read DBs
- Move then delete: `api/internal/app/usecases/chat/internal/turn/` — `ingest.go`, `conversation.go`, `nested.go`, `tool_diff.go`, `attachments.go` move into `ingress` (attachments into `api/internal/app/usecases/chat/attachments.go`); everything else (`activity_reads.go`, `asks.go`, `choices.go`, `compaction.go`, `livetext.go`, `message.go`, `observation.go`, `steer.go` (moved), `turn.go`, `turn_open.go`, `turns.go`, `types.go`, `internal/stream/`) is deleted with its tests
- Delete: `api/internal/app/repositories/chat/activity/` (whole package); `api/internal/app/usecases/chat/choicewatch.go` and test; the activity-aggregate cases of `api/internal/app/usecases/chat/{turn_test.go,turn_export_test.go,answers_test.go,harness_test.go}` (R17: delete, do not split); `domain.ChatActivity`, `ActivityDelta`, `MaxOpenPerTurn`, `OpenTurnOrders` in `api/internal/domain/chat_activity.go` (keep the `Activity*` read shapes the projection returns until S2)
- Test (black box): extend `api/tests/regression_agent_message_stream_test.go`, `api/tests/regression_agent_nested_choice_test.go`, `api/tests/regression_agent_choice_frame_test.go`

**Interfaces:**
- Consumes: T2–T6.
- Produces, `package ledger` bodies exactly as the index's vocabulary table (rows added in T7), plus:

```go
type MessageKey struct{ TurnID, Scope, MessageID string }
type MessageText struct {
	Key       MessageKey
	Text      string
	Settled   bool  // stream complete, a complete row exists, or its turn ended
	FirstSeq  int64 // seq of its first row
	SettleSeq int64 // seq of the row that settled it (0 while unsettled)
	Phase     string
}
// Messages folds assistant_text rows: text is the contiguous stream 0..k when
// chunk k is final; else the complete row (index -1); else the chunks present, in
// index order, duplicates by index ignored.
func Messages(rows []Row) []MessageText
```

- Produces, `ingress`:

```go
// Map decides the rows for one parsed event against the chat's state. It does no
// I/O: payload blobs are written before the Do that calls it (content-addressed,
// idempotent), and their refs are passed in.
func Map(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) (rows []ledger.Row, fx Effects)
type Source struct{ RunnerID, Provider string }
type BlobRefs struct{ Input, Result string }
type Effects struct {
	Reply      []byte      // steer stdout
	HoldAsk    string      // ask id whose response is held
	Live       *LiveDelta  // ephemeral text for the old message_delta frame
}
```

- Produces, `transcript`:

```go
// Projector keeps the old read shapes of up to 64 chats (LRU), advanced by ledger
// publication and rebuilt from Store.Read on a miss. S2 deletes it.
func New(store *ledgerstore.Store) *Projector
func (p *Projector) Apply(chatID string, rows []ledger.Row) // the publication subscriber
func (p *Projector) Messages(ctx context.Context, chatID string, after, before, limit int) (domain.LedgerPage, error)
func (p *Projector) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error)
func (p *Projector) Activity(ctx context.Context, chatID string) (Activity, error) // tools, subagents, interruptions, choices
func (p *Projector) PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
```

**Mapper table** (scope `s` = `ev.Subagent.ID` when set; a codex nested child keeps today's `nested.go` routing, now emitting rows with `Scope` = the child thread id):

| Canonical event | Rows |
|---|---|
| `message_delta` with `Sequenced` | `assistant_text{messageId, index, final, text}`, unless a row with the same (turn, scope, messageId, index) exists |
| `message_delta` unsequenced, `reasoning_delta`, `tool_output_delta`, `diff_update` | no row; `fx.Live` (ephemeral; R7) |
| `message_complete` | `assistant_text{messageId, index: -1, text, phase}` |
| `turn_stop` with a message | `assistant_text{index: -1, text, messageId: the running turn's last main-scope message id, or "final-"+turnID when it has none}` before T5's `turn_ended`; the turn's accumulated live reasoning, if any, as one `reasoning{messageId, text}` row |
| `tool_pre` / `tool_post` / `tool_fail` | `tool_call{phase, toolId, name, kind, target, locations, inputRef, resultRef, status, error, durationMs, diff, spawnsScope: ev.Tool.NestedSessionID}` in scope `s`; an empty tool id is minted as `"tool-"+uuid` on pre only |
| `subagent_pre` / `subagent_post` | `subagent{phase: start|stop, type}` with `Scope` = the subagent id |
| `permission` / `elicitation` | `ask{askId: uuid, …ChoicePrompt fields, toolId: the open tool in scope s with the same tool name, else ""}`; `fx.HoldAsk` when answerable |
| answer through Crowbar / CLI hung up / the bound tool's post / its turn ended | `ask_resolved{answered|proceeded|proceeded|abandoned}` — the last names only that turn's asks |
| `plan_update` | `plan{op: replace, items}` in scope `s` |
| `notification` | `marker{notification, detail: message}` |
| `compact_pre` / `compact_post` | `marker{compaction_started}` / `marker{compaction_ended}`; a `turn_stop` whose provider turn id is the open compaction's is `marker{compaction_ended}` only (replaces `turn/compaction.go`'s latch) |
| Crowbar's own Stop, provider switch, model/effort change | `marker{stopped}`, `marker{provider_switched, detail}`, `marker{model_changed, detail}`, `marker{effort_changed, detail}` |

**Projection rules (old wire, unchanged shapes):**
- Message `Sequence` = `SettleSeq` (so `?after=` paging still returns a message once it settles); `DisplayOrder` = `FirstSeq`; `ItemIndex` = 0; only settled messages are listed (a non-final stream lives only on the socket, as today).
- User message `TurnID` = its `requestId` for Crowbar prompts (the client matches its queue on it), else the ledger turn id.
- Assistant message `TurnID` = `"<turnId>:<k>"`, `k` = its ordinal among the turn's main-scope messages by `FirstSeq`. A main-scope tool's `TurnID` = `"<turnId>:<k>"` with `k` = the number of that turn's main-scope messages whose `FirstSeq` precedes the tool's pre row. Scoped tools carry `TurnID ""` and `SubagentID` = scope.
- A tool still running when its turn ended reads `status: "abandoned"` (derived at read time from the named turn; never a sweep).
- Interruptions: each marker is `{ID: "m-<seq>", Seq, DisplayOrder: seq, Kind, Detail, At}` with kinds `compaction` (resolved at its `compaction_ended`), `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification`, `session_changed`, `session_conflict`, `dead_letter`; each ask also yields an interruption of its kind, resolved at its `ask_resolved`.
- Choices: one per ask; `Answerable` from the answer desk as today.
- The turn's latest main-scope plan is attached to the turn's last assistant message (`ActivityTurn.Plan`), as today.
- Cache coherence without a lock across the two: `Apply` on a chat that is not cached does nothing; `Apply` on a cached chat whose head is not `seq − 1` of the first row drops that chat's entry (it is rebuilt from `Store.Read` on the next read). A read that misses builds from `Store.Read`, then installs only if no entry appeared meanwhile.

- [ ] **Step 1: Unit tests** (`package ledger_test`, `package ingress`, `package transcript_test`)

```go
func TestMessages_CompleteFinalStreamWins(t *testing.T)        // idx0,idx1(final) + complete "TAIL" → stream text
func TestMessages_CompleteRowFillsAGappedStream(t *testing.T)  // idx0,idx2(final) + complete → complete text
func TestMessages_LateFinalChunkAfterTheCompleteRow(t *testing.T) // idx0, complete(full), idx1(final) → full text, never the tail
func TestMessages_DuplicateChunkIndexesAreIgnored(t *testing.T)
func TestMap_SubagentToolCallsAreRecordedUnderTheirScope(t *testing.T) // ev.Subagent.ID "A" → tool_call Scope "A"
func TestMap_UnsequencedDeltaWritesNoRow(t *testing.T)
func TestMap_AnAskBindsToTheOpenToolOfItsScope(t *testing.T)
func TestMap_ACompactionTurnStopEndsTheCompactionNotTheTurn(t *testing.T)
func TestProjector_ToolsAttachToTheReplyThatFollowsThem(t *testing.T)
// user, msg1(final), tool, msg2(final) → tool.TurnID == msg2.TurnID != msg1.TurnID
func TestProjector_AToolOpenWhenItsTurnEndedReadsAbandoned(t *testing.T)
func TestProjector_RebuildsFromRowsOnAMiss(t *testing.T) // 65 chats touched → the first is rebuilt identically
```

- [ ] **Step 2: Symptom 3 black-box tests** — extend `api/tests/regression_agent_message_stream_test.go` (keep every existing test in it green; switch its helpers to `kit.FakeProvider` + `Launch.PostHook` if the relay route is in the way):

```go
func TestRegression_Symptom3_ATailChunkAfterStopNeverOverwritesTheReply(t *testing.T)
// replay tests/testdata/hooks/claude-subagent-session 00..34 through the claude fake
// (rewrite the recorded session id to the chat's bound one) → the assistant message of
// the turn that 28-Stop.json closes equals 28-Stop.json's last_assistant_message in full;
// after 30..34 (the next turn) it is still that full text
func TestRegression_Symptom3_ASubagentsToolCallsAreRecorded(t *testing.T)
// replay 00..10 → GET …/activity has tool calls with subagentId == the SubagentStart agent_id
func TestRegression_Symptom3_TextSaidBeforeAProcessExitsIsKept(t *testing.T)
// user_prompt, message_delta idx0 final "KEPT", then l.Kill → GET …/messages has "KEPT"
func TestRegression_Symptom3_AStoppedTurnsLateFinalTextIsRecordedNotReopened(t *testing.T)
// running turn, Stop, then turn_stop{last_assistant_message:"LATE"} from the same runner
// → GET …/messages has "LATE"; working stays false
```

`TestRegression_AChoiceOpensAndClearsOverTheChatSocket`, `…ReplayedToIt`, `…SeveralChoicesRideOneFrame…`, `…RelayExpiredIsPushedAsUnanswerable`, `TestRegression_ANestedConversationsPermissionBecomesAnAnswerableCard`, `…NestedAskDoesNotChangeHowNestedToolRowsRender`, `…NestedAskAfterTheTurnClosedLeavesNoPendingCard`, `TestRegression_CompactPreAndPostPushLiveOverTheSocket`, `TestRegression_AStreamingMessageCarriesTheMomentItBegan…`, `TestRegression_ProviderSwitchRecordsAMarkerInterruption` and the rest of `regression_agent_switch_markers_test.go`, `TestRegression_AThreadReadsTheChatItHangsOff` and the rest of `regression_agent_chat_threads_test.go`, `TestRegression_TelemetrySurvives*` must stay green.

- [ ] **Step 3: Run; the tail-overwrite, subagent-tool and kill cases FAIL today**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom3_' ./tests/` → FAIL.

- [ ] **Step 4: Implement the domain folds, the mapper, the live buffer, the projector, the blob move, the descriptor event; rewire every reader; delete the activity aggregate and the `turn` package**

- [ ] **Step 5: Run to green**

Run: `cd api && go test -tags noEmbed ./internal/...` scoped to the touched trees: `./internal/domain/... ./internal/adapter/... ./internal/engine/agents/... ./internal/app/... ./internal/api/...` → PASS.
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(Symptom2_|Symptom3_|Symptom4_|EveryAssistantMessage|AMessageIsVisible|AGrowingMessage|ProviderWithNoStreaming|TheTerminatingHook|ALossyTerminatingHook|AStreamedReply|AChoice|SeveralChoices|ANested|CompactPreAndPost|AStreamingMessageCarries|ProviderSwitch|SwitchingTo|SelectionChange|SwitchingProviders|AFailureFromBefore|AConcurrentFailure|AThread|AChatCannot|SiblingThreads|AFiledChat|FoldersAre|ReParenting|AChatCreated|TelemetrySurvives|ConcurrentHooks|HooksAfterChatDelete)|TestFakeProvider_' ./tests/` → PASS.
Run: `cd api && make descriptor-check lint deadcode && go vet -tags noEmbed ./...`; `test ! -d api/internal/app/repositories/chat/activity && test ! -d api/internal/app/usecases/chat/internal/turn`.

- [ ] **Step 6: Commit**

```bash
git add -A api
git commit -m "fix(api): the transcript is ledger rows; nothing a hook says is overwritten or dropped

A late chunk can no longer replace a finished reply, and a subagent's tool
calls are recorded under its scope. Delete the activity aggregate and the turn package.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** all four `TestRegression_Symptom3_*` green and every listed must-stay-green test green; no `agent_activity*.db` is opened; the old endpoints answer from the projection.

---

### Task 8: Tool and MCP detail

**Goal:** Tool rows and asks carry what the hooks already send: the agent's own title for the call, the MCP server and tool, the exit code, an interrupt flag, the target from every key Claude Code itself uses, and (on asks) the tool target and input. Two generic grammar extensions make this expressible in YAML: a `pattern:` transform and a composite `kind_map` key.

**Files:**
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema/vocabulary.yaml` — `tool_pre`/`tool_post`/`tool_fail` gain optional `tool_title`, `mcp_server`, `mcp_tool`, `exit_code`, `interrupted`; `permission`/`elicitation` gain `tool_target`, `tool_input`, `mcp_server`
- Modify: `api/internal/engine/agents/internal/spec/{expr.go,expr_test.go}` and `api/internal/engine/agents/internal/mapping/{mapping.go,mapping_test.go}` — `{from: <path>, pattern: '<regex with group v>'}` and `kind_map` keyed by `{join: [path, path]}` (values joined with `/`)
- Modify: `api/internal/engine/agents/internal/protocol/internal/translate/inbound/{hooks.go,choice.go}` and tests; `api/internal/engine/agents/internal/models/event.go` — `ToolEvent{Title, MCPServer, MCPTool string; ExitCode *int; Interrupted bool}`, `ChoicePrompt{ToolTarget string; ToolInput []byte; MCPServer string}`
- Modify: `claude.yaml`, `codex.yaml` (values below) and their fixture assertions in `api/internal/engine/agents/internal/protocol/internal/descriptor/{fixture_test.go,v3_test.go}`
- Modify: `api/internal/domain/ledger/{tool.go,ask.go}`, `api/internal/app/usecases/chat/internal/ingress/rows.go` — the new fields on `tool_call`/`ask`; the ask binds to the open tool of its scope with the same tool name **and** canonical-JSON-equal input (additions design §F) and falls back to name only when the ask carries no input
- Modify: `api/internal/api/v0/dto/agent.go` — `AgentToolCallDTO` gains `title`, `mcpServer`, `mcpTool`, `exitCode` (all `omitempty`; additive); `AgentChoiceDTO` gains `toolTarget`, `mcpServer` (additive) — S2 renders them

**Descriptor values** (exact):

```yaml
# claude.yaml — tool_pre (tool_post/tool_fail inherit the same map keys they already share)
kind_map: { Read: read, Write: edit, Edit: edit, MultiEdit: edit, NotebookEdit: edit, Bash: execute,
  Grep: search, Glob: search, WebFetch: fetch, WebSearch: search, Agent: subagent, Task: subagent,
  TaskCreate: plan, TaskUpdate: plan, TaskList: plan, TaskGet: plan, ToolSearch: meta,
  SubagentHandback: meta, AskUserQuestion: ask }
map:
  tool_title:  { first_present: [tool_input.description, tool_input.subject] }
  tool_target: { first_present: [tool_input.file_path, tool_input.notebook_path, tool_input.command,
                                 tool_input.url, tool_input.pattern, tool_input.query, tool_input.taskId] }
  mcp_server:  mcp_server.name
  mcp_tool:    { from: tool_name, pattern: '^mcp__.+?__(?P<v>.+)$' }
# claude.yaml — tool_fail adds:   interrupted: is_interrupt
# claude.yaml — permission adds:  tool_target (same first_present), tool_input: tool_input, mcp_server: mcp_server.name

# codex.yaml — api channel tool_pre/tool_post
map:
  tool_kind:   { first_present: ["item.commandActions[0].type", item.type] }
  tool_target: { first_present: ["item.commandActions[0].command", item.query, "item.changes[0].path"] }
  tool_title:  { first_present: [item.prompt] }
  mcp_server:  item.server
  mcp_tool:    item.tool
  exit_code:   item.exitCode
kind_map: { read: read, listFiles: search, search: search, unknown: execute, commandExecution: execute,
  fileChange: edit, mcpToolCall: mcp, webSearch: search, collabAgentToolCall: subagent }
# codex.yaml — hooks channel tool_pre
map:
  tool_target: { first_present: [tool_input.command, tool_input.file_path] }
  tool_title:  tool_input.message
  mcp_tool:    { from: tool_name, pattern: '^mcp__.+?__(?P<v>.+)$' }
```

Field paths come from real captures (claude 2.1.302, codex 0.158.0): Claude Bash `tool_input.description`; MCP `tool_name: "mcp__echo__shout"`, `mcp_server: {name: "echo", source: "dynamic"}`; `PostToolUseFailure.is_interrupt`; codex `commandExecution.commandActions[]{type: read|listFiles|search|unknown, command}`, `exitCode`; `mcpToolCall{server, tool, arguments}`; codex hooks `mcp__echo__shout` (no `mcp_server` field). Add one recorded fixture per new mapping under `api/internal/engine/agents/internal/protocol/testdata/fixtures/{claude,codex}/` (scrub paths to `/ws` and `/h`, drop `transcript_path` home segments and any token) and assert it in `fixture_test.go`.

- [ ] **Step 1: Tests**

```go
func TestPatternTransform_ExtractsGroupV(t *testing.T)            // spec/expr_test.go
func TestKindMap_JoinKey(t *testing.T)                            // mapping_test.go: join of item.type,item.tool
func TestClaudeDescriptor_MapsMCPServerToolAndTitle(t *testing.T) // fixture PreToolUse.mcp.json → server "echo", tool "shout"
func TestClaudeDescriptor_MapsBashDescriptionAsTitle(t *testing.T)
func TestClaudeDescriptor_ToolFailCarriesInterrupt(t *testing.T)
func TestCodexDescriptor_CommandActionsGiveKindTargetAndExitCode(t *testing.T)
func TestMap_AnAskBindsByToolNameAndCanonicalInput(t *testing.T)  // ingress: two open Bash tools, ask input equals the second's → toolId of the second
// api/tests/regression_chat_tool_detail_test.go (new; the stream test file is over 400 lines)
func TestRegression_AnMCPToolRowNamesItsServerAndTool(t *testing.T) // claude fake: tool_pre mcp__echo__shout + mcp_server → GET …/activity toolCall mcpServer "echo", mcpTool "shout"
```

- [ ] **Step 2: Run; they FAIL** (`go test -tags noEmbed ./internal/engine/agents/... ./internal/app/usecases/chat/internal/ingress/...`).

- [ ] **Step 3: Implement the grammar, the vocabulary, the mappings, the fields, the ask binding; delete `features.collab_agents=true` from `codex.yaml` only in T9**

- [ ] **Step 4: Run to green** plus `make descriptor-check lint deadcode`, `go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(AnMCPToolRow|Symptom3_|AChoice|ANested|HooksDeliveryOfAChannelSplit)' ./tests/`.

- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "feat(api): tool rows carry title, MCP server/tool, exit code and interrupt from the descriptors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** no Go file names a provider tool (`Bash`, `mcp__`, `commandActions`) outside tests; the new fields reach the ledger rows and, additively, the old DTO.

---

### Task 9: Subagents and plans as entities

**Goal:** A subagent is an entity with its own lifecycle, bound to the tool call that spawned it, independent of the parent turn; its result, title, type and usage come from the descriptors; Claude's todo hooks and Codex's plan become plan rows; busy no longer needs the `async_work` level. Codex child frames that arrive before their bind are held in the R15 buffer, not dropped.

**Files:**
- Modify: `vocabulary.yaml` — `tool_pre`/`tool_post`/`tool_fail` gain `scope_id`, `spawns_scope`, `addresses_scope`; `subagent_pre`/`subagent_post` gain `title`, `status` (with `status_map`), `result`, `usage.tokens`, `usage.tool_uses`, `usage.duration`; new in-events `plan_step` {`scope_id`, `step_id`, `text`, `detail`, `status`} and `subagent_result` {`scope_id`, `status`, `result`, `usage.*`}; `in:` accepts a list (several wire events feed one canonical event, as `ask:` already does on the codex api channel)
- Modify: `api/internal/engine/agents/internal/spec/{expr.go,expr_test.go}`, `mapping/mapping.go` — `{json: <path>, path: <path>}` transform (decode a JSON-encoded string, then select)
- Modify: `api/internal/engine/agents/internal/spec/injected.go` and `translate/inbound/hooks.go` — `injected_prompts` with `kind: subagent_result` produce a `subagent_result` event instead of a user prompt
- Modify: `claude.yaml`, `codex.yaml` (values below); delete `features.collab_agents=true` from `codex.yaml`
- Modify: `api/internal/engine/agents/internal/models/event.go` — `SubagentEvent{ID, AgentType, Title, Status, Result string; Usage *SubagentUsage}`, `PlanStepEvent`, `ToolEvent{ScopeID, SpawnsScope, AddressesScope string}` (`NestedSessionID` renamed to `SpawnsScope`)
- Modify: `api/internal/domain/ledger/{subagent.go,plan.go,state.go,turn.go}` — subagent lifecycle and `plan` upsert fold; `Busy()` drops the `async_work` clause; `TurnEnded.AsyncWork` deleted
- Modify: `api/internal/app/usecases/chat/internal/ingress/{rows.go,nested.go}` — scope rows for every child event; the R15 buffer
- Modify: `api/internal/app/usecases/chat/internal/transcript/activity.go` — `ActivitySubagent` from the entity; the subagent's `Messages` from its `result` and its scope's settled text
- Modify: `api/internal/api/v0/dto/agent.go` — `AgentSubagentDTO` gains `title`, `status`, `toolUses`, `tokens` (additive)
- Modify: `api/tests/kit/fakeprovider.go` — `ClaudeHookRoutes` becomes `func ClaudeHookRoutes(hookEventName, toolName string) []string`, mirroring the new registrations (`TaskCreated`/`TaskCompleted` → `plan_step`; `PostToolUse` → `tool_post`, plus `plan_step` only when `toolName == "TaskUpdate"`); `ReplayHooks` takes that function

**Interfaces:**

```go
package ledger
type SubagentStatus string // starting | running | done | failed | interrupted | abandoned
// Next: starting→running|abandoned, running→done|failed|interrupted, starting→failed.
func (s SubagentStatus) Next(to SubagentStatus) (SubagentStatus, bool)
type Subagent struct {
	Scope, SpawnToolID, Type, Title, Result string
	Status    SubagentStatus
	TurnID    string // the turn that spawned it; it may outlive that turn
	StartedAt time.Time
	EndedAt   time.Time
	Usage     SubagentUsage
}
func (s State) Subagents() []Subagent // non-terminal ones; terminal ones are read from rows
```

**Lifecycle rules** (additions design §C; each is a row, never a timer):
1. A `tool_call pre` whose `kind` is `subagent` creates the entity `starting`: `subagent{phase: start, spawnToolId, type, title}` with `Scope ""`; the entity is keyed by `spawnToolId` until bound.
2. Bound by whichever arrives first — `subagent_pre` with `scope_id`, the spawning call's `tool_post` `spawnsScope`, or (codex api) `spawnsScope` = `receiverThreadIds[0]` — as one `subagent{phase: bound, spawnToolId}` row whose `Scope` is the real scope → `running`. A `subagent_pre` with no matching starting entity (a subagent the main agent did not spawn through a tool call) creates the entity directly as `running`.
3. `subagent_post` / `subagent_result` / the child's `turn_stop` (codex) → `done` (or `failed` by `status_map`), with `result` and `usage`.
4. The user's Stop or a runner exit → every `starting`/`running` subagent of the chat → `interrupted`. A parent turn that ends while a subagent is still `starting` → that one `abandoned`.
5. A `subagent{phase: start}` row with a non-empty `Scope` (written by T7, before this task) folds as started and bound.
6. A `subagent_post` with no prior start and an empty `agent_type` is a harness-internal agent: no row (it is not a fact about this chat; dead-letter reason `internal_subagent`).
7. R15: a frame whose scope is not yet bound waits in a per-chat buffer of 256 frames, drained in arrival order on bind; on the parent turn's end the remainder is dead-lettered (`unclaimed_scope`); a full buffer dead-letters the oldest.
8. Codex `wait`/`closeAgent` calls (`addressesScope` set) are `meta`: recorded as tool rows, never rendered as their own card (S2).

**Descriptor values** (additions design §3.1 sketches, verified against captures):

```yaml
# claude.yaml
tool_pre:  { map: { scope_id: agent_id } }               # plus T8's map
tool_post: { map: { spawns_scope: tool_response.agentId, scope_id: agent_id } }
subagent_pre:  { map: { scope_id: agent_id, agent_type: agent_type } }
subagent_post: { map: { scope_id: agent_id, result: last_assistant_message } }
plan_step:
  - in: TaskCreated
    map: { step_id: task_id, text: task_subject, detail: task_description, scope_id: agent_id }
    status: pending
  - in: TaskCompleted
    map: { step_id: task_id }
    status: done
  - in: PostToolUse
    when: { tool_name: TaskUpdate }
    map: { step_id: tool_input.taskId, status: tool_response.statusChange.to, text: tool_input.subject }
    status_map: { pending: pending, in_progress: active, completed: done, deleted: removed }
injected_prompts:
  - kind: subagent_result
    needle: "<task-notification>"
    scope_id: { pattern: '<task-id>(?P<v>[^<]+)</task-id>' }
    status:   { pattern: '<status>(?P<v>[^<]+)</status>' }
    result:   { pattern: '(?s)<result>(?P<v>.*)</result>' }
  - kind: subagent_result
    needle: "<agent-message"
    scope_id: { pattern: 'from="(?P<v>[^"]+)"' }
idle:
  in: Notification
  when: { notification_type: idle_prompt }   # replaces equality on the message text
# claude settings.json hook registration: add TaskCreated and TaskCompleted posting to
# {hook_url}/plan_step, and a second PostToolUse entry with "matcher":"TaskUpdate" posting to /plan_step

# codex.yaml — api channel
tool_pre/tool_post: { map: { spawns_scope: "item.receiverThreadIds[0]", addresses_scope: "item.receiverThreadIds[0]" } }
kind_map join: { join: [item.type, item.tool] }   # collabAgentToolCall/spawnAgent ≠ …/wait
# codex.yaml — hooks channel
tool_post: { map: { spawns_scope: { json: tool_response, path: agent_id } } }
subagent_pre / subagent_post: drop `unverified`
```

Before relying on `Notification.notification_type`, capture one real `idle_prompt` Notification from `claude` (scratch dir, `env -i`, cheapest model, `--safe-mode`) and add it as a fixture; if the capture does not carry the field, keep the message-equality `when:` and record that in the commit body.

- [ ] **Step 1: Tests**

```go
func TestSubagentStatus_Next(t *testing.T)
func TestFold_ASubagentOutlivesItsParentTurnAndKeepsTheChatBusy(t *testing.T)
// tool pre(kind subagent) T1, subagent start bound A, turn_ended T1 → Busy (A running);
// subagent stop A → !Busy
func TestFold_AnUnboundSubagentIsAbandonedWhenItsTurnEnds(t *testing.T)
func TestFold_StopInterruptsEveryLiveSubagent(t *testing.T)
func TestFold_PlanUpsertAndReplace(t *testing.T)
func TestJSONTransform_DecodesAStringField(t *testing.T)
func TestInjectedSubagentResultIsNotAUserPrompt(t *testing.T) // claude fixture UserPromptSubmit.AgentMessage.json → subagent_result
func TestIngress_ChildFramesBeforeTheBindAreHeldThenDrained(t *testing.T)
// api/tests (black box, claude fake replaying claude-subagent-session)
func TestRegression_ABackgroundSubagentKeepsTheChatWorkingUntilItStops(t *testing.T)
// replay through 11-Stop.json (parent Stop while the agent runs) → working true;
// replay 17-SubagentStop.json → the subagent finished; after the last SubagentStop working false
func TestRegression_ATodoListBecomesPlanRows(t *testing.T)
// post TaskCreated ×2 and a TaskUpdate to in_progress through /plan_step → the chat feed's
// plan frame carries both items, one "active"
```

`TestRegression_CodexBackgroundedSubagentKeepsChatWorking` (live, skipped without codex) must still compile.

- [ ] **Step 2: Run; they FAIL.**

- [ ] **Step 3: Implement the grammar, vocabulary, descriptor values, entity fold, ingress routing and buffer, projection; delete `AsyncWork` end to end (`TurnEnded.AsyncWork`, `CanonicalEvent.AsyncWork`, the `async_work` descriptor key and its vocabulary entry) and the `routeNestedSubagentEvent` special path**

- [ ] **Step 4: Run to green** — `go test -tags noEmbed ./internal/engine/agents/... ./internal/domain/ledger/... ./internal/app/usecases/chat/...`; `go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(ABackgroundSubagent|ATodoList|Symptom3_|Symptom4_|ANested|AStreamingMessageCarries)' ./tests/`; `make descriptor-check lint deadcode`.

- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "feat(api): subagents and plans are ledger entities fed by the descriptors

Busy no longer leans on an async-work level; codex child frames that precede
their bind are held, not dropped.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** no subagent event is dropped silently; a background subagent keeps the chat busy until its own stop; `async_work` no longer exists anywhere.
