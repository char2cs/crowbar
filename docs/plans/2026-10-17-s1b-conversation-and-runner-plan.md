# S1b — The chat owns its conversation; the runner is a supervisor: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The chat ledger owns `{provider, sessionId, state}`. Resume is always by the stored id, and only the CLI's explicit refusal loses a session (T3, which closes symptom 2). The runner aggregate is replaced by an in-memory map keyed by chat; its durable facts are `runner_started` and `runner_exited` rows (T4).

**Architecture:** The index is `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. These tasks require T1–T2 committed:
- `domain/ledger` (`Row`, `NewRow`, `DecodeBody`, `Check`, `Fold`, `State` with `Head`, `HasOpenWork`, `PendingPrompts`, `Receipt`);
- `adapter/store/ledger`, imported as `ledgerstore` (`Do`, `Append`, `State`, `Read`, `OpenChats`, `Subscribe`, `Forget`, `FileName`);
- prompts as `user_message`/`turn_started`/`prompt_settled` rows;
- the `kit.FakeProvider` harness (`InstallFakeProvider`, `AwaitLaunch`, `Launch.PostHook`, `Launch.Kill`, `RefuseResume`, `BreakSpawn`).

**Tech Stack:** Go, SQLite, testify, `api/tests`, the migration engine (`api/internal/core/migrate`, `api/internal/adapter/migrations`).

**Spec:** architecture §3 ("The chat owns its conversation", "The runner is a supervisor"), §5 (conversation and runner diagrams), §9 (resume hazards); report §2 row "Session id lost"; spike §A1 table, §A3 rules, §A8.

---

### Task 3: The chat owns its conversation (symptom 2)

**Goal:** A session id is chat state in the ledger:
- Claude's id is minted by Crowbar and passed with `--session-id` at launch. Codex's id is taken from its announcement.
- Every later launch resumes by that id.
- Only an exit that the descriptor recognises as the CLI's refusal moves the conversation to `lost`.
- Deleted: session inference, the quarantine, the provider-folder search and the transcript rung.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- Resume only by the stored session id, never by `--continue`/`--last`.
- Never read a provider's home or transcript. The `session.locate` lookup is deleted here; until it is, `kit.InstallFakeProvider` points `CLAUDE_CONFIG_DIR`/`CODEX_HOME` at temp dirs.
- No provider vocabulary in non-test Go: refusal needles live in the descriptors.
- Behaviour changes, accepted provisionally on 2026-10-18 (the user can override them before this task lands):
  - `/clear` typed in the TUI keeps the same chat and shows a `session_changed` marker;
  - `/resume` of another chat's session shows `session_conflict`, and the binding stays;
  - a chat whose provider session was pruned starts fresh, with no memory of the conversation. No transcript is ever handed over.

**Files:**
- Create: `api/internal/domain/ledger/conversation.go`, `marker.go` and `conversation_test.go`, `marker_test.go`
- Modify: `api/internal/domain/ledger/{row.go,state.go,state_test.go}` — kinds `KindSessionBound`, `KindSessionLost`, `KindMarker`; the conversation fold
- Modify: `api/internal/adapter/store/ledger/{schema.go,store.go,read.go,store_test.go}` — the session index (below)
- Modify: `api/internal/engine/agents/internal/spec/descriptor.go` — `SessionSpec{Resume, Assign *ArgSpec; Refused *SessionRefusedSpec}`; delete `Locate`/`SessionLocateSpec`
- Create: `api/internal/engine/agents/session.go` and `session_test.go` — implementations of the two new `Agent` methods:
  - `SessionAssignArg() (string, bool)`;
  - `RefusedResume(exitCode int, output string) (line string, refused bool)`.
- Modify: `api/internal/engine/agents/agents.go` (718 lines) — the `Agent` interface gains the two method lines; delete `SessionExists` and `Decide` (`agents.go:428`). The file must net-shrink.
- Modify: `api/internal/engine/agents/agents_test.go` (1,774) — deletions only. New cases go in `session_test.go`.
- Modify: `descriptors-v3/claude.yaml` and `codex.yaml` under `api/internal/engine/agents/internal/protocol/internal/descriptor/` — the `session:` blocks (below); delete their `locate:` blocks and the comments about them.
- Modify: wherever the descriptor schema declares `session.locate` (under `…/descriptor/internal/schema/`), and the `api/internal/engine/agents/descriptorcheck/` rules that mention `locate`.
- Modify: `api/internal/core/terminal/{terminal.go,lifecycle.go,lifecycle_test.go}`:
  - `CreateCommand(…, onExit func(Exit))`, with `type Exit struct{ Code int; Screen string }` declared in `terminal.go` (426 lines; keep it ≤ 500);
  - `Exit` is filled at reap from the session's exit code and its final screen text (the same text `Screen(sessionID, 0)` serves);
  - update every caller: `api/internal/app/usecases/chat/internal/shared/seam/seam.go` `TerminalCommander`, runner spawn/attach, and the terminal usecase callers (which pass a func that ignores the value).
  - `lifecycle.go` is 756 lines. If the reap change adds lines, split it below 500 in this task by concept (for example the reap path into `reap.go`, with `reap_test.go`), unless trimming narrative comments makes the edit net-shrinking.
- Modify: `api/internal/app/usecases/chat/internal/runner/{spawn.go,spawnsteps.go,spawnplan.go,promptdelivery.go,switch.go,resume.go,revive.go,apiconn.go,apirunner.go,supervisor.go,lifecycle.go,resume_injection.go}`. Each stays ≤ 500 (`spawn.go` is 473). Their oversize `_internal_test.go` files (`apiconn` 534, `attach` 626, `apirunner` 835) only shrink.
- Modify: `api/internal/app/usecases/chat/internal/turn/{ingest.go,conversation.go}` — `session_start` goes to the new decision; the identity filter compares with the ledger binding
- Create: `api/internal/app/usecases/chat/internal/shared/snapshot/ledger.go` and `ledger_test.go` — `LedgerFacts` and `ApplyLedger` (below). `snapshot.go` is 476 lines; only the minimal hook-in lands there, and it stays ≤ 500.
- Modify: `api/internal/app/usecases/chat/placement_reads.go` — `ConversationsForChat` reads the snapshot's conversations (`:22` reads the runner store today)
- Modify: `api/internal/api/v0/dto/agent.go` (1,016 lines):
  - `AgentChatDTO.Conversation *AgentConversationDTO` (`providerId`, `sessionId`, `state`), omitted when unbound; `ChatRuntime.Conversation`; `AgentChatDetailDTO.Conversations` built from the ledger.
  - This adds lines, so split the file below 500 in this task by concept (a pure move, for example chat / runner-session / activity DTO files, each ≤ 500). Later tasks edit the split files.
- Modify: `api/internal/domain/agent_session.go` — delete `AgentRungTranscript`, `AgentRungSession`, `AgentRungFresh` and the `Rung` field. `session.rung` is no longer sent, so the client's `describeRung` never fires.
- Delete:
  - `api/internal/app/usecases/chat/internal/runner/{ladder.go,session.go,sessionorigin.go}` and `{session,sessionorigin}_internal_test.go`;
  - `api/internal/engine/agents/session_locate.go`, `api/internal/engine/agents/internal/sessionstore/` and `api/internal/engine/agents/internal/move/`;
  - `api/internal/engine/agents/internal/models/move.go` and the `Move*` aliases in `api/internal/engine/agents/aliases.go`;
  - runner aggregate commands `api/internal/engine/agents/runner/internal/commands/{bind_session.go,move.go}` with their event-store methods;
  - the `CurrentSession`, `CurrentSessionSince`, `CurrentSessionResumable` and `LaunchSessionID` fields of `models.Runner`, with their columns;
  - the `agent_chat_conversations` projection and both heal markers (`runner/internal/store/{heal.go,…}`);
  - `ChatForSession`, `LiveRunnersForSession`, `LastConversation` and the runner-store `ConversationsForChat`.
- Delete tests of deleted behaviour:
  - the move/clear tests in `api/tests/agent_runner_moves_test.go` (795 lines, deletions only; keep `TestRegression_DeleteChat_LeavesProviderSessionIntact` and `TestRegression_RenameResolvesChatAtCallTime`);
  - `TestAgent_LiveClearRegistersNewChat` in `api/tests/integration/agent/agent_gaps_test.go`;
  - the ladder/quarantine/locate cases in `api/internal/app/usecases/chat/runner_test.go` and `switch_test.go`, and in `internal/runner/{resume,switch}_internal_test.go`.
- Test (black box): create `api/tests/regression_chat_session_test.go`

**Interfaces:**
- Consumes: `ledgerstore.Store` (T2); `kit.FakeProvider`, `Launch.Kill`, `FakeProvider.RefuseResume` (T1).
- Produces, `package ledger`:

```go
const (KindSessionBound Kind = "session_bound"; KindSessionLost Kind = "session_lost"; KindMarker Kind = "marker")
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
	Provider, SessionID string
	State               ConversationState
	BoundAt             time.Time
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

