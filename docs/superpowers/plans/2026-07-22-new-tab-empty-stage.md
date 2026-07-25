# New Tab Empty Stage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the pane's one-button empty state with a "Nameplate" surface, and promote it from a `!activeBuffer` fallback into a real `newTab` buffer you open with ⌘T.

**Architecture:** The `newTab` buffer type already exists and is dead code — `tab-bar.tsx:63` filters it out of the strip and `pane-container.tsx:131` skips rendering it, and nothing ever mints one. This turns it on. Store rules (open/consume/spawn) live in `buffer-slice` and `pane-slice`; the surface is one new presentational component reading chords from the keymap registry.

**Tech Stack:** React 19, Zustand (+immer), Tailwind v4, Vitest, TypeScript.

**Spec:** `docs/superpowers/specs/2026-07-22-new-tab-empty-stage-design.md`

## Global Constraints

- **macOS is the only target.** The `mod+j` / xterm collision documented in the spec is DEFERRED — do not touch `terminal.tsx`.
- Component files are **kebab-case**; the exported React component stays PascalCase.
- Tests live in `web/src/__tests__/` **mirroring** `web/src/`, using `@/` imports — never `../../`.
- Store selectors must be **narrow**: `useXxxStore((s) => s.field)`, never `useXxxStore()`.
- `getState()` only inside event handlers and `useEffect` bodies — never in a render path.
- Stores must **not** import from `components/`.
- **No sleeps, `Eventually`, or poll timeouts in tests.** Block on real signals.
- Run one test file: `cd web && bunx vitest run <path>`
- Run all: `cd web && bun run test`
- Typecheck: `cd web && bunx tsc --noEmit`

---

## File Structure

| File | Responsibility |
| --- | --- |
| `web/src/features/keymaps/registry.ts` | four command entries (modify) |
| `web/src/features/workspace/stores/slices/buffer-slice.ts` | `openNewTab`, consume-untouched (modify) |
| `web/src/features/workspace/stores/slices/pane-slice.ts` | spawn-on-empty, closeability sync (modify) |
| `web/src/features/tabs/components/tab-bar.tsx` | stop filtering `newTab` (modify) |
| `web/src/features/tabs/components/tab-bar-item.tsx` | New Tab icon (modify) |
| `web/src/features/panes/components/new-tab-view.tsx` | **the surface** (create) |
| `web/src/features/panes/components/pane-container.tsx` | render `newTab`, repoint fallback (modify) |
| `web/src/features/panes/components/empty-editor-state.tsx` | **delete** |
| `web/src/features/panes/hooks/use-pane-keyboard.ts` | chord handlers, ⌘W guard (modify) |
| `web/src/components/ui/crowbar-mark.tsx` | the glyph alone, for the 16px tab icon (create) |
| `web/src/styles/theme.css` | `--logo-ink` token + pane container queries (modify) |
| `web/src/features/workspace/stores/persisted-layout.ts` | strip `newTab` from the saved snapshot (create) |
| `web/src/features/workspace/stores/workspace-store-registry.ts` | use it at the save site (modify) |
| `web/src/features/workspace/components/workspace-view.tsx` | open on a New Tab after hydration (modify) |

---

### Task 1: Keymap registry — four commands

**Files:**
- Modify: `web/src/features/keymaps/registry.ts:22` (constants), `:82` (TAB_NEW_TERMINAL entry)
- Test: `web/src/__tests__/features/keymaps/registry.test.ts` (create)

**Interfaces:**
- Produces: `TAB_NEW = 'tabs.new'`, `TAB_NEW_FILE = 'tabs.newFile'`, `AGENT_NEW_CHAT = 'agent.newChat'` exported from `@/features/keymaps/registry`; `TAB_NEW_TERMINAL` keeps its id `'tabs.newTerminal'` but its `defaultChord` becomes `'mod+j'`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/keymaps/registry.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import {
  COMMANDS,
  getCommand,
  TAB_NEW,
  TAB_NEW_TERMINAL,
  TAB_NEW_FILE,
  AGENT_NEW_CHAT,
} from '@/features/keymaps/registry'

describe('keymap registry — New Tab commands', () => {
  it('binds mod+t to New Tab, not New Terminal', () => {
    expect(getCommand(TAB_NEW)?.defaultChord).toBe('mod+t')
  })

  it('moves New Terminal to mod+j', () => {
    expect(getCommand(TAB_NEW_TERMINAL)?.defaultChord).toBe('mod+j')
  })

  it('binds New File and New Chat', () => {
    expect(getCommand(TAB_NEW_FILE)?.defaultChord).toBe('mod+n')
    expect(getCommand(AGENT_NEW_CHAT)?.defaultChord).toBe('mod+shift+n')
  })

  // Every chord the New Tab surface draws must be rebindable, or the badge
  // it renders can never go out of sync with reality by design.
  it('makes all four live-editable', () => {
    for (const id of [TAB_NEW, TAB_NEW_TERMINAL, TAB_NEW_FILE, AGENT_NEW_CHAT]) {
      expect(getCommand(id)?.liveEditable).toBe(true)
    }
  })

  it('has no duplicate default chords', () => {
    const chords = COMMANDS.map((c) => c.defaultChord)
    expect(new Set(chords).size).toBe(chords.length)
  })
})
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/keymaps/registry.test.ts`
Expected: FAIL — `TAB_NEW` is not exported (import error).

- [ ] **Step 3: Add the constants**

In `web/src/features/keymaps/registry.ts`, beside the existing `TAB_NEW_TERMINAL` export (line ~22):

```ts
export const TAB_NEW = 'tabs.new'
export const TAB_NEW_TERMINAL = 'tabs.newTerminal'
export const TAB_NEW_FILE = 'tabs.newFile'
export const AGENT_NEW_CHAT = 'agent.newChat'
```

- [ ] **Step 4: Add the entries and retarget New Terminal**

In the `COMMANDS` array, replace the existing `TAB_NEW_TERMINAL` entry with these four:

```ts
  {
    id: TAB_NEW,
    label: 'New tab',
    category: 'Tabs',
    defaultChord: 'mod+t',
    liveEditable: true,
  },
  {
    // Moved off mod+t, which is now New Tab.
    id: TAB_NEW_TERMINAL,
    label: 'New terminal tab',
    category: 'Tabs',
    defaultChord: 'mod+j',
    liveEditable: true,
  },
  {
    id: TAB_NEW_FILE,
    label: 'New file',
    category: 'Tabs',
    defaultChord: 'mod+n',
    liveEditable: true,
  },
  {
    id: AGENT_NEW_CHAT,
    label: 'New chat',
    category: 'Tabs',
    defaultChord: 'mod+shift+n',
    liveEditable: true,
  },
