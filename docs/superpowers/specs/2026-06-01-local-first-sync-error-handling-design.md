# Crowbar Local-First Sync & Failure Handling Design

**Date:** 2026-06-01
**Status:** Approved
**Scope:** Full local-first data architecture — `Loadable<T>` state machine, IndexedDB-as-source-of-truth, store-owned WebSocket sync, optimistic mutations, and a unified failure-handling UI. Replaces TanStack Query as the data layer.

---

## Context

A QA failure-mode sweep (all 12 fault-injection keys + error rate at 100%, via the Developer settings chaos panel) revealed that Crowbar's happy path is solid but its failure path is almost entirely silent. The dominant pattern is the **silent void**: when an API call fails, the UI renders an empty container — no spinner, no error, no retry, no toast. Three variations make it worse:

1. **Wrong empty states** — a failed `git/status` renders "Not a Git repository" (offering a destructive "Initialize Repository" action); a failed `projects` fetch renders the new-user "No projects yet" onboarding. Both actively mislead the user.
2. **Silent action failure** — clicking a file that fails to load does nothing (no tab, no error, an uncaught promise in console); clicking "Merge commit" closes the dialog with zero feedback.
3. **Cache masking** — the About and Diff tabs render from IDB cache and look correct, but the user has no signal that the data may be stale after a network failure.

### Findings catalogue (from the sweep)

| Flow | Fault | Today | Severity |
|---|---|---|---|
| Workspaces sidebar | `workspaces` 500 | Blank panel, no retry | Medium |
| Branch Review: Commits | `git-commits` 500 | "Commit history" heading + black void | Medium |
| Branch Review: About | `branch-description`/`chats` 500 | Renders from IDB cache, no stale signal | Low |
| Branch Review: Diff | `branch-diff` 500 | Renders from IDB cache, no stale signal | Low |
| Open conversation | `markdown-chat` 500 | New tab, black void, compose bar renders | Medium |
| File open (click) | `file-content` 500 | Nothing happens; uncaught promise | High |
| Git sidebar | `git-status` 500 | "Not a Git repository" + Initialize button | **Critical** |
| Projects page | `projects` 500 | "No projects yet" onboarding | **Critical** |
| Select project menu | `projects` 500 | Empty list, no error | **Critical** |
| Merge commit | (stub) | Dialog closes, no feedback | High |
| New Terminal | daemon IPC | Black screen, no status | Medium |
| Review thread reply/resolve | (IDB local) | Works correctly | — (OK) |
| Merge strategy picker | (local UI) | Works correctly | — (OK) |

All four severity tiers are in scope.

### Architectural decision

Rather than bolt error states onto the existing dual-paradigm data layer (TanStack Query for some data, Zustand + manual fetch for the rest), we adopt a **full local-first architecture** modeled on Linear's approach: *the UI reads only from a local store; IndexedDB is the source of truth; the server is a sync peer, not a data source.* This eliminates the silent-void class of bug structurally — there is almost always local data to render — and unifies the two paradigms into one.

This supersedes the TanStack Query cache-persistence portion of `2026-05-30-api-layer-ws-channels-query-cache-design.md`. The REST endpoints, WebSocket channels, MSW mocking, and chaos middleware from that design remain in force; only the client-side query/cache mechanism changes.

### Backend reality (verified)

- WebSocket hubs already exist: `GET /api/v0/ws/workspaces`, `/ws/git?repo=`, `/ws/files?workspaceId=`, `/ws/chat/:chatId`, `/ws/terminal/:sessionId`, `/ws/daemon` (`api/internal/wshub/hub.go`, `api/internal/api/v0/router.go`).
- Event types exist: `WorkspaceEvent {workspaceId, action}`, `GitEvent {repo, changed}`, `FileEvent {workspaceId, path}`, `ChatChunk`, `TerminalFrame`, `DaemonStatus` (`api/internal/fixtures/types.go`).
- **No delta protocol** — events signal "something changed"; payloads are full. Phase 2 `applyDelta` is therefore a targeted re-fetch, not in-place patching. A true delta protocol is a future backend milestone, out of scope here.
- **No auth** currently; not addressed here.
- Branch-review endpoints (`/api/v0/branch-review/:wsId/{diff,threads,description,chats}`) are MSW frontend mocks only.

