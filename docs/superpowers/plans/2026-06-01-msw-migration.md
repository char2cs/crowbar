# MSW Migration + Flow Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every data load go through the MSW/API layer (no component imports from `lib/mock/*` directly), remove the scrapped flow/workflow feature, and enforce the startup ordering contract so the sidebar + hydration work correctly.

**Architecture:** React Query fetches from MSW on cold start → result seeds Zustand via `useEffect` → IDB persists both the query cache and direct Zustand mutations → `hydrate.ts` restores IDB into Zustand on startup. Stores are never replaced — they're the permanent in-memory cache. The cold-start guard ensures API data never overwrites user-authored IDB content (threads, descriptions).

**Tech Stack:** React, Zustand (immer), TanStack Query, MSW 2, IndexedDB (`idb`), TypeScript, Vitest + jsdom

---

## Task 1: Delete flow feature files

**Files:**
- Delete: `web/src/features/workflow/` (entire directory)
- Delete: `web/src/features/workspace/stores/slices/workflow-slice.ts`
- Delete: `web/src/features/workspace/stores/hooks/use-workflow.ts`
- Delete: `web/src/components/layout/WorkspaceStepTabs.tsx`
- Delete: `web/src/features/workspace/components/WorkspaceStepFooter.tsx`
- Delete: `web/src/lib/mock/flows.ts`
- Delete: `web/src/mocks/handlers/flows.ts`
- Delete: `web/src/routes/workspaces/$wsId/$step.tsx`

- [ ] **Step 1: Delete the files**

```bash
cd web
rm -rf src/features/workflow
rm src/features/workspace/stores/slices/workflow-slice.ts
rm src/features/workspace/stores/hooks/use-workflow.ts
rm src/components/layout/WorkspaceStepTabs.tsx
rm src/features/workspace/components/WorkspaceStepFooter.tsx
rm src/lib/mock/flows.ts
rm src/mocks/handlers/flows.ts
rm "src/routes/workspaces/\$wsId/\$step.tsx"
```

- [ ] **Step 2: Confirm TypeScript now fails (expected)**

```bash
cd web && npx tsc --noEmit 2>&1 | head -50
```

Expected: errors about missing imports of deleted files. These are fixed in Task 2. Do not proceed to commit yet.

---

## Task 2: Remove all flow references from remaining files

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/queries.ts`
- Modify: `web/src/lib/mock/workspaces.ts`
- Modify: `web/src/mocks/handlers/workspaces.ts`
- Modify: `web/src/mocks/handlers/index.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.types.ts`
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`
- Modify: `web/src/components/workspace/WorkspaceCreationForm.tsx`
- Modify: `web/src/routes/workspaces/new.tsx`

- [ ] **Step 1: Update `lib/types.ts` — remove flow types, slim `WorkspacePayload`**

Replace the entire file:

```ts
// web/src/lib/types.ts
export interface WorkspacePayload {
  id: string
  repoId: string
  branch: string
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  toolCalls?: number
  durationSec?: number
}

export interface Project {
  id: string
  name: string
  path: string
  lastActivity: Date
}
```

- [ ] **Step 2: Update `lib/api.ts` — remove `fetchFlows` and `flowName` from `postWorkspace`**

```ts
import type { WorkspacePayload, ChatMessage, Project } from './types'
import { useChaosStore } from '@/lib/store/chaos'

const crowbar = (window as any).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const { latency, errorRate } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...init?.headers, ...chaosHeaders },
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/api/v0/workspaces/${wsId}`)
}

export function postWorkspace(repoId: string, branch: string): Promise<WorkspacePayload> {
  return apiFetch('/api/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch }),
  })
}

export function fetchConversation(wsId: string, step: string): Promise<{ messages: ChatMessage[] }> {
  return apiFetch(`/api/v0/conversations/${wsId}/${step}`)
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/api/v0/projects')
}

export function postProject(name: string, path: string): Promise<Project> {
  return apiFetch('/api/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}
```

- [ ] **Step 3: Update `lib/queries.ts` — remove `flowsQueryOptions` and `fetchFlows` import**

```ts
import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchConversation, apiFetch } from './api'
import type { GitStatus, Commit, Branch } from '@/lib/mock/git-data'
import type { FileNode } from '@/lib/mock/files'

export const workspaceQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['workspace', wsId],
    queryFn: () => fetchWorkspace(wsId),
  })

export const conversationQueryOptions = (wsId: string, step: string) =>
  queryOptions({
    queryKey: ['conversation', wsId, step],
    queryFn: () => fetchConversation(wsId, step),
  })

