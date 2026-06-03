# Pane System Normalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the recursive PaneNode tree with a flat `panes: Record<string, PaneGroup>` + `LayoutNode` structural tree, delete the legacy standalone pane store, add CSS custom-property resize, keyboard accessibility on handles, per-pane error boundaries, and proper IndexDB persistence.

**Architecture:** Layout structure (splits + sizes) lives in a lightweight `LayoutNode` tree containing only IDs. All `PaneGroup` data lives in a flat record keyed by pane ID — O(1) access and mutation. The renderer reads pane data from the flat map and writes CSS custom properties on the split container during drag; the store only updates on mouseup.

**Tech Stack:** React 18, Zustand + Immer, TypeScript strict, Vite, `idb` library (already installed), `nanoid` (already installed), Tailwind CSS.

**Spec:** `docs/superpowers/specs/2026-05-31-pane-system-normalization-design.md`

---

## File Map

| Status | Path | Role |
|---|---|---|
| **Modify** | `web/src/features/panes/types/pane.ts` | Add `LayoutLeaf`, `LayoutSplit`, `LayoutNode`; retire `PaneSplit`/`PaneNode` at end |
| **Create** | `web/src/features/panes/utils/pane-layout.ts` | All layout-structural ops on `LayoutNode` |
| **Create** | `web/src/__tests__/features/panes/utils/pane-layout.test.ts` | Tests for pane-layout.ts |
| **Rewrite** | `web/src/features/workspace/stores/slices/pane-slice.ts` | New flat-map state shape + all actions |
| **Modify** | `web/src/features/workspace/stores/workspace-store.ts` | Update `WorkspaceSnapshot`, remove `findPaneGroup` import |
| **Modify** | `web/src/features/workspace/stores/hooks/use-pane-store.ts` | New hooks for flat map |
| **Create** | `web/src/features/panes/components/pane-boundary.tsx` | Per-pane error boundary |
| **Rewrite** | `web/src/features/panes/components/pane-resize-handle.tsx` | Container-ref resize + keyboard |
| **Rewrite** | `web/src/features/panes/components/pane-node-renderer.tsx` | CSS custom props + flat map reads |
| **Modify** | `web/src/features/panes/components/split-view-root.tsx` | Updated hooks |
| **Modify** | `web/src/features/tabs/components/tab-bar.tsx` | Replace tree traversal with flat map |
| **Modify** | `web/src/features/workspace/stores/workspace-store-registry.ts` | `getActiveWorkspaceStore` + persistence shape |
| **Modify** | `web/src/features/panes/utils/pane-command-actions.ts` | Migrate from legacy store |
| **Modify** | `web/src/features/panes/utils/pane-drop-actions.ts` | Migrate from legacy store |
| **Modify** | `web/src/features/panes/utils/pane-buffer-actions.ts` | Migrate from legacy store |
| **Modify** | `web/src/features/panes/utils/pane-split-actions.ts` | Migrate from legacy store |
| **Modify** | `web/src/lib/persistence/schemas.ts` | `WorkspaceLayout` typed fields |
| **Modify** | `web/src/lib/persistence/idb.ts` | Version bump to 2 + migration |
| **Modify** | `web/src/features/workspace/stores/workspace-persistence.ts` | Snapshot type update |
| **Delete** | `web/src/features/panes/stores/pane-store.ts` | Legacy singleton — removed |
| **Delete** | `web/src/features/panes/utils/pane-tree.ts` | Replaced by pane-layout.ts |

---

## Task 1: Add LayoutNode types to pane.ts

**Files:**
- Modify: `web/src/features/panes/types/pane.ts`

- [ ] **Step 1: Add new layout types**

Open `web/src/features/panes/types/pane.ts`. The file currently exports `PaneGroup`, `PaneSplit`, `PaneNode`, `SplitDirection`, `SplitPlacement`, `PanePosition`, `ROOT_PANE_POSITION`, and `PaneState`. Add the three new types below the `PaneGroup` interface, before `PaneSplit`:

```ts
export interface LayoutLeaf {
  type: 'pane'
  id: string
}

export interface LayoutSplit {
  type: 'split'
  id: string
  direction: 'horizontal' | 'vertical'
  sizes: [number, number]
  first: LayoutNode
  second: LayoutNode
}

export type LayoutNode = LayoutLeaf | LayoutSplit
```

Keep `PaneSplit`, `PaneNode` in the file for now — they will be removed in Task 12 after all consumers are migrated.

- [ ] **Step 2: Verify typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane.ts" | head -20
```

Expected: no errors in `pane.ts`.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/types/pane.ts
git commit -m "feat(pane-types): add LayoutLeaf, LayoutSplit, LayoutNode types"
```

---

## Task 2: Create pane-layout.ts

**Files:**
- Create: `web/src/features/panes/utils/pane-layout.ts`

This file replaces `pane-tree.ts`. It works exclusively on `LayoutNode` — no `PaneGroup` data, no buffer operations.

- [ ] **Step 1: Create the file**

Create `web/src/features/panes/utils/pane-layout.ts` with the following complete content:

```ts
import { nanoid } from 'nanoid'
import { DEFAULT_SPLIT_RATIO, MIN_PANE_SIZE } from '../constants/pane'
import type { LayoutLeaf, LayoutNode, LayoutSplit } from '../types/pane'

export interface FlatLayoutEntry {
  node: LayoutNode
  size: number
  path: Array<{ splitId: string; childIndex: 0 | 1 }>
}

export function createLeaf(id: string): LayoutLeaf {
  return { type: 'pane', id }
}

export function createSplit(
  direction: 'horizontal' | 'vertical',
  first: LayoutNode,
  second: LayoutNode,
  sizes: [number, number] = DEFAULT_SPLIT_RATIO,
): LayoutSplit {
  return { type: 'split', id: nanoid(), direction, sizes: normalizeSizes(sizes), first, second }
}

function normalizeSizes(sizes: [number, number]): [number, number] {
  const a = Number.isFinite(sizes[0]) ? sizes[0] : DEFAULT_SPLIT_RATIO[0]
  const b = Number.isFinite(sizes[1]) ? sizes[1] : DEFAULT_SPLIT_RATIO[1]
  const total = a + b
  if (total <= 0) return DEFAULT_SPLIT_RATIO
  const norm: [number, number] = [(a / total) * 100, (b / total) * 100]
  const min = Math.min(MIN_PANE_SIZE, 49)
  if (norm[0] < min) return [min, 100 - min]
  if (norm[1] < min) return [100 - min, min]
  return norm
}

export function findLeaf(root: LayoutNode, paneId: string): LayoutLeaf | null {
  if (root.type === 'pane') return root.id === paneId ? root : null
  return findLeaf(root.first, paneId) ?? findLeaf(root.second, paneId)
}

export function findSplit(root: LayoutNode, splitId: string): LayoutSplit | null {
  if (root.type === 'pane') return null
  if (root.id === splitId) return root
  return findSplit(root.first, splitId) ?? findSplit(root.second, splitId)
}

export function findParentSplit(root: LayoutNode, nodeId: string): LayoutSplit | null {
  if (root.type === 'pane') return null
  if (root.first.id === nodeId || root.second.id === nodeId) return root
  return findParentSplit(root.first, nodeId) ?? findParentSplit(root.second, nodeId)
}

export function getAllLeafIds(root: LayoutNode): string[] {
  if (root.type === 'pane') return [root.id]
  return [...getAllLeafIds(root.first), ...getAllLeafIds(root.second)]
}

export function getFirstLeafId(root: LayoutNode): string {
  if (root.type === 'pane') return root.id
  return getFirstLeafId(root.first)
}

export function splitLayout(
  root: LayoutNode,
  paneId: string,
  direction: 'horizontal' | 'vertical',
  placement: 'before' | 'after' = 'after',
): { layout: LayoutNode; newPaneId: string } | null {
  const newPaneId = nanoid()
  const result = insertLeaf(root, paneId, createLeaf(newPaneId), direction, placement)
  if (!result) return null
  return { layout: result, newPaneId }
}

function insertLeaf(
  node: LayoutNode,
  targetId: string,
  newLeaf: LayoutLeaf,
  direction: 'horizontal' | 'vertical',
  placement: 'before' | 'after',
): LayoutNode | null {
  if (node.type === 'pane') {
    if (node.id !== targetId) return null
    const first = placement === 'before' ? newLeaf : node
    const second = placement === 'before' ? node : newLeaf
    return createSplit(direction, first, second)
  }
  const newFirst = insertLeaf(node.first, targetId, newLeaf, direction, placement)
  if (newFirst) return { ...node, first: newFirst }
  const newSecond = insertLeaf(node.second, targetId, newLeaf, direction, placement)
  if (newSecond) return { ...node, second: newSecond }
  return null
}

export function closeLayout(root: LayoutNode, paneId: string): LayoutNode | null {
  if (root.type === 'pane') return root.id === paneId ? null : root
  const newFirst = closeLayout(root.first, paneId)
  const newSecond = closeLayout(root.second, paneId)
  if (newFirst === null && newSecond === null) return null
  if (newFirst === null) return newSecond
  if (newSecond === null) return newFirst
  return { ...root, first: newFirst, second: newSecond }
}

export function updateSplitSizes(
  root: LayoutNode,
  splitId: string,
  sizes: [number, number],
): LayoutNode {
  if (root.type === 'pane') return root
  if (root.id === splitId) return { ...root, sizes: normalizeSizes(sizes) }
  return {
    ...root,
    first: updateSplitSizes(root.first, splitId, sizes),
    second: updateSplitSizes(root.second, splitId, sizes),
  }
}

export function distributeSplit(root: LayoutNode, splitId: string): LayoutNode {
  return updateSplitSizes(root, splitId, DEFAULT_SPLIT_RATIO)
}

export function normalizeLayout(root: LayoutNode): LayoutNode {
  if (root.type === 'pane') return root
  const first = normalizeLayout(root.first)
  const second = normalizeLayout(root.second)
  return { ...root, sizes: normalizeSizes(root.sizes), first, second }
}

export function flattenForRender(
  split: LayoutSplit,
  parentSize = 100,
  path: Array<{ splitId: string; childIndex: 0 | 1 }> = [],
): FlatLayoutEntry[] {
  const entries: FlatLayoutEntry[] = []
  for (let i = 0; i < 2; i++) {
    const idx = i as 0 | 1
    const child = idx === 0 ? split.first : split.second
    const childSize = (split.sizes[idx] / 100) * parentSize
    const childPath = [...path, { splitId: split.id, childIndex: idx }]
    if (child.type === 'split' && child.direction === split.direction) {
      entries.push(...flattenForRender(child, childSize, childPath))
    } else {
      entries.push({ node: child, size: childSize, path: childPath })
    }
  }
  return entries
}

export function resizeFlattenedLayout(
  root: LayoutNode,
  topSplitId: string,
  index: number,
  sizes: [number, number],
): LayoutNode {
  const split = findSplit(root, topSplitId)
  if (!split) return root
  const entries = flattenForRender(split)
  if (index < 0 || index >= entries.length - 1) return root

  const total = entries[index].size + entries[index + 1].size
  const newEntries = entries.map((e, i) => {
    if (i === index) return { ...e, size: (sizes[0] / 100) * total }
    if (i === index + 1) return { ...e, size: (sizes[1] / 100) * total }
    return e
  })
  return writeFlatSizesToLayout(newEntries, root)
}

function writeFlatSizesToLayout(
  entries: FlatLayoutEntry[],
  root: LayoutNode,
): LayoutNode {
  const splitTotals = new Map<string, { first: number; second: number }>()
  for (const entry of entries) {
    for (const step of entry.path) {
      if (!splitTotals.has(step.splitId)) {
        splitTotals.set(step.splitId, { first: 0, second: 0 })
      }
      const t = splitTotals.get(step.splitId)!
      if (step.childIndex === 0) t.first += entry.size
      else t.second += entry.size
    }
  }
  let next = root
  for (const [splitId, totals] of splitTotals) {
    const sum = totals.first + totals.second
    if (sum <= 0) continue
    next = updateSplitSizes(next, splitId, [
      (totals.first / sum) * 100,
      (totals.second / sum) * 100,
    ])
  }
  return next
}

export function getAdjacentLeafId(
  root: LayoutNode,
  paneId: string,
  direction: 'left' | 'right' | 'up' | 'down',
): string | null {
  const splitDir = direction === 'left' || direction === 'right' ? 'horizontal' : 'vertical'
  const forward = direction === 'right' || direction === 'down'
  return findAdjacent(root, paneId, splitDir, forward)
}

function findAdjacent(
  root: LayoutNode,
  paneId: string,
  splitDir: 'horizontal' | 'vertical',
  forward: boolean,
): string | null {
  if (root.type === 'pane') return null
  const firstIds = getAllLeafIds(root.first)
  const secondIds = getAllLeafIds(root.second)
  if (root.direction === splitDir) {
    if (firstIds.includes(paneId) && forward) return getFirstLeafId(root.second)
    if (secondIds.includes(paneId) && !forward) return getFirstLeafId(root.first)
  }
  return (
    findAdjacent(root.first, paneId, splitDir, forward) ??
    findAdjacent(root.second, paneId, splitDir, forward)
  )
}
```

- [ ] **Step 2: Verify typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-layout" | head -20
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/utils/pane-layout.ts
git commit -m "feat(pane-layout): add normalized layout utility replacing pane-tree"
```

---

## Task 3: Tests for pane-layout.ts

**Files:**
- Create: `web/src/__tests__/features/panes/utils/pane-layout.test.ts`

- [ ] **Step 1: Create test file**

```ts
import { describe, it, expect } from 'vitest'
import {
  createLeaf,
  createSplit,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  getFirstLeafId,
  updateSplitSizes,
  distributeSplit,
  flattenForRender,
  resizeFlattenedLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'

describe('createLeaf', () => {
  it('creates a leaf node with given id', () => {
    const leaf = createLeaf('abc')
    expect(leaf).toEqual({ type: 'pane', id: 'abc' })
  })
})

describe('createSplit', () => {
  it('creates a split with default 50/50 sizes', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    expect(split.type).toBe('split')
    expect(split.direction).toBe('horizontal')
    expect(split.first).toBe(a)
    expect(split.second).toBe(b)
    expect(split.sizes[0] + split.sizes[1]).toBeCloseTo(100)
  })

  it('clamps sizes below MIN_PANE_SIZE', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b, [5, 95])
    expect(split.sizes[0]).toBeGreaterThanOrEqual(10)
  })
})

describe('findLeaf', () => {
  it('finds a leaf at root', () => {
    const a = createLeaf('a')
    expect(findLeaf(a, 'a')).toBe(a)
  })

  it('finds a leaf nested in split', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    expect(findLeaf(split, 'b')?.id).toBe('b')
  })

  it('returns null for unknown id', () => {
    expect(findLeaf(createLeaf('a'), 'z')).toBeNull()
  })
})