The session index is maintained in the same transaction as each `session_bound` row. A `session_bound` whose (provider, session) already maps to another chat fails the whole `Do` with `ErrSessionClaimed`. `Forget` drops the chat's index rows.

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

`RefusedResume` returns the first line of `output` that contains any needle. `refused == true` requires both `exitCode == Refused.ExitCode` and a matched needle.

**Launch decision.** One function, `launchSession(st ledger.State, d engineagents.Agent) (resumeID string, assign *ledger.Row)` in `runner/promptdelivery.go`. It replaces `resumeTarget`, `resumableConversation`, `ladderTarget` and `verifiedResume`.
- The provider's conversation is `bound` → resume with its `SessionID` (the `session.resume` arg). Never `--continue`/`--last`.
- Unbound or `lost`, and the descriptor declares `assign` → mint `uuid.NewString()` and pass the `assign` arg. Append `session_bound{cause: assigned}` **before** the fork (`cause: fresh` when the previous state was `lost`).
- Unbound or `lost`, and no `assign` → launch fresh; the binding arrives with the announcement.

**Announcement decision**, under `Store.Do`. It handles the `session_start` hook and the api connection's `SessionOrigin` settle callback (which replaces `originatedSessions`):
- The id equals the provider's bound id → no row.
- The provider is unbound → `session_bound{cause: announced}`.
- The id differs → check `SessionOwner`:
  - owned by another chat → `marker{session_conflict, detail: id}`, binding unchanged;
  - otherwise → `session_bound{cause: announced, previous}` + `marker{session_changed, detail: id}`.
