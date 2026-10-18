# S1a — Behaviour harness and the ledger store: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** First, a black-box harness that drives the daemon with a fake provider CLI and real recorded hook payloads (T1). Then the ledger store with its first owner: prompts become `user_message` rows, and the fsync'd prompt journal is deleted (T2).

**Architecture:** The index is `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. Each task below states every rule it needs.

**Tech Stack:** Go, SQLite (gorm + raw SQL), testify, `api/tests` (`//go:build integration`).

**Spec:** architecture §3 (journal invariants), §4 (a prompt end to end), §5 (turn diagram); report §4.3; spike §A8.

---

### Task 1: Behaviour harness — fake provider CLI and recorded-hook replay

**Goal:** Tests can:
- create a chat whose provider CLI is a small fake binary that records its argv and runner token;
- post hooks to the real HTTP hook route with that token;
- replay a recorded Claude session through the shipped Claude descriptor's own mapping.

Characterization tests pin today's behaviour, so later tasks can prove they keep it.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run these:
- `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or the full vitest suite;
- any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...` — their TestMains read the real `~/.claude.json` and `~/.codex`. Compile them with `gates` only.

Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`. Never bare `git stash`, never push.

**Files:**
- Chore commit first (carry C4): `gofmt -w api/tests/integration/scripted/livemodel_test.go` (still unformatted at S1 start), committed alone as `chore(api): gofmt livemodel_test.go`.
- Create: `api/tests/kit/fakecli/main.go` (`//go:build integration`, `package main`)
- Create: `api/tests/kit/fakeprovider.go` (`//go:build integration`, `package kit`)
- Create: `api/tests/fake_provider_test.go` (`//go:build integration`, `package tests`)
- Move (`git mv`): `api/internal/app/usecases/chat/testdata/claude-subagent-session/` → `api/tests/testdata/hooks/claude-subagent-session/` (44 scrubbed real Claude payloads, `00-SessionStart.json` … `43-SessionEnd.json`)
- Modify: `api/internal/app/usecases/chat/turn_test.go:3001` — glob the moved directory (`filepath.Join("..", "..", "..", "..", "tests", "testdata", "hooks", "claude-subagent-session", "*.json")`). The file has 3,061 lines; this one-line change adds none, so no split is needed (pre-flight R8: only a task that adds lines to an oversize file splits it).

**Interfaces:**
- Consumes: the existing `tests` harness in `api/tests/harness_test.go` (`newHarness`, `newHarnessAt(t, home)`, `h.url`, `h.home`, `h.shutdown()`, `h.crash()`, `h.Quiesce()`) and helpers `importWritableWorkspace`, `repoBase`, `getAgentChat`, `readUntil`, `h.dial`. The harness boots through the migration engine (`kit.BootHome`, then `app.New(ctx, engines, adapters, session)`), and `h.crash()` releases the home lock, so `newHarnessAt` on the same home works.
- Produces (all in `package kit`):

```go
// FakeCLIPath builds tests/kit/fakecli once per test binary (sync.Once, `go build
// -tags integration`) into a temp dir and returns the binary path.
func FakeCLIPath(t testing.TB) string

// FakeProvider is a provider descriptor installed under <home>/descriptors whose
// process is the fake CLI.
type FakeProvider struct {
	ID  string // descriptor id the chat is created with
	Dir string // capture directory the fake CLI writes into
}

// FakeSpec selects what the installed descriptor is built from.
type FakeSpec struct {
	Base       string                   // shipped descriptor to keep ("claude"); "" = EventsYAML alone
	ID         string                   // installed id; defaults to Base
	EventsYAML string                   // replaces the events: block when non-empty (stub providers)
	Patch      func(doc map[string]any) // test-specific edit of the decoded document
}

// InstallFakeProvider writes the descriptor and points the daemon's provider
// homes at temp dirs (below). Tests that call it must not run in parallel.
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

// RefuseResume makes every later launch that resumes sessionID print line and
// exit 1 (fakecli reads <Dir>/refuse-<sessionID>).
func (f FakeProvider) RefuseResume(t testing.TB, sessionID, line string)

// Kill sends SIGTERM to the launch's process (an exit with no hook).
func (l Launch) Kill(t testing.TB)