### Existing primitives to reuse (do not reinvent)

- **`apiFetch<T>(path, init)`** (`web/src/lib/api.ts`) — the HTTP primitive; resolves base URL from `window.__CROWBAR__.api`. All fetchers already route through it (`fetchWorkspace`, `fetchProjects`, etc.).
- **`createWSManager()` → `wsManager`** (`web/src/lib/ws/manager.ts`) — `subscribe(endpoint, cb): () => void` and `send(endpoint, data)`. Already keyed by endpoint string with a callback set, reconnect with exponential backoff (1s → 30s), and last-unsubscribe-closes semantics. **This is the connection dedup/ref-count layer.** The store sync layer wraps it; it does not build its own socket bookkeeping.
- **IDB infra** (`web/src/lib/persistence/idb.ts`, `getDB()`, `schemas.ts`) — `crowbar` database, currently v4. Persistence-module pattern in `lib/persistence/*` wraps all IDB access; stores/components never call IDB directly (except the chaos store).
- **`hydrate.ts`** (`web/src/lib/persistence/hydrate.ts`) — `hydrateWorkspace()`, `hydratePreferences()`, `hydrateSidebar()` run at app start to restore stores from IDB before render.
- **`toast`** (`web/src/components/ui/toast.tsx`) — `toast.success/error`.
- **`ErrorBoundary`** (`web/src/components/ErrorBoundary.tsx`) — visual baseline for `<InlineError>` (destructive/10 background, mono message, "Try again").
- **Zustand slice composition** — the workspace store (`features/workspace/stores/`) already composes `pane-slice`, `buffer-slice`, `branch-review-slice`, etc. `createLoadableSlice` follows this established pattern.

---

## Architecture Overview

Three layers, one source of truth, three async flows.

```
UI — React components
  read-only via narrow selectors · never fetch · never touch IDB
  render Loadable<T> exclusively through <DataState>
        ↕
Store layer — Zustand (replaces TanStack Query entirely)
  one store per domain · each holds Loadable<T> + fetch/applyDelta/optimisticWrite
  simple domains: pure createLoadableSlice factory
  complex domains: createLoadableSlice + domain-specific actions
        ↕
Persistence — IndexedDB (the source of truth)
  end-state schema · stores write on every successful fetch · app reads on every start
        ↕
Network — sync peers (never the source of truth)
  HTTP via apiFetch (pull: initial load, retry)
  WebSocket via wsManager (push: change events → re-fetch → IDB write)
```

### Flow 1 — Startup read (hydration)

```
app boot → read IDB (all domains, concurrent) → set Loadable = loading(staleData)
         → render immediately (stale data visible, or spinner if no cache)
         → HTTP fetch (background) → write IDB → set Loadable = success(fresh)
```

### Flow 2 — Live sync (delta application)

```
WS event (WorkspaceEvent / GitEvent / FileEvent)
  → store.applyDelta(event)  [Phase 1: targeted re-fetch; Phase 2: in-place patch]
  → write IDB → set Loadable = success(fresh)
```

### Flow 3 — Optimistic write (mutation)

```
user action → write IDB immediately (tempId) + set optimistic Loadable state
            → UI responds instantly
            → HTTP POST (async)
                 success → reconcile (replace tempId with server id)
                 failure → rollback IDB + store, toast "Failed to save"
```

---

## Layer 1 — `Loadable<T>` (the universal state machine)

New file: `web/src/lib/loadable.ts`. A discriminated union making impossible states unrepresentable and carrying stale data through every non-fresh state so the IDB fallback is built into the type.

