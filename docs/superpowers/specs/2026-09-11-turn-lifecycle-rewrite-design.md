# Prompt Durability and Branch Reconciliation

Status: Approved for implementation, scope finalized after three rounds of
code verification
Date: 2026-09-11

## 1. How this spec got here

The user asked for a "big bang" rewrite of everything behind a run of live
bugs this session: Claude pane desync, a Codex working-status desync
recurring across four fix rounds, and a prompt silently lost after idle.
Three successive drafts of this spec narrowed the actual scope, each time by
reading real code instead of trusting a summary of it:

1. **Draft 1** proposed deleting `turn/idle.go`, `turn/stall.go`,
   `termwait`'s detector triad, and the `inflight` process-local mirror as
   four redundant liveness guesses. Reading the actual implementations
   showed they are four narrow, already-layered, incident-informed
   mechanisms solving four different problems. **Kept, unmodified.**
2. **Draft 2** proposed replacing `namesAnotherConversation` (the round-4
   sub-agent fix) with a structural nested-turn model. Tracing Codex's real
   `collabAgentToolCall` wire mechanism through `descriptors-v3/codex.yaml`
   showed the existing filter is already the complete, correct fix — the
   parent's `wait` tool call already stays `Running` for the sub-agent's
   whole duration, which is what keeps the spinner correctly lit; the filter
   only needs to (and does) stop the child's own turn lifecycle from
   touching the parent's. A structural model would have been new,
   unrequested, risky scope. **Kept, unmodified.**
3. **Draft 3** (this one) discovered that the branch this work was being
   built on (`fix-codex-turn-tracking`, based on `origin/develop`) was a
   sibling of a *second*, never-merged branch, `codex-no-scroll` — the
   branch this worktree was actually on at the start of this session,
   containing 25 commits of real, live-measured work: the queued-pin fix,
   the turn-overshoot/bounce fix, the first-open cascade fix, the
   tab-switch-reveal fix, a WS-subscription race fix, and several more
   working-status/streaming fixes. Every scroll symptom this spec's earlier
   drafts proposed rewriting `use-transcript-anchor.ts` to fix —
   overshoot, bounce, first-open jump, tab-switch reveal — already has a
   surgical, live-measured fix on that branch. Rewriting the hook would have
   discarded working, tested code and reintroduced already-fixed bugs.

`codex-no-scroll` has been merged into this work's branch (§5). It merged
clean; `go build`, `go vet`, and `bun tsc --noEmit` all pass on the result.

## 2. What's actually left