export const fileTreeQueryOptions = (rootPath: string) =>
  queryOptions({
    queryKey: ['file-tree', rootPath] as const,
    queryFn: () => apiFetch<FileNode>(`/api/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
  })

export const fileContentQueryOptions = (path: string) =>
  queryOptions({
    queryKey: ['file-content', path] as const,
    queryFn: () => apiFetch<string>(`/api/v0/fs/file?path=${encodeURIComponent(path)}`),
    staleTime: 5 * 60 * 1000,
  })

export const gitStatusQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-status', repoPath] as const,
    queryFn: () => apiFetch<GitStatus>(`/api/v0/git/status?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitHistoryQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-history', repoPath] as const,
    queryFn: () => apiFetch<Commit[]>(`/api/v0/git/log?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitBranchesQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-branches', repoPath] as const,
    queryFn: () => apiFetch<Branch[]>(`/api/v0/git/branches?repo=${encodeURIComponent(repoPath)}`),
  })

export const workspacesQueryOptions = () =>
  queryOptions({
    queryKey: ['workspaces'] as const,
    queryFn: () => apiFetch<import('@/lib/store/sidebar').Repo[]>('/api/v0/workspaces'),
  })

export const projectsQueryOptions = () =>
  queryOptions({
    queryKey: ['projects'] as const,
    queryFn: () => apiFetch<import('@/lib/types').Project[]>('/api/v0/projects'),
  })
```

- [ ] **Step 4: Update `lib/mock/workspaces.ts` — remove all flow fields**

```ts
import { nanoid } from 'nanoid'
import type { WorkspacePayload } from '@/lib/types'
import type { Repo } from '@/lib/store/sidebar'

const INITIAL_WORKSPACES: WorkspacePayload[] = [
  { id: 'ws3', repoId: 'crowbar', branch: 'feature/app-design' },
  { id: 'ws2', repoId: 'crowbar', branch: 'feature/api-backend' },
  { id: 'ws1', repoId: 'crowbar', branch: 'enhancement/scaffold' },
  { id: 'qc1', repoId: 'quiver-core', branch: 'develop' },
  { id: 'qd1', repoId: 'quiver-desktop', branch: 'develop' },
  { id: 'qd2', repoId: 'quiver-desktop', branch: 'feature/quiver-shell' },
]

const store = new Map<string, WorkspacePayload>(
  INITIAL_WORKSPACES.map(ws => [ws.id, ws]),
)

export function getMockWorkspace(wsId: string): WorkspacePayload | undefined {
  return store.get(wsId)
}

export function createMockWorkspace(repoId: string, branch: string): WorkspacePayload {
  const id = nanoid()
  const ws: WorkspacePayload = { id, repoId, branch }
  store.set(id, ws)
  return ws
}

export function deleteMockWorkspace(id: string): void {
  store.delete(id)
}

export function getMockRepos(): Repo[] {
  return [
    {
      id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
      workspaces: [
        { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'ws3', branch: 'feature/app-design', parentId: 'ws-develop', status: 'pr-open', added: 5672, age: '16h ago' },
        { id: 'ws1', branch: 'enhancement/scaffold', parentId: 'ws3', status: 'agent-running', added: 22892, age: '3d ago' },
        { id: 'ws-fix', branch: 'fix/toolbar-crash', parentId: 'ws3', status: 'new', age: 'just now' },
        { id: 'ws2', branch: 'feature/api-backend', parentId: 'ws-develop', status: 'pr-merged', added: 27347, deleted: 455, age: '1d ago' },
        { id: 'ws4', branch: 'feature/ws-channels', parentId: 'ws-develop', status: 'pr-open', added: 8841, deleted: 203, age: '2d ago' },
        { id: 'ws5', branch: 'refactor/query-layer', parentId: 'ws-develop', status: 'agent-running', added: 103482, deleted: 88910, age: '5d ago', hasConflicts: true },
        { id: 'ws6', branch: 'chore/bump-deps', parentId: 'ws-develop', status: 'pr-closed', added: 312, deleted: 298, age: '6d ago' },
      ],
    },
    {
      id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
      workspaces: [
        { id: 'qc-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qc1', branch: 'feature/old-auth', parentId: 'qc-develop', status: 'pr-closed', age: '3d ago' },
        { id: 'qc2', branch: 'feature/oauth2', parentId: 'qc-develop', status: 'pr-open', added: 4521, deleted: 89, age: '1d ago' },
        { id: 'qc3', branch: 'fix/token-expiry', parentId: 'qc2', status: 'new', added: 47, age: 'just now', hasConflicts: true },
        { id: 'qc4', branch: 'perf/redis-cache', parentId: 'qc-develop', status: 'agent-running', added: 1823, deleted: 402, age: '12h ago' },
      ],
    },
    {
      id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
      workspaces: [
        { id: 'qd-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qd2', branch: 'feature/quiver-shell', parentId: 'qd-develop', status: 'pr-open', added: 13485, deleted: 69, age: '3d ago' },
        { id: 'qd3', branch: 'feat/native-file-picker', parentId: 'qd-develop', status: 'agent-running', added: 2341, age: '8h ago' },
        { id: 'qd4', branch: 'fix/startup-crash', parentId: 'qd-develop', status: 'pr-merged', added: 23, deleted: 7, age: '4d ago' },
      ],
    },
    {
      id: 'quiver-cloud', name: 'quiver.cloud', avatarLabel: 'Q', avatarColor: 'bg-sky-700',
      workspaces: [
        { id: 'qcl-develop', branch: 'develop', status: 'locked', age: '—' },
        { id: 'qcl1', branch: 'feature/multi-tenant', parentId: 'qcl-develop', status: 'pr-open', added: 31204, deleted: 1823, age: '2d ago' },
        { id: 'qcl2', branch: 'feat/s3-presign', parentId: 'qcl-develop', status: 'new', added: 892, age: '4h ago' },
        { id: 'qcl3', branch: 'chore/infra-terraform', parentId: 'qcl-develop', status: 'pr-merged', added: 4812, deleted: 3201, age: '7d ago' },
      ],
    },
  ]
}
```

- [ ] **Step 5: Update `mocks/handlers/workspaces.ts` — remove `flowName` from POST handler**

```ts
import { http, HttpResponse } from 'msw'
import { getMockWorkspace, createMockWorkspace, getMockRepos } from '@/lib/mock/workspaces'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces', () => {
    return HttpResponse.json(getMockRepos())
  }),

  http.get('/api/v0/workspaces/:id', ({ params }) => {
    const ws = getMockWorkspace(params.id as string)
    if (!ws) return HttpResponse.json({ error: 'not found' }, { status: 404 })
    return HttpResponse.json(ws)
  }),

  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string }
    const ws = createMockWorkspace(body.repoId, body.branch)
    return HttpResponse.json(ws, { status: 201 })
  }),
]
```

- [ ] **Step 6: Update `mocks/handlers/index.ts` — remove `flowHandlers`**

```ts
import { workspaceHandlers } from './workspaces'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'
import { markdownChatHandlers } from './markdown-chat'
import { gitWsHandler } from './ws/git'
import { chatWsHandler } from './ws/chat'
import { terminalWsHandler } from './ws/terminal'

export const handlers = [
  ...workspaceHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
  ...markdownChatHandlers,
  gitWsHandler,
  chatWsHandler,
  terminalWsHandler,
]
```

- [ ] **Step 7: Update `workspace-store.ts` — remove `WorkflowSlice` and `currentStepId`**

```ts
// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { createBranchReviewSlice } from './slices/branch-review-slice'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import { saveBranchReview } from '@/lib/persistence/branch-review'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'panes' | 'rootLayout' | 'bottomLayout'
    | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'recentFiles'
    | 'terminalLayout'
  >
>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...createBranchReviewSlice(set, get, api),
      ...(snapshot ?? {}),
    }))
  )

  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane = state.panes[state.activePaneId] ?? null
    saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
  })

  store.subscribe((state, prev) => {
    const br = state.branchReview
    const prevBr = prev.branchReview
    if (
      br.description === prevBr.description &&
      br.mergeStrategy === prevBr.mergeStrategy &&
      br.activeSubtab === prevBr.activeSubtab &&
      br.threads === prevBr.threads
    ) return
    saveBranchReview(wsId, {
      description: br.description,
      mergeStrategy: br.mergeStrategy,
      activeSubtab: br.activeSubtab,
      threads: br.threads,
    })
  })

  return store
}
```

- [ ] **Step 8: Update `workspace-store.types.ts` — remove `WorkflowSlice`**

```ts
// web/src/features/workspace/stores/workspace-store.types.ts
import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'
import type { LspSlice } from './slices/lsp-slice'
import type { TerminalSlice } from './slices/terminal-slice'
import type { FileWatcherSlice } from './slices/file-watcher-slice'
import type { RecentFilesSlice } from './slices/recent-files-slice'
import type { BranchReviewSlice } from './slices/branch-review-slice'

export interface WorkspaceBaseState {
  workspaceId: string
}

export type WorkspaceState =
  & WorkspaceBaseState
  & PaneSlice
  & BufferSlice
  & LspSlice
  & TerminalSlice
  & FileWatcherSlice
  & RecentFilesSlice
  & BranchReviewSlice
```

- [ ] **Step 9: Update `use-workspace-effects.ts` — remove all flow imports and the flow-seeding `useEffect`**

```ts
import { useEffect } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import type { AppFile } from '@/features/file-system/types/app'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const repoPath = `/repos/${wsId}`

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open branchReview buffer
  useEffect(() => {
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
```

Note: file tree seeding is moved to its own hook in Task 7. We keep this hook minimal for buffer management only.

- [ ] **Step 10: Update `WorkspaceCreationForm.tsx` — remove `flows` prop and `flowName` state**

```tsx
import { useState } from 'react'
import { Button } from '@/components/ui/button'

interface Props {
  repos: { id: string; name: string }[]
  onSubmit: (data: { repoId: string; branch: string }) => void
  loading?: boolean
}

export function WorkspaceCreationForm({ repos, onSubmit, loading }: Props) {
  const [repoId, setRepoId] = useState(repos[0]?.id ?? '')
  const [branch, setBranch] = useState('')

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ repoId, branch })
      }}
    >
      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Repo</span>
        <select
          aria-label="Repo"
          value={repoId}
          onChange={e => setRepoId(e.target.value)}
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground"
        >
          {repos.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
        </select>
      </label>

      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Branch</span>
        <input
          aria-label="Branch"
          type="text"
          value={branch}
          onChange={e => setBranch(e.target.value)}
          placeholder="feature/my-feature"
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground"
        />
      </label>

      <Button type="submit" disabled={!branch.trim() || loading}>
        {loading ? 'Creating…' : 'Create workspace'}
      </Button>
    </form>
  )
}
```

- [ ] **Step 11: Update `routes/workspaces/new.tsx` — remove flow query, update navigation**

```tsx
import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'
import { useSidebarStore } from '@/lib/store/sidebar'

export const Route = createFileRoute('/workspaces/new')({
  component: NewWorkspacePage,
})

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
  { id: 'quiver-desktop', name: 'quiver.desktop' },
]

export function NewWorkspacePage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const addWorkspace = useSidebarStore(s => s.addWorkspace)

  const handleSubmit = async (data: { repoId: string; branch: string }) => {
    setLoading(true)
    try {
      const ws = await postWorkspace(data.repoId, data.branch)
      addWorkspace(data.repoId, ws.id, data.branch)
      void navigate({ to: '/workspaces/$wsId', params: { wsId: ws.id } })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex flex-1 items-center justify-center">
      <div className="w-full max-w-sm">
        <h1 className="mb-6 text-lg font-semibold text-foreground">New workspace</h1>
        <WorkspaceCreationForm
          repos={REPOS}
          onSubmit={handleSubmit}
          loading={loading}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 12: Verify TypeScript is clean**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors. If any remain, they will point to files that still import from deleted modules — fix them following the same patterns above.

- [ ] **Step 13: Run tests**

```bash
cd web && npx vitest run
```

Expected: all tests pass (or only pre-existing failures, none introduced by these changes).

- [ ] **Step 14: Commit**

```bash
git add -A
git commit -m "feat: remove flow/workflow feature entirely"
```

---

## Task 3: Add new MSW handlers for branch-review and file content

**Files:**
- Create: `web/src/mocks/handlers/branch-review.ts`
- Modify: `web/src/mocks/handlers/fs.ts`
- Modify: `web/src/mocks/handlers/index.ts`
- Test: `web/src/__tests__/mocks/handlers/branch-review.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/mocks/handlers/branch-review.test.ts
import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest'
import { setupServer } from 'msw/node'
import { branchReviewHandlers } from '@/mocks/handlers/branch-review'
import { fsHandlers } from '@/mocks/handlers/fs'

const server = setupServer(...branchReviewHandlers, ...fsHandlers)
beforeAll(() => server.listen())
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('branch-review handlers', () => {
  it('GET /api/v0/branch-review/:wsId/diff returns a MultiFileDiff', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/diff')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(data).toHaveProperty('files')
    expect(data).toHaveProperty('totalFiles')
    expect(Array.isArray(data.files)).toBe(true)
  })

  it('GET /api/v0/branch-review/:wsId/threads returns an array', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/threads')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(Array.isArray(data)).toBe(true)
  })

  it('GET /api/v0/branch-review/:wsId/description returns a string', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/description')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(typeof data).toBe('string')
  })

  it('GET /api/v0/branch-review/:wsId/chats returns an array', async () => {
    const res = await fetch('/api/v0/branch-review/ws3/chats')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(Array.isArray(data)).toBe(true)
  })

  it('GET /api/v0/fs/file returns file content string', async () => {
    const res = await fetch('/api/v0/fs/file?path=api/go.mod')
    expect(res.ok).toBe(true)
    const data = await res.json()
    expect(typeof data).toBe('string')
  })
})
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/mocks/handlers/branch-review.test.ts
```

Expected: FAIL — `branchReviewHandlers` does not exist.

- [ ] **Step 3: Create `mocks/handlers/branch-review.ts`**

```ts
import { http, HttpResponse } from 'msw'
import {
  getMockBranchDiff,
  getMockBranchReviewThreads,
  getMockBranchReviewDescription,
  getMockBranchReviewChats,
} from '@/lib/mock/branch-diff'

export const branchReviewHandlers = [
  http.get('/api/v0/branch-review/:wsId/diff', ({ params }) =>
    HttpResponse.json(getMockBranchDiff(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/threads', ({ params }) =>
    HttpResponse.json(getMockBranchReviewThreads(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/description', ({ params }) =>
    HttpResponse.json(getMockBranchReviewDescription(params.wsId as string))
  ),
  http.get('/api/v0/branch-review/:wsId/chats', ({ params }) =>
    HttpResponse.json(getMockBranchReviewChats(params.wsId as string))
  ),
]
```

- [ ] **Step 4: Update `mocks/handlers/fs.ts` — add file content endpoint**

```ts
import { http, HttpResponse } from 'msw'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'

export const fsHandlers = [
  http.get('/api/v0/fs/tree', ({ request }) => {
    const root = new URL(request.url).searchParams.get('root') ?? ''
    return HttpResponse.json(getMockFileTree(root))
  }),

  http.get('/api/v0/fs/file', ({ request }) => {
    const path = new URL(request.url).searchParams.get('path') ?? ''
    return HttpResponse.json(getMockFileContent(path))
  }),
]
```

- [ ] **Step 5: Update `mocks/handlers/index.ts` — register branch-review handlers**

```ts
import { workspaceHandlers } from './workspaces'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'
import { markdownChatHandlers } from './markdown-chat'
import { branchReviewHandlers } from './branch-review'
import { gitWsHandler } from './ws/git'
import { chatWsHandler } from './ws/chat'
import { terminalWsHandler } from './ws/terminal'

export const handlers = [
  ...workspaceHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
  ...markdownChatHandlers,
  ...branchReviewHandlers,
  gitWsHandler,
  chatWsHandler,
  terminalWsHandler,
]
```

- [ ] **Step 6: Run test — confirm it passes**

```bash
cd web && npx vitest run src/__tests__/mocks/handlers/branch-review.test.ts
```

Expected: all 5 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/mocks/handlers/branch-review.ts web/src/mocks/handlers/fs.ts web/src/mocks/handlers/index.ts web/src/__tests__/mocks/handlers/branch-review.test.ts
git commit -m "feat: add branch-review and file-content MSW handlers"
```

---

## Task 4: Add feature-level query options files

**Files:**
- Create: `web/src/features/branch-review/queries.ts`
- Create: `web/src/features/markdown-chat/queries.ts`
- Test: `web/src/__tests__/features/branch-review/queries.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/branch-review/queries.test.ts
import { describe, it, expect } from 'vitest'
import { branchDiffQueryOptions, branchThreadsQueryOptions, branchDescriptionQueryOptions, branchChatsQueryOptions } from '@/features/branch-review/queries'

describe('branch-review query options', () => {
  it('branchDiffQueryOptions has correct queryKey', () => {
    const opts = branchDiffQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-diff', 'ws3'])
  })

  it('branchThreadsQueryOptions has correct queryKey', () => {
    const opts = branchThreadsQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-threads', 'ws3'])
  })

  it('branchDescriptionQueryOptions has correct queryKey', () => {
    const opts = branchDescriptionQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-description', 'ws3'])
  })

  it('branchChatsQueryOptions has correct queryKey', () => {
    const opts = branchChatsQueryOptions('ws3')
    expect(opts.queryKey).toEqual(['branch-chats', 'ws3'])
  })
})
```

- [ ] **Step 2: Run to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/queries.test.ts
```

Expected: FAIL — module does not exist.

- [ ] **Step 3: Create `features/branch-review/queries.ts`**

```ts
import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'

export const branchDiffQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-diff', wsId] as const,
    queryFn: () => apiFetch<MultiFileDiff>(`/api/v0/branch-review/${wsId}/diff`),
  })

export const branchThreadsQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-threads', wsId] as const,
    queryFn: () => apiFetch<ReviewThread[]>(`/api/v0/branch-review/${wsId}/threads`),
  })

export const branchDescriptionQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-description', wsId] as const,
    queryFn: () => apiFetch<string>(`/api/v0/branch-review/${wsId}/description`),
  })

