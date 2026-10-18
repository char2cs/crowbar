# S1c — Turns, one ingress, and the transcript as rows: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The turn is a ledger state machine with a Crowbar-minted id, and "busy" is its fold (T5, symptom 4). One hook ingress resolves, records and only then replies, and dead-letters what it cannot place (T6). Every transcript fact is a row, and the activity aggregate is deleted (T7, symptom 3). Tool rows carry the MCP and tool detail the hooks already send (T8). T9 (subagents and plans) is in `docs/plans/2026-10-17-s1d-feed-migration-gate-plan.md`.

**Architecture:** The index is `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. These tasks require T1–T4 committed: the ledger store (`ledgerstore`: `Do`, `Append`, `State`, `Read`, `OpenChats`, `Subscribe`, `Forget`, `SessionOwner`) and its fold; prompts as rows; chat-owned conversations (`session_bound`, `marker`); the in-memory live table (`Runners.RunnerByID`, `LiveRunnerForChat`) with `runner_started`/`runner_exited` rows; `snapshot.LedgerFacts{Conversations, LastExit}` with `ApplyLedger`; and the `kit.FakeProvider` harness.

**Tech Stack:** Go, SQLite, testify, `api/tests`, the descriptor grammar under `api/internal/engine/agents/internal/{spec,mapping,protocol}`.

**Spec:** architecture §3 (journal invariants), §4 (hook ingress paragraph, canonical vocabulary), §5 (turn diagram); report §2 rows "Mid-turn messages lost" and "Prompt sent, nothing renders", §4.3 ingress contract; additions design §3.1, §3.2, §4.C, §4.F.

---

### Task 5: The turn is a state machine in the ledger (symptom 4)

**Goal:**
- `turn_started` / `turn_ended` / `turn_failed` rows own the turn, and `State.Busy()` is the only busy answer.
- Every close names its turn. Crowbar records a Stop at once.
- A runner's exit and a daemon restart close the running turn.
- Deleted: the chat aggregate's turn state, the in-flight turn registry and work mirror, the stall closer, the message-quiet/idle/stall timers, the 2 h staleness nets and the switch force-kill.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- No timer decides a turn. Crowbar writes `turn_ended{interrupted}` itself when it sends an interrupt. A provider idle hook closes a running turn at once.
- Behaviour changes, accepted provisionally on 2026-10-18 (the user can override them before this task lands):
  - Escape pressed inside the TUI leaves the chat working until the idle notification or Stop;
  - a switch while a turn runs is refused with 409;
  - Stop on an api-channel chat returns at once and never kills.
- `TurnStatus` has no `retracted` value (S2 pre-flight amendment).
- termwait's 2 s screen poll stays. Its replacement belongs to S6 (pre-flight R26). This task keeps only the terminal-prompt and terminal-notice detection.
- Per-chat serialisation of user-initiated spawns stays on today's `inflight` gate until T6 moves it onto ingress's per-chat lock (pre-flight R22). Do not add a second per-chat lock here.
- Oversize files (pre-flight R8). Each edit must be net-shrinking; otherwise split the file below 500 in this task (trimming narrative comments to ≤3-line whys counts as offset):
  - `api/internal/app/usecases/workspace/internal/hierarchy/worktree.go` (1,826);
  - `api/internal/api/v0/container.go` (904);
  - `api/internal/app/repositories/container.go` (851);
  - `api/internal/app/usecases/container.go` (1,080);
  - `api/internal/app/repositories/chat/event_store.go` (698).
  - `chat_test.go` (2,605) and `termwait_test.go` only shrink.
  - A split wiring file's new path joins `scripts/ratchet/modules.json` `wiring`.

**Files:**
- Create: `api/internal/domain/ledger/turn.go` and `turn_test.go`
- Modify: `api/internal/domain/ledger/{row.go,state.go,state_test.go}` — kinds `KindTurnEnded`, `KindTurnFailed`; the turn fold; `Busy`; `HasOpenWork` includes a running turn; `Check` requires a `TurnID` on both
- Modify: `api/internal/app/usecases/chat/internal/turn/{turn.go,turn_open.go,turns.go,types.go,ingest.go,observation.go,message.go,choices.go}` — lifecycle decisions write rows (below). The activity record keeps receiving its turn commands until T7.
- Modify: `api/internal/app/usecases/chat/internal/runner/{prompts.go,steer.go,stop.go,switch.go,promptswitch.go,lifecycle.go,runner.go,types.go,terminalwait.go,errors.go}`
- Modify: `api/internal/app/usecases/chat/internal/runner/internal/termwait/{termwait.go,evaluate.go,sweep.go,run.go,termwait_test.go}`:
  - keep only terminal-prompt and terminal-notice detection;
  - a notice with `ends_turn` reports to a callback that appends `turn_failed{reason: <notice kind>}`.
- Modify: `api/internal/app/usecases/chat/internal/shared/snapshot/ledger.go` — `LedgerFacts.Busy bool` drives the snapshot's `working`. The split `api/internal/api/v0/dto/` chat file reads `AgentChatDTO.Working` from the snapshot, not from `domain.Chat`.
- Modify: `api/internal/app/repositories/chat/{event_store.go,internal/store/hub.go,internal/store/store.go,internal/store/storage.go}` — drop the turn commands and the `turn_started`/`turn_stopped` event kinds
- Modify: `api/internal/domain/chat.go` — delete `Working`, `CurrentTurnStarted`, `LastActivityAt`, `AsyncWork`, `LedgerCursor`
- Modify: `api/internal/app/repositories/container.go`:
  - delete `registerAgentWorkingProjection`;
  - export `SetAgentWorking(ctx context.Context, wsID, chatID string, working bool)` (the body of `setAgentTurn` plus the rebroadcast);
  - `api/internal/app/usecases/container.go` passes it to chat `Deps.AgentWorking` as a constructor argument.
- Modify these readers of `domain.Chat.Working` so they call the chat usecase's `ChatBusy(ctx, chatID) (bool, error)` (served from the snapshot) instead:
  - `api/internal/api/v0/endpoints/home/handlers/home.go`;
  - `api/internal/api/v0/dto/{chat_worktree.go,workspace.go}`;
  - `api/internal/api/v0/container.go`;
  - `hierarchy/worktree.go` (`guardNotWorking`, `chatObserver.Working`; `Usecase.Working` in `api/internal/app/usecases/chat/aliases.go:222` reads `inflight.Work` today).
- Modify: `runner/lifecycle.go` `ReconcileRunnersOnBoot` (T4's ledger boot pass) — the `Do` that appends `runner_exited{daemon_restart}` also appends `turn_ended{interrupted, reason: daemon_restart}` for a running turn.
- Modify (descriptor, pre-flight R4):
  - `descriptors-v3/claude.yaml`: `user_prompt.map` and `turn_stop.map` gain `turn_id: prompt_id`;
  - `…/descriptor/internal/schema/vocabulary.yaml`: `user_prompt.optional` gains `turn_id`, and `turn_stop`'s comment no longer says Go never reads `turn_id`;
  - assert in `…/descriptor/v3_test.go` (keep ≤ 500) that `UserPromptSubmit.json` and `Stop.json` parse with `TurnID` == their `prompt_id`.
  - Without this, the late-stop rule below never applies to real Claude traffic.
- Fix (carry C1) — deleting a home workspace's only chat reaps the home, leaving the live project homeless:
  - Chat delete → `api/internal/app/usecases/chat/internal/tree/delete_chat.go` `planReap`/`isDefaultCheckout` (a project home has no repo, so it is not a "default checkout") → `DeleteCascade` (`hierarchy/worktree.go:1474`), which refuses only Locked and `IsDefault` roots.
  - Write the failing black-box test first. Then fix the owner: a project's home workspace is never reaped by a chat delete, and `DeleteCascade` refuses a home root as it refuses a default one.
- Delete:
  - `api/internal/app/repositories/chat/internal/commands/{start_turn.go,stop_turn.go,working.go}` (and their test cases);
  - `api/internal/app/usecases/chat/internal/turn/{stall.go,idle.go,subagent_staleness.go,tool_staleness.go}` and `{idle,subagent_staleness,tool_staleness}_internal_test.go`;
  - `api/internal/app/usecases/chat/internal/runner/switchwait.go`;
  - `api/internal/app/usecases/chat/internal/shared/inflight/internal/turnstate/`, and the `Turns`/`Work`/`Steered` exports of `inflight.go`;
  - `seam.go` `Stall`; `interruptBound`, `turnEndedAfterInterrupt`, `AwaitTurnComplete`; `SetSwitchAwaitTimeout`, `SetMessageAwaitTimeout`;
  - termwait `DefaultStallQuiet`, `DefaultMessageQuiet`, `DefaultIdleQuiet`, `Messages`, `Idle`, `Work`, `Stalled` and their tests;
  - the cases for deleted timers in `api/internal/app/usecases/chat/{idle_export_test.go,idle_notification_test.go,stop_invariants_test.go}`.
- Test (black box): create `api/tests/regression_chat_turn_test.go`; extend `api/tests/regression_default_checkout_owner_delete_test.go` (C1)

**Interfaces:**
- Consumes: T2 `user_message`/`turn_started`/`prompt_settled` and `PendingPrompts`; T4 live table and `runner_exited`; T3 `ApplyLedger`.
- Produces, `package ledger`:

```go
const (KindTurnEnded Kind = "turn_ended"; KindTurnFailed Kind = "turn_failed")
type TurnStatus string
const (TurnPending TurnStatus = "pending"; TurnRunning TurnStatus = "running"; TurnDone TurnStatus = "done"
	TurnFailed TurnStatus = "failed"; TurnInterrupted TurnStatus = "interrupted")
