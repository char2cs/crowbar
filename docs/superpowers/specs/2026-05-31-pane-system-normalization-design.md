# Pane System Normalization — Design Spec

**Date:** 2026-05-31
**Branch:** enhancement/design-language
**Status:** Approved for implementation

---

## Problem

The pane system has three compounding issues that block production readiness:

1. **Mixed data and structure.** `PaneNode = PaneGroup | PaneSplit` embeds pane data inside the layout tree. Every buffer mutation (add, remove, pin, set active) requires a full immutable tree rewrite from root to leaf — O(n) where n is the number of panes.

2. **Duplicate stores.** Two stores exist in parallel: `features/panes/stores/pane-store.ts` (legacy singleton) and `workspace/stores/slices/pane-slice.ts` (workspace-scoped, what the UI actually reads). Utility files (`pane-command-actions`, `pane-drop-actions`, `pane-buffer-actions`, `pane-split-actions`) still write to the legacy store — changes are silently dropped from the UI. This is a live bug.

3. **Fragile resize.** The resize handle walks the DOM at drag start to find pane elements through `display:contents` wrappers. Any change to the DOM structure silently breaks resize.

---

## Goals

- O(1) pane lookups and buffer mutations
- Single source of truth (legacy store deleted)
- Resize with zero DOM traversal
- Keyboard accessibility on resize handles
- Per-pane error isolation
- Layout persistence to IndexDB (already wired, just needs correct shape)
- TypeScript-clean throughout, no `unknown[]` in persistence schema

---

## Type System

### Retired types

`PaneSplit` and `PaneNode` are retired. They conflated layout structure with pane data.

### New layout types (`features/panes/types/pane.ts`)

```ts
// Layout tree — structural only, contains no pane data
export type LayoutNode = LayoutLeaf | LayoutSplit

export interface LayoutLeaf {
  type: 'pane'
  id: string  // references panes[id] in the store
}

export interface LayoutSplit {
  type: 'split'
  id: string  // stable ID for resize targeting and React keys
  direction: 'horizontal' | 'vertical'
  sizes: [number, number]  // percentages, must sum to 100
  first: LayoutNode
  second: LayoutNode
}
```

Named `first`/`second` fields instead of `children[0]`/`children[1]` — eliminates array indexing at every callsite.

`PaneGroup` is unchanged. It is the canonical pane data type and requires no modification.

---

## Store Shape

### Before

```ts
// pane-slice.ts
paneRoot: PaneNode    // recursive — data and structure mixed
bottomRoot: PaneNode
```

### After

```ts
// pane-slice.ts
panes: Record<string, PaneGroup>  // flat map — O(1) lookup and mutation
rootLayout: LayoutNode             // structure only, no pane data
bottomLayout: LayoutNode
activePaneId: string
mostRecentActivePaneIds: string[]
fullscreenPaneId: string | null
paneActions: PaneActions
```

`panes` holds every `PaneGroup` from both trees in a single flat record. `rootLayout` and `bottomLayout` are pure structural trees containing only IDs and split metadata.

### Action interface changes

**Buffer operations** — no longer recursive tree rewrites:
```ts
addBufferToPane(paneId, bufferId, setActive?): void
  // before: addBufferToPane(tree, paneId, bufferId) — O(n) tree rewrite
  // after:  state.panes[paneId].bufferIds.push(bufferId) — O(1)

removeBufferFromPane(paneId, bufferId, preserveEmptyPane?): void
activatePaneBuffer(paneId, bufferId): void
setPanePreviewBuffer(paneId, bufferId): void
setPaneBufferPinned(paneId, bufferId, pinned): void
reorderPaneBuffers(paneId, startIndex, endIndex): void
```

**Layout operations** — still traverse `LayoutNode` tree, but the tree is lightweight (IDs + sizes only):
```ts
splitPane(paneId, direction, bufferId?, placement?): string | null
closePane(paneId): void
resizePaneSplit(splitId, sizes: [number, number]): void  // no index param — binary splits are unambiguous
distributePaneSplit(splitId): void
```

Note: `resizePaneSplit` drops the `index` parameter from the current API. Binary splits always resize `[first%, second%]` with no ambiguity. The `splitId` identifies which split node.

---

## File Plan

### Deleted

