# S2b — The ChatStore and the feed: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A chat view renders from one window-level store filled by the chat's feed: rows folded into items with stable ids and one order key, live text in a side channel, every REST read and poll gone (T3). Chat entities, busy, telemetry and providers then leave the workspace stores, and one lifecycle subscription per endpoint fills the same ChatStore (T4).

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (shared vocabulary, R7–R11, R15–R17, R19, R21–R23). Requires T1, T2.

**Tech Stack:** React 19, zustand `createStore` + `useStore`, `@tanstack/react-virtual`, Plate, vitest + Testing Library.

**Spec:** report §2 symptoms 4 and 7, §4.4; architecture §4 ("The one stream"); additions §3.2, §5 (rules that keep the budgets).

Before `bun install` or a build: `df -h /System/Volumes/Data`; under 8 GB free, stop and report.

---

### Task 3: The transcript comes from the feed

**Goal:** A chat view attaches to `GET /v0/chats/:chatId/feed?since=N`, folds rows into items keyed by stable id (R8) and renders one component per item kind. Live deltas update one item's live text. The ledger poll, the activity poll, the streaming copy in the workspace slice, the activity-components projection and every REST transcript read are deleted. A settling reply keeps its mounted renderer (R11). Closes symptom 4 (nothing renders after send: the transcript was only polled and gated on the chat being "known") and the client half of symptom 7 (each delta re-parsed and re-sorted the whole transcript).

