# S2a — Daemon contract for the ChatStore: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The feed gives the client what it needs to drop every REST read: deltas that carry only new text, a snapshot of live text after `ready`, asks that say whether Crowbar can answer them, and tool payloads readable by blob ref (T1). The daemon gains the `retract` verb that turns Esc-after-send into a fact (T2).

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (R3–R6). Requires all of S1.

**Tech Stack:** Go, gin, gorilla/websocket, testify, `api/tests` with `kit.FakeProvider`.

**Spec:** report §2 (symptoms 5, 7), §4.2 (outbox diagram); additions §4 F, H; architecture §4 ("The one stream"); S1 T7 (mapper, live buffer, blobs), T10 (feed), R10 (Stop).

Before any build: `df -h /System/Volumes/Data`; under 8 GB free, stop and report.

---

### Task 1: Feed contract for the client (R3, R4, R5)

**Goal:** A delta frame carries only the text that is new since the previous frame of its stream; a client attaching mid-stream gets one snapshot per live stream right after `ready`; every `ask` row says whether Crowbar holds the hook response; tool inputs and results are readable by their blob ref. Closes the wire half of symptom 7 (each delta re-sent the whole message, O(L²) bytes per reply).

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/ingress/live.go` and `live_internal_test.go` — the live buffer keeps, per (turn, scope, id, stream), the text so far and a counter `n`; each provider delta becomes one `FeedDelta` with the suffix (or `Replace` when the new text does not extend the old); `Snapshot(chatID) []FeedDelta` returns one `Replace` frame per live stream; an entry is dropped when its complete row lands or its turn closes (as S1 does today)
- Modify: `api/internal/app/usecases/chat/feed.go` and `feed_internal_test.go` — the new `FeedDelta`; after `ready`, `OpenFeed` sends `Snapshot(chatID)` taken after the subscriber was registered
- Modify: `api/internal/domain/ledger/ask.go` and `ask_test.go` — `AskBody.Answerable bool` (`json:"answerable"`)
- Modify: `api/internal/app/usecases/chat/internal/ingress/rows.go` and `rows_test.go` — set `Answerable` from the same decision that sets `fx.HoldAsk`
- Create: `api/internal/api/v0/endpoints/chat/handlers/blobs.go` and `blobs_test.go`
- Modify: `api/internal/app/usecases/chat/assembly.go` (or the file holding `OpenFeed`'s siblings) — `ReadBlob`
- Modify: `api/internal/api/v0/router.go` — `chats.GET("/:chatId/blobs/:ref", …)` beside the feed route (the flat `chats` group, not `chatScoped`)
- Test (black box): extend `api/tests/chat_feed_test.go`; create `api/tests/regression_chat_feed_delta_test.go`

**Interfaces:**

```go
package chat

type FeedDelta struct {
	ID      string `json:"id"` // message id (answer, reasoning) or tool id (tool_output, diff)
	TurnID  string `json:"turnId"`
	Scope   string `json:"scope,omitempty"`
	Stream  string `json:"stream"` // "answer" | "reasoning" | "tool_output" | "diff"
	N       int    `json:"n"`      // 1-based position of this frame in its stream
	Text    string `json:"text"`   // the new suffix; everything so far when Replace
	Replace bool   `json:"replace,omitempty"`
}

// ReadBlob returns a content-addressed payload a row of chatID refers to.
// apperr.ErrNotFound for an unknown chat or a swept/unknown ref;
// apperr invalid-argument for a ref that is not 64 lowercase hex characters.
func (u *Usecase) ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)
```

Handler `GET /v0/chats/:chatId/blobs/:ref`: 200 with the bytes (`Content-Type: text/plain; charset=utf-8`), 400 for a malformed ref, 404 otherwise. Delete the field `MessageID` and the cumulative-text comment of S1's `FeedDelta` in the same change (no alias).

- [ ] **Step 1: Failing tests**

```go
// ingress/live_internal_test.go
func TestLive_EachDeltaCarriesOnlyTheNewSuffix(t *testing.T)
// provider deltas "a","ab","abc" for one message → FeedDelta{N:1,"a"}, {N:2,"b"}, {N:3,"c"}
func TestLive_ARewriteThatDoesNotExtendIsAReplace(t *testing.T)
// "abc" then "xy" → {N:4, Text:"xy", Replace:true}
func TestLive_SnapshotHasOneReplacePerLiveStream(t *testing.T)
// two messages + one tool_output live → 3 frames, Replace, N = each stream's current n
func TestLive_ACompleteRowDropsItsStreamFromTheSnapshot(t *testing.T)

