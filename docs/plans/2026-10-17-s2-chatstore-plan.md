# S2 — Window-level ChatStore fed by push: Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks are strictly sequential: each one assumes every earlier task, and all of S1, is committed.

**Goal:** One window-level `ChatStore`, filled only by S1's per-chat feed (rows by `seq`, plus an ephemeral delta side channel) and the scope lifecycle feed. It renders each row kind with one renderer under a stable key, and sends prompts through an outbox state machine that only ledger facts advance, with `retract` so Esc after send gives the prompt back. Every chat poll, the streaming copy, client prompt persistence, the chat slice in workspace stores, remount-on-settle and the daemon's R13 read projection are deleted.

**Architecture:** `features/agent/stores/` holds the ChatStore: lifecycle entities per chat (`chats`), a per-chat feed state (`feeds`: rows folded into items keyed by stable id, one order key, turn facts, live text) and the outbox. Every component subscribes with a narrow selector to the one slice it draws, so a delta re-renders its streaming row and nothing else. The daemon gains three small contract changes (append-only deltas, `answerable` on asks, a blob read route) and one verb (`retract`); S2 ends by deleting the read projection S1 kept for the old client.

**Tech Stack:** React 19, zustand (vanilla `createStore` + a selector hook), `@tanstack/react-virtual`, Plate (the only markdown renderer), `@/components/ui/*`, phosphor icons, shiki, vitest + Testing Library (`web/src/__tests__/`), Go (`api/`, `-tags noEmbed`), gin, gorilla/websocket, testify, `api/tests` black-box suite over S1's `kit.FakeProvider`.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §3, §4 ("The one stream"), §5, §11 row S2 · `docs/plans/2026-10-16-chat-surface-report.md` §2 (symptoms 4–8), §4.2 (outbox diagram), §4.4 (client) · `docs/plans/2026-10-16-chat-surface-additions-design.md` §3.2, §4 A–J, §5 budgets · the S1 plan (`docs/plans/2026-10-17-s1-chat-ledger-plan.md` and s1a–s1d): row vocabulary, R4, R5, R7, R10, R13, T10 feed. Executors read the sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s2a-daemon-contract-plan.md` | T1 feed contract for the client · T2 `retract` |
| `docs/plans/2026-10-17-s2b-store-and-feed-plan.md` | T3 the transcript comes from the feed (symptoms 4, 7) · T4 chat entities move to the ChatStore |
| `docs/plans/2026-10-17-s2c-outbox-scroll-composer-plan.md` | T5 outbox and Esc → retract (symptom 5) · T6 scroll without a settle gate (symptom 6) · T7 one composer that serializes on send (symptom 7) |
| `docs/plans/2026-10-17-s2d-rows-plan.md` | T8 tool and MCP rows (A, B) · T9 subagent card, todo strip, working line, reasoning (C, D, E, J; symptom 8) · T10 approvals, errors, interrupt, lost session (F, G, H) |
| `docs/plans/2026-10-17-s2e-deletion-and-gate-plan.md` | T11 delete the daemon's R13 projection · T12 chat-surface hygiene pass · T13 gate |

13 tasks, one commit each. T3 is the large cutover: the transcript, its live text and every REST read move to the feed together, because the old row list joins messages and tool calls that the feed delivers as one ordered stream.

---

## Global Constraints

Every task's requirements include these. Quoted from the specs, `CLAUDE.md` and the user's standing rules.