```

- [ ] **Step 5: Run the test and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/keymaps/registry.test.ts`
Expected: PASS, 5 tests.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/keymaps/registry.ts web/src/__tests__/features/keymaps/registry.test.ts
git commit -m "feat(keymaps): add New Tab/File/Chat commands, move New Terminal to mod+j"
```

---

### Task 2: `openNewTab` — one per pane, focus the existing

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` (`BufferActions` interface ~line 34, action body)
- Test: `web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts` (create)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `bufferActions.openNewTab(paneId?: string): string | null` — returns the buffer id of the
  New Tab now active in `paneId` (defaults to `activePaneId`), or `null` when that pane is not
  registered (minting a buffer no pane holds would orphan it). Idempotent per pane, and it never
  changes which pane is globally active — moving focus is the caller's business.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'

vi.mock('@/features/terminal/lib/kill-terminal-session', () => ({
  killTerminalSession: vi.fn(async () => {}),
}))
vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  clearReconnect: vi.fn(),
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
}))
vi.mock('@/features/agent/api/agent-api', () => ({
  stopChat: vi.fn(async () => {}),
  deleteChat: vi.fn(async () => {}),
}))

// A pane double that actually tracks membership, so "which pane holds this
// buffer" is a real question the slice can get wrong.
function makePaneActions(panes: Record<string, string[]>) {
  return {
    addBufferToPane: vi.fn((paneId: string, bufferId: string) => {
      panes[paneId] ??= []
      if (!panes[paneId].includes(bufferId)) panes[paneId].push(bufferId)
    }),
    removeBufferFromPane: vi.fn((paneId: string, bufferId: string) => {
      panes[paneId] = (panes[paneId] ?? []).filter((id) => id !== bufferId)
    }),
    setPanePreviewBuffer: vi.fn(),
    clearPreviewBufferEverywhere: vi.fn(),
    activatePaneBuffer: vi.fn(),
    setActivePane: vi.fn(),
    getPaneById: vi.fn((id: string) => ({ id, bufferIds: panes[id] ?? [] })),
    getPaneByBufferId: vi.fn((bufferId: string) => {
      const entry = Object.entries(panes).find(([, ids]) => ids.includes(bufferId))
      return entry ? { id: entry[0], bufferIds: entry[1] } : null
    }),
  }
}

function makeStore() {
  const panes: Record<string, string[]> = { root: [] }
  const paneActions = makePaneActions(panes)
  const store = createStore<
    BufferSlice & { paneActions: typeof paneActions; workspaceId: string; activePaneId: string }
  >()(
    immer((set, get) => ({
      ...createBufferSlice(...([set, get, {}] as unknown as Parameters<typeof createBufferSlice>)),
      paneActions,
      workspaceId: 'ws-test',
      activePaneId: 'root',
    })),
  )
  return { store, panes, paneActions }
}

describe('buffer-slice — openNewTab', () => {
  let ctx: ReturnType<typeof makeStore>
  beforeEach(() => {
    ctx = makeStore()
  })

  it('creates a newTab buffer named "New Tab"', () => {
    const id = ctx.store.getState().bufferActions.openNewTab()
    const buf = ctx.store.getState().buffers.find((b) => b.id === id)
    expect(buf?.type).toBe('newTab')
    expect(buf?.name).toBe('New Tab')
  })

  it('called twice on the same pane returns the SAME id and mints only one buffer', () => {
    const first = ctx.store.getState().bufferActions.openNewTab()
    const second = ctx.store.getState().bufferActions.openNewTab()
    expect(second).toBe(first)
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(1)
  })

  it('re-activates the existing New Tab rather than re-adding it', () => {
    const id = ctx.store.getState().bufferActions.openNewTab()
    ctx.paneActions.activatePaneBuffer.mockClear()
    ctx.store.getState().bufferActions.openNewTab()
    expect(ctx.paneActions.activatePaneBuffer).toHaveBeenCalledWith('root', id)
  })

  it('gives a DIFFERENT pane its own New Tab', () => {
    const rootId = ctx.store.getState().bufferActions.openNewTab('root')
    const splitId = ctx.store.getState().bufferActions.openNewTab('split-1')
    expect(splitId).not.toBe(rootId)
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(2)
  })
})
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`
Expected: FAIL — `bufferActions.openNewTab is not a function`.

- [ ] **Step 3: Declare it on the interface**

In `buffer-slice.ts`, add to `BufferActions` (after `openContent`):

```ts
  /**
   * Open (or re-focus) the pane's New Tab. A pane holds at most ONE — a second
   * identical blank is clutter, not a feature — so this is idempotent per pane
   * and returns the id either way.
   */
  openNewTab(paneId?: string): string
```

- [ ] **Step 4: Add the shared lookup**

Task 3 needs this same question answered ("does this pane hold a New Tab?"), so it
is one module-level helper, not two copies. Add above `createBufferSlice` in
`buffer-slice.ts`:

```ts
/** The New Tab a pane is holding, if any. A pane holds at most one. */
function findNewTabInPane(
  buffers: PaneContent[],
  pane: { bufferIds: string[] } | null | undefined,
): PaneContent | undefined {
  if (!pane) return undefined
  for (const id of pane.bufferIds) {
    const buf = buffers.find((b) => b.id === id)
    if (buf?.type === 'newTab') return buf
  }
  return undefined
}
```

- [ ] **Step 5: Implement the action**

In the `bufferActions` object, directly after `openContent`:

```ts
      openNewTab(paneId) {
        const targetPane = paneId ?? get().activePaneId
        const existing = findNewTabInPane(
          get().buffers,
          get().paneActions.getPaneById(targetPane),
        )
        if (existing) {
          get().paneActions.setActivePane(targetPane)
          get().paneActions.activatePaneBuffer(targetPane, existing.id)
          return existing.id
        }
        const id = nanoid()
        set((state) => {
          state.buffers.push({
            id,
            type: 'newTab',
            path: '',
            name: 'New Tab',
            isPinned: false,
            isPreview: false,
            isActive: false,
          } satisfies NewTabContent)
        })
        get().paneActions.addBufferToPane(targetPane, id, true)
        return id
      },
```

- [ ] **Step 6: Run the test and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`
Expected: PASS, 4 tests.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts
git commit -m "feat(panes): add openNewTab, one New Tab per pane"
```

---

### Task 3: Opening anything consumes an untouched New Tab

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` (`openContent`, just before `addBufferToPane` at ~line 291)
- Test: `web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts` (extend)

**Interfaces:**
- Consumes: `openNewTab` and the module-level `findNewTabInPane(buffers, pane)` helper, both from Task 2.
- Produces: no new API. `openContent` now removes an untouched New Tab from the pane it is opening into.

- [ ] **Step 1: Write the failing test**

Append to `buffer-slice-new-tab.test.ts`, inside a new `describe`:

```ts
describe('buffer-slice — New Tab is consumed', () => {
  let ctx: ReturnType<typeof makeStore>
  beforeEach(() => {
    ctx = makeStore()
  })

  it('opening a file in the same pane replaces the New Tab', () => {
    const newTabId = ctx.store.getState().bufferActions.openNewTab('root')
    ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeUndefined()
    expect(ctx.store.getState().buffers.filter((b) => b.type === 'newTab')).toHaveLength(0)
    expect(ctx.panes.root).not.toContain(newTabId)
  })

  it('opening in a DIFFERENT pane leaves the New Tab alone', () => {
    const newTabId = ctx.store.getState().bufferActions.openNewTab('split-1')
    ctx.store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: '',
    })
    expect(ctx.store.getState().buffers.find((b) => b.id === newTabId)).toBeDefined()
  })

  it('openNewTab itself does not consume the New Tab it just found', () => {
    const first = ctx.store.getState().bufferActions.openNewTab('root')
    expect(ctx.store.getState().bufferActions.openNewTab('root')).toBe(first)
  })
})
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`
Expected: FAIL — first test: the `newTab` buffer is still present.

- [ ] **Step 3: Consume in `openContent`**

In `openContent`, immediately **before** the existing `get().paneActions.addBufferToPane(get().activePaneId, id, true)` line (~291):

```ts
        // A New Tab is a placeholder for "something will go here". The moment
        // something does, it has served its purpose — replace it in place rather
        // than leaving a blank tab sitting next to the thing it produced. Scoped
        // to the target pane: a New Tab in another split is not this one's business.
        const targetPaneId = get().activePaneId
        const staleNewTab = findNewTabInPane(
          get().buffers,
          get().paneActions.getPaneById(targetPaneId),
        )
        if (staleNewTab) {
          get().paneActions.removeBufferFromPane(targetPaneId, staleNewTab.id, true)
          set((state) => {
            state.buffers = state.buffers.filter((b) => b.id !== staleNewTab.id)
          })
        }
```

> `preserveEmptyPane = true` is deliberate: the pane is about to receive the new
> buffer, so it must not be torn down for being momentarily empty.

- [ ] **Step 4: Run the test and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`
Expected: PASS, 7 tests.

- [ ] **Step 5: Run the full buffer-slice suite for regressions**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/`
Expected: PASS — no existing test breaks.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts
git commit -m "feat(panes): opening content consumes an untouched New Tab"
```

---

### Task 4: A pane is never tab-less

**Files:**
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts:212-264` (`removeBufferFromPane`)
- Test: `web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts` (extend)

**Interfaces:**
- Consumes: `bufferActions.openNewTab(paneId)` from Task 2.
- Produces: after `removeBufferFromPane` empties a **surviving** pane, that pane holds exactly one `newTab` buffer, flagged `isUncloseable`.

- [ ] **Step 1: Write the failing test**

Append to `pane-slice.test.ts`. Note the store double must now carry `bufferActions`:

```ts
describe('pane-slice — a pane is never tab-less', () => {
  it('spawns a New Tab when the last tab in the root pane closes', () => {
    const openNewTab = vi.fn()
    const store = createStore<PaneSlice & { bufferActions: { openNewTab: typeof openNewTab } }>()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        bufferActions: { openNewTab },
      })),
    )
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')
    expect(openNewTab).toHaveBeenCalledWith(ROOT_PANE_ID)
  })

  it('does NOT spawn one when the removed buffer was itself a New Tab', () => {
    const openNewTab = vi.fn()
    const store = createStore<
      PaneSlice & { bufferActions: { openNewTab: typeof openNewTab }; buffers: unknown[] }
    >()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        bufferActions: { openNewTab },
        buffers: [{ id: 'nt-1', type: 'newTab' }],
      })),
    )
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'nt-1')
    // Otherwise removing a New Tab mints a New Tab, forever.
    expect(openNewTab).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`
Expected: FAIL — `openNewTab` was never called.

- [ ] **Step 3: Spawn after the mutation**

In `pane-slice.ts`, `removeBufferFromPane`, capture the removed buffer's type **before** the `set(...)` block:

```ts
      removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
        // Read the type BEFORE the mutation: after it, the buffer may be gone.
        // A New Tab must never spawn its own replacement, or the pane can never
        // be emptied and closeBuffer recurses.
        const removedWasNewTab =
          (get().buffers as PaneContent[] | undefined)?.find((b) => b.id === bufferId)?.type ===
          'newTab'
```

Then **after** the closing `})` of the existing `set(...)` call, append:

```ts
        // A pane is never tab-less: an empty pane that SURVIVED (root, bottom, or
        // an explicit preserveEmptyPane) gets a New Tab so there is always
        // something to look at and somewhere to start from.
        const survivor = get().panes[paneId]
        if (!removedWasNewTab && survivor && survivor.bufferIds.length === 0) {
          get().bufferActions.openNewTab(paneId)
        }
      },
```

Add `PaneContent` to the type imports at the top of `pane-slice.ts` if it is not already imported.

- [ ] **Step 4: Run the test and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing closeability test**

Append to the same describe:

```ts
  it('marks a sole New Tab uncloseable, and a New Tab beside others closeable', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-1', type: 'newTab', isUncloseable: false },
      { id: 'buf-1', type: 'editor', isUncloseable: false },
    ])
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(true)

    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(false)
  })
```

Add this helper near the top of the file:

```ts
function makeStoreWithBuffers(buffers: Array<Record<string, unknown>>) {
  return createStore<PaneSlice & { buffers: Array<Record<string, unknown>> }>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
      buffers,
      bufferActions: { openNewTab: vi.fn() },
    })),
  )
}
```

- [ ] **Step 6: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`
Expected: FAIL — `isUncloseable` is still `false` after the first add.

- [ ] **Step 7: Sync closeability on membership change**

Add this module-level helper to `pane-slice.ts`, above `createPaneSlice`:

```ts
/**
 * A New Tab that is a pane's ONLY tab has no close affordance: closing it would
 * immediately spawn another (see removeBufferFromPane), so the × would be a
 * button that visibly does nothing. Beside other tabs it closes normally.
 * Derived state, so it is re-synced wherever a pane's membership changes.
 */
function syncNewTabCloseability(state: { panes: Record<string, { bufferIds: string[] }>; buffers?: PaneContent[] }, paneId: string): void {
  const pane = state.panes[paneId]
  if (!pane || !Array.isArray(state.buffers)) return
  const sole = pane.bufferIds.length === 1
  for (const id of pane.bufferIds) {
    const buf = state.buffers.find((b) => b.id === id)
    if (buf?.type === 'newTab') buf.isUncloseable = sole
  }
}
```

Call it as the last statement inside the `set(...)` block of **`addBufferToPane`**, **`removeBufferFromPane`** and **`moveBufferToPane`** (for `moveBufferToPane`, call it for both `fromPaneId` and `toPaneId`):

```ts
          syncNewTabCloseability(state, paneId)