- The hook's `model`/`effort` keep updating the live runner's reported model, as today. No runner moves, and no chat is minted.

**Identity filter** (`turn/conversation.go`): a non-`session_start` hook whose session id is non-empty and differs from the provider's bound id belongs to another conversation. Nested-subagent routing runs first, as today. The filter uses no timestamps, no runner fields and no api-origin set.

**Refusal** (the runner exit handler, and the api `thread/resume` error path in `apiconn.go`):
- Applies when the runner launched with a resume id and `d.RefusedResume(exit.Code, exit.Screen)` matches (on the api channel, the RPC error message).
- One `Store.Do` appends `session_lost{reason: <matched line>}` and, for a pending prompt, `prompt_settled{consumed:false, reason: session_lost}`.
- The exit reason is `domain.AgentExitResumeFailed`.
- The api path no longer opens a replacement thread, and no longer hands over a transcript.

**Snapshot facts** (lock order: ledger writer → snapshot owner; the snapshot never calls the store while holding its own lock). In `snapshot/ledger.go`:

```go
// LedgerFacts are the ledger-derived parts of a chat snapshot. T4 adds LastExit,
// T5 adds Busy.
type LedgerFacts struct {
	Conversations []ledger.Conversation
}
// ApplyLedger installs facts published by the chat's ledger writer, bumps the
// chat's version and emits a snapshot frame when they changed.
func (s *Snapshots) ApplyLedger(ctx context.Context, chatID string, facts LedgerFacts)
```