// Next: pending→running, running→done|failed|interrupted; nothing else.
func (s TurnStatus) Next(to TurnStatus) (TurnStatus, bool)

type TurnEnded struct {
	Status    string `json:"status"` // "done" | "interrupted"
	Reason    string `json:"reason,omitempty"`
	AsyncWork int    `json:"asyncWork,omitempty"` // deleted in T9
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
// Busy: an open main-scope turn ∨ the last turn_ended carried asyncWork > 0 and a
// runner is live (T9 replaces the second clause with live subagents).
func (s State) Busy() bool
```

Turn-end reasons (constants):
- `ReasonStopped = "stopped"`, `ReasonProviderIdle = "provider_idle"`, `ReasonSuperseded = "superseded_by_prompt"`, `ReasonTurnFailed = "turn_failed"`;
- `ReasonRunnerExited` and `ReasonDaemonRestart` already exist.

**Decisions.** Each is one `Store.Do`; the activity record is told the same facts as today until T7.

| Trigger | Rows |
|---|---|
| `user_prompt` hook (T2 decision) while a different turn is running | first `turn_ended{done, reason: superseded_by_prompt}` for the running turn, then T2's rows |
| `turn_stop` hook, running turn T, event `providerTurnId` empty or equal to T's | `turn_ended{done, asyncWork: ev.AsyncWork, effort: ev.Effort}` for T. If a steer prompt is pending, also `turn_started{requestId}` for it, and the hook reply is `PromptSteer(text)`. |
| `turn_stop` naming another provider turn id, or with no running turn | nothing (late) |
| `turn_failed` hook, running turn T | `turn_failed{reason, detail}` for T. A pending steer prompt gets `prompt_settled{consumed:false, reason: turn_failed}`. |
| idle hook, running turn T | `turn_ended{done, reason: provider_idle}` for T. With no running turn: T2's not-acknowledged settle. |
| terminal notice with `ends_turn`, running turn T | `turn_failed{reason: <notice kind>, detail: <captured line>}` |
| user Stop (`StopChat`) | For a running turn T: `turn_ended{interrupted, reason: stopped}`. For every pending Crowbar prompt (steer or not): `prompt_settled{consumed:false, reason: stopped}`. **Then** signal the provider (api interrupt call, else graceful terminate) and return, with no wait. |
| runner exit (T4 decision) | `turn_ended{interrupted, reason: runner_exited}` for a running turn, in the same `Do` |
| switch / selection change / prompt while `Busy()` | refused with `ErrPromptBusy` (409). Steer-eligible prompts park as in T2. |

Publication (the `assembly.go` subscriber):
- a `turn_started` row → `snapshots.Announce(ctx, chatID, "turn_started")`;
- `turn_ended`/`turn_failed` → `Announce(…, "turn_stopped")`;
- every batch → `ApplyLedger(…{Busy: st.Busy()})`, and when busy flipped, `AgentWorking(ctx, wsID, chatID, busy)`.

- [ ] **Step 1: Domain tests**

```go
func TestTurnStatus_Next(t *testing.T) // all 25 pairs; exactly the 4 allowed transitions succeed

func TestFold_TurnLifecycle(t *testing.T)
// user_message T1, turn_started T1 → RunningTurn T1, Busy
// turn_ended{done} T1 → no RunningTurn, !Busy
// turn_ended{done} T1 again (late) → unchanged
// turn_started T2, turn_failed T2 → T2 failed, !Busy

func TestFold_AsyncWorkKeepsTheChatBusyOnlyWhileARunnerLives(t *testing.T)
// runner_started r1, turn_started T1, turn_ended{done, asyncWork:1} → Busy
// runner_exited r1 → !Busy
```

- [ ] **Step 2: Symptom 4 and C1 black-box tests.** In `api/tests/regression_chat_turn_test.go`. The provider-turn-id cases use the claude fake (`Base: "claude"`), whose `user_prompt`/`turn_stop` now map `turn_id: prompt_id`. Pre-flight R18: `ARunnerExit…` is characterization (label it so in the commit body), and `ProviderIdle…` asserts the very next read, never `require.Eventually`.

```go
func TestRegression_Symptom4_ARunnerExitClosesItsTurnAndTheNextPromptGoesThrough(t *testing.T)
// characterization: prompt, user_prompt hook → working true; l.Kill → readUntil turn_stopped
// frame; GET chat working false; POST a new prompt → 2xx and a new launch is recorded

func TestRegression_Symptom4_StopClosesTheTurnAtOnce(t *testing.T)
// running turn; POST …/stop returns; the very next GET shows working false;
// the chat feed carried turn_stopped before the runner "exited" frame

func TestRegression_Symptom4_ProviderIdleClosesARunningTurn(t *testing.T)
// running turn; post event "idle" → the very next GET shows working false; POST prompt → accepted

func TestRegression_Symptom4_ALateStopNeverClosesTheNextTurn(t *testing.T)
// user_prompt{prompt_id:A} → Stop chat (A interrupted) → user_prompt{prompt_id:B} typed at
// the terminal (provider origin) → turn_stop{prompt_id:A} → working still true;
// turn_stop{prompt_id:B} → working false

func TestRegression_Symptom4_CrashMidTurnLeavesTheChatUsable(t *testing.T)
// running turn; h.crash(); h2 := newHarnessAt(t, home) → chat working false,
// session.exitReason "daemon_restart"; POST prompt → 2xx, new launch recorded

func TestRegression_Symptom4_ASwitchWhileATurnRunsIsRefusedNotForced(t *testing.T)
// running turn; POST …/switch to another provider → 409 at once; the running launch's
// process is still alive (syscall.Kill(pid, 0) == nil)

// api/tests/regression_default_checkout_owner_delete_test.go
func TestRegression_DeletingAHomesOnlyChatKeepsTheHome(t *testing.T)
// a project whose home workspace holds exactly one chat; DELETE that chat → 2xx;
// the project still lists its home workspace and the home directory exists
```

These must stay green:
- `TestRegression_ChatStoppedMidTurn_SpunTheWorkspaceForever`, `TestRegression_GracefulShutdown_MidTurn_ClosesAbandonedTurn`;
- `TestRegression_WorkspaceWorkingReflectsAgentTurn`, `TestRegression_WorkspaceWorkingOverlappingChatsBroadcastOnce`, `TestRegression_HomeWorkspaceWorkingReflectsAgentTurn`;
- `TestRegression_DeletingTheDefaultCheckoutOwnerKeepsTheCheckout`;
- every `regression_steered_prompt_test.go` test.

Delete `TestRegression_WorkspaceWorkingClearsWhenClearAbandonsATurn`: the `/clear` move it relies on was deleted in T3.

- [ ] **Step 3: Run; the stop, idle, late-stop, crash, switch and home-delete cases FAIL today**

Run: `bbx 'TestRegression_(Symptom4_|DeletingAHomesOnlyChat)'` → FAIL:
- Stop waits for the 10 s bound on api chats and does not record first;
- idle arms a 5 s latch;
- a late stop closes the open turn;
- a switch blocks on the 120 s bound and then force-kills;
- the home is reaped.

- [ ] **Step 4: Implement** the fold, the decision table and the descriptor mapping; route every busy reader to the snapshot; make the C1 fix at its owner; delete everything listed.

- [ ] **Step 5: Run to green**

Run: `gox go test -tags noEmbed -cover ./internal/domain/ledger/... ./internal/engine/agents/... ./internal/app/... ./internal/api/...` → PASS. `domain/ledger` stays ≥ 92%; report the other numbers.
Run: `bbx 'TestRegression_(Symptom4_|Symptom2_|DeletingAHomesOnlyChat|DeletingTheDefaultCheckout|ChatStoppedMidTurn|GracefulShutdown|WorkspaceWorking|HomeWorkspaceWorking|PromptSentMidTurn|SteeredPrompt|CommandSentMidTurn|PromptSentWhileIdle|StopRefusesAParkedPrompt|AgentStop|AgentClose)|TestFakeProvider_'` → PASS.
Run: `gox make descriptor-check` and `gates` → clean. Then `grep -rn 'DefaultStallQuiet\|DefaultMessageQuiet\|DefaultIdleQuiet\|CloseStalledTurn\|AwaitTurnComplete' api/internal` → nothing.

- [ ] **Step 6: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "fix(api): the turn is a ledger state machine; busy is its fold

Every close names its turn. Delete the stall closer, quiet/idle timers,
staleness nets, switch force-kill and the chat aggregate's turn state.
A chat delete never reaps its project's home.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- All six `TestRegression_Symptom4_*` and `TestRegression_DeletingAHomesOnlyChatKeepsTheHome` pass.
- No `time.After`/`time.NewTimer`/ticker in `turn/` or `runner/` decides a turn.
- `domain.Chat` carries no turn state.
- Every edited oversize file shrank or was split.

---

### Task 6: One hook ingress

**Goal:** A single `ingress` package receives every hook (the HTTP route, the relay route and the codex api pump):
- it resolves the runner through the live table and parses through the descriptor;
- it holds the chat's per-chat lock for the whole hook, and replies only after its rows commit;
- anything it cannot place is answered non-2xx and dead-lettered.

Deleted: the per-runner hook gate, the startup barrier, the delivery-id dedup, the 3 s step-out and the `inflight` spawn gate.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- Never acknowledge a hook before its row is written; never drop one without a dead-letter row.
- Behaviour change, accepted provisionally on 2026-10-18: hooks Crowbar cannot map get 422 (the CLI may print it as a hook error), and late hooks for a deleted chat get 404.
- One per-chat lock (pre-flight R22):
  - Ingress owns it, and user-initiated spawns take the same lock. It replaces `inflight`'s spawn gate.
  - Lock order: ingress lock → ledger writer → snapshot owner.
  - A spawn holds the lock only for its decision and its `runner_started` append. It never holds it while waiting for a previous CLI to exit: that CLI's own hooks need the lock to be answered.
- Answer desk by ask id (pre-flight R10): `HTTPHook` holds asks on a desk keyed by the delivery id this task removes. So the desk is rekeyed here, by the ask's id: today the activity choice id, from T7 the `ask` row's `askId`. T11 then deletes only relay-only code.
- One test file per source file (pre-flight R28): `deadletter.go` gets `deadletter_internal_test.go`.
- Oversize (pre-flight R8): `api/internal/api/v0/endpoints/chat/handlers/handlers.go` (639) and `hooks_test.go` (838) must net-shrink, or be split below 500 in this task.

**Files:**
- Create: `api/internal/app/usecases/chat/internal/ingress/ingress.go`, `deadletter.go`, `lock.go` and `ingress_internal_test.go`, `deadletter_internal_test.go`, `lock_internal_test.go`
- Modify: `api/internal/adapter/store/ledger/{schema.go,store.go,store_test.go}` — the dead-letter table, `DeadLetter` and `DeadLetterCount`
- Modify: `api/internal/app/usecases/chat/internal/turn/{ingest.go,turns.go,types.go}`:
  - `ingestResolvedHook`'s dispatch is called by `ingress`;
  - `IngestHook`, `IngestHookDelivery` and `TakeHookReply` are removed from `TurnUsecase`.
- Modify: `api/internal/api/v0/endpoints/chat/handlers/{hooks.go,handlers.go}`:
  - `HTTPHook` and `Hooks` call `Ingress.Hook`;
  - `HTTPHook` no longer mints a delivery id (`hooks.go:133`);
  - it holds the response on `Reply.Ask`.
- Modify: `api/internal/app/usecases/chat/internal/shared/answerdesk/{answerdesk.go,types.go,answerdesk_test.go}` — `Hold`, `Pending`, `Await` and `Abandon` are keyed by ask id. `answerdesk_test.go` (508) must net-shrink.
- Modify: `api/internal/app/usecases/chat/answers.go` — `PendingAnswer`/`AwaitAnswer`/`AbandonAnswer` take the ask id
- Modify: `api/internal/app/usecases/chat/internal/runner/apiconn.go` — the pump calls `Ingress.Hook` with the api channel, and keeps reading after an error (the error is already recorded)
- Modify: `api/internal/app/usecases/chat/internal/runner/{spawn.go,prompts.go}` — user-initiated spawns take `ingress`'s per-chat lock (a `ChatLock` interface, passed as a constructor argument)
- Modify: `api/internal/api/v0/endpoints/health/handlers/health.go` and its test:
  - `chatDeadLetters` (a count) on the health response;
  - `Check` takes no dependencies today, so give it the count through a constructor argument (no package var).
- Modify: `api/internal/app/usecases/chat/{assembly.go,chat.go}` — `PurgeChat` removes the chat's live entry from the table before terminating its CLI, so later hooks cannot resolve to the deleted chat.
- Delete:
  - `api/internal/app/usecases/chat/internal/turn/delivery.go` and `api/internal/app/usecases/chat/internal/turn/internal/dedup/`;
  - `api/internal/app/usecases/chat/internal/shared/inflight/` (the remaining `pending` barrier and `gate`; the package is gone after this task);
  - the `AwaitOpen` 3 s wait and the gate step-out in `turn/message.go`;
  - `api/tests/integration/agent/barriers_test.go`;
  - the `api/internal/app/usecases/chat/hook_ingest_bench_test.go` cases that exercise the barrier.
- Test (black box): extend `api/tests/agent_home_hook_test.go` and `api/tests/regression_chat_turn_test.go`

**Interfaces:**
- Consumes: the live table (`RunnerByID`), `engineagents.Agent.ParseHook`, and the T2–T5 decisions.
- Produces:

```go
package ingress
var (
	ErrUnknownRunner = fmt.Errorf("ingress: unknown runner: %w", apperr.ErrNotFound)      // 404
	ErrUnmappable    = fmt.Errorf("ingress: unmappable hook: %w", apperr.ErrUnprocessable) // 422, mapped in api/libs/status.go
)
type Reply struct {
	Body []byte // stdout the CLI receives (a steer); nil = empty 200
	Ask  string // non-empty: the response is held on this ask id (answer desk)
}
func (i *Ingress) Hook(ctx context.Context, runnerID, provider, event string,
	raw []byte, channel engineagents.Channel) (Reply, error)
// Lock serialises one chat's hooks and its user-initiated spawns.
func (i *Ingress) Lock(chatID string) (unlock func())

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

**Ingress contract** (report §4.3), in this order:
1. A `RunnerByID` miss → `DeadLetter{reason: "unknown_runner"}`, `ErrUnknownRunner`. A bad token stays 401 in the handler, with no dead letter: an unauthenticated body is not a fact.
2. Resolve the descriptor and `ParseHook`:
   - A variant mismatch (`inbound.VariantMismatchError`) is a declared filter → `Reply{}` (200), no record. This is a `when:` that selects another variant of the same wire hook; every Notification is posted to both `/notification` and `/idle`.
   - `ErrForeignConversation` → `DeadLetter{reason: "foreign_conversation"}`, `Reply{}` (200).
   - `ErrHookUndeclared`, `ErrRequiredFieldMissing` or any other parse error → one `Store.Append` of a `marker{dead_letter, detail: "<event>: <error>"}` row, then `DeadLetter{reason: "unmappable"}`, and `ErrUnmappable`.
3. Take the chat's lock, run the existing dispatch (`session_start`, turn lifecycle, observations), and release.
4. Return only after step 3's rows have committed. Asks keep the held response:
   - `Hook` returns `Reply{Ask}`, and the handler holds the HTTP response after `Hook` returned;
   - the hold is never under the chat lock, so a held permission never blocks the chat's other hooks;
   - surface-gated events stay a declared filter and return `Reply{}`.

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
func TestHook_AnAskIsHeldOnItsAskID(t *testing.T)
// deadletter_internal_test.go
func TestDeadLetter_RecordsTheReasonAndTheRunner(t *testing.T)
// lock_internal_test.go
func TestLock_SerialisesOneChatAndNotOthers(t *testing.T)

// answerdesk_test.go (rekeyed cases replace the delivery-id ones)
func TestDesk_AnAnswerReleasesTheRelayHeldOnThatAsk(t *testing.T)

// api/tests/regression_chat_turn_test.go
func TestRegression_ConcurrentHooksForOneChatAllLand(t *testing.T)
// 20 goroutines each post tool_pre then tool_post for their own tool id → all 200;
// GET …/activity has 20 tool calls, all status "ok"
func TestRegression_HooksAfterChatDeleteAreRefusedNotResurrected(t *testing.T)
// l := AwaitLaunch; DELETE the chat; l.PostHook(tool_pre) → 404; GET /v0/health
// chatDeadLetters ≥ 1; GET …/chats/<id> → 404
func TestRegression_AnUnmappableHookIsRefusedVisibly(t *testing.T)
// post user_prompt {"session_id":"s"} with no prompt (claude.yaml requires
// [session_id, message]) → 422; health chatDeadLetters +1
func TestRegression_AHookRepliesOnlyAfterItsRowsAreReadable(t *testing.T)
// post user_prompt; on its response, GET …/messages already contains it
```

These must stay green:
- `TestRegression_AgentHomeCallbacksReachDaemon`;
- `TestRegression_InternalSessionHooksAreDroppedNotFailed` (now: acknowledged and dead-lettered);
- `TestRegression_HooksDeliveryOfAChannelSplit*`;
- `TestRegression_AChoiceOpensAndClearsOverTheChatSocket` and the other `regression_agent_choice_frame_test.go` tests that hold a permission over `HTTPHook`.

- [ ] **Step 2: Run; the concurrency, delete and 422 cases FAIL today**

Run: `bbx 'TestRegression_(ConcurrentHooksForOneChat|HooksAfterChatDelete|AnUnmappableHook|AHookRepliesOnlyAfter)'`. Today an unmappable hook gets a silent 200, and a deleted chat's runner is displaced rather than unknown.

- [ ] **Step 3: Implement** `ingress`, its lock, the dead-letter table, the desk rekey, and the handler, pump and spawn wiring. Delete the gate, barrier, dedup and step-out.

- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./internal/app/... ./internal/adapter/... ./internal/api/...` → PASS; `ingress` ≥ 92%.
- `bbx 'TestRegression_(ConcurrentHooksForOneChat|HooksAfterChatDelete|AnUnmappableHook|AHookRepliesOnlyAfter|AgentHomeCallbacks|InternalSessionHooks|HooksDeliveryOfAChannelSplit|AChoice|SeveralChoices|Symptom2_|Symptom4_)|TestFakeProvider_'` → PASS.
- `gates` → clean.
- `test ! -d api/internal/app/usecases/chat/internal/shared/inflight`.

- [ ] **Step 5: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "fix(api): one hook ingress that records before it replies and dead-letters what it cannot place

Delete the per-runner hook gate, startup barrier, inert delivery dedup and step-out.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- No code path answers a hook 2xx without either committed rows or a dead-letter entry.
- The `inflight` package is gone, and one per-chat lock orders hooks and spawns.
- Held asks are keyed by ask id.

---

### Task 7: The transcript is the ledger (symptom 3)

**Goal:** Every content fact a hook carries becomes a row, through one pure mapper.
- These read endpoints become projections of those rows: `/messages`, `/activity`, `/activity/:toolId/payload`, `/choices`, the `get_chat_log` MCP tool and the handoff.
- So do the old live frames: `message_delta`, `plan`, `choice`, `compaction_*`.
- A subagent's own tool calls are recorded under its scope instead of dropped.
- The activity aggregate and the `turn` package are deleted.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- Nothing a hook says is overwritten or dropped.
- Ingress dedupes by natural keys (pre-flight R30). Once the inert delivery dedup is gone, a CLI retry must not duplicate a fact:
  - a `user_prompt` whose `providerTurnId` (Claude: `prompt_id`) names a turn already started → no rows;
  - a `tool_call` for an existing (toolId, phase) → no row;
  - an `ask` for an existing (toolId, event) → no new ask; the held reply joins the existing ask id;
  - a `subagent` row for an existing (scope, phase) → no row.
  - `State` keeps these keys.
- Projection fill (pre-flight R9): a projector miss builds and installs under the chat's ledger writer (`Store.View`). A publication can then never be skipped by a stale fill, which would recreate the "nothing renders" class. The projector is a derived stage adapter that S2 deletes (pre-flight R27).
- `turn_failed` is projected as a `notice` message carrying its `detail`, so failure reasons (usage limit, API error) stay visible (pre-flight R11).
- A `user_message{origin: harness}` is projected with role `harness`, never `user` (pre-flight R12).
- `cmd/crowbar-seed-chat/fixture.go` writes the activity aggregate directly. Rewrite it onto `ledgerstore.Append` in this task, or the build breaks (pre-flight R13).
- `ask` rows carry `answerable`, set from the same decision that sets `fx.HoldAsk` (S2 pre-flight amendment).
- The lifecycle `message_delta` frame on `…/chats/ws` stays cumulative.
- One test file per source file (pre-flight R28): `ingress/steer.go` → `steer_test.go`, `transcript/choices.go` → `choices_test.go`.
- Oversize (pre-flight R8):
  - `api/tests/regression_agent_message_stream_test.go` (458): the Symptom 3 tests go in a new `api/tests/regression_chat_transcript_test.go`.
  - These test files only shrink: `turn_test.go` (3,061), `answers_test.go` (845), `harness_test.go` (1,616), `chat_test.go` (2,605), `shared/tools/toolset_test.go` (2,430), `handlers/chats_test.go` (1,644).
  - `handlers/chats.go` (611) and the containers net-shrink, or are split below 500.
- Coverage checkpoint (pre-flight R15): report `go test -cover` for every touched package. CI owns the 92% module floor; do not run `make test-coverage`.

**Files:**
- Create: `api/internal/domain/ledger/{message.go,tool.go,ask.go,subagent.go,plan.go}` and their `_test.go`; `marker.go` gains the T7 marker kinds
- Create: `api/internal/app/usecases/chat/internal/ingress/{rows.go,live.go,steer.go}` and `rows_test.go`, `live_internal_test.go`, `steer_test.go`:
  - the mapper; the ephemeral live-text buffer (text so far per (turn, scope, id, stream)); steer (moved from `turn/steer.go`).
- Create: `api/internal/app/usecases/chat/internal/transcript/{transcript.go,messages.go,activity.go,choices.go}` and `transcript_test.go`, `messages_test.go`, `activity_test.go`, `choices_test.go`
- Modify: `api/internal/adapter/store/ledger/{store.go,store_test.go}` — `View`
- Move: `api/internal/app/repositories/chat/activity/internal/store/internal/content/content.go` → `api/internal/adapter/store/ledger/blobs.go` (same on-disk directory, `h.Content()` = `state/content/`; same sha256 refs), with its test
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema/vocabulary.yaml`:
  - new canonical in-event `message_complete` {`session_id`, `turn_id`, `message_id`, `text`, `phase`};
  - `codex.yaml`'s api channel maps it from `item/completed` when `item.type == agentMessage` (`message_id: item.id`, `text: item.text`, `phase: item.phase`);
  - `claude.yaml` lists it in `not_emitted`;
  - `models.CanonicalEvent` gains `Complete *MessageComplete`.
- Modify:
  - `api/internal/app/usecases/chat/{assembly.go,chat.go,turn.go,answers.go,aliases.go,runner.go}`;
  - `internal/conversation/{chatlog.go,handoff.go,conversation.go}`;
  - `internal/shared/answerdesk/{answerdesk.go,types.go}` (its `Ledger` port becomes an ask resolver that appends `ask_resolved`);
  - the `internal/shared/tools/` readers of activity;
  - `internal/runner/{stop.go,switch.go,promptswitch.go,compact.go,apiconn.go}` (markers instead of activity interruptions).
- Modify: `api/internal/api/v0/endpoints/chat/handlers/{activity.go,activity_components.go,answers.go,chats.go}` — read through the transcript projection; response shapes unchanged
- Modify: `api/internal/app/repositories/container.go`, `api/internal/app/container.go` (drop `axAgentActivity`), `api/internal/adapter/container.go` (drop the `agent_activity` event, snapshot and read DBs), `api/internal/app/usecases/container.go` — drop `AgentActivity`. Step 4's legacy reader (T4) keeps reading `agent_activity.db` read-only.
- Modify: `api/cmd/crowbar-seed-chat/fixture.go` (and its test) — seed through `ledgerstore.Open(storesqlite.OpenDB(filepath.Join(h.Store(), ledgerstore.FileName)))` and `Append`
- Move then delete `api/internal/app/usecases/chat/internal/turn/`:
  - `ingest.go`, `conversation.go`, `nested.go`, `tool_diff.go` move into `ingress`;
  - `attachments.go` moves to `api/internal/app/usecases/chat/attachments.go`;
  - everything else is deleted with its tests: `activity_reads.go`, `asks.go`, `choices.go`, `compaction.go`, `livetext.go`, `message.go`, `observation.go`, `turn.go`, `turn_open.go`, `turns.go`, `types.go`, `internal/stream/`.
- Delete:
  - `api/internal/app/repositories/chat/activity/` (the whole package);
  - `api/internal/app/usecases/chat/choicewatch.go` and its test;
  - the activity-aggregate cases of `api/internal/app/usecases/chat/{turn_test.go,turn_export_test.go,answers_test.go,harness_test.go}`, including the harness-role cases at `turn_test.go:1851–2040` (their behaviour is re-proven below);
  - `domain.ChatActivity`, `ActivityDelta`, `MaxOpenPerTurn` and `OpenTurnOrders` in `api/internal/domain/chat_activity.go` (keep the `Activity*` read shapes the projection returns until S2).
- Test (black box): create `api/tests/regression_chat_transcript_test.go`. Keep `regression_agent_message_stream_test.go`, `regression_agent_nested_choice_test.go` and `regression_agent_choice_frame_test.go` green.

**Interfaces:**
- Consumes: T2–T6.
- Produces, `package ledger` bodies (field names as in the index vocabulary), plus:

```go
const (KindAssistantText Kind = "assistant_text"; KindReasoning Kind = "reasoning"; KindToolCall Kind = "tool_call"
	KindAsk Kind = "ask"; KindAskResolved Kind = "ask_resolved"; KindSubagent Kind = "subagent"; KindPlan Kind = "plan")
type AskBody struct {
	AskID, Kind, PromptID, ToolID, ToolName, Title, Question, Mode string // json: askId, kind, promptId, toolId, toolName, title, question, mode
	Multi                                                         bool   `json:"multi"`
	Options, Questions, Schema                                    json.RawMessage
	Answerable                                                    bool   `json:"answerable"`
}
type MessageKey struct{ TurnID, Scope, MessageID string }
type MessageText struct {
	Key       MessageKey
	Text      string
	Settled   bool  // stream complete, a complete row exists, or its turn ended
	FirstSeq  int64 // seq of its first row
	SettleSeq int64 // seq of the row that settled it (0 while unsettled)
	Phase     string
}
// Messages folds assistant_text rows. The text is the contiguous stream 0..k when
// chunk k is final; else the complete row (index -1); else the chunks present, in
// index order, with duplicate indexes ignored.
func Messages(rows []Row) []MessageText
```

- Produces, store: `func (s *Store) View(ctx context.Context, chatID string, fn func(ledger.State) error) error`. It runs `fn` under the chat's writer; `fn` may call `Read` but never `Do`/`Append`.
- Produces, `ingress`:

```go
// Map decides the rows for one parsed event against the chat's state. It does no
// I/O: payload blobs are written before the Do that calls it (content-addressed,
// idempotent), and their refs are passed in.
func Map(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) (rows []ledger.Row, fx Effects)
type Source struct{ RunnerID, Provider string }
type BlobRefs struct{ Input, Result string }
type Effects struct {
	Reply   []byte     // steer stdout
	HoldAsk string     // ask id whose response is held; the ask row's answerable is HoldAsk != ""
	Live    *LiveDelta // ephemeral text for the cumulative lifecycle message_delta frame
}
```

- Produces, `transcript`:

```go
// Projector keeps the old read shapes of up to 64 chats (LRU), advanced by ledger
// publication and rebuilt from Store.Read under Store.View on a miss. S2 deletes it.
func New(store *ledgerstore.Store) *Projector
func (p *Projector) Apply(chatID string, rows []ledger.Row) // the publication subscriber
func (p *Projector) Messages(ctx context.Context, chatID string, after, before, limit int) (domain.LedgerPage, error)
func (p *Projector) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error)
func (p *Projector) Activity(ctx context.Context, chatID string) (Activity, error) // tools, subagents, interruptions, choices
func (p *Projector) PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
```

**Mapper table.** The scope `s` = `ev.Subagent.ID` when set. A codex nested child keeps today's `nested.go` routing, now emitting rows whose `Scope` is the child thread id. Every row is subject to the natural-key dedup above.

| Canonical event | Rows |
|---|---|
| `user_prompt` | T2/T5's decision (crowbar / harness / provider origin), moved here |
| `message_delta` with `Sequenced` | `assistant_text{messageId, index, final, text}`, unless a row with the same (turn, scope, messageId, index) exists |
| `message_delta` unsequenced, `reasoning_delta`, `tool_output_delta`, `diff_update` | no row; `fx.Live` (ephemeral) |
| `message_complete` | `assistant_text{messageId, index: -1, text, phase}` |
| `turn_stop` with a message | `assistant_text{index: -1, text, messageId}` before T5's `turn_ended`. The messageId is the running turn's last main-scope message id, or `"final-"+turnID` when it has none. The turn's accumulated live reasoning, if any, becomes one `reasoning{messageId, text}` row. |
| `tool_pre` / `tool_post` / `tool_fail` | `tool_call{phase, toolId, name, kind, target, locations, inputRef, resultRef, status, error, durationMs, diff, spawnsScope: ev.Tool.NestedSessionID}` in scope `s`. An empty tool id is minted as `"tool-"+uuid`, on pre only. |
| `subagent_pre` / `subagent_post` | `subagent{phase: start\|stop, type}` with `Scope` = the subagent id |
| `permission` / `elicitation` | `ask{askId: uuid, …ChoicePrompt fields, toolId, answerable}`. `toolId` is the open tool in scope `s` with the same tool name, else `""`. `fx.HoldAsk` is set when Crowbar can hold the reply. |
| answered through Crowbar / CLI hung up / the bound tool's post / its turn ended | `ask_resolved` with `answered` / `proceeded` / `proceeded` / `abandoned`. A turn end names only that turn's asks. |
| `plan_update` | `plan{op: replace, items}` in scope `s` |
| `notification` | `marker{notification, detail: message}` |
| `compact_pre` / `compact_post` | `marker{compaction_started}` / `marker{compaction_ended}`. A `turn_stop` whose provider turn id is the open compaction's is `marker{compaction_ended}` only (replaces `turn/compaction.go`'s latch). |
| Crowbar's own Stop, provider switch, model/effort change | `marker{stopped}`, `marker{provider_switched, detail}`, `marker{model_changed, detail}`, `marker{effort_changed, detail}` |

**Projection rules** (old wire, shapes unchanged):
- Message `Sequence` = `SettleSeq`, so `?after=` paging still returns a message once it settles. `DisplayOrder` = `FirstSeq`; `ItemIndex` = 0. Only settled messages are listed: a non-final stream lives only on the socket, as today.
- User message `TurnID` = its `requestId` for Crowbar prompts (the client matches its queue on it), else the ledger turn id. Role is `user` for crowbar/provider origin and `harness` for harness origin.
- A `turn_failed` row is a `notice` message carrying its `detail` (else its `reason`), in its turn.
- Assistant message `TurnID` = `"<turnId>:<k>"`, where `k` is its ordinal among the turn's main-scope messages by `FirstSeq`.
- A main-scope tool's `TurnID` = `"<turnId>:<k>"`, where `k` is the number of that turn's main-scope messages whose `FirstSeq` precedes the tool's pre row. Scoped tools carry `TurnID ""` and `SubagentID` = scope.
- A tool still running when its turn ended reads `status: "abandoned"`. This is derived at read time from the named turn, never by a sweep.
- Interruptions: each marker is `{ID: "m-<seq>", Seq, DisplayOrder: seq, Kind, Detail, At}`. The kinds are `compaction` (resolved at its `compaction_ended`), `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification`, `session_changed`, `session_conflict` and `dead_letter`. Each ask also yields an interruption of its kind, resolved at its `ask_resolved`.
- Choices: one per ask; `Answerable` from the row.
- The turn's latest main-scope plan is attached to the turn's last assistant message (`ActivityTurn.Plan`), as today.
- Coherence:
  - `Apply` on an uncached chat does nothing.
  - `Apply` on a cached chat whose head is not `seq − 1` of the first row drops that chat's entry.
  - A read that misses calls `Store.View(chatID, …)`, then `Read`, build and install, all inside `fn`; publication runs under the same writer, so it cannot interleave.
  - The projector never holds its own lock while calling the store.

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
func TestMap_AHeldAskIsAnswerable(t *testing.T)
func TestMap_AnAskCrowbarCannotHoldIsNotAnswerable(t *testing.T)
func TestMap_AnInjectedPromptIsHarnessNotUser(t *testing.T)
func TestMap_ARepeatedUserPromptOpensNoSecondTurn(t *testing.T)   // same providerTurnId twice → rows once
func TestMap_ARepeatedToolPhaseWritesNoRow(t *testing.T)
func TestMap_ARepeatedAskJoinsTheFirst(t *testing.T)              // same (toolId,event) → one ask, same HoldAsk
func TestSteer_APendingSteerRidesTheTurnStopReply(t *testing.T)   // steer_test.go
func TestProjector_ToolsAttachToTheReplyThatFollowsThem(t *testing.T)
// user, msg1(final), tool, msg2(final) → tool.TurnID == msg2.TurnID != msg1.TurnID
func TestProjector_AToolOpenWhenItsTurnEndedReadsAbandoned(t *testing.T)
func TestProjector_RebuildsFromRowsOnAMiss(t *testing.T) // 65 chats touched → the first is rebuilt identically
func TestProjector_AFillRacingAPublicationIsNeverStale(t *testing.T) // miss + concurrent Append → the read includes the row
func TestProjector_AFailedTurnReadsAsANotice(t *testing.T)
func TestProjector_AHarnessMessageKeepsItsRole(t *testing.T)
func TestChoices_OnePerAskWithItsAnswerable(t *testing.T)         // choices_test.go
```