// PostHook posts payload to <baseURL>/chats/runners/<runner>/hooks/<provider>/<event>
// with header X-Crowbar-Runner-Token: l.Token. baseURL is the server URL plus the
// repo or home route prefix.
func (l Launch) PostHook(t testing.TB, baseURL, provider, event string, payload []byte) *http.Response

// ClaudeHookRoutes maps a recorded payload's hook_event_name to the canonical
// events the shipped claude descriptor posts it as (Notification goes to two).
// T9 turns this into a function of (hookEventName, toolName).
var ClaudeHookRoutes map[string][]string

// ReplayHooks posts every *.json file under dir (lexical order) through l, once per
// route in routes[payload.hook_event_name], replacing every key of rewrite with its
// value in the raw bytes first. A payload whose hook name has no route is skipped
// and returned in skipped.
func (l Launch) ReplayHooks(t testing.TB, baseURL, provider, dir string,
	routes map[string][]string, rewrite map[string]string) (responses []*http.Response, skipped []string)
```

`ClaudeHookRoutes` values are copied from the `hooks` block that `claude.yaml`'s `hooks_injection` writes into `{tmp}/settings.json`:
- `SessionStart→session_start`, `UserPromptSubmit→user_prompt`, `Stop→turn_stop`, `StopFailure→turn_failed`;
- `MessageDisplay→message_delta`, `PreToolUse→tool_pre`, `PostToolUse→tool_post`, `PostToolUseFailure→tool_fail`;
- `SubagentStart→subagent_pre`, `SubagentStop→subagent_post`, `PermissionRequest→permission`, `Elicitation→elicitation`;
- `PreCompact→compact_pre`, `PostCompact→compact_post`, `SessionEnd→session_end`;
- `Notification→notification,idle` (claude.yaml registers two HTTP hooks for it).

**The fake CLI** (`fakecli/main.go`), exactly:
1. Read env `FAKECLI_DIR`, `FAKECLI_SEG`, `FAKECLI_TOKEN`. Write `<FAKECLI_DIR>/<FAKECLI_SEG>.json` = `Launch{RunnerID: FAKECLI_SEG, Token, Argv: os.Args[1:], PID: os.Getpid()}`. Write it to a temp name, then rename, so `AwaitLaunch` never reads half a file.
2. Find the resume value `v`: the arg after `--resume`, or the arg after `resume` when that is the first positional. If `<FAKECLI_DIR>/refuse-<v>` exists, print its content to stdout and `os.Exit(1)`.
3. Otherwise copy stdin to stdout until EOF (exit 0) or SIGTERM/SIGINT (exit 143).

**The installed descriptor** (`InstallFakeProvider`):
- **Provider homes first (pre-flight R5):** call `t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())` and `t.Setenv("CODEX_HOME", t.TempDir())` before anything can launch.
  - Why: until T3 deletes `session.locate`, any resume makes the in-process daemon glob `<root_env>/projects/*/<id>.jsonl`. With `CLAUDE_CONFIG_DIR` unset, that root is the real `~/.claude`.
  - With the env set, today's "not found" behaviour is kept without reading a provider home.
- With `Base: "claude"`, read the shipped YAML at `api/internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3/claude.yaml` (locate it via `runtime.Caller`) and decode it to `map[string]any`. Then:
  - set `spawn.cmd` and `runtime.spawn.cmd` to `FakeCLIPath(t)`;
  - drop these blocks (the fake takes no `--model`): `spawn.forbid_flags`, `spawn.env`, `model`, `effort`, `permission_levels`, `telemetry`, `presentation.slash_catalog`, `mcp_injection`, `context_inject`, `resume_context_inject`, `terminal_prompts`, `terminal_notices`;
  - replace `hooks_injection` with four `set_env` steps:
    - `FAKECLI_DIR` = the capture dir (`t.TempDir()`);
    - `FAKECLI_SEG` = `{segid}`;
    - `FAKECLI_TOKEN` = `{runner_token}`;
    - `FAKECLI_HOOK` = `{hook_curl} {hook_url}` (this keeps `descriptorcheck`'s `hooks.not_wired` rule satisfied);
  - apply `spec.Patch`, set `id`, and write `<home>/descriptors/<id>.yaml`.
- With `Base: ""`, start from `{id, spawn: {cmd, interactive_required: true}, runtime: {transport: hooks, hooks: {format: json, delivery: http}}, presentation: {prompt_submit: {strategy: restart_tui, fresh: [--, {message}], resume: [--, {message}]}}, session: {resume: {arg: "--resume {id}"}}, events: <EventsYAML>}` and the same `hooks_injection`.
- If the descriptor gate refuses the override (the chat create returns 4xx), fix the document shape in `InstallFakeProvider`. Never relax a `descriptorcheck` rule.

`kit/fakeprovider.go` and `kit/fakecli/main.go` are non-test files in the `test-support` ratchet module. They may name Claude hook events: they are test support, not `api/internal`.

- [ ] **Step 0: Chore commit (C4)**

Run: `gofmt -l api/tests/integration/scripted/livemodel_test.go` → the file is listed. Then run `gofmt -w` on it and `gofmt -l` again; nothing is listed.

```bash
git add api/tests/integration/scripted/livemodel_test.go
git commit -m "chore(api): gofmt livemodel_test.go

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 1: Write the harness tests.** They exercise the kit, and they pass against today's daemon once the kit works.

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

- [ ] **Step 2: Run them; they fail (kit missing)**

Run: `bbx 'TestFakeProvider_'` → compile failure: `kit.InstallFakeProvider` undefined.

- [ ] **Step 3: Implement** `fakecli/main.go` and `fakeprovider.go` as specified. Move the fixture directory and repoint `turn_test.go:3001`.

- [ ] **Step 4: Run to green**

Run: `bbx 'TestFakeProvider_'` → PASS.
Run: `gox go test -tags noEmbed -run 'TestTurns' ./internal/app/usecases/chat/` → PASS; the moved fixture still resolves.
Run: `gates` → clean. The fakecli is a `main` under `tests/`, outside deadcode's `./cmd/...` roots.
Run: `grep -rn 'CLAUDE_CONFIG_DIR\|CODEX_HOME' api/tests/kit/fakeprovider.go` → both are set.

- [ ] **Step 5: Commit**

```bash
git add api/tests/kit/fakecli api/tests/kit/fakeprovider.go api/tests/fake_provider_test.go \
  api/tests/testdata/hooks api/internal/app/usecases/chat/turn_test.go
git commit -m "test(api): drive chats with a fake provider CLI over the real hook route

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- The three `TestFakeProvider_*` tests pass against the unchanged daemon.
- No production file changed.
- The fake points both provider homes at temp dirs.
- `livemodel_test.go` is gofmt-clean, in its own chore commit.

---

### Task 2: The ledger store, with prompts as its first rows

**Goal:** `domain/ledger` and `adapter/store/ledger` exist and own prompts:
- A prompt is a `user_message` row with a Crowbar-minted turn id, written before the CLI is touched.
- The provider's acknowledgement is a `turn_started` row.
- A prompt that ends without a turn is a `prompt_settled` row.
- Deleted: the prompt journal, its recovery code and the 30 s delivery-quiet sweep.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run these:
- `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or the full vitest suite;
- any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`.

Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- The chat ledger is its own append-only table with a per-chat single writer, not an asynx aggregate.
- Never acknowledge a prompt before its row commits.
- No timer settles a prompt. A prompt the provider never acknowledges blocks the next send with `ErrPromptBusy` (409) until a settling fact arrives. There is no automatic redelivery. This behaviour change was accepted provisionally on 2026-10-18; the user may override it before this task lands.
- Paths come from `core/home` (`h.Store()`), never a hand-built home path. `api/internal/core/home/architecture_test.go` enforces this.

**Files:**
- Create: `api/internal/domain/ledger/row.go`, `prompt.go`, `state.go` and `row_test.go`, `prompt_test.go`, `state_test.go`
- Create: `api/internal/adapter/store/ledger/store.go`, `schema.go`, `read.go` and `store_test.go`, `read_test.go`
- Create: `api/internal/adapter/chat_ledger.go`:
  - opens `filepath.Join(h.Store(), ledgerstore.FileName)` with `storesqlite.OpenDB` (one writer connection; WAL is set by `OpenDB`) through the existing `opens` helper;
  - accessor `func (c *Container) ChatLedgerDB() *gorm.DB`.
- Modify: `api/internal/adapter/container.go` (498 lines) — add only the field and its close. Offset these lines by trimming narrative comments to ≤3-line whys so the file stays ≤ 500. Add `api/internal/adapter/chat_ledger.go` to `scripts/ratchet/modules.json` `wiring` (it is split wiring).
- Modify: `api/internal/app/repositories/container.go` (851) — field `ChatLedger *ledgerstore.Store`, built with `ledgerstore.Open(adapters.ChatLedgerDB(), time.Now)`.
- Modify: `api/internal/app/usecases/container.go` (1,080) and `api/internal/app/usecases/chat/assembly.go`:
  - `Deps.Ledger *ledgerstore.Store`, passed to the runner and turn components as a constructor argument;
  - remove `runner.Runners.SetPromptJournal` (`runner.go:260`) and its wiring.
  - Both containers are oversize: the journal wiring removed must outweigh what is added, so the edit is net-shrinking. If it is not, split the file below 500 in this task and add the new paths to `modules.json` `wiring`.
- Modify: `api/internal/app/usecases/chat/internal/runner/{prompts.go,promptswitch.go,switchwait.go,errors.go,runner.go,steer.go,types.go}` — journal calls replaced (below). `prompts.go` (432) stays ≤ 500.
- Modify: `api/internal/app/usecases/chat/internal/runner/lifecycle.go` — in `ReconcileRunnersOnBoot` (`lifecycle.go:113`), the call at `:163` to `reconcilePromptJournalsOnBoot` (`prompts.go:409`) becomes the boot prompt pass (below). Pre-flight R19: the pass lives here, not in `app/container.go`, which this task does not edit.
- Modify: `api/internal/app/usecases/chat/internal/turn/turn_open.go` — acceptance writes `turn_started`
- Modify: `api/internal/app/usecases/chat/internal/turn/idle.go` — an idle report with a pending prompt and no running turn settles it (T5 deletes this file and keeps the settle)
- Modify: `api/internal/app/usecases/chat/internal/runner/internal/termwait/{termwait.go,evaluate.go,sweep.go,termwait_test.go}` — delete the delivery half (`Deliveries`, `Delivery`, `DefaultDeliveryQuiet`, its evaluate branch and its tests). `termwait_test.go` (1,464) only shrinks.
- Modify: `api/internal/app/usecases/chat/internal/conversation/conversation.go` — `Ledger.Forget(ctx, chatID)` joins the chat purge.
- Delete:
  - `api/internal/adapter/store/agentjournal/` (all files);
  - `api/internal/app/usecases/chat/internal/runner/{promptrecovery.go,pendingprompt.go,settle.go}` and `{promptrecovery,pendingprompt,settle}_internal_test.go`;
  - the journal cases in `runner/prompts_internal_test.go`, `api/internal/app/usecases/chat/runner_test.go` (5,482; deletions only) and `harness_test.go` (1,616; deletions only);
  - the `api/deadcode-baseline.txt` line `internal/adapter/store/agentjournal/agentjournal.go: WithDirSync` (the baseline shrinks).
- Modify: `api/tests/kit/fakeprovider.go` — add `BreakSpawn`
- Test (black box): extend `api/tests/regression_pending_prompt_test.go`. `api/tests/regression_steered_prompt_test.go` must stay green.

**Interfaces:**
- Produces, `package ledger` (domain): the `Row` envelope, `Kind`, `NewRow`, `DecodeBody`, `Check`, `Fold`, `Apply`:

```go
type Kind string
type Row struct {
	ChatID string          `json:"chatId"`
	Seq    int64           `json:"seq"`    // assigned by the store, 1-based, contiguous per chat
	Kind   Kind            `json:"kind"`
	TurnID string          `json:"turnId,omitempty"` // Crowbar-minted (uuid v4); "" for chat-level rows
	Scope  string          `json:"scope,omitempty"`  // "" = main agent, else a subagent scope id
	At     time.Time       `json:"at"`              // stamped by the store's clock
	Body   json.RawMessage `json:"body"`
}
func NewRow[B any](kind Kind, turnID, scope string, body B) (Row, error)
func DecodeBody[B any](r Row) (B, error)
// Check refuses only malformed rows: unknown kind; user_message or turn_started
// without TurnID (later tasks add their turn-scoped kinds); an undecodable body.
func Check(s State, r Row) error

const (KindUserMessage Kind = "user_message"; KindTurnStarted Kind = "turn_started"; KindPromptSettled Kind = "prompt_settled")
const (OriginCrowbar = "crowbar"; OriginProvider = "provider"; OriginHarness = "harness")
const (DeliveryRestart = "restart"; DeliverySteer = "steer"; DeliveryAPI = "api")

type UserMessage struct {
	RequestID string `json:"requestId,omitempty"`
	Text      string `json:"text"`
	Origin    string `json:"origin"`             // OriginCrowbar | OriginProvider | OriginHarness
	Delivery  string `json:"delivery,omitempty"` // crowbar origin only
}
type TurnStarted struct {
	RequestID      string `json:"requestId,omitempty"` // crowbar origin only
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	RunnerID       string `json:"runnerId,omitempty"`
}
type PromptSettled struct {
	RequestID string `json:"requestId"`
	Consumed  bool   `json:"consumed"`
	Reason    string `json:"reason"`
}

type PromptStatus string // PromptPending | PromptDelivered | PromptSettled (architecture §5)
type Prompt struct {
	RequestID, TurnID, Text, Delivery string
	Status                            PromptStatus
	Consumed                          bool
	Reason                            string
	At                                time.Time
}

type State struct { /* ChatID, head, prompts; later tasks add fields */ }
func Fold(chatID string, rows []Row) State
func (s *State) Apply(r Row)                            // never fails; a disallowed transition is a late row
func (s State) Head() int64
func (s State) HasOpenWork() bool                       // T2: a pending prompt; later tasks extend it
func (s State) PendingPrompts() []Prompt                // crowbar origin, Status==PromptPending, oldest first
func (s State) Receipt(requestID string) (Prompt, bool) // every crowbar prompt ever, by request id
```

Settle reasons (constants in `prompt.go`):
- `ReasonProviderBuiltin = "provider_builtin"` (consumed);
- `ReasonNotAcknowledged = "not_acknowledged"`;
- `ReasonRunnerExited = "runner_exited"`;
- `ReasonDaemonRestart = "daemon_restart"`;
- `ReasonDeliveryFailed = "delivery_failed"`;
- `ReasonSessionLost = "session_lost"` (used from T3).

- Produces, `package ledger` under `adapter/store` (import it as `ledgerstore`):

```go
const FileName = "chat_ledger.db" // under h.Store(); the adapter and migration step 6 both open it