- The ledger subscriber registered in `assembly.go` calls `ApplyLedger` for every published batch.
- For a chat that has never received facts, `Snapshots.Get` reads `Store.State` **before** taking its own lock. It installs the result only if publication has not installed facts meanwhile.
- `ChatRuntime.Conversation` is the conversation of the chat's current provider: the live runner's provider, else `chat.ProviderID`.
- `ConversationsForChat` maps the snapshot's conversations to `agents.ChatConversation{ChatID, ProviderID, SessionID, FirstSeenAt: BoundAt, LastActiveAt: BoundAt}`.

- [ ] **Step 1: Domain, store, descriptor and terminal tests**

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

// api/internal/engine/agents/session_test.go
func TestRefusedResume_NeedsTheExitCodeAndTheLine(t *testing.T)
// claude descriptor: (1, "…No conversation found with session ID: abc…") → refused, line;
// (1, "Killed") → not refused; (143, "No conversation found…") → not refused

// api/internal/core/terminal/lifecycle_test.go (388 lines; keep ≤ 500)
func TestCreateCommand_OnExitCarriesCodeAndFinalScreen(t *testing.T)
// argv: a test helper process that prints "bye" and exits 3 → onExit receives
// Exit{Code: 3, Screen containing "bye"}
```

- [ ] **Step 2: Symptom 2 black-box tests.** In `api/tests/regression_chat_session_test.go`, each with the fake provider `Base: "claude"`; the shipped `session:` block now carries `assign` and `refused`. Pre-flight R18: a test asserts state a user can observe today (the next launch's argv, `liveRunnerId`, the chat list). A test that passes before the fix is labelled characterization in the commit body.

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
// resumed launch (argv "--resume" S); post user_prompt{S} and message_delta before
// session_start{S} → GET …/messages contains the user text; then l.Kill(t) and POST a
// prompt → that launch's argv has "--resume" S

func TestRegression_Symptom2_OnlyAnExplicitRefusalLosesTheSession(t *testing.T)
// fake.RefuseResume(t, S, "No conversation found with session ID: "+S)
// POST prompt → the resumed launch exits 1 → conversation.state "lost";
// session.exitReason "resume_failed"; the chat feed carries prompt_settled{promptConsumed:false}
// POST prompt again → argv has "--session-id" S2, S2 != S; conversation {S2, bound}
// separately: a launch killed with SIGTERM (exit 143, no line) leaves {S, bound}

func TestRegression_Symptom2_AnAnnouncedSessionChangeStaysOnTheChat(t *testing.T)
// live runner posts session_start{S2} (S2 != S) → GET …/chats lists the same number of
// chats as before; liveRunnerId unchanged; detail conversations lists S then S2

func TestRegression_Symptom2_ASessionOwnedByAnotherChatIsNeverTaken(t *testing.T)
// chat A bound SA; chat B's runner posts session_start{SA} → both chats still listed;
// A's liveRunnerId and B's liveRunnerId unchanged (today B's runner moves onto A);
// B's next launch argv does not carry SA

func TestRegression_Symptom2_SwitchingBackResumesTheProvidersOwnSession(t *testing.T)
// install a second fake (Base "claude", ID "claudealt"); chat on "claude" bound S;
// prompt staged with provider "claudealt" (its own session A1), then staged back to
// "claude" → that launch's argv has "--resume" S
```

- [ ] **Step 3: Run them; the refusal, kill, restart, move and switch-back cases FAIL today**

Run: `bbx 'TestRegression_Symptom2_'` → FAIL: no `--session-id` at launch; the killed-launch case resumes from the transcript; a restart re-infers; a foreign session moves the runner.

- [ ] **Step 4: Implement**:
- the domain kinds and the fold; the store's session index;
- the descriptor fields and values; the terminal `Exit`;
- the launch and announcement decisions; refusal handling; the DTO fields and the DTO split;
- delete the files listed above, every caller of the removed runner fields, and the obsolete tests.

- [ ] **Step 5: Run to green**

