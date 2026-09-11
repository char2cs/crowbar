# Scroll Anchor Rewrite and Prompt Durability

Status: Approved for implementation, revised twice after code verification
(§1, §4)
Date: 2026-09-11

## 1. Problem

Over one testing cycle, five distinct live bugs were reported and fixed one at a
time: Claude pane desync ("agent exited" while still running), a Codex
working-status desync that recurred across four separate fix rounds, and a
prompt silently lost after idle with no recovery path anywhere. The
working-status bug's actual root cause, found on the fourth round, was that
Codex pushes a sub-agent's entire independent turn lifecycle
(`turn/started`..`item/*`..`turn/completed`, `thread/status/changed(idle)`)
down the *same* websocket as the parent thread, and Crowbar's event ingestion
had no way to tell the two apart. The round-4 fix
(`namesAnotherConversation`) is a same-session-ID filter — and, as §4 traces
in detail, it turns out to already be the complete, correct fix, not a
stopgap for something bigger.

**Revision history, both made before any implementation began:**

1. An earlier draft characterized Crowbar's existing idle-detection machinery
   (`turn/idle.go`, `turn/stall.go`, `termwait`'s detector triad, the
   `inflight` package) as four redundant, scattered guesses at liveness, on
   the strength of a first-pass research summary and the pre-compaction
   session history. Reading the actual implementations (§3) showed that was
   wrong: those are four narrow, already-layered, incident-informed
   mechanisms solving four different problems. Kept as-is.
2. A second draft then proposed replacing `namesAnotherConversation` (the
   round-4 sub-agent fix) with a structural nested-turn model. Tracing
   Codex's actual wire mechanism through the descriptor (§4) showed the
   round-4 fix is already the complete, correct fix for the reported bug, and
   that a structural model would be unrequested new scope carrying
   acknowledged correlation risk. Not built.

What remains, validated against the actual code rather than a summary of it:
`use-transcript-anchor.ts` has grown to ~20 independent ref-based state
variables across 816 lines, the direct result of seven rounds of point
patches to the same jump/bounce/overshoot symptom — this diagnosis holds up
and is unrelated to any backend liveness code. And `agentjournal.PromptRequest`
durably records only a `TextHash` — the one piece of information needed to
actually recover a lost prompt was never the one being stored, even though
the write is already correctly timed.

This spec replaces the scroll anchor hook and closes the prompt journal's
content gap. Everything else that was in scope in an earlier draft is kept
as-is, verified correct rather than rewritten.

## 2. Reference research

Two real systems were read (not copied from, adapted to Crowbar's own
architecture and its "no provider-specific code" law) to validate the design
before committing to it:

- **Athas** (`~/Projects/Cloned/athas`) models a turn as one awaited request
  future; its Rust ACP bridge only emits completion when that future
  genuinely resolves — no polling, no quiet-timer. (Its TypeScript layer
  *does* bolt a 10s/60s inactivity timer on top of that clean signal, which
  is the anti-pattern this spec avoids repeating on Crowbar's frontend.) Its
  chat view uses a single boolean "stuck to bottom" flag, cleared on user
  scroll, and never unmounts a background pane — switching tabs has nothing
  to restore because the DOM never went away.
- **Zeron** models a turn as an append-only journal whose last event is a
  terminal `Done`; nothing else may declare completion. Sub-agent and hook
  events are explicitly wrapped (`Subagent{parent_tool_use_id, event}`) rather
  than flattened onto the parent's stream and filtered — structural scoping,
  not a same-ID filter. Its scroll model is a `pinned` boolean with a
  direction-aware restick band and a spring that only chases a monotonically
  growing target, so it cannot overshoot. Its stall-watchdog was **removed on
  review** with the rationale "agents may legitimately wait far longer than
  any timeout; a live child IS the working signal" — later a narrow
  quiesce-watchdog was reintroduced, but only for harnesses lacking a
  deterministic turn-end signal, and it only ever parks state to Idle, never
  fabricates a result. Its prompt text is written synchronously to a durable
  CRDT/SQLite store before dispatch, decoupled entirely from whether the
  request ever completes.

Both systems independently converge on three ideas: one authoritative
terminal signal, explicit scoping for nested activity, and durable text
written before the outcome is known. Reading Crowbar's own code against that
(§3, §4) showed the first two are already substantially in place —
`domain.Chat`'s asynx-sourced `StartTurn`/`StopTurn` commands are the single
writer, the existing idle/stall detectors are narrow corroborated fallbacks
rather than independent guesses, and the sub-agent scoping bug already has a
correct, complete fix. The third idea (durable text) was already correctly
timed and just needed its content gap closed. The genuine remaining gap this
spec closes is narrower: the scroll anchor hook (§5) and the prompt journal's
content (§6).

## 3. What the backend liveness code actually is (kept, not rewritten)

**System of record is `domain.Chat`**, the existing asynx event-sourced
aggregate with `StartTurn`/`StopTurn`/`AbandonTurn`/`RestateAsyncWork`
commands — already structurally sound, the same shape Zeron's journal is.
Verified in this codebase (not assumed):

- **`inflight.Work`/`inflight.Turns`** (`shared/inflight/internal/turnstate`)
  is a process-local mirror set synchronously from each command's own result,
  read by `ChatWorking`/`AwaitTurnComplete` in preference to the lagging
  asynx projection, with the projection only as a boot-time seed. `Turns` is
  keyed by *runner*, not chat, specifically so a provider switch never kills
  a CLI mid-answer (the "No conversation found with session ID" incident this
  prevents is cited in its own doc comment). This is correct, load-bearing,
  and **kept as-is** — it is not a duplicate of the durable projection, it is
  what makes reads correct despite the projection's async lag.
- **`turn/idle.go`'s idle latch** is armed only on a provider's own
  idle-report and read by exactly one consumer, `termwait`'s sweep — its doc
  comment states this directly ("the terminal-wait sweep is its only
  reader"). It exists for the one case nothing else catches: a turn that
  only reasoned and produced no notice, no half-streamed message, and no
  explicit close. **Kept as-is.**
- **`turn/stall.go`** delegates its actual pattern matching to the
  descriptor (`engineagents.NoticeMatcher`/`MatchTerminalPrompt`) — already
  provider-agnostic — and executes turn abandonment
  (`CloseStalledTurn`/`AbandonTurn`). **Kept as-is.**
- **`termwait`'s three-tier detector** (`providerSaysItIsIdle` →
  `stalled` → `abandonedMessage`, in `evaluate.go`) is ordered by
  authoritativeness (provider's own idle report first, screen-notice second,
  message-silence third) and every tier is corroborated by pending-choices
  and `OpenWork` checks before it acts — this is the narrow,
  never-fabricate-success watchdog pattern Zeron converged on, already
  implemented, with measured production data behind its thresholds (e.g. the
  31s-quiet mid-security-review case this session's own reporter hit). **Kept
  as-is.** `SettleDelivery`'s PTY-activity-gated grace period and the
  `promptConsumed` split (round-4 fix) are a distinct, narrower concern —
  delivery-retry UX, not turn liveness — and are also kept as-is (§6).
- **Frontend `use-agent-activity.ts`** does not own `working`/`compacting` —
  they arrive as props from the WS-pushed chat state. Its falling-edge retry
  exists only to catch the separate, push-less activity-timeline endpoint
  (the tool-call list) landing its last update slightly after the boolean
  flips — a real, bounded (≤1.6s), well-scoped race unrelated to the turn
  bug. **Kept as-is.**

None of the above is where the bug lived. It lived in ingestion having no
model of a nested turn at all (§4).

## 4. Sub-agent bleed-through: already fixed, not rebuilt

**Second revision.** The original diagnosis (round-4 bug: a Codex sub-agent's
own turn lifecycle corrupting the parent chat's spinner) named
`namesAnotherConversation` — a same-session-ID filter in `turn/ingest.go` —
as a stopgap needing a "real" structural nested-turn model. Tracing Codex's
actual collab-agent wire mechanism through `descriptors-v3/codex.yaml:422-464`
(itself the record of two independent live captures) shows this is wrong:

- The model's `spawnAgent`/`wait`/`closeAgent`/`sendInput`/`resumeAgent` calls
  are its OWN tool, `collabAgentToolCall` — already mapped as ordinary tool
  rows via the existing `tool_pre`/`tool_post`/`tool_fail` events
  (`codex.yaml:130-184`), targets and all (`spawnAgent`'s target is the prompt
  it hands the sub-agent; `wait`/`closeAgent`'s target is
  `receiverThreadIds[0]`, the child's own thread id).
- The `wait` call's tool row stays `Running` for the *entire* duration the
  child thread works, and only completes once the child finishes — which is
  exactly what `turn/stall.go`'s `OpenWork` already reads (open tool calls)
  to keep the parent's spinner correctly lit for the whole delegation, via
  machinery `namesAnotherConversation` doesn't touch at all.
- `namesAnotherConversation` prevents exactly one thing: the child thread's
  *own* `turn/started`..`item/*`..`turn/completed`..`thread/status/changed`
  cycle, pushed down the same websocket, from being misattributed to the
  parent's `StopTurn`/idle state. That is the entire bug that was reported
  live (spinner dark, "This agent has exited", while the child was still
  working) — and it is what this filter already, correctly, fully prevents.

So the round-4 fix is not a partial patch standing in for a missing
structural model — it is the complete, correct fix for the bug that was
reported. What a "real" nested-turn model would add on top is *richness*
(rendering the child's own live messages/tool calls, not just a running
`wait` row and a final opaque `agentsStates` JSON blob) — a new capability,
never reported as broken, and one the descriptor's own author explicitly
flagged as carrying real risk: correlating `receiverThreadIds[0]` against
`agentsStates`, keyed by an arbitrary runtime thread ID the mapping grammar
cannot address today, is called out by name as work where "a broken or
unverified nested-subagent mapping would be worse than none."

**Decision: do not build it.** It is unrequested scope, on a system already
behaving correctly, carrying acknowledged correlation risk, for a feature no
live bug report has asked for. `namesAnotherConversation` is kept exactly
as-is. This section's job in the plan is verification, not construction: a
regression test that drives a real Codex sub-agent delegation live and
confirms the parent's turn stays open and correctly attributed throughout
(§9) — proving the existing fix, not replacing it.

## 5. Design: scroll anchor

`use-transcript-anchor.ts` is replaced wholesale (same file path, new
implementation — the hook's external interface to the chat pane is
preserved) with a single-source-of-truth model:

- One `pinned` boolean (default true), cleared synchronously by any
  user-initiated scroll/wheel/touch input.
- Re-arms only via `shouldRestick`: `distanceFromBottom <= RESTICK_BAND_PX`
  (~48–70px) **and** `distanceFromBottom < previousDistance` — direction-aware,
  so a small upward notch near the bottom can't re-lock the pin and cause a
  bounce.
- A single ResizeObserver on the transcript's content box drives a rAF-stepped
  approach toward `target = scrollHeight - clientHeight`. The step only ever
  moves toward a monotonically increasing target — it cannot overshoot,
  because there's nothing to overshoot past; a growing target is chased, not
  a fixed one aimed at.
- First reveal of a populated transcript (`wasEmpty` true→false, e.g. opening
  an existing chat) snaps instantly with no animation, before first paint —
  fixes "starts at top, then jumps."
- No separate tab-switch-reveal case: this depends on the already-shipped
  `pane-container.tsx` permanently-mounted-pane fix (`visibility: hidden` on
  inactive buffers) staying exactly as-is. Because the container is never
  unmounted on tab switch, there is nothing to restore, so the special-cased
  reveal logic in the old hook is deleted rather than ported.

## 6. Design: prompt durability

The investigation found the timing here was already correct — the prompt
journal's `Begin()` call already writes a row before the outgoing TUI/wire
call is touched. The gap is narrower than originally assumed: it's a content
gap, not a timing gap.

- `agentjournal.PromptRequest` gains a `Text string` field, written at the
  same existing `Begin()` call site. `TextHash` can be dropped or kept purely
  as a dedup/integrity aid; it is no longer the only record.
- `use-prompt-queue.ts`'s recovery path (`RecoverOrphanedDispatches` or its
  WS-surfaced equivalent) can now rehydrate a lost queued prompt's real text
  from the backend record, instead of depending solely on the frontend's own
  React-state/localStorage copy surviving. This closes the idle-loss bug at
  its root — the backend now holds the recoverable truth — rather than only
  patching the frontend symptom.
- The existing `dispatching→spawned→accepted|failed|uncertain|settled` state
  machine and the `promptConsumed` split (added in the round-4 fix) are
  sound and are kept as-is.

## 7. Explicitly out of scope (kept as-is)

- `turn/idle.go`, `turn/stall.go`, `termwait`'s detector triad and settle
  logic, `inflight.Work`/`inflight.Turns`, `use-agent-activity.ts`,
  `namesAnotherConversation` and its call site in `turn/ingest.go` — all
  verified correct and load-bearing in §3–§4. Not touched.
- `pane-container.tsx` permanently-mounted-pane fix — depended on, unchanged.
- The WS broadcaster registration-before-handshake fix already shipped this
  session — unrelated, unmodified.
- `promptsigil`, the composer structure fix, and the terminal-connection
  resolver's null-vs-unknown fix — unrelated subsystems. `fix-composer-bugs`
  and `fix-claude-pane-desync` ship independently of this rewrite.
- `fix-codex-turn-tracking`'s `WireRef`-based approval-vocabulary fix
  (real `ask:` methods, `accept`/`decline` vocabulary) and its `termwait`
  `promptConsumed` split are both already-correct and kept as-is — this
  branch is built directly on top of `fix-codex-turn-tracking` (rebased),
  so they're simply present, not re-implemented.

## 8. Deletions and additions (exact)

Deleted: nothing. Every mechanism examined in §3–§4 is correct and stays.

Added:
- `use-transcript-anchor.ts`'s entire implementation is replaced in place
  (same file path, same external interface) with the single-`pinned`-boolean
  model (§5) — this is a rewrite of one file's contents, not a deletion of a
  mechanism.
- `agentjournal.PromptRequest.Text` (§6).

## 9. Testing and verification plan

- One `TestRegression_*` per historically reported bug, named for its source
  per project convention: sub-agent bleed-through (a real Codex sub-agent
  delegation must leave the parent turn open, correctly attributed, spinner
  lit, throughout — proving §4's existing fix rather than building anything
  new), idle-loss (prompt text recoverable from the journal, not just React
  state), scroll bounce near turn completion, scroll overshoot while a
  message is pending, first-open top-then-jump, tab-switch reveal.
- Live verification via Tauri MCP against a real spawned `codex` process
  exercising an actual `collabAgentToolCall` delegation (`-c
  features.collab_agents=true`, matching the descriptor's own capture setup)
  or the security-review-hook path the original bug report narrowed this
  to — no screen recording, driven the same way as the rest of this
  session's live verification.
- Docker-based Linux CI reproduction for anything touching the WS broadcast
  path, per existing project convention, before considering it settled.
- No incremental merge to `develop` mid-rewrite; the branch lands as one
  coherent piece once every item above is live-verified.

## 10. Rollout

Branch `rework/turn-lifecycle`, built on top of `fix-codex-turn-tracking`
(rebased onto it rather than bare `origin/develop`), since §7 keeps that
branch's approval-vocabulary and `promptConsumed` work as-is — they arrive
by being on the same branch, not by re-implementation. `fix-claude-pane-desync`
and `fix-composer-bugs` are unrelated subsystems and stay independent, free
to ship on their own regardless of this work. Not pushed or reported as done
until every item in §9 is live-verified and green.
