# S1a — Behaviour harness and the ledger store: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A black-box harness that drives the daemon with a fake provider CLI and real recorded hook payloads (T1), then the ledger store with its first owner: prompts become `user_message` rows and the fsync'd prompt journal is deleted (T2).

**Architecture:** See the index, `docs/plans/2026-10-17-s1-chat-ledger-plan.md` — its Global Constraints, Rulings, shared row vocabulary and Review Focus apply to every task here.

**Tech Stack:** Go, SQLite (gorm + raw SQL), testify, `api/tests` (`//go:build integration`).

**Spec:** architecture §3 (journal invariants), §4 (a prompt end to end), §5 (turn diagram); report §4.3; spike §A8.

---

### Task 1: Behaviour harness — fake provider CLI and recorded-hook replay

**Goal:** Tests can create a chat whose provider CLI is a small fake binary that records its argv and runner token, post hooks to the real HTTP hook route with that token, and replay a recorded Claude session through the shipped Claude descriptor's own mapping. Characterization tests pin today's behaviour so later tasks prove they keep it.

**Files:**
- Create: `api/tests/kit/fakecli/main.go` (`//go:build integration`, `package main`)
- Create: `api/tests/kit/fakeprovider.go` (`//go:build integration`, `package kit`)
- Create: `api/tests/fake_provider_test.go` (`//go:build integration`, `package tests`)
- Move (`git mv`): `api/internal/app/usecases/chat/testdata/claude-subagent-session/` → `api/tests/testdata/hooks/claude-subagent-session/` (44 already-scrubbed real Claude payloads)
- Modify: `api/internal/app/usecases/chat/turn_test.go:3001` — glob the moved directory (`filepath.Join("..", "..", "..", "..", "tests", "testdata", "hooks", "claude-subagent-session", "*.json")`)

**Interfaces:**
- Consumes: the existing `tests` harness (`newHarness`, `h.url`, `h.home`, `importWritableWorkspace`, `repoBase`, `getAgentChat`, `readUntil`, `h.dial`).
- Produces (all in `package kit`):

```go
// FakeCLIPath builds tests/kit/fakecli once per test binary (sync.Once, `go build
// -tags integration`) into a temp dir and returns the binary path.
func FakeCLIPath(t testing.TB) string

// FakeProvider is a provider descriptor installed under <home>/descriptors whose
// process is the fake CLI.
type FakeProvider struct {
	ID       string // descriptor id the chat is created with
	Dir      string // capture directory the fake CLI writes into
}

// FakeSpec selects what the installed descriptor is built from.
type FakeSpec struct {
	// Base names a shipped descriptor ("claude") whose events, session and
	// presentation blocks are kept verbatim; "" builds from EventsYAML alone.
	Base string
	// ID overrides the installed id; defaults to Base.
	ID string
	// EventsYAML replaces the events: block when non-empty (stub providers).
	EventsYAML string
	// Patch edits the decoded descriptor document before it is written
	// (a test-specific tweak, e.g. an extra field map).
	Patch func(doc map[string]any)
}

func InstallFakeProvider(t testing.TB, home string, spec FakeSpec) FakeProvider

// Launch is what one fake CLI process recorded at start.
type Launch struct {
	RunnerID string   `json:"runnerId"`
	Token    string   `json:"token"`
	Argv     []string `json:"argv"`
	PID      int      `json:"pid"`
}

// AwaitLaunch blocks (require.Eventually, 10s, 20ms tick) until the fake CLI for
// runnerID has written its capture, and returns it.
func (f FakeProvider) AwaitLaunch(t testing.TB, runnerID string) Launch

// RefuseResume makes every later launch that resumes sessionID print the
// provider's refusal line and exit 1 (fakecli reads <Dir>/refuse-<sessionID>).
func (f FakeProvider) RefuseResume(t testing.TB, sessionID, line string)

// Kill sends SIGTERM to the launch's process (an exit with no hook).
func (l Launch) Kill(t testing.TB)

// PostHook posts payload to <baseURL>/chats/runners/<runner>/hooks/<provider>/<event>
// with header X-Crowbar-Runner-Token: l.Token and returns the response.
// baseURL is the server URL plus the repo or home route prefix.
func (l Launch) PostHook(t testing.TB, baseURL, provider, event string, payload []byte) *http.Response