func Open(db *gorm.DB, clock func() time.Time) (*Store, error)

// Do runs decide under chatID's writer. decide receives the chat's current State
// and returns the rows to append. Under the writer: seqs are assigned, Check runs
// on each row, rows and projections commit in one transaction, the cached State
// advances, and every subscriber is called. Do returns the committed rows and the
// State after them. A decide that returns no rows commits nothing and calls no
// subscriber.
func (s *Store) Do(ctx context.Context, chatID string,
	decide func(ledger.State) ([]ledger.Row, error)) ([]ledger.Row, ledger.State, error)
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

`State` exists once, in the domain package. The store returns `ledger.State`; it never defines a second type.

**Schema** (`schema.go`, `CREATE TABLE IF NOT EXISTS`, run in `Open`):

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

`open` is `1` when `State.HasOpenWork()` is true after the append.

**Writer and cache:**
- A `map[string]*chatWriter` behind a `sync.Mutex`.
- Each `chatWriter` holds its own `sync.Mutex` and the cached `State`, loaded on first touch by `Fold(Read(chat, 0, 0))`. It holds nothing else.
- `Forget` takes the chat's writer, deletes its rows and its heads row in one transaction, and drops the cache entry.

**Runner changes:**
- `SubmitPrompt` (`prompts.go`) decides inside `Ledger.Do`:
  - A receipt exists for `requestID` (`st.Receipt(requestID)`):
    - same text → return the existing submission (an idempotent resend);
    - different text → `ErrPromptRequestIDConflict` (keep its 409 mapping);
    - status settled → the same conflict error.
  - `len(st.PendingPrompts()) > 0` → `ErrPromptBusy`.
  - Otherwise → append `user_message{requestId, text, origin: crowbar, delivery}` with `TurnID = uuid.NewString()`.
  - The outgoing CLI is touched only after the row commits: restart, api push, or steer park.
  - A failure before the replacement process exists appends `prompt_settled{consumed:false, reason: delivery_failed}` and returns the error.