- [ ] **Step 2: Symptom 3 black-box tests.** Create `api/tests/regression_chat_transcript_test.go` (claude fake). The replay rewrites the recorded session id `68a2773c-78c5-4edd-b4fe-bf5bd1309895` to the chat's bound one.

```go
func TestRegression_Symptom3_ATailChunkAfterStopNeverOverwritesTheReply(t *testing.T)
// replay tests/testdata/hooks/claude-subagent-session 00..34 → the assistant message of
// the turn that 28-Stop.json closes equals 28-Stop.json's last_assistant_message in full;
// after 30..34 (the next turn) it is still that full text
func TestRegression_Symptom3_ASubagentsToolCallsAreRecorded(t *testing.T)
// replay 00..10 → GET …/activity has tool calls with subagentId == the SubagentStart agent_id
func TestRegression_Symptom3_TextSaidBeforeAProcessExitsIsKept(t *testing.T)
// user_prompt, message_delta idx0 final "KEPT", then l.Kill → GET …/messages has "KEPT"
func TestRegression_Symptom3_AStoppedTurnsLateFinalTextIsRecordedNotReopened(t *testing.T)
// running turn, Stop, then turn_stop{last_assistant_message:"LATE"} from the same runner
// → GET …/messages has "LATE"; working stays false
func TestRegression_ARetriedHookDeliveryRecordsItsFactOnce(t *testing.T)
// post the same tool_pre and the same permission twice → one tool call, one choice card
func TestRegression_ATurnFailureReasonStaysVisible(t *testing.T)
// running turn; post turn_failed{reason, message:"usage limit"} → GET …/messages has a
// notice whose text carries "usage limit"
```