// ClaudeHookRoutes maps a recorded payload's hook_event_name to the canonical
// events the shipped claude descriptor posts it as (Notification goes to two).
var ClaudeHookRoutes map[string][]string

// ReplayHooks posts every *.json file under dir (lexical order) through l, once per
// route in routes[payload.hook_event_name], replacing every key of rewrite with its
// value in the raw bytes first (session ids). A payload whose hook name has no
// route is skipped and returned in the skipped list.
func (l Launch) ReplayHooks(t testing.TB, baseURL, provider, dir string,
	routes map[string][]string, rewrite map[string]string) (responses []*http.Response, skipped []string)
```

`ClaudeHookRoutes` values (copy them from the `hooks` block that `claude.yaml`'s `hooks_injection` writes into `{tmp}/settings.json`): `SessionStart→session_start`, `UserPromptSubmit→user_prompt`, `Stop→turn_stop`, `StopFailure→turn_failed`, `MessageDisplay→message_delta`, `PreToolUse→tool_pre`, `PostToolUse→tool_post`, `PostToolUseFailure→tool_fail`, `SubagentStart→subagent_pre`, `SubagentStop→subagent_post`, `PermissionRequest→permission`, `Elicitation→elicitation`, `PreCompact→compact_pre`, `PostCompact→compact_post`, `SessionEnd→session_end`, `Notification→notification,idle` (claude.yaml registers two HTTP hooks for it).

**The fake CLI** (`fakecli/main.go`), exactly:
1. Read env `FAKECLI_DIR`, `FAKECLI_SEG`, `FAKECLI_TOKEN`. Write `<FAKECLI_DIR>/<FAKECLI_SEG>.json` = `Launch{RunnerID: FAKECLI_SEG, Token, Argv: os.Args[1:], PID: os.Getpid()}`, written to a temp name then renamed (so `AwaitLaunch` never reads half a file).
2. If argv contains a resume flag value `v` (the arg following `--resume`, or the arg following `resume` as the first positional) and `<FAKECLI_DIR>/refuse-<v>` exists: print that file's content to stdout and `os.Exit(1)`.
3. Otherwise copy stdin to stdout until EOF (exit 0) or SIGTERM/SIGINT (exit 143, as the real CLIs do).

**The installed descriptor** (`InstallFakeProvider`): with `Base: "claude"`, read the shipped YAML from `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml` (locate via `runtime.Caller`), decode to `map[string]any`, then:
- set `spawn.cmd` and `runtime.spawn.cmd` to `FakeCLIPath(t)`;
- drop `spawn.forbid_flags`, `spawn.env`, `model`, `effort` (the fake takes no `--model`), `permission_levels`, `telemetry`, `presentation.slash_catalog`, `mcp_injection`, `context_inject`, `resume_context_inject`, `terminal_prompts`, `terminal_notices`;
- replace `hooks_injection` with four `set_env` steps: `FAKECLI_DIR` = the capture dir (`t.TempDir()`), `FAKECLI_SEG` = `{segid}`, `FAKECLI_TOKEN` = `{runner_token}`, `FAKECLI_HOOK` = `{hook_curl} {hook_url}` (the last keeps `descriptorcheck`'s `hooks.not_wired` rule satisfied);
- apply `spec.Patch`, set `id`, write `<home>/descriptors/<id>.yaml`.
With `Base: ""`, start from `{id, spawn: {cmd, interactive_required: true}, runtime: {transport: hooks, hooks: {format: json, delivery: http}}, presentation: {prompt_submit: {strategy: restart_tui, fresh: [--, {message}], resume: [--, {message}]}}, session: {resume: {arg: "--resume {id}"}}, events: <EventsYAML>}` and the same `hooks_injection`.

If the descriptor gate refuses the override (the chat create returns 4xx), fix the document shape in `InstallFakeProvider`; never relax a `descriptorcheck` rule.

- [ ] **Step 1: Write the harness tests (they exercise the kit; they pass against today's daemon once the kit works)**

In `api/tests/fake_provider_test.go`:

```go
func TestFakeProvider_LaunchIsCapturedAndHooksReachTheChat(t *testing.T)
// install Base "claude"; create a chat with provider "claude" (POST repoBase/chats
// {"provider":"claude","workspaceId":…}); l := fake.AwaitLaunch(t, detail.LiveRunnerID)
// assert l.Token != "" and l.Argv does not contain "--resume"
// post session_start {"session_id":"s-1","source":"startup"}, user_prompt
// {"session_id":"s-1","prompt":"hello"}, turn_stop {"session_id":"s-1",
// "last_assistant_message":"HI"} — each response status 200
// GET …/chats/<id>/messages: roles ["user","assistant"], texts ["hello","HI"]