- The steered path keeps today's mechanism: the prompt rides the turn-end hook reply. Its pending prompt is the `user_message{delivery: steer}` row instead of the journal record.
- `promptswitch.go` / `switchwait.go`: delete `awaitPromptDeliverySettled`, `promptDeliveryAwait` and its 30 s bound. A switch or selection change with a pending prompt returns `ErrPromptBusy` at once.
- `PendingPrompt(ctx, chatID)` (now in `prompts.go`): the oldest of `st.PendingPrompts()` → `domain.PendingPrompt{Text, State: "pending", RequestID}`.

**Acceptance on the hook path** (`turn_open.go`). Where the journal's `ConfirmAccepted` runs on `user_prompt` today, call `Ledger.Do` with this decision:
- Normalise the hook text exactly as `turn_open.go` does today: injected prefix, sigil escape, attachment materialisation reversed.
- The oldest pending Crowbar prompt's text equals it → append `turn_started{requestId, providerTurnId: ev.TurnID, runnerId}` under that prompt's `TurnID`.
- `engineagents.MatchInjectedPrompt(agent, text)` matches (pre-flight R12) → append `user_message{origin: harness, text}` + `turn_started{runnerId, providerTurnId}` under a new `TurnID`. This is the provider's own harness speaking; it is recorded today with role `harness` (`turn_open.go:79`). The pending Crowbar prompt stays pending, and no title is derived.
- Otherwise → append `user_message{origin: provider, text}` + `turn_started{runnerId, providerTurnId}` under a new `TurnID`: a prompt typed at the terminal. The pending Crowbar prompt stays pending.

