# S1b — The chat owns its conversation; the runner is a supervisor: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The chat ledger owns `{provider, sessionId, state}`; resume is always by the stored id and only the CLI's explicit refusal loses a session (T3, closes symptom 2). The runner aggregate is replaced by an in-memory map keyed by chat whose durable facts are `runner_started`/`runner_exited` rows (T4).

**Architecture:** See the index, `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. Requires T1–T2 committed: `domain/ledger`, `adapter/store/ledger` (`Store.Do`, `Append`, `State`, `Read`, `OpenChats`, `Subscribe`, `Forget`), and the `kit.FakeProvider` harness.

**Tech Stack:** Go, SQLite, testify, `api/tests`.

**Spec:** architecture §3 ("The chat owns its conversation", "The runner is a supervisor"), §5 (conversation and runner diagrams), §9 (resume hazards); report §2 row "Session id lost"; spike §A1 table, §A3 rules, §A8.

---

### Task 3: The chat owns its conversation (symptom 2)

**Goal:** A session id is chat state in the ledger. Claude's id is minted by Crowbar and passed with `--session-id` at launch; Codex's is taken from its announcement. Every later launch resumes by that id. Only an exit that the descriptor recognises as the CLI's refusal moves the conversation to `lost`. Session inference, the quarantine, the provider-folder search and the transcript rung are deleted.

**Files:**
- Create: `api/internal/domain/ledger/conversation.go`, `marker.go` and `conversation_test.go`, `marker_test.go`
- Modify: `api/internal/domain/ledger/{row.go,state.go,state_test.go}` — kinds `KindSessionBound`, `KindSessionLost`, `KindMarker`; conversation fold
- Modify: `api/internal/adapter/store/ledger/{schema.go,store.go,read.go,store_test.go}` — the session index (below)
- Modify: `api/internal/engine/agents/internal/spec/descriptor.go` — `SessionSpec{Resume, Assign *ArgSpec; Refused *SessionRefusedSpec}`; delete `Locate`/`SessionLocateSpec`
- Modify: `api/internal/engine/agents/agents.go` — `Agent` gains `SessionAssignArg() (string, bool)` and `RefusedResume(exitCode int, output string) (line string, refused bool)`; delete `SessionExists`, `Decide`
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml` and `codex.yaml` — `session:` blocks (below); delete their `locate:` blocks and the comments about them
- Modify: `api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema/vocabulary.yaml` (or wherever `session.locate` is declared in the schema) and `api/internal/engine/agents/descriptorcheck/` rules that mention `locate`
- Modify: `api/internal/core/terminal/{terminal.go,lifecycle.go,lifecycle_test.go}` — `CreateCommand(…, onExit func(Exit))` with `type Exit struct{ Code int; Screen string }`, filled at reap from the session's exit code and its final screen text (the same text `Screen(sessionID, 0)` serves); every caller updated (`api/internal/app/usecases/chat/internal/shared/seam/seam.go` `TerminalCommander`, runner spawn/attach, terminal usecase callers pass a func that ignores the value)
- Modify: `api/internal/app/usecases/chat/internal/runner/{spawn.go,spawnsteps.go,spawnplan.go,promptdelivery.go,switch.go,resume.go,revive.go,apiconn.go,apirunner.go,supervisor.go,lifecycle.go,resume_injection.go}`
- Modify: `api/internal/app/usecases/chat/internal/turn/{ingest.go,conversation.go}` — `session_start` goes to the new decision; the identity filter compares with the ledger binding
- Modify: `api/internal/app/usecases/chat/internal/shared/snapshot/snapshot.go` and `snapshot_test.go` — `LedgerFacts`, `ApplyLedger` (below)
- Modify: `api/internal/api/v0/dto/agent.go` — `AgentChatDTO.Conversation *AgentConversationDTO` (`providerId`, `sessionId`, `state`), omitted when unbound; `ChatRuntime.Conversation`; `AgentChatDetailDTO.Conversations` built from the ledger
- Modify: `api/internal/domain/agent_session.go` — delete `AgentRungTranscript`, `AgentRungSession`, `AgentRungFresh` and the `Rung` field (`session.rung` is no longer sent; the client's `describeRung` then never fires, which is the honest reading)
- Delete: `api/internal/app/usecases/chat/internal/runner/{ladder.go,session.go,sessionorigin.go}` and `{session,sessionorigin}_internal_test.go`; `api/internal/engine/agents/session_locate.go`; `api/internal/engine/agents/internal/sessionstore/`; `api/internal/engine/agents/internal/move/`; `api/internal/engine/agents/internal/models/move.go` and the `Move*` aliases in `aliases.go`; runner aggregate commands `api/internal/engine/agents/runner/internal/commands/{bind_session.go,move.go}` with their event-store methods, the `CurrentSession`, `CurrentSessionSince`, `CurrentSessionResumable`, `LaunchSessionID` fields of `models.Runner` and their columns, the `agent_chat_conversations` projection and both heal markers (`runner/internal/store/{heal.go,…}`), `ChatForSession`, `LiveRunnersForSession`, `LastConversation`, `ConversationsForChat`
- Delete tests of deleted behaviour: move/clear tests in `api/tests/agent_runner_moves_test.go` (keep `TestRegression_DeleteChat_LeavesProviderSessionIntact` and `TestRegression_RenameResolvesChatAtCallTime`), `TestAgent_LiveClearRegistersNewChat` in `api/tests/integration/agent/agent_gaps_test.go`, ladder/quarantine/locate cases in `api/internal/app/usecases/chat/runner_test.go`, `switch_test.go`, `internal/runner/{resume,switch}_internal_test.go`
- Test (black box): create `api/tests/regression_chat_session_test.go`

**Interfaces:**
- Consumes: `ledgerstore.Store` (T2); `kit.FakeProvider`, `Launch.Kill`, `FakeProvider.RefuseResume` (T1).
- Produces, `package ledger`:

```go
type SessionBound struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Cause     string `json:"cause"` // CauseAssigned | CauseAnnounced | CauseFresh
	Previous  string `json:"previous,omitempty"`
}
type SessionLost struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}
type Marker struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}
const (MarkerSessionChanged = "session_changed"; MarkerSessionConflict = "session_conflict")

type ConversationState string // ConversationUnbound | ConversationBound | ConversationLost
// Next reports the state a transition lands in and whether the conversation
// diagram (architecture §5) allows it: unbound→bound, bound→lost, lost→bound,
// bound→bound.
func (s ConversationState) Next(to ConversationState) (ConversationState, bool)

type Conversation struct {
	Provider  string
	SessionID string
	State     ConversationState
	BoundAt   time.Time
}
func (s State) Conversation(provider string) (Conversation, bool)
func (s State) Conversations() []Conversation // every (provider, session) ever bound, oldest first
```

- Produces, store:

```go
// SessionOwner answers which chat a (provider, session) is bound to.
func (s *Store) SessionOwner(ctx context.Context, provider, sessionID string) (chatID string, ok bool, err error)
var ErrSessionClaimed = errors.New("ledger: session is bound to another chat")
```

Schema addition, maintained in the same transaction as each `session_bound` row; a `session_bound` whose (provider, session) already maps to another chat fails the whole `Do` with `ErrSessionClaimed`:

```sql
CREATE TABLE chat_ledger_sessions (
  provider TEXT NOT NULL, session_id TEXT NOT NULL, chat_id TEXT NOT NULL,
  PRIMARY KEY (provider, session_id)) WITHOUT ROWID;
```

**Descriptor values** (exact):

```yaml
# claude.yaml
session:
  resume:  { arg: "--resume {id}" }
  assign:  { arg: "--session-id {id}" }
  refused: { exit_code: 1, needles: ["No conversation found with session ID"] }
# codex.yaml
session:
  resume:  { arg: "resume {id}" }
  refused: { exit_code: 1, needles: ["no rollout found for thread id"] }
```

`RefusedResume` returns the first line of `output` containing any needle, and `refused == true` only when `exitCode == Refused.ExitCode` and a needle matched. Both conditions are required.

**Launch decision** (replaces `resumeTarget`, `resumableConversation`, `ladderTarget`, `verifiedResume`; one function `launchSession(st ledger.State, d engineagents.Agent) (resumeID string, assign *ledger.Row)` in `runner/promptdelivery.go`):
- the provider's conversation is `bound` → resume with its `SessionID` (`session.resume` arg); never `--continue`/`--last`;
- unbound or `lost`, descriptor declares `assign` → mint `uuid.NewString()`, append `session_bound{cause: assigned}` (or `fresh` when the previous state was `lost`) **before** the fork, pass the `assign` arg;
- unbound or `lost`, no `assign` → launch fresh; the binding arrives with the announcement.

**Announcement decision** (`session_start` hook, and the api connection's `SessionOrigin` settle callback — which replaces `originatedSessions`), under `Store.Do`:
- id equals the provider's bound id → no row;
- provider unbound → `session_bound{cause: announced}`;
- id differs → `SessionOwner`: owned by another chat → `marker{session_conflict, detail: id}`, binding unchanged; else `session_bound{cause: announced, previous}` + `marker{session_changed, detail: id}`.
The hook's `model`/`effort` keep updating the live runner's reported model as today. No runner moves, no chat is minted.

**Identity filter** (`turn/conversation.go`): a non-`session_start` hook whose session id is non-empty and differs from the provider's bound id is another conversation (nested-subagent routing runs first, as today). No timestamps, no runner fields, no api-origin set.

**Refusal** (runner exit handler and the api `thread/resume` error path in `apiconn.go`): when the runner launched with a resume id and `d.RefusedResume(exit.Code, exit.Screen)` (or, on the api channel, the RPC error message) matches → one `Store.Do` appending `session_lost{reason: <matched line>}` and, for a pending prompt, `prompt_settled{consumed:false, reason: session_lost}`; the exit reason is `domain.AgentExitResumeFailed`. The api path no longer opens a replacement thread and no longer hands over a transcript.

**DTO and snapshot (lock order from the index):** add to `api/internal/app/usecases/chat/internal/shared/snapshot/snapshot.go`:

```go
// LedgerFacts are the ledger-derived parts of a chat snapshot.
type LedgerFacts struct {
	Conversations []ledger.Conversation // T3
	LastExit      *ledger.RunnerExit    // T4
	Busy          bool                  // T5
}
// ApplyLedger installs facts published by the chat's ledger writer, bumps the
// chat's version and emits a snapshot frame when they changed.
func (s *Snapshots) ApplyLedger(ctx context.Context, chatID string, facts LedgerFacts)
```

The ledger subscriber registered in `assembly.go` calls `ApplyLedger` for every published batch. `Snapshots.Get` for a chat it has never received facts for reads `Store.State` **before** taking its own lock and installs the result only if publication has not installed facts meanwhile. `ChatRuntime.Conversation` is the conversation of the chat's current provider (live runner's provider, else `chat.ProviderID`); `ConversationsForChat` returns the snapshot's conversations mapped to `agents.ChatConversation{ChatID, ProviderID, SessionID, FirstSeenAt: BoundAt, LastActiveAt: BoundAt}`.

- [ ] **Step 1: Domain and store tests**

```go
func TestConversationState_Next(t *testing.T)
// table over all 9 (from,to) pairs: allowed exactly unbound→bound, bound→lost,
// lost→bound, bound→bound

func TestFold_ConversationPerProvider(t *testing.T)
// session_bound{p1,S1,assigned}, session_bound{p2,X1,announced}, session_lost{p1,S1},
// session_bound{p1,S2,fresh,previous:S1}
// Conversation("p1") == {S2, bound}; Conversation("p2") == {X1, bound};
// Conversations() lists S1, X1, S2 in that order

func TestStore_ASessionCanBeBoundToOneChatOnly(t *testing.T)
// c1 binds (p,S); c2 Append session_bound(p,S) → ErrSessionClaimed, c2 Head unchanged;
// SessionOwner(p,S) == c1; Forget(c1) → SessionOwner(p,S) not ok
```

In `api/internal/engine/agents/agents_test.go` (extend):

```go
func TestRefusedResume_NeedsTheExitCodeAndTheLine(t *testing.T)
// claude descriptor: (1, "…No conversation found with session ID: abc…") → refused, line;
// (1, "Killed") → not refused; (143, "No conversation found…") → not refused
```

In `api/internal/core/terminal/lifecycle_test.go` (extend):

```go
func TestCreateCommand_OnExitCarriesCodeAndFinalScreen(t *testing.T)
// argv: a test helper process that prints "bye" and exits 3 → onExit receives
// Exit{Code: 3, Screen containing "bye"}
```

- [ ] **Step 2: Symptom 2 black-box tests** — `api/tests/regression_chat_session_test.go`, fake provider `Base: "claude"` (the shipped `session:` block now carries `assign` and `refused`):

```go
func TestRegression_Symptom2_ALaunchCarriesASessionIDTheChatAlreadyOwns(t *testing.T)
// create chat; l := AwaitLaunch → argv contains "--session-id" followed by S (uuid)
// GET …/chats/<id> → conversation == {providerId:"claude", sessionId:S, state:"bound"}

func TestRegression_Symptom2_EveryLaterLaunchResumesTheBoundSession(t *testing.T)
// post session_start{S}, user_prompt, turn_stop; POST prompt 2 → new launch argv has
// "--resume" S and no "--session-id"
// l2.Kill(t) with no hook (the old 30 s quarantine path); POST prompt 3 → argv "--resume" S
// h.shutdown(); h2 := newHarnessAt(t, home); POST prompt 4 → argv "--resume" S

func TestRegression_Symptom2_HooksBeforeTheAnnouncementNeverUnbindTheSession(t *testing.T)
// resumed launch; post user_prompt{S} and message_delta before session_start{S}
// → conversation still {S, bound}; GET …/messages contains the user text

func TestRegression_Symptom2_OnlyAnExplicitRefusalLosesTheSession(t *testing.T)
// fake.RefuseResume(t, S, "No conversation found with session ID: "+S)
// POST prompt → the resumed launch exits 1 → conversation.state "lost";
// session.exitReason "resume_failed"; the chat feed carries prompt_settled{promptConsumed:false}
// POST prompt again → argv has "--session-id" S2, S2 != S; conversation {S2, bound}
// separately: a launch killed with SIGTERM (exit 143, no line) leaves {S, bound}

func TestRegression_Symptom2_AnAnnouncedSessionChangeStaysOnTheChat(t *testing.T)
// live runner posts session_start{S2} (S2 != S) → conversation {S2, bound};
// GET …/chats lists the same number of chats as before; liveRunnerId unchanged;
// detail conversations lists S then S2

func TestRegression_Symptom2_ASessionOwnedByAnotherChatIsNeverTaken(t *testing.T)
// chat A bound SA; chat B's runner posts session_start{SA} → B's conversation unchanged,
// A's unchanged, both still listed

func TestRegression_Symptom2_SwitchingBackResumesTheProvidersOwnSession(t *testing.T)
// install a second fake (Base "claude", ID "claudealt"); chat on "claude" bound S;
// prompt staged with provider "claudealt" (its own session A1), then staged back to
// "claude" → that launch's argv has "--resume" S
```

- [ ] **Step 3: Run them; the refusal, kill, restart and switch-back cases FAIL today**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom2_' ./tests/`
Expected: FAIL — no `--session-id` at launch; the killed-launch case resumes from the transcript; a restart re-infers.

- [ ] **Step 4: Implement**: domain kinds and fold; store session index; descriptor fields and values; terminal `Exit`; launch and announcement decisions; refusal handling; DTO fields; delete the files listed above and every caller of the removed runner fields; delete the obsolete tests.

- [ ] **Step 5: Run to green**

Run: `cd api && go test -tags noEmbed ./internal/domain/ledger/... ./internal/adapter/store/ledger/... ./internal/engine/agents/... ./internal/core/terminal/... ./internal/app/usecases/chat/... ./internal/api/...` → PASS.
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom2_|TestFakeProvider_|TestRegression_(AgentBootReconcile|AfterRestart|InternalProviderSession|InternalSessionHooks|DeleteChat_LeavesProviderSessionIntact|RenameResolvesChatAtCallTime|ProviderSwitchRecordsAMarker|SwitchingToTheSameProvider)' ./tests/` → PASS.
Run: `cd api && make descriptor-check lint deadcode && go vet -tags noEmbed ./...` → clean.
Run: `grep -rn 'locate\|Quarantin\|transcript rung\|MoveToNew' api/internal --include=*.go --include=*.yaml` → nothing outside tests.

- [ ] **Step 6: Commit**

```bash
git add -A api
git commit -m "fix(api): the chat owns its provider session; resume only by its id

A session is lost only on the CLI's explicit refusal. Delete the resume
ladder, quarantine, provider-folder lookup, transcript rung and runner moves.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** all seven `TestRegression_Symptom2_*` green; nothing in non-test Go reads a provider's home; the runner aggregate carries no session field; `agent_chat_conversations` no longer exists.

---

### Task 4: The runner is a supervisor

**Goal:** Which CLI runs a chat is an in-memory map with one entry per chat; its durable facts are `runner_started` and `runner_exited` rows. The runner aggregate, its event log, read model, placement history and heal code are deleted. At boot every chat the ledger shows with a live runner gets `runner_exited{daemon_restart}`.

**Files:**
- Create: `api/internal/domain/ledger/runner.go` and `runner_test.go`
- Create: `api/internal/app/usecases/chat/internal/runner/live.go` and `live_internal_test.go`
- Modify: `api/internal/domain/ledger/{row.go,state.go}` — kinds `KindRunnerStarted`, `KindRunnerExited`; `HasOpenWork` true while a runner is live
- Modify: `api/internal/app/usecases/chat/internal/runner/{runner.go,spawn.go,spawnrecord.go,apirunner.go,attach.go,processes.go,stop.go,lifecycle.go,phase.go,resume.go,revive.go,switch.go,prompts.go,capabilities.go,surface.go,terminalwait.go,types.go,errors.go,supervisor.go}` — every runner-store call goes to the live table or the ledger
- Modify: `api/internal/app/usecases/chat/{assembly.go,chat.go,runner.go,aliases.go,providers.go}`, `internal/fanout/fanout.go` (runner half deleted), `internal/shared/snapshot/snapshot.go` (`Reader.AllLive` reads the live table), `internal/provider/mcp.go` (runner → chat through the live table), `internal/turn/{ingest.go,turns.go}` (runner resolution through the live table)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/*.go` — the dormant sentinel is `chat.ErrNoLiveRunner` (keep the 404-free "dormant" reading the handlers give `agentrunner.ErrNotFound` today); `api/internal/api/libs/status.go` mapping updated
- Modify: `api/internal/app/repositories/container.go`, `api/internal/app/container.go` (`reconcileAgentRunners` becomes the ledger boot pass; `shutdownAgentRunners` reads the live table), `api/internal/adapter/container.go` (drop the `agent_runner` event, snapshot and read DBs)
- Modify: `api/internal/app/upgrade.go` — delete the `chat-providers-from-history` step (its source, the runner placement history, is deleted here; the layout-1 conversion's session binding, T12, replaces it). If the migration-engine plan has landed, the step lives in `api/internal/app/legacy_steps.go` and `api/internal/adapter/migrations/catalog.go` instead: remove it there, and from that plan's catalog tests
- Modify: `api/internal/api/v0/dto/agent.go` — `ChatRuntime.LiveRunner` from the live table; `Session` (exit reason and time) from `LedgerFacts.LastExit` (T3's snapshot facts, filled from `State.LastExit()`)
- Delete: `api/internal/engine/agents/runner/` (whole package, all tests); `api/internal/engine/agents/internal/models/runner.go` `ChatPlacement` (and its alias); `api/internal/app/usecases/chat/internal/conversation/provider_backfill.go` and `api/internal/app/usecases/chat/provider_backfill_test.go`; `RestateProvidersFromHistory` call at boot; `lastActiveProviderID`'s placement fallback in `runner/resume.go` (resume uses `chat.ProviderID`); `runner/lifecycle.go` `retireOthersOn`, `evictHolderOf`, `retireOlderThan`, `requirePlacement`, `ReconcileRunnersOnBoot`; `sessionBook` remnants in `runner/supervisor.go` (the file is deleted once empty); `TurnUsecase.Interruptions` and its handler use (the dormant provider fallback it fed is gone)
- Delete tests of deleted behaviour: `api/tests/regression_resume_provider_from_runner_history_test.go` (keep `TestRegression_FreshlyCreatedChatCarriesItsProviderFromBirth` and `TestRegression_ResumeWithNoEvidenceAnywhereFailsHonestlyAndCreatesNoChat` by moving them into `api/tests/agent_active_provider_test.go`); placement/eviction cases in `api/internal/app/usecases/chat/runner_test.go` and `internal/runner/lifecycle_internal_test.go`
- Test (black box): extend `api/tests/agent_boot_reconcile_test.go` and `api/tests/agent_stop_test.go`

**Interfaces:**
- Consumes: T2 store; T3 `Conversation`, `RefusedResume`, `terminal.Exit`.
- Produces, `package ledger`:

```go
type RunnerStarted struct {
	RunnerID, Provider, TerminalSession   string // json: runnerId, provider, terminalSession
	LaunchModel, LaunchEffort             string // json: launchModel, launchEffort
	LaunchPermissionLevel, ResumeSessionID string // json: launchPermissionLevel, resumeSessionId
}
type RunnerExited struct {
	RunnerID string `json:"runnerId"`
	Reason   string `json:"reason"` // a domain.AgentExit* value
	ExitCode int    `json:"exitCode"`
}
type RunnerExit struct{ RunnerID, Reason string; ExitCode int; At time.Time }
func (s State) LiveRunner() (RunnerStarted, bool) // a runner_started with no matching runner_exited
func (s State) LastExit() (RunnerExit, bool)
```

- Produces, `runner` package (unexported table, exported reads through `Runners`):

```go
type liveTable struct{ /* mu; byChat, byRunner map[string]*liveEntry */ }
type liveEntry struct {
	runner  agents.Runner  // models.Runner without the deleted session fields
	exiting bool
	exited  chan struct{}  // closed by remove
	cause   string         // exit reason noted by Stop/switch; "" = exited on its own
}
func (t *liveTable) add(r agents.Runner) error // ErrChatHasLiveRunner while an entry exists for r.CurrentChatID
func (t *liveTable) byChat(chatID string) (agents.Runner, bool)
func (t *liveTable) byRunner(runnerID string) (agents.Runner, bool) // also while exiting
func (t *liveTable) markExiting(runnerID, cause string) (<-chan struct{}, bool)
func (t *liveTable) remove(runnerID string) (liveEntry, bool)
func (t *liveTable) all() []agents.Runner

func (rs *Runners) LiveRunnerForChat(ctx context.Context, chatID string) (agents.Runner, error) // ErrNoLiveRunner when dormant
func (rs *Runners) AllLive(ctx context.Context) ([]agents.Runner, error)
func (rs *Runners) RunnerByID(runnerID string) (agents.Runner, bool)
```

**Lifecycle** (no timers; replaces the aggregate's Start/Displace/Exit and eviction):
1. Spawn: build the plan → `add` (so its hooks resolve from the first byte) → `Store.Append(runner_started{…})` → fork. A fork error removes the entry and appends `runner_exited{spawn_failed}` (+ the T2 prompt settle).
2. Replacement (restart per prompt, switch, surface move): `markExiting(old, cause)`, terminate gracefully, wait on the returned channel (closed by step 3, not by a clock), then spawn. A hook from the exiting runner still resolves to its chat (fixes report L5).
3. Exit callback (`onExit(terminal.Exit)`): `remove` → one `Store.Do` appending `runner_exited{reason: cause or refusal (T3) or exited, exitCode}` plus the T2 prompt settle → `snapshots.ApplyRunner(ctx, runner, previous, seq of the runner_exited row, "exited")`.
4. Boot (`api/internal/app/container.go`, replacing `reconcileAgentRunners`): for each chat in `Store.OpenChats` whose `State.LiveRunner()` is set → reap that runner's tmp dir exactly as `reapCrashOrphanRunnerTmp` does today, then append `runner_exited{daemon_restart}` (+ prompt settle).
5. Snapshot and frames: `ApplyRunner` is called with kind `started` after step 1's append and `exited` after step 3's; the version argument is the row's `seq`. The `displaced` and `moved` kinds are no longer produced (no runner leaves a chat while running; R6 removed moves); the web's handling of them stays dead but harmless.

- [ ] **Step 1: Unit tests**

```go
// api/internal/domain/ledger/runner_test.go
func TestFold_LiveRunnerAndLastExit(t *testing.T)
// runner_started{r1}, runner_exited{r1,"stopped",0}, runner_started{r2}
// LiveRunner().RunnerID == "r2"; LastExit() == {r1,"stopped"}; HasOpenWork() true
// then runner_exited{r2,"exited",143} → no LiveRunner; LastExit().Reason "exited"

// api/internal/app/usecases/chat/internal/runner/live_internal_test.go
func TestLiveTable_OneEntryPerChat(t *testing.T)
// add(r1 on c) ok; add(r2 on c) → ErrChatHasLiveRunner; remove(r1); add(r2 on c) ok
func TestLiveTable_AnExitingRunnerStillResolvesUntilItsExit(t *testing.T)
// markExiting(r1) → byRunner(r1) ok, byChat(c) ok; channel open; remove(r1) closes it
// → byRunner(r1) not ok
```

- [ ] **Step 2: Black-box tests (extend existing files)**

```go
// api/tests/agent_boot_reconcile_test.go
func TestRegression_ARunnersExitReasonSurvivesARestart(t *testing.T)
// create chat, l.Kill → session.exitReason "exited"; h.shutdown(); h2 on the same
// home → GET …/chats/<id> session.exitReason "exited" (today: lost with memory)
func TestRegression_ABootWithALiveRunnerRecordsDaemonRestart(t *testing.T)
// create chat (runner live); h.crash(); h2 on the same home → liveRunnerId "",
// session.exitReason "daemon_restart"; POST prompt → a new launch is recorded

// api/tests/agent_stop_test.go
func TestRegression_AReplacementWaitsForTheOldCLIToExitNotAClock(t *testing.T)
// POST prompt 1 then prompt 2 (restart per message): the fake recorded launch 2
// only after launch 1's process was gone (launch-2 capture exists ⇒ process 1 no
// longer running: check with syscall.Kill(l1.PID, 0) == ESRCH)
```

`TestRegression_AgentStopKeepsChatDormantAndResumable`, `TestRegression_AgentCloseEndsTheLiveRunnerAndKeepsTheChat`, `TestRegression_AgentChatActiveProviderID*`, `TestAgentREST_Scope`, `TestRegression_ProviderThatExitsDuringStartupIsRefused`, `TestRegression_RefusedSpawnLeavesNoChatBehind` must stay green.

- [ ] **Step 3: Run; the restart-durability test FAILS today**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(ARunnersExitReason|ABootWithALiveRunner|AReplacementWaits)' ./tests/` → `ARunnersExitReasonSurvivesARestart` FAILS.

- [ ] **Step 4: Implement the fold, the live table, the lifecycle above; delete the runner aggregate and everything listed; bring each edited runner file under 500 lines**

- [ ] **Step 5: Run to green**

Run: `cd api && go test -tags noEmbed ./internal/domain/ledger/... ./internal/app/... ./internal/api/... ./internal/adapter/...` → PASS.
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(ARunnersExitReason|ABootWithALiveRunner|AReplacementWaits|AgentStop|AgentClose|AgentChatActiveProviderID|AgentBootReconcile|AfterRestart|ProviderThatExitsDuringStartup|RefusedSpawn|Symptom2_)|TestAgentREST_Scope|TestAgentWS_|TestFakeProvider_' ./tests/` → PASS.
Run: `cd api && make lint deadcode && go vet -tags noEmbed ./...`; `test ! -d api/internal/engine/agents/runner`.

- [ ] **Step 6: Commit**

```bash
git add -A api
git commit -m "refactor(api): the runner is an in-memory supervisor; its facts are ledger rows

Delete the runner aggregate, its read model, placement history and heal code.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** "one runner per chat" is the map-key invariant; a chat's last exit reason survives a restart; `agent_runner*.db` is no longer opened; the deadcode gate is clean.
