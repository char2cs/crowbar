import { beforeEach, expect, test } from 'vitest'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

const FIXTURE_REPOS: Repo[] = [
  {
    id: 'crowbar',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      {
        id: 'ws3',
        branch: 'feature/app-design',
        parentId: 'ws-develop',
        status: 'pr-open',
        added: 5672,
        age: '16h ago',
      },
      {
        id: 'ws1',
        branch: 'enhancement/scaffold',
        parentId: 'ws3',
        status: 'new',
        working: true,
        added: 22892,
        age: '3d ago',
      },
      {
        id: 'ws-fix',
        branch: 'fix/toolbar-crash',
        parentId: 'ws3',
        status: 'new',
        age: 'just now',
      },
      {
        id: 'ws2',
        branch: 'feature/api-backend',
        parentId: 'ws-develop',
        status: 'pr-merged',
        added: 27347,
        deleted: 455,
        age: '1d ago',
      },
      {
        id: 'ws4',
        branch: 'feature/ws-channels',
        parentId: 'ws-develop',
        status: 'pr-open',
        added: 8841,
        deleted: 203,
        age: '2d ago',
      },
      {
        id: 'ws5',
        branch: 'refactor/query-layer',
        parentId: 'ws-develop',
        status: 'pr-conflicts',
        added: 103482,
        deleted: 88910,
        age: '5d ago',
      },
      {
        id: 'ws6',
        branch: 'chore/bump-deps',
        parentId: 'ws-develop',
        status: 'pr-closed',
        added: 312,
        deleted: 298,
        age: '6d ago',
      },
    ],
  },
  {
    id: 'quiver-core',
    name: 'quiver.core',
    avatarLabel: 'Q',
    avatarColor: 'bg-emerald-700',
    workspaces: [
      { id: 'qc-develop', branch: 'develop', status: 'locked', age: '—' },
      {
        id: 'qc1',
        branch: 'feature/old-auth',
        parentId: 'qc-develop',
        status: 'pr-closed',
        age: '3d ago',
      },
      {
        id: 'qc2',
        branch: 'feature/oauth2',
        parentId: 'qc-develop',
        status: 'pr-open',
        added: 4521,
        deleted: 89,
        age: '1d ago',
      },
    ],
  },
]

beforeEach(() => {
  useSidebarStore.setState({
    repos: FIXTURE_REPOS.map((r) => ({ ...r, workspaces: [...r.workspaces] })),
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    activeTab: 'workspaces',
  })
})

test('addWorkspace appends to the correct repo', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
  expect(repo.workspaces.some((w) => w.id === 'ws-new')).toBe(true)
})

test('addWorkspace does not affect other repos', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const other = useSidebarStore.getState().repos.find((r) => r.id === 'quiver-core')!
  expect(other.workspaces.some((w) => w.id === 'ws-new')).toBe(false)
})

test('deleteWorkspace removes from repo', () => {
  useSidebarStore.getState().deleteWorkspace('ws3')
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
  expect(repo.workspaces.some((w) => w.id === 'ws3')).toBe(false)
})

test('toggleRepo flips collapsed state', () => {
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(true)
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(false)
})

test('addWorkspace stores parentId when provided', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-child', 'feature/child', 'ws-develop')
  const ws = useSidebarStore
    .getState()
    .repos.find((r) => r.id === 'crowbar')!
    .workspaces.find((w) => w.id === 'ws-child')!
  expect(ws.parentId).toBe('ws-develop')
})

test('addWorkspace stores no parentId when omitted', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-root', 'feature/root')
  const ws = useSidebarStore
    .getState()
    .repos.find((r) => r.id === 'crowbar')!
    .workspaces.find((w) => w.id === 'ws-root')!
  expect(ws.parentId).toBeUndefined()
})

test('renameWorkspace updates branch on matching workspace', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.branch).toBe('feature/renamed')
})

test('renameWorkspace leaves other workspaces unchanged', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws1')!
  expect(ws.branch).toBe('enhancement/scaffold')
})

test('reparentWorkspace changes parentId', () => {
  useSidebarStore.getState().reparentWorkspace('ws2', 'ws3')
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws2')!
  expect(ws.parentId).toBe('ws3')
})