```ts
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
  status: 'success', data, fetchedAt: at,
})

export const failed = <T>(error: Error, prev: Loadable<T>): Loadable<T> => ({
  status: 'error', error,
  staleData: dataOf(prev) ?? null,
  staleAt: fetchedAtOf(prev) ?? null,
})

// Read stale-or-fresh data regardless of status.
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

> `Date.now()` is centralized in the `success` constructor so it can be injected/frozen in tests.

---

## Layer 2 — Persistence (IndexedDB)

Crowbar is pre-production with **zero users**. Per the project's no-legacy-migration rule, we **do not write any migration or data-preservation code**. The schema is set to its desired end state; developers clear their dev IndexedDB (delete the `crowbar` database) to pick it up. The only reason the version number moves at all is the IndexedDB API requirement that the `upgrade()` callback fires only on a version increment — it is a mechanism to create object stores, not a migration ladder.

Add six new object stores. Each stored record is `{ data: T; fetchedAt: number }` so `fetchedAt` survives reloads and feeds the stale banner.

| New store | Key | Holds |
|---|---|---|
| `workspaces-data` | `'workspaces'` (singleton) | Workspace list snapshot |
| `git-data` | `repoPath` | `{ status, commits, branches, stashes }` |
| `file-tree-data` | `rootPath` | File tree snapshot |
| `branch-review-data` | `wsId` | `{ diff, chats }` (description + threads remain in existing `branch-review` store) |
| `chat-history` | `chatId` | Markdown chat turn history |
| `projects-data` | `'projects'` (singleton) | Project list snapshot |

Create these stores in the `upgrade()` callback and **drop `query-cache` in the same upgrade** (`db.deleteObjectStore('query-cache')`) — no users means no reason to preserve it or stage its removal. Set the schema to its end state; no `if (oldVersion < N)` migration branches are needed beyond creating the desired stores. Add matching typed schemas to `lib/persistence/schemas.ts` (`CrowbarDB`). Developers with an existing dev database clear IndexedDB to adopt the new schema.

New persistence-module helpers (one file per store, mirroring `branch-review.ts`/`sidebar-ui.ts`), exposing typed `saveX(key, record)` / `loadX(key)`. The generic slice calls these via a small adapter (see below).

---

## Layer 3 — Store layer

### 3a. The shared slice — `createLoadableSlice<T>`

New file: `web/src/lib/store/loadable-slice.ts`. Written once; provides all infrastructure (IDB read/write, WS subscription via `wsManager`, `Loadable` transitions, optimistic write + rollback). Every domain store inherits it.

```ts
import { idle, loading, success, failed, type Loadable } from '@/lib/loadable'
import { wsManager } from '@/lib/ws/manager'

export interface LoadableSlice<T, K extends unknown[] = [string]> {
  data: Loadable<T>
  fetch: (...args: K) => Promise<void>
  startSync: (...args: K) => () => void   // returns unsubscribe
  applyDelta: (event: unknown, ...args: K) => Promise<void>
  optimisticWrite: (optimistic: T, commit: () => Promise<T | void>) => Promise<void>
}

interface LoadableConfig<T, K extends unknown[]> {
  fetcher: (...args: K) => Promise<T>
  idbLoad: (...args: K) => Promise<{ data: T; fetchedAt: number } | undefined>
  idbSave: (data: T, ...args: K) => Promise<void>
  wsEndpoint?: (...args: K) => string
}

