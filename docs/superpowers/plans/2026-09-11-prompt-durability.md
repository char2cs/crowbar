# Prompt Durability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the one verified remaining gap from `docs/superpowers/specs/2026-09-11-turn-lifecycle-rewrite-design.md`: a queued prompt's actual text is durably recoverable from the backend, not just its hash, so a lost frontend copy (idle tab, crash, cleared storage) can be shown back to the user instead of vanishing.

**Architecture:** The prompt journal (`agentjournal.PromptRequest`) gains a `Text` field written at the same already-correctly-timed `Begin()` call. A new read path (`PromptRequests.LatestRequest` → `Runners.PendingPrompt` usecase method → HTTP handler → DTO) exposes it. The frontend calls it once when a chat becomes visible and, if the backend has an unresolved delivery the local queue doesn't know about, seeds a recovered row using the existing `outcome_uncertain` state and retry affordances — no new frontend state machine.

**Tech Stack:** Go (gin, testify), TypeScript/React (vitest), the existing `agentjournal` filesystem journal.

**Spec:** `docs/superpowers/specs/2026-09-11-turn-lifecycle-rewrite-design.md` (§3, §6)

## Global Constraints

- No provider-specific code in Go — this feature is fully provider-agnostic already (project-wide law; not directly implicated here, but no task may introduce a codex/claude branch).
- Test files: Go tests live beside their source (`<source>_test.go`); TS tests live under `web/src/__tests__/` mirroring `web/src/` (per `CLAUDE.md`).
- Every backend bug/gap gets a `TestRegression_*` per project convention; this plan's Task 7 is that test.
- Never run the full `vitest run` — targeted/modified-file tests only, per project convention.
- Verify live via Tauri MCP (real spawned `codex`/`claude`), never screen recording — Task 8.

---

### Task 1: Add `Text` to the prompt journal and thread it through `Begin`

**Files:**
- Modify: `api/internal/adapter/store/agentjournal/prompt_requests.go:46-56` (struct), `:75-83` (interface), `:227-273` (`Begin` impl)
- Modify: `api/internal/app/usecases/chat/internal/runner/prompts.go:75-77`, `:175-177` (both call sites)
- Test: `api/internal/adapter/store/agentjournal/prompt_requests_test.go` (27 existing `.Begin(` call sites gain one more argument)

**Interfaces:**
- Produces: `PromptRequest.Text string` (JSON tag `"text"`); `PromptRequests.Begin(dir, requestID, text, textHash, providerID, outgoingRunnerID, replacementRunnerID string, now time.Time) (PromptRequest, bool, error)` — `text` inserted as the third parameter, immediately after `requestID` and before the existing `textHash`.

- [ ] **Step 1: Write the failing test**

Add to `api/internal/adapter/store/agentjournal/prompt_requests_test.go`, right after `TestJournal_BeginRecordsADispatchingIntent`:

```go
func TestJournal_BeginStoresTheLiteralPromptText(t *testing.T) {
	j, dir := journal(t)

	record, _, err := j.Begin(dir, "req-1", "please rename this function", "hash", "claude", "runner-out", "runner-new", jnow)

	require.NoError(t, err)
	assert.Equal(t, "please rename this function", record.Text)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/adapter/store/agentjournal/... -run TestJournal_BeginStoresTheLiteralPromptText -v`
Expected: FAIL to compile — `Begin` does not accept this many arguments yet.

- [ ] **Step 3: Add the field and thread the parameter**

In `prompt_requests.go`, add the field to the struct (right after `RequestID`):

```go
type PromptRequest struct {
	RequestID         string    `json:"requestId"`
	Text              string    `json:"text"`
	TextHash          string    `json:"textHash"`
	State             string    `json:"state"`
	ProviderID        string    `json:"providerId"`
	OutgoingRunnerID  string    `json:"outgoingRunnerId,omitempty"`
	RunnerID          string    `json:"runnerId,omitempty"`
	TerminalSessionID string    `json:"terminalSessionId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
