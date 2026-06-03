# Data-Fetching Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all raw `useEffect + apiFetch` component-level fetching with the established `createLoadableSlice` store pattern so every data source gets IDB caching, stale-while-revalidate, and consistent loading states.

**Architecture:** Three violations exist: `GitHistoryList` (should read from the already-populated `useGitStore.commits`), `HydrationGate` (should use the proper `LoadableSlice` stores instead of raw `apiFetch` for initial seeding), and `ChatTree` (needs a `useChatListStore` with `createLoadableSlice` instead of a bare `useEffect`). Each fix is independent and ships in its own commit.

**Tech Stack:** Zustand, `createLoadableSlice` (`@/lib/store/loadable-slice`), `dataOf` (`@/lib/loadable`), IDB via `idb` package, Vitest + Testing Library.

---

## File Map

**Task 1 — GitHistoryList:**
- Modify: `web/src/features/git/components/git-history-list.tsx` — remove useState+useEffect+apiFetch, read from `useGitStore`

**Task 2 — HydrationGate:**
- Modify: `web/src/components/hydration-gate.tsx` — replace raw `apiFetch` with proper store `fetch()` calls

**Task 3 — ChatTree / useChatListStore:**
- Modify: `web/src/lib/persistence/cache-store.ts` — add `'chats-data'` to `CacheStoreName`
- Modify: `web/src/lib/persistence/idb.ts` — bump DB to v6, add `'chats-data'` object store
- Create: `web/src/lib/store/chat-list-store.ts` — `LoadableSlice<ProjectChat[], [string]>` keyed by wsId
- Modify: `web/src/components/layout/chat-tree.tsx` — replace `useEffect+apiFetch` with store fetch + store subscription

---

## Task 1: Fix GitHistoryList

`git-store.ts` already fetches commits via `createLoadableSlice` and exposes them as `useGitStore(s => s.commits)`. `GitHistoryList` duplicates this with its own `useState + useEffect + apiFetch`. Replace it entirely.

**Reference files to read first:**
- `web/src/features/git/stores/git-store.ts` — has `commits: GitCommit[]` and `gitData: Loadable<GitData>`
- `web/src/features/git/components/git-commit-history.tsx` — existing component that already reads from `useGitStore` correctly

**Files:**
- Modify: `web/src/features/git/components/git-history-list.tsx`

- [ ] **Step 1: Read the current file and reference files**

```bash
cat web/src/features/git/components/git-history-list.tsx
cat web/src/features/git/stores/git-store.ts | head -100
grep -n "commits\|gitData\|loading\|GitCommit" web/src/features/git/stores/git-store.ts | head -20
```

- [ ] **Step 2: Rewrite git-history-list.tsx**

Replace `web/src/features/git/components/git-history-list.tsx` entirely:

```tsx
import { ScrollArea } from '@/components/ui/scroll-area'
import { useGitStore } from '@/features/git/stores/git-store'
import { dataOf } from '@/lib/loadable'

export function GitHistoryList() {
  const gitData = useGitStore(s => s.gitData)
  const commits = useGitStore(s => s.commits)

  const isLoading = gitData.status === 'idle' || gitData.status === 'loading' && !dataOf(gitData)

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">
        Loading…
      </div>
    )
  }

  if (!commits.length) {
    return (
      <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">
        No commits
      </div>
    )
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {commits.map(commit => (
          <div
            key={commit.hash}
            className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer"
          >
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">
              {commit.shortHash}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">
                {commit.author} · {commit.date}
              </p>
            </div>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
```

> **Note on field names:** `useGitStore.commits` has type `GitCommit[]`. Read `git-store.ts` to confirm the exact field names on `GitCommit` (may be `shortHash`/`hash`/`message`/`author`/`date` or similar). Adjust the JSX to match the actual type — do NOT invent field names.

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "git-history-list" | head -5
```

Expected: no errors. If you get "Property X does not exist on type GitCommit", read `git-store.ts` to find the correct field name and fix the JSX.

- [ ] **Step 4: Run tests**

```bash
cd web && npm test 2>&1 | tail -10
```

Expected: same pass/fail count as before this task (no new failures).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/components/git-history-list.tsx
git commit -m "fix(git): read commit history from useGitStore instead of bare apiFetch"
```