**Files:**
- Create: `web/src/features/agent/api/chat-rows.ts` — the wire types (index, "Shared vocabulary")
- Create: `web/src/features/agent/stores/chat-store.ts` — `chatStore`, `useChatStore`, `feeds`, `applyFeedFrame(chatId, frame)`, `dropFeed(chatId)`
- Create: `web/src/features/agent/stores/lib/transcript-items.ts` — the `TranscriptItem` union for `user`, `text`, `tool`, `ask`, `marker` and the id functions of R8
- Create: `web/src/features/agent/stores/lib/fold-rows.ts` — `emptyFeed()`, `applyRow(feed, row)`, facts
- Create: `web/src/features/agent/stores/lib/fold-live.ts` — `applyDelta(feed, delta)`, text assembly from chunks (S1 `Messages` rule), epoch reset
- Create: `web/src/features/agent/stores/chat-feed.ts` — `attachFeed(chatId): () => void` (refcounted, R17), the `wsManager` subscription with the R15 URL resolver, rows before `ready` folded into a draft feed and published once at `ready`, deltas coalesced per animation frame (`requestAnimationFrame`; a row frame flushes pending deltas first)
- Create: `web/src/features/agent/hooks/use-chat-feed.ts` — `useChatFeed(chatId)` (attach in an effect), `useChatOrder(chatId)`, `useChatItem(chatId, id)`, `useLiveText(chatId, id)`, `useTurnFacts(chatId)`, `useFeedStatus(chatId)`
- Create: `web/src/features/agent/transcript/rows/transcript-row.tsx` (switch by item kind), `rows/user-row.tsx`, `rows/text-row.tsx`, `rows/tool-row.tsx`, `rows/tool-payload.tsx`, `rows/marker-row.tsx`
- Rewrite: `web/src/features/agent/transcript/agent-transcript.tsx` — its virtualizer, anchor wiring and settle gate are kept as they are (T6 owns scroll); its data inputs become `useChatOrder` plus the live tail; under 500 lines
- Rewrite: `web/src/features/agent/chat/agent-chat-view.tsx` — reads facts, asks and live text from the store by `chatId`; the data props listed below are deleted; under 500 lines (split by responsibility, e.g. the queue/composer wiring into `chat/use-chat-send.ts`)
- Modify: `web/src/features/agent/components/agent-chat-pane.tsx` — stops reading `streaming*`, `choices`, `compacting`, `turnRevision`, `settledPrompts`, `abandonedPrompts` and stops passing them; under 500 lines (split along its existing sections, each file named for what it exports)
- Modify: `web/src/features/agent/api/agent-api.ts` — delete `getChatMessages`, `getChatActivity`, `getToolPayload`, the pending-choice and pending-prompt reads, `AgentActivity`, `AgentToolCall`, `AgentSubagent`, `AgentInterruption` and the ledger page types; keep `AgentChoice*` (the answer API and `composer-choice.tsx` use them); add `retractPrompt` and `readBlob`; under 500 lines (split `api/agent-api.ts` (chats, commands) from `api/agent-providers-api.ts` or by what the remaining exports group into)
- Modify: `web/src/features/agent/composer/lib/composer-state.ts` — asks come from the fold (`AgentChoice`-shaped `ask` items with `pending`), not `pendingChoices(activity)`
- Modify: `web/src/features/agent/activity/working-line.tsx` — inputs from `useTurnFacts` and the live reasoning; behaviour unchanged (T9 rewrites it)
- Modify: `web/src/features/agent/lib/agent-activity.ts` — every helper that takes `AgentActivity` is rewritten over items and facts (`choiceDetail(ask, tool?)`, `runningTools(facts, items)`) or deleted; what survives serves `tool-row.tsx`, `working-line.tsx` and `composer-choice.tsx` until T8–T10
- Modify: `web/src/features/agent/composer/composer-choice.tsx` (599 lines) — its `activity` prop becomes the ask list and the bound tool item; split under 500 per R19 (the card in `composer-choice.tsx`, the option/question controls in `composer-choice-controls.tsx`); T10 reworks the card itself
- Modify: `web/src/features/workspace/stores/slices/agent-chats-slice.ts` — delete `streamingMessages`, `streamingReasoning`, `streamingToolOutput`, `streamingPlan`, `streamingDiff`, `choices`, `compacting`, `turnRevision`, `settledPrompts`, `abandonedPrompts`, `notifyAgentChatMessages` and their setters; under 500 lines
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts` — delete the `message_delta`, `plan`, `choice`, `compaction_*`, `prompt_settled` cases and the batcher
- Modify: `web/src/lib/ws/manager.ts` — `subscribe(endpoint, cb, url?: () => string)`; `open` uses `url?.() ?? endpoint` for the transport (R15)
- Modify: `web/src/lib/boot.ts` — drop `prefetchVisibleChats`
- Unchanged: `web/src/features/agent/hooks/use-prompt-queue.ts` (692 lines; editing it would force R19). `agent-chat-view` derives its existing `working`, `settledPrompts`, `abandonedPrompts` options from `facts.running` and `facts.prompts`; T5 replaces the hook
- Delete (source and tests): `web/src/features/agent/hooks/use-chat-messages.ts`, `hooks/use-agent-activity.ts`, `stores/chat-ledger-store.ts`, `lib/prefetch-visible-chats.ts`, `lib/activity-components.ts`, `transcript/activity-component-row.tsx`, `transcript/lib/flatten-transcript-rows.ts`, `transcript/message-row.tsx` (split into `user-row`/`text-row`), `transcript/event-divider.tsx` (into `marker-row`), `transcript/turn-tools.tsx` (into `tool-row`), `transcript/tool-payload-panel.tsx` (into `tool-payload`), `web/src/features/workspace/stores/streaming-message-batcher.ts`; the old `agent-transcript.test.tsx`, `agent-transcript.scale.test.tsx`, `agent-chat-view.test.tsx`, `message-row.test.tsx`, `message-row-memo.test.tsx`, `turn-tools.test.tsx`, `use-chat-messages.test.ts`, `use-agent-activity.test.tsx` (R19: replaced, their surviving cases move to the new files)
- Modify: `web/knip.json` — drop the entries for deleted files (`activity-components`, `agent-transcript.tsx`, `turn-tools.tsx`, `agent-api.ts` if clean); `web/eslint.config.js` — drop the `MAX_LINES_BASELINE` entries of every file brought under the cap and the `EXHAUSTIVE_DEPS_BASELINE` entry for `agent-transcript.tsx` once the rewrite clears it
- Test: `web/src/__tests__/features/agent/stores/chat-store.test.ts`, `stores/lib/fold-rows.test.ts`, `stores/lib/fold-live.test.ts`, `stores/chat-feed.test.ts`, `transcript/agent-transcript.test.tsx` (new, ≤500), `transcript/rows/text-row.test.tsx`, `transcript/rows/tool-row.test.tsx`, `chat/agent-chat-view.test.tsx` (new, ≤500); extend `web/src/__tests__/lib/ws/manager.test.ts`

**Props deleted from `AgentChatViewProps`:** `working`, `compacting`, `turnRevision`, `settledPrompts`, `abandonedPrompts`, `streamingMessages`, `reasoning`, `toolOutput`, `plan`, `pendingChoices`, `diff`, `onStreamingSettled`, `onRefreshChat` (a reconnect is the feed's, not the view's).

**Item fields this task defines** (`transcript-items.ts`):

```ts
interface ItemBase { id: string; seq: number; turnId?: string; liveBorn: boolean } // liveBorn: first row or delta arrived after `ready`
export interface UserItem extends ItemBase { kind: 'user'; text: string; origin: 'crowbar' | 'provider'; requestId?: string }
export interface TextItem extends ItemBase { kind: 'text'; messageId: string; chunks: Record<number, string>; finalIndex?: number; complete?: string; text: string; settled: boolean; phase: '' | 'commentary' }
export interface ToolItem extends ItemBase { kind: 'tool'; toolId: string; scope: string; body: ToolCallBody; status: 'running' | 'ok' | 'error' | 'declined' | 'interrupted' | 'abandoned' }
export interface AskItem extends ItemBase, AgentChoice { kind: 'ask' } // AgentChoice.id = askId; pending until ask_resolved
export interface MarkerItem extends ItemBase { kind: 'marker'; marker: MarkerKind | 'turn_interrupted' | 'turn_failed' | 'session_lost' | 'runner_exited'; detail?: string; reason?: string }
```

A tool whose turn closed while it was running reads `abandoned` (derived in the fold from the named turn's `turn_ended`, never swept). Only main-scope rows become top-level items in this task; scoped rows are kept in the feed (`feed.scoped[scope]`) for T9's card.

- [ ] **Step 1: Failing tests** (fake `wsManager` with one `vi.mock('@/lib/ws/manager')`; frames are pushed synchronously; rAF faked with `vi.useFakeTimers({ toFake: ['requestAnimationFrame'] })`)

```ts
// stores/lib/fold-rows.test.ts
it('orders items by the seq of their first row and never re-sorts')
it('keeps the p:<requestId> id from user_message through settle')
it('folds numbered chunks into one text and settles on the final chunk')
it('prefers the complete row over chunks once it lands')
it('reads a tool still running when its turn ended as abandoned')
it('applies a row once when it arrives in replay and live')            // Review Focus 1
it('keeps the identity of untouched items, order and facts')            // structural sharing
// stores/lib/fold-live.test.ts
it('appends a delta when n follows the last and ignores a repeated n')
it('replaces the live text on a replace frame')
it('drops live text when ready carries a new epoch')                    // Review Focus 4
it('symptom 7: a delta changes only its own item live text')            // order, items, facts keep identity
// stores/chat-feed.test.ts
it('attaches once for two views of one chat and detaches with the last')
it('resumes with since=seq after a 4000 close')                         // Review Focus 1
it('drops the feed on a deleted frame')
it('publishes replayed rows once, at ready')
it('symptom 4: a missed prompt_settled arrives on resume since=seq')
// transcript/agent-transcript.test.tsx (vi.mock of rows/tool-row with a render counter)
it('symptom 7: a delta re-renders only its streaming row')
it('folds 5000 replayed rows into one order without re-rendering per row') // Review Focus 5, React.Profiler commit count ≤ 3
// transcript/rows/text-row.test.tsx
it('keeps the live renderer mounted when a streamed reply settles')     // Review Focus 2: same DOM node before/after
it('renders a reply that was settled at mount statically')
// chat/agent-chat-view.test.tsx
it('symptom 4: renders rows for a chat no workspace lists')             // no entity in any store; rows appear
it('symptom 4: renders the reply to a sent prompt with no transcript request') // vi.spyOn(globalThis, 'fetch'): no /messages or /activity call
// lib/ws/manager.test.ts
it('reopens with the url resolver evaluated at reconnect time')
```

- [ ] **Step 2: Run; they FAIL.** `cd web && bunx vitest run src/__tests__/features/agent/stores src/__tests__/features/agent/transcript src/__tests__/features/agent/chat/agent-chat-view.test.tsx src/__tests__/lib/ws/manager.test.ts`
- [ ] **Step 3: Implement the store, fold and feed client.** `applyRow` is a pure function of `(feed, row)` and ignores a row whose `seq` is not above `feed.seq`; the store's action does `set({ feeds: { ...feeds, [chatId]: next } })` only when `next !== feed`. A text item's `text` is recomputed only for the item a row touches.
- [ ] **Step 4: Port the row renderers.** `user-row` and `text-row` take today's `message-row.tsx` visuals (turn bar, copy, time, arrival animation for `liveBorn`), with R11: `const [live] = useState(() => !item.settled)` picks `MarkdownMessage` or `MarkdownMessageStatic` once; the live renderer reads `useLiveText` while unsettled and `item.text` after. `tool-row` takes today's per-tool presentation from `turn-tools.tsx` and `lib/agent-activity.ts` (status glyph, label, payload expand) for one `ToolItem`; `tool-payload` reads `inputRef`/`resultRef` through `readBlob`. `marker-row` takes `event-divider.tsx`. Each row element carries `data-row-id={item.id}` (the live check and tests find rows by it).
- [ ] **Step 5: Cut the view over.** Rewrite `agent-transcript.tsx`'s inputs and `agent-chat-view.tsx`; delete the props listed above in `agent-chat-pane.tsx`; delete the slice's event half and the stream hook's event cases; delete the files listed. Bring every edited file under 500 lines and its comments to R18.
- [ ] **Step 6: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent src/__tests__/features/workspace/stores src/__tests__/lib/ws
cd web && bun tsc --noEmit && bunx eslint src/features/agent src/features/workspace src/lib --max-warnings 0
cd web && bunx prettier --check src/features/agent src/features/workspace src/lib src/__tests__/features/agent && bun run knip && bun run lint:baselines && bun run lint:doctor
```