test('reparentWorkspace to undefined makes workspace a repo root', () => {
  useSidebarStore.getState().reparentWorkspace('ws3', undefined)
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.parentId).toBeUndefined()
})

test('reparentWorkspace rejects cycles: descendant cannot become ancestor', () => {
  // ws3 is a child of ws-develop; making ws-develop a child of ws3 would cycle
  useSidebarStore.getState().reparentWorkspace('ws-develop', 'ws3')
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws-develop')!
  expect(ws.parentId).toBeUndefined() // unchanged
})

test('reparentWorkspace rejects cross-repo moves', () => {
  // qc1 is in quiver-core; ws3 is in crowbar
  useSidebarStore.getState().reparentWorkspace('ws3', 'qc1')
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.parentId).toBe('ws-develop') // unchanged
})

// ---------------------------------------------------------------------------
// §6 WS-driven cache: applyWorkspaceDTO merges a complete WorkspaceDTO by id
// (insert / replace) and a status:'deleted' tombstone removes it — there is no
// optimistic BFS delete anymore; the backend owns the cascade.
// ---------------------------------------------------------------------------
import type { WorkspaceDTO } from '@/lib/types'

const dto = (id: string, repoId: string, over: Partial<WorkspaceDTO> = {}): WorkspaceDTO => ({
  id,
  repoId,
  projectId: 'p1',
  branch: 'feature/x',
  parentId: '',
  forkPointSha: '',
  status: 'new',
  working: false,
  lastError: '',
  added: 0,
  deleted: 0,
  mergeStrategy: 'merge',
  canMergeLocally: false,
  mergeConflicts: false,
  parentBranch: '',
  prUrl: '',
  prTitle: '',
  prTargetBranch: '',
  ...over,
})

test('applyWorkspaceDTO inserts a new workspace into its repo', () => {
  useSidebarStore.getState().applyWorkspaceDTO(dto('ws-new', 'crowbar', { branch: 'feature/new' }))
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
  const created = repo.workspaces.find((w) => w.id === 'ws-new')!
  expect(created).toBeDefined()
  expect(created.branch).toBe('feature/new')
  expect(created.status).toBe('new')
})

test('applyWorkspaceDTO replaces fields of an existing workspace by id', () => {
  useSidebarStore
    .getState()
    .applyWorkspaceDTO(dto('ws3', 'crowbar', { status: 'pr-merged', working: true }))
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.status).toBe('pr-merged')
  expect(ws.working).toBe(true)
})

test('applyWorkspaceDTO with status:deleted removes the workspace (no BFS)', () => {
  useSidebarStore.getState().applyWorkspaceDTO(dto('ws3', 'crowbar', { status: 'deleted' }))
  const ids = useSidebarStore.getState().repos.flatMap((r) => r.workspaces.map((w) => w.id))
  expect(ids).not.toContain('ws3')
  // children of ws3 are NOT removed locally — the backend emits a tombstone per id
  expect(ids).toContain('ws1')
})

test('applyWorkspaceDTO ignores a workspace for an unknown repo', () => {
  useSidebarStore.getState().applyWorkspaceDTO(dto('ws-x', 'no-such-repo'))
  const ids = useSidebarStore.getState().repos.flatMap((r) => r.workspaces.map((w) => w.id))
  expect(ids).not.toContain('ws-x')
})

import { loadSidebarUI } from '@/lib/persistence/sidebar-ui'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { describe } from 'vitest'

describe('toggleRepo persistence', () => {
  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState({
      repos: [],
      collapsedRepos: new Set<string>(),
      collapsedWorkspaces: new Set<string>(),
      activeTab: 'workspaces',
    })
  })

  test('writes collapsed state to IDB after toggling on', async () => {
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise((r) => setTimeout(r, 20))
    const saved = await loadSidebarUI()
    expect(saved?.collapsedRepos).toContain('crowbar')
  })

  test('removes repo from IDB after toggling off', async () => {
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise((r) => setTimeout(r, 20))
    useSidebarStore.getState().toggleRepo('crowbar')
    await new Promise((r) => setTimeout(r, 20))
    const saved = await loadSidebarUI()
    expect(saved?.collapsedRepos).not.toContain('crowbar')
  })
})