---

## Task 2: Fix HydrationGate

`HydrationGate` calls `apiFetch('/api/v0/workspaces')` and `apiFetch('/api/v0/projects')` directly, bypassing the established `useWorkspaceListStore` and `useProjectDataStore` LoadableSlice stores (which have IDB caching). Replace the raw calls with the proper store `fetch()` then read the result via `dataOf`.

The sequential order MUST be preserved: repos and projects must be loaded into sidebar/project stores before `hydrateSidebar()` runs (which applies hierarchy overrides to `s.repos`).

**Files:**
- Modify: `web/src/components/hydration-gate.tsx`

- [ ] **Step 1: Read the current file and imports**

```bash
cat web/src/components/hydration-gate.tsx
cat web/src/lib/store/workspace-list.ts
cat web/src/lib/store/projects.ts
grep -n "dataOf" web/src/lib/loadable.ts | head -5
```

- [ ] **Step 2: Rewrite hydration-gate.tsx**

Replace `web/src/components/hydration-gate.tsx` entirely:

```tsx
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar } from '@/lib/persistence/hydrate'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    async function boot() {
      // Step 1+2: seed stores via LoadableSlice (IDB-cached, stale-while-revalidate).
      // Must complete BEFORE hydrateSidebar which applies hierarchy overrides to s.repos.
      await Promise.all([
        useWorkspaceListStore.getState().fetch(),
        useProjectDataStore.getState().fetch(),
      ])

      const repos = dataOf(useWorkspaceListStore.getState().data) ?? []
      const projects = dataOf(useProjectDataStore.getState().data) ?? []

      useSidebarStore.getState().setRepos(repos)
      useProjectStore.getState().setProjects(projects)

      // Step 3+4: overlay IDB-persisted UI state on top of API data
      await Promise.all([hydratePreferences(), hydrateSidebar()])
    }

    boot()
      .catch(() => {})
      .finally(() => setHydrated(true))
  }, [])

  if (!hydrated) return null

  return <>{children}</>
}
```

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "hydration-gate" | head -5
```

Expected: no errors.

- [ ] **Step 4: Run tests**

```bash
cd web && npm test 2>&1 | tail -10
```

Expected: same pass/fail count as before.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/hydration-gate.tsx
git commit -m "fix(boot): use LoadableSlice stores in HydrationGate instead of bare apiFetch"
```

---

## Task 3: Create useChatListStore + fix ChatTree

Create a proper `LoadableSlice` store for chats (keyed by workspace ID) and wire `ChatTree` to use it. The chat list store provides IDB caching and consistent loading states. `useSidebarStore.chats` remains the mutable state for local operations (create/rename/delete) — the store seeds it when data loads.

**Files:**
- Modify: `web/src/lib/persistence/cache-store.ts`
- Modify: `web/src/lib/persistence/idb.ts`
- Create: `web/src/lib/store/chat-list-store.ts`
- Modify: `web/src/components/layout/chat-tree.tsx`

- [ ] **Step 1: Read the reference files**

```bash
cat web/src/lib/persistence/cache-store.ts
cat web/src/lib/persistence/idb.ts
cat web/src/lib/store/workspace-list.ts
cat web/src/components/layout/chat-tree.tsx
```

- [ ] **Step 2: Add 'chats-data' to CacheStoreName**

In `web/src/lib/persistence/cache-store.ts`, add `'chats-data'` to the union:

```ts
export type CacheStoreName =
  | 'workspaces-data'
  | 'git-data'
  | 'file-tree-data'
  | 'branch-review-data'
  | 'chat-history'
  | 'projects-data'
  | 'chats-data'       // ADD THIS LINE
```

- [ ] **Step 3: Add the IDB object store (DB version 6)**

In `web/src/lib/persistence/idb.ts`:

1. Bump the version from `5` to `6`:
```ts
_db = await openDB<CrowbarDB>('crowbar', 6, {
```

2. Add a new `oldVersion < 6` block after the existing `< 5` block:
```ts
      if (oldVersion < 6) {
        db.createObjectStore('chats-data', { keyPath: 'key' })
      }
```