func TestFakeProvider_ReplaysTheRecordedClaudeSubagentSession(t *testing.T)
// install Base "claude"; create chat; l := AwaitLaunch; ReplayHooks over
// tests/testdata/hooks/claude-subagent-session with ClaudeHookRoutes and an empty
// rewrite (from T3 on, callers map the recorded session id
// 68a2773c-78c5-4edd-b4fe-bf5bd1309895 to the chat's bound session id)
// assert every response status < 500 and skipped is empty
// assert GET …/messages contains the user text of 01-UserPromptSubmit.json
// assert GET …/activity toolCalls is non-empty

func TestFakeProvider_KillEndsTheRunnerWithNoHook(t *testing.T)
// create chat, l := AwaitLaunch, dial …/chats/ws, l.Kill(t)
// readUntil a frame with kind "exited" and runnerId == l.RunnerID
// GET …/chats/<id>: liveRunnerId == ""
```

- [ ] **Step 2: Run them to see them fail (kit missing)**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestFakeProvider_' ./tests/`
Expected: compile failure — `kit.InstallFakeProvider` undefined.

- [ ] **Step 3: Implement `fakecli/main.go` and `fakeprovider.go` as specified above; move the fixture directory and repoint `turn_test.go:3001`**

- [ ] **Step 4: Run to green**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestFakeProvider_' ./tests/` → PASS.
Run: `cd api && go test -tags noEmbed -run 'TestTurns' ./internal/app/usecases/chat/` → the moved fixture still resolves.
Run: `cd api && make lint deadcode` → clean (the fakecli is a `main` under `tests/`, outside deadcode's `./cmd/...` roots).

- [ ] **Step 5: Commit**

```bash
git add api/tests/kit/fakecli api/tests/kit/fakeprovider.go api/tests/fake_provider_test.go \
  api/tests/testdata/hooks api/internal/app/usecases/chat/turn_test.go
git commit -m "test(api): drive chats with a fake provider CLI over the real hook route

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** three `TestFakeProvider_*` tests green against the unchanged daemon; no production file changed.

---

### Task 2: The ledger store, with prompts as its first rows

**Goal:** `domain/ledger` and `adapter/store/ledger` exist and are the owner of prompts. A prompt is a `user_message` row (with a Crowbar-minted turn id) written before the CLI is touched; the provider's acknowledgement is a `turn_started` row; a prompt that ends without a turn is a `prompt_settled` row. The prompt journal, its recovery code and the 30 s delivery-quiet sweep are deleted.

**Files:**
- Create: `api/internal/domain/ledger/row.go`, `prompt.go`, `state.go` and `row_test.go`, `prompt_test.go`, `state_test.go`
- Create: `api/internal/adapter/store/ledger/store.go`, `schema.go`, `read.go` and `store_test.go`, `read_test.go`
- Modify: `api/internal/adapter/container.go` — open `<home>/state/store/chat_ledger.db` with `sqlite.OpenDB`; accessor `ChatLedgerDB() *gorm.DB`; close it in `Close`
- Modify: `api/internal/app/repositories/container.go` — `ChatLedger *ledgerstore.Store`, built with `ledgerstore.Open(adapters.ChatLedgerDB(), time.Now)`
- Modify: `api/internal/app/usecases/container.go` and `api/internal/app/usecases/chat/assembly.go` — `Deps.Ledger *ledgerstore.Store`, passed to the runner and turn components as a constructor argument; `runner.Runners.SetPromptJournal` removed
- Modify: `api/internal/app/usecases/chat/internal/runner/{prompts.go,promptswitch.go,switchwait.go,errors.go,runner.go,steer.go,types.go}` — journal calls replaced (below)
- Modify: `api/internal/app/usecases/chat/internal/turn/turn_open.go` — acceptance writes `turn_started`
- Modify: `api/internal/app/usecases/chat/internal/turn/idle.go` — an idle report with a pending prompt and no running turn settles it (R3)
- Modify: `api/internal/app/usecases/chat/internal/runner/internal/termwait/{termwait.go,evaluate.go,sweep.go,termwait_test.go}` — delete the delivery half (`Deliveries`, `Delivery`, `DefaultDeliveryQuiet`, its evaluate branch and tests)
- Modify: `api/internal/app/usecases/chat/internal/conversation/conversation.go` (purge) — `Ledger.Forget(ctx, chatID)` joins the purge
- Modify: `api/internal/app/container.go` — replace the boot journal reconcile with the boot prompt pass (below)
- Delete: `api/internal/adapter/store/agentjournal/` (all files); `api/internal/app/usecases/chat/internal/runner/{promptrecovery.go,pendingprompt.go,settle.go}` and `{promptrecovery,pendingprompt,settle}_internal_test.go`; journal cases in `runner/prompts_internal_test.go`, `api/internal/app/usecases/chat/runner_test.go`, `harness_test.go`; `api/deadcode-baseline.txt` entry `internal/adapter/store/agentjournal/agentjournal.go: WithDirSync` (the baseline shrinks)
- Modify: `api/tests/kit/fakeprovider.go` — add `BreakSpawn`
- Test (black box): `api/tests/regression_pending_prompt_test.go` (extend), `api/tests/regression_steered_prompt_test.go` (must stay green)

**Interfaces:**
- Produces, `package ledger` (domain): the `Row` envelope, `Kind`, `NewRow`, `DecodeBody`, `Check` exactly as in the index's shared vocabulary; kinds `KindUserMessage`, `KindTurnStarted`, `KindPromptSettled`; bodies:

```go
type UserMessage struct {
	RequestID string `json:"requestId,omitempty"`
	Text      string `json:"text"`
	Origin    string `json:"origin"`             // OriginCrowbar | OriginProvider
	Delivery  string `json:"delivery,omitempty"` // DeliveryRestart | DeliverySteer | DeliveryAPI
}
type TurnStarted struct {
	RequestID      string `json:"requestId,omitempty"`
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	RunnerID       string `json:"runnerId,omitempty"`
}
type PromptSettled struct {
	RequestID string `json:"requestId"`
	Consumed  bool   `json:"consumed"`
	Reason    string `json:"reason"`
}

// PromptStatus is a prompt's place in the turn diagram (architecture §5).
type PromptStatus string // PromptPending | PromptDelivered | PromptSettled

type Prompt struct {
	RequestID string
	TurnID    string
	Text      string
	Delivery  string
	Status    PromptStatus
	Consumed  bool
	Reason    string
	At        time.Time
}

type State struct { /* ChatID, Head int64, prompts; later tasks add fields */ }
func Fold(chatID string, rows []Row) State
func (s *State) Apply(r Row)
func (s State) Head() int64
func (s State) PendingPrompts() []Prompt              // crowbar-origin, Status==PromptPending, oldest first
func (s State) Receipt(requestID string) (Prompt, bool) // every crowbar prompt ever, by request id
```

Settle reasons (constants in `prompt.go`): `ReasonProviderBuiltin = "provider_builtin"` (consumed), `ReasonNotAcknowledged = "not_acknowledged"`, `ReasonRunnerExited = "runner_exited"`, `ReasonDaemonRestart = "daemon_restart"`, `ReasonDeliveryFailed = "delivery_failed"`, `ReasonSessionLost = "session_lost"` (used from T3).

- Produces, `package ledger` under `adapter/store` (import as `ledgerstore`):

```go
func Open(db *gorm.DB, clock func() time.Time) (*Store, error)

// Do runs decide under chatID's writer: it receives the chat's current State and
// returns the rows to append. Seqs are assigned, Check runs on each row, rows
// and projections commit in one transaction, the cached State advances, and
// every subscriber is called — all before the writer is released. Returns the
// committed rows and the State after them. decide returning no rows commits
// nothing and calls no subscriber.
func (s *Store) Do(ctx context.Context, chatID string,
	decide func(State) ([]ledger.Row, error)) ([]ledger.Row, ledger.State, error)

// Append is Do with a fixed row list.
func (s *Store) Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error)

