/**
 * The per-repo TREE reseed-on-signal mechanism — folders (Task 34) and the chat
 * rows that ride the same loop (Task D).
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
 * directly and reading `Repo.folders` / `Repo.chats` back off the real
 * assembled tree — so it proves the row a signal-triggered reseed writes
 * actually reaches the sidebar, not just that a function was called.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'

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

// repos/workspaces never need to be real here — this file only exercises the
// per-repo TREE path, which touches no WS at all. A no-op recorder keeps their
// subscriptions inert without dragging a real WebSocket into the test.
vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/api', () => ({
  API_BASE: '',
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchFolders: (...args: unknown[]) => fetchFolders(...args),
  fetchRepoChats: (...args: unknown[]) => fetchRepoChats(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue(null),
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { success } from '@/lib/loadable'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { resetDB } from '@/lib/persistence/idb'
import type { ChatDTO, FolderDTO, Project, RepoDTO } from '@/lib/types'

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

const chatDTO = (id: string, repoId: string, over: Partial<ChatDTO> = {}): ChatDTO => ({
  id,
  repoId,
  projectId: 'p1',
  workspaceId: '',
  parentId: '',
  title: id,
  order: 0,
  ...over,
})

/** A minimal `Repo` row — just enough for desiredKeys() to open r1's tree
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
  fetchRepoChats.mockResolvedValue([])

  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project('p1')]) })
  useSidebarStore.setState({
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
  useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set() })
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

  // Fix round 1 (C1): a folder write applied ONLY through `applyFolderDTO`
  // (useSidebarStore) is invisible to `crowbar_folders` — the IndexedDB cache
  // every rebuild (readVisibleRepoTree) reads folders from exclusively. The
  // real write sites (row-actions.ts, drop-actions.ts, removal-commit.ts)
  // now also `bump` the repo's folder signal so the cache converges; these
  // two tests simulate that same two-step effect (direct store write + bump)
  // and prove a rebuild that has nothing to do with this folder — the same
  // kind that fires routinely, e.g. any repo's `defaultWorking` flipping —
  // does not revert it.
  it('a folder created locally survives an unrelated rebuild once the write also bumps the signal', async () => {
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

    // What row-actions.ts's performCreateFolder does on a successful create:
    // apply the response directly (instant feedback) AND bump the signal
    // (writes crowbar_folders).
    fetchFolders.mockResolvedValue([folderDTO('f1', 'r1', { name: 'New folder' })])
    act(() => {
      useSidebarStore.getState().applyFolderDTO(folderDTO('f1', 'r1', { name: 'New folder' }))
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()
    expect(
      useSidebarStore
        .getState()
        .repos.find((r) => r.id === 'r1')
        ?.folders?.map((f) => f.id),
    ).toEqual(['f1'])

    // An unrelated rebuild — here, a second repo landing, which is what
    // scheduleRebuild()'s isOpening branch fires on; any other reason a
    // rebuild runs would exercise the same crowbar_folders read.
    act(() => {
      useSidebarStore
        .getState()
        .setRepos([...useSidebarStore.getState().repos, repoRow('r2', 'p1')])
    })
    await settle()

    expect(
      useSidebarStore
        .getState()
        .repos.find((r) => r.id === 'r1')
        ?.folders?.map((f) => f.id),
    ).toEqual(['f1'])
  })

  it('a folder deleted locally stays gone after an unrelated rebuild once the write also bumps the signal', async () => {
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

    // What removal-commit.ts's sendRemoval does on a successful delete:
    // tombstone the row directly AND bump the signal.
    fetchFolders.mockResolvedValue([])
    act(() => {
      useSidebarStore.getState().applyFolderDTO({
        id: 'f1',
        repoId: 'r1',
        projectId: 'p1',
        name: '',
        order: 0,
        status: 'deleted',
      })
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()
    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.folders).toBeUndefined()

    // An unrelated rebuild must not bring the deleted row back.
    act(() => {
      useSidebarStore
        .getState()
        .setRepos([...useSidebarStore.getState().repos, repoRow('r2', 'p1')])
    })
    await settle()

    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.folders).toBeUndefined()
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

/**
 * The CHAT half of the same per-repo tree subscription (Task D).
 *
 * Chat rows reach `Repo.chats` — and therefore `rows-from-repo.ts`'s tree — the
 * same way folders reach `Repo.folders`: one repo-scoped GET on open, replayed
 * whenever this repo's tree generation moves. Same file, same real rebuild
 * pipeline, because the point is the same: proving the row a reseed writes
 * actually lands on the repo, not merely that a function was called.
 */
