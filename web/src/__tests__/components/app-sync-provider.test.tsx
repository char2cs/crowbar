import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor, act } from '@testing-library/react'

// §7 startup is driven by GET seeds + subscribeEntityStream subscriptions and a
// one-time maybeWipeOnVersionChange() BEFORE seeding. We mock those seams and
// assert the provider wires them up and tears every subscription down on unmount.
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

// Only the network seams are faked — `workspaceDTOFromWorktreeFrame` (the chat
// stream's frame mapper) stays REAL, so a test that reaches for it gets the
// mapping the provider actually installs.
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  fetchRepos: (...args: unknown[]) => fetchRepos(...args),
  fetchWorkspaces: (...args: unknown[]) => fetchWorkspaces(...args),
  fetchFolders: (...args: unknown[]) => fetchFolders(...args),
  fetchRepoChats: (...args: unknown[]) => fetchRepoChats(...args),
  fetchProjects: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue(null),
}))

import { AppSyncProvider, SUBSCRIPTION_GRACE_MS } from '@/components/app-sync-provider'
import { idle, success } from '@/lib/loadable'
import { useProjectStore } from '@/lib/store/projects'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { EntityChange } from '@/lib/ws/entity-stream'
import type { Project, WorkspaceDTO } from '@/lib/types'

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date(0),
})

interface StreamOptions {
  endpoint: string
  onChange?: (change: EntityChange) => void
}

/** Every stream opened this test, in order, with its own unsubscribe spy. */
const opened: Array<{ options: StreamOptions; unsubscribe: ReturnType<typeof vi.fn> }> = []

const endpoints = (): string[] => opened.map((s) => s.options.endpoint)
const streamFor = (endpoint: string) => opened.find((s) => s.options.endpoint === endpoint)
const liveEndpoints = (): string[] =>
  opened.filter((s) => s.unsubscribe.mock.calls.length === 0).map((s) => s.options.endpoint)

function repo(id: string, projectId: string, overrides: Partial<Repo> = {}): Repo {
  return {
    id,
    projectId,
    name: id,
    avatarLabel: id[0].toUpperCase(),
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...overrides,
  }
}

function wsDTO(id: string, repoId: string, overrides: Partial<WorkspaceDTO> = {}): WorkspaceDTO {
  return {
    id,
    repoId,
    projectId: 'p1',
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
    ...overrides,
  }
}

/** Let the provider's queued work (seed promises + its one-frame rebuild batch) settle. */
async function settle(ms = 20): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  opened.length = 0
  subscribeEntityStream.mockImplementation((options: StreamOptions) => {
    const unsubscribe = vi.fn()
    opened.push({ options, unsubscribe })
    return unsubscribe
  })
  fetchRepos.mockResolvedValue([])
  fetchWorkspaces.mockResolvedValue([])
  fetchFolders.mockResolvedValue([])
  fetchRepoChats.mockResolvedValue([])
  useProjectStore.setState({ activeProjectId: 'p1' })
  // Two KNOWN projects. Visibility is now "every known project minus the folded
  // ones", so p2 starts open and a `toggleProject` folds it away.
  useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
  useSidebarStore.setState({
    repos: [],
    collapsedRepos: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
  useFolderSignalStore.setState({ generations: {} })
  vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
  vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
  vi.spyOn(useWorkspaceListStore.getState(), 'fetch').mockResolvedValue(undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('AppSyncProvider §7 startup', () => {
  it('wipes the cache on version change BEFORE seeding, then seeds projects', async () => {
    render(
      <AppSyncProvider>
        <div>child</div>
      </AppSyncProvider>,
    )
    await waitFor(() => expect(wipe).toHaveBeenCalledOnce())
    await waitFor(() => expect(useProjectDataStore.getState().fetch).toHaveBeenCalled())
  })

  it("subscribes the active project's repos entity stream", async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(endpoints()).toContain('/v0/projects/p1/repos'))
  })

  it('subscribes the active project repos stream when the project becomes active AFTER mount (first-run/OOBE)', async () => {
    // Fresh start: the provider mounts at the root before any project exists, so
    // activeProjectId is empty AND the project list is still idle. The §7 startup
    // must still (re)subscribe the active project's repos/workspaces once the user
    // imports their first project — otherwise the entity cache is never populated
    // and the sidebar stays empty.
    useProjectDataStore.setState({ data: idle() })
    useProjectStore.setState({ activeProjectId: '' })
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(useProjectDataStore.getState().fetch).toHaveBeenCalled())
    // No repos stream yet — there is no active project.
    expect(endpoints().some((e) => e.endsWith('/repos'))).toBe(false)

    // The user imports their first project after mount.
    act(() => {
      useProjectStore.setState({ activeProjectId: 'p-late' })
    })

    await waitFor(() => expect(endpoints()).toContain('/v0/projects/p-late/repos'))
  })

  it('tears every subscription down on unmount', async () => {
    const { unmount } = render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(opened.length).toBeGreaterThan(0))
    unmount()
    expect(opened.every((s) => s.unsubscribe.mock.calls.length === 1)).toBe(true)
  })
})