func (s *Store) State(ctx context.Context, chatID string) (ledger.State, error)
func (s *Store) Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error) // limit<=0 = all
func (s *Store) OpenChats(ctx context.Context) ([]string, error) // chats whose heads row has open=1
func (s *Store) Forget(ctx context.Context, chatID string) error

// Subscriber is called after commit, under the chat's writer, in seq order.
// It must not block: hand work to a channel or do it in O(1).
type Subscriber func(chatID string, rows []ledger.Row, st ledger.State)
func (s *Store) Subscribe(fn Subscriber)
```

`State` is re-exported from the store package only as `ledger.State` from the domain package; do not define a second type.

**Schema** (`schema.go`, created with `CREATE TABLE IF NOT EXISTS`, run in `Open`):

```sql
CREATE TABLE chat_ledger (
  chat_id TEXT NOT NULL, seq INTEGER NOT NULL, kind TEXT NOT NULL,
  turn_id TEXT NOT NULL DEFAULT '', scope TEXT NOT NULL DEFAULT '',
  at INTEGER NOT NULL,   -- unix microseconds from the store clock
  body BLOB NOT NULL,
  PRIMARY KEY (chat_id, seq)) WITHOUT ROWID;
CREATE TABLE chat_ledger_heads (
  chat_id TEXT PRIMARY KEY, last_seq INTEGER NOT NULL, open INTEGER NOT NULL);
