# Empty composer → first-turn transition

**Date:** 2026-08-26

**Status:** design, prototyped in the chat-state-lab review artifact
(https://claude.ai/code/artifact/8b011a0a-cd20-4c4a-9836-0f1e195ac98f)

None of the behaviour below exists in `web/src` today. Every "current" line cites the file that
proves it; every "proposed" line is new work.

## 1. Why

The blank chat (`AgentEmptyDocument`, `agent-empty-document.tsx`) is a full-pane writing
document, not a message box — that's already the app's own framing (see the component's doc
comment: "a DOCUMENT, not an empty transcript with a message box under it"). But the moment the
first prompt is sent, that framing is dropped instantly: `enqueueDraft` pushes into
`prompts.enqueue` (`agent-chat-view.tsx:295-304`), `queue.length` becomes non-zero, `blank` flips
to `false` (`agent-chat-view.tsx:360-361`), and `AgentEmptyDocument` is unmounted in favour of the
transcript + `.dock` tree on the very next render. There is no transition — React tears down one
component tree and mounts a completely different one.

This document specifies three changes that carry the "it's a document" idea through that moment
instead of dropping it, plus one open question about what "stop" should mean while it's happening.

## 2. Composer → dock arrival animation

**Current:** hard remount, no animation, no shared element (confirmed above).

**Proposed:** when the first prompt is sent —

1. The empty document's controls bar (`.dochandle`, currently just chips + Send —
   `agent-empty-document.tsx:137-155`) is replaced, in the same frame, by the real running dock
   (message field + Send/Stop, `.pill` + hidden `.underbar`) positioned exactly where the old bar
   was. This swap is instant — no crossfade, no morph tween on the shape itself.
2. That dock then eases from the old bar's position down to its normal resting spot at the
   bottom of the pane (~320ms, ease-out).
3. The underbar (provider/model/effort chips, the view switcher, the context gauge) stays hidden
   until the slide finishes, then fades/slides in over ~200ms.

The written-in-progress text itself does not need its own animation: because the frozen document
(§3) sits at the same typographic column as the composer was, it only needs to stop being
editable in place — the moving piece is the control bar, not the words.

Prototype: `animateFirstSend()` in the lab artifact — measures `#docHandle`'s position, renders
the destination dock offset by that delta via `transform`, then animates `transform` back to
zero and reveals `.underbar` on `transitionend`.

## 3. First turn renders as a frozen document, not a bubble

**Current:** every user message, including the first, renders identically —
`className={cn('row', user && 'me')}` wrapping `className={cn(user && 'bubble', ...)}`
(`message-row.tsx:54-71`). There is no per-turn special case.

**Proposed:** the first user turn of a chat keeps the empty document's own typography — full
width, 16px / 1.7 line-height, no bubble background, no right alignment — instead of switching to
the `.bubble` treatment. It still renders through the same markdown pipeline every other message
uses (Plate — no exception here); only the wrapping class changes. It is never editable in place
once sent, matching every other message row.

**Every turn after the first is an ordinary bubble.** This only touches turn #1.

Implementation note: "first turn" must be resolved from the message's absolute sequence (e.g.
`sequence === 0`), not from position in the currently loaded window — `hasOlder` /
"Load earlier messages" (`agent-transcript.tsx:89-101`, `24-101`) means the true first message
isn't always the first one on screen once older history has been paged in.

A plain hairline (no label, unlike the compaction divider) follows the frozen document, marking
where it ends and the conversation begins. It renders as soon as the turn freezes, whether or not
anything has replied to it yet — waiting for the first reply would mean the line pops in with a
layout shift once the agent responds.

Prototype: `row('user', text, {firstTurn: true})` in the lab artifact, and the standalone
"First turn: frozen document" state, which also shows a second, ordinary bubble right after it
for contrast.

## 4. Spinner/working-line sits at the tail, not as a new block

