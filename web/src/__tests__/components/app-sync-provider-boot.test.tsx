import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, waitFor } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'

// End-to-end boot, with the REAL entity-stream, the REAL entity cache and the
// REAL tree builder — only the network is faked. The unit tests around this one
// mock subscribeEntityStream, so they can prove which streams get opened but
// not that the sequence they belong to actually fills the sidebar. The failure
// this guards is the one those tests cannot see and that a user would meet as a
// permanently empty sidebar: repo seed → cache → rebuild → reconcile → per-repo
// workspace stream → cache → rebuild → rows, where subscribing the workspace
// streams now depends on the repos having landed in the tree first.
//
// Real timers throughout, deliberately: fake-indexeddb schedules its own work,
// and every assertion below blocks on the real signal (the tree contents) via
// waitFor rather than on an elapsed duration.
const { fetchRepos, fetchWorkspaces, fetchFolders, fetchRepoChats, subscribe } = vi.hoisted(() => ({
  fetchRepos: vi.fn(),
  fetchWorkspaces: vi.fn(),
  fetchFolders: vi.fn(),
  fetchRepoChats: vi.fn().mockResolvedValue([]),
  subscribe: vi.fn(),
}))

// Only the network seams are faked. `workspaceDTOFromWorktreeFrame` stays REAL:
// a worktree's live updates ride the chat lifecycle socket, so that mapper is
// part of the daemon→cache→tree path this file exists to exercise end to end.
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchFolders: (...args: unknown[]) => fetchFolders(...args),
  fetchRepoChats: (...args: unknown[]) => fetchRepoChats(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue(null),
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe: (...args: unknown[]) => subscribe(...args), send: vi.fn() },
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { success } from '@/lib/loadable'
import { resetDB } from '@/lib/persistence/idb'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { FolderDTO, Project, RepoDTO, WorkspaceDTO } from '@/lib/types'

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date(0),
})

const repoDTO = (id: string, projectId: string): RepoDTO => ({
  id,
  projectId,
  name: id,
  path: `/p/${id}`,
  defaultBranch: 'main',
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
})