export function createLoadableSlice<T, K extends unknown[] = [string]>(
  cfg: LoadableConfig<T, K>,
) {
  return (set: (p: Partial<{ data: Loadable<T> }>) => void,
          get: () => { data: Loadable<T> }) => ({
    data: idle() as Loadable<T>,

    fetch: async (...args: K) => {
      const cached = await cfg.idbLoad(...args)
      set({ data: loading(cached ? success(cached.data, cached.fetchedAt) : get().data) })
      try {
        const fresh = await cfg.fetcher(...args)
        await cfg.idbSave(fresh, ...args)
        set({ data: success(fresh) })
      } catch (err) {
        set({ data: failed(err as Error, get().data) })
      }
    },

    startSync: (...args: K) => {
      if (!cfg.wsEndpoint) return () => {}
      // wsManager dedups by endpoint string and ref-counts via its callback set.
      return wsManager.subscribe(cfg.wsEndpoint(...args), (event) => {
        void (get() as any).applyDelta(event, ...args)
      })
    },

    applyDelta: async (_event: unknown, ...args: K) => {
      // Phase 1: any change event ⇒ targeted re-fetch.
      // Phase 2: branch on event shape and patch in place (requires backend deltas).
      await (get() as any).fetch(...args)
    },

    optimisticWrite: async (optimistic: T, commit: () => Promise<T | void>) => {
      const prev = get().data
      set({ data: success(optimistic) })
      try {
        const confirmed = await commit()
        if (confirmed !== undefined) set({ data: success(confirmed) })
      } catch (err) {
        set({ data: prev })           // rollback
        throw err                      // caller toasts
      }
    },
  })
}
```

> The `as any` casts on `get()` cross the slice/host boundary (host adds `applyDelta`/`fetch`); they are localized to the slice and covered by tests. Concrete stores type their public surface fully.

### 3b. Domain stores

| Store | File | Approach | Loadable field / data shape |
|---|---|---|---|
| `useWorkspaceListStore` | `lib/store/workspace-list.ts` | **new** — pure factory | `Loadable<Workspace[]>` |
| `useProjectStore` | `lib/store/projects.ts` | **extend** — add factory slice | `Loadable<Project[]>` |
| `useFileTreeStore` | `features/files/stores/file-tree-store.ts` | **new** — pure factory | `Loadable<FileNode>` |
| `useChatStore` | `features/markdown-chat/stores/chat-store.ts` | **new** — pure factory | `Loadable<ChatTurn[]>` |
| `useGitStore` | `features/git/stores/git-store.ts` | **extend** — slice + actions | `Loadable<GitData>` |
| `useBranchReviewStore` | `features/workspace/stores/branch-review-slice.ts` | **extend** — slice + actions | `Loadable<{diff, chats}>` |

**Pure-factory example (projects):**

```ts
export const useProjectStore = create<LoadableSlice<Project[]>>()((set, get) =>
  createLoadableSlice<Project[]>({
    fetcher: () => fetchProjects(),
    idbLoad: () => loadProjectsData(),
    idbSave: (data) => saveProjectsData(data),
    wsEndpoint: () => '/api/v0/ws/projects',  // if/when backend adds it; omit otherwise
  })(set, get),
)
```

**Slice + extensions example (git):** spreads the slice, then adds `loadMoreCommits`, branch/stash helpers, `setCurrentBranch`. Domain actions read/write the same `data: Loadable<GitData>` through the `Loadable` constructors. `GitData = { status, commits, branches, stashes }`; `fetcher` runs the existing `Promise.all([getGitStatus, getGitLog, getBranches, getStashes])` currently in `git-view.tsx`'s `loadInitialGitData`.

**Branch-review extensions:** `addThread`, `resolveThread`, `deleteThread`, `addMessage` become `optimisticWrite` calls (threads/description already persist to the existing `branch-review` IDB store; this preserves that and adds the `{diff, chats}` Loadable). Existing local-only thread CRUD (verified working in the sweep) is retained.

**`git-blame-store` refactor:** convert `blameData/isLoading/errors` Maps to `Map<string, Loadable<GitBlame>>`, eliminating its impossible-state surface (`isLoading && error`). Same `Loadable` helpers; per-file keyed.

### 3c. Selectors & CLAUDE.md compliance

- Components subscribe with narrow selectors: `useGitStore(s => s.data)`. Never bare `useStore()`.
- `getState()` only in event handlers / effects.
- Stores never import from `components/`.

---

## Layer 4 — Sync wiring

WebSocket subscriptions are owned by the components that own the data, via small hooks, so React controls lifecycle. No global singleton `SyncEngine`. `wsManager` already provides connection dedup + ref-count + reconnect by endpoint.

**Per-resource sync hooks** (workspace-scoped data):

```ts
// features/git/hooks/use-git-sync.ts
export function useGitSync(repoPath: string | undefined) {
  useEffect(() => {
    if (!repoPath) return
    return useGitStore.getState().startSync(repoPath)   // unsubscribe on unmount/arg change
  }, [repoPath])
}
```

Equivalent hooks: `useFileTreeSync(rootPath)`, `useBranchReviewSync(wsId)`, `useChatSync(chatId)`.

**App-global sync** (workspace list, projects) via a root provider component using `useEffect` — still no module singleton:

```tsx
// components/app-sync-provider.tsx
export function AppSyncProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    const unsubs = [
      useWorkspaceListStore.getState().startSync(),
      useProjectStore.getState().startSync(),
    ]
    return () => unsubs.forEach(u => u())
  }, [])
  return <>{children}</>
}
```

Mounted near the app root (inside the router, so it lives for the session). On `wsManager` reconnect (it emits `{reconnected: true}` to callbacks), `applyDelta` re-fetches to catch missed events; the slice handler treats any message — including the reconnect sentinel — as a re-fetch trigger.

---

## Layer 5 — UI components

New files in `web/src/components/ui/`. `<DataState>` is the **only** component that reads a `Loadable<T>`.

### `<DataState>` (`data-state.tsx`)

```tsx
interface DataStateProps<T> {
  loadable: Loadable<T>
  onRetry: () => void
  children: (data: T) => ReactNode
  loadingLabel?: string
  emptyMessage?: string
  isEmpty?: (data: T) => boolean   // default: Array.isArray(d) && d.length === 0
}