```

`open` is `1` when `State.HasOpenWork()` is true after the append (T2: a pending prompt; later tasks add a live runner, a running turn, a live subagent). `HasOpenWork` is a `State` method each task extends.

**Writer and cache:** a `map[string]*chatWriter` behind a `sync.Mutex`; each `chatWriter` holds its own `sync.Mutex`, the cached `State` (loaded on first touch by `Fold(Read(chat, 0, 0))`), and nothing else. `Forget` takes the chat's writer, deletes its rows and heads row in one transaction and drops the cache entry.

**Runner changes:**
- `SubmitPrompt` (`prompts.go`): inside `Ledger.Do` — if `st.Receipt(requestID)` exists: same text → return the existing submission (idempotent resend), different text → `apperr` conflict (`ErrPromptRequestIDConflict`, keep its 409 mapping), status settled → the same conflict error; if `len(st.PendingPrompts()) > 0` → `ErrPromptBusy`; else append `user_message{requestId, text, origin: crowbar, delivery}` with `TurnID = uuid.NewString()`. Only after the row commits is the outgoing CLI touched (restart, api push, or steer park). A failure before the replacement process exists appends `prompt_settled{consumed:false, reason: delivery_failed}` and returns the error.
- The steered path keeps today's mechanism (the prompt rides the turn-end hook reply); its pending prompt is the `user_message{delivery: steer}` row instead of the journal record.
- `promptswitch.go` / `switchwait.go`: delete `awaitPromptDeliverySettled`, `promptDeliveryAwait` and its 30 s bound; a switch or selection change with a pending prompt returns `ErrPromptBusy` at once.
- `PendingPrompt(ctx, chatID)` (now in `prompts.go`): oldest of `st.PendingPrompts()` → `domain.PendingPrompt{Text, State: "pending", RequestID}`.

**Acceptance on the hook path** (`turn_open.go`): where today the journal's `ConfirmAccepted` runs on `user_prompt`, call `Ledger.Do` with this decision:
- normalise the hook text exactly as `turn_open.go` does today (injected prefix, sigil escape, attachment materialisation reversed);
- if the oldest pending crowbar prompt's text equals it → append `turn_started{requestId, providerTurnId: ev.TurnID, runnerId}` with that prompt's `TurnID`;
- else → append `user_message{origin: provider, text}` + `turn_started{runnerId, providerTurnId}` under a new `TurnID` (a prompt typed at the terminal; the pending crowbar prompt stays pending).
The chat aggregate's `StartTurn` and the activity record continue exactly as today (T5 and T7 replace them); the activity user-turn id for a crowbar prompt stays its `requestId`, as today.

**R3 settles** (all via `Ledger.Do`):
- `compact_pre`/`compact_post` while a crowbar prompt is pending and no turn has started for it → `prompt_settled{consumed:true, reason: provider_builtin}` (replaces the journal's built-in settle).
- idle hook (`turn/idle.go`, where the latch is armed today) while a crowbar prompt is pending and the chat aggregate reports no open turn → `prompt_settled{consumed:false, reason: not_acknowledged}`.
- runner exit reconcile (`runner/lifecycle.go reconcileRunnerExit`) with a pending prompt → `prompt_settled{consumed:false, reason: runner_exited}`.
- boot (`api/internal/app/container.go`, in place of `reconcilePromptJournalsOnBoot`): for each chat in `Ledger.OpenChats` with pending prompts → `prompt_settled{consumed:false, reason: daemon_restart}`.

**Publication:** in `assembly.go`, `d.Ledger.Subscribe(u.publishLedgerRows)`; for every `prompt_settled` row call `feed.PromptSettled(chatID, workspaceID, requestID, consumed)` (the existing `seam.ChatFeed` hook that produces today's `prompt_settled` frame). Workspace id comes from the chat snapshot cache, not a store read.

- [ ] **Step 1: Domain tests (table tests, `package ledger_test`)**

```go
func TestCheck_RefusesMalformedRows(t *testing.T)
// unknown kind → error; KindUserMessage with TurnID "" → error;
// body "{" → error; a well-formed user_message → nil