These must stay green:
- `TestRegression_AChoiceOpensAndClearsOverTheChatSocket` and the rest of `regression_agent_choice_frame_test.go`;
- `TestRegression_ANestedConversationsPermissionBecomesAnAnswerableCard` and the rest of `regression_agent_nested_choice_test.go`;
- `TestRegression_CompactPreAndPostPushLiveOverTheSocket`;
- every test in `regression_agent_message_stream_test.go`;
- `TestRegression_ProviderSwitchRecordsAMarkerInterruption` and the rest of `regression_agent_switch_markers_test.go`;
- `TestRegression_AThreadReadsTheChatItHangsOff` and the rest of `regression_agent_chat_threads_test.go`;
- `TestRegression_TelemetrySurvives*`;
- `api/internal/app/usecases/chat/internal/conversation/chatlog_test.go`.

- [ ] **Step 3: Run; the tail-overwrite, subagent-tool, kill, retry and failure-reason cases FAIL today**

Run: `bbx 'TestRegression_(Symptom3_|ARetriedHookDelivery|ATurnFailureReason)'` → FAIL.

- [ ] **Step 4: Implement** the domain folds, the mapper with its dedup, the live buffer, the projector with `View`, the blob move and the descriptor event; rewire every reader and the seed tool; delete the activity aggregate and the `turn` package.