| File | Reason |
|---|---|
| `features/panes/stores/pane-store.ts` | Legacy singleton — replaced by workspace slice |
| `features/panes/utils/pane-tree.ts` | Data-embedded-in-tree ops — replaced by pane-layout.ts |

### New

| File | Purpose |
|---|---|
| `features/panes/utils/pane-layout.ts` | Layout-structural ops on `LayoutNode`. ~200 lines. |
| `features/panes/components/pane-boundary.tsx` | Per-pane error boundary |

### Modified (16 files)

**Core store:**
- `features/panes/types/pane.ts` — add `LayoutNode`, `LayoutLeaf`, `LayoutSplit`; retire `PaneSplit`, `PaneNode`
- `features/workspace/stores/slices/pane-slice.ts` — new state shape, all actions rewritten
- `features/workspace/stores/workspace-store.types.ts` — `WorkspaceState` updated
- `features/workspace/stores/workspace-store.ts` — `WorkspaceSnapshot` updated
- `features/workspace/stores/hooks/use-pane-store.ts` — hooks updated for flat map

**Registry and persistence:**
- `features/workspace/stores/workspace-store-registry.ts` — adds `getActiveWorkspaceStore()` for imperative callers; updates `saveWorkspaceLayout` call shape
- `features/workspace/stores/workspace-persistence.ts` — snapshot type updated
- `lib/persistence/schemas.ts` — `WorkspaceLayout` fields updated (see Persistence section)
- `lib/persistence/idb.ts` — version bump to 2, migration block

**Renderer:**
- `features/panes/components/pane-node-renderer.tsx` — CSS custom properties; reads from flat `panes` map; passes `splitContainerRef` to handles; wraps leaves in `PaneBoundary`
- `features/panes/components/pane-resize-handle.tsx` — CSS var writes; keyboard handler; receives `splitContainerRef`
- `features/panes/components/split-view-root.tsx` — hook updates

**Migrated utility files (legacy store → workspace store):**
- `features/panes/utils/pane-command-actions.ts`
- `features/panes/utils/pane-drop-actions.ts`
- `features/panes/utils/pane-buffer-actions.ts`
- `features/panes/utils/pane-split-actions.ts`

**Consumers:**
- `features/tabs/components/tab-bar.tsx` — replace `usePaneRoot`/`useBottomRoot` + `findPaneGroup` with `getPaneById` from flat map

---

## pane-layout.ts

Replaces `pane-tree.ts`. Works exclusively on `LayoutNode` — no `PaneGroup` data, no buffer operations.

**Functions:**

```ts
createLeaf(id: string): LayoutLeaf
createSplit(direction, first, second, sizes?): LayoutSplit
splitLayout(root, paneId, direction, placement): { layout: LayoutNode; newPaneId: string } | null
closeLayout(root, paneId): LayoutNode | null
updateSplitSizes(root, splitId, sizes): LayoutNode
distributeSplit(root, splitId): LayoutNode  // equalize all sibling sizes
normalizeLayout(root): LayoutNode           // collapse degenerate single-child splits
findLeaf(root, paneId): LayoutLeaf | null
findSplit(root, splitId): LayoutSplit | null
findParentSplit(root, nodeId): LayoutSplit | null
getAllLeafIds(root): string[]
getFirstLeafId(root): string
getAdjacentLeafId(root, paneId, direction): string | null
flattenForRender(root, splitId): FlatLayoutEntry[]  // same-direction flatten for renderer
```

`flattenForRender` replaces `flattenPaneSplit`. It flattens same-direction nested splits into a linear array for the renderer's flex layout, same behavior as today.

---

## Resize: CSS Custom Properties

### Why this eliminates DOM traversal

Currently the handle walks the DOM at drag start:
```ts
firstPaneElRef.current = handle.previousElementSibling  // fragile
const nextContents = handle.parentElement?.nextElementSibling
secondPaneElRef.current = nextContents?.firstElementChild  // more fragile
```

With CSS custom properties, the handle only needs the split container — passed as a ref from the renderer.

### Renderer side

The split container div receives CSS vars for each pane size:

```tsx
const splitContainerRef = useRef<HTMLDivElement>(null)

<div
  ref={splitContainerRef}
  className={`flex h-full w-full ${isHorizontal ? 'flex-row' : 'flex-col'}`}
  style={Object.fromEntries(
    flatEntries.map((entry, i) => [`--pane-${i}-size`, entry.size])
  )}
>
  {flatEntries.map((entry, i) => (
    <div key={entry.id} className="contents">
      <div
        className="min-h-0 min-w-0 overflow-hidden"
        style={{
          [isHorizontal ? 'width' : 'height']:
            `calc(var(--pane-${i}-size) * 1% - ${handleDeduction})`,
        }}
      />
      {i < flatEntries.length - 1 && (
        <FlatResizeHandle
          direction={node.direction}
          index={i}
          initialSizes={[flatEntries[i].size, flatEntries[i + 1].size]}
          splitContainerRef={splitContainerRef}
          onResize={handleFlatResize}
          onReset={handleFlatReset}
        />
      )}
    </div>
  ))}
</div>
```

### Handle side (pane-resize-handle.tsx)

**On mousedown:** measure container size once from `splitContainerRef.current.getBoundingClientRect()`.

**During drag (rAF):** write CSS vars on the container:
```ts
splitContainerRef.current.style.setProperty(`--pane-${index}-size`, String(newFirst))
splitContainerRef.current.style.setProperty(`--pane-${index + 1}-size`, String(newSecond))
```

