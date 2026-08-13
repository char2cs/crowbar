# Chats Panel Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** SUPERSEDED beyond Task 5 — kept as the record of the first phase.

Tasks 1–5 landed as written. The three follow-on plans sketched at the bottom
were NOT executed as described: rather than build a second folder tree beside
the sidebar's, the shared logic was extracted first — `api/internal/app/tree`
(the pure sibling-order planner, lifted out of the sidebar's folder usecase) and
`web/src/components/tree-dnd` (the drop matrix and DOM hit-test, lifted out of
`drop-rules.ts`/`drop-target-dom.ts`) — and both trees were then built on top of
them. Persistence also moved from the client to the daemon. The design of record
is the spec; read it, not the Remaining Plans section below.

**Changes made during implementation, and why:**
- `chatVisibility` → **`shownChatIds`**, returning one `Set<string>`. The
  `openElsewhere` half went with the hidden-tab dot: two states, not three.
- `panes` is `Record<string, PaneGroup>` and **not optional**, so the planned
  `panes ?? {}` was dead on arrival and is gone (also removed from the one
  pre-existing site in the same function).
- **`FlatRow` carries the chat, not its id.** The plan had the panel look each id
  back up in a `Map`; that lookup can never miss, so it was an unreachable failure
  branch in the render path and a way for the drawn rows and the drag's list to
  disagree. `flattenChatTree` is now generic over `T extends TreeChat` and hands
  back the caller's own objects.
- `buildForest` is shared by `flattenChatTree` and `toSearchNodes`, with a test
  asserting they expose the same row set — a row on screen can never be
  unreachable by the field above it.
- Titles are normalised to `UNTITLED_CHAT_LABEL` once, in `treeChats`. That made
  the drag ghost's own `|| 'Untitled chat'` fallback dead, so it was removed.

**Goal:** Land the two parts of the chats-tree design that need no backend — a row is lit only while its chat is on screen, and a search field that makes a crowded panel navigable — and extract the list into a tree-shaped renderer the hierarchy work drops into.

**Architecture:** Three pure modules under `features/agent/lib/` carry the logic (`shown-chats.ts`, `chat-search.ts`, `chat-tree.ts`), each unit-testable with no React. `agent-chats-panel.tsx` keeps ownership of the store and the drag, and swaps its windowed flat list for the tree renderer. Nothing here touches Go, the wire, or persistence.

**Tech Stack:** React 19, Zustand (vanilla stores via `useStore`), TanStack Virtual, Vitest 4 + Testing Library, Tailwind v4 with the tokens in `web/src/styles/theme.css`.

## Global Constraints

- **Design source of truth:** `docs/superpowers/specs/2026-08-12-chats-tree-design.md`. Interaction and geometry are normative in the mock: https://claude.ai/code/artifact/9a84a3cb-2deb-4a20-9b34-17c93b0bbce3
- **Test location:** every test goes in `web/src/__tests__/` mirroring `web/src/`. Never create `features/X/tests/`.
- **Test imports:** use `@/` imports inside test files, never relative `../../`.
- **Component files:** kebab-case filenames, PascalCase exported component names.
- **Store reads:** `useXxxStore((s) => s.narrowField)` with a narrow selector in the render path; `.getState()` only inside event handlers and effects.
- **Stores must not import from `components/`.**
- **Row chrome:** always the shared constants in `web/src/components/layout/workspace-row-base.ts` (`ROW_BASE`, `ROW_ACTIVE`, `ROW_INACTIVE`, `ROW_INDENT_STEP`, `ROW_SUB_ACTION_HOVER`, `ROW_GLYPH_BOX`). Never inline a row style or a faded-foreground variant.
- **Vitest 4:** mock implementations must be `function`, not arrow, where `this` is used.
- **`bun tsc`, never `bunx tsc`** — a different package.
- Run tests from `web/`: `bun run test -- <path>`.
- Do not commit unless the task says to; do not open a PR.

---

## File Structure

**Create**
- `web/src/features/agent/lib/shown-chats.ts` — which chats are on screen. Pure; takes buffers, panes and `rootLayout`, returns one id set.
- `web/src/features/agent/lib/chat-search.ts` — query → `{match, keep, ctx}` id sets over a chat tree, plus the highlight splitter.
- `web/src/features/agent/lib/chat-tree.ts` — flat `AgentChat[]` + parent map → an ordered, indented render list. Ships in this plan holding only root-level rows; the hierarchy plan feeds it real parents without changing its shape.
- `web/src/features/agent/components/agent-chats-search.tsx` — the search field and its result line.
- Tests mirroring each of the above under `web/src/__tests__/`.

**Modify**
- `web/src/features/agent/components/agent-chats-panel.tsx` — swap `openChatIds` for the on-screen set, mount the search field, render through `chat-tree.ts`.
- `web/src/features/agent/components/agent-chat-row.tsx` — accept `query`, `depth` and `ctx`.

---

## Task 1: Which chats are on screen

The panel currently lights a row for any chat with an open buffer. A chat sitting in a background tab, or in a pane the user has hidden, stays lit with nothing on screen to justify it.

**Files:**
- Create: `web/src/features/agent/lib/shown-chats.ts`
- Test: `web/src/__tests__/features/agent/lib/shown-chats.test.ts`

**Interfaces:**
- Consumes: from the workspace store — `buffers`, `panes`, and `rootLayout`. A pane is `PaneGroup` (`web/src/features/panes/types/pane.ts`): `{ id, type: 'group', bufferIds: string[], activeBufferId: string | null }`. The layout is `LayoutNode` from the same file: `{type:'pane', id}` or `{type:'split', first, second, ...}`.
- **`rootLayout` is load-bearing, not decoration.** The store holds panes that are not on screen: `bottomLayout` exists but **nothing renders it** (asserted in `web/src/__tests__/features/workspace/components/workspace-view.test.tsx:114`), so a chat whose only tab sits in `BOTTOM_PANE_ID` must not light a row. A pane is on screen exactly when it is a leaf of `rootLayout`.
- Produces:
  ```ts
  import type { LayoutNode } from '@/features/panes/types/pane'

  /** The chats on screen. Two states, not three — a chat parked in a tab you
   *  cannot see gets no mark, so no second set is computed. */
  export function shownChatIds(
    buffers: readonly { id: string; type: string; chatId?: string }[],
    panes: Readonly<Record<string, { bufferIds: string[]; activeBufferId: string | null }>>,
    rootLayout: LayoutNode,
  ): Set<string>
  ```

- [x] **Step 1: Write the failing test**

`web/src/__tests__/features/agent/lib/shown-chats.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { chatVisibility } from '@/features/agent/lib/shown-chats'

import type { LayoutNode } from '@/features/panes/types/pane'

const buf = (id: string, chatId: string) => ({ id, type: 'agentChat', chatId })
const leaf = (id: string): LayoutNode => ({ type: 'pane', id })
const split = (first: LayoutNode, second: LayoutNode): LayoutNode => ({
  type: 'split',
  id: 's',
  direction: 'horizontal',
  sizes: [50, 50],
  first,
  second,
})

describe('chatVisibility', () => {
  it('lights only the active tab of a pane', () => {
    const { shown, openElsewhere } = chatVisibility(
      [buf('b1', 'c1'), buf('b2', 'c2')],
      { p1: { bufferIds: ['b1', 'b2'], activeBufferId: 'b1' } },
      leaf('p1'),
    )
    expect([...shown]).toEqual(['c1'])
    expect([...openElsewhere]).toEqual(['c2'])
  })

  it('lights the active tab of every pane in the layout', () => {
    const { shown } = chatVisibility(
      [buf('b1', 'c1'), buf('b2', 'c2')],
      {
        p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
        p2: { bufferIds: ['b2'], activeBufferId: 'b2' },
      },
      split(leaf('p1'), leaf('p2')),
    )
    expect([...shown].sort()).toEqual(['c1', 'c2'])
  })

  it('does NOT light a pane that exists in the store but not in the layout', () => {
    // The regression this whole module exists for: `bottomLayout` panes are in
    // `panes` and nothing renders them. A chat parked there is on screen nowhere.
    const { shown, openElsewhere } = chatVisibility(
      [buf('b1', 'c1'), buf('b2', 'c2')],
      {
        p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
        bottom: { bufferIds: ['b2'], activeBufferId: 'b2' },
      },
      leaf('p1'),
    )
    expect([...shown]).toEqual(['c1'])
    expect([...openElsewhere]).toEqual(['c2'])
  })

  it('does not light a pane with nothing selected', () => {
    const { shown, openElsewhere } = chatVisibility(
      [buf('b1', 'c1')],
      { p1: { bufferIds: ['b1'], activeBufferId: null } },
      leaf('p1'),
    )
    expect(shown.size).toBe(0)
    expect([...openElsewhere]).toEqual(['c1'])
  })

  it('does not light a chat whose buffer no pane lists', () => {
    const { shown, openElsewhere } = chatVisibility([buf('b1', 'c1')], {}, leaf('p1'))
    expect(shown.size).toBe(0)
    expect([...openElsewhere]).toEqual(['c1'])
  })

  it('ignores buffers that are not agent chats', () => {
    const { shown } = chatVisibility(
      [{ id: 'b1', type: 'file' }],
      { p1: { bufferIds: ['b1'], activeBufferId: 'b1' } },
      leaf('p1'),
    )
    expect(shown.size).toBe(0)
  })

  it('a chat open in two panes is shown if either shows it', () => {
    const { shown, openElsewhere } = chatVisibility(
      [buf('b1', 'c1'), buf('b2', 'c1')],
      {
        p1: { bufferIds: ['b1'], activeBufferId: 'b1' },
        p2: { bufferIds: ['b2'], activeBufferId: null },
      },
      split(leaf('p1'), leaf('p2')),
    )
    expect([...shown]).toEqual(['c1'])
    // Never in both sets — openElsewhere is strictly "not shown".
    expect(openElsewhere.has('c1')).toBe(false)
  })
})
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/shown-chats.test.ts`
Expected: FAIL — cannot resolve `@/features/agent/lib/shown-chats`.

- [x] **Step 3: Write minimal implementation**

`web/src/features/agent/lib/shown-chats.ts`:

```ts
import type { LayoutNode } from '@/features/panes/types/pane'

/**
 * Which chats are actually on screen.
 *
 * A row is lit when its chat is the ACTIVE TAB OF AN ON-SCREEN PANE — not when
 * it merely has a buffer. The old rule lit any open chat, so a chat in a
 * background tab stayed lit with nothing on screen to justify it.
 *
 * "On screen" is read from the LAYOUT, not from the panes record, and that is
 * the whole reason this is a module rather than three lines in the panel. The
 * store holds panes nothing renders: `bottomLayout` and its BOTTOM_PANE_ID exist
 * and no component draws them (workspace-view.test.tsx:114). Iterating
 * Object.values(panes) would light a chat parked there — a lit row for a pane
 * the user cannot see, which is the exact defect this replaces.
 */
export interface ChatVisibility {
  shown: Set<string>
  openElsewhere: Set<string>
}

interface BufferLike {
  id: string
  type: string
  chatId?: string
}

interface PaneLike {
  bufferIds: string[]
  activeBufferId: string | null
}

/** The pane ids this layout actually puts on screen. */
function renderedPaneIds(node: LayoutNode, out: Set<string> = new Set()): Set<string> {
  if (node.type === 'pane') {
    out.add(node.id)
    return out
  }
  renderedPaneIds(node.first, out)
  renderedPaneIds(node.second, out)
  return out
}

export function chatVisibility(
  buffers: readonly BufferLike[],
  panes: Readonly<Record<string, PaneLike>>,
  rootLayout: LayoutNode,
): ChatVisibility {
  const chatByBuffer = new Map<string, string>()
  for (const b of buffers) {
    if (b.type === 'agentChat' && b.chatId) chatByBuffer.set(b.id, b.chatId)
  }

  const onScreen = renderedPaneIds(rootLayout)
  const shown = new Set<string>()
  for (const [paneId, pane] of Object.entries(panes)) {
    if (!onScreen.has(paneId)) continue
    const active = pane.activeBufferId
    if (!active || !pane.bufferIds.includes(active)) continue
    const chatId = chatByBuffer.get(active)
    if (chatId) shown.add(chatId)
  }

  // Strictly disjoint: a chat shown in one pane is not "open elsewhere" just
  // because a second pane also holds it in a background tab.
  const openElsewhere = new Set<string>()
  for (const chatId of chatByBuffer.values()) {
    if (!shown.has(chatId)) openElsewhere.add(chatId)
  }

  return { shown, openElsewhere }
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/shown-chats.test.ts`
Expected: PASS, 7 tests.

- [x] **Step 5: Wire it into the panel**

In `web/src/features/agent/components/agent-chats-panel.tsx`, delete the `openChatIds` memo:

```tsx
  const openChatIds = useMemo(
    () => new Set(buffers.flatMap((b) => (b.type === 'agentChat' ? [b.chatId] : []))),
    [buffers],
  )
```

Replace with, alongside the existing `buffers` subscription:

```tsx
  const panes = useStore(store, (s) => s.panes)
  // rootLayout, not just panes: the store holds panes nothing renders. See
  // shown-chats.ts.
  const rootLayout = useStore(store, (s) => s.rootLayout)
  const { shown, openElsewhere } = useMemo(
    () => chatVisibility(buffers, panes ?? {}, rootLayout),
    [buffers, panes, rootLayout],
  )
```

Add the import:

```tsx
import { chatVisibility } from '@/features/agent/lib/shown-chats'
```

At the `<AgentChatRow>` call site, change:

```tsx
                  active={openChatIds.has(chat.id)}
```

to:

```tsx
                  active={shown.has(chat.id)}
                  openElsewhere={openElsewhere.has(chat.id)}
```

- [x] **Step 6: Draw the dot on the row**

In `web/src/features/agent/components/agent-chat-row.tsx`, add to `AgentChatRowProps`:

```tsx
  /** Has a tab you cannot see — a background tab or a hidden pane. */
  openElsewhere?: boolean
```

Destructure it with `openElsewhere = false` alongside `dragging = false`, and insert after the title span, before the closing `</div>`:

```tsx
      {/* "Open somewhere you cannot see" is a genuinely different state from
          "not open", and it is not the active surface — so it gets the smallest
          mark that can carry it, and gives way the moment the row is hovered and
          its actions want the space. */}
      {openElsewhere && !renaming && (
        <span
          data-open-elsewhere="true"
          aria-hidden="true"
          className="size-1.25 shrink-0 rounded-full bg-muted-foreground group-hover:hidden"
        />
      )}
```

Add `group` to the row's `cn(...)` class list so `group-hover:hidden` resolves.

- [x] **Step 7: Write the panel-level regression test**

`web/src/__tests__/features/agent/components/agent-chats-panel-lit-rows.test.tsx` — follow the existing harness in `agent-chats-panel.test.tsx` for store seeding and mocking; copy its `beforeEach` verbatim rather than inventing a second setup.

```tsx
import { describe, expect, it } from 'vitest'
import { chatVisibility } from '@/features/agent/lib/shown-chats'

// The defect this file exists for: a chat with a buffer in a pane that is not
// showing it must NOT be lit. Asserted at the seam the panel consumes, so it
// fails if anyone reinstates the buffers-only rule.
describe('lit rows follow what is on screen', () => {
  it('a background tab is not lit', () => {
    const { shown } = chatVisibility(
      [
        { id: 'b1', type: 'agentChat', chatId: 'visible' },
        { id: 'b2', type: 'agentChat', chatId: 'background' },
      ],
      { p1: { bufferIds: ['b1', 'b2'], activeBufferId: 'b1' } },
      { type: 'pane', id: 'p1' },
    )
    expect(shown.has('background')).toBe(false)
  })
})
```

- [x] **Step 8: Run the agent test suite**

Run: `cd web && bun run test -- src/__tests__/features/agent`
Expected: PASS. If `agent-chats-panel.test.tsx` asserts the old lit behaviour, update those assertions to seed `panes` — do not weaken them.

- [x] **Step 9: Typecheck**

Run: `cd web && bun tsc --noEmit`
Expected: no errors.

- [x] **Step 10: Commit**

```bash
git add web/src/features/agent/lib/shown-chats.ts \
        web/src/__tests__/features/agent/lib/shown-chats.test.ts \
        web/src/__tests__/features/agent/components/agent-chats-panel-lit-rows.test.tsx \
        web/src/features/agent/components/agent-chats-panel.tsx \
        web/src/features/agent/components/agent-chat-row.tsx
git commit -m "fix(chats): light a row only while its chat is on screen"
```

Report the commit SHA.

---

## Task 2: Search over the chat list

**Files:**
- Create: `web/src/features/agent/lib/chat-search.ts`
- Test: `web/src/__tests__/features/agent/lib/chat-search.test.ts`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces:
  ```ts
  export interface SearchNode { id: string; title: string; children: readonly SearchNode[] }
  export interface SearchSets {
    /** Titles containing the query. */
    match: Set<string>
    /** Everything rendered: matches, their ancestors, their descendants. */
    keep: Set<string>
    /** Kept only as an ancestor of a match — drawn dimmed. */
    ctx: Set<string>
  }
  export function searchChats(roots: readonly SearchNode[], query: string): SearchSets
  export function splitHighlight(title: string, query: string): { before: string; hit: string; after: string }
  ```

- [x] **Step 1: Write the failing test**

`web/src/__tests__/features/agent/lib/chat-search.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { searchChats, splitHighlight } from '@/features/agent/lib/chat-search'

const n = (id: string, title: string, children: ReturnType<typeof n>[] = []) => ({ id, title, children })

const roots = [
  n('f1', 'Release 1.4', [
    n('c1', 'Ship the sidebar redesign', [n('c2', 'Drag ghost drifts off the cursor')]),
    n('c3', 'Changelog for 1.4'),
  ]),
  n('c4', 'Drag rules for the chats tree'),
]

describe('searchChats', () => {
  it('an empty query matches nothing and keeps nothing', () => {
    const { match, keep } = searchChats(roots, '')
    expect(match.size).toBe(0)
    expect(keep.size).toBe(0)
  })

  it('matches case-insensitively on a substring', () => {
    expect([...searchChats(roots, 'DRAG').match].sort()).toEqual(['c2', 'c4'])
  })

  it('keeps ancestors of a match, and marks them as context', () => {
    const { keep, ctx } = searchChats(roots, 'drag')
    expect(keep.has('f1')).toBe(true)
    expect(keep.has('c1')).toBe(true)
    expect(ctx.has('f1')).toBe(true)
    expect(ctx.has('c1')).toBe(true)
  })

  it('never marks a match itself as context, even when it is also an ancestor', () => {
    const { ctx } = searchChats(roots, 'sidebar')
    // c1 matches AND is c2's parent; it must render as a hit, not as scaffolding.
    expect(ctx.has('c1')).toBe(false)
  })

  it('keeps the whole subtree under a matched row', () => {
    const { keep } = searchChats(roots, 'sidebar')
    expect(keep.has('c2')).toBe(true)
  })

  it('drops branches with no match anywhere in them', () => {
    const { keep } = searchChats(roots, 'drag')
    expect(keep.has('c3')).toBe(false)
  })
})

describe('splitHighlight', () => {
  it('splits around the first case-insensitive hit', () => {
    expect(splitHighlight('Drag rules', 'rul')).toEqual({ before: 'Drag ', hit: 'rul', after: 'es' })
  })

  it('returns the whole title as before when there is no hit', () => {
    expect(splitHighlight('Drag rules', 'zzz')).toEqual({ before: 'Drag rules', hit: '', after: '' })
  })

  it('returns the whole title as before for an empty query', () => {
    expect(splitHighlight('Drag rules', '')).toEqual({ before: 'Drag rules', hit: '', after: '' })
  })
})
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/chat-search.test.ts`
Expected: FAIL — cannot resolve `@/features/agent/lib/chat-search`.

- [x] **Step 3: Write minimal implementation**

`web/src/features/agent/lib/chat-search.ts`:

```ts
/**
 * Filtering the chats panel by title.
 *
 * A hit is rendered with the rows above it, dimmed, rather than alone: a chat
 * that turns up with no folder and no parent chat around it has lost the two
 * facts that say what it is. Descendants of a hit come along whole, because a
 * matched parent is usually what you were looking for and its threads are the
 * answer.
 */
export interface SearchNode {
  id: string
  title: string
  children: readonly SearchNode[]
}

export interface SearchSets {
  match: Set<string>
  keep: Set<string>
  ctx: Set<string>
}

export function searchChats(roots: readonly SearchNode[], query: string): SearchSets {
  const q = query.trim().toLowerCase()
  const match = new Set<string>()
  const keep = new Set<string>()
  const ctx = new Set<string>()
  if (!q) return { match, keep, ctx }

  const keepSubtree = (node: SearchNode): void => {
    keep.add(node.id)
    for (const c of node.children) keepSubtree(c)
  }

  const walk = (nodes: readonly SearchNode[], trail: readonly string[]): void => {
    for (const node of nodes) {
      if (node.title.toLowerCase().includes(q)) {
        match.add(node.id)
        keepSubtree(node)
        for (const ancestor of trail) {
          keep.add(ancestor)
          ctx.add(ancestor)
        }
      }
      walk(node.children, [...trail, node.id])
    }
  }
  walk(roots, [])

  // A row that matched is a HIT, whatever else it is. Resolved after the walk
  // because a match can be discovered after it has already been added as some
  // earlier hit's ancestor.
  for (const id of match) ctx.delete(id)

  return { match, keep, ctx }
}

export function splitHighlight(
  title: string,
  query: string,
): { before: string; hit: string; after: string } {
  const q = query.trim().toLowerCase()
  const i = q ? title.toLowerCase().indexOf(q) : -1
  if (i < 0) return { before: title, hit: '', after: '' }
  return {
    before: title.slice(0, i),
    hit: title.slice(i, i + q.length),
    after: title.slice(i + q.length),
  }
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/chat-search.test.ts`
Expected: PASS, 9 tests.

- [x] **Step 5: Commit**

```bash
git add web/src/features/agent/lib/chat-search.ts \
        web/src/__tests__/features/agent/lib/chat-search.test.ts
git commit -m "feat(chats): title search with ancestor context"
```

Report the commit SHA.

---

## Task 3: The search field

**Files:**
- Create: `web/src/features/agent/components/agent-chats-search.tsx`
- Test: `web/src/__tests__/features/agent/components/agent-chats-search.test.tsx`
- Modify: `web/src/features/agent/components/agent-chats-panel.tsx`

**Interfaces:**
- Consumes: nothing; the panel owns the query state and passes it down.
- Produces:
  ```ts
  export function AgentChatsSearch(props: {
    value: string
    resultCount: number | null   // null while the query is empty
    onChange: (next: string) => void
  }): React.JSX.Element
  ```

- [x] **Step 1: Write the failing test**

`web/src/__tests__/features/agent/components/agent-chats-search.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentChatsSearch } from '@/features/agent/components/agent-chats-search'

describe('AgentChatsSearch', () => {
  it('reports each keystroke', async () => {
    const onChange = vi.fn()
    render(<AgentChatsSearch value="" resultCount={null} onChange={onChange} />)
    await userEvent.type(screen.getByRole('searchbox', { name: /search chats/i }), 'd')
    expect(onChange).toHaveBeenCalledWith('d')
  })

  it('hides the result line while the query is empty', () => {
    render(<AgentChatsSearch value="" resultCount={null} onChange={vi.fn()} />)
    expect(screen.queryByTestId('chat-search-meta')).toBeNull()
  })

  it('counts results, singular and plural', () => {
    const { rerender } = render(<AgentChatsSearch value="a" resultCount={1} onChange={vi.fn()} />)
    expect(screen.getByTestId('chat-search-meta')).toHaveTextContent('1 chat')
    rerender(<AgentChatsSearch value="a" resultCount={3} onChange={vi.fn()} />)
    expect(screen.getByTestId('chat-search-meta')).toHaveTextContent('3 chats')
  })

  it('says so when a query matches nothing', () => {
    render(<AgentChatsSearch value="zzz" resultCount={0} onChange={vi.fn()} />)
    expect(screen.getByTestId('chat-search-meta')).toHaveTextContent('No chats')
  })

  it('escape clears the query', async () => {
    const onChange = vi.fn()
    render(<AgentChatsSearch value="drag" resultCount={2} onChange={onChange} />)
    screen.getByRole('searchbox', { name: /search chats/i }).focus()
    await userEvent.keyboard('{Escape}')
    expect(onChange).toHaveBeenCalledWith('')
  })
})
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && bun run test -- src/__tests__/features/agent/components/agent-chats-search.test.tsx`
Expected: FAIL — cannot resolve `@/features/agent/components/agent-chats-search`.

- [x] **Step 3: Write minimal implementation**

`web/src/features/agent/components/agent-chats-search.tsx`:

```tsx
import { MagnifyingGlass } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'

interface AgentChatsSearchProps {
  value: string
  /** null while the query is empty — the result line is not drawn at all then. */
  resultCount: number | null
  onChange: (next: string) => void
}

/**
 * The chats panel's filter.
 *
 * No scope control. The panel is workspace-scoped by construction, and on a
 * project home that workspace already IS the long list this field exists for — a
 * chip offering to widen it would name a second scope the panel cannot show.
 */
export function AgentChatsSearch({ value, resultCount, onChange }: AgentChatsSearchProps) {
  return (
    <div className="flex shrink-0 flex-col px-2 pb-1.5">
      <div
        className={cn(
          'flex h-[30px] items-center gap-1.5 rounded-lg px-2',
          'bg-sidebar-element-idle text-muted-foreground',
          'focus-within:ring-2 focus-within:ring-ring',
        )}
      >
        <MagnifyingGlass aria-hidden="true" className="size-4 shrink-0" />
        <input
          type="search"
          aria-label="Search chats"
          placeholder="Search chats"
          autoComplete="off"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              e.stopPropagation()
              onChange('')
            }
          }}
          className="min-w-0 flex-1 border-0 bg-transparent text-[12.5px] text-foreground outline-none placeholder:text-muted-foreground"
        />
      </div>
      {resultCount !== null && (
        <div
          data-testid="chat-search-meta"
          className="flex justify-between gap-2 px-1 pt-1 font-mono text-[10.5px] text-muted-foreground"
        >
          <span>
            {resultCount === 0
              ? 'No chats'
              : `${resultCount} ${resultCount === 1 ? 'chat' : 'chats'}`}
          </span>
          <span>esc to clear</span>
        </div>
      )}
    </div>
  )
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && bun run test -- src/__tests__/features/agent/components/agent-chats-search.test.tsx`
Expected: PASS, 5 tests.

- [x] **Step 5: Mount it in the panel and filter the list**

In `agent-chats-panel.tsx`, add the import and state:

```tsx
import { AgentChatsSearch } from './agent-chats-search'
import { searchChats } from '@/features/agent/lib/chat-search'
```

```tsx
  const [query, setQuery] = useState('')
```

After the existing `ordered` memo, derive the filtered list. The panel is still flat in this plan, so every chat is its own root with no children:

```tsx
  // Flat today; the hierarchy plan replaces this with the real tree and the
  // ancestor-context half of searchChats starts doing work.
  const search = useMemo(
    () => searchChats(ordered.map((c) => ({ id: c.id, title: c.title || 'Untitled chat', children: [] })), query),
    [ordered, query],
  )
  const visible = useMemo(
    () => (query.trim() ? ordered.filter((c) => search.keep.has(c.id)) : ordered),
    [ordered, query, search],
  )
```

Render the field directly above the scroll container:

```tsx
      <AgentChatsSearch
        value={query}
        resultCount={query.trim() ? search.match.size : null}
        onChange={setQuery}
      />
```

- [x] **Step 6: Point the virtualizer and the rows at `visible`**

Replace every remaining use of `ordered` in the render path and in the drag wiring with `visible`:

- `useAgentChatListVirtualizer(ordered.length)` → `useAgentChatListVirtualizer(visible.length)`
- `const chat = ordered[virtualItem.index]` → `const chat = visible[virtualItem.index]`
- `useAgentChatListDrag({ ..., ordered, ... })` → `ordered: visible`
- `const draggingChat = draggingId ? ordered.find(...)` → `visible.find(...)`
- The `primaryProvider && ordered.length > 0` separator guard → `visible.length > 0`

Then disable reordering while filtered — a drop index computed against a filtered list would write a bogus order for the rows it cannot see:

```tsx
    onReorder: (dragId, slot) => {
      if (query.trim()) return
      store.getState().setAgentChatOrder(reorderIds(visible.map((c) => c.id), dragId, slot))
    },
```

- [x] **Step 7: Highlight the hit on the row**

In `agent-chat-row.tsx`, add to the props:

```tsx
  /** The active query, for marking the matched substring. '' when not filtering. */
  query?: string
```

Replace the plain title span:

```tsx
        <span className="min-w-0 flex-1 truncate">{title}</span>
```

with:

```tsx
        <span className="min-w-0 flex-1 truncate">
          {(() => {
            const { before, hit, after } = splitHighlight(title, query ?? '')
            if (!hit) return title
            return (
              <>
                {before}
                <mark className="rounded-[3px] bg-[color-mix(in_oklch,var(--secondary)_45%,transparent)] px-px text-inherit">
                  {hit}
                </mark>
                {after}
              </>
            )
          })()}
        </span>
```

Import `splitHighlight` from `@/features/agent/lib/chat-search`, and pass `query={query}` at the panel's `<AgentChatRow>` call site.

- [x] **Step 8: Run the suite and typecheck**

Run: `cd web && bun run test -- src/__tests__/features/agent && bun tsc --noEmit`
Expected: PASS, no type errors.

- [x] **Step 9: Commit**

```bash
git add web/src/features/agent/components/agent-chats-search.tsx \
        web/src/__tests__/features/agent/components/agent-chats-search.test.tsx \
        web/src/features/agent/components/agent-chats-panel.tsx \
        web/src/features/agent/components/agent-chat-row.tsx
git commit -m "feat(chats): search the chat list, with the hit marked"
```

Report the commit SHA.

---

## Task 4: The tree renderer

Windowing and hierarchy do not compose: a virtualizer needs a flat array of equal-height rows, and a tree is neither until it is flattened. This task flattens explicitly, so the list stays windowed and the hierarchy plan has somewhere to put parents.

**Files:**
- Create: `web/src/features/agent/lib/chat-tree.ts`
- Test: `web/src/__tests__/features/agent/lib/chat-tree.test.ts`
- Modify: `web/src/features/agent/components/agent-chats-panel.tsx`

**Interfaces:**
- Consumes: `SearchSets` from Task 2 (`@/features/agent/lib/chat-search`).
- Produces:
  ```ts
  export interface TreeChat { id: string; parentId: string; title: string }
  export interface FlatRow { id: string; depth: number; ctx: boolean }
  export function flattenChatTree(
    chats: readonly TreeChat[],
    opts: { collapsed: ReadonlySet<string>; keep: ReadonlySet<string> | null; ctx: ReadonlySet<string> },
  ): FlatRow[]
  export function toSearchNodes(chats: readonly TreeChat[]): SearchNode[]
  ```

- [x] **Step 1: Write the failing test**

`web/src/__tests__/features/agent/lib/chat-tree.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { flattenChatTree, toSearchNodes } from '@/features/agent/lib/chat-tree'

const chats = [
  { id: 'a', parentId: '', title: 'A' },
  { id: 'a1', parentId: 'a', title: 'A1' },
  { id: 'a1x', parentId: 'a1', title: 'A1X' },
  { id: 'b', parentId: '', title: 'B' },
]
const none = new Set<string>()

describe('flattenChatTree', () => {
  it('emits depth-first order with one indent step per level', () => {
    expect(flattenChatTree(chats, { collapsed: none, keep: null, ctx: none })).toEqual([
      { id: 'a', depth: 0, ctx: false },
      { id: 'a1', depth: 1, ctx: false },
      { id: 'a1x', depth: 2, ctx: false },
      { id: 'b', depth: 0, ctx: false },
    ])
  })

  it('a collapsed row hides its whole subtree, not just its children', () => {
    const rows = flattenChatTree(chats, { collapsed: new Set(['a']), keep: null, ctx: none })
    expect(rows.map((r) => r.id)).toEqual(['a', 'b'])
  })

  it('keeps only rows in the keep set when one is given', () => {
    const rows = flattenChatTree(chats, { collapsed: none, keep: new Set(['a', 'a1']), ctx: none })
    expect(rows.map((r) => r.id)).toEqual(['a', 'a1'])
  })

  it('ignores collapse while a keep set is active', () => {
    // Search must be able to show a hit inside a folded parent.
    const rows = flattenChatTree(chats, {
      collapsed: new Set(['a']),
      keep: new Set(['a', 'a1']),
      ctx: none,
    })
    expect(rows.map((r) => r.id)).toEqual(['a', 'a1'])
  })

  it('marks context rows', () => {
    const rows = flattenChatTree(chats, { collapsed: none, keep: new Set(['a', 'a1']), ctx: new Set(['a']) })
    expect(rows.find((r) => r.id === 'a')?.ctx).toBe(true)
    expect(rows.find((r) => r.id === 'a1')?.ctx).toBe(false)
  })

  it('treats a chat whose parent is missing as a root, never dropping it', () => {
    const orphaned = [{ id: 'x', parentId: 'gone', title: 'X' }]
    expect(flattenChatTree(orphaned, { collapsed: none, keep: null, ctx: none })).toEqual([
      { id: 'x', depth: 0, ctx: false },
    ])
  })

  it('does not loop on a parent cycle', () => {
    const cyclic = [
      { id: 'p', parentId: 'q', title: 'P' },
      { id: 'q', parentId: 'p', title: 'Q' },
    ]
    const rows = flattenChatTree(cyclic, { collapsed: none, keep: null, ctx: none })
    expect(rows.map((r) => r.id).sort()).toEqual(['p', 'q'])
  })
})

describe('toSearchNodes', () => {
  it('nests children under their parent', () => {
    const [a] = toSearchNodes(chats)
    expect(a.id).toBe('a')
    expect(a.children.map((c) => c.id)).toEqual(['a1'])
    expect(a.children[0].children.map((c) => c.id)).toEqual(['a1x'])
  })
})
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/chat-tree.test.ts`
Expected: FAIL — cannot resolve `@/features/agent/lib/chat-tree`.

- [x] **Step 3: Write minimal implementation**

`web/src/features/agent/lib/chat-tree.ts`:

```ts
import type { SearchNode } from './chat-search'

/**
 * The chats panel is windowed, and a virtualizer wants a flat array. So the
 * tree is flattened here, once, into rows that carry their own depth — the same
 * shape the workspace tree renders, minus its React.
 *
 * Two defences that are not decoration. A chat whose parent id names nothing
 * renders as a root rather than vanishing: a row that exists in the store and
 * nowhere on screen is the worst failure this panel has. And the visited set
 * makes a parent cycle finite — the backend refuses cycles at the command, but a
 * renderer that hangs on bad data is a renderer that hangs.
 */
export interface TreeChat {
  id: string
  parentId: string
  title: string
}

export interface FlatRow {
  id: string
  depth: number
  ctx: boolean
}

interface FlattenOpts {
  collapsed: ReadonlySet<string>
  /** Rows to render, or null for "everything". Set while a search is active. */
  keep: ReadonlySet<string> | null
  ctx: ReadonlySet<string>
}

function childMap(chats: readonly TreeChat[]): Map<string, TreeChat[]> {
  const byId = new Set(chats.map((c) => c.id))
  const kids = new Map<string, TreeChat[]>()
  for (const c of chats) {
    // An unknown parent means root, never "drop this row".
    const key = c.parentId && byId.has(c.parentId) ? c.parentId : ''
    const list = kids.get(key)
    if (list) list.push(c)
    else kids.set(key, [c])
  }
  return kids
}

export function flattenChatTree(chats: readonly TreeChat[], opts: FlattenOpts): FlatRow[] {
  const kids = childMap(chats)
  const out: FlatRow[] = []
  const visited = new Set<string>()

  const walk = (parentId: string, depth: number): void => {
    for (const child of kids.get(parentId) ?? []) {
      if (visited.has(child.id)) continue
      visited.add(child.id)
      if (opts.keep && !opts.keep.has(child.id)) continue
      out.push({ id: child.id, depth, ctx: opts.ctx.has(child.id) })
      // A search shows hits wherever they live, so collapse is ignored while one
      // is active — otherwise a folded parent could hide the only result.
      if (!opts.keep && opts.collapsed.has(child.id)) continue
      walk(child.id, depth + 1)
    }
  }
  walk('', 0)

  // Anything a cycle kept out of the walk still has to render.
  for (const c of chats) {
    if (visited.has(c.id)) continue
    if (opts.keep && !opts.keep.has(c.id)) continue
    visited.add(c.id)
    out.push({ id: c.id, depth: 0, ctx: opts.ctx.has(c.id) })
  }
  return out
}

export function toSearchNodes(chats: readonly TreeChat[]): SearchNode[] {
  const kids = childMap(chats)
  const build = (parentId: string, seen: ReadonlySet<string>): SearchNode[] =>
    (kids.get(parentId) ?? [])
      .filter((c) => !seen.has(c.id))
      .map((c) => ({
        id: c.id,
        title: c.title,
        children: build(c.id, new Set([...seen, c.id])),
      }))
  return build('', new Set())
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && bun run test -- src/__tests__/features/agent/lib/chat-tree.test.ts`
Expected: PASS, 8 tests.

- [x] **Step 5: Render the panel through it**

In `agent-chats-panel.tsx`, replace the `search`/`visible` memos from Task 3 with the tree-shaped versions. `AgentChat` has no `parentId` until the hierarchy plan lands, so read it defensively — every chat is a root today and the renderer is already correct for that:

```tsx
  const treeChats = useMemo(
    () =>
      ordered.map((c) => ({
        id: c.id,
        parentId: (c as { parentId?: string }).parentId ?? '',
        title: c.title || 'Untitled chat',
      })),
    [ordered],
  )
  const search = useMemo(() => searchChats(toSearchNodes(treeChats), query), [treeChats, query])
  const rows = useMemo(
    () =>
      flattenChatTree(treeChats, {
        collapsed,
        keep: query.trim() ? search.keep : null,
        ctx: search.ctx,
      }),
    [treeChats, collapsed, query, search],
  )
  const byId = useMemo(() => new Map(ordered.map((c) => [c.id, c])), [ordered])
```

Add the collapse set alongside `renamingId`:

```tsx
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())
```

Drive the virtualizer from `rows.length`, and inside the virtual-item map resolve `const row = rows[virtualItem.index]` then `const chat = byId.get(row.id)`. Pass `depth={row.depth}` and `ctx={row.ctx}` to `AgentChatRow`, and swap the drag hook's `ordered` for `rows.map((r) => byId.get(r.id)!)`.

- [x] **Step 6: Indent and dim the row**

In `agent-chat-row.tsx`, add `depth?: number` and `ctx?: boolean` to the props, and wrap the returned row in the indent box the workspace tree uses:

```tsx
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: (depth ?? 0) * ROW_INDENT_STEP }}>
      {/* existing row div, with `ctx && 'opacity-45'` added to its cn(...) */}
    </div>
```

Import `ROW_INDENT_STEP` and `ROW_INDENT_TRANSITION` from `@/components/layout/workspace-row-base`.

- [x] **Step 7: Verify the row box did not change height**

The virtualizer positions rows at `index * AGENT_CHAT_ROW_HEIGHT`; an indent wrapper that adds any vertical box would desynchronise the drop geometry from the paint.

Add to `web/src/__tests__/features/agent/components/agent-chat-row.test.tsx`:

```tsx
it('indenting does not change the row height', () => {
  const { container, rerender } = render(<AgentChatRow {...baseProps} depth={0} />)
  const flat = container.querySelector('[data-agent-chat-drop]')!.getBoundingClientRect().height
  rerender(<AgentChatRow {...baseProps} depth={3} />)
  const deep = container.querySelector('[data-agent-chat-drop]')!.getBoundingClientRect().height
  expect(deep).toBe(flat)
})
```

Use the file's existing `baseProps`; if it has none, build one from the props the other tests in that file already pass.

- [x] **Step 8: Run the suite and typecheck**

Run: `cd web && bun run test -- src/__tests__/features/agent && bun tsc --noEmit`
Expected: PASS, no type errors.

- [x] **Step 9: Commit**

```bash
git add web/src/features/agent/lib/chat-tree.ts \
        web/src/__tests__/features/agent/lib/chat-tree.test.ts \
        web/src/__tests__/features/agent/components/agent-chat-row.test.tsx \
        web/src/features/agent/components/agent-chats-panel.tsx \
        web/src/features/agent/components/agent-chat-row.tsx
git commit -m "refactor(chats): render the panel through a flattened tree"
```

Report the commit SHA.

---

## Task 5: Live verification in the Tauri app

Compiling and green tests do not prove a sidebar. This task exercises the states in the running app.

**Files:** none — this is a manual gate.

- [x] **Step 1: Confirm no other dev instance is running**

Run: `pgrep -fl crowbar-api`
Expected: no output. If there is output, stop and resolve it — two daemons on one socket is its own bug.

- [x] **Step 2: Start the isolated dev app**

Run: `make dev-desktop`

This must use the worktree's own `CROWBAR_HOME`. Never touch `~/.crowbar` — that is real work.

- [x] **Step 3: Exercise the lit-row rule**

In a workspace with at least three chats:
1. Open chat A. Its row lights.
2. Open chat B in the same pane. A's row goes dark and shows the muted dot; B lights.
3. Split the pane, open chat C in the second pane. Both B and C are lit.
4. Close C's tab. C goes dark, dot gone.

Record: any row lit with nothing on screen is a failure of Task 1.

- [x] **Step 4: Exercise search**

Type a substring matching two chats. Confirm the count line reads `2 chats`, the substring is marked on both rows, and `esc` clears the field and restores the full list.

- [x] **Step 5: Confirm the drag still works**

Drag a row to reorder it, then drag one onto the trash. Both must behave exactly as before this plan — nothing here was meant to change them.

- [x] **Step 6: Record the result**

Write what you did and what you saw into the PR body or the task report. "Tested manually" without the states listed is not a result.

---

## Self-Review

**Spec coverage.** §2.5 search → Tasks 2–3. §2.6 lit rows → Task 1. §2.1 rows (indent, tokens) → Task 4. §2.2 drag, §2.3 refusals, §2.4 kept rows, §1 threads, §1.4 folders, §3 persistence, §4 lineage → **not in this plan by design**; they need the backend and are listed under Remaining Plans below.

**Placeholders.** None: every code step carries the code, every test step the test, every run step the command and its expected result.

**Type consistency.** `chatVisibility(buffers, panes, rootLayout)` returns `{shown, openElsewhere}`, consumed under those names in Task 1 Step 5. `searchChats` returns `{match, keep, ctx}`, consumed in Tasks 3 and 4. `flattenChatTree` returns `FlatRow[]` with `{id, depth, ctx}`, consumed in Task 4 Steps 5–6. `SearchNode` is defined in `chat-search.ts` and imported by `chat-tree.ts`. `AgentChatRow` gains `openElsewhere`, `query`, `depth`, `ctx` — each added in the task that first passes it.

---

## Remaining Plans

Three plans follow this one. Each ships working software on its own; each is written once the open question it depends on is answered.

**Plan 2 — Hierarchy backend.** `ParentID` on `domain.AgentChat` set by an asynx command and folded from an event; a new `AgentChatFolder` aggregate; server-side cycle refusal; REST + WS frames. *Blocked on spec Q1* (does re-parenting rewrite context retroactively) — the answer decides whether the reparent command also writes a ledger turn.

**Plan 3 — Hierarchy frontend.** Folder rows, kept rows, and the drag, reusing `drop-rules.ts` / `drop-indicator.tsx` / `drag-ghost.tsx` / `drop-target-dom.ts` / `edge-scroll.ts` rather than re-deriving them; per-container sibling order. *Blocked on Plan 2 and on spec Q2* (does deleting a parent take its threads).

**Plan 4 — Threads read their parents.** Lineage named through `contextInject` at spawn; `get_chat_log` proved to permit ancestor reads and to refuse descendant and sibling reads. *Blocked on Plan 2.* The refusals are the load-bearing half and get black-box `TestRegression_*` coverage.