Run: `gox go test -tags noEmbed -cover ./internal/domain/ledger/... ./internal/adapter/store/ledger/... ./internal/engine/agents/... ./internal/core/terminal/... ./internal/app/usecases/chat/... ./internal/api/...` → PASS. `domain/ledger` and `adapter/store/ledger` stay ≥ 92%.
Run: `bbx 'TestRegression_Symptom2_|TestFakeProvider_|TestRegression_(AgentBootReconcile|AfterRestart|InternalProviderSession|InternalSessionHooks|DeleteChat_LeavesProviderSessionIntact|RenameResolvesChatAtCallTime|ProviderSwitchRecordsAMarker|SwitchingToTheSameProvider)'` → PASS.
Run: `gox make descriptor-check` and `gates` → clean.
Run: `grep -rnE 'session\.locate|SessionLocate|Quarantin|MoveToNew|AgentRungTranscript' api/internal --include=*.go --include=*.yaml | grep -v _test.go` → nothing.

- [ ] **Step 6: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "fix(api): the chat owns its provider session; resume only by its id

A session is lost only on the CLI's explicit refusal. Delete the resume
ladder, quarantine, provider-folder lookup, transcript rung and runner moves.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- All seven `TestRegression_Symptom2_*` pass.
- Nothing in non-test Go reads a provider's home.
- The runner aggregate carries no session field, and `agent_chat_conversations` is no longer written.
- `dto/agent.go` is split below 500, and every edited file is ≤ 500 lines or shrank.

---

### Task 4: The runner is a supervisor

**Goal:** Which CLI runs a chat is an in-memory map with one entry per chat; its durable facts are `runner_started` and `runner_exited` rows.
- Deleted: the runner aggregate, its event log, read model, placement history and heal code.
- At boot, every chat the ledger shows with a live runner gets `runner_exited{daemon_restart}`.
- Migration step 4 `chat-providers-from-history` stays. Its history now comes from a read-only reader of the legacy databases, because the aggregate that held that history is deleted here.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- No timers: a replacement waits on the old process's exit channel, not a clock.
- Step 4 keeps its number, name, constant (`migrations.StepChatProviders`) and marker adoption. The engine tests that enumerate steps 1..5 stay unchanged (pre-flight R2, R24).
- A legacy reader lives inside its step's package, `api/internal/adapter/migrations/chatledger/` (store module), and takes a `home.Home`. It never builds a home path by hand (pre-flight R33).
- The restart tests depend on `tests/harness_test.go`'s `crash()` releasing the home lock (`crashDie` closes the migrate session). This was verified at S1 start (pre-flight R32), so `newHarnessAt` on the same home works.
- Coverage checkpoint (pre-flight R15): after this task, report `go test -cover` for every touched package. Do not run `make test-coverage`: it runs `go test ./...`, so CI owns the 92% module floor.

**Files:**
- Create: `api/internal/domain/ledger/runner.go` and `runner_test.go`
- Create: `api/internal/app/usecases/chat/internal/runner/live.go` and `live_internal_test.go`
- Create `api/internal/adapter/migrations/chatledger/` with:
  - `legacy.go` — the legacy row structs and DDL of `agent_chat_conversations` (`chat_id, session_id, workspace_id, provider_id, first_seen_at, last_active_at`) and `agent_chat_placements` (`chat_id, provider_id, workspace_id, last_placed_at`) in `agent_runner.db`, and of `agent_interruptions` (`key, id, turn_id, chat_id, seq, display_order, kind, detail, at, resolved_at`) in `agent_activity.db`. Copy them from `engine/agents/runner/internal/store/{storage.go,placements.go}` and `repositories/chat/activity/internal/store/internal/storage/rows.go` before those are deleted. Struct-only, so it needs no test file.
  - `open.go` and `open_test.go`: `openLegacy(path string) (*sql.DB, bool, error)`.
    - A missing file is empty input (`false`).
    - Otherwise open read-only with `immutable=1`, so no `-wal`/`-shm` is created.
    - When a non-empty `<path>-wal` sits beside it (a crashed old daemon, or a database this daemon still has open), open with `mode=ro` instead. `immutable=1` cannot see pages still in the WAL, and a silent partial read would lose rows.
  - `providers.go` and `providers_test.go`: `NewestProviders(ctx context.Context, h home.Home) (map[string]string, error)`.
    - Chat id → the newest provider among: its conversations (`last_active_at`), its `provider_switched` interruptions (`detail` at `at`), and its placements (`last_placed_at`).
    - The rule is today's `newestPlacedProvider` (`usecases/chat/internal/conversation/provider_backfill.go`), unchanged.
    - It reads `h.Store()/agent_runner.db` and `h.Store()/agent_activity.db`.