export const branchChatsQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-chats', wsId] as const,
    queryFn: () => apiFetch<BranchReviewChat[]>(`/api/v0/branch-review/${wsId}/chats`),
  })
```

- [ ] **Step 4: Create `features/markdown-chat/queries.ts`**

```ts
import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

export const markdownChatQueryOptions = (wsId: string, stepId: string) =>
  queryOptions({
    queryKey: ['markdown-chat', wsId, stepId] as const,
    queryFn: () => apiFetch<MarkdownTurn[]>(`/api/v0/markdown-chat/${wsId}/${stepId}`),
  })
```

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/queries.test.ts
```

Expected: all 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/branch-review/queries.ts web/src/features/markdown-chat/queries.ts web/src/__tests__/features/branch-review/queries.test.ts
git commit -m "feat: add feature-level query options for branch-review and markdown-chat"
```

---

## Task 5: Fix stores — remove hardcoded mock seeds

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/lib/store/projects.ts`
- Modify: `web/src/lib/store/conversations.ts`
- Test: `web/src/__tests__/lib/store/projects.test.ts`
- Test: `web/src/__tests__/lib/store/conversations.test.ts`

- [ ] **Step 1: Write the failing test for projects store**

```ts
// web/src/__tests__/lib/store/projects.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useProjectStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

const mockProject: Project = {
  id: 'p1', name: 'my-app', path: '/repos/my-app', lastActivity: new Date(),
}

beforeEach(() => {
  useProjectStore.setState({ projects: [], activeProjectId: '' })
})

describe('useProjectStore', () => {
  it('starts with empty projects', () => {
    expect(useProjectStore.getState().projects).toHaveLength(0)
  })

  it('setProjects replaces the full list', () => {
    useProjectStore.getState().setProjects([mockProject])
    expect(useProjectStore.getState().projects).toHaveLength(1)
    expect(useProjectStore.getState().projects[0].id).toBe('p1')
  })

  it('addProject appends to the list', () => {
    useProjectStore.getState().setProjects([mockProject])
    const second: Project = { ...mockProject, id: 'p2', name: 'other' }
    useProjectStore.getState().addProject(second)
    expect(useProjectStore.getState().projects).toHaveLength(2)
  })
})
```