- [ ] **Step 5: Run to green**
- `gox go test -tags noEmbed -cover ./internal/domain/... ./internal/adapter/... ./internal/engine/agents/... ./internal/app/... ./internal/api/... ./cmd/crowbar-seed-chat/...` → PASS. `ingress`, `transcript` and `domain/ledger` must each be ≥ 92%; report the rest.
- `bbx` with the regex below → PASS:

  ```
  TestRegression_(Symptom2_|Symptom3_|Symptom4_|ARetriedHookDelivery|ATurnFailureReason|EveryAssistantMessage|AMessageIsVisible|AGrowingMessage|ProviderWithNoStreaming|TheTerminatingHook|ALossyTerminatingHook|AStreamedReply|AChoice|SeveralChoices|ANested|CompactPreAndPost|AStreamingMessageCarries|ProviderSwitch|SwitchingTo|SelectionChange|SwitchingProviders|AFailureFromBefore|AConcurrentFailure|AThread|AChatCannot|SiblingThreads|AFiledChat|FoldersAre|ReParenting|AChatCreated|TelemetrySurvives|ConcurrentHooks|HooksAfterChatDelete)|TestFakeProvider_
  ```

- `gox make descriptor-check` and `gates` → clean.
- `test ! -d api/internal/app/repositories/chat/activity && test ! -d api/internal/app/usecases/chat/internal/turn`.