describe('AppSyncProvider subscribes by visibility', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('subscribes every KNOWN project at boot, because every project is open', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    expect(endpoints()).toContain('/v0/projects/p1/repos')
    expect(endpoints()).toContain('/v0/projects/p2/repos')
  })

  it('does not subscribe a project that has been folded away', async () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['p2']) })
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    expect(endpoints()).toContain('/v0/projects/p1/repos')
    expect(endpoints()).not.toContain('/v0/projects/p2/repos')
  })

  it('re-opening a project subscribes exactly one repo stream', async () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['p2']) })
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    const before = endpoints().length

    act(() => {
      useSidebarStore.getState().toggleProject('p2')
    })
    await settle()

    expect(endpoints()).toContain('/v0/projects/p2/repos')
    // Exactly one new stream: the project's repo list. Its per-repo workspace
    // streams only follow once repos actually land in the tree.
    expect(endpoints().length).toBe(before + 1)
    // ...and the already-open streams were left strictly alone.
    expect(streamFor('/v0/projects/p1/repos')!.unsubscribe).not.toHaveBeenCalled()
  })

  it('a project the list has not delivered yet costs no stream at all', async () => {
    // Open-by-default must not mean "subscribe the world": a project is only
    // visible once /v0/projects has actually delivered its row.
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    expect(endpoints()).not.toContain('/v0/projects/p3/repos')

    act(() => {
      useProjectDataStore.setState({
        data: success([project('p1'), project('p2'), project('p3')]),
      })
    })
    await settle()

    expect(endpoints()).toContain('/v0/projects/p3/repos')
  })

  it('collapsing a project tears its stream down — but only after the grace period', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    const p2 = streamFor('/v0/projects/p2/repos')!

    act(() => {
      useSidebarStore.getState().toggleProject('p2')
    })
    await settle()
    // Still live: a collapse/expand tap must not thrash the socket.
    expect(p2.unsubscribe).not.toHaveBeenCalled()

    await settle(SUBSCRIPTION_GRACE_MS)
    expect(p2.unsubscribe).toHaveBeenCalledOnce()
    // The other projects' streams are untouched — subscriptions are keyed.
    expect(streamFor('/v0/projects/p1/repos')!.unsubscribe).not.toHaveBeenCalled()
  })

  it('does not rebuild the cached tree just because a project or repo closed', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1')])
    })
    await settle()
    const rebuild = useWorkspaceListStore.getState().fetch as ReturnType<typeof vi.fn>
    rebuild.mockClear()

    act(() => {
      useSidebarStore.getState().toggleRepo('r1')
      useSidebarStore.getState().toggleProject('p2')
    })
    await settle()

    expect(rebuild).not.toHaveBeenCalled()
  })

  it('re-opening inside the grace period keeps the same stream (no reseed)', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    const openedCount = endpoints().filter((e) => e === '/v0/projects/p2/repos').length

    act(() => {
      useSidebarStore.getState().toggleProject('p2')
    })
    await settle()
    act(() => {
      useSidebarStore.getState().toggleProject('p2')
    })
    await settle(SUBSCRIPTION_GRACE_MS * 2)

    expect(streamFor('/v0/projects/p2/repos')!.unsubscribe).not.toHaveBeenCalled()
    expect(endpoints().filter((e) => e === '/v0/projects/p2/repos').length).toBe(openedCount)
  })

  // A repo's worktrees ride its CHAT stream now — a worktree is held by a chat,
  // so `.../chats/ws` is where its `worktree_state` frames arrive; there is no
  // `.../workspaces` stream left to open.
  it("subscribes a visible project's repo worktree streams, and not a collapsed repo's", async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()

    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1'), repo('r2', 'p1')])
    })
    await settle()
    expect(endpoints()).toContain('/v0/projects/p1/repos/r1/chats/ws')
    expect(endpoints()).toContain('/v0/projects/p1/repos/r2/chats/ws')

    act(() => {
      useSidebarStore.getState().toggleRepo('r2')
    })
    await settle(SUBSCRIPTION_GRACE_MS)

    expect(liveEndpoints()).toContain('/v0/projects/p1/repos/r1/chats/ws')
    expect(liveEndpoints()).not.toContain('/v0/projects/p1/repos/r2/chats/ws')
  })

  // Task 34: folders no longer open a WS subscription at all (their dedicated
  // REST+WS resource is gone) — they fetch once on open and again whenever
  // useFolderSignalStore's per-repo generation moves. "Subscribed" therefore
  // means "reacts to that repo's signal", checked by bumping it and watching
  // fetchFolders fire (or not). The reseed mechanism itself is covered in
  // app-sync-provider-folders.test.tsx; these tests only cover WHEN it is
  // wired up — the same visibility rule the workspace streams already prove.
  it("fetches an expanded repo's folders, and stops reacting to its signal once it collapses", async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()

    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1'), repo('r2', 'p1')])
    })
    await settle()
    expect(fetchFolders).toHaveBeenCalledWith('p1', 'r1')
    expect(fetchFolders).toHaveBeenCalledWith('p1', 'r2')

    act(() => {
      useSidebarStore.getState().toggleRepo('r2')
    })
    await settle(SUBSCRIPTION_GRACE_MS)
    fetchFolders.mockClear()

    act(() => {
      useFolderSignalStore.getState().bump('r1')
      useFolderSignalStore.getState().bump('r2')
    })
    await settle()

    expect(fetchFolders).toHaveBeenCalledWith('p1', 'r1')
    expect(fetchFolders).not.toHaveBeenCalledWith('p1', 'r2')
  })

  it('costs nothing for a repo nobody has expanded yet', async () => {
    // Folders follow the lazy rule the workspace streams established: cost is
    // proportional to what is on screen, so a collapsed repo is never fetched
    // at all.
    useSidebarStore.setState({ collapsedRepos: new Set(['r1']) })
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1')])
    })
    await settle()
    expect(fetchFolders).not.toHaveBeenCalledWith('p1', 'r1')
  })

  it('does not keep reacting to a collapsed repo’s folder signal for a working agent', async () => {
    // The workspace stream stays up so the avatar spinner can stop; a folder
    // reports nothing live, so it has no reason to.
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1', { defaultWorking: true })])
      useSidebarStore.getState().toggleRepo('r1')
    })
    await settle(SUBSCRIPTION_GRACE_MS)

    expect(liveEndpoints()).toContain('/v0/projects/p1/repos/r1/chats/ws')
    fetchFolders.mockClear()

    act(() => {
      useFolderSignalStore.getState().bump('r1')
    })
    await settle()
    expect(fetchFolders).not.toHaveBeenCalled()
  })

  it('keeps a collapsed repo subscribed while its repo-home agent is working', async () => {
    // `defaultWorking` is the one live signal a collapsed repo still renders
    // (the spinner on its avatar). Dropping the stream mid-turn would freeze it
    // on forever, which is worse than not showing it at all.
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1', { defaultWorking: true })])
      useSidebarStore.getState().toggleRepo('r1')
    })
    await settle(SUBSCRIPTION_GRACE_MS)
    expect(liveEndpoints()).toContain('/v0/projects/p1/repos/r1/chats/ws')

    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1', { defaultWorking: false })])
    })
    await settle(SUBSCRIPTION_GRACE_MS)
    expect(liveEndpoints()).not.toContain('/v0/projects/p1/repos/r1/chats/ws')
  })
})