```

Update the interface method doc and signature:

```go
	// Begin records a dispatching intent for requestID, creating the journal
	// directory if needed. text is the literal prompt, stored so a lost
	// frontend copy can be recovered later (see LatestRequest) — it is never
	// used for matching, only textHash is. It reports whether an ATTEMPT for
	// this id already existed (in which case the caller must classify it
	// rather than dispatch), and refuses with ErrPromptBusy,
	// ErrPromptOutcomeUnknown or ErrPromptRequestIDConflict when the journal
	// already owes an answer.
	Begin(
		dir string,
		requestID string,
		text string,
		textHash string,
		providerID string,
		outgoingRunnerID string,
		replacementRunnerID string,
		now time.Time,
	) (PromptRequest, bool, error)
```

Update the implementation signature and the record it builds:

```go
func (s *promptRequests) Begin(
	dir string,
	requestID string,
	text string,
	textHash string,
	providerID string,
	outgoingRunnerID string,
	replacementRunnerID string,
	now time.Time,
) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(dir); err != nil {
		return PromptRequest{}, false, err
	}
	existing, found, err := readPromptRequest(dir, requestID)
	if err != nil {
		return PromptRequest{}, false, err
	}
	if found {
		reused, done, reuseErr := reusablePromptRequest(existing, textHash)
		if done || reuseErr != nil {
			return reused, done, reuseErr
		}
	}

	if err := recoverOrphanedDispatchesLocked(dir, now, s.write); err != nil {
		return PromptRequest{}, false, err
	}
	if err := requireNoActiveRequest(dir, requestID); err != nil {
		return PromptRequest{}, false, err
	}
	record := PromptRequest{
		RequestID:        requestID,
		Text:              text,
		TextHash:         textHash,
		State:            PromptStateDispatching,
		ProviderID:       providerID,
		OutgoingRunnerID: outgoingRunnerID,
		RunnerID:         replacementRunnerID,
		CreatedAt:        now.UTC(),
		UpdatedAt:        now.UTC(),
	}
	if err := s.write(dir, record); err != nil {
		return PromptRequest{}, false, err
	}
	return record, false, nil
}
```

Update both production call sites in `api/internal/app/usecases/chat/internal/runner/prompts.go`:

Line ~75 (inside `SubmitPrompt`):
```go
	prior, existingAttempt, err := rs.prompts.Begin(
		journalDir, clientRequestID, text, textHash, live.ProviderID, live.ID, replacementRunnerID, time.Now(),
	)
```

Line ~175 (inside `submitPromptOverAPI`):
```go
	prior, existingAttempt, err := rs.prompts.Begin(
		journalDir, clientRequestID, text, textHash, live.ProviderID, live.ID, live.ID, time.Now(),
	)
```

(Both functions already have `text` as their own parameter — see `prompts.go:25` for `SubmitPrompt` and `prompts.go:169` for `submitPromptOverAPI`; no new plumbing needed, just passing the value already in scope.)

- [ ] **Step 4: Fix the remaining 26 test call sites by following the compiler**

Run: `cd api && go vet ./internal/adapter/store/agentjournal/...`

Each reported error names a `.Begin(dir, "req-...", ...)` call missing the new argument. For every one, insert a `text` argument immediately after the request-id argument and before the existing hash argument. Use `""` for any pre-existing test that doesn't assert on `Text` (the value is irrelevant to what that test checks — an empty string is a legitimate, real test input, not a placeholder). For example, `TestJournal_BeginRecordsADispatchingIntent` becomes:

```go
	record, existing, err := j.Begin(dir, "req-1", "", "hash", "claude", "runner-out", "runner-new", jnow)
```

Repeat for every remaining call site `go vet` reports, until it's clean.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test ./internal/adapter/store/agentjournal/... -v`
Expected: PASS, including the new `TestJournal_BeginStoresTheLiteralPromptText`.

- [ ] **Step 6: Build the rest of the tree that calls `Begin`**

Run: `cd api && go build ./internal/... && go vet ./internal/...`
Expected: clean (the two production call sites were already fixed in Step 3).

- [ ] **Step 7: Commit**

```bash
git add api/internal/adapter/store/agentjournal/prompt_requests.go api/internal/adapter/store/agentjournal/prompt_requests_test.go api/internal/app/usecases/chat/internal/runner/prompts.go
git commit -m "feat(agent): store a prompt request's literal text, not just its hash"
```

