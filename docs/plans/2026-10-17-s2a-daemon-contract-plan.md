# S2a — Daemon contract for the ChatStore: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tool payloads are readable by blob ref, and a send the daemon refuses because the session is lost says so with a typed code (T1). The daemon gains the `retract` verb that turns Esc-after-send into a fact (T2). S1 already shipped the rest of the client contract (P3): append-only numbered deltas, the post-`ready` snapshot, and `answerable` on asks.

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (R3–R6, R27). Requires all of S1, including its final-review fix dispatch.

**Tech Stack:** Go, gin, gorilla/websocket, testify, `api/tests` (bbx) with `kit.InstallFakeProvider` and HTTP hooks.

**Spec:** report §2 (symptoms 5, 7), §4.2 (outbox diagram); additions §4 F, H; architecture §4 ("The one stream"); S1 T7 (mapper, blobs), T10 (feed), R10 (Stop); S1 ledger C24 / C27.

Before any build: free disk on the worktree's filesystem (`df -h .`); under 8 GB free, stop and report. Go tests always run with `T=$(mktemp -d)` and `HOME=$T/home CROWBAR_HOME=` (index, Global Constraints).

---

### Task 1: Blobs by ref, and the typed session-lost send error (R5, R27)

**Goal:** Tool inputs and results are readable by their blob ref, so `/activity/:toolId/payload` can go. A prompt the daemon refuses because the chat's session is lost answers 409 with code `session_lost` (today: `request_outcome_uncertain`, whose words tell the user the provider "may already have this message"), and so do resume and switch when the provider refuses the session.

Already done by S1 and **not** part of this task (verify, then skip): `FeedDelta{ID, TurnID, Scope, Stream, N, Text, Replace}` and `Recorder.Snapshot` (`api/internal/app/usecases/chat/internal/ingress/live.go`), the snapshot after `ready` (`api/internal/app/usecases/chat/feed.go` `attachFeed`), `ledger.Ask.Answerable` (`api/internal/domain/ledger/ask.go`). `TestRegression_Symptom7_*` delta tests, if absent from `api/tests/chat_feed_test.go`, are added here as characterization tests (they pass on arrival; label them so in the commit body).

**Files:**
- Create: `api/internal/api/v0/endpoints/chat/handlers/blobs.go` and `blobs_test.go`
- Modify: `api/internal/app/usecases/chat/feed.go` (beside `OpenFeed`) — `ReadBlob`, over `adapter/store/ledger.Blobs.Get(ref)` after `u.chats.GetChat` (wire the `*Blobs` through `assembly.go` if the usecase does not hold it yet; a required constructor argument, never a nil-checked setter)
- Modify: `api/internal/api/v0/router.go` — `chats.GET("/:chatId/blobs/:ref", …)` beside the feed route (the flat `chats` group, not `chatScoped`)
- Modify: `api/internal/app/usecases/chat/internal/runner/errors.go` — `ErrPromptSessionLost = fmt.Errorf("agent: the provider no longer has this chat's session: %w", apperr.ErrConflict)`
- Modify: `api/internal/app/usecases/chat/internal/runner/prompts.go` — where a send ends with its prompt settled `reason: session_lost` (or the carrying spawn returned `ErrSpawnSessionRefused`), return `ErrPromptSessionLost` instead of `uncertainPromptOutcome(…)`; a send whose outcome truly is unknown keeps `ErrPromptOutcomeUnknown`
- Modify: `api/internal/app/usecases/chat/runner.go` — re-export `ErrPromptSessionLost`; `PromptCodeSessionLost = "session_lost"`; `PromptErrorCode` maps both `ErrPromptSessionLost` and `ErrSpawnSessionRefused` to it (checked before the generic conflict cases)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/switch.go` — `Resume` answers through `writeCodedErr(ctx, err, agentusecase.PromptErrorCode(err))`, as `Switch` does
- Test (black box): extend `api/tests/chat_feed_test.go` (blob read) and `api/tests/regression_chat_session_test.go` (session lost; 314 lines, stays ≤500)

**Interfaces:**

```go
package chat

// ReadBlob returns a content-addressed payload a row of chatID refers to.
// apperr.ErrNotFound for an unknown chat or a swept/unknown ref;
// apperr invalid-argument for a ref that is not 64 lowercase hex characters.
func (u *Usecase) ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)
```

Handler `GET /v0/chats/:chatId/blobs/:ref`: 200 with the bytes (`Content-Type: text/plain; charset=utf-8`), 400 for a malformed ref, 404 otherwise.

- [ ] **Step 1: Failing tests**

```go
// handlers/blobs_test.go
func TestBlobs_ReturnsTheBytesForAKnownRef(t *testing.T)
func TestBlobs_MalformedRefIs400(t *testing.T)
func TestBlobs_UnknownRefIs404(t *testing.T)
func TestBlobs_UnknownChatIs404(t *testing.T)

// chat runner.go's test file (runner_test.go)
func TestPromptErrorCode_ALostSessionIsSessionLost(t *testing.T)   // ErrPromptSessionLost and ErrSpawnSessionRefused → "session_lost"

// api/tests/chat_feed_test.go
func TestChatFeed_ToolPayloadIsReadableByItsRef(t *testing.T)
// post a pre-tool hook with an input → the feed's tool_call row has inputRef r → GET /v0/chats/<id>/blobs/r == the input JSON

