# Workspace Store Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace global Athas singleton stores (`usePaneStore`, `useBufferStore`) with a per-workspace Zustand store composed of focused slices, accessed via React context, with localStorage persistence per workspace.

**Architecture:** Each workspace gets a `WorkspaceStore` instance created by `createWorkspaceStore(wsId)` and stored in a registry (`Map<wsId, StoreApi>`). The store is provided via `WorkspaceStoreContext`. Athas UI components are updated to call workspace-scoped hooks directly. The old global stores are deleted. Crowbar's `FlowContent` replaces `FlowTab` (which used `<Outlet />`), and `WorkspaceStepFooter` moves outside the pane area.

**Tech Stack:** React 18, Zustand 5 (`createStore` + `immer` middleware), TypeScript, Vitest, `@testing-library/react`, TanStack Router v2

---

## File Map

### Created
```
web/src/features/workflow/types/workflow.ts
web/src/features/workspace/stores/workspace-store.types.ts
web/src/features/workspace/stores/workspace-store.ts
web/src/features/workspace/stores/workspace-store-registry.ts
web/src/features/workspace/stores/workspace-persistence.ts
web/src/features/workspace/stores/workspace-context.ts
web/src/features/workspace/stores/slices/pane-slice.ts
web/src/features/workspace/stores/slices/buffer-slice.ts
web/src/features/workspace/stores/slices/workflow-slice.ts
web/src/features/workspace/stores/slices/lsp-slice.ts
web/src/features/workspace/stores/slices/terminal-slice.ts
web/src/features/workspace/stores/slices/file-watcher-slice.ts
web/src/features/workspace/stores/slices/recent-files-slice.ts
web/src/features/workspace/stores/hooks/use-pane-store.ts
web/src/features/workspace/stores/hooks/use-buffer-store.ts
web/src/features/workspace/stores/hooks/use-workflow.ts
web/src/features/workspace/stores/hooks/use-workspace-effects.ts
web/src/features/workspace/components/WorkspaceView.tsx
web/src/features/workspace/components/WorkspaceLayoutRoot.tsx
web/src/features/workspace/components/WorkspaceStepFooter.tsx
web/src/features/workflow/components/FlowContent.tsx
web/src/features/workflow/components/ChatView.tsx
web/src/features/workflow/components/DiffView.tsx
web/src/features/workflow/components/SplitView.tsx

web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts
web/src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts
web/src/__tests__/features/workspace/stores/workspace-store-registry.test.ts
web/src/__tests__/features/workspace/stores/workspace-persistence.test.ts
```

### Modified
```
web/src/features/panes/components/split-view-root.tsx
web/src/features/panes/components/pane-node-renderer.tsx
web/src/features/panes/components/pane-container.tsx
web/src/features/tabs/components/tab-bar.tsx
web/src/features/tabs/components/tab-bar-item.tsx
web/src/features/athas-editor/hooks/use-lsp-integration.ts
web/src/features/athas-editor/hooks/use-lsp-initialization.ts
web/src/components/layout/IDEShell.tsx
web/src/main.tsx
```

### Deleted
```
web/src/features/panes/stores/pane-store.ts
web/src/features/editor/stores/buffer-store.ts
web/src/components/layout/FlowTab.tsx
```

---

## Phase 1 — Type Foundations

### Task 1: Workflow Types

**Files:**
- Create: `web/src/features/workflow/types/workflow.ts`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workflow/types/workflow.ts

export type StepContentType = 'chat' | 'diff' | 'split'

export interface FlowStep {
  id: string
  label: string
  icon?: string
  contentType: StepContentType
  isCompleted: boolean
  isActive: boolean
}

