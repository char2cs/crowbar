# S1d — Subagents, chat feed, relay retirement, migration step 6 and the S1 gate: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Subagents and plans are ledger entities (T9). A per-chat WebSocket pushes ledger rows by `seq`, resumable from any `seq`, with suffix deltas (T10). The relay hook transport is retired, so HTTP hooks are the one ingress (T11). Migration step 6 `chat-ledger` converts legacy chat records at boot (T12). The S1 gate is measured (T13).

**Architecture:** The index is `docs/plans/2026-10-17-s1-chat-ledger-plan.md`. T9 requires T1–T8 committed (`docs/plans/2026-10-17-s1c-turns-and-rows-plan.md`); each later task requires the ones before it.

**Tech Stack:** Go, gorilla/websocket, SQLite, testify, `api/tests`, the descriptor grammar, and the migration engine (`api/internal/migrate`, `api/internal/adapter/migrations`).

**Spec:** architecture §4 ("The one stream" properties), §9 and spike Part D (migration engine contract, D3 step 2 "Session binding"); report §4.4; additions design §3.1, §4.C.

---

### Task 9: Subagents and plans as entities

**Goal:**
- A subagent is an entity with its own lifecycle, bound to the tool call that spawned it and independent of the parent turn.
- Its result, title, type and usage come from the descriptors.
- Claude's todo hooks and Codex's plan become plan rows.
- Busy no longer needs the `async_work` level.
- Codex child frames that arrive before their bind are held in the bounded buffer, not dropped.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`. No real provider CLI runs.

**Rules this task carries:**
- A hand-back opens a provider turn (pre-flight R3). Claude runs a model turn on a subagent's hand-back (fixture 23 → 27 → 28). So a `subagent_result` from a user-prompt hook writes:
  - the `subagent{phase: result}` row;
  - **and** `turn_started` with no `user_message`, keyed by the hook's `prompt_id` as `providerTurnId`, and a new Crowbar turn id.
  - Without this, 28-Stop finds no running turn, and Symptom 3 goes red.
- One canonical name per fact (pre-flight R16): rename the existing `subagent_id` to `scope_id` everywhere (`vocabulary.yaml` lines 30–31, 53, 76–77, 97, 102, 107; `claude.yaml:76,116,132,152`; `codex.yaml`; Go readers). Do not add a second field.
- Fixture facts (pre-flight R16): in `claude-subagent-session`, the real subagent stops are `25-SubagentStop.json` (`a3edc502687d15840`) and `26-SubagentStop.json` (`a6fec6ffa95979d56`), matching `05`/`06-SubagentStart.json`. `17`, `31`, `35` and `37` carry `agent_type ""`: they are harness-internal, and rule 6 applies to them.
- The idle event keeps its message-equality `when:` (pre-flight R17). No safe real `idle_prompt` capture exists, so do not switch it to `notification_type`.
- `kit.ClaudeHookRoutes` becomes a function. Update every caller (pre-flight R21): `api/tests/fake_provider_test.go`, `api/tests/regression_chat_transcript_test.go` (T7) and this task's tests.
- Behaviour change, accepted provisionally on 2026-10-18: a background subagent keeps the chat working until its own stop.
- Oversize (pre-flight R8): `mapping/mapping_test.go` (503) must net-shrink, or be split below 500; new transform tests go in `spec/expr_test.go`. `mapping.go`, `inbound/hooks.go` and the split DTO files stay ≤ 500.

**Files:**
- Modify: `vocabulary.yaml`:
  - `tool_pre`/`tool_post`/`tool_fail` carry `scope_id` (renamed), `spawns_scope` (renamed from `nested_session_id`) and a new `addresses_scope`;
  - `subagent_pre`/`subagent_post` gain `title`, `status` (with `status_map`), `result`, `usage.tokens`, `usage.tool_uses` and `usage.duration`;
  - new in-events `plan_step` {`scope_id`, `step_id`, `text`, `detail`, `status`} and `subagent_result` {`scope_id`, `status`, `result`, `usage.*`};
  - `in:` accepts a list, so several wire events feed one canonical event (as `ask:` already does on the codex api channel).
- Modify: `api/internal/engine/agents/internal/spec/{expr.go,expr_test.go}` and `mapping/mapping.go` — a `{json: <path>, path: <path>}` transform: decode a JSON-encoded string, then select
- Modify: `api/internal/engine/agents/internal/spec/injected.go` and `translate/inbound/hooks.go` — `injected_prompts` with `kind: subagent_result` produce a `subagent_result` event carrying the hook's `prompt_id` instead of a user prompt
- Modify: `claude.yaml` and `codex.yaml` (values below); delete `features.collab_agents=true` from `codex.yaml`
- Modify: `api/internal/engine/agents/internal/models/event.go`:
  - `SubagentEvent{ID, AgentType, Title, Status, Result string; Usage *SubagentUsage}`;
  - `PlanStepEvent`;
  - `ToolEvent{ScopeID, SpawnsScope, AddressesScope string}` (`NestedSessionID` renamed `SpawnsScope`).
- Modify: `api/internal/domain/ledger/{subagent.go,plan.go,state.go,turn.go}`:
  - the subagent lifecycle and the `plan` upsert fold;
  - `Busy()` drops the `async_work` clause, and `TurnEnded.AsyncWork` is deleted.
- Modify: `api/internal/app/usecases/chat/internal/ingress/{rows.go,nested.go}` — scope rows for every child event; the hand-back turn; the buffer
- Modify: `api/internal/app/usecases/chat/internal/transcript/activity.go` — `ActivitySubagent` comes from the entity; the subagent's `Messages` from its `result` and its scope's settled text
- Modify: the split `api/internal/api/v0/dto/` files — `AgentSubagentDTO` gains `title`, `status`, `toolUses`, `tokens` (additive)
- Modify: `api/tests/kit/fakeprovider.go`:
  - `ClaudeHookRoutes` becomes `func ClaudeHookRoutes(hookEventName, toolName string) []string`, mirroring the new registrations: `TaskCreated`/`TaskCompleted` → `plan_step`; `PostToolUse` → `tool_post`, plus `plan_step` only when `toolName == "TaskUpdate"`;
  - `ReplayHooks` takes that function.

**Interfaces:**

```go
package ledger
type SubagentStatus string // starting | running | done | failed | interrupted | abandoned
// Next: starting→running|failed|abandoned, running→done|failed|interrupted.
func (s SubagentStatus) Next(to SubagentStatus) (SubagentStatus, bool)
type SubagentUsage struct{ Tokens, ToolUses, DurationMs int64 }
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
1. A `tool_call pre` whose `kind` is `subagent` creates the entity `starting`: `subagent{phase: start, spawnToolId, type, title}` with `Scope ""`. The entity is keyed by `spawnToolId` until it is bound.
2. Bound by whichever arrives first: `subagent_pre` with `scope_id`; the spawning call's `tool_post` `spawnsScope`; or (codex api) `spawnsScope` = `receiverThreadIds[0]`. The bind is one `subagent{phase: bound, spawnToolId}` row whose `Scope` is the real scope; the entity becomes `running`. A `subagent_pre` with no matching starting entity creates the entity directly as `running`.
3. `subagent_post`, `subagent_result`, or the child's `turn_stop` (codex) → `done` (or `failed`, by `status_map`), with `result` and `usage`.
4. The user's Stop or a runner exit moves every `starting`/`running` subagent of the chat to `interrupted`. A parent turn that ends while a subagent is still `starting` moves that one to `abandoned`.
5. A `subagent{phase: start}` row with a non-empty `Scope` (written by T7) folds as started and bound.
6. A `subagent_post` with no prior start and an empty `agent_type` comes from a harness-internal agent. It writes no row; it is dead-lettered with reason `internal_subagent`.
7. Bounded buffer: a frame whose scope is not yet bound waits in a per-chat buffer of 256 frames, drained in arrival order on bind. At the parent turn's end, the remainder is dead-lettered (`unclaimed_scope`); a full buffer dead-letters its oldest frame.
8. Codex `wait`/`closeAgent` calls (`addressesScope` set) are `meta`: recorded as tool rows, never rendered as their own card (S2).