- Modify: `api/internal/app/usecases/chat/internal/conversation/provider_backfill.go` and `api/internal/app/usecases/chat/provider_backfill_test.go`:
  - `RestateProvidersFromHistory(ctx, newest map[string]string) (int, error)` and `StaleProviders(ctx, newest map[string]string) ([]string, error)` take that map;
  - delete `newestPlacedProvider` and the runner-store and activity reads;
  - the facade in `api/internal/app/usecases/chat/runner.go` (474 lines) follows and stays ≤ 500.
- Modify: `api/internal/app/legacy_steps.go`:
  - `newLegacyFuncs(repos, ucs, h home.Home)`;
  - `restateProviders` and `staleProviders` become closures that call `chatledger.NewestProviders(ctx, h)`, then the usecase methods above;
  - `api/internal/app/container.go:198` passes `adapters.Home()` (one line, same length).
- Modify: `api/internal/domain/ledger/{row.go,state.go}` — kinds `KindRunnerStarted`, `KindRunnerExited`; `HasOpenWork` is true while a runner is live
- Modify: `api/internal/app/usecases/chat/internal/runner/{runner.go,spawn.go,spawnrecord.go,apirunner.go,attach.go,processes.go,stop.go,lifecycle.go,phase.go,resume.go,revive.go,switch.go,prompts.go,capabilities.go,surface.go,terminalwait.go,types.go,errors.go,supervisor.go}`:
  - every runner-store call goes to the live table or the ledger;
  - each file stays ≤ 500 (`spawn.go` is 473; move code into `live.go` rather than grow it).
- Modify:
  - `api/internal/app/usecases/chat/{assembly.go,chat.go,aliases.go,providers.go,placement_reads.go}`;
  - `internal/fanout/fanout.go` (the runner half is deleted);
  - `internal/shared/snapshot/snapshot.go` (`Reader.AllLive` reads the live table);
  - `internal/provider/mcp.go` (runner → chat through the live table);
  - `internal/turn/{ingest.go,turns.go}` (runner resolution through the live table).
- Modify: `api/internal/api/v0/endpoints/chat/handlers/*.go`:
  - the dormant sentinel is `chat.ErrNoLiveRunner`; keep the 404-free "dormant" reading the handlers give `agentrunner.ErrNotFound` today;
  - `api/internal/api/libs/status.go` mapping updated;
  - `handlers.go` (639) and `chats.go` (611) must net-shrink, or be split below 500 in this task.
- Modify:
  - `api/internal/app/repositories/container.go`, `api/internal/app/usecases/container.go`, `api/internal/app/container.go` (drop `axAgentRunner`; `shutdownAgentRunners` reads the live table), `api/internal/adapter/container.go` (drop the `agent_runner` event, snapshot and read DBs);
  - all deletions, so each oversize container shrinks.
- Modify: the split `api/internal/api/v0/dto/` files (T3):
  - `ChatRuntime.LiveRunner` comes from the live table;
  - `Session` (exit reason and time) comes from `LedgerFacts.LastExit`.
- Modify: `api/internal/app/usecases/chat/internal/shared/snapshot/ledger.go` — `LedgerFacts` gains `LastExit *ledger.RunnerExit`, filled from `State.LastExit()`.
- Delete:
  - `api/internal/engine/agents/runner/` (the whole package, all tests);
  - `models.ChatPlacement` in `api/internal/engine/agents/internal/models/runner.go`, and its alias;
  - `lastActiveProviderID`'s placement fallback in `runner/resume.go` (resume uses `chat.ProviderID`, which step 4 restated);
  - from `runner/lifecycle.go`: `retireOthersOn`, `evictHolderOf`, `retireOlderThan`, `requirePlacement`;
  - the `sessionBook` remnants in `runner/supervisor.go` (the file goes once empty);
  - `TurnUsecase.Interruptions` and its handler use.
