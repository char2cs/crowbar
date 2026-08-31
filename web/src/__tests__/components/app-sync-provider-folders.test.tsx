/**
 * The folders reseed-on-signal mechanism (Task 34).
 *
 * The backend's dedicated folders REST+WS resource was deleted (its own plan
 * is closed) — folders are `domain.Chat` rows now, read only through
 * `.../chats/folders`, and the only live-update path left is an id-only
 * `folder_*` invalidation frame on a WORKSPACE's chats WS
 * (use-workspace-agent-chats-stream.ts), which bumps `useFolderSignalStore`'s
 * per-repo generation. `app-sync-provider.tsx`'s folders subscription watches
 * that generation and re-runs `fetchFolders` whenever it moves — a full
 * reseed, not an incremental merge, because there is no per-DTO frame left to
 * merge.
 *
 * Unlike `app-sync-provider.test.tsx` (which mocks `useWorkspaceListStore`'s
 * `fetch` to a no-op spy and only checks call counts), this file lets the
 * REAL rebuild pipeline run — seeding the entity-cache IndexedDB stores
 * directly and reading `Repo.folders` back off the real assembled tree — so
 * it proves the row a signal-triggered reseed writes actually reaches the
 * sidebar, not just that a function was called.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'

const { wipe, subscribeEntityStream, fetchRepos, fetchWorkspaces, fetchFolders } = vi.hoisted(
  () => ({
    wipe: vi.fn().mockResolvedValue(undefined),
    subscribeEntityStream: vi.fn(),
    fetchRepos: vi.fn(),
    fetchWorkspaces: vi.fn(),
    fetchFolders: vi.fn(),
  }),
)

vi.mock('@/lib/persistence/idb', async () => {
  const actual =
    await vi.importActual<typeof import('@/lib/persistence/idb')>('@/lib/persistence/idb')
  return { ...actual, maybeWipeOnVersionChange: wipe }
})

// repos/workspaces never need to be real here — this file only exercises the
// FOLDERS path, which touches no WS at all. A no-op recorder keeps their
// subscriptions inert without dragging a real WebSocket into the test.
vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/api', () => ({
  API_BASE: '',
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchFolders: (...args: unknown[]) => fetchFolders(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue(null),
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { success } from '@/lib/loadable'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { resetDB } from '@/lib/persistence/idb'
import type { FolderDTO, Project, RepoDTO } from '@/lib/types'

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

const folderDTO = (id: string, repoId: string, over: Partial<FolderDTO> = {}): FolderDTO => ({
  id,
  repoId,
  projectId: 'p1',
  parentId: '',
  name: id,
  order: 0,
  ...over,
})

/** A minimal `Repo` row — just enough for desiredKeys() to open r1's folders
 *  subscription. The REAL rebuild (readVisibleRepoTree) supersedes this from
 *  the entity cache on the very next settle(); it only bootstraps "r1 is a
 *  known, expanded repo" the way a live repos stream normally would. */
const repoRow = (id: string, projectId: string) => ({
  id,
  projectId,
  name: id,
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  workspaces: [],
})

async function settle(ms = 40): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(async () => {
  vi.clearAllMocks()
  // A fresh in-memory IndexedDB per test — the entity-cache DB handle is
  // module-memoized and otherwise leaks rows across tests in this file.
  resetDB()
  globalThis.indexedDB = new IDBFactory()

  subscribeEntityStream.mockImplementation(() => vi.fn())
  fetchRepos.mockResolvedValue([])
  fetchWorkspaces.mockResolvedValue([])
  fetchFolders.mockResolvedValue([])

  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project('p1')]) })
  useSidebarStore.setState({
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
  useFolderSignalStore.setState({ generations: {} })
  // Seeded with REAL timers still active — fake-indexeddb's open/transaction
  // completion relies on scheduling that vi.useFakeTimers() would otherwise
  // have to be manually advanced through, and this write has nothing to do
  // with the mechanism under test.
  await upsertEntity('crowbar_repos', repoDTO('r1', 'p1'))
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('folders reseed-on-signal', () => {
  it("seeds a repo's folders once on open, without waiting for any signal", async () => {
    fetchFolders.mockResolvedValue([folderDTO('f1', 'r1')])

    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repoRow('r1', 'p1')])
    })
    await settle()

    expect(fetchFolders).toHaveBeenCalledWith('p1', 'r1')
    expect(
      useSidebarStore
        .getState()
        .repos.find((r) => r.id === 'r1')
        ?.folders?.map((f) => f.id),
    ).toEqual(['f1'])
  })

  it('a folder_* signal on the workspace stream reseeds this repo, and the new row reaches Repo.folders', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repoRow('r1', 'p1')])
    })
    await settle()
    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.folders).toBeUndefined()

    // The signal use-workspace-agent-chats-stream.ts fires on a folder_created/
    // folder_updated/folder_deleted frame.
    fetchFolders.mockResolvedValue([folderDTO('f1', 'r1', { name: 'Spikes' })])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()

    const r1 = useSidebarStore.getState().repos.find((r) => r.id === 'r1')
    expect(r1?.folders).toEqual([
      { id: 'f1', repoId: 'r1', parentId: undefined, name: 'Spikes', order: 0 },
    ])
  })

  it('a reseed drops a folder the fresh list no longer carries (tombstone via diff)', async () => {
    fetchFolders.mockResolvedValue([folderDTO('f1', 'r1')])
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repoRow('r1', 'p1')])
    })
    await settle()
    expect(
      useSidebarStore
        .getState()
        .repos.find((r) => r.id === 'r1')
        ?.folders?.map((f) => f.id),
    ).toEqual(['f1'])

    fetchFolders.mockResolvedValue([])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()

    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.folders).toBeUndefined()
  })

  it('a signal for a DIFFERENT repo does not reseed this one', async () => {
    fetchFolders.mockResolvedValue([folderDTO('f1', 'r1')])
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repoRow('r1', 'p1')])
    })
    await settle()
    fetchFolders.mockClear()

    act(() => {
      useFolderSignalStore.getState().bump('r2')
    })
    await settle()

    expect(fetchFolders).not.toHaveBeenCalled()
  })

  it('drops the signal subscription on unmount', async () => {
    const { unmount } = render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repoRow('r1', 'p1')])
    })
    await settle()
    unmount()
    fetchFolders.mockClear()

    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()

    expect(fetchFolders).not.toHaveBeenCalled()
  })
})