- [ ] **Step 2: Write the failing test for conversations store**

```ts
// web/src/__tests__/lib/store/conversations.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useConversationStore } from '@/lib/store/conversations'

beforeEach(() => {
  useConversationStore.setState({ sessions: new Map() })
})

describe('useConversationStore', () => {
  it('getMessages returns empty array on cold start (no mock seed)', () => {
    const msgs = useConversationStore.getState().getMessages('ws3', 'brainstorm')
    expect(msgs).toHaveLength(0)
  })

  it('getMessages returns the same empty array on repeated calls', () => {
    const first = useConversationStore.getState().getMessages('ws1', 'spec')
    const second = useConversationStore.getState().getMessages('ws1', 'spec')
    expect(first).toBe(second)
  })
})
```

- [ ] **Step 3: Run to confirm both fail**

```bash
cd web && npx vitest run src/__tests__/lib/store/projects.test.ts src/__tests__/lib/store/conversations.test.ts
```

Expected: projects test fails on `setProjects` not a function; conversations test fails because `getMessages` returns mock data.

- [ ] **Step 4: Update `lib/store/projects.ts`**

```ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'

interface ProjectState {
  projects: Project[]
  activeProjectId: string
  setActiveProject: (id: string) => void
  setProjects: (projects: Project[]) => void
  addProject: (project: Project) => void
}

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      projects: [],
      activeProjectId: '',
      setActiveProject: (id) => set({ activeProjectId: id }),
      setProjects: (projects) => set({ projects }),
      addProject: (project) => set(s => ({ projects: [...s.projects, project] })),
    }),
    { name: 'crowbar.activeProject', partialize: s => ({ activeProjectId: s.activeProjectId }) },
  ),
)
```