const wsDTO = (
  id: string,
  repoId: string,
  projectId: string,
  over: Partial<WorkspaceDTO> = {},
): WorkspaceDTO => ({
  id,
  repoId,
  projectId,
  branch: `feature/${id}`,
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

/**
 * The chat-stream frame that carries `ws`'s worktree state.
 *
 * A worktree is HELD BY A CHAT, so it has no push channel of its own any more:
 * its live updates arrive as `worktree_state` events on the repo's chat feed,
 * with the git half nested inside. The chat is named twice on purpose —
 * `chatId` is who the event is about, `worktree.owningChatId` is who holds the
 * worktree — because a thread of that chat receives the SAME worktree object and
 * is filtered out by exactly that comparison.
 */
const worktreeFrame = (ws: WorkspaceDTO, chatId = `chat-${ws.id}`, owningChatId = chatId) => ({
  chatId,
  workspaceId: ws.id,
  repoId: ws.repoId,
  kind: 'worktree_state',
  working: ws.working,
  worktree: {
    branch: ws.branch,
    status: ws.status,
    lastError: ws.lastError,
    working: ws.working,
    isDefault: ws.isDefault,
    added: ws.added,
    deleted: ws.deleted,
    mergeStrategy: ws.mergeStrategy,
    canMergeLocally: ws.canMergeLocally,
    mergeConflicts: ws.mergeConflicts,
    parentBranch: ws.parentBranch,
    prUrl: ws.prUrl,
    prTitle: ws.prTitle,
    prTargetBranch: ws.prTargetBranch,
    localPath: ws.localPath,
    heldByPath: ws.heldByPath,
    forkPointSha: ws.forkPointSha,
    parentId: ws.parentId,
    owningChatId,
  },
})

const folderDTO = (
  id: string,
  repoId: string,
  projectId: string,
  over: Partial<FolderDTO> = {},
): FolderDTO => ({
  id,
  repoId,
  projectId,
  name: id,
  order: 0,
  ...over,
})

/** Live frame handlers the code under test registered, by endpoint. */
const handlers = new Map<string, (data: unknown) => void>()

const repoIds = (): string[] =>
  useSidebarStore
    .getState()
    .repos.map((r) => r.id)
    .sort()

const workspaceIdsOf = (repoId: string): string[] =>
  useSidebarStore
    .getState()
    .repos.find((r) => r.id === repoId)
    ?.workspaces.map((w) => w.id) ?? []

const folderIdsOf = (repoId: string): string[] =>
  useSidebarStore
    .getState()
    .repos.find((r) => r.id === repoId)
    ?.folders?.map((f) => f.id) ?? []

/** Deliver a live frame on an endpoint and let the resulting merge commit. */
async function push(endpoint: string, frame: unknown): Promise<void> {
  const handler = handlers.get(endpoint)
  expect(handler, `no live subscription on ${endpoint}`).toBeDefined()
  await act(async () => {
    handler!(frame)
  })
}

beforeEach(async () => {
  vi.clearAllMocks()
  handlers.clear()
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  subscribe.mockImplementation((endpoint: string, cb: (data: unknown) => void) => {
    handlers.set(endpoint, cb)
    return () => handlers.delete(endpoint)
  })
  fetchRepos.mockImplementation((projectId: string) =>
    Promise.resolve(projectId === 'p1' ? [repoDTO('r1', 'p1')] : [repoDTO('r2', 'p2')]),
  )
  fetchWorkspaces.mockImplementation((_projectId: string, repoId: string) =>
    Promise.resolve(
      repoId === 'r1'
        ? [
            wsDTO('w-default', 'r1', 'p1', { isDefault: true, branch: 'main' }),
            wsDTO('w1', 'r1', 'p1'),
          ]
        : [wsDTO('w2', 'r2', 'p2')],
    ),
  )
  fetchFolders.mockImplementation((projectId: string, repoId: string) =>
    Promise.resolve(repoId === 'r1' ? [folderDTO('f1', 'r1', projectId, { name: 'spikes' })] : []),
  )
  useProjectStore.setState({ activeProjectId: 'p1' })
  // p2 is KNOWN and therefore open: every project the list delivers is visible
  // until the user folds it away.
  useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
  useSidebarStore.setState({
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
  useFolderSignalStore.setState({ generations: {} })
  vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
  vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

/** Mount and wait until the active project's tree has actually filled in. */
async function boot(): Promise<void> {
  render(
    <AppSyncProvider>
      <div />
    </AppSyncProvider>,
  )
  await waitFor(() => expect(workspaceIdsOf('r1')).toEqual(['w1']))
}

describe('AppSyncProvider boot, end to end', () => {
  it('fills the sidebar for the active project from seeds alone', async () => {
    await boot()
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'r1')!
    expect(repoIds()).toContain('r1')
    // The default (main-worktree) workspace is the repo header, not a row.
    expect(repo.defaultWorkspaceId).toBe('w-default')
    expect(repo.defaultBranch).toBe('main')
  })

  it('puts the daemon’s folders on the repo, from the seed alone', async () => {
    // The regression this pins: every folder test before it seeded repo.folders
    // straight into the sidebar store, so nothing ever exercised daemon → cache
    // → tree. With the folders stream unregistered and readVisibleRepoTree
    // building without them, the whole feature rendered nothing while every
    // suite stayed green — a folder the daemon had already created simply never
    // appeared in the sidebar.
    await boot()
    await waitFor(() => expect(folderIdsOf('r1')).toEqual(['f1']))
    const folder = useSidebarStore.getState().repos.find((r) => r.id === 'r1')!.folders![0]
    expect(folder.name).toBe('spikes')
    expect(folder.repoId).toBe('r1')
    // A folder is repo-scoped: r2 has none, and the shared cache must not leak
    // r1's onto it.
    await waitFor(() => expect(workspaceIdsOf('r2')).toEqual(['w2']))
    expect(folderIdsOf('r2')).toEqual([])
  })

  // Task 34: the dedicated folders REST+WS resource is gone (the backend plan
  // that carried it is closed) — there is no per-DTO push frame left to
  // deliver a folder change. use-workspace-agent-chats-stream.ts bumps
  // useFolderSignalStore's per-repo generation on a folder_* frame instead,
  // and app-sync-provider.tsx's folders subscription reseeds (a full
  // fetchFolders re-run, not a merge) whenever that generation moves — proven
  // here the same end-to-end way the workspace frames above are.
  it('a folder_* signal reseeds the repo, and the row the daemon just created appears', async () => {
    // What the user actually does: create a folder from a row's `+`. The
    // create's own response lands it immediately (sidebar-placement.ts); this
    // proves the OTHER path — a change made elsewhere reaching this client via
    // the signal + reseed.
    await boot()
    await waitFor(() => expect(folderIdsOf('r1')).toEqual(['f1']))

    fetchFolders.mockResolvedValue([
      folderDTO('f1', 'r1', 'p1', { name: 'spikes' }),
      folderDTO('f2', 'r1', 'p1', { name: 'nested', parentId: 'f1', order: 1 }),
    ])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await waitFor(() => expect(folderIdsOf('r1')).toEqual(['f1', 'f2']))
    const nested = useSidebarStore
      .getState()
      .repos.find((r) => r.id === 'r1')!
      .folders!.find((f) => f.id === 'f2')!
    expect(nested).toMatchObject({ name: 'nested', parentId: 'f1', order: 1 })
  })

  it('a folder_* signal reseed drops a row the fresh list no longer carries', async () => {
    await boot()
    await waitFor(() => expect(folderIdsOf('r1')).toEqual(['f1']))

    fetchFolders.mockResolvedValue([])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await waitFor(() => expect(folderIdsOf('r1')).toEqual([]))
    // The repo's workspace rows are untouched by any of it.
    expect(workspaceIdsOf('r1')).toEqual(['w1'])
  })

  it('a live worktree_state frame updates the row it names', async () => {
    await boot()
    await push(
      '/v0/projects/p1/repos/r1/chats/ws',
      worktreeFrame(wsDTO('w1', 'r1', 'p1', { working: true, status: 'pr-open' })),
    )
    await waitFor(() => {
      const w1 = useSidebarStore.getState().repos[0].workspaces.find((w) => w.id === 'w1')!
      expect(w1.working).toBe(true)
      expect(w1.status).toBe('pr-open')
    })
  })

  it('a live frame for the DEFAULT workspace keeps the repo header live', async () => {
    // The repo avatar's agent spinner reads repo.defaultWorking. It is not a
    // tree row, so the incremental merge has to lift it onto the header itself.
    await boot()
    await push(
      '/v0/projects/p1/repos/r1/chats/ws',
      worktreeFrame(
        wsDTO('w-default', 'r1', 'p1', { isDefault: true, branch: 'main', working: true }),
      ),
    )
    await waitFor(() => expect(useSidebarStore.getState().repos[0].defaultWorking).toBe(true))
    // ...and it still is not a row.
    expect(workspaceIdsOf('r1')).toEqual(['w1'])
  })

  it('a live tombstone removes the row', async () => {
    await boot()
    await push(
      '/v0/projects/p1/repos/r1/chats/ws',
      worktreeFrame(wsDTO('w1', 'r1', 'p1', { status: 'deleted' })),
    )
    await waitFor(() => expect(workspaceIdsOf('r1')).toEqual([]))
  })

  // The chat feed carries THREE vocabularies (chats, runners, folders) and only
  // one of them is about a worktree. Everything else has to fall straight
  // through, or the hottest frames in the app (a turn starting and stopping)
  // would each write the workspace cache.
  it('ignores every frame on the chat feed that is not this chat’s worktree state', async () => {
    await boot()
    await push('/v0/projects/p1/repos/r1/chats/ws', {
      chatId: 'chat-w1',
      workspaceId: 'w1',
      kind: 'turn_started',
      working: true,
    })
    await push('/v0/projects/p1/repos/r1/chats/ws', {
      chatId: 'chat-w1',
      kind: 'folder_created',
      folderId: 'f9',
    })
    // A THREAD of the owning chat gets the same worktree object; only the
    // owning row is the worktree's row.
    await push(
      '/v0/projects/p1/repos/r1/chats/ws',
      worktreeFrame(wsDTO('w1', 'r1', 'p1', { status: 'deleted' }), 'chat-w1-thread', 'chat-w1'),
    )

    expect(workspaceIdsOf('r1')).toEqual(['w1'])
    const w1 = useSidebarStore.getState().repos[0].workspaces.find((w) => w.id === 'w1')!
    expect(w1.working).toBe(false)
  })

  it('a reconnect sentinel reseeds without emptying the tree', async () => {
    await boot()
    await push('/v0/projects/p1/repos/r1/chats/ws', { reconnected: true })
    await waitFor(() => expect(workspaceIdsOf('r1')).toEqual(['w1']))
  })

  it('fills every known project at boot, not just the active one', async () => {
    await boot()
    await waitFor(() => expect(workspaceIdsOf('r2')).toEqual(['w2']))
    expect(repoIds()).toEqual(['r1', 'r2'])
    expect(workspaceIdsOf('r1')).toEqual(['w1'])
  })

  it('collapsing a project keeps its rows cached for an instant re-open', async () => {
    await boot()
    await waitFor(() => expect(repoIds()).toEqual(['r1', 'r2']))
    const cached = useSidebarStore.getState().repos.find((repo) => repo.id === 'r2')

    await act(async () => {
      useSidebarStore.getState().toggleProject('p2')
    })
    expect(useSidebarStore.getState().collapsedProjects.has('p2')).toBe(true)
    expect(repoIds()).toEqual(['r1', 'r2'])
    expect(useSidebarStore.getState().repos.find((repo) => repo.id === 'r2')).toBe(cached)
    // ...and the active project's rows are still there.
    expect(workspaceIdsOf('r1')).toEqual(['w1'])
  })

  it('re-opening a project exposes the already-cached rows synchronously', async () => {
    await boot()
    const cached = useSidebarStore.getState().repos.find((repo) => repo.id === 'r2')
    await act(async () => {
      useSidebarStore.getState().toggleProject('p2')
    })

    await act(async () => {
      useSidebarStore.getState().toggleProject('p2')
    })
    expect(useSidebarStore.getState().collapsedProjects.has('p2')).toBe(false)
    expect(repoIds()).toEqual(['r1', 'r2'])
    expect(useSidebarStore.getState().repos.find((repo) => repo.id === 'r2')).toBe(cached)
  })
})