export interface FlowDefinition {
  flowId: string
  /** Identifies the flow template, e.g. 'crowbar-default'. Extensible to any flow type. */
  flowType: string
  steps: FlowStep[]
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/workflow/types/workflow.ts
git commit -m "feat(workspace): workflow type definitions"
```

---

### Task 2: WorkspaceState Union Type

This file is imported by all slices to type their `set`/`get` parameters. It avoids circular imports by declaring the full state type in one place.

**Files:**
- Create: `web/src/features/workspace/stores/workspace-store.types.ts`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workspace/stores/workspace-store.types.ts
import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'
import type { WorkflowSlice } from './slices/workflow-slice'
import type { LspSlice } from './slices/lsp-slice'
import type { TerminalSlice } from './slices/terminal-slice'
import type { FileWatcherSlice } from './slices/file-watcher-slice'
import type { RecentFilesSlice } from './slices/recent-files-slice'

export interface WorkspaceBaseState {
  workspaceId: string
}

export type WorkspaceState =
  & WorkspaceBaseState
  & PaneSlice
  & BufferSlice
  & WorkflowSlice
  & LspSlice
  & TerminalSlice
  & FileWatcherSlice
  & RecentFilesSlice
```

This file has forward-reference imports — TypeScript resolves them at compile time even before the slice files exist. The plan creates slices next.

- [ ] **Step 2: Commit**

```bash
git add web/src/features/workspace/stores/workspace-store.types.ts
git commit -m "feat(workspace): WorkspaceState union type"
```

---

## Phase 2 — Slices

Each slice file exports:
1. A `*Slice` TypeScript interface describing the state shape
2. A `create*Slice(set, get)` factory function returning the initial state + actions

All slices use Immer (mutations in `set` callbacks are safe). Cross-slice reads use `get()`.

### Task 3: Pane Slice

**Files:**
- Create: `web/src/features/workspace/stores/slices/pane-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`

Key note: `splitPane` from `pane-tree.ts` returns the new `PaneNode` root (not the new pane ID). To recover the new pane ID, diff old vs new pane groups before/after the split.

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { getAllPaneGroups } from '@/features/panes/utils/pane-tree'

function makeStore() {
  return createStore<PaneSlice>()(immer((set, get) => createPaneSlice(set as any, get as any)))
}

describe('pane-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('initialises with a single empty root group', () => {
    const { paneRoot } = store.getState()
    expect(paneRoot.type).toBe('group')
    expect(paneRoot.id).toBe(ROOT_PANE_ID)
    expect(paneRoot.bufferIds).toEqual([])
    expect(paneRoot.activeBufferId).toBeNull()
  })

  it('splitPane returns a new pane ID and updates root to a split', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    const root = store.getState().paneRoot
    expect(root.type).toBe('split')
    const groups = getAllPaneGroups(root)
    expect(groups.map(g => g.id)).toContain(newPaneId!)
  })

  it('addBufferToPane adds bufferId to the correct group', () => {
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const root = store.getState().paneRoot
    expect(root.type).toBe('group')
    if (root.type === 'group') {
      expect(root.bufferIds).toContain('buf-1')
      expect(root.activeBufferId).toBe('buf-1')
    }
  })

  it('removeBufferFromPane removes bufferId from the group', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')
    const root = store.getState().paneRoot
    if (root.type === 'group') {
      expect(root.bufferIds).not.toContain('buf-1')
      expect(root.bufferIds).toContain('buf-2')
    }
  })

  it('getAllPaneGroups returns all leaf groups', () => {
    const actions = store.getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const groups = actions.getAllPaneGroups()
    expect(groups).toHaveLength(2)
  })

  it('togglePaneFullscreen sets fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    expect(store.getState().fullscreenPaneId).toBe(ROOT_PANE_ID)
  })

  it('exitPaneFullscreen clears fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    store.getState().paneActions.exitPaneFullscreen()
    expect(store.getState().fullscreenPaneId).toBeNull()
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
```

Expected: `Cannot find module '@/features/workspace/stores/slices/pane-slice'`

- [ ] **Step 3: Implement the slice**

```typescript
// web/src/features/workspace/stores/slices/pane-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup, PaneNode, SplitDirection, SplitPlacement } from '@/features/panes/types/pane'
import {
  addBufferToPane,
  closePane,
  distributeFlattenedPaneSplit,
  findPaneGroup,
  findPaneGroupByBufferId,
  getAllPaneGroups,
  getAdjacentPane,
  moveBufferBetweenPanes,
  normalizePaneTree,
  removeBufferFromPane,
  reorderPaneBuffers,
  resizeFlattenedPaneSplit,
  setActivePaneBuffer,
  setPaneBufferPinned,
  setPaneLocked,
  setPanePreviewBuffer,
  splitPane as splitPaneUtil,
  updatePaneSizes,
} from '@/features/panes/utils/pane-tree'

export interface PaneActions {
  splitPane(paneId: string, direction: SplitDirection, bufferId?: string, placement?: SplitPlacement): string | null
  closePane(paneId: string): void
  setActivePane(paneId: string): void
  activatePaneBuffer(paneId: string, bufferId: string | null): void
  addBufferToPane(paneId: string, bufferId: string, setActive?: boolean): void
  removeBufferFromPane(paneId: string, bufferId: string, preserveEmptyPane?: boolean): void
  moveBufferToPane(bufferId: string, fromPaneId: string, toPaneId: string): void
  setPanePreviewBuffer(paneId: string, bufferId: string | null): void
  setPaneBufferPinned(paneId: string, bufferId: string, pinned: boolean): void
  reorderPaneBuffers(paneId: string, startIndex: number, endIndex: number): void
  resizePaneSplit(splitId: string, index: number, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByBufferId(bufferId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
}

export interface PaneSlice {
  paneRoot: PaneNode
  bottomRoot: PaneNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: PaneActions
}

function createInitialRoot(): PaneGroup {
  return { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function createInitialBottomRoot(): PaneGroup {
  return { id: BOTTOM_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

export const createPaneSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get) => ({
  paneRoot: createInitialRoot(),
  bottomRoot: createInitialBottomRoot(),
  activePaneId: ROOT_PANE_ID,
  mostRecentActivePaneIds: [ROOT_PANE_ID],
  fullscreenPaneId: null,

  paneActions: {
    splitPane(paneId, direction, bufferId?, placement = 'after') {
      let newPaneId: string | null = null
      set(state => {
        const oldIds = getAllPaneGroups(state.paneRoot).map(g => g.id)
        state.paneRoot = splitPaneUtil(state.paneRoot, paneId, direction, bufferId, placement)
        const newGroup = getAllPaneGroups(state.paneRoot).find(g => !oldIds.includes(g.id))
        newPaneId = newGroup?.id ?? null
        if (newPaneId) {
          state.activePaneId = newPaneId
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
        }
      })
      return newPaneId
    },

    closePane(paneId) {
      set(state => {
        const result = closePane(state.paneRoot, paneId)
        if (result !== null) {
          state.paneRoot = normalizePaneTree(result)
        }
        if (state.activePaneId === paneId) {
          const remaining = getAllPaneGroups(state.paneRoot)
          state.activePaneId = remaining[0]?.id ?? ROOT_PANE_ID
        }
        state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(id => id !== paneId)
        if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
      })
    },

    setActivePane(paneId) {
      set(state => {
        state.activePaneId = paneId
        state.mostRecentActivePaneIds = [paneId, ...state.mostRecentActivePaneIds.filter(id => id !== paneId)]
      })
    },

    activatePaneBuffer(paneId, bufferId) {
      set(state => {
        state.paneRoot = setActivePaneBuffer(state.paneRoot, paneId, bufferId)
        state.activePaneId = paneId
      })
    },

    addBufferToPane(paneId, bufferId, setActive = true) {
      set(state => {
        state.paneRoot = addBufferToPane(state.paneRoot, paneId, bufferId, setActive)
      })
    },

    removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
      set(state => {
        state.paneRoot = removeBufferFromPane(state.paneRoot, paneId, bufferId)
        if (!preserveEmptyPane) {
          const pane = findPaneGroup(state.paneRoot, paneId)
          if (pane && pane.bufferIds.length === 0 && paneId !== ROOT_PANE_ID) {
            const result = closePane(state.paneRoot, paneId)
            if (result !== null) state.paneRoot = normalizePaneTree(result)
          }
        }
      })
    },

    moveBufferToPane(bufferId, fromPaneId, toPaneId) {
      set(state => {
        state.paneRoot = moveBufferBetweenPanes(state.paneRoot, bufferId, fromPaneId, toPaneId)
      })
    },

    setPanePreviewBuffer(paneId, bufferId) {
      set(state => {
        state.paneRoot = setPanePreviewBuffer(state.paneRoot, paneId, bufferId)
      })
    },

    setPaneBufferPinned(paneId, bufferId, pinned) {
      set(state => {
        state.paneRoot = setPaneBufferPinned(state.paneRoot, paneId, bufferId, pinned)
      })
    },

    reorderPaneBuffers(paneId, startIndex, endIndex) {
      set(state => {
        state.paneRoot = reorderPaneBuffers(state.paneRoot, paneId, startIndex, endIndex)
      })
    },

    resizePaneSplit(splitId, index, sizes) {
      set(state => {
        state.paneRoot = resizeFlattenedPaneSplit(state.paneRoot, splitId, index, sizes)
      })
    },

    distributePaneSplit(splitId) {
      set(state => {
        state.paneRoot = distributeFlattenedPaneSplit(state.paneRoot, splitId)
      })
    },

    togglePaneFullscreen(paneId) {
      set(state => {
        state.fullscreenPaneId = state.fullscreenPaneId === paneId ? null : paneId
      })
    },

    exitPaneFullscreen() {
      set(state => { state.fullscreenPaneId = null })
    },

    getAllPaneGroups() { return getAllPaneGroups(get().paneRoot) },
    getPaneById(paneId) { return findPaneGroup(get().paneRoot, paneId) },
    getPaneByBufferId(bufferId) { return findPaneGroupByBufferId(get().paneRoot, bufferId) },
    getActivePane() { return findPaneGroup(get().paneRoot, get().activePaneId) },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
```

Expected: all 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/pane-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
git commit -m "feat(workspace): pane slice with TDD"
```

---

### Task 4: Buffer Slice

**Files:**
- Create: `web/src/features/workspace/stores/slices/buffer-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createBufferSlice, type BufferSlice } from '@/features/workspace/stores/slices/buffer-slice'

function makeStore() {
  return createStore<BufferSlice>()(immer((set, get) => createBufferSlice(set as any, get as any)))
}

describe('buffer-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts empty', () => {
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('openContent creates an editor buffer and returns its id', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/src/index.ts', name: 'index.ts', content: 'const x = 1',
    })
    expect(id).toBeTruthy()
    expect(store.getState().buffers).toHaveLength(1)
    const buf = store.getState().buffers[0]
    expect(buf.type).toBe('editor')
    expect(buf.path).toBe('/src/index.ts')
    expect(buf.id).toBe(id)
  })

  it('openContent with the same path returns the existing buffer id', () => {
    const spec = { type: 'editor' as const, path: '/src/index.ts', name: 'index.ts', content: '' }
    const id1 = store.getState().bufferActions.openContent(spec)
    const id2 = store.getState().bufferActions.openContent(spec)
    expect(id1).toBe(id2)
    expect(store.getState().buffers).toHaveLength(1)
  })

  it('openContent creates a crowbarChat buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'crowbarChat', wsId: 'ws-1', name: 'Chat',
    })
    expect(id).toBeTruthy()
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.type).toBe('crowbarChat')
  })

  it('closeBuffer removes it from the list', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })
    store.getState().bufferActions.closeBuffer(id)
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('preview flag is set when isPreview is true', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/b.ts', name: 'b.ts', content: '', isPreview: true,
    })
    const buf = store.getState().bufferActions.getBufferById(id)
    expect(buf?.isPreview).toBe(true)
  })

  it('pin toggles isPinned on the buffer', () => {
    const id = store.getState().bufferActions.openContent({
      type: 'editor', path: '/c.ts', name: 'c.ts', content: '',
    })
    store.getState().bufferActions.setPinned(id, true)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(true)
    store.getState().bufferActions.setPinned(id, false)
    expect(store.getState().bufferActions.getBufferById(id)?.isPinned).toBe(false)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
```

Expected: `Cannot find module '@/features/workspace/stores/slices/buffer-slice'`

- [ ] **Step 3: Implement the slice**

```typescript
// web/src/features/workspace/stores/slices/buffer-slice.ts
import { nanoid } from 'nanoid'
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  CrowbarChatContent,
  DiffContent,
  EditorContent,
  PaneContent,
} from '@/features/panes/types/pane-content'

// Only the content types Crowbar actively creates. All 20 Athas types remain in
// pane-content.ts for type-checking, but only these are opened via this slice.
export type OurOpenContentSpec =
  | { type: 'editor'; path: string; name: string; content: string; isPreview?: boolean; language?: string }
  | { type: 'crowbarChat'; wsId: string; name: string }
  | { type: 'diff'; path: string; name: string; content: string }

export interface BufferActions {
  openContent(spec: OurOpenContentSpec): string
  closeBuffer(bufferId: string): void
  setPinned(bufferId: string, pinned: boolean): void
  setPreview(bufferId: string): void
  promotePreview(bufferId: string): void
  getBufferById(bufferId: string): PaneContent | undefined
}

export interface BufferSlice {
  buffers: PaneContent[]
  bufferActions: BufferActions
}

export const createBufferSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BufferSlice
> = (set, get) => ({
  buffers: [],

  bufferActions: {
    openContent(spec) {
      // Deduplicate by path for file-backed content
      if (spec.type === 'editor') {
        const existing = get().buffers.find(b => b.type === 'editor' && b.path === spec.path)
        if (existing) return existing.id
      }
      if (spec.type === 'crowbarChat') {
        const existing = get().buffers.find(b => b.type === 'crowbarChat' && (b as CrowbarChatContent).wsId === spec.wsId)
        if (existing) return existing.id
      }

      const id = nanoid()
      const base = { id, isPinned: false, isActive: false, isPreview: spec.type === 'editor' ? (spec.isPreview ?? false) : false }

      let buffer: PaneContent
      if (spec.type === 'editor') {
        buffer = {
          ...base,
          type: 'editor',
          path: spec.path,
          name: spec.name,
          content: spec.content,
          savedContent: spec.content,
          isDirty: false,
          isVirtual: false,
          language: spec.language,
          tokens: [],
        } satisfies EditorContent
      } else if (spec.type === 'crowbarChat') {
        buffer = {
          ...base,
          type: 'crowbarChat',
          path: '',
          name: spec.name,
          wsId: spec.wsId,
        } satisfies CrowbarChatContent
      } else {
        buffer = {
          ...base,
          type: 'diff',
          path: spec.path,
          name: spec.name,
          content: spec.content,
          savedContent: spec.content,
        } satisfies DiffContent
      }

      set(state => { state.buffers.push(buffer) })
      return id
    },

    closeBuffer(bufferId) {
      set(state => { state.buffers = state.buffers.filter(b => b.id !== bufferId) })
    },

    setPinned(bufferId, pinned) {
      set(state => {
        const buf = state.buffers.find(b => b.id === bufferId)
        if (buf) buf.isPinned = pinned
      })
    },

    setPreview(bufferId) {
      set(state => {
        const buf = state.buffers.find(b => b.id === bufferId)
        if (buf) buf.isPreview = true
      })
    },

    promotePreview(bufferId) {
      set(state => {
        const buf = state.buffers.find(b => b.id === bufferId)
        if (buf) buf.isPreview = false
      })
    },

    getBufferById(bufferId) {
      return get().buffers.find(b => b.id === bufferId)
    },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
```

Expected: all 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
git commit -m "feat(workspace): buffer slice with TDD"
```

---

### Task 5: Workflow Slice

**Files:**
- Create: `web/src/features/workspace/stores/slices/workflow-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createWorkflowSlice, type WorkflowSlice } from '@/features/workspace/stores/slices/workflow-slice'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

const mockFlow: FlowDefinition = {
  flowId: 'flow-1',
  flowType: 'crowbar-default',
  steps: [
    { id: 'brainstorm', label: 'Brainstorm', contentType: 'chat', isCompleted: false, isActive: true },
    { id: 'spec', label: 'Spec', contentType: 'diff', isCompleted: false, isActive: false },
    { id: 'build', label: 'Build', contentType: 'split', isCompleted: false, isActive: false },
  ],
}

function makeStore() {
  return createStore<WorkflowSlice>()(immer((set, get) => createWorkflowSlice(set as any, get as any)))
}

describe('workflow-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts with no flow and no step', () => {
    expect(store.getState().flowDefinition).toBeNull()
    expect(store.getState().currentStepId).toBeNull()
  })

  it('setFlowDefinition stores the definition', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    expect(store.getState().flowDefinition?.flowId).toBe('flow-1')
    expect(store.getState().flowDefinition?.steps).toHaveLength(3)
  })

  it('setCurrentStep updates currentStepId', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    store.getState().workflowActions.setCurrentStep('spec')
    expect(store.getState().currentStepId).toBe('spec')
  })

  it('markStepCompleted sets isCompleted on the step', () => {
    store.getState().workflowActions.setFlowDefinition(mockFlow)
    store.getState().workflowActions.markStepCompleted('brainstorm')
    const step = store.getState().flowDefinition?.steps.find(s => s.id === 'brainstorm')
    expect(step?.isCompleted).toBe(true)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/slices/workflow-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

export interface WorkflowActions {
  setFlowDefinition(def: FlowDefinition): void
  setCurrentStep(stepId: string): void
  markStepCompleted(stepId: string): void
}

export interface WorkflowSlice {
  flowDefinition: FlowDefinition | null
  currentStepId: string | null
  workflowActions: WorkflowActions
}

export const createWorkflowSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  WorkflowSlice
> = (set) => ({
  flowDefinition: null,
  currentStepId: null,

  workflowActions: {
    setFlowDefinition(def) {
      set(state => {
        state.flowDefinition = def
        if (!state.currentStepId) {
          state.currentStepId = def.steps[0]?.id ?? null
        }
      })
    },
    setCurrentStep(stepId) {
      set(state => { state.currentStepId = stepId })
    },
    markStepCompleted(stepId) {
      set(state => {
        const step = state.flowDefinition?.steps.find(s => s.id === stepId)
        if (step) step.isCompleted = true
      })
    },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/workflow-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/workflow-slice.test.ts
git commit -m "feat(workspace): workflow slice with TDD"
```

---

### Task 6: LSP Slice

The LSP implementation lives in `features/editor/lsp/` (18 files, ~2,300 lines) — kept completely as-is. This slice makes LSP workspace-scoped by owning the workspace root and status.

**Files:**
- Create: `web/src/features/workspace/stores/slices/lsp-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createLspSlice, type LspSlice } from '@/features/workspace/stores/slices/lsp-slice'

function makeStore() {
  return createStore<LspSlice>()(immer((set, get) => createLspSlice(set as any, get as any)))
}

describe('lsp-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts with empty workspace root and idle status', () => {
    expect(store.getState().workspaceRoot).toBe('')
    expect(store.getState().lspStatus.status).toBe('idle')
  })

  it('setWorkspaceRoot updates root', () => {
    store.getState().lspActions.setWorkspaceRoot('/home/user/project')
    expect(store.getState().workspaceRoot).toBe('/home/user/project')
  })

  it('updateLspStatus updates status fields', () => {
    store.getState().lspActions.updateLspStatus({ status: 'running', supportedLanguages: ['typescript'] })
    expect(store.getState().lspStatus.status).toBe('running')
    expect(store.getState().lspStatus.supportedLanguages).toEqual(['typescript'])
  })

  it('setCompletionHandlers stores handlers', () => {
    const isSupported = () => true
    store.getState().lspActions.setCompletionHandlers({ isLanguageSupported: isSupported })
    expect(store.getState().isLanguageSupported).toBe(isSupported)
  })

  it('clearCompletionCache empties the cache', () => {
    store.getState().lspActions.updateCompletionCache('key-1', [{ label: 'foo' } as any])
    store.getState().lspActions.clearCompletionCache()
    expect(Object.keys(store.getState().completionCache)).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/slices/lsp-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { CompletionItem } from 'vscode-languageserver-types'

export interface LspStatusInfo {
  status: 'idle' | 'starting' | 'running' | 'error'
  activeWorkspaces?: string[]
  lastError?: string
  supportedLanguages?: string[]
}

export type CompletionCache = Record<string, CompletionItem[]>

export interface CompletionHandlers {
  getCompletions?: (filePath: string, line: number, character: number) => Promise<CompletionItem[]>
  isLanguageSupported?: (filePath: string) => boolean
}

export interface LspActions {
  setWorkspaceRoot(root: string): void
  setCompletionHandlers(handlers: CompletionHandlers): void
  updateLspStatus(info: Partial<LspStatusInfo>): void
  updateCompletionCache(key: string, items: CompletionItem[]): void
  clearCompletionCache(): void
}

export interface LspSlice {
  workspaceRoot: string
  lspStatus: LspStatusInfo
  completionCache: CompletionCache
  currentCompletionRequest: AbortController | null
  getCompletions?: (filePath: string, line: number, character: number) => Promise<CompletionItem[]>
  isLanguageSupported?: (filePath: string) => boolean
  lspActions: LspActions
}

export const createLspSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  LspSlice
> = (set) => ({
  workspaceRoot: '',
  lspStatus: { status: 'idle' },
  completionCache: {},
  currentCompletionRequest: null,
  getCompletions: undefined,
  isLanguageSupported: undefined,

  lspActions: {
    setWorkspaceRoot(root) {
      set(state => { state.workspaceRoot = root })
    },
    setCompletionHandlers({ getCompletions, isLanguageSupported }) {
      set(state => {
        if (getCompletions) state.getCompletions = getCompletions
        if (isLanguageSupported) state.isLanguageSupported = isLanguageSupported
      })
    },
    updateLspStatus(info) {
      set(state => { Object.assign(state.lspStatus, info) })
    },
    updateCompletionCache(key, items) {
      set(state => { state.completionCache[key] = items })
    },
    clearCompletionCache() {
      set(state => { state.completionCache = {} })
    },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/lsp-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/lsp-slice.test.ts
git commit -m "feat(workspace): LSP slice with TDD"
```

---

### Task 7: Terminal Slice

The 32-file terminal implementation (`features/terminal/`) stays intact. This slice adds workspace ownership — tracking which session IDs belong to this workspace.

**Files:**
- Create: `web/src/features/workspace/stores/slices/terminal-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createTerminalSlice, type TerminalSlice } from '@/features/workspace/stores/slices/terminal-slice'

function makeStore() {
  return createStore<TerminalSlice>()(immer((set, get) => createTerminalSlice(set as any, get as any)))
}

describe('terminal-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts with no sessions and default layout', () => {
    expect(store.getState().terminalSessionIds.size).toBe(0)
    expect(store.getState().terminalLayout.widthMode).toBe('full')
    expect(store.getState().terminalLayout.tabLayout).toBe('horizontal')
  })

  it('registerSession adds the session ID', () => {
    store.getState().terminalActions.registerSession('sess-1')
    expect(store.getState().terminalActions.hasSession('sess-1')).toBe(true)
  })

  it('unregisterSession removes the session ID', () => {
    store.getState().terminalActions.registerSession('sess-1')
    store.getState().terminalActions.unregisterSession('sess-1')
    expect(store.getState().terminalActions.hasSession('sess-1')).toBe(false)
  })

  it('hasSession returns false for unknown sessions', () => {
    expect(store.getState().terminalActions.hasSession('not-here')).toBe(false)
  })

  it('setWidthMode updates the layout', () => {
    store.getState().terminalActions.setWidthMode('editor')
    expect(store.getState().terminalLayout.widthMode).toBe('editor')
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/slices/terminal-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

export interface TerminalLayout {
  widthMode: 'full' | 'editor'
  tabLayout: 'horizontal' | 'vertical'
  tabSidebarWidth: number
  tabSidebarPosition: 'left' | 'right'
}

export interface TerminalActions {
  registerSession(sessionId: string): void
  unregisterSession(sessionId: string): void
  hasSession(sessionId: string): boolean
  setWidthMode(mode: 'full' | 'editor'): void
  setTabLayout(layout: 'horizontal' | 'vertical'): void
  setTabSidebarWidth(width: number): void
  setTabSidebarPosition(pos: 'left' | 'right'): void
}

export interface TerminalSlice {
  terminalSessionIds: Set<string>
  terminalLayout: TerminalLayout
  terminalActions: TerminalActions
}

export const createTerminalSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  TerminalSlice
> = (set, get) => ({
  terminalSessionIds: new Set(),
  terminalLayout: {
    widthMode: 'full',
    tabLayout: 'horizontal',
    tabSidebarWidth: 200,
    tabSidebarPosition: 'left',
  },

  terminalActions: {
    registerSession(sessionId) {
      set(state => { state.terminalSessionIds.add(sessionId) })
    },
    unregisterSession(sessionId) {
      set(state => { state.terminalSessionIds.delete(sessionId) })
    },
    hasSession(sessionId) {
      return get().terminalSessionIds.has(sessionId)
    },
    setWidthMode(mode) {
      set(state => { state.terminalLayout.widthMode = mode })
    },
    setTabLayout(layout) {
      set(state => { state.terminalLayout.tabLayout = layout })
    },
    setTabSidebarWidth(width) {
      set(state => { state.terminalLayout.tabSidebarWidth = width })
    },
    setTabSidebarPosition(pos) {
      set(state => { state.terminalLayout.tabSidebarPosition = pos })
    },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/terminal-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/terminal-slice.test.ts
git commit -m "feat(workspace): terminal slice with TDD"
```

---

### Task 8: File Watcher Slice

Currently a 23-line stub. This is the real implementation.

**Files:**
- Create: `web/src/features/workspace/stores/slices/file-watcher-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createFileWatcherSlice, type FileWatcherSlice } from '@/features/workspace/stores/slices/file-watcher-slice'

function makeStore() {
  return createStore<FileWatcherSlice>()(immer((set, get) => createFileWatcherSlice(set as any, get as any)))
}

describe('file-watcher-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts with no watched paths and no pending saves', () => {
    expect(store.getState().fileWatcherActions.getWatchedPaths()).toHaveLength(0)
    expect(store.getState().fileWatcherActions.isPendingSave('/a.ts')).toBe(false)
  })

  it('watchPath adds a path', () => {
    store.getState().fileWatcherActions.watchPath('/src/index.ts')
    expect(store.getState().fileWatcherActions.getWatchedPaths()).toContain('/src/index.ts')
  })

  it('unwatchPath removes a path', () => {
    store.getState().fileWatcherActions.watchPath('/src/index.ts')
    store.getState().fileWatcherActions.unwatchPath('/src/index.ts')
    expect(store.getState().fileWatcherActions.getWatchedPaths()).not.toContain('/src/index.ts')
  })

  it('markPendingSave / clearPendingSave roundtrip', () => {
    store.getState().fileWatcherActions.markPendingSave('/src/index.ts')
    expect(store.getState().fileWatcherActions.isPendingSave('/src/index.ts')).toBe(true)
    store.getState().fileWatcherActions.clearPendingSave('/src/index.ts')
    expect(store.getState().fileWatcherActions.isPendingSave('/src/index.ts')).toBe(false)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/slices/file-watcher-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

export interface FileWatcherActions {
  watchPath(path: string): void
  unwatchPath(path: string): void
  markPendingSave(path: string): void
  clearPendingSave(path: string): void
  isPendingSave(path: string): boolean
  getWatchedPaths(): string[]
}

export interface FileWatcherSlice {
  watchedPaths: Set<string>
  pendingSaves: Set<string>
  fileWatcherActions: FileWatcherActions
}

export const createFileWatcherSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  FileWatcherSlice
> = (set, get) => ({
  watchedPaths: new Set(),
  pendingSaves: new Set(),

  fileWatcherActions: {
    watchPath(path) { set(state => { state.watchedPaths.add(path) }) },
    unwatchPath(path) { set(state => { state.watchedPaths.delete(path) }) },
    markPendingSave(path) { set(state => { state.pendingSaves.add(path) }) },
    clearPendingSave(path) { set(state => { state.pendingSaves.delete(path) }) },
    isPendingSave(path) { return get().pendingSaves.has(path) },
    getWatchedPaths() { return Array.from(get().watchedPaths) },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/file-watcher-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/file-watcher-slice.test.ts
git commit -m "feat(workspace): file watcher slice with TDD"
```

---

### Task 9: Recent Files Slice

Currently a 7-line no-op. Real implementation — max 50 entries, most recent first.

**Files:**
- Create: `web/src/features/workspace/stores/slices/recent-files-slice.ts`
- Test: `web/src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createRecentFilesSlice, type RecentFilesSlice } from '@/features/workspace/stores/slices/recent-files-slice'

function makeStore() {
  return createStore<RecentFilesSlice>()(immer((set, get) => createRecentFilesSlice(set as any, get as any)))
}

describe('recent-files-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => { store = makeStore() })

  it('starts with no recent files', () => {
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })

  it('addRecentFile adds an entry', () => {
    store.getState().recentFilesActions.addRecentFile('/src/index.ts', 'index.ts')
    const files = store.getState().recentFilesActions.getRecentFiles()
    expect(files).toHaveLength(1)
    expect(files[0].path).toBe('/src/index.ts')
    expect(files[0].name).toBe('index.ts')
  })

  it('addRecentFile moves existing entry to front', () => {
    const actions = store.getState().recentFilesActions
    actions.addRecentFile('/a.ts', 'a.ts')
    actions.addRecentFile('/b.ts', 'b.ts')
    actions.addRecentFile('/a.ts', 'a.ts')
    const files = actions.getRecentFiles()
    expect(files).toHaveLength(2)
    expect(files[0].path).toBe('/a.ts')
  })

  it('caps at 50 entries', () => {
    const actions = store.getState().recentFilesActions
    for (let i = 0; i < 60; i++) {
      actions.addRecentFile(`/file${i}.ts`, `file${i}.ts`)
    }
    expect(actions.getRecentFiles()).toHaveLength(50)
  })

  it('removeRecentFile removes the entry', () => {
    store.getState().recentFilesActions.addRecentFile('/a.ts', 'a.ts')
    store.getState().recentFilesActions.removeRecentFile('/a.ts')
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })

  it('clearRecentFiles empties the list', () => {
    store.getState().recentFilesActions.addRecentFile('/a.ts', 'a.ts')
    store.getState().recentFilesActions.clearRecentFiles()
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/slices/recent-files-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

const MAX_RECENT_FILES = 50

export interface RecentFileEntry {
  path: string
  name: string
  lastOpenedAt: number
}

export interface RecentFilesActions {
  addRecentFile(path: string, name: string): void
  removeRecentFile(path: string): void
  clearRecentFiles(): void
  getRecentFiles(): RecentFileEntry[]
}

export interface RecentFilesSlice {
  recentFiles: RecentFileEntry[]
  recentFilesActions: RecentFilesActions
}

export const createRecentFilesSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  RecentFilesSlice
> = (set, get) => ({
  recentFiles: [],

  recentFilesActions: {
    addRecentFile(path, name) {
      set(state => {
        state.recentFiles = state.recentFiles.filter(f => f.path !== path)
        state.recentFiles.unshift({ path, name, lastOpenedAt: Date.now() })
        if (state.recentFiles.length > MAX_RECENT_FILES) {
          state.recentFiles = state.recentFiles.slice(0, MAX_RECENT_FILES)
        }
      })
    },
    removeRecentFile(path) {
      set(state => { state.recentFiles = state.recentFiles.filter(f => f.path !== path) })
    },
    clearRecentFiles() {
      set(state => { state.recentFiles = [] })
    },
    getRecentFiles() { return get().recentFiles },
  },
})
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/recent-files-slice.ts \
        web/src/__tests__/features/workspace/stores/slices/recent-files-slice.test.ts
git commit -m "feat(workspace): recent files slice with TDD"
```

---

## Phase 3 — Store Assembly

### Task 10: WorkspaceStore Factory

**Files:**
- Create: `web/src/features/workspace/stores/workspace-store.ts`

- [ ] **Step 1: Create the factory**

```typescript
// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createWorkflowSlice } from './slices/workflow-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'paneRoot' | 'bottomRoot' | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'currentStepId'
    | 'recentFiles'
    | 'terminalLayout'
  >
>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  return createStore<WorkspaceState>()(
    immer((set, get) => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get),
      ...createBufferSlice(set, get),
      ...createWorkflowSlice(set, get),
      ...createLspSlice(set, get),
      ...createTerminalSlice(set, get),
      ...createFileWatcherSlice(set, get),
      ...createRecentFilesSlice(set, get),
      // Apply persisted snapshot over defaults (snapshot wins for serialisable fields)
      ...(snapshot ?? {}),
    }))
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles with no errors**

```bash
cd web && npx tsc --noEmit 2>&1 | grep workspace-store
```

Expected: no output (no errors).

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/stores/workspace-store.ts
git commit -m "feat(workspace): workspace store factory"
```

---

### Task 11: WorkspaceStoreRegistry

**Files:**
- Create: `web/src/features/workspace/stores/workspace-store-registry.ts`
- Test: `web/src/__tests__/features/workspace/stores/workspace-store-registry.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/workspace-store-registry.test.ts
import { describe, it, expect, afterEach } from 'vitest'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'

afterEach(() => {
  // Clean up so tests don't bleed into each other
  getAllActiveWorkspaceIds().forEach(id => destroyWorkspaceStore(id))
})

describe('workspace-store-registry', () => {
  it('getOrCreate returns the same instance for the same wsId', () => {
    const a = getOrCreateWorkspaceStore('ws-a')
    const b = getOrCreateWorkspaceStore('ws-a')
    expect(a).toBe(b)
  })

  it('getOrCreate returns different instances for different wsIds', () => {
    const a = getOrCreateWorkspaceStore('ws-x')
    const b = getOrCreateWorkspaceStore('ws-y')
    expect(a).not.toBe(b)
  })

  it('destroyWorkspaceStore removes the instance', () => {
    const first = getOrCreateWorkspaceStore('ws-z')
    destroyWorkspaceStore('ws-z')
    const second = getOrCreateWorkspaceStore('ws-z')
    expect(first).not.toBe(second)
  })

  it('getAllActiveWorkspaceIds returns ids of live stores', () => {
    getOrCreateWorkspaceStore('ws-1')
    getOrCreateWorkspaceStore('ws-2')
    const ids = getAllActiveWorkspaceIds()
    expect(ids).toContain('ws-1')
    expect(ids).toContain('ws-2')
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-store-registry.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/workspace-store-registry.ts
import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'

const registry = new Map<string, WorkspaceStore>()

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    const snapshot = loadFromLocalStorage(wsId)
    registry.set(wsId, createWorkspaceStore(wsId, snapshot ?? undefined))
  }
  return registry.get(wsId)!
}

export function destroyWorkspaceStore(wsId: string): void {
  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-store-registry.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/workspace-store-registry.ts \
        web/src/__tests__/features/workspace/stores/workspace-store-registry.test.ts
git commit -m "feat(workspace): store registry with TDD"
```

---

### Task 12: WorkspacePersistence

**Files:**
- Create: `web/src/features/workspace/stores/workspace-persistence.ts`
- Test: `web/src/__tests__/features/workspace/stores/workspace-persistence.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// web/src/__tests__/features/workspace/stores/workspace-persistence.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { saveToLocalStorage, loadFromLocalStorage } from '@/features/workspace/stores/workspace-persistence'

describe('workspace-persistence', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('saveToLocalStorage writes a JSON string to localStorage', () => {
    saveToLocalStorage('ws-1', { currentStepId: 'brainstorm' })
    const raw = localStorage.getItem('workspace:ws-1:state')
    expect(raw).not.toBeNull()
    expect(JSON.parse(raw!).currentStepId).toBe('brainstorm')
  })

  it('loadFromLocalStorage returns the saved snapshot', () => {
    saveToLocalStorage('ws-2', { currentStepId: 'spec', recentFiles: [] })
    const snap = loadFromLocalStorage('ws-2')
    expect(snap?.currentStepId).toBe('spec')
    expect(snap?.recentFiles).toEqual([])
  })

  it('loadFromLocalStorage returns null for unknown wsId', () => {
    expect(loadFromLocalStorage('ws-unknown')).toBeNull()
  })

  it('loadFromLocalStorage returns null for corrupt data', () => {
    localStorage.setItem('workspace:ws-corrupt:state', '{{not json}}')
    expect(loadFromLocalStorage('ws-corrupt')).toBeNull()
  })
})
```

- [ ] **Step 2: Run tests — expect failure**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-persistence.test.ts
```

- [ ] **Step 3: Implement**

```typescript
// web/src/features/workspace/stores/workspace-persistence.ts
import type { WorkspaceSnapshot } from './workspace-store'

function storageKey(wsId: string): string {
  return `workspace:${wsId}:state`
}

export function saveToLocalStorage(wsId: string, snapshot: WorkspaceSnapshot): void {
  try {
    // Sets with non-JSON-serialisable values (Set, Map) are converted
    const serialisable = {
      ...snapshot,
      // terminalSessionIds is a Set — omit from persistence (sessions reconnect on reload)
    }
    localStorage.setItem(storageKey(wsId), JSON.stringify(serialisable))
  } catch {
    // localStorage full or unavailable — silently skip
  }
}

export function loadFromLocalStorage(wsId: string): WorkspaceSnapshot | null {
  try {
    const raw = localStorage.getItem(storageKey(wsId))
    if (!raw) return null
    return JSON.parse(raw) as WorkspaceSnapshot
  } catch {
    return null
  }
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/workspace/stores/workspace-persistence.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/workspace-persistence.ts \
        web/src/__tests__/features/workspace/stores/workspace-persistence.test.ts
git commit -m "feat(workspace): localStorage persistence with TDD"
```

---

### Task 13: WorkspaceStoreContext + Provider

**Files:**
- Create: `web/src/features/workspace/stores/workspace-context.ts`

- [ ] **Step 1: Create the context file**

```typescript
// web/src/features/workspace/stores/workspace-context.ts
import { createContext, useContext, type ReactNode } from 'react'
import { useStore } from 'zustand'
import type { WorkspaceStore } from './workspace-store'
import type { WorkspaceState } from './workspace-store.types'

export const WorkspaceStoreContext = createContext<WorkspaceStore | null>(null)

/**
 * Returns a value from the active workspace store using a selector.
 * Must be called inside a component tree wrapped in WorkspaceStoreProvider.
 */
export function useWorkspaceStoreContext<T>(selector: (state: WorkspaceState) => T): T {
  const store = useContext(WorkspaceStoreContext)
  if (!store) throw new Error('useWorkspaceStoreContext must be used inside WorkspaceStoreProvider')
  return useStore(store, selector)
}

/**
 * Returns the raw WorkspaceStore instance (for imperative access outside React).
 */
export function useWorkspaceStore(): WorkspaceStore {
  const store = useContext(WorkspaceStoreContext)
  if (!store) throw new Error('useWorkspaceStore must be used inside WorkspaceStoreProvider')
  return store
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | grep workspace-context
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/stores/workspace-context.ts
git commit -m "feat(workspace): store context and provider hook"
```

---

## Phase 4 — New Hooks

These replace the global `usePaneStore` and `useBufferStore`. They are the single honest source of truth for all workspace-scoped state.

### Task 14: use-pane-store.ts

**Files:**
- Create: `web/src/features/workspace/stores/hooks/use-pane-store.ts`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workspace/stores/hooks/use-pane-store.ts
import { useWorkspaceStoreContext } from '../workspace-context'
import type { PaneActions } from '../slices/pane-slice'
import type { PaneGroup, PaneNode } from '@/features/panes/types/pane'

export const usePaneRoot = (): PaneNode =>
  useWorkspaceStoreContext(s => s.paneRoot)

export const useBottomRoot = (): PaneNode =>
  useWorkspaceStoreContext(s => s.bottomRoot)

export const useFullscreenPaneId = (): string | null =>
  useWorkspaceStoreContext(s => s.fullscreenPaneId)

export const useActivePaneId = (): string =>
  useWorkspaceStoreContext(s => s.activePaneId)

export const useMostRecentActivePaneIds = (): string[] =>
  useWorkspaceStoreContext(s => s.mostRecentActivePaneIds)

export const usePaneActions = (): PaneActions =>
  useWorkspaceStoreContext(s => s.paneActions)

export const useAllPaneGroups = (): PaneGroup[] =>
  useWorkspaceStoreContext(s => s.paneActions.getAllPaneGroups())
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-pane-store.ts
git commit -m "feat(workspace): pane store hooks"
```

---

### Task 15: use-buffer-store.ts

**Files:**
- Create: `web/src/features/workspace/stores/hooks/use-buffer-store.ts`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workspace/stores/hooks/use-buffer-store.ts
import { useWorkspaceStoreContext } from '../workspace-context'
import type { BufferActions } from '../slices/buffer-slice'
import type { PaneContent } from '@/features/panes/types/pane-content'

export const useBuffers = (): PaneContent[] =>
  useWorkspaceStoreContext(s => s.buffers)

export const useBufferActions = (): BufferActions =>
  useWorkspaceStoreContext(s => s.bufferActions)

export const useBufferById = (id: string): PaneContent | undefined =>
  useWorkspaceStoreContext(s => s.buffers.find(b => b.id === id))
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-buffer-store.ts
git commit -m "feat(workspace): buffer store hooks"
```

---

### Task 16: use-workflow.ts

**Files:**
- Create: `web/src/features/workspace/stores/hooks/use-workflow.ts`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workspace/stores/hooks/use-workflow.ts
import { useWorkspaceStoreContext } from '../workspace-context'
import type { WorkflowActions } from '../slices/workflow-slice'
import type { FlowDefinition, FlowStep } from '@/features/workflow/types/workflow'

export const useFlowDefinition = (): FlowDefinition | null =>
  useWorkspaceStoreContext(s => s.flowDefinition)

export const useCurrentStepId = (): string | null =>
  useWorkspaceStoreContext(s => s.currentStepId)

export const useCurrentStep = (): FlowStep | null =>
  useWorkspaceStoreContext(s => {
    if (!s.flowDefinition || !s.currentStepId) return null
    return s.flowDefinition.steps.find(step => step.id === s.currentStepId) ?? null
  })

export const useWorkflowActions = (): WorkflowActions =>
  useWorkspaceStoreContext(s => s.workflowActions)
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-workflow.ts
git commit -m "feat(workspace): workflow hooks"
```

---

## Phase 5 — Component Migration

**Strategy:** Update all Athas component imports in one go. The old global stores remain on disk until all components compile correctly, then delete them. This prevents a partial-broken state.

### Task 17: Update split-view-root.tsx

**Files:**
- Modify: `web/src/features/panes/components/split-view-root.tsx`

- [ ] **Step 1: Update imports and hook calls**

Replace the entire file content:

```typescript
// web/src/features/panes/components/split-view-root.tsx
import { useEffect, useMemo } from 'react'
import { IS_MAC } from '@/utils/platform'
import {
  usePaneRoot,
  useBottomRoot,
  useFullscreenPaneId,
  usePaneActions,
} from '@/features/workspace/stores/hooks/use-pane-store'
import { PaneContainer } from './pane-container'
import { PaneNodeRenderer } from './pane-node-renderer'

export function SplitViewRoot() {
  const root = usePaneRoot()
  const bottomRoot = useBottomRoot()
  const fullscreenPaneId = useFullscreenPaneId()
  const { exitPaneFullscreen, getAllPaneGroups } = usePaneActions()

  const fullscreenPane = useMemo(
    () => fullscreenPaneId
      ? (getAllPaneGroups().find(pane => pane.id === fullscreenPaneId) ?? null)
      : null,
    [fullscreenPaneId, getAllPaneGroups, root, bottomRoot],
  )

  useEffect(() => {
    if (fullscreenPaneId && !fullscreenPane) exitPaneFullscreen()
  }, [exitPaneFullscreen, fullscreenPane, fullscreenPaneId])

  const titleBarHeight = IS_MAC ? 44 : 28
  const footerHeight = 32

  return (
    <>
      <div className="h-full w-full overflow-hidden">
        <PaneNodeRenderer node={root} hiddenPaneId={fullscreenPaneId} />
      </div>

      {fullscreenPane && (
        <div
          className="fixed inset-x-2 z-[10040]"
          style={{ top: `${titleBarHeight + 8}px`, bottom: `${footerHeight + 8}px` }}
        >
          <div className="h-full overflow-hidden rounded-xl border border-border/80 bg-primary-bg shadow-2xl">
            <PaneContainer pane={fullscreenPane} />
          </div>
        </div>
      )}
    </>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep split-view-root
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/panes/components/split-view-root.tsx
git commit -m "feat(workspace): migrate SplitViewRoot to workspace hooks"
```

---

### Task 18: Update pane-node-renderer.tsx

**Files:**
- Modify: `web/src/features/panes/components/pane-node-renderer.tsx`

- [ ] **Step 1: Update import**

In `pane-node-renderer.tsx`, replace:
```typescript
import { usePaneStore } from "../stores/pane-store";
```
With:
```typescript
import { usePaneActions } from "@/features/workspace/stores/hooks/use-pane-store";
```

Then replace all usages of `usePaneStore.use.actions()` with `usePaneActions()`.

The file calls `usePaneStore.use.actions()` to get `distributePaneSplit` and `resizePaneSplit`. After the change:

```typescript
// Before:
const { distributePaneSplit, resizePaneSplit } = usePaneStore.use.actions();
// After:
const { distributePaneSplit, resizePaneSplit } = usePaneActions();
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep pane-node-renderer
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/panes/components/pane-node-renderer.tsx
git commit -m "feat(workspace): migrate PaneNodeRenderer to workspace hooks"
```

---

### Task 19: Update pane-container.tsx

`pane-container.tsx` is the largest file (~1,138 lines). It imports from both `usePaneStore` and `useBufferStore`. The change is imports and hook call sites only — no logic changes.

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx`

- [ ] **Step 1: Replace store imports at the top of the file**

Find all imports from the old stores:
```typescript
import { usePaneStore } from "../stores/pane-store"   // or similar path
import { useBufferStore } from "@/features/editor/stores/buffer-store"
```

Replace with:
```typescript
import {
  usePaneRoot,
  usePaneActions,
  useActivePaneId,
  useFullscreenPaneId,
} from "@/features/workspace/stores/hooks/use-pane-store"
import {
  useBuffers,
  useBufferActions,
  useBufferById,
} from "@/features/workspace/stores/hooks/use-buffer-store"
```

- [ ] **Step 2: Replace all hook call sites**

Search for every `usePaneStore.use.*()` call in the file and replace with the matching named hook:

| Old call | New call |
|---|---|
| `usePaneStore.use.root()` | `usePaneRoot()` |
| `usePaneStore.use.activePaneId()` | `useActivePaneId()` |
| `usePaneStore.use.fullscreenPaneId()` | `useFullscreenPaneId()` |
| `usePaneStore.use.actions()` | `usePaneActions()` |
| `useBufferStore.use.buffers()` | `useBuffers()` |
| `useBufferStore.use.actions()` | `useBufferActions()` |

Search the file for all remaining `usePaneStore` and `useBufferStore` references:
```bash
grep -n "usePaneStore\|useBufferStore" web/src/features/panes/components/pane-container.tsx
```

There should be zero remaining after the replacement.

- [ ] **Step 3: Find and update crowbarChat buffer rendering**

`pane-container.tsx` renders the content for each buffer type. Find the section that renders `crowbarChat` — it currently renders `<FlowTab>` or uses `<Outlet />`. Update it to render `<FlowContent wsId={buffer.wsId} />`:

```typescript
// Find the crowbarChat case (may be in a switch or if-chain)
// Before:
case 'crowbarChat':
  return <FlowTab workspaceId={buffer.wsId} />

// After:
import { FlowContent } from '@/features/workflow/components/FlowContent'
// ...
case 'crowbarChat':
  return <FlowContent workspaceId={(buffer as CrowbarChatContent).wsId} />
```

`FlowContent` is created in Task 24. For now, create a placeholder so the file compiles:
```typescript
// Temporary stub — replaced in Task 24
// web/src/features/workflow/components/FlowContent.tsx
export function FlowContent({ workspaceId }: { workspaceId: string }) {
  return <div className="flex h-full items-center justify-center text-muted-foreground">Loading workspace {workspaceId}…</div>
}
```

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep pane-container
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/panes/components/pane-container.tsx \
        web/src/features/workflow/components/FlowContent.tsx
git commit -m "feat(workspace): migrate PaneContainer to workspace hooks"
```

---

### Task 20: Update tab-bar.tsx and tab-bar-item.tsx

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx`

- [ ] **Step 1: Update tab-bar.tsx imports**

Replace `usePaneStore` and `useBufferStore` imports:
```typescript
import { usePaneActions, useActivePaneId } from "@/features/workspace/stores/hooks/use-pane-store"
import { useBuffers, useBufferActions } from "@/features/workspace/stores/hooks/use-buffer-store"
```

Replace all `.use.*()` call sites with named hooks (same mapping table as Task 19).

- [ ] **Step 2: Update tab-bar-item.tsx imports**

Apply the same pattern. Run:
```bash
grep -n "usePaneStore\|useBufferStore" \
  web/src/features/tabs/components/tab-bar.tsx \
  web/src/features/tabs/components/tab-bar-item.tsx
```

Expected: zero matches.

- [ ] **Step 3: Update LSP hooks**

`use-lsp-integration.ts` and `use-lsp-initialization.ts` in `features/athas-editor/hooks/` import from the global `lsp-store`. Update them to import from the workspace LSP slice:

```typescript
// Before:
import { useLspStore } from '@/features/editor/lsp/lsp-store'
// After:
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

// Replace useLspStore.use.* calls with:
const lspActions = useWorkspaceStoreContext(s => s.lspActions)
const lspStatus = useWorkspaceStoreContext(s => s.lspStatus)
const getCompletions = useWorkspaceStoreContext(s => s.getCompletions)
const isLanguageSupported = useWorkspaceStoreContext(s => s.isLanguageSupported)
```

- [ ] **Step 4: Type-check all modified files**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -E "(tab-bar|lsp-integration|lsp-initialization)"
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx \
        web/src/features/tabs/components/tab-bar-item.tsx \
        web/src/features/athas-editor/hooks/use-lsp-integration.ts \
        web/src/features/athas-editor/hooks/use-lsp-initialization.ts
git commit -m "feat(workspace): migrate TabBar and LSP hooks to workspace context"
```

---

### Task 21: Delete Old Global Stores

The old stores are now unused. Delete them and confirm no remaining references.

**Files:**
- Delete: `web/src/features/panes/stores/pane-store.ts`
- Delete: `web/src/features/editor/stores/buffer-store.ts`
- Delete: `web/src/components/layout/FlowTab.tsx`

- [ ] **Step 1: Check for remaining references**

```bash
grep -rn "from.*pane-store\|from.*buffer-store\|from.*FlowTab" \
  web/src/features web/src/components web/src/lib \
  --include="*.ts" --include="*.tsx"
```

Expected: zero matches (if any found, fix them before deleting).

- [ ] **Step 2: Delete the files**

```bash
rm web/src/features/panes/stores/pane-store.ts
rm web/src/features/editor/stores/buffer-store.ts
rm web/src/components/layout/FlowTab.tsx
```

- [ ] **Step 3: Type-check clean**

```bash
cd web && npx tsc --noEmit 2>&1
```

Expected: zero errors.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(workspace): delete old global stores and FlowTab"
```

---

## Phase 6 — New Workspace Components

### Task 22: WorkspaceView

The entry point for a workspace. Retrieves (or creates) the workspace store and provides it via context.

**Files:**
- Create: `web/src/features/workspace/components/WorkspaceView.tsx`

- [ ] **Step 1: Create the file**

```typescript
// web/src/features/workspace/components/WorkspaceView.tsx
import { useEffect } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore } from '../stores/workspace-store-registry'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

interface WorkspaceViewProps {
  wsId: string
  /** Human-readable workspace label, e.g. "my-repo / main" */
  label?: string
}

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceViewInner wsId={wsId} label={label} />
    </WorkspaceStoreContext.Provider>
  )
}

/** Inner component — has access to WorkspaceStoreContext */
function WorkspaceViewInner({ wsId, label }: WorkspaceViewProps) {
  useWorkspaceEffects(wsId, label)
  return <WorkspaceLayoutRoot />
}
```

- [ ] **Step 2: Create use-workspace-effects.ts**

```typescript
// web/src/features/workspace/stores/hooks/use-workspace-effects.ts
import { useEffect } from 'react'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import type { AppFile } from '@/features/file-system/types/app'

/**
 * Seeds the file system store and opens the crowbarChat buffer whenever
 * the active workspace changes. Replaces the useEffect calls in IDEShell.
 */
export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const repoPath = `/repos/${wsId}`   // placeholder until real backend

  // Seed file system
  useEffect(() => {
    const files = getMockFileTree(repoPath) as AppFile[]
    useFileSystemStore.setState({
      rootFolderPath: repoPath,
      files,
      handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
        if (revealOrIsDir === true) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        bufferActions.openContent({ type: 'editor', path, name, content })
      },
      handleFileSelect: (path: string, isDir?: boolean) => {
        if (isDir) return
        const name = path.split('/').pop() ?? path
        const content = getMockFileContent(path)
        bufferActions.openContent({ type: 'editor', path, name, content, isPreview: true })
      },
    })
  }, [repoPath])

  // Open the crowbarChat buffer for this workspace
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/components/WorkspaceView.tsx \
        web/src/features/workspace/stores/hooks/use-workspace-effects.ts
git commit -m "feat(workspace): WorkspaceView + workspace effects hook"
```

---

### Task 23: WorkspaceLayoutRoot + WorkspaceStepFooter

**Files:**
- Create: `web/src/features/workspace/components/WorkspaceLayoutRoot.tsx`
- Create: `web/src/features/workspace/components/WorkspaceStepFooter.tsx`

- [ ] **Step 1: Create WorkspaceLayoutRoot**

```typescript
// web/src/features/workspace/components/WorkspaceLayoutRoot.tsx
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { WorkspaceStepFooter } from './WorkspaceStepFooter'

/**
 * Fills the main content area of a workspace.
 * The pane area takes all available height; the step footer is always visible below it.
 */
export function WorkspaceLayoutRoot() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Pane area — flex-1 so it fills everything above the footer */}
      <div className="min-h-0 flex-1 overflow-hidden">
        <SplitViewRoot />
      </div>

      {/* Step footer — always visible, outside the pane system */}
      <WorkspaceStepFooter />
    </div>
  )
}
```

- [ ] **Step 2: Create WorkspaceStepFooter**

```typescript
// web/src/features/workspace/components/WorkspaceStepFooter.tsx
import { useFlowDefinition, useCurrentStepId, useWorkflowActions } from '../stores/hooks/use-workflow'
import { usePaneActions, useActivePaneId } from '../stores/hooks/use-pane-store'
import { useBuffers } from '../stores/hooks/use-buffer-store'
import type { CrowbarChatContent } from '@/features/panes/types/pane-content'

/**
 * Always-visible step tab bar at the bottom of the workspace.
 * Clicking a step:
 *   1. Sets currentStepId in the workflow slice
 *   2. If the crowbarChat buffer is not active in the current pane, activates it
 */
export function WorkspaceStepFooter() {
  const flowDefinition = useFlowDefinition()
  const currentStepId = useCurrentStepId()
  const { setCurrentStep } = useWorkflowActions()
  const activePaneId = useActivePaneId()
  const paneActions = usePaneActions()
  const buffers = useBuffers()

  if (!flowDefinition) return null

  function handleStepClick(stepId: string) {
    setCurrentStep(stepId)

    // Navigate to crowbarChat buffer if it isn't the active buffer in the active pane
    const chatBuffer = buffers.find(
      b => b.type === 'crowbarChat'
    ) as CrowbarChatContent | undefined

    if (chatBuffer) {
      const activePane = paneActions.getActivePane()
      if (activePane?.activeBufferId !== chatBuffer.id) {
        // If the chat buffer is in an existing pane, activate it there
        const chatPane = paneActions.getPaneByBufferId(chatBuffer.id)
        if (chatPane) {
          paneActions.activatePaneBuffer(chatPane.id, chatBuffer.id)
          paneActions.setActivePane(chatPane.id)
        } else {
          // Buffer not in any pane — add it to the active pane
          paneActions.addBufferToPane(activePaneId, chatBuffer.id, true)
        }
      }
    }
  }

  return (
    <div className="flex h-8 shrink-0 items-center gap-1 border-t border-border bg-card px-2">
      {flowDefinition.steps.map(step => (
        <button
          key={step.id}
          onClick={() => handleStepClick(step.id)}
          className={[
            'flex h-6 items-center gap-1.5 rounded px-2 text-xs font-medium transition-colors',
            step.id === currentStepId
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-foreground',
            step.isCompleted && step.id !== currentStepId
              ? 'opacity-60'
              : '',
          ].join(' ')}
        >
          {step.isCompleted && step.id !== currentStepId && (
            <span className="text-[10px]">✓</span>
          )}
          {step.label}
        </button>
      ))}
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/components/WorkspaceLayoutRoot.tsx \
        web/src/features/workspace/components/WorkspaceStepFooter.tsx
git commit -m "feat(workspace): WorkspaceLayoutRoot and WorkspaceStepFooter"
```

---

### Task 24: FlowContent + Step Views

Replace the stub `FlowContent.tsx` (created in Task 19) with the real implementation. Then create `ChatView`, `DiffView`, and `SplitView`.

**Files:**
- Replace: `web/src/features/workflow/components/FlowContent.tsx`
- Create: `web/src/features/workflow/components/ChatView.tsx`
- Create: `web/src/features/workflow/components/DiffView.tsx`
- Create: `web/src/features/workflow/components/SplitView.tsx`

- [ ] **Step 1: Create ChatView**

```typescript
// web/src/features/workflow/components/ChatView.tsx
interface ChatViewProps {
  workspaceId: string
  stepId: string
}

/**
 * Renders the conversational chat interface for a workflow step.
 * Connects to the backend conversation for this workspace + step combination.
 */
export function ChatView({ workspaceId, stepId }: ChatViewProps) {
  // TODO(backend): replace with real conversation component when backend is wired
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <p className="text-sm">Chat — workspace: {workspaceId}, step: {stepId}</p>
    </div>
  )
}
```

- [ ] **Step 2: Create DiffView**

```typescript
// web/src/features/workflow/components/DiffView.tsx
interface DiffViewProps {
  workspaceId: string
  stepId: string
}

/**
 * Renders a diff viewer with inline comments for a workflow step.
 * Driven by the backend diff payload for this step.
 */
export function DiffView({ workspaceId, stepId }: DiffViewProps) {
  // TODO(backend): replace with real diff viewer when backend is wired
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <p className="text-sm">Diff — workspace: {workspaceId}, step: {stepId}</p>
    </div>
  )
}
```

- [ ] **Step 3: Create SplitView**

```typescript
// web/src/features/workflow/components/SplitView.tsx
import { ChatView } from './ChatView'
import { DiffView } from './DiffView'

interface SplitViewProps {
  workspaceId: string
  stepId: string
}

/**
 * Renders chat and diff side by side. Chat on the left, diff on the right.
 */
export function SplitView({ workspaceId, stepId }: SplitViewProps) {
  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex-1 min-w-0 border-r border-border overflow-hidden">
        <ChatView workspaceId={workspaceId} stepId={stepId} />
      </div>
      <div className="flex-1 min-w-0 overflow-hidden">
        <DiffView workspaceId={workspaceId} stepId={stepId} />
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Replace FlowContent stub with real implementation**

```typescript
// web/src/features/workflow/components/FlowContent.tsx
import { useCurrentStep } from '@/features/workspace/stores/hooks/use-workflow'
import { ChatView } from './ChatView'
import { DiffView } from './DiffView'
import { SplitView } from './SplitView'

interface FlowContentProps {
  workspaceId: string
}

/**
 * Renders the content for the currently active workflow step.
 * The backend drives what is shown via the step's contentType field.
 * No router Outlet — reads step from the workspace store directly.
 */
export function FlowContent({ workspaceId }: FlowContentProps) {
  const currentStep = useCurrentStep()

  if (!currentStep) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">No active step</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {currentStep.contentType === 'chat' && (
        <ChatView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'diff' && (
        <DiffView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'split' && (
        <SplitView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
    </div>
  )
}
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workflow/components/FlowContent.tsx \
        web/src/features/workflow/components/ChatView.tsx \
        web/src/features/workflow/components/DiffView.tsx \
        web/src/features/workflow/components/SplitView.tsx
git commit -m "feat(workflow): FlowContent with ChatView, DiffView, SplitView"
```

---

## Phase 7 — Shell & Routes

### Task 25: Simplify IDEShell

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

- [ ] **Step 1: Rewrite IDEShell**

Replace the file content:

```typescript
// web/src/components/layout/IDEShell.tsx
import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
import { SidebarHeader } from './SidebarHeader'
import { SidebarTabs } from './SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { WorkspaceView } from '@/features/workspace/components/WorkspaceView'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { ErrorBoundary } from '@/components/ErrorBoundary'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find(r => r.workspaces.some(ws => ws.id === activeWorkspaceId))
  const activeWorkspace = activeRepo?.workspaces.find(ws => ws.id === activeWorkspaceId)
  const workspaceLabel = activeWorkspace
    ? `${activeRepo?.name ?? ''} / ${activeWorkspace.branch}`
    : undefined

  const chatTabLabel = chats.find(c => c.id === activeChatId)?.title ?? 'Chat'

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <ResizablePanelGroup orientation="horizontal" className="h-full">

        {/* ── Sidebar ─────────────────────────────────────────── */}
        <ResizablePanel defaultSize="20%" minSize="12%" maxSize="45%" className="flex flex-col overflow-hidden">
          <div className="flex h-full flex-col overflow-hidden border-r border-border bg-card">
            <ErrorBoundary>
              <SidebarHeader
                userInitials="MU"
                onProjectsClick={() => void navigate({ to: '/projects' })}
                onProjectSelect={() => void navigate({ to: '/' })}
                onSettingsClick={() => setSettingsOpen(true)}
              />
              <SidebarTabs
                chats={chats}
                repos={repos}
                collapsedRepos={collapsedRepos}
                activeChatId={activeChatId}
                activeWorkspaceId={activeWorkspaceId}
                activeWorkspaceRepoPath={activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'}
                onChatClick={id => void navigate({ to: '/chat/$chatId', params: { chatId: id } })}
                onWorkspaceClick={(_repoId, wsId) => void navigate({ to: '/workspaces/$wsId', params: { wsId } })}
                onNewChat={() => {
                  const chat = createMockChat()
                  addChat({ id: chat.id, title: chat.title, age: chat.age })
                  void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
                }}
                onNewWorkspace={() => void navigate({ to: '/workspaces/new' })}
                onDeleteChat={id => { deleteChat(id); if (activeChatId === id) void navigate({ to: '/' }) }}
                onDeleteWorkspace={wsId => { deleteWorkspace(wsId); if (activeWorkspaceId === wsId) void navigate({ to: '/' }) }}
                onRepoToggle={toggleRepo}
              />
            </ErrorBoundary>
          </div>
        </ResizablePanel>

        <ResizableHandle />

        {/* ── Main content ──────────────────────────────────────── */}
        <ResizablePanel className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <ErrorBoundary>
            {activeWorkspaceId ? (
              <WorkspaceView wsId={activeWorkspaceId} label={workspaceLabel} />
            ) : activeChatId ? (
              <div className="flex h-full flex-col overflow-hidden">
                <div className="flex items-center border-b border-border px-3 py-1 text-sm font-medium">
                  {chatTabLabel}
                </div>
                <Outlet />
              </div>
            ) : (
              <div className="flex h-full flex-col overflow-hidden">
                <Outlet />
              </div>
            )}
          </ErrorBoundary>
        </ResizablePanel>

      </ResizablePanelGroup>

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  )
}
```

- [ ] **Step 2: Update workspace route**

Find the workspace route file (likely `web/src/routes/workspaces/$wsId.tsx` or similar). The route should no longer have a `$step` segment. If a `/$step` child route exists, delete it. The route just renders `<IDEShell />` as the root layout — `WorkspaceView` handles everything from there.

Search:
```bash
find web/src/routes -name "*.tsx" | xargs grep -l "wsId\|workspaces"
```

Ensure no route file contains `<Outlet />` inside a workspace-specific component. Only `IDEShell` uses `<Outlet />` (for the non-workspace chat view).

- [ ] **Step 3: Type-check and run dev**

```bash
cd web && npx tsc --noEmit 2>&1
```

Expected: zero errors.

```bash
cd web && npm run dev
```

Navigate to a workspace. Confirm:
- Workspace renders inside the main panel
- Step footer is visible at the bottom
- Sidebar still works
- Settings dialog opens

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx
git commit -m "feat(workspace): simplify IDEShell — delegates to WorkspaceView"
```

---

### Task 26: Seed Mock Flow Definition

The workflow slice starts with `flowDefinition: null`. Until the real backend exists, seed a mock flow so the step footer renders.

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`

- [ ] **Step 1: Add flow seeding to use-workspace-effects.ts**

Add a `useEffect` that sets a mock flow definition:

```typescript
// Add this import at the top
import { useWorkflowActions } from './use-workflow'
import type { FlowDefinition } from '@/features/workflow/types/workflow'

// Add inside useWorkspaceEffects():
const { setFlowDefinition, setCurrentStep } = useWorkflowActions()

useEffect(() => {
  const mockFlow: FlowDefinition = {
    flowId: `flow-${wsId}`,
    flowType: 'crowbar-default',
    steps: [
      { id: 'brainstorm', label: 'Brainstorm', contentType: 'chat', isCompleted: false, isActive: true },
      { id: 'spec',       label: 'Spec',       contentType: 'diff', isCompleted: false, isActive: false },
      { id: 'build',      label: 'Build',      contentType: 'split', isCompleted: false, isActive: false },
      { id: 'ai_review',  label: 'AI Review',  contentType: 'diff', isCompleted: false, isActive: false },
      { id: 'human_review', label: 'Review',   contentType: 'chat', isCompleted: false, isActive: false },
    ],
  }
  setFlowDefinition(mockFlow)
  setCurrentStep('brainstorm')
}, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 2: Manual smoke test**

```bash
cd web && npm run dev
```

1. Open a workspace
2. The step footer should show: Brainstorm | Spec | Build | AI Review | Review
3. Brainstorm should be highlighted as active
4. Click "Spec" → step highlights Spec, main area shows DiffView placeholder
5. Click "Build" → step highlights Build, main area shows SplitView placeholder (chat + diff side by side)
6. Click a file in the file explorer → preview tab appears in the pane
7. Close the tab → tab disappears
8. Drag a tab to split the pane → both panes visible simultaneously

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-workspace-effects.ts
git commit -m "feat(workspace): seed mock flow definition for development"
```

---

## Phase 8 — Full Test Run

### Task 27: Run All Tests

- [ ] **Step 1: Run the full test suite**

```bash
cd web && npx vitest run
```

Expected: all slice tests pass. If any fail, fix them before proceeding.

- [ ] **Step 2: Type-check the entire project**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "feat(workspace): workspace store refactor complete — all tests passing"
```

---

## Known Follow-Ons (Out of Scope This Plan)

- `ChatView`, `DiffView`, `SplitView` real implementations (backend wiring)
- LSP server connection inside `lsp-slice`
- Terminal WebSocket/PTY connection inside `terminal-slice`
- Real file system backend (currently using mocks)
- Workspace snapshot auto-save on state change (currently only loaded on first visit)