---

### Task 2: Add `PromptRequests.LatestRequest`

**Files:**
- Modify: `api/internal/adapter/store/agentjournal/prompt_requests.go` (interface + impl)
- Test: `api/internal/adapter/store/agentjournal/prompt_requests_test.go`

**Interfaces:**
- Consumes: `readPromptRequests(dir string) ([]PromptRequest, error)` (existing, `prompt_requests.go:588`).
- Produces: `PromptRequests.LatestRequest(dir string) (PromptRequest, bool, error)` — the single most-recently-updated record in the journal directory, regardless of state; `found=false` when the directory has no records.

- [ ] **Step 1: Write the failing test**

Add to `prompt_requests_test.go`:

```go
func TestJournal_LatestRequestReturnsTheMostRecentlyUpdatedRecord(t *testing.T) {
	j, dir := journal(t)

	_, _, err := j.Begin(dir, "req-1", "first", "hash-1", "claude", "out", "new-1", jnow)
	require.NoError(t, err)
	_, err = j.MarkAccepted(dir, "req-1", jnow)
	require.NoError(t, err)

	later := jnow.Add(time.Second)
	_, _, err = j.Begin(dir, "req-2", "second", "hash-2", "claude", "out", "new-2", later)
	require.NoError(t, err)

	latest, found, err := j.LatestRequest(dir)

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "req-2", latest.RequestID)
	assert.Equal(t, "second", latest.Text)
}

func TestJournal_LatestRequestFindsNothingInAnEmptyJournal(t *testing.T) {
	j, dir := journal(t)

	_, found, err := j.LatestRequest(dir)

	require.NoError(t, err)
	assert.False(t, found)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/adapter/store/agentjournal/... -run TestJournal_LatestRequest -v`
Expected: FAIL to compile — `LatestRequest` does not exist yet.

- [ ] **Step 3: Add the interface method and implementation**

In `prompt_requests.go`, add to the `PromptRequests` interface, right after `ActiveDelivery`:

```go
	// LatestRequest returns the journal's most recently updated record, if
	// any, regardless of its state — unlike ActiveDelivery, which only
	// surfaces a record still genuinely in flight. Its caller decides what a
	// given state means for their own purpose (see Runners.PendingPrompt).
	LatestRequest(
		dir string,
	) (PromptRequest, bool, error)
```

Add the implementation, right after `ActiveDelivery`'s implementation:

```go
func (s *promptRequests) LatestRequest(dir string) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := readPromptRequests(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PromptRequest{}, false, nil
		}
		return PromptRequest{}, false, err
	}
	var latest PromptRequest
	found := false
	for _, record := range records {
		if !found || record.UpdatedAt.After(latest.UpdatedAt) {
			latest = record
			found = true
		}
	}
	return latest, found, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/adapter/store/agentjournal/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/adapter/store/agentjournal/prompt_requests.go api/internal/adapter/store/agentjournal/prompt_requests_test.go
git commit -m "feat(agent): let the prompt journal report its most recent record"
```

---

### Task 3: Add the `Runners.PendingPrompt` usecase method

**Files:**
- Create: `api/internal/app/usecases/chat/internal/runner/pendingprompt.go`
- Create: `api/internal/domain/agent_pending_prompt.go`

**Interfaces:**
- Consumes: `rs.chats.GetChat(ctx, chatID) (domain.Chat, error)`, `rs.ws.AgentChatsDir(ctx, workspaceID) (string, error)`, `rs.prompts.Dir(chatsDir, chatID) string`, `rs.prompts.LatestRequest(dir) (agentjournal.PromptRequest, bool, error)` (Task 2) — all already-existing `Runners` fields, same pattern as `promptrecovery.go`.
- Produces: `domain.PendingPrompt{ Text string; State string }`; `(*runner.Runners).PendingPrompt(ctx context.Context, chatID string) (domain.PendingPrompt, bool, error)`.