// feed_internal_test.go
func TestFeeds_ReadyIsFollowedBySnapshotsThenLiveFrames(t *testing.T)
func TestFeeds_ADeltaRacingTheSnapshotNeverDuplicatesText(t *testing.T)
// a delta published between registration and snapshot: the client rule
// (append only when n == last+1, Replace sets last = n) yields the exact text

// ingress/rows_test.go
func TestMap_AHeldAskIsAnswerable(t *testing.T)
func TestMap_AnAskCrowbarCannotHoldIsNotAnswerable(t *testing.T)

// handlers/blobs_test.go
func TestBlobs_ReturnsTheBytesForAKnownRef(t *testing.T)
func TestBlobs_MalformedRefIs400(t *testing.T)
func TestBlobs_UnknownRefIs404(t *testing.T)
func TestBlobs_UnknownChatIs404(t *testing.T)

// api/tests/regression_chat_feed_delta_test.go (stub fake whose message_delta maps no index)
func TestRegression_Symptom7_DeltaFramesCarryOnlyNewText(t *testing.T)
// post cumulative deltas of a 2,000-character reply in 100 steps; sum of delta
// frame text lengths == 2,000 (today ≈ 101,000)
func TestRegression_Symptom7_AMidStreamDialGetsOneSnapshotThenSuffixes(t *testing.T)

// api/tests/chat_feed_test.go
func TestChatFeed_AskRowsSayWhetherCrowbarCanAnswer(t *testing.T)
// claude fake PermissionRequest → ask row answerable:true; after the CLI hangs up → ask_resolved{proceeded}
func TestChatFeed_ToolPayloadIsReadableByItsRef(t *testing.T)
// replay tool_pre with input → tool_call row inputRef r → GET /v0/chats/<id>/blobs/r == the input JSON
```

- [ ] **Step 2: Run; they FAIL** (`N`, `Replace`, `Answerable`, `ReadBlob` undefined).
- [ ] **Step 3: Implement** the buffer counters and suffixes, the snapshot after `ready`, the `Answerable` field, `ReadBlob` (reads S1's `adapter/store/ledger/blobs.go` by ref after checking the chat exists), the handler and route.
- [ ] **Step 4: Run to green**

```bash
cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/ledger/... ./internal/api/...
cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestChatFeed_|TestRegression_Symptom7_|TestRegression_AGrowingMessage' ./tests/
make lint && make deadcode && (cd api && go vet -tags noEmbed ./...)
```

- [ ] **Step 5: Commit**

```bash
git add api
git commit -m "feat(api): feed deltas carry only new text; asks say answerable; blobs by ref

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** a reply of length L costs O(L) bytes on the feed; a client attaching mid-stream shows the text so far without a REST read; an ask row alone decides whether its card has answer buttons; nothing the client needs is reachable only through `/activity`.

---

### Task 2: `retract` — Esc after send is a fact (R6)

**Goal:** `POST …/chats/:id/prompts/:requestId/retract` withdraws a prompt. Before delivery the prompt settles `retracted` and its text comes back; after delivery its running turn ends interrupted and is signalled like Stop; a settled prompt is a 409. Closes the daemon half of symptom 5 ("the queue has no cancel in-flight → back to draft transition").

