import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, render, waitFor } from '@testing-library/react'

// REGRESSION (restyle v2 follow-up): GET .../workspaces mints a workspace's
// owning chat on its first read, and the engine fires the repo's chats GET in
// parallel — so the chat list can land WITHOUT the owner the workspace rows
// already name, and nothing re-reads it until an unrelated structural frame.
// The workspaces seed is the one place that knows an owner is unlisted, so it
// bumps the repo's tree signal (a real signal, never a timer).
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
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { wipeEntityCache } from '@/lib/persistence/idb'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import type { EntityChange } from '@/lib/ws/entity-stream'
import type { ChatDTO, Project, RepoDTO, WorkspaceDTO } from '@/lib/types'

interface StreamOptions {
  endpoint: string
  seed?: () => Promise<unknown>
  onChange?: (change: EntityChange) => void
  /** The teardown the engine was handed for this stream. */
  dispose?: ReturnType<typeof vi.fn>
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

function chat(id: string): ChatDTO {
  return {
    id,
    repoId: 'r1',
    projectId: 'p1',
    type: 'chat',
    workspaceId: 'ws-locked',
    ownsWorktree: true,
    parentId: '',
    title: id,
    order: 0,
  }
}

function workspace(owningChatId: string): WorkspaceDTO {
  return {
    provisioning: 'provisioned',
    id: 'ws-locked',
    repoId: 'r1',
    projectId: 'p1',
    branch: 'release',
    parentId: '',
    forkPointSha: '',
    status: 'locked',
    working: false,
    lastError: '',
    added: 0,
    deleted: 0,
    mergeStrategy: '',
    canMergeLocally: false,
    mergeConflicts: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
    owningChatId,
  }
}

/** Resolves once the engine has opened `repoId`'s tree gate — the boot reseed landed. */
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

async function bootWithWorkspaces(rows: WorkspaceDTO[]): Promise<void> {
  fetchWorkspaces.mockResolvedValue(rows)
  render(
    <AppSyncProvider>
      <div />
    </AppSyncProvider>,
  )
  await waitFor(() =>
    expect(opened.some((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')).toBe(true),
  )
  await whenTreeSeeded('r1')
  expect(fetchRepoChats).toHaveBeenCalledTimes(1)
  // The mocked stream never seeds on its own — drive the workspaces seed.
  await opened.find((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')?.seed?.()
}

beforeEach(async () => {
  vi.clearAllMocks()
  opened.length = 0
  subscribeEntityStream.mockImplementation((options: StreamOptions) => {
    const dispose = vi.fn()
    opened.push({ ...options, dispose })
    return dispose
  })
  await wipeEntityCache()
  await upsertEntity('crowbar_repos', repoDTO)
  fetchRepos.mockResolvedValue([repoDTO])
  fetchFolders.mockResolvedValue([])
  fetchRepoChats.mockResolvedValue([chat('owner-locked')])
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project]) })
  useSidebarStore.setState({ ...getInitialState(), repos: [] })
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

describe('the workspaces seed vs the chat list', () => {
  it('re-reads the chats when a workspace names an owner the list does not hold', async () => {
    await bootWithWorkspaces([workspace('owner-minted-late')])
    await waitFor(() => expect(fetchRepoChats).toHaveBeenCalledTimes(2))
  })

  // A cold cache: the boot tree read is still in flight when the workspaces
  // answer, so the cache cannot hold the owner yet — that read is the answer.
  it('judges owners against the tree read in flight, not the cache it has yet to fill', async () => {
    let answerChats: (rows: ChatDTO[]) => void = () => {}
    fetchRepoChats.mockImplementation(
      () =>
        new Promise<ChatDTO[]>((resolve) => {
          answerChats = resolve
        }),
    )
    fetchWorkspaces.mockResolvedValue([workspace('owner-locked')])
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await waitFor(() => expect(fetchRepoChats).toHaveBeenCalledTimes(1))
    const stream = opened.find((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')
    const seeded = stream?.seed?.()
    await waitFor(() => expect(fetchWorkspaces).toHaveBeenCalled())
    answerChats([chat('owner-locked')])
    await seeded
    await whenTreeSeeded('r1')
    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBe(0)
    expect(fetchRepoChats).toHaveBeenCalledTimes(1)
  })

  // The workspace route guard redirects on "read, and not there": the mark
  // must not precede the rows, or a live workspace reads as gone.
  it('marks a repo workspace list read only once its rows are in the tree', async () => {
    let rowsAtMark: string[] | undefined
    const unsubscribe = useFolderSignalStore.subscribe((state) => {
      if (rowsAtMark || !state.seededWorkspaceRepoIds.has('r1')) return
      const repo = useSidebarStore.getState().repos.find((r) => r.id === 'r1')
      rowsAtMark = repo?.workspaces.map((ws) => ws.id) ?? []
    })
    await bootWithWorkspaces([workspace('owner-locked')])
    const stream = opened.find((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')
    await upsertEntity('crowbar_workspaces', workspace('owner-locked'))
    stream?.onChange?.({ kind: 'seed' })
    await waitFor(() => expect(rowsAtMark).toBeDefined())
    unsubscribe()
    expect(rowsAtMark).toContain('ws-locked')
  })

  it('leaves the chat list alone when every owner is already listed', async () => {
    // The seed awaits its own owner check, so once it resolves any bump has landed.
    await bootWithWorkspaces([workspace('owner-locked')])
    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBe(0)
    expect(fetchRepoChats).toHaveBeenCalledTimes(1)
  })
})

describe('a repo tombstone', () => {
  // A deleted repo's scope 404s: its streams must not outlive the tombstone by
  // a grace period, reseeding against it on every cascade frame.
  it('tears the repo scoped streams down at once', async () => {
    await bootWithWorkspaces([workspace('owner-locked')])
    const streamOf = (endpoint: string) => opened.find((s) => s.endpoint === endpoint)
    act(() => {
      streamOf('/v0/projects/p1/repos')?.onChange?.({
        kind: 'frame',
        frame: { id: 'r1', status: 'deleted' },
      })
    })
    expect(streamOf('/v0/projects/p1/repos/r1/chats/ws')?.dispose).toHaveBeenCalled()
    fetchRepoChats.mockClear()
    act(() => useFolderSignalStore.getState().bump('r1'))
    expect(fetchRepoChats).not.toHaveBeenCalled()
  })

  // The daemon announces the delete (the row carries `deleting`) before it
  // tombstones the repo's chats: those frames must not re-read a repo on its
  // way out.
  it('a repo being deleted stops re-reading its tree before its chats go', async () => {
    await bootWithWorkspaces([workspace('owner-locked')])
    const streamOf = (endpoint: string) => opened.find((s) => s.endpoint === endpoint)
    const rebuild = vi.spyOn(useWorkspaceListStore.getState(), 'fetch')
    const deleting: RepoDTO = { ...repoDTO, deleting: true }
    await upsertEntity('crowbar_repos', deleting)
    act(() => {
      streamOf('/v0/projects/p1/repos')?.onChange?.({ kind: 'frame', frame: deleting })
    })
    expect(streamOf('/v0/projects/p1/repos/r1/chats/ws')?.dispose).toHaveBeenCalled()
    // The rebuild the frame armed reads the same row back and keeps it closed.
    await waitFor(() => expect(rebuild).toHaveBeenCalled())
    await act(() => rebuild.mock.results[0]?.value)
    fetchRepoChats.mockClear()
    act(() => useFolderSignalStore.getState().bump('r1'))
    expect(fetchRepoChats).not.toHaveBeenCalled()
    expect(opened.filter((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')).toHaveLength(1)
  })

  // A delete that stopped (the row comes back with lastError) is a live repo again.
  it('a repo whose delete stopped streams again', async () => {
    await bootWithWorkspaces([workspace('owner-locked')])
    const repos = opened.find((s) => s.endpoint === '/v0/projects/p1/repos')!
    const deleting: RepoDTO = { ...repoDTO, deleting: true }
    act(() => {
      repos.onChange?.({ kind: 'frame', frame: deleting })
    })
    const stopped: RepoDTO = { ...deleting, lastError: 'work at risk' }
    await upsertEntity('crowbar_repos', stopped)
    act(() => {
      repos.onChange?.({ kind: 'frame', frame: stopped })
    })
    await waitFor(() =>
      expect(opened.filter((s) => s.endpoint === '/v0/projects/p1/repos/r1/chats/ws')).toHaveLength(
        2,
      ),
    )
  })
})