**No narrow unit test for this task.** `Runners` (`runner.go:34-74`) carries 15+ dependencies and this package has no shared test-constructor for it — every sibling method with the same shape (`ConfirmPromptAccepted`, `ReconcilePendingPromptFromLedger` in `promptrecovery.go`) has zero internal unit tests of its own for exactly that reason; they're proven through the `api/tests` integration harness instead. This method follows that same established pattern — its behavioral proof is Task 7's integration test, which exercises it through the real HTTP route with a real `Runners` wired by the real app bootstrap, a stronger test than a hand-built stub tree would be. This task's own steps are build-verified only.

- [ ] **Step 1: Add the domain type**

Create `api/internal/domain/agent_pending_prompt.go`:

```go
package domain

// PendingPrompt is the most recent prompt submission a chat's journal has not
// confirmed the provider accepted — recovered so a client whose own copy of
// the text was lost (an idle tab, a crash, cleared local storage) can show it
// back to the user instead of losing it outright.
type PendingPrompt struct {
	Text  string
	State string
}
```

- [ ] **Step 2: Add the usecase method**

Create `api/internal/app/usecases/chat/internal/runner/pendingprompt.go`:

```go
package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// PendingPrompt returns the chat's most recent prompt submission, if the
// journal has not yet confirmed the provider accepted it. A record already
// in PromptStateAccepted means the ledger has the reply's own request — the
// client's own transcript read already shows it, so there is nothing to
// recover. A record with no stored text is a pre-migration record, written
// before PromptRequest gained a Text field: also nothing to recover.
func (rs *Runners) PendingPrompt(
	ctx context.Context,
	chatID string,
) (domain.PendingPrompt, bool, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.PendingPrompt{}, false, fmt.Errorf("agent: pending prompt: chat: %w", err)
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return domain.PendingPrompt{}, false, fmt.Errorf("agent: pending prompt: chats dir: %w", err)
	}
	dir := rs.prompts.Dir(chatsDir, chat.ID)
	record, found, err := rs.prompts.LatestRequest(dir)
	if err != nil || !found {
		return domain.PendingPrompt{}, false, err
	}
	if record.State == agentjournal.PromptStateAccepted || record.Text == "" {
		return domain.PendingPrompt{}, false, nil
	}
	return domain.PendingPrompt{Text: record.Text, State: record.State}, true, nil
}
```

- [ ] **Step 3: Build and vet the whole tree**

Run: `cd api && go build ./internal/... && go vet ./internal/...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add api/internal/domain/agent_pending_prompt.go api/internal/app/usecases/chat/internal/runner/pendingprompt.go
git commit -m "feat(agent): recover a chat's most recent unaccepted prompt text"
```

---

### Task 4: Expose `PendingPrompt` over HTTP

**Files:**
- Modify: `api/internal/api/v0/endpoints/chat/handlers/handlers.go:145` (`RunnerUsecase` interface)
- Modify: `api/internal/api/v0/dto/agent.go` (new DTO, beside `PromptSubmissionDTO`)
- Create: `api/internal/api/v0/endpoints/chat/handlers/pendingprompt.go`
- Modify: `api/internal/api/v0/endpoints/chat/routes.go:96` (new route, beside `slash-catalog`)
- Test: `api/internal/api/v0/endpoints/chat/routes_test.go`

**Interfaces:**
- Consumes: `Runners.PendingPrompt(ctx, chatID) (domain.PendingPrompt, bool, error)` (Task 3); `h.requireChatInWorkspace(ctx, id) (domain.Chat, bool)` (existing, used by every sibling handler in this package); `libs.WriteQueryOK`, `libs.StatusAndMessage`, `libs.WriteErr` (existing).
- Produces: `GET {chatBase}/:id/pending-prompt` → `204` (nothing pending) or `200` with `dto.PendingPromptDTO{ Text string; State string }`.

- [ ] **Step 1: Write the failing test**

`routes_test.go` already has a `stubUsecase` implementing `RunnerUsecase` for route-existence tests (see its `SlashCatalog` stub at line 167) and a table of routes near line 335. Add a stub method and a route-table entry:

Add to `stubUsecase` in `routes_test.go`, beside its `SlashCatalog` stub:

```go
func (stubUsecase) PendingPrompt(
	ctx context.Context,
	chatID string,
) (domain.PendingPrompt, bool, error) {
	return domain.PendingPrompt{}, false, nil
}
```

Add to the route table near line 335, beside the `slash-catalog` entry:

```go
		{http.MethodGet, base + "/chats/c1/pending-prompt"},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/chat/... -run TestRoutes -v`
Expected: FAIL to compile — `stubUsecase` does not satisfy `RunnerUsecase` (missing method) and the route does not exist.

- [ ] **Step 3: Add the DTO**

In `api/internal/api/v0/dto/agent.go`, right after `PromptSubmissionDTO`:

```go
// PendingPromptDTO is the wire shape of domain.PendingPrompt — a prompt
// submission the journal has not yet confirmed the provider accepted, sent
// so a client whose own local copy was lost can recover the literal text.
type PendingPromptDTO struct {
	Text  string `json:"text"`
	State string `json:"state"`
}
```

- [ ] **Step 4: Add the interface method**

In `api/internal/api/v0/endpoints/chat/handlers/handlers.go`, add to `RunnerUsecase`, right after `SubmitPrompt`:

```go
	// PendingPrompt returns chatID's most recent prompt submission the
	// journal has not yet confirmed the provider accepted, so a client whose
	// own local copy of the text was lost can recover it.
	PendingPrompt(
		ctx context.Context,
		chatID string,
	) (domain.PendingPrompt, bool, error)
```

- [ ] **Step 5: Add the handler**

Create `api/internal/api/v0/endpoints/chat/handlers/pendingprompt.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) PendingPrompt(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	pending, found, err := h.runners.PendingPrompt(ctx.Request.Context(), chat.ID)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	if !found {
		ctx.Status(http.StatusNoContent)
		return
	}
	libs.WriteQueryOK(ctx, dto.PendingPromptDTO{Text: pending.Text, State: pending.State})
}
```

- [ ] **Step 6: Register the route**

In `api/internal/api/v0/endpoints/chat/routes.go`, right after the `slash-catalog` line:

```go
	wsScoped.GET("/chats/:id/pending-prompt", h.PendingPrompt)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd api && go test ./internal/api/v0/endpoints/chat/... -v`
Expected: PASS.

- [ ] **Step 8: Build and vet**

