# Local-First Sync & Failure Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace TanStack Query with a local-first data layer (`Loadable<T>` + IndexedDB source-of-truth + store-owned WebSocket sync + optimistic mutations) and a unified failure-handling UI, eliminating the silent-void failure class found in the chaos sweep.

**Architecture:** UI reads only from Zustand stores via narrow selectors. Each domain store holds a `Loadable<T>` and inherits IDB read/write + WS sync + optimistic-write from a shared `createLoadableSlice` factory. IndexedDB is the source of truth; HTTP/WS are sync peers. A single `<DataState>` component renders every loadable (spinner / inline error / stale-banner + content). React Query is removed last.

**Tech Stack:** TypeScript, React, Zustand (+ slice composition), `idb`, existing `apiFetch` + `wsManager` primitives, Vitest + jsdom + fake-indexeddb + Testing Library.

**Spec:** `docs/superpowers/specs/2026-06-01-local-first-sync-error-handling-design.md`

**Conventions (from CLAUDE.md):**
- Test files mirror source under `web/src/__tests__/`; use `@/` imports.
- Component files kebab-case; exported component PascalCase.
- Narrow selectors `useStore(s => s.field)`; `getState()` only in handlers/effects; stores never import from `components/`.
- Run a single test file: `cd web && npx vitest run src/__tests__/path/to/file.test.ts`
- All commands below assume CWD is `web/` unless stated.

---

## Phase 0 — Foundation

### Task 1: `Loadable<T>` state machine

**Files:**
- Create: `web/src/lib/loadable.ts`
- Test: `web/src/__tests__/lib/loadable.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/loadable.test.ts
import { describe, it, expect } from 'vitest'
import { idle, loading, success, failed, dataOf, fetchedAtOf } from '@/lib/loadable'

describe('loadable', () => {
  it('idle has no data', () => {
    expect(dataOf(idle())).toBeUndefined()
    expect(fetchedAtOf(idle())).toBeUndefined()
  })

  it('success carries data and fetchedAt', () => {
    const l = success([1, 2], 1000)
    expect(l.status).toBe('success')
    expect(dataOf(l)).toEqual([1, 2])
    expect(fetchedAtOf(l)).toBe(1000)
  })

  it('loading from a previous success carries stale data', () => {
    const l = loading(success(['x'], 500))
    expect(l.status).toBe('loading')
    expect(dataOf(l)).toEqual(['x'])
    expect(fetchedAtOf(l)).toBe(500)
  })

  it('loading from idle has no stale data', () => {
    const l = loading(idle())
    expect(dataOf(l)).toBeUndefined()
  })

  it('failed preserves stale data from previous success', () => {
    const l = failed(new Error('boom'), success(['y'], 700))
    expect(l.status).toBe('error')
    expect(l.error.message).toBe('boom')
    expect(dataOf(l)).toEqual(['y'])
    expect(fetchedAtOf(l)).toBe(700)
  })

  it('failed from loading-with-stale keeps the stale data', () => {
    const l = failed(new Error('x'), loading(success(['z'], 900)))
    expect(dataOf(l)).toEqual(['z'])
    expect(fetchedAtOf(l)).toBe(900)
  })

  it('failed from idle has null stale data', () => {
    const l = failed(new Error('x'), idle())
    expect(l.status === 'error' && l.staleData).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/loadable.test.ts`
Expected: FAIL — "Failed to resolve import '@/lib/loadable'".

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/loadable.ts
export type Loadable<T> =
  | { status: 'idle' }
  | { status: 'loading'; staleData?: T; staleAt?: number }
  | { status: 'success'; data: T; fetchedAt: number }
  | { status: 'error'; error: Error; staleData: T | null; staleAt: number | null }

export const idle = (): Loadable<never> => ({ status: 'idle' })

export const loading = <T>(prev?: Loadable<T>): Loadable<T> => ({
  status: 'loading',
  staleData: dataOf(prev),
  staleAt: fetchedAtOf(prev),
})

export const success = <T>(data: T, at: number = Date.now()): Loadable<T> => ({
  status: 'success',
  data,
  fetchedAt: at,
})

export const failed = <T>(error: Error, prev: Loadable<T>): Loadable<T> => ({
  status: 'error',
  error,
  staleData: dataOf(prev) ?? null,
  staleAt: fetchedAtOf(prev) ?? null,
})

export const dataOf = <T>(l?: Loadable<T>): T | undefined => {
  if (!l) return undefined
  if (l.status === 'success') return l.data
  if (l.status === 'loading' || l.status === 'error') return l.staleData ?? undefined
  return undefined
}