describe('AppSyncProvider merges frames incrementally', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it('merges a workspace frame by id without rebuilding the whole tree', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1')])
    })
    await settle()

    const workspaces = streamFor('/v0/projects/p1/repos/r1/chats/ws')!
    const rebuild = useWorkspaceListStore.getState().fetch as ReturnType<typeof vi.fn>
    rebuild.mockClear()

    act(() => {
      workspaces.options.onChange!({ kind: 'frame', frame: wsDTO('w1', 'r1') })
    })
    await settle()

    // The row is in the tree...
    expect(useSidebarStore.getState().repos[0].workspaces.map((w) => w.id)).toEqual(['w1'])
    // ...and no whole-tree rebuild was triggered to put it there.
    expect(rebuild).not.toHaveBeenCalled()
  })

  it('applies a workspace tombstone incrementally', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore
        .getState()
        .setRepos([repo('r1', 'p1', { workspaces: [{ id: 'w1', branch: 'feature/w1', age: '' }] })])
    })
    await settle()

    const workspaces = streamFor('/v0/projects/p1/repos/r1/chats/ws')!
    const rebuild = useWorkspaceListStore.getState().fetch as ReturnType<typeof vi.fn>
    rebuild.mockClear()

    act(() => {
      workspaces.options.onChange!({
        kind: 'frame',
        frame: wsDTO('w1', 'r1', { status: 'deleted' }),
      })
    })
    await settle()

    expect(useSidebarStore.getState().repos[0].workspaces).toEqual([])
    expect(rebuild).not.toHaveBeenCalled()
  })

  // Folders no longer carry a live per-DTO push frame at all (Task 34 — the
  // backend's dedicated folders resource is gone) — there is nothing left to
  // "merge by id" or "no-op" incrementally the way a workspace frame does.
  // Every folder change is a full reseed instead; that mechanism (seed on
  // open, reseed on useFolderSignalStore's per-repo signal, tombstones
  // dropped by diffing the fresh list against the cache) is covered in
  // app-sync-provider-folders.test.tsx, alongside this file's own
  // subscribes-by-visibility folders coverage above.

  it('still rebuilds from the cache on a seed (which is authoritative and prunes)', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1')])
    })
    await settle()

    const workspaces = streamFor('/v0/projects/p1/repos/r1/chats/ws')!
    const rebuild = useWorkspaceListStore.getState().fetch as ReturnType<typeof vi.fn>
    rebuild.mockClear()

    act(() => {
      workspaces.options.onChange!({ kind: 'seed' })
    })
    await settle()

    expect(rebuild).toHaveBeenCalled()
  })

  it('coalesces a burst of seeds into a single rebuild', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo('r1', 'p1'), repo('r2', 'p1')])
    })
    await settle()

    const rebuild = useWorkspaceListStore.getState().fetch as ReturnType<typeof vi.fn>
    rebuild.mockClear()

    act(() => {
      streamFor('/v0/projects/p1/repos/r1/chats/ws')!.options.onChange!({ kind: 'seed' })
      streamFor('/v0/projects/p1/repos/r2/chats/ws')!.options.onChange!({ kind: 'seed' })
      streamFor('/v0/projects/p1/repos')!.options.onChange!({ kind: 'seed' })
    })
    await settle()

    expect(rebuild).toHaveBeenCalledTimes(1)
  })
})