- [ ] **Step 6: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "fix(api): the transcript is ledger rows; nothing a hook says is overwritten or dropped

A late chunk can no longer replace a finished reply, and a subagent's tool
calls are recorded under its scope. Delete the activity aggregate and the turn package.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- All four `TestRegression_Symptom3_*` and every listed must-stay-green test pass.
- No `agent_activity*.db` is opened, except read-only by step 4's reader.
- The old endpoints answer from the projection, and the seed tool writes rows.
- The coverage numbers are reported.

---

### Task 8: Tool and MCP detail

**Goal:** Tool rows and asks carry what the hooks already send:
- the agent's own title for the call; the MCP server and tool; the exit code; an interrupt flag;
- the target, from every key Claude Code itself uses;
- on asks, the tool target and input.

Two generic grammar extensions make this expressible in YAML: a `pattern:` transform and a composite `kind_map` key.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- No real provider CLI runs, and no provider home is touched.
- No Go file names a provider tool; tool names live in YAML.
- Fixtures come from recorded captures (pre-flight R17). First, copy the needed captures from the session scratchpad: `/private/tmp/claude-501/-Users-char2cs--crowbar-projects-cb81ea03-54b7-4093-8bdd-fdd6b183e91d-github-com-char2cs-crowbar-fix-refactor-chats-worktree/503ae3f2-1747-426a-8623-d22231419e7d/scratchpad/design-hooks/logs/{claude.jsonl,claude2.jsonl,claude-tui.jsonl,codex-app.jsonl,codex-hooks.jsonl}`.
  - Each capture you need becomes one JSON file under `api/internal/engine/agents/internal/protocol/testdata/fixtures/{claude,codex}/`.
  - Scrub the runner token, the MCP bearer, user paths (to `/ws` and `/h`), the home segments of `transcript_path`, and session/user ids.
  - If those files are gone, stop and report. Never fabricate a fixture.