The chat aggregate's `StartTurn` and the activity record continue exactly as today; T5 and T7 replace them. The activity user-turn id for a Crowbar prompt stays its `requestId`. The harness unit tests in `api/internal/app/usecases/chat/turn_test.go` (`TestRegression_HarnessInjectedPromptIsRecordedAsHarnessNotUser` and its siblings, lines 1851–2040) stay green.

**Settles** (design R3), all via `Ledger.Do`:
- `compact_pre` or `compact_post` while a Crowbar prompt is pending and no turn has started for it → `prompt_settled{consumed:true, reason: provider_builtin}`.
- The idle hook (in `turn/idle.go`, where the latch is armed today) while a Crowbar prompt is pending and the chat aggregate reports no open turn → `prompt_settled{consumed:false, reason: not_acknowledged}`.
- Runner exit (`runner/lifecycle.go` `reconcileRunnerExit`, `:169`) with a pending prompt → `prompt_settled{consumed:false, reason: runner_exited}`.
- Boot: `reconcilePendingPromptsOnBoot` in `runner/prompts.go` replaces `reconcilePromptJournalsOnBoot`, at the same call site in `ReconcileRunnersOnBoot`. For each chat in `Ledger.OpenChats` with pending prompts → `prompt_settled{consumed:false, reason: daemon_restart}`.