One genuine gap survives three rounds of verification:
`agentjournal.PromptRequest` durably records only a `TextHash`. The write is
already correctly timed (before the outgoing TUI is touched — confirmed in
`prompt_requests.go`'s own `Begin()`), and the existing hash-comparison
reconciliation (`promptrecovery.go`) already correctly prevents duplicate
submission. But if the frontend's own copy of a queued prompt's text is
lost — an idle tab, a crash, a cleared `localStorage` — there is no way to
get it back: the backend never kept the one thing that would let it be
shown to the user again. No commit on either branch touches this.

This spec's remaining scope is: close that gap, and verify (not rebuild)
everything §1 traced through — a live pass exercising sub-agent delegation,
Codex and Claude liveness, and the reconciled scroll behavior together for
the first time, since they never ran on the same branch until now.

## 3. Design: prompt text durability

**Backend** (`api/internal/adapter/store/agentjournal/prompt_requests.go`):

- `PromptRequest` gains `Text string \`json:"text"\`` alongside `TextHash`.
- `PromptRequests.Begin` gains a `text string` parameter, stored on the
  record it writes. Both call sites in
  `api/internal/app/usecases/chat/internal/runner/prompts.go` (line ~75,
  the `restart_tui` path, and line ~175, `submitPromptOverAPI`) already have
  the literal, pre-dispatch-rewrite `text` value in scope — the same value
  `agentjournal.PromptTextHash(text)` is already computed from at line 31 —
  so this is a threading change, not a new source of truth.
- New method on the `RunnerUsecase` interface
  (`api/internal/api/v0/endpoints/chat/handlers/handlers.go`):
  `PendingPrompt(ctx context.Context, chatID string) (domain.PendingPrompt, bool, error)`,
  implemented by reading `rs.prompts.ActiveDelivery(dir)` (already exists)
  and returning its `Text` when the record is in a non-terminal state
  (`dispatching`, `spawned`, `uncertain`) — the states where the frontend
  might have lost its own copy but the prompt has not been proven to have
  failed or landed.
- New domain type `domain.PendingPrompt{ Text string; State string }` and
  DTO `PendingPromptDTO{ Text string \`json:"text"\`; State string
  \`json:"state"\` }` in `api/internal/api/v0/dto/agent.go`, following the
  existing `PromptSubmissionDTO` pattern.
- New read-only HTTP route, same router group as the existing chat/prompt
  endpoints: `GET /workspaces/{workspaceId}/chats/{chatId}/pending-prompt` →
  `204` when nothing is pending, `200` with `PendingPromptDTO` otherwise.

**Frontend** (`web/src/features/agent/hooks/use-prompt-queue.ts` and its
chat-open path): on a chat becoming visible (mirroring the pattern
`use-agent-activity.ts` already uses for its own one-time backfill read —
see its `visible` effect), call the new endpoint. If it returns a pending
prompt whose text is not already present in this hook's own queue state
(compare by `PromptTextHash`-equivalent, or simply by presence of ANY row
for this chat, since only one delivery is ever active at a time — see
`agentjournal`'s own single-active-delivery invariant), synthesize a queue
row from the recovered text in the `outcome_uncertain` state, with the same
retry/edit affordances an ordinary uncertain row already has. This is
additive to the existing queue model (§6 below is unchanged); it is a new
source that can populate a row the frontend didn't already know about,
never a change to how an existing row behaves.

## 4. Explicitly out of scope (kept as-is, verified correct)

- `turn/idle.go`, `turn/stall.go`, `termwait`'s detector triad and settle
  logic, `inflight.Work`/`inflight.Turns`, `use-agent-activity.ts`,
  `namesAnotherConversation` — all traced and confirmed correct (§1.1,
  §1.2). Not touched.
- `use-transcript-anchor.ts`'s `pinTurnToTop`/`tailRoom` mechanism and every
  scroll fix from `codex-no-scroll` (queued-pin skip, overshoot/bounce
  re-measurement, first-open cascade gate, tab-switch reveal via the
  `visible` prop) — already correct, live-measured, merged (§1.3). Not
  touched.
- The existing `dispatching→spawned→accepted|failed|uncertain|settled`
  prompt state machine and the `promptConsumed` delivery-settle split — both
  already correct, kept (§2).
- `promptsigil`, the composer structure fix, and the terminal-connection
  resolver's null-vs-unknown fix — unrelated subsystems on separate,
  unmerged branches (`fix-composer-bugs`, `fix-claude-pane-desync`). Not
  part of this work; ship independently.

## 5. Branch state

Working branch: `rework/turn-lifecycle`. Built as:
`origin/develop` → `fix-codex-turn-tracking` (round-4 sub-agent fix,
approval-vocabulary fix, `promptConsumed` split — rebased onto, not
reimplemented) → this spec, twice revised → merged with `codex-no-scroll`
(25 commits: all four scroll-fix rounds, a WS-subscription race fix, and
several working-status/streaming fixes that substantially overlap with what
PR #171 already merged to `develop` from the same starting point — the
merge was clean, and post-merge `go build`, `go vet ./internal/...`, and
`bun tsc --noEmit` all pass with zero errors).

`fix-claude-pane-desync` and `fix-composer-bugs` remain untouched, unrelated
branches, free to ship independently of this work.

## 6. Testing and verification plan

- Unit/regression: write `TestRegression_*` (backend) and matching TS tests
  for the new `PendingPrompt` read path and frontend recovery — the one
  piece of new code this spec introduces.
- Run the existing test suites on the merged branch as a whole (not just the
  new code) — both `codex-no-scroll`'s and `fix-codex-turn-tracking`'s
  tests need to pass together for the first time. `go test` (backend,
  targeted packages touched by the merge: `turn`, `termwait`, `ws`, `spec`)
  and the TS tests for `use-transcript-anchor`, `agent-transcript`,
  `use-agent-activity`, `agent-activity` (lib).
- Live verification via Tauri MCP, no screen recording:
  - A real Codex sub-agent delegation (`-c features.collab_agents=true`) or
    the security-review-hook path originally reported: parent turn stays
    open, spinner lit, correctly attributed throughout.
  - Scroll behavior end-to-end on a live chat: open an existing chat (lands
    at bottom, no top-then-jump), send a prompt while idle (pins to top,
    room reserved), send a prompt while a turn is already running (no pin,
    no blank viewport), watch a turn to completion (no bounce).
  - Kill/restart the daemon (or clear the frontend's local state) with a
    prompt mid-flight, confirm the new `pending-prompt` endpoint recovers
    the real text into the queue instead of losing it.
  - Both Codex and Claude, since the two providers exercise different
    transports through all of the above.
- Docker-based Linux CI reproduction for anything touching the WS broadcast
  path, per existing project convention.
- Report back only once every item above is live-verified and green.
