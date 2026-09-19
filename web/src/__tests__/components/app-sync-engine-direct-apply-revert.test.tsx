import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'

// REGRESSION (restyle v2, K3 class f): a repo-scoped chat/folder drop applies
// the PATCH response straight into `useSidebarStore` (drop-actions.ts's
// `fireRowPlacementCall`, cases 'chat' / 'folder') and bumps the folder
// signal so the repo's chats/folders get re-read. Nothing writes the applied
// row into the `crowbar_chats` / `crowbar_folders` entity cache. Every
// `rebuildSidebar` (app-sync-engine.ts) is `setRepos(readVisibleRepoTree())`,
// i.e. a wholesale replacement from THAT cache — so the first rebuild that
// runs before the bumped reseed lands (the `placement_set` frame the daemon
// broadcast for the same PATCH already made the WORKSPACES stream reseed and
// schedule one) puts the row back where it was. The reseed then moves it
// again: a drop that lands, snaps back, and lands a second time.
const { wipe, subscribeEntityStream, fetchRepos, fetchWorkspaces, fetchFolders, fetchRepoChats } =
  vi.hoisted(() => ({
    wipe: vi.fn().mockResolvedValue(undefined),
    subscribeEntityStream: vi.fn(),
    fetchRepos: vi.fn(),
    fetchWorkspaces: vi.fn(),
    fetchFolders: vi.fn(),
    fetchRepoChats: vi.fn(),
  }))

vi.mock('@/lib/persistence/idb', async () => {
  const actual =
    await vi.importActual<typeof import('@/lib/persistence/idb')>('@/lib/persistence/idb')
  return { ...actual, maybeWipeOnVersionChange: wipe }
})

vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe: () => () => {}, send: vi.fn() },
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchFolders: (...args: unknown[]) => fetchFolders(...args),
  fetchRepoChats: (...args: unknown[]) => fetchRepoChats(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue(null),
  fetchHomeChats: vi.fn().mockResolvedValue([]),
  fetchHomeFolders: vi.fn().mockResolvedValue([]),
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { success } from '@/lib/loadable'
import { getAllEntities, upsertEntity } from '@/lib/persistence/entity-cache'
import { wipeEntityCache } from '@/lib/persistence/idb'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, getInitialState, type Repo } from '@/lib/store/sidebar'
import { applyChatPlacement } from '@/lib/store/applied-placement'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { EntityChange } from '@/lib/ws/entity-stream'
import type { ChatDTO, Project, RepoDTO } from '@/lib/types'

interface StreamOptions {
  endpoint: string
  onChange?: (change: EntityChange) => void
}

const opened: StreamOptions[] = []

const project: Project = { id: 'p1', name: 'p1', path: '/p/p1', lastActivity: new Date(0) }

const repoDTO: RepoDTO = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  path: '/p/p1/repo-alpha',
  defaultBranch: 'main',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
  order: 0,
}

function chat(id: string, order: number): ChatDTO {
  return {
    id,
    repoId: 'r1',
    projectId: 'p1',
    type: 'chat',
    workspaceId: '',
    ownsWorktree: false,
    parentId: '',
    title: id,
    order,
  }
}

/** Resolves on the first store publish `predicate` accepts — a real signal, never a sleep. */
function whenSidebar(predicate: (repos: readonly Repo[]) => boolean): Promise<void> {
  return new Promise((resolve) => {
    if (predicate(useSidebarStore.getState().repos)) {
      resolve()
      return
    }
    const unsubscribe = useSidebarStore.subscribe((state) => {
      if (!predicate(state.repos)) return
      unsubscribe()
      resolve()
    })
  })
}

/** Resolves once `rebuildSidebar` hands its cache-sourced rows to the store
 *  (`setRepos`), whether or not that publish changes anything. */
function whenRebuildApplied(): Promise<void> {
  return new Promise((resolve) => {
    const original = useSidebarStore.getState().setRepos
    useSidebarStore.setState({
      setRepos: (repos) => {
        original(repos)
        useSidebarStore.setState({ setRepos: original })
        resolve()
      },
    })
  })
}