**Publication:** in `assembly.go`, `d.Ledger.Subscribe(u.publishLedgerRows)`. For every `prompt_settled` row, call `feed.PromptSettled(chatID, workspaceID, requestID, consumed)`. This is the existing `seam.ChatFeed` hook behind today's `prompt_settled` frame. The workspace id comes from the chat snapshot cache, not from a store read.

- [ ] **Step 1: Domain tests** (table tests, `package ledger_test`)

```go
func TestCheck_RefusesMalformedRows(t *testing.T)
// unknown kind → error; KindUserMessage with TurnID "" → error;
// body "{" → error; a well-formed user_message → nil

func TestFold_PromptLifecycle(t *testing.T)
// rows: user_message{r1,"a"} T1 → turn_started{r1} T1 → user_message{r2,"b"} T2 →
//       prompt_settled{r2,false,"runner_exited"}
// Receipt("r1").Status == PromptDelivered; Receipt("r2").Status == PromptSettled,
// Consumed false, Reason "runner_exited"; PendingPrompts() empty; Head() == 4

func TestFold_ProviderAndHarnessOriginLeaveTheCrowbarPromptPending(t *testing.T)
// user_message{r1,"a",crowbar} T1, user_message{"typed",provider} T2, turn_started{} T2,
// user_message{"<agent-message…>",harness} T3, turn_started{} T3 → PendingPrompts() == [r1]

func TestFold_LateRowsChangeNothing(t *testing.T)
// turn_started naming an unknown requestId and prompt_settled for an already
// delivered request leave Receipt unchanged; Apply never panics
```