describe('getAllLeafIds', () => {
  it('returns single id for leaf', () => {
    expect(getAllLeafIds(createLeaf('x'))).toEqual(['x'])
  })

  it('returns all leaf ids in order', () => {
    const tree = createSplit('horizontal',
      createLeaf('a'),
      createSplit('horizontal', createLeaf('b'), createLeaf('c'))
    )
    expect(getAllLeafIds(tree)).toEqual(['a', 'b', 'c'])
  })
})

describe('splitLayout', () => {
  it('splits a single leaf and returns new pane id', () => {
    const root = createLeaf('root')
    const result = splitLayout(root, 'root', 'horizontal', 'after')
    expect(result).not.toBeNull()
    expect(result!.layout.type).toBe('split')
    expect(getAllLeafIds(result!.layout)).toHaveLength(2)
    expect(getAllLeafIds(result!.layout)[0]).toBe('root')
  })

  it('places new pane before when placement is before', () => {
    const root = createLeaf('root')
    const result = splitLayout(root, 'root', 'horizontal', 'before')!
    expect(getAllLeafIds(result.layout)[1]).toBe('root')
  })

  it('returns null for unknown pane id', () => {
    expect(splitLayout(createLeaf('a'), 'z', 'horizontal')).toBeNull()
  })
})

describe('closeLayout', () => {
  it('returns null when closing the only leaf', () => {
    expect(closeLayout(createLeaf('a'), 'a')).toBeNull()
  })

  it('returns the sibling when closing one side of a split', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    const result = closeLayout(split, 'a')
    expect(result?.type).toBe('pane')
    expect((result as any).id).toBe('b')
  })

  it('collapses nested split correctly', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const c = createLeaf('c')
    const inner = createSplit('horizontal', b, c)
    const outer = createSplit('horizontal', a, inner)
    const result = closeLayout(outer, 'b')!
    expect(getAllLeafIds(result)).toEqual(['a', 'c'])
  })
})

describe('flattenForRender', () => {
  it('flattens same-direction nested splits', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const c = createLeaf('c')
    const inner = createSplit('horizontal', b, c, [50, 50])
    const outer = createSplit('horizontal', a, inner, [50, 50])
    const entries = flattenForRender(outer)
    expect(entries).toHaveLength(3)
    expect(entries.map(e => (e.node as any).id)).toEqual(['a', 'b', 'c'])
  })

  it('does not flatten cross-direction splits', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const c = createLeaf('c')
    const inner = createSplit('vertical', b, c)
    const outer = createSplit('horizontal', a, inner)
    const entries = flattenForRender(outer)
    expect(entries).toHaveLength(2)
    expect(entries[0].node.id).toBe('a')
    expect(entries[1].node.type).toBe('split')
  })
})

describe('resizeFlattenedLayout', () => {
  it('updates split sizes on resize', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b, [50, 50])
    const result = resizeFlattenedLayout(split, split.id, 0, [70, 30])
    const updated = findSplit(result, split.id)!
    expect(updated.sizes[0]).toBeGreaterThan(60)
    expect(updated.sizes[0] + updated.sizes[1]).toBeCloseTo(100)
  })
})

describe('getAdjacentLeafId', () => {
  it('returns right sibling for rightward navigation', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    expect(getAdjacentLeafId(split, 'a', 'right')).toBe('b')
  })

  it('returns left sibling for leftward navigation', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    expect(getAdjacentLeafId(split, 'b', 'left')).toBe('a')
  })

  it('returns null when no adjacent pane exists', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b)
    expect(getAdjacentLeafId(split, 'a', 'left')).toBeNull()
  })
})
```

- [ ] **Step 2: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/panes/utils/pane-layout.test.ts
```

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/__tests__/features/panes/utils/pane-layout.test.ts
git commit -m "test(pane-layout): full coverage of layout utility functions"
```

---

## Task 4: Rewrite pane-slice.ts

**Files:**
- Rewrite: `web/src/features/workspace/stores/slices/pane-slice.ts`

This is the largest single change. The state shape changes from `paneRoot: PaneNode` + `bottomRoot: PaneNode` to `panes: Record<string, PaneGroup>` + `rootLayout: LayoutNode` + `bottomLayout: LayoutNode`.

- [ ] **Step 1: Replace the entire file**

Replace `web/src/features/workspace/stores/slices/pane-slice.ts` with:

```ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup, LayoutNode, SplitDirection, SplitPlacement } from '@/features/panes/types/pane'
import {
  createLeaf,
  createSplit,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  getFirstLeafId,
  updateSplitSizes,
  distributeSplit,
  resizeFlattenedLayout,
  normalizeLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'
import { nanoid } from 'nanoid'

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
  clearPreviewBufferEverywhere(bufferId: string): void
  switchToNextBufferInPane(): void
  switchToPreviousBufferInPane(): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
}

export interface PaneSlice {
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: PaneActions
}

