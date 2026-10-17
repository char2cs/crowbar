# S1d — Chat feed, relay retirement, layout-1 conversion and the S1 gate: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A per-chat WebSocket pushes ledger rows by `seq`, resumable from any `seq` (T10); the relay hook transport is retired so HTTP hooks are the one ingress (T11); S1's piece of the legacy → layout-1 conversion exists as a tested, deterministic function (T12); the S1 gate is measured (T13).

**Architecture:** See the index, `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. Requires T1–T9.

**Tech Stack:** Go, gorilla/websocket, SQLite, cobra (`api/cmd/crowbar`), testify, `api/tests`.

**Spec:** architecture §4 ("The one stream" properties), §9 and spike Part D (migration engine contract, D3 step 2 "Session binding"); report §4.4.

---

### Task 10: The chat feed pushes rows

**Goal:** `GET /v0/chats/:chatId/feed?since=N` upgrades to a WebSocket that attaches live first, replays every row with `seq > N`, sends a `ready` frame carrying the daemon epoch and the head `seq`, then forwards each newly committed row once, in order. Ephemeral text rides the same socket as `delta` frames. A client that cannot keep up is told to resync and disconnected; nothing is silently skipped.

**Files:**
- Create: `api/internal/app/usecases/chat/feed.go` and `feed_internal_test.go`
- Create: `api/internal/api/v0/endpoints/chat/handlers/feed.go` and `feed_test.go`
- Create: `api/tests/chat_feed_test.go`
- Modify: `api/internal/app/usecases/chat/assembly.go` — one feed registry, fed by the ledger subscriber and by the ingress `Effects.Live`; epoch = `uuid.NewString()` minted once in `New`
- Modify: `api/internal/api/v0/router.go` — mount on the `chats` group (not `chatScoped`: a feed needs no worktree, so a home or bubble chat's feed works), next to `chatScoped.GET("/ws", …)`
- Modify: `api/internal/app/usecases/chat/chat.go` — `PurgeChat` tells the chat's feeds `deleted`

**Interfaces:**

```go
package chat
// FeedFrame is one frame on a chat feed. Exactly one of Row, Delta is set for
// "row"/"delta"; Epoch and Seq are set on "ready"; Seq on "resync".
type FeedFrame struct {
	Type  string      `json:"type"` // "row" | "ready" | "delta" | "resync" | "deleted"
	Row   *ledger.Row `json:"row,omitempty"`
	Epoch string      `json:"epoch,omitempty"`
	Seq   int64       `json:"seq,omitempty"`
	Delta *FeedDelta  `json:"delta,omitempty"`
}
type FeedDelta struct {
	MessageID string `json:"messageId"`
	Stream    string `json:"stream"` // "answer" | "reasoning" | "tool_output" | "diff"
	Text      string `json:"text"`   // everything so far
}
// OpenFeed attaches to chatID's live rows, then replays rows with seq > since.
// The returned channel yields replayed rows, the ready frame, then live frames;
// it is closed after a resync or deleted frame, or when ctx ends.
// apperr.ErrNotFound for an unknown chat.
func (u *Usecase) OpenFeed(ctx context.Context, chatID string, since int64) (<-chan FeedFrame, error)
```

The registry (`feed.go`): `newFeeds(capacity int)`; production capacity `1024`. Each subscriber owns a buffered channel of that capacity; the ledger subscriber does a non-blocking send of each row, and a full buffer marks the subscriber overflowed: it receives `resync{seq: last row it was sent}` and is closed. Replay dedupes by `seq`: live rows with `seq ≤` the last replayed row are dropped, so a row committed while the replay ran is delivered exactly once.

Handler: 404 (before upgrade) when `OpenFeed` returns not-found; `since` absent = 0, negative or non-integer = 400; write each frame as JSON text; on `resync`/`deleted` close with code 4000 / 4001.

- [ ] **Step 1: Tests**

```go
// feed_internal_test.go (newFeeds(2))
func TestFeeds_AFullBufferResyncsInsteadOfSkipping(t *testing.T)
// publish 3 rows without reading → frames: row1,row2,resync{seq:2}; channel closed
func TestFeeds_LiveRowsDuringReplayAreDeliveredOnce(t *testing.T)