export function DataState<T>({ loadable, onRetry, children, loadingLabel, emptyMessage, isEmpty }: DataStateProps<T>) {
  const stale = dataOf(loadable)

  if (stale === undefined) {
    if (loadable.status === 'loading') return <LoadingSpinner label={loadingLabel} />
    if (loadable.status === 'error')   return <InlineError error={loadable.error} onRetry={onRetry} />
    return null // idle
  }

  const data = loadable.status === 'success' ? loadable.data : stale
  const showBanner = loadable.status !== 'success'
  const empty = (isEmpty ?? ((d) => Array.isArray(d) && d.length === 0))(data)

  return (
    <>
      {showBanner && (
        <StaleBanner
          at={fetchedAtOf(loadable) ?? null}
          onRetry={onRetry}
          isRefreshing={loadable.status === 'loading'}
        />
      )}
      {empty && emptyMessage ? <EmptyState message={emptyMessage} /> : children(data)}
    </>
  )
}
```

### `<StaleBanner>` (`stale-banner.tsx`)
Amber bar (existing warning token), one line: "Showing cached data · last updated {relative(at)} · Retry". When `isRefreshing`, the action reads "Refreshing…" (no button). Uses `formatRelativeDate`.

### `<InlineError>` (`inline-error.tsx`)
Shown only when there is no stale data. Matches `ErrorBoundary` styling: ⚠ heading "Failed to load", `error.message` in mono (dev builds), "↺ Retry" button → `onRetry`.

### `useRetry` (`web/src/lib/store/use-retry.ts`)
`useRetry(useStore, ...args)` returns `() => useStore.getState().fetch(...args)`, so call sites don't hand-wire retry.

### Call-site transformation (representative — `commits-tab.tsx`)

```tsx
// After
const gitData = useGitStore(s => s.data)
const retry   = useRetry(useGitStore, repoPath)
useGitSync(repoPath)

