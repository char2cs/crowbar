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

test('setRepos is silent when a cache rebuild only recreated object identities', () => {
  const before = useSidebarStore.getState().repos
  const rebuilt = before.map((repo) => ({
    ...repo,
    workspaces: repo.workspaces.map((workspace) => ({ ...workspace })),
  }))
  let notifications = 0
  const unsubscribe = useSidebarStore.subscribe(() => {
    notifications += 1
  })

  useSidebarStore.getState().setRepos(rebuilt)

  expect(useSidebarStore.getState().repos).toBe(before)
  expect(notifications).toBe(0)
  unsubscribe()
})

test('setRepos preserves unaffected repos and rows when one workspace changed', () => {
  const before = useSidebarStore.getState().repos
  const rebuilt = before.map((repo, repoIndex) => ({
    ...repo,
    workspaces: repo.workspaces.map((workspace, workspaceIndex) => ({
      ...workspace,
      ...(repoIndex === 0 && workspaceIndex === 0 ? { working: true } : {}),
    })),
  }))

  useSidebarStore.getState().setRepos(rebuilt)

  const after = useSidebarStore.getState().repos
  expect(after).not.toBe(before)
  expect(after[0]).not.toBe(before[0])
  expect(after[0].workspaces[0]).not.toBe(before[0].workspaces[0])
  expect(after[0].workspaces[1]).toBe(before[0].workspaces[1])
  expect(after[1]).toBe(before[1])
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

// A branch rename moves the git branch AND the workspace's on-disk directory,
// so it is the daemon's to perform; the renamed row comes back through
// applyWorkspaceDTO. The store deliberately exposes no local rename — one was
// what made the sidebar relabel a row while the branch never changed.
test('a renamed workspace arrives through applyWorkspaceDTO', () => {
  const before = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!

  useSidebarStore.getState().applyWorkspaceDTO({
    ...before,
    repoId: 'crowbar',
    branch: 'feature/renamed',
  } as never)

  const workspaces = useSidebarStore.getState().repos.flatMap((r) => r.workspaces)
  expect(workspaces.find((w) => w.id === 'ws3')!.branch).toBe('feature/renamed')
  expect(workspaces.find((w) => w.id === 'ws1')!.branch).toBe('enhancement/scaffold')
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

// applyWorkspaceDTO is now the ONLY path a live workspace frame takes (the
// whole-tree rebuild it used to trigger is reserved for seeds), so everything a
// rebuild used to fix up has to happen here.
test('applyWorkspaceDTO never adds the default workspace as a tree row', () => {
  useSidebarStore
    .getState()
    .applyWorkspaceDTO(dto('ws-default', 'crowbar', { isDefault: true, branch: 'develop' }))
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
  expect(repo.workspaces.map((w) => w.id)).not.toContain('ws-default')
})

test('applyWorkspaceDTO lifts the default workspace onto the repo header', () => {
  // The repo avatar's agent spinner and the lock gating read defaultWorking /
  // defaultWorkspaceStatus. Dropping the frame here froze both at page-load value.
  useSidebarStore.getState().applyWorkspaceDTO(
    dto('ws-default', 'crowbar', {
      isDefault: true,
      branch: 'develop',
      working: true,
      status: 'locked',
    }),
  )
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
  expect(repo.defaultWorkspaceId).toBe('ws-default')
  expect(repo.defaultBranch).toBe('develop')
  expect(repo.defaultWorking).toBe(true)
  expect(repo.defaultWorkspaceStatus).toBe('locked')

  useSidebarStore
    .getState()
    .applyWorkspaceDTO(
      dto('ws-default', 'crowbar', { isDefault: true, branch: 'develop', working: false }),
    )
  expect(useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!.defaultWorking).toBe(
    false,
  )
})

// A frame is merged with {...w, ...ws}, so a field the daemon CLEARED has to
// arrive as an explicit undefined/'' or the stale value survives forever. The
// whole-tree rebuild that every frame used to trigger hid this; incremental
// merge is the only path now.
test('applyWorkspaceDTO clears a parentId the daemon dropped (reparent to root)', () => {
  useSidebarStore.getState().applyWorkspaceDTO(dto('ws3', 'crowbar', { parentId: '' }))
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.parentId).toBeUndefined()
})

test('applyWorkspaceDTO clears parentBranch and prUrl the daemon dropped', () => {
  useSidebarStore
    .getState()
    .applyWorkspaceDTO(
      dto('ws3', 'crowbar', { parentBranch: 'develop', prUrl: 'https://example.com/pr/1' }),
    )
  useSidebarStore.getState().applyWorkspaceDTO(dto('ws3', 'crowbar'))
  const ws = useSidebarStore
    .getState()
    .repos.flatMap((r) => r.workspaces)
    .find((w) => w.id === 'ws3')!
  expect(ws.parentBranch).toBeUndefined()
  expect(ws.prUrl).toBeUndefined()
})

test('applyWorkspaceDTO keeps the tree identity when a frame changes nothing', () => {
  // A reconnect reseed or a duplicate push must not hand out a new repos array:
  // every sidebar subscriber re-derives on identity, so a no-op frame would
  // still cost a render pass across the whole tree.
  const first = dto('ws3', 'crowbar', { status: 'pr-open', branch: 'feature/app-design' })
  useSidebarStore.getState().applyWorkspaceDTO(first)
  const after = useSidebarStore.getState().repos
  useSidebarStore.getState().applyWorkspaceDTO({ ...first })
  expect(useSidebarStore.getState().repos).toBe(after)
})

test('applyWorkspaceDTO keeps the tree identity when a default frame changes nothing', () => {
  const frame = dto('ws-default', 'crowbar', { isDefault: true, branch: 'develop' })
  useSidebarStore.getState().applyWorkspaceDTO(frame)
  const after = useSidebarStore.getState().repos
  useSidebarStore.getState().applyWorkspaceDTO({ ...frame })
  expect(useSidebarStore.getState().repos).toBe(after)
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