**Descriptor values** (additions design §3.1 sketches, checked against the captures):

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
# idle keeps its message-equality when: (no real capture carries notification_type)
# claude settings.json hook registration: add TaskCreated and TaskCompleted posting to
# {hook_url}/plan_step, and a second PostToolUse entry with "matcher":"TaskUpdate" posting to /plan_step

# codex.yaml — api channel
tool_pre/tool_post: { map: { spawns_scope: "item.receiverThreadIds[0]", addresses_scope: "item.receiverThreadIds[0]" } }
kind_map join: { join: [item.type, item.tool] }   # collabAgentToolCall/spawnAgent ≠ …/wait
# codex.yaml — hooks channel
tool_post: { map: { spawns_scope: { json: tool_response, path: agent_id } } }
subagent_pre / subagent_post: drop `unverified`
```

- [ ] **Step 1: Tests**

```go
func TestSubagentStatus_Next(t *testing.T)
func TestFold_ASubagentOutlivesItsParentTurnAndKeepsTheChatBusy(t *testing.T)
// tool pre(kind subagent) T1, subagent start bound A, turn_ended T1 → Busy (A running);
// subagent stop A → !Busy
func TestFold_AnUnboundSubagentIsAbandonedWhenItsTurnEnds(t *testing.T)
func TestFold_StopInterruptsEveryLiveSubagent(t *testing.T)
func TestFold_PlanUpsertAndReplace(t *testing.T)
func TestJSONTransform_DecodesAStringField(t *testing.T)            // spec/expr_test.go
func TestInjectedSubagentResultIsNotAUserPrompt(t *testing.T)       // claude fixture UserPromptSubmit.AgentMessage.json → subagent_result
func TestMap_AHandBackOpensAProviderTurnWithNoUserMessage(t *testing.T) // turn_started{providerTurnId: prompt_id}, no user_message
func TestIngress_ChildFramesBeforeTheBindAreHeldThenDrained(t *testing.T)
// api/tests (black box, claude fake replaying claude-subagent-session, ReplayHooks with the
// ClaudeHookRoutes function, recorded session id rewritten to the bound one)
func TestRegression_ABackgroundSubagentKeepsTheChatWorkingUntilItStops(t *testing.T)
// replay 00..11 (11-Stop ends the parent turn while a3edc… and a6fec… run) → working true;
// 12..25 (25 stops a3edc…) → still true; 26 (a6fec… stops) and 27..28 (28-Stop closes the
// hand-back turn that 23 opened) → working false; 28's last_assistant_message is recorded
func TestRegression_ATodoListBecomesPlanRows(t *testing.T)
// post TaskCreated ×2 and a TaskUpdate to in_progress through /plan_step → the chat feed's
// plan frame carries both items, one "active"
```

`TestRegression_CodexBackgroundedSubagentKeepsChatWorking` (live, under `api/tests/integration/agent`) must still compile: `gates` vets it. Never run it.

- [ ] **Step 2: Run; they FAIL.**

- [ ] **Step 3: Implement:**
- the grammar, the vocabulary rename and additions, the descriptor values, the entity fold, ingress routing, the hand-back turn and the buffer, and the projection;
- delete `AsyncWork` end to end: `TurnEnded.AsyncWork`, `CanonicalEvent.AsyncWork`, the `async_work` descriptor key and its vocabulary entry;
- delete the `routeNestedSubagentEvent` special path.

- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./internal/engine/agents/... ./internal/domain/ledger/... ./internal/app/usecases/chat/...`.
- `bbx 'TestRegression_(ABackgroundSubagent|ATodoList|Symptom3_|Symptom4_|ANested|AStreamingMessageCarries)|TestFakeProvider_'`.
- `gox make descriptor-check` and `gates`.
- `grep -rn 'subagent_id\|async_work\|AsyncWork\|NestedSessionID' api/internal` → nothing.

- [ ] **Step 5: Commit**

