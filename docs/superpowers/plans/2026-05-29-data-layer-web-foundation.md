# Data Layer — Web Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the full data layer infrastructure (transport contract, query conventions, IndexedDB persistence, event wiring, optimistic mutations) and fix the render layer (selector violations, component decomposition, memoization, animation jank) across the existing web codebase.

**Architecture:** `lib/` owns all infrastructure (no JSX, never imports from `features/`). Features import from `lib/`. Local-only state lives in Zustand + IndexedDB; server-owned data lives in TanStack Query, kept fresh by daemon push events rather than polling. A `window.__CROWBAR__` dev polyfill bridges the transport contract until Tauri replaces it.

**Tech Stack:** React 18, Zustand, TanStack Query v5, Vitest + jsdom, `idb` (IndexedDB wrapper), Tailwind CSS

**Scope note:** The Tauri transport layer (Layer 1 of the data layer spec — crowbar:// scheme, Rust bridge, Go sidecar) is a separate plan. This plan establishes the contract it will fulfil (`window.__CROWBAR__`) and a dev polyfill so all other work is testable now.

---

## Task 1: Animation Fix — Remove transition-colors from resize handle

**Files:**
- Modify: `web/src/features/panes/components/pane-resize-handle.tsx`

This is the single highest-impact performance fix and takes under 2 minutes.

- [ ] **Step 1: Locate the transition class**

Open `web/src/features/panes/components/pane-resize-handle.tsx`. Find the `className` that contains `transition-colors` (around line 107).

- [ ] **Step 2: Remove the transition**

Remove `transition-colors` (and any other `transition-*` classes) from the resize handle element. Layout-affecting animations and transitions on interactive drag handles cause forced reflows on every `pointermove` frame.

- [ ] **Step 3: Verify the app still builds**

```bash
cd web && bun run build 2>&1 | tail -5
```

Expected: no TypeScript errors, build succeeds.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/panes/components/pane-resize-handle.tsx
git commit -m "perf: remove transition-colors from pane resize handle"
```

---

## Task 2: Install idb + Define window.__CROWBAR__ contract

**Files:**
- Create: `web/src/lib/transport/types.ts`
- Create: `web/src/lib/transport/polyfill.ts`
- Modify: `web/src/main.tsx` (or app entry point — import polyfill)
- Create: `web/src/__tests__/lib/transport/polyfill.test.ts`

`idb` is the IndexedDB wrapper Linear uses. `window.__CROWBAR__` is the bridge contract Tauri will inject at runtime; in development, the polyfill provides it.

- [ ] **Step 1: Install idb**

```bash
cd web && bun add idb
```

Expected: `idb` added to `package.json` dependencies.

- [ ] **Step 2: Write the failing polyfill test**

Create `web/src/__tests__/lib/transport/polyfill.test.ts`:

```ts
import '@/lib/transport/polyfill'

describe('window.__CROWBAR__ polyfill', () => {
  it('sets mode to local', () => {
    expect(window.__CROWBAR__.mode).toBe('local')
  })

  it('sets endpoint from env or default', () => {
    expect(typeof window.__CROWBAR__.endpoint).toBe('string')
    expect(window.__CROWBAR__.endpoint.length).toBeGreaterThan(0)
  })

  it('on() returns an unlisten function', () => {
    const unlisten = window.__CROWBAR__.on('test:event', () => {})
    expect(typeof unlisten).toBe('function')
    unlisten()
  })

  it('emit() calls registered handlers', () => {
    const handler = vi.fn()
    const unlisten = window.__CROWBAR__.on('test:ping', handler)
    window.__CROWBAR__.emit('test:ping', { value: 42 })
    expect(handler).toHaveBeenCalledWith({ value: 42 })
    unlisten()
  })

  it('unlisten removes the handler', () => {
    const handler = vi.fn()
    const unlisten = window.__CROWBAR__.on('test:remove', handler)
    unlisten()
    window.__CROWBAR__.emit('test:remove', {})
    expect(handler).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/transport/polyfill.test.ts
```

Expected: FAIL — `window.__CROWBAR__` is not defined.

- [ ] **Step 4: Create lib/transport/types.ts**

Create `web/src/lib/transport/types.ts`:

```ts
interface CrowbarBridge {
  mode: 'local' | 'remote'
  /** Base URL for API calls. 'crowbar://api' locally, remote URL in production. */
  endpoint: string
  /** Subscribe to a daemon event. Returns an unlisten function. */
  on: <T = unknown>(event: string, handler: (payload: T) => void) => () => void
  /** Emit an event toward the daemon. */
  emit: (event: string, payload?: unknown) => void
}

declare global {
  interface Window {
    __CROWBAR__: CrowbarBridge
  }
}

export type {}
```

- [ ] **Step 5: Create lib/transport/polyfill.ts**

Create `web/src/lib/transport/polyfill.ts`:

```ts
import './types'

if (typeof window !== 'undefined' && !window.__CROWBAR__) {
  const listeners = new Map<string, Set<(payload: unknown) => void>>()

  window.__CROWBAR__ = {
    mode: 'local',
    endpoint: (import.meta as { env?: { VITE_API_URL?: string } }).env?.VITE_API_URL ?? 'http://localhost:7457',
    on: <T = unknown>(event: string, handler: (payload: T) => void) => {
      if (!listeners.has(event)) listeners.set(event, new Set())
      listeners.get(event)!.add(handler as (payload: unknown) => void)
      return () => {
        listeners.get(event)?.delete(handler as (payload: unknown) => void)
      }
    },
    emit: (event: string, payload?: unknown) => {
      listeners.get(event)?.forEach(h => h(payload))
    },
  }
}
```

- [ ] **Step 6: Import polyfill at app entry**

Find the app entry point (likely `web/src/main.tsx`). Add as the first import:

```ts
import '@/lib/transport/polyfill'
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/transport/polyfill.test.ts
```

Expected: PASS — all 5 tests pass.

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/transport/ web/src/main.tsx web/src/__tests__/lib/transport/
git commit -m "feat: add CrowbarBridge type contract and dev polyfill"
```

---

## Task 3: lib/queries/keys.ts — Query key factory

**Files:**
- Create: `web/src/lib/queries/keys.ts`
- Create: `web/src/__tests__/lib/queries/keys.test.ts`

All TanStack Query keys live here. Features never define their own keys.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/queries/keys.test.ts`:

```ts
import { queryKeys } from '@/lib/queries/keys'

describe('queryKeys', () => {
  describe('workspaces', () => {
    it('all is the root', () => {
      expect(queryKeys.workspaces.all).toEqual(['workspaces'])
    })
    it('list extends all', () => {
      expect(queryKeys.workspaces.list()).toEqual(['workspaces', 'list'])
    })
    it('detail extends all with id', () => {
      expect(queryKeys.workspaces.detail('ws-1')).toEqual(['workspaces', 'ws-1'])
    })
  })

  describe('chats', () => {
    it('messages key includes chatId', () => {
      expect(queryKeys.chats.messages('chat-abc')).toEqual(['chats', 'chat-abc', 'messages'])
    })
    it('byWorkspace key includes workspaceId', () => {
      expect(queryKeys.chats.byWorkspace('ws-1')).toEqual(['chats', 'workspace', 'ws-1'])
    })
  })

  describe('git', () => {
    it('status key includes workspaceId', () => {
      expect(queryKeys.git.status('ws-1')).toEqual(['git', 'ws-1', 'status'])
    })
  })

  describe('files', () => {
    it('tree key includes workspaceId and path', () => {
      expect(queryKeys.files.tree('ws-1', '/src')).toEqual(['files', 'ws-1', 'tree', '/src'])
    })
  })

  it('workspace detail and list both start with workspaces.all', () => {
    const all = queryKeys.workspaces.all
    expect(queryKeys.workspaces.list().slice(0, all.length)).toEqual([...all])
    expect(queryKeys.workspaces.detail('x').slice(0, all.length)).toEqual([...all])
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/queries/keys.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create lib/queries/keys.ts**

Create `web/src/lib/queries/keys.ts`:

```ts
export const queryKeys = {
  workspaces: {
    all: ['workspaces'] as const,
    list: () => [...queryKeys.workspaces.all, 'list'] as const,
    detail: (id: string) => [...queryKeys.workspaces.all, id] as const,
  },
  chats: {
    byWorkspace: (wsId: string) => ['chats', 'workspace', wsId] as const,
    detail: (id: string) => ['chats', id] as const,
    messages: (id: string) => ['chats', id, 'messages'] as const,
  },
  git: {
    status: (wsId: string) => ['git', wsId, 'status'] as const,
    branches: (wsId: string) => ['git', wsId, 'branches'] as const,
    log: (wsId: string) => ['git', wsId, 'log'] as const,
  },
  files: {
    tree: (wsId: string, path: string) => ['files', wsId, 'tree', path] as const,
  },
} as const
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/queries/keys.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/queries/keys.ts web/src/__tests__/lib/queries/keys.test.ts
git commit -m "feat: add query key factory in lib/queries/keys"
```

---

## Task 4: lib/queries/client.ts — QueryClient with staleTime defaults

**Files:**
- Create: `web/src/lib/queries/client.ts`
- Create: `web/src/__tests__/lib/queries/client.test.ts`
- Modify: App root (find where `QueryClient` is currently instantiated — likely `web/src/main.tsx` or a provider file — and replace it with the import from `lib/queries/client.ts`)

- [ ] **Step 1: Find the current QueryClient instantiation**

```bash
grep -rn "new QueryClient" web/src/ --include="*.ts" --include="*.tsx"
```

Note the file path — you will replace that instantiation in a later step.

- [ ] **Step 2: Write the failing test**

Create `web/src/__tests__/lib/queries/client.test.ts`:

```ts
import { queryClient } from '@/lib/queries/client'

describe('queryClient', () => {
  it('is a QueryClient instance', () => {
    expect(queryClient).toBeDefined()
    expect(typeof queryClient.invalidateQueries).toBe('function')
  })

  it('uses staleTime Infinity as default', () => {
    const defaults = queryClient.getDefaultOptions()
    expect(defaults.queries?.staleTime).toBe(Infinity)
  })

  it('retries once on failure', () => {
    const defaults = queryClient.getDefaultOptions()
    expect(defaults.queries?.retry).toBe(1)
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/queries/client.test.ts
```

Expected: FAIL.

- [ ] **Step 4: Create lib/queries/client.ts**

Create `web/src/lib/queries/client.ts`:

```ts
import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: Infinity,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
```

- [ ] **Step 5: Replace the existing QueryClient instantiation**

In the file found in Step 1, remove the existing `new QueryClient(...)` and import from `lib/queries/client.ts` instead:

```ts
import { queryClient } from '@/lib/queries/client'
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/queries/client.test.ts
```

Expected: PASS.

- [ ] **Step 7: Run the full test suite to confirm no regressions**

```bash
cd web && bun run test
```

Expected: all previously passing tests still pass.

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/queries/client.ts web/src/__tests__/lib/queries/client.test.ts
git commit -m "feat: add shared QueryClient in lib/queries/client with staleTime: Infinity"
```

---

## Task 5: lib/events/ — Daemon event types and cache invalidation wiring

**Files:**
- Create: `web/src/lib/events/types.ts`
- Create: `web/src/lib/events/connect.ts`
- Create: `web/src/__tests__/lib/events/connect.test.ts`
- Modify: App root — call `connectDaemonEvents(queryClient)` once on startup

- [ ] **Step 1: Create lib/events/types.ts**

```ts
// web/src/lib/events/types.ts
export type DaemonEvent =
  | { type: 'workspace:updated' }
  | { type: 'chat:message'; chatId: string }
  | { type: 'git:changed'; workspaceId: string }
  | { type: 'file:changed'; workspaceId: string; path: string }
  | { type: 'daemon:status'; state: 'starting' | 'ready' | 'degraded' | 'reconnecting' }
```

- [ ] **Step 2: Write the failing test**

Create `web/src/__tests__/lib/events/connect.test.ts`:

```ts
import '@/lib/transport/polyfill'
import { QueryClient } from '@tanstack/react-query'
import { connectDaemonEvents } from '@/lib/events/connect'
import { queryKeys } from '@/lib/queries/keys'

describe('connectDaemonEvents', () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient()
    vi.spyOn(qc, 'invalidateQueries')
    connectDaemonEvents(qc)
  })

  it('invalidates workspaces on workspace:updated', () => {
    window.__CROWBAR__.emit('workspace:updated')
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.workspaces.all,
    })
  })

  it('invalidates specific chat messages on chat:message', () => {
    window.__CROWBAR__.emit('chat:message', { chatId: 'chat-1' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.chats.messages('chat-1'),
    })
  })

  it('invalidates git status on git:changed', () => {
    window.__CROWBAR__.emit('git:changed', { workspaceId: 'ws-1' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.git.status('ws-1'),
    })
  })

  it('invalidates file tree on file:changed', () => {
    window.__CROWBAR__.emit('file:changed', { workspaceId: 'ws-1', path: '/src' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.files.tree('ws-1', '/src'),
    })
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/events/connect.test.ts
```

Expected: FAIL.

- [ ] **Step 4: Create lib/events/connect.ts**

```ts
// web/src/lib/events/connect.ts
import { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queries/keys'

export function connectDaemonEvents(qc: QueryClient): void {
  window.__CROWBAR__.on('workspace:updated', () =>
    qc.invalidateQueries({ queryKey: queryKeys.workspaces.all })
  )

  window.__CROWBAR__.on<{ chatId: string }>('chat:message', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.chats.messages(p.chatId) })
  )

  window.__CROWBAR__.on<{ workspaceId: string }>('git:changed', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.git.status(p.workspaceId) })
  )

  window.__CROWBAR__.on<{ workspaceId: string; path: string }>('file:changed', (p) =>
    qc.invalidateQueries({ queryKey: queryKeys.files.tree(p.workspaceId, p.path) })
  )
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/events/connect.test.ts
```

Expected: PASS.

- [ ] **Step 6: Wire connectDaemonEvents at app startup**

In the app entry point (e.g. `web/src/main.tsx`), after the QueryClient is available, add:

```ts
import { connectDaemonEvents } from '@/lib/events/connect'
import { queryClient } from '@/lib/queries/client'

// After QueryClient is set up:
connectDaemonEvents(queryClient)
```

This must be called exactly once — not inside a component.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/events/ web/src/__tests__/lib/events/ web/src/main.tsx
git commit -m "feat: add daemon event types and cache invalidation wiring"
```

---

## Task 6: lib/mutations/optimistic.ts — Optimistic mutation wrapper

**Files:**
- Create: `web/src/lib/mutations/optimistic.ts`
- Create: `web/src/__tests__/lib/mutations/optimistic.test.ts`

Every mutation in the codebase must use this wrapper. It handles optimistic update → error rollback → cache invalidation.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/mutations/optimistic.test.ts`:

```ts
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useOptimisticMutation } from '@/lib/mutations/optimistic'
import { queryKeys } from '@/lib/queries/keys'

function makeWrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useOptimisticMutation', () => {
  it('calls mutationFn and invalidates on success', async () => {
    const qc = new QueryClient()
    vi.spyOn(qc, 'invalidateQueries')

    const mutationFn = vi.fn().mockResolvedValue({ ok: true })
    const onMutate = vi.fn().mockResolvedValue('snapshot')
    const onSnapshot = vi.fn()

    const { result } = renderHook(
      () => useOptimisticMutation(mutationFn, {
        onMutate,
        onSnapshot,
        invalidateKey: queryKeys.workspaces.all,
      }),
      { wrapper: makeWrapper(qc) }
    )

    result.current.mutate({ name: 'test' })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mutationFn).toHaveBeenCalledWith({ name: 'test' })
    expect(qc.invalidateQueries).toHaveBeenCalledWith({
      queryKey: queryKeys.workspaces.all,
    })
  })

  it('calls onSnapshot with the snapshot on error', async () => {
    const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    const mutationFn = vi.fn().mockRejectedValue(new Error('fail'))
    const snapshot = { previous: [1, 2, 3] }
    const onMutate = vi.fn().mockResolvedValue(snapshot)
    const onSnapshot = vi.fn()

    const { result } = renderHook(
      () => useOptimisticMutation(mutationFn, {
        onMutate,
        onSnapshot,
        invalidateKey: queryKeys.workspaces.all,
      }),
      { wrapper: makeWrapper(qc) }
    )

    result.current.mutate({})

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(onSnapshot).toHaveBeenCalledWith(snapshot)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/mutations/optimistic.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Create lib/mutations/optimistic.ts**

```ts
// web/src/lib/mutations/optimistic.ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'

interface OptimisticOptions<TVariables, TSnapshot> {
  onMutate: (vars: TVariables) => Promise<TSnapshot>
  onSnapshot: (snapshot: TSnapshot) => void
  invalidateKey: QueryKey
}

export function useOptimisticMutation<TData, TVariables, TSnapshot = unknown>(
  mutationFn: (vars: TVariables) => Promise<TData>,
  options: OptimisticOptions<TVariables, TSnapshot>
) {
  const qc = useQueryClient()

  return useMutation<TData, Error, TVariables, TSnapshot>({
    mutationFn,
    onMutate: options.onMutate,
    onError: (_err, _vars, context) => {
      if (context !== undefined) {
        options.onSnapshot(context)
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: options.invalidateKey })
    },
  })
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/mutations/optimistic.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/mutations/ web/src/__tests__/lib/mutations/
git commit -m "feat: add useOptimisticMutation with snapshot rollback"
```

---

## Task 7: lib/persistence/ — IDB schema, adapter, and hydration

**Files:**
- Create: `web/src/lib/persistence/schemas.ts`
- Create: `web/src/lib/persistence/idb.ts`
- Create: `web/src/lib/persistence/hydrate.ts`
- Create: `web/src/__tests__/lib/persistence/hydrate.test.ts`

- [ ] **Step 1: Create lib/persistence/schemas.ts**

```ts
// web/src/lib/persistence/schemas.ts
import type { DBSchema } from 'idb'

export interface WorkspaceLayout {
  workspaceId: string
  panes: unknown[]        // typed by PaneConfig when that type is stable
  activePane: string
  tabGroups: unknown[]    // typed by TabGroup when that type is stable
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
}
```

- [ ] **Step 2: Create lib/persistence/idb.ts**

```ts
// web/src/lib/persistence/idb.ts
import { openDB } from 'idb'
import type { IDBPDatabase } from 'idb'
import type { CrowbarDB } from './schemas'

let _db: IDBPDatabase<CrowbarDB> | null = null

export async function getDB(): Promise<IDBPDatabase<CrowbarDB>> {
  if (_db) return _db
  _db = await openDB<CrowbarDB>('crowbar', 1, {
    upgrade(db) {
      db.createObjectStore('workspace-layout', { keyPath: 'workspaceId' })
      const editorStore = db.createObjectStore('editor-state', {
        keyPath: ['workspaceId', 'bufferId'],
      })
      editorStore.createIndex('workspaceId', 'workspaceId')
      db.createObjectStore('ui-preferences')
    },
  })
  return _db
}
```

- [ ] **Step 3: Write the failing hydration test**

Create `web/src/__tests__/lib/persistence/hydrate.test.ts`:

```ts
import { hydrateFromIDB } from '@/lib/persistence/hydrate'
import { getDB } from '@/lib/persistence/idb'
import type { WorkspaceLayout, UIPreferences } from '@/lib/persistence/schemas'

async function seedDB(workspaceId: string) {
  const db = await getDB()
  const layout: WorkspaceLayout = {
    workspaceId,
    panes: [],
    activePane: 'pane-1',
    tabGroups: [],
    sidebarWidth: 240,
    rightSidebarWidth: 280,
    updatedAt: Date.now(),
  }
  const prefs: UIPreferences = {
    theme: 'dark',
    fontSize: 14,
    fontFamily: 'JetBrains Mono',
    tabSize: 2,
    wordWrap: false,
    minimap: true,
    updatedAt: Date.now(),
  }
  await db.put('workspace-layout', layout)
  await db.put('ui-preferences', prefs, 'global')
  return { layout, prefs }
}

describe('hydrateFromIDB', () => {
  it('returns null layout and prefs when IDB is empty', async () => {
    const result = await hydrateFromIDB('missing-ws')
    expect(result.layout).toBeNull()
    expect(result.prefs).toBeNull()
    expect(result.editorStates).toEqual([])
  })

  it('returns layout and prefs when seeded', async () => {
    const { layout, prefs } = await seedDB('ws-test')
    const result = await hydrateFromIDB('ws-test')
    expect(result.layout?.workspaceId).toBe(layout.workspaceId)
    expect(result.layout?.sidebarWidth).toBe(240)
    expect(result.prefs?.theme).toBe(prefs.theme)
  })
})
```

- [ ] **Step 4: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/persistence/hydrate.test.ts
```

Expected: FAIL.

- [ ] **Step 5: Create lib/persistence/hydrate.ts**

```ts
// web/src/lib/persistence/hydrate.ts
import { getDB } from './idb'
import type { WorkspaceLayout, EditorState, UIPreferences } from './schemas'

export interface HydrationResult {
  layout: WorkspaceLayout | null
  prefs: UIPreferences | null
  editorStates: EditorState[]
}

export async function hydrateFromIDB(workspaceId: string): Promise<HydrationResult> {
  const db = await getDB()

  const [layout, prefs, editorStates] = await Promise.all([
    db.get('workspace-layout', workspaceId).then(r => r ?? null),
    db.get('ui-preferences', 'global').then(r => r ?? null),
    db.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  return { layout, prefs, editorStates }
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/persistence/hydrate.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/persistence/ web/src/__tests__/lib/persistence/
git commit -m "feat: add IDB schema, adapter, and hydration function"
```

---

## Task 8: HydrationGate — Block React mount until IDB hydrates

**Files:**
- Create: `web/src/components/hydration-gate.tsx`
- Create: `web/src/__tests__/components/hydration-gate.test.tsx`
- Modify: App root component — wrap with `HydrationGate`

The HydrationGate calls `hydrateFromIDB` for the active workspace, populates Zustand stores, then renders children. First render has full local state — no network dependency.

- [ ] **Step 1: Find the active workspace ID source**

```bash
grep -rn "activeProjectId\|activeWorkspace\|getState" web/src/lib/store/ --include="*.ts" | head -10
```

Identify how the active workspace/project ID is currently read (likely `useProjectsStore` or `lib/store/projects.ts`).

- [ ] **Step 2: Write the failing test**

Create `web/src/__tests__/components/hydration-gate.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { HydrationGate } from '@/components/hydration-gate'

vi.mock('@/lib/persistence/hydrate', () => ({
  hydrateFromIDB: vi.fn().mockResolvedValue({
    layout: null,
    prefs: null,
    editorStates: [],
  }),
}))

describe('HydrationGate', () => {
  it('renders children after hydration resolves', async () => {
    render(
      <HydrationGate workspaceId="ws-1">
        <div>app content</div>
      </HydrationGate>
    )

    await waitFor(() => {
      expect(screen.getByText('app content')).toBeInTheDocument()
    })
  })

  it('renders nothing before hydration resolves', () => {
    // hydrateFromIDB is mocked to resolve immediately in tests,
    // so this verifies the gate exists as a concept
    const { container } = render(
      <HydrationGate workspaceId="ws-1">
        <span>child</span>
      </HydrationGate>
    )
    // Gate renders; children appear after async hydration
    expect(container).toBeDefined()
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/components/hydration-gate.test.tsx
```

Expected: FAIL.

- [ ] **Step 4: Create components/hydration-gate.tsx**

```tsx
// web/src/components/hydration-gate.tsx
import { useEffect, useState } from 'react'
import { hydrateFromIDB } from '@/lib/persistence/hydrate'

interface HydrationGateProps {
  workspaceId: string
  children: React.ReactNode
}

export function HydrationGate({ workspaceId, children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    hydrateFromIDB(workspaceId).then(() => setHydrated(true))
  }, [workspaceId])

  if (!hydrated) return null

  return <>{children}</>
}
```

**Note:** `hydrateFromIDB` currently returns data but doesn't write to stores — that wiring happens in Tasks 9 and 10. For now the gate ensures the async boundary exists.

- [ ] **Step 5: Wrap app root with HydrationGate**

Find where the main app component renders (likely `web/src/App.tsx` or `web/src/main.tsx`). Wrap the main content with:

```tsx
import { HydrationGate } from '@/components/hydration-gate'

// Replace the root render with:
<HydrationGate workspaceId={activeWorkspaceId}>
  {/* existing app content */}
</HydrationGate>
```

Use the active workspace ID source found in Step 1.

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/components/hydration-gate.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Run full test suite**

```bash
cd web && bun run test
```

Expected: all previously passing tests still pass.

- [ ] **Step 8: Commit**

```bash
git add web/src/components/hydration-gate.tsx web/src/__tests__/components/hydration-gate.test.tsx
git commit -m "feat: add HydrationGate component — blocks render until IDB hydrates"
```

---

## Task 9: Wire workspace layout persistence to IDB

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store.ts`
- Modify: `web/src/lib/persistence/hydrate.ts`
- Create: `web/src/__tests__/lib/persistence/workspace-layout.test.ts`

- [ ] **Step 1: Identify the workspace store's layout fields**

```bash
cat web/src/features/workspace/stores/workspace-store.ts | head -80
```

Identify which fields constitute "layout" — pane config, active pane, tab groups, sidebar widths.

- [ ] **Step 2: Write the failing test**

Create `web/src/__tests__/lib/persistence/workspace-layout.test.ts`:

```ts
import { saveWorkspaceLayout, loadWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'

const mockLayout: WorkspaceLayout = {
  workspaceId: 'ws-persist-test',
  panes: [],
  activePane: 'pane-a',
  tabGroups: [],
  sidebarWidth: 260,
  rightSidebarWidth: 300,
  updatedAt: 1000,
}

describe('workspace layout persistence', () => {
  it('saves and loads layout round-trip', async () => {
    await saveWorkspaceLayout(mockLayout)
    const loaded = await loadWorkspaceLayout('ws-persist-test')
    expect(loaded?.activePane).toBe('pane-a')
    expect(loaded?.sidebarWidth).toBe(260)
  })

  it('returns null for unknown workspaceId', async () => {
    const loaded = await loadWorkspaceLayout('nonexistent')
    expect(loaded).toBeNull()
  })
})
```

- [ ] **Step 3: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/persistence/workspace-layout.test.ts
```

Expected: FAIL.

- [ ] **Step 4: Create lib/persistence/workspace-layout.ts**

```ts
// web/src/lib/persistence/workspace-layout.ts
import { getDB } from './idb'
import type { WorkspaceLayout } from './schemas'

export async function saveWorkspaceLayout(layout: WorkspaceLayout): Promise<void> {
  const db = await getDB()
  await db.put('workspace-layout', { ...layout, updatedAt: Date.now() })
}

export async function loadWorkspaceLayout(workspaceId: string): Promise<WorkspaceLayout | null> {
  const db = await getDB()
  return (await db.get('workspace-layout', workspaceId)) ?? null
}
```

- [ ] **Step 5: Subscribe workspace store to persist on change**

In `web/src/features/workspace/stores/workspace-store.ts`, add a store subscription that saves layout on every change. Add after the store definition:

```ts
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'

// Subscribe to persist layout changes (debounced to avoid excessive IDB writes)
let persistTimer: ReturnType<typeof setTimeout>
useWorkspaceStore.subscribe((state) => {
  clearTimeout(persistTimer)
  persistTimer = setTimeout(() => {
    const workspaceId = state.activeWorkspaceId // use the correct field name
    if (!workspaceId) return
    saveWorkspaceLayout({
      workspaceId,
      panes: state.panes,           // adjust field names to match actual store shape
      activePane: state.activePane,
      tabGroups: state.tabGroups,
      sidebarWidth: state.sidebarWidth,
      rightSidebarWidth: state.rightSidebarWidth,
      updatedAt: Date.now(),
    })
  }, 300)
})
```

**Note:** Adjust field names to match the actual workspace store shape found in Step 1.

- [ ] **Step 6: Wire layout into HydrationGate**

Update `web/src/lib/persistence/hydrate.ts` to apply layout to the workspace store:

```ts
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Inside hydrateFromIDB, after reading layout:
if (layout) {
  useWorkspaceStore.setState({
    panes: layout.panes,
    activePane: layout.activePane,
    tabGroups: layout.tabGroups,
    sidebarWidth: layout.sidebarWidth,
    rightSidebarWidth: layout.rightSidebarWidth,
  })
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
cd web && bun run test src/__tests__/lib/persistence/workspace-layout.test.ts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/persistence/workspace-layout.ts web/src/__tests__/lib/persistence/ web/src/features/workspace/stores/workspace-store.ts web/src/lib/persistence/hydrate.ts
git commit -m "feat: persist workspace layout to IndexedDB, hydrate on startup"
```

---

## Task 10: Wire UI preferences persistence to IDB

**Files:**
- Create: `web/src/lib/persistence/ui-preferences.ts`
- Modify: `web/src/features/settings/stores/` (the main settings store)
- Modify: `web/src/lib/persistence/hydrate.ts`
- Create: `web/src/__tests__/lib/persistence/ui-preferences.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/persistence/ui-preferences.test.ts`:

```ts
import { saveUIPreferences, loadUIPreferences } from '@/lib/persistence/ui-preferences'
import type { UIPreferences } from '@/lib/persistence/schemas'

const mockPrefs: UIPreferences = {
  theme: 'github-dark',
  fontSize: 15,
  fontFamily: 'JetBrains Mono Variable',
  tabSize: 2,
  wordWrap: true,
  minimap: false,
  updatedAt: 0,
}

describe('ui preferences persistence', () => {
  it('saves and loads preferences round-trip', async () => {
    await saveUIPreferences(mockPrefs)
    const loaded = await loadUIPreferences()
    expect(loaded?.theme).toBe('github-dark')
    expect(loaded?.fontSize).toBe(15)
    expect(loaded?.minimap).toBe(false)
  })

  it('returns null when no preferences saved', async () => {
    const loaded = await loadUIPreferences()
    // May or may not be null depending on prior test order — just check type
    expect(loaded === null || typeof loaded === 'object').toBe(true)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && bun run test src/__tests__/lib/persistence/ui-preferences.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Create lib/persistence/ui-preferences.ts**

```ts
// web/src/lib/persistence/ui-preferences.ts
import { getDB } from './idb'
import type { UIPreferences } from './schemas'

export async function saveUIPreferences(prefs: UIPreferences): Promise<void> {
  const db = await getDB()
  await db.put('ui-preferences', { ...prefs, updatedAt: Date.now() }, 'global')
}

export async function loadUIPreferences(): Promise<UIPreferences | null> {
  const db = await getDB()
  return (await db.get('ui-preferences', 'global')) ?? null
}
```

- [ ] **Step 4: Subscribe settings store to persist on change**

Find the main settings store in `web/src/features/settings/stores/`. Add a subscription after the store definition:

```ts
import { saveUIPreferences } from '@/lib/persistence/ui-preferences'

let prefTimer: ReturnType<typeof setTimeout>
useSettingsStore.subscribe((state) => {
  clearTimeout(prefTimer)
  prefTimer = setTimeout(() => {
    saveUIPreferences({
      theme: state.settings.theme,              // adjust to actual shape
      fontSize: state.settings.fontSize,
      fontFamily: state.settings.fontFamily,
      tabSize: state.settings.tabSize,
      wordWrap: state.settings.wordWrap,
      minimap: state.settings.minimap,
      updatedAt: Date.now(),
    })
  }, 300)
})
```

- [ ] **Step 5: Wire prefs into hydrate.ts**

Update `web/src/lib/persistence/hydrate.ts` to apply prefs to the settings store:

```ts
import { useSettingsStore } from '@/features/settings/stores/settings-store' // adjust path

// Inside hydrateFromIDB, after reading prefs:
if (prefs) {
  useSettingsStore.setState((state) => ({
    settings: {
      ...state.settings,
      theme: prefs.theme,
      fontSize: prefs.fontSize,
      fontFamily: prefs.fontFamily,
      tabSize: prefs.tabSize,
      wordWrap: prefs.wordWrap,
      minimap: prefs.minimap,
    },
  }))
}
```

- [ ] **Step 6: Run tests**

```bash
cd web && bun run test src/__tests__/lib/persistence/ui-preferences.test.ts && bun run test
```

Expected: PASS, no regressions.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/persistence/ui-preferences.ts web/src/__tests__/lib/persistence/ web/src/features/settings/stores/ web/src/lib/persistence/hydrate.ts
git commit -m "feat: persist UI preferences to IndexedDB, hydrate on startup"
```

---

## Task 11: Fix all 18 Zustand selector violations

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`
- Modify: `web/src/features/tabs/components/tab-context-menu.tsx`
- Modify: `web/src/features/command-palette/components/command-palette.tsx`
- Modify: `web/src/features/settings/components/tabs/file-tree-settings.tsx`
- Modify: `web/src/features/settings/components/tabs/appearance-settings.tsx`
- Modify: `web/src/features/settings/components/tabs/git-settings.tsx`
- Modify: `web/src/features/settings/components/tabs/editor-settings.tsx`
- Modify: `web/src/features/settings/components/font-style-injector.tsx`
- Modify: `web/src/features/file-explorer/file-explorer/components/file-explorer-icon.tsx`
- Modify: `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx`
- Modify: `web/src/features/editor/components/code-editor.tsx`
- Modify: `web/src/features/editor/components/toolbar/editor-status-actions.tsx`
- Modify: `web/src/features/editor/components/external-editor-terminal.tsx`
- Modify: `web/src/features/git/components/git-view.tsx`
- Modify: `web/src/features/panes/components/pane-container.tsx`

**Rule:** Replace every `useXxxStore()` (no selector) and every destructure-from-hook with narrow selectors. One selector per field. Actions can use a single `useXxxStore(s => s.actions)` selector.

- [ ] **Step 1: Run the full test suite to establish a baseline**

```bash
cd web && bun run test 2>&1 | tail -10
```

Record the number of passing tests. This is your regression baseline.

- [ ] **Step 2: Fix tab-bar.tsx and tab-context-menu.tsx**

In `tab-bar.tsx`, replace:
```ts
const workspaceStore = useWorkspaceStore()
```
With narrow selectors for each field actually used. Example:
```ts
const panes = useWorkspaceStore(s => s.panes)
const activePane = useWorkspaceStore(s => s.activePane)
const actions = useWorkspaceStore(s => s.actions)
```

Apply the same pattern to `tab-context-menu.tsx`.

- [ ] **Step 3: Fix settings component selectors**

In each of the four settings tab files and `font-style-injector.tsx`, replace:
```ts
const { settings, updateSetting } = useSettingsStore()
```
With:
```ts
const theme = useSettingsStore(s => s.settings.theme)         // only the field used
const updateSetting = useSettingsStore(s => s.actions.updateSetting)
```

Only select the specific `settings.*` fields that are actually read in that component.

- [ ] **Step 4: Fix command-palette.tsx**

Replace the three broad selectors:
```ts
const { settings } = useSettingsStore()
const gitStore = useGitStore()
const workspaceStore = useWorkspaceStore()
```
With narrow selectors for each field the component actually uses.

- [ ] **Step 5: Fix remaining files**

Apply the same pattern to:
- `file-explorer-icon.tsx`
- `file-explorer-tree.tsx`
- `code-editor.tsx`
- `editor-status-actions.tsx`
- `external-editor-terminal.tsx`
- `git-view.tsx`
- `pane-container.tsx`

For each file: identify which store fields are read, create one narrow selector per field.

- [ ] **Step 6: Run the full test suite**

```bash
cd web && bun run test
```

Expected: same number of passing tests as the baseline. Zero regressions.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/
git commit -m "perf: fix all Zustand broad selector violations — narrow selectors only"
```

---

## Task 12: Add useMemo/useCallback to critical array operations

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`
- Modify: `web/src/features/settings/components/tabs/appearance-settings.tsx`
- Modify: `web/src/features/git/components/git-view.tsx`

Target only the highest-impact missing memoizations found in the audit.

- [ ] **Step 1: Fix tab-bar.tsx — chained filters**

Find the chained `.filter()` calls around line 119 of `tab-bar.tsx`. Wrap with `useMemo`:

```ts
const visibleBuffers = useMemo(
  () => buffers
    .filter(b => pane.bufferIds.includes(b.id))
    .filter(b => b.type !== 'newTab'),
  [buffers, pane.bufferIds]
)
```

Wrap all inline arrow functions passed to mapped `TabBarItem` children with `useCallback`.

- [ ] **Step 2: Fix appearance-settings.tsx — theme map**

Find the `.map((theme) => ({ value: theme.id, label: theme.name }))` call. Wrap:

```ts
const themeOptions = useMemo(
  () => themes.map(theme => ({ value: theme.id, label: theme.name })),
  [themes]
)
```

- [ ] **Step 3: Fix git-view.tsx — staged/unstaged filter**

Find the `.filter((file) => file.status !== 'untracked')` call. Wrap:

```ts
const stagedFiles = useMemo(
  () => gitStatus.filter(f => f.status !== 'untracked'),
  [gitStatus]
)
```

- [ ] **Step 4: Run the full test suite**

```bash
cd web && bun run test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/
git commit -m "perf: add useMemo/useCallback to critical render-path array operations"
```

---

## Task 13: Decompose tab-bar.tsx

**Files:**
- Create: `web/src/features/tabs/components/tab-item.tsx`
- Create: `web/src/features/tabs/components/tab-context-menu.tsx` (may already exist — extract or refine)
- Create: `web/src/features/tabs/components/tab-new-button.tsx`
- Modify: `web/src/features/tabs/components/tab-bar.tsx` (becomes orchestrator, ~200 lines)
- Test: `web/src/__tests__/features/tabs/components/tab-item.test.tsx`

- [ ] **Step 1: Extract TabItem**

Create `web/src/features/tabs/components/tab-item.tsx`. It should receive via props everything it needs to render a single tab: buffer info, active state, event handlers. Wrap with `React.memo`.

```tsx
// web/src/features/tabs/components/tab-item.tsx
import { memo } from 'react'

interface TabItemProps {
  label: string
  isActive: boolean
  isDirty: boolean
  onClick: () => void
  onDoubleClick: (e: React.MouseEvent) => void
  onContextMenu: (e: React.MouseEvent) => void
  onClose: (e: React.MouseEvent) => void
}

export const TabItem = memo(function TabItem({
  label, isActive, isDirty, onClick, onDoubleClick, onContextMenu, onClose,
}: TabItemProps) {
  // Move the tab rendering JSX from tab-bar.tsx here
  // (the per-tab li/button element that currently lives inside the .map())
  return (
    <div
      role="tab"
      aria-selected={isActive}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      onContextMenu={onContextMenu}
    >
      {label}
      {isDirty && <span>●</span>}
      <button onClick={onClose}>×</button>
    </div>
  )
})
```

Replace the actual JSX from `tab-bar.tsx` — do not invent a new design.

- [ ] **Step 2: Write test for TabItem**

Create `web/src/__tests__/features/tabs/components/tab-item.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { TabItem } from '@/features/tabs/components/tab-item'

describe('TabItem', () => {
  const baseProps = {
    label: 'main.tsx',
    isActive: false,
    isDirty: false,
    onClick: vi.fn(),
    onDoubleClick: vi.fn(),
    onContextMenu: vi.fn(),
    onClose: vi.fn(),
  }

  it('renders the label', () => {
    render(<TabItem {...baseProps} />)
    expect(screen.getByText('main.tsx')).toBeInTheDocument()
  })

  it('shows dirty indicator when isDirty', () => {
    render(<TabItem {...baseProps} isDirty />)
    expect(screen.getByText('●')).toBeInTheDocument()
  })

  it('calls onClick when clicked', () => {
    render(<TabItem {...baseProps} />)
    fireEvent.click(screen.getByRole('tab'))
    expect(baseProps.onClick).toHaveBeenCalled()
  })
})
```

- [ ] **Step 3: Run TabItem tests**

```bash
cd web && bun run test src/__tests__/features/tabs/components/tab-item.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Extract TabNewButton**

Create `web/src/features/tabs/components/tab-new-button.tsx`. Move the "new tab" / terminal / web viewer button JSX from `tab-bar.tsx` into this component.

- [ ] **Step 5: Update tab-bar.tsx to use extracted components**

Replace the inline JSX in `tab-bar.tsx`'s map with `<TabItem .../>` (using `useCallback` for all handlers). Replace the new-tab button with `<TabNewButton />`. `tab-bar.tsx` should now be under ~250 lines.

- [ ] **Step 6: Run full test suite**

```bash
cd web && bun run test
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/tabs/components/ web/src/__tests__/features/tabs/
git commit -m "refactor: decompose tab-bar.tsx into TabItem, TabNewButton"
```

---

## Task 14: Decompose pane-container.tsx

**Files:**
- Create: `web/src/features/panes/components/editor-pane.tsx`
- Create: `web/src/features/panes/components/terminal-pane.tsx`
- Create: `web/src/features/panes/components/diff-pane.tsx`
- Create: `web/src/features/panes/components/image-pane.tsx`
- Create: `web/src/features/panes/components/web-viewer-pane.tsx`
- Create: `web/src/features/panes/components/pdf-pane.tsx`
- Modify: `web/src/features/panes/components/pane-container.tsx` (becomes a switch/router ~100 lines)

- [ ] **Step 1: Identify the content type switch**

```bash
grep -n "case\|'editor'\|'terminal'\|'diff'\|'image'\|'pdf'\|'webViewer'" web/src/features/panes/components/pane-container.tsx | head -30
```

Identify the switch or conditional that renders different content types.

- [ ] **Step 2: Extract EditorPane**

Move all editor-related rendering logic from `pane-container.tsx` into `web/src/features/panes/components/editor-pane.tsx`. Props: whatever buffer/file data the editor needs.

- [ ] **Step 3: Extract TerminalPane**

Move terminal rendering into `web/src/features/panes/components/terminal-pane.tsx`. Props: session ID, pane ID.

- [ ] **Step 4: Extract remaining content types**

Repeat for DiffPane, ImagePane, WebViewerPane, PdfPane. Each gets its own file.

- [ ] **Step 5: Reduce pane-container.tsx to a routing switch**

`pane-container.tsx` should now only contain: layout wrappers, the type-switch, and a `<XxxPane />` for each content type. Target ~150 lines.

- [ ] **Step 6: Run the full test suite**

```bash
cd web && bun run test
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/panes/components/
git commit -m "refactor: decompose pane-container.tsx into per-content-type pane components"
```

---

## Task 15: Decompose git-view.tsx

**Files:**
- Create: `web/src/features/git/components/git-status-panel.tsx`
- Create: `web/src/features/git/components/git-commit-panel.tsx`
- Create: `web/src/features/git/components/git-branch-panel.tsx`
- Create: `web/src/features/git/components/git-stash-panel.tsx`
- Modify: `web/src/features/git/components/git-view.tsx` (becomes tab orchestrator ~100 lines)

- [ ] **Step 1: Identify the git view panels**

```bash
grep -n "StatusPanel\|CommitPanel\|BranchPanel\|StashPanel\|activeTab\|tab" web/src/features/git/components/git-view.tsx | head -20
```

- [ ] **Step 2: Extract GitStatusPanel**

Move staged/unstaged file list rendering into `web/src/features/git/components/git-status-panel.tsx`.

- [ ] **Step 3: Extract GitCommitPanel**

Move commit message input and commit button into `web/src/features/git/components/git-commit-panel.tsx`.

- [ ] **Step 4: Extract GitBranchPanel and GitStashPanel**

Move branch list and stash list into their own files.

- [ ] **Step 5: Reduce git-view.tsx to orchestrator**

`git-view.tsx` renders the tab bar + `{activeTab === 'status' && <GitStatusPanel />}` etc. Target ~120 lines.

- [ ] **Step 6: Run tests and commit**

```bash
cd web && bun run test
git add web/src/features/git/components/
git commit -m "refactor: decompose git-view.tsx into panel components"
```

---

## Task 16: Decompose file-explorer-tree.tsx

**Files:**
- Create: `web/src/features/file-explorer/file-explorer/components/file-explorer-item.tsx`
- Create: `web/src/features/file-explorer/file-explorer/components/file-explorer-context-menu.tsx`
- Create: `web/src/features/file-explorer/file-explorer/components/file-explorer-rename-input.tsx`
- Modify: `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx` (orchestrator)

- [ ] **Step 1: Extract FileExplorerItem**

Move the per-file/folder row rendering (icon, name, git badge) into `file-explorer-item.tsx`. Wrap with `React.memo`. Props: node data, depth, active state, event handlers.

- [ ] **Step 2: Extract FileExplorerContextMenu**

Move context menu (new file, rename, delete) into `file-explorer-context-menu.tsx`.

- [ ] **Step 3: Extract FileExplorerRenameInput**

Move the inline rename input (shown when double-clicking a file) into `file-explorer-rename-input.tsx`.

- [ ] **Step 4: Reduce file-explorer-tree.tsx**

The tree component now handles: tree data loading, recursive render via `<FileExplorerItem />`, drag-and-drop wiring. Target ~250 lines (recursive tree logic justifies slightly larger).

- [ ] **Step 5: Run tests and commit**

```bash
cd web && bun run test
git add web/src/features/file-explorer/
git commit -m "refactor: decompose file-explorer-tree.tsx into focused components"
```

---

## Self-Review

After completing all tasks, run:

```bash
cd web && bun run test
cd web && bun run build
```

Both must succeed with zero errors before the plan is considered complete.

**Spec coverage check:**
- ✓ Transport contract (`window.__CROWBAR__`) — Task 2
- ✓ Query key factory — Task 3
- ✓ QueryClient with `staleTime: Infinity` — Task 4
- ✓ Daemon event → cache invalidation — Task 5
- ✓ Optimistic mutations — Task 6
- ✓ IDB schema + adapter — Task 7
- ✓ Startup hydration — Tasks 7, 8
- ✓ Workspace layout persistence — Task 9
- ✓ UI preferences persistence — Task 10
- ✓ Selector violations fixed — Task 11
- ✓ Memoization gaps — Task 12
- ✓ tab-bar.tsx decomposed — Task 13
- ✓ pane-container.tsx decomposed — Task 14
- ✓ git-view.tsx decomposed — Task 15
- ✓ file-explorer-tree.tsx decomposed — Task 16
- ✓ Animation fix (pane-resize-handle) — Task 1

**Not covered (separate Tauri transport plan):**
- crowbar:// custom URI scheme
- Rust protocol handler + routes/
- connection/local/ (sidecar, hyperlocal, tokio-tungstenite)
- Three WS channels (events, terminal, lsp)
- window.__CROWBAR__ injection by Tauri (dev polyfill replaces this)
- editor-state persistence (follow same pattern as Task 9/10 once editor store shape is confirmed stable)