**Files:**
- Create: `api/internal/app/usecases/chat/internal/runner/retract.go` and `retract_internal_test.go`
- Modify: `api/internal/app/usecases/chat/runner.go` (the facade) — `Retract`
- Modify: `api/internal/domain/ledger/` — the reason constant `ReasonRetracted = "retracted"` beside the existing `prompt_settled`/`turn_ended` reasons (find them with `grep -rn 'stopped' api/internal/domain/ledger/*.go`); delete the unwritten `TurnRetracted` status and its fold cases
- Modify: `api/internal/domain/` — `AgentExitRetracted` beside the other `AgentExit*` values
- Create: `api/internal/api/v0/endpoints/chat/handlers/retract.go` and `retract_test.go`
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` and `routes_test.go`, `api/internal/api/v0/endpoints/home/routes.go` and its routes test — `POST /chats/:id/prompts/:requestId/retract` beside `/stop`
- Modify: `api/internal/api/v0/dto/agent.go` — `AgentRetractDTO{State string \`json:"state"\`; Text string \`json:"text"\`}`
- Test (black box): create `api/tests/regression_agent_retract_test.go`

**Interfaces:**

```go
package runner

type RetractResult struct {
	State string // "retracted" | "interrupted"
	Text  string // the prompt's text, from its user_message row
}

// Retract withdraws requestID in chatID. Pending (a user_message with no
// turn_started and no prompt_settled): appends prompt_settled{consumed:false,
// reason: retracted}; when that prompt was delivered by restarting the CLI, the
// runner it started is terminated gracefully and exits with AgentExitRetracted.
// Delivered with its turn running: appends turn_ended{interrupted, reason:
// retracted}, then signals the provider exactly as Stop does. Settled, or its
// turn closed: apperr conflict. Unknown requestID: apperr not found.
func (s *Service) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error)
```

`Service` stands for the type `stop.go`'s `Stop` is a method of; `Retract` sits beside it.

Order inside `Retract`: decide from the chat's `ledger.State` under its writer, append the row, then act on the runner (terminate or signal) without waiting for confirmation (S1 R10). A later `UserPromptSubmit` for the retracted text is mapped as S1 maps any unmatched provider prompt: a provider-origin `user_message` and its `turn_started`. The handler maps conflict → 409 and not-found → 404, and answers `AgentRetractDTO`.

- [ ] **Step 1: Failing tests**

```go
// runner/retract_internal_test.go
func TestRetract_APendingPromptSettlesRetractedAndReturnsItsText(t *testing.T)
func TestRetract_ARestartDeliveredPromptTerminatesTheRunnerItStarted(t *testing.T)
func TestRetract_ASteerDeliveredPromptLeavesTheRunnerRunning(t *testing.T)
func TestRetract_ARunningTurnEndsInterruptedBeforeTheProviderIsSignalled(t *testing.T)
// fake signaller records the State's last row at the moment it is called: turn_ended{interrupted, retracted}
func TestRetract_ASettledPromptIsAConflict(t *testing.T)
func TestRetract_AnUnknownRequestIsNotFound(t *testing.T)

// handlers/retract_test.go
func TestRetract_RespondsWithStateAndText(t *testing.T)
func TestRetract_ConflictIs409AndUnknownIs404(t *testing.T)

// api/tests/regression_agent_retract_test.go (claude fake)
func TestRegression_Symptom5_RetractBeforeDeliveryReturnsTheText(t *testing.T)
// send r1; before any hook, POST retract → 200 {retracted, text}; feed rows end with
// prompt_settled{r1, consumed:false, reason:retracted}; the fake CLI launched for r1 exits;
// runner_exited{reason: retracted}
func TestRegression_Symptom5_TheNextPromptIsAcceptedAfterARetract(t *testing.T)   // no ErrPromptBusy
func TestRegression_Symptom5_RetractAfterDeliveryInterruptsTheTurn(t *testing.T)
// post user_prompt (turn_started) → retract → {interrupted, text}; turn_ended{interrupted, retracted}
func TestRegression_Symptom5_RetractingASettledPromptIs409(t *testing.T)
func TestRegression_Symptom5_ALateAckAfterRetractIsRecordedAsAProviderTurn(t *testing.T)
// retract r1 pending, then post user_prompt with r1's text → user_message{origin: provider}
// + turn_started; r1's prompt_settled is unchanged
```

- [ ] **Step 2: Run; they FAIL** (route and `Retract` missing).
- [ ] **Step 3: Implement** `Retract`, the reason and exit constants, the handler, the routes and the DTO; delete `TurnRetracted`.
- [ ] **Step 4: Run to green**

```bash
cd api && go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/... ./internal/api/...
cd api && go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom5_|TestRegression_Symptom4_|Stop' ./tests/
make lint && make deadcode && (cd api && go vet -tags noEmbed ./...)
```

- [ ] **Step 5: Commit**

```bash
git add api
git commit -m "feat(api): retract withdraws a prompt before delivery and interrupts after

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** every state of the report §4.2 diagram that leads back to `draft` has a row and a verb; a retracted chat accepts the next prompt at once; nothing in the path waits on a clock.