export const fetchedAtOf = <T>(l?: Loadable<T>): number | undefined => {
  if (!l) return undefined
  if (l.status === 'success') return l.fetchedAt
  if (l.status === 'loading' || l.status === 'error') return l.staleAt ?? undefined
  return undefined
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/loadable.test.ts`
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/loadable.ts web/src/__tests__/lib/loadable.test.ts
git commit -m "feat: add Loadable<T> state machine"
```

---

### Task 2: IndexedDB schema — new stores + drop query-cache

**Files:**
- Modify: `web/src/lib/persistence/schemas.ts`
- Modify: `web/src/lib/persistence/idb.ts`
- Test: `web/src/__tests__/lib/persistence/idb-schema.test.ts`

No data migration (pre-production, zero users). Set the schema to its end state; bump version to 5 only because IndexedDB requires a version increment to run `upgrade()`. Drop `query-cache` in the same upgrade.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/persistence/idb-schema.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { getDB, resetDB } from '@/lib/persistence/idb'

beforeEach(() => { resetDB() })

describe('idb schema v5', () => {
  it('creates the six new local-first stores', async () => {
    const db = await getDB()
    const names = Array.from(db.objectStoreNames)
    expect(names).toContain('workspaces-data')
    expect(names).toContain('git-data')
    expect(names).toContain('file-tree-data')
    expect(names).toContain('branch-review-data')
    expect(names).toContain('chat-history')
    expect(names).toContain('projects-data')
  })

  it('drops the query-cache store', async () => {
    const db = await getDB()
    expect(Array.from(db.objectStoreNames)).not.toContain('query-cache')
  })

  it('round-trips a record through git-data', async () => {
    const db = await getDB()
    await db.put('git-data', { key: '/repo', data: { n: 1 }, fetchedAt: 42 })
    const rec = await db.get('git-data', '/repo')
    expect(rec?.fetchedAt).toBe(42)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/persistence/idb-schema.test.ts`
Expected: FAIL — `workspaces-data` not in objectStoreNames.

- [ ] **Step 3: Add schema types**

In `web/src/lib/persistence/schemas.ts`, add this generic record type after the existing interfaces (before `export interface CrowbarDB`):

```ts
export interface CachedRecord<T> {
  key: string
  data: T
  fetchedAt: number
}
```

Then inside `CrowbarDB`, **remove** the `'query-cache'` block and **add** the six new stores:

```ts
  'workspaces-data': {
    key: string
    value: CachedRecord<unknown>
  }
  'git-data': {
    key: string
    value: CachedRecord<unknown>
  }
  'file-tree-data': {
    key: string
    value: CachedRecord<unknown>
  }
  'branch-review-data': {
    key: string
    value: CachedRecord<unknown>
  }
  'chat-history': {
    key: string
    value: CachedRecord<unknown>
  }
  'projects-data': {
    key: string
    value: CachedRecord<unknown>
  }
```

- [ ] **Step 4: Update the upgrade callback**

In `web/src/lib/persistence/idb.ts`, change the version to `5` and append a v5 branch that creates the new stores and drops `query-cache`:

```ts
  _db = await openDB<CrowbarDB>('crowbar', 5, {
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
      if (oldVersion < 4) {
        db.createObjectStore('branch-review', { keyPath: 'wsId' })
      }
      if (oldVersion < 5) {
        if (db.objectStoreNames.contains('query-cache')) {
          db.deleteObjectStore('query-cache')
        }
        for (const name of [
          'workspaces-data', 'git-data', 'file-tree-data',
          'branch-review-data', 'chat-history', 'projects-data',
        ] as const) {
          db.createObjectStore(name, { keyPath: 'key' })
        }
      }
    },
  })
```

> Note: the v1 branch still creates `query-cache` then v5 deletes it; this is correct for a fresh DB (both run in sequence) and keeps each historical branch intact.

- [ ] **Step 5: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/persistence/idb-schema.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/persistence/schemas.ts web/src/lib/persistence/idb.ts web/src/__tests__/lib/persistence/idb-schema.test.ts
git commit -m "feat: idb schema for local-first cache stores, drop query-cache"
```

---

### Task 3: Generic cache persistence helper

**Files:**
- Create: `web/src/lib/persistence/cache-store.ts`
- Test: `web/src/__tests__/lib/persistence/cache-store.test.ts`

A single typed helper used by every domain store, instead of one bespoke module per store.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/persistence/cache-store.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache, loadCache } from '@/lib/persistence/cache-store'

beforeEach(() => { resetDB() })

describe('cache-store', () => {
  it('returns undefined when nothing cached', async () => {
    expect(await loadCache('projects-data', 'projects')).toBeUndefined()
  })

  it('saves and loads a record with fetchedAt', async () => {
    await saveCache('projects-data', 'projects', [{ id: 'a' }], 123)
    const rec = await loadCache<{ id: string }[]>('projects-data', 'projects')
    expect(rec?.data).toEqual([{ id: 'a' }])
    expect(rec?.fetchedAt).toBe(123)
  })

  it('overwrites an existing key', async () => {
    await saveCache('projects-data', 'projects', [{ id: 'a' }], 1)
    await saveCache('projects-data', 'projects', [{ id: 'b' }], 2)
    const rec = await loadCache<{ id: string }[]>('projects-data', 'projects')
    expect(rec?.data).toEqual([{ id: 'b' }])
    expect(rec?.fetchedAt).toBe(2)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/persistence/cache-store.test.ts`
Expected: FAIL — cannot resolve `@/lib/persistence/cache-store`.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/persistence/cache-store.ts
import { getDB } from './idb'
import type { CrowbarDB, CachedRecord } from './schemas'

type CacheStoreName =
  | 'workspaces-data' | 'git-data' | 'file-tree-data'
  | 'branch-review-data' | 'chat-history' | 'projects-data'

export async function saveCache<T>(
  store: CacheStoreName,
  key: string,
  data: T,
  fetchedAt: number = Date.now(),
): Promise<void> {
  const db = await getDB()
  await db.put(store, { key, data, fetchedAt } as CrowbarDB[CacheStoreName]['value'])
}

export async function loadCache<T>(
  store: CacheStoreName,
  key: string,
): Promise<CachedRecord<T> | undefined> {
  const db = await getDB()
  const rec = await db.get(store, key)
  return rec as CachedRecord<T> | undefined
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/persistence/cache-store.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/persistence/cache-store.ts web/src/__tests__/lib/persistence/cache-store.test.ts
git commit -m "feat: generic IDB cache-store helper"
```

---

### Task 4: `createLoadableSlice` factory

**Files:**
- Create: `web/src/lib/store/loadable-slice.ts`
- Test: `web/src/__tests__/lib/store/loadable-slice.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/loadable-slice.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { create } from 'zustand'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache } from '@/lib/persistence/cache-store'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { dataOf } from '@/lib/loadable'

beforeEach(() => { resetDB() })

function makeStore(fetcher: (key: string) => Promise<number[]>) {
  return create<LoadableSlice<number[]>>()((set, get) =>
    createLoadableSlice<number[]>({
      store: 'projects-data',
      fetcher,
    })(set, get),
  )
}

describe('createLoadableSlice', () => {
  it('fetch writes IDB and sets success', async () => {
    const store = makeStore(async () => [1, 2, 3])
    await store.getState().fetch('projects')
    expect(store.getState().data.status).toBe('success')
    expect(dataOf(store.getState().data)).toEqual([1, 2, 3])
    const { loadCache } = await import('@/lib/persistence/cache-store')
    expect((await loadCache('projects-data', 'projects'))?.data).toEqual([1, 2, 3])
  })

  it('fetch failure preserves stale data from IDB', async () => {
    await saveCache('projects-data', 'projects', [9, 9], 100)
    const store = makeStore(async () => { throw new Error('boom') })
    await store.getState().fetch('projects')
    expect(store.getState().data.status).toBe('error')
    expect(dataOf(store.getState().data)).toEqual([9, 9])
  })

  it('optimisticWrite commits on success', async () => {
    const store = makeStore(async () => [1])
    await store.getState().optimisticWrite([7], async () => [7])
    expect(dataOf(store.getState().data)).toEqual([7])
  })

  it('optimisticWrite rolls back and rethrows on failure', async () => {
    const store = makeStore(async () => [1])
    await store.getState().fetch('projects')   // data = [1]
    await expect(
      store.getState().optimisticWrite([7], async () => { throw new Error('no') }),
    ).rejects.toThrow('no')
    expect(dataOf(store.getState().data)).toEqual([1])  // rolled back
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/store/loadable-slice.test.ts`
Expected: FAIL — cannot resolve `@/lib/store/loadable-slice`.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/store/loadable-slice.ts
import { idle, loading, success, failed, type Loadable } from '@/lib/loadable'
import { saveCache, loadCache } from '@/lib/persistence/cache-store'
import { wsManager } from '@/lib/ws/manager'

type CacheStoreName =
  | 'workspaces-data' | 'git-data' | 'file-tree-data'
  | 'branch-review-data' | 'chat-history' | 'projects-data'

export interface LoadableSlice<T, K extends unknown[] = [string]> {
  data: Loadable<T>
  fetch: (...args: K) => Promise<void>
  startSync: (...args: K) => () => void
  applyDelta: (event: unknown, ...args: K) => Promise<void>
  optimisticWrite: (optimistic: T, commit: () => Promise<T | void>) => Promise<void>
}

interface LoadableConfig<T, K extends unknown[]> {
  store: CacheStoreName
  fetcher: (...args: K) => Promise<T>
  cacheKey?: (...args: K) => string
  wsEndpoint?: (...args: K) => string
}

type Setter<T> = (partial: Partial<LoadableSlice<T, never[]>>) => void
type Getter<T> = () => LoadableSlice<T, never[]>

export function createLoadableSlice<T, K extends unknown[] = [string]>(
  cfg: LoadableConfig<T, K>,
) {
  const keyOf = (...args: K) =>
    cfg.cacheKey ? cfg.cacheKey(...args) : (args[0] as string)

  return (set: Setter<T>, get: Getter<T>): LoadableSlice<T, K> => ({
    data: idle() as Loadable<T>,

    fetch: async (...args: K) => {
      const cached = await loadCache<T>(cfg.store, keyOf(...args))
      set({
        data: loading(cached ? success(cached.data, cached.fetchedAt) : get().data),
      })
      try {
        const fresh = await cfg.fetcher(...args)
        await saveCache(cfg.store, keyOf(...args), fresh)
        set({ data: success(fresh) })
      } catch (err) {
        set({ data: failed(err as Error, get().data) })
      }
    },

    startSync: (...args: K) => {
      if (!cfg.wsEndpoint) return () => {}
      return wsManager.subscribe(cfg.wsEndpoint(...args), (event) => {
        void get().applyDelta(event, ...(args as unknown[]))
      })
    },

    applyDelta: async (_event: unknown, ...args: K) => {
      await get().fetch(...args)
    },

    optimisticWrite: async (optimistic: T, commit: () => Promise<T | void>) => {
      const prev = get().data
      set({ data: success(optimistic) })
      try {
        const confirmed = await commit()
        if (confirmed !== undefined) set({ data: success(confirmed) })
      } catch (err) {
        set({ data: prev })
        throw err
      }
    },
  })
}
```

> The `Setter`/`Getter` aliases use `never[]` to keep the factory assignable to any concrete store's `create` signature; the public `LoadableSlice<T, K>` return type preserves the real argument tuple for consumers.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/store/loadable-slice.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/loadable-slice.ts web/src/__tests__/lib/store/loadable-slice.test.ts
git commit -m "feat: createLoadableSlice factory (fetch/sync/optimistic)"
```

---

### Task 5: `<InlineError>` and `<StaleBanner>`

**Files:**
- Create: `web/src/components/ui/inline-error.tsx`
- Create: `web/src/components/ui/stale-banner.tsx`
- Test: `web/src/__tests__/components/ui/inline-error.test.tsx`
- Test: `web/src/__tests__/components/ui/stale-banner.test.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/components/ui/inline-error.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { InlineError } from '@/components/ui/inline-error'

describe('InlineError', () => {
  it('shows the failure heading and calls onRetry', () => {
    const onRetry = vi.fn()
    render(<InlineError error={new Error('nope')} onRetry={onRetry} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledOnce()
  })
})
```

```tsx
// web/src/__tests__/components/ui/stale-banner.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { StaleBanner } from '@/components/ui/stale-banner'

describe('StaleBanner', () => {
  it('shows cached label and a retry control when not refreshing', () => {
    const onRetry = vi.fn()
    render(<StaleBanner at={Date.now() - 60_000} onRetry={onRetry} isRefreshing={false} />)
    expect(screen.getByText(/cached data/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('shows Refreshing… and no retry button while refreshing', () => {
    render(<StaleBanner at={Date.now()} onRetry={() => {}} isRefreshing />)
    expect(screen.getByText(/refreshing/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npx vitest run src/__tests__/components/ui/inline-error.test.tsx src/__tests__/components/ui/stale-banner.test.tsx`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement `inline-error.tsx`**

```tsx
// web/src/components/ui/inline-error.tsx
import { Button } from '@/components/ui/button'

interface InlineErrorProps {
  error: Error
  onRetry: () => void
  title?: string
}

export function InlineError({ error, onRetry, title = 'Failed to load' }: InlineErrorProps) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
      <span className="text-lg opacity-50">⚠</span>
      <p className="text-sm font-medium text-foreground">{title}</p>
      {import.meta.env.DEV && (
        <p className="font-mono text-[11px] text-muted-foreground">{error.message}</p>
      )}
      <Button variant="outline" size="sm" onClick={onRetry} className="mt-1">
        ↺ Retry
      </Button>
    </div>
  )
}
```

- [ ] **Step 4: Implement `stale-banner.tsx`**

```tsx
// web/src/components/ui/stale-banner.tsx
import { formatRelativeDate } from '@/utils/date'

interface StaleBannerProps {
  at: number | null
  onRetry: () => void
  isRefreshing: boolean
}

export function StaleBanner({ at, onRetry, isRefreshing }: StaleBannerProps) {
  return (
    <div className="flex items-center gap-2 border-b border-amber-900/40 bg-amber-950/20 px-3 py-1 text-[11px] text-amber-500/90">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" />
      <span>
        Showing cached data
        {at ? ` · last updated ${formatRelativeDate(new Date(at).toISOString())}` : ''}
      </span>
      {isRefreshing ? (
        <span className="opacity-70">· Refreshing…</span>
      ) : (
        <button type="button" onClick={onRetry} className="underline hover:text-amber-300">
          Retry
        </button>
      )}
    </div>
  )
}
```

> Verify `formatRelativeDate` accepts an ISO string (it does in `commits-tab.tsx`). If it accepts a number directly, pass `at` instead of converting.

- [ ] **Step 5: Run tests to verify they pass**

Run: `npx vitest run src/__tests__/components/ui/inline-error.test.tsx src/__tests__/components/ui/stale-banner.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ui/inline-error.tsx web/src/components/ui/stale-banner.tsx web/src/__tests__/components/ui/inline-error.test.tsx web/src/__tests__/components/ui/stale-banner.test.tsx
git commit -m "feat: InlineError and StaleBanner components"
```

---

### Task 6: `<DataState>` component

**Files:**
- Create: `web/src/components/ui/data-state.tsx`
- Test: `web/src/__tests__/components/ui/data-state.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/ui/data-state.test.tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DataState } from '@/components/ui/data-state'
import { idle, loading, success, failed } from '@/lib/loadable'

const renderList = (data: string[]) => <ul>{data.map(d => <li key={d}>{d}</li>)}</ul>

describe('DataState', () => {
  it('idle renders nothing', () => {
    const { container } = render(<DataState loadable={idle()} onRetry={() => {}}>{renderList}</DataState>)
    expect(container).toBeEmptyDOMElement()
  })

  it('loading with no stale shows a spinner', () => {
    render(<DataState loadable={loading(idle())} onRetry={() => {}} loadingLabel="Loading items">{renderList}</DataState>)
    expect(screen.getByLabelText('Loading items')).toBeInTheDocument()
  })

  it('error with no stale shows inline error', () => {
    render(<DataState loadable={failed(new Error('x'), idle())} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })

  it('success renders children with data', () => {
    render(<DataState loadable={success(['a', 'b'])} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText('a')).toBeInTheDocument()
    expect(screen.getByText('b')).toBeInTheDocument()
  })

  it('error with stale shows banner and renders stale content', () => {
    const l = failed(new Error('x'), success(['cached'], 1000))
    render(<DataState loadable={l} onRetry={() => {}}>{renderList}</DataState>)
    expect(screen.getByText(/cached data/i)).toBeInTheDocument()
    expect(screen.getByText('cached')).toBeInTheDocument()
  })

  it('success with empty data and emptyMessage shows the empty message', () => {
    render(<DataState loadable={success<string[]>([])} onRetry={() => {}} emptyMessage="Nothing here">{renderList}</DataState>)
    expect(screen.getByText('Nothing here')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/components/ui/data-state.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```tsx
// web/src/components/ui/data-state.tsx
import type { ReactNode } from 'react'
import { type Loadable, dataOf, fetchedAtOf } from '@/lib/loadable'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { InlineError } from '@/components/ui/inline-error'
import { StaleBanner } from '@/components/ui/stale-banner'

interface DataStateProps<T> {
  loadable: Loadable<T>
  onRetry: () => void
  children: (data: T) => ReactNode
  loadingLabel?: string
  emptyMessage?: string
  isEmpty?: (data: T) => boolean
}

export function DataState<T>({
  loadable, onRetry, children, loadingLabel, emptyMessage, isEmpty,
}: DataStateProps<T>) {
  const stale = dataOf(loadable)

  if (stale === undefined) {
    if (loadable.status === 'loading') return <LoadingSpinner label={loadingLabel} />
    if (loadable.status === 'error') return <InlineError error={loadable.error} onRetry={onRetry} />
    return null
  }

  const data = loadable.status === 'success' ? loadable.data : stale
  const showBanner = loadable.status !== 'success'
  const empty = (isEmpty ?? ((d: T) => Array.isArray(d) && d.length === 0))(data)

  return (
    <>
      {showBanner && (
        <StaleBanner
          at={fetchedAtOf(loadable) ?? null}
          onRetry={onRetry}
          isRefreshing={loadable.status === 'loading'}
        />
      )}
      {empty && emptyMessage ? (
        <p className="p-6 text-center text-sm text-muted-foreground">{emptyMessage}</p>
      ) : (
        children(data)
      )}
    </>
  )
}
```

> Confirm `LoadingSpinner` accepts a `label` prop (it does — used in `git-view.tsx`). Confirm it renders an element queryable by `getByLabelText(label)`; if not, add `aria-label={label}` wrapper.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/components/ui/data-state.test.tsx`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ui/data-state.tsx web/src/__tests__/components/ui/data-state.test.tsx
git commit -m "feat: DataState component (loading/error/stale/success)"
```

---

### Task 7: `useRetry` hook

**Files:**
- Create: `web/src/lib/store/use-retry.ts`
- Test: `web/src/__tests__/lib/store/use-retry.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/use-retry.test.ts
import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { create } from 'zustand'
import { useRetry } from '@/lib/store/use-retry'

describe('useRetry', () => {
  it('returns a stable callback that calls fetch with the given args', () => {
    const fetchSpy = vi.fn()
    const useStore = create<{ fetch: (k: string) => void }>()(() => ({ fetch: fetchSpy }))
    const { result } = renderHook(() => useRetry(useStore, 'repo-1'))
    result.current()
    expect(fetchSpy).toHaveBeenCalledWith('repo-1')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/store/use-retry.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/store/use-retry.ts
import { useCallback } from 'react'
import type { StoreApi, UseBoundStore } from 'zustand'

interface HasFetch<K extends unknown[]> {
  fetch: (...args: K) => unknown
}

export function useRetry<S extends HasFetch<K>, K extends unknown[]>(
  useStore: UseBoundStore<StoreApi<S>>,
  ...args: K
): () => void {
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useCallback(() => { void useStore.getState().fetch(...args) }, [useStore, ...args])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/store/use-retry.test.ts`
Expected: PASS (1 test).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/use-retry.ts web/src/__tests__/lib/store/use-retry.test.ts
git commit -m "feat: useRetry hook"
```

---

## Phase 1 — Simple stores + call-site swaps

### Task 8: Projects store (extend with loadable slice)

**Files:**
- Modify: `web/src/lib/store/projects.ts`
- Test: `web/src/__tests__/lib/store/projects-loadable.test.ts`

The existing `useProjectStore` keeps its local `projects`/`activeProjectId` UI state. Add a **separate** loadable store `useProjectDataStore` for server data, so we don't break existing consumers mid-migration. Call sites move to the data store in Task 12.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/projects-loadable.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'

vi.mock('@/lib/api', () => ({ fetchProjects: vi.fn(async () => [{ id: 'p1', name: 'P1', path: '/p1' }]) }))

beforeEach(() => { resetDB() })

describe('useProjectDataStore', () => {
  it('fetch populates loadable success', async () => {
    const { useProjectDataStore } = await import('@/lib/store/projects')
    await useProjectDataStore.getState().fetch()
    expect(useProjectDataStore.getState().data.status).toBe('success')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/store/projects-loadable.test.ts`
Expected: FAIL — `useProjectDataStore` is not exported.

- [ ] **Step 3: Add the data store**

Append to `web/src/lib/store/projects.ts`:

```ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { fetchProjects } from '@/lib/api'
import type { Project } from '@/lib/types'

export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    fetcher: () => fetchProjects(),
    cacheKey: () => 'projects',
  })(set, get),
)
```

> If `projects.ts` already imports `create` or `Project`, reuse the existing imports rather than duplicating.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/store/projects-loadable.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/projects.ts web/src/__tests__/lib/store/projects-loadable.test.ts
git commit -m "feat: project data store (loadable)"
```

---

### Task 9: Workspace-list store

**Files:**
- Create: `web/src/lib/store/workspace-list.ts`
- Test: `web/src/__tests__/lib/store/workspace-list.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/workspace-list.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => [{ id: 'r1', name: 'repo', workspaces: [] }]) }))

beforeEach(() => { resetDB() })

describe('useWorkspaceListStore', () => {
  it('fetch loads repos into loadable', async () => {
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()
    expect(dataOf(useWorkspaceListStore.getState().data)).toEqual([{ id: 'r1', name: 'repo', workspaces: [] }])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/lib/store/workspace-list.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/lib/store/workspace-list.ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { Repo } from '@/lib/store/sidebar'

export const useWorkspaceListStore = create<LoadableSlice<Repo[], []>>()((set, get) =>
  createLoadableSlice<Repo[], []>({
    store: 'workspaces-data',
    fetcher: () => apiFetch<Repo[]>('/api/v0/workspaces'),
    cacheKey: () => 'workspaces',
    wsEndpoint: () => '/api/v0/ws/workspaces',
  })(set, get),
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/lib/store/workspace-list.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/workspace-list.ts web/src/__tests__/lib/store/workspace-list.test.ts
git commit -m "feat: workspace-list data store (loadable)"
```

---

### Task 10: File-tree store

**Files:**
- Create: `web/src/features/files/stores/file-tree-store.ts`
- Test: `web/src/__tests__/features/files/stores/file-tree-store.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/files/stores/file-tree-store.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => ({ name: 'root', children: [] })) }))

beforeEach(() => { resetDB() })

describe('useFileTreeStore', () => {
  it('fetch loads the tree for a root path', async () => {
    const { useFileTreeStore } = await import('@/features/files/stores/file-tree-store')
    await useFileTreeStore.getState().fetch('/root')
    expect(dataOf(useFileTreeStore.getState().data)).toEqual({ name: 'root', children: [] })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/features/files/stores/file-tree-store.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/features/files/stores/file-tree-store.ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { FileNode } from '@/lib/mock/files'

export const useFileTreeStore = create<LoadableSlice<FileNode>>()((set, get) =>
  createLoadableSlice<FileNode>({
    store: 'file-tree-data',
    fetcher: (rootPath: string) =>
      apiFetch<FileNode>(`/api/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
    wsEndpoint: (rootPath: string) =>
      `/api/v0/ws/files?workspaceId=${encodeURIComponent(rootPath)}`,
  })(set, get),
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/features/files/stores/file-tree-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/files/stores/file-tree-store.ts web/src/__tests__/features/files/stores/file-tree-store.test.ts
git commit -m "feat: file-tree data store (loadable)"
```

---

### Task 11: Chat-history store

**Files:**
- Create: `web/src/features/markdown-chat/stores/chat-store.ts`
- Test: `web/src/__tests__/features/markdown-chat/stores/chat-store.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/markdown-chat/stores/chat-store.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => [{ id: 't1', role: 'user', content: 'hi' }]) }))

beforeEach(() => { resetDB() })

describe('useChatStore', () => {
  it('fetch loads chat turns keyed by chatId', async () => {
    const { useChatStore } = await import('@/features/markdown-chat/stores/chat-store')
    await useChatStore.getState().fetch('chat-ws3-0')
    expect(dataOf(useChatStore.getState().data)).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/features/markdown-chat/stores/chat-store.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```ts
// web/src/features/markdown-chat/stores/chat-store.ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

export const useChatStore = create<LoadableSlice<MarkdownTurn[]>>()((set, get) =>
  createLoadableSlice<MarkdownTurn[]>({
    store: 'chat-history',
    fetcher: (chatId: string) =>
      apiFetch<MarkdownTurn[]>(`/api/v0/markdown-chat/${chatId}/initial`),
    wsEndpoint: (chatId: string) => `/api/v0/ws/chat/${chatId}`,
  })(set, get),
)
```

> The existing markdown-chat query used `(wsId, stepId)`. For the cold-start history load the chat view passes a single `chatId`; the `stepId` segment is fixed to `initial` here. If the chat view needs a real `stepId`, change the key tuple to `[string, string]` and `cacheKey: (id, step) => \`${id}:${step}\``. Confirm against `markdown-chat-view.tsx` during Task 13.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/features/markdown-chat/stores/chat-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/stores/chat-store.ts web/src/__tests__/features/markdown-chat/stores/chat-store.test.ts
git commit -m "feat: chat-history data store (loadable)"
```

---

### Task 12: Swap Projects call site to DataState

**Files:**
- Modify: `web/src/components/projects/ProjectListPage.tsx`
- Test: `web/src/__tests__/components/projects/project-list-page.test.tsx`

This is the **critical wrong-empty-state fix** for projects.

- [ ] **Step 1: Read the current component**

Run: `sed -n '1,60p' web/src/components/projects/ProjectListPage.tsx` — note how `projects` is consumed and where `projects.length === 0` renders the onboarding state.

- [ ] **Step 2: Write the failing test**

```tsx
// web/src/__tests__/components/projects/project-list-page.test.tsx
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { failed, success } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { ProjectListPage } from '@/components/projects/ProjectListPage'

beforeEach(() => { useProjectDataStore.setState({ data: success([]) }) })

describe('ProjectListPage error vs empty', () => {
  it('shows inline error (not onboarding) when the fetch failed with no cache', () => {
    useProjectDataStore.setState({ data: failed(new Error('500'), { status: 'idle' }) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    expect(screen.queryByText(/no projects yet/i)).toBeNull()
  })

  it('shows onboarding when the fetch succeeded but list is empty', () => {
    useProjectDataStore.setState({ data: success([]) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText(/no projects yet/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `npx vitest run src/__tests__/components/projects/project-list-page.test.tsx`
Expected: FAIL — error case still shows onboarding (or component throws because it reads `useProjectStore`).

- [ ] **Step 4: Rewire the component**

In `ProjectListPage.tsx`: keep `useProjectStore` for `activeProjectId`/`setActiveProject` if used, but drive the list + empty/error from the data store. Add at top of the component:

```tsx
import { DataState } from '@/components/ui/data-state'
import { useRetry } from '@/lib/store/use-retry'
import { useProjectDataStore } from '@/lib/store/projects'
```

Replace the `projects.length === 0 ? (onboarding) : (list)` block with:

```tsx
const projectsLoadable = useProjectDataStore(s => s.data)
const retryProjects = useRetry(useProjectDataStore)

return (
  <DataState
    loadable={projectsLoadable}
    onRetry={retryProjects}
    loadingLabel="Loading projects"
    emptyMessage="No projects yet"
  >
    {(projects) => (
      <div className="...existing list wrapper classes...">
        {projects.map(project => (
          /* ...existing project row markup, unchanged... */
        ))}
      </div>
    )}
  </DataState>
)
```

Keep the existing "Import project" button outside `DataState` (in the page header) so it shows in every state. The onboarding "No projects yet" body becomes the `emptyMessage`; if the onboarding had an Import CTA in the body, render it via a custom empty branch instead of `emptyMessage` (use `isEmpty` + conditional children).

- [ ] **Step 5: Trigger the fetch on mount**

Ensure the data store is populated. Add near the top of the component:

```tsx
import { useEffect } from 'react'
// ...
useEffect(() => { void useProjectDataStore.getState().fetch() }, [])
```

- [ ] **Step 6: Run test to verify it passes**

Run: `npx vitest run src/__tests__/components/projects/project-list-page.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 7: Commit**

```bash
git add web/src/components/projects/ProjectListPage.tsx web/src/__tests__/components/projects/project-list-page.test.tsx
git commit -m "fix: projects page distinguishes load error from empty"
```

---

### Task 13: Swap remaining simple call sites (commits, chat history, file tree)

**Files:**
- Modify: `web/src/features/branch-review/components/commits-tab.tsx`
- Modify: `web/src/features/markdown-chat/components/markdown-chat-view.tsx`
- Modify: file-tree consumer (the sidebar Files tab tree component; locate via `fileTreeQueryOptions` usage)
- Test: `web/src/__tests__/features/branch-review/commits-tab.test.tsx`

This task swaps the `useQuery`-based simple reads to the new stores + `DataState`. Git data is migrated in Phase 2; here `commits-tab` moves to the new `useGitStore` only after Task 15 — so in this task migrate **chat history** and **file tree**, and write the commits test as part of Task 15. (Ordering note: do chat + file-tree here.)

- [ ] **Step 1: Find the file-tree consumer**

Run: `grep -rl "fileTreeQueryOptions" web/src --include=*.tsx`
Note the component path; call it `<FileTreeConsumer>` below.

- [ ] **Step 2: Migrate markdown-chat-view**

In `markdown-chat-view.tsx`, replace the `useQuery(markdownChatQueryOptions(...))` cold-start history load:

```tsx
import { DataState } from '@/components/ui/data-state'
import { useRetry } from '@/lib/store/use-retry'
import { useChatStore } from '@/features/markdown-chat/stores/chat-store'
// ...
const historyLoadable = useChatStore(s => s.data)
const retryHistory = useRetry(useChatStore, chatId)
useEffect(() => { void useChatStore.getState().fetch(chatId) }, [chatId])
```

Wrap the rendered history list in:

```tsx
<DataState loadable={historyLoadable} onRetry={retryHistory} loadingLabel="Loading conversation" emptyMessage="No messages yet">
  {(turns) => /* existing turn-rendering JSX over `turns` */}
</DataState>
```

Confirm the `chatId`/`stepId` shape matches Task 11's note; adjust the store key tuple if a real `stepId` is required.

- [ ] **Step 3: Migrate the file-tree consumer**

In `<FileTreeConsumer>`, replace `useQuery(fileTreeQueryOptions(rootPath))` with:

```tsx
import { DataState } from '@/components/ui/data-state'
import { useRetry } from '@/lib/store/use-retry'
import { useFileTreeStore } from '@/features/files/stores/file-tree-store'
// ...
const treeLoadable = useFileTreeStore(s => s.data)
const retryTree = useRetry(useFileTreeStore, rootPath)
useEffect(() => { void useFileTreeStore.getState().fetch(rootPath) }, [rootPath])
```

Wrap the tree render in `<DataState loadable={treeLoadable} onRetry={retryTree} loadingLabel="Loading files">{(tree) => /* existing tree JSX */}</DataState>`.

- [ ] **Step 4: Manual verification (no unit test for tree UI here)**

Run: `npx vitest run` to confirm nothing else broke.
Expected: PASS (existing suite green).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/components/markdown-chat-view.tsx web/src/features/files/
git commit -m "refactor: chat history and file tree read from loadable stores"
```

---

## Phase 2 — Complex stores

### Task 14: Git data store (slice + git-specific actions)

**Files:**
- Modify: `web/src/features/git/stores/git-store.ts`
- Test: `web/src/__tests__/features/git/git-data-store.test.ts`

Add a `data: Loadable<GitData>` driven by the loadable slice while keeping existing git-store actions. `GitData = { status, commits, branches, stashes }`.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/git/git-data-store.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/features/git/api/git-status-api', () => ({ getGitStatus: vi.fn(async () => ({ branch: 'main', files: [] })), initRepository: vi.fn() }))
vi.mock('@/features/git/api/git-commits-api', () => ({ getGitLog: vi.fn(async () => [{ hash: 'abc', message: 'm', date: '2026-01-01' }]) }))
vi.mock('@/features/git/api/git-branches-api', () => ({ getBranches: vi.fn(async () => ['main']) }))
vi.mock('@/features/git/api/git-stash-api', () => ({ getStashes: vi.fn(async () => []) }))

beforeEach(() => { resetDB() })

describe('git data store', () => {
  it('fetchGitData aggregates status/commits/branches/stashes into loadable', async () => {
    const { useGitStore } = await import('@/features/git/stores/git-store')
    await useGitStore.getState().fetchGitData('/repo')
    const data = dataOf(useGitStore.getState().gitData)
    expect(data?.status.branch).toBe('main')
    expect(data?.commits).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npx vitest run src/__tests__/features/git/git-data-store.test.ts`
Expected: FAIL — `gitData`/`fetchGitData` not present.

- [ ] **Step 3: Add the loadable slice to the git store**

In `git-store.ts`, define the aggregate type and add a loadable field + actions. Add near the top:

```ts
import { createLoadableSlice } from '@/lib/store/loadable-slice'
import type { Loadable } from '@/lib/loadable'
import { getGitStatus } from '../api/git-status-api'
import { getGitLog } from '../api/git-commits-api'
import { getBranches } from '../api/git-branches-api'
import { getStashes } from '../api/git-stash-api'
import type { GitStatus, GitCommit, GitStash } from '../types/git-types'

export interface GitData {
  status: GitStatus
  commits: GitCommit[]
  branches: string[]
  stashes: GitStash[]
}

async function fetchAllGitData(repoPath: string): Promise<GitData> {
  const [status, commits, branches, stashes] = await Promise.all([
    getGitStatus(repoPath), getGitLog(repoPath, 50, 0), getBranches(repoPath), getStashes(repoPath),
  ])
  return { status, commits, branches, stashes }
}
```

Inside the `create(...)` store object, spread the slice (renaming its `fetch` to `fetchGitData` to avoid colliding with any existing action) and expose `gitData`:

```ts
// within create<GitState>()((set, get) => ({ ...existing fields...,
...((() => {
  const slice = createLoadableSlice<GitData>({
    store: 'git-data',
    fetcher: fetchAllGitData,
    wsEndpoint: (repoPath: string) => `/api/v0/ws/git?repo=${encodeURIComponent(repoPath)}`,
  })(
    (partial) => set(partial as Partial<GitState>),
    () => ({ data: get().gitData } as never),
  )
  return {
    gitData: slice.data,
    fetchGitData: slice.fetch,
    startGitSync: slice.startSync,
    applyGitDelta: slice.applyDelta,
  }
})()),
```

Add `gitData: Loadable<GitData>; fetchGitData: (repoPath: string) => Promise<void>; startGitSync: (repoPath: string) => () => void; applyGitDelta: (e: unknown, repoPath: string) => Promise<void>` to the `GitState` interface.

> The adapter wires the slice's `set`/`get` to the host store's `gitData` field. Because the slice writes `{ data }`, map it to `gitData` via the setter/getter shims above. Verify the existing `GitState` interface name; adjust if different.

- [ ] **Step 4: Run test to verify it passes**

Run: `npx vitest run src/__tests__/features/git/git-data-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/stores/git-store.ts web/src/__tests__/features/git/git-data-store.test.ts
git commit -m "feat: git data store loadable slice"
```

---

### Task 15: Commits tab + git-view read from git data store

**Files:**
- Modify: `web/src/features/branch-review/components/commits-tab.tsx`
- Modify: `web/src/features/git/components/git-view.tsx`
- Test: `web/src/__tests__/features/branch-review/commits-tab.test.tsx`
- Test: `web/src/__tests__/features/git/git-view-error.test.tsx`

This contains the **critical git wrong-empty-state fix**.

- [ ] **Step 1: Write the commits-tab failing test**

```tsx
// web/src/__tests__/features/branch-review/commits-tab.test.tsx
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { success, failed, loading, idle } from '@/lib/loadable'
import { useGitStore } from '@/features/git/stores/git-store'
import { CommitsTab } from '@/features/branch-review/components/commits-tab'

const gitData = (commits: any[]) => ({ status: { branch: 'main', files: [] }, commits, branches: [], stashes: [] })

beforeEach(() => { useGitStore.setState({ gitData: idle() } as any) })

describe('CommitsTab', () => {
  it('renders commits on success', () => {
    useGitStore.setState({ gitData: success(gitData([{ hash: 'abcdef0', message: 'hello', date: '2026-01-01' }])) } as any)
    render(<CommitsTab repoPath="/repo" />)
    expect(screen.getByText('hello')).toBeInTheDocument()
  })

  it('shows inline error when no cache and fetch failed', () => {
    useGitStore.setState({ gitData: failed(new Error('500'), idle()) } as any)
    render(<CommitsTab repoPath="/repo" />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/branch-review/commits-tab.test.tsx`
Expected: FAIL — component still uses `useQuery`.

- [ ] **Step 3: Rewrite commits-tab**

```tsx
// web/src/features/branch-review/components/commits-tab.tsx
import { useEffect } from 'react'
import { formatRelativeDate } from '@/utils/date'
import { FramePanel, FrameTitle } from '@/components/ui/frame'
import { DataState } from '@/components/ui/data-state'
import { useRetry } from '@/lib/store/use-retry'
import { useGitStore } from '@/features/git/stores/git-store'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const gitData = useGitStore(s => s.gitData)
  const retry = useRetry(useGitStore, repoPath)  // NOTE: see Step 4 about fetchGitData

  useEffect(() => { void useGitStore.getState().fetchGitData(repoPath) }, [repoPath])

  return (
    <div className="flex flex-col gap-4">
      <FrameTitle className="text-base">Commit history</FrameTitle>
      <DataState loadable={gitData} onRetry={retry} loadingLabel="Loading commits" emptyMessage="No commits yet"
                 isEmpty={(d) => d.commits.length === 0}>
        {({ commits }) => (
          <div className="flex flex-col gap-1.5">
            {commits.map(commit => (
              <FramePanel key={commit.hash} className="py-2 px-3">
                <div className="flex items-center gap-3">
                  <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">{commit.hash.slice(0, 7)}</span>
                  <span className="flex-1 truncate text-sm text-foreground">{commit.message}</span>
                  <span className="shrink-0 text-xs text-muted-foreground/50">{formatRelativeDate(commit.date)}</span>
                </div>
              </FramePanel>
            ))}
          </div>
        )}
      </DataState>
    </div>
  )
}
```

- [ ] **Step 4: Fix `useRetry` target for git**

`useRetry(useGitStore, repoPath)` calls `useGitStore.getState().fetch(...)`, but git's action is named `fetchGitData`. Add an overload-free wrapper: change the retry line to:

```tsx
import { useCallback } from 'react'
// ...
const retry = useCallback(() => { void useGitStore.getState().fetchGitData(repoPath) }, [repoPath])
```

Remove the `useRetry` import from this file.

- [ ] **Step 5: Run commits-tab test**

Run: `npx vitest run src/__tests__/features/branch-review/commits-tab.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 6: Write the git-view error-vs-empty test**

```tsx
// web/src/__tests__/features/git/git-view-error.test.tsx
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { failed, idle } from '@/lib/loadable'
import { useGitStore } from '@/features/git/stores/git-store'
import { useRepositoryStore } from '@/features/git/stores/git-repository-store'
import GitView from '@/features/git/components/git-view'

describe('GitView error vs empty', () => {
  beforeEach(() => {
    useGitStore.setState({ gitData: idle() } as any)
  })

  it('shows inline error (not "Not a Git repository") when status fetch failed', () => {
    useRepositoryStore.setState({ activeRepoPath: '/repo' } as any)
    useGitStore.setState({ gitData: failed(new Error('500'), idle()) } as any)
    render(<GitView repoPath="/repo" />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    expect(screen.queryByText(/not a git repository/i)).toBeNull()
  })
})
```

> If `useRepositoryStore` uses the `.use.activeRepoPath()` auto-selector pattern, set state via its documented setter instead of `setState`. Inspect `git-repository-store.ts` and adapt the test's state seeding accordingly.

- [ ] **Step 7: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/git/git-view-error.test.tsx`
Expected: FAIL — error path still renders "Not a Git repository".

- [ ] **Step 8: Disambiguate git-view error vs empty**

In `git-view.tsx`, read the loadable status: `const gitData = useGitStore(s => s.gitData)`. Replace the `if (!gitStatus) { return <SidebarEmptyActionState message="Not a Git repository" .../> }` block (around line 500) with status-aware branches:

```tsx
import { DataState } from '@/components/ui/data-state'
import { dataOf } from '@/lib/loadable'
// ...
// Only a genuinely absent repo shows the "Not a Git repository" CTA:
if (!activeRepoPath) {
  return (/* existing "No repository selected" empty action state, unchanged */)
}

// Failed fetch with no cache → inline error, NOT the Initialize CTA:
if (gitData.status === 'error' && dataOf(gitData) === undefined) {
  return (
    <SidebarPanel className="gap-2 p-2">
      <InlineError error={gitData.error} onRetry={() => void useGitStore.getState().fetchGitData(activeRepoPath)} />
    </SidebarPanel>
  )
}

if (gitData.status === 'loading' && dataOf(gitData) === undefined) {
  return (/* existing "Loading Git status..." empty state, unchanged */)
}
```

Keep the rest of the component (which reads `gitStatus` from the store) working; `gitStatus` continues to be set by existing actions during the transition. Import `InlineError` and `SidebarPanel` as needed.

- [ ] **Step 9: Run to verify it passes**

Run: `npx vitest run src/__tests__/features/git/git-view-error.test.tsx`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/features/branch-review/components/commits-tab.tsx web/src/features/git/components/git-view.tsx web/src/__tests__/features/branch-review/commits-tab.test.tsx web/src/__tests__/features/git/git-view-error.test.tsx
git commit -m "fix: commits tab and git view read loadable; error != empty"
```

---

### Task 16: Branch-review data store (diff + chats) and About/Diff swaps

**Files:**
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts`
- Create: `web/src/features/branch-review/stores/branch-review-data-store.ts`
- Modify: `web/src/features/branch-review/components/branch-review-pane.tsx`
- Modify: `web/src/features/branch-review/components/about-tab.tsx`
- Test: `web/src/__tests__/features/branch-review/branch-review-data-store.test.ts`

Keep threads/description in the existing branch-review slice (already IDB-persisted and working). Add a separate loadable store for `{ diff, chats }` (the server reads that fail silently today).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/branch-review/branch-review-data-store.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'

vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn(async (path: string) =>
    path.includes('/diff') ? { files: [] } : [{ id: 'c1', title: 'chat' }]),
}))

beforeEach(() => { resetDB() })

describe('branch-review data store', () => {
  it('fetches diff and chats together', async () => {
    const { useBranchReviewDataStore } = await import('@/features/branch-review/stores/branch-review-data-store')
    await useBranchReviewDataStore.getState().fetch('ws3')
    const data = dataOf(useBranchReviewDataStore.getState().data)
    expect(data?.diff).toEqual({ files: [] })
    expect(data?.chats).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/branch-review/branch-review-data-store.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the data store**

```ts
// web/src/features/branch-review/stores/branch-review-data-store.ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'

export interface BranchReviewData {
  diff: MultiFileDiff
  chats: BranchReviewChat[]
}

async function fetchBranchReviewData(wsId: string): Promise<BranchReviewData> {
  const [diff, chats] = await Promise.all([
    apiFetch<MultiFileDiff>(`/api/v0/branch-review/${wsId}/diff`),
    apiFetch<BranchReviewChat[]>(`/api/v0/branch-review/${wsId}/chats`),
  ])
  return { diff, chats }
}

export const useBranchReviewDataStore = create<LoadableSlice<BranchReviewData>>()((set, get) =>
  createLoadableSlice<BranchReviewData>({
    store: 'branch-review-data',
    fetcher: fetchBranchReviewData,
  })(set, get),
)
```

- [ ] **Step 4: Run to verify it passes**

Run: `npx vitest run src/__tests__/features/branch-review/branch-review-data-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Swap branch-review-pane and about-tab reads**

In `branch-review-pane.tsx`, replace `useQuery(branchDiffQueryOptions(wsId))` and `useQuery(branchChatsQueryOptions(wsId))` with the data store + `DataState`, mirroring Task 15's pattern (effect calls `fetch(wsId)`, render via `DataState`, `isEmpty` per sub-view). In `about-tab.tsx`, the chats list reads from `useBranchReviewDataStore(s => s.data)`. Description + threads continue to come from the existing branch-review slice (unchanged).

- [ ] **Step 6: Run the full suite**

Run: `npx vitest run`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/branch-review/
git commit -m "feat: branch-review diff/chats loadable store + DataState swaps"
```

---

### Task 17: Refactor git-blame-store to `Loadable`

**Files:**
- Modify: `web/src/features/git/stores/git-blame-store.ts`
- Test: `web/src/__tests__/features/git/git-blame-store.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/git/git-blame-store.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { dataOf } from '@/lib/loadable'

vi.mock('@/features/git/api/git-blame-api', () => ({
  getGitBlame: vi.fn(async () => ({ lines: [{ line_number: 1, total_lines: 1 }] })),
}))

describe('git-blame-store loadable', () => {
  beforeEach(async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    useGitBlameStore.setState({ blame: new Map() } as any)
  })

  it('loads blame into a per-file Loadable', async () => {
    const { useGitBlameStore } = await import('@/features/git/stores/git-blame-store')
    await useGitBlameStore.getState().loadBlameForFile('/repo', 'a.ts')
    const entry = useGitBlameStore.getState().blame.get('a.ts')
    expect(entry?.status).toBe('success')
    expect(dataOf(entry)?.lines).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/git/git-blame-store.test.ts`
Expected: FAIL — `blame` map / `Loadable` shape not present.

- [ ] **Step 3: Refactor the store**

Replace the three Maps (`blameData`, `isLoading`, `errors`) with a single `blame: Map<string, Loadable<GitBlame>>`. Keep `fileToRepo`. Rewrite `loadBlameForFile`:

```ts
import { create } from 'zustand'
import { getGitBlame } from '../api/git-blame-api'
import type { GitBlame, GitBlameLine } from '../types/git-types'
import { loading, success, failed, idle, dataOf, type Loadable } from '@/lib/loadable'

interface GitBlameState {
  blame: Map<string, Loadable<GitBlame>>
  fileToRepo: Map<string, string>
  loadBlameForFile: (repoPath: string, filePath: string) => Promise<void>
  clearBlameForFile: (filePath: string) => void
  clearAllBlame: () => void
  getBlameForLine: (filePath: string, lineNumber: number) => GitBlameLine | null
  getRepoPath: (filePath: string) => string | null
}

export const useGitBlameStore = create<GitBlameState>((set, get) => ({
  blame: new Map(),
  fileToRepo: new Map(),

  loadBlameForFile: async (repoPath, filePath) => {
    const current = get().blame.get(filePath)
    if (current && (current.status === 'success' || current.status === 'loading')) return
    set({ blame: new Map(get().blame).set(filePath, loading(current ?? idle())) })
    try {
      const data = await getGitBlame(repoPath, filePath)
      if (!data) throw new Error('Failed to load blame data')
      set({
        blame: new Map(get().blame).set(filePath, success(data)),
        fileToRepo: new Map(get().fileToRepo).set(filePath, repoPath),
      })
    } catch (err) {
      set({ blame: new Map(get().blame).set(filePath, failed(err as Error, current ?? idle())) })
    }
  },

  clearBlameForFile: (filePath) => {
    const blame = new Map(get().blame); blame.delete(filePath)
    const fileToRepo = new Map(get().fileToRepo); fileToRepo.delete(filePath)
    set({ blame, fileToRepo })
  },

  clearAllBlame: () => set({ blame: new Map(), fileToRepo: new Map() }),

  getBlameForLine: (filePath, lineNumber) => {
    const data = dataOf(get().blame.get(filePath))
    if (!data) return null
    for (const line of data.lines) {
      const start = line.line_number
      const end = start + line.total_lines - 1
      if (lineNumber >= start && lineNumber <= end) return line
    }
    return null
  },

  getRepoPath: (filePath) => get().fileToRepo.get(filePath) ?? null,
}))
```

- [ ] **Step 4: Update blame consumers**

Run: `grep -rl "useGitBlameStore" web/src --include=*.tsx --include=*.ts | grep -v __tests__`
For each consumer reading `isLoading`/`errors`/`blameData`, switch to `blame.get(path)` + `dataOf`/`status`. Update accordingly (the public methods `getBlameForLine`/`getRepoPath` are unchanged).

- [ ] **Step 5: Run to verify it passes + full suite**

Run: `npx vitest run src/__tests__/features/git/git-blame-store.test.ts && npx vitest run`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/git/stores/git-blame-store.ts web/src/__tests__/features/git/git-blame-store.test.ts web/src/features/git/
git commit -m "refactor: git-blame-store uses per-file Loadable"
```

---

## Phase 3 — Sync wiring

### Task 18: Sync hooks + AppSyncProvider

**Files:**
- Create: `web/src/features/git/hooks/use-git-sync.ts`
- Create: `web/src/features/files/hooks/use-file-tree-sync.ts`
- Create: `web/src/components/app-sync-provider.tsx`
- Modify: app root (locate the top-level layout that wraps routes; e.g. `web/src/routes/__root.tsx`)
- Test: `web/src/__tests__/features/git/use-git-sync.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/features/git/use-git-sync.test.tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useGitStore } from '@/features/git/stores/git-store'
import { useGitSync } from '@/features/git/hooks/use-git-sync'

describe('useGitSync', () => {
  beforeEach(() => { vi.restoreAllMocks() })

  it('starts sync on mount and unsubscribes on unmount', () => {
    const unsub = vi.fn()
    const startSpy = vi.spyOn(useGitStore.getState(), 'startGitSync').mockReturnValue(unsub)
    const { unmount } = renderHook(() => useGitSync('/repo'))
    expect(startSpy).toHaveBeenCalledWith('/repo')
    unmount()
    expect(unsub).toHaveBeenCalledOnce()
  })

  it('does nothing when repoPath is undefined', () => {
    const startSpy = vi.spyOn(useGitStore.getState(), 'startGitSync')
    renderHook(() => useGitSync(undefined))
    expect(startSpy).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/git/use-git-sync.test.tsx`
Expected: FAIL — hook not found.

- [ ] **Step 3: Implement the hooks**

```ts
// web/src/features/git/hooks/use-git-sync.ts
import { useEffect } from 'react'
import { useGitStore } from '@/features/git/stores/git-store'

export function useGitSync(repoPath: string | undefined) {
  useEffect(() => {
    if (!repoPath) return
    return useGitStore.getState().startGitSync(repoPath)
  }, [repoPath])
}
```

```ts
// web/src/features/files/hooks/use-file-tree-sync.ts
import { useEffect } from 'react'
import { useFileTreeStore } from '@/features/files/stores/file-tree-store'

export function useFileTreeSync(rootPath: string | undefined) {
  useEffect(() => {
    if (!rootPath) return
    return useFileTreeStore.getState().startSync(rootPath)
  }, [rootPath])
}
```

- [ ] **Step 4: Implement AppSyncProvider**

```tsx
// web/src/components/app-sync-provider.tsx
import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'

export function AppSyncProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    void useWorkspaceListStore.getState().fetch()
    void useProjectDataStore.getState().fetch()
    const unsubs = [
      useWorkspaceListStore.getState().startSync(),
      useProjectDataStore.getState().startSync?.() ?? (() => {}),
    ]
    return () => unsubs.forEach(u => u())
  }, [])
  return <>{children}</>
}
```

> `useProjectDataStore` has no `wsEndpoint` (no `/ws/projects` yet), so `startSync` returns a no-op unsubscribe — safe to call. The `?.()` guard is belt-and-suspenders.

- [ ] **Step 5: Mount AppSyncProvider at the app root**

Run: `grep -rn "Outlet\|RouterProvider\|<App" web/src/routes/__root.tsx web/src/main.tsx` to find the wrap point. Wrap the routed content: `<AppSyncProvider><Outlet /></AppSyncProvider>` (or around the existing top-level layout). Add the import.

- [ ] **Step 6: Wire `useGitSync` into git-view and `useFileTreeSync` into the file tree consumer**

In `git-view.tsx`, add `useGitSync(activeRepoPath)` near the top of the component. In `<FileTreeConsumer>`, add `useFileTreeSync(rootPath)`.

- [ ] **Step 7: Run tests**

Run: `npx vitest run src/__tests__/features/git/use-git-sync.test.tsx && npx vitest run`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/git/hooks/use-git-sync.ts web/src/features/files/hooks/use-file-tree-sync.ts web/src/components/app-sync-provider.tsx web/src/routes/__root.tsx web/src/features/git/components/git-view.tsx web/src/__tests__/features/git/use-git-sync.test.tsx
git commit -m "feat: store-owned WebSocket sync hooks + AppSyncProvider"
```

---

## Phase 4 — Optimistic mutations

### Task 19: Optimistic review-thread mutations

**Files:**
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts`
- Test: `web/src/__tests__/features/branch-review/optimistic-threads.test.ts`

Review-thread CRUD already writes locally; make the write path explicitly optimistic with rollback when a server call is wired (today the API is a mock — the pattern must be in place and tested with a stubbed commit).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/branch-review/optimistic-threads.test.ts
import { describe, it, expect, vi } from 'vitest'
import { addThreadOptimistic } from '@/features/workspace/stores/slices/branch-review-slice'

describe('addThreadOptimistic', () => {
  it('keeps the thread when the server call succeeds', async () => {
    const apply = vi.fn()
    const rollback = vi.fn()
    await addThreadOptimistic({ apply, rollback, commit: async () => {} })
    expect(apply).toHaveBeenCalledOnce()
    expect(rollback).not.toHaveBeenCalled()
  })

  it('rolls back when the server call fails', async () => {
    const apply = vi.fn()
    const rollback = vi.fn()
    await expect(addThreadOptimistic({ apply, rollback, commit: async () => { throw new Error('x') } })).resolves.toBeUndefined()
    expect(rollback).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `npx vitest run src/__tests__/features/branch-review/optimistic-threads.test.ts`
Expected: FAIL — `addThreadOptimistic` not exported.

- [ ] **Step 3: Implement the helper**

Add to `branch-review-slice.ts`:

```ts
interface OptimisticOp {
  apply: () => void
  rollback: () => void
  commit: () => Promise<void>
}

export async function addThreadOptimistic({ apply, rollback, commit }: OptimisticOp): Promise<void> {
  apply()
  try {
    await commit()
  } catch {
    rollback()
  }
}
```

Wire the existing `addReviewThread`/`addReviewMessage`/`resolveReviewThread` handlers in the slice to route through `addThreadOptimistic` where a server `commit` exists (for now `commit` resolves immediately for mocked endpoints; the structure is what matters). Show a toast on rollback in the **component** that calls these (per CLAUDE.md, stores don't import components): the handler returns a boolean/throws and the component toasts.

- [ ] **Step 4: Run to verify it passes**

Run: `npx vitest run src/__tests__/features/branch-review/optimistic-threads.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/branch-review-slice.ts web/src/__tests__/features/branch-review/optimistic-threads.test.ts
git commit -m "feat: optimistic review-thread mutation with rollback"
```

---

## Phase 5 — Action feedback + remaining critical fixes

### Task 20: File-open failure toast

**Files:**
- Modify: the file-open handler (locate via `openContent` for editor buffers and the file-tree click handler)
- Test: `web/src/__tests__/features/<area>/file-open-error.test.ts` (path depends on handler location)

- [ ] **Step 1: Locate the handler**

Run: `grep -rn "openContent\|fileContentQueryOptions\|/fs/file" web/src --include=*.ts --include=*.tsx | grep -v __tests__`
Identify the function that loads file content when a tree node is opened.

- [ ] **Step 2: Write the failing test** (adapt path to the handler's module)

```ts
import { describe, it, expect, vi } from 'vitest'
import { toast } from '@/components/ui/toast'

vi.mock('@/components/ui/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))
vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(async () => { throw new Error('500') }) }))

describe('openFile failure', () => {
  it('shows an error toast when content load fails', async () => {
    const { openFileSafe } = await import('@/.../file-open')  // adjust to real module
    await openFileSafe('vite.config.ts')
    expect(toast.error).toHaveBeenCalledWith('Failed to open file', expect.objectContaining({ description: 'vite.config.ts' }))
  })
})
```

- [ ] **Step 3: Run to verify it fails** — module/function missing or no toast.

- [ ] **Step 4: Wrap the load**

In the handler, wrap the content fetch:

```ts
import { toast } from '@/components/ui/toast'

export async function openFileSafe(path: string): Promise<void> {
  try {
    const content = await apiFetch<string>(`/api/v0/fs/file?path=${encodeURIComponent(path)}`)
    // ...existing buffer-open logic with `content`...
  } catch {
    toast.error('Failed to open file', { description: path })
  }
}
```

If the existing flow uses `useQuery` for file content, replace it with this imperative fetch on click so failures are catchable (file content is not cached as a domain store per the spec).

- [ ] **Step 5: Run to verify it passes.**

- [ ] **Step 6: Commit**

```bash
git add web/src/<handler> web/src/__tests__/<test>
git commit -m "fix: toast on file-open failure instead of silent no-op"
```

---

### Task 21: Merge commit feedback

**Files:**
- Modify: the merge button/dialog component (locate via `onMerge`/"Merge commit")
- Test: `web/src/__tests__/features/branch-review/merge-feedback.test.tsx`

- [ ] **Step 1: Locate**

Run: `grep -rn "onMerge\|Merge commit\|mergeStrategy" web/src --include=*.tsx | grep -v __tests__`

- [ ] **Step 2: Write the failing test**

```tsx
// web/src/__tests__/features/branch-review/merge-feedback.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { toast } from '@/components/ui/toast'
import { MergeCommitButton } from '@/features/branch-review/components/merge-commit-button' // adjust

vi.mock('@/components/ui/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

describe('MergeCommitButton', () => {
  it('shows success toast on success', async () => {
    render(<MergeCommitButton onMerge={async () => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /merge commit/i }))
    await waitFor(() => expect(toast.success).toHaveBeenCalled())
  })

  it('shows error toast on failure', async () => {
    render(<MergeCommitButton onMerge={async () => { throw new Error('x') }} />)
    fireEvent.click(screen.getByRole('button', { name: /merge commit/i }))
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
  })
})
```

> Adapt the import/props to the real merge component. If merge is triggered from a dialog confirm button, render that and target it.

- [ ] **Step 3: Run to verify it fails.**

- [ ] **Step 4: Add in-flight state + toasts**

In the merge handler:

```tsx
const [merging, setMerging] = useState(false)
async function handleMerge() {
  setMerging(true)
  try {
    await onMerge(/* {title, description} */)
    toast.success('Merged successfully')
  } catch {
    toast.error('Merge failed — check logs')
  } finally {
    setMerging(false)
  }
}
```

Disable the button and show "Merging…" while `merging`.

- [ ] **Step 5: Run to verify it passes.**

- [ ] **Step 6: Commit**

```bash
git add web/src/features/branch-review/ web/src/__tests__/features/branch-review/merge-feedback.test.tsx
git commit -m "fix: merge commit shows in-flight state and success/error toast"
```

---

### Task 22: Workspace sidebar + terminal status

**Files:**
- Modify: workspace sidebar tree consumer (locate via `workspacesQueryOptions` / the Workspaces tab)
- Modify: `web/src/features/terminal/components/terminal-session.tsx` (or the session component showing the black screen)
- Test: `web/src/__tests__/components/layout/workspace-tree-error.test.tsx`

- [ ] **Step 1: Migrate the workspace tree to the loadable store**

Find the Workspaces sidebar tree render (uses repos/workspaces). Swap its data source to `useWorkspaceListStore(s => s.data)` + `DataState` with `loadingLabel="Loading workspaces"` and an inline-error branch (no stale → InlineError). The fetch is already triggered by `AppSyncProvider`.

- [ ] **Step 2: Write the failing test**

```tsx
// web/src/__tests__/components/layout/workspace-tree-error.test.tsx
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { failed, idle } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { WorkspaceTree } from '@/components/layout/workspace-tree' // adjust to real export

beforeEach(() => { useWorkspaceListStore.setState({ data: idle() } as any) })

describe('WorkspaceTree error', () => {
  it('shows inline error when workspaces fail to load with no cache', () => {
    useWorkspaceListStore.setState({ data: failed(new Error('500'), idle()) } as any)
    render(<WorkspaceTree />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 3: Run to verify it fails, then wire DataState, then pass.**

Run: `npx vitest run src/__tests__/components/layout/workspace-tree-error.test.tsx`

- [ ] **Step 4: Terminal connecting/failed state**

In the terminal session component, replace the bare black `xterm` mount with a small status overlay: show "Connecting…" until the session/WS is ready, and "Terminal failed to start" on error. This uses local component state from the terminal session lifecycle (not `Loadable`). Minimal: track a `status: 'connecting' | 'ready' | 'error'` and render a centered message for non-ready states behind/over the terminal element.

- [ ] **Step 5: Run the full suite.**

Run: `npx vitest run`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/ web/src/features/terminal/ web/src/__tests__/components/layout/workspace-tree-error.test.tsx
git commit -m "fix: workspace sidebar error state + terminal connecting/failed status"
```

---

## Phase 6 — Hydration expansion

### Task 23: Hydrate all loadable stores from IDB on startup

**Files:**
- Modify: `web/src/lib/persistence/hydrate.ts`
- Modify: `web/src/main.tsx`
- Test: `web/src/__tests__/lib/persistence/hydrate-all.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/persistence/hydrate-all.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { saveCache } from '@/lib/persistence/cache-store'
import { hydrateAllStores } from '@/lib/persistence/hydrate'
import { useProjectDataStore } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'

beforeEach(() => { resetDB(); useProjectDataStore.setState({ data: { status: 'idle' } } as any) })

describe('hydrateAllStores', () => {
  it('seeds project data store from IDB as stale loading state', async () => {
    await saveCache('projects-data', 'projects', [{ id: 'p1' }], 555)
    await hydrateAllStores()
    expect(dataOf(useProjectDataStore.getState().data)).toEqual([{ id: 'p1' }])
  })
})
```

- [ ] **Step 2: Run to verify it fails** — `hydrateAllStores` not exported.

- [ ] **Step 3: Implement `hydrateAllStores`**

Add to `hydrate.ts`:

```ts
import { loadCache } from './cache-store'
import { loading, success, idle } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'

export async function hydrateAllStores(): Promise<void> {
  const [projects, workspaces] = await Promise.all([
    loadCache('projects-data', 'projects'),
    loadCache('workspaces-data', 'workspaces'),
  ])
  useProjectDataStore.setState({
    data: projects ? loading(success(projects.data as never, projects.fetchedAt)) : idle(),
  })
  useWorkspaceListStore.setState({
    data: workspaces ? loading(success(workspaces.data as never, workspaces.fetchedAt)) : idle(),
  })
}
```

> Only the app-global stores (projects, workspaces) hydrate eagerly here. Per-workspace stores (git, file-tree, chat, branch-review) hydrate lazily via their own `fetch()` (which already reads IDB first) when their component mounts — no central hydration needed.

- [ ] **Step 4: Call it in main.tsx before render**

In `main.tsx`, before `ReactDOM.createRoot(...).render(...)`, add `await hydrateAllStores()` (the entry is already async, alongside existing hydration calls). If the existing bootstrap is not async, wrap render in an async IIFE consistent with the current pattern.

- [ ] **Step 5: Run to verify it passes + full suite.**

Run: `npx vitest run src/__tests__/lib/persistence/hydrate-all.test.ts && npx vitest run`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/persistence/hydrate.ts web/src/main.tsx web/src/__tests__/lib/persistence/hydrate-all.test.ts
git commit -m "feat: hydrate app-global loadable stores from IDB on startup"
```

---

## Phase 7 — Remove TanStack Query

### Task 24: Delete query modules and providers

**Files:**
- Delete: `web/src/lib/queries.ts`, `web/src/features/branch-review/queries.ts`, `web/src/features/markdown-chat/queries.ts`
- Delete: `web/src/lib/persistence/query-persister.ts`
- Modify: `web/src/main.tsx` (remove `PersistQueryClientProvider`/`QueryClientProvider`)
- Modify: `web/src/lib/queries/client.ts` (delete if now unused)

- [ ] **Step 1: Confirm no remaining `useQuery` consumers**

Run: `grep -rn "useQuery\|queryOptions\|QueryClient\|PersistQueryClient" web/src --include=*.ts --include=*.tsx | grep -v __tests__`
Expected: empty (every consumer migrated in Tasks 12–22). If any remain, migrate them using the `DataState` + store pattern before deleting.

- [ ] **Step 2: Remove providers from main.tsx**

Delete the `PersistQueryClientProvider`/`QueryClientProvider` wrappers and their imports; render the app tree directly (keeping `AppSyncProvider`).

- [ ] **Step 3: Delete the query modules**

```bash
git rm web/src/lib/queries.ts web/src/features/branch-review/queries.ts web/src/features/markdown-chat/queries.ts web/src/lib/persistence/query-persister.ts
# delete web/src/lib/queries/client.ts only if grep shows no importers:
grep -rn "queries/client" web/src --include=*.ts --include=*.tsx || git rm web/src/lib/queries/client.ts
```

- [ ] **Step 4: Remove the dependency**

```bash
npm uninstall @tanstack/react-query @tanstack/react-query-persist-client @tanstack/query-async-storage-persister
```

- [ ] **Step 5: Typecheck + full suite + build**

Run: `npx tsc --noEmit && npx vitest run && npm run build`
Expected: clean typecheck, all tests pass, build succeeds. Fix any dangling imports surfaced by `tsc`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove TanStack Query — local-first stores are the data layer"
```

---

### Task 25: Final verification against the chaos sweep

**Files:** none (manual verification).

- [ ] **Step 1: Run the app**

Run: `npm run dev` (from `web/`). Open the app.

- [ ] **Step 2: Re-run the failure sweep**

In Developer settings → Fault Injection, set all faults to 100% (or run the localStorage helper from the QA session). Reload. Walk each flow from the spec's findings table and confirm:
- Workspaces sidebar → inline error or stale data, never blank.
- Commits tab → inline error / stale, never black void.
- Open conversation → error/stale, not a black void.
- File open → error toast, not silent.
- Git sidebar → inline error, NOT "Not a Git repository".
- Projects page → inline error, NOT "No projects yet".
- Merge commit → in-flight state + toast.
- Terminal → "Connecting…"/failed status, not black screen.

- [ ] **Step 3: Reset chaos and confirm happy path still works**

Set all faults to 0, reload, confirm normal operation + stale banners disappear after successful refresh.

- [ ] **Step 4: Commit any fixes found, then done.**

---

## Self-Review notes

- **Spec coverage:** `Loadable<T>` (T1), IDB end-state schema + drop query-cache (T2), generic cache helper (T3), `createLoadableSlice` (T4), `<InlineError>`/`<StaleBanner>`/`<DataState>`/`useRetry` (T5–T7), simple stores (T8–T11) + swaps (T12–T13), git + branch-review stores + critical empty-vs-error fixes (T14–T16), git-blame refactor (T17), store-owned sync + AppSyncProvider (T18), optimistic mutations (T19), action feedback for file-open/merge/terminal/workspace sidebar (T20–T22), hydration (T23), React Query removal (T24), chaos re-verification (T25). All four severity tiers + all three phases covered.
- **No singleton SyncEngine** — replaced by store-owned `startSync` + hooks + `AppSyncProvider`, per the design revision.
- **Type consistency:** `createLoadableSlice` config uses `store`/`fetcher`/`cacheKey`/`wsEndpoint` consistently across T4, T8–T11, T14, T16. `fetchGitData` (not `fetch`) is the git action name everywhere it's referenced (T14, T15, T18). `dataOf`/`fetchedAtOf`/`loading`/`success`/`failed`/`idle` names match T1 throughout.
- **Known adaptation points flagged inline:** exact file-tree consumer, merge component, and file-open handler paths are located via `grep` in their tasks (they weren't pinned during exploration); markdown-chat `stepId` shape confirmed at swap time; `useRepositoryStore` state-seeding pattern verified in T15.
