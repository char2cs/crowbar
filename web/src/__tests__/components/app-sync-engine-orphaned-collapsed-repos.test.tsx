import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, act, screen } from '@testing-library/react'

// REGRESSION (restyle v2): a `collapsedRepos` set the previous build persisted
// (every repo but the one you were last in) used to gate the sync engine's
// per-repo TREE subscription (app-sync-engine.ts `desiredKeys`, `showsRows`),
// but the restyled sidebar never wrote that set — it folds rows through
// `collapsedChatRows` (sidebar-tree.tsx). So on the first launch over an
// existing profile every repo the old sidebar had collapsed rendered EXPANDED
// (header + branch rows off the one-shot workspace GET) while its
// chats/folders were never read: no thread rows, no folders, no live frames,
// and no way in the UI to ever un-collapse it. The field is gone from the
// store now (hydrate.ts ignores the retired key); this pins that a repo whose
// restyled fold set is empty streams its whole tree.
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

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => [],
  subscribeChatWorking: () => () => {},
  readChatWorking: () => false,
}))

import { AppSyncProvider } from '@/components/app-sync-provider'
import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { success } from '@/lib/loadable'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import type { Project } from '@/lib/types'

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date(0),
})

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: 'chat-main',
  defaultWorkspaceStatus: 'locked',
  workspaces: [
    {
      id: 'ws-feat',
      branch: 'feat/x',
      age: '',
      order: 0,
      status: 'new',
      owningChatId: 'chat-feat',
    },
  ],
  chats: [
    {
      id: 'chat-main',
      repoId: 'r1',
      title: '',
      order: 0,
      ownsWorktree: true,
      workspaceId: 'ws-main',
    },
    {
      id: 'chat-feat',
      repoId: 'r1',
      title: 'Feature',
      order: 0,
      ownsWorktree: true,
      workspaceId: 'ws-feat',
    },
  ],
}

async function settle(ms = 20): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  subscribeEntityStream.mockImplementation(() => vi.fn())
  fetchRepos.mockResolvedValue([])
  fetchWorkspaces.mockResolvedValue([])
  fetchFolders.mockResolvedValue([])
  fetchRepoChats.mockResolvedValue([])
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project('p1')]) })
  // The restyled tree's own fold set is empty — nothing on screen is folded.
  useSidebarStore.setState({
    repos: [],
    collapsedChatRows: new Set<string>(),
  })
  useFolderSignalStore.setState({
    generations: {},
    seededRepoIds: new Set(),
    seededWorkspaceRepoIds: new Set(),
  })
  vi.spyOn(useProjectDataStore.getState(), 'fetch').mockResolvedValue(undefined)
  vi.spyOn(useProjectDataStore.getState(), 'startSync').mockReturnValue(() => {})
  vi.spyOn(useWorkspaceListStore.getState(), 'fetch').mockResolvedValue(undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('a repo the OLD build persisted as collapsed', () => {
  it('is drawn fully expanded by the restyled tree (its fold state lives in collapsedChatRows)', () => {
    render(
      <SidebarTree
        rows={rowsFromRepo(repo)}
        onOpen={() => {}}
        onTrash={() => {}}
        onCreate={() => {}}
        scrollRef={{ current: null }}
        onDrop={() => {}}
        onPaneDrop={() => {}}
      />,
    )
    // Header AND its branch row are on screen: nothing hides this repo's body.
    expect(screen.getByText('repo-alpha')).toBeInTheDocument()
    expect(screen.getByText('Feature')).toBeInTheDocument()
  })

  it('still gets its chats and folders read by the sync engine, exactly like an expanded repo', async () => {
    render(
      <AppSyncProvider>
        <div />
      </AppSyncProvider>,
    )
    await settle()
    act(() => {
      useSidebarStore.getState().setRepos([repo])
    })
    await settle()
    const endpoints = subscribeEntityStream.mock.calls.map(
      (call) => (call[0] as { endpoint: string }).endpoint,
    )
    expect(endpoints).toContain('/v0/projects/p1/repos/r1/chats/ws')
    // The tree rows the SAME expanded repo draws are read too: no persisted
    // entry from the previous build can hide a repo's threads for good.
    expect(fetchRepoChats).toHaveBeenCalledWith('p1', 'r1')
    expect(fetchFolders).toHaveBeenCalledWith('p1', 'r1')
  })
})