```

- [ ] **Step 8: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add web/src/features/workspace/stores/slices/pane-slice.ts web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
git commit -m "feat(panes): spawn a New Tab when a pane empties; sole New Tab is uncloseable"
```

---

### Task 5: Show the New Tab in the tab strip

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx:63`
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx` (icon switch)
- Test: `web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx` (extend)

**Interfaces:**
- Consumes: nothing.
- Produces: a `newTab` buffer renders a tab labelled "New Tab" carrying the Crowbar mark, with no × when `isUncloseable`.

- [ ] **Step 1: Write the failing test**

Append to `tab-bar-item.test.tsx`:

```ts
  it('renders a New Tab with its label and no close button when uncloseable', () => {
    const buffer = {
      id: 'nt-1',
      type: 'newTab' as const,
      path: '',
      name: 'New Tab',
      isPinned: false,
      isPreview: false,
      isActive: true,
      isUncloseable: true,
    }
    render(
      <TabBarItem
        buffer={buffer}
        displayName="New Tab"
        index={0}
        isActive
        isDraggedTab={false}
        onDoubleClick={vi.fn()}
        onContextMenu={vi.fn()}
        onKeyDown={vi.fn()}
        handleTabClose={vi.fn()}
        handleTabPin={vi.fn()}
      />,
    )
    expect(screen.getByText('New Tab')).toBeInTheDocument()
    expect(screen.queryByLabelText(/close/i)).not.toBeInTheDocument()
  })
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/tabs/components/tab-bar-item.test.tsx`
Expected: FAIL — no icon case for `newTab`; the tab renders without its mark.

- [ ] **Step 3: Stop filtering `newTab` out of the strip**

In `tab-bar.tsx:63`, change:

```ts
      if (b && b.type !== 'newTab') next.push(b)
```

to:

```ts
      if (b) next.push(b)
```

Delete the now-false comment at `:112` that reads "newTab buffers are already filtered out inside the hook."

- [ ] **Step 4: Give it an icon**

In `tab-bar-item.tsx`, add to the imports:

```ts
import { CrowbarMark } from '@/components/ui/crowbar-mark'
```

and add a `newTab` case to the icon switch, beside the existing `terminal` case:

```tsx
  if (buffer.type === 'newTab') {
    return <CrowbarMark className="size-4 shrink-0 text-muted-foreground" />
  }
```

- [ ] **Step 5: Create the mark component**

Create `web/src/components/ui/crowbar-mark.tsx`. This is the glyph alone (the wordmark
component already in the tree is the full lockup, which is the wrong shape for a 16px tab):

```tsx
import type React from 'react'

/** The Crowbar mark alone — no wordmark. Source: desktop/src-tauri/icons/crowbar.icon. */
export function CrowbarMark(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 146 145" fill="currentColor" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path d="M72.873 15C104.836 15 130.747 40.7436 130.747 72.5C130.747 104.256 104.836 130 72.873 130C40.9106 130 15 104.256 15 72.5C15 40.7437 40.9106 15.0002 72.873 15ZM74.9004 42.8174C72.2697 42.3669 69.5369 42.7093 66.542 44.4268C61.4646 47.3388 57.6943 50.8091 56.4541 56.2373C54.5839 64.4226 51.3599 72.0406 47.9434 79.4844C47.4037 80.66 46.7269 81.8276 46.1729 82.8574C45.5963 83.9291 45.1125 84.9197 44.8125 85.9287C42.9488 92.1979 41.3662 99.9677 40.5137 106.489C40.805 106.437 41.1189 106.36 41.3232 106.248C43.5044 105.052 45.6522 103.793 47.8574 102.644C49.1085 101.991 49.6782 101.034 49.9277 99.6807C50.7951 94.972 52.1261 90.3742 53.5264 85.7988C53.6344 85.4454 53.6387 84.9585 54.1865 84.8115C54.5877 88.7651 54.956 92.6482 55.3867 96.5234C55.6002 98.4432 56.2162 98.7468 57.8789 97.8203C61.0656 96.0446 64.2321 94.2327 67.415 92.4502C68.2328 91.9922 68.7259 91.4043 68.9033 90.4219C69.2932 88.2623 69.8475 86.1317 70.3047 83.9834C71.4089 78.7947 73.0867 73.6389 71.3145 68.2656C71.2642 68.1127 71.2941 67.9348 71.2578 67.7754C70.7202 65.4256 71.733 63.5901 73.251 61.9443C74.6855 60.3892 76.2762 60.2829 77.9375 61.5732C78.8633 62.2925 79.6177 63.1636 80.2705 64.1289C82.8309 67.9155 84.5382 72.0859 85.748 76.4551C88.4209 86.108 91.0353 95.7776 93.6719 105.44C94.6917 109.178 95.7912 111.705 96.8447 113.429C101.364 110.81 105.408 107.473 108.823 103.576C107.36 98.3302 105.1 90.5395 102.061 79.501C100.191 72.7122 98.5796 66.1397 95.1748 60.2295C91.9494 54.631 88.3876 49.6184 83.0322 46.3027C80.2471 44.5785 77.5865 43.2774 74.9004 42.8174ZM72.873 25.1533C46.5546 25.1535 25.2197 46.3513 25.2197 72.5C25.2197 84.8411 29.975 96.0764 37.7607 104.504C38.6681 98.1999 40.1742 91.0056 41.9375 85.0742C42.3288 83.758 42.9347 82.5452 43.5312 81.4365C44.1503 80.286 44.7299 79.294 45.2168 78.2334C48.6182 70.8227 51.7325 63.437 53.5303 55.5684C55.0427 48.9493 59.6644 44.9123 65.0488 41.8242C68.6535 39.757 72.0946 39.293 75.4072 39.8604C78.6643 40.4182 81.7172 41.9603 84.6113 43.752L85.168 44.1055C90.859 47.8174 94.5872 53.2002 97.7744 58.7324C101.415 65.0516 103.125 72.064 104.953 78.7041C107.65 88.5 109.672 95.5276 111.137 100.723C117.036 92.8415 120.527 83.0764 120.527 72.5C120.527 46.3512 99.1916 25.1533 72.873 25.1533Z" />
    </svg>
  )
}
```

- [ ] **Step 6: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/tabs/components/`
Expected: PASS — including the existing tab-bar rerender tests.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx web/src/features/tabs/components/tab-bar-item.tsx web/src/components/ui/crowbar-mark.tsx web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx
git commit -m "feat(tabs): render the New Tab in the tab strip"
```

---

### Task 6: The Nameplate surface