function makeRootLeaf(): PaneGroup {
  return { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function makeBottomLeaf(): PaneGroup {
  return { id: BOTTOM_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function layoutContainsPane(layout: LayoutNode, paneId: string): boolean {
  return findLeaf(layout, paneId) !== null
}

function getLayoutForPane(
  state: Pick<PaneSlice, 'rootLayout' | 'bottomLayout'>,
  paneId: string,
): 'rootLayout' | 'bottomLayout' {
  if (layoutContainsPane(state.rootLayout, paneId)) return 'rootLayout'
  if (layoutContainsPane(state.bottomLayout, paneId)) return 'bottomLayout'
  return paneId === BOTTOM_PANE_ID ? 'bottomLayout' : 'rootLayout'
}

export const createPaneSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get) => {
  const rootGroup = makeRootLeaf()
  const bottomGroup = makeBottomLeaf()

  return {
    panes: { [ROOT_PANE_ID]: rootGroup, [BOTTOM_PANE_ID]: bottomGroup },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf(BOTTOM_PANE_ID),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    fullscreenPaneId: null,

    paneActions: {
      splitPane(paneId, direction, bufferId?, placement = 'after') {
        let newPaneId: string | null = null
        set(state => {
          const target = getLayoutForPane(state, paneId)
          const layout = state[target]
          const result = splitLayout(layout, paneId, direction, placement)
          if (!result) return
          state[target] = result.layout
          newPaneId = result.newPaneId
          const newGroup: PaneGroup = {
            id: newPaneId,
            type: 'group',
            bufferIds: bufferId ? [bufferId] : [],
            activeBufferId: bufferId ?? null,
          }
          state.panes[newPaneId] = newGroup
          state.activePaneId = newPaneId
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
        })
        return newPaneId
      },

      closePane(paneId) {
        set(state => {
          const target = getLayoutForPane(state, paneId)
          const layout = state[target]
          const closingPane = state.panes[paneId]
          const result = closeLayout(layout, paneId)

          if (result !== null) {
            state[target] = normalizeLayout(result)
            const remainingIds = getAllLeafIds(state[target])
            const fallbackId = remainingIds[0] ?? (target === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID)

            if (closingPane) {
              for (const bufferId of closingPane.bufferIds) {
                const fallbackPane = state.panes[fallbackId]
                if (fallbackPane && !fallbackPane.bufferIds.includes(bufferId)) {
                  fallbackPane.bufferIds.push(bufferId)
                }
              }
              if (state.activePaneId === paneId && closingPane.activeBufferId) {
                const fallbackPane = state.panes[fallbackId]
                if (fallbackPane) fallbackPane.activeBufferId = closingPane.activeBufferId
              }
            }

            if (state.activePaneId === paneId) state.activePaneId = fallbackId
          } else {
            const fallbackId = target === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID
            const fresh = target === 'rootLayout' ? makeRootLeaf() : makeBottomLeaf()
            state.panes[fallbackId] = fresh
            state[target] = createLeaf(fallbackId)
            if (state.activePaneId === paneId) {
              state.activePaneId = target === 'rootLayout' ? ROOT_PANE_ID : ROOT_PANE_ID
            }
          }

          delete state.panes[paneId]
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
          const pane = state.panes[paneId]
          if (!pane) return
          pane.activeBufferId = bufferId
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [paneId, ...state.mostRecentActivePaneIds.filter(id => id !== paneId)]
        })
      },

      addBufferToPane(paneId, bufferId, setActive = true) {
        set(state => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.bufferIds.includes(bufferId)) pane.bufferIds.push(bufferId)
          if (setActive) pane.activeBufferId = bufferId
        })
      },

      removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
        set(state => {
          const pane = state.panes[paneId]
          if (!pane) return
          pane.bufferIds = pane.bufferIds.filter(id => id !== bufferId)
          if (pane.activeBufferId === bufferId) {
            pane.activeBufferId = pane.bufferIds[0] ?? null
          }
          if (pane.previewBufferId === bufferId) pane.previewBufferId = null

          if (!preserveEmptyPane && pane.bufferIds.length === 0 && paneId !== ROOT_PANE_ID && paneId !== BOTTOM_PANE_ID) {
            const target = getLayoutForPane(state, paneId)
            const result = closeLayout(state[target], paneId)
            if (result !== null) {
              state[target] = normalizeLayout(result)
              const remainingIds = getAllLeafIds(state[target])
              if (state.activePaneId === paneId) {
                state.activePaneId = remainingIds[0] ?? ROOT_PANE_ID
              }
              delete state.panes[paneId]
              state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(id => id !== paneId)
            }
          }
        })
      },

      moveBufferToPane(bufferId, fromPaneId, toPaneId) {
        set(state => {
          const fromPane = state.panes[fromPaneId]
          const toPane = state.panes[toPaneId]
          if (!fromPane || !toPane) return

          fromPane.bufferIds = fromPane.bufferIds.filter(id => id !== bufferId)
          if (fromPane.activeBufferId === bufferId) {
            fromPane.activeBufferId = fromPane.bufferIds[0] ?? null
          }

          if (!toPane.bufferIds.includes(bufferId)) toPane.bufferIds.push(bufferId)
          toPane.activeBufferId = bufferId
          state.activePaneId = toPaneId
          state.mostRecentActivePaneIds = [toPaneId, ...state.mostRecentActivePaneIds.filter(id => id !== toPaneId)]

          if (fromPane.bufferIds.length === 0 && fromPaneId !== ROOT_PANE_ID && fromPaneId !== BOTTOM_PANE_ID) {
            const target = getLayoutForPane(state, fromPaneId)
            const result = closeLayout(state[target], fromPaneId)
            if (result !== null) {
              state[target] = normalizeLayout(result)
              delete state.panes[fromPaneId]
              state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(id => id !== fromPaneId)
            }
          }
        })
      },

      setPanePreviewBuffer(paneId, bufferId) {
        set(state => {
          const pane = state.panes[paneId]
          if (pane) pane.previewBufferId = bufferId
        })
      },

      setPaneBufferPinned(paneId, bufferId, pinned) {
        set(state => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.pinnedBufferIds) pane.pinnedBufferIds = []
          if (pinned) {
            if (!pane.pinnedBufferIds.includes(bufferId)) pane.pinnedBufferIds.push(bufferId)
          } else {
            pane.pinnedBufferIds = pane.pinnedBufferIds.filter(id => id !== bufferId)
          }
        })
      },

      reorderPaneBuffers(paneId, startIndex, endIndex) {
        set(state => {
          const pane = state.panes[paneId]
          if (!pane) return
          const ids = [...pane.bufferIds]
          const [moved] = ids.splice(startIndex, 1)
          ids.splice(endIndex, 0, moved)
          pane.bufferIds = ids
        })
      },

      resizePaneSplit(splitId, index, sizes) {
        set(state => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = resizeFlattenedLayout(state.rootLayout, splitId, index, sizes)
          } else if (findSplit(state.bottomLayout, splitId)) {
            state.bottomLayout = resizeFlattenedLayout(state.bottomLayout, splitId, index, sizes)
          }
        })
      },

      distributePaneSplit(splitId) {
        set(state => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = distributeSplit(state.rootLayout, splitId)
          } else {
            state.bottomLayout = distributeSplit(state.bottomLayout, splitId)
          }
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

      getAllPaneGroups() {
        return Object.values(get().panes)
      },

      getPaneById(paneId) {
        return get().panes[paneId] ?? null
      },

      getPaneByBufferId(bufferId) {
        return Object.values(get().panes).find(p => p.bufferIds.includes(bufferId)) ?? null
      },

      getActivePane() {
        return get().panes[get().activePaneId] ?? null
      },

      clearPreviewBufferEverywhere(bufferId) {
        set(state => {
          for (const pane of Object.values(state.panes)) {
            if (pane.previewBufferId === bufferId) pane.previewBufferId = null
          }
        })
      },

      switchToNextBufferInPane() {
        const state = get()
        const pane = state.panes[state.activePaneId]
        if (!pane || pane.bufferIds.length <= 1) return
        const currentIndex = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : -1
        const nextIndex = (currentIndex + 1) % pane.bufferIds.length
        get().paneActions.activatePaneBuffer(pane.id, pane.bufferIds[nextIndex])
      },

      switchToPreviousBufferInPane() {
        const state = get()
        const pane = state.panes[state.activePaneId]
        if (!pane || pane.bufferIds.length <= 1) return
        const currentIndex = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : 0
        const prevIndex = (currentIndex - 1 + pane.bufferIds.length) % pane.bufferIds.length
        get().paneActions.activatePaneBuffer(pane.id, pane.bufferIds[prevIndex])
      },

      navigateToPane(direction) {
        const state = get()
        const allLayouts: LayoutNode[] = [state.rootLayout, state.bottomLayout]
        for (const layout of allLayouts) {
          const adj = getAdjacentLeafId(layout, state.activePaneId, direction)
          if (adj && state.panes[adj]) {
            set(s => {
              s.activePaneId = adj
              s.mostRecentActivePaneIds = [adj, ...s.mostRecentActivePaneIds.filter(id => id !== adj)]
            })
            return
          }
        }
      },
    },
  }
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-slice\|workspace-store.types" | head -30
```

Expected: errors only from files that still import the old `paneRoot`/`bottomRoot` fields — those will be fixed in subsequent tasks.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/workspace/stores/slices/pane-slice.ts
git commit -m "feat(pane-slice): rewrite with flat panes map + LayoutNode trees"
```

---

## Task 5: Update workspace-store.ts and workspace-store.types.ts

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store.ts`
- Modify: `web/src/features/workspace/stores/workspace-persistence.ts`

- [ ] **Step 1: Update WorkspaceSnapshot in workspace-store.ts**

In `web/src/features/workspace/stores/workspace-store.ts`, replace the `WorkspaceSnapshot` type and remove the `findPaneGroup` import:

Remove this import:
```ts
import { findPaneGroup } from '@/features/panes/utils/pane-tree'
```

Replace `WorkspaceSnapshot` with:
```ts
export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'panes' | 'rootLayout' | 'bottomLayout'
    | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'currentStepId'
    | 'recentFiles'
    | 'terminalLayout'
  >