describe('chat rows on the same reseed loop', () => {
  it("seeds a repo's chats once on open, and they reach Repo.chats", async () => {
    fetchRepoChats.mockResolvedValue([chatDTO('c1', 'r1', { title: 'Fix the parser' })])

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

    expect(fetchRepoChats).toHaveBeenCalledWith('p1', 'r1')
    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.chats).toEqual([
      {
        id: 'c1',
        repoId: 'r1',
        parentId: undefined,
        workspaceId: undefined,
        title: 'Fix the parser',
        order: 0,
      },
    ])
  })

  it('a structural chat frame’s signal reseeds this repo and the new row lands', async () => {
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
    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.chats).toBeUndefined()

    // The signal use-workspace-agent-chats-stream.ts fires on created /
    // deleted / title_set / placement_set / order_set.
    fetchRepoChats.mockResolvedValue([chatDTO('c1', 'r1', { parentId: 'f1' })])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()

    expect(
      useSidebarStore
        .getState()
        .repos.find((r) => r.id === 'r1')
        ?.chats?.map((c) => [c.id, c.parentId]),
    ).toEqual([['c1', 'f1']])
  })

  it('a reseed drops a chat the fresh list no longer carries', async () => {
    fetchRepoChats.mockResolvedValue([chatDTO('c1', 'r1')])
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
        ?.chats?.map((c) => c.id),
    ).toEqual(['c1'])

    fetchRepoChats.mockResolvedValue([])
    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()

    expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.chats).toBeUndefined()
  })

  it('a folders failure does not discard chats that came back fine', async () => {
    // The two halves are read in parallel and settled independently: the next
    // signal may be a long way off, so one half's transient error must not cost
    // the other half's fresh rows.
    fetchFolders.mockRejectedValue(new Error('boom'))
    fetchRepoChats.mockResolvedValue([chatDTO('c1', 'r1')])

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
        ?.chats?.map((c) => c.id),
    ).toEqual(['c1'])
  })

  // The bug class tasks 21/22/26/34 each found a version of, checked end to end
  // through the real cache: a frame for one repo must not reseed another, and a
  // cache holding both repos' rows must not hand either the other's.
  describe('cross-repo isolation', () => {
    it('a signal for a DIFFERENT repo does not reseed this one’s chats', async () => {
      fetchRepoChats.mockResolvedValue([chatDTO('c1', 'r1')])
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
      fetchRepoChats.mockClear()

      act(() => {
        useFolderSignalStore.getState().bump('r2')
      })
      await settle()

      expect(fetchRepoChats).not.toHaveBeenCalled()
    })

    it('one repo’s chats never land on another repo, and a reseed never prunes them', async () => {
      // A SECOND real repo in the entity cache — beforeEach seeds only r1, and
      // the rebuild reads repos from there, so a bootstrap row alone would not
      // survive the first rebuild. Seeded on real timers for the same reason
      // beforeEach does: fake-indexeddb's completion rides scheduling.
      vi.useRealTimers()
      await upsertEntity('crowbar_repos', repoDTO('r2', 'p1'))
      vi.useFakeTimers()

      fetchRepoChats.mockImplementation((_projectId: string, repoId: string) =>
        Promise.resolve([chatDTO(`${repoId}-chat`, repoId)]),
      )
      render(
        <AppSyncProvider>
          <div />
        </AppSyncProvider>,
      )
      await settle()
      act(() => {
        useSidebarStore.getState().setRepos([repoRow('r1', 'p1'), repoRow('r2', 'p1')])
      })
      await settle()

      const chatsOf = (id: string) =>
        useSidebarStore
          .getState()
          .repos.find((r) => r.id === id)
          ?.chats?.map((c) => c.id)
      expect(chatsOf('r1')).toEqual(['r1-chat'])
      expect(chatsOf('r2')).toEqual(['r2-chat'])

      // r1 reseeds to nothing. Its prune is authoritative over ITS rows only —
      // wiping the whole store here is what would take r2's row with it.
      fetchRepoChats.mockImplementation((_projectId: string, repoId: string) =>
        Promise.resolve(repoId === 'r1' ? [] : [chatDTO('r2-chat', 'r2')]),
      )
      act(() => {
        useFolderSignalStore.getState().bump('r1')
      })
      await settle()

      expect(chatsOf('r1')).toBeUndefined()
      expect(chatsOf('r2')).toEqual(['r2-chat'])
    })
  })
})

/**
 * The other thing the chats half of a reseed produces: the record that this
 * repo's tree has been READ, which is what lets a consumer tell an empty chat
 * list apart from one that has not arrived (`Repo.chats`'s own "not yet"
 * contract). `rows-from-repo.ts` cannot identify a branch row without it.
 */
describe('the chats reseed records that the repo’s tree has been read', () => {
  it('marks the repo seeded once its chats come back — even when there are none', async () => {
    fetchRepoChats.mockResolvedValue([])

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

    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(true)
  })

  // Fix round 2, Critical: the flag used to be raised the moment the chats
  // FETCH resolved. That is only a cache write — `scheduleRebuild` then arms a
  // 16ms timer and awaits the workspace list before `setRepos` puts the rows in
  // the store. A render landing in that window saw the repo marked seeded while
  // `repos` still held the PRE-seed chats (no `type` at all, on any cached row
  // written before the field existed), and `rows-from-repo.ts` throws on those.
  it('does not raise the flag until the rebuilt rows are actually in the store', async () => {
    fetchRepoChats.mockResolvedValue([chatDTO('b1', 'r1', { type: 'branch', workspaceId: 'ws-1' })])

    // The state of the sidebar store AT THE INSTANT the gate opened — the only
    // thing a consumer could have read on that render.
    const opened: { repos: Repo[] | null } = { repos: null }
    const unsubscribe = useFolderSignalStore.subscribe((state) => {
      if (opened.repos === null && state.seededRepoIds.has('r1')) {
        opened.repos = useSidebarStore.getState().repos
      }
    })

    try {
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

      expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(true)
      const chats = opened.repos?.find((r) => r.id === 'r1')?.chats
      expect(chats).toEqual([expect.objectContaining({ id: 'b1', type: 'branch' })])
    } finally {
      unsubscribe()
    }
  })

  it('leaves it unseeded when the chats read FAILS — an unread tree is not an empty one', async () => {
    fetchRepoChats.mockRejectedValue(new Error('offline'))

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

    expect(useFolderSignalStore.getState().seededRepoIds.has('r1')).toBe(false)
  })
})