**Files:**
- Create: `web/src/features/panes/components/new-tab-view.tsx`
- Modify: `web/src/styles/theme.css` (four theme blocks + container-query rules)
- Test: `web/src/__tests__/features/panes/components/new-tab-view.test.tsx` (create)

**Interfaces:**
- Consumes: `TAB_NEW_TERMINAL`, `TAB_NEW_FILE`, `AGENT_NEW_CHAT` (Task 1).
- Produces: `<NewTabView />` — no props; reads workspace/chat state from the store.

- [ ] **Step 1: Add the theme token and container rules**

`theme.css` has exactly two colour-token blocks: `:root` and `.dark`. (`data-theme`
carries the theme *id* — `crowbar`/`zen` — not light/dark; `settings-effects.ts`
toggles the `.dark` class.) Add to `:root`:

```css
  /* Isologo fill, defined per-theme rather than as a descendant override on the
     component, so it cannot go stale for one of the two grounds. */
  --logo-ink: var(--primary);
```

and to `.dark`:

```css
  --logo-ink: oklch(0.62 0.076 122);
```

Then append the container-query rules at the end of the file — see the committed
`theme.css` for the exact block.

- [ ] **Step 2: Write the failing test**

Create `web/src/__tests__/features/panes/components/new-tab-view.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { NewTabView } from '@/features/panes/components/new-tab-view'

const { chordMap } = vi.hoisted(() => ({ chordMap: { current: {} as Record<string, string> } }))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => chordMap.current,
}))

const { chats } = vi.hoisted(() => ({ chats: { current: [] as Array<Record<string, unknown>> } }))

vi.mock('@tanstack/react-router', () => ({
  useRouterState: () => '/ide/p1/r1/ws-1',
}))

vi.mock('@/components/layout/context-pill-model', () => ({
  deriveContextPillModel: () => ({ kind: 'workspace', branchName: 'enhancement/restyling' }),
}))

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (sel: (s: unknown) => unknown) =>
    sel({
      agentChats: { chats: chats.current, working: {}, order: [] },
      workspaceId: 'ws-1',
    }),
  useWorkspaceStore: () => ({ getState: () => ({ bufferActions: { openContent: vi.fn() } }) }),
}))

const makeChats = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    id: `c${i}`,
    title: `Chat ${i}`,
    providerIcon: '<svg />',
    updatedAt: 1000 - i,
  }))

describe('NewTabView', () => {
  it('renders the three create actions', () => {
    chats.current = []
    render(<NewTabView />)
    expect(screen.getByRole('button', { name: /new file/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new terminal/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new chat/i })).toBeInTheDocument()
  })

  it('shows chord badges resolved from the keymap, NOT hardcoded defaults', () => {
    // The whole reason these go through the registry: rebinding must move the badge.
    chordMap.current = { 'tabs.newTerminal': 'mod+shift+9' }
    chats.current = []
    render(<NewTabView />)
    expect(screen.getByText('⌘⇧9')).toBeInTheDocument()
    expect(screen.queryByText('⌘J')).not.toBeInTheDocument()
    chordMap.current = {}
  })

  it('caps history at three rows and hands the rest off', () => {
    chats.current = makeChats(24)
    render(<NewTabView />)
    expect(screen.getAllByTestId('nt-chat-row')).toHaveLength(3)
    expect(screen.getByText(/21 more in this worktree/i)).toBeInTheDocument()
  })

  it('renders no hand-off row when everything fits', () => {
    chats.current = makeChats(2)
    render(<NewTabView />)
    expect(screen.getAllByTestId('nt-chat-row')).toHaveLength(2)
    expect(screen.queryByText(/more in this worktree/i)).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 3: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/panes/components/new-tab-view.test.tsx`
Expected: FAIL — cannot resolve `new-tab-view`.

- [ ] **Step 4: Write the component**

Create `web/src/features/panes/components/new-tab-view.tsx`:

```tsx
import { useCallback, useMemo } from 'react'
import { File as FileIcon, TerminalWindow, ChatCircle } from '@phosphor-icons/react'
import { CrowbarWordmark } from '@/components/ui/crowbar-wordmark'
import { AgentChatGlyph } from '@/features/agent/components/agent-chat-glyph'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { formatChord } from '@/features/keymaps/utils/chord'
import {
  AGENT_NEW_CHAT,
  SIDEBAR_TAB_CHATS,
  TAB_NEW_FILE,
  TAB_NEW_TERMINAL,
} from '@/features/keymaps/registry'
import { useWorkspaceStore, useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { deriveContextPillModel } from '@/components/layout/context-pill-model'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'
import { useRouterState } from '@tanstack/react-router'

/** Matches the three create actions, so the two columns stay the same height. */
const HISTORY_CAP = 3

/**
 * "Where am I" — the branch for a repo workspace, the project name for Project
 * Home. Same derivation the context pill uses (components/layout/context-pill.tsx),
 * so the two can never disagree about what workspace you are looking at.
 */
function useContextLabel(): string {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const activeWorkspaceId = parseWorkspaceScopeFromPath(pathname)?.wsId
  const model = deriveContextPillModel({
    activeWorkspaceId,
    isHomeRoute: !activeWorkspaceId && /\/ide\/[^/]+\/home$/.test(pathname),
    repos,
    projects,
    activeProjectId,
  })
  if (model.kind === 'workspace') return model.branchName
  if (model.kind === 'home' || model.kind === 'project') return model.projectName
  return ''
}

function Chord({ commandId }: { commandId: string }) {
  const chordMap = useEffectiveChordMap()
  const chord = chordMap[commandId]
  if (!chord) return null
  return (
    <kbd className="shrink-0 rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
      {formatChord(chord)}
    </kbd>
  )
}

/**
 * The surface a pane shows when it holds nothing yet — and, since it is a real
 * `newTab` buffer, the surface ⌘T opens.
 *
 * Everything sits as ONE cluster on the floor of the pane, reading top-down:
 * isologo → worktree → what you can do in it. The cluster is anchored rather
 * than centred so it does not drift as a split resizes, and the two columns are
 * bottom-aligned with matched row heights so they read as one list.
 */
export function NewTabView() {
  const workspaceStore = useWorkspaceStore()
  // The per-workspace store holds only `workspaceId`; the human-readable branch
  // lives in the sidebar store. `deriveContextPillModel` already resolves that —
  // and already handles Project Home, which has a project name and no branch —
  // so reuse it rather than re-deriving and drifting from the context pill.
  const label = useContextLabel()
  const chats = useWorkspaceStoreContext((s) => s.agentChats.chats)
  const working = useWorkspaceStoreContext((s) => s.agentChats.working)

  const recent = useMemo(() => [...chats].slice(0, HISTORY_CAP), [chats])
  const overflow = chats.length - recent.length

  const openTerminal = useCallback(() => {
    workspaceStore.getState().bufferActions.openContent({ type: 'terminal' })
  }, [workspaceStore])

  // A virtual buffer needs neither a target directory nor write access, so New
  // File works on a locked worktree (protected branches, Project Home) where the
  // file-explorer's New File is hidden outright.
  const openFile = useCallback(() => {
    workspaceStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'untitled:Untitled',
      name: 'Untitled',
      content: '',
      isVirtual: true,
    })
  }, [workspaceStore])

  const openChats = useCallback(() => useSidebarStore.getState().setActiveTab('chats'), [])

  return (
    <div className="relative h-full w-full">
      <div className="absolute inset-x-6 bottom-5 flex flex-col items-start gap-4">
        <div className="px-2.5">
          <CrowbarWordmark
            className="mb-3 block h-auto w-[clamp(108px,15cqw,152px)] text-[var(--logo-ink)]"
            role="img"
            aria-label="Crowbar"
          />
          <h2 className="ui-text-sm font-semibold text-foreground">{label}</h2>
        </div>

        <div className="nt-cols flex flex-col items-start gap-3.5">
          <div className="flex w-[262px] flex-col gap-0.5">
            <ActionRow icon={<FileIcon />} label="New File" commandId={TAB_NEW_FILE} onClick={openFile} />
            <ActionRow icon={<TerminalWindow />} label="New Terminal" commandId={TAB_NEW_TERMINAL} onClick={openTerminal} />
            <ActionRow icon={<ChatCircle />} label="New Chat" commandId={AGENT_NEW_CHAT} onClick={openChats} />
          </div>

          {chats.length > 0 && (
            <div className="nt-history flex w-[262px] flex-col gap-0.5">
              {recent.map((chat) => (
                <button
                  key={chat.id}
                  type="button"
                  data-testid="nt-chat-row"
                  onClick={openChats}
                  className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] hover:bg-muted"
                >
                  <AgentChatGlyph
                    providerIcon={chat.providerIcon ?? ''}
                    working={Boolean(working[chat.id])}
                    className="size-3.5"
                  />
                  <span className="ui-text-xs min-w-0 flex-1 truncate text-foreground">{chat.title}</span>
                </button>
              ))}
              {overflow > 0 && (
                <button
                  type="button"
                  onClick={openChats}
                  className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] text-muted-foreground hover:bg-muted"
                >
                  <span className="ui-text-xs min-w-0 flex-1 truncate pl-[22px]">
                    {overflow} more in this worktree
                  </span>
                  <Chord commandId={SIDEBAR_TAB_CHATS} />
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function ActionRow({
  icon,
  label,
  commandId,
  onClick,
}: {
  icon: React.ReactNode
  label: string
  commandId: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      // min-h + leading pinned so these rows and the chat rows are the same
      // height: one is a <button> and one a <div>, and their default line boxes
      // differ by a pixel, which reads as two staggered columns.
      className="flex min-h-[30px] w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left leading-[18px] hover:bg-muted"
    >
      <span className="shrink-0 text-muted-foreground [&_svg]:size-3.5">{icon}</span>
      <span className="ui-text-xs min-w-0 flex-1 text-foreground">{label}</span>
      <Chord commandId={commandId} />
    </button>
  )
}
```

- [ ] **Step 5: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/panes/components/new-tab-view.test.tsx`
Expected: PASS, 4 tests.

- [ ] **Step 6: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: no errors. If `workspaceName` does not exist on the workspace store, use the field that does — check `workspace-store.types.ts` and adjust the selector.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/panes/components/new-tab-view.tsx web/src/styles/theme.css web/src/__tests__/features/panes/components/new-tab-view.test.tsx
git commit -m "feat(panes): add the New Tab surface"
```

---

### Task 7: Render it, and delete the old empty state

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx:131`, `:468` (switch), `:592` (fallback)
- Delete: `web/src/features/panes/components/empty-editor-state.tsx`
- Test: `web/src/__tests__/features/panes/components/pane-container-new-tab.test.tsx` (create)

**Interfaces:**
- Consumes: `<NewTabView />` (Task 6).

- [ ] **Step 1: Stop skipping `newTab` in the render list**

At `pane-container.tsx:131`, delete this line:

```ts
      if (buffer.type === 'newTab') return []
```

- [ ] **Step 2: Add the render case**

Add `newTab` to the `renderActiveBuffer` switch (~line 470), and import the component:

```tsx
import { NewTabView } from './new-tab-view'
```

```tsx
        case 'newTab':
          return <NewTabView />
```

- [ ] **Step 3: Repoint the fallback**

At `:592`, replace:

```tsx
        {!activeBuffer && <EmptyEditorState />}
```

with:

```tsx
        {/* Under the New Tab rules a pane always holds at least one buffer, so
            this should be unreachable. Kept — pointed at the same component — so
            that if a bug ever does strand a pane with no tabs, it shows a usable
            surface instead of a blank rectangle with no way out. */}
        {!activeBuffer && <NewTabView />}
```

Remove the now-unused `EmptyEditorState` import.

- [ ] **Step 4: Delete the old component**

```bash
git rm web/src/features/panes/components/empty-editor-state.tsx
```

- [ ] **Step 5: Confirm nothing still imports it**

Run: `cd web && grep -rn "empty-editor-state\|EmptyEditorState" src/`
Expected: no output.

- [ ] **Step 6: Run the pane-container suite**

Run: `cd web && bunx vitest run src/__tests__/features/panes/`
Expected: PASS.

- [ ] **Step 7: Typecheck and commit**

```bash
cd web && bunx tsc --noEmit
git add -A web/src/features/panes/components/
git commit -m "feat(panes): render the New Tab surface, remove EmptyEditorState"
```

---

### Task 8: Wire the chords

**Files:**
- Modify: `web/src/features/panes/hooks/use-pane-keyboard.ts:14` (imports), `:47` (handlers)
- Test: `web/src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts` (extend)

**Interfaces:**
- Consumes: `TAB_NEW`, `TAB_NEW_FILE` (Task 1); `openNewTab` (Task 2).

- [ ] **Step 1: Write the failing test**

Append to the existing `use-pane-keyboard` test file:

```ts
  it('mod+t opens a New Tab, not a terminal', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true }))
    expect(openNewTab).toHaveBeenCalledTimes(1)
    expect(openContent).not.toHaveBeenCalledWith({ type: 'terminal' })
  })

  it('mod+j opens a terminal', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', metaKey: true }))
    expect(openContent).toHaveBeenCalledWith({ type: 'terminal' })
  })

  it('mod+n opens an untitled virtual buffer', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true }))
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'editor', isVirtual: true }),
    )
  })
```

Add `openNewTab: vi.fn()` to whatever `bufferActions` double the file already builds.

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts`
Expected: FAIL — mod+t still calls `openContent({ type: 'terminal' })`.

