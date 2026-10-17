# S2d — Canonical rows: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every row kind S1 writes has one renderer that shows the detail S1 records: tool and MCP rows by family with live and settled labels and run grouping (T8); the subagent card, the todo strip, the working line and reasoning (T9); approvals, turn failures, interrupts and a lost session (T10). Closes symptom 8.

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (R8, R11, R13, R14, D6, D7). Requires T1–T7. All labels are computed on the client from row fields and never stored twice (additions §4 A).

**Tech Stack:** React 19, `@/components/ui/*`, CSS-variable tokens, phosphor icons, shiki (lazy) for code, Plate (static) for markdown, vitest + Testing Library.

**Spec:** additions design §3.2, §4 A–J, §5 (budgets and the rules that keep them); report §2 symptom 8; S1 T8 (tool and MCP fields), T9 (subagent lifecycle rules 1–8, plan rows).

Shared rules for T8–T10: rows render on `@/components/ui/*` with CSS-variable tokens (no raw colours); live verbs shimmer and settled ones do not (no shimmer exists today: T8 adds one CSS keyframe in `web/src/features/agent/styles/transcript.css` beside `chat-fresh-text`, on tokens, and the app-wide reduced-motion kill-switch in `src/index.css` covers it; no animation library); clocks write `textContent` through `activity/lib/text-clock.ts` (T9) and never re-render; large results stay lazy (expand loads the blob); enter animations play only for `liveBorn` items. No test asserts a Tailwind class: tests read text, roles, `aria-*` and `data-row-id`/`data-status` attributes.

---

### Task 8: Tool and MCP rows by family (A, B)

**Goal:** A tool row's label comes from its family, status, title, target and MCP fields, with a fallback chain so a row never shows nothing. Consecutive read, search, fetch and read-only MCP rows of one turn fold into one run line. Edits, commands, subagents and asks are cards and never fold.