>
```

In `createWorkspaceStore`, replace the subscriber that uses `findPaneGroup`:

```ts
store.subscribe((state, prev) => {
  if (state.buffers === prev.buffers) return
  const activePane = state.panes[state.activePaneId] ?? null
  saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
})
```

- [ ] **Step 2: Update workspace-persistence.ts**

In `web/src/features/workspace/stores/workspace-persistence.ts`, ensure the snapshot key names match. The `loadFromLocalStorage` function returns `WorkspaceSnapshot | null` — its type will automatically update since `WorkspaceSnapshot` now picks `panes | rootLayout | bottomLayout` instead of `paneRoot | bottomRoot`. No code changes needed here if the file only does `JSON.parse(raw) as WorkspaceSnapshot`.

- [ ] **Step 3: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "workspace-store\b" | head -20
```

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/workspace/stores/workspace-store.ts src/features/workspace/stores/workspace-persistence.ts
git commit -m "feat(workspace-store): update snapshot type to normalized pane shape"
```

---

## Task 6: Update use-pane-store.ts hooks

**Files:**
- Rewrite: `web/src/features/workspace/stores/hooks/use-pane-store.ts`

- [ ] **Step 1: Replace file content**

```ts
import { useWorkspaceStoreContext } from '../workspace-context'
import type { PaneActions } from '../slices/pane-slice'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'

export const useRootLayout = (): LayoutNode =>
  useWorkspaceStoreContext(s => s.rootLayout)

export const useBottomLayout = (): LayoutNode =>
  useWorkspaceStoreContext(s => s.bottomLayout)

export const usePanes = (): Record<string, PaneGroup> =>
  useWorkspaceStoreContext(s => s.panes)

export const useFullscreenPaneId = (): string | null =>
  useWorkspaceStoreContext(s => s.fullscreenPaneId)

export const useActivePaneId = (): string =>
  useWorkspaceStoreContext(s => s.activePaneId)

export const useMostRecentActivePaneIds = (): string[] =>
  useWorkspaceStoreContext(s => s.mostRecentActivePaneIds)

export const usePaneActions = (): PaneActions =>
  useWorkspaceStoreContext(s => s.paneActions)

export const usePaneById = (paneId: string): PaneGroup | null =>
  useWorkspaceStoreContext(s => s.panes[paneId] ?? null)
```

Note: `usePaneRoot` and `useBottomRoot` are removed — replaced by `useRootLayout` and `useBottomLayout`. Any file importing them will show TypeScript errors, which will be fixed in subsequent tasks.

- [ ] **Step 2: Typecheck to find all consumers**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "usePaneRoot\|useBottomRoot" | head -20
```

Make note of which files have errors — they will each be fixed in later tasks.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/workspace/stores/hooks/use-pane-store.ts
git commit -m "feat(use-pane-store): update hooks for normalized layout (useRootLayout, usePanes)"
```

---

## Task 7: Create pane-boundary.tsx

**Files:**
- Create: `web/src/features/panes/components/pane-boundary.tsx`

- [ ] **Step 1: Create the file**

```tsx
import React from 'react'

interface Props {
  paneId: string
  children: React.ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class PaneBoundary extends React.Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidUpdate(prevProps: Props) {
    if (prevProps.paneId !== this.props.paneId && this.state.hasError) {
      this.setState({ hasError: false, error: null })
    }
  }

  render() {
    if (this.state.hasError) {
      return (
        <PaneErrorState
          error={this.state.error}
          onRetry={() => this.setState({ hasError: false, error: null })}
        />
      )
    }
    return this.props.children
  }
}

function PaneErrorState({ error, onRetry }: { error: Error | null; onRetry: () => void }) {
  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-3 bg-background p-8 text-center">
      <div className="text-sm font-medium text-destructive">This pane encountered an error</div>
      {error?.message && (
        <pre className="max-w-sm overflow-auto rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          {error.message}
        </pre>
      )}
      <button
        type="button"
        onClick={onRetry}
        className="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-foreground hover:bg-accent/80"
      >
        Retry
      </button>
    </div>
  )
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-boundary" | head -10
```

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/components/pane-boundary.tsx
git commit -m "feat(pane-boundary): add per-pane error boundary component"
```

---

## Task 8: Rewrite pane-resize-handle.tsx

**Files:**
- Rewrite: `web/src/features/panes/components/pane-resize-handle.tsx`

- [ ] **Step 1: Replace the entire file**

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'
import { MIN_PANE_SIZE } from '../constants/pane'

interface PaneResizeHandleProps {
  direction: 'horizontal' | 'vertical'
  index: number
  initialSizes: [number, number]
  splitContainerRef: React.RefObject<HTMLElement | null>
  onResize: (sizes: [number, number]) => void
  onReset?: () => void
}

export function PaneResizeHandle({
  direction,
  index,
  initialSizes,
  splitContainerRef,
  onResize,
  onReset,
}: PaneResizeHandleProps) {
  const [isDragging, setIsDragging] = useState(false)
  const isHorizontal = direction === 'horizontal'

  const startPositionRef = useRef(0)
  const startSizesRef = useRef<[number, number]>(initialSizes)
  const containerSizeRef = useRef(0)
  const latestPositionRef = useRef(0)
  const rafIdRef = useRef<number | null>(null)
  const committedSizesRef = useRef<[number, number]>(initialSizes)

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      const container = splitContainerRef.current
      if (!container) return

      const rect = container.getBoundingClientRect()
      containerSizeRef.current = isHorizontal ? rect.width : rect.height
      startSizesRef.current = initialSizes
      committedSizesRef.current = initialSizes
      startPositionRef.current = isHorizontal ? e.clientX : e.clientY
      latestPositionRef.current = startPositionRef.current
      setIsDragging(true)
    },
    [isHorizontal, initialSizes, splitContainerRef],
  )

  useEffect(() => {
    if (!isDragging) return

    const handleMouseMove = (e: MouseEvent) => {
      latestPositionRef.current = isHorizontal ? e.clientX : e.clientY
      if (rafIdRef.current !== null) return
      rafIdRef.current = requestAnimationFrame(() => {
        rafIdRef.current = null
        const containerSize = containerSizeRef.current
        if (containerSize === 0) return

        const delta = latestPositionRef.current - startPositionRef.current
        const deltaPct = (delta / containerSize) * 100
        const [startFirst, startSecond] = startSizesRef.current
        const total = startFirst + startSecond

        let newFirst = startFirst + deltaPct
        let newSecond = startSecond - deltaPct

        if (newFirst < MIN_PANE_SIZE) { newFirst = MIN_PANE_SIZE; newSecond = total - MIN_PANE_SIZE }
        if (newSecond < MIN_PANE_SIZE) { newSecond = MIN_PANE_SIZE; newFirst = total - MIN_PANE_SIZE }

        committedSizesRef.current = [newFirst, newSecond]

        const container = splitContainerRef.current
        if (container) {
          container.style.setProperty(`--pane-${index}-size`, String(newFirst))
          container.style.setProperty(`--pane-${index + 1}-size`, String(newSecond))
        }
      })
    }

    const handleMouseUp = () => {
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
      const container = splitContainerRef.current
      if (container) {
        container.style.removeProperty(`--pane-${index}-size`)
        container.style.removeProperty(`--pane-${index + 1}-size`)
      }
      onResize(committedSizesRef.current)
      setIsDragging(false)
    }

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
      if (rafIdRef.current !== null) {
        cancelAnimationFrame(rafIdRef.current)
        rafIdRef.current = null
      }
    }
  }, [isDragging, isHorizontal, index, onResize, splitContainerRef])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const relevant = isHorizontal
        ? ['ArrowLeft', 'ArrowRight']
        : ['ArrowUp', 'ArrowDown']
      if (!relevant.includes(e.key)) return
      e.preventDefault()
      const step = e.shiftKey ? 10 : 2
      const [first, second] = initialSizes
      const total = first + second
      const positive = e.key === 'ArrowRight' || e.key === 'ArrowDown'
      const delta = positive ? step : -step
      const newFirst = Math.max(MIN_PANE_SIZE, Math.min(total - MIN_PANE_SIZE, first + delta))
      onResize([newFirst, total - newFirst])
    },
    [isHorizontal, initialSizes, onResize],
  )

  return (
    <div
      className={`group relative flex shrink-0 items-center justify-center ${
        isHorizontal ? 'h-full w-1 cursor-col-resize' : 'h-1 w-full cursor-row-resize'
      }`}
      onMouseDown={handleMouseDown}
      onDoubleClick={onReset}
      onKeyDown={handleKeyDown}
      role="separator"
      aria-orientation={isHorizontal ? 'vertical' : 'horizontal'}
      aria-valuenow={Math.round(initialSizes[0])}
      aria-valuemin={MIN_PANE_SIZE}
      aria-valuemax={100 - MIN_PANE_SIZE}
      tabIndex={0}
    >
      <div
        className={`${isDragging ? 'bg-accent' : 'bg-transparent group-hover:bg-accent'} ${
          isHorizontal ? 'h-full w-px' : 'h-px w-full'
        }`}
      />
      {isDragging && (
        <div
          className={`fixed inset-0 z-50 ${isHorizontal ? 'cursor-col-resize' : 'cursor-row-resize'}`}
        />
      )}
    </div>
  )
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-resize-handle" | head -10
```

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/components/pane-resize-handle.tsx
git commit -m "feat(pane-resize-handle): CSS custom property resize + keyboard a11y"
```