/** Resolves once the engine has opened `repoId`'s tree gate — the last boot rebuild. */
function whenTreeSeeded(repoId: string): Promise<void> {
  return new Promise((resolve) => {
    if (useFolderSignalStore.getState().seededRepoIds.has(repoId)) {
      resolve()
      return
    }
    const unsubscribe = useFolderSignalStore.subscribe((state) => {
      if (!state.seededRepoIds.has(repoId)) return
      unsubscribe()
      resolve()
    })
  })
}

const orderOf = (repos: readonly Repo[], chatId: string): number | undefined =>
  repos.find((r) => r.id === 'r1')?.chats?.find((c) => c.id === chatId)?.order

beforeEach(async () => {
  vi.clearAllMocks()
  opened.length = 0
  subscribeEntityStream.mockImplementation((options: StreamOptions) => {
    opened.push(options)
    return vi.fn()
  })
  await wipeEntityCache()
  await upsertEntity('crowbar_repos', repoDTO)
  await upsertEntity('crowbar_chats', chat('c1', 0))
  await upsertEntity('crowbar_chats', chat('c2', 1))
  fetchRepos.mockResolvedValue([repoDTO])
  fetchWorkspaces.mockResolvedValue([])
  fetchFolders.mockResolvedValue([])
  fetchRepoChats.mockResolvedValue([chat('c1', 0), chat('c2', 1)])
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarStore.setState(getInitialState())
  useFolderSignalStore.setState({
    generations: {},
    seededRepoIds: new Set(),
    seededWorkspaceRepoIds: new Set(),
  })
  vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
  vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('a directly-applied chat placement vs the next cache-sourced rebuild', () => {
  it('survives a rebuild that runs while the bumped chats reseed is still in flight', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    // Boot: the repo seed lands, the tree reseed writes c1/c2, the rebuild
    // opens the gate with c1 at 0.
    opened.find((s) => s.endpoint === '/v0/projects/p1/repos')?.onChange?.({ kind: 'seed' })
    await whenSidebar((repos) => orderOf(repos, 'c1') === 0 && orderOf(repos, 'c2') === 1)
    await whenTreeSeeded('r1')

    // The drop: the PATCH answered {c1: order 1, shifted: [c2: order 0]}.
    // drop-actions.ts applies it and bumps the repo's tree signal; the
    // daemon's own re-read is still in flight.
    let resolveReseed: (rows: ChatDTO[]) => void = () => {}
    fetchRepoChats.mockImplementation(
      () =>
        new Promise<ChatDTO[]>((resolve) => {
          resolveReseed = resolve
        }),
    )
    const movedRepoId = await applyChatPlacement({ id: 'c1', parentId: '', order: 1 }, [
      { id: 'c2', parentId: '', order: 0 },
    ])
    expect(movedRepoId).toBe('r1')
    expect(orderOf(useSidebarStore.getState().repos, 'c1')).toBe(1)
    expect(orderOf(useSidebarStore.getState().repos, 'c2')).toBe(0)
    // The cache every rebuild reads from already carries the move, with the
    // rest of each row (type, workspaceId, ownsWorktree, title) intact.
    const cached = await getAllEntities<ChatDTO>('crowbar_chats')
    expect(cached.find((c) => c.id === 'c1')).toEqual({ ...chat('c1', 1), parentId: '' })
    expect(cached.find((c) => c.id === 'c2')).toEqual({ ...chat('c2', 0), parentId: '' })

    // The same PATCH's `placement_set` frame made the workspaces stream
    // reseed, and that seed schedules a rebuild of its own. Its `setRepos`
    // call is the signal that rebuild has run.
    const rebuilt = whenRebuildApplied()
    opened
      .find((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')
      ?.onChange?.({ kind: 'seed' })
    await rebuilt

    // The applied placement must not be undone by that rebuild. Today it is:
    // the rebuild reads `crowbar_chats`, which still says c1 is at 0.
    expect(orderOf(useSidebarStore.getState().repos, 'c1')).toBe(1)
    expect(orderOf(useSidebarStore.getState().repos, 'c2')).toBe(0)

    resolveReseed([chat('c1', 1), chat('c2', 0)])
    await whenSidebar((repos) => orderOf(repos, 'c1') === 1 && orderOf(repos, 'c2') === 0)
  })
})
