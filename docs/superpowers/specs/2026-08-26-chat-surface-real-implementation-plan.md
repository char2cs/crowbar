# Chat surface changes: real implementation plan

**Date:** 2026-08-26

**Status:** approved for implementation, phased. Supersedes nothing — the five lab-phase docs
below stay as the evidence trail for *why* each change is what it is; this doc is *what happens
to `web/src`, in what order, verified how*.

- `2026-08-26-queued-row-styling-fix.md` — **done**, already applied to `transcript.css`
- `2026-08-26-composer-centric-interruptions-design.md`
- `2026-08-26-harness-message-typography-design.md`
- `2026-08-26-compacting-working-line-design.md`
- `2026-08-26-chat-empty-composer-transition-design.md`

Backend-touching work is in scope where a change genuinely needs it — confirmed explicitly rather
than assumed.

## Method

Each phase: re-verify the exact current real source immediately before touching it (not from
memory of an earlier read in this process — see Phase 3.1 below for why that matters), implement,
verify in the running dev-desktop app (not headless, not the lab — the lab and any isolated CSS
harness were stand-ins for real verification, never a substitute for it), run the test suite for
touched files, then move to the next phase.

## Phase 0 — done

Queued-row styling (`transcript.css`): `.queued`/`.txt`/`.err`/`.acts` reconnected to real intent,
diagonal hatch on `.bad`, error line hidden, actions moved outside the pill. Uncommitted.

## Phase 1 — CSS-only

1. **Multi-question unified box.** Re-verified against real source this round, not just the lab:
   `composer.css:738-745` — `.agent-chat .asks` is a bare `display:flex; flex-direction:column;
   gap:8px`, and `.agent-chat .asks .pill.asking{margin:0}` only resets margin. This is a
   genuinely unstyled real gap, same class of bug as queued-row — not a lab invention. Give
   `.asks` the box (border/background/radius, `.pill.multi`'s own radius since this is never a
   one-liner); strip `.pill.asking`'s own box when nested inside it.
2. **Remove the Compact button.** `controls/provider-bar.tsx` — delete the button, its handler
   wiring, and any now-unused icon import.

## Phase 2 — component logic

3. **Harness message: typography + accordion.** `transcript/message-row.tsx`'s harness branch —
   render the payload as `<code>`, add a `<details>` fallback for long payloads (length/line-count
   heuristic, no semantic parsing of the payload — see the harness-message spec for why that
   matters). No real precedent for the size threshold; pick one, don't import the lab's number
   uncritically.
4. **Compacting's working line.** `activity/working-line.tsx`'s `blocked` check
   (`pendingChoices(activity).length > 0 || blockedOn(activity) !== null`) currently silences the
   spinner for every interruption kind, compaction included. Needs a compaction-specific carve-out
   — show the spinner with a fixed "Compacting" verb (no rotating flavor text) instead of nothing.
5. **Permission-card copy.** `composer/composer-choice.tsx` — remove the explanatory sentences at
   lines 179, 189-190, 469-470, 481-482, 588, 591 (exact real strings, confirmed this round);
   replace with a Terminal action living in the same bar as the other buttons, per variant.
6. **Send-button spinner.** `composer/agent-composer.tsx` — a `sending` visual state on the send
   button, driven by `hooks/use-prompt-queue.ts`'s real `deliveryPending` (confirmed to already
   exist at line 157 — no new state needed), with a tooltip via the existing `Button` `tooltip`
   prop.

## Phase 3 — structural

7. **Move reviving/idle-failed/terminal-wait out of the pane-level overlay, into the composer.**
   `components/agent-chat-pane.tsx`'s `.pane-banner`-style rendering (~lines 1010-1046) moves into
   the composer tree, alongside where `dormant`/`unsupported`/`terminal_wait` signposts already
   live. Drop the redundant top banner for `terminal_wait` — the bottom `ComposerSignpost` already
   says the same thing.

   **3.1 — dropped from scope, not implemented:** auto-resume for a clean exit. Re-verified
   `agent-chat-pane.tsx` in full before writing this plan, not from earlier memory in this
   process, and found the real behavior already does almost exactly what was proposed, more
   carefully:
   - `'failed'` already only appears after a genuine automatic attempt fails — `revive()` fires
     automatically the moment a pane finds its chat dormant (line 498), and the Resume button
     only ever shows once that has demonstrably failed. Nothing to build; the positioning/copy
     work already done for this state stands.
   - `'exited'` is deliberately *not* retried past one attempt — `attemptedRef` enforces exactly
     one revive per chat per pane mount, spent before the request goes out, specifically so "a
     budget spent on success cannot loop; a budget spent on failure cannot retry-storm." A CLI
     that dies while the pane is open (`handleSessionGone`) waits for the daemon's own
     confirmation before trying again, because reviving on the client's own suspicion risks
     re-attaching to the dead PTY the daemon hasn't reaped yet.

   Building "auto-retry on exited" as originally scoped would mean overriding a documented safety
   decision (retry-storms, duplicate CLI spawns) to fix something that, on inspection, isn't
   broken. Nothing changes here.

8. **Empty → first-turn transition.** The largest, newest piece — dock arrival animation, the
   first turn's frozen-document treatment, the divider, the interrupted-turn record, stop
   before/after dispatch. Real files: `chat/agent-chat-view.tsx`, `chat/agent-empty-document.tsx`,
   `composer/agent-composer.tsx`, `transcript/message-row.tsx`, `transcript/agent-transcript.tsx`.
   Flagged as its own focused pass, not a quick port: the arrival animation needs to measure the
   empty document's handle position before `AgentEmptyDocument` unmounts and apply it to the
   dock as it mounts — two conditionally-rendered branches, not two states of one tree — which is
   real React sequencing work, not a CSS/copy change. Re-verify the current shape of all five
   files immediately before starting, the same way Phase 3.1 just changed under a fresh look.

## What Phase 3.1's finding changes about "ready for everything"

Nothing outside item 8 itself. It's a demonstration of why "re-verify immediately before touching,
not from memory" is in the Method section above, not a decoration — the same kind of fresh look
turned real, unstyled CSS into confirmed bugs earlier this session, and just turned an assumed gap
into confirmed-already-correct code. Worth taking as read for Phase 3.2 (the pane-banner move)
too: re-check the exact current `agent-chat-pane.tsx` render block before writing that code, not
this doc's line numbers, which will drift the moment item 7 itself lands.