---

## Task 9: Rewrite pane-node-renderer.tsx

**Files:**
- Rewrite: `web/src/features/panes/components/pane-node-renderer.tsx`

- [ ] **Step 1: Replace the entire file**

```tsx
import { memo, useCallback, useMemo, useRef } from 'react'
import { usePaneActions, usePanes } from '@/features/workspace/stores/hooks/use-pane-store'
import type { LayoutNode, PanePosition } from '../types/pane'
import { ROOT_PANE_POSITION } from '../types/pane'
import { flattenForRender, type FlatLayoutEntry } from '../utils/pane-layout'
import { PaneContainer } from './pane-container'
import { PaneResizeHandle } from './pane-resize-handle'
import { PaneBoundary } from './pane-boundary'

interface PaneNodeRendererProps {
  node: LayoutNode
  hiddenPaneId?: string | null
  position?: PanePosition
}

interface FlatResizeHandleProps {
  direction: 'horizontal' | 'vertical'
  index: number
  entries: FlatLayoutEntry[]
  splitContainerRef: React.RefObject<HTMLElement | null>
  onReset: () => void
  onResize: (index: number, sizes: [number, number]) => void
}

const FlatResizeHandle = memo(function FlatResizeHandle({
  direction,
  index,
  entries,
  splitContainerRef,
  onReset,
  onResize,
}: FlatResizeHandleProps) {
  const handleResize = useCallback(
    (sizes: [number, number]) => onResize(index, sizes),
    [index, onResize],
  )
  return (
    <PaneResizeHandle
      direction={direction}
      index={index}
      initialSizes={[entries[index].size, entries[index + 1].size]}
      splitContainerRef={splitContainerRef}
      onResize={handleResize}
      onReset={onReset}
    />
  )
})

function childPosition(
  parent: PanePosition,
  index: number,
  total: number,
  direction: 'horizontal' | 'vertical',
): PanePosition {
  const isFirst = index === 0
  const isLast = index === total - 1
  if (direction === 'horizontal') {
    return { atLeft: isFirst && parent.atLeft, atTop: parent.atTop, atRight: isLast && parent.atRight, atBottom: parent.atBottom }
  }
  return { atLeft: parent.atLeft, atTop: isFirst && parent.atTop, atRight: parent.atRight, atBottom: isLast && parent.atBottom }
}

export function PaneNodeRenderer({
  node,
  hiddenPaneId = null,
  position = ROOT_PANE_POSITION,
}: PaneNodeRendererProps) {
  const panes = usePanes()
  const { distributePaneSplit, resizePaneSplit } = usePaneActions()
  const splitContainerRef = useRef<HTMLElement | null>(null)

  const flatEntries = useMemo(() => {
    if (node.type !== 'split') return null
    return flattenForRender(node)
  }, [node])

  const handleFlatResize = useCallback(
    (index: number, sizes: [number, number]) => {
      if (node.type !== 'split') return
      resizePaneSplit(node.id, index, sizes)
    },
    [node, resizePaneSplit],
  )

  const handleFlatReset = useCallback(() => {
    if (node.type !== 'split') return
    distributePaneSplit(node.id)
  }, [distributePaneSplit, node])

  if (node.type === 'pane') {
    if (hiddenPaneId && node.id === hiddenPaneId) {
      return <div className="h-full w-full bg-background" aria-hidden="true" />
    }
    const pane = panes[node.id]
    if (!pane) return null
    return (
      <PaneBoundary paneId={node.id}>
        <PaneContainer pane={pane} position={position} />
      </PaneBoundary>
    )
  }

  if (!flatEntries || flatEntries.length === 0) return null

  const isHorizontal = node.direction === 'horizontal'
  const totalSize = flatEntries.reduce((sum, e) => sum + e.size, 0)
  const handleWidth = 4
  const handleCount = flatEntries.length - 1

  const cssVars = Object.fromEntries(
    flatEntries.map((entry, i) => [`--pane-${i}-size`, String((entry.size / totalSize) * 100)])
  )

  return (
    <div
      ref={splitContainerRef as React.RefObject<HTMLDivElement>}
      className={`flex h-full w-full ${isHorizontal ? 'flex-row' : 'flex-col'}`}
      style={cssVars as React.CSSProperties}
    >
      {flatEntries.map((entry, index) => {
        const handleDeduction = `${(handleWidth * handleCount) / flatEntries.length}px`
        const entryPosition = childPosition(position, index, flatEntries.length, node.direction)

        return (
          <div key={entry.node.id} className="contents">
            <div
              className="min-h-0 min-w-0 overflow-hidden"
              style={{
                [isHorizontal ? 'width' : 'height']:
                  `calc(var(--pane-${index}-size) * 1% - ${handleDeduction})`,
              }}
            >
              {entry.node.type === 'split' ? (
                <PaneNodeRenderer
                  node={entry.node}
                  hiddenPaneId={hiddenPaneId}
                  position={entryPosition}
                />
              ) : entry.node.id === hiddenPaneId ? (
                <div className="h-full w-full bg-background" aria-hidden="true" />
              ) : (() => {
                const pane = panes[entry.node.id]
                if (!pane) return null
                return (
                  <PaneBoundary paneId={entry.node.id}>
                    <PaneContainer pane={pane} position={entryPosition} />
                  </PaneBoundary>
                )
              })()}
            </div>
            {index < flatEntries.length - 1 && (
              <FlatResizeHandle
                direction={node.direction}
                index={index}
                entries={flatEntries}
                splitContainerRef={splitContainerRef}
                onReset={handleFlatReset}
                onResize={handleFlatResize}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-node-renderer" | head -10
```

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/components/pane-node-renderer.tsx
git commit -m "feat(pane-node-renderer): CSS custom property resize + flat panes map + PaneBoundary"
```

---

## Task 10: Update split-view-root.tsx

**Files:**
- Modify: `web/src/features/panes/components/split-view-root.tsx`

- [ ] **Step 1: Replace the file**

```tsx
import { useEffect, useMemo } from 'react'
import { IS_MAC } from '@/utils/platform'
import {
  useRootLayout,
  useBottomLayout,
  useFullscreenPaneId,
  usePaneActions,
  usePanes,
} from '@/features/workspace/stores/hooks/use-pane-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { PaneContainer } from './pane-container'
import { PaneNodeRenderer } from './pane-node-renderer'
import { PaneBoundary } from './pane-boundary'
import { ROOT_PANE_POSITION } from '../types/pane'

