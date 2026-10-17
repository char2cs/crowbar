# S2c — Outbox, scroll and the one composer: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sending is an outbox that only rows advance, with Esc mapped to `retract` (T5). The transcript shows its rows on the first frame and follows the bottom with the eased follow, with no settle gate and no measured padding (T6). One composer serves every prompt, including the first, and serializes markdown once, on send (T7).

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (R6, R7, R11, R12, R14, R17, R20, D1, D2, D4, D8). Requires T1–T4.

**Tech Stack:** React 19, zustand, `@tanstack/react-virtual`, Plate, vitest + Testing Library.

**Spec:** report §2 symptoms 5, 6, 7 and §4.2 (outbox diagram), §4.4; additions §4 H, I, §5 (Enter budget: same frame, no network).

---

### Task 5: The outbox and Esc → retract

**Goal:** A prompt is in the transcript in the same render as Enter, under the key it keeps until it settles (`p:<requestId>`). The outbox sends one prompt at a time, when the rows say the chat can take it, and hands the prompt to the rows once its `user_message` lands. Esc in an empty composer retracts the newest prompt and puts its text back (D2). The busy recheck, the 6 s terminal hint and prompt persistence are deleted. Closes symptom 5 ("Escape only acts when working && live … the queue has no cancel in-flight → back to draft transition").

**Files:**
- Create: `web/src/features/agent/stores/lib/outbox.ts` — pure transitions over `OutboxItem[]` and `TurnFacts`: `enqueue`, `nextToSend`, `onSent`, `onSendFailed`, `onRowsApplied` (drops items whose `user_message` landed), `newestRetractable`
- Create: `web/src/features/agent/stores/chat-outbox.ts` — the effects: `sendPrompt(chatId, text)`, `retractNewest(chatId): Promise<string | null>` (the text to restore, or null when it stopped instead), `retryPrompt`, `editPrompt`, `discardPrompt`, `discardUnsent(chatId)`; drains on enqueue, after every applied row frame and on `ready` (facts, never a clock); holds a feed attachment while the chat's outbox is non-empty (R17)
- Modify: `web/src/features/agent/stores/chat-store.ts` — `outbox: Record<string, OutboxItem[]>`; `applyFeedFrame` calls the drain after rows change facts
- Modify: `web/src/features/agent/transcript/rows/user-row.tsx` — draws an outbox item and a row item alike: `queued` and `sending`/`pending` show the sending mark, `failed` offers Retry and Edit, a queued item keeps today's queued-row actions; one component for every state so the key never changes component
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx` — the list is `order`, then `liveTail`, then the outbox ids not yet in `order`
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx`, `chat/use-chat-send.ts` (from T3) — Enter calls `sendPrompt`; the Escape branch (today `event.key === 'Escape' && working && live`) calls `retractNewest` and loads the returned text into the composer; `cancelUnsentPrompts` calls `discardUnsent`; the composer is an input whenever the chat exists, also while the runner spawns
- Modify: `web/src/features/agent/components/agent-chat-pane.tsx` — queue counts from the outbox; the terminal-wait banner only from the entity's `terminalWait` (R20)
- Delete (source and tests): `web/src/features/agent/hooks/use-prompt-queue.ts`, `lib/prompt-queue-persistence.ts`, `transcript/queued-row.tsx`; the `queue` reads in `api/agent-api.ts`, if any remain
- Modify: `web/knip.json` (drop `use-prompt-queue.ts`, `prompt-queue-persistence.ts`), `web/eslint.config.js` (drop `use-prompt-queue.ts` from `MAX_LINES_BASELINE`)
- Test: `web/src/__tests__/features/agent/stores/lib/outbox.test.ts`, `stores/chat-outbox.test.ts`, `transcript/rows/user-row.test.tsx`; extend `chat/agent-chat-view.test.tsx`

Old persisted queues stay in users' localStorage untouched; no code reads or migrates them.