**Files:**
- Create: `web/src/features/agent/transcript/lib/tool-family.ts` — `toolFamily(body: ToolCallBody): ToolFamily` (`read | search | execute | edit | fetch | mcp | subagent | plan | meta | other`) from `kind`, `mcpServer` and the name; `meta` covers the provider's tool-discovery and hand-back tools, `addressesScope` calls and Crowbar's own `mcp__crowbar__*` (shown as quiet markers, e.g. "Renamed chat to …"); decided once, stored on `ToolItem.family` by the fold
- Create: `web/src/features/agent/transcript/lib/tool-label.ts` — `liveLabel(item)`, `settledLabel(item)` per the family table (additions §4 A), `commandProgramName(cmd)` (unwraps `bash -lc`, `sudo`, `env`), title chain (title → program name → raw command), target chain (target → first scalar input → humanized name), `mcpArgsSummary` (first 1–2 scalar args, `key: value`, truncated), `resultSummary` (first text line), `formatDuration`, `tailOf` (moved from `lib/agent-activity.ts`)
- Create: `web/src/features/agent/transcript/lib/tool-runs.ts` — `displayRows(order, items)`: groups runs (key `g:<firstItemId>`, stable as the run grows), hides `meta` and `plan`; a run breaks at text, an edit, a failed command, a subagent or an ask; `runSentence(items, live)` ("Reading 2 files, searching 1 pattern…" / "Read 2 files, searched 1 pattern, called echo")
- Rewrite: `web/src/features/agent/transcript/rows/tool-row.tsx` — status glyph and state text (running: spinner, verb shimmer; failed: error glyph and the first error line; declined: "Denied"; interrupted: "Stopped"; abandoned: "Result unavailable" in the warning token); inline extra (execute: `$ cmd` and the live output tail from `useLiveText`; a neutral `exit N` chip on a non-zero exit with output; edit: `+a −d`); expand per family (read: first ~40 lines, shiki, lazy; search: hit list; execute: full output; edit: the diff, open by default when ≤12 lines changed; fetch: excerpt; mcp: args JSON and result; other: raw JSON); under 300 lines, family bodies in `rows/tool-detail.tsx` if needed
- Create: `web/src/features/agent/transcript/rows/tool-run-row.tsx` — the run line (live ticker naming the newest call, settled sentence), expand lists its members as `tool-row`s
- Create: `web/src/features/agent/transcript/rows/mcp-server-badge.tsx` — the server as the main noun, a badge whose colour is a stable hash of the name into the existing chart/accent tokens; Crowbar's own server shows the Crowbar glyph
- Modify: `web/src/features/agent/stores/lib/fold-rows.ts` — `family` on tool items; a `tool_call` of family `subagent` makes no tool item (T9's card takes its place)
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx` — renders `displayRows` (memoized on `order`, whose identity changes only when an item is added; `family` never changes, so grouping depends on nothing else)
- Modify: `web/src/features/agent/lib/agent-activity.ts` — delete the tool helpers moved above
- Modify: `web/src/features/agent/styles/transcript.css` — the verb shimmer keyframe and the family row spacing, on tokens
- Test: `web/src/__tests__/features/agent/transcript/lib/tool-family.test.ts`, `lib/tool-label.test.ts`, `lib/tool-runs.test.ts`, `rows/tool-row.test.tsx`, `rows/tool-run-row.test.tsx`

- [ ] **Step 1: Failing tests**

```ts
// lib/tool-label.test.ts
it('symptom 8: labels a command by its title, then its unwrapped program name')  // bash -lc 'npm test' → "Running npm test"
it('labels a read live and settled with its line range and count')
it('falls back to the first scalar input, then the humanized name, for the target')
it('symptom 8: names the MCP server and summarizes the first two scalar args')
// lib/tool-runs.test.ts
it('folds consecutive reads and searches of one turn into one run with a stable key')
it('breaks a run at text, an edit, a failed command, a subagent or an ask')
it('hides meta and plan tool rows')
// rows/tool-row.test.tsx
it('opens an edit diff by default when it changes 12 lines or fewer')
it('shows the first error line of a failed tool inline')
it('shows a neutral exit chip for a non-zero exit with output')
it('reads Result unavailable for a tool its turn abandoned')
it('loads the payload from the blob route only when expanded')
// rows/tool-run-row.test.tsx
it('names the newest call while live and summarizes the run once settled')
```

- [ ] **Step 2: Run; they FAIL.** `cd web && bunx vitest run src/__tests__/features/agent/transcript`
- [ ] **Step 3: Implement** the three libs, the rows and the transcript's display rows; delete the moved helpers.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/transcript src/__tests__/features/agent/stores
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat(web): tool and MCP rows by family with run grouping

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** every tool row in the S1 recorded Claude subagent session (`api/tests/testdata/hooks/claude-subagent-session`, rendered in a test from its rows) has a non-empty label; no label text is stored in the store.

---

### Task 9: Subagent card, todo strip, working line, reasoning (C, D, E, J)

**Goal:** A subagent is one card from spawn to result, with its child rows nested, its current action derived from them and its result as markdown. The plan docks above the composer as a todo strip and leaves a collapsed snapshot in the transcript at turn end. The working line states the most specific true thing. Reasoning persists as a collapsed "Thought for Ns" row; commentary renders as lighter text. Closes symptom 8 ("subagent tool calls are dropped … there's no push").

**Files:**
- Modify: `web/src/features/agent/stores/lib/transcript-items.ts` — `SubagentItem { kind: 'subagent'; spawnToolId?; scope?; title; type; status: 'starting' | 'running' | 'done' | 'failed' | 'interrupted' | 'abandoned'; result?; usage?; children: string[]; parentWaiting: boolean; startedAt }`, `ReasoningItem { kind: 'reasoning'; text; startedAt; endedAt }`, `PlanItem` snapshot `{ kind: 'plan'; id: 'l:<turnId>'; items }`
- Modify: `web/src/features/agent/stores/lib/fold-rows.ts` — the subagent rules of S1 T9 (start keyed by `spawnToolId`, bound maps the scope to the card, stop/result/usage, interrupt and abandon); scoped rows become the card's `children` in `seq` order, including rows whose `seq` precedes the bind (they wait in `feed.scoped[scope]` and are attached when the bind names the scope); `addressesScope` calls set `parentWaiting` while the card runs; `plan` rows update `facts.plan[scope]`; `turn_ended` writes the main-scope plan snapshot item when the turn had plan rows; `reasoning` rows make reasoning items
- Create: `web/src/features/agent/transcript/rows/subagent-card.tsx` — header (title, type chip, status glyph, clock; once done `· N tool uses · Nk tokens`); line 2 `▸` the live label of the newest running child, else the newest settled one, else "Thinking…" (computed, never stored); "Parent waiting…" when `parentWaiting`; expanded: the children through the same row components, indented, grouped by `displayRows`, then the result through `MarkdownMessageStatic`
- Create: `web/src/features/agent/transcript/rows/reasoning-row.tsx` — collapsed "Thought for Ns", expands to the text (Plate static)
- Create: `web/src/features/agent/transcript/rows/plan-row.tsx` — the collapsed snapshot; `web/src/features/agent/activity/todo-strip.tsx` — docked above the composer: collapsed `◼ <active item>  done/total ▾`, expanded the item list; main scope only (a subagent's plan shows inside its card)
- Create: `web/src/features/agent/activity/lib/text-clock.ts` — one module-level 1 s `setInterval`, started by the first registered element and cleared with the last; each tick writes the elapsed time into each element's `textContent` (R14)
- Rewrite: `web/src/features/agent/activity/working-line.tsx` — the priority list of additions §4 E over `useTurnFacts`, the outbox and the live text: (1) waiting for your approval or answer (no spinner), (2) compacting, (3) the active todo's text, (4) the running main-agent tool's live label, (5) "Waiting for N agents", (6) the reasoning headline (`activity/lib/reasoning-headline.ts`), (7) nothing while answer text streams, (8) "Thinking…"; the clock counts from `facts.running.startedAt`, or from Enter during the optimistic phase; keeps the verb rotation
- Modify: `web/src/features/agent/activity/subagent-shelf.tsx` — running cards' title, current action and clock (through `text-clock.ts`, its own timer deleted)
- Modify: `web/src/features/agent/transcript/rows/text-row.tsx` — `phase: 'commentary'` renders with the muted text token
- Modify: `web/src/features/agent/transcript/rows/transcript-row.tsx`, `chat/agent-chat-view.tsx` (mount the strip)
- Test: `web/src/__tests__/features/agent/transcript/rows/subagent-card.test.tsx`, `rows/reasoning-row.test.tsx`, `activity/todo-strip.test.tsx`, `activity/lib/text-clock.test.ts`; extend `activity/working-line.test.tsx`, `activity/subagent-shelf.test.tsx`, `stores/lib/fold-rows.test.ts`

- [ ] **Step 1: Failing tests**

```ts
// stores/lib/fold-rows.test.ts
it('creates a subagent card on its spawn and keeps its key when it binds')
it('attaches child rows that precede their card bind')                  // Review Focus 6
it('marks running cards interrupted when the turn is stopped')
it('marks a card abandoned when its parent turn ends before it binds')
// rows/subagent-card.test.tsx
it('symptom 8: a subagent card shows its child tool rows and its result')   // replayed from the S1 recorded subagent session's rows
it('derives the current action from the newest running child')
it('shows Parent waiting while the parent addresses the subagent')
// activity/working-line.test.tsx
it('symptom 8: names the active todo while a turn runs')
it('prefers a pending approval over every other state')
it('shows nothing extra while answer text streams')
it('counts the clock from Enter before the turn starts')
// activity/lib/text-clock.test.ts (fake timers)
it('writes elapsed time into each element without a React render')
it('stops its interval when the last element unregisters')
// activity/todo-strip.test.tsx
it('shows the active item and progress collapsed and every item expanded')
// rows/reasoning-row.test.tsx
it('collapses persisted reasoning to Thought for Ns')
```

- [ ] **Step 2: Run; they FAIL.**
- [ ] **Step 3: Implement** the fold rules, the card, reasoning and plan rows, the strip, the clock and the working line; move the shelf and the working line onto the shared clock.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/transcript src/__tests__/features/agent/stores src/__tests__/features/agent/activity
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat(web): subagent cards, todo strip, working line and reasoning from rows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** a Claude subagent's own tool calls are visible inside its card; the working line never guesses; `features/agent` has exactly one clock interval.

---

### Task 10: Approvals, turn failures, interrupt, lost session (F, G, H, R13)

**Goal:** Pending asks render as a card stack docked over the composer, with the target and input the ask carries; the bound tool row shows it is awaiting approval and, once resolved, carries a one-line note instead of a separate row. A failed turn is a row with Retry and Open terminal. Stop shows "Stopping…" in the same frame and "Interrupted · you stopped this turn" when the row lands. A lost session offers "Start a new conversation".

**Files:**
- Rewrite: `web/src/features/agent/composer/composer-choice.tsx` and `composer/composer-choice-controls.tsx` (split in T3) — the card stack: header `<verb> <provider> … i/n`; body by the bound tool's family (command in a mono block plus its description; an edit's diff preview; `server · tool` and formatted args for MCP; questions and forms keep today's controls); number keys pick an option; Enter means allow-once only while the card has focus; answers through `answerChoice` as today, showing a 400 or 409 as today
- Create: `web/src/features/agent/composer/lib/ask-labels.ts` — `optionLabel`, `pickedOptionLabels`, `describeResolvedChoice`, `choiceQuestions` (moved from `lib/agent-activity.ts`)
- Delete: `web/src/features/agent/lib/agent-activity.ts` and its test (everything left has moved)
- Modify: `web/src/features/agent/stores/lib/fold-rows.ts` — an ask with a `toolId` sets `ToolItem.askId` while pending and `askNote` once resolved, and makes no top-level item; an ask with no tool is an `a:<askId>` item that renders as a one-line marker once resolved; a `turn_ended{interrupted}` turns its running tools `interrupted` (a `done` turn still reads `abandoned`); `stopping` is set by the Stop action and cleared by that turn's `turn_ended`
- Modify: `web/src/features/agent/transcript/rows/tool-row.tsx` — `data-status="awaiting_approval"` and scroll-into-view while its ask is pending; the resolved note ("Allowed once", "Denied")
- Modify: `web/src/features/agent/transcript/rows/marker-row.tsx` — `turn_failed`: headline, collapsible detail, Retry (re-sends the turn's prompt text through the outbox) and Open terminal; `turn_interrupted`: "Interrupted · you stopped this turn" (reasons `stopped`, `retracted`); `session_lost`: the reason and "Start a new conversation" (calls `resumeChat`, R13); `runner_exited` with a non-normal reason: the reason and Open terminal
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx` — the Stop action sets `stopping` before the request; the working line shows "Stopping…" while it is set
- Test: extend `web/src/__tests__/features/agent/composer/composer-choice.test.tsx` (under 500 per R19), `transcript/rows/tool-row.test.tsx`; create `transcript/rows/marker-row.test.tsx`, `composer/lib/ask-labels.test.ts`; delete `lib/agent-activity.test.ts`

- [ ] **Step 1: Failing tests**

```ts
// composer/composer-choice.test.tsx
it('shows the command and its description for a command approval')
it('shows the server and formatted args for an MCP approval')
it('picks an option with its number key')
it('allows once on Enter only while the card has focus')
it('counts pending asks as i of n')
// transcript/rows/tool-row.test.tsx
it('marks the bound tool row awaiting approval and replaces the ask with a one-line note once resolved')
// transcript/rows/marker-row.test.tsx
it('offers Retry and Open terminal on a failed turn')
it('reads Interrupted · you stopped this turn after Stop')
it('shows Stopping… in the same render as the Stop click')
it('offers Start a new conversation on a lost session')
```

- [ ] **Step 2: Run; they FAIL.**
- [ ] **Step 3: Implement** the card stack, the fold changes, the tool row's approval state and note, the marker rows and `stopping`; move the ask helpers and delete `lib/agent-activity.ts`.
- [ ] **Step 4: Run to green**

```bash
cd web && bunx vitest run src/__tests__/features/agent/composer src/__tests__/features/agent/transcript src/__tests__/features/agent/stores src/__tests__/features/agent/chat
cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent src/__tests__/features/agent
cd web && bun run knip && bun run lint:baselines && bun run lint:doctor
```

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat(web): approval card stack, failure and interrupt rows, lost-session action

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** every additions-design element A–J has a renderer fed only by rows, live text and the outbox; `web/src/features/agent/lib/` holds no activity code.