- Oversize (pre-flight R8):
  - `…/descriptor/fixture_test.go` (508) gets no new lines. A fixture listed in an event's `fixtures:` YAML list is already walked by `TestV3Descriptors_ResolveAgainstRecordedTraffic`; value assertions go in `v3_test.go` (keep it ≤ 500).
  - `mapping/mapping_test.go` (503) must net-shrink (trim narrative comments), or be split below 500.
  - `mapping.go` (433) and `inbound/hooks.go` (406) stay ≤ 500.

**Files:**
- Modify: `…/descriptor/internal/schema/vocabulary.yaml`:
  - `tool_pre`/`tool_post`/`tool_fail` gain optional `tool_title`, `mcp_server`, `mcp_tool`, `exit_code`, `interrupted`;
  - `permission`/`elicitation` gain `tool_target`, `tool_input`, `mcp_server`.
- Modify: `api/internal/engine/agents/internal/spec/{expr.go,expr_test.go}` and `api/internal/engine/agents/internal/mapping/{mapping.go,mapping_test.go}`:
  - `{from: <path>, pattern: '<regex with group v>'}`;
  - `kind_map` keyed by `{join: [path, path]}` (values joined with `/`).
- Modify: `api/internal/engine/agents/internal/protocol/internal/translate/inbound/{hooks.go,choice.go}` and their tests; `api/internal/engine/agents/internal/models/event.go`:
  - `ToolEvent{Title, MCPServer, MCPTool string; ExitCode *int; Interrupted bool}`;
  - `ChoicePrompt{ToolTarget string; ToolInput []byte; MCPServer string}`.