**State machine** (report §4.2; the outbox's own states are memory only, R7):

```text
Enter ─▶ queued ──(nextToSend: facts.running == null, no pending prompt, no terminalWait)──▶ sending
sending ──(POST 202)──▶ waits for its user_message row ─▶ leaves the outbox; state read from rows:
          pending (user_message) ─▶ delivered (turn_started) ─▶ settled (prompt_settled consumed)
                                                           └─▶ refused (prompt_settled not consumed, reason ≠ retracted)
                                    any of pending/delivered ─▶ retracted (prompt_settled reason retracted, or turn_ended reason retracted)
sending ──(409 busy)──▶ queued        sending ──(other error)──▶ failed (Retry, Edit)
Esc: queued → removed, text back · sending → retractOnAck, retract when the POST resolves
     pending, or delivered without output → POST retract, text back · delivered with output → Stop, no restore
```

A retracted prompt's item leaves `order` when its turn produced no output; with output it stays, under its divider.

- [ ] **Step 1: Failing tests** (one `vi.mock('@/features/agent/api/agent-api')`; feed frames pushed through `chatStore.getState().applyFeedFrame`)

```ts
// stores/lib/outbox.test.ts
it('sends the head only when no turn runs and no prompt is pending')
it('keeps a prompt queued when the send answers busy')
it('marks a prompt failed with the service reason on any other error')
it('hands the prompt to the rows once its user_message lands')
it('sends the next prompt on the turn_ended row, never on a clock')   // vi.useFakeTimers: advancing time sends nothing
// stores/chat-outbox.test.ts
it('symptom 5: Esc on a queued prompt restores it without a request')
it('symptom 5: Esc before delivery retracts and restores the draft')
it('symptom 5: Esc while the runner is spawning retracts and restores the draft')
it('symptom 5: Esc after delivery with no output interrupts and restores')
it('stops without restoring once the turn has output')
it('retracts after the send resolves when Esc came first')            // Review Focus 3
it('restores the text when the retract answers retracted even though turn_started arrived') // Review Focus 3
it('keeps the feed attached while the outbox is non-empty')
// transcript/rows/user-row.test.tsx
it('shows the prompt in the same render as Enter')                     // no await between sendPrompt and the assertion
it('keeps the same row element from queued to settled')
it('offers Retry and Edit when a send fails')
// chat/agent-chat-view.test.tsx
it('symptom 5: the composer accepts a prompt while the runner is spawning')
it('symptom 5: Esc in an empty composer puts the retracted text back')
it('shows the terminal-wait banner only from the chat terminalWait')
```

- [ ] **Step 2: Run; they FAIL.** `cd web && bunx vitest run src/__tests__/features/agent/stores src/__tests__/features/agent/transcript/rows src/__tests__/features/agent/chat`
- [ ] **Step 3: Implement** `outbox.ts`, `chat-outbox.ts`, the store field, the user row's states, the Esc and Enter wiring; delete the hook, the persistence and the queued row.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/stores src/__tests__/features/agent/transcript src/__tests__/features/agent/chat src/__tests__/features/agent/components
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat(web): outbox driven by ledger rows; Esc retracts the newest prompt

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** `grep -rn "setInterval\|setTimeout" web/src/features/agent/stores web/src/features/agent/chat` finds nothing; Esc gives back the text in every state of the diagram that the report routes to `draft`.

---

### Task 6: Scroll without a settle gate

**Goal:** The transcript's rows are visible on the first frame; there is no visibility gate, no measured tail-room padding and no arm timer. Follow-at-bottom keeps the eased follow; pin-to-top is a spacer computed from the virtualizer's own offsets; `overflow-anchor` keeps a reader's position (R12). Closes symptom 6 ("the transcript box is sized to the whole history but hidden until a settle loop finishes … the scroll layout is measured imperatively in a feedback loop").

**Files:**
- Rewrite: `web/src/features/agent/hooks/use-transcript-anchor.ts` (879 lines) — under 300: stuck/unstuck from scroll events, follow through `createFollowScroll` when stuck and the virtualizer's total size grows, `pinItemToTop(itemId)` that returns the spacer height `max(0, viewport − header − (totalSize − itemStart))` from the virtualizer, position restore from `transcript-scroll-positions.ts`; delete `tailRoom`'s DOM measurement, `ARM_QUIET_MS`, `READER_INPUT_MS`, `PIN_SETTLE_GRACE_MS`, `armTimer`, `beginReveal`, the internal ResizeObservers that fed the loop
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx` — delete `SETTLE_QUIET_FRAMES`, the `settled` state and the hidden-until-settled style; render the spacer after the last row; the virtualizer adjusts scroll on size changes only for rows above the viewport; a feed in `status: 'error'` renders its error with a Retry that re-attaches the feed (never a blank list)
- Delete (source and tests): `web/src/features/agent/hooks/use-scroll-frame-span.ts` (its 200 ms idle timer; T13 measures scroll frames in Tauri instead); the old `use-transcript-anchor.test.tsx` (1,687 lines, R19: replaced)
- Modify: `web/knip.json` (drop `use-transcript-anchor.ts`, `transcript-scroll-positions.ts` entries when clean), `web/eslint.config.js` (drop `use-transcript-anchor.ts` and its test from `MAX_LINES_BASELINE`)
- Test: `web/src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx` (new, ≤500); extend `transcript/agent-transcript.test.tsx`; keep `hooks/lib/follow-scroll.test.ts` green unchanged

- [ ] **Step 1: Failing tests** (jsdom layout stubbed as the old anchor suite does: element sizes through `vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect')`/`offsetHeight`; rAF faked)

```ts
// transcript/agent-transcript.test.tsx
it('symptom 6: rows are visible on the first render')                  // toBeVisible on the first and last row, no frames advanced
it('symptom 6: a feed that failed shows its error and a Retry, not a blank list')
// hooks/use-transcript-anchor.test.tsx
it('follows new rows with the eased follow while stuck to the bottom')  // scrollTop takes an intermediate value on the way (no snap)
it('stops following when the reader scrolls up and resumes at the bottom')
it('pins a sent prompt to the top with a spacer from virtualizer offsets')
it('keeps the reader position when a row above the viewport grows')
it('restores the saved position when the chat view remounts')
```

- [ ] **Step 2: Run; they FAIL** (the gate hides rows until three quiet frames).
- [ ] **Step 3: Implement** the anchor rewrite and the transcript changes; delete the gate, the measured padding, the timers and the scroll-frame hook.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/hooks src/__tests__/features/agent/transcript
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "fix(web): transcript rows show on the first frame; scroll follows without a settle loop

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** no `visibility` or `opacity` gate on the list; `use-transcript-anchor.ts` has no `setTimeout` and no `ResizeObserver` of its own; the follow still eases.

---

### Task 7: One composer that serializes on send

**Goal:** Typing re-renders the composer and nothing else, and markdown is produced once, when the prompt is sent. The empty chat's first prompt uses the same composer, in the document placement (D8). Closes the keystroke half of symptom 7 ("Per keystroke: the whole draft re-serialized and the whole column re-rendered") and the last part of symptom 5 ("While the CLI is spawning the composer isn't even an input", for the first prompt).

**Files:**
- Modify: `web/src/features/agent/composer/plate/chat-markdown-editor.tsx` (521 lines) — replace `onChange(markdown)` with `onShapeChange({ empty, slashQuery })`, computed from the first and last blocks' text without serializing; the handle gains `getMarkdown()`, `setMarkdown(text)` (the retract restore), `clear()`; under 500 lines
- Modify: `web/src/features/agent/composer/agent-composer.tsx`, `composer/composer-field.tsx`, `composer/lib/composer-state.ts` — the draft string prop and `onDraftChange` are deleted; send reads `getMarkdown()` once; a `placement: 'document' | 'dock'` prop renders the one editor instance in either position without remounting it
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx`, `chat/use-chat-send.ts` — `const [draft, setDraft]` is deleted; the slash picker, prompt history, stash and the Esc-empty check read the composer's shape; T5's restore calls `setMarkdown`
- Modify: `web/src/features/agent/chat/agent-empty-document.tsx` — keeps the empty-chat layout and signpost; its own editor, attachment upload, Excalidraw takeover, drop handling and plus button are deleted, and it hosts the one composer in `placement="document"`; `lastLineTop` stays the arrival animation's origin on first send
- Modify: `web/src/features/agent/hooks/use-prompt-history.ts`, `hooks/use-slash-catalog.ts` — inputs from the composer's shape (no markdown)
- Modify: `web/knip.json` (drop `agent-empty-document.tsx` if clean), `web/eslint.config.js` (drop `agent-empty-document.test.tsx`, `chat-markdown-editor.test.tsx` from `MAX_LINES_BASELINE` once their replacements are under the cap)
- Test: extend `web/src/__tests__/features/agent/composer/agent-composer.test.tsx` (bring under 500 per R19: cases for deleted props go), `composer/plate/chat-markdown-editor.test.tsx` (likewise), `chat/agent-empty-document.test.tsx` (likewise), `chat/agent-chat-view.test.tsx`

- [ ] **Step 1: Failing tests** (`vi.mock('@/features/agent/composer/plate/chat-composer-serialization', …)` wrapping the real module with a call counter; `vi.mock('@/features/agent/transcript/agent-transcript', …)` with a render counter)

```ts
// chat/agent-chat-view.test.tsx
it('symptom 7: typing re-renders neither the transcript nor serializes the draft') // 20 characters typed → 0 serializations, 0 transcript renders
it('serializes the draft exactly once on send')
// chat/agent-empty-document.test.tsx
it('symptom 5: the first prompt is typed into the one composer and sends through the outbox')
it('keeps the composer mounted when the first send moves it to the dock') // same editable node before and after
// composer/plate/chat-markdown-editor.test.tsx
it('reports empty and the slash query without producing markdown')
it('loads restored markdown through setMarkdown')
```

Type through the same input path `chat-markdown-editor.test.tsx` already uses. `agent-chat-view.test.tsx` stays at ≤3 `vi.mock`: its feed frames go through `chatStore.getState().applyFeedFrame`, never a mocked socket, leaving room for the API mock and these two.

- [ ] **Step 2: Run; they FAIL.**
- [ ] **Step 3: Implement** the shape callback and handle, the placement prop, the empty-document cutover; delete the draft state and the duplicated editor code.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/composer src/__tests__/features/agent/chat src/__tests__/features/agent/hooks
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "perf(web): one composer serializes on send; typing re-renders only the composer

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** a keystroke produces one composer render and zero markdown serializations; there is one Plate composer editor in `features/agent`.