- [ ] **Step 5: Update `lib/store/sidebar.ts` — replace `INITIAL_REPOS` with empty array**

Remove the `INITIAL_REPOS` constant entirely and update `getInitialState`:

```ts
function getInitialState() {
  return {
    chats: [],
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    activeTab: 'workspaces' as SidebarTab,
  }
}
```

Also remove the `getAllMockChats` import and the chats seeding from `getInitialState` — chats will come from the conversations API.

The top of the file becomes:

```ts
import { create } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
```

- [ ] **Step 6: Update `lib/store/conversations.ts` — remove mock import, return empty on cold start**

```ts
import { create } from 'zustand'
import type { ChatMessage } from '@/lib/types'

type SessionKey = string // `${wsId}:${step}`

interface ConversationState {
  sessions: Map<SessionKey, ChatMessage[]>
  getMessages: (wsId: string, step: string) => ChatMessage[]
  appendMessage: (wsId: string, step: string, message: ChatMessage) => void
  pushStreamChunk: (wsId: string, step: string, chunk: string, meta: Omit<ChatMessage, 'id' | 'content'>) => void
  finalizeStream: (wsId: string, step: string, finalId: string) => void
}

function sessionKey(wsId: string, step: string): SessionKey {
  return `${wsId}:${step}`
}

export const useConversationStore = create<ConversationState>()((set, get) => ({
  sessions: new Map(),

  getMessages(wsId, step) {
    const k = sessionKey(wsId, step)
    const existing = get().sessions.get(k)
    if (existing !== undefined) return existing
    // Cold start — return and cache empty array; component seeds from API
    set(s => {
      const next = new Map(s.sessions)
      next.set(k, [])
      return { sessions: next }
    })
    return []
  },

  appendMessage(wsId, step, message) {
    set(s => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const next = new Map(s.sessions)
      next.set(k, [...current, message])
      return { sessions: next }
    })
  },

  pushStreamChunk(wsId, step, chunk, meta) {
    set(s => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const last = current[current.length - 1]
      let updated: ChatMessage[]
      if (last?.id === 'streaming' && last.role === 'assistant') {
        updated = [...current.slice(0, -1), { ...last, content: last.content + chunk }]
      } else {
        updated = [...current, { ...meta, id: 'streaming', content: chunk }]
      }
      const next = new Map(s.sessions)
      next.set(k, updated)
      return { sessions: next }
    })
  },

  finalizeStream(wsId, step, finalId) {
    set(s => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const next = new Map(s.sessions)
      next.set(k, current.map(m => (m.id === 'streaming' ? { ...m, id: finalId } : m)))
      return { sessions: next }
    })
  },
}))
```

- [ ] **Step 7: Run tests — confirm they pass**

```bash
cd web && npx vitest run src/__tests__/lib/store/projects.test.ts src/__tests__/lib/store/conversations.test.ts
```

Expected: all tests PASS.