return (
  <DataState loadable={gitData} onRetry={retry} loadingLabel="Loading commits" emptyMessage="No commits yet">
    {({ commits }) => commits.map(c => <CommitRow key={c.hash} commit={c} />)}
  </DataState>
)
```

### Critical wrong-empty-state fixes

**`git-view.tsx`** — the `!gitStatus` branch (line ~500) currently renders "Not a Git repository" for *both* a genuinely absent repo and a failed fetch. Disambiguate via `useGitStore(s => s.data).status`:
- `!activeRepoPath` → "Not a Git repository" (the only legitimate empty state; keeps Browse/Initialize).
- `status === 'loading'` && no stale → loading.
- `status === 'error'` && no stale → `<InlineError>` (no Initialize button).
- otherwise render git UI (with `<StaleBanner>` when stale).

**`ProjectListPage`** — `projects.length === 0` currently catches empty *and* failed. After: drive from `useProjectStore(s => s.data)`; `status === 'error'` && no stale → `<InlineError>`; success && empty → "No projects yet" onboarding.

### Action feedback (High-severity fixes)

- **File open** (`file-content`): `bufferActions.openContent` (or the file-tree click handler) wraps the load; on failure `toast.error('Failed to open file', { description: path })` and does not leave a half-open buffer.
- **Merge commit**: the button gains in-flight state (spinner + "Merging…", disabled) and `toast.success('Merged successfully')` / `toast.error('Merge failed — check logs')`. Implemented via `optimisticWrite` where a store-backed mutation exists, else a local `useState` one-shot.
- **Terminal**: surface a "Connecting…" state and a failure message instead of a black screen (terminal uses its own session/IPC path, not `Loadable`; minimal state addition in the terminal session component).

---

## Migration sequence (one PR, reviewable commit-by-commit)

The app stays working at every step; React Query and the new stores coexist until the final removal.

1. **Foundation** — `loadable.ts`, `loadable-slice.ts`, IDB schema (new stores + drop `query-cache`) + persistence helpers, `<DataState>`/`<StaleBanner>`/`<InlineError>`/`useRetry`.
2. **Simple stores** — projects, workspace-list, file-tree, chat (pure factory) + swap their call sites to `DataState`.
3. **Complex stores** — git, branch-review (slice + extensions); refactor `git-blame-store` off Maps; swap call sites.
4. **Sync wiring** — `useXxxSync` hooks + `<AppSyncProvider>`; `applyDelta` = re-fetch.
5. **Optimistic mutations** — review threads, merge commit, workspace create + rollback.
6. **Action feedback + critical fixes** — file-open toast, merge feedback, terminal status, git/projects error-vs-empty.
7. **Hydration expansion** — `hydrateAllStores()` extends `hydrate.ts` to all new domains (concurrent IDB reads → `loading(staleData)`).
8. **Remove TanStack Query** — delete `useQuery` call sites, `lib/queries.ts`, `features/*/queries.ts`, `PersistQueryClientProvider`/`QueryClientProvider`; drop the `query-cache` object store in the schema upgrade (no users → removed outright, no migration).

---

## Testing strategy

Tests in `web/src/__tests__/` mirroring source; `@/` imports (per CLAUDE.md).

| Unit | Focus |
|---|---|
| `loadable.ts` | Constructors + `dataOf`/`fetchedAtOf` across all 4 statuses; stale data survives `loading`/`error`; injected clock |
| `loadable-slice.ts` | `fetch` writes IDB on success; failed `fetch` preserves stale; `startSync` subscribes/unsubscribes via `wsManager`; `optimisticWrite` commits and rolls back |
| `DataState` | Render matrix: 4 statuses × (stale / no-stale); banner only when stale + non-success; empty vs `emptyMessage` |
| Domain stores | `applyDelta` triggers re-fetch; git `loadMoreCommits` appends; branch-review optimistic CRUD |
| `git-view` / `ProjectListPage` | error ≠ empty: failed fetch → `<InlineError>`; true-empty → onboarding |
| `git-blame-store` | per-file `Loadable` keying; no impossible states |

The chaos panel (`useChaosStore`) is the integration harness: each silent-void finding from the sweep becomes an assertion that the corresponding surface now shows stale data, an inline error, or a toast — never a blank void.

---

## Non-goals / future milestones

- **Delta sync protocol** — backend enriching WS events with payloads so `applyDelta` patches in place instead of re-fetching. Requires Go changes.
- **Offline write queue** — persisting the optimistic mutation queue across reloads/reconnects for true offline editing.
- **Auth** — 401 handling, session boot script (Linear-style `localStorage` gate).
- **Granular reactivity** (MobX/signals) — current Zustand + narrow selectors is sufficient at Crowbar's scale.