export function SplitViewRoot() {
  const rootLayout = useRootLayout()
  const bottomLayout = useBottomLayout()
  const fullscreenPaneId = useFullscreenPaneId()
  const panes = usePanes()
  const { exitPaneFullscreen } = usePaneActions()

  const fullscreenPane = useMemo(
    () => (fullscreenPaneId ? (panes[fullscreenPaneId] ?? null) : null),
    [fullscreenPaneId, panes],
  )

  useEffect(() => {
    if (fullscreenPaneId && !fullscreenPane) exitPaneFullscreen()
  }, [exitPaneFullscreen, fullscreenPane, fullscreenPaneId])

  const isBottomPaneVisible = useUIState(state => state.isBottomPaneVisible)
  const rootPosition = useMemo(
    () => ({ ...ROOT_PANE_POSITION, atBottom: !isBottomPaneVisible }),
    [isBottomPaneVisible],
  )

  const titleBarHeight = IS_MAC ? 44 : 28
  const footerHeight = 32

  return (
    <>
      <div className="h-full w-full overflow-hidden">
        <PaneNodeRenderer node={rootLayout} hiddenPaneId={fullscreenPaneId} position={rootPosition} />
      </div>

      {fullscreenPane && (
        <div
          className="fixed inset-x-2 z-[10040]"
          style={{ top: `${titleBarHeight + 8}px`, bottom: `${footerHeight + 8}px` }}
        >
          <div className="h-full overflow-hidden rounded-xl border border-border/80 bg-background shadow-2xl">
            <PaneBoundary paneId={fullscreenPane.id}>
              <PaneContainer pane={fullscreenPane} />
            </PaneBoundary>
          </div>
        </div>
      )}
    </>
  )
}
```

- [ ] **Step 2: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "split-view-root" | head -10
```

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/panes/components/split-view-root.tsx
git commit -m "feat(split-view-root): use normalized layout hooks (useRootLayout, usePanes)"
```

---

## Task 11: Update tab-bar.tsx

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`

The tab-bar currently reads `usePaneRoot()` and `useBottomRoot()` and calls `findPaneGroup` from `pane-tree`. Replace with `usePaneById` and direct flat map access.

- [ ] **Step 1: Update imports**

Remove `usePaneRoot, useBottomRoot` from the `use-pane-store` import. Add `usePaneById`:

```ts
import {
  usePaneById,
  usePaneActions,
} from '@/features/workspace/stores/hooks/use-pane-store'
```

Remove the import of `findPaneGroup` from `pane-tree`.

- [ ] **Step 2: Replace pane lookup**

Find the section that reads:
```ts
const paneRoot = usePaneRoot();
const bottomRoot = useBottomRoot();
...
const pane = paneId
  ? paneId === BOTTOM_PANE_ID
    ? findPaneGroup(bottomRoot, BOTTOM_PANE_ID)
    : findPaneGroup(paneRoot, paneId)
  : null;
```

Replace with:
```ts
const pane = usePaneById(paneId ?? '')
```

- [ ] **Step 3: Fix isInSplit**

Find `const isInSplit = paneRoot.type === "split";` and replace with:
```ts
const isInSplit = pane !== null && paneId !== null
```

(The tab-bar uses `isInSplit` to decide whether to show the close-pane button. A pane that exists with an ID is always in a potentially-split layout — this is a safe approximation. The more accurate version requires reading the layout tree which tab-bar shouldn't need to do.)

- [ ] **Step 4: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "tab-bar" | head -20
```

Fix any remaining TypeScript errors related to the pane lookup change.

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/tabs/components/tab-bar.tsx
git commit -m "feat(tab-bar): replace tree traversal with flat map pane lookup"
```

---

## Task 12: Update workspace-store-registry.ts

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store-registry.ts`

Add `getActiveWorkspaceStore` / `setActiveWorkspaceId` for imperative callers outside React. Update the persistence save call to use the new field names.

- [ ] **Step 1: Replace the file**

```ts
import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'

const registry = new Map<string, WorkspaceStore>()
const persistTimers = new Map<string, ReturnType<typeof setTimeout>>()

let _activeWorkspaceId: string | null = null

export function setActiveWorkspaceId(wsId: string): void {
  _activeWorkspaceId = wsId
}

export function getActiveWorkspaceStore(): WorkspaceStore | null {
  if (!_activeWorkspaceId) return null
  return registry.get(_activeWorkspaceId) ?? null
}

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    const snapshot = loadFromLocalStorage(wsId)
    const store = createWorkspaceStore(wsId, snapshot === null ? undefined : snapshot)

    store.subscribe((state) => {
      const existing = persistTimers.get(wsId)
      if (existing !== undefined) clearTimeout(existing)
      const timer = setTimeout(() => {
        persistTimers.delete(wsId)
        saveWorkspaceLayout({
          workspaceId: wsId,
          panes: state.panes,
          rootLayout: state.rootLayout,
          bottomLayout: state.bottomLayout,
          activePaneId: state.activePaneId,
          sidebarWidth: 0,
          rightSidebarWidth: 0,
          updatedAt: Date.now(),
        })
      }, 300)
      persistTimers.set(wsId, timer)
    })

    registry.set(wsId, store)
  }
  return registry.get(wsId)!
}