- [ ] **Step 3: Update the imports**

In `use-pane-keyboard.ts`, add to the registry import block:

```ts
  TAB_NEW,
  TAB_NEW_FILE,
```

- [ ] **Step 4: Replace the handler**

Replace the existing `TAB_NEW_TERMINAL` block (~line 47) with:

```ts
      if (matches(TAB_NEW)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openNewTab()
        return
      }

      if (matches(TAB_NEW_TERMINAL)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openContent({ type: 'terminal' })
        return
      }

      if (matches(TAB_NEW_FILE)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openContent({
          type: 'editor',
          path: 'untitled:Untitled',
          name: 'Untitled',
          content: '',
          isVirtual: true,
        })
        return
      }
```

- [ ] **Step 5: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/panes/hooks/use-pane-keyboard.ts web/src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts
git commit -m "feat(keymaps): wire New Tab, New Terminal (mod+j) and New File"
```

---

### Task 9: Never persist a New Tab

**Files:**
- Create: `web/src/features/workspace/stores/persisted-layout.ts`
- Modify: `web/src/features/workspace/stores/workspace-store-registry.ts:69-79`
- Test: `web/src/__tests__/features/workspace/stores/persisted-layout.test.ts` (create)

**Interfaces:**
- Produces: `stripNewTabs(snapshot)` — takes `{ buffers, panes }` and returns the
  same shape with every `newTab` buffer removed **and** its id stripped from every
  pane's `bufferIds` / `activeBufferId`.

> The save site is `workspace-store-registry.ts:69`, which currently persists
> `buffers: state.buffers` unfiltered. Note `panes` is persisted alongside it, so
> dropping a buffer without also stripping its id from `pane.bufferIds` strands
> that id — exactly the hazard `pane-slice.ts` warns about ("activating one of
> those ghosts blanked the pane in the running app"). Both must be filtered
> together, which is why this is one pure function rather than an inline
> `.filter`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/stores/persisted-layout.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { stripNewTabs } from '@/features/workspace/stores/persisted-layout'

describe('stripNewTabs', () => {
  const snapshot = {
    buffers: [
      { id: 'nt-1', type: 'newTab', path: '', name: 'New Tab' },
      { id: 'e-1', type: 'editor', path: '/a.ts', name: 'a.ts' },
    ],
    panes: {
      root: { id: 'root', bufferIds: ['nt-1', 'e-1'], activeBufferId: 'nt-1' },
      split: { id: 'split', bufferIds: ['nt-2'], activeBufferId: 'nt-2' },
    },
  } as never

  it('drops newTab buffers', () => {
    expect(stripNewTabs(snapshot).buffers.map((b) => b.id)).toEqual(['e-1'])
  })

  it('strips their ids out of pane membership, so no id is left stranded', () => {
    expect(stripNewTabs(snapshot).panes.root.bufferIds).toEqual(['e-1'])
  })

  it('repoints activeBufferId when it pointed at a New Tab', () => {
    const out = stripNewTabs(snapshot)
    expect(out.panes.root.activeBufferId).toBe('e-1')
    // Nothing left to activate — null, never a dangling id.
    expect(out.panes.split.activeBufferId).toBeNull()
  })
})
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/persisted-layout.test.ts`
Expected: FAIL — cannot resolve `persisted-layout`.

- [ ] **Step 3: Write the function**

Create `web/src/features/workspace/stores/persisted-layout.ts`:

```ts
import type { PaneContent } from '@/features/panes/types/pane-content'

interface PersistablePane {
  id: string
  bufferIds: string[]
  activeBufferId: string | null
  [key: string]: unknown
}

interface Snapshot {
  buffers: PaneContent[]
  panes: Record<string, PersistablePane>
}

/**
 * A New Tab carries no state, and restoring one would fight the rule that a
 * workspace always opens on a fresh New Tab.
 *
 * Pane membership is persisted alongside the buffers, so dropping a buffer
 * WITHOUT stripping its id from `bufferIds` leaves a stranded id — the pane then
 * activates a buffer that does not exist and renders blank (see the comment in
 * pane-slice's removeBufferFromPane). Both halves move together, here.
 */
export function stripNewTabs<T extends Snapshot>(snapshot: T): T {
  const doomed = new Set(
    snapshot.buffers.filter((b) => b.type === 'newTab').map((b) => b.id),
  )
  if (doomed.size === 0) return snapshot

  const panes: Record<string, PersistablePane> = {}
  for (const [paneId, pane] of Object.entries(snapshot.panes)) {
    const bufferIds = pane.bufferIds.filter((id) => !doomed.has(id))
    panes[paneId] = {
      ...pane,
      bufferIds,
      activeBufferId:
        pane.activeBufferId && doomed.has(pane.activeBufferId)
          ? (bufferIds[0] ?? null)
          : pane.activeBufferId,
    }
  }

  return {
    ...snapshot,
    buffers: snapshot.buffers.filter((b) => !doomed.has(b.id)),
    panes,
  }
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/stores/persisted-layout.test.ts`
Expected: PASS, 3 tests.

- [ ] **Step 5: Use it at the save site**

In `workspace-store-registry.ts`, add the import:

```ts
import { stripNewTabs } from './persisted-layout'
```

and wrap the persisted fields (replacing the `panes:` and `buffers:` lines at ~:71 and ~:77):

```ts
        const persistable = stripNewTabs({ buffers: state.buffers, panes: state.panes })
        saveWorkspaceLayout({
          workspaceId: wsId,
          panes: persistable.panes,
          rootLayout: state.rootLayout,
          bottomLayout: state.bottomLayout,
          activePaneId: state.activePaneId,
          mostRecentActivePaneIds: state.mostRecentActivePaneIds,
          buffers: persistable.buffers,
          sidebarWidth: 0,
          rightSidebarWidth: 0,
          updatedAt: Date.now(),
        })
```

- [ ] **Step 6: Typecheck and commit**

```bash
cd web && bunx tsc --noEmit
git add web/src/features/workspace/stores/persisted-layout.ts web/src/features/workspace/stores/workspace-store-registry.ts web/src/__tests__/features/workspace/stores/persisted-layout.test.ts
git commit -m "feat(persistence): never persist a New Tab, and never strand its id"
```

---

### Task 10: A workspace opens on a New Tab

This is what makes Project Home "behave exactly like any other workspace" — it has
nothing to restore, so without this it would hydrate to a genuinely tab-less pane.

**Files:**
- Modify: `web/src/features/workspace/components/workspace-view.tsx` (after the hydration effect, ~line 92)
- Test: `web/src/__tests__/features/workspace/components/workspace-view-new-tab.test.tsx` (create)

**Interfaces:**
- Consumes: `bufferActions.openNewTab()` (Task 2).

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/workspace/components/workspace-view-new-tab.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useOpenOnNewTab } from '@/features/workspace/components/workspace-view'