- [ ] **Step 2: Store tests** (`package ledger_test` in `adapter/store/ledger`, temp-dir SQLite via `storesqlite.OpenDB`)

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

- [ ] **Step 3: Black-box tests** (extend existing files)

Add `func (f FakeProvider) BreakSpawn(t testing.TB)` to `api/tests/kit/fakeprovider.go`. It rewrites `<home>/descriptors/<id>.yaml` with `spawn.cmd` and `runtime.spawn.cmd` set to `filepath.Join(f.Dir, "missing-cli")` and bumps the file's mtime.

In `api/tests/regression_pending_prompt_test.go` (each test uses `kit.InstallFakeProvider` with `Base: "claude"`):

```go
func TestRegression_APromptWhoseLaunchFailsIsSettledNotLost(t *testing.T)
// create the chat, then fake.BreakSpawn(t) (descriptors reload on mtime)
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

These must stay green unchanged: `TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue` and every test in `regression_steered_prompt_test.go`.

- [ ] **Step 4: Run the new tests and see them fail**

Run: `gox go test -tags noEmbed ./internal/domain/ledger/... ./internal/adapter/store/ledger/...` → compile failure (packages missing).
Run: `bbx 'TestRegression_(APromptWhoseLaunchFails|ResendingTheSameRequestID|APromptTheProviderNever|PendingPromptSurvives)'` → expect:
- `APromptTheProviderNeverAcknowledgesSettlesOnIdle` FAILS: today the second prompt is refused until the 30 s sweep;
- the other two are characterization tests and may already pass.

- [ ] **Step 5: Implement** the domain package, the store, the wiring, the runner and turn changes, and the boot pass. Delete `agentjournal`, `promptrecovery.go`, `pendingprompt.go`, `settle.go` and the termwait delivery half, with their tests.

- [ ] **Step 6: Run to green**

Run: `gox go test -tags noEmbed -cover ./internal/domain/ledger/... ./internal/adapter/store/ledger/... ./internal/app/usecases/chat/... ./internal/adapter/...` → PASS. `domain/ledger` and `adapter/store/ledger` are new packages and must each be ≥ 92%. Report the other packages' numbers.
Run: `bbx 'TestRegression_(APromptWhoseLaunchFails|ResendingTheSameRequestID|APromptTheProviderNever|PendingPromptSurvives|PromptSentMidTurn|SteeredPrompt|CommandSentMidTurn|PromptSentWhileIdle|StopRefusesAParkedPrompt|ComposerBodyWithSelection|SelectionChangedMidTurn)|TestFakeProvider_'` → PASS.
Run: `gates` → clean. Then `grep -rn agentjournal api/` → nothing, and `git diff --stat HEAD -- web/` → empty.

- [ ] **Step 7: Commit**

```bash
git add -A api/internal/domain/ledger api/internal/adapter api/internal/app api/tests api/deadcode-baseline.txt scripts/ratchet/modules.json
git commit -m "feat(api): per-chat ledger store; prompts are ledger rows

Delete the prompt journal, its recovery and the 30 s delivery-quiet sweep.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- A prompt's existence, acknowledgement and settlement are ledger rows.
- Nothing writes under `chats/<id>/prompt-requests` any more.
- The `pending-prompt` endpoint and the `prompt_settled` frame are served from the ledger.
- `api/internal/adapter/store/agentjournal` no longer exists, and the deadcode baseline shrank by one line.
- Every file this task touched is ≤ 500 lines, or is oversize and shrank.