**Current:** `WorkingLine` is the last child of `.stream` (`agent-transcript.tsx:157-161`) and
gets the same uniform `gap: 18px` as every other message-to-message spacing
(`transcript.css:75`) — it reads as a new block, not a continuation of whatever came before it.

**Proposed:** the working line (spinner + verb + elapsed time + running tools) sits glued to the
end of whatever is currently the last line in the transcript — the sent user text before the
agent has produced anything, then the tail of the agent's own streamed reply once it starts —
rather than floating a full row-gap below it.

Prototype: tightened via `margin-top: -14px` on `.activity` against the `18px` stream gap,
leaving ~4px before its own internal padding. Exact numbers are a starting point for visual
tuning, not a spec requirement.

## 5. Stop during the first turn

**Current:** `stopChat` gracefully terminates the CLI and explicitly leaves the chat "DORMANT and
resumable... This is NOT deleteChat: the chat is preserved" (`agent-api.ts:818-828`). It does not
retract anything from the ledger or the local queue.

**Proposed, scoped to what stays honest with that contract:**

- **Stopped before the prompt was ever dispatched** (still sitting unsent in the local queue —
  the window `prompts.remove` / `cancelUnsentPrompts` already cover): reverses the arrival
  animation and returns to the editable empty document with the draft text restored. Nothing was
  ever recorded anywhere, so this is a true undo, not a fiction.
- **Stopped after the prompt was actually dispatched** (the CLI has it, possibly already produced
  partial output): the frozen document (§3) stays exactly where it is, and the dock does not snap
  back to blank. Doing so would show "nothing happened" in a chat the backend still has a real,
  resumable turn recorded against, which is the same class of bug as inventing UI that isn't
  backed by real state.

This is a narrower version of the original ask ("if the user stops the first turn, everything
must go into place like it was originally") — agreed for the pre-dispatch case, not the
post-dispatch one. Flagged for confirmation; not yet fully signed off.

Prototype: `stopFirstTurnEarly()` / `stopFirstTurnLate()` in the lab artifact, gated on a
`firstTurnDispatched` flag that flips true when the simulated turn starts doing work (~900ms into
the demo timeline — arbitrary, standing in for "the CLI picked it up").

### 5.1 How the post-dispatch stop is recorded

First pass reused `pillHalted` (the dock's warning-pill treatment) for this too. That pill is
real — verified earlier against `agent-chat-pane.tsx` for the case it actually covers, a
provider-stated halt reason sourced from a real `notice` message (rate limit, needs-review). A
plain user-initiated Stop has no such message and no real precedent at all — grepped `api/` and
`web/src/` for "Interrupted": the only hit is an unrelated test file. Reusing the pill there was
an invention, not a fidelity match.

Second pass replaced it with a monospace "└ Interrupted · ..." line styled after Claude Code's
own CLI. Wrong in a different way — that borrowed a *different product's* visual identity rather
than Crowbar's own, which reads as exactly the kind of fabrication this whole exercise has been
trying to root out, just imported from somewhere else instead of invented from nothing.

**Proposed instead:** Crowbar already has a real, shipped pattern for "an event happened at this
point in the transcript, not a message" — `CompactionDivider` (`compaction-divider.tsx`): a
horizontal rule with a centered pill tag, `<div class="divider" role="separator"><span
class="ln"/><span class="tag">Compacted</span><span class="ln"/></div>`. This reuses that
markup byte-for-byte, with the tag reading "Interrupted" — the one difference between the two
is text, exactly how `CompactionDivider` itself already distinguishes "Compacted" from
"Compacted automatically." No new visual language, no new CSS.

The "what happens next" nudge moves into the composer's own placeholder ("What should the agent
do next?") instead of living in the marker — placeholder text that varies by context is already
real (the empty document's "Describe the change…" vs. the running composer's "Message the
agent…"), so this reuses an existing mechanism rather than adding a new one.

Prototype: `interruptDivider()` in the lab artifact, used by both `stopFirstTurnLate()` and the
static "First turn: stopped after dispatch" state. `pillHalted` itself is untouched — it still
backs the separate, verified "Halted" state.