```bash
git add -A api scripts/ratchet/modules.json
git commit -m "feat(api): subagents and plans are ledger entities fed by the descriptors

Busy no longer leans on an async-work level; codex child frames that precede
their bind are held, not dropped; a subagent's hand-back opens its own turn.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- No subagent event is dropped silently.
- A background subagent keeps the chat busy until its own stop.
- The hand-back's reply is recorded.
- `async_work` and `subagent_id` no longer exist anywhere.

---

### Task 10: The chat feed pushes rows

**Goal:** `GET /v0/chats/:chatId/feed?since=N` upgrades to a WebSocket that:
- attaches live first, replays every row with `seq > N`, and sends a `ready` frame carrying the daemon epoch and the head `seq`;
- then sends one snapshot frame per live text stream, and forwards each newly committed row once, in order.

Ephemeral text rides the same socket as `delta` frames, and each frame carries only new text, numbered per stream. A client that cannot keep up is told to resync and is disconnected; nothing is silently skipped.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- Final delta shape now (S2 pre-flight amendment P3). This task writes the `FeedDelta` S2 renders, so S2 never rewrites it:
  - each delta carries only the new suffix, numbered per stream (`n`, 1-based);
  - a rewrite that does not extend the text so far is a `replace`;
  - after `ready`, a snapshot of one `replace` frame per live stream.
- The server never drops a delta silently (P6): deltas share the subscriber's buffer, and overflow is a `resync`.
- The lifecycle `message_delta` frame on `…/chats/ws` stays cumulative (P5).
- No timer: reuse the existing WebSocket keepalive and add no ticker (the counters ratchet must not grow).

**Files:**
- Create: `api/internal/app/usecases/chat/feed.go` and `feed_internal_test.go`
- Create: `api/internal/api/v0/endpoints/chat/handlers/feed.go` and `feed_test.go`
- Create: `api/tests/chat_feed_test.go`
- Modify: `api/internal/app/usecases/chat/internal/ingress/live.go` and `live_internal_test.go`:
  - the live buffer keeps, per (turn, scope, id, stream), the text so far and a counter `n`;
  - each provider delta becomes one `FeedDelta` carrying the suffix, or `Replace` with the full text when the new text does not extend the old;
  - `Snapshot(chatID) []FeedDelta` returns one `Replace` frame per live stream, with that stream's current `n`;
  - an entry is dropped when its complete row lands or its turn closes;
  - the cumulative text still feeds the lifecycle `message_delta` frame.
- Modify: `api/internal/app/usecases/chat/assembly.go` (287 lines):
  - one feed registry, fed by the ledger subscriber and by the ingress `Effects.Live`;
  - epoch = `uuid.NewString()`, minted once in `New`.
- Modify: `api/internal/api/v0/router.go` — mount on the flat `chats` group (not `chatScoped`: a feed needs no worktree, so a home or bubble chat's feed works), next to `chatScoped.GET("/ws", …)`
- Modify: `api/internal/app/usecases/chat/chat.go` (411; keep ≤ 500) — `PurgeChat` tells the chat's feeds `deleted`

**Interfaces:**

```go
package chat
// FeedFrame is one frame on a chat feed. Row is set on "row", Delta on "delta",
// Epoch and Seq on "ready", Seq on "resync".
type FeedFrame struct {
	Type  string      `json:"type"` // "row" | "ready" | "delta" | "resync" | "deleted"
	Row   *ledger.Row `json:"row,omitempty"`
	Epoch string      `json:"epoch,omitempty"`
	Seq   int64       `json:"seq,omitempty"`
	Delta *FeedDelta  `json:"delta,omitempty"`
}
type FeedDelta struct {
	ID      string `json:"id"` // message id (answer, reasoning) or tool id (tool_output, diff)
	TurnID  string `json:"turnId"`
	Scope   string `json:"scope,omitempty"`
	Stream  string `json:"stream"` // "answer" | "reasoning" | "tool_output" | "diff"
	N       int    `json:"n"`      // 1-based position of this frame in its stream
	Text    string `json:"text"`   // the new suffix; everything so far when Replace
	Replace bool   `json:"replace,omitempty"`
}
// OpenFeed attaches to chatID's live rows, then replays rows with seq > since.
// The channel yields the replayed rows, the ready frame, one Replace delta per live
// stream (Snapshot taken after the subscriber was registered), then live frames.
// It is closed after a resync or deleted frame, or when ctx ends.
// apperr.ErrNotFound for an unknown chat.
func (u *Usecase) OpenFeed(ctx context.Context, chatID string, since int64) (<-chan FeedFrame, error)
```

**Registry** (`feed.go`):
- `newFeeds(capacity int)`; the production capacity is `1024`.
- Each subscriber owns a buffered channel of that capacity. The ledger subscriber and the live-delta path each do a non-blocking send.
- A full buffer marks the subscriber overflowed: it receives `resync{seq: the last row it was sent}` and is closed.
- Replay dedupes by `seq`: live rows with `seq ≤` the last replayed row are dropped, so a row committed while the replay ran is delivered exactly once.
- A client applies a delta only when `n == last+1`; a `Replace` sets `last = n`. A delta racing the snapshot therefore never duplicates text.

**Handler:**
- 404 (before upgrade) when `OpenFeed` returns not-found.
- `since` absent = 0; negative or non-integer = 400.
- Each frame is written as JSON text. On `resync`/`deleted`, close with code 4000 / 4001.

- [ ] **Step 1: Tests**

```go
// ingress/live_internal_test.go
func TestLive_EachDeltaCarriesOnlyTheNewSuffix(t *testing.T)
// provider deltas "a","ab","abc" for one message → FeedDelta{N:1,"a"}, {N:2,"b"}, {N:3,"c"}
func TestLive_ARewriteThatDoesNotExtendIsAReplace(t *testing.T)
// "abc" then "xy" → {N:4, Text:"xy", Replace:true}
func TestLive_SnapshotHasOneReplacePerLiveStream(t *testing.T)
// two messages + one tool_output live → 3 frames, Replace, N = each stream's current n
func TestLive_ACompleteRowDropsItsStreamFromTheSnapshot(t *testing.T)