Run: `cd api && go build ./internal/... && go vet ./internal/...`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add api/internal/api/v0/dto/agent.go api/internal/api/v0/endpoints/chat/handlers/handlers.go api/internal/api/v0/endpoints/chat/handlers/pendingprompt.go api/internal/api/v0/endpoints/chat/routes.go api/internal/api/v0/endpoints/chat/routes_test.go
git commit -m "feat(agent): expose a chat's pending prompt over HTTP"
```

---

### Task 5: Frontend API client for the new endpoint

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts` (new export, beside `getChatTelemetry`)
- Test: `web/src/__tests__/features/agent/api/agent-api.test.ts` (if this file doesn't exist yet, check for the nearest existing test of a sibling function in this file first — e.g. `getChatTelemetry` — and match its mocking pattern exactly)

**Interfaces:**
- Consumes: `apiFetch<T>(url, init, retryOpts)` (existing, used by every other function in this file), `chatBase(wsId)` (existing, `agent-api.ts:9`).
- Produces: `getPendingPrompt(wsId: string, id: string, signal?: AbortSignal): Promise<PendingPrompt | null>`; `interface PendingPrompt { text: string; state: string }` (exported from this file).

- [ ] **Step 1: Write the failing test**

First, run: `grep -n "getChatTelemetry" web/src/__tests__/features/agent/api/agent-api.test.ts` to find that function's existing test and copy its exact mock-fetch setup pattern (this file already mocks `apiFetch`'s underlying transport — match whatever it does, don't invent a new mocking approach). Then add, following that same pattern:

```ts
describe('getPendingPrompt', () => {
  it('returns null when the backend has nothing pending (204)', async () => {
    mockFetchResponse(204, null) // use this test file's own existing 204-mocking helper, matching getChatTelemetry's test
    const result = await getPendingPrompt('ws-1', 'chat-1')
    expect(result).toBeNull()
  })

  it('returns the recovered text and state when something is pending', async () => {
    mockFetchResponse(200, { text: 'please rename this function', state: 'dispatching' })
    const result = await getPendingPrompt('ws-1', 'chat-1')
    expect(result).toEqual({ text: 'please rename this function', state: 'dispatching' })
  })
})
```

(`mockFetchResponse` is a placeholder name for whatever this test file's real existing helper is called — use the real one found above.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun vitest run src/__tests__/features/agent/api/agent-api.test.ts -t getPendingPrompt`
Expected: FAIL — `getPendingPrompt` is not exported yet.

- [ ] **Step 3: Add the function**

In `web/src/features/agent/api/agent-api.ts`, right after `getChatTelemetry`:

```ts
export interface PendingPrompt {
  text: string
  state: string
}

/** Recover a chat's most recent prompt submission the backend has not yet
 *  confirmed the provider accepted — used to rehydrate a queued prompt whose
 *  local copy was lost (an idle tab, a crash, cleared storage). Null means
 *  nothing to recover, not an error. */
export async function getPendingPrompt(
  wsId: string,
  id: string,
  signal?: AbortSignal,
): Promise<PendingPrompt | null> {
  const raw = await apiFetch<PendingPrompt | null>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/pending-prompt`,
    { signal },
    { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
  )
  return raw ?? null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bun vitest run src/__tests__/features/agent/api/agent-api.test.ts -t getPendingPrompt`
Expected: PASS.

- [ ] **Step 5: Type-check**

Run: `cd web && bun tsc --noEmit`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agent/api/agent-api.ts web/src/__tests__/features/agent/api/agent-api.test.ts
git commit -m "feat(agent): add a client for recovering a chat's pending prompt"
```

---

### Task 6: Wire recovery into the prompt queue

**Files:**
- Modify: `web/src/features/agent/hooks/use-prompt-queue.ts` (new recovery effect)
- Test: `web/src/__tests__/features/agent/hooks/use-prompt-queue.test.ts` (existing file, per `CLAUDE.md`'s mirror-structure rule — find and match its existing render/mock setup for this hook)

**Interfaces:**
- Consumes: `getPendingPrompt(wsId, chatId, signal)` (Task 5); `PromptQueueOptions` (existing, `use-prompt-queue.ts:77-106` — this task reads its existing `wsId`, `chatId`, `visible` fields, adds no new required option); `PromptQueueItem` (existing, `prompt-queue-persistence.ts:10-21`: `{ clientRequestId: string; text: string; state: PromptQueueState; createdAt: string; submittedAt?: string; baselineSequence: number; error?: string; waitForIdleEpoch?: number }`) and its `'outcome_uncertain'` state (`PromptQueueState`, same file line 8); the hook's own `queue` state and `setQueue` setter (existing, `use-prompt-queue.ts:145`).
- Produces: no new public return field — this task adds an internal effect only; the existing return shape (`queue`, `persistenceLost`, `cancelableCount`, ... — `use-prompt-queue.ts:574-589`) is unchanged.

- [ ] **Step 1: Write the failing test**

First run: `sed -n '1,60p' web/src/__tests__/features/agent/hooks/use-prompt-queue.test.ts` to see this hook's existing test setup (how it renders the hook, what it mocks `submitAgentPrompt` with, and its `PromptQueueOptions` fixture) and match that exactly. Then add:

```ts
it('recovers a lost queued prompt from the backend when the chat becomes visible', async () => {
  vi.mocked(getPendingPrompt).mockResolvedValueOnce({
    text: 'please rename this function',
    state: 'dispatching',
  })

  const { result } = renderHook(() =>
    usePromptQueue({ ...baseOptions, visible: true }), // baseOptions: this test file's existing fixture
  )

  await waitFor(() => {
    expect(result.current.queue).toContainEqual(
      expect.objectContaining({ text: 'please rename this function', state: 'outcome_uncertain' }),
    )
  })
})

it('does not recover anything when the backend has nothing pending', async () => {
  vi.mocked(getPendingPrompt).mockResolvedValueOnce(null)

  const { result } = renderHook(() =>
    usePromptQueue({ ...baseOptions, visible: true }),
  )

  await waitFor(() => expect(vi.mocked(getPendingPrompt)).toHaveBeenCalled())
  expect(result.current.queue).toEqual([])
})
```

Add `getPendingPrompt` to this test file's existing `vi.mock('@/features/agent/api/agent-api', ...)` block (it already mocks `submitAgentPrompt` from the same module — add `getPendingPrompt: vi.fn()` beside it).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun vitest run src/__tests__/features/agent/hooks/use-prompt-queue.test.ts -t "recovers a lost queued prompt"`
Expected: FAIL — no such behavior exists yet.

- [ ] **Step 3: Add the recovery effect**

In `use-prompt-queue.ts`, add the import at the top:

```ts
import { getPendingPrompt, submitAgentPrompt, type AgentChatMessage, type AgentPromptResult } from '@/features/agent/api/agent-api'
```

Inside `usePromptQueue`, after the existing chat-change reset effect (mirroring `use-agent-activity.ts`'s own "read once when visible" pattern — a one-shot effect gated on `visible`, re-armed on `chatId` change), add:

```ts
  // Recover a queued prompt this tab's own state lost entirely — an idle
  // reload, a crash, cleared storage. Runs once per chat becoming visible; a
  // chat with nothing pending costs one 204. See PendingPromptDTO.
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    void (async () => {
      const pending = await getPendingPrompt(wsId, chatId, controller.signal).catch(() => null)
      if (!pending || controller.signal.aborted) return
      setQueue((current) => {
        if (current.some((item) => item.text.trim() === pending.text.trim())) return current
        const recovered: PromptQueueItem = {
          clientRequestId: requestId(),
          text: pending.text,
          state: 'outcome_uncertain',
          createdAt: new Date().toISOString(),
          baselineSequence: getBaseline(),
        }
        return [...current, recovered]
      })
    })()
    return () => controller.abort()
  }, [visible, wsId, chatId, getBaseline])
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && bun vitest run src/__tests__/features/agent/hooks/use-prompt-queue.test.ts`
Expected: PASS, including both new tests and every pre-existing one in this file (dedup logic must not disturb a chat with an already-known queue item).

- [ ] **Step 5: Type-check**

Run: `cd web && bun tsc --noEmit`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/agent/hooks/use-prompt-queue.ts web/src/__tests__/features/agent/hooks/use-prompt-queue.test.ts
git commit -m "feat(agent): recover a lost queued prompt when its chat reopens"
```

---

### Task 7: Backend regression test for the end-to-end idle-loss scenario

**Files:**
- Create: `api/tests/regression_pending_prompt_test.go` (package `tests`, `//go:build integration`)

**Interfaces:**
- Consumes real harness helpers, verified by reading `api/tests/agent_abandoned_turn_test.go` and `api/tests/harness_test.go` directly (not guessed): `newHarness(t) *harness`, `writeLiveStubProviderDescriptor(t, h)`, `importWritableWorkspace(t, h) importedWorkspace` (exact return type: whatever `imported` is in `agent_abandoned_turn_test.go` — used there as `wsBase(imported)` and passed to `createLiveStubChat`), `createLiveStubChat(t, h, imported) (chatID, runnerID string)`, `wsBase(imported) string`, `(*harness).raw(method, path string, body any, wantStatus int) *http.Response` (`harness_test.go:310`), `(*harness).get(path string, out any)` (envelope-decoding GET, used by `getAgentChat`, `agent_rest_scope_test.go:127`), `h.Quiesce()`.

- [ ] **Step 1: Write the test**

Create `api/tests/regression_pending_prompt_test.go`:

```go
//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue proves
// the fix for "user's turns after some time of idle is lost, and does not
// record anywhere": a submitted prompt's literal text is recoverable from
// the backend even when the client that sent it has no memory of having
// done so — not merely a hash it cannot use to show the user anything back.
//
// This is the first test in this suite to drive the real POST .../prompts
// route against a live-stub chat (createLiveStubChat) rather than a
// hooks-simulated one — if the stub provider needs setup beyond what
// writeLiveStubProviderDescriptor/createLiveStubChat already give it to
// accept a real prompt dispatch, check api/tests/agent_runner_moves_test.go
// (the richest existing user of createLiveStubChat) for what else those
// tests configure before submitting to a live stub.
func TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)

	const submittedText = "please rename this function to something clearer"

	resp := h.raw(http.MethodPost, wsBase(imported)+"/chats/"+chatID+"/prompts",
		map[string]string{"text": submittedText, "clientRequestId": "11111111-1111-1111-1111-111111111111"},
		http.StatusOK,
	)
	_ = resp.Body.Close()

	// Simulate the frontend having forgotten this prompt entirely — a fresh
	// read with no prior client-side state, immediately after dispatch and
	// before anything has confirmed acceptance. If the live stub's own
	// hooks reply fast enough that this reads 204 (already accepted) instead
	// of 200, that is real information about this suite's timing, not a
	// flaw in the assertion — narrow the read to right after the POST
	// (already done above) rather than adding a wait for a state this test
	// is specifically about catching BEFORE it resolves.
	var got struct {
		Text  string `json:"text"`
		State string `json:"state"`
	}
	h.get(wsBase(imported)+"/chats/"+chatID+"/pending-prompt", &got)

	assert.Equal(t, submittedText, got.Text)
	require.NotEmpty(t, got.State)
}
```

- [ ] **Step 2: Run the test**

Run: `cd api && go test -tags integration ./tests/... -run TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue -v`
Expected: PASS. If it fails because the live-stub chat's prompt dispatch behaves differently than assumed above (a different status code, a different settle timing), that failure is real information — adjust the assertions to match what the harness actually does, per `agent_runner_moves_test.go`'s own patterns for this same stub, rather than forcing the test to match this plan's guess.

- [ ] **Step 3: Commit**

```bash
git add api/tests/regression_pending_prompt_test.go
git commit -m "test(agent): regression test for prompt-text recovery after idle loss"
```

---

### Task 8: Full verification pass

Not a code task — this is the plan's own closing gate, per the spec's §6 and the user's explicit "do not report back until live tested, perfectly working, production worth it."

- [ ] **Step 1: Run the full targeted test suites on the merged branch**

```bash
cd api && go build ./internal/... && go vet ./internal/...
go test ./internal/... 2>&1 | tail -100
go test -tags integration ./tests/... 2>&1 | tail -100
cd ../web && bun tsc --noEmit
bun vitest run src/__tests__/features/agent 2>&1 | tail -100
```

All must be clean. Any failure is investigated and fixed before proceeding — per project convention, reproduce any flaky-looking failure in the Docker Linux CI recipe before dismissing it as pre-existing flake.

- [ ] **Step 2: Live-verify sub-agent delegation (Codex)**

Via Tauri MCP against a real spawned `codex` process with `-c features.collab_agents=true` (or the security-review-hook path originally reported): drive a prompt that causes delegation, confirm the parent's spinner stays lit for the whole delegation and the turn is correctly attributed throughout — proving spec §1.2's finding, not building anything new.

- [ ] **Step 3: Live-verify scroll behavior end-to-end**

Via Tauri MCP: open an existing multi-turn chat (must land at bottom, no top-then-jump), send a prompt to an idle chat (pins to top, room reserved, no overshoot), send a prompt while a turn is already running (no pin, queued row visible, no blank viewport), watch a turn to completion (no bounce at the end), switch tabs and back (no glide).

- [ ] **Step 4: Live-verify prompt recovery**

Via Tauri MCP: submit a prompt, then simulate the frontend losing its own copy (reload the webview, or clear its local storage) before the delivery settles, reopen the chat, confirm the recovered prompt appears with retry/edit affordances instead of vanishing.

- [ ] **Step 5: Repeat Steps 2-4 against Claude**

Claude's PTY/hook transport exercises different code paths than Codex's JSON-RPC one for every item above — each must be independently confirmed, not assumed from Codex's result.

- [ ] **Step 6: Report**

Only once every step above is green: report completion, summarizing what was verified and any Docker CI reproduction done, per the user's explicit instruction not to surface until certain.
