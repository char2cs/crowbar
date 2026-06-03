# Sidebar Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist sidebar repo collapse state and workspace hierarchy (parentId) to IndexedDB so both survive page reloads.

**Architecture:** Collapse state is a local UI preference written directly to IDB on every toggle. Workspace reparenting flows through a stub API (`lib/api/workspace.ts`) that simulates a backend call, then a notification handler writes to IDB and updates the store — the seam where a real backend + WebSocket would slot in. Both are hydrated at app start inside `HydrationGate` via `Promise.all`.

**Tech Stack:** Vitest, `fake-indexeddb` (IDB mocking in tests), `idb` library, Zustand

**Spec:** `docs/superpowers/specs/2026-06-01-sidebar-persistence-design.md`

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| Modify | `web/src/lib/persistence/schemas.ts` | Add `SidebarUI`, `WorkspaceHierarchy` types + stores to `CrowbarDB` |
| Modify | `web/src/lib/persistence/idb.ts` | Bump DB to version 3, add two new object stores |
| Create | `web/src/lib/persistence/sidebar-ui.ts` | `saveSidebarUI` / `loadSidebarUI` |
| Create | `web/src/lib/persistence/workspace-hierarchy.ts` | `saveWorkspaceHierarchy` / `loadWorkspaceHierarchy` / `loadAllWorkspaceHierarchies` |
| Create | `web/src/lib/api/workspace.ts` | Stub `reparentWorkspace` + `handleWorkspaceReparented` notification handler |
| Modify | `web/src/lib/store/sidebar.ts` | `toggleRepo` fires-and-forgets `saveSidebarUI` after updating store |
| Modify | `web/src/lib/persistence/hydrate.ts` | Add `hydrateSidebar()` |
| Modify | `web/src/components/hydration-gate.tsx` | Run `hydrateSidebar()` in parallel with `hydratePreferences()` |
| Modify | `web/src/components/layout/workspace-tree-context.tsx` | Replace direct store reparent calls with `reparentWorkspace` API call |
| Create | `web/src/__tests__/lib/persistence/sidebar-ui.test.ts` | Round-trip tests |
| Create | `web/src/__tests__/lib/persistence/workspace-hierarchy.test.ts` | Round-trip tests |
| Create | `web/src/__tests__/lib/api/workspace.test.ts` | Stub + handler integration |
| Modify | `web/src/__tests__/lib/persistence/hydrate.test.ts` | Add `hydrateSidebar` cases |

---

## Task 1: Schema additions and IDB version bump

**Files:**
- Modify: `web/src/lib/persistence/schemas.ts`
- Modify: `web/src/lib/persistence/idb.ts`

- [ ] **Step 1: Add new interfaces and store entries to schemas.ts**

Replace the contents of `web/src/lib/persistence/schemas.ts` with:

```ts
import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'

export interface WorkspaceLayout {
  workspaceId: string
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  buffers: PaneContent[]
  sidebarWidth: number
  rightSidebarWidth: number
  updatedAt: number
}

export interface EditorState {
  workspaceId: string
  bufferId: string
  cursorLine: number
  cursorColumn: number
  scrollTop: number
  folds: [number, number][]
  updatedAt: number
}

export interface UIPreferences {
  theme: string
  fontSize: number
  fontFamily: string
  tabSize: number
  wordWrap: boolean
  minimap: boolean
  updatedAt: number
}

export interface SidebarUI {
  collapsedRepos: string[]
  updatedAt: number
}

export interface WorkspaceHierarchy {
  repoId: string
  entries: Array<{ wsId: string; parentId?: string }>
  updatedAt: number
}

export interface CrowbarDB extends DBSchema {
  'workspace-layout': {
    key: string
    value: WorkspaceLayout
  }
  'editor-state': {
    key: [string, string]
    value: EditorState
    indexes: { workspaceId: string }
  }
  'ui-preferences': {
    key: string
    value: UIPreferences
  }
  'query-cache': {
    key: string
    value: string
  }
  'sidebar-ui': {
    key: string
    value: SidebarUI
  }
  'workspace-hierarchy': {
    key: string
    value: WorkspaceHierarchy
  }
}
```

- [ ] **Step 2: Bump DB version and add upgrade block in idb.ts**

Replace the contents of `web/src/lib/persistence/idb.ts` with:

```ts
import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _db = await openDB<CrowbarDB>('crowbar', 3, {
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
      if (oldVersion < 3) {
        db.createObjectStore('sidebar-ui')
        db.createObjectStore('workspace-hierarchy', { keyPath: 'repoId' })
      }
    },
  })
  return _db
}

/** Only for testing — resets the module-level singleton so tests get a fresh database. */
export function resetDB(): void {
  _db = null
}
```

- [ ] **Step 3: Commit**

```bash
cd web
git add src/lib/persistence/schemas.ts src/lib/persistence/idb.ts
git commit -m "feat(persistence): add sidebar-ui and workspace-hierarchy IDB stores (v3)"
```

---

## Task 2: sidebar-ui persistence (TDD)

**Files:**
- Create: `web/src/lib/persistence/sidebar-ui.ts`
- Create: `web/src/__tests__/lib/persistence/sidebar-ui.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/persistence/sidebar-ui.test.ts`:

```ts
import { saveSidebarUI, loadSidebarUI } from '@/lib/persistence/sidebar-ui'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('sidebar-ui persistence', () => {
  it('returns null when nothing is stored', async () => {
    const result = await loadSidebarUI()
    expect(result).toBeNull()
  })

  it('saves and loads collapsedRepos round-trip', async () => {
    await saveSidebarUI(['crowbar', 'quiver-core'])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['crowbar', 'quiver-core'])
  })

  it('overwrites previous record on second save', async () => {
    await saveSidebarUI(['crowbar'])
    await saveSidebarUI(['quiver-core'])
    const result = await loadSidebarUI()
    expect(result?.collapsedRepos).toEqual(['quiver-core'])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/sidebar-ui.test.ts
```

Expected: FAIL — `Cannot find module '@/lib/persistence/sidebar-ui'`

- [ ] **Step 3: Implement sidebar-ui.ts**

Create `web/src/lib/persistence/sidebar-ui.ts`:

```ts
import { getDB } from './idb'
import type { SidebarUI } from './schemas'

export async function saveSidebarUI(collapsedRepos: string[]): Promise<void> {
  const db = await getDB()
  await db.put('sidebar-ui', { collapsedRepos, updatedAt: Date.now() }, 'global')
}

export async function loadSidebarUI(): Promise<SidebarUI | null> {
  const db = await getDB()
  return (await db.get('sidebar-ui', 'global')) ?? null
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/sidebar-ui.test.ts
```

Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
cd web
git add src/lib/persistence/sidebar-ui.ts src/__tests__/lib/persistence/sidebar-ui.test.ts
git commit -m "feat(persistence): add sidebar-ui save/load"
```

---

## Task 3: workspace-hierarchy persistence (TDD)

**Files:**
- Create: `web/src/lib/persistence/workspace-hierarchy.ts`
- Create: `web/src/__tests__/lib/persistence/workspace-hierarchy.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/persistence/workspace-hierarchy.test.ts`:

```ts
import {
  saveWorkspaceHierarchy,
  loadWorkspaceHierarchy,
  loadAllWorkspaceHierarchies,
} from '@/lib/persistence/workspace-hierarchy'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('workspace-hierarchy persistence', () => {
  it('returns null for unknown repoId', async () => {
    const result = await loadWorkspaceHierarchy('nope')
    expect(result).toBeNull()
  })

  it('saves and loads entries round-trip', async () => {
    const entries = [
      { wsId: 'ws1', parentId: 'ws-develop' },
      { wsId: 'ws2' },
    ]
    await saveWorkspaceHierarchy('crowbar', entries)
    const result = await loadWorkspaceHierarchy('crowbar')
    expect(result?.repoId).toBe('crowbar')
    expect(result?.entries).toEqual(entries)
  })

  it('overwrites previous record for same repoId', async () => {
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1', parentId: 'ws-develop' }])
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1' }])
    const result = await loadWorkspaceHierarchy('crowbar')
    expect(result?.entries[0].parentId).toBeUndefined()
  })

  it('loadAllWorkspaceHierarchies returns all stored records', async () => {
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1' }])
    await saveWorkspaceHierarchy('quiver-core', [{ wsId: 'qc1', parentId: 'qc-develop' }])
    const all = await loadAllWorkspaceHierarchies()
    expect(all).toHaveLength(2)
    const repoIds = all.map(h => h.repoId)
    expect(repoIds).toContain('crowbar')
    expect(repoIds).toContain('quiver-core')
  })

  it('loadAllWorkspaceHierarchies returns empty array when nothing stored', async () => {
    const all = await loadAllWorkspaceHierarchies()
    expect(all).toEqual([])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/workspace-hierarchy.test.ts
```

Expected: FAIL — `Cannot find module '@/lib/persistence/workspace-hierarchy'`

- [ ] **Step 3: Implement workspace-hierarchy.ts**

Create `web/src/lib/persistence/workspace-hierarchy.ts`:

```ts
import { getDB } from './idb'
import type { WorkspaceHierarchy } from './schemas'

export async function saveWorkspaceHierarchy(
  repoId: string,
  entries: Array<{ wsId: string; parentId?: string }>,
): Promise<void> {
  const db = await getDB()
  await db.put('workspace-hierarchy', { repoId, entries, updatedAt: Date.now() })
}

export async function loadWorkspaceHierarchy(repoId: string): Promise<WorkspaceHierarchy | null> {
  const db = await getDB()
  return (await db.get('workspace-hierarchy', repoId)) ?? null
}

export async function loadAllWorkspaceHierarchies(): Promise<WorkspaceHierarchy[]> {
  const db = await getDB()
  return db.getAll('workspace-hierarchy')
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/workspace-hierarchy.test.ts
```

Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
cd web
git add src/lib/persistence/workspace-hierarchy.ts src/__tests__/lib/persistence/workspace-hierarchy.test.ts
git commit -m "feat(persistence): add workspace-hierarchy save/load"
```

---

## Task 4: Stub API and notification handler (TDD)

**Files:**
- Create: `web/src/lib/api/workspace.ts`
- Create: `web/src/__tests__/lib/api/workspace.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/api/workspace.test.ts`:

```ts
import { reparentWorkspace, handleWorkspaceReparented } from '@/lib/api/workspace'
import { loadWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useSidebarStore.setState((useSidebarStore as any).getInitialState())
})

describe('handleWorkspaceReparented', () => {
  it('updates the sidebar store parentId', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    const ws = repo.workspaces.find(w => w.id === 'ws3')!
    expect(ws.parentId).toBe('ws-develop')
  })

  it('writes hierarchy to IDB after updating store', async () => {
    await handleWorkspaceReparented('ws3', 'ws-develop', 'crowbar')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy).not.toBeNull()
    const entry = hierarchy!.entries.find(e => e.wsId === 'ws3')
    expect(entry?.parentId).toBe('ws-develop')
  })

  it('removes parentId when newParentId is undefined', async () => {
    await handleWorkspaceReparented('ws3', undefined, 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    const ws = repo.workspaces.find(w => w.id === 'ws3')!
    expect(ws.parentId).toBeUndefined()
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    const entry = hierarchy!.entries.find(e => e.wsId === 'ws3')
    expect(entry?.parentId).toBeUndefined()
  })
})

describe('reparentWorkspace', () => {
  it('resolves without throwing', async () => {
    await expect(reparentWorkspace('ws3', 'ws-develop', 'crowbar')).resolves.toBeUndefined()
  })

  it('updates store and IDB (delegates to handler)', async () => {
    await reparentWorkspace('ws3', 'ws-develop', 'crowbar')
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    expect(repo.workspaces.find(w => w.id === 'ws3')?.parentId).toBe('ws-develop')
    const hierarchy = await loadWorkspaceHierarchy('crowbar')
    expect(hierarchy?.entries.find(e => e.wsId === 'ws3')?.parentId).toBe('ws-develop')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd web && npx vitest run src/__tests__/lib/api/workspace.test.ts
```

Expected: FAIL — `Cannot find module '@/lib/api/workspace'`

- [ ] **Step 3: Create lib/api directory and implement workspace.ts**

Create `web/src/lib/api/workspace.ts`:

```ts
import { saveWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

/**
 * Stub: simulates a backend reparent call. Replace body with real HTTP call
 * when backend exists; keep handleWorkspaceReparented as the WS/SSE handler.
 */
export async function reparentWorkspace(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  await handleWorkspaceReparented(wsId, newParentId, repoId)
}

/**
 * Notification handler — called by the stub above and, in future, by the
 * real-time WS/SSE message from the backend.
 */
export async function handleWorkspaceReparented(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  useSidebarStore.getState().reparentWorkspace(wsId, newParentId)

  const repo = useSidebarStore.getState().repos.find(r => r.id === repoId)
  if (!repo) return

  const entries = repo.workspaces.map(w => ({
    wsId: w.id,
    ...(w.parentId !== undefined && { parentId: w.parentId }),
  }))

  await saveWorkspaceHierarchy(repoId, entries)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd web && npx vitest run src/__tests__/lib/api/workspace.test.ts
```

Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
cd web
git add src/lib/api/workspace.ts src/__tests__/lib/api/workspace.test.ts
git commit -m "feat(api): add workspace reparent stub and notification handler"
```

---

## Task 5: Wire toggleRepo to persist collapse state

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/__tests__/lib/store/sidebar.test.ts`

- [ ] **Step 1: Add IDB write tests for toggleRepo**

Open `web/src/__tests__/lib/store/sidebar.test.ts` and add at the end of the file:

```ts
import { loadSidebarUI } from '@/lib/persistence/sidebar-ui'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'

describe('toggleRepo persistence', () => {
  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('writes collapsed state to IDB after toggling on', async () => {
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise(r => setTimeout(r, 20))
    const saved = await loadSidebarUI()
    expect(saved?.collapsedRepos).toContain('crowbar')
  })

  it('removes repo from IDB after toggling off', async () => {
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise(r => setTimeout(r, 20))
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise(r => setTimeout(r, 20))
    const saved = await loadSidebarUI()
    expect(saved?.collapsedRepos).not.toContain('crowbar')
  })
})
```

- [ ] **Step 2: Run new tests to verify they fail**

```bash
cd web && npx vitest run src/__tests__/lib/store/sidebar.test.ts
```

Expected: The two new `toggleRepo persistence` tests FAIL (IDB not written yet).

- [ ] **Step 3: Update toggleRepo in sidebar.ts to fire-and-forget IDB write**

In `web/src/lib/store/sidebar.ts`, add the import at the top:

```ts
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
```

Then replace the `toggleRepo` action:

```ts
  toggleRepo: (repoId) =>
    set(s => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      saveSidebarUI([...next])
      return { collapsedRepos: next }
    }),
```

- [ ] **Step 4: Run all sidebar tests to verify they pass**

```bash
cd web && npx vitest run src/__tests__/lib/store/sidebar.test.ts
```

Expected: PASS (all tests including the two new ones)

- [ ] **Step 5: Commit**

```bash
cd web
git add src/lib/store/sidebar.ts src/__tests__/lib/store/sidebar.test.ts
git commit -m "feat(store): persist sidebar collapse state to IDB on toggle"
```

---

## Task 6: hydrateSidebar (TDD)

**Files:**
- Modify: `web/src/lib/persistence/hydrate.ts`
- Modify: `web/src/__tests__/lib/persistence/hydrate.test.ts`

- [ ] **Step 1: Write failing tests for hydrateSidebar**

Open `web/src/__tests__/lib/persistence/hydrate.test.ts` and add at the end of the file:

```ts
import { hydrateSidebar } from '@/lib/persistence/hydrate'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import { saveWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('hydrateSidebar', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('does nothing when IDB is empty', async () => {
    await hydrateSidebar()
    expect(useSidebarStore.getState().collapsedRepos.size).toBe(0)
  })

  it('restores collapsedRepos from IDB', async () => {
    await saveSidebarUI(['crowbar', 'quiver-core'])
    await hydrateSidebar()
    const { collapsedRepos } = useSidebarStore.getState()
    expect(collapsedRepos.has('crowbar')).toBe(true)
    expect(collapsedRepos.has('quiver-core')).toBe(true)
  })

  it('overlays parentId values from IDB onto repos', async () => {
    await saveWorkspaceHierarchy('crowbar', [
      { wsId: 'ws3', parentId: 'ws-develop' },
      { wsId: 'ws1', parentId: 'ws3' },
    ])
    await hydrateSidebar()
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    expect(repo.workspaces.find(w => w.id === 'ws3')?.parentId).toBe('ws-develop')
    expect(repo.workspaces.find(w => w.id === 'ws1')?.parentId).toBe('ws3')
  })

  it('clears parentId for workspaces not in hierarchy entries', async () => {
    await saveWorkspaceHierarchy('crowbar', [
      { wsId: 'ws1' },
    ])
    await hydrateSidebar()
    const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
    expect(repo.workspaces.find(w => w.id === 'ws1')?.parentId).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run new tests to verify they fail**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/hydrate.test.ts
```

Expected: `hydrateSidebar` tests FAIL — function not exported yet.

- [ ] **Step 3: Implement hydrateSidebar in hydrate.ts**

Open `web/src/lib/persistence/hydrate.ts`. Add imports at the top:

```ts
import { loadSidebarUI } from './sidebar-ui'
import { loadAllWorkspaceHierarchies } from './workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'
```

Then add `hydrateSidebar` at the end of the file:

```ts
export async function hydrateSidebar(): Promise<void> {
  const [sidebarUI, hierarchies] = await Promise.all([
    loadSidebarUI(),
    loadAllWorkspaceHierarchies(),
  ])

  if (sidebarUI) {
    useSidebarStore.setState({ collapsedRepos: new Set(sidebarUI.collapsedRepos) })
  }

  if (hierarchies.length > 0) {
    useSidebarStore.setState(s => ({
      repos: s.repos.map(repo => {
        const hierarchy = hierarchies.find(h => h.repoId === repo.id)
        if (!hierarchy) return repo
        const entryMap = new Map(hierarchy.entries.map(e => [e.wsId, e.parentId]))
        return {
          ...repo,
          workspaces: repo.workspaces.map(ws =>
            entryMap.has(ws.id)
              ? { ...ws, parentId: entryMap.get(ws.id) }
              : ws,
          ),
        }
      }),
    }))
  }
}
```

- [ ] **Step 4: Run all hydrate tests to verify they pass**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/hydrate.test.ts
```

Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
cd web
git add src/lib/persistence/hydrate.ts src/__tests__/lib/persistence/hydrate.test.ts
git commit -m "feat(persistence): add hydrateSidebar for collapse and hierarchy restore"
```

---

## Task 7: Wire HydrationGate and workspace-tree-context

**Files:**
- Modify: `web/src/components/hydration-gate.tsx`
- Modify: `web/src/components/layout/workspace-tree-context.tsx`

- [ ] **Step 1: Update HydrationGate to run hydrateSidebar in parallel**

Replace the contents of `web/src/components/hydration-gate.tsx` with:

```tsx
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar } from '@/lib/persistence/hydrate'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    Promise.all([hydratePreferences(), hydrateSidebar()])
      .then(() => setHydrated(true))
      .catch(() => setHydrated(true))
  }, [])

  if (!hydrated) return null

  return <>{children}</>
}
```

- [ ] **Step 2: Update workspace-tree-context.tsx drop handler to use the API**

Open `web/src/components/layout/workspace-tree-context.tsx`. Add import at the top of the file (with existing imports):

```ts
import { reparentWorkspace } from '@/lib/api/workspace'
```

Find the `onPointerUp` handler. Replace the two `useSidebarStore.getState().reparentWorkspace(...)` calls:

Before:
```ts
      } else if (target?.startsWith('ws:')) {
        const targetWsId = target.slice(3)
        if (targetWsId !== ws.id) {
          const repos = useSidebarStore.getState().repos
          const targetRepo = repos.find(r => r.workspaces.some(w => w.id === targetWsId))
          if (targetRepo?.id === ws.repoId) {
            useSidebarStore.getState().reparentWorkspace(ws.id, targetWsId)
          }
        }
      } else if (target?.startsWith('repo:')) {
        const targetRepoId = target.slice(5)
        if (targetRepoId === ws.repoId) {
          useSidebarStore.getState().reparentWorkspace(ws.id, undefined)
        }
      }
```

After:
```ts
      } else if (target?.startsWith('ws:')) {
        const targetWsId = target.slice(3)
        if (targetWsId !== ws.id) {
          const repos = useSidebarStore.getState().repos
          const targetRepo = repos.find(r => r.workspaces.some(w => w.id === targetWsId))
          if (targetRepo?.id === ws.repoId) {
            void reparentWorkspace(ws.id, targetWsId, ws.repoId)
          }
        }
      } else if (target?.startsWith('repo:')) {
        const targetRepoId = target.slice(5)
        if (targetRepoId === ws.repoId) {
          void reparentWorkspace(ws.id, undefined, ws.repoId)
        }
      }
```

- [ ] **Step 3: Verify HydrationGate test still passes**

```bash
cd web && npx vitest run src/__tests__/components/hydration-gate.test.tsx
```

Expected: PASS

- [ ] **Step 4: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: All tests PASS. Fix any failures before committing.

- [ ] **Step 5: Commit**

```bash
cd web
git add src/components/hydration-gate.tsx src/components/layout/workspace-tree-context.tsx
git commit -m "feat: wire sidebar persistence hydration and reparent API to UI"
```

---

## Task 8: Verify end-to-end

- [ ] **Step 1: Run the full test suite one final time**

```bash
cd web && npx vitest run
```

Expected: All tests PASS with no failures or skips.

- [ ] **Step 2: Confirm no TypeScript errors**

```bash
cd web && npx tsc --noEmit
```

Expected: No errors.

- [ ] **Step 3: Done — report back to user**