func TestFold_PromptLifecycle(t *testing.T)
// rows: user_message{r1,"a"} T1 → turn_started{r1} T1 → user_message{r2,"b"} T2 →
//       prompt_settled{r2,false,"runner_exited"}
// Receipt("r1").Status == PromptDelivered; Receipt("r2").Status == PromptSettled,
// Consumed false, Reason "runner_exited"; PendingPrompts() empty; Head() == 4

func TestFold_ProviderOriginPromptLeavesTheCrowbarPromptPending(t *testing.T)
// user_message{r1,"a",crowbar} T1, user_message{"typed",provider} T2, turn_started{} T2
// PendingPrompts() == [r1]

func TestFold_LateRowsChangeNothing(t *testing.T)
// turn_started naming an unknown requestId and prompt_settled for an already
// delivered request leave Receipt unchanged; Apply never panics
```

- [ ] **Step 2: Store tests (`package ledger_test` in `adapter/store/ledger`, temp-dir SQLite via `sqlite.OpenDB`)**

```go
func TestStore_AppendAssignsContiguousSeqsFromOne(t *testing.T)
// Append 3 rows to "c1" → seqs 1,2,3; Append 1 to "c2" → seq 1; At == clock value

func TestStore_ConcurrentAppendsGetContiguousSeqsPublishedInOrder(t *testing.T)
// 50 goroutines each Append one row to "c1"; subscriber records seqs
// → recorded == 1..50 in order; Read(c1,0,0) has 50 rows

func TestStore_DoSeesTheStateItDecidesAgainst(t *testing.T)
// two concurrent Do calls that each append a user_message only when
// PendingPrompts() is empty → exactly one commits; the other returns no rows

func TestStore_SubscriberRunsAfterCommit(t *testing.T)
// inside the subscriber, a fresh Read(chat) already returns the published rows

func TestStore_RejectsMalformedRowsAtomically(t *testing.T)
// Append(valid, malformed) → error; Read returns nothing; Head unchanged

func TestStore_StateSurvivesReopenWithManyRows(t *testing.T)
// append 5000 rows (pending/settled prompts), close, Open again → State equals
// the State before close (compare Head, PendingPrompts, Receipt of 3 sampled ids)