(`src/__tests__/features/agent` is the module's own suite, not the full suite.)

- [ ] **Step 7: Commit**

```bash
git add web
git commit -m "feat(web): chat transcript renders from the per-chat feed; polls and streaming copy deleted

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** with the network panel open, a chat view makes no transcript or activity request after attaching; a streamed reply is one row from first token to settle; the only timers left in `features/agent` are in `use-prompt-queue.ts` (T5), the anchor and scroll-frame hooks (T6), the working-line and subagent-shelf clocks (T9) and `use-slash-catalog.ts`.

---

### Task 4: Chat entities move to the ChatStore

**Goal:** Chat metadata, `working`, `terminalWait`, telemetry and the provider list are read from window-level stores. One lifecycle subscription per endpoint replaces one per mounted workspace (R16). The workspace chat slice, its stream hook and the registry's busy scans are deleted, and so are the pane-session and telemetry hooks whose facts now come from rows.

**Files:**
- Modify: `web/src/features/agent/stores/chat-store.ts` — `chats: Record<string, AgentChat>`, `byWorkspace: Record<string, string[]>` (kept in the reducer so no selector builds an array), `scopes: Record<string, 'seeding' | 'ready' | 'error'>`, `telemetry: Record<string, AgentTelemetry | null>`
- Create: `web/src/features/agent/stores/chat-scopes.ts` — `attachScope(wsId): () => void` refcounted per lifecycle endpoint (`${chatBase(wsId)}/ws`); seeds the endpoint's chats once with `listChats`; applies frames through `reduceChatFrame`; on `{ reconnected: true }` reseeds (a fact, not a timer); keeps `adoptIfViewless` and `releaseRunner` behaviour; loads providers through `useAgentProvidersStore.getState().load(wsId)` when its status is `idle`
- Move: `web/src/features/agent/lib/reduce-chat-frame.ts` → `web/src/features/agent/stores/lib/reduce-chat-frame.ts` (and its test)
- Create: `web/src/features/agent/hooks/use-chat-scope.ts` — `useChatScope(wsId)`; `web/src/features/agent/hooks/use-chat-entity.ts` — `useChatField(chatId, pick)`, `useChatWorking(chatId)`, `useWorkspaceChatIds(wsId)`, `useChatTelemetry(chatId)`
- Create: `web/src/features/agent/composer/lib/excalidraw-edit-requests.ts` — a small zustand store keyed by chat id, replacing the slice's `excalidrawEditRequests`
- Modify (readers, one selector swap each): `web/src/components/sidebar/sidebar-tree.tsx`, `components/sidebar/lib/sidebar-drop-policy.ts`, `components/sidebar/lib/use-recents-chat.ts`, `web/src/features/tabs/components/chat-tab-item.tsx`, `features/tabs/components/chat-branch-header.tsx`, `web/src/features/git/components/review-thread-item.tsx` (split under 500, R19; git-review must not grow), `web/src/features/panes/hooks/use-pane-keyboard.ts`, `web/src/features/workspace/components/workspace-view.tsx` (mounts `useChatScope`), `web/src/features/agent/components/agent-chat-pane.tsx`, `features/agent/composer/agent-composer.tsx`, `features/agent/composer/plate/attachments/excalidraw-preview.tsx`, `features/agent/hooks/use-chat-title.ts`, `features/agent/hooks/lib/transcript-scroll-positions.ts`, `features/agent/components/agent-terminal-wait-banner.tsx`, `features/settings/stores/agent-providers-store.ts` (readers only; no code added)
- Modify: `web/src/features/workspace/stores/workspace-store.ts`, `workspace-store.types.ts`, `workspace-store-registry.ts` (delete `readChatWorking`, `isChatWorking`, `subscribeChatWorking` and the `agentChats` scans), `web/src/features/panes/stores/window-pane-store.types.ts` (comment)
- Delete (source and tests): `web/src/features/workspace/stores/slices/agent-chats-slice.ts`, `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`, `web/src/features/agent/hooks/use-pane-session.ts`, `lib/session-status.ts`, `hooks/use-agent-telemetry.ts`, `tree/lib/chat-rows.ts`, `tree/lib/chat-search.ts`; `setActiveAgentChatId` callers (`use-pane-keyboard.ts`, `review-thread-item.tsx`: nothing reads `activeChatId`); `selectEnabledProviders` (readers use the providers store); `followRunner` and the `moved` case (R22)
- Modify: `web/src/features/agent/components/agent-chat-pane.tsx` — exit and resume state from `facts.lastExit` and `facts.sessionLost` (T3's fold) instead of `use-pane-session`
- Modify: `web/knip.json` (drop `agent-chats-slice.ts`, `use-workspace-agent-chats-stream.ts`, `tree/lib/chat-rows.ts`, `tree/lib/chat-search.ts`), `web/eslint.config.js` (drop the slice's and the stream hook's `MAX_LINES_BASELINE` entries and their tests')
- Test: extend `web/src/__tests__/features/agent/stores/chat-store.test.ts`; create `stores/chat-scopes.test.ts`, `hooks/use-chat-entity.test.ts`; update the reader tests (`components/sidebar/sidebar-tree.test.tsx`, `features/tabs/components/chat-tab-item.test.tsx`, the pane tests) to seed `chatStore` instead of a workspace store; fold `agent-chat-pane-terminal-wait.test.tsx`, `-unknown-chat-wedge`, `-provider-catalog`, `-split` into `agent-chat-pane.test.tsx` cases that still apply (R19; each file ≤500)

- [ ] **Step 1: Failing tests**

```ts
// stores/chat-scopes.test.ts
it('subscribes once per lifecycle endpoint for two workspaces of one repo')
it('applies a lifecycle frame once however many workspaces are mounted')   // today K copies
it('unsubscribes when the last workspace of the endpoint unmounts')
it('reseeds an endpoint after the socket reports reconnected')
it('adopts a chat that starts working with no pane as a background chat')
// stores/chat-store.test.ts
it('indexes chats by workspace without building arrays in selectors')
it('keeps the newer telemetry when an older observedAt arrives')
// hooks/use-chat-entity.test.ts
it('re-renders a sidebar row only when its own chat working flips')
// components/agent-chat-pane.test.tsx
it('shows the exit reason from the last runner_exited row')
```

- [ ] **Step 2: Run; they FAIL.**
- [ ] **Step 3: Implement** the entity half of the store, `chat-scopes.ts`, the hooks; swap every reader; delete the slice, the stream hook, the registry scans and the files listed; bring every edited file under 500 lines and its comments to R18.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent src/__tests__/features/workspace src/__tests__/features/tabs src/__tests__/features/panes src/__tests__/features/git/components src/__tests__/components/sidebar src/__tests__/features/settings/stores
cd web && bun tsc --noEmit && bunx eslint src --max-warnings 0 && bunx prettier --check src
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit** (body lists the net line change of `web.sidebar`, `web.git-review`, `web.workspace-panes`, `web.settings`, R1)

```bash
git add web
git commit -m "refactor(web): chat entities live in the window ChatStore; workspace chat slice deleted

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** `grep -rn agentChats web/src` finds nothing; a repo with three mounted workspaces has one lifecycle socket callback; no workspace store holds chat state.