// api/tests/regression_chat_session_test.go (fake CLI: create <dir>/refuse-<session> as the existing Symptom2 tests do)
func TestRegression_C27_ASendRefusedForALostSessionIsTypedSessionLost(t *testing.T)
// a send whose carrying launch is refused → 409 {code: "session_lost"}; the feed has prompt_settled{reason: session_lost}; never request_outcome_uncertain
func TestRegression_C27_AResumeRefusedForALostSessionIsTypedSessionLost(t *testing.T)
// POST /resume on that chat → 409 {code: "session_lost"}
```

- [ ] **Step 2: Run; they FAIL** (`ReadBlob`, the route and `session_lost` missing; the send answers `request_outcome_uncertain`).
- [ ] **Step 3: Implement** `ReadBlob`, the handler and route; the error, its code and the resume mapping.
- [ ] **Step 4: Run to green**

```bash
T=$(mktemp -d); cd api
HOME=$T/home CROWBAR_HOME= go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/ledger/... ./internal/api/...
HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -count=1 -run 'TestChatFeed_|TestRegression_C27_|TestRegression_Symptom2_' ./tests/
golangci-lint run --build-tags noEmbed ./... && scripts/deadcode-check.sh && go vet -tags noEmbed ./...
cd .. && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; rm -rf $T
```

- [ ] **Step 5: Commit** (stage by explicit path; body: go.* loc deltas per R1)

```bash
git add api/internal/api/v0/endpoints/chat/handlers/blobs.go api/internal/api/v0/endpoints/chat/handlers/blobs_test.go \
  api/internal/api/v0/router.go api/internal/app/usecases/chat/feed.go api/internal/app/usecases/chat/runner.go \
  api/internal/app/usecases/chat/internal/runner/errors.go api/internal/app/usecases/chat/internal/runner/prompts.go \
  api/internal/api/v0/endpoints/chat/handlers/switch.go api/tests/chat_feed_test.go api/tests/regression_chat_session_test.go  # + any test file touched
git commit -m "feat(api): blobs by ref; a send refused for a lost session says session_lost

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

**Acceptance:** nothing the client needs is reachable only through `/activity`; every refusal caused by a lost session carries `session_lost`, and `request_outcome_uncertain` is left only for a delivery Crowbar truly cannot confirm.

---

### Task 2: `retract` — Esc after send is a fact (R6)

**Goal:** `POST …/chats/:id/prompts/:requestId/retract` withdraws a prompt. Before delivery the prompt settles `retracted` and its text comes back; after delivery its running turn ends interrupted and is signalled like Stop; a settled prompt is a 409. Closes the daemon half of symptom 5 ("the queue has no cancel in-flight → back to draft transition").

**Files:**
- Create: `api/internal/app/usecases/chat/internal/runner/retract.go` and `retract_internal_test.go`
- Modify: `api/internal/app/usecases/chat/runner.go` (the facade) — `Retract`
- Modify: `api/internal/domain/ledger/prompt.go` and `turn.go` — the reason constant `ReasonRetracted = "retracted"` beside the existing prompt-settle reasons (`ReasonSessionLost`, …) and turn-end reasons (`ReasonStopped`, …); one constant, declared once, used by both rows. There is no `TurnRetracted` to delete (P3)
- Modify: `api/internal/domain/agent_session.go` — `AgentExitRetracted` beside the other `AgentExit*` values
- Create: `api/internal/api/v0/endpoints/chat/handlers/retract.go` and `retract_test.go`
- Modify: `api/internal/api/v0/endpoints/chat/routes.go` and `routes_test.go` (`repoScoped.POST("/chats/:id/stop", …)` is the anchor), `api/internal/api/v0/endpoints/home/routes.go` and `routes_test.go` (`home.POST("/chats/:id/stop", h.RequireHomeWorkspace, ah.Stop)`) — `POST /chats/:id/prompts/:requestId/retract` beside `/stop`
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
func (rs *Runners) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error)
```

`Retract` sits beside `(*Runners).StopChat` (`runner/stop.go`); the facade `(*Usecase).Retract` sits beside `(*Usecase).StopChat` in `usecases/chat/runner.go`.

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

// api/tests/regression_agent_retract_test.go (fake CLI over the shipped claude descriptor; hooks posted over HTTP)
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
- [ ] **Step 3: Implement** `Retract`, the reason and exit constants, the handler, the routes and the DTO.
- [ ] **Step 4: Run to green**

```bash
T=$(mktemp -d); cd api
HOME=$T/home CROWBAR_HOME= go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/... ./internal/api/...
HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom5_|TestRegression_Symptom4_|Stop' ./tests/
golangci-lint run --build-tags noEmbed ./... && scripts/deadcode-check.sh && go vet -tags noEmbed ./...
cd .. && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; rm -rf $T
```

- [ ] **Step 5: Commit** (stage every created and modified path by name; body: go.* loc deltas per R1)

```bash
git add <the paths in Files>
git commit -m "feat(api): retract withdraws a prompt before delivery and interrupts after

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

**Acceptance:** every state of the report §4.2 diagram that leads back to `draft` has a row and a verb; a retracted chat accepts the next prompt at once; nothing in the path waits on a clock.
