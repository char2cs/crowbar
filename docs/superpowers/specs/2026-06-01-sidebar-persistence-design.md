# Sidebar Persistence — Design Spec

**Date:** 2026-06-01  
**Branch:** enhancement/design-language  
**Status:** Approved

---

## Problem

Two pieces of sidebar state are lost on page reload:

1. **Repo collapse state** (`collapsedRepos`) — which repos the user has collapsed in the sidebar.
2. **Workspace hierarchy** (`parentId` on each workspace) — the result of drag-and-drop reparenting operations.

Both must survive a full page reload. Workspace reparenting is architecturally a backend operation (even though no backend exists yet), so it must flow through a stubbed API layer with a clear seam for future replacement.

---

## Architecture

### Two distinct persistence concerns

| Concern | Source of truth | IDB role |
|---|---|---|
| Sidebar collapse state | Client (UI preference) | Source of truth |
| Workspace hierarchy | Backend (future) | Local cache of server state |

### Data flow

**Collapse toggle:**
```
user clicks → toggleRepo() → update store → saveSidebarUI() → IDB
```

**Reparent (drag-drop):**
```
user drops → reparentWorkspace() API stub → handleWorkspaceReparented() → saveWorkspaceHierarchy() + store.reparentWorkspace()
```
The `handleWorkspaceReparented` function is the notification handler seam. When a real backend + WebSocket/SSE arrives, the stub call is replaced with a real HTTP call, and the WS/SSE event fires `handleWorkspaceReparented` instead.

**Hydration (page load):**
```
HydrationGate → Promise.all([hydratePreferences(), hydrateSidebar()])
                                                    ↓
                                      loadSidebarUI() → set collapsedRepos
                                      loadAllWorkspaceHierarchies() → overlay parentId on repos
```

---

## IDB Schema — version 3

Two new stores added to `CrowbarDB` in `lib/persistence/idb.ts`:

### `sidebar-ui`
```ts
interface SidebarUI {
  collapsedRepos: string[]
  updatedAt: number
}
// key: string — single record under 'global'
```

### `workspace-hierarchy`
```ts
interface WorkspaceHierarchy {
  repoId: string
  entries: Array<{ wsId: string; parentId?: string }>
  updatedAt: number
}
// keyPath: 'repoId'
```

DB version bumps 2 → 3 with a new `oldVersion < 3` upgrade block.

---

## New Files

### `lib/persistence/sidebar-ui.ts`
- `saveSidebarUI(collapsedRepos: string[]): Promise<void>`
- `loadSidebarUI(): Promise<SidebarUI | null>`

### `lib/persistence/workspace-hierarchy.ts`
- `saveWorkspaceHierarchy(repoId: string, entries: Array<{wsId: string; parentId?: string}>): Promise<void>`
- `loadWorkspaceHierarchy(repoId: string): Promise<WorkspaceHierarchy | null>`
- `loadAllWorkspaceHierarchies(): Promise<WorkspaceHierarchy[]>`

### `lib/api/workspace.ts`
- `reparentWorkspace(wsId: string, newParentId: string | undefined, repoId: string): Promise<void>` — stub, resolves immediately then calls handler
- `handleWorkspaceReparented(wsId: string, newParentId: string | undefined, repoId: string): Promise<void>` — the notification handler seam; computes new entries from store, writes to IDB, updates store

---

## Modified Files

### `lib/persistence/schemas.ts`
Add `SidebarUI` and `WorkspaceHierarchy` interfaces. Add both stores to `CrowbarDB`.

### `lib/persistence/idb.ts`
Bump DB version to 3. Add `oldVersion < 3` block creating `sidebar-ui` and `workspace-hierarchy` stores.

### `lib/persistence/hydrate.ts`
Add `hydrateSidebar(): Promise<void>`:
1. Load `sidebar-ui` → `useSidebarStore.setState({ collapsedRepos: new Set(record.collapsedRepos) })`
2. Load all hierarchy records → for each, find matching repo in store and overlay `parentId` on each workspace

### `lib/store/sidebar.ts`
`toggleRepo` action: after updating `collapsedRepos`, fire-and-forget call to `saveSidebarUI([...next])`.

### `components/hydration-gate.tsx`
Replace single `hydratePreferences()` call with `Promise.all([hydratePreferences(), hydrateSidebar()])`.

### `components/layout/workspace-tree-context.tsx`
In the `onPointerUp` drop handler, replace direct `useSidebarStore.getState().reparentWorkspace(...)` calls with `reparentWorkspace(wsId, newParentId, repoId)` from `lib/api/workspace.ts` (fire-and-forget, no await needed in event handler).

---

## Testing

Mirror existing test structure under `web/src/__tests__/`:

- `__tests__/lib/persistence/sidebar-ui.test.ts` — save/load round-trip
- `__tests__/lib/persistence/workspace-hierarchy.test.ts` — save/load/loadAll round-trip
- `__tests__/lib/api/workspace.test.ts` — stub resolves, IDB written, store updated
- `__tests__/lib/persistence/hydrate.test.ts` — extend existing file with `hydrateSidebar` cases

All tests use `@/` imports. IDB tests follow the pattern established in `workspace-layout.test.ts`.

---

## Out of scope

- Real backend API or WebSocket integration
- Syncing hierarchy across tabs/devices
- Persisting workspace order within a repo (only `parentId` is persisted, not sort order)