- "no feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change." Every task deletes what it replaces in its own commit.
- "Same features as today, fewer lines." Nothing a user can do today is lost by the end of S2. A task may show a simpler presentation that a later S2 task finishes (T3's tool row before T8), never a missing capability.
- "A fix starts with a failing test that reproduces it. Never fix a symptom with a timer, poll, retry, extra flag, or resync effect." Symptoms 4–8 each get a failing regression test first, in the task that closes it (names in each task).
- "No timer-gated UI." The only `setTimeout`/`setInterval` left in `web/src/features/agent` at the end are the allowlist in R14.
- "Plate is the only markdown renderer, never react-markdown." Rendering goes onto `@/components/ui/*` with CSS-variable tokens, phosphor icons, shiki for code.
- Preserve every animation: the per-word `chat-fresh-text` fade (`transcript/plate/streaming-value-patch.ts`, `chat-fresh-text-plugin.tsx`), the eased follow (`hooks/lib/follow-scroll.ts`), the arrival slide (`chat/lib/arrival-animation.ts`, `data-arriving`), the working-line verb rotation, shimmer on live verbs. "A snap is a defect." Never `content-visibility` (it caused a CPU melt).
- Store patterns: `useChatStore((s) => s.narrow.field)` with a narrow selector, never a bare `useChatStore()`; `chatStore.getState()` only inside event handlers and `useEffect` bodies; stores never import from `components/` or any `*.tsx`; a selector never returns a fresh array or object (keep indexes in the store, or use `useShallow`).
- Files: kebab-case component files; every file a task creates or edits is ≤500 lines when the task commits (R19). Comments explain *why* in ≤3 lines (R18).
- Tests: in `web/src/__tests__/` mirroring `src`, `@/` imports, extend the unit's test file, ≤3 `vi.mock` per file, no Tailwind-class assertions, no reading `src/` text, no sleeps (`waitFor`, fake timers, resolved promises). Go: one test file per source file, `require.Eventually`/channels, no `time.Sleep`, every backend bug gets a black-box `TestRegression_*`.
- Running tests: targeted only, never the full suite. Web: `cd web && bunx vitest run <paths>`. Types: `cd web && bun tsc --noEmit` (never `bunx tsc`: that resolves a different package). Before each web commit also: `bunx eslint <touched paths> --max-warnings 0`, `bunx prettier --check <touched paths>`, `bun run knip`, `bun run lint:baselines`, `bun run lint:doctor`. Go: `cd api && go test -tags noEmbed ./internal/<pkg>/...`, black box `go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/`, then `make lint`, `make deadcode`, `go vet -tags noEmbed ./...`.
- Baselines only shrink. A task that fixes a file listed in `web/knip.json` `ignoreIssues`, a `*_BASELINE` list in `web/eslint.config.js` or `api/deadcode-baseline.txt` deletes that entry in the same commit (`check-knip.mjs` and `check-eslint-baselines.mjs` fail on a stale entry). Never add an entry.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state`; dev runs use the worktree's `CROWBAR_HOME` (`make dev`, `make dev-desktop`). Never read or write `~/.claude` or `~/.codex`. Real provider CLIs only in scratch dirs with `env -i`. Check `df -h /System/Volumes/Data` before any build or `bun install`; under 8 GB free, stop and report. Never bare `git stash`. Never push. No scratch files inside the worktree (sibling sessions commit with `git add -A`); scratch goes in the session scratchpad.

## Rulings (open decisions resolved for S2)

| # | Decision | Ruling |
|---|---|---|
| R1 | `make ratchet` per task | Tasks do not run it: `runGate` fails on any unwritten shrink, so it would fail after every deleting task. Each task checks the 500-line cap with `wc -l` on what it touched. T13 runs `make ratchet ARGS=--update` once against the post-S1 baselines, then `make ratchet`, and commits the lower numbers. A task that edits files outside `web/src/features/agent` must not grow that file's module: it states the net line change per module in its commit body. |
| R2 | Where the ChatStore lives | `web/src/features/agent/stores/` (chat-surface). `CLAUDE.md` says server-adjacent "conversations" go in `lib/store/`, but `features/agent/stores/chat-ledger-store.ts` already sets this precedent, and moving chat code into the sidebar module would hide it from the chat-surface budget. |
| R3 | Delta frames (S1 `FeedDelta.text` is everything so far, which is O(L²) on the wire: symptom 7) | T1: `delta{id, turnId, scope, stream, n, text, replace}`. `n` is the frame's 1-based position in its stream; `text` is only the new suffix, or everything so far when `replace` is true (a non-prefix rewrite, or the snapshot every live stream gets right after `ready`). The client appends when `n == last+1`, replaces on `replace`, ignores `n <= last`. `id` is the message id for `answer`/`reasoning` and the tool id for `tool_output`/`diff`. |
| R4 | Whether Crowbar can answer an ask (S1 rows lack it; the `/choices` GET carried it) | T1: the `ask` body gains `answerable` (bool), set by the mapper from its hold decision. It never changes on the row: a CLI that hangs up, the bound tool's post, or the turn ending each write `ask_resolved`. |
| R5 | Tool input/output after `/activity/:toolId/payload` goes | T1: `GET /v0/chats/:chatId/blobs/:ref` on the flat `chats` group (no worktree needed). `ref` must be 64 lowercase hex (else 400); unknown chat or swept blob → 404. Blobs are content-addressed and local to this user's daemon, so cross-chat reads are not a privilege boundary; the chat id only scopes the URL. |
| R6 | `retract` (report §4.2, additions H; S1 has `TurnRetracted` with no writer) | T2: `POST …/chats/:id/prompts/:requestId/retract`. Pending prompt → `prompt_settled{consumed:false, reason: retracted}`, and a runner started with that prompt (`delivery: restart`) is terminated (`runner_exited{reason: retracted}`); response `{state: "retracted", text}`. Delivered with its turn running → `turn_ended{interrupted, reason: retracted}`, then the provider is signalled exactly as Stop does (S1 R10); response `{state: "interrupted", text}`. Settled prompt or closed turn → 409. Unknown request → 404. A provider acknowledgement that lands after a retract is recorded as what it is: a provider-origin `user_message` and its `turn_started`. `TurnRetracted` is deleted (the reason field carries it). |
| R7 | Who owns the outbox | The client, in the ChatStore, per architecture §5 and additions §I: `queued` and `sending` exist only in memory; from the `user_message` row on, the prompt's state is read from rows (`pending → delivered → settled | refused | retracted`). Persistence is deleted: an unsent prompt does not survive an app reload (decision D1). |
| R8 | Item identity and order | Item ids: `p:<requestId>` (Crowbar prompt; the same key from the optimistic echo through settle), `u:<seq>` (provider-origin prompt), `m:<turnId>:<messageId>` (assistant text), `r:<turnId>:<messageId>` (reasoning), `t:<toolId>` (tool), `s:<spawnToolId>` (subagent card, keyed by the call that spawned it so binding never changes the key; it takes that call's place, and the spawning `tool_call` makes no tool item; a subagent with no spawning call: `s:<scope>`), `a:<askId>` (ask), `k:<seq>` (marker, turn end, failure, lost session). Order is the `seq` of the item's first row and never changes. An item known only from live deltas sits in a live tail after the ordered list and moves into the order, same key, when its first row lands. Rows in a subagent scope are the card's children, never top-level items. |
| R9 | Busy | The pane reads turn facts from its own fold of rows (running turn, pending prompts, open asks, live subagents). Sidebar and tabs, which have no feed, read the entity's server-folded `working` from lifecycle frames. No client code writes `working`. |
| R10 | Telemetry and providers | Telemetry moves to `ChatStore.telemetry[chatId]`: the lifecycle `telemetry` frame, plus one GET when a feed attaches; a value with an older `observedAt` never replaces a newer one. Providers are read from the window-level `useAgentProvidersStore`; the per-workspace copy and its 3× retry + toast are deleted (D5). |
| R11 | Remount on settle | A text row picks its renderer once, at mount: an item that is not settled when its row mounts keeps the live Plate editor (with the fresh-word fade) until that row unmounts; an item settled at mount renders static. Settling changes the item's data, never the component. Virtualizer unmount/remount on scroll is not a settle. |
| R12 | Scroll anchoring: report §4.4 says "CSS bottom anchoring", the user's rule says preserve the eased follow | Keep `hooks/lib/follow-scroll.ts` for follow-at-bottom (a rAF animation, not a timer). Delete the reveal/visibility settle gate (`SETTLE_QUIET_FRAMES`), the measured tail-room padding, `ARM_QUIET_MS`, `READER_INPUT_MS`, `PIN_SETTLE_GRACE_MS`, the arm timer and `use-scroll-frame-span`. Pin-to-top becomes a spacer computed from the virtualizer's own offsets (never measured DOM padding). `overflow-anchor` keeps a reader's position while rows above change size. |
| R13 | Lost session (S1 R5 leaves the affordance to S2) | A `session_lost` row renders as a marker with one action, "Start a new conversation", which calls the existing `resumeChat` (the next prompt starts fresh, as S1 R5 does). Rebinding to another session id is S8's (decision D3). |
| R14 | "Zero polling loops" | At T13 the only `setTimeout`/`setInterval` matches under `web/src/features/agent` are: the shared 1 s text clock (`activity/lib/text-clock.ts`, writes `textContent`, no re-render), the working-line verb rotation, and `use-slash-catalog`'s input debounce. Feed reconnects use `wsManager`'s existing backoff (R15); no new timer anywhere. |
| R15 | Resuming a feed with `since=seq` through `wsManager` (its URL is fixed at subscribe) | T3: `wsManager.subscribe(endpoint, cb, url?)` takes an optional `() => string` evaluated at every (re)open; the endpoint string stays the channel key. The feed passes `() => /v0/chats/<id>/feed?since=<feed.seq>`. Close 4000 (resync) reconnects through the same path; 4001 (deleted) drops the feed. |
| R16 | Lifecycle subscriptions | T4: one `wsManager` subscription per lifecycle endpoint (`${chatBase(wsId)}/ws`), refcounted by the workspace views that need it, applied by one reducer into `ChatStore.chats`. Today every mounted workspace applies every frame of its repo into its own copy (K-fold). After T11 the lifecycle feed carries entity frames only. |
| R17 | Feed lifetime | A chat's feed is attached while a view of it is mounted or its outbox is non-empty. Detaching drops its feed state; re-attaching replays from `since=0`. No LRU, no cache. |
| R18 | Comments (33% of `features/agent` is comment lines) | A task that edits a file brings that file's comments to the ≤3-line *why* rule: narratives, incident history and restated code are deleted. T12 applies the rule to the files no earlier task touched. Comment trimming never changes code. |
| R19 | 500-line cap | A task that edits a production or test file over 500 lines brings it under 500 in that task, split by responsibility and named for what each part exports. An oversize test file whose source is rewritten is replaced, not split: its cases for deleted behaviour go, the remaining ones move to the test file of the source they exercise. Outside `features/agent` (`review-thread-item.tsx` in T4), the split is offset by R18 so the module does not grow. |
| R20 | The 6 s "terminal may be waiting" hint in `use-prompt-queue.ts` | Deleted: it is a timer guessing a fact the daemon reports. The banner shows when the chat entity carries `terminalWait` (S1 keeps termwait's detection). Decision D4. |
| R21 | Command URLs | Commands (send, stop, retract, answer, resume, settings) keep today's repo/home-scoped routes, built from the chat entity's own `workspaceId`, never from an ambient workspace. The new reads (feed, blobs) are flat. Unifying the URLs is S4's. |
| R22 | Runner moves | S1 R6 removed runner moves between chats. T4 deletes `followRunner` and the `moved` frame case; if `grep -rn '"moved"' api/internal` still finds a lifecycle kind at T4, stop and report. |
| R23 | Activity components (`/activity` `components`, `lib/activity-components.ts`, `activity-component-row.tsx`, `endpoints/chat/handlers/activity_components.go`) | A second projection of the same facts. T3 deletes the client half, T11 the server half. One renderer per row kind replaces it. |
| R24 | Optional turn fold ("Worked for 42s · 9 tool calls", additions A) | Not in S2 (decision D6). |
| R25 | The live check's provider | The S1 T1 fake CLI, built into the session scratchpad; its descriptor is written into the dev home (`<worktree>/.crowbar/descriptors/`, git-ignored) from the shipped `claude.yaml` by the same edits `kit.InstallFakeProvider` makes; hooks are posted with `curl --unix-socket` to the dev daemon's socket in that home. Nothing of the driver is committed (T13). |

## Decisions that change what the user sees

Each is ruled above so the plan can proceed; the user may overturn any of them before the task that implements it.

| # | Change | Task | Alternative |
|---|---|---|---|
| D1 | A prompt queued but not yet sent is lost if the app reloads (today it survives in localStorage). | T5 | (b) a daemon-held queue with a delivery driver: survives reloads, costs a new daemon state machine. |
| D2 | Esc in an empty composer acts on your newest prompt: still queued → back into the composer; sent but not delivered → retracted, back into the composer; delivered with no output yet → the turn is interrupted and the text comes back; once output has arrived → Esc stops the turn and the text stays in the transcript. | T5 | Esc only ever stops, never restores. |
| D3 | A lost session shows "Start a new conversation" and nothing else; picking another session to rebind waits for S8. | T10 | Ship a session picker in S2. |
| D4 | The "terminal may be waiting" hint no longer appears after 6 s on its own; the banner shows only when the daemon detects a terminal prompt. | T5 | Keep a client hint (needs a timer, which the rules forbid). |
| D5 | A failed provider-list load no longer retries 3× and toasts; the provider picker shows the store's failed state. | T4 | A Retry action on that state (small, additive). |
| D6 | No "Worked for Ns · N tool calls" fold of a finished turn. | — | Add it in S2 (T8, ~80 lines). |
| D7 | Tool rows by family with run grouping and edit diffs open by default when ≤12 lines changed; subagent cards replace the Agent tool row; a todo strip docks above the composer; reasoning persists as a collapsed "Thought for Ns" row; approvals become a card stack docked over the composer; a failed turn is a row with Retry and Open terminal; an interrupted turn shows "Interrupted · you stopped this turn". | T8–T10 | These are the additions design as written; listed so they are not a surprise. |
| D8 | The first prompt of an empty chat is typed into the same composer as every other prompt, placed where the empty-chat document is today. The layout and the move to the dock on send stay; the separate first-prompt editor goes. | T7 | Keep two editors. |

## Shared vocabulary

**Wire, `web/src/features/agent/api/chat-rows.ts`** (TypeScript mirror of S1's `ledger.Row` and T10's frames, as amended by T1):

```ts
export interface LedgerRow<K extends RowKind = RowKind> {
  chatId: string
  seq: number
  kind: K
  turnId?: string
  scope?: string // '' or absent = main agent
  at: string
  body: RowBodies[K]
}
export type RowKind = keyof RowBodies
export interface RowBodies {
  user_message: { requestId: string; text: string; origin: 'crowbar' | 'provider'; delivery?: 'restart' | 'steer' | 'api' }
  turn_started: { requestId?: string; providerTurnId?: string; runnerId: string }
  prompt_settled: { requestId: string; consumed: boolean; reason: string }
  turn_ended: { status: 'done' | 'interrupted'; reason?: string; effort?: string }
  turn_failed: { reason: string; detail?: string }
  session_bound: { provider: string; sessionId: string; cause: 'assigned' | 'announced' | 'fresh'; previous?: string }
  session_lost: { provider: string; sessionId: string; reason: string }
  runner_started: { runnerId: string; provider: string; terminalSession?: string }
  runner_exited: { runnerId: string; reason: string; exitCode?: number }
  assistant_text: { messageId: string; index: number; final?: boolean; text: string; phase?: '' | 'commentary' }
  reasoning: { messageId: string; text: string }
  marker: { kind: MarkerKind; detail?: string }
  tool_call: ToolCallBody // S1 T7–T9 fields: toolId, phase, name, kind, target, locations, inputRef, resultRef, status, error, durationMs, diff, spawnsScope, title, mcpServer, mcpTool, exitCode, interrupted, addressesScope
  ask: AskBody // S1 T7/T8 fields + answerable (T1)
  ask_resolved: { askId: string; resolution: 'answered' | 'proceeded' | 'abandoned'; optionIds?: string[] }
  subagent: { phase: 'start' | 'stop' | 'bound' | 'result'; spawnToolId?: string; type?: string; title?: string; status?: string; result?: string; usage?: { tokens?: number; toolUses?: number; durationMs?: number } }
  plan: { op: 'replace' | 'upsert'; items: PlanItem[] }
}
export type FeedFrame =
  | { type: 'row'; row: LedgerRow }
  | { type: 'ready'; epoch: string; seq: number }
  | { type: 'delta'; delta: FeedDelta }
  | { type: 'resync'; seq: number }
  | { type: 'deleted' }
export interface FeedDelta {
  id: string
  turnId: string
  scope?: string
  stream: 'answer' | 'reasoning' | 'tool_output' | 'diff'
  n: number
  text: string
  replace?: boolean
}
```

**Store, `web/src/features/agent/stores/chat-store.ts`** (T3 adds `feeds`, T4 `chats`/`byWorkspace`/`scopes`/`telemetry`, T5 `outbox`):

```ts
export interface ChatFeed {
  status: 'connecting' | 'live' | 'error'
  error?: string
  epoch: string
  seq: number // last applied row
  order: string[] // item ids, by first seq
  liveTail: string[] // ids known only from deltas
  items: Record<string, TranscriptItem>
  live: Record<string, string> // item id → live text (answer, reasoning, tool output, diff)
  liveN: Record<string, number>
  facts: TurnFacts
}
export interface TurnFacts {
  running: { turnId: string; requestId?: string; startedAt: string; seq: number } | null
  prompts: Record<string, PromptFact> // requestId → state from rows
  compacting: boolean
  openAsks: string[]
  liveSubagents: string[] // scopes
  plan: Record<string, PlanItem[]> // by scope, latest
  lastExit: { runnerId: string; reason: string; exitCode?: number; seq: number } | null
  sessionLost: { provider: string; reason: string; seq: number } | null
  output: Record<string, true> // turn ids that produced text or a tool
}
export type PromptFact =
  | { state: 'pending'; seq: number }
  | { state: 'delivered'; turnId: string }
  | { state: 'settled' }
  | { state: 'refused'; reason: string }
  | { state: 'retracted' }
export interface OutboxItem {
  requestId: string
  text: string
  state: 'queued' | 'sending' | 'failed'
  error?: string
  retractOnAck?: boolean
}
export const chatStore: StoreApi<ChatStoreState>
export function useChatStore<T>(selector: (s: ChatStoreState) => T): T
```

`TranscriptItem` is a union by `kind` (`user | text | reasoning | tool | subagent | ask | marker`) with the id scheme of R8; each kind's fields are defined in the task that first renders it (T3 for user/text/tool/ask/marker, T9 for subagent/reasoning). The fold (`stores/lib/fold-rows.ts`) is pure: `applyRow(feed, row): ChatFeed` and `applyDelta(feed, delta): ChatFeed` return a new feed whose unchanged items, `order` and `facts` keep their identity (structural sharing is what makes per-row subscriptions cheap). Text follows S1's `Messages` rule: the contiguous chunks `0..k` when chunk `k` is final, else the complete row (`index: -1`), else the chunks present in index order; settled when a final chunk is contiguous, a complete row exists, or its turn ended.

## Ownership after S2

| Fact | Owner after S2 | Owner today (deleted) |
|---|---|---|
| Transcript content (text, tools, asks, markers, subagents, plan) | `ChatStore.feeds[chat].items`, folded from feed rows | `stores/chat-ledger-store.ts` REST pages + `use-chat-messages` poll; `use-agent-activity` poll + `/activity`; `lib/activity-components.ts`; workspace slice `choices`, `streamingPlan` |
| Streaming text | `feeds[chat].live`, superseded by the item's row | slice `streamingMessages/Reasoning/ToolOutput/Diff`, `streaming-message-batcher.ts`, lifecycle `message_delta` |
| Turn running, pending prompt, compaction, last exit (pane) | `feeds[chat].facts` | slice `working`, `compacting`, `turnRevision`, `settledPrompts`, `abandonedPrompts`; `use-pane-session`, `session-status.ts` |
| Busy (sidebar, tabs) | entity `working` in `ChatStore.chats`, server-folded | one copy per mounted workspace (`agentChats.working`), `readChatWorking`/`isChatWorking` scans |
| Chat entities | `ChatStore.chats` + `byWorkspace` index, one lifecycle subscription per endpoint | `agent-chats-slice.ts` per workspace, `use-workspace-agent-chats-stream.ts` per workspace |
| Unsent prompts | `ChatStore.outbox` (memory) | `use-prompt-queue.ts` + `lib/prompt-queue-persistence.ts` (localStorage) |
| Prompt state after send | rows (`user_message`, `turn_started`, `prompt_settled`) | lifecycle `prompt_settled` frames + 1 s busy recheck |
| Telemetry | `ChatStore.telemetry` | slice `telemetry` + `use-agent-telemetry.ts` |
| Providers | `useAgentProvidersStore` (window) | slice `providers` per workspace + seed retry |
| Excalidraw edit requests (composer ↔ preview) | `composer/lib/excalidraw-edit-requests.ts` (tiny store keyed by chat) | slice `excalidrawEditRequests` |
| Old read shapes | none | daemon `transcript` projector (S1 R13), `/messages`, `/activity`, payload, `/choices`, `/pending-prompt`, lifecycle event frames |

## Deletion table (S2 "Deletes" → task → files)

| Deleted | Task | Files (sources and their tests) |
|---|---|---|
| Ledger poll | T3 | `web/src/features/agent/hooks/use-chat-messages.ts`, `stores/chat-ledger-store.ts`, `lib/prefetch-visible-chats.ts` (+ its call in `web/src/lib/boot.ts`) |
| Activity poll | T3 | `hooks/use-agent-activity.ts`, `lib/activity-components.ts`, `transcript/activity-component-row.tsx`, `transcript/lib/flatten-transcript-rows.ts`, the `getChatActivity`/`getToolPayload`/`getChatMessages`/pending-choice reads in `api/agent-api.ts` |
| Busy poll | T5 | `hooks/use-prompt-queue.ts` (`BUSY_RECHECK_MS` interval, `AWAITING_TERMINAL_HINT_MS` timeout) |
| Streaming copy | T3 | slice fields `streamingMessages`, `streamingReasoning`, `streamingToolOutput`, `streamingPlan`, `streamingDiff` and their setters; `web/src/features/workspace/stores/streaming-message-batcher.ts`; the `message_delta`/`plan`/`choice`/`compaction_*`/`prompt_settled` cases in `use-workspace-agent-chats-stream.ts` |
| Client prompt persistence | T5 | `lib/prompt-queue-persistence.ts`, `transcript/queued-row.tsx` (its rendering moves into the one user row) |
| Chat slice in workspace stores | T3 (event half), T4 (rest) | `web/src/features/workspace/stores/slices/agent-chats-slice.ts`, `stores/hooks/use-workspace-agent-chats-stream.ts`, `readChatWorking`/`isChatWorking`/`subscribeChatWorking` in `workspace-store-registry.ts`, the `agentChats` key in `workspace-store.ts`/`.types.ts`; `hooks/use-pane-session.ts`, `lib/session-status.ts`, `hooks/use-agent-telemetry.ts`; dead `tree/lib/chat-rows.ts`, `tree/lib/chat-search.ts` |
| Remount on settle | T3 | the `streaming ? <MarkdownMessage> : <MarkdownMessageStatic>` swap in `transcript/message-row.tsx` (R11); index keys |
| Scroll feedback loop | T6 | the settle gate, tail-room measurement and timers in `hooks/use-transcript-anchor.ts`, `hooks/use-scroll-frame-span.ts` |
| Duplicate first-prompt composer | T7 | `chat/agent-empty-document.tsx` editor half |
| R13 read shapes + LRU projection | T3 (client), T11 (daemon) | `api/internal/app/usecases/chat/internal/transcript/` (whole package); handlers for `/messages`, `/activity`, `/activity/:toolId/payload`, `/choices` GET, `/pending-prompt`; `handlers/activity_components.go`; their DTOs in `api/internal/api/v0/dto/agent.go`; lifecycle event frames; chat DTO `session` |

## Module keys (`scripts/ratchet/modules.json`) and counters

| Key | Direction | Why |
|---|---|---|
| `web.chat-surface` | falls 21,130 → ≤ 12,000 | the gate |
| `web.workspace-panes` | falls (≈ −1,300) | slice, workspace stream hook, batcher, registry scans deleted |
| `web.sidebar` | falls slightly | `readChatWorking` subscriptions become one selector |
| `web.git-review` | must not rise | `review-thread-item.tsx` split (R19) offset by comment trimming |
| `web.core` | ≈ −4 | `wsManager` URL resolver (+4, T3), `boot.ts` prefetch (−2, T3), `lib/ws/chat-frame-kinds.ts` event kinds (−6, T11) |
| `web.settings` | must not rise | `agent-providers-store.ts` gains no code; readers only |
| `web.total` | falls | sum |
| `web.tests` | falls | old suites of deleted modules (≈ 16k lines) go; new suites are smaller |
| `go.chat` | falls | `internal/transcript/`, read handlers |
| `go.dto` | falls | activity, choice, pending-prompt, component, `session` DTOs |
| `go.http` | ≈ 0 | routes deleted (−) vs retract and blobs routes (+) |
| `go.tests` | falls | the projector's and read handlers' tests |
| `web.timers` (counter) | falls by ≥ 12 | 16 matches in `features/agent` today; 3 remain (R14) |
| `web.empty-catch`, `web.swallowed-promise` | must not rise | the feed has no swallowed errors: a failed attach is `status: 'error'` |

## Gate baseline (measured at `94d001ddd`, today's HEAD)

Committed ratchet baselines (`scripts/ratchet/*-baseline.json` at HEAD): `web.chat-surface` 21,130 · `web.workspace-panes` 14,211 · `web.sidebar` 18,562 · `web.core` 5,979 · `web.git-review` 5,890 · `web.total` 105,828 · `web.tests` 113,678 · `go.chat` 38,317 · `go.dto` 2,165 · `go.http` 10,606 · `web.timers` 87. S1 does not touch `web/`, so the web numbers are also the post-S1 numbers; the Go keys are compared against whatever S1's T13 committed.

`web/src/features/agent` today (physical lines, as the ratchet counts them; comment = lines starting `//`, `*` or `/*`):

| Directory | Lines | Comment | Code (no blank/comment) | T13 target |
|---|---|---|---|---|
| `activity` | 647 | 158 | 455 | ≤ 560 |
| `api` | 1,490 | 624 | 772 | ≤ 600 |
| `chat` | 1,944 | 654 | 1,209 | ≤ 700 |
| `components` | 1,485 | 580 | 838 | ≤ 700 |
| `composer` | 5,392 | 1,634 | 3,434 | ≤ 3,900 |
| `controls` | 877 | 187 | 649 | ≤ 720 |
| `hooks` | 3,122 | 1,249 | 1,704 | ≤ 600 |
| `lib` | 1,151 | 304 | 760 | ≤ 150 |
| `shared` | 241 | 31 | 183 | ≤ 230 |
| `stores` | 160 | 18 | 132 | ≤ 1,050 |
| `terminal` | 228 | 59 | 161 | ≤ 210 |
| `transcript` | 3,915 | 1,329 | 2,427 | ≤ 2,450 |
| `tree` | 478 | 173 | 272 | 0 |
| **total** | **21,130** | **7,000** | **12,996** | **≤ 12,000** (the targets sum to 11,870) |

Timers in `features/agent` today (16 matches of `\b(setTimeout|setInterval)\b`): `use-agent-activity.ts` 4, `use-chat-messages.ts` 2, `use-prompt-queue.ts` 2, `use-transcript-anchor.ts` 2, `use-scroll-frame-span.ts` 2, `working-line.tsx` 2, `subagent-shelf.tsx` 1, `use-slash-catalog.ts` 1.

## Review Focus

The inputs the spec implies but no feature test exercises, most likely to bite first. Each has a test in the owning task.

1. **A row committed while the feed attaches.** S1's replay dedupes on the server; the client must still apply each `seq` once and in order across a reconnect with `since`. Tests: T3 `applies a row once when it arrives in replay and live`, `resumes with since=seq after a 4000 close`.
2. **The settle that used to remount.** A text item streams, its complete row lands, the turn ends: the same DOM node must stay mounted with the live editor (R11). Test: T3 `keeps the live renderer mounted when a streamed reply settles` (asserts the row element is the same node before and after).
3. **Esc racing delivery.** Esc while the POST is in flight, and Esc at the moment `turn_started` lands. Tests: T5 `retracts after the send resolves when Esc came first`, `restores the text when the retract answers retracted even though turn_started arrived`; T2 `TestRegression_Symptom5_ALateAckAfterRetractIsRecordedAsAProviderTurn`.
4. **Daemon restart under a live chat.** A new `epoch` on `ready`: rows continue by `seq`, but live text is gone and must be dropped, not left frozen. Test: T3 `drops live text when ready carries a new epoch`.
5. **A long chat re-attached** (R17 drops feed state on detach). 5,000 rows replayed must fold without per-row re-sorting or re-rendering the list per row. Test: T3 `folds 5000 replayed rows into one order without re-rendering per row` (counts list commits with `React.Profiler`).
6. **Scoped rows before their card.** A child tool row whose scope's card has not been created yet (S1 R15 buffers on the server, but a `subagent{bound}` can follow the child rows by `seq` on replay). Test: T9 `attaches child rows that precede their card's bind`.

## Spec issues found while planning

1. S1 has no row for a retracted prompt and `TurnRetracted` has no writer. Settled by R6 (a `reason`, not a status).
2. S1's `FeedDelta.text` is everything so far, while the report and the additions doc require append-only deltas. Settled by R3.
3. S1's ask rows lack `answerable`, which the `/choices` read carried. Settled by R4.
4. Architecture §5 draws the outbox in the client, and "Prompts are ledger rows" removes the localStorage queue: together they mean unsent prompts die with the app. Settled by R7, surfaced as D1.
5. Report §4.4 asks for CSS bottom anchoring; the user's standing rule forbids losing the eased follow. Settled by R12.
6. Telemetry is not a ledger row in S1, so the ChatStore keeps one non-row field. Settled by R10.
7. `CLAUDE.md` says "conversations" live in `lib/store/`; the existing chat ledger store lives in `features/agent/stores/`. Settled by R2.
8. `CLAUDE.md` lists `bunx tsc --noEmit` as a CI check; `bunx tsc` resolves a different npm package. Use `bun tsc --noEmit`.
9. `features/agent ≤ 12k` is reachable only if rewritten and remaining files follow the comment rule: today's code without comments and blanks is already 12,996 lines. T12 exists for that; if T13 still misses, it reports the per-directory gap and never moves code out of `features/agent`.
10. S1 T7 deletes the activity aggregate but does not list `api/cmd/crowbar-seed-chat`, which writes it. S2 assumes S1 left that command appending ledger rows; T13's long-chat check uses it. If it does not compile at T13, stop and report.
11. Additions §4 A–J name third-party products as references; S2's tasks describe the behaviour only.