export function destroyWorkspaceStore(wsId: string): void {
  const existing = persistTimers.get(wsId)
  if (existing !== undefined) {
    clearTimeout(existing)
    persistTimers.delete(wsId)
  }
  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
```

- [ ] **Step 2: Wire setActiveWorkspaceId**

Find where `WorkspaceStoreContext.Provider` is rendered in the app (likely `features/workspace/components/WorkspaceProvider.tsx` or similar). Add a `useEffect` that calls `setActiveWorkspaceId(wsId)` when the workspace mounts:

```ts
import { setActiveWorkspaceId } from '../stores/workspace-store-registry'

// Inside the provider component, after creating/getting the store:
useEffect(() => {
  setActiveWorkspaceId(wsId)
}, [wsId])
```

- [ ] **Step 3: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "workspace-store-registry\|WorkspaceProvider" | head -20
```

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/workspace/stores/workspace-store-registry.ts
git commit -m "feat(workspace-registry): add getActiveWorkspaceStore + normalized persistence"
```

---

## Task 13: Migrate 4 utility files from legacy store

**Files:**
- Modify: `web/src/features/panes/utils/pane-command-actions.ts`
- Modify: `web/src/features/panes/utils/pane-drop-actions.ts`
- Modify: `web/src/features/panes/utils/pane-buffer-actions.ts`
- Modify: `web/src/features/panes/utils/pane-split-actions.ts`

All four files currently import from `../stores/pane-store`. Replace with `getActiveWorkspaceStore`.

- [ ] **Step 1: Update pane-split-actions.ts**

Replace the file contents — it's typically just a few lines that wrap `usePaneStore.getState().actions.splitPane`:

```ts
import { getActiveWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { SplitDirection, SplitPlacement } from '../types/pane'

export function splitEditorPane(
  paneId: string,
  direction: SplitDirection,
  bufferId?: string,
  placement?: SplitPlacement,
): string | null {
  return getActiveWorkspaceStore()?.getState().paneActions.splitPane(paneId, direction, bufferId, placement) ?? null
}
```

- [ ] **Step 2: Update pane-drop-actions.ts**

Read the current file first with Read tool, then replace all `usePaneStore` references with `getActiveWorkspaceStore()?.getState().paneActions`:

```ts
import { getActiveWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
```

Replace every `usePaneStore.getState().actions.X(...)` with `getActiveWorkspaceStore()?.getState().paneActions.X(...)`.

- [ ] **Step 3: Update pane-buffer-actions.ts**

Same pattern as above — replace `usePaneStore` with `getActiveWorkspaceStore()?.getState().paneActions`.

- [ ] **Step 4: Update pane-command-actions.ts**

This file uses `usePaneStore.getState().root` and `.bottomRoot` for tree traversal. After normalization, replace with flat map access:

- `usePaneStore.getState().root` → `getActiveWorkspaceStore()?.getState().rootLayout`
- `usePaneStore.getState().bottomRoot` → `getActiveWorkspaceStore()?.getState().bottomLayout`
- `getAllPaneGroups(state.root)` → `Object.values(getActiveWorkspaceStore()?.getState().panes ?? {})`
- `usePaneStore.getState().actions.*` → `getActiveWorkspaceStore()?.getState().paneActions.*`

- [ ] **Step 5: Typecheck all four**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "pane-command-actions\|pane-drop-actions\|pane-buffer-actions\|pane-split-actions" | head -20
```

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/panes/utils/pane-command-actions.ts \
  src/features/panes/utils/pane-drop-actions.ts \
  src/features/panes/utils/pane-buffer-actions.ts \
  src/features/panes/utils/pane-split-actions.ts
git commit -m "feat(pane-utils): migrate all utility files from legacy store to workspace store"
```

---

## Task 14: Update persistence schema and IndexDB version

**Files:**
- Modify: `web/src/lib/persistence/schemas.ts`
- Modify: `web/src/lib/persistence/idb.ts`

- [ ] **Step 1: Update schemas.ts**

Replace the `WorkspaceLayout` interface:

```ts
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'

export interface WorkspaceLayout {
  workspaceId: string
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  sidebarWidth: number
  rightSidebarWidth: number
  updatedAt: number
}
```

- [ ] **Step 2: Bump IDB version to 2 in idb.ts**

In `web/src/lib/persistence/idb.ts`, change the `openDB` call:

```ts
_db = await openDB<CrowbarDB>('crowbar', 2, {
  upgrade(db, oldVersion) {
    if (oldVersion < 1) {
      db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
      const editorStore = db.createObjectStore('editor-state', {
        keyPath: ['workspaceId', 'bufferId'],
      })
      editorStore.createIndex('workspaceId', 'workspaceId')
      db.createObjectStore('ui-preferences')
      db.createObjectStore('query-cache')
    }
    if (oldVersion < 2) {
      db.deleteObjectStore('workspace-layout')
      db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
    }
  },
})
```

- [ ] **Step 3: Typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "schemas\|idb\b" | head -20
```

- [ ] **Step 4: Commit**

```bash
cd web && git add src/lib/persistence/schemas.ts src/lib/persistence/idb.ts
git commit -m "feat(persistence): update WorkspaceLayout schema + IDB v2 migration"
```

---

## Task 15: Delete legacy files

**Files:**
- Delete: `web/src/features/panes/stores/pane-store.ts`
- Delete: `web/src/features/panes/utils/pane-tree.ts`
- Modify: `web/src/features/panes/types/pane.ts` (remove `PaneSplit`, `PaneNode`)

- [ ] **Step 1: Delete the files**

```bash
rm web/src/features/panes/stores/pane-store.ts
rm web/src/features/panes/utils/pane-tree.ts
```

- [ ] **Step 2: Remove retired types from pane.ts**

In `web/src/features/panes/types/pane.ts`, remove the `PaneSplit` interface, the `PaneNode` type alias, `SplitDirection`, and `SplitPlacement` **only if** they are no longer referenced anywhere. Check first:

```bash
cd web && grep -rn "PaneSplit\|PaneNode\b\|SplitDirection\|SplitPlacement" src/ --include="*.ts" --include="*.tsx" | grep -v "pane.ts$" | grep -v "pane-layout.ts" | grep -v "__tests__"
```

Remove any that show zero references. `SplitDirection` and `SplitPlacement` are used in `pane-slice.ts` exports so keep them. Remove only `PaneSplit` and `PaneNode`.

- [ ] **Step 3: Full typecheck**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -v "markdown-chat" | head -40
```

Expected: zero errors (the markdown-chat errors are pre-existing and unrelated to this refactor).

Fix any errors that appear before committing.

- [ ] **Step 4: Run all pane tests**

```bash
cd web && npx vitest run src/__tests__/features/panes/
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd web && git add -A
git commit -m "feat(pane-system): delete legacy pane-store.ts and pane-tree.ts

Complete normalization: flat panes map, LayoutNode trees, CSS custom
property resize, keyboard a11y on handles, per-pane error boundaries,
IndexDB v2 persistence schema."
```

---

## Task 16: Final verification

- [ ] **Step 1: Full typecheck — zero errors in changed files**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -v "markdown-chat"
```

- [ ] **Step 2: Run full test suite**

```bash
cd web && npx vitest run
```

- [ ] **Step 3: Verify no remaining references to deleted files**

```bash
grep -rn "pane-store\|pane-tree\|usePaneRoot\|useBottomRoot\|paneRoot\b\|bottomRoot\b" \
  web/src/ --include="*.ts" --include="*.tsx" | grep -v ".test.ts" | grep -v "pane-layout.ts"
```

Expected: zero results.

- [ ] **Step 4: Verify success criteria from spec**

```bash
# Verify O(1) lookup shape exists in the slice
grep -n "panes\[paneId\]" web/src/features/workspace/stores/slices/pane-slice.ts | head -5

# Verify CSS custom property writes in resize handle
grep -n "style.setProperty" web/src/features/panes/components/pane-resize-handle.tsx

# Verify keyboard handler on resize handle
grep -n "onKeyDown\|ArrowLeft\|ArrowRight" web/src/features/panes/components/pane-resize-handle.tsx

# Verify PaneBoundary wraps PaneContainer in renderer
grep -n "PaneBoundary" web/src/features/panes/components/pane-node-renderer.tsx

# Verify IDB version is 2
grep -n "openDB.*crowbar.*2" web/src/lib/persistence/idb.ts
```

- [ ] **Step 5: Final commit if any cleanup needed**

```bash
cd web && git add -A && git commit -m "chore(pane-system): final cleanup and verification"
```
