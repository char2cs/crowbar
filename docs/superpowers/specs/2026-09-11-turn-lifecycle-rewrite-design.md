# Turn Lifecycle Rewrite: Liveness, Scroll Anchor, Prompt Durability

Status: Approved for implementation, revised after code verification (§1)
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
had no concept of a nested turn — it attributed everything on that connection
to the one open conversation. The round-4 fix (`namesAnotherConversation`) is
a same-ID filter over a flat model, not a structural fix.

**Revision note:** an earlier draft of this section characterized Crowbar's
existing idle-detection machinery (`turn/idle.go`, `turn/stall.go`,
`termwait`'s detector triad, the `inflight` package) as four redundant,
scattered guesses at liveness, on the strength of a first-pass research
summary and the pre-compaction session history. Reading the actual
implementations (see §3) showed that characterization was wrong: those are
four narrow, already-layered, incident-informed mechanisms solving four
different problems, not one problem four times. They are kept. The one
validated structural gap is nested/sub-turn scoping (§4); the frontend
scroll anchor (§5) and the prompt journal's content gap (§6) stand as
originally scoped. This is a smaller rewrite than first proposed to the
user, corrected before implementation began.

`use-transcript-anchor.ts` has grown to ~20 independent ref-based state
variables across 816 lines, the direct result of seven rounds of point patches
to the same underlying jump/bounce/overshoot symptom — this part of the
diagnosis holds up and is unrelated to the backend liveness code. And
`agentjournal.PromptRequest` durably records only a `TextHash` — the one piece
of information needed to actually recover a lost prompt was never the one
being stored, even though the write is already correctly timed.

This spec makes the nested-turn model structural, replaces the scroll anchor
hook, and closes the prompt journal's content gap. Nothing here is kept
alongside a legacy fallback: the filter-based sub-turn guard and the old
scroll anchor hook are fully replaced, not shimmed.

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
terminal signal, explicit (not filtered) scoping for nested activity, and
durable text written before the outcome is known. Reading Crowbar's own code
against that (§3) showed the first idea is already substantially in place —
`domain.Chat`'s asynx-sourced `StartTurn`/`StopTurn` commands are the single
writer, and the existing idle/stall detectors are already narrow, ordered,
corroborated fallbacks rather than independent guesses. The second idea
(explicit nested scoping) is the genuine, validated gap this spec closes.
The third (durable text) was already correctly timed and just needed its
content gap closed.

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
- A child turn closes the same way a parent does today: an explicit wire
  close event (`turn_stop`/`turn_failed`), now routed to the child's own
  `Turn` via `turn_scope` resolution instead of being filtered out or
  misapplied to the parent. The §3 idle-latch/stall fallback stays keyed by
  chat for the top-level turn it protects today; nothing in this session's
  bug history shows a child turn that silently never closes, so extending
  that fallback to be per-`Turn` (chat *or* nested) is a narrow, optional
  generalization of the existing keying, not a new mechanism — implement it
  only if live testing (§9) surfaces a stuck child turn.

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
  logic, `inflight.Work`/`inflight.Turns`, `use-agent-activity.ts` — all
  verified correct and load-bearing in §3. Not touched.
- `spec/wire_ref.go` — extended (new `turn_scope` field), not replaced.
- `pane-container.tsx` permanently-mounted-pane fix — depended on, unchanged.
- The WS broadcaster registration-before-handshake fix already shipped this
  session — unrelated, unmodified.
- `promptsigil`, the composer structure fix, and the terminal-connection
  resolver's null-vs-unknown fix — unrelated subsystems. `fix-composer-bugs`
  and `fix-claude-pane-desync` ship independently of this rewrite.
- `fix-codex-turn-tracking`'s `WireRef`-based approval-vocabulary fix
  (real `ask:` methods, `accept`/`decline` vocabulary) is orthogonal to
  liveness and is folded into this work as-is. That branch's
  `namesAnotherConversation` guard is superseded by §4's structural fix; its
  `termwait` `promptConsumed` split is already-correct and kept per §3/§6 —
  neither needs to land separately.

## 8. Deletions and additions (exact)

Deleted:
- `namesAnotherConversation` and its call site in `turn/ingest.go` —
  superseded by structural `turn_scope` resolution (§4).
- Entire prior contents of `use-transcript-anchor.ts` (replaced in place,
  same file path) — superseded by the single-`pinned`-boolean model (§5).

Added:
- `Turn.ParentTurnID`, the `ThreadID → TurnID` resolution map, and the
  descriptor `turn_scope` field (§4).
- `agentjournal.PromptRequest.Text` (§6).

Nothing else is deleted. §7 is the explicit, verified list of what stays.

## 9. Testing and verification plan

- One `TestRegression_*` per historically reported bug, named for its source
  per project convention: sub-agent bleed-through (parent turn must stay open
  and correctly attributed through a real nested sub-agent turn), idle-loss
  (prompt text recoverable from the journal, not just React state), scroll
  bounce near turn completion, scroll overshoot while a message is pending,
  first-open top-then-jump, tab-switch reveal.
- Live verification via Tauri MCP against a real spawned `codex` process
  exercising an actual sub-agent delegation (or the security-review-hook path
  the original bug report narrowed this to) — no screen recording, driven the
  same way as the rest of this session's live verification.
- Confirm §3's kept detectors (idle latch, `OpenWork`'s tool/subagent scan,
  `termwait`'s triad) still behave correctly once nested turns are structural
  — in particular, that `OpenWork`'s subagent-open check still reflects a
  live child turn under the new model, since a false read there would
  darken the parent's spinner under a live subagent (the exact historical
  bug, via a different path).
- Docker-based Linux CI reproduction for anything touching the WS broadcast
  path, per existing project convention, before considering it settled.
- No incremental merge to `develop` mid-rewrite; the branch lands as one
  coherent piece once every item above is live-verified.

## 10. Rollout

Single branch off `origin/develop` (suggested name: `rework/turn-lifecycle`).
Not pushed or reported as done until every item in section 9 is live-verified
and green. The three already-fixed, unpushed branches from the prior session
(`fix-claude-pane-desync`, `fix-composer-bugs`, `fix-codex-turn-tracking`) are
otherwise unaffected; whether to push `fix-codex-turn-tracking`'s
already-orthogonal pieces separately is a decision deferred until after this
rewrite lands.