const openNewTab = vi.fn()

function makeStore(bufferCount: number) {
  return {
    getState: () => ({
      buffers: Array.from({ length: bufferCount }, (_, i) => ({ id: `b${i}` })),
      bufferActions: { openNewTab },
    }),
  }
}

describe('useOpenOnNewTab', () => {
  beforeEach(() => openNewTab.mockClear())

  it('opens a New Tab when a hydrated workspace restored nothing', () => {
    renderHook(() => useOpenOnNewTab(makeStore(0) as never, true))
    expect(openNewTab).toHaveBeenCalledTimes(1)
  })

  it('does nothing when buffers were restored', () => {
    renderHook(() => useOpenOnNewTab(makeStore(3) as never, true))
    expect(openNewTab).not.toHaveBeenCalled()
  })

  it('waits for hydration — never races the restore', () => {
    renderHook(() => useOpenOnNewTab(makeStore(0) as never, false))
    expect(openNewTab).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/components/workspace-view-new-tab.test.tsx`
Expected: FAIL — `useOpenOnNewTab` is not exported.

- [ ] **Step 3: Add the hook**

In `workspace-view.tsx`, add above the `WorkspaceView` component:

```tsx
/**
 * A workspace never lands tab-less. Runs once hydration has settled — a
 * workspace with nothing to restore (a fresh worktree, or Project Home, which
 * has no files of its own) opens on a New Tab exactly like any other.
 *
 * Gated on `hydrated` rather than firing on mount: opening a New Tab before the
 * restore lands would race it and leave a blank tab beside the restored files.
 */
export function useOpenOnNewTab(store: WorkspaceStore, hydrated: boolean): void {
  useEffect(() => {
    if (!hydrated) return
    if (store.getState().buffers.length > 0) return
    store.getState().bufferActions.openNewTab()
  }, [store, hydrated])
}
```

Then call it inside `WorkspaceView`, after the existing hydration effects:

```tsx
  useOpenOnNewTab(store, hydrated)
```

Import `WorkspaceStore` as a type from `../stores/workspace-store` if it is not
already imported in this file.

- [ ] **Step 4: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/workspace/components/workspace-view-new-tab.test.tsx`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/components/workspace-view.tsx web/src/__tests__/features/workspace/components/workspace-view-new-tab.test.tsx
git commit -m "feat(workspace): open every workspace, including Project Home, on a New Tab"
```

---

### Task 11: ⌘W on a sole New Tab closes the split, never the tab

**Files:**
- Modify: `web/src/features/panes/hooks/use-pane-keyboard.ts` (`TAB_CLOSE` branch)
- Test: `web/src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts` (extend)

**Interfaces:**
- Consumes: `TAB_CLOSE` (existing); `paneActions.closePane` — confirm the exact
  name with `grep -n "closePane\|closeLayout" web/src/features/workspace/stores/slices/pane-slice.ts`
  and use whichever the slice exports.

- [ ] **Step 1: Write the failing test**

Append to `use-pane-keyboard.test.ts`:

```ts
  it('mod+w on a sole New Tab in a split closes the split pane', () => {
    setPaneState({ activeBufferId: 'nt-1', bufferIds: ['nt-1'], paneCount: 2 })
    setBuffers([{ id: 'nt-1', type: 'newTab' }])
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).toHaveBeenCalled()
    expect(removeBufferFromPane).not.toHaveBeenCalled()
  })

  it('mod+w on a sole New Tab in the LAST pane does nothing', () => {
    setPaneState({ activeBufferId: 'nt-1', bufferIds: ['nt-1'], paneCount: 1 })
    setBuffers([{ id: 'nt-1', type: 'newTab' }])
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).not.toHaveBeenCalled()
    expect(removeBufferFromPane).not.toHaveBeenCalled()
  })
```

Extend the file's existing store double with `closePane`, `setPaneState` and
`setBuffers` helpers matching the shape it already uses for `activeBufferId`.

- [ ] **Step 2: Run it and watch it fail**

Run: `cd web && bunx vitest run src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts`
Expected: FAIL — `removeBufferFromPane` was called.

- [ ] **Step 3: Add the guard**

In the `TAB_CLOSE` branch, immediately after the existing
`const buf = state.buffers.find((b) => b.id === bufferId)` line:

```ts
        // A sole New Tab has no close: closing it would spawn another (see
        // pane-slice's removeBufferFromPane), so ⌘W would visibly do nothing.
        // In a SPLIT that keystroke means "dismiss this split" — which is the
        // only reading that leaves the user anywhere new. In the last remaining
        // pane there is nowhere to go, so it is a genuine no-op.
        const pane = state.panes[paneId]
        if (buf?.type === 'newTab' && pane && pane.bufferIds.length === 1) {
          if (Object.keys(state.panes).length > 1) {
            state.paneActions.closePane(paneId)
          }
          return
        }
```

- [ ] **Step 4: Run it and watch it pass**

Run: `cd web && bunx vitest run src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts`
Expected: PASS.

- [ ] **Step 5: Full suite, lint and typecheck**

```bash
cd web && bun run test && bunx tsc --noEmit && bun run lint
```
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/panes/hooks/use-pane-keyboard.ts web/src/__tests__/features/panes/hooks/use-pane-keyboard.test.ts
git commit -m "feat(panes): mod+w on a sole New Tab dismisses the split, never the tab"
```

---

## Manual verification (required — not covered by any test)

Run the app: `make dev-desktop` (never against production Crowbar).

- [ ] ⌘T opens a New Tab; pressing it again focuses the same one rather than stacking blanks.
- [ ] Clicking New Terminal inside it **replaces** that tab — no leftover blank beside the terminal.
- [ ] Closing the last tab leaves a New Tab, and that tab has **no ×**.
- [ ] ⌘J opens a terminal; ⌘N opens an untitled buffer.
- [ ] Rebind New Terminal in Settings → Keybindings and confirm **the badge on the surface changes**.
- [ ] Project Home opens on a New Tab, identical to any other workspace.
- [ ] **Judge the isologo green against composited vibrancy**, not against `--pane-background` — vibrancy lightens whatever sits on it, so the token value is optimistic. Screenshot the real window; adjust `--logo-ink` if it reads washed out.
- [ ] Check the surface at a genuinely portrait window — the layout rules were measured in a browser rig, not in WKWebView.

## Deferred (do not implement)

`mod+j` is `Ctrl+J` off macOS, which is `^J` (line feed). `terminal.tsx:548` passes
Ctrl-without-Cmd to xterm and `use-pane-keyboard` has no focus guard, so both would
fire. macOS-only, so it cannot happen on the shipping platform. See the spec's
"Known issue" section before adding non-macOS support.