- Modify: `claude.yaml`, `codex.yaml` (values below), their `fixtures:` lists, and assertions in `…/descriptor/v3_test.go`
- Modify: `api/internal/domain/ledger/{tool.go,ask.go}` and `api/internal/app/usecases/chat/internal/ingress/rows.go`:
  - the new fields on `tool_call`/`ask`;
  - an ask binds to the open tool of its scope with the same tool name **and** canonical-JSON-equal input (additions design §F), falling back to the name alone when the ask carries no input.
- Modify: the split `api/internal/api/v0/dto/` files (additive, S2 renders them):
  - `AgentToolCallDTO` gains `title`, `mcpServer`, `mcpTool`, `exitCode` (all `omitempty`);
  - `AgentChoiceDTO` gains `toolTarget`, `mcpServer`.

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

The field paths come from the captures (claude 2.1.302, codex 0.158.0):
- Claude: Bash `tool_input.description`; MCP `tool_name: "mcp__echo__shout"`, `mcp_server: {name: "echo", source: "dynamic"}`; `PostToolUseFailure.is_interrupt`.
- codex: `commandExecution.commandActions[]{type: read|listFiles|search|unknown, command}`, `exitCode`; `mcpToolCall{server, tool, arguments}`; hooks `mcp__echo__shout` (no `mcp_server` field).

Add one fixture per new mapping, listed in the event's `fixtures:`.

- [ ] **Step 1: Tests**

```go
func TestPatternTransform_ExtractsGroupV(t *testing.T)            // spec/expr_test.go
func TestKindMap_JoinKey(t *testing.T)                            // mapping_test.go: join of item.type,item.tool
func TestClaudeDescriptor_MapsMCPServerToolAndTitle(t *testing.T) // v3_test.go: fixture PreToolUse.mcp.json → server "echo", tool "shout"
func TestClaudeDescriptor_MapsBashDescriptionAsTitle(t *testing.T)
func TestClaudeDescriptor_ToolFailCarriesInterrupt(t *testing.T)
func TestCodexDescriptor_CommandActionsGiveKindTargetAndExitCode(t *testing.T)
func TestMap_AnAskBindsByToolNameAndCanonicalInput(t *testing.T)  // ingress: two open Bash tools, ask input equals the second's → toolId of the second
// api/tests/regression_chat_tool_detail_test.go (new)
func TestRegression_AnMCPToolRowNamesItsServerAndTool(t *testing.T) // claude fake: tool_pre mcp__echo__shout + mcp_server → GET …/activity toolCall mcpServer "echo", mcpTool "shout"
```

- [ ] **Step 2: Run; they FAIL.** Run: `gox go test -tags noEmbed ./internal/engine/agents/... ./internal/app/usecases/chat/internal/ingress/...`

- [ ] **Step 3: Implement** the fixtures, the grammar, the vocabulary, the mappings, the fields and the ask binding. Leave `features.collab_agents=true` in `codex.yaml` until T9.

- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./internal/engine/agents/... ./internal/domain/ledger/... ./internal/app/usecases/chat/... ./internal/api/...`.
- `gox make descriptor-check`.
- `bbx 'TestRegression_(AnMCPToolRow|Symptom3_|AChoice|ANested|HooksDeliveryOfAChannelSplit)'`.
- `gates`.

- [ ] **Step 5: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "feat(api): tool rows carry title, MCP server/tool, exit code and interrupt from the descriptors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- No Go file outside tests names a provider tool (`Bash`, `mcp__`, `commandActions`).
- The new fields reach the ledger rows and, additively, the old DTO.
- Every fixture comes from a scrubbed capture.