// feed_internal_test.go (newFeeds(2))
func TestFeeds_AFullBufferResyncsInsteadOfSkipping(t *testing.T)
// publish 3 rows without reading → frames: row1,row2,resync{seq:2}; channel closed
func TestFeeds_LiveRowsDuringReplayAreDeliveredOnce(t *testing.T)
func TestFeeds_ReadyIsFollowedBySnapshotsThenLiveFrames(t *testing.T)
func TestFeeds_ADeltaRacingTheSnapshotNeverDuplicatesText(t *testing.T)
// a delta published between registration and snapshot: applying the client rule
// (append only when n == last+1, Replace sets last = n) yields the exact text

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
// stub fake (Base "", message_delta maps no index): post "a" then "ab" → delta frames
// {n:1,"a"}, {n:2,"b"}, no row frame for them; re-dial since=0 → a snapshot Replace "ab" only
func TestChatFeed_EpochChangesAcrossARestart(t *testing.T)          // ready.epoch differs after newHarnessAt on the same home; seqs continue
```

- [ ] **Step 2: Run; they FAIL** (route missing; `N`, `Replace`, `Snapshot` undefined).
- [ ] **Step 3: Implement** the buffer counters and suffixes, the registry, `OpenFeed` with the post-ready snapshot, the handler and the route.
- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./internal/app/usecases/chat/... ./internal/api/...`;
- `bbx 'TestChatFeed_|TestAgentWS_|TestRegression_(AGrowingMessage|AStreamingMessageCarries|EveryAssistantMessage)'` (the lifecycle socket's cumulative `message_delta` stays green);
- `gates`.
- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "feat(api): a per-chat feed pushes ledger rows by seq, resumable and loud on overflow

Deltas carry only new text, numbered per stream, with a snapshot after ready.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- A client can render a chat from the feed alone.
- Reconnecting with `since` loses and repeats nothing.
- Overflow is a frame, never a gap.
- Deltas are suffixes with a per-stream `n`.

---

### Task 11: Retire the relay hook transport

**Goal:** HTTP hooks are the single ingress. Deleted: the `/chats/hooks`, `/chats/hooks/await` and `/chats/hooks/abandon` routes (repo and home mounts), their handlers, the `crowbar hook` subcommand and the `{crowbar_hook} hook` wiring form. Every black-box test posts through the real HTTP hook route.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`.

**Rules this task carries:**
- The answer desk was rekeyed by ask id in T6, so `HTTPHook`'s held response no longer depends on a delivery id. Delete only what the relay alone uses (pre-flight R10).
- Known consequence (pre-flight §6 item 11): a custom descriptor override that still uses `{crowbar_hook} hook` is refused by `descriptorcheck`, and its finding names the replacement.
- Oversize (pre-flight R8): the ported test files `api/tests/agent_runner_moves_test.go` (shrunk in T3), `agent_rest_scope_test.go` (511) and `regression_agent_chat_threads_test.go` (590) must net-shrink. Swapping a helper call for `Launch.PostHook` should not add lines; if a file grows, split it below 500. `handlers.go` and `hooks_test.go` only shrink.

**Files:**
- Delete: `api/cmd/crowbar/{hook.go,hook_delivery.go,hook_answer.go,hook_test.go,hook_answer_test.go}`, and `newHookCmd()` from the root command in `api/cmd/crowbar/main.go`
- Modify: drop the three routes from `api/internal/api/v0/endpoints/chat/{routes.go,routes_test.go}` (`routes.go:156-159`) and from `api/internal/api/v0/endpoints/home/routes.go` (`:175`, `:180`, `:181`) and its test; the runner-keyed `HTTPHook` routes (`chat/routes.go:156`, `home/routes.go:174`) stay
- Modify: `api/internal/api/v0/endpoints/chat/handlers/{hooks.go,hooks_test.go,answers.go,answers_test.go,handlers.go}`:
  - drop `Hooks`, `AwaitHookAnswer` and `AbandonHookAnswer`;
  - drop the `AgentHookAckDTO`/`AgentHookAwaitDTO`/`AgentHookAnswerDTO` types in the split `api/internal/api/v0/dto/` files.
- Modify: `api/internal/app/usecases/chat/internal/shared/answerdesk/` and `api/internal/app/usecases/chat/answers.go`:
  - delete what only the relay used: the declared-wait long-poll the `await` route served, and anything that only the deleted handlers call;
  - keep the ask-id `Hold`/`Await`/`Abandon` that `HTTPHook` uses.
- Modify: `api/internal/engine/agents/descriptorcheck/{safety.go,rules.go}` and their tests:
  - `hookCommandStarts` = `{"{hook_curl} "}`;
  - a value containing `{crowbar_hook} hook ` is an error finding `hooks.relay_retired`, with fix text "post hooks to {hook_url}/<event> (HTTP) or through {hook_curl}";
  - the `hooks.not_wired` fix text names only `{hook_url}`;
  - `api/internal/engine/agents/internal/models/template.go` drops the `{crowbar_hook}` placeholder (keep `{crowbar}`, the binary path the MCP injection uses).
- Modify: `api/internal/engine/agents/descriptorcheck/testdata/fakecli/descriptor.yaml` to the HTTP form. Leave `testdata/base-70ec430/*.yaml` byte-identical (the historical baseline); update only the expected findings of the test that checks them.
- Modify the black-box tests:
  - files: `api/tests/{regression_agent_message_stream_test.go,agent_home_hook_test.go,agent_internal_session_test.go,agent_working_overlay_test.go,regression_agent_chat_threads_test.go,regression_agent_chat_folders_test.go,regression_sidebar_forest_test.go,regression_steered_prompt_test.go}` and every other user of `postProviderHook`, `postHomeHook`, `postMemStubHook`, `postAgentHook` or `stubDescriptor` (22 files at S1 start);
  - install a `kit.FakeProvider` (`Base: ""` with the test's `EventsYAML`) and post with `Launch.PostHook`;
  - delete the five helpers (defined in `agent_home_hook_test.go`, `regression_agent_message_stream_test.go`, `agent_working_overlay_test.go`, `agent_internal_session_test.go`) and `stubDescriptor`.
- Delete: `api/tests/regression_agent_answer_window_test.go` (the relay's ack-await window no longer exists)
- Modify: `api/tests/regression_agent_choice_frame_test.go` — `TestRegression_AChoiceWhoseRelayExpiredIsClosedAsProceeded` (`:149`) becomes `TestRegression_AHeldAskWhoseCLIHungUpIsClosedAsProceeded`, over `HTTPHook`
- Modify: `api/ARCHITECTURE.md` (243 lines; edit on top of the engine's rewrite) — the `cmd/` table, and the "Agent turn" flow: hooks are posted by the CLI to the daemon's loopback hook server

- [ ] **Step 1: Write the replacement test.** In `api/internal/engine/agents/descriptorcheck/rules_test.go`, add `TestRelayHookWiringIsRefusedWithTheReplacement`: a descriptor with `{crowbar_hook} hook tool_pre` → finding `hooks.relay_retired`, severity error.
- [ ] **Step 2: Run it; it FAILS.**
- [ ] **Step 3: Port the black-box helpers; delete the transport and the relay-only desk paths; update descriptorcheck and the docs.**
- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./cmd/... ./internal/engine/agents/... ./internal/app/usecases/chat/... ./internal/api/...`;
- `bbx '.'`: the whole black-box package once; every test in it now uses the HTTP route;
- `gox make descriptor-check` and `gates`;
- `grep -rn 'chats/hooks\|crowbar_hook\|AwaitHookAnswer' api --include=*.go` → only `descriptorcheck` (the refusal) and the `base-70ec430` testdata.
- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "refactor(api): HTTP hooks are the only hook ingress; retire the relay transport

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- There is one ingress path, and no test or production code posts to `/chats/hooks`.
- Held permissions still work over `HTTPHook`.
- The deadcode baseline did not grow.

---

### Task 12: Migration step 6 `chat-ledger` — legacy chats become ledger rows

**Goal:** At boot, the migration engine turns every legacy chat's records into ledger rows. The records are: conversation history, activity turns, tool calls, subagents, interruptions, choices and prompt journal files. The work:
- a deterministic, pure conversion;
- a read-only legacy reader;
- an empty-chat-only store import;
- a registered pre-open step with `Plan`, `Apply` and `Verify`.

There is no CLI: the dry run is `crowbar migrate --plan`. Until the release, the step runs on dev homes only.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage`, or any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`. Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`. Never run `crowbar migrate` against a real home: tests build their homes under `os.TempDir()`, and the test binary refuses any other home.

**Rules this task carries** (pre-flight R1, R11, R12, R14, R33, R34; the S2 pre-flight; carry C2):
- The step is number 6, `chat-ledger`, `migrate.PreOpen`, `From: 0, To: 0`.
  - It is in-layout, not the layout-1 bump: a bump would force retiring the post-open legacy steps.
  - Pre-open steps run in number order, so it runs after step 2 and before the adapter opens any store.
- The code lives in `api/internal/adapter/migrations/chatledger/` (store module), next to the reader T4 created there. `api/internal/migrate/` is not used: it would be an unassigned ratchet module and a second `migrate` package.
- A `Plan` opens no database and writes nothing. It only stats files.
- Legacy databases are opened by T4's `openLegacy`:
  - `immutable=1` normally;
  - `mode=ro` when a non-empty `-wal` sits beside the file, so rows still in the WAL are not lost.
  - Test databases are built in WAL mode.
- Paths come from `home.Home`: `h.Store()` for the databases, and `filepath.Join(h.Root(), "chats")` for the legacy prompt journal. That journal is a legacy shape and lives inside the step.
- Checkpoint labels follow the engine's convention: `rows`, `skipped_rows`, `skipped_chats`.
- Carry C2: the engine already reports progress while the step runs. Health says `migrating` with detail "Crowbar is upgrading its data (step i of n: chat-ledger)", and the desktop holds `migrating`. The web progress screen belongs to S3 T2. S1 changes no web file.

**Files:**
- Modify: `api/internal/adapter/migrations/chatledger/legacy.go` — add the row structs and DDL of `agent_turns`, `agent_tool_calls`, `agent_subagents` and `agent_choices` (columns from `repositories/chat/activity/internal/store/internal/storage/rows.go` at S1 start), and `LegacyPrompt` (the `prompt-requests/*.json` record)
- Create: `api/internal/adapter/migrations/chatledger/{read.go,chatledger.go,step.go}` and `read_test.go`, `chatledger_test.go`, `step_test.go`, `testdata/legacy-chat-a.json`, `testdata/legacy-chat-b.json`
- Modify: `api/internal/adapter/store/ledger/{store.go,store_test.go}` — `Import`, `ErrChatNotEmpty`
- Modify: `api/internal/adapter/migrations/catalog.go`:
  - `StepChatLedger = 6`;
  - `legacySteps()` gains `migrate.Step{Number: StepChatLedger, Name: "chat-ledger", Phase: migrate.PreOpen, From: 0, To: 0, Plan: chatledger.Plan}`, built directly: it has no legacy marker, so it is not built through `legacyStep`.
- Modify: `api/internal/adapter/migrations/workspace_paths.go` — `PreOpenImpls()` adds `StepChatLedger: chatledger.Impl()`
- Modify the tests that enumerate every step:
  - `api/internal/internal_test.go:118` (`step <= 5` → `step <= 6`);
  - `api/internal/bootreport_test.go:67-79`: 6 healths; `workspace-paths-table` is index 1 of 6, `chat-ledger` index 2 of 6, `project-homes` index 3 of 6; `refusals[1].Error` is "Crowbar is upgrading its data (step 2 of 6: chat-ledger)", and the last is "(step 6 of 6: hook-deliveries)".
  - Then run `grep -rnE 'StepHookDeliveries|hook-deliveries|step <= 5|of 5|Total: 5' api --include=*_test.go` and update any other list of all steps.
  - The post-open lists in `api/internal/app/container_test.go:103`, `api/internal/app/legacy_steps_internal_test.go:104` and `api/internal/adapter/migrations/workspace_paths_test.go:116` stay as they are, since step 6 is pre-open.
  - `cmd/crowbar-seed-chat/main_test.go:27` runs `PreOpenImpls()`, which now includes step 6 on an empty fixture home; it must stay green.
- Test: extend `api/internal/adapter/migrations/catalog_test.go`, `api/cmd/crowbar/migrate_test.go` and `api/tests/regression_upgrade_boot_steps_test.go` (236 lines)

**Interfaces:**

```go
package chatledger
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
type LegacyPrompt struct{ RequestID, Text, State, ProviderID string; CreatedAt, UpdatedAt time.Time } // json: requestId, text, state, providerId, createdAt, updatedAt
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
// Owner answers which chat already holds a (provider, session) binding.
type Owner func(provider, sessionID string) (chatID string, ok bool)
func ConvertChat(in LegacyChat, owner Owner) (Result, error)

// ReadLegacy reads h.Store()/agent_activity.db and agent_runner.db through openLegacy
// and <h.Root()>/chats/*/prompt-requests/*.json. Missing files are empty input.
func ReadLegacy(ctx context.Context, h home.Home) ([]LegacyChat, error)

func Plan(ctx context.Context, env migrate.Env) (migrate.Plan, error)
func Impl() migrate.Impl

// adapter/store/ledger
var ErrChatNotEmpty = errors.New("ledger: import into a chat that has rows")
// Import writes rows (Seq 1..n, each passing Check) into a chat with no rows,
// keeping their At and Seq, maintaining heads and the session index in the same
// transaction. It publishes nothing.
func (s *Store) Import(ctx context.Context, chatID string, rows []ledger.Row) error
```

**Conversion rules (exact):**
1. **Turn ids are deterministic:** `uuid.NewSHA1(uuid.NameSpaceURL, []byte("crowbar:layout1:"+chatID+":"+legacyTurnID)).String()`. Running the step twice yields identical rows.
2. **Session binding (spike D3):**
   - Per provider, the current session is the one whose newest `LegacyTurn` (by `StartedAt`, matching `SessionID`) is latest. When no turn carries a session id, it is the conversation with the latest `LastActiveAt`.
   - Emit `session_bound{cause: announced}` for every other conversation of the provider first (oldest `FirstSeenAt` first, so `Conversations()` keeps the history), then the current one last.
   - A session that `owner` reports bound to another chat becomes `marker{session_conflict, detail: id}` instead. `Import` therefore never meets `ErrSessionClaimed`.
3. **Turns:** order legacy turns by `(DisplayOrder, ItemIndex, Seq)`.
   - A `user` row opens a ledger turn: `user_message{origin: crowbar, requestId: legacy id}` when a `LegacyPrompt` with that request id exists, else `user_message{origin: provider}`; then `turn_started`.
   - A `harness` row opens a turn the same way with `user_message{origin: harness}`.
   - Each following `assistant` row (until the next `user`/`harness` row) becomes `assistant_text{messageId: legacy id, index: -1, final: true, text}` in that turn.
   - The turn ends with `turn_ended{done, effort}` or, by the last assistant row's `Status`, `turn_failed{reason: status}` / `turn_ended{interrupted}`.
   - A `notice` row in the turn makes its end `turn_failed{reason: "notice", detail: text}` instead.
   - A non-empty `Plan` becomes a `plan{op: replace}` row before the end.
4. **Tools:** `tool_call{phase: pre}` at `StartedAt` and, when `EndedAt` is set, `tool_call{phase: post|fail}` at `EndedAt`.
   - `RequestRef`/`ResultRef` are kept verbatim; the blob directory `h.Content()` is unchanged.
   - The tool's ledger turn is the one whose legacy turn id equals the tool's `TurnID`, or whose `open-…`/`msg-…` id maps to it.
   - A tool with a `SubagentID` gets `Scope` = that id. An unplaceable tool is skipped, with a reason.
5. **Subagents:** `subagent{phase: start, type}` with `Scope` = the legacy id, then:
   - when ended, `subagent{phase: stop, status: done, result: last of Messages}`;
   - when never ended, `subagent{phase: stop, status: abandoned}` at its turn's end. A resting chat has no live subagent.
6. **Interruptions:**
   - `compaction` → `marker{compaction_started}`, plus `marker{compaction_ended}` at `ResolvedAt`;
   - `stopped`, `provider_switched`, `model_changed`, `effort_changed`, `notification` → a `marker` of the same kind;
   - `inferred` → `turn_ended{interrupted, reason: inferred}` for its turn when that turn has no end; otherwise skipped;
   - `permission`/`elicitation` are covered by choices and skipped.
7. **Choices:** `ask{askId: legacy id, …, answerable: false}` at `At` (no CLI holds a legacy ask). When resolved, `ask_resolved{resolution, optionIds}` at `ResolvedAt`.
8. **Prompts:**
   - `accepted` → covered by its turn;
   - `dispatching`, `spawned`, `uncertain` → `user_message{origin: crowbar, requestId}` + `prompt_settled{consumed: false, reason: daemon_restart}`;
   - `failed` → the same with `reason: delivery_failed`;
   - `settled` → `user_message` + `prompt_settled{consumed: false, reason: not_acknowledged}`.
9. **Order:** all rows by `At`, ties by the legacy `Seq`, then by the rule order above; then `Seq` 1..n. `ledger.Check` must pass for every row, and `ledger.Fold(chatID, rows)` must leave no running turn, no pending prompt, no starting/running subagent and no live runner.

**Plan:**
- Stat `h.Store()/agent_activity.db`, `h.Store()/agent_runner.db` and `<h.Root()>/chats/*/prompt-requests`.
- None exists → an empty `migrate.Plan`.
- Otherwise one action: `{Kind: migrate.Convert, Subject: "legacy chat history → chat ledger", Count: migrate.Unknown}`, with `RewritesState: true` so the engine backs up `state/` first.

**Apply** (`Impl().Apply`; runs under the home lock):
1. `ReadLegacy(ctx, env.Home)`. No chats → return at once, creating nothing.
2. Open `filepath.Join(env.Home.Store(), ledgerstore.FileName)` with `storesqlite.OpenDB` and `ledgerstore.Open`; close it before returning.
3. Convert chats in order of their oldest legacy record (ties by chat id), so "earlier chat" is well defined and a rerun makes the same choices. For each chat:
   - `cp.Done(chatID)` → skip;
   - else `ConvertChat(chat, owner over Store.SessionOwner)` → `Store.Import`;
   - `ErrChatNotEmpty` (a dev home that ran an intermediate S1 build) → count `skipped_chats`;
   - any other error is returned, and the step halts `step_failed`;
   - then `cp.Mark(chatID, migrate.Counts{"rows": n, "skipped_rows": len(res.Skipped)})`.

**Verify:** reopen the store. For every chat Apply converted, `Store.State(chatID)` folds to rest: no running turn, no pending prompt, no live subagent, no live runner (`!HasOpenWork()`).

- [ ] **Step 1: Tests**

The fixtures:
- `legacy-chat-a.json`: two claude conversations, the older with turns; 3 user/assistant exchanges plus 1 harness row and 1 notice row; 2 tools, one nested under a subagent; 1 ended subagent and 1 unended; 1 compaction; 1 resolved permission choice; 1 `uncertain` prompt.
- `legacy-chat-b.json`: codex; untagged turns only; one `inferred` interruption; its only session already owned by another chat.

```go
func TestConvertChat_BindsTheNewestSessionThatHasTurns(t *testing.T)
func TestConvertChat_FallsBackToTheLastActiveConversation(t *testing.T)
func TestConvertChat_ASessionAnotherChatOwnsIsAConflictMarker(t *testing.T) // chat b
func TestConvertChat_TurnsBecomePromptTextAndAnEnd(t *testing.T)
func TestConvertChat_HarnessRowsKeepTheirOriginAndNoticesFailTheTurn(t *testing.T)
func TestConvertChat_ToolsKeepTheirBlobRefsAndScopes(t *testing.T)
func TestConvertChat_AnUnendedSubagentIsAbandoned(t *testing.T)
func TestConvertChat_InterruptionsAndChoicesAreNotAnswerable(t *testing.T)
func TestConvertChat_UnfinishedPromptsAreSettledNeverReplayed(t *testing.T)
func TestConvertChat_IsDeterministic(t *testing.T)            // two runs → reflect.DeepEqual
func TestConvertChat_FoldsToARestingChat(t *testing.T)        // every row passes Check; Fold is at rest
func TestReadLegacy_ReadsWithoutWriting(t *testing.T)
// temp home: WAL-mode legacy DBs from legacy.go's DDL with chat a inserted and
// checkpointed, its prompt file written; ReadLegacy equals the fixture; every file's
// mtime and size unchanged; no -wal/-shm created
func TestStore_ImportKeepsSeqAndAtAndRefusesANonEmptyChat(t *testing.T)   // adapter/store/ledger
func TestPlan_AHomeWithoutLegacyRecordsHasNothingToDo(t *testing.T)      // catalog_test.go
func TestPlan_LegacyRecordsAreOneConversionOfUnknownSize(t *testing.T)   // catalog_test.go: Convert, Unknown, RewritesState
func TestApply_ConvertsEachChatOnceAndResumesFromItsCheckpoints(t *testing.T) // step_test.go via migrations.Begin + RunPreOpen
func TestApply_AChatAlreadyInTheLedgerIsSkippedAndCounted(t *testing.T)
func TestVerify_FailsOnAChatLeftWithOpenWork(t *testing.T)
func TestMigrateCmd_PlanNamesTheChatLedgerStep(t *testing.T)            // cmd/crowbar/migrate_test.go: nothing written
// api/tests/regression_upgrade_boot_steps_test.go
func TestRegression_Upgrade_LegacyChatsBecomeLedgerRowsAtBoot(t *testing.T)
// h.crash(); write chat a's legacy rows into state/store/*.db and its prompt file;
// forgetMigrations; h2 := newHarnessAt → GET …/messages shows the legacy exchanges,
// the chat's conversation is bound to the current session, working false;
// h2.shutdown(); h3 := newHarnessAt → the same messages; migratetest.Applied(t, homeAt(t, home))[6]
// has one run with rows > 0
```

- [ ] **Step 2: Run; they FAIL.** The package, `Import` and `StepChatLedger` are missing.
- [ ] **Step 3: Implement** `legacy.go`, `read.go`, `chatledger.go`, `step.go`, `Import`, the catalog entry and the impl; update the enumerating tests.
- [ ] **Step 4: Run to green**
- `gox go test -tags noEmbed -cover ./internal/ ./internal/adapter/... ./cmd/crowbar/... ./cmd/crowbar-seed-chat/... ./internal/app/...` → PASS; `chatledger` ≥ 92%.
- `bbx 'TestRegression_Upgrade_|TestFakeProvider_'` → PASS.
- `gates` → clean. The step is reachable from `./cmd/...` through the daemon boot and `crowbar migrate --plan`.
- [ ] **Step 5: Commit**

```bash
git add -A api
git commit -m "feat(api): migration step 6 converts legacy chat records into ledger rows

Pre-open and in-layout; checkpointed per chat; the dry run is crowbar migrate --plan.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- An upgraded home shows its legacy history from the ledger after one boot, and a second boot converts nothing.
- The conversion is pure and deterministic; the reader never writes.
- No `chat-ledger` CLI exists, and `crowbar migrate --plan` names the step.

---

### Task 13: The S1 gate

**Goal:** Prove S1's gate:
- symptoms 2, 3 and 4 are closed by HTTP/WS behaviour tests;
- the chat subtree is 10k source lines and 20k test lines smaller;
- every repository gate the standing rules allow is green, and the app is verified live;
- what remains (ratchet growth, the coverage floor) is reported to the user with counts.

**Commands** (run from the repo root; define once per shell):

```bash
df -h /System/Volumes/Data   # under 8 GB free: stop and report
gox() { local T rc; T=$(mktemp -d); (cd api && env GOMODCACHE=/Users/char2cs/go/pkg/mod GOCACHE=/Users/char2cs/Library/Caches/go-build GOPATH=/Users/char2cs/go HOME=$T/home CROWBAR_HOME=$T/crowbar "$@"); rc=$?; rm -rf "$T"; return $rc; }
bbx() { gox env PATH=/usr/bin:/bin:/usr/sbin:/sbin:$(go env GOROOT)/bin go test -tags 'integration noEmbed' -count=1 -run "$1" ./tests/; }
gates() { gox make lint deadcode && gox go vet -tags noEmbed ./... && gox go vet -tags 'integration noEmbed' ./tests/... && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; }
```

Never run any of these:
- `go test ./...`, `make test-integration`, `make pr-checks`, `make test-coverage` (all run `./...`, and the integration TestMains read the real `~/.claude.json` and `~/.codex`);
- any `-tags integration` test run on `./tests/integration/...` or `./tests/kit/...`;
- the full vitest suite.

Never touch `~/.crowbar`, `~/.claude`, `~/.claude.json` or `~/.codex`. Real provider CLIs are not used.

**Files:**
- Modify: `api/ARCHITECTURE.md` (on top of the engine's and T11's edits):
  - `engine/agents` no longer holds a runner aggregate;
  - `adapter/` lists `state/store/chat_ledger.db`;
  - the types list drops `agent_activity` and `agent_runner`;
  - boot names migration step 6;
  - the "Agent turn" request flow names the ledger and the ingress;
  - `tests/` names the fake provider harness.
- Modify: `api/deadcode-baseline.txt`, only by `scripts/deadcode-check.sh --update` and only if it shrank
- Modify: `scripts/ratchet/file-size-baseline.json` and `scripts/ratchet/counters-baseline.json`, only by their `--update`, which refuses any growth

- [ ] **Step 1: Disk check.** `df -h /System/Volumes/Data`: there was about 127 GB free at S1 start; under 8 GB, stop and report. A cold Tauri build needs about 6 GB.

- [ ] **Step 2: Symptom tests**

Run: `bbx 'TestRegression_Symptom(2|3|4)_' -v 2>&1 | grep -E '^(--- |ok|FAIL)'`. If `bbx` does not pass `-v` through, run `gox env PATH=… go test -tags 'integration noEmbed' -count=1 -v -run '…' ./tests/` in the same form.
Expected: 7 `Symptom2`, 4 `Symptom3` and 6 `Symptom4` tests, each `--- PASS`, and the package `ok`. Count the PASS lines; a trailing `ok` alone proves nothing.

- [ ] **Step 3: Line counts.** Run from `api/`, and compare with the baseline at S1 start (`ec50d1548`: source 41,482, tests 72,643):

```bash
cd api
paths="internal/app/usecases/chat internal/app/repositories/chat internal/engine/agents/runner
internal/engine/agents/internal/move internal/engine/agents/internal/sessionstore
internal/engine/agents/session_locate.go internal/adapter/store/agentjournal internal/adapter/store/ledger
internal/domain/ledger internal/adapter/migrations/chatledger internal/api/v0/endpoints/chat internal/domain/chat.go
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
kit=$( [ -e tests/kit/fakeprovider.go ] && wc -l < tests/kit/fakeprovider.go || echo 0 )
echo "chat subtree: src=$src test=$((tst+bt+kit))"
```

Expected: `src ≤ 31482` and `test ≤ 52643`. If either misses, the gate fails: rerun the loop printing each path, report the gap per directory, and stop. Do not move code out of the measured paths to pass.

- [ ] **Step 4: Repository gates**
- Run: `gates` → clean. Then `gox make descriptor-check`.
- Run: `bbx '.'`: the whole black-box package once, under the stripped PATH → PASS.
- Coverage: run `gox go test -tags noEmbed -count=1 -cover` over every package S1 created or touched. Record each number: every new package must be ≥ 92%. Report the list. The module floor (`make test-coverage`) is CI's.
- Ratchet: run `make ratchet` at the repo root. It is read-only without `ARGS`.
  - `file-size` and `counters` must pass. Then write their lower baselines with `node scripts/ratchet/file-size.mjs --update` and `node scripts/ratchet/counters.mjs --update`.
  - `loc-budgets` already failed at S1 start on the engine's growth. Record every `grew` key with its baseline and current value, the S1 delta and the engine delta separately. Include the web keys: the engine added +91 `web.total` and +217 `web.tests`; S1 adds none.
  - Do not raise a budget (pre-flight R7, provisional). Put the list in the task report for the user.
- Run: `git diff ec50d1548 --stat -- web/` → empty. S1 changed no client file; the engine's web changes predate S1.
- Run: `grep -rnE 'claude|codex|anthropic|openai' api/internal --include=*.go | grep -v _test.go | grep -v '/testdata/'`, and compare with the same `git grep` at `ec50d1548`. Nothing new: no provider vocabulary was added to Go.

- [ ] **Step 5: Live check in Tauri (isolated home)**
- Setup:
  - Run `df` first.
  - `make dev-desktop` from the worktree root, with `CROWBAR_HOME` unset or set to `<worktree>/.crowbar`. The Makefile defaults it to the worktree's own `.crowbar`; never the default home. `web/node_modules` and `desktop/src-tauri/target` exist at S1 start.
  - The dev daemon runs migration step 6 on that dev home at boot. Confirm in its log, or with `crowbar migrate --plan` against the dev home only, that the step applied, and that any chats the dev home already had still show their history.
  - Install the fake provider descriptor into the dev home as `<dev home>/descriptors/claude.yaml`, built as `kit.InstallFakeProvider` builds it, with `FAKECLI_DIR` a scratch directory.
- With the Tauri MCP:
  - open a scratch repo project, create a chat, send a prompt, and read the capture for the runner id and token;
  - post `user_prompt`, two `message_delta` chunks, a `tool_pre`/`tool_post` pair and `turn_stop` to the dev hook server (`<dev home>/run/hooks.url`) with curl;
  - confirm in the window that the prompt echo, the streaming text, the tool row and the final reply render, and that the spinner stops;
  - click Stop on a running turn and confirm the spinner stops at once;
  - quit and relaunch the dev app; confirm the transcript and the dormant state are intact;
  - send a prompt and confirm the capture shows `--resume <same id>`.
- Real provider CLIs are not used.

- [ ] **Step 6: Commit**

```bash
git add api/ARCHITECTURE.md api/deadcode-baseline.txt scripts/ratchet/file-size-baseline.json scripts/ratchet/counters-baseline.json
git commit -m "docs(api): describe the chat ledger architecture after S1

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:**
- All 17 symptom tests pass, and both line budgets are met.
- `gates`, `make descriptor-check` and the black-box package pass.
- The coverage list and the `loc-budgets` growth list are in the report for the user.
- The live check passes, and no web file changed since `ec50d1548`.