**On mouseup:** call `onResize([newFirst, newSecond])` once. Store commits. React re-renders with new sizes baked into the layout node. CSS vars on the container are cleared (React's inline style takes over).

No `firstPaneElRef`, no `secondPaneElRef`, no DOM traversal. The handle is self-contained.

### Props interface

```ts
interface PaneResizeHandleProps {
  direction: 'horizontal' | 'vertical'
  index: number                                  // which gap in the flat array
  initialSizes: [number, number]                 // percentage sizes at render time
  splitContainerRef: React.RefObject<HTMLElement> // provided by renderer
  onResize: (sizes: [number, number]) => void
  onReset?: () => void
}
```

---

## Keyboard Resize

`role="separator"` divs are required by ARIA to support keyboard interaction when they are focusable and interactive. This is not optional — it is the spec.

```tsx
onKeyDown={(e) => {
  const isRelevant = isHorizontal
    ? e.key === 'ArrowLeft' || e.key === 'ArrowRight'
    : e.key === 'ArrowUp' || e.key === 'ArrowDown'
  if (!isRelevant) return
  e.preventDefault()

  const step = e.shiftKey ? 10 : 2  // % — Shift for coarse, plain for fine
  const [first, second] = initialSizes
  const total = first + second
  const isPositive = e.key === 'ArrowRight' || e.key === 'ArrowDown'
  const delta = isPositive ? step : -step
  const newFirst = Math.max(MIN_PANE_SIZE, Math.min(total - MIN_PANE_SIZE, first + delta))
  onResize([newFirst, total - newFirst])
}}
```

`tabIndex={0}` on the handle div. Focus ring styled via the existing `group-focus-visible` Tailwind pattern.

---

## Error Boundary

`PaneBoundary` is a React class component (error boundaries cannot be function components).

```tsx
// pane-boundary.tsx
interface State { hasError: boolean; error: Error | null }

class PaneBoundary extends React.Component<
  { paneId: string; children: React.ReactNode },
  State
> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidUpdate(prevProps: { paneId: string }) {
    // Reset when the pane itself changes (e.g., navigating to a different pane context)
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
```

`PaneErrorState` is a simple functional component showing the error message and a retry button. Styled consistently with the app's empty states.

In `pane-node-renderer.tsx`, every `<PaneContainer>` usage is wrapped:

```tsx
<PaneBoundary paneId={entry.id}>
  <PaneContainer pane={panes[entry.id]} position={entryPosition} />
</PaneBoundary>
```

Note: `panes[entry.id]` — the renderer now reads pane data from the flat map, not from the tree node.

---

## Legacy Store Migration

### Problem

`pane-command-actions.ts`, `pane-drop-actions.ts`, `pane-buffer-actions.ts`, `pane-split-actions.ts` all call `usePaneStore.getState()` — the legacy singleton. This is an imperative pattern (called from keybinding handlers, not React components), so `useWorkspaceStoreContext` (hook-based) is not available.

### Solution

`workspace-store-registry.ts` exports a new function:

```ts
let _activeWorkspaceId: string | null = null

export function setActiveWorkspaceId(wsId: string): void {
  _activeWorkspaceId = wsId
}

export function getActiveWorkspaceStore(): WorkspaceStore | null {
  if (!_activeWorkspaceId) return null
  return registry.get(_activeWorkspaceId) ?? null
}
```

`setActiveWorkspaceId` is called from the workspace context provider when the active workspace changes.

All four utility files replace `usePaneStore.getState().actions.*` with:
```ts
getActiveWorkspaceStore()?.getState().paneActions.*
```

---

## Persistence

### IndexDB schema update (version 2)

```ts
// lib/persistence/schemas.ts
export interface WorkspaceLayout {
  workspaceId: string
  panes: Record<string, PaneGroup>  // was: unknown[]
  rootLayout: LayoutNode             // replaces panes: unknown[]
  bottomLayout: LayoutNode           // replaces tabGroups: unknown[]
  activePaneId: string               // was: activePane
  sidebarWidth: number
  rightSidebarWidth: number
  updatedAt: number
}
```

### IDB version bump

```ts
// lib/persistence/idb.ts
_db = await openDB<CrowbarDB>('crowbar', 2, {
  upgrade(db, oldVersion) {
    if (oldVersion < 1) { /* existing stores */ }
    if (oldVersion < 2) {
      // Schema shape changed — simplest migration is a clean slate.
      // Users lose their pane layout on first upgrade; all other data is preserved.
      db.deleteObjectStore('workspace-layout')
      db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
    }
  },
})
```

Clean-slate migration is the right call here. The pane layout is a UI convenience, not user data. Editor state, preferences, and query cache are all unaffected (separate object stores). The worst outcome is the user's split layout resets once on upgrade.

### WorkspaceSnapshot (localStorage fast-restore)

```ts
// workspace-store.ts
export interface WorkspaceSnapshot {
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  buffers: PaneContent[]
}
```

The registry save call in `workspace-store-registry.ts`:

```ts
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
```

---

## Implementation Order

The changes are interdependent. This order minimizes broken intermediate states:

1. **Types** — `pane.ts`: add `LayoutNode`, `LayoutLeaf`, `LayoutSplit`. Keep `PaneSplit`/`PaneNode` as deprecated aliases until all consumers are updated.
2. **`pane-layout.ts`** — new file, no consumers yet. Write and test in isolation.
3. **`pane-slice.ts`** — rewrite with new state shape. This is the largest single change.
4. **`workspace-store.types.ts` + `workspace-store.ts`** — update `WorkspaceState` and `WorkspaceSnapshot`.
5. **`use-pane-store.ts` hooks** — update to read from flat map.
6. **`pane-node-renderer.tsx`** — CSS custom properties, read from `panes` map, wrap in `PaneBoundary`.
7. **`pane-resize-handle.tsx`** — container ref, CSS var writes, keyboard handler.
8. **`pane-boundary.tsx`** — new component.
9. **`split-view-root.tsx`** — minor hook updates.
10. **`tab-bar.tsx`** — replace tree traversal with flat map lookup.
11. **Utility files** (4) — migrate from legacy store to `getActiveWorkspaceStore()`.
12. **`workspace-store-registry.ts`** — add `getActiveWorkspaceStore()`, `setActiveWorkspaceId()`, update persistence call.
13. **`lib/persistence/schemas.ts` + `idb.ts`** — schema update + version bump.
14. **`workspace-persistence.ts`** — snapshot type update.
15. **Delete** `pane-store.ts` and `pane-tree.ts`.
16. **Remove deprecated aliases** from `pane.ts`.

---

## Success Criteria

- `getPaneById(id)` is `state.panes[id]` — one line, O(1)
- Adding a buffer to a pane touches exactly one object in the store
- Dragging a resize handle writes only CSS custom properties until mouseup
- Arrow keys move resize handles by 2% (10% with Shift)
- A buffer crash renders a retry UI without affecting other panes
- Pane layout survives a page reload (persisted to IndexDB)
- Zero references to `pane-store.ts` or `pane-tree.ts` remain
- TypeScript reports zero errors in changed files