- Delete tests of deleted behaviour: the placement/eviction cases in `api/internal/app/usecases/chat/runner_test.go` (deletions only) and `internal/runner/lifecycle_internal_test.go`.
- Modify: `api/tests/regression_resume_provider_from_runner_history_test.go`.
  - Keep all four tests. `TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory` and `TestRegression_ResumeFromRunnerHistoryPicksTheNEWESTPlacement` now prove step 4.
  - Their seeding helper writes the legacy rows straight into `state/store/agent_runner.db` (and `agent_activity.db` for `provider_switched`) with `chatledger`'s DDL.
  - Seed between a crash and a boot on which step 4 has not run, as `tests/base_era_seed_test.go` does with `forgetMigrations`.
- Test (black box): extend `api/tests/agent_boot_reconcile_test.go` and `api/tests/agent_stop_test.go`

**Interfaces:**
- Consumes: the T2 store; T3's `Conversation`, `RefusedResume`, `terminal.Exit` and `LedgerFacts`.
- Produces, `package ledger`:

```go
const (KindRunnerStarted Kind = "runner_started"; KindRunnerExited Kind = "runner_exited")
type RunnerStarted struct {
	RunnerID              string `json:"runnerId"`
	Provider              string `json:"provider"`
	TerminalSession       string `json:"terminalSession"`
	LaunchModel           string `json:"launchModel"`
	LaunchEffort          string `json:"launchEffort"`
	LaunchPermissionLevel string `json:"launchPermissionLevel"`
	ResumeSessionID       string `json:"resumeSessionId"`
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

- Produces, in the `runner` package (an unexported table, with exported reads through `Runners`):

```go
type liveTable struct{ /* mu; byChat, byRunner map[string]*liveEntry */ }
type liveEntry struct {
	runner  agents.Runner // models.Runner without the deleted session fields
	exiting bool
	exited  chan struct{} // closed by remove
	cause   string        // exit reason noted by Stop/switch; "" = exited on its own
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
1. Spawn: build the plan → `add`, so its hooks resolve from the first byte → `Store.Append(runner_started{…})` → fork. A fork error removes the entry and appends `runner_exited{spawn_failed}`, plus the T2 prompt settle.
2. Replacement (restart per prompt, switch, surface move): `markExiting(old, cause)`, terminate gracefully, then wait on the returned channel, which step 3 closes (not a clock), then spawn. A hook from the exiting runner still resolves to its chat (fixes report L5).
3. Exit callback (`onExit(terminal.Exit)`): `remove` → one `Store.Do` appending `runner_exited{reason, exitCode}` plus the T2 prompt settle. The reason is the noted cause, else the T3 refusal, else `exited`. Then call `snapshots.ApplyRunner(ctx, runner, previous, <seq of the runner_exited row>, "exited")`.
4. Boot: `ReconcileRunnersOnBoot` (`runner/lifecycle.go`) keeps its name and its call site (`app/container.go` `reconcileAgentRunners`, which runs before `RunPostOpen`). Its body becomes the ledger pass. For each chat in `Store.OpenChats` whose `State.LiveRunner()` is set:
   - reap that runner's tmp dir exactly as `reapCrashOrphanRunnerTmp` does today;
   - append `runner_exited{daemon_restart}`, plus the prompt settle;
   - then run T2's `reconcilePendingPromptsOnBoot`.
5. Snapshot and frames: `ApplyRunner` is called with kind `started` after step 1's append, and with `exited` after step 3's. The version argument is the row's `seq`. The `displaced` and `moved` kinds are no longer produced; the web's handling of them stays dead but harmless.

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

// api/internal/adapter/migrations/chatledger (test DBs created in WAL mode with legacy.go's DDL)
func TestOpenLegacy_ReadsWithoutWriting(t *testing.T)
// a checkpointed WAL-mode db → rows read; every file's mtime and size unchanged; no -wal/-shm created
func TestOpenLegacy_ReadsRowsStillInTheWAL(t *testing.T)
// rows written and left uncheckpointed (a second connection holds the db open) → all rows read
func TestNewestProviders_TheNewestOfConversationsSwitchesAndPlacementsWins(t *testing.T)
// one chat with each source newest in turn → the expected provider each time
func TestNewestProviders_MissingDatabasesAreEmpty(t *testing.T)
```

- [ ] **Step 2: Black-box tests** (extend existing files)

```go
// api/tests/agent_boot_reconcile_test.go
func TestRegression_ARunnersExitReasonSurvivesARestart(t *testing.T)
// create chat, l.Kill → session.exitReason "exited"; h.shutdown(); h2 := newHarnessAt(t, home)
// → GET …/chats/<id> session.exitReason "exited" (today: lost with memory)
func TestRegression_ABootWithALiveRunnerRecordsDaemonRestart(t *testing.T)
// create chat (runner live); h.crash(); h2 := newHarnessAt(t, home) → liveRunnerId "",
// session.exitReason "daemon_restart"; POST prompt → a new launch is recorded

// api/tests/agent_stop_test.go
func TestRegression_AReplacementWaitsForTheOldCLIToExitNotAClock(t *testing.T)
// POST prompt 1 then prompt 2 (restart per message): the fake recorded launch 2
// only after launch 1's process was gone (launch-2 capture exists ⇒
// syscall.Kill(l1.PID, 0) == ESRCH)
```

These must stay green:
- `TestRegression_AgentStopKeepsChatDormantAndResumable`, `TestRegression_AgentCloseEndsTheLiveRunnerAndKeepsTheChat`;
- `TestRegression_AgentChatActiveProviderID*`, `TestAgentREST_Scope`;
- `TestRegression_ProviderThatExitsDuringStartupIsRefused`, `TestRegression_RefusedSpawnLeavesNoChatBehind`;
- the four tests of `regression_resume_provider_from_runner_history_test.go`;
- `TestRegression_Upgrade_*` (`api/tests/regression_upgrade_boot_steps_test.go`).

- [ ] **Step 3: Run; the restart-durability test FAILS today**

Run: `bbx 'TestRegression_(ARunnersExitReason|ABootWithALiveRunner|AReplacementWaits)'` → `ARunnersExitReasonSurvivesARestart` FAILS.

- [ ] **Step 4: Implement:**
- the fold, the live table, the lifecycle above, the legacy reader and step 4's new history source;
- delete the runner aggregate and everything listed;
- keep every edited runner file ≤ 500.

- [ ] **Step 5: Run to green**

Run: `gox go test -tags noEmbed -cover ./internal/ ./internal/domain/ledger/... ./internal/adapter/... ./internal/app/... ./internal/api/...` → PASS. `./internal/` is the boot package alone, not recursive, and it holds the step tests. `adapter/migrations/chatledger` is new and must be ≥ 92%; report every other touched package's number.
Run: `bbx 'TestRegression_(ARunnersExitReason|ABootWithALiveRunner|AReplacementWaits|AgentStop|AgentClose|AgentChatActiveProviderID|AgentBootReconcile|AfterRestart|ProviderThatExitsDuringStartup|RefusedSpawn|Symptom2_|ResumeDormantChat|ResumeFromRunnerHistory|FreshlyCreatedChat|ResumeWithNoEvidence|Upgrade_)|TestAgentREST_Scope|TestAgentWS_|TestFakeProvider_'` → PASS.
Run: `gates`, then `test ! -d api/internal/engine/agents/runner`.

- [ ] **Step 6: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "refactor(api): the runner is an in-memory supervisor; its facts are ledger rows

Delete the runner aggregate, its read model, placement history and heal code.
Step 4 restates chat providers from a read-only reader of the legacy history.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- "One runner per chat" is the map-key invariant.
- A chat's last exit reason survives a restart.
- `agent_runner*.db` is opened only by step 4's read-only reader.
- Step 4 still restates providers on an upgraded home.
- The deadcode gate is clean, and the per-package coverage numbers are reported.
