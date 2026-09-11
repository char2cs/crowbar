# Turn Lifecycle Rewrite: Liveness, Scroll Anchor, Prompt Durability

Status: Approved for implementation (big-bang, no legacy retained)
Date: 2026-09-11

## 1. Problem

Over one testing cycle, five distinct live bugs were reported and fixed one at a
time: Claude pane desync ("agent exited" while still running), a Codex
working-status desync that recurred across four separate fix rounds, and a
prompt silently lost after idle with no recovery path anywhere. Each fix was
real and verified, but they share one root cause: **liveness and durability
are currently inferred from scattered, independent signals instead of read
from one authoritative source.**

Concretely, today's system has *four* parallel mechanisms that can each
independently believe a turn is running or not:

1. `turn/idle.go` — an idle latch armed on provider-reported idle, cleared on
   turn open/close, read by a sweep. Its own comment documents that it can't
   act directly because Codex's `idle` status can fire sub-milliseconds before
   a healthy `turn/completed` — a fragile ordering race baked into the design.
2. `turn/stall.go` — a 120s PTY screen-scraping fallback
   (`MatchTerminalNotice`/`MatchTerminalPrompt`).
3. `termwait/evaluate.go` — delivery-settle timing measured against PTY screen
   activity, independent of the turn package's own state.
4. `inflight.Work`/`inflight.Turns` — a process-local mirror of turn state,
   which exists *specifically* because the durable, asynx-projected read
   model (`domain.Chat.Working`) lags behind writes. A second in-memory shadow
   was added to paper over the first model's own lag, rather than fixing it.

On the frontend, `use-agent-activity.ts` layers a 400ms×4 falling-edge retry
on top of all of that, to paper over the same class of read-lag one layer up.
`use-transcript-anchor.ts` has grown to ~20 independent ref-based state
variables across 816 lines, the direct result of seven rounds of point patches
to the same underlying jump/bounce/overshoot symptom. And
`agentjournal.PromptRequest` durably records only a `TextHash` — the one piece
of information needed to actually recover a lost prompt was never the one
being stored.

None of these are bugs in isolation so much as symptoms of not having one
authoritative model. This spec replaces all four turn-liveness mechanisms,
the frontend retry hack, the scroll anchor hook, and the prompt journal's
content gap with one coherent design. Per direction, this is a big-bang
replacement — nothing above is kept as a fallback path.

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

Both systems independently converge on the same three ideas this spec adopts:
one authoritative terminal signal, explicit (not filtered) scoping for nested
activity, and durable text written before the outcome is known.

## 3. Design: turn/liveness authoritative model

**System of record stays `domain.Chat`**, the existing asynx event-sourced
aggregate with `StartTurn`/`StopTurn`/`AbandonTurn` commands — this is already
structurally an append-only journal, the same shape Zeron uses. The problem
was never this model; it was the parallel side-channels that could each
independently decide a turn was over.

Changes:

- **Delete `turn/idle.go` and `turn/stall.go` entirely.** No idle latch, no
  sweep, no PTY screen-scraping. A turn ends only when `StopTurn` is called,
  and `StopTurn` may only be called from the two sources below.
- **Per-transport terminal signal, descriptor-declared.** Extend the existing
  `WireRef` mechanism with a `presentation.liveness` block:
  - `terminal: <WireRef>` names the wire event(s) that deterministically end a
    turn (Codex: `turn_stop || turn_failed`).
  - `deterministic_end: bool` — when false (Claude's PTY/hook transport has
    no single reliable terminal event), the runner runs one generic, bounded
    quiesce watchdog: after N seconds of silence with zero in-flight
    tool/question state, it calls `StopTurn` with an explicit `Idle` outcome
    — never `Completed`, never a fabricated result. This is the one
    remaining timeout in the system, and it is explicit UI-patience policy,
    not a liveness signal, matching Zeron's own narrowed design. The Go
    runner reads this from the descriptor; it does not know "Claude" —
    consistent with the existing no-provider-specific-code law.
- **No process-local mirror.** Delete `inflight.Work`/`inflight.Turns`. The
  WS broadcast of a turn-state change fires synchronously from the same
  command handler that commits `StartTurn`/`StopTurn` to the aggregate — not
  from the async read-model projector. The durable projection still exists
  for reload/history, but is never on the live-update path, so there is
  nothing to lag and nothing to shadow. This extends the same fix already
  shipped this session for the WS broadcaster (register before handshake
  returns): one writer, one path, no dual-source race.