- [ ] **Step 8: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add web/src/lib/store/sidebar.ts web/src/lib/store/projects.ts web/src/lib/store/conversations.ts web/src/__tests__/lib/store/projects.test.ts web/src/__tests__/lib/store/conversations.test.ts
git commit -m "feat: remove hardcoded mock seeds from sidebar, projects, and conversations stores"
```

---

## Task 6: Fix HydrationGate — enforce startup ordering contract

**Files:**
- Modify: `web/src/components/hydration-gate.tsx`
- Modify: `web/src/components/layout/IDEShell.tsx`

The ordering contract (from the spec) is mandatory:
1. Fetch repos → `setRepos(data)`
2. Fetch projects → `setProjects(data)`
3. `hydrateSidebar()` (maps over `s.repos` — must have data first)
4. `hydratePreferences()`

`IDEShell.tsx` has a raw `fetch('/api/v0/workspaces')` on line 42 that duplicates this — remove it.

- [ ] **Step 1: Update `components/hydration-gate.tsx`**

```tsx
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar } from '@/lib/persistence/hydrate'
import { apiFetch } from '@/lib/api'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    async function boot() {
      // Step 1+2: seed stores from API — must happen BEFORE hydrateSidebar
      // because hydrateSidebar maps over s.repos to apply hierarchy overrides
      await Promise.all([
        apiFetch<Repo[]>('/api/v0/workspaces')
          .then(repos => useSidebarStore.getState().setRepos(repos))
          .catch(() => {}),
        apiFetch<Project[]>('/api/v0/projects')
          .then(projects => useProjectStore.getState().setProjects(projects))
          .catch(() => {}),
      ])

      // Step 3+4: overlay IDB-persisted state on top of API data
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

- [ ] **Step 2: Remove the raw fetch from `IDEShell.tsx`**

Delete lines 40–46 (the `useEffect` that calls `fetch('/api/v0/workspaces')`):

```tsx
// DELETE this block entirely — repos are now loaded in HydrationGate
useEffect(() => {
  fetch('/api/v0/workspaces')
    .then(r => r.ok ? r.json() as Promise<Repo[]> : Promise.reject())
    .then(repos => useSidebarStore.getState().setRepos(repos))
    .catch(() => { /* keep hardcoded initial data */ })
}, [])
```

Also remove the unused `Repo` type import from `IDEShell.tsx` if it's no longer used there.

- [ ] **Step 3: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/hydration-gate.tsx web/src/components/layout/IDEShell.tsx
git commit -m "feat: enforce startup ordering — fetch repos+projects before hydrateSidebar"
```

---

## Task 7: Fix `use-workspace-effects` — file tree and content via API

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`

`getMockFileTree` and `getMockFileContent` are replaced with `queryClient.fetchQuery`. The query client is the singleton from `lib/queries/client.ts`.

- [ ] **Step 1: Update `use-workspace-effects.ts`**

```ts
import { useEffect } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { fileTreeQueryOptions, fileContentQueryOptions } from '@/lib/queries'
import { queryClient } from '@/lib/queries/client'
import type { AppFile } from '@/features/file-system/types/app'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const repoPath = `/repos/${wsId}`

  // Seed file system from API
  useEffect(() => {
    queryClient.fetchQuery(fileTreeQueryOptions(repoPath))
      .then(files => {
        useFileSystemStore.setState({
          rootFolderPath: repoPath,
          files: files as AppFile[],
          handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
            if (revealOrIsDir === true) return
            const name = path.split('/').pop() ?? path
            const content = await queryClient.fetchQuery(fileContentQueryOptions(path))
            bufferActions.openContent({ type: 'editor', path, name, content })
          },
          handleFileSelect: (path: string, isDir?: boolean) => {
            if (isDir) return
            const name = path.split('/').pop() ?? path
            queryClient.fetchQuery(fileContentQueryOptions(path)).then(content => {
              bufferActions.openContent({ type: 'editor', path, name, content, isPreview: true })
            })
          },
        })
      })
      .catch(() => {})
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open crowbarChat buffer
  useEffect(() => {
    const name = label ?? 'Workspace'
    bufferActions.openContent({ type: 'crowbarChat', wsId, name })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open branchReview buffer
  useEffect(() => {
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
```

- [ ] **Step 2: Fix `SidebarTabs.tsx` — read files from store instead of calling `getMockFileTree`**

`SidebarTabs.tsx` already imports `useFileSystemStore` but passes `getMockFileTree(activeWorkspaceRepoPath)` directly to `FileExplorerTree` on line 37. Change it to read from the store, which `use-workspace-effects` now populates via the API:

```tsx
import { Suspense } from 'react'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { WorkspaceTree } from './workspace-tree'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import GitView from '@/features/git/components/git-view'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

interface SidebarTabsProps {
  activeWorkspaceRepoPath: string
}

export function SidebarTabs({ activeWorkspaceRepoPath }: SidebarTabsProps) {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsContent value="workspaces" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspaceTree />
      </TabsContent>

      <TabsContent value="files" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={files}
              rootFolderPath={activeWorkspaceRepoPath}
              onFileSelect={(path, isDir) => {
                if (isDir) {
                  useFileTreeStore.getState().toggleFolder(path)
                } else {
                  handleFileSelect?.(path, false)
                }
              }}
              onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
                if (!isDir) void handleFileOpen(path, false)
              } : undefined}
              onCreateNewFileInDirectory={() => {}}
            />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>

      <TabsContent value="git" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <GitView repoPath={activeWorkspaceRepoPath} />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>
    </Tabs>
  )
}
```

- [ ] **Step 3: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/workspace/stores/hooks/use-workspace-effects.ts web/src/components/layout/SidebarTabs.tsx
git commit -m "feat: seed file tree and content from API instead of direct mock import"
```

---

## Task 8: Fix branch-review components — diff, threads, description, chats, commits

**Files:**
- Modify: `web/src/features/branch-review/components/branch-review-pane.tsx`
- Modify: `web/src/features/branch-review/components/about-tab.tsx`
- Modify: `web/src/features/branch-review/components/commits-tab.tsx`

- [ ] **Step 1: Update `branch-review-pane.tsx` — diff always from API; threads + description cold-start guarded**

```tsx
import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useWorkspaceStoreContext, useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'
import { branchDiffQueryOptions, branchThreadsQueryOptions, branchDescriptionQueryOptions } from '@/features/branch-review/queries'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import { Frame, FrameHeader, FramePanel, FrameTitle, FrameDescription } from '@/components/ui/frame'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { MergeButton } from './merge-button'
import { AboutTab } from './about-tab'
import { CommitsTab } from './commits-tab'
import { BranchReviewDiffViewer } from './branch-review-diff-viewer'

interface BranchReviewPaneProps {
  wsId: string
  branchName: string
}

export function BranchReviewPane({ wsId, branchName }: BranchReviewPaneProps) {
  const store = useWorkspaceStore()

  const description = useWorkspaceStoreContext(s => s.branchReview.description)
  const mergeStrategy = useWorkspaceStoreContext(s => s.branchReview.mergeStrategy)
  const activeSubtab = useWorkspaceStoreContext(s => s.branchReview.activeSubtab)
  const diffCache = useWorkspaceStoreContext(s => s.branchReview.diffCache)
  const threads = useWorkspaceStoreContext(s => s.branchReview.threads)

  const parentBranch = useSidebarStore(s => {
    const allWs = s.repos.flatMap(r => r.workspaces)
    const ws = allWs.find(w => w.id === wsId)
    if (!ws?.parentId) return null
    return allWs.find(w => w.id === ws.parentId)?.branch ?? null
  })

  const status = useSidebarStore(s => {
    const allWs = s.repos.flatMap(r => r.workspaces)
    return allWs.find(w => w.id === wsId)?.status ?? 'new'
  })

  // Diff always comes fresh from the API — never stored in IDB
  const { data: diff } = useQuery(branchDiffQueryOptions(wsId))
  useEffect(() => {
    if (diff) store.getState().setBranchReviewDiff(diff)
  }, [diff, store])

  // Threads — cold-start guard: only seed from API if IDB didn't restore any
  const { data: apiThreads } = useQuery(branchThreadsQueryOptions(wsId))
  useEffect(() => {
    if (!apiThreads || store.getState().branchReview.threads.length > 0) return
    apiThreads.forEach(t => store.getState().addReviewThread(t))
  }, [apiThreads, store])

  // Description — cold-start guard: only seed from API if IDB didn't restore one
  const { data: apiDescription } = useQuery(branchDescriptionQueryOptions(wsId))
  useEffect(() => {
    if (!apiDescription || store.getState().branchReview.description) return
    store.getState().setBranchReviewDescription(apiDescription)
  }, [apiDescription, store])

  function handleAddThread(filePath: string, lineNumber: number) {
    const thread: ReviewThread = {
      id: `thread-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      filePath,
      lineNumber,
      side: 'right',
      messages: [],
      isResolved: false,
    }
    store.getState().addReviewThread(thread)
  }

  function handleReply(threadId: string, body: string) {
    const message: ReviewMessage = {
      id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      author: null,
      isAgent: false,
      body,
      createdAt: new Date().toISOString(),
    }
    store.getState().addReviewMessage(threadId, message)
  }

  function handleResolve(threadId: string) {
    store.getState().resolveReviewThread(threadId)
  }

  function handleOpenConversation(id: string) {
    store.getState().bufferActions.openContent({ type: 'crowbarChat', wsId: id, name: id })
  }

  return (
    <Frame className="h-full overflow-hidden rounded-none p-2">
      <Tabs
        value={activeSubtab}
        onValueChange={v => store.getState().setBranchReviewSubtab(v as typeof activeSubtab)}
        className="flex h-full flex-col overflow-hidden gap-0"
      >
        <FrameHeader className="shrink-0 flex-col gap-2 border-b border-border/50 pb-0">
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 flex-col gap-0.5">
              <FrameTitle className="flex items-center gap-2 text-base">
                <WorkspaceBranchIcon status={status} />
                <span className="truncate">{branchName}</span>
                {parentBranch && (
                  <Badge variant="outline" className="shrink-0 text-xs font-normal text-muted-foreground">
                    → {parentBranch}
                  </Badge>
                )}
              </FrameTitle>
              {diffCache && (
                <FrameDescription className="flex items-center gap-1.5">
                  <span>{diffCache.totalFiles} file{diffCache.totalFiles !== 1 ? 's' : ''}</span>
                  <span className="text-muted-foreground/40">·</span>
                  <span className="text-git-added">+{diffCache.totalAdditions.toLocaleString()}</span>
                  <span className="text-git-deleted">−{diffCache.totalDeletions.toLocaleString()}</span>
                </FrameDescription>
              )}
            </div>
            <div className="shrink-0">
              <MergeButton
                strategy={mergeStrategy}
                isLocked={false}
                hasConflicts={false}
                onMerge={() => {}}
                onStrategyChange={s => store.getState().setBranchReviewMergeStrategy(s)}
              />
            </div>
          </div>
          <TabsList className="w-fit">
            <TabsTab value="about">About</TabsTab>
            <TabsTab value="commits">Commits</TabsTab>
            <TabsTab value="diff">Diff</TabsTab>
          </TabsList>
        </FrameHeader>

        <FramePanel className="min-h-0 flex-1 overflow-y-auto p-0">
          <TabsPanel value="about" className="p-5">
            <AboutTab
              wsId={wsId}
              description={description}
              onDescriptionChange={v => store.getState().setBranchReviewDescription(v)}
              onOpenConversation={handleOpenConversation}
            />
          </TabsPanel>

          <TabsPanel value="commits" className="p-5">
            <CommitsTab repoPath={wsId} />
          </TabsPanel>

          <TabsPanel value="diff" className="overflow-hidden">
            {diffCache ? (
              <BranchReviewDiffViewer
                multiDiff={diffCache}
                threads={threads}
                onAddThread={handleAddThread}
                onReply={handleReply}
                onResolve={handleResolve}
              />
            ) : (
              <p className="p-5 text-xs text-muted-foreground/50">Loading diff…</p>
            )}
          </TabsPanel>
        </FramePanel>
      </Tabs>
    </Frame>
  )
}
```

- [ ] **Step 2: Update `about-tab.tsx` — chats from `useQuery`**

```tsx
import { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useQuery } from '@tanstack/react-query'
import CodeMirror from '@uiw/react-codemirror'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { branchChatsQueryOptions } from '@/features/branch-review/queries'
import { FramePanel, FrameTitle } from '@/components/ui/frame'
import { cn } from '@/utils/cn'

const transparentTheme = EditorView.theme({
  '&': { backgroundColor: 'transparent !important', color: 'var(--foreground)' },
  '&.cm-focused': { outline: 'none !important', backgroundColor: 'transparent !important' },
  '.cm-content': { caretColor: 'var(--foreground)', padding: '0' },
  '.cm-cursor': { borderLeftColor: 'var(--foreground)' },
  '.cm-placeholder': { color: 'var(--muted-foreground)', opacity: '0.4' },
  '.cm-line': { padding: '0' },
  '.cm-scroller': { fontFamily: 'inherit', backgroundColor: 'transparent !important' },
  '.cm-gutters': { backgroundColor: 'transparent !important', border: 'none' },
  '.cm-activeLine': { backgroundColor: 'transparent !important' },
  '.cm-activeLineGutter': { backgroundColor: 'transparent !important' },
  '.cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 20%, transparent) !important' },
  '&.cm-focused .cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--primary) 30%, transparent) !important' },
})

interface AboutTabProps {
  wsId: string
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ wsId, description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const [editing, setEditing] = useState(false)
  const { data: chats = [] } = useQuery(branchChatsQueryOptions(wsId))

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <FrameTitle className="text-base">Description</FrameTitle>
        {editing ? (
          <CodeMirror
            autoFocus
            value={description}
            placeholder="Describe what this branch does, its goals, and any context needed for review…"
            extensions={[markdown(), transparentTheme]}
            onChange={onDescriptionChange}
            onBlur={() => setEditing(false)}
            basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
            className="text-sm"
          />
        ) : description ? (
          <div
            onClick={() => setEditing(true)}
            className="prose prose-sm prose-invert max-w-none cursor-text text-sm text-foreground
              [&_h1]:text-base [&_h1]:font-semibold [&_h1]:mb-2 [&_h1]:mt-3
              [&_h2]:text-sm [&_h2]:font-semibold [&_h2]:mb-1.5 [&_h2]:mt-3
              [&_h3]:text-sm [&_h3]:font-medium [&_h3]:mb-1 [&_h3]:mt-2
              [&_p]:mb-2 [&_p]:leading-relaxed
              [&_ul]:my-1.5 [&_ul]:pl-4 [&_li]:my-0.5
              [&_ol]:my-1.5 [&_ol]:pl-4
              [&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-xs [&_code]:font-mono
              [&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:overflow-x-auto
              [&_pre_code]:bg-transparent [&_pre_code]:p-0
              [&_strong]:font-semibold [&_strong]:text-foreground
              [&_em]:italic
              [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground
              [&_hr]:border-border [&_hr]:my-3
              [&_a]:text-primary [&_a]:underline-offset-2 [&_a]:hover:underline"
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{description}</ReactMarkdown>
          </div>
        ) : (
          <p
            onClick={() => setEditing(true)}
            className="cursor-text text-sm text-muted-foreground/40"
          >
            Describe what this branch does, its goals, and any context needed for review…
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <FrameTitle className="text-base">Conversations</FrameTitle>
        {chats.length === 0 ? (
          <p className="text-sm text-muted-foreground/40">No conversations yet.</p>
        ) : (
          <div className="flex flex-col gap-1.5">
            {chats.map(chat => (
              <FramePanel
                key={chat.id}
                className="cursor-pointer py-2.5 px-3 transition-colors hover:bg-accent/20"
                onClick={() => onOpenConversation(chat.id)}
                role="button"
                tabIndex={0}
              >
                <div className="flex items-center gap-2.5">
                  <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                    chat.isActive ? 'bg-green-500' : 'bg-muted-foreground/30')} />
                  <span className="flex-1 truncate text-sm text-foreground">{chat.title}</span>
                  <span className="text-xs text-muted-foreground/50">{chat.age}</span>
                </div>
              </FramePanel>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Update `commits-tab.tsx` — commit history from `useQuery`**

```tsx
import { useQuery } from '@tanstack/react-query'
import { gitHistoryQueryOptions } from '@/lib/queries'
import { formatRelativeDate } from '@/utils/date'
import { FramePanel, FrameTitle } from '@/components/ui/frame'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const { data: commits = [], isLoading } = useQuery(gitHistoryQueryOptions(repoPath))

  return (
    <div className="flex flex-col gap-4">
      <FrameTitle className="text-base">Commit history</FrameTitle>
      {isLoading ? (
        <p className="text-xs text-muted-foreground/50">Loading…</p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {commits.map(commit => (
            <FramePanel key={commit.hash} className="py-2 px-3">
              <div className="flex items-center gap-3">
                <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">
                  {commit.hash.slice(0, 7)}
                </span>
                <span className="flex-1 truncate text-sm text-foreground">{commit.message}</span>
                <span className="shrink-0 text-xs text-muted-foreground/50">
                  {formatRelativeDate(commit.date)}
                </span>
              </div>
            </FramePanel>
          ))}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/branch-review/components/branch-review-pane.tsx web/src/features/branch-review/components/about-tab.tsx web/src/features/branch-review/components/commits-tab.tsx
git commit -m "feat: branch-review loads diff, threads, description, chats, and commits from API"
```

---

## Task 9: Fix markdown-chat initial turns — seed from API on cold start

**Files:**
- Modify: `web/src/features/markdown-chat/components/markdown-chat-view.tsx`

Replace the direct `getMockMarkdownTurns` call with `useQuery(markdownChatQueryOptions(...))`. The store is only seeded when it's empty (cold start guard).

- [ ] **Step 1: Update the seed `useEffect` in `markdown-chat-view.tsx`**

Replace the import and the seed effect. Change these two sections only (everything else in the file stays identical):

Remove this import:
```ts
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
```

Add this import (keep `simulateMarkdownStream` for now — it is removed in Task 10):
```ts
import { simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { useQuery } from '@tanstack/react-query'
import { markdownChatQueryOptions } from '@/features/markdown-chat/queries'
```

Replace the seed `useEffect`:
```ts
// Seed turns on mount from API — only when the store is empty (cold start)
const { data: initialTurns } = useQuery(markdownChatQueryOptions(workspaceId, stepId))
useEffect(() => {
  const state = store.getState()
  if (!initialTurns || state.turns.length > 0) return
  initialTurns.forEach(t => state.appendTurn(t))
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, [initialTurns])
```

Remove the old `useEffect` block that was:
```ts
useEffect(() => {
  const state = store.getState()
  if (state.turns.length > 0) return
  getMockMarkdownTurns(workspaceId, stepId).forEach((t) => state.appendTurn(t))
  return () => { cancelStreamRef.current?.() }
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, [workspaceId, stepId])
```

Keep the cleanup return in the component's `useEffect` that watches `[workspaceId, stepId]` — move it to an explicit cleanup effect:
```ts
useEffect(() => {
  return () => { cancelStreamRef.current?.() }
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, [workspaceId, stepId])
```

- [ ] **Step 2: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/components/markdown-chat-view.tsx
git commit -m "feat: seed markdown-chat turns from API on cold start"
```

---

## Task 10: Wire streaming through WSManager — replace `simulateMarkdownStream`

**Files:**
- Modify: `web/src/mocks/handlers/ws/chat.ts`
- Modify: `web/src/features/markdown-chat/components/markdown-chat-view.tsx`

- [ ] **Step 1: Update `mocks/handlers/ws/chat.ts` — make message-triggered**

The current handler auto-streams on connect without waiting for a message. Update it to wait for a `message` event before starting the stream:

```ts
import { ws } from 'msw'

const MOCK_RESPONSE =
  'Great point. Let me think through this carefully.\n\n' +
  'There are several considerations here:\n\n' +
  '1. **Performance** — the current approach has O(n²) complexity\n' +
  '2. **Correctness** — edge cases around empty inputs\n' +
  '3. **Maintainability** — the code is hard to follow\n\n' +
  'My recommendation is to refactor the core loop first.'

export const chatWsHandler = ws.link('/api/v0/ws/chat/:chatId').addEventListener('connection', ({ client }) => {
  client.addEventListener('message', () => {
    // Stream response word-by-word on receiving any message
    const words = MOCK_RESPONSE.split(' ')
    let i = 0
    const interval = setInterval(() => {
      if (i < words.length) {
        const chunk = (i === 0 ? '' : ' ') + words[i]
        client.send(JSON.stringify({ content: chunk, done: false }))
        i++
      } else {
        client.send(JSON.stringify({ content: '', done: true }))
        clearInterval(interval)
      }
    }, 40)

    client.addEventListener('close', () => clearInterval(interval))
  })
})
```

- [ ] **Step 2: Update `markdown-chat-view.tsx` — replace `simulateMarkdownStream` with `wsManager`**

Remove the `simulateMarkdownStream` import. Add `wsManager`:

```ts
import { wsManager } from '@/lib/ws/manager'
```

Remove `MOCK_RESPONSE` constant entirely.

Replace the `handleSubmit` streaming section:

```ts
// Before
setIsStreaming(true)
cancelStreamRef.current = simulateMarkdownStream(
  MOCK_RESPONSE,
  (chunk) => state.updateStreamingTurn(agentId, chunk),
  () => {
    state.finalizeStreamingTurn(agentId)
    cancelStreamRef.current = null
    setIsStreaming(false)
  },
)

// After
const endpoint = `/api/v0/ws/chat/${workspaceId}`
setIsStreaming(true)
const unsubscribe = wsManager.subscribe(endpoint, (msg: unknown) => {
  const m = msg as { content: string; done: boolean }
  if (!m.done) {
    state.updateStreamingTurn(agentId, m.content)
  } else {
    state.finalizeStreamingTurn(agentId)
    cancelStreamRef.current = null
    setIsStreaming(false)
    unsubscribe()
  }
})
wsManager.send(endpoint, { turnId: agentId, message: content })
cancelStreamRef.current = unsubscribe
```

- [ ] **Step 3: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/mocks/handlers/ws/chat.ts web/src/features/markdown-chat/components/markdown-chat-view.tsx
git commit -m "feat: replace simulateMarkdownStream with WSManager — streaming now goes through MSW WS layer"
```

---

## Task 11: Verify success criteria

- [ ] **Step 1: Check no component imports directly from `lib/mock/` (except MSW handlers and lib/mock files themselves)**

```bash
cd web
grep -r "from '@/lib/mock/" src --include="*.ts" --include="*.tsx" \
  | grep -v "src/mocks/" \
  | grep -v "src/lib/mock/" \
  | grep -v "__tests__"
```

Expected: **zero lines**. Any remaining output is a missed bypass — find and fix it.

- [ ] **Step 2: TypeScript clean**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors.

- [ ] **Step 3: All tests pass**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 4: Check no flow references remain**

```bash
cd web
grep -r "FlowDefinition\|flowsQuery\|fetchFlows\|FEATURE_DEV_FLOW\|WorkflowSlice\|workflow-slice\|use-workflow\|flowName\|currentState" \
  src --include="*.ts" --include="*.tsx" \
  | grep -v "__tests__" \
  | grep -v "node_modules"
```

Expected: zero lines.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: verify MSW migration and flow removal complete"
```