3. Update the `CrowbarDB` schema type. Read `web/src/lib/persistence/schemas.ts` to find where `CrowbarDB` is defined, then add an entry for `'chats-data'` following the same `CachedRecord` pattern used by `'workspaces-data'` and others.

- [ ] **Step 4: Verify the CrowbarDB schema**

```bash
grep -n "CrowbarDB\|workspaces-data\|CachedRecord" web/src/lib/persistence/schemas.ts | head -20
```

The schema will look like:
```ts
'chats-data': {
  key: string
  value: CachedRecord<unknown>
  indexes: Record<never, never>
}
```

Add it to the `CrowbarDB` interface following the exact same pattern as the other cached stores.

- [ ] **Step 5: Create useChatListStore**

Create `web/src/lib/store/chat-list-store.ts`:

```ts
import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { ProjectChat } from '@/lib/store/sidebar'

export const useChatListStore = create<LoadableSlice<ProjectChat[], [string]>>()((set, get) =>
  createLoadableSlice<ProjectChat[], [string]>({
    store: 'chats-data',
    fetcher: (wsId: string) => apiFetch<ProjectChat[]>(`/api/v0/chats?wsId=${wsId}`),
    cacheKey: (wsId: string) => wsId,
  })(set, get),
)
```

- [ ] **Step 6: Fix ChatTree to use the store**

In `web/src/components/layout/chat-tree.tsx`:

1. Remove the `apiFetch` import (it's no longer used in this file)
2. Add imports:
```ts
import { useChatListStore } from '@/lib/store/chat-list-store'
import { dataOf } from '@/lib/loadable'
```

3. Replace the `useEffect + apiFetch` block (find it by `apiFetch('/api/v0/chats`)`) with:
```ts
  // Fetch via LoadableSlice (IDB-cached, stale-while-revalidate)
  useEffect(() => {
    void useChatListStore.getState().fetch(wsId)
  }, [wsId])

  // Seed sidebar store when loadable data arrives
  useEffect(() => {
    return useChatListStore.subscribe(state => {
      const fetched = dataOf(state.data)
      if (!fetched) return
      const existing = new Set(useSidebarStore.getState().chats.map(c => c.id))
      const fresh = fetched.filter(c => !existing.has(c.id))
      if (fresh.length > 0) {
        fresh.forEach(c => useSidebarStore.getState().addChat(c))
      }
    })
  }, [])
```

4. Remove `useMemo` from the React import if it's unused after this change.

- [ ] **Step 7: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "chat-list-store\|chat-tree\|chats-data\|idb\|schemas" | head -10
```

Expected: no errors.

- [ ] **Step 8: Run tests**

```bash
cd web && npm test 2>&1 | tail -10
```

Expected: same pass/fail count as before.

- [ ] **Step 9: Commit**

```bash
git add web/src/lib/persistence/cache-store.ts web/src/lib/persistence/idb.ts web/src/lib/persistence/schemas.ts web/src/lib/store/chat-list-store.ts web/src/components/layout/chat-tree.tsx
git commit -m "fix(chats): load chat list via LoadableSlice (IDB cache + stale-while-revalidate)"
```

---

## Self-Review Notes

- **Task 1:** Field names on `GitCommit` must be verified from `git-store.ts` before writing JSX. The plan explicitly calls this out.
- **Task 2:** Sequential order preserved — `fetch()` both stores in parallel, then `setRepos`/`setProjects`, then `hydrateSidebar`. `dataOf` returns the fresh data since we just awaited `fetch()`.
- **Task 3:** IDB version bump is required — adding a new object store to an existing IDB database always requires a version increment. The `oldVersion < 6` guard ensures existing users' DBs get the new store on first load without losing existing data.
- **Type coverage:** `CrowbarDB` schema in `schemas.ts` must be updated to include `'chats-data'` — Task 3 Step 4 covers this.
- **No `app-sync-provider` changes needed:** Chat fetching is triggered per-workspace inside `ChatTree` (on `wsId` change), which is the correct scope since chats are workspace-scoped.