- **`termwait` collapses into a query, not a parallel heuristic.**
  `SettleDeliveryFor` becomes: has `StopTurn` fired for the turn this
  delivery opened, and did we observe the provider's own ack event for this
  delivery (per-transport, descriptor-declared, same as today's
  `promptConsumed` split — that part of the round-4 fix was already correct
  and is kept). Delete the PTY-activity-based grace check in `evaluate.go`
  and the timer sweep in `sweep.go`. One bounded "how long before we offer
  retry UI" timeout remains, but it is explicitly a UI-patience policy on top
  of an already-known answer, not a guess at liveness.
- **Frontend `use-agent-activity.ts` loses the falling-edge retry hack.** It
  exists only to paper over the exact read-lag eliminated above; once the
  store updates synchronously off the real command commit, the 1200ms
  poll/retry block is dead code and is deleted, not kept as a safety net.

## 4. Design: nested/sub-turn scoping

Today, `turn/ingest.go`'s `namesAnotherConversation` filters out a child
thread's events by comparing IDs — a guard on a flat model, not a structural
fix (its own doc comment already names the real problem: "a connection is not
a conversation"). `descriptors-v3/codex.yaml` independently documents this
exact gap: sub-agent events need "a nested-thread model `StartSubagent`/
`StopSubagent` do not have," and ship as unwired dead weight today.

This spec adds that model, mirroring Zeron's explicit-wrapper approach:

- Every `Turn` gains `ParentTurnID *TurnID` (nil for a top-level turn).
- The descriptor's event mapping gains a `turn_scope` field naming which wire
  field carries thread/session identity (extending the existing `WireRef`
  pattern, not a new bespoke mechanism). The runner keeps a
  `ThreadID → TurnID` map; an inbound event resolves to the specific `Turn`
  its thread ID names, creating a new child `Turn` on first sight if needed,
  rather than being matched-or-dropped against the parent.
- A child turn's `StopTurn` closes only that child. The parent's liveness is
  never touched by a child's lifecycle — this is what makes cross-attribution
  structurally impossible instead of filtered-after-the-fact.
- `namesAnotherConversation` is deleted; its job is now handled by turn
  resolution being correct by construction.

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

- `spec/wire_ref.go` — extended (new `turn_scope`/`liveness` fields), not
  replaced.
- `pane-container.tsx` permanently-mounted-pane fix — depended on, unchanged.
- The WS broadcaster registration-before-handshake fix already shipped this
  session — its pattern is extended to turn broadcasts, not modified itself.
- `promptsigil`, the composer structure fix, and the terminal-connection
  resolver's null-vs-unknown fix — unrelated subsystems. `fix-composer-bugs`
  and `fix-claude-pane-desync` ship independently of this rewrite.
- `fix-codex-turn-tracking`'s `WireRef`-based approval-vocabulary fix
  (real `ask:` methods, `accept`/`decline` vocabulary) is orthogonal to
  liveness and is folded into this work as-is. That branch's
  `namesAnotherConversation` guard and `termwait` `promptConsumed` split are
  superseded/absorbed by sections 3–4 and 6 above respectively — they don't
  need to land separately.

## 8. Deletions (exact, big-bang — nothing below is kept as a fallback)

- `api/internal/app/usecases/chat/internal/turn/idle.go`
- `api/internal/app/usecases/chat/internal/turn/stall.go`
- `inflight.Work` / `inflight.Turns` types and all call sites
- `namesAnotherConversation` and its call site in `turn/ingest.go`
- PTY-activity settle check in `termwait/evaluate.go`; timer sweep in
  `termwait/sweep.go`
- Falling-edge retry block in `use-agent-activity.ts`
- Entire prior contents of `use-transcript-anchor.ts` (replaced in place)

## 9. Testing and verification plan

- One `TestRegression_*` per historically reported bug, named for its source
  per project convention: sub-agent bleed-through, idle-loss, working-status
  desync during a security-review hook, scroll bounce near turn completion,
  scroll overshoot while a message is pending, first-open top-then-jump,
  tab-switch reveal.
- Live verification via Tauri MCP against real spawned `codex` and `claude`
  processes for both the deterministic-end path (Codex) and the
  quiesce-watchdog path (Claude) — no screen recording, driven the same way
  as the rest of this session's live verification.
- Docker-based Linux CI reproduction for anything touching the WS broadcast
  path, per existing project convention, before considering it settled.
- No incremental merge to `develop` mid-rewrite; the branch lands as one
  coherent piece once every item above is live-verified on both providers.

## 10. Rollout

Single branch off `origin/develop` (suggested name: `rework/turn-lifecycle`).
Not pushed or reported as done until every item in section 9 is live-verified
and green. The three already-fixed, unpushed branches from the prior session
(`fix-claude-pane-desync`, `fix-composer-bugs`, `fix-codex-turn-tracking`) are
otherwise unaffected; whether to push `fix-codex-turn-tracking`'s
already-orthogonal pieces separately is a decision deferred until after this
rewrite lands.