// handlers/feed_test.go
func TestFeed_UnknownChatIs404(t *testing.T)
func TestFeed_BadSinceIs400(t *testing.T)

// api/tests/chat_feed_test.go (claude fake)
func TestChatFeed_ReplaysFromSinceThenGoesLive(t *testing.T)
// N rows exist; dial since=0 → row frames seq 1..N, then ready{epoch≠"", seq:N};
// post a hook → a row frame with seq N+1
func TestChatFeed_ResumeSendsOnlyNewerRows(t *testing.T)            // since=N-2 → exactly 2 rows then ready
func TestChatFeed_RowsCommittedWhileAttachingAreNeitherLostNorDoubled(t *testing.T)
// a goroutine posts 30 hooks while the test dials since=0 → the seqs read are 1..head, contiguous, no repeats
func TestChatFeed_UnnumberedDeltasAreLiveOnly(t *testing.T)
// stub fake whose message_delta maps no index: post two deltas → two delta frames, no row frame for them;
// re-dial since=0 → no delta frames replayed
func TestChatFeed_EpochChangesAcrossARestart(t *testing.T)          // ready.epoch differs after newHarnessAt on the same home; seqs continue
```

- [ ] **Step 2: Run; they FAIL (route missing).**
- [ ] **Step 3: Implement the registry, `OpenFeed`, the handler and the route.**
- [ ] **Step 4: Run to green** — `go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/api/...`; `go test -tags 'integration noEmbed' -count=1 -run 'TestChatFeed_|TestAgentWS_|TestRegression_AGrowingMessage' ./tests/`; `make lint deadcode`.
- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "feat(api): a per-chat feed pushes ledger rows by seq, resumable and loud on overflow

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** a client can render a chat from the feed alone; reconnecting with `since` loses and repeats nothing; overflow is a frame, never a gap.

---

### Task 11: Retire the relay hook transport (R9)

**Goal:** HTTP hooks are the single ingress. The `/chats/hooks`, `/chats/hooks/await`, `/chats/hooks/abandon` routes (repo and home mounts), their handlers, the `crowbar hook` subcommand and the `{crowbar_hook} hook` wiring form are deleted; every black-box test posts through the real HTTP hook route.

**Files:**
- Delete: `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go,hook_test.go,hook_answer_test.go}`; `newHookCmd()` from the root command in `api/cmd/crowbar/main.go`
- Modify: `api/internal/api/v0/endpoints/chat/{routes.go,routes_test.go}`, `api/internal/api/v0/endpoints/home/{routes.go,…_test.go}` — drop the three routes; `api/internal/api/v0/endpoints/chat/handlers/{hooks.go,hooks_test.go,answers.go,answers_test.go,handlers.go}` — drop `Hooks`, `AwaitHookAnswer`, `AbandonHookAnswer` and the `AgentHookAckDTO`/`AgentHookAwaitDTO`/`AgentHookAnswerDTO` types in `api/internal/api/v0/dto/agent.go`
- Modify: `api/internal/app/usecases/chat/internal/shared/answerdesk/` — delete what only the relay used (delivery-id long-poll `Pending`, `Await`, `Abandon` by delivery id) and keep what `HTTPHook`'s held response uses; `api/internal/app/usecases/chat/answers.go` likewise
- Modify: `api/internal/engine/agents/descriptorcheck/{safety.go,rules.go}` and tests — `hookCommandStarts` = `{"{hook_curl} "}`; a value containing `{crowbar_hook} hook ` is an error finding `hooks.relay_retired` with fix text "post hooks to {hook_url}/<event> (HTTP) or through {hook_curl}"; the `hooks.not_wired` fix text names only `{hook_url}`; `api/internal/engine/agents/internal/models/template.go` — drop the `{crowbar_hook}` placeholder (keep `{crowbar}`, the binary path the MCP injection uses)
- Modify: `api/internal/engine/agents/descriptorcheck/testdata/fakecli/descriptor.yaml` to the HTTP form. Leave `testdata/base-70ec430/*.yaml` byte-identical (historical baseline); update only the expected findings of the test that checks them.
- Modify: `api/tests/{regression_agent_message_stream_test.go,agent_home_hook_test.go,agent_internal_session_test.go,agent_working_overlay_test.go,regression_agent_chat_threads_test.go,regression_agent_chat_folders_test.go,regression_sidebar_forest_test.go,regression_steered_prompt_test.go}` and every other user of `postProviderHook`, `postHomeHook`, `postMemStubHook`, `postAgentHook`, `stubDescriptor` — install a `kit.FakeProvider` (`Base: ""` with the test's `EventsYAML`) and post with `Launch.PostHook`; delete the five helpers and `stubDescriptor`
- Delete: `api/tests/regression_agent_answer_window_test.go` (the relay's ack-await window no longer exists); `api/tests/regression_agent_choice_frame_test.go`'s `…RelayExpiredIsPushedAsUnanswerable` becomes `…AHeldAskWhoseCLIHungUpIsPushedAsUnanswerable` over `HTTPHook`
- Modify: `api/ARCHITECTURE.md` `cmd/` table and "Agent turn" flow — hooks are posted by the CLI to the daemon's loopback hook server

- [ ] **Step 1: Write the replacement test** — `api/internal/engine/agents/descriptorcheck/rules_test.go`: `TestRelayHookWiringIsRefusedWithTheReplacement` (a descriptor with `{crowbar_hook} hook tool_pre` → finding `hooks.relay_retired`, severity error).
- [ ] **Step 2: Run it; it FAILS.**
- [ ] **Step 3: Port the black-box helpers, delete the transport and the dead answer-desk paths, update descriptorcheck and the docs.**
- [ ] **Step 4: Run to green** — `go test -tags noEmbed ./cmd/... ./internal/engine/agents/... ./internal/app/usecases/chat/... ./internal/api/...`; `go test -tags 'integration noEmbed' -count=1 ./tests/` (the whole black-box package, once — every test there now uses the HTTP route); `make descriptor-check lint deadcode`; `grep -rn 'chats/hooks\|crowbar_hook\|AwaitHookAnswer' api --include=*.go` → only `descriptorcheck` (the refusal) and `base-70ec430` testdata.
- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "refactor(api): HTTP hooks are the only hook ingress; retire the relay transport

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** one ingress path; no test or production code posts to `/chats/hooks`; the deadcode baseline did not grow.

---

### Task 12: Legacy → layout-1 conversion for chats (R12)

**Goal:** A deterministic, pure function turns one chat's legacy records — conversation history, activity turns, tool calls, subagents, interruptions, choices, prompt journal files — into the ledger rows S1 defines; a read-only reader assembles those records from a legacy home; `crowbar chat-ledger plan --home <dir>` prints what it would write. Nothing is written anywhere. The migration engine (`docs/plans/2026-10-17-migration-engine-plan.md`) will run it as part of spike Part D step 2 and then delete the `chat-ledger` subcommand.

**Files:**
- Create: `api/internal/migrate/layout1/{chatledger.go,legacy.go,read.go}` and `chatledger_test.go`, `read_test.go`, `testdata/legacy-chat-a.json`, `testdata/legacy-chat-b.json`
- Create: `api/cmd/crowbar/chatledger.go` and `chatledger_test.go`; `api/cmd/crowbar/main.go` adds `newChatLedgerCmd()` (not `migrate`: the engine plan owns that name)

**Interfaces:**

```go
package layout1
// The legacy shapes live here and nowhere else (spike Part D: "legacy readers live
// inside the step"). Columns copied from the pre-S1 read models.
type LegacyConversation struct{ ChatID, ProviderID, SessionID string; FirstSeenAt, LastActiveAt time.Time }
type LegacyTurn struct {        // agent_turns
	ID, ChatID, Role, ProviderID, RunnerID, SessionID, Text, Effort, Status, Diff, Plan string
	Seq, DisplayOrder int64; ItemIndex int; StartedAt time.Time; EndedAt *time.Time
}
type LegacyTool struct {        // agent_tool_calls
	ID, TurnID, ChatID, Name, Kind, Locations, Target, RequestRef, ResultRef, Diff, Status, Error, SubagentID string
	Seq int64; DurationMS int; StartedAt time.Time; EndedAt *time.Time
}
type LegacySubagent struct{ ID, TurnID, ChatID, AgentType, Messages string; Seq int64; StartedAt time.Time; EndedAt *time.Time }
type LegacyInterruption struct{ ID, TurnID, ChatID, Kind, Detail string; Seq, DisplayOrder int64; At time.Time; ResolvedAt *time.Time }
type LegacyChoice struct {      // agent_choices, JSON columns kept as strings
	ID, TurnID, ChatID, Kind, PromptID, ToolID, ToolName, Title, Question, Mode, Options, Questions, Schema, Resolution, AnsweredOptionIDs string
	Multi bool; Seq int64; At time.Time; ResolvedAt *time.Time
}
type LegacyPrompt struct{ RequestID, Text, State, ProviderID string; CreatedAt, UpdatedAt time.Time } // prompt-requests/*.json
type LegacyChat struct {
	ChatID        string
	Conversations []LegacyConversation
	Turns         []LegacyTurn
	Tools         []LegacyTool
	Subagents     []LegacySubagent
	Interruptions []LegacyInterruption
	Choices       []LegacyChoice
	Prompts       []LegacyPrompt
}
type Result struct {
	Rows    []ledger.Row // At carries the legacy timestamp; Seq is 1..n in output order
	Skipped []string     // one line per legacy record not converted, with why
}
func ConvertChat(in LegacyChat) (Result, error)

// ReadLegacy opens <home>/state/store/agent_activity.db and agent_runner.db with
// mode=ro and reads <home>/chats/*/prompt-requests/*.json. Missing files are empty input.
func ReadLegacy(ctx context.Context, home string) ([]LegacyChat, error)
```

**Conversion rules (exact):**
1. **Turn ids are deterministic:** `uuid.NewSHA1(uuid.NameSpaceURL, []byte("crowbar:layout1:"+chatID+":"+legacyTurnID)).String()`. Running the step twice yields identical rows (Part D: steps are idempotent).
2. **Session binding (spike D3):** per provider, the current session is the session id among that provider's `LegacyConversation`s whose newest `LegacyTurn` (by `StartedAt`, matching `SessionID`) is latest; when no turn carries a session id, the conversation with the latest `LastActiveAt`. Emit `session_bound{cause: announced}` for every other conversation of the provider first (oldest `FirstSeenAt` first, so `Conversations()` keeps history), then the current one last.
3. **Turns:** order legacy turns by `(DisplayOrder, ItemIndex, Seq)`. A `user` row opens a ledger turn: `user_message{origin: crowbar, requestId: legacy id}` when a `LegacyPrompt` with that request id exists, else `user_message{origin: provider}`; then `turn_started`. Each following `assistant` row (until the next `user` row) becomes `assistant_text{messageId: legacy id, index: -1, final: true, text}` in that turn; the turn then ends with `turn_ended{done, effort}` or, by the last assistant row's `Status`, `turn_failed{reason: status}` / `turn_ended{interrupted}`. A non-empty `Plan` becomes a `plan{op: replace}` row before the end. `harness` and `notice` rows are skipped with a reason.
4. **Tools:** `tool_call{phase: pre}` at `StartedAt` and, when `EndedAt` is set, `tool_call{phase: post|fail}` at `EndedAt`, keeping `RequestRef`/`ResultRef` verbatim (the blob directory is unchanged). The tool's ledger turn is the one whose legacy turn id equals the tool's `TurnID`, or whose `open-…`/`msg-…` id maps to it; a tool with a `SubagentID` gets `Scope` = that id. Unplaceable tools are skipped with a reason.
5. **Subagents:** `subagent{phase: start, type}` and, when ended, `subagent{phase: stop, result: last of Messages}` with `Scope` = the legacy id.
6. **Interruptions:** `compaction` → `marker{compaction_started}` (+ `marker{compaction_ended}` at `ResolvedAt`); `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification` → `marker` of the same kind; `inferred` → `turn_ended{interrupted, reason: inferred}` for its turn when that turn has no end, else skipped; `permission`/`elicitation` are covered by choices and skipped.
7. **Choices:** `ask{askId: legacy id, …}` at `At`; when resolved, `ask_resolved{resolution, optionIds}` at `ResolvedAt`.
8. **Prompts:** `accepted` → covered by its turn; `dispatching`, `spawned`, `uncertain` → `user_message{origin: crowbar, requestId}` + `prompt_settled{consumed: false, reason: daemon_restart}`; `failed` → the same with `reason: delivery_failed`; `settled` → `user_message` + `prompt_settled{consumed: false, reason: not_acknowledged}`.
9. **Order:** all rows by `At`, ties by the legacy `Seq`, then by the rule order above; then `Seq` 1..n. `ledger.Check` must pass for every row and `ledger.Fold(chatID, rows)` must leave no running turn, no pending prompt and no starting/running subagent.

**CLI:** `crowbar chat-ledger plan --home <dir>` (required flag; no default) prints `chat <id>: <rows> rows, <skipped> skipped` per chat and `total: <chats> chats, <rows> rows, <skipped> skipped`; it never writes. Run it only against fixture homes built in tests — never against `~/.crowbar`.

- [ ] **Step 1: Tests** — fixtures `legacy-chat-a.json` (two claude conversations, the older with turns; 3 user/assistant exchanges; 2 tools incl. one nested under a subagent; 1 subagent; 1 compaction; 1 resolved permission choice; 1 `uncertain` prompt) and `legacy-chat-b.json` (codex, untagged turns only, one `inferred` interruption):

```go
func TestConvertChat_BindsTheNewestSessionThatHasTurns(t *testing.T)
func TestConvertChat_FallsBackToTheLastActiveConversation(t *testing.T) // chat b
func TestConvertChat_TurnsBecomePromptTextAndAnEnd(t *testing.T)
func TestConvertChat_ToolsKeepTheirBlobRefsAndScopes(t *testing.T)
func TestConvertChat_InterruptionsAndChoices(t *testing.T)
func TestConvertChat_UnfinishedPromptsAreSettledNeverReplayed(t *testing.T)
func TestConvertChat_IsDeterministic(t *testing.T)            // two runs → reflect.DeepEqual
func TestConvertChat_FoldsToARestingChat(t *testing.T)        // every row passes Check; Fold: no running turn/pending prompt/live subagent
func TestReadLegacy_ReadsWithoutWriting(t *testing.T)
// build a temp home: create the legacy tables from the DDL in legacy.go, insert chat a,
// write its prompt file; ReadLegacy; assert equality with the fixture; assert every
// file's mtime and size are unchanged and no -wal/-shm file was created
// api/cmd/crowbar/chatledger_test.go
func TestChatLedgerPlan_PrintsCountsAndWritesNothing(t *testing.T)
func TestChatLedgerPlan_RequiresHome(t *testing.T)
```

- [ ] **Step 2: Run; they FAIL (package missing).**
- [ ] **Step 3: Implement `legacy.go` (types + legacy DDL used by the reader's queries and by tests), `read.go`, `chatledger.go`, the subcommand.**
- [ ] **Step 4: Run to green** — `go test -tags noEmbed ./internal/migrate/... ./cmd/crowbar/...`; `make lint deadcode` (the subcommand makes the step reachable from `./cmd/...`).
- [ ] **Step 5: Commit**

```bash
git add -A api/internal/migrate api/cmd/crowbar
git commit -m "feat(api): legacy chat records to ledger rows, as a deterministic migration step

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** the conversion is pure and deterministic; the reader never writes; the dry run is the only wiring and prints counts.

---

### Task 13: The S1 gate

**Goal:** Prove S1's gate — symptoms 2, 3 and 4 closed by HTTP/WS behaviour tests; chat subtree −10k source lines and −20k test lines — with every repository gate green and the app verified live.

**Files:**
- Modify: `api/ARCHITECTURE.md` — `engine/agents` no longer holds a runner aggregate; `adapter/` lists `state/store/chat_ledger.db`; types list drops `agent_activity` and `agent_runner`; request flow "Agent turn" names the ledger and the ingress; `tests/` names the fake provider harness
- Modify: `api/deadcode-baseline.txt` only by `scripts/deadcode-check.sh --update` and only if it shrank

- [ ] **Step 1: Disk check** — `df -h /System/Volumes/Data`; under 8 GB free → stop and report.

- [ ] **Step 2: Symptom tests**

Run: `cd api && go test -tags 'integration noEmbed' -count=1 -v -run 'TestRegression_Symptom(2|3|4)_' ./tests/ 2>&1 | tee /dev/stderr | grep -E '^(--- |ok|FAIL)'`
Expected: 7 `Symptom2`, 4 `Symptom3`, 6 `Symptom4` tests, each `--- PASS`; package `ok`. Count the PASS lines; do not read a trailing `ok` as proof by itself.

- [ ] **Step 3: Line counts** (run from `api/`; compare with the index baseline: source 41,356, tests 72,596 at `9641256b5`):

```bash
cd api
paths="internal/app/usecases/chat internal/app/repositories/chat internal/engine/agents/runner
internal/engine/agents/internal/move internal/engine/agents/internal/sessionstore
internal/engine/agents/session_locate.go internal/adapter/store/agentjournal internal/adapter/store/ledger
internal/domain/ledger internal/migrate internal/api/v0/endpoints/chat internal/domain/chat.go
internal/domain/chat_activity.go internal/domain/agent_session.go internal/domain/agent_pending_prompt.go
internal/domain/chat_log.go cmd/crowbar/hook.go cmd/crowbar/hook_delivery.go cmd/crowbar/hook_answer.go"
src=0; tst=0
for p in $paths; do
  [ -e "$p" ] || continue
  s=$(find "$p" -name '*.go' ! -name '*_test.go' -print0 | xargs -0 cat 2>/dev/null | wc -l)
  t=$(find "$p" -name '*_test.go' -print0 | xargs -0 cat 2>/dev/null | wc -l)
  src=$((src+s)); tst=$((tst+t))
done
bt=$(find tests -name '*_test.go' \( -name '*agent*' -o -name '*chat*' -o -path '*/integration/agent/*' \) -print0 | xargs -0 cat | wc -l)
echo "chat subtree: src=$src test=$((tst+bt))"
```

Expected: `src ≤ 31356` and `test ≤ 52596`. If either misses, the gate fails: report the gap per directory (rerun the loop printing each path) and stop. Do not move code out of the measured paths to pass.

- [ ] **Step 4: Repository gates**

Run: `cd api && make pr-checks` (lint, deadcode, `go test -race`, coverage ≥ 92, vet) → all pass; then `make descriptor-check`.
Run: `cd api && env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin make test-integration` (no provider CLI on `PATH`, so live provider tests skip and nothing touches a provider home) → pass.
Run: `git diff 9641256b5 --stat -- web/` → empty (S1 changed no client file).
Run: `grep -rnE 'claude|codex|anthropic|openai' api/internal --include=*.go | grep -v _test.go | grep -v '/testdata/'` → nothing new compared with `git grep` at `9641256b5` (no provider vocabulary added to Go).

- [ ] **Step 5: Live check in Tauri (isolated home)** — `make dev-desktop` from the worktree root (its `CROWBAR_HOME` is the worktree's own `.crowbar`; never the default home). Install the fake provider descriptor into that dev home (`<dev home>/descriptors/claude.yaml`, built as `kit.InstallFakeProvider` builds it, `FAKECLI_DIR` a scratch directory). With the Tauri MCP: open a scratch repo project; create a chat; send a prompt; read the capture to get the runner id and token; post `user_prompt`, two `message_delta` chunks, a `tool_pre`/`tool_post` pair and `turn_stop` to the dev hook server (`<dev home>/run/hooks.url`) with curl; confirm in the window that the prompt echo, the streaming text, the tool row and the final reply render and the spinner stops; click Stop on a running turn and confirm the spinner stops at once; quit and relaunch the dev app and confirm the transcript and the dormant state are intact; send a prompt and confirm the capture shows `--resume <same id>`. Real provider CLIs are not used in this check; a real-CLI smoke runs only if the user asks for it in the session.

- [ ] **Step 6: Commit**

```bash
git add api/ARCHITECTURE.md api/deadcode-baseline.txt
git commit -m "docs(api): describe the chat ledger architecture after S1

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** 17 symptom tests pass; both line budgets met; `make pr-checks`, `make descriptor-check` and `make test-integration` green; the live check passes; no web file changed.