func TestStore_OpenChatsListsOnlyChatsWithOpenWork(t *testing.T)
// c1 pending prompt, c2 settled, c3 delivered → OpenChats == [c1]

func TestStore_ForgetErasesAChat(t *testing.T)
// Forget(c1) → Read empty, State Head 0, OpenChats excludes it; c2 untouched
```

- [ ] **Step 3: Black-box tests (extend existing files)**

In `api/tests/regression_pending_prompt_test.go` (uses `kit.InstallFakeProvider` with `Base: "claude"`):

```go
func TestRegression_APromptWhoseLaunchFailsIsSettledNotLost(t *testing.T)
// create the chat, then fake.BreakSpawn(t) (rewrites the installed descriptor's
// spawn.cmd to a path that does not exist; descriptors reload on mtime)
// POST …/prompts → error status; the chat feed (…/chats/ws) carries
// prompt_settled{clientRequestId, promptConsumed:false}; GET …/pending-prompt → empty

func TestRegression_ResendingTheSameRequestIDNeverDeliversTwice(t *testing.T)
// POST …/prompts twice with the same clientRequestId and text
// → both 2xx; the fake recorded exactly one launch carrying that text

func TestRegression_APromptTheProviderNeverAcknowledgesSettlesOnIdle(t *testing.T)
// POST prompt (launch recorded), then post event "idle" with
// {"session_id":…,"message":"Claude is waiting for your input"} (claude.yaml's idle
// when:) and no user_prompt → the feed carries prompt_settled{promptConsumed:false} and an
// immediate second POST …/prompts with a new id is accepted (not 409)
```

Add to `api/tests/kit/fakeprovider.go`: `func (f FakeProvider) BreakSpawn(t testing.TB)` — rewrites `<home>/descriptors/<id>.yaml` with `spawn.cmd` and `runtime.spawn.cmd` set to `filepath.Join(f.Dir, "missing-cli")` and bumps the file's mtime.

`TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue` and every test in `regression_steered_prompt_test.go` must stay green unchanged.

- [ ] **Step 4: Run the new tests and see them fail**

Run: `cd api && go test -tags noEmbed ./internal/domain/ledger/... ./internal/adapter/store/ledger/...` → compile failure (packages missing).
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(APromptWhoseLaunchFails|ResendingTheSameRequestID|APromptTheProviderNever|PendingPromptSurvives)' ./tests/` → `APromptTheProviderNeverAcknowledgesSettlesOnIdle` FAILS (today the second prompt is refused until the 30 s sweep); the other two are characterization and may already pass.

- [ ] **Step 5: Implement the domain package, the store, the wiring, the runner/turn changes, the boot pass; delete `agentjournal`, `promptrecovery.go`, `pendingprompt.go`, `settle.go` and the termwait delivery half, with their tests**

- [ ] **Step 6: Run to green**

Run: `cd api && go test -tags noEmbed ./internal/domain/ledger/... ./internal/adapter/store/ledger/... ./internal/app/usecases/chat/... ./internal/adapter/...` → PASS.
Run: `cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_(APromptWhoseLaunchFails|ResendingTheSameRequestID|APromptTheProviderNever|PendingPromptSurvives|PromptSentMidTurn|SteeredPrompt|CommandSentMidTurn|PromptSentWhileIdle|StopRefusesAParkedPrompt|ComposerBodyWithSelection|SelectionChangedMidTurn)|TestFakeProvider_' ./tests/` → PASS.
Run: `cd api && make lint deadcode && go vet -tags noEmbed ./...` → clean; `grep -rn agentjournal api/` → nothing.

- [ ] **Step 7: Commit**

```bash
git add -A api/internal/domain/ledger api/internal/adapter api/internal/app api/tests api/deadcode-baseline.txt
git commit -m "feat(api): per-chat ledger store; prompts are ledger rows

Delete the prompt journal, its recovery and the 30 s delivery-quiet sweep.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** a prompt's existence, acknowledgement and settlement are ledger rows; no file under `chats/<id>/prompt-requests` is written; the `pending-prompt` endpoint and the `prompt_settled` frame are served from the ledger; `api/internal/adapter/store/agentjournal` no longer exists; deadcode baseline shrank by one.
